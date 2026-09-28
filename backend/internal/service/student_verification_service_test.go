package service

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"testing"
	"time"

	dbent "github.com/Wei-Shaw/sub2api/ent"
	"github.com/Wei-Shaw/sub2api/internal/domain"
	"github.com/stretchr/testify/require"

	"entgo.io/ent/dialect"
	entsql "entgo.io/ent/dialect/sql"
	"github.com/DATA-DOG/go-sqlmock"
)

// =============================================================================
// fakes
// =============================================================================

// fakeStudentEmailCache 内存 EmailCache（GETDEL 语义原子化，模拟 Redis）。
type fakeStudentEmailCache struct {
	EmailCache // 嵌入接口：未实现的缓存方法测试不触达

	mu       sync.Mutex
	codes    map[string]*VerificationCodeData
	cooldown map[string]time.Time
	userRate map[int64]int
	sendFail bool
}

func newFakeStudentEmailCache() *fakeStudentEmailCache {
	return &fakeStudentEmailCache{
		codes:    map[string]*VerificationCodeData{},
		cooldown: map[string]time.Time{},
		userRate: map[int64]int{},
	}
}

func (f *fakeStudentEmailCache) GetNotifyCodeUserRate(ctx context.Context, userID int64) (int64, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	return int64(f.userRate[userID]), nil
}

func (f *fakeStudentEmailCache) IncrNotifyCodeUserRate(ctx context.Context, userID int64, window time.Duration) (int64, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.userRate[userID]++
	return int64(f.userRate[userID]), nil
}

func (f *fakeStudentEmailCache) ReserveVerificationCodeCooldown(ctx context.Context, key string, cooldown time.Duration) (bool, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if until, ok := f.cooldown[key]; ok && until.After(time.Now()) {
		return false, nil
	}
	f.cooldown[key] = time.Now().Add(cooldown)
	return true, nil
}

func (f *fakeStudentEmailCache) ReleaseVerificationCodeCooldown(ctx context.Context, key string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	delete(f.cooldown, key)
	return nil
}

func (f *fakeStudentEmailCache) SetVerificationCode(ctx context.Context, key string, data *VerificationCodeData, ttl time.Duration) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.codes[key] = data
	return nil
}

// ConsumeVerificationCode 复刻 Redis GETDEL：取走即删（并发下只有一个赢家）。
func (f *fakeStudentEmailCache) ConsumeVerificationCode(ctx context.Context, key string) (*VerificationCodeData, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	data, ok := f.codes[key]
	if !ok {
		return nil, nil
	}
	delete(f.codes, key)
	return data, nil
}

type fakeVerificationRepo struct {
	StudentVerificationRepository // 嵌入接口
	mu       sync.Mutex
	nextID   int64
	last     *StudentVerification
	linked   map[int64]int64 // verificationID -> grantID
	latest   *StudentVerification
	linkErr  error
}

func newFakeVerificationRepo() *fakeVerificationRepo {
	return &fakeVerificationRepo{linked: map[int64]int64{}}
}

func (f *fakeVerificationRepo) Insert(ctx context.Context, v *StudentVerification) (int64, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.nextID++
	v.ID = f.nextID
	now := time.Now()
	v.VerifiedAt = now
	cp := *v
	f.last = &cp
	f.latest = &cp
	return v.ID, nil
}

func (f *fakeVerificationRepo) LinkBenefitGrant(ctx context.Context, id, grantID int64) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.linkErr != nil {
		return f.linkErr
	}
	f.linked[id] = grantID
	return nil
}

func (f *fakeVerificationRepo) LatestVerifiedByUser(ctx context.Context, userID int64) (*StudentVerification, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.latest != nil {
		cp := *f.latest
		return &cp, nil
	}
	return nil, nil
}

func (f *fakeVerificationRepo) RevokeByUser(ctx context.Context, userID int64, revokedBy int64, reason string) (int64, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.latest != nil {
		f.latest.Status = domain.StudentVerificationStatusRevoked
		return 1, nil
	}
	return 0, nil
}

func (f *fakeVerificationRepo) GetByID(ctx context.Context, id int64) (*StudentVerification, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.last != nil {
		cp := *f.last
		return &cp, nil
	}
	return nil, fmt.Errorf("not found")
}

func (f *fakeVerificationRepo) AdminList(ctx context.Context, filter *StudentVerificationAdminFilter) (*StudentVerificationAdminList, error) {
	return &StudentVerificationAdminList{Items: []StudentVerificationAdminItem{}}, nil
}

// stubEmptySettingRepo 返回空配置的设置仓储（SMTP 未配置路径）。
type stubEmptySettingRepo struct{ SettingRepository }

func (stubEmptySettingRepo) GetMultiple(ctx context.Context, keys []string) (map[string]string, error) {
	return map[string]string{}, nil
}

type fakeStudentSettingReader struct {
	enabled   bool
	groupID   int64
	planID    int64
	days      int
	code      string
	rewardOn  bool
	rewardAmt float64
	rewardCpg string
}

func (f *fakeStudentSettingReader) IsStudentVerificationEnabled(ctx context.Context) bool { return f.enabled }
func (f *fakeStudentSettingReader) GetStudentBenefitGroupID(ctx context.Context) int64 { return f.groupID }
func (f *fakeStudentSettingReader) GetStudentBenefitPlanID(ctx context.Context) int64 { return f.planID }
func (f *fakeStudentSettingReader) GetStudentBenefitValidityDays(ctx context.Context) int { return f.days }
func (f *fakeStudentSettingReader) GetStudentBenefitCode(ctx context.Context) string { return f.code }
func (f *fakeStudentSettingReader) IsStudentVerificationRewardEnabled(ctx context.Context) bool { return f.rewardOn }
func (f *fakeStudentSettingReader) GetStudentVerificationRewardAmount(ctx context.Context) float64 { return f.rewardAmt }
func (f *fakeStudentSettingReader) GetStudentVerificationRewardCampaign(ctx context.Context) string { return f.rewardCpg }

// stubRewardGrantService 余额奖励占位（默认 nil 跳过；这里不触达真服务）。

func newTestStudentVerificationService(t *testing.T, reader *fakeStudentSettingReader, cache *fakeStudentEmailCache) (*StudentVerificationService, *fakeVerificationRepo, *fakeGrantRepo, *fakeClaimRepo, *fakeSubscriptionWorld, *sqlmock.Sqlmock) {
	t.Helper()
	// 品牌在服务构造时读取：测试固定为 HUBU（学生域名 stu.hubu.edu.cn）
	t.Setenv("BRAND", "hubu")
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	t.Cleanup(func() { _ = db.Close() })
	client := dbent.NewClient(dbent.Driver(entsql.OpenDB(dialect.Postgres, db)))

	grantRepo := newFakeGrantRepo()
	claimRepo := newFakeClaimRepo()
	world := newFakeSubscriptionWorld()
	world.now = func() time.Time { return time.Date(2026, 9, 29, 12, 0, 0, 0, time.UTC) }
	grantSvc := NewSubscriptionGrantService(
		client, grantRepo, claimRepo,
		&fakeUserRepoGrant{users: map[int64]bool{1: true, 2: true}},
		&fakeGroupRepoGrant{group: &Group{SubscriptionType: SubscriptionTypeSubscription}},
		world, world, nil,
	)
	grantSvc.SetNowFunc(world.now)

	// 空 settings → SMTP 未配置 → SendEmail 报 ErrEmailNotConfigured（sendCode 走失败释放路径）
	emailSvc := NewEmailService(&stubEmptySettingRepo{}, cache)
	svc := NewStudentVerificationService(client, newFakeVerificationRepo(), grantSvc, nil, reader, emailSvc)
	// 注：NewStudentVerificationService 内部已创建 fake repo；为了断言，再取出来。
	repo := newFakeVerificationRepo()
	_ = repo
	return svc, nil, grantRepo, claimRepo, world, &mock
}

// =============================================================================
// 邮箱归一化 / 域名校验（后端权威校验）
// =============================================================================

func TestStudentEmailNormalize_DomainRejected(t *testing.T) {
	svc, _, _, _, _, _ := newTestStudentVerificationService(t, &fakeStudentSettingReader{enabled: true, groupID: 10, days: 30}, newFakeStudentEmailCache())

	for _, bad := range []string{
		"student@gmail.com",
		"student@hubu.edu.cn",     // 教职工域，非学生域
		"student@stu.hubu.edu.cn.evil.com",
		"student@stu.hubu.edu.cn.evil.cn",
		"@stu.hubu.edu.cn",
		"not-an-email",
		"student@@stu.hubu.edu.cn",
		"  ",
	} {
		_, err := svc.normalizeStudentEmail(bad)
		require.Error(t, err, "should reject: %q", bad)
		require.True(t, err == ErrStudentEmailInvalid || err.Error() == ErrStudentEmailInvalid.Error())
	}

	// 合法：大小写/空格归一化
	normalized, err := svc.normalizeStudentEmail("  Student@STU.hubu.edu.cn ")
	require.NoError(t, err)
	require.Equal(t, "student@stu.hubu.edu.cn", normalized)
}

// =============================================================================
// 发送验证码：限流 / 冷却 / 失败释放
// =============================================================================

func TestStudentSendCode_DisabledFails(t *testing.T) {
	cache := newFakeStudentEmailCache()
	svc, _, _, _, _, _ := newTestStudentVerificationService(t, &fakeStudentSettingReader{enabled: false}, cache)
	err := svc.SendVerificationCode(context.Background(), 1, "s@stu.hubu.edu.cn")
	require.True(t, err == ErrStudentVerificationDisabled || err.Error() == ErrStudentVerificationDisabled.Error())
}

func TestStudentSendCode_UserRateLimited(t *testing.T) {
	cache := newFakeStudentEmailCache()
	svc, _, _, _, _, _ := newTestStudentVerificationService(t, &fakeStudentSettingReader{enabled: true, groupID: 10, days: 30}, cache)
	ctx := context.Background()
	// 预置已发满 5 次
	cache.mu.Lock()
	cache.userRate[1] = notifyCodeUserRateLimit
	cache.mu.Unlock()
	err := svc.SendVerificationCode(ctx, 1, "s@stu.hubu.edu.cn")
	require.True(t, err == ErrNotifyCodeUserRateLimit || err.Error() == ErrNotifyCodeUserRateLimit.Error())
}

func TestStudentSendCode_SMTPFailureReleasesCooldown(t *testing.T) {
	cache := newFakeStudentEmailCache()
	svc, _, _, _, _, _ := newTestStudentVerificationService(t, &fakeStudentSettingReader{enabled: true, groupID: 10, days: 30}, cache)
	ctx := context.Background()
	// SMTP 未配置（settingRepo=nil）→ 发送失败 → 冷却必须被释放（不锁死用户）
	err := svc.SendVerificationCode(ctx, 1, "s@stu.hubu.edu.cn")
	require.Error(t, err)
	cache.mu.Lock()
	_, cooldownHeld := cache.cooldown["student-verification:user:1"]
	cache.mu.Unlock()
	require.False(t, cooldownHeld, "cooldown must be released on send failure")
	require.Empty(t, cache.codes, "no code stored on failure")
}

// =============================================================================
// 验证 + 发放：正常流 / 错误码 / 过期 / 重放 / 并发
// =============================================================================

func seedCode(t *testing.T, cache *fakeStudentEmailCache, email, code string, expires time.Time) {
	t.Helper()
	require.NoError(t, cache.SetVerificationCode(context.Background(), "student-verification:user:1", &VerificationCodeData{
		Code: code, Target: email, CreatedAt: time.Now(), ExpiresAt: expires,
	}, verifyCodeTTL))
}

func studentVerifyCmd() *fakeStudentSettingReader {
	return &fakeStudentSettingReader{enabled: true, groupID: 10, days: 30, code: "HUBU_STUDENT_WELCOME"}
}

func TestStudentVerify_HappyPath(t *testing.T) {
	cache := newFakeStudentEmailCache()
	reader := studentVerifyCmd()
	svc, _, _, claimRepo, world, mock := newTestStudentVerificationService(t, reader, cache)
	seedCode(t, cache, "stu@stu.hubu.edu.cn", "123456", time.Now().Add(5*time.Minute))

	(*mock).ExpectBegin()
	(*mock).ExpectCommit()

	result, err := svc.VerifyEmail(context.Background(), 1, "Stu@STU.hubu.edu.cn", " 123456 ", "1.2.3.4", "ua")
	require.NoError(t, err)
	require.Equal(t, "stu@stu.hubu.edu.cn", result.Verification.Email)
	require.Equal(t, "HUBU_EMAIL", result.Verification.Provider)
	require.Equal(t, GrantOutcomeActivatedNew, result.Outcome.Action)
	require.Equal(t, domain.SubscriptionGrantStatusFulfilled, result.Grant.Status)

	// 订阅真实生效
	sub := world.get(1, 10)
	require.NotNil(t, sub)
	require.True(t, sub.IsActive())
	require.True(t, sub.ExpiresAt.Equal(time.Date(2026, 10, 29, 12, 0, 0, 0, time.UTC)), "got %v", sub.ExpiresAt)

	// claim 永久事实
	require.Len(t, claimRepo.claims, 1)
	// OTP 单次消费
	cache.mu.Lock()
	_, still := cache.codes["student-verification:user:1"]
	cache.mu.Unlock()
	require.False(t, still, "OTP must be consumed once")
}

func TestStudentVerify_WrongCode(t *testing.T) {
	cache := newFakeStudentEmailCache()
	svc, _, grantRepo, claimRepo, world, mock := newTestStudentVerificationService(t, studentVerifyCmd(), cache)
	seedCode(t, cache, "stu@stu.hubu.edu.cn", "123456", time.Now().Add(5*time.Minute))

	(*mock).ExpectBegin()
	(*mock).ExpectRollback()
	_, err := svc.VerifyEmail(context.Background(), 1, "stu@stu.hubu.edu.cn", "999999", "", "")
	require.True(t, err == ErrInvalidVerifyCode || err.Error() == ErrInvalidVerifyCode.Error())
	require.Empty(t, claimRepo.claims)
	require.Nil(t, world.get(1, 10))
	_ = grantRepo
}

func TestStudentVerify_ExpiredCode(t *testing.T) {
	cache := newFakeStudentEmailCache()
	svc, _, _, _, world, mock := newTestStudentVerificationService(t, studentVerifyCmd(), cache)
	seedCode(t, cache, "stu@stu.hubu.edu.cn", "123456", time.Now().Add(-time.Minute)) // 已过期

	(*mock).ExpectBegin()
	(*mock).ExpectRollback()
	_, err := svc.VerifyEmail(context.Background(), 1, "stu@stu.hubu.edu.cn", "123456", "", "")
	require.True(t, err == ErrInvalidVerifyCode || err.Error() == ErrInvalidVerifyCode.Error())
	require.Nil(t, world.get(1, 10))
}

func TestStudentVerify_EmailMismatchRejected(t *testing.T) {
	cache := newFakeStudentEmailCache()
	svc, _, _, _, _, mock := newTestStudentVerificationService(t, studentVerifyCmd(), cache)
	seedCode(t, cache, "stu@stu.hubu.edu.cn", "123456", time.Now().Add(5*time.Minute))

	(*mock).ExpectBegin()
	(*mock).ExpectRollback()
	// 验证码发给 A 邮箱，却用 B 邮箱提交 → 拒绝（Target 绑定校验）
	_, err := svc.VerifyEmail(context.Background(), 1, "other@stu.hubu.edu.cn", "123456", "", "")
	require.True(t, err == ErrInvalidVerifyCode || err.Error() == ErrInvalidVerifyCode.Error())
}

func TestStudentVerify_ReplayRejected(t *testing.T) {
	cache := newFakeStudentEmailCache()
	svc, _, _, _, world, mock := newTestStudentVerificationService(t, studentVerifyCmd(), cache)
	seedCode(t, cache, "stu@stu.hubu.edu.cn", "123456", time.Now().Add(5*time.Minute))

	(*mock).ExpectBegin()
	(*mock).ExpectCommit()
	_, err := svc.VerifyEmail(context.Background(), 1, "stu@stu.hubu.edu.cn", "123456", "", "")
	require.NoError(t, err)

	// 重放：OTP 已消费 → 拒绝（不产生第二次发放）
	(*mock).ExpectBegin()
	(*mock).ExpectRollback()
	_, err = svc.VerifyEmail(context.Background(), 1, "stu@stu.hubu.edu.cn", "123456", "", "")
	require.True(t, err == ErrInvalidVerifyCode || err.Error() == ErrInvalidVerifyCode.Error())
	require.Len(t, world.subs[1], 1)
}

// 场景 18.16：并发两个 verification success 请求 → 仍只有一个 Grant
func TestStudentVerify_ConcurrentSingleGrant(t *testing.T) {
	cache := newFakeStudentEmailCache()
	svc, _, grantRepo, claimRepo, _, mock := newTestStudentVerificationService(t, studentVerifyCmd(), cache)
	seedCode(t, cache, "stu@stu.hubu.edu.cn", "123456", time.Now().Add(5*time.Minute))

	const n = 6
	var wg sync.WaitGroup
	wins := make([]*StudentVerificationResult, n)
	errs := make([]error, n)
	mockMu := sync.Mutex{}
	for i := 0; i < n; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			// GETDEL 原子性：只有赢家拿到 code；输家在事务开始前失败（mock 计数宽松处理）
			mockMu.Lock()
			(*mock).ExpectBegin()
			(*mock).ExpectCommit()
			mockMu.Unlock()
			result, err := svc.VerifyEmail(context.Background(), 1, "stu@stu.hubu.edu.cn", "123456", "", "")
			wins[i], errs[i] = result, err
		}(i)
	}
	wg.Wait()
	success := 0
	for i := range wins {
		if errs[i] == nil && wins[i] != nil {
			success++
		}
	}
	require.Equal(t, 1, success, "并发验证只能有一个赢家")
	require.Len(t, claimRepo.claims, 1)
	require.Len(t, grantRepo.grants, 1)
}

// 场景 18.17：同一邮箱换账号（user 2 用 user 1 验证过的邮箱）→ benefit claim 冲突拒绝
func TestStudentVerify_SameEmailDifferentUserRejected(t *testing.T) {
	cache := newFakeStudentEmailCache()
	svc, _, _, claimRepo, world, mock := newTestStudentVerificationService(t, studentVerifyCmd(), cache)

	// 用户 1 认证成功
	seedCode(t, cache, "stu@stu.hubu.edu.cn", "111111", time.Now().Add(5*time.Minute))
	(*mock).ExpectBegin()
	(*mock).ExpectCommit()
	_, err := svc.VerifyEmail(context.Background(), 1, "stu@stu.hubu.edu.cn", "111111", "", "")
	require.NoError(t, err)

	// 用户 2 用同一邮箱：假 data 无法区分 user（cacheKey 按 user 隔离），
	// 模拟用户 2 拿到自己的 code（例如攻击者复用同一邮箱）
	seedCode2(t, cache, 2, "stu@stu.hubu.edu.cn", "222222")
	(*mock).ExpectBegin()
	(*mock).ExpectRollback()
	_, err = svc.VerifyEmail(context.Background(), 2, "stu@stu.hubu.edu.cn", "222222", "", "")
	require.Error(t, err)
	require.True(t, errors.Is(err, ErrBenefitAlreadyClaimed), "expected BENEFIT_ALREADY_CLAIMED, got %v", err)
	require.Len(t, claimRepo.claims, 1, "同一邮箱全局只能领取一次")
	require.Nil(t, world.get(2, 10))
}

func seedCode2(t *testing.T, cache *fakeStudentEmailCache, userID int64, email, code string) {
	t.Helper()
	require.NoError(t, cache.SetVerificationCode(context.Background(), fmt.Sprintf("student-verification:user:%d", userID), &VerificationCodeData{
		Code: code, Target: email, CreatedAt: time.Now(), ExpiresAt: time.Now().Add(5 * time.Minute),
	}, verifyCodeTTL))
}

// 配置缺失：认证开启但权益未配置 → 明确报错，不落认证
func TestStudentVerify_MisconfiguredBenefitFails(t *testing.T) {
	cache := newFakeStudentEmailCache()
	svc, _, _, _, _, _ := newTestStudentVerificationService(t, &fakeStudentSettingReader{enabled: true, groupID: 0, days: 0}, cache)
	seedCode(t, cache, "stu@stu.hubu.edu.cn", "123456", time.Now().Add(5*time.Minute))

	_, err := svc.VerifyEmail(context.Background(), 1, "stu@stu.hubu.edu.cn", "123456", "", "")
	require.True(t, errors.Is(err, ErrStudentBenefitMisconfigured))
}
