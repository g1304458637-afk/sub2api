//go:build unit

package handler

// Phase 0 §13 — Ops fallback 传播机制的代码级契约测试。
//
// 现网先例（gateway_handler.go antigravity prompt-too-long 兜底路径）：
//  1. :1006 CheckBillingEligibility 对兜底分组显式传 nil subscription；
//  2. :995 要求兜底分组必须是 standard（非 subscription）类型；
//  3. :1018 `currentSubscription = eligibility.SubscriptionForBilling(nil)` + :1020 `retryWithFallback = true`
//     → break 重进重试循环 → 下一次迭代 :947 的 usage record task 捕获到 nil，
//     下游 buildUsageBillingCommand 走 BalanceCost。
//
// 该传播链证明："handler 决定模式 → 传 nil 订阅 → 余额计费" 在现网已成立，
// 未来 "subscription quota exceeded + auto_payg_fallback" 复用同一机制
// （差别仅是：不再要求切换分组，且 nil 的原因是额度超限而非确定性失败）。
//
// 若未来重构移动/改名这些锚点，此测试失败即提示重新评估 PAYG fallback 落点。

import (
	"os"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestPhase0OpsFallbackNilSubscriptionPrecedent(t *testing.T) {
	t.Parallel()

	src, err := os.ReadFile("gateway_handler.go")
	require.NoError(t, err)
	lines := strings.Split(string(src), "\n")

	nilAssignLine := -1
	retryFlagLine := -1
	eligibilityNilSubLine := -1
	subscriptionTypeGuardLine := -1
	usageCaptureLine := -1

	for i, line := range lines {
		n := i + 1
		switch {
		case strings.Contains(line, "currentSubscription = eligibility.SubscriptionForBilling(nil)"):
			require.Equal(t, -1, nilAssignLine, "expect a single nil-assignment anchor")
			nilAssignLine = n
		case strings.Contains(line, "retryWithFallback = true"):
			retryFlagLine = n
		case strings.Contains(line, "fallbackAPIKey, fallbackGroup, nil,"):
			eligibilityNilSubLine = n
		case strings.Contains(line, "fallbackGroup.SubscriptionType == service.SubscriptionTypeSubscription"):
			subscriptionTypeGuardLine = n
		case strings.Contains(line, "Subscription:       currentSubscription"):
			usageCaptureLine = n
		}
	}

	require.Greater(t, nilAssignLine, 0, "ops fallback precedent (`currentSubscription = eligibility.SubscriptionForBilling(nil)`) must exist")
	require.Greater(t, retryFlagLine, 0, "retry loop re-entry flag must exist")
	require.Greater(t, eligibilityNilSubLine, 0, "fallback eligibility check must pass nil subscription")
	require.Greater(t, subscriptionTypeGuardLine, 0, "fallback group must be guarded against subscription type")
	require.Greater(t, usageCaptureLine, 0, "usage record task must capture currentSubscription")

	// 控制流次序：nil 赋值与 retry 标志同处兜底分支（retry 紧随其后），
	// 兜底资格检查在赋值之前完成（先验证可兜底，再切模式）。
	require.Less(t, retryFlagLine, nilAssignLine+5,
		"retry flag must be set immediately after the nil assignment")
	require.Less(t, eligibilityNilSubLine, nilAssignLine,
		"fallback eligibility (nil subscription) precedes the mode switch")
	require.Less(t, subscriptionTypeGuardLine, eligibilityNilSubLine,
		"subscription-type guard precedes eligibility check")
}
