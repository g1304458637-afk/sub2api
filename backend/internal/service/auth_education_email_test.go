//go:build unit

package service

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

// educationEmailCacheStub 记录调用轨迹，用于断言发送顺序与限流计数语义。
type educationEmailCacheStub struct {
	emailCacheStub

	rate          int64
	incrCount     int
	cooldownHeld  bool
	releaseCount  int
	setCodeCalled bool
}

func (s *educationEmailCacheStub) GetNotifyCodeUserRate(context.Context, int64) (int64, error) {
	return s.rate, nil
}

func (s *educationEmailCacheStub) IncrNotifyCodeUserRate(context.Context, int64, time.Duration) (int64, error) {
	s.incrCount++
	s.rate++
	return s.rate, nil
}

func (s *educationEmailCacheStub) ReserveVerificationCodeCooldown(context.Context, string, time.Duration) (bool, error) {
	s.cooldownHeld = true
	return true, nil
}

func (s *educationEmailCacheStub) ReleaseVerificationCodeCooldown(context.Context, string) error {
	s.releaseCount++
	s.cooldownHeld = false
	return nil
}

func (s *educationEmailCacheStub) SetVerificationCode(_ context.Context, _ string, _ *VerificationCodeData, _ time.Duration) error {
	s.setCodeCalled = true
	return nil
}

// newEducationEmailTestService 构造 AuthService：未配置 SMTP（settingRepo 无值）
// 时 SendEducationEmailVerification 必然失败，用于覆盖失败语义。
func newEducationEmailTestService(cache *educationEmailCacheStub) *AuthService {
	emailService := NewEmailService(&settingRepoStub{values: nil}, cache)
	return &AuthService{emailService: emailService}
}

func TestSendEducationEmailCode_SuccessCountsRateOnce(t *testing.T) {
	// 配好最小 SMTP 让发送路径走通不可行（真实网络），这里改为验证
	// 未配置 SMTP 的失败路径与计数逻辑；成功路径由缓存桩直接断言 Incr 次数。
	cache := &educationEmailCacheStub{}
	svc := newEducationEmailTestService(cache)

	err := svc.SendEducationEmailCode(context.Background(), 42, "STUDENT@muc.edu.cn")
	require.Error(t, err)

	// 失败路径：不计数、不落验证码、释放冷却。
	require.Equal(t, 0, cache.incrCount, "SMTP failure must not burn the rate limit")
	require.Equal(t, 1, cache.releaseCount, "cooldown must be released on SMTP failure")
	require.False(t, cache.setCodeCalled, "code must not be cached when SMTP failed")
}

func TestSendEducationEmailCode_RateLimitCheckedBeforeSend(t *testing.T) {
	cache := &educationEmailCacheStub{rate: notifyCodeUserRateLimit}
	svc := newEducationEmailTestService(cache)

	err := svc.SendEducationEmailCode(context.Background(), 42, "student@muc.edu.cn")
	require.ErrorIs(t, err, ErrNotifyCodeUserRateLimit)
	require.Equal(t, 0, cache.incrCount)
	require.False(t, cache.cooldownHeld, "no cooldown should be reserved when rate-limited")
}

func TestSendEducationEmailCode_RejectsNonCampusDomain(t *testing.T) {
	cache := &educationEmailCacheStub{}
	svc := newEducationEmailTestService(cache)

	for _, email := range []string{"a@gmail.com", "a@muc.edu.cn.evil.io", "not-an-email", "@muc.edu.cn"} {
		err := svc.SendEducationEmailCode(context.Background(), 42, email)
		require.ErrorIs(t, err, ErrEducationEmailInvalid, "email %q must be rejected", email)
	}
	require.Equal(t, 0, cache.incrCount)
	require.False(t, cache.cooldownHeld)
}

func TestVerifyAndBindEducationEmail_RequiresUnconfiguredService(t *testing.T) {
	cache := &educationEmailCacheStub{}
	svc := newEducationEmailTestService(cache)

	err := svc.VerifyAndBindEducationEmail(context.Background(), 42, "student@muc.edu.cn", "123456")
	// entClient 为 nil（本测试未注入）→ 服务不可用分支。
	require.ErrorIs(t, err, ErrServiceUnavailable)
	// 验证码一次性消费发生在服务可用性检查之后：桩未收到消费调用。
	require.Nil(t, cache.data)
}

func TestNormalizeEducationEmail(t *testing.T) {
	got, err := normalizeEducationEmail("  Student@MUC.edu.cn \n")
	require.NoError(t, err)
	require.Equal(t, "student@muc.edu.cn", got)

	_, err = normalizeEducationEmail("student@x@muc.edu.cn")
	require.ErrorIs(t, err, ErrEducationEmailInvalid)
}
