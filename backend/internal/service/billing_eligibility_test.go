//go:build unit

package service

import (
	"context"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/stretchr/testify/require"
)

type eligibilityMatrixCache struct {
	billingCacheWorkerStub
	usage, balance float64
	status         string
	expires        time.Time
}

func (c *eligibilityMatrixCache) GetUserBalance(context.Context, int64) (float64, error) {
	return c.balance, nil
}
func (c *eligibilityMatrixCache) GetSubscriptionCache(context.Context, int64, int64) (*SubscriptionCacheData, error) {
	return &SubscriptionCacheData{Status: c.status, ExpiresAt: c.expires, WeeklyUsage: c.usage}, nil
}

func TestBillingEligibilityAndSettlementMatrix(t *testing.T) {
	cases := []struct {
		name             string
		usage, balance   float64
		hasSub, fallback bool
		target           BillingTarget
		wantErr          error
	}{
		{"subscription_available", 0, 0, true, false, BillingTargetSubscription, nil},
		{"last_request_admitted_at_99", 99, 0, true, false, BillingTargetSubscription, nil},
		{"next_request_at_100_uses_wallet", 100, 1, true, true, BillingTargetWallet, nil},
		{"over_limit_uses_wallet", 101, 1, true, true, BillingTargetWallet, nil},
		{"over_limit_wallet_empty", 101, 0, true, true, "", ErrInsufficientBalance},
		{"fallback_opt_out", 100, 1, true, false, "", ErrWeeklyLimitExceeded},
		{"standard_without_subscription", 0, 1, false, false, BillingTargetWallet, nil},
		{"standard_empty_wallet", 0, 0, false, false, "", ErrInsufficientBalance},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			limit := 100.0
			group := &Group{ID: 2, SubscriptionType: SubscriptionTypeStandard, WeeklyLimitUSD: &limit}
			var sub *UserSubscription
			if tc.hasSub {
				group.SubscriptionType = SubscriptionTypeSubscription
				sub = &UserSubscription{ID: 3, AutoPaygFallback: tc.fallback}
			}
			cache := &eligibilityMatrixCache{usage: tc.usage, balance: tc.balance, status: SubscriptionStatusActive, expires: time.Now().Add(time.Hour)}
			svc := &BillingCacheService{cache: cache, cfg: &config.Config{}}
			user := &User{ID: 1}
			key := &APIKey{ID: 4, UserID: 1, User: user, Group: group}
			decision, err := svc.CheckBillingEligibility(context.Background(), user, key, group, sub, "")
			if tc.wantErr != nil {
				require.ErrorIs(t, err, tc.wantErr)
				require.Empty(t, decision.Target)
				return
			}
			require.NoError(t, err)
			require.Equal(t, tc.target, decision.Target)
			billSub := decision.SubscriptionForBilling(sub)
			cmd := buildUsageBillingCommand("matrix-"+tc.name, &UsageLog{Model: "test", BillingType: BillingTypeBalance}, &postUsageBillingParams{
				User: user, APIKey: key, Account: &Account{ID: 5}, Subscription: billSub, IsSubscriptionBill: billSub != nil, Cost: &CostBreakdown{TotalCost: 1, ActualCost: 1},
			})
			if tc.target == BillingTargetSubscription {
				require.Equal(t, 1.0, cmd.SubscriptionCost)
				require.Zero(t, cmd.BalanceCost)
				require.Equal(t, sub.ID, *cmd.SubscriptionID)
			} else {
				require.Equal(t, 1.0, cmd.BalanceCost)
				require.Zero(t, cmd.SubscriptionCost)
				require.Nil(t, cmd.SubscriptionID)
			}
		})
	}
}

func TestBillingEligibilityNeverFallsBackForInvalidEntitlement(t *testing.T) {
	for _, status := range []string{SubscriptionStatusExpired, SubscriptionStatusSuspended} {
		t.Run(status, func(t *testing.T) {
			cache := &eligibilityMatrixCache{usage: 101, balance: 100, status: status, expires: time.Now().Add(time.Hour)}
			svc := &BillingCacheService{cache: cache, cfg: &config.Config{}}
			limit := 100.0
			group := &Group{ID: 2, SubscriptionType: SubscriptionTypeSubscription, WeeklyLimitUSD: &limit}
			_, err := svc.CheckBillingEligibility(context.Background(), &User{ID: 1}, nil, group, &UserSubscription{AutoPaygFallback: true}, "")
			require.ErrorIs(t, err, ErrSubscriptionInvalid)
		})
	}
}
