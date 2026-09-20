//go:build integration

package repository

// Phase 4 —— 统一账户状态（Wallet + Subscription Status）集成测试。
//
// 覆盖 §29 场景矩阵：PAYG only / Subscription+Wallet / 多订阅 / NULL limit /
// 0% / 63% / 100% / overshoot 106.5→user 100 / 过期订阅排除 / 到期钳制周期 /
// Reset 后 0%（Phase 2 Reset Core 契约）/ Website×MUCODE 一致性。

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"testing"
	"time"

	dbent "github.com/Wei-Shaw/sub2api/ent"
	"github.com/Wei-Shaw/sub2api/internal/domain"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/stretchr/testify/require"
)

func phase4RandSuffix() string {
	b := make([]byte, 4)
	_, _ = rand.Read(b)
	return hex.EncodeToString(b)
}

func phase4MustStack(
	t *testing.T,
	client *dbent.Client,
	weeklyLimit *float64,
	weeklyUsage float64,
	anchorOffset time.Duration, // 0 = 未激活窗口（anchor NULL）
) (*service.User, *service.Group, *service.UserSubscription) {
	t.Helper()
	user, group, sub, _, _ := phase0MustSubscriptionStack(t, client, 25, weeklyLimit)
	if anchorOffset != 0 {
		anchor := time.Now().Add(anchorOffset).Truncate(time.Microsecond)
		phase0SetWeeklyWindow(t, sub.ID, anchor, weeklyUsage)
	}
	return user, group, sub
}

func phase4NewStatusService(t *testing.T, client *dbent.Client) (*service.AccountStatusService, *service.SubscriptionService) {
	t.Helper()
	userRepo := NewUserRepository(client, integrationDB)
	subRepo := NewUserSubscriptionRepository(client)
	subSvc := service.NewSubscriptionService(nil, subRepo, nil, client, nil)
	svc := service.NewAccountStatusService(userRepo, subRepo, NewGroupRepository(client, integrationDB), subSvc, false)
	return svc, subSvc
}

func TestPhase4AccountStatusPaygOnlyWalletAlwaysPresent(t *testing.T) {
	ctx := context.Background()
	client := testEntClient(t)
	user := mustCreateUser(t, client, &service.User{
		Email: fmtPhase4Email("payg"), PasswordHash: "hash", Balance: 100,
	})
	phase0CleanupStack(t, user.ID, 0, 0)

	svc, _ := phase4NewStatusService(t, client)
	status, err := svc.GetAccountStatus(ctx, user.ID)
	require.NoError(t, err)

	// §2/§18：即使没有任何订阅，Wallet 也必须始终存在
	require.Equal(t, "100.00000000", status.Wallet.Balance)
	require.Equal(t, "USD", status.Wallet.CanonicalCurrency)
	require.Empty(t, status.Subscriptions)
}

func TestPhase4AccountStatusSubscriptionWithWalletAndPercent(t *testing.T) {
	ctx := context.Background()
	client := testEntClient(t)
	weeklyLimit := 10.0
	user, group, sub := phase4MustStack(t, client, &weeklyLimit, 6.3, -72*time.Hour)
	// fixture 默认 expires=now+24h 会触发到期钳制；本测试用满 30 天周期
	_, err0 := integrationDB.Exec(
		"UPDATE user_subscriptions SET expires_at = $1 WHERE id = $2",
		time.Now().Add(30*24*time.Hour), sub.ID)
	require.NoError(t, err0)
	phase0CleanupStack(t, user.ID, group.ID, 0)

	svc, _ := phase4NewStatusService(t, client)
	status, err := svc.GetAccountStatus(ctx, user.ID)
	require.NoError(t, err)

	// Wallet 与 Subscription 并存且语义分离（§2/§19）
	require.Equal(t, "25.00000000", status.Wallet.Balance)
	require.Len(t, status.Subscriptions, 1)
	st := status.Subscriptions[0]
	require.Equal(t, sub.ID, st.ID)
	require.Equal(t, group.Name, st.Name)
	require.NotNil(t, st.WeeklyUsagePercent)
	require.InDelta(t, 63.0, *st.WeeklyUsagePercent, 1e-6)
	require.Equal(t, service.UsageStatusNormal, st.UsageStatus)
	require.False(t, st.PaygFallback)
	require.Equal(t, 0, st.ResetCardsAvailable, "reset card runtime not enabled yet")

	// 周期起点 = 锚点；终点 = 锚点+7d（远晚于 30 天到期，不被钳制）
	require.NotNil(t, st.WeeklyPeriodStartedAt)
	require.NotNil(t, st.WeeklyPeriodEndsAt)
	anchor := *st.WeeklyPeriodStartedAt
	require.Equal(t, anchor.Add(7*24*time.Hour).Format(time.RFC3339Nano),
		st.WeeklyPeriodEndsAt.Format(time.RFC3339Nano))
}

func TestPhase4AccountStatusMultipleSubscriptions(t *testing.T) {
	ctx := context.Background()
	client := testEntClient(t)
	limit := 10.0
	user := mustCreateUser(t, client, &service.User{
		Email: fmtPhase4Email("multi"), PasswordHash: "hash",
	})
	g1 := mustCreateGroup(t, client, &service.Group{
		Name: "phase4-multi-basic-" + phase4RandSuffix(), SubscriptionType: service.SubscriptionTypeSubscription,
		WeeklyLimitUSD: &limit,
	})
	g2 := mustCreateGroup(t, client, &service.Group{
		Name: "phase4-multi-pro-" + phase4RandSuffix(), SubscriptionType: service.SubscriptionTypeSubscription,
		WeeklyLimitUSD: &limit,
	})
	phase0CleanupStack(t, user.ID, g1.ID, 0)
	t.Cleanup(func() { _, _ = integrationDB.Exec("DELETE FROM groups WHERE id = $1", g2.ID) })

	sub := mustCreateSubscription(t, client, &service.UserSubscription{UserID: user.ID, GroupID: g1.ID})
	_ = mustCreateSubscription(t, client, &service.UserSubscription{UserID: user.ID, GroupID: g2.ID})
	phase0SetWeeklyWindow(t, sub.ID, time.Now().Add(-24*time.Hour), 2)
	// 第二条订阅（Pro 组）由下方百分比断言隐式覆盖；锚点 + 9/10 用量
	_, err2 := integrationDB.Exec(
		"UPDATE user_subscriptions SET weekly_window_start = $1, weekly_usage_usd = 9 WHERE group_id = $2 AND user_id = $3",
		time.Now().Add(-48*time.Hour), g2.ID, user.ID)
	require.NoError(t, err2)

	svc, _ := phase4NewStatusService(t, client)
	status, err := svc.GetAccountStatus(ctx, user.ID)
	require.NoError(t, err)
	require.Len(t, status.Subscriptions, 2, "Basic + Pro subscriptions coexist (§10)")

	byGroup := map[int64]service.AccountSubscriptionStatus{}
	for _, st := range status.Subscriptions {
		byGroup[st.GroupID] = st
	}
	require.InDelta(t, 20.0, *byGroup[g1.ID].WeeklyUsagePercent, 1e-6)
	require.InDelta(t, 90.0, *byGroup[g2.ID].WeeklyUsagePercent, 1e-6)
	require.Equal(t, service.UsageStatusNearLimit, byGroup[g2.ID].UsageStatus)
}

func TestPhase4AccountStatusUnmeteredWhenWeeklyLimitNull(t *testing.T) {
	ctx := context.Background()
	client := testEntClient(t)
	// weekly_limit = NULL；且 usage 非零也不应显示为"有额度百分比"
	user, group, _ := phase4MustStack(t, client, nil, 99, -24*time.Hour)
	phase0CleanupStack(t, user.ID, group.ID, 0)

	svc, _ := phase4NewStatusService(t, client)
	status, err := svc.GetAccountStatus(ctx, user.ID)
	require.NoError(t, err)
	require.Len(t, status.Subscriptions, 1)
	st := status.Subscriptions[0]
	require.Nil(t, st.WeeklyUsagePercent, "NULL limit must not render as 0%")
	require.Equal(t, service.UsageStatusUnmetered, st.UsageStatus)
}

func TestPhase4AccountStatusExhaustedAndOvershootClamped(t *testing.T) {
	ctx := context.Background()
	client := testEntClient(t)
	limit := 20.0
	user, group, sub := phase4MustStack(t, client, &limit, 21.3, -24*time.Hour) // 106.5%
	phase0CleanupStack(t, user.ID, group.ID, 0)

	svc, _ := phase4NewStatusService(t, client)
	status, err := svc.GetAccountStatus(ctx, user.ID)
	require.NoError(t, err)
	st := status.Subscriptions[0]
	require.NotNil(t, st.WeeklyUsagePercent)
	require.InDelta(t, 100.0, *st.WeeklyUsagePercent, 1e-6,
		"overshoot 106.5% must be clamped to 100 for users (§26)")
	require.Equal(t, service.UsageStatusExhausted, st.UsageStatus)

	// 边界：恰好 100%
	phase0SetWeeklyWindow(t, sub.ID, time.Now().Add(-24*time.Hour), 20)
	status, _ = svc.GetAccountStatus(ctx, user.ID)
	st = status.Subscriptions[0]
	require.InDelta(t, 100.0, *st.WeeklyUsagePercent, 1e-6)
	require.Equal(t, service.UsageStatusExhausted, st.UsageStatus)

	// 边界：0%（有额度、无使用）
	phase0SetWeeklyWindow(t, sub.ID, time.Now().Add(-24*time.Hour), 0)
	status, _ = svc.GetAccountStatus(ctx, user.ID)
	st = status.Subscriptions[0]
	require.NotNil(t, st.WeeklyUsagePercent)
	require.InDelta(t, 0.0, *st.WeeklyUsagePercent, 1e-6)
	require.Equal(t, service.UsageStatusNormal, st.UsageStatus)
}

func TestPhase4AccountStatusExpiredSubscriptionExcludedAndPeriodClamped(t *testing.T) {
	ctx := context.Background()
	client := testEntClient(t)
	user := mustCreateUser(t, client, &service.User{
		Email: fmtPhase4Email("expiring"), PasswordHash: "hash",
	})
	group := mustCreateGroup(t, client, &service.Group{
		Name: "phase4-expiring-" + phase4RandSuffix(), SubscriptionType: service.SubscriptionTypeSubscription,
	})
	phase0CleanupStack(t, user.ID, group.ID, 0)

	sub := mustCreateSubscription(t, client, &service.UserSubscription{UserID: user.ID, GroupID: group.ID})
	// 锚点 8 天前（自然周期早已跨过 7 天边界），订阅 1 天后到期：
	// 状态里的 period end 必须被 expires_at 钳制（最后一个周期 <7 天）
	anchor := time.Now().Add(-8 * 24 * time.Hour)
	phase0SetWeeklyWindow(t, sub.ID, anchor, 1)
	_, err := integrationDB.Exec(
		"UPDATE user_subscriptions SET expires_at = $1 WHERE id = $2",
		time.Now().Add(24*time.Hour), sub.ID)
	require.NoError(t, err)

	// 已过期订阅不出现在列表（同组部分唯一索引限制一条非删除行，故直接翻转状态）
	svc, _ := phase4NewStatusService(t, client)
	_, err = integrationDB.Exec(
		"UPDATE user_subscriptions SET status = 'expired' WHERE id = $1", sub.ID)
	require.NoError(t, err)
	status, err := svc.GetAccountStatus(ctx, user.ID)
	require.NoError(t, err)
	require.Empty(t, status.Subscriptions, "expired subscription must be excluded")

	// 恢复 active 后做周期钳制断言
	_, err = integrationDB.Exec(
		"UPDATE user_subscriptions SET status = 'active' WHERE id = $1", sub.ID)
	require.NoError(t, err)
	status, err = svc.GetAccountStatus(ctx, user.ID)
	require.NoError(t, err)
	require.Len(t, status.Subscriptions, 1)
	st := status.Subscriptions[0]
	require.NotNil(t, st.WeeklyPeriodEndsAt)
	require.True(t, st.WeeklyPeriodEndsAt.Before(time.Now().Add(25*time.Hour)),
		"period end must be clamped by expires_at, not anchor+7d")
}

func TestPhase4AccountStatusFallbackFlagAndResetContract(t *testing.T) {
	ctx := context.Background()
	client := testEntClient(t)
	subRepo := NewUserSubscriptionRepository(client)

	limit := 10.0
	user, group, sub := phase4MustStack(t, client, &limit, 8.8, -72*time.Hour)
	// 延长到期，避免 period end 被 fixture 的 +24h 到期钳制
	_, err0 := integrationDB.Exec(
		"UPDATE user_subscriptions SET expires_at = $1 WHERE id = $2",
		time.Now().Add(30*24*time.Hour), sub.ID)
	require.NoError(t, err0)
	phase0CleanupStack(t, user.ID, group.ID, 0)

	// §25：Reset Core（Admin Reset 入口）之后，状态必须立即反映 0% + 新锚点
	subSvc := service.NewSubscriptionService(nil, subRepo, nil, client, nil)
	effectiveAt := time.Now().Add(-time.Hour).Truncate(time.Microsecond)
	// Reset Core 的 now 未导出；effectiveAt 取过去时刻即可通过未来守卫（service 内部用真实 time.Now）
	_, err := subSvc.ResetSubscriptionWeeklyPeriod(ctx, &service.WeeklyResetInput{
		UserSubscriptionID:   sub.ID,
		EffectiveAt:          effectiveAt,
		Source:               domain.WeeklyResetSourceAdminManual,
		IgnoreLifecycleCheck: true,
	})
	require.NoError(t, err)

	// §22：fallback 是展示配置值（Phase 1 列），不实现 Runtime；置 true 验证读取
	_, err = integrationDB.Exec(
		"UPDATE user_subscriptions SET auto_payg_fallback = true WHERE id = $1", sub.ID)
	require.NoError(t, err)

	svc, _ := phase4NewStatusService(t, client)
	status, err := svc.GetAccountStatus(ctx, user.ID)
	require.NoError(t, err)
	st := status.Subscriptions[0]
	require.NotNil(t, st.WeeklyUsagePercent)
	require.InDelta(t, 0.0, *st.WeeklyUsagePercent, 1e-6, "reset must read as 0%")
	require.Equal(t, effectiveAt.Format(time.RFC3339Nano), st.WeeklyPeriodStartedAt.Format(time.RFC3339Nano),
		"period start = reset effective_at (re-anchoring)")
	require.Equal(t, effectiveAt.Add(7*24*time.Hour).Format(time.RFC3339Nano),
		st.WeeklyPeriodEndsAt.Format(time.RFC3339Nano), "next period end = new anchor + 7d")
	require.True(t, st.PaygFallback, "payg_fallback reads the persisted column value")
}

func TestPhase4WebsiteMucodeConsistency(t *testing.T) {
	ctx := context.Background()
	client := testEntClient(t)
	limit := 10.0
	user, group, sub := phase4MustStack(t, client, &limit, 6.3, -72*time.Hour)
	phase0CleanupStack(t, user.ID, group.ID, 0)

	svc, _ := phase4NewStatusService(t, client)

	// Website：全量账户状态；MUCODE：Key 所属 Group 的单条状态
	website, err := svc.GetAccountStatus(ctx, user.ID)
	require.NoError(t, err)
	mucode, err := svc.GetGroupSubscriptionStatus(ctx, user.ID, group.ID)
	require.NoError(t, err)

	// 同一钱包同值（§24）
	require.Equal(t, website.Wallet.Balance, mucodeWalletBalance(t, svc, user.ID))
	// 同一订阅：MUCODE 单条必须与 Website 列表中的对应条目全等
	require.Len(t, website.Subscriptions, 1)
	require.Equal(t, website.Subscriptions[0], *mucode)

	// 语义抽查
	require.InDelta(t, 63.0, *mucode.WeeklyUsagePercent, 1e-6)
	require.Equal(t, sub.ID, mucode.ID)

	// 不同 Group（无订阅）→ nil（MUCODE 钱包模式不受影响）
	none, err := svc.GetGroupSubscriptionStatus(ctx, user.ID, 999999)
	require.NoError(t, err)
	require.Nil(t, none)
}

func mucodeWalletBalance(t *testing.T, svc *service.AccountStatusService, userID int64) string {
	t.Helper()
	w, err := svc.GetWallet(context.Background(), userID)
	require.NoError(t, err)
	return w.Balance
}

func fmtPhase4Email(tag string) string {
	return fmt.Sprintf("phase4-%s-%d@example.com", tag, time.Now().UnixNano())
}

