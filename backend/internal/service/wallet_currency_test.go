package service

import (
	"math"
	"testing"

	dbent "github.com/Wei-Shaw/sub2api/ent"
)

func TestWalletCurrencyConversionsUseFixedCNYContract(t *testing.T) {
	if got := WalletUSDToCNY(2.5); math.Abs(got-16.75) > 1e-10 {
		t.Fatalf("WalletUSDToCNY(2.5) = %v, want 16.75", got)
	}
	if got, err := paymentAmountToWalletCNY(16.75, "CNY", 1); err != nil || math.Abs(got-16.75) > 1e-10 {
		t.Fatalf("CNY payment conversion = %v, %v; want 16.75, nil", got, err)
	}
	if got, err := paymentAmountToWalletCNY(2.5, "USD", 1); err != nil || math.Abs(got-16.75) > 1e-10 {
		t.Fatalf("USD payment conversion = %v, %v; want 16.75, nil", got, err)
	}
	if got, err := walletCNYToPaymentAmount(16.75, "USD"); err != nil || math.Abs(got-2.5) > 1e-10 {
		t.Fatalf("provider USD conversion = %v, %v; want 2.5, nil", got, err)
	}
	if _, err := paymentAmountToWalletCNY(2.5, "EUR", 1); err == nil {
		t.Fatal("unsupported wallet payment currency should fail")
	}
}

func TestWalletCurrencyContractFixesDisplayAndLegacyRates(t *testing.T) {
	for _, rate := range []float64{0, 6.7, 7.15, math.NaN(), math.Inf(1)} {
		if got := normalizeUSDToCNYDisplayRate(rate); got != WalletUSDToCNYRate {
			t.Errorf("normalizeUSDToCNYDisplayRate(%v) = %v, want %v", rate, got, WalletUSDToCNYRate)
		}
	}
	if isValidUSDToCNYDisplayRate(0) || isValidUSDToCNYDisplayRate(7.15) || !isValidUSDToCNYDisplayRate(6.7) {
		t.Fatal("display rate validation must allow only the fixed 6.7 contract")
	}
	if normalizeBalanceRechargeMultiplier(6.7) != 1 || normalizeSubscriptionUSDToCNYRate(7.15) != 0 {
		t.Fatal("legacy wallet multipliers and subscription conversion must be retired")
	}
}

func TestPaymentOrderAmountCurrencyUsesCanonicalSnapshot(t *testing.T) {
	canonical := &dbent.PaymentOrder{
		ProviderSnapshot: map[string]interface{}{
			"amount_currency": "cny",
		},
	}
	if got := PaymentOrderAmountCurrency(canonical); got != "CNY" {
		t.Fatalf("canonical order amount currency = %q, want CNY", got)
	}
	legacy := &dbent.PaymentOrder{}
	if got := PaymentOrderAmountCurrency(legacy); got != "USD" {
		t.Fatalf("legacy order amount currency = %q, want USD", got)
	}
}
