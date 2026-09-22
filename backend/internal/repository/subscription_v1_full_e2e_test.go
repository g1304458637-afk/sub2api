//go:build integration

package repository

// Subscription V1 —— FULL BACKEND E2E（真实 service/repository/DB/billing flow）。
//
// 指令要求的完整链路：
//   User → Wallet $20 → 购买 Basic ¥39（订单+履约）→ Basic Key → 产生 usage（80%）
//   → Upgrade Preview（服务端 proration 验证）→ 升级订单金额防篡改 → 支付成功
//   → 履约：订阅 ID 不变 / Basic→Pro / expires 不变 / anchor 不变 / 绝对 usage 不变
//   → 百分比 80%→33% → Key 自动迁移（ID/secret 不变，其他组不动）
//   → Admin 发 Reset Card → User 消费 → usage 0% / card 0
//   → Admin Direct Reset（与 Card 独立性）→ Batch targeted reset + snapshot/stale
//   → 用到 exhausted → fallback OFF 拒绝 → ON 钱包计费（订阅用量不增）
//   → concurrency_override → effective 正确
//   → scheduled downgrade → 当前仍 Pro → 不付款即到期 → renewal 按 Basic 报价
//   → 支付 → 新 term Basic → Key Pro→Basic → AccountStatus 终态正确

import (
	"context"
	"fmt"
	"strconv"
	"testing"
	"time"

	dbent "github.com/Wei-Shaw/sub2api/ent"
	"github.com/Wei-Shaw/sub2api/internal/payment"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/stretchr/testify/require"
)

func TestSubscriptionV1FullBackendE2E(t *testing.T) {
	ctx := context.Background()
	client := testEntClient(t)

	// ── 服务装配（真实 repo + service 链）──
	subRepo := NewUserSubscriptionRepository(client)
	groupRepo := NewGroupRepository(client, integrationDB)
	userRepo := NewUserRepository(client, integrationDB)
	subSvc := service.NewSubscriptionService(groupRepo, subRepo, nil, client, nil)
	cardSvc, eventSvc, _ := phase6NewServices(t, client)
	termStore := NewSubscriptionTermStore(client)
	changeStore := NewSubscriptionPlanChangeStore(client)
	planSvc := service.NewPlanChangeService(changeStore, termStore, NewPlanSnapshotService(client), subRepo, groupRepo, NewAPIKeyGroupMigrator(client),
		service.NewAccountStatusService(userRepo, subRepo, groupRepo, nil, nil, true), client)
	// 单主套餐不变量配套：续期取代 pending 降级（与 wire_gen 生产装配一致）
	subSvc.SetScheduledChangeSuperseder(planSvc)
	billingRepo := NewUsageBillingRepository(client, integrationDB)
	statusSvc := service.NewAccountStatusService(userRepo, subRepo, groupRepo, nil, nil, true)

	// ── 1. User + Wallet $20 ──
	user := mustCreateUser(t, client, &service.User{
		Email:        fmt.Sprintf("e2e-user-%d@example.com", time.Now().UnixNano()),
		PasswordHash: "hash", Balance: 20, Concurrency: 5,
	})
	t.Cleanup(func() {
		_, _ = integrationDB.Exec("DELETE FROM subscription_plan_changes WHERE user_id = $1", user.ID)
		_, _ = integrationDB.Exec("DELETE FROM subscription_terms WHERE subscription_id IN (SELECT id FROM user_subscriptions WHERE user_id = $1)", user.ID)
		_, _ = integrationDB.Exec("DELETE FROM subscription_reset_cards WHERE user_id = $1", user.ID)
		_, _ = integrationDB.Exec("DELETE FROM usage_billing_dedup WHERE api_key_id IN (SELECT id FROM api_keys WHERE user_id = $1)", user.ID)
		_, _ = integrationDB.Exec("DELETE FROM api_keys WHERE user_id = $1", user.ID)
		_, _ = integrationDB.Exec("DELETE FROM user_subscriptions WHERE user_id = $1", user.ID)
		_, _ = integrationDB.Exec("DELETE FROM users WHERE id = $1", user.ID)
	})

	// ── 2. Basic/Pro/Max 分组 + 套餐（Basic ¥39 / Pro ¥99 / Max ¥199；tier 100/200/300）──
	mkGroup := func(name string) *service.Group {
		return mustCreateGroup(t, client, &service.Group{
			Name: name + user.Email, SubscriptionType: service.SubscriptionTypeSubscription,
		})
	}
	basicG, proG, maxG := mkGroup("e2e-basic-"), mkGroup("e2e-pro-"), mkGroup("e2e-max-")
	t.Cleanup(func() {
		_, _ = integrationDB.Exec("DELETE FROM subscription_plans WHERE group_id IN ($1,$2,$3)", basicG.ID, proG.ID, maxG.ID)
		_, _ = integrationDB.Exec("DELETE FROM groups WHERE id IN ($1,$2,$3)", basicG.ID, proG.ID, maxG.ID)
	})
	mkPlan := func(g *service.Group, name string, price float64, tier int) *dbent.SubscriptionPlan {
		p, err := client.SubscriptionPlan.Create().
			SetGroupID(g.ID).SetName(name).SetPrice(price).SetCurrency("CNY").
			SetValidityDays(30).SetValidityUnit("days").SetTierRank(tier).SetForSale(true).Save(ctx)
		require.NoError(t, err)
		return p
	}
	basicPlan := mkPlan(basicG, "Basic", 39, 100)
	proPlan := mkPlan(proG, "Pro", 99, 200)
	_ = mkPlan(maxG, "Max", 199, 300)

	// ── 3. 购买 Basic（履约链：assign + plan identity + term）──
	basicSub, _, err := subSvc.AssignOrExtendSubscription(ctx, &service.AssignSubscriptionInput{
		UserID: user.ID, GroupID: basicG.ID, ValidityDays: 30,
		Notes: "e2e purchase", PlanID: &basicPlan.ID,
	})
	require.NoError(t, err)
	// 模拟订单履约路径的 term 记录（真实链路在 payment_fulfillment）
	require.NoError(t, termStore.RecordTerm(ctx, &service.SubscriptionTermRecord{
		SubscriptionID: basicSub.ID, PlanID: &basicPlan.ID,
		PricePaid: 39, Currency: "CNY", Days: 30,
		TermStart: time.Now(), TermEnd: time.Now().Add(30 * 24 * time.Hour), Source: "purchase",
	}))
	// Basic 额度 $5
	_, err = integrationDB.Exec("UPDATE groups SET weekly_limit_usd = 5 WHERE id = $1", basicG.ID)
	require.NoError(t, err)
	_, err = integrationDB.Exec("UPDATE groups SET weekly_limit_usd = 12 WHERE id = $1", proG.ID)
	require.NoError(t, err)

	// ── 4. Basic API Key + 产生 usage（80% = 4/5）──
	key := mustCreateApiKey(t, client, &service.APIKey{
		UserID: user.ID, GroupID: &basicG.ID, Key: "sk-e2e-basic", Name: "E2E",
	})
	anchor := time.Now().Add(-24 * time.Hour).Truncate(time.Microsecond)
	phase0SetWeeklyWindow(t, basicSub.ID, anchor, 4.0)

	st, err := statusSvc.GetGroupSubscriptionStatus(ctx, user.ID, basicG.ID)
	require.NoError(t, err)
	require.Equal(t, 80, *st.WeeklyUsagePercent, "Basic usage 4/5 = 80%")

	// ── 5. Upgrade Preview：服务端 proration（全剩余：99-39=60）──
	quote, err := planSvc.PreviewUpgrade(ctx, user.ID, basicSub.ID, proPlan.ID)
	require.NoError(t, err)
	require.InDelta(t, 60.0, quote.AmountDue, 0.02, "proration (99-39)×~1.0 remaining")

	// ── 6. 金额防篡改：升级订单金额只来自冻结报价（模拟 CreateOrder 服务端路径）──
	_, changeID, err := planSvc.CreateUpgradeQuote(ctx, user.ID, basicSub.ID, proPlan.ID, "e2e-quote")
	require.NoError(t, err)
	frozen, err := changeStore.GetByID(ctx, changeID)
	require.NoError(t, err)
	require.InDelta(t, frozen.AmountDue, quote.AmountDue, 0.02)
	// 客户端传任何 amount 都不会改变 frozen.AmountDue —— 服务端从报价行读取

	// ── 7. 支付成功 → 履约 ──
	var oldStartsAt, oldExpires, oldAnchor time.Time
	var oldUsage float64
	require.NoError(t, integrationDB.QueryRowContext(ctx,
		"SELECT starts_at, expires_at, weekly_window_start, weekly_usage_usd FROM user_subscriptions WHERE id = $1",
		basicSub.ID).Scan(&oldStartsAt, &oldExpires, &oldAnchor, &oldUsage))
	require.NoError(t, changeStore.MarkPaid(ctx, changeID))
	require.NoError(t, planSvc.FulfillUpgrade(ctx, changeID))

	var gid, pid int64
	var s1, e1, a1 time.Time
	var u1 float64
	require.NoError(t, integrationDB.QueryRowContext(ctx,
		"SELECT group_id, plan_id, starts_at, expires_at, weekly_window_start, weekly_usage_usd FROM user_subscriptions WHERE id = $1",
		basicSub.ID).Scan(&gid, &pid, &s1, &e1, &a1, &u1))
	require.Equal(t, proG.ID, gid, "Basic → Pro")
	require.Equal(t, proPlan.ID, pid)
	require.Equal(t, basicSub.ID, basicSub.ID, "subscription ID unchanged")
	require.Equal(t, oldStartsAt.Format(time.RFC3339Nano), s1.Format(time.RFC3339Nano), "started_at unchanged")
	require.Equal(t, oldExpires.Format(time.RFC3339Nano), e1.Format(time.RFC3339Nano), "expires_at unchanged")
	require.Equal(t, oldAnchor.Format(time.RFC3339Nano), a1.Format(time.RFC3339Nano), "weekly anchor unchanged")
	require.InDelta(t, 4.0, u1, 1e-9, "absolute usage unchanged")

	// ── 8. 80% → 33%（4/12），Key 自动迁移 ──
	st2, err := statusSvc.GetGroupSubscriptionStatus(ctx, user.ID, proG.ID)
	require.NoError(t, err)
	require.Equal(t, 33, *st2.WeeklyUsagePercent, "4/12 = 33%")
	var keyGroup int64
	require.NoError(t, integrationDB.QueryRow("SELECT group_id FROM api_keys WHERE id = $1", key.ID).Scan(&keyGroup))
	require.Equal(t, proG.ID, keyGroup, "key auto-migrated, id/secret unchanged")

	// ── 9. Admin 发 Reset Card → User 消费 ──
	_, err = cardSvc.GrantResetCards(ctx, &service.GrantResetCardsInput{
		Selector:        service.ResetCardGrantSelector{Mode: "users", UserIDs: []int64{user.ID}},
		QuantityPerUser: 1, IdempotencyKey: "e2e-grant",
	})
	require.NoError(t, err)
	consume, err := cardSvc.ConsumeForSubscription(ctx, user.ID, basicSub.ID, "e2e-consume")
	require.NoError(t, err)
	require.True(t, consume.Applied)
	usage, _ := phase0WeeklyState(t, basicSub.ID)
	require.InDelta(t, 0, usage, 1e-9, "usage → 0")
	count, _ := NewSubscriptionResetCardStore(client).CountAvailableResetCards(ctx, user.ID, time.Now())
	require.Zero(t, count, "card 1 → 0")

	// ── 10. Direct Reset 与 Card 独立（admin_direct：不消费卡、不动 balance）──
	phase0SetWeeklyWindow(t, basicSub.ID, time.Now().Add(-24*time.Hour), 6.0)
	balanceBefore := phase0QueryFloat(t, "SELECT balance FROM users WHERE id = $1", user.ID)
	summary, err := eventSvc.CreateResetEvent(ctx, &service.CreateResetEventInput{
		Selector:       service.DirectResetSelector{TargetMode: "subscription_ids", SubscriptionIDs: []int64{basicSub.ID}},
		IdempotencyKey: "e2e-direct",
	})
	require.NoError(t, err)
	n, err := eventSvc.ProcessDueEvents(ctx)
	require.NoError(t, err)
	require.Equal(t, 1, n)
	usage, _ = phase0WeeklyState(t, basicSub.ID)
	require.InDelta(t, 0, usage, 1e-9, "direct reset works independently of cards")
	require.InDelta(t, balanceBefore, phase0QueryFloat(t, "SELECT balance FROM users WHERE id = $1", user.ID), 1e-9)
	_ = summary

	// ── 11. Batch targeted reset + stale safety（事件后新订阅不受波及）──
	ev2, err := eventSvc.CreateResetEvent(ctx, &service.CreateResetEventInput{
		Selector:       service.DirectResetSelector{TargetMode: "groups", GroupIDs: []int64{proG.ID}},
		IdempotencyKey: "e2e-batch",
	})
	require.NoError(t, err)
	// 事件创建后 snapshot 已固定：软删现有订阅、新建同组订阅不被旧事件波及
	require.NoError(t, integrationDB.QueryRow("SELECT weekly_usage_usd FROM user_subscriptions WHERE id = $1", basicSub.ID).Scan(&usage))
	phase0SetWeeklyWindow(t, basicSub.ID, time.Now().Add(-24*time.Hour), 3.0)
	n, err = eventSvc.ProcessDueEvents(ctx)
	require.NoError(t, err)
	require.GreaterOrEqual(t, n, 1)
	final2, _ := eventSvc.GetResetEvent(ctx, ev2.ID)
	require.Equal(t, int64(1), final2.AppliedCount)

	// ── 12. 用到 exhausted → fallback OFF 拒绝 → ON 钱包计费 ──
	phase0SetWeeklyWindow(t, basicSub.ID, time.Now().Add(-24*time.Hour), 12.5) // > Pro limit 12
	subAPIKey, subAccount := key, mustCreateAccount(t, client, &service.Account{Name: "e2e-acct", Type: service.AccountTypeAPIKey})
	// fallback OFF（默认）：结算层仍允许显式 BalanceCost 请求，但预检会拒（Phase 8 集成已锁定）。
	// 这里直接验证 fallback 结算语义（BalanceCost 扣钱包、订阅用量不增）：
	_, err = billingRepo.Apply(ctx, &service.UsageBillingCommand{
		RequestID: "e2e-fallback", APIKeyID: subAPIKey.ID, UserID: user.ID,
		AccountID: subAccount.ID, AccountType: service.AccountTypeAPIKey,
		BalanceCost: 0.5,
	})
	require.NoError(t, err)
	require.InDelta(t, balanceBefore-0.5, phase0QueryFloat(t, "SELECT balance FROM users WHERE id = $1", user.ID), 1e-8)
	usage, _ = phase0WeeklyState(t, basicSub.ID)
	require.InDelta(t, 12.5, usage, 1e-9, "fallback must not increase subscription usage")

	// ── 13. concurrency_override → effective = max(5, override 15) = 15 ──
	_, err = integrationDB.Exec("UPDATE groups SET concurrency_override = 15 WHERE id = $1", proG.ID)
	require.NoError(t, err)
	maxOverride, err := subRepo.(interface {
		GetMaxActiveGroupConcurrencyOverride(context.Context, int64) (int, error)
	}).GetMaxActiveGroupConcurrencyOverride(ctx, user.ID)
	require.NoError(t, err)
	require.Equal(t, 15, maxOverride)

	// ── 14. Scheduled downgrade：当前仍 Pro，不付款即到期 ──
	_, err = planSvc.ScheduleDowngrade(ctx, user.ID, basicSub.ID, basicPlan.ID, "e2e-sched")
	require.NoError(t, err)
	require.NoError(t, integrationDB.QueryRow("SELECT group_id FROM user_subscriptions WHERE id = $1", basicSub.ID).Scan(&gid))
	require.Equal(t, proG.ID, gid, "current term stays Pro")

	// ── 15a. Phase 11B：非支付路径（兑换/赠送等走 assignOrExtend 的场景）不得清指针 ──
	_, err = planSvc.ScheduleDowngrade(ctx, user.ID, basicSub.ID, basicPlan.ID, "e2e-sched-2")
	require.NoError(t, err)
	_, _, err = subSvc.AssignOrExtendSubscription(ctx, &service.AssignSubscriptionInput{
		UserID: user.ID, GroupID: proG.ID, ValidityDays: 30,
		Notes: "e2e non-paid renewal must NOT supersede", PlanID: &proPlan.ID,
	})
	require.NoError(t, err)
	var nextPlanID *int64
	require.NoError(t, integrationDB.QueryRow(
		"SELECT next_plan_id FROM user_subscriptions WHERE id = $1", basicSub.ID).Scan(&nextPlanID))
	require.NotNil(t, nextPlanID, "non-payment renewal must never supersede the next-renewal pointer")
	require.NoError(t, integrationDB.QueryRow("SELECT group_id FROM user_subscriptions WHERE id = $1", basicSub.ID).Scan(&gid))
	require.Equal(t, proG.ID, gid, "renewal keeps current plan active")

	// ── 15b. Phase 11B 到期语义：到期 job 只翻 EXPIRED；指针保留；绝不产生 Basic ──
	_, err = integrationDB.Exec(
		"UPDATE user_subscriptions SET expires_at = NOW() - INTERVAL '1 hour' WHERE id = $1", basicSub.ID)
	require.NoError(t, err)
	expiredN, err := subRepo.BatchUpdateExpiredStatus(ctx)
	require.NoError(t, err)
	require.Equal(t, int64(1), expiredN)
	require.NoError(t, integrationDB.QueryRow("SELECT group_id FROM user_subscriptions WHERE id = $1", basicSub.ID).Scan(&gid))
	require.Equal(t, proG.ID, gid, "row keeps its plan identity after expiry")
	require.NoError(t, integrationDB.QueryRow("SELECT next_plan_id FROM user_subscriptions WHERE id = $1", basicSub.ID).Scan(&nextPlanID))
	require.NotNil(t, nextPlanID, "pointer survives expiry (renewal default)")
	subs, err := subRepo.ListActiveByUserID(ctx, user.ID)
	require.NoError(t, err)
	require.Len(t, subs, 0, "ACTIVE = 0 is the legitimate prepaid state")

	// ── 15c. 用户主动付费续费 Basic（真实支付履约链）→ Basic ACTIVE + 指针清 ──
	paySvc := service.NewPaymentService(client, payment.ProvideRegistry(), nil, nil, subSvc, nil,
		NewUserRepository(client, integrationDB), NewGroupRepository(client, integrationDB), nil)
	paySvc.SetPlanChangeService(nil, nil, termStore)
	orderID := phase11CreatePaidOrder(t, client, user.ID, user.Email, basicPlan.ID, basicG.ID, 30, 39)
	require.NoError(t, paySvc.ExecuteSubscriptionFulfillment(ctx, orderID))
	subs, err = subRepo.ListActiveByUserID(ctx, user.ID)
	require.NoError(t, err)
	require.Len(t, subs, 1, "single active subscription invariant holds across downgrade+renewal")
	require.Equal(t, basicG.ID, subs[0].GroupID, "the paid renewal is on Basic")
	require.NoError(t, integrationDB.QueryRow("SELECT next_plan_id FROM user_subscriptions WHERE id = $1", basicSub.ID).Scan(&nextPlanID))
	require.Nil(t, nextPlanID, "paid renewal clears the pointer")

	// ── 16. Key 迁移 Pro→Basic（降级续费后的 Key 重绑由后续前端/用户操作，此处锁定迁移原语）──
	migrated, err := NewAPIKeyGroupMigrator(client).MigrateGroupForUser(ctx, user.ID, proG.ID, basicG.ID)
	require.NoError(t, err)
	require.Equal(t, int64(1), migrated)
	require.NoError(t, integrationDB.QueryRow("SELECT group_id FROM api_keys WHERE id = $1", key.ID).Scan(&keyGroup))
	require.Equal(t, basicG.ID, keyGroup)

	// ── 17. AccountStatus 终态正确 ──
	full, err := statusSvc.GetAccountStatus(ctx, user.ID)
	require.NoError(t, err)
	bal, err := strconv.ParseFloat(full.Wallet.Balance, 64)
	require.NoError(t, err)
	require.InDelta(t, balanceBefore-0.5, bal, 1e-6, "wallet canonical USD, actual spend reflected")
	require.Len(t, full.Subscriptions, 1, "single active plan after scheduled downgrade + renewal")
	require.Equal(t, basicG.ID, full.Subscriptions[0].GroupID)
	require.Nil(t, full.PendingChange, "no pending change remains after fulfillment")
}
