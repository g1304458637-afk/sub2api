//go:build integration

package repository

import (
	"context"
	"fmt"
	"github.com/Wei-Shaw/sub2api/ent/subscriptionplan"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/stretchr/testify/require"
	"testing"
	"time"
)

func TestConsolidationContinuousUpgradePaidOrders(t *testing.T) {
	ctx := context.Background()
	client := testEntClient(t)
	s := phase10Setup(t, client, 100, 200, 15, 30)
	now := s.basicSub.ExpiresAt.Add(-15 * 24 * time.Hour)
	s.svc.SetNow(func() time.Time { return now })
	max, err := client.SubscriptionPlan.Query().Where(subscriptionplan.GroupIDEQ(s.maxG.ID)).Only(ctx)
	require.NoError(t, err)
	_, err = client.SubscriptionPlan.UpdateOneID(max.ID).SetPrice(300).Save(ctx)
	require.NoError(t, err)
	pay := phase11NewPaidRenewalStack(t, client, s, nil, s.terms)
	pay.SetPlanChangeService(s.svc, s.changes, s.terms)
	for i, target := range []int64{s.proPlan.ID, max.ID} {
		key := fmt.Sprintf("continuous-paid-%d", i)
		quote, changeID, err := s.svc.CreateUpgradeQuote(ctx, s.user.ID, s.basicSub.ID, target, key)
		require.NoError(t, err)
		require.Equal(t, 50.0, quote.AmountDue)
		require.Equal(t, float64(50+i*50), quote.UnusedCredit)
		require.Equal(t, float64(100+i*50), quote.ProratedCharge)
		replay, sameID, err := s.svc.CreateUpgradeQuote(ctx, s.user.ID, s.basicSub.ID, target, key)
		require.NoError(t, err)
		require.Equal(t, changeID, sameID)
		require.Equal(t, quote.AmountDue, replay.AmountDue)
		orderID := phase11CreatePaidOrder(t, client, s.user.ID, s.user.Email, target, s.proG.ID, 30, quote.AmountDue)
		_, err = client.PaymentOrder.UpdateOneID(orderID).SetOrderType("plan_change").SetPlanChangeID(changeID).Save(ctx)
		require.NoError(t, err)
		require.NoError(t, s.changes.MarkPendingPayment(ctx, changeID, orderID))
		require.ErrorIs(t, s.svc.FulfillUpgrade(ctx, changeID), service.ErrPlanQuoteStatusInvalid, "unpaid quote cannot grant entitlement")
		require.NoError(t, pay.ExecutePlanChangeFulfillment(ctx, orderID))
		require.NoError(t, pay.ExecutePlanChangeFulfillment(ctx, orderID))
		var amount float64
		var count int
		var start, end time.Time
		require.NoError(t, integrationDB.QueryRow("SELECT COUNT(*), SUM(price_paid), MIN(term_start), MAX(term_end) FROM subscription_terms WHERE order_id = $1", orderID).Scan(&count, &amount, &start, &end))
		require.Equal(t, 1, count)
		require.Equal(t, quote.AmountDue, amount)
		require.True(t, now.Equal(start))
		require.True(t, s.basicSub.ExpiresAt.Equal(end))
	}
}

func TestConsolidationEarlyPaidRenewalWindow(t *testing.T) {
	ctx := context.Background()
	client := testEntClient(t)
	s := phase10Setup(t, client, 100, 200, 15, 30)
	subSvc := service.NewSubscriptionService(NewGroupRepository(client, integrationDB), NewUserSubscriptionRepository(client), nil, client, nil)
	pay := phase11NewPaidRenewalStack(t, client, s, subSvc, s.terms)
	orderID := phase11CreatePaidOrder(t, client, s.user.ID, s.user.Email, s.basicPlan.ID, s.basicG.ID, 30, 100)
	require.NoError(t, pay.ExecuteSubscriptionFulfillment(ctx, orderID))
	require.NoError(t, pay.ExecuteSubscriptionFulfillment(ctx, orderID))
	var start, end time.Time
	var source string
	require.NoError(t, integrationDB.QueryRow("SELECT term_start, term_end, source FROM subscription_terms WHERE order_id = $1", orderID).Scan(&start, &end, &source))
	require.True(t, start.Equal(s.basicSub.ExpiresAt))
	require.True(t, end.Equal(start.AddDate(0, 0, 30)))
	require.Equal(t, "renewal", source)
}
