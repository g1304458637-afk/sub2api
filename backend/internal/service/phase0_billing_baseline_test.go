//go:build unit

package service

// Phase 0 — Existing Billing Baseline Lock（单元测试部分）。
//
// 锁定范围：
//   §5  detached billing context（流式断开后结算继续）
//   §6  周期自然滚动：锚点 + N×7 天步进、到期钳制（#5051 行为）
//   §9  订阅到期 > 周期计划（expiry 优先于 weekly schedule）
//   §10 weekly limit 判定（overshoot 后下一请求被拒）
//   §12/§13 buildUsageBillingCommand：subscription=nil → BalanceCost（PAYG fallback 传播基线）
//   §18 daily/monthly limit 是硬闸门；NULL 时不构成限制
//   §19 API Key 5h/1d/7d 限窗默认关闭（用户自配置保护，与订阅周额度无关）

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

// ---- helpers ----

func phase0WeeklyAnchor() time.Time {
	// 用户示例锚点：2026-09-20 15:00 UTC
	return time.Date(2026, 9, 20, 15, 0, 0, 0, time.UTC)
}

func phase0SubWithWindow(anchor time.Time, expires time.Time, weeklyUsage float64) *UserSubscription {
	return &UserSubscription{
		ID:                1,
		UserID:            10,
		GroupID:           20,
		StartsAt:          anchor,
		ExpiresAt:         expires,
		Status:            SubscriptionStatusActive,
		WeeklyWindowStart: &anchor,
		WeeklyUsageUSD:    weeklyUsage,
	}
}

// ---- §6 Weekly Window 自然滚动 ----

func TestPhase0WeeklyWindow_NaturalNextPeriodIsAnchorPlusSevenDays(t *testing.T) {
	t.Parallel()
	sub := phase0SubWithWindow(phase0WeeklyAnchor(), phase0WeeklyAnchor().Add(30*24*time.Hour), 0)

	// now = 9/22 → 未到期，不推进
	now := phase0WeeklyAnchor().Add(2 * 24 * time.Hour)
	require.False(t, sub.NeedsWeeklyResetAt(now))
	require.Equal(t, phase0WeeklyAnchor().Add(7*24*time.Hour).Format(time.RFC3339),
		sub.WeeklyResetTime().Format(time.RFC3339),
		"next natural reset = anchor + 7d (9/27 15:00)")

	// now = 9/28 16:00 → 跨过 1 个周期，新锚点 = 9/27 15:00（不是 now、也不是 anchor+7d 的一次性 +7 之外）
	now2 := phase0WeeklyAnchor().Add(8*24*time.Hour + time.Hour)
	require.True(t, sub.NeedsWeeklyResetAt(now2))
	newStart, ok := sub.automaticWindowStartAt(sub.WeeklyWindowStart, 7*24*time.Hour, now2)
	require.True(t, ok)
	require.Equal(t, phase0WeeklyAnchor().Add(7*24*time.Hour).Format(time.RFC3339),
		newStart.Format(time.RFC3339))
}

func TestPhase0WeeklyWindow_MultiPeriodGapStepsToCurrentPeriod(t *testing.T) {
	t.Parallel()
	// 长时间无请求：锚点 9/20 15:00，now = 10/12 16:00（跨过 9/27、10/4、10/11 三个周期边界）
	sub := phase0SubWithWindow(phase0WeeklyAnchor(), phase0WeeklyAnchor().Add(60*24*time.Hour), 99)
	now := phase0WeeklyAnchor().Add(22*24*time.Hour + time.Hour)

	newStart, ok := sub.automaticWindowStartAt(sub.WeeklyWindowStart, 7*24*time.Hour, now)
	require.True(t, ok)
	// 必须直接步进到当前所属周期起点 10/11 15:00，而不是只 +7d（9/27）
	require.Equal(t, phase0WeeklyAnchor().Add(21*24*time.Hour).Format(time.RFC3339),
		newStart.Format(time.RFC3339),
		"multi-period gap must step to the current period anchor")
}

func TestPhase0WeeklyWindow_ClampedAtSubscriptionExpiry(t *testing.T) {
	t.Parallel()
	// #5051 语义：最后一个不完整周期不得重复发放额度。
	// 锚点 9/20 15:00，订阅 10/5 00:00 到期；now = 10/12（已过期）→
	// 新锚点最多推进到 10/4 15:00，且不能越过 expires_at。
	expires := time.Date(2026, 10, 5, 0, 0, 0, 0, time.UTC)
	sub := phase0SubWithWindow(phase0WeeklyAnchor(), expires, 99)
	now := expires.Add(7 * 24 * time.Hour)

	newStart, ok := sub.automaticWindowStartAt(sub.WeeklyWindowStart, 7*24*time.Hour, now)
	require.True(t, ok)
	require.True(t, newStart.Before(expires), "stepped anchor must stay before expiry")
	require.Equal(t, phase0WeeklyAnchor().Add(14*24*time.Hour).Format(time.RFC3339),
		newStart.Format(time.RFC3339),
		"anchor clamped to last full period boundary before expiry (10/4 15:00)")

	// 越过到期日的锚点推进不被允许
	_, ok2 := sub.automaticWindowStartAt(&newStart, 7*24*time.Hour, now)
	require.False(t, ok2, "no further period may start at/after expiry")
}

// ---- §9 Subscription Expiry 优先级 ----

func TestPhase0SubscriptionExpiryOverridesWeeklySchedule(t *testing.T) {
	t.Parallel()
	svc := NewSubscriptionService(nil, userSubRepoNoop{}, nil, nil, nil)

	weeklyLimit := 100.0
	group := &Group{WeeklyLimitUSD: &weeklyLimit}

	// 周期尚未结束（anchor 9/20 → 下次重置 9/27），但订阅 9/25 到期；
	// 注入 svc.now = 9/26：必须因订阅过期被拒，而不是继续放行到 9/27。
	anchor := phase0WeeklyAnchor()
	expires := anchor.Add(5 * 24 * time.Hour) // 9/25 15:00
	sub := phase0SubWithWindow(anchor, expires, 10)
	svc.now = func() time.Time { return anchor.Add(6 * 24 * time.Hour) } // 9/26 15:00

	_, err := svc.ValidateAndCheckLimits(sub, group)
	require.Error(t, err, "expired subscription must be rejected even though the weekly period has not ended")
	require.ErrorIs(t, err, ErrSubscriptionExpired)

	// 到期前一切正常
	subOK := phase0SubWithWindow(anchor, anchor.Add(30*24*time.Hour), 10)
	needsMaintenance, err := svc.ValidateAndCheckLimits(subOK, group)
	require.NoError(t, err)
	require.False(t, needsMaintenance)
}

// ---- §10 Weekly Limit 判定 ----

func TestPhase0WeeklyLimitCheckBoundary(t *testing.T) {
	t.Parallel()
	limit := 20.0
	group := &Group{WeeklyLimitUSD: &limit}
	sub := phase0SubWithWindow(phase0WeeklyAnchor(), phase0WeeklyAnchor().Add(30*24*time.Hour), 19.8)

	require.True(t, sub.CheckWeeklyLimit(group, 0), "19.8 < 20 → eligible at request start")
	require.True(t, sub.CheckWeeklyLimit(group, 0.2), "19.8+0.2 = 20.0 ≤ 20 → exactly at limit still allowed")

	sub.WeeklyUsageUSD = 21.3
	require.False(t, sub.CheckWeeklyLimit(group, 0), "21.3 after overshoot → next request rejected")

	noLimit := &Group{}
	require.True(t, (&UserSubscription{WeeklyUsageUSD: 1e9}).CheckWeeklyLimit(noLimit, 0),
		"weekly_limit_usd = NULL → no weekly gate")
}

// ---- §12/§13 buildUsageBillingCommand：nil 订阅 → BalanceCost ----

func TestPhase0NilSubscriptionRoutesToBalanceCost(t *testing.T) {
	t.Parallel()

	apiKey := &APIKey{ID: 2}
	user := &User{ID: 3}
	account := &Account{ID: 4, Type: AccountTypeAPIKey}
	cost := &CostBreakdown{TotalCost: 1.0, ActualCost: 1.5}

	// 场景 1：Standard 分组（无订阅）→ BalanceCost
	cmd := buildUsageBillingCommand("req-std", nil, &postUsageBillingParams{
		Cost: cost, User: user, APIKey: apiKey, Account: account,
		IsSubscriptionBill: false,
	})
	require.NotNil(t, cmd)
	require.InDelta(t, 1.5, cmd.BalanceCost, 1e-9)
	require.Zero(t, cmd.SubscriptionCost)
	require.Nil(t, cmd.SubscriptionID)

	// 场景 2：订阅分组但 currentSubscription = nil（PAYG fallback 将复用的传播机制，
	// 现网先例 gateway_handler.go:1018 ops fallback key 路径）→ 同样走 BalanceCost
	cmd2 := buildUsageBillingCommand("req-fallback", nil, &postUsageBillingParams{
		Cost: cost, User: user, APIKey: apiKey, Account: account,
		IsSubscriptionBill: false,
	})
	require.NotNil(t, cmd2)
	require.InDelta(t, 1.5, cmd2.BalanceCost, 1e-9)
	require.Zero(t, cmd2.SubscriptionCost)
	require.Nil(t, cmd2.SubscriptionID)

	// 场景 3：对照——订阅在位时走 SubscriptionCost，balance 不动
	sub := &UserSubscription{ID: 9}
	cmd3 := buildUsageBillingCommand("req-sub", nil, &postUsageBillingParams{
		Cost: cost, User: user, APIKey: apiKey, Account: account,
		Subscription:       sub,
		IsSubscriptionBill: true,
	})
	require.NotNil(t, cmd3)
	require.InDelta(t, 1.5, cmd3.SubscriptionCost, 1e-9)
	require.Zero(t, cmd3.BalanceCost)
	require.NotNil(t, cmd3.SubscriptionID)
	require.Equal(t, int64(9), *cmd3.SubscriptionID)
}

// ---- §5 Detached billing context ----

func TestPhase0DetachedBillingContextSurvivesParentCancel(t *testing.T) {
	t.Parallel()

	parent, cancel := context.WithCancel(context.Background())
	billingCtx, billingCancel := detachedBillingContext(parent)
	defer billingCancel()

	cancel()
	require.NoError(t, billingCtx.Err(),
		"billing context must ignore parent cancellation (stream disconnect keeps settlement alive)")
	deadline, hasDeadline := billingCtx.Deadline()
	require.True(t, hasDeadline && deadline.After(time.Now()),
		"billing context carries its own timeout")

	// 流式上游 context 同理
	streamParent, streamCancel := context.WithCancel(context.Background())
	streamCtx, streamRelease := detachStreamUpstreamContext(streamParent, true)
	defer streamRelease()
	streamCancel()
	require.NoError(t, streamCtx.Err(), "stream upstream context must ignore client disconnect")

	// 非流式请求沿用请求 context（跟随取消）
	nonStreamCtx, nonStreamRelease := detachStreamUpstreamContext(parent, false)
	defer nonStreamRelease()
	cancel()
	require.Error(t, nonStreamCtx.Err(), "non-stream context must follow request cancellation")
}

// ---- §18 Daily / Monthly 硬闸门 ----

func TestPhase0DailyMonthlyLimitsAreHardGates(t *testing.T) {
	t.Parallel()
	svc := NewSubscriptionService(nil, userSubRepoNoop{}, nil, nil, nil)

	weeklyLimit, dailyLimit, monthlyLimit := 100.0, 5.0, 50.0
	full := &Group{WeeklyLimitUSD: &weeklyLimit, DailyLimitUSD: &dailyLimit, MonthlyLimitUSD: &monthlyLimit}

	// daily 达到并超过、weekly 未达到 → 硬拒。
	// 注意：CheckDailyLimit 的边界语义是 usage + additional <= limit（恰好等于放行），
	// 而预检缓存路径 checkSubscriptionEligibility 用 usage >= limit（恰好等于拒绝）；
	// 这里取明确超限值，边界差异记录为 Phase 0 Finding。
	subDaily := phase0SubWithWindow(phase0WeeklyAnchor(), phase0WeeklyAnchor().Add(30*24*time.Hour), 10)
	subDaily.DailyUsageUSD = 5.5
	_, err := svc.ValidateAndCheckLimits(subDaily, full)
	require.ErrorIs(t, err, ErrDailyLimitExceeded)

	// monthly 达到并超过、weekly 未达到 → 硬拒
	subMonthly := phase0SubWithWindow(phase0WeeklyAnchor(), phase0WeeklyAnchor().Add(30*24*time.Hour), 10)
	subMonthly.MonthlyUsageUSD = 50.5
	_, err = svc.ValidateAndCheckLimits(subMonthly, full)
	require.ErrorIs(t, err, ErrMonthlyLimitExceeded)

	// daily/monthly = NULL → 完全不构成闸门（即使用量巨大）
	nulls := &Group{WeeklyLimitUSD: &weeklyLimit}
	subNull := phase0SubWithWindow(phase0WeeklyAnchor(), phase0WeeklyAnchor().Add(30*24*time.Hour), 10)
	subNull.DailyUsageUSD = 1e9
	subNull.MonthlyUsageUSD = 1e9
	_, err = svc.ValidateAndCheckLimits(subNull, nulls)
	require.NoError(t, err, "NULL daily/monthly limits are not hidden gates")
}

// ---- §19 API Key 限窗默认关闭 ----

func TestPhase0APIKeyRateLimitsDefaultOff(t *testing.T) {
	t.Parallel()
	key := &APIKey{}
	require.False(t, key.HasRateLimits(),
		"API Key 5h/1d/7d windows default to 0 (off); they are per-key user-configured protections, unrelated to subscription weekly quota")

	key2 := &APIKey{RateLimit7d: 10}
	require.True(t, key2.HasRateLimits())
}
