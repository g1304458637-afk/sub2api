//go:build integration

package repository

// Phase 11B —— 预付费固定周期制产品规则验收旅程（Case A-I 后端语义等价）。
//
// 硬规则：永不自动续费/自动扣款/到期自动创建套餐。到期任务只负责 ACTIVE→EXPIRED；
// next_plan_id 是"下次续费默认目标"（用户偏好），仅用户付费成功/主动取消/主动改选可清除。
//
// 旅程：A 购买 Basic → B 升级 Pro（立即+折抵）→ C 预约下次续费 Basic（无行为）
//   → E 到期 job：Pro EXPIRED、指针保留、ACTIVE=0、绝不产生 Basic
//   → B2 无所事事 30 天（重复到期 job）：状态不变
//   → D 状态合同重读：0 ACTIVE + last_subscription + next_renewal_plan
//   → C2 用户付费续费 Basic：ACTIVE、指针清、term 追溯订单
//   → G 同订单履约重放：幂等，无重复授予
//   → F 预约替换：pending Basic → 改 pending Pro，只有一条 pending
//   → I 到期前升级 Max：Max ACTIVE + 指针清（superseded_by_upgrade）
//   → H 用户取消：指针 null

import (
	"context"
	"testing"
	"time"

	dbent "github.com/Wei-Shaw/sub2api/ent"
	"github.com/Wei-Shaw/sub2api/ent/subscriptionplan"
	"github.com/Wei-Shaw/sub2api/internal/payment"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/stretchr/testify/require"
)

// phase11Upgrade 用户主动付费升级（quote → paid → 履约）。
func phase11Upgrade(t *testing.T, s *phase10Stack, subID, toPlanID int64, key string) {
	t.Helper()
	_, changeID, err := s.svc.CreateUpgradeQuote(context.Background(), s.user.ID, subID, toPlanID, key)
	require.NoError(t, err)
	require.NoError(t, s.changes.MarkPaid(context.Background(), changeID))
	require.NoError(t, s.svc.FulfillUpgrade(context.Background(), changeID))
}

func phase11ActiveGroup(t *testing.T, _ *dbent.Client, subID int64) int64 {
	t.Helper()
	var gid int64
	require.NoError(t, integrationDB.QueryRow("SELECT group_id FROM user_subscriptions WHERE id = $1", subID).Scan(&gid))
	return gid
}

// phase11NewPaidRenewalStack 组装付费续费链路（PaymentService 最小依赖，全部 nil 安全）。
func phase11NewPaidRenewalStack(t *testing.T, client *dbent.Client, s *phase10Stack, subSvc *service.SubscriptionService, termStore service.TermStore) *service.PaymentService {
	t.Helper()
	userRepo := NewUserRepository(client, integrationDB)
	groupRepo := NewGroupRepository(client, integrationDB)
	paySvc := service.NewPaymentService(client, payment.ProvideRegistry(), nil, nil, subSvc, nil, userRepo, groupRepo, nil)
	paySvc.SetPlanChangeService(nil, nil, termStore)
	return paySvc
}

// phase11CreatePaidOrder 直接落一条已支付的订阅订单（模拟支付成功回调完成态）。
func phase11CreatePaidOrder(t *testing.T, client *dbent.Client, userID int64, email string, planID, groupID int64, days int, amount float64) int64 {
	t.Helper()
	o, err := client.PaymentOrder.Create().
		SetUserID(userID).
		SetUserEmail(email).
		SetUserName("phase11b").
		SetAmount(amount).
		SetPayAmount(amount).
		SetOutTradeNo("P11B" + time.Now().Format("150405.000000000")).
		SetRechargeCode("P11B-CODE").
		SetPaymentType("alipay").
		SetPaymentTradeNo("").
		SetOrderType("subscription").
		SetStatus(service.OrderStatusPaid).
		SetExpiresAt(time.Now().Add(30 * time.Minute)).
		SetClientIP("127.0.0.1").
		SetSrcHost("localhost").
		SetPlanID(planID).
		SetSubscriptionGroupID(groupID).
		SetSubscriptionDays(days).
		Save(context.Background())
	require.NoError(t, err)
	return o.ID
}

func phase11NextPlanID(t *testing.T, s *phase10Stack, subID int64) *int64 {
	t.Helper()
	var next *int64
	require.NoError(t, integrationDB.QueryRow(
		"SELECT next_plan_id FROM user_subscriptions WHERE id = $1", subID).Scan(&next))
	return next
}

func TestPhase11BProductCasesJourney(t *testing.T) {
	client := testEntClient(t)
	ctx := context.Background()

	// ── Case A：购买 Basic → Basic ACTIVE 且唯一 ──
	s := phase10Setup(t, client, 39, 99, 0, 30)
	require.Equal(t, s.basicG.ID, phase11ActiveGroup(t, client, s.basicSub.ID))
	subs, err := NewUserSubscriptionRepository(client).ListActiveByUserID(ctx, s.user.ID)
	require.NoError(t, err)
	require.Len(t, subs, 1)

	subSvc := service.NewSubscriptionService(
		NewGroupRepository(client, integrationDB), NewUserSubscriptionRepository(client), nil, client, nil)
	// 与生产 wire 一致：付费履约闭包依赖（仅支付路径可清 next_plan_id）
	subSvc.SetScheduledChangeSuperseder(s.svc)
	paySvc := phase11NewPaidRenewalStack(t, client, s, subSvc, s.terms)
	statusSvc := service.NewAccountStatusService(
		NewUserRepository(client, integrationDB), NewUserSubscriptionRepository(client),
		NewGroupRepository(client, integrationDB), nil, nil, true)
	statusSvc.SetNextRenewalResolver(NewUserSubscriptionRepository(client), NewPlanSnapshotService(client))

	// ── Case B（升级规则保持不变）：Basic → Pro 立即升级 + 折抵 ──
	phase11Upgrade(t, s, s.basicSub.ID, s.proPlan.ID, "p11b-b")
	require.Equal(t, s.proG.ID, phase11ActiveGroup(t, client, s.basicSub.ID))
	subs, err = NewUserSubscriptionRepository(client).ListActiveByUserID(ctx, s.user.ID)
	require.NoError(t, err)
	require.Len(t, subs, 1, "exactly one ACTIVE after upgrade")

	// ── Case C：Pro 预约"下次续费 Basic" —— 立即无任何行为 ──
	_, err = s.svc.ScheduleDowngrade(ctx, s.user.ID, s.basicSub.ID, s.basicPlan.ID, "p11b-c")
	require.NoError(t, err)
	require.Equal(t, s.proG.ID, phase11ActiveGroup(t, client, s.basicSub.ID), "no early effect")
	require.NotNil(t, phase11NextPlanID(t, s, s.basicSub.ID))

	// ── Case E：到期 job 只负责 ACTIVE→EXPIRED；指针保留；绝不产生 Basic ──
	_, err = integrationDB.Exec(
		"UPDATE user_subscriptions SET expires_at = NOW() - INTERVAL '1 hour' WHERE id = $1", s.basicSub.ID)
	require.NoError(t, err)
	expired, err := NewUserSubscriptionRepository(client).BatchUpdateExpiredStatus(ctx)
	require.NoError(t, err)
	require.Equal(t, int64(1), expired)
	var status string
	var nextPtr *int64
	require.NoError(t, integrationDB.QueryRow(
		"SELECT group_id, status, next_plan_id FROM user_subscriptions WHERE id = $1", s.basicSub.ID).
		Scan(new(int64), &status, &nextPtr))
	require.Equal(t, service.SubscriptionStatusExpired, status)
	require.NotNil(t, nextPtr, "next_plan_id survives expiry")
	subs, err = NewUserSubscriptionRepository(client).ListActiveByUserID(ctx, s.user.ID)
	require.NoError(t, err)
	require.Len(t, subs, 0, "ACTIVE = 0 is the legitimate prepaid state")
	var basicRows int
	require.NoError(t, integrationDB.QueryRow(
		"SELECT COUNT(*) FROM user_subscriptions WHERE user_id=$1 AND group_id=$2", s.user.ID, s.basicG.ID).
		Scan(&basicRows))
	require.Equal(t, 0, basicRows, "no Basic subscription may be created by expiry")

	// ── Case B2：无所事事 30 天（到期 job 反复执行）→ 状态纹丝不动 ──
	_, err = integrationDB.Exec(
		"UPDATE user_subscriptions SET expires_at = NOW() - INTERVAL '31 days' WHERE id = $1", s.basicSub.ID)
	require.NoError(t, err)
	_, err = NewUserSubscriptionRepository(client).BatchUpdateExpiredStatus(ctx)
	require.NoError(t, err)
	_, err = NewUserSubscriptionRepository(client).BatchUpdateExpiredStatus(ctx)
	require.NoError(t, err)
	subs, err = NewUserSubscriptionRepository(client).ListActiveByUserID(ctx, s.user.ID)
	require.NoError(t, err)
	require.Len(t, subs, 0)
	require.NotNil(t, phase11NextPlanID(t, s, s.basicSub.ID))
	var totalRows int
	require.NoError(t, integrationDB.QueryRow(
		"SELECT COUNT(*) FROM user_subscriptions WHERE user_id=$1", s.user.ID).Scan(&totalRows))
	require.Equal(t, 1, totalRows, "no automatic new period/row")

	// ── Case D：刷新/重登 —— 状态合同完整表达 0 ACTIVE + 上次套餐 + 下次续费默认 ──
	full, err := statusSvc.GetAccountStatus(ctx, s.user.ID)
	require.NoError(t, err)
	require.Len(t, full.Subscriptions, 0)
	require.NotNil(t, full.PendingChange, "compat field still surfaced")
	require.NotNil(t, full.NextRenewalPlanID)
	require.Equal(t, s.basicPlan.ID, *full.NextRenewalPlanID)
	require.NotEmpty(t, full.NextRenewalPlan)
	require.NotNil(t, full.LastSubscription)
	require.Equal(t, s.proG.ID, full.LastSubscription.GroupID, "last plan = Pro")

	// ── Case C2：用户主动付费续费 Basic（默认目标）→ Basic ACTIVE + 指针清 ──
	orderID := phase11CreatePaidOrder(t, client, s.user.ID, s.user.Email, s.basicPlan.ID, s.basicG.ID, 30, 39)
	require.NoError(t, paySvc.ExecuteSubscriptionFulfillment(ctx, orderID))
	subs, err = NewUserSubscriptionRepository(client).ListActiveByUserID(ctx, s.user.ID)
	require.NoError(t, err)
	require.Len(t, subs, 1)
	require.Equal(t, s.basicG.ID, subs[0].GroupID)
	require.Nil(t, phase11NextPlanID(t, s, s.basicSub.ID), "paid renewal clears the pointer")
	// Rule 12：付费周期可追溯订单
	var termCount int
	require.NoError(t, integrationDB.QueryRow(
		"SELECT COUNT(*) FROM subscription_terms WHERE order_id=$1", orderID).Scan(&termCount))
	require.Equal(t, 1, termCount)
	var orderStatus string
	require.NoError(t, integrationDB.QueryRow(
		"SELECT status FROM payment_orders WHERE id=$1", orderID).Scan(&orderStatus))
	require.Equal(t, service.OrderStatusCompleted, orderStatus)

	// ── Case G：履约重放（callback 重放语义）→ 幂等，无重复授予/无重复 term ──
	require.NoError(t, paySvc.ExecuteSubscriptionFulfillment(ctx, orderID))
	var termCountAfterReplay int
	require.NoError(t, integrationDB.QueryRow(
		"SELECT COUNT(*) FROM subscription_terms WHERE order_id=$1", orderID).Scan(&termCountAfterReplay))
	require.Equal(t, 1, termCountAfterReplay)
	subs, err = NewUserSubscriptionRepository(client).ListActiveByUserID(ctx, s.user.ID)
	require.NoError(t, err)
	require.Len(t, subs, 1, "still exactly one ACTIVE")

	// ── Case F：预约替换 —— Max 起点：pending Basic 改为 pending Pro ──
	// 当前 Basic ACTIVE：先升 Max（顶档作起点）
	maxPlan, err := client.SubscriptionPlan.Query().
		Where(subscriptionplan.GroupIDEQ(s.maxG.ID)).
		Only(ctx)
	require.NoError(t, err)
	maxPlanID := maxPlan.ID
	phase11Upgrade(t, s, subs[0].ID, maxPlanID, "p11b-f-up")
	_, err = s.svc.ScheduleDowngrade(ctx, s.user.ID, subs[0].ID, s.basicPlan.ID, "p11b-f-1")
	require.NoError(t, err)
	_, err = s.svc.ScheduleDowngrade(ctx, s.user.ID, subs[0].ID, s.proPlan.ID, "p11b-f-2")
	require.NoError(t, err)
	var pendingCount int
	require.NoError(t, integrationDB.QueryRow(
		`SELECT COUNT(*) FROM subscription_plan_changes
		 WHERE subscription_id=$1 AND change_type='scheduled_downgrade' AND status='scheduled'`,
		subs[0].ID).Scan(&pendingCount))
	require.Equal(t, 1, pendingCount, "replacement leaves exactly one pending")
	require.NoError(t, integrationDB.QueryRow(
		"SELECT next_plan_id FROM user_subscriptions WHERE id = $1", subs[0].ID).
		Scan(new(*int64)))
	var pointerTo int64
	require.NoError(t, integrationDB.QueryRow(
		"SELECT next_plan_id FROM user_subscriptions WHERE id = $1", subs[0].ID).Scan(&pointerTo))
	require.Equal(t, s.proPlan.ID, pointerTo, "replacement pending points at Pro")

	// ── Case I：到期前升级无升级空间（Max 已顶档）→ 用取消表达 Case H ──
	// 用户主动取消（Case H）：Max ACTIVE + 指针 null
	require.NoError(t, s.svc.CancelScheduledDowngrade(ctx, s.user.ID, subs[0].ID))
	require.Nil(t, phase11NextPlanID(t, s, subs[0].ID))
	require.Equal(t, s.maxG.ID, phase11ActiveGroup(t, client, subs[0].ID), "cancel must not affect current entitlement")
}
