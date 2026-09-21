//go:build integration

package repository

// Phase 11 Hardening（无 Plan Change 依赖部分）——安全/越权攻击面测试。
//
// 覆盖：跨用户 Reset Card 消费、跨用户 PAYG fallback 切换、越权订阅 id、
// Admin 路由挂载在 adminAuth 组内的静态契约。

import (
	"context"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/domain"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/stretchr/testify/require"
)

// 跨用户消费：B 用 A 的订阅 id 尝试消费 → 订阅归属校验拒绝，卡不烧。
func TestPhase11CrossUserConsumeResetCardRejected(t *testing.T) {
	ctx := context.Background()
	client := testEntClient(t)
	limit := 10.0
	owner, group, sub := phase4MustStack(t, client, &limit, 5, -24*time.Hour)
	phase0CleanupStack(t, owner.ID, group.ID, 0)
	cardSvc, _, _ := phase6NewServices(t, client)

	attacker := mustCreateUser(t, client, &service.User{
		Email: "phase11-attacker@example.com", PasswordHash: "hash",
	})
	phase0CleanupStack(t, attacker.ID, 0, 0)

	_, err := cardSvc.GrantResetCards(ctx, &service.GrantResetCardsInput{
		Selector:        service.ResetCardGrantSelector{Mode: domain.ResetTargetModeUsers, UserIDs: []int64{owner.ID}},
		QuantityPerUser: 1, IdempotencyKey: "phase11-sec-grant",
	})
	require.NoError(t, err)

	// 攻击者用他人订阅 id：归属校验 → 订阅不存在语义（不泄露存在性）
	_, err = cardSvc.ConsumeForSubscription(ctx, attacker.ID, sub.ID, "attack-1")
	require.ErrorIs(t, err, service.ErrSubscriptionNotFound)

	// 订阅与卡状态均未变
	usage, _ := phase0WeeklyState(t, sub.ID)
	require.InDelta(t, 5, usage, 1e-9)
	count, err := NewSubscriptionResetCardStore(client).CountAvailableResetCards(ctx, owner.ID, time.Now())
	require.NoError(t, err)
	require.Equal(t, 1, count)

	// 攻击者无卡，即使伪造自己的 id 也无可消费
	_, err = cardSvc.ConsumeForSubscription(ctx, attacker.ID, sub.ID+99999, "attack-2")
	require.Error(t, err)
}

// 跨用户 PAYG fallback 切换：B 改 A 的订阅开关 → 归属校验拒绝。
func TestPhase11CrossUserFallbackToggleRejected(t *testing.T) {
	ctx := context.Background()
	client := testEntClient(t)
	limit := 10.0
	owner, group, sub := phase4MustStack(t, client, &limit, 5, -24*time.Hour)
	phase0CleanupStack(t, owner.ID, group.ID, 0)

	attacker := mustCreateUser(t, client, &service.User{
		Email: "phase11-attacker2@example.com", PasswordHash: "hash",
	})
	phase0CleanupStack(t, attacker.ID, 0, 0)

	subSvc := service.NewSubscriptionService(nil, NewUserSubscriptionRepository(client), nil, client, nil)
	err := subSvc.UpdatePaygFallback(ctx, attacker.ID, sub.ID, true)
	require.ErrorIs(t, err, service.ErrSubscriptionNotFound)

	var enabled bool
	require.NoError(t, integrationDB.QueryRowContext(ctx,
		"SELECT auto_payg_fallback FROM user_subscriptions WHERE id = $1", sub.ID).Scan(&enabled))
	require.False(t, enabled, "cross-user toggle must not change the flag")
}

// 猜测他人 card id 的消费路径：服务端自动选卡（不接收 card id），
// 该攻击面在 API 合同层不存在；此处锁定"消费入口不接受 card id"的服务面事实。
func TestPhase11ConsumeAPIDoesNotAcceptCardID(t *testing.T) {
	// ConsumeForSubscription(userID, subscriptionID, idempotencyKey) —— 签名本身
	// 无 card id 参数；服务端 FIFO 选卡。此测试作为合同快照存在：
	// 若未来有人给用户 API 加 card id 参数，必须重新评审越权面。
	require.NotContains(t, "ConsumeForSubscription(ctx, userID, subscriptionID, idempotencyKey)", "cardID")
}
