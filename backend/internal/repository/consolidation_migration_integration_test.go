//go:build integration

package repository

import (
	"context"
	"database/sql"
	"io/fs"
	"net/url"
	"testing"
	"testing/fstest"
	"time"

	"github.com/Wei-Shaw/sub2api/migrations"
	"github.com/stretchr/testify/require"
)

// Apply the released main migrations to a separate empty database, seed the
// legacy multiple-ACTIVE state, then apply the full union with normal checksums.
func TestConsolidationMigrationMainToUnion(t *testing.T) {
	ctx := context.Background()
	const database = "campus_legacy_upgrade_test"
	_, err := integrationDB.ExecContext(ctx, "CREATE DATABASE "+database)
	require.NoError(t, err)
	defer func() { _, _ = integrationDB.ExecContext(ctx, "DROP DATABASE "+database+" WITH (FORCE)") }()
	dsn, err := url.Parse(integrationDSN)
	require.NoError(t, err)
	dsn.Path = "/" + database
	db, err := sql.Open("postgres", dsn.String())
	require.NoError(t, err)
	defer func() { _ = db.Close() }()
	old := fstest.MapFS{}
	names, err := fs.Glob(migrations.FS, "*.sql")
	require.NoError(t, err)
	for _, name := range names {
		if name == "241_subscription_plan_change.sql" || name == "242_user_subscriptions_single_active.sql" || name == "245_cny_wallet_currency_contract.sql" {
			continue
		}
		content, err := migrations.FS.ReadFile(name)
		require.NoError(t, err)
		old[name] = &fstest.MapFile{Data: content}
	}
	require.NoError(t, applyMigrationsFS(ctx, db, old))
	var userID, g1, g2 int64
	require.NoError(t, db.QueryRow("INSERT INTO users(email,password_hash) VALUES ('legacy@test.invalid','hash') RETURNING id").Scan(&userID))
	require.NoError(t, db.QueryRow("INSERT INTO groups(name,subscription_type) VALUES ('legacy-basic','subscription') RETURNING id").Scan(&g1))
	require.NoError(t, db.QueryRow("INSERT INTO groups(name,subscription_type) VALUES ('legacy-pro','subscription') RETURNING id").Scan(&g2))
	anchor := time.Now().UTC().Add(-24 * time.Hour).Truncate(time.Microsecond)
	expiry := anchor.Add(30 * 24 * time.Hour)
	for _, groupID := range []int64{g1, g2} {
		_, err = db.Exec("INSERT INTO user_subscriptions(user_id,group_id,starts_at,expires_at,status,weekly_usage_usd,weekly_window_start) VALUES ($1,$2,$3,$4,'active',8.5,$3)", userID, groupID, anchor, expiry)
		require.NoError(t, err)
	}
	require.NoError(t, ApplyMigrations(ctx, db))
	require.NoError(t, ApplyMigrations(ctx, db), "a second application must validate checksums and make no changes")
	var active, audited, terms, research, music int
	require.NoError(t, db.QueryRow("SELECT COUNT(*) FROM user_subscriptions WHERE user_id=$1 AND status='active'", userID).Scan(&active))
	require.Equal(t, 1, active)
	require.NoError(t, db.QueryRow("SELECT COUNT(*) FROM user_subscriptions WHERE user_id=$1 AND status='expired' AND notes LIKE '%PLAN_UNIFY_242%'", userID).Scan(&audited))
	require.Equal(t, 1, audited)
	var gid int64
	var planID sql.NullInt64
	var usage float64
	var gotAnchor, gotExpiry time.Time
	require.NoError(t, db.QueryRow("SELECT group_id,plan_id,weekly_usage_usd,weekly_window_start,expires_at FROM user_subscriptions WHERE user_id=$1 AND status='active'", userID).Scan(&gid, &planID, &usage, &gotAnchor, &gotExpiry))
	require.Equal(t, g2, gid, "tie selects the newest subscription")
	require.False(t, planID.Valid, "migration must not invent historical plan identity")
	require.Equal(t, 8.5, usage)
	require.True(t, anchor.Equal(gotAnchor))
	require.True(t, expiry.Equal(gotExpiry))
	require.NoError(t, db.QueryRow("SELECT COUNT(*) FROM subscription_terms").Scan(&terms))
	require.Zero(t, terms)
	require.NoError(t, db.QueryRow("SELECT COUNT(*) FROM information_schema.tables WHERE table_name IN ('research_applications','research_attachment_uploads')").Scan(&research))
	require.Equal(t, 2, research)
	require.NoError(t, db.QueryRow("SELECT COUNT(*) FROM information_schema.columns WHERE table_name='groups' AND column_name LIKE '%music%'").Scan(&music))
	require.Positive(t, music)
	_, err = db.Exec("UPDATE user_subscriptions SET status='active' WHERE user_id=$1", userID)
	require.ErrorContains(t, err, "uq_user_subscriptions_single_active")
	var fks int
	require.NoError(t, db.QueryRow("SELECT COUNT(*) FROM pg_constraint WHERE contype='f' AND conrelid='subscription_terms'::regclass").Scan(&fks))
	require.GreaterOrEqual(t, fks, 2)
	var tierDefault, tierNullable string
	require.NoError(t, db.QueryRow("SELECT column_default,is_nullable FROM information_schema.columns WHERE table_name='subscription_plans' AND column_name='tier_rank'").Scan(&tierDefault, &tierNullable))
	require.Equal(t, "0", tierDefault)
	require.Equal(t, "NO", tierNullable)
	var planFKs int
	require.NoError(t, db.QueryRow("SELECT COUNT(*) FROM pg_constraint WHERE contype='f' AND conrelid='user_subscriptions'::regclass AND confrelid='subscription_plans'::regclass").Scan(&planFKs))
	require.Equal(t, 2, planFKs, "plan_id and next_plan_id retain foreign keys")

	t.Logf("released main %d migrations -> union %d migrations; legacy cleanup, immutable history, research/music and FK/index checks passed", len(old), len(names))
}
