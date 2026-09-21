//go:build integration

package repository

// Phase 4.1 —— 状态合同定稿测试：
//   - 整数百分比（floor + 100 钳制；status 按 raw 分档）
//   - Reset Card 账户级真实 COUNT（expiry 三边界；不改卡状态）
//   - display_name = Group 名（Plan SKU 身份不在订阅上）
//   - 账户级 vs 订阅级卡语义（多订阅不复制计数）

import (
	"context"
	"testing"
	"time"

	dbent "github.com/Wei-Shaw/sub2api/ent"
	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/Wei-Shaw/sub2api/internal/domain"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/stretchr/testify/require"
)

func TestPhase41DisplayPercentIntegerContract(t *testing.T) {
	t.Parallel()
	cases := []struct {
		raw     float64
		display int
		status  service.UsageStatus
	}{
		{0, 0, service.UsageStatusNormal},
		{0.1, 0, service.UsageStatusNormal},
		{12.99, 12, service.UsageStatusNormal},
		{63.42, 63, service.UsageStatusNormal},
		{69.99, 69, service.UsageStatusNormal},
		{70, 70, service.UsageStatusHigh},
		{89.99, 89, service.UsageStatusHigh},
		{90, 90, service.UsageStatusNearLimit},
		{99.99, 99, service.UsageStatusNearLimit},
		{100, 100, service.UsageStatusExhausted},
		{106.5, 100, service.UsageStatusExhausted},
	}
	for _, c := range cases {
		require.Equal(t, c.display, service.UserDisplayPercent(c.raw),
			"display(raw=%v)：floor + >=100 钳制", c.raw)
		require.Equal(t, c.status, service.ClassifyUsageStatus(c.raw),
			"status(raw=%v) 必须按 raw 分档，而非 display 整数", c.raw)
	}
	// 99.99：显示 99 但状态 near_limit —— display 与 status 解耦的直接证据
	require.Equal(t, 99, service.UserDisplayPercent(99.99))
	require.Equal(t, service.UsageStatusNearLimit, service.ClassifyUsageStatus(99.99))
}

func phase41NewStatusService(t *testing.T, client *dbent.Client) *service.AccountStatusService {
	t.Helper()
	userRepo := NewUserRepository(client, integrationDB)
	subRepo := NewUserSubscriptionRepository(client)
	subSvc := service.NewSubscriptionService(nil, subRepo, nil, client, nil)
	return service.NewAccountStatusService(userRepo, subRepo, NewGroupRepository(client, integrationDB), subSvc, NewSubscriptionResetCardRepository(client), false)
}

// Reset Card 真实只读 COUNT：正常卡 / 永久卡 / 过期三边界 / 非可用状态。
func TestPhase41ResetCardCountAvailability(t *testing.T) {
	ctx := context.Background()
	client := testEntClient(t)
	reader := NewSubscriptionResetCardRepository(client)
	user := mustCreateUser(t, client, &service.User{
		Email: "phase41-cards@example.com", PasswordHash: "hash",
	})
	phase0CleanupStack(t, user.ID, 0, 0)

	now := time.Now()
	newCard := func(expires *time.Time, status string) int64 {
		card, err := client.SubscriptionResetCard.Create().
			SetUserID(user.ID).
			SetStatus(status).
			SetSourceType(domain.ResetCardSourceAdminGrant).
			SetMetadata(map[string]any{}).
			SetNillableExpiresAt(expires).
			Save(ctx)
		require.NoError(t, err)
		t.Cleanup(func() {
			_, _ = integrationDB.Exec("DELETE FROM subscription_reset_cards WHERE id = $1", card.ID)
		})
		return card.ID
	}

	// 无卡
	n, err := reader.CountAvailableResetCards(ctx, user.ID, now)
	require.NoError(t, err)
	require.Zero(t, n)

	// 永久有效卡（expires_at NULL）
	newCard(nil, domain.ResetCardStatusAvailable)
	// 未来过期
	future := now.Add(24 * time.Hour)
	newCard(&future, domain.ResetCardStatusAvailable)

	n, err = reader.CountAvailableResetCards(ctx, user.ID, now)
	require.NoError(t, err)
	require.Equal(t, 2, n)

	// 过期三边界：> now 可用；== now 不可用；< now 不可用（§17）
	past := now.Add(-time.Second)
	newCard(&past, domain.ResetCardStatusAvailable)
	exact := now.Add(time.Second) // 留 1 秒后过期；用更晚的 now 语义由调用方传入
	n, err = reader.CountAvailableResetCards(ctx, user.ID, exact)
	require.NoError(t, err)
	require.Equal(t, 2, n, "expires_at == now → unavailable（严格大于才可用）")

	n, err = reader.CountAvailableResetCards(ctx, user.ID, past.Add(time.Second))
	require.NoError(t, err)
	require.Equal(t, 2, n, "expires_at < now → unavailable")

	// 非可用状态不计入
	used := now.Add(24 * time.Hour)
	newCard(&used, domain.ResetCardStatusUsed)
	revoked := now.Add(24 * time.Hour)
	newCard(&revoked, domain.ResetCardStatusRevoked)
	n, err = reader.CountAvailableResetCards(ctx, user.ID, now)
	require.NoError(t, err)
	require.Equal(t, 2, n, "used/revoked cards must not count toward available")
}

// 账户级卡语义：多订阅用户的 reset_cards 在 Account 级一份，订阅条目不含卡字段；
// display_name 来自 Group（无 plan identity）。
func TestPhase41AccountLevelResetCardsAndDisplayName(t *testing.T) {
	ctx := context.Background()
	client := testEntClient(t)
	userRepo := NewUserRepository(client, integrationDB)
	subRepo := NewUserSubscriptionRepository(client)
	subSvc := service.NewSubscriptionService(nil, subRepo, nil, client, nil)
	svc := service.NewAccountStatusService(userRepo, subRepo, NewGroupRepository(client, integrationDB), subSvc, NewSubscriptionResetCardRepository(client), false)

	limit := 10.0
	user := mustCreateUser(t, client, &service.User{
		Email: "phase41-acct@example.com", PasswordHash: "hash",
	})
	gPro := mustCreateGroup(t, client, &service.Group{
		Name: "Pro", SubscriptionType: service.SubscriptionTypeSubscription, WeeklyLimitUSD: &limit,
	})
	gMax := mustCreateGroup(t, client, &service.Group{
		Name: "Max", SubscriptionType: service.SubscriptionTypeSubscription, WeeklyLimitUSD: &limit,
	})
	phase0CleanupStack(t, user.ID, gPro.ID, 0)
	t.Cleanup(func() { _, _ = integrationDB.Exec("DELETE FROM groups WHERE id = $1", gMax.ID) })

	subPro := mustCreateSubscription(t, client, &service.UserSubscription{UserID: user.ID, GroupID: gPro.ID})
	_ = mustCreateSubscription(t, client, &service.UserSubscription{UserID: user.ID, GroupID: gMax.ID})
	phase0SetWeeklyWindow(t, subPro.ID, time.Now().Add(-24*time.Hour), 6.3)

	// 发两张卡（模拟 campaign 赠送：直接落卡，Runtime 属后续 Phase）
	future := time.Now().Add(14 * 24 * time.Hour)
	for i := 0; i < 2; i++ {
		_, err := client.SubscriptionResetCard.Create().
			SetUserID(user.ID).
			SetSourceType(domain.ResetCardSourceCampaign).
			SetExpiresAt(future).
			SetMetadata(map[string]any{}).
			Save(ctx)
		require.NoError(t, err)
	}
	t.Cleanup(func() {
		_, _ = integrationDB.Exec("DELETE FROM subscription_reset_cards WHERE user_id = $1", user.ID)
	})

	status, err := svc.GetAccountStatus(ctx, user.ID)
	require.NoError(t, err)

	// 账户级一份：2 张卡 ≠ 每订阅各 2 张
	require.Equal(t, 2, status.ResetCards.Available)
	require.Len(t, status.Subscriptions, 2)

	// display_name 契约：来自 Group 名（Pro/Max），与任何 Plan SKU 名无关；
	// 编译期即不存在订阅级卡字段（卡计数只在 Account 级 reset_cards）。
	byGroup := map[int64]service.AccountSubscriptionStatus{}
	for _, st := range status.Subscriptions {
		byGroup[st.GroupID] = st
	}
	require.Equal(t, "Pro", byGroup[gPro.ID].DisplayName)
	require.Equal(t, "Max", byGroup[gMax.ID].DisplayName)
	require.Equal(t, 63, *byGroup[gPro.ID].WeeklyUsagePercent) // 仅 Pro 有 6.3/10
	require.Equal(t, 0, *byGroup[gMax.ID].WeeklyUsagePercent)
}

// 配置兜底回读：monitorOnly=true 的只读服务（不跑维护写入）同样返回正确状态。
func TestPhase41MonitorOnlyStatusService(t *testing.T) {
	ctx := context.Background()
	client := testEntClient(t)
	limit := 10.0
	user, group, sub := phase4MustStack(t, client, &limit, 6.3, -72*time.Hour)
	phase0CleanupStack(t, user.ID, group.ID, 0)

	userRepo := NewUserRepository(client, integrationDB)
	svc := service.NewAccountStatusService(userRepo, NewUserSubscriptionRepository(client), NewGroupRepository(client, integrationDB), nil, nil, true)
	_ = config.Config{} // config 未参与状态计算

	status, err := svc.GetAccountStatus(ctx, user.ID)
	require.NoError(t, err)
	require.Len(t, status.Subscriptions, 1)
	require.Equal(t, 63, *status.Subscriptions[0].WeeklyUsagePercent)
	require.Equal(t, sub.ID, status.Subscriptions[0].ID)
}
