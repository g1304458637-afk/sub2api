//go:build integration

package repository

import (
	"context"
	"fmt"
	"os"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/domain"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
)

func TestDualQuotaSettlementResetAndDedup(t *testing.T) {
	ctx := context.Background()
	client := testEntClient(t)
	limit := 50.0
	user, group, sub, key, account := phase0MustSubscriptionStack(t, client, 100, &limit)
	_, err := integrationDB.ExecContext(ctx, `UPDATE groups SET quota_policy='dual_window_v1',short_limit_usd=10 WHERE id=$1`, group.ID)
	require.NoError(t, err)
	repo := NewUsageBillingRepository(client, integrationDB)
	cmd := &service.UsageBillingCommand{RequestID: uuid.NewString(), UserID: user.ID, APIKeyID: key.ID, AccountID: account.ID, SubscriptionID: &sub.ID, SubscriptionCost: 8}
	_, err = repo.Apply(ctx, cmd)
	require.NoError(t, err)
	_, err = repo.Apply(ctx, cmd)
	require.NoError(t, err)
	require.Equal(t, 8.0, phase0QueryFloat(t, "SELECT short_usage_usd FROM user_subscriptions WHERE id=$1", sub.ID))
	require.Equal(t, 8.0, phase0QueryFloat(t, "SELECT weekly_usage_usd FROM user_subscriptions WHERE id=$1", sub.ID))
	core, _ := phase2NewCoreService(t, client, nil)
	at := time.Now().Add(time.Millisecond).Truncate(time.Microsecond) // PostgreSQL TIMESTAMPTZ stores microsecond precision.
	time.Sleep(2 * time.Millisecond)
	res, err := core.ResetSubscriptionWeeklyPeriod(ctx, &service.WeeklyResetInput{UserSubscriptionID: sub.ID, EffectiveAt: at, Source: domain.WeeklyResetSourceAdminDirect, DualWindows: true})
	require.NoError(t, err)
	require.Equal(t, service.WeeklyResetApplied, res.Status)
	require.Equal(t, 0.0, res.Subscription.ShortUsageUSD)
	require.Equal(t, 0.0, res.Subscription.WeeklyUsageUSD)
	require.Equal(t, 8.0, res.Subscription.MonthlyUsageUSD)
	// Late settlement is intentionally charged to the freshly reset window.
	cmd.RequestID = uuid.NewString()
	cmd.SubscriptionCost = 2
	_, err = repo.Apply(ctx, cmd)
	require.NoError(t, err)
	require.Equal(t, 2.0, phase0QueryFloat(t, "SELECT short_usage_usd FROM user_subscriptions WHERE id=$1", sub.ID))
	var wg sync.WaitGroup
	errs := make(chan error, 4)
	for i := 0; i < 4; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_, e := core.ResetSubscriptionWeeklyPeriod(ctx, &service.WeeklyResetInput{UserSubscriptionID: sub.ID, EffectiveAt: at, Source: domain.WeeklyResetSourceAdminDirect, DualWindows: true})
			errs <- e
		}()
	}
	wg.Wait()
	close(errs)
	for e := range errs {
		require.NoError(t, e)
	}
	require.Equal(t, 2.0, phase0QueryFloat(t, "SELECT short_usage_usd FROM user_subscriptions WHERE id=$1", sub.ID))
	require.Equal(t, 1.0, phase0QueryFloat(t, "SELECT count(*) FROM subscription_dual_reset_audit WHERE subscription_id=$1", sub.ID))
	require.Equal(t, 100.0, phase0QueryFloat(t, "SELECT balance FROM users WHERE id=$1", user.ID))
}

func TestDualQuotaMaintenanceDoesNotActivateOnRead(t *testing.T) {
	ctx := context.Background()
	client := testEntClient(t)
	limit := 50.0
	_, g, sub, _, _ := phase0MustSubscriptionStack(t, client, 0, &limit)
	_, err := integrationDB.ExecContext(ctx, `UPDATE groups SET quota_policy='dual_window_v1',short_limit_usd=10 WHERE id=$1`, g.ID)
	require.NoError(t, err)
	repo := NewUserSubscriptionRepository(client).(*userSubscriptionRepository)
	now := time.Now().Truncate(time.Microsecond)
	fresh, err := repo.MaintainDualWindows(ctx, sub.ID, now, false)
	require.NoError(t, err)
	require.Nil(t, fresh.ShortWindowStart)
	fresh, err = repo.MaintainDualWindows(ctx, sub.ID, now, true)
	require.NoError(t, err)
	require.WithinDuration(t, now, *fresh.ShortWindowStart, time.Microsecond)
	require.NoError(t, repo.IncrementUsage(ctx, sub.ID, 4))
	fresh, err = repo.MaintainDualWindows(ctx, sub.ID, now.Add(5*time.Hour), false)
	require.NoError(t, err)
	require.Equal(t, 0.0, fresh.ShortUsageUSD)
	require.Equal(t, 4.0, fresh.WeeklyUsageUSD)
	require.WithinDuration(t, now, *fresh.WeeklyWindowStart, time.Microsecond)
}

func TestDualQuotaCardAtomicReplayAndUpgrade(t *testing.T) {
	ctx := context.Background()
	client := testEntClient(t)
	limit := 50.0
	user, g, sub, _, _ := phase0MustSubscriptionStack(t, client, 0, &limit)
	_, err := integrationDB.ExecContext(ctx, `UPDATE groups SET quota_policy='dual_window_v1',short_limit_usd=10 WHERE id=$1`, g.ID)
	require.NoError(t, err)
	repo := NewUserSubscriptionRepository(client).(*userSubscriptionRepository)
	_, err = repo.MaintainDualWindows(ctx, sub.ID, time.Now().Add(-time.Hour), true)
	require.NoError(t, err)
	require.NoError(t, repo.IncrementUsage(ctx, sub.ID, 8))
	// Changing the tier/group preserves absolute usage and both anchors.
	before, err := repo.GetByID(ctx, sub.ID)
	require.NoError(t, err)
	newLimit := 100.0
	next := mustCreateGroup(t, client, &service.Group{Name: "dual-upgrade-" + uuid.NewString(), Platform: "openai", SubscriptionType: service.SubscriptionTypeSubscription, QuotaPolicy: service.QuotaPolicyDualWindow, ShortLimitUSD: func() *float64 { v := 20.0; return &v }(), WeeklyLimitUSD: &newLimit})
	plan, err := client.SubscriptionPlan.Create().SetGroupID(next.ID).SetName("dual-upgrade").SetPrice(20).Save(ctx)
	require.NoError(t, err)
	require.NoError(t, repo.SwitchPlan(ctx, sub.ID, next.ID, plan.ID))
	after, err := repo.GetByID(ctx, sub.ID)
	require.NoError(t, err)
	require.Equal(t, before.ShortUsageUSD, after.ShortUsageUSD)
	require.Equal(t, before.WeeklyUsageUSD, after.WeeklyUsageUSD)
	require.Equal(t, before.ShortWindowStart, after.ShortWindowStart)
	require.Equal(t, before.WeeklyWindowStart, after.WeeklyWindowStart)
	cards, _, _ := phase6NewServices(t, client)
	_, err = cards.GrantResetCards(ctx, &service.GrantResetCardsInput{Selector: service.ResetCardGrantSelector{Mode: "users", UserIDs: []int64{user.ID}}, QuantityPerUser: 2, IdempotencyKey: "dual-grant-" + uuid.NewString()})
	require.NoError(t, err)
	_, err = cards.ConsumeForSubscription(ctx, user.ID, sub.ID, "dual-old-"+uuid.NewString())
	require.Error(t, err)
	n, err := cards.CountAvailableResetCards(ctx, user.ID)
	require.NoError(t, err)
	require.EqualValues(t, 2, n)
	operation := "dual-use-" + uuid.NewString()
	first, err := cards.ConsumeForSubscription(ctx, user.ID, sub.ID, operation, 2)
	require.NoError(t, err)
	again, err := cards.ConsumeForSubscription(ctx, user.ID, sub.ID, operation, 2)
	require.NoError(t, err)
	require.Equal(t, first.CardID, again.CardID)
	n, err = cards.CountAvailableResetCards(ctx, user.ID)
	require.NoError(t, err)
	require.EqualValues(t, 1, n)
	fresh, err := repo.GetByID(ctx, sub.ID)
	require.NoError(t, err)
	require.Zero(t, fresh.ShortUsageUSD)
	require.Zero(t, fresh.WeeklyUsageUSD)
	require.Equal(t, 8.0, fresh.MonthlyUsageUSD)
	// Full quota is still allowed to reset (explicit product choice).
	_, err = cards.ConsumeForSubscription(ctx, user.ID, sub.ID, "dual-full-"+uuid.NewString(), 2)
	require.NoError(t, err)
}

func TestDualQuotaUpgradeFulfillmentAndStatus(t *testing.T) {
	ctx := context.Background()
	client := testEntClient(t)
	stack := phase10Setup(t, client, 39, 99, 15, 30)
	_, err := integrationDB.ExecContext(ctx, "UPDATE groups SET quota_policy='dual_window_v1',short_limit_usd=10,weekly_limit_usd=50 WHERE id=$1", stack.basicG.ID)
	require.NoError(t, err)
	_, err = integrationDB.ExecContext(ctx, "UPDATE groups SET quota_policy='dual_window_v1',short_limit_usd=20,weekly_limit_usd=100 WHERE id=$1", stack.proG.ID)
	require.NoError(t, err)
	repo := NewUserSubscriptionRepository(client).(*userSubscriptionRepository)
	_, err = repo.MaintainDualWindows(ctx, stack.basicSub.ID, time.Now().Add(-time.Hour), true)
	require.NoError(t, err)
	require.NoError(t, repo.IncrementUsage(ctx, stack.basicSub.ID, 8))
	before, err := repo.GetByID(ctx, stack.basicSub.ID)
	require.NoError(t, err)
	quote, id, err := stack.svc.CreateUpgradeQuote(ctx, stack.user.ID, before.ID, stack.proPlan.ID, "dual-upgrade-"+uuid.NewString())
	require.NoError(t, err)
	require.InDelta(t, 20.0, *quote.ShortRemainingPercentBefore, 1e-6)
	require.InDelta(t, 60.0, *quote.ShortRemainingPercentAfter, 1e-6)
	require.InDelta(t, 92.0, *quote.WeeklyRemainingPercentAfter, 1e-6)
	require.NoError(t, stack.changes.MarkPaid(ctx, id))
	require.NoError(t, stack.svc.FulfillUpgrade(ctx, id))
	require.NoError(t, stack.svc.FulfillUpgrade(ctx, id))
	after, err := repo.GetByID(ctx, before.ID)
	require.NoError(t, err)
	require.Equal(t, before.ShortUsageUSD, after.ShortUsageUSD)
	require.Equal(t, before.WeeklyUsageUSD, after.WeeklyUsageUSD)
	require.Equal(t, before.ShortWindowStart, after.ShortWindowStart)
	require.Equal(t, before.WeeklyWindowStart, after.WeeklyWindowStart)
	require.Equal(t, before.ExpiresAt, after.ExpiresAt)
	statusSvc := service.NewAccountStatusService(NewUserRepository(client, integrationDB), repo, NewGroupRepository(client, integrationDB), nil, nil, false)
	status, err := statusSvc.GetGroupSubscriptionStatus(ctx, stack.user.ID, stack.proG.ID)
	require.NoError(t, err)
	require.InDelta(t, 60.0, status.ShortWindow.RemainingPercent, 1e-6)
	require.InDelta(t, 92.0, status.WeeklyWindow.RemainingPercent, 1e-6)
	require.Equal(t, 20.0, phase0QueryFloat(t, "SELECT balance FROM users WHERE id=$1", stack.user.ID))
}

func TestDualQuotaStatisticsAndWeeklyExpiry(t *testing.T) {
	ctx := context.Background()
	client := testEntClient(t)
	limit := 50.0
	_, group, sub, _, _ := phase0MustSubscriptionStack(t, client, 0, &limit)
	_, err := integrationDB.ExecContext(ctx, "UPDATE groups SET quota_policy='dual_window_v1',short_limit_usd=10 WHERE id=$1", group.ID)
	require.NoError(t, err)
	now := time.Now().Truncate(time.Microsecond)
	_, err = integrationDB.ExecContext(ctx, "UPDATE user_subscriptions SET expires_at=$2, short_window_start=$3, weekly_window_start=$3, monthly_window_start=$3, daily_window_start=$3, short_usage_usd=8,weekly_usage_usd=9,monthly_usage_usd=10,daily_usage_usd=2 WHERE id=$1", sub.ID, now.Add(40*24*time.Hour), now.Add(-31*24*time.Hour))
	require.NoError(t, err)
	repo := NewUserSubscriptionRepository(client).(*userSubscriptionRepository)
	fresh, err := repo.MaintainDualWindows(ctx, sub.ID, now, false)
	require.NoError(t, err)
	require.Zero(t, fresh.ShortUsageUSD)
	require.Zero(t, fresh.WeeklyUsageUSD)
	require.Zero(t, fresh.DailyUsageUSD)
	require.Zero(t, fresh.MonthlyUsageUSD)
	require.WithinDuration(t, now.Add(-3*24*time.Hour), *fresh.WeeklyWindowStart, time.Microsecond)
}

func TestDualQuotaCohortMigrationDryRunAndIdempotency(t *testing.T) {
	ctx := context.Background()
	client := testEntClient(t)
	limit := 50.0
	_, group, sub, _, _ := phase0MustSubscriptionStack(t, client, 0, &limit)
	_, err := integrationDB.ExecContext(ctx, "UPDATE groups SET daily_limit_usd=10 WHERE id=$1", group.ID)
	require.NoError(t, err)
	anchor := time.Now().Add(-time.Hour).Truncate(time.Microsecond)
	phase0SetWeeklyWindow(t, sub.ID, anchor, 40)
	source, err := os.ReadFile("../../../scripts/migrate-dual-quota.sql")
	require.NoError(t, err)
	sql := string(source)
	sql = sql[strings.Index(sql, "BEGIN;"):strings.LastIndex(sql, "\\if :apply")]
	sql = strings.ReplaceAll(sql, ":'group_ids'", fmt.Sprintf("'%d,%d'", group.ID, group.ID))
	_, err = integrationDB.ExecContext(ctx, sql+"ROLLBACK;")
	require.NoError(t, err)
	var policy string
	require.NoError(t, integrationDB.QueryRowContext(ctx, "SELECT quota_policy FROM groups WHERE id=$1", group.ID).Scan(&policy))
	require.Equal(t, "legacy", policy)
	for i := 0; i < 2; i++ {
		_, err = integrationDB.ExecContext(ctx, sql+"COMMIT;")
		require.NoError(t, err)
	}
	require.Equal(t, 1.0, phase0QueryFloat(t, "SELECT count(*) FROM subscription_quota_policy_audit WHERE group_id=$1", group.ID))
	fresh, err := NewUserSubscriptionRepository(client).GetByID(ctx, sub.ID)
	require.NoError(t, err)
	require.Equal(t, 40.0, fresh.WeeklyUsageUSD)
	require.True(t, anchor.Equal(*fresh.WeeklyWindowStart))
	require.Nil(t, fresh.ShortWindowStart)
	require.Zero(t, fresh.ShortUsageUSD)
	require.Equal(t, 10.0, phase0QueryFloat(t, "SELECT short_limit_usd FROM groups WHERE id=$1", group.ID))
}

func TestDualQuotaConcurrentResetSettlementConservesUsage(t *testing.T) {
	ctx := context.Background()
	client := testEntClient(t)
	limit := 50.0
	user, group, sub, key, account := phase0MustSubscriptionStack(t, client, 100, &limit)
	_, err := integrationDB.ExecContext(ctx, "UPDATE groups SET quota_policy='dual_window_v1',short_limit_usd=10 WHERE id=$1", group.ID)
	require.NoError(t, err)
	repo := NewUsageBillingRepository(client, integrationDB)
	charge := func(cost float64) error {
		_, e := repo.Apply(ctx, &service.UsageBillingCommand{RequestID: uuid.NewString(), UserID: user.ID, APIKeyID: key.ID, AccountID: account.ID, SubscriptionID: &sub.ID, SubscriptionCost: cost})
		return e
	}
	require.NoError(t, charge(8))
	core, _ := phase2NewCoreService(t, client, nil)
	ready := make(chan struct{})
	results := make(chan error, 6)
	for i := 0; i < 5; i++ {
		go func() { <-ready; results <- charge(1) }()
	}
	go func() {
		<-ready
		_, e := core.ResetSubscriptionWeeklyPeriod(ctx, &service.WeeklyResetInput{UserSubscriptionID: sub.ID, EffectiveAt: time.Now(), Source: domain.WeeklyResetSourceAdminDirect, DualWindows: true})
		results <- e
	}()
	close(ready)
	for i := 0; i < 6; i++ {
		require.NoError(t, <-results)
	}
	cleared := phase0QueryFloat(t, "SELECT previous_short_usage FROM subscription_dual_reset_audit WHERE subscription_id=$1", sub.ID)
	current := phase0QueryFloat(t, "SELECT short_usage_usd FROM user_subscriptions WHERE id=$1", sub.ID)
	require.Equal(t, 13.0, cleared+current)
	require.Equal(t, 13.0, phase0QueryFloat(t, "SELECT monthly_usage_usd FROM user_subscriptions WHERE id=$1", sub.ID))
	require.Equal(t, current, phase0QueryFloat(t, "SELECT weekly_usage_usd FROM user_subscriptions WHERE id=$1", sub.ID))
}

func TestDualQuotaMigrationCannotSplitSettlementPolicy(t *testing.T) {
	ctx := context.Background()
	client := testEntClient(t)
	limit := 50.0
	_, group, sub, _, _ := phase0MustSubscriptionStack(t, client, 0, &limit)
	phase0SetWeeklyWindow(t, sub.ID, time.Now().Add(-time.Hour), 0)
	billing, err := integrationDB.BeginTx(ctx, nil)
	require.NoError(t, err)
	defer billing.Rollback()
	require.NoError(t, incrementUsageBillingSubscription(ctx, billing, sub.ID, 1))
	migration, err := integrationDB.BeginTx(ctx, nil)
	require.NoError(t, err)
	_, err = migration.ExecContext(ctx, "SET LOCAL lock_timeout='50ms'")
	require.NoError(t, err)
	_, err = migration.ExecContext(ctx, "UPDATE groups SET quota_policy='dual_window_v1',short_limit_usd=10 WHERE id=$1", group.ID)
	require.Error(t, err, "policy update must wait for the old-policy settlement")
	require.Contains(t, err.Error(), "lock timeout")
	require.NoError(t, migration.Rollback())
	require.NoError(t, billing.Commit())
	_, err = integrationDB.ExecContext(ctx, "UPDATE groups SET quota_policy='dual_window_v1',short_limit_usd=10 WHERE id=$1", group.ID)
	require.NoError(t, err)
	require.NoError(t, NewUserSubscriptionRepository(client).IncrementUsage(ctx, sub.ID, 1))
	require.Equal(t, 1.0, phase0QueryFloat(t, "SELECT short_usage_usd FROM user_subscriptions WHERE id=$1", sub.ID))
	require.Equal(t, 2.0, phase0QueryFloat(t, "SELECT weekly_usage_usd FROM user_subscriptions WHERE id=$1", sub.ID))
}

func TestDualQuotaTransactionalClientRetainsCallerRollback(t *testing.T) {
	ctx := context.Background()
	client := testEntClient(t)
	limit := 50.0
	_, group, sub, _, _ := phase0MustSubscriptionStack(t, client, 0, &limit)
	_, err := integrationDB.ExecContext(ctx, "UPDATE groups SET quota_policy='dual_window_v1',short_limit_usd=10 WHERE id=$1", group.ID)
	require.NoError(t, err)
	tx, err := client.Tx(ctx)
	require.NoError(t, err)
	defer tx.Rollback()
	repo := NewUserSubscriptionRepository(tx.Client())
	require.NoError(t, repo.IncrementUsage(ctx, sub.ID, 1.25))
	updated, err := repo.GetByID(ctx, sub.ID)
	require.NoError(t, err)
	require.Equal(t, 1.25, updated.ShortUsageUSD)
	require.NoError(t, tx.Rollback())
	require.Zero(t, phase0QueryFloat(t, "SELECT short_usage_usd FROM user_subscriptions WHERE id=$1", sub.ID))
}
