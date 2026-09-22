//go:build integration

package repository

// Phase 11B —— 付费续费语义专项（Case D / Case G / RULE 12）。

import (
	"context"
	"testing"

	dbent "github.com/Wei-Shaw/sub2api/ent"
	"github.com/Wei-Shaw/sub2api/ent/paymentorder"
	"github.com/Wei-Shaw/sub2api/internal/payment"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/stretchr/testify/require"
)

// phase11BSetupProWithPointer：Pro ACTIVE + next_plan=Basic + term + 一条 Key。
func phase11BSetupProWithPointer(t *testing.T, client *dbent.Client) (*phase10Stack, *service.UserSubscription) {
	t.Helper()
	ctx := context.Background()
	s := phase10Setup(t, client, 39, 99, 0, 30)
	// 升到 Pro（立即履约）
	_, changeID, err := s.svc.CreateUpgradeQuote(ctx, s.user.ID, s.basicSub.ID, s.proPlan.ID, "p11b-d")
	require.NoError(t, err)
	require.NoError(t, s.changes.MarkPaid(ctx, changeID))
	require.NoError(t, s.svc.FulfillUpgrade(ctx, changeID))
	// 预约"下次续费 Basic"
	_, err = s.svc.ScheduleDowngrade(ctx, s.user.ID, s.basicSub.ID, s.basicPlan.ID, "p11b-d-sched")
	require.NoError(t, err)
	// 到期（到期 job 只翻 EXPIRED，指针保留）
	_, err = integrationDB.ExecContext(ctx,
		"UPDATE user_subscriptions SET expires_at = NOW() - INTERVAL '1 hour' WHERE id = $1", s.basicSub.ID)
	require.NoError(t, err)
	_, err = NewUserSubscriptionRepository(client).BatchUpdateExpiredStatus(ctx)
	require.NoError(t, err)
	subs, err := NewUserSubscriptionRepository(client).ListActiveByUserID(ctx, s.user.ID)
	require.NoError(t, err)
	require.Len(t, subs, 0, "precondition: nothing active")
	require.NotNil(t, phase11NextPlanID(t, s, s.basicSub.ID), "precondition: pointer survives expiry")
	return s, s.basicSub
}

// Case D：付款失败 = 没有成功支付 = 不授予任何权益。
func TestPhase11BCaseDPaymentFailureGrantsNothing(t *testing.T) {
	client := testEntClient(t)
	ctx := context.Background()
	s, _ := phase11BSetupProWithPointer(t, client)

	subSvc := service.NewSubscriptionService(
		NewGroupRepository(client, integrationDB), NewUserSubscriptionRepository(client), nil, client, nil)
	subSvc.SetScheduledChangeSuperseder(s.svc)
	paySvc := phase11NewPaidRenewalStack(t, client, s, subSvc, s.terms)

	// 订单停留在 pending（支付失败/未支付）：履约入口必须拒绝
	orderID := phase11CreatePaidOrder(t, client, s.user.ID, s.user.Email, s.basicPlan.ID, s.basicG.ID, 30, 39)
	_, err := client.PaymentOrder.Update().Where(paymentorder.IDEQ(orderID)).
		SetStatus(service.OrderStatusPending).Save(ctx)
	require.NoError(t, err)
	err = paySvc.ExecuteSubscriptionFulfillment(ctx, orderID)
	require.Error(t, err, "unpaid order must not fulfill")

	// 期望状态：ACTIVE = 0、指针保留、无任何新周期
	subs, err := NewUserSubscriptionRepository(client).ListActiveByUserID(ctx, s.user.ID)
	require.NoError(t, err)
	require.Len(t, subs, 0)
	require.NotNil(t, phase11NextPlanID(t, s, s.basicSub.ID), "next_plan_id untouched by failed payment")
}

// Case G：到期 job / 维护重放 / 回调重放 不得清指针、不得创建套餐、不得重复处理。
func TestPhase11BCaseGSystemJobsNeverTouchPointer(t *testing.T) {
	client := testEntClient(t)
	ctx := context.Background()
	s, _ := phase11BSetupProWithPointer(t, client)

	subRepo := NewUserSubscriptionRepository(client)
	// 到期 job 重放
	for i := 0; i < 3; i++ {
		_, err := subRepo.BatchUpdateExpiredStatus(ctx)
		require.NoError(t, err)
	}
	require.NotNil(t, phase11NextPlanID(t, s, s.basicSub.ID), "expiry job must never clear the pointer")

	// 履约重放：指针只清一次（保持清除态），term/审计不重复
	subSvc := service.NewSubscriptionService(
		NewGroupRepository(client, integrationDB), NewUserSubscriptionRepository(client), nil, client, nil)
	subSvc.SetScheduledChangeSuperseder(s.svc)
	paySvc := phase11NewPaidRenewalStack(t, client, s, subSvc, s.terms)
	orderID := phase11CreatePaidOrder(t, client, s.user.ID, s.user.Email, s.basicPlan.ID, s.basicG.ID, 30, 39)
	require.NoError(t, paySvc.ExecuteSubscriptionFulfillment(ctx, orderID))
	require.NoError(t, paySvc.ExecuteSubscriptionFulfillment(ctx, orderID))
	require.NoError(t, paySvc.ExecuteSubscriptionFulfillment(ctx, orderID))

	var pointerRows int
	require.NoError(t, integrationDB.QueryRow(
		"SELECT COUNT(*) FROM user_subscriptions WHERE user_id=$1 AND next_plan_id IS NOT NULL", s.user.ID).
		Scan(&pointerRows))
	require.Equal(t, 0, pointerRows, "pointer cleared exactly once, stays cleared")
	var termCount int
	require.NoError(t, integrationDB.QueryRow(
		"SELECT COUNT(*) FROM subscription_terms WHERE order_id=$1", orderID).Scan(&termCount))
	require.Equal(t, 1, termCount, "exactly one order-linked term across replays")
	subs, err := subRepo.ListActiveByUserID(ctx, s.user.ID)
	require.NoError(t, err)
	require.Len(t, subs, 1, "no duplicate grants across replays")

	// 到期 job 在续费后再跑：不得取消/破坏任何状态
	_, err = subRepo.BatchUpdateExpiredStatus(ctx)
	require.NoError(t, err)
	subs, err = subRepo.ListActiveByUserID(ctx, s.user.ID)
	require.NoError(t, err)
	require.Len(t, subs, 1, "renewed term untouched by expiry job")
}

// RULE 12：term 追溯缺失时履约必须失败（无来源授予 = 拒绝），且不产生任何 ACTIVE。
func TestPhase11BRule12MissingTermFailsFulfillment(t *testing.T) {
	client := testEntClient(t)
	ctx := context.Background()
	s, _ := phase11BSetupProWithPointer(t, client)

	// 不注入 termStore：付费周期无法落 order 链快照 → 履约必须整体失败
	subSvc := service.NewSubscriptionService(
		NewGroupRepository(client, integrationDB), NewUserSubscriptionRepository(client), nil, client, nil)
	subSvc.SetScheduledChangeSuperseder(s.svc)
	paySvc := service.NewPaymentService(client, payment.ProvideRegistry(), nil, nil, subSvc, nil,
		NewUserRepository(client, integrationDB), NewGroupRepository(client, integrationDB), nil)

	orderID := phase11CreatePaidOrder(t, client, s.user.ID, s.user.Email, s.basicPlan.ID, s.basicG.ID, 30, 39)
	err := paySvc.ExecuteSubscriptionFulfillment(ctx, orderID)
	require.Error(t, err, "traceability violation must fail the fulfillment")

	subs, err := NewUserSubscriptionRepository(client).ListActiveByUserID(ctx, s.user.ID)
	require.NoError(t, err)
	require.Len(t, subs, 0, "no entitlement without traceable source")
	var orderStatus string
	require.NoError(t, integrationDB.QueryRow(
		"SELECT status FROM payment_orders WHERE id=$1", orderID).Scan(&orderStatus))
	require.NotEqual(t, service.OrderStatusCompleted, orderStatus)
	require.NotNil(t, phase11NextPlanID(t, s, s.basicSub.ID), "failed fulfillment must not touch the pointer")
}
