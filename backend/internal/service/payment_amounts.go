package service

import (
	"math"

	"github.com/Wei-Shaw/sub2api/internal/payment"
	"github.com/shopspring/decimal"
)

const defaultBalanceRechargeMultiplier = 1.0

func normalizeUSDToCNYDisplayRate(_ float64) float64 { return WalletUSDToCNYRate }

func isValidUSDToCNYDisplayRate(rate float64) bool {
	return !math.IsNaN(rate) && !math.IsInf(rate, 0) && rate == WalletUSDToCNYRate
}

func normalizeBalanceRechargeMultiplier(_ float64) float64 { return defaultBalanceRechargeMultiplier }

// normalizeSubscriptionUSDToCNYRate is retired because subscription prices
// are now stored and charged in CNY.
func normalizeSubscriptionUSDToCNYRate(_ float64) float64 { return 0 }

func calculateCreditedBalance(paymentAmount, multiplier float64) float64 {
	return decimal.NewFromFloat(paymentAmount).
		Mul(decimal.NewFromFloat(normalizeBalanceRechargeMultiplier(multiplier))).
		Round(2).
		InexactFloat64()
}

func calculateGatewayRefundAmount(orderAmount, payAmount, refundAmount float64, currency string) float64 {
	if orderAmount <= 0 || payAmount <= 0 || refundAmount <= 0 {
		return 0
	}
	fractionDigits := int32(payment.CurrencyMaxFractionDigits(currency))
	if math.Abs(refundAmount-orderAmount) <= paymentAmountToleranceForCurrency(currency) {
		return decimal.NewFromFloat(payAmount).Round(fractionDigits).InexactFloat64()
	}
	return decimal.NewFromFloat(payAmount).
		Mul(decimal.NewFromFloat(refundAmount)).
		Div(decimal.NewFromFloat(orderAmount)).
		Round(fractionDigits).
		InexactFloat64()
}
