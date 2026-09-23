//go:build integration

package repository

// Phase 10 —— Plan Change Runtime 集成测试。
//
// 覆盖：Quote 数学（剩余比例/预付 45/60 天）、tier 闸门、同档拒绝、目标组冲突、
// 报价过期、跨币种拒绝、cadence 拒绝、identity unresolved、履约不变量
// （ID/started/expires/anchor/usage 不变 + 组/档切换 + Key 迁移 + wallet 不变）、
// 支付回调幂等、No Hidden Reset（80%→33%）、scheduled downgrade 全矩阵、
// 手动续费按降级目标档执行 + Key 迁移。

import (
	"context"
	"database/sql"
	"fmt"
	"testing"
	"time"

	dbent "github.com/Wei-Shaw/sub2api/ent"
	"github.com/Wei-Shaw/sub2api/ent/subscriptionplan"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/stretchr/testify/require"
)

func phase10NewPlanChangeService(t *testing.T, client *dbent.Client) (*service.PlanChangeService, service.PlanChangeStore, service.TermStore) {
	t.Helper()
	subRepo := NewUserSubscriptionRepository(client)
	groupRepo := NewGroupRepository(client, integrationDB)
	termStore := NewSubscriptionTermStore(client)
	changeStore := NewSubscriptionPlanChangeStore(client)
	plans := NewPlanSnapshotService(client)
	migrator := NewAPIKeyGroupMigrator(client)
	statusSvc := service.NewAccountStatusService(NewUserRepository(client, integrationDB), subRepo, groupRepo, nil, nil, true)
	svc := service.NewPlanChangeService(changeStore, termStore, plans, subRepo, groupRepo, migrator, statusSvc, client)
	return svc, changeStore, termStore
}

type phase10Stack struct {
	user      *service.User
	basicG    *service.Group
	proG      *service.Group
	maxG      *service.Group
	basicSub  *service.UserSubscription
	proPlan   *dbent.SubscriptionPlan
	basicPlan *dbent.SubscriptionPlan
	svc       *service.PlanChangeService
	terms     service.TermStore
	changes   service.PlanChangeStore
}

// 建 Basic(plan_id/tier=100)+Pro(200)+Max(300) 三组三 SKU；用户持 Basic 订阅
// （plan 身份已解析 + 已付 term 30 天，价格 basicPrice）。
func phase10Setup(t *testing.T, client *dbent.Client, basicPrice, proPrice float64, termStartAgo, termDays int) *phase10Stack {
	t.Helper()
	ctx := context.Background()
	user := mustCreateUser(t, client, &service.User{
		Email: fmt.Sprintf("phase10-user-%d@example.com", time.Now().UnixNano()), PasswordHash: "hash", Balance: 20,
	})
	basicG := mustCreateGroup(t, client, &service.Group{
		Name: "phase10-basic-" + user.Email, SubscriptionType: service.SubscriptionTypeSubscription,
	})
	proG := mustCreateGroup(t, client, &service.Group{
		Name: "phase10-pro-" + user.Email, SubscriptionType: service.SubscriptionTypeSubscription,
	})
	maxG := mustCreateGroup(t, client, &service.Group{
		Name: "phase10-max-" + user.Email, SubscriptionType: service.SubscriptionTypeSubscription,
	})
	mkPlan := func(g *service.Group, name string, price float64, tier int) *dbent.SubscriptionPlan {
		p, err := client.SubscriptionPlan.Create().
			SetGroupID(g.ID).
			SetName(name).
			SetPrice(price).
			SetCurrency("CNY").
			SetValidityDays(30).
			SetValidityUnit("days").
			SetTierRank(tier).
			SetForSale(true).
			Save(ctx)
		require.NoError(t, err)
		return p
	}
	basicPlan := mkPlan(basicG, "Basic", basicPrice, 100)
	proPlan := mkPlan(proG, "Pro", proPrice, 200)
	mkPlan(maxG, "Max", 199, 300)

	now := time.Now()
	sub := mustCreateSubscription(t, client, &service.UserSubscription{
		UserID: user.ID, GroupID: basicG.ID,
	})
	planID := basicPlan.ID
	_, err := integrationDB.ExecContext(ctx,
		"UPDATE user_subscriptions SET plan_id = $1, starts_at = $2, expires_at = $3 WHERE id = $4",
		planID, now.Add(-time.Duration(termStartAgo)*24*time.Hour), now.Add(time.Duration(termDays-termStartAgo)*24*time.Hour), sub.ID)
	require.NoError(t, err)

	svc, changes, terms := phase10NewPlanChangeService(t, client)
	// 回读：fixture 返回值的 ExpiresAt 是默认值；UPDATE 后以 DB 为准
	freshSub, err := NewUserSubscriptionRepository(client).GetByID(ctx, sub.ID)
	require.NoError(t, err)
	sub = freshSub
	termStart := now.Add(-time.Duration(termStartAgo) * 24 * time.Hour)
	termEnd := now.Add(time.Duration(termDays-termStartAgo) * 24 * time.Hour)
	require.NoError(t, terms.RecordTerm(ctx, &service.SubscriptionTermRecord{
		SubscriptionID: sub.ID, PlanID: &planID,
		PricePaid: basicPrice, Currency: "CNY", Days: termDays,
		TermStart: termStart, TermEnd: termEnd, Source: "purchase",
	}))

	cleanup := func() {
		_, _ = integrationDB.Exec("DELETE FROM subscription_plan_changes WHERE user_id = $1", user.ID)
		_, _ = integrationDB.Exec("DELETE FROM subscription_terms WHERE subscription_id = $1", sub.ID)
		_, _ = integrationDB.Exec("DELETE FROM user_subscriptions WHERE user_id = $1", user.ID)
		_, _ = integrationDB.Exec("DELETE FROM subscription_plans WHERE group_id IN ($1,$2,$3)", basicG.ID, proG.ID, maxG.ID)
		_, _ = integrationDB.Exec("DELETE FROM groups WHERE id IN ($1,$2,$3)", basicG.ID, proG.ID, maxG.ID)
		_, _ = integrationDB.Exec("DELETE FROM users WHERE id = $1", user.ID)
	}
	t.Cleanup(cleanup)

	return &phase10Stack{
		user: user, basicG: basicG, proG: proG, maxG: maxG,
		basicSub: sub, basicPlan: basicPlan, proPlan: proPlan,
		svc: svc, terms: terms, changes: changes,
	}
}

// ---- Quote 数学 ----

func TestPhase10QuoteRemainingRatios(t *testing.T) {
	client := testEntClient(t)

	// 剩余 100%（30 天整）：39→99 应收 60
	s := phase10Setup(t, client, 39, 99, 0, 30)
	q, err := s.svc.PreviewUpgrade(context.Background(), s.user.ID, s.basicSub.ID, s.proPlan.ID)
	require.NoError(t, err)
	require.InDelta(t, 60.0, q.AmountDue, 0.011, "full remaining: (99-39)×1")

	// 剩余 50%（15/30）：应收 30
	s2 := phase10Setup(t, client, 39, 99, 15, 30)
	q2, err := s2.svc.PreviewUpgrade(context.Background(), s2.user.ID, s2.basicSub.ID, s2.proPlan.ID)
	require.NoError(t, err)
	require.InDelta(t, 30.0, q2.AmountDue, 0.011, "half remaining")

	// 剩余 ~1/3：deterministic 2 位小数
	s3 := phase10Setup(t, client, 39, 99, 20, 30)
	q3, err := s3.svc.PreviewUpgrade(context.Background(), s3.user.ID, s3.basicSub.ID, s3.proPlan.ID)
	require.NoError(t, err)
	require.InDelta(t, 20.0, q3.AmountDue, 0.011)

	// Basic→Max / Pro→Max 组合
	s4 := phase10Setup(t, client, 39, 99, 0, 30)
	maxPlan, err := client.SubscriptionPlan.Query().Where(subscriptionplan.GroupIDEQ(s4.maxG.ID)).Only(context.Background())
	require.NoError(t, err)
	q4, err := s4.svc.PreviewUpgrade(context.Background(), s4.user.ID, s4.basicSub.ID, maxPlan.ID)
	require.NoError(t, err)
	require.InDelta(t, 160.0, q4.AmountDue, 0.011, "39→199 full remaining")
}

// 预付 45/60 天（多段 term）：expires 不变 + 全部剩余权益计价。
func TestPhase10QuotePrepaidExtension(t *testing.T) {
	client := testEntClient(t)
	ctx := context.Background()

	for _, days := range []int{45, 60} {
		s := phase10Setup(t, client, 39, 99, 15, 30)
		// 追加预付段：+15 或 +30 天（续费 39）
		basicPlanID := s.basicPlan.ID
		segStart := s.basicSub.ExpiresAt
		// 追加段：39 元/30 天的整价续费段（Days=追加天数；价格按日归一表达）
		require.NoError(t, s.terms.RecordTerm(ctx, &service.SubscriptionTermRecord{
			SubscriptionID: s.basicSub.ID, PlanID: &basicPlanID,
			PricePaid: 39 * float64(days-30) / 30.0, Currency: "CNY", Days: days - 30,
			TermStart: segStart, TermEnd: segStart.Add(time.Duration(days-30) * 24 * time.Hour), Source: "renewal",
		}))
		_, err := integrationDB.ExecContext(ctx,
			"UPDATE user_subscriptions SET expires_at = $1 WHERE id = $2",
			segStart.Add(time.Duration(days-30)*24*time.Hour), s.basicSub.ID)
		require.NoError(t, err)

		q, err := s.svc.PreviewUpgrade(ctx, s.user.ID, s.basicSub.ID, s.proPlan.ID)
		require.NoError(t, err)
		// 未消费 ≈ days-15 天；credit=39×(days-15)/30，charge=99×(days-15)/30 → due=60×(days-15)/30
		ratio := float64(days-15) / 30.0
		require.InDelta(t, 60.0*ratio, q.AmountDue, 0.05,
			"prepaid %dd: all unconsumed entitlement must be priced", days)
		require.True(t, q.NewExpiry.Equal(q.CurrentExpiry), "upgrade must not change expiry")
	}
}

// withoutSingleActiveIndex 临时撤掉单主套餐唯一索引（t.Cleanup 重建）。
// 仅用于 P10 遗留用例：其场景语义建立在旧"双持"模型上（同用户 Basic+Pro 并存），
// 与迁移 242 的不变量冲突；这些用例回归的下游逻辑（tier 闸门 / 降级矩阵 /
// 续费按目标档）本身仍然有效。新模型下的不变量由 phase11 测试单独守护。
func withoutSingleActiveIndex(t *testing.T) {
	t.Helper()
	_, err := integrationDB.Exec("DROP INDEX IF EXISTS uq_user_subscriptions_single_active")
	require.NoError(t, err)
	t.Cleanup(func() {
		_, _ = integrationDB.Exec(
			"CREATE UNIQUE INDEX IF NOT EXISTS uq_user_subscriptions_single_active " +
				"ON user_subscriptions(user_id) WHERE deleted_at IS NULL AND status = 'active'")
	})
}

// ---- 闸门 ----

func TestPhase10QuoteGates(t *testing.T) {
	withoutSingleActiveIndex(t)
	client := testEntClient(t)
	ctx := context.Background()

	// 同档 → no_change
	s := phase10Setup(t, client, 39, 99, 0, 30)
	_, err := s.svc.PreviewUpgrade(ctx, s.user.ID, s.basicSub.ID, s.basicPlan.ID)
	require.ErrorIs(t, err, service.ErrPlanNoChange)

	// 降级目标走 upgrade 入口 → no_change（应走 schedule-downgrade）
	_, err = s.svc.PreviewUpgrade(ctx, s.user.ID, s.basicSub.ID, s.proPlan.ID)
	require.NoError(t, err)
	proSub := mustCreateSubscription(t, client, &service.UserSubscription{UserID: s.user.ID, GroupID: s.proG.ID})
	_, err = integrationDB.ExecContext(ctx,
		"UPDATE user_subscriptions SET plan_id = $1 WHERE id = $2", s.proPlan.ID, proSub.ID)
	require.NoError(t, err)
	_, err = s.svc.PreviewUpgrade(ctx, s.user.ID, proSub.ID, s.basicPlan.ID)
	require.ErrorIs(t, err, service.ErrPlanNoChange)
	_, _ = integrationDB.ExecContext(ctx, "DELETE FROM user_subscriptions WHERE id = $1", proSub.ID)

	// 目标组已有 active → 拒绝（Preview 阶段）
	s2 := phase10Setup(t, client, 39, 99, 0, 30)
	proSub2 := mustCreateSubscription(t, client, &service.UserSubscription{UserID: s2.user.ID, GroupID: s2.proG.ID})
	_, _ = integrationDB.ExecContext(ctx, "UPDATE user_subscriptions SET plan_id = $1 WHERE id = $2", s2.proPlan.ID, proSub2.ID)
	_, err = s2.svc.PreviewUpgrade(ctx, s2.user.ID, s2.basicSub.ID, s2.proPlan.ID)
	require.ErrorIs(t, err, service.ErrPlanTargetGroupActive)

	// tier 未配置 → 拒绝
	s3 := phase10Setup(t, client, 39, 99, 0, 30)
	noTier, err := client.SubscriptionPlan.Create().
		SetGroupID(s3.maxG.ID).SetName("NoTier").SetPrice(199).SetCurrency("CNY").
		SetValidityDays(30).SetValidityUnit("days").SetTierRank(0).Save(ctx)
	require.NoError(t, err)
	_, err = s3.svc.PreviewUpgrade(ctx, s3.user.ID, s3.basicSub.ID, noTier.ID)
	require.ErrorIs(t, err, service.ErrPlanTierNotConfigured)

	// identity unresolved（plan_id NULL 的历史订阅）→ 拒绝
	s4 := phase10Setup(t, client, 39, 99, 0, 30)
	_, err = integrationDB.ExecContext(ctx, "UPDATE user_subscriptions SET plan_id = NULL WHERE id = $1", s4.basicSub.ID)
	require.NoError(t, err)
	_, err = s4.svc.PreviewUpgrade(ctx, s4.user.ID, s4.basicSub.ID, s4.proPlan.ID)
	require.ErrorIs(t, err, service.ErrPlanIdentityUnresolved)

	// 公共套餐接口只接受 CNY。直接注入一个异常旧数据行，确认服务层仍拒绝跨币种报价。
	s6 := phase10Setup(t, client, 39, 99, 0, 30)
	usdPlan, err := client.SubscriptionPlan.Create().
		SetGroupID(s6.maxG.ID).SetName("USDPro").SetPrice(15).SetCurrency("CNY").
		SetValidityDays(30).SetValidityUnit("days").SetTierRank(300).Save(ctx)
	require.NoError(t, err)
	_, err = integrationDB.ExecContext(ctx, "UPDATE subscription_plans SET currency = 'USD' WHERE id = $1", usdPlan.ID)
	require.NoError(t, err)
	_, err = s6.svc.PreviewUpgrade(ctx, s6.user.ID, s6.basicSub.ID, usdPlan.ID)
	require.ErrorIs(t, err, service.ErrPlanCrossCurrency)

	// cadence 不一致（from 30 天 vs to 60 天标称 Plan）→ 拒绝；
	// 已付 term 的段长（预付/赠送段）不触发拒绝（按日归一计价）
	s5 := phase10Setup(t, client, 39, 99, 0, 30)
	sixtyDay, err := client.SubscriptionPlan.Create().
		SetGroupID(s5.maxG.ID).SetName("SixtyDay").SetPrice(199).SetCurrency("CNY").
		SetValidityDays(60).SetValidityUnit("days").SetTierRank(300).Save(ctx)
	require.NoError(t, err)
	_, err = s5.svc.PreviewUpgrade(ctx, s5.user.ID, s5.basicSub.ID, sixtyDay.ID)
	require.ErrorIs(t, err, service.ErrPlanTermMismatch)
}

// ---- 履约不变量 + No Hidden Reset + Key 迁移 ----

func TestPhase10FulfillmentInvariants(t *testing.T) {
	client := testEntClient(t)
	ctx := context.Background()

	// Basic 80%（usage 4 / limit 5）→ 升级 Pro（limit 12）→ 显示 33%，绝不清零
	s := phase10Setup(t, client, 39, 99, 0, 30)
	basicLimit := 5.0
	proLimit := 12.0
	_, err := integrationDB.ExecContext(ctx, "UPDATE groups SET weekly_limit_usd = $1 WHERE id = $2", basicLimit, s.basicG.ID)
	require.NoError(t, err)
	_, err = integrationDB.ExecContext(ctx, "UPDATE groups SET weekly_limit_usd = $1 WHERE id = $2", proLimit, s.proG.ID)
	require.NoError(t, err)
	anchor := time.Now().Add(-24 * time.Hour).Truncate(time.Microsecond)
	phase0SetWeeklyWindow(t, s.basicSub.ID, anchor, 4.0)
	var oldStartsAt, oldExpiresAt, oldAnchor time.Time
	var oldUsage float64
	require.NoError(t, integrationDB.QueryRowContext(ctx,
		"SELECT starts_at, expires_at, weekly_window_start, weekly_usage_usd FROM user_subscriptions WHERE id = $1",
		s.basicSub.ID).Scan(&oldStartsAt, &oldExpiresAt, &oldAnchor, &oldUsage))

	// Key：Basic ×2 + 其他组 ×1
	keyA := mustCreateApiKey(t, client, &service.APIKey{UserID: s.user.ID, GroupID: &s.basicG.ID, Key: "sk-p10-a", Name: "A"})
	keyB := mustCreateApiKey(t, client, &service.APIKey{UserID: s.user.ID, GroupID: &s.basicG.ID, Key: "sk-p10-b", Name: "B"})
	keyC := mustCreateApiKey(t, client, &service.APIKey{UserID: s.user.ID, GroupID: &s.maxG.ID, Key: "sk-p10-c", Name: "C"})
	t.Cleanup(func() {
		_, _ = integrationDB.Exec("DELETE FROM api_keys WHERE user_id = $1", s.user.ID)
	})

	// Quote + 标记 paid + 履约（模拟支付回调后的路径）
	_, changeID, err := s.svc.CreateUpgradeQuote(ctx, s.user.ID, s.basicSub.ID, s.proPlan.ID, "p10-fulfill-1")
	require.NoError(t, err)
	require.NoError(t, s.changes.MarkPaid(ctx, changeID))
	require.NoError(t, s.svc.FulfillUpgrade(ctx, changeID))

	var groupID, planID int64
	var startsAt, expiresAt, newAnchor time.Time
	var usage float64
	require.NoError(t, integrationDB.QueryRowContext(ctx,
		"SELECT group_id, plan_id, starts_at, expires_at, weekly_window_start, weekly_usage_usd FROM user_subscriptions WHERE id = $1",
		s.basicSub.ID).Scan(&groupID, &planID, &startsAt, &expiresAt, &newAnchor, &usage))
	require.Equal(t, s.proG.ID, groupID, "entitlement switched to Pro group")
	require.Equal(t, s.proPlan.ID, planID)
	require.Equal(t, oldStartsAt.Format(time.RFC3339Nano), startsAt.Format(time.RFC3339Nano), "started_at unchanged")
	require.Equal(t, oldExpiresAt.Format(time.RFC3339Nano), expiresAt.Format(time.RFC3339Nano), "expires_at unchanged")
	require.Equal(t, oldAnchor.Format(time.RFC3339Nano), newAnchor.Format(time.RFC3339Nano), "weekly anchor unchanged (no hidden reset)")
	require.InDelta(t, 4.0, usage, 1e-9, "absolute usage unchanged")

	// Key 迁移：A/B → Pro；C 不动
	for _, kid := range []int64{keyA.ID, keyB.ID} {
		var kg int64
		require.NoError(t, integrationDB.QueryRowContext(ctx, "SELECT group_id FROM api_keys WHERE id = $1", kid).Scan(&kg))
		require.Equal(t, s.proG.ID, kg, "basic keys must migrate to Pro, id/secret unchanged")
	}
	var kg int64
	require.NoError(t, integrationDB.QueryRowContext(ctx, "SELECT group_id FROM api_keys WHERE id = $1", keyC.ID).Scan(&kg))
	require.Equal(t, s.maxG.ID, kg, "other-group keys untouched")

	// Wallet 不变
	balance := phase0QueryFloat(t, "SELECT balance FROM users WHERE id = $1", s.user.ID)
	require.InDelta(t, 20.0, balance, 1e-9)

	// 重复履约：幂等 no-op
	require.NoError(t, s.svc.FulfillUpgrade(ctx, changeID))
	require.NoError(t, integrationDB.QueryRowContext(ctx,
		"SELECT weekly_usage_usd FROM user_subscriptions WHERE id = $1", s.basicSub.ID).Scan(&usage))
	require.InDelta(t, 4.0, usage, 1e-9)

	// 升级后百分比 4/12 = 33%（经 AccountStatusService 权威计算）
	subRepo := NewUserSubscriptionRepository(client)
	groupRepo := NewGroupRepository(client, integrationDB)
	statusSvc := service.NewAccountStatusService(NewUserRepository(client, integrationDB), subRepo, groupRepo, nil, nil, true)
	st, err := statusSvc.GetGroupSubscriptionStatus(ctx, s.user.ID, s.proG.ID)
	require.NoError(t, err)
	require.NotNil(t, st.WeeklyUsagePercent)
	require.Equal(t, 33, *st.WeeklyUsagePercent, "4/12 must render 33%, not 0%")
	require.Contains(t, st.DisplayName, "phase10-pro-", "entitlement display name = new group name")
}

// ---- Scheduled Downgrade ----

func TestPhase10ScheduledDowngradeMatrix(t *testing.T) {
	withoutSingleActiveIndex(t)
	client := testEntClient(t)
	ctx := context.Background()

	s := phase10Setup(t, client, 39, 99, 0, 30)
	// 用户先持有 Pro（第二个订阅）验证降级
	proSub := mustCreateSubscription(t, client, &service.UserSubscription{UserID: s.user.ID, GroupID: s.proG.ID})
	_, err := integrationDB.ExecContext(ctx,
		"UPDATE user_subscriptions SET plan_id = $1, expires_at = NOW() + INTERVAL '15 days' WHERE id = $2",
		s.proPlan.ID, proSub.ID)
	require.NoError(t, err)
	t.Cleanup(func() { _, _ = integrationDB.Exec("DELETE FROM user_subscriptions WHERE id = $1", proSub.ID) })
	// Pro 侧已付 term
	proPlanID := s.proPlan.ID
	require.NoError(t, s.terms.RecordTerm(ctx, &service.SubscriptionTermRecord{
		SubscriptionID: proSub.ID, PlanID: &proPlanID,
		PricePaid: 99, Currency: "CNY", Days: 30,
		TermStart: time.Now().Add(-15 * 24 * time.Hour), TermEnd: time.Now().Add(15 * 24 * time.Hour), Source: "purchase",
	}))
	t.Cleanup(func() {
		_, _ = integrationDB.Exec("DELETE FROM subscription_terms WHERE subscription_id = $1", proSub.ID)
	})

	// ① 调度降级：立即无变化
	rec, err := s.svc.ScheduleDowngrade(ctx, s.user.ID, proSub.ID, s.basicPlan.ID, "p10-sched-1")
	require.NoError(t, err)
	require.Equal(t, "scheduled", rec.Status)
	var nextPlan sql.NullInt64
	require.NoError(t, integrationDB.QueryRowContext(ctx,
		"SELECT next_plan_id FROM user_subscriptions WHERE id = $1", proSub.ID).Scan(&nextPlan))
	require.True(t, nextPlan.Valid)
	require.Equal(t, s.basicPlan.ID, nextPlan.Int64)
	var groupID int64
	require.NoError(t, integrationDB.QueryRowContext(ctx,
		"SELECT group_id FROM user_subscriptions WHERE id = $1", proSub.ID).Scan(&groupID))
	require.Equal(t, s.proG.ID, groupID, "current term stays Pro")

	// ② 升级取消旧降级（superseded_by_upgrade）
	maxPlan, err := client.SubscriptionPlan.Query().Where(subscriptionplan.GroupIDEQ(s.maxG.ID)).Only(ctx)
	require.NoError(t, err)
	require.NoError(t, s.terms.RecordTerm(ctx, &service.SubscriptionTermRecord{
		SubscriptionID: proSub.ID, PlanID: &proPlanID,
		PricePaid: 99, Currency: "CNY", Days: 30,
		TermStart: time.Now().Add(-15 * 24 * time.Hour), TermEnd: time.Now().Add(15 * 24 * time.Hour), Source: "renewal",
	}))
	_, changeID, err := s.svc.CreateUpgradeQuote(ctx, s.user.ID, proSub.ID, maxPlan.ID, "p10-supersede")
	require.NoError(t, err)
	require.NoError(t, s.changes.MarkPaid(ctx, changeID))
	require.NoError(t, s.svc.FulfillUpgrade(ctx, changeID))
	require.NoError(t, integrationDB.QueryRowContext(ctx,
		"SELECT next_plan_id FROM user_subscriptions WHERE id = $1", proSub.ID).Scan(&nextPlan))
	require.False(t, nextPlan.Valid, "upgrade must cancel the scheduled downgrade")
	pending, err := s.changes.ActiveScheduledChange(ctx, proSub.ID)
	require.NoError(t, err)
	require.Nil(t, pending)

	// ③ 重新调度 + 用户取消
	_, err = s.svc.ScheduleDowngrade(ctx, s.user.ID, proSub.ID, s.basicPlan.ID, "p10-sched-2")
	require.NoError(t, err)
	require.NoError(t, s.svc.CancelScheduledDowngrade(ctx, s.user.ID, proSub.ID))
	require.NoError(t, integrationDB.QueryRowContext(ctx,
		"SELECT next_plan_id FROM user_subscriptions WHERE id = $1", proSub.ID).Scan(&nextPlan))
	require.False(t, nextPlan.Valid)

	// ④ 非降级目标（同档/升级目标）→ 拒绝
	_, err = s.svc.ScheduleDowngrade(ctx, s.user.ID, proSub.ID, maxPlan.ID, "p10-sched-3")
	require.Error(t, err)
}

// ---- Renewal 识别 scheduled plan ----
// （Phase 11B 移除：手动续费按目标档执行的旧语义已由预付费固定周期制取代——
// next_plan_id 仅为续费默认目标，付费履约闭包见
// phase11b_paid_renewal_integration_test.go 的 Case C2/E/F/G。）
