//go:build integration

package repository

import (
	"context"
	"fmt"
	"sync"
	"testing"

	dbent "github.com/Wei-Shaw/sub2api/ent"
	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/stretchr/testify/require"
	"github.com/stretchr/testify/suite"
)

// 学生奖励发放的数据库级语义验证：
// 幂等（唯一约束）、事务原子性（发放记录 + 余额同生共死）、充值口径隔离、余额历史来源。
type RewardGrantServiceSuite struct {
	IntegrationDBSuite
	settingService *service.SettingService
	rewardRepo     service.RewardGrantRepository
	userRepo       service.UserRepository
	svc            *service.RewardGrantService
}

func (s *RewardGrantServiceSuite) SetupTest() {
	s.IntegrationDBSuite.SetupTest()
	settingRepo := NewSettingRepository(s.client)
	s.settingService = service.NewSettingService(settingRepo, &config.Config{})
	s.rewardRepo = NewRewardGrantRepository(s.client)
	s.userRepo = NewUserRepository(s.client, integrationDB)
	s.svc = service.NewRewardGrantService(s.client, s.rewardRepo, s.userRepo, s.settingService, nil, nil)
}

// txCtx 让 Reward Service 内联运行在 suite 事务里（随测试自动回滚）。
func (s *RewardGrantServiceSuite) txCtx() context.Context {
	return dbent.NewTxContext(s.ctx, s.tx)
}

func (s *RewardGrantServiceSuite) setRewardSettings(enabled bool, amount string, campaign string) {
	ctx := s.txCtx()
	for key, value := range map[string]string{
		"student_verification_reward_enabled":  fmt.Sprintf("%t", enabled),
		"student_verification_reward_amount":   amount,
		"student_verification_reward_campaign": campaign,
	} {
		_, err := s.client.ExecContext(ctx,
			`UPDATE settings SET value = $1, updated_at = NOW() WHERE key = $2`, value, key)
		s.RequireNoError(err, "update setting "+key)
	}
}

func (s *RewardGrantServiceSuite) createUserWithBalance(balance float64) *service.User {
	// Email 留空由 fixture 生成 RFC3339Nano 唯一地址
	return mustCreateUser(s.T(), s.client, &service.User{Balance: balance})
}

func (s *RewardGrantServiceSuite) queryScalar(query string, args ...any) (float64, bool) {
	rows, err := s.client.QueryContext(s.txCtx(), query, args...)
	s.RequireNoError(err)
	defer func() { _ = rows.Close() }()
	if !rows.Next() {
		s.RequireNoError(rows.Err())
		return 0, false
	}
	var v float64
	s.RequireNoError(rows.Scan(&v))
	return v, true
}

func (s *RewardGrantServiceSuite) rewardCount(userID int64) int64 {
	v, ok := s.queryScalar(`SELECT COUNT(*) FROM reward_grants WHERE user_id = $1`, userID)
	s.True(ok)
	return int64(v)
}

func TestRewardGrantServiceSuite(t *testing.T) {
	suite.Run(t, new(RewardGrantServiceSuite))
}

// 首次发放：balance 10 + 奖励 20 = 30，total_recharged 不变，无伪兑换码。
func (s *RewardGrantServiceSuite) TestFirstGrant() {
	s.setRewardSettings(true, "20.00000000", "2026_fall")
	user := s.createUserWithBalance(10)
	verificationID := int64(887)

	result, err := s.svc.GrantStudentVerificationReward(s.txCtx(), user.ID, verificationID, nil)
	s.RequireNoError(err)
	s.Equal(service.RewardGrantStatusGranted, result.Status)
	s.True(result.Granted())
	s.NotNil(result.Grant)
	s.Equal(verificationID, *result.Grant.SourceID)
	s.Equal("2026_fall", result.Grant.Campaign)

	balance, ok := s.queryScalar(`SELECT balance::double precision FROM users WHERE id = $1`, user.ID)
	s.True(ok)
	s.InDelta(30.0, balance, 1e-8)

	totalRecharged, ok := s.queryScalar(`SELECT total_recharged::double precision FROM users WHERE id = $1`, user.ID)
	s.True(ok)
	s.InDelta(0.0, totalRecharged, 1e-8)

	s.Equal(int64(1), s.rewardCount(user.ID))

	redeemRows, ok := s.queryScalar(`SELECT COUNT(*) FROM redeem_codes WHERE used_by = $1`, user.ID)
	s.True(ok)
	s.InDelta(0.0, redeemRows, 1e-8)
}

// 重复调用 10 次（同 user / 同 source_type / 同 campaign）：只发一次。
func (s *RewardGrantServiceSuite) TestRepeatedGrantsIdempotent() {
	s.setRewardSettings(true, "20.00000000", "2026_fall")
	user := s.createUserWithBalance(10)

	var grantedCount int
	for i := 0; i < 10; i++ {
		result, err := s.svc.GrantStudentVerificationReward(s.txCtx(), user.ID, 887, nil)
		s.RequireNoError(err)
		s.True(result.Granted(), "every retry is a business-level success")
		if result.Status == service.RewardGrantStatusGranted {
			grantedCount++
		} else {
			s.Equal(service.RewardGrantStatusAlreadyGranted, result.Status)
		}
	}
	s.Equal(1, grantedCount)

	balance, ok := s.queryScalar(`SELECT balance::double precision FROM users WHERE id = $1`, user.ID)
	s.True(ok)
	s.InDelta(30.0, balance, 1e-8)
	s.Equal(int64(1), s.rewardCount(user.ID))
}

// 配置关闭 / 配置不完整：不建记录、不加余额。
func (s *RewardGrantServiceSuite) TestDisabledAndInvalidConfig() {
	// disabled
	s.setRewardSettings(false, "20.00000000", "2026_fall")
	user := s.createUserWithBalance(10)
	result, err := s.svc.GrantStudentVerificationReward(s.txCtx(), user.ID, 887, nil)
	s.RequireNoError(err)
	s.Equal(service.RewardGrantStatusDisabled, result.Status)
	s.False(result.Granted())
	s.Equal(int64(0), s.rewardCount(user.ID))

	// invalid config: enabled but amount = 0
	s.setRewardSettings(true, "0.00000000", "2026_fall")
	result, err = s.svc.GrantStudentVerificationReward(s.txCtx(), user.ID, 887, nil)
	s.RequireNoError(err)
	s.Equal(service.RewardGrantStatusInvalidConfig, result.Status)

	// invalid config: enabled but empty campaign
	s.setRewardSettings(true, "20.00000000", "")
	result, err = s.svc.GrantStudentVerificationReward(s.txCtx(), user.ID, 887, nil)
	s.RequireNoError(err)
	s.Equal(service.RewardGrantStatusInvalidConfig, result.Status)

	balance, ok := s.queryScalar(`SELECT balance::double precision FROM users WHERE id = $1`, user.ID)
	s.True(ok)
	s.InDelta(10.0, balance, 1e-8)
	s.Equal(int64(0), s.rewardCount(user.ID))
}

// Campaign 切换：2026_fall 发过后，2027_spring 可再次发放（底层能力）。
func (s *RewardGrantServiceSuite) TestCampaignSwitchAllowsNewGrant() {
	s.setRewardSettings(true, "20.00000000", "2026_fall")
	user := s.createUserWithBalance(10)

	result, err := s.svc.GrantStudentVerificationReward(s.txCtx(), user.ID, 887, nil)
	s.RequireNoError(err)
	s.Equal(service.RewardGrantStatusGranted, result.Status)

	s.setRewardSettings(true, "20.00000000", "2027_spring")
	result, err = s.svc.GrantStudentVerificationReward(s.txCtx(), user.ID, 887, nil)
	s.RequireNoError(err)
	s.Equal(service.RewardGrantStatusGranted, result.Status)

	balance, ok := s.queryScalar(`SELECT balance::double precision FROM users WHERE id = $1`, user.ID)
	s.True(ok)
	s.InDelta(50.0, balance, 1e-8)
	s.Equal(int64(2), s.rewardCount(user.ID))
}

// 通用 Reward 键语义：同 campaign、不同 idempotency_key → 允许多笔；
// 同 idempotency_key → 永远只有一笔（campaign 不再是唯一约束维度）。
func (s *RewardGrantServiceSuite) TestGenericRewardKeySemantics() {
	user := s.createUserWithBalance(0)

	// 同 campaign，两个不同业务事件（不同 key）→ 两笔都成功
	cmdA := service.GrantRewardCommand{
		UserID:         user.ID,
		IdempotencyKey: fmt.Sprintf("campaign_grant:event-1001:%d", user.ID),
		SourceType:     "campaign_grant",
		Campaign:       "spring_festival",
		Amount:         5,
	}
	resultA, err := s.svc.GrantReward(s.txCtx(), cmdA)
	s.RequireNoError(err)
	s.Equal(service.RewardGrantStatusGranted, resultA.Status)

	cmdB := cmdA
	cmdB.IdempotencyKey = fmt.Sprintf("campaign_grant:event-1002:%d", user.ID)
	resultB, err := s.svc.GrantReward(s.txCtx(), cmdB)
	s.RequireNoError(err)
	s.Equal(service.RewardGrantStatusGranted, resultB.Status)

	balance, ok := s.queryScalar(`SELECT balance::double precision FROM users WHERE id = $1`, user.ID)
	s.True(ok)
	s.InDelta(10.0, balance, 1e-8)

	// 同 key 重放 → already_granted，绝不再加钱
	resultA2, err := s.svc.GrantReward(s.txCtx(), cmdA)
	s.RequireNoError(err)
	s.Equal(service.RewardGrantStatusAlreadyGranted, resultA2.Status)
	s.NotNil(resultA2.Grant)
	s.Equal(cmdA.IdempotencyKey, resultA2.Grant.IdempotencyKey)

	balanceAfterReplay, ok := s.queryScalar(`SELECT balance::double precision FROM users WHERE id = $1`, user.ID)
	s.True(ok)
	s.InDelta(10.0, balanceAfterReplay, 1e-8)

	s.Equal(int64(2), s.rewardCount(user.ID))

	byCampaign, err := s.rewardRepo.GetByUserSourceCampaign(s.txCtx(), user.ID, "campaign_grant", "spring_festival")
	s.RequireNoError(err)
	s.Len(byCampaign, 2, "same campaign may hold multiple grants with distinct keys")
}

// 并发撞同一个通用 key：20 goroutine 只有一笔（独立事务路径，全局 client）。
func TestGenericRewardConcurrentSameKey(t *testing.T) {
	ctx := context.Background()
	client := testEntClient(t)
	userRepo := NewUserRepository(client, integrationDB)
	svc := service.NewRewardGrantService(client, NewRewardGrantRepository(client), userRepo,
		service.NewSettingService(NewSettingRepository(client), &config.Config{}), nil, nil)

	user := mustCreateUser(t, client, &service.User{Balance: 0})

	const goroutines = 20
	results := make(chan service.RewardGrantResult, goroutines)
	errCh := make(chan error, goroutines)
	var wg sync.WaitGroup
	for i := 0; i < goroutines; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			result, err := svc.GrantReward(ctx, service.GrantRewardCommand{
				UserID:         user.ID,
				IdempotencyKey: "compensation:op-9001",
				SourceType:     "admin_compensation",
				Campaign:       "ops_2026",
				Amount:         3,
			})
			if err != nil {
				errCh <- err
				return
			}
			results <- *result
		}()
	}
	wg.Wait()
	close(results)
	close(errCh)

	for err := range errCh {
		require.NoError(t, err)
	}
	granted := 0
	for result := range results {
		if result.Status == service.RewardGrantStatusGranted {
			granted++
		} else {
			require.Equal(t, service.RewardGrantStatusAlreadyGranted, result.Status)
		}
	}
	require.Equal(t, 1, granted)

	var count int64
	require.NoError(t, integrationDB.QueryRow(
		`SELECT COUNT(*) FROM reward_grants WHERE idempotency_key = $1`, "compensation:op-9001").Scan(&count))
	require.EqualValues(t, 1, count)

	var balance float64
	require.NoError(t, integrationDB.QueryRow(
		`SELECT balance::double precision FROM users WHERE id = $1`, user.ID).Scan(&balance))
	require.InDelta(t, 3.0, balance, 1e-8)
}

// 余额历史能力：ListByUser 可读、SumPositiveBalanceByUser 口径不含奖励。
func (s *RewardGrantServiceSuite) TestRewardVisibleInQueriesWithoutRechargeSemantics() {
	s.setRewardSettings(true, "20.00000000", "2026_fall")
	user := s.createUserWithBalance(10)

	_, err := s.svc.GrantStudentVerificationReward(s.txCtx(), user.ID, 887, nil)
	s.RequireNoError(err)

	grants, err := s.rewardRepo.ListByUser(s.txCtx(), user.ID, 100)
	s.RequireNoError(err)
	s.Len(grants, 1)
	s.Equal("2026_fall", grants[0].Campaign)
	s.Equal("student_verification:"+fmt.Sprintf("%d", user.ID)+":2026_fall", grants[0].IdempotencyKey)

	byKey, err := s.rewardRepo.GetByIdempotencyKey(s.txCtx(), grants[0].IdempotencyKey)
	s.RequireNoError(err)
	s.NotNil(byKey)
	s.Equal(grants[0].ID, byKey.ID)

	bySource, err := s.rewardRepo.GetBySource(s.txCtx(), service.RewardSourceStudentVerification, 887)
	s.RequireNoError(err)
	s.Len(bySource, 1)

	redeemRepo := NewRedeemCodeRepository(s.client)
	sum, err := redeemRepo.SumPositiveBalanceByUser(s.txCtx(), user.ID)
	s.RequireNoError(err)
	s.InDelta(0.0, sum, 1e-8)
}

// enableRewardSettingsGlobal 直接更新真库 settings 行（迁移默认关闭），
// 供走"服务自开事务"路径的独立测试启用奖励。
func enableRewardSettingsGlobal(t *testing.T, campaign string) {
	t.Helper()
	for key, value := range map[string]string{
		"student_verification_reward_enabled":  "true",
		"student_verification_reward_amount":   "20.00000000",
		"student_verification_reward_campaign": campaign,
	} {
		_, err := integrationDB.Exec(
			`UPDATE settings SET value = $1, updated_at = NOW() WHERE key = $2`, value, key)
		require.NoError(t, err, "update setting "+key)
	}
}

// 并发：20 个 goroutine 同发一笔，只能有一次 granted、余额只加一次。
func TestRewardGrantConcurrentSingleCredit(t *testing.T) {
	ctx := context.Background()
	client := testEntClient(t)
	enableRewardSettingsGlobal(t, "2026_fall")
	settingService := service.NewSettingService(NewSettingRepository(client), &config.Config{})
	userRepo := NewUserRepository(client, integrationDB)
	svc := service.NewRewardGrantService(client, NewRewardGrantRepository(client), userRepo, settingService, nil, nil)

	user := mustCreateUser(t, client, &service.User{Balance: 10})

	const goroutines = 20
	results := make(chan service.RewardGrantResult, goroutines)
	errCh := make(chan error, goroutines)
	var wg sync.WaitGroup
	for i := 0; i < goroutines; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			result, err := svc.GrantStudentVerificationReward(ctx, user.ID, 887, nil)
			if err != nil {
				errCh <- err
				return
			}
			results <- *result
		}()
	}
	wg.Wait()
	close(results)
	close(errCh)

	for err := range errCh {
		require.NoError(t, err)
	}
	granted, alreadyGranted := 0, 0
	for result := range results {
		switch result.Status {
		case service.RewardGrantStatusGranted:
			granted++
		case service.RewardGrantStatusAlreadyGranted:
			alreadyGranted++
		default:
			t.Fatalf("unexpected status: %s", result.Status)
		}
	}
	require.Equal(t, 1, granted, "exactly one goroutine should actually grant")
	require.Equal(t, goroutines-1, alreadyGranted)

	var balance float64
	require.NoError(t, integrationDB.QueryRow(
		`SELECT balance::double precision FROM users WHERE id = $1`, user.ID).Scan(&balance))
	require.InDelta(t, 30.0, balance, 1e-8)

	var count int64
	require.NoError(t, integrationDB.QueryRow(
		`SELECT COUNT(*) FROM reward_grants WHERE user_id = $1`, user.ID).Scan(&count))
	require.EqualValues(t, 1, count)
}

// 事务原子性：余额加失败时，发放记录必须随之回滚（不允许"有记录没加钱"）。
func TestRewardGrantRollsBackWhenBalanceFails(t *testing.T) {
	ctx := context.Background()
	client := testEntClient(t)
	enableRewardSettingsGlobal(t, "2026_fall")
	settingService := service.NewSettingService(NewSettingRepository(client), &config.Config{})
	failingUserRepo := &failingAdjustUserRepo{UserRepository: NewUserRepository(client, integrationDB)}
	svc := service.NewRewardGrantService(client, NewRewardGrantRepository(client), failingUserRepo, settingService, nil, nil)

	user := mustCreateUser(t, client, &service.User{Balance: 10})
	failingUserRepo.failOn = user.ID

	result, err := svc.GrantStudentVerificationReward(ctx, user.ID, 887, nil)
	require.Error(t, err)
	require.Nil(t, result)

	var count int64
	require.NoError(t, integrationDB.QueryRow(
		`SELECT COUNT(*) FROM reward_grants WHERE user_id = $1`, user.ID).Scan(&count))
	require.EqualValues(t, 0, count, "grant row must be rolled back with the balance change")

	var balance float64
	require.NoError(t, integrationDB.QueryRow(
		`SELECT balance::double precision FROM users WHERE id = $1`, user.ID).Scan(&balance))
	require.InDelta(t, 10.0, balance, 1e-8)
}

// failingAdjustUserRepo 只在指定用户上让 AdjustBalance 失败，用于制造事务内失败。
type failingAdjustUserRepo struct {
	service.UserRepository
	failOn int64
}

func (f *failingAdjustUserRepo) AdjustBalance(ctx context.Context, id int64, delta float64) (service.BalanceChange, error) {
	if id == f.failOn {
		return service.BalanceChange{}, fmt.Errorf("injected adjust failure")
	}
	return f.UserRepository.AdjustBalance(ctx, id, delta)
}
