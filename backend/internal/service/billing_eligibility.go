package service

import "errors"

// BillingTarget identifies the balance actually charged for an admitted request.
type BillingTarget string

const (
	BillingTargetWallet       BillingTarget = "wallet"
	BillingTargetSubscription BillingTarget = "subscription"
)

// BillingEligibility is valid only when CheckBillingEligibility returns no error.
// Admission is checked before forwarding; the same target must survive asynchronous
// usage recording. It never overrides API key, model or account permissions.
type BillingEligibility struct {
	Target BillingTarget
}

// SubscriptionForBilling translates the decision to the existing settlement
// contract: nil selects BalanceCost, non-nil selects SubscriptionCost.
func (e BillingEligibility) SubscriptionForBilling(subscription *UserSubscription) *UserSubscription {
	if e.Target == BillingTargetSubscription {
		return subscription
	}
	return nil
}

// IsSubscriptionLimitError distinguishes exhausted entitlements from invalid,
// expired or suspended subscriptions, which must never enable PAYG fallback.
func IsSubscriptionLimitError(err error) bool {
	return errors.Is(err, ErrShortLimitExceeded) || errors.Is(err, ErrDailyLimitExceeded) || errors.Is(err, ErrWeeklyLimitExceeded) || errors.Is(err, ErrMonthlyLimitExceeded)
}
