package service

import (
	"context"
	"fmt"
	"math"
	"strings"

	dbent "github.com/Wei-Shaw/sub2api/ent"
	"github.com/shopspring/decimal"
)

// WalletUSDToCNYRate is the fixed accounting conversion used at the wallet
// boundary. Model metering and provider prices remain USD; wallet movements
// are stored in CNY.
const WalletUSDToCNYRate = 6.7

func walletUSDToCNY(amountUSD float64) float64 {
	if amountUSD == 0 || math.IsNaN(amountUSD) || math.IsInf(amountUSD, 0) {
		return amountUSD
	}
	return decimal.NewFromFloat(amountUSD).
		Mul(decimal.NewFromFloat(WalletUSDToCNYRate)).
		Round(8).
		InexactFloat64()
}

// WalletUSDToCNY converts USD-denominated model costs at wallet boundaries.
// Model usage records themselves remain USD.
func WalletUSDToCNY(amountUSD float64) float64 { return walletUSDToCNY(amountUSD) }

func walletAmountToCNY(amount float64, currency string) (float64, error) {
	switch strings.ToUpper(strings.TrimSpace(currency)) {
	case "CNY", "RMB", "CNH":
		return decimal.NewFromFloat(amount).Round(8).InexactFloat64(), nil
	case "", "USD":
		return walletUSDToCNY(amount), nil
	default:
		return 0, fmt.Errorf("wallet amounts only support CNY and USD, got %q", currency)
	}
}

// paymentAmountToWalletCNY converts the amount entered in the selected
// payment method's currency into the canonical wallet currency.
func paymentAmountToWalletCNY(amount float64, currency string, multiplier float64) (float64, error) {
	if amount <= 0 || math.IsNaN(amount) || math.IsInf(amount, 0) {
		return 0, fmt.Errorf("payment amount must be positive and finite")
	}
	var cny decimal.Decimal
	switch strings.ToUpper(strings.TrimSpace(currency)) {
	case "CNY", "RMB", "CNH", "":
		cny = decimal.NewFromFloat(amount)
	case "USD":
		cny = decimal.NewFromFloat(amount).Mul(decimal.NewFromFloat(WalletUSDToCNYRate))
	default:
		return 0, fmt.Errorf("wallet top-ups only support CNY and USD settlement, got %q", currency)
	}
	if normalizeBalanceRechargeMultiplier(multiplier) != 1 {
		return 0, fmt.Errorf("wallet top-up multiplier must be 1")
	}
	return cny.Round(2).InexactFloat64(), nil
}

// walletCNYToPaymentAmount converts a canonical wallet or plan amount to the
// currency accepted by a payment provider. Fee calculation happens afterward.
func walletCNYToPaymentAmount(amountCNY float64, currency string) (float64, error) {
	if amountCNY < 0 || math.IsNaN(amountCNY) || math.IsInf(amountCNY, 0) {
		return 0, fmt.Errorf("wallet amount must be finite and nonnegative")
	}
	switch strings.ToUpper(strings.TrimSpace(currency)) {
	case "CNY", "RMB", "CNH", "":
		return decimal.NewFromFloat(amountCNY).Round(2).InexactFloat64(), nil
	case "USD":
		return decimal.NewFromFloat(amountCNY).
			Div(decimal.NewFromFloat(WalletUSDToCNYRate)).
			Round(2).
			InexactFloat64(), nil
	default:
		return 0, fmt.Errorf("wallet purchases only support CNY and USD settlement, got %q", currency)
	}
}

// PaymentOrderAmountCurrency is the currency unit of payment_orders.amount.
// It is deliberately distinct from PaymentOrderCurrency, which describes the
// provider settlement amount in payment_orders.pay_amount.
func PaymentOrderAmountCurrency(order *dbent.PaymentOrder) string {
	if order != nil && order.ProviderSnapshot != nil {
		if currency, ok := order.ProviderSnapshot["amount_currency"].(string); ok {
			currency = strings.ToUpper(strings.TrimSpace(currency))
			if currency == "CNY" || currency == "USD" {
				return currency
			}
		}
	}
	// Rows predating the currency contract are USD-denominated.
	return "USD"
}

// paymentOrderAmountToWalletCNY converts an order amount (or a partial refund
// expressed in the same unit as order.amount) to the current wallet unit.
func paymentOrderAmountToWalletCNY(order *dbent.PaymentOrder, amount float64) float64 {
	if PaymentOrderAmountCurrency(order) == "USD" {
		return walletUSDToCNY(amount)
	}
	return amount
}

func annotateWalletHistoryCurrencies(_ context.Context, _ *dbent.Client, rows []RedeemCode) {
	for i := range rows {
		switch rows[i].Type {
		case RedeemTypeBalance, "admin_balance", RedeemTypeAffiliateBalance, RedeemTypeRewardGrant:
		default:
			continue
		}
		rows[i].Currency = "CNY"
	}
}
