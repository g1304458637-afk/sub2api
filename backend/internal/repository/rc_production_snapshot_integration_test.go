//go:build integration

package repository

import (
	"context"
	"os"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestRCProductionSnapshotUpgrade(t *testing.T) {
	if os.Getenv("SUB2API_TEST_SCHEMA_SNAPSHOT") == "" {
		t.Skip("requires schema-only production snapshot and synthetic seed")
	}
	ctx := context.Background()
	var count int
	require.NoError(t, integrationDB.QueryRowContext(ctx, "SELECT count(*) FROM schema_migrations").Scan(&count))
	require.Equal(t, 293, count)
	var status, notes string
	require.NoError(t, integrationDB.QueryRowContext(ctx, "SELECT status, notes FROM user_subscriptions WHERE id=-9001").Scan(&status, &notes))
	require.Equal(t, "expired", status)
	require.Contains(t, notes, "historical fixture")
	require.Contains(t, notes, "PLAN_UNIFY_242")
	require.NoError(t, integrationDB.QueryRowContext(ctx, "SELECT status FROM user_subscriptions WHERE id=-9002").Scan(&status))
	require.Equal(t, "active", status)
	var balance, usage float64
	require.NoError(t, integrationDB.QueryRowContext(ctx, "SELECT balance FROM users WHERE id=-9001").Scan(&balance))
	require.Equal(t, 42.5, balance)
	require.NoError(t, integrationDB.QueryRowContext(ctx, "SELECT weekly_usage_usd FROM user_subscriptions WHERE id=-9002").Scan(&usage))
	require.Equal(t, 50.0, usage)
	require.NoError(t, integrationDB.QueryRowContext(ctx, "SELECT count(*) FROM subscription_reset_cards WHERE user_id=-9001 AND status='available'").Scan(&count))
	require.Equal(t, 1, count)
	_, err := integrationDB.ExecContext(ctx, "UPDATE user_subscriptions SET status='active' WHERE id=-9001")
	require.Error(t, err, "single active subscription constraint must reject duplicate")
	for _, table := range []string{"subscription_terms", "subscription_plan_changes", "subscription_reset_operations", "research_applications", "reward_grants"} {
		var exists bool
		require.NoError(t, integrationDB.QueryRowContext(ctx, "SELECT to_regclass($1) IS NOT NULL", "public."+table).Scan(&exists))
		require.True(t, exists, table)
	}
	require.NoError(t, ApplyMigrations(ctx, integrationDB), "second startup must be idempotent")
	_, err = integrationDB.ExecContext(ctx, "UPDATE subscription_plans SET tier_rank=CASE name WHEN 'Basic' THEN 10 WHEN 'Pro' THEN 20 WHEN 'Max' THEN 30 END WHERE id IN (-9001,-9002,-9003)")
	require.NoError(t, err)
	require.NoError(t, integrationDB.QueryRowContext(ctx, "SELECT count(*) FROM subscription_plans WHERE id IN (-9001,-9002,-9003) AND currency='USD' AND validity_days=30 AND for_sale AND ((name='Basic' AND price=9 AND tier_rank=10) OR (name='Pro' AND price=29 AND tier_rank=20) OR (name='Max' AND price=79 AND tier_rank=30))").Scan(&count))
	require.Equal(t, 3, count)
}
