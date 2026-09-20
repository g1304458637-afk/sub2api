//go:build unit

package service

// Phase 2 —— 统一 Reset Core 单元测试。
// 覆盖：§30 re-anchor、§31 新轨道自然续推、§32 stale 事件、§33 同时刻、
// §34 事件 retry 幂等、§36 过期拒绝、§37 未来 effective、Admin 兼容路径。

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/Wei-Shaw/sub2api/internal/domain"
)

// ---- stubs ----

type phase2ResetRepo struct {
	UserSubscriptionRepository // 未实现方法 panic（不应被调用）
	sub                        UserSubscription
	casCalls                   int
}

func (r *phase2ResetRepo) snapshot() *UserSubscription {
	s := r.sub
	return &s
}

func (r *phase2ResetRepo) GetByIDForUpdate(context.Context, int64) (*UserSubscription, error) {
	return r.snapshot(), nil
}

func (r *phase2ResetRepo) GetByID(context.Context, int64) (*UserSubscription, error) {
	return r.snapshot(), nil
}

func (r *phase2ResetRepo) ResetWeeklyUsage(_ context.Context, _ int64, expectedWindowStart *time.Time, newWindowStart time.Time) error {
	r.casCalls++
	// 模拟 CAS：expected 与当前 anchor 不一致 → no-op（行锁下不应发生）
	if (expectedWindowStart == nil) != (r.sub.WeeklyWindowStart == nil) {
		return nil
	}
	if expectedWindowStart != nil && !expectedWindowStart.Equal(*r.sub.WeeklyWindowStart) {
		return nil
	}
	newAnchor := newWindowStart
	r.sub.WeeklyWindowStart = &newAnchor
	r.sub.WeeklyUsageUSD = 0
	return nil
}

type phase2AppRepo struct {
	appID      int64
	status     string // "" = 尚无 application
	claims     int
	finalizes  int
	lastPrev   *time.Time
	lastPrevUs *float64
}

func (r *phase2AppRepo) ClaimWeeklyResetApplication(_ context.Context, _, _ int64, _ time.Time) (bool, string, int64, error) {
	if r.status != "" {
		return false, r.status, r.appID, nil
	}
	r.claims++
	r.appID++
	r.status = domain.ResetApplicationStatusApplied
	return true, r.status, r.appID, nil
}

func (r *phase2AppRepo) FinalizeClaimedWeeklyResetApplication(_ context.Context, _ int64, previousStart *time.Time, previousUsage *float64, _ time.Time, finalStatus string) error {
	r.finalizes++
	r.status = finalStatus
	r.lastPrev = previousStart
	r.lastPrevUs = previousUsage
	return nil
}

// ---- helpers ----

func phase2Anchor() time.Time { return time.Date(2026, 9, 20, 15, 0, 0, 0, time.UTC) }

func phase2NewCore(t *testing.T, anchor *time.Time, usage float64, now time.Time) (*SubscriptionService, *phase2ResetRepo) {
	t.Helper()
	repo := &phase2ResetRepo{sub: UserSubscription{
		ID:                1,
		UserID:            10,
		GroupID:           20,
		StartsAt:          phase2Anchor(),
		ExpiresAt:         phase2Anchor().Add(30 * 24 * time.Hour),
		Status:            SubscriptionStatusActive,
		WeeklyWindowStart: anchor,
		WeeklyUsageUSD:    usage,
	}}
	svc := NewSubscriptionService(nil, repo, nil, nil, nil)
	svc.now = func() time.Time { return now }
	return svc, repo
}

// ---- §30 基本 Re-Anchoring ----

func TestPhase2ResetCore_ReAnchorsWeeklyPeriod(t *testing.T) {
	t.Parallel()
	effective := time.Date(2026, 9, 24, 18, 0, 0, 0, time.UTC)
	now := effective.Add(time.Hour)
	anchor := phase2Anchor()
	svc, repo := phase2NewCore(t, &anchor, 8.8, now)

	res, err := svc.ResetSubscriptionWeeklyPeriod(context.Background(), &WeeklyResetInput{
		UserSubscriptionID: 1,
		EffectiveAt:        effective,
		Source:             domain.WeeklyResetSourceResetCard,
	})
	require.NoError(t, err)
	require.Equal(t, WeeklyResetApplied, res.Status)

	require.Equal(t, effective.Format(time.RFC3339), repo.sub.WeeklyWindowStart.Format(time.RFC3339),
		"anchor must move to effective_at")
	require.InDelta(t, 0, repo.sub.WeeklyUsageUSD, 1e-9, "usage must be zeroed")
	require.Equal(t, 1, repo.casCalls)
	// 生命周期字段不可触碰
	require.Equal(t, phase2Anchor().Format(time.RFC3339), repo.sub.StartsAt.Format(time.RFC3339))
	require.Equal(t, phase2Anchor().Add(30*24*time.Hour).Format(time.RFC3339), repo.sub.ExpiresAt.Format(time.RFC3339))
	require.NotNil(t, res.Subscription)
}

// ---- §31 新轨道自然续推 ----

func TestPhase2ResetCore_NaturalWindowContinuesFromNewAnchor(t *testing.T) {
	t.Parallel()
	effective := time.Date(2026, 9, 24, 18, 0, 0, 0, time.UTC)
	now := effective.Add(time.Hour)
	anchor := phase2Anchor() // 9/20 15:00（旧轨道）
	svc, repo := phase2NewCore(t, &anchor, 5, now)

	_, err := svc.ResetSubscriptionWeeklyPeriod(context.Background(), &WeeklyResetInput{
		UserSubscriptionID: 1, EffectiveAt: effective, Source: domain.WeeklyResetSourceGlobalReset,
	})
	require.NoError(t, err)

	// now 推进到 10/2 19:00：应从新锚点 9/24 18:00 推进到 10/1 18:00，
	// 而不是回到旧轨道 started_at + N×7d（9/27 15:00）
	now2 := time.Date(2026, 10, 2, 19, 0, 0, 0, time.UTC)
	sub := repo.sub
	require.True(t, sub.NeedsWeeklyResetAt(now2))
	next, ok := sub.automaticWindowStartAt(sub.WeeklyWindowStart, 7*24*time.Hour, now2)
	require.True(t, ok)
	require.Equal(t, time.Date(2026, 10, 1, 18, 0, 0, 0, time.UTC).Format(time.RFC3339), next.Format(time.RFC3339))
	require.NotEqual(t, phase2Anchor().Add(7*24*time.Hour).Format(time.RFC3339), next.Format(time.RFC3339),
		"must NOT fall back to the old started_at track")
}

// ---- §32/§33 stale 与同时刻 ----

func TestPhase2ResetCore_StaleEventNeverRollsBack(t *testing.T) {
	t.Parallel()
	anchor := time.Date(2026, 9, 25, 0, 0, 0, 0, time.UTC) // 用户已推进
	effective := time.Date(2026, 9, 24, 18, 0, 0, 0, time.UTC)
	now := effective.Add(48 * time.Hour)
	svc, repo := phase2NewCore(t, &anchor, 5.5, now)

	res, err := svc.ResetSubscriptionWeeklyPeriod(context.Background(), &WeeklyResetInput{
		UserSubscriptionID: 1, EffectiveAt: effective, Source: domain.WeeklyResetSourceGlobalReset,
	})
	require.NoError(t, err)
	require.Equal(t, WeeklyResetSkippedStale, res.Status)
	require.Equal(t, anchor.Format(time.RFC3339), repo.sub.WeeklyWindowStart.Format(time.RFC3339),
		"newer anchor must not be rolled back")
	require.InDelta(t, 5.5, repo.sub.WeeklyUsageUSD, 1e-9, "usage must be untouched")
	require.Zero(t, repo.casCalls)
}

func TestPhase2ResetCore_SameTimeEventIsStale(t *testing.T) {
	t.Parallel()
	effective := time.Date(2026, 9, 24, 18, 0, 0, 0, time.UTC)
	now := effective.Add(time.Minute)
	anchor := effective // anchor == effective_at → stale（不得清掉新周期内的消费）
	svc, repo := phase2NewCore(t, &anchor, 2.5, now)

	res, err := svc.ResetSubscriptionWeeklyPeriod(context.Background(), &WeeklyResetInput{
		UserSubscriptionID: 1, EffectiveAt: effective, Source: domain.WeeklyResetSourceGlobalReset,
	})
	require.NoError(t, err)
	require.Equal(t, WeeklyResetSkippedStale, res.Status)
	require.InDelta(t, 2.5, repo.sub.WeeklyUsageUSD, 1e-9)
	require.Zero(t, repo.casCalls)
}

// ---- §34 事件 retry 幂等 ----

func TestPhase2ResetCore_EventRetryIdempotent(t *testing.T) {
	t.Parallel()
	effective := time.Date(2026, 9, 24, 18, 0, 0, 0, time.UTC)
	now := effective.Add(time.Hour)
	anchor := phase2Anchor()
	svc, repo := phase2NewCore(t, &anchor, 7, now)
	appRepo := &phase2AppRepo{}
	svc.SetResetApplicationRepository(appRepo)
	eventID := int64(99)

	statuses := make([]WeeklyResetStatus, 0, 10)
	for i := 0; i < 10; i++ {
		res, err := svc.ResetSubscriptionWeeklyPeriod(context.Background(), &WeeklyResetInput{
			UserSubscriptionID: 1, EffectiveAt: effective, Source: domain.WeeklyResetSourceGlobalReset,
			ResetEventID: &eventID,
		})
		require.NoError(t, err)
		statuses = append(statuses, res.Status)
	}

	require.Equal(t, WeeklyResetApplied, statuses[0], "first application must apply")
	for i := 1; i < 10; i++ {
		require.Equal(t, WeeklyResetAlreadyApplied, statuses[i],
			"retries must be idempotent, got %s at round %d", statuses[i], i)
	}
	require.Equal(t, 1, repo.casCalls, "reset must execute exactly once")
	require.Equal(t, 1, appRepo.claims)
	require.Equal(t, 1, appRepo.finalizes)
	require.Equal(t, domain.ResetApplicationStatusApplied, appRepo.status)
	require.InDelta(t, 0, repo.sub.WeeklyUsageUSD, 1e-9)
}

func TestPhase2ResetCore_StaleEventWritesSkippedApplication(t *testing.T) {
	t.Parallel()
	anchor := time.Date(2026, 9, 25, 0, 0, 0, 0, time.UTC)
	effective := time.Date(2026, 9, 24, 18, 0, 0, 0, time.UTC)
	now := effective.Add(48 * time.Hour)
	svc, _ := phase2NewCore(t, &anchor, 5.5, now)
	appRepo := &phase2AppRepo{}
	svc.SetResetApplicationRepository(appRepo)
	eventID := int64(7)

	res, err := svc.ResetSubscriptionWeeklyPeriod(context.Background(), &WeeklyResetInput{
		UserSubscriptionID: 1, EffectiveAt: effective, Source: domain.WeeklyResetSourceGlobalReset,
		ResetEventID: &eventID,
	})
	require.NoError(t, err)
	require.Equal(t, WeeklyResetSkippedStale, res.Status)
	require.Equal(t, domain.ResetApplicationStatusSkipped, res.ApplicationStatus)
	require.Equal(t, domain.ResetApplicationStatusSkipped, appRepo.status,
		"skipped application must be committed as audit (not rolled back)")
	require.NotNil(t, appRepo.lastPrevUs)
	require.InDelta(t, 5.5, *appRepo.lastPrevUs, 1e-9, "previous usage recorded for audit")
}

// ---- §36 过期 / §37 未来 / 输入校验 ----

func TestPhase2ResetCore_ExpiredSubscriptionRejected(t *testing.T) {
	t.Parallel()
	effective := time.Date(2026, 9, 24, 18, 0, 0, 0, time.UTC)
	now := effective.Add(time.Hour)
	anchor := phase2Anchor()
	svc, repo := phase2NewCore(t, &anchor, 3, now)
	repo.sub.ExpiresAt = now.Add(-time.Hour) // 已过期

	_, err := svc.ResetSubscriptionWeeklyPeriod(context.Background(), &WeeklyResetInput{
		UserSubscriptionID: 1, EffectiveAt: effective, Source: domain.WeeklyResetSourceGlobalReset,
	})
	require.ErrorIs(t, err, ErrSubscriptionExpired, "reset must not revive an expired subscription")
	require.InDelta(t, 3, repo.sub.WeeklyUsageUSD, 1e-9, "state untouched")
	require.Zero(t, repo.casCalls)
}

func TestPhase2ResetCore_FutureEffectiveRejected(t *testing.T) {
	t.Parallel()
	now := time.Date(2026, 9, 24, 17, 0, 0, 0, time.UTC)
	anchor := phase2Anchor()
	svc, repo := phase2NewCore(t, &anchor, 3, now)

	_, err := svc.ResetSubscriptionWeeklyPeriod(context.Background(), &WeeklyResetInput{
		UserSubscriptionID: 1,
		EffectiveAt:        now.Add(time.Hour), // 未来
		Source:             domain.WeeklyResetSourceGlobalReset,
	})
	require.ErrorIs(t, err, ErrResetNotYetEffective, "V1 does not anchor into the future")
	require.Zero(t, repo.casCalls)
}

func TestPhase2ResetCore_InvalidInputRejected(t *testing.T) {
	t.Parallel()
	now := time.Date(2026, 9, 24, 17, 0, 0, 0, time.UTC)
	anchor := phase2Anchor()
	svc, _ := phase2NewCore(t, &anchor, 0, now)

	_, err := svc.ResetSubscriptionWeeklyPeriod(context.Background(), &WeeklyResetInput{
		UserSubscriptionID: 1, Source: domain.WeeklyResetSourceAdminManual,
	})
	require.ErrorIs(t, err, ErrResetInvalidEffectiveTime)

	_, err = svc.ResetSubscriptionWeeklyPeriod(context.Background(), &WeeklyResetInput{
		UserSubscriptionID: 1, EffectiveAt: now,
	})
	require.ErrorIs(t, err, ErrInvalidInput, "source is required")

	_, err = svc.ResetSubscriptionWeeklyPeriod(context.Background(), nil)
	require.ErrorIs(t, err, ErrInvalidInput)
}

// ---- Admin 兼容路径：IgnoreLifecycleCheck 保留存量语义 ----

func TestPhase2ResetCore_AdminLegacyAllowsExpired(t *testing.T) {
	t.Parallel()
	effective := time.Date(2026, 9, 24, 18, 0, 0, 0, time.UTC)
	now := effective.Add(time.Hour)
	anchor := phase2Anchor()
	svc, repo := phase2NewCore(t, &anchor, 9, now)
	repo.sub.ExpiresAt = now.Add(-time.Hour) // 已过期

	res, err := svc.ResetSubscriptionWeeklyPeriod(context.Background(), &WeeklyResetInput{
		UserSubscriptionID:   1,
		EffectiveAt:          effective,
		Source:               domain.WeeklyResetSourceAdminManual,
		IgnoreLifecycleCheck: true,
	})
	require.NoError(t, err)
	require.Equal(t, WeeklyResetApplied, res.Status, "admin manual reset keeps legacy behavior on expired subs")
	require.InDelta(t, 0, repo.sub.WeeklyUsageUSD, 1e-9)
}

// ---- 事件驱动未接线仓储 → 明确报错（不静默跳过审计） ----

func TestPhase2ResetCore_EventWithoutRepoFails(t *testing.T) {
	t.Parallel()
	effective := time.Date(2026, 9, 24, 18, 0, 0, 0, time.UTC)
	now := effective.Add(time.Hour)
	anchor := phase2Anchor()
	svc, repo := phase2NewCore(t, &anchor, 1, now)
	eventID := int64(5)

	_, err := svc.ResetSubscriptionWeeklyPeriod(context.Background(), &WeeklyResetInput{
		UserSubscriptionID: 1, EffectiveAt: effective, Source: domain.WeeklyResetSourceGlobalReset,
		ResetEventID: &eventID,
	})
	require.Error(t, err, "event-driven reset without wired application repo must fail loudly")
	require.InDelta(t, 1, repo.sub.WeeklyUsageUSD, 1e-9, "no reset performed")
}
