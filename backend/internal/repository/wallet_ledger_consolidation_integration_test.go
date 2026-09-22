//go:build integration

package repository

import (
	"context"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/stretchr/testify/require"
	"strconv"
	"testing"
	"time"
)

func TestConsolidationWalletLedgerMoneyOnlyAndPagination(t *testing.T) {
	ctx := context.Background()
	client := testEntClient(t)
	limit := 10.0
	user, _, _, key, account := phase0MustSubscriptionStack(t, client, 100, &limit)
	now := time.Now().UTC().Truncate(time.Microsecond)
	for _, row := range []struct {
		code, typ string
		amount    float64
	}{
		{"ordinary", "balance", 11}, {"adjust", "admin_balance", -2},
		{"nonmoney", "concurrency", 999}, {"entitlement", "subscription", 500}, {"paid", "balance", 20},
	} {
		_, err := client.RedeemCode.Create().SetCode(row.code).SetType(row.typ).SetValue(row.amount).SetStatus("used").SetUsedBy(user.ID).SetUsedAt(now).Save(ctx)
		require.NoError(t, err)
	}
	_, err := integrationDB.Exec("INSERT INTO reward_grants (user_id,idempotency_key,source_type,campaign,amount) VALUES ($1,'ledger-reward','campaign_grant','test',3)", user.ID)
	require.NoError(t, err)
	order, err := client.PaymentOrder.Create().SetUserID(user.ID).SetUserEmail(user.Email).SetUserName("ledger").SetAmount(20).SetPayAmount(20).SetOutTradeNo("ledger-paid").SetRechargeCode("paid").SetPaymentType("alipay").SetPaymentTradeNo("").SetOrderType("balance").SetStatus(service.OrderStatusRefunded).SetExpiresAt(now.Add(time.Hour)).SetClientIP("127.0.0.1").SetSrcHost("localhost").Save(ctx)
	require.NoError(t, err)
	_, err = client.PaymentAuditLog.Create().SetOrderID(strconv.FormatInt(order.ID, 10)).SetAction("REFUND_SUCCESS").SetDetail(`{"refundAmount":7,"balanceDeducted":2.5}`).SetCreatedAt(now).Save(ctx)
	require.NoError(t, err)
	for _, row := range []struct {
		billing int8
		cost    float64
		request string
	}{{0, 4, "ledger-wallet"}, {1, 99, "ledger-subscription"}} {
		_, err := client.UsageLog.Create().SetUserID(user.ID).SetAPIKeyID(key.ID).SetAccountID(account.ID).SetModel("test").SetRequestID(row.request).SetBillingType(row.billing).SetActualCost(row.cost).SetCreatedAt(now).Save(ctx)
		require.NoError(t, err)
	}
	svc := service.NewWalletLedgerService(client, NewRewardGrantRepository(client))
	all, total, err := svc.ListUserLedger(ctx, user.ID, 100, 0)
	require.NoError(t, err)
	require.Equal(t, int64(6), total)
	amounts := map[string]float64{}
	ids := map[string]bool{}
	for _, row := range all {
		amounts[row.Type] += row.Amount
		require.False(t, ids[row.ID])
		ids[row.ID] = true
	}
	require.Equal(t, map[string]float64{"wallet_recharge": 20, "redeem_balance": 11, "manual_adjustment": -2, "reward": 3, "payg_usage": -4, "refund": -2.5}, amounts)
	for offset := 0; offset < 6; offset += 2 {
		page, count, err := svc.ListUserLedger(ctx, user.ID, 2, offset)
		require.NoError(t, err)
		require.Equal(t, total, count)
		require.Equal(t, all[offset:offset+2], page)
	}
	empty, count, err := svc.ListUserLedger(ctx, user.ID, 2, 100)
	require.NoError(t, err)
	require.Empty(t, empty)
	require.Equal(t, total, count)
}
