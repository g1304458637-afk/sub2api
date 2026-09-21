package service

import (
	"time"
	"context"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
	dbent "github.com/Wei-Shaw/sub2api/ent"
	"github.com/stretchr/testify/require"

	"entgo.io/ent/dialect"
	entsql "entgo.io/ent/dialect/sql"
)

// ---------- fakes ----------

type fakeRewardGrantRepo struct {
	insertCalls int
	lastInsert  *RewardGrant
	// conflict 为 true 时模拟 UNIQUE(idempotency_key) 命中（返回 inserted=false）
	conflict bool
	// existing 为 conflict=true 时 GetByIdempotencyKey 的返回值
	existing *RewardGrant
	// failInsert 非空时 InsertIdempotent 返回该错误
	failInsert error
}

func (f *fakeRewardGrantRepo) InsertIdempotent(ctx context.Context, grant *RewardGrant) (bool, error) {
	f.insertCalls++
	if f.failInsert != nil {
		return false, f.failInsert
	}
	if f.conflict {
		return false, nil
	}
	grant.ID = 100
	f.lastInsert = grant
	return true, nil
}

func (f *fakeRewardGrantRepo) GetByIdempotencyKey(ctx context.Context, idempotencyKey string) (*RewardGrant, error) {
	return f.existing, nil
}

func (f *fakeRewardGrantRepo) GetByUserSourceCampaign(ctx context.Context, userID int64, sourceType, campaign string) ([]RewardGrant, error) {
	return nil, nil
}

func (f *fakeRewardGrantRepo) ListByUser(ctx context.Context, userID int64, limit int) ([]RewardGrant, error) {
	return nil, nil
}

func (f *fakeRewardGrantRepo) CountByUser(ctx context.Context, userID int64) (int64, error) {
	return 0, nil
}

func (f *fakeRewardGrantRepo) ListAll(ctx context.Context, userID *int64, limit, offset int) ([]RewardGrant, int64, error) {
	return nil, 0, nil
}

func (f *fakeRewardGrantRepo) StatsRange(ctx context.Context, userID *int64, from, to time.Time) (int64, float64, error) {
	return 0, 0, nil
}

func (f *fakeRewardGrantRepo) GetBySource(ctx context.Context, sourceType string, sourceID int64) ([]RewardGrant, error) {
	return nil, nil
}

// 嵌入接口（nil）：只覆写被测方法，其余不应被调用。
type fakeBalanceUserRepo struct {
	UserRepository
	adjustCalls int
	adjustDelta float64
	adjustErr   error
}

func (f *fakeBalanceUserRepo) AdjustBalance(ctx context.Context, id int64, delta float64) (BalanceChange, error) {
	f.adjustCalls++
	f.adjustDelta = delta
	if f.adjustErr != nil {
		return BalanceChange{}, f.adjustErr
	}
	return BalanceChange{Old: 10, New: 10 + delta}, nil
}

type fakeSettingReader struct {
	enabled  bool
	amount   float64
	campaign string
}

func (f fakeSettingReader) IsStudentVerificationRewardEnabled(ctx context.Context) bool {
	return f.enabled
}

func (f fakeSettingReader) GetStudentVerificationRewardAmount(ctx context.Context) float64 {
	return f.amount
}

func (f fakeSettingReader) GetStudentVerificationRewardCampaign(ctx context.Context) string {
	return f.campaign
}

func newRewardGrantTestService(repo *fakeRewardGrantRepo, userRepo *fakeBalanceUserRepo, reader fakeSettingReader) *RewardGrantService {
	return &RewardGrantService{
		rewardRepo:    repo,
		userRepo:      userRepo,
		settingReader: reader,
	}
}

// newTxMockEntClient 提供一个带预期 BEGIN 的 ent client，
// 让 GrantReward 的"自开事务"路径在单测里可跑（fake repo 不触 SQL，只需 BEGIN/COMMIT 配对）。
func newTxMockEntClient(t *testing.T) (*dbent.Client, sqlmock.Sqlmock) {
	t.Helper()
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	t.Cleanup(func() { _ = db.Close() })
	driver := entsql.OpenDB(dialect.Postgres, db)
	client := dbent.NewClient(dbent.Driver(driver))
	t.Cleanup(func() { _ = client.Close() })
	return client, mock
}

// ---------- GrantStudentVerificationReward ----------

func TestGrantStudentVerificationReward_Disabled(t *testing.T) {
	t.Parallel()
	repo := &fakeRewardGrantRepo{}
	userRepo := &fakeBalanceUserRepo{}
	svc := newRewardGrantTestService(repo, userRepo, fakeSettingReader{enabled: false, amount: 20, campaign: "2026_fall"})

	result, err := svc.GrantStudentVerificationReward(context.Background(), 1, 887, nil)
	require.NoError(t, err)
	require.Equal(t, RewardGrantStatusDisabled, result.Status)
	require.False(t, result.Granted())
	require.Zero(t, repo.insertCalls)
	require.Zero(t, userRepo.adjustCalls)
}

func TestGrantStudentVerificationReward_InvalidConfig(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name   string
		reader fakeSettingReader
	}{
		{"zero amount", fakeSettingReader{enabled: true, amount: 0, campaign: "2026_fall"}},
		{"negative amount", fakeSettingReader{enabled: true, amount: -5, campaign: "2026_fall"}},
		{"empty campaign", fakeSettingReader{enabled: true, amount: 20, campaign: ""}},
	}
	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			repo := &fakeRewardGrantRepo{}
			userRepo := &fakeBalanceUserRepo{}
			svc := newRewardGrantTestService(repo, userRepo, tc.reader)

			result, err := svc.GrantStudentVerificationReward(context.Background(), 1, 887, nil)
			require.NoError(t, err)
			require.Equal(t, RewardGrantStatusInvalidConfig, result.Status)
			require.False(t, result.Granted())
			require.Zero(t, repo.insertCalls)
			require.Zero(t, userRepo.adjustCalls)
		})
	}
}

func TestGrantStudentVerificationReward_Granted(t *testing.T) {
	t.Parallel()
	repo := &fakeRewardGrantRepo{}
	userRepo := &fakeBalanceUserRepo{}
	reader := fakeSettingReader{enabled: true, amount: 20, campaign: "2026_fall"}
	client, mock := newTxMockEntClient(t)
	mock.ExpectBegin()
	mock.ExpectCommit()
	svc := newRewardGrantTestService(repo, userRepo, reader)
	svc.entClient = client

	result, err := svc.GrantStudentVerificationReward(context.Background(), 42, 887, nil)
	require.NoError(t, err)
	require.Equal(t, RewardGrantStatusGranted, result.Status)
	require.True(t, result.Granted())
	require.Equal(t, 1, repo.insertCalls)
	require.Equal(t, 1, userRepo.adjustCalls)
	require.Equal(t, float64(20), userRepo.adjustDelta)
	// 确定性幂等键：student_verification:{userID}:{campaign}
	require.NotNil(t, repo.lastInsert)
	require.Equal(t, "student_verification:42:2026_fall", repo.lastInsert.IdempotencyKey)
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestGrantStudentVerificationReward_AlreadyGrantedSkipsBalance(t *testing.T) {
	t.Parallel()
	repo := &fakeRewardGrantRepo{conflict: true, existing: &RewardGrant{ID: 7, Campaign: "2026_fall"}}
	userRepo := &fakeBalanceUserRepo{}
	client, mock := newTxMockEntClient(t)
	mock.ExpectBegin()
	mock.ExpectCommit()
	svc := newRewardGrantTestService(repo, userRepo, fakeSettingReader{enabled: true, amount: 20, campaign: "2026_fall"})
	svc.entClient = client

	result, err := svc.GrantStudentVerificationReward(context.Background(), 42, 887, nil)
	require.NoError(t, err)
	require.Equal(t, RewardGrantStatusAlreadyGranted, result.Status)
	require.True(t, result.Granted())
	require.NotNil(t, result.Grant)
	require.Equal(t, int64(7), result.Grant.ID)
	// 幂等关键断言：已发放过时绝不再加余额
	require.Zero(t, userRepo.adjustCalls)
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestGrantStudentVerificationReward_InvalidArgs(t *testing.T) {
	t.Parallel()
	repo := &fakeRewardGrantRepo{}
	userRepo := &fakeBalanceUserRepo{}
	svc := newRewardGrantTestService(repo, userRepo, fakeSettingReader{enabled: true, amount: 20, campaign: "2026_fall"})

	for _, tc := range []struct {
		name string
		user int64
		ver  int64
	}{
		{"zero user", 0, 887},
		{"zero verification", 42, 0},
	} {
		_, err := svc.GrantStudentVerificationReward(context.Background(), tc.user, tc.ver, nil)
		require.Error(t, err, tc.name)
	}
}

// ---------- GrantReward ----------

func TestGrantReward_InvalidCommand(t *testing.T) {
	t.Parallel()
	repo := &fakeRewardGrantRepo{}
	userRepo := &fakeBalanceUserRepo{}
	svc := newRewardGrantTestService(repo, userRepo, fakeSettingReader{enabled: true, amount: 20, campaign: "c"})

	badSourceID := int64(0)
	cmds := []GrantRewardCommand{
		{UserID: 0, SourceType: "x", Campaign: "c", Amount: 1, IdempotencyKey: "k1"},
		{UserID: 1, SourceType: "", Campaign: "c", Amount: 1, IdempotencyKey: "k1"},
		{UserID: 1, SourceType: "x", Campaign: "", Amount: 1, IdempotencyKey: "k1"},
		{UserID: 1, SourceType: "x", Campaign: "c", Amount: 0, IdempotencyKey: "k1"},
		{UserID: 1, SourceType: "x", Campaign: "c", Amount: -1, IdempotencyKey: "k1"},
		{UserID: 1, SourceType: "x", Campaign: "c", Amount: 1, IdempotencyKey: ""}, // 缺幂等键
	}
	for _, cmd := range cmds {
		_, err := svc.GrantReward(context.Background(), cmd)
		require.ErrorIs(t, err, ErrRewardInvalidCommand)
	}
	_ = badSourceID
	require.Zero(t, userRepo.adjustCalls)
}

// 同一确定性幂等键：重复调用返回 ALREADY_GRANTED 且回查到原记录（键维度，而非 campaign 维度）。
func TestGrantReward_SameIdempotencyKeyReturnsExisting(t *testing.T) {
	t.Parallel()
	repo := &fakeRewardGrantRepo{conflict: true, existing: &RewardGrant{ID: 9, IdempotencyKey: "referral:77"}}
	userRepo := &fakeBalanceUserRepo{}
	client, mock := newTxMockEntClient(t)
	mock.ExpectBegin()
	mock.ExpectCommit()
	svc := newRewardGrantTestService(repo, userRepo, fakeSettingReader{})
	svc.entClient = client

	result, err := svc.GrantReward(context.Background(), GrantRewardCommand{
		UserID: 42, IdempotencyKey: "referral:77", SourceType: "referral", Campaign: "spring", Amount: 5,
	})
	require.NoError(t, err)
	require.Equal(t, RewardGrantStatusAlreadyGranted, result.Status)
	require.NotNil(t, result.Grant)
	require.Equal(t, int64(9), result.Grant.ID)
	require.Zero(t, userRepo.adjustCalls)
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestGrantReward_AdjustBalanceFailurePropagates(t *testing.T) {
	t.Parallel()
	repo := &fakeRewardGrantRepo{}
	userRepo := &fakeBalanceUserRepo{adjustErr: ErrUserNotFound}
	client, mock := newTxMockEntClient(t)
	mock.ExpectBegin()
	mock.ExpectRollback()
	svc := newRewardGrantTestService(repo, userRepo, fakeSettingReader{enabled: true, amount: 20, campaign: "c"})
	svc.entClient = client

	result, err := svc.GrantReward(context.Background(), GrantRewardCommand{
		UserID: 1, IdempotencyKey: "student_verification:1:c", SourceType: RewardSourceStudentVerification, Campaign: "c", Amount: 20,
	})
	require.Error(t, err)
	require.Nil(t, result)
	// 事务语义（真库 rollback）由集成测试验证；这里断言余额接口确实被调用过
	require.Equal(t, 1, userRepo.adjustCalls)
	require.NoError(t, mock.ExpectationsWereMet())
}
