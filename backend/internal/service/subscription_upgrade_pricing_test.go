package service

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

type upgradePricingRepo struct {
	UserSubscriptionRepository
	sub *UserSubscription
}

func (r *upgradePricingRepo) GetByID(context.Context, int64) (*UserSubscription, error) {
	return r.sub, nil
}
func (r *upgradePricingRepo) GetActiveByUserIDAndGroupID(context.Context, int64, int64) (*UserSubscription, error) {
	return nil, ErrSubscriptionNotFound
}

type upgradePricingPlans map[int64]*PlanSnapshot

func (p upgradePricingPlans) GetPlan(_ context.Context, id int64) (*PlanSnapshot, error) {
	return p[id], nil
}

type upgradePricingTerms []SubscriptionTermRecord

func (t upgradePricingTerms) UnconsumedTerms(context.Context, int64, time.Time) ([]SubscriptionTermRecord, error) {
	return t, nil
}
func (t upgradePricingTerms) RecordTerm(context.Context, *SubscriptionTermRecord) error { return nil }

type upgradePricingStore struct {
	PlanChangeStore
	record *PlanChangeRecord
}

func (s *upgradePricingStore) CreateQuote(_ context.Context, r *PlanChangeRecord) (int64, error) {
	s.record = r
	return 42, nil
}

func TestUpgradePricingImmutableMatrix(t *testing.T) {
	start := time.Date(2026, 9, 1, 0, 0, 0, 0, time.UTC)
	type term struct {
		start, end int
		price      float64
		source     string
	}
	cases := []struct {
		name                       string
		now, end, fromDays, toDays int
		fromPrice, toPrice         float64
		terms                      []term
		credit, gross, due         float64
		reject                     bool
	}{
		{"Basic_Pro_immediate", 0, 30, 30, 30, 100, 200, []term{{0, 30, 100, "purchase"}}, 100, 200, 100, false},
		{"Basic_Max", 15, 30, 30, 30, 100, 300, []term{{0, 30, 100, "purchase"}}, 50, 150, 100, false},
		{"Pro_Max", 15, 30, 30, 30, 200, 300, []term{{0, 30, 200, "purchase"}}, 100, 150, 50, false},
		{"Basic_Pro_Max_immediate", 15, 30, 30, 30, 200, 300, []term{{0, 30, 100, "purchase"}, {15, 30, 50, "upgrade"}}, 100, 150, 50, false},
		{"Basic_Pro_then_Max_next_day", 16, 30, 30, 30, 200, 300, []term{{0, 30, 100, "purchase"}, {15, 30, 50, "upgrade"}}, 93.33, 140, 46.67, false},
		{"mid_cycle", 15, 30, 30, 30, 100, 200, []term{{0, 30, 100, "purchase"}}, 50, 100, 50, false},
		{"last_day", 29, 30, 30, 30, 100, 200, []term{{0, 30, 100, "purchase"}}, 3.33, 6.67, 3.34, false},
		{"sixty_day_plans", 30, 60, 60, 60, 100, 200, []term{{0, 60, 100, "purchase"}}, 50, 100, 50, false},
		{"mixed_cadence_rejected", 15, 30, 30, 60, 100, 200, []term{{0, 30, 100, "purchase"}}, 0, 0, 0, true},
		{"renewal_then_upgrade", 15, 60, 30, 30, 100, 200, []term{{0, 30, 100, "purchase"}, {30, 60, 100, "renewal"}}, 150, 300, 150, false},
		{"historical_segments", 45, 90, 30, 30, 100, 200, []term{{0, 30, 100, "purchase"}, {30, 60, 90, "renewal"}, {60, 90, 80, "renewal"}}, 125, 300, 175, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			now := start.AddDate(0, 0, tc.now)
			expiry := start.AddDate(0, 0, tc.end)
			id := int64(1)
			sub := &UserSubscription{ID: 9, UserID: 8, GroupID: 1, PlanID: &id, Status: SubscriptionStatusActive, ExpiresAt: expiry}
			terms := upgradePricingTerms{}
			for i, x := range tc.terms {
				orderID := int64(i + 10)
				terms = append(terms, SubscriptionTermRecord{SubscriptionID: 9, OrderID: &orderID, Currency: "CNY", TermStart: start.AddDate(0, 0, x.start), TermEnd: start.AddDate(0, 0, x.end), PricePaid: x.price, Source: x.source})
			}
			plans := upgradePricingPlans{1: &PlanSnapshot{ID: 1, GroupID: 1, Price: tc.fromPrice, Currency: "CNY", ValidityDays: tc.fromDays, TierRank: 1}, 2: &PlanSnapshot{ID: 2, GroupID: 2, Price: tc.toPrice, Currency: "CNY", ValidityDays: tc.toDays, TierRank: 2}}
			store := &upgradePricingStore{}
			svc := NewPlanChangeService(store, terms, plans, &upgradePricingRepo{sub: sub}, nil, nil, nil, nil)
			svc.SetNow(func() time.Time { return now })
			q, err := svc.PreviewUpgrade(context.Background(), 8, 9, 2)
			if tc.reject {
				require.ErrorIs(t, err, ErrPlanTermMismatch)
				return
			}
			require.NoError(t, err)
			require.Equal(t, tc.credit, q.UnusedCredit)
			require.Equal(t, tc.gross, q.ProratedCharge)
			require.Equal(t, tc.due, q.AmountDue)
			require.InDelta(t, q.ProratedCharge-q.UnusedCredit, q.AmountDue, 0.00001)
			repeated, err := svc.PreviewUpgrade(context.Background(), 8, 9, 2)
			require.NoError(t, err)
			require.Equal(t, q, repeated)
			frozen, changeID, err := svc.CreateUpgradeQuote(context.Background(), 8, 9, 2, tc.name)
			require.NoError(t, err)
			require.Equal(t, int64(42), changeID)
			require.Equal(t, q, frozen)
			require.Equal(t, now, *store.record.TermStart)
			require.Equal(t, expiry, *store.record.TermEnd)
			require.Equal(t, tc.due, store.record.AmountDue)
			require.Equal(t, int64(9), store.record.SubscriptionID)
			require.Nil(t, store.record.OrderID, "quote has not yet been atomically bound to a payment order")
		})
	}
}
