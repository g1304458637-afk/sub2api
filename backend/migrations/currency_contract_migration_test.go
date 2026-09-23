package migrations

import (
	"strings"
	"testing"
)

func TestCNYWalletMigrationSnapshotsPlanCurrencyBeforeNormalization(t *testing.T) {
	source, err := FS.ReadFile("245_cny_wallet_currency_contract.sql")
	if err != nil {
		t.Fatal(err)
	}
	sql := string(source)

	snapshotAt := strings.Index(sql, "CREATE TEMPORARY TABLE cny_wallet_plan_rates")
	termSnapshotAt := strings.Index(sql, "CREATE TEMPORARY TABLE cny_wallet_term_rates")
	planUpdateAt := strings.Index(sql, "UPDATE subscription_plans p")
	termUpdateAt := strings.Index(sql, "UPDATE subscription_terms t")
	if snapshotAt < 0 || planUpdateAt < 0 || snapshotAt > planUpdateAt {
		t.Fatal("the original plan currency factor must be captured before plans are normalized")
	}
	if termSnapshotAt < 0 || termUpdateAt < 0 || termSnapshotAt > termUpdateAt {
		t.Fatal("the original paid-term currency factor must be captured before terms are normalized")
	}
	for _, statement := range []string{
		"UPDATE subscription_terms t",
		"UPDATE subscription_plan_changes c",
		"cny_wallet_plan_change_rates rates",
		"cny_wallet_term_rates rates",
		"FROM cny_wallet_plan_rates rates",
		"'wallet_currency_contract', 'CNY_V1'",
	} {
		if !strings.Contains(sql, statement) {
			t.Errorf("migration is missing expected CNY conversion step %q", statement)
		}
	}
	if strings.Contains(strings.ToUpper(sql), "UPDATE USAGE_LOGS") || strings.Contains(strings.ToLower(sql), "set pay_amount") {
		t.Fatal("model metering rows and provider settlement amounts must retain their source currency")
	}
	markerAt := strings.Index(sql, "VALUES ('wallet_currency_contract', 'CNY_V1')")
	ordersAt := strings.Index(sql, "UPDATE payment_orders o")
	if markerAt < 0 || ordersAt < 0 || markerAt < ordersAt {
		t.Fatal("the CNY contract marker must be written only after all data conversions")
	}
}
