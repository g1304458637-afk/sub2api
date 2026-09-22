//go:build integration

package repository

import (
	"context"
	"github.com/stretchr/testify/require"
	"testing"
	"time"
)

func TestRewardArrivalCommittedUserReceipts(t *testing.T) {
	ctx := context.Background()
	client := testEntClient(t)
	limit := 100.0
	owner, group, sub, _, _ := phase0MustSubscriptionStack(t, client, 0, &limit)
	other, otherGroup, _, _, _ := phase0MustSubscriptionStack(t, client, 0, &limit)
	phase0CleanupStack(t, owner.ID, group.ID, 0)
	phase0CleanupStack(t, other.ID, otherGroup.ID, 0)
	// Fixtures live in a rollback-only transaction; the SQL is the production read query.
	tx := testEntTx(t)
	_, err := tx.Client().ExecContext(ctx, `INSERT INTO subscription_reset_cards(user_id,status,source_type) VALUES ($1,'available','admin_grant'),($1,'available','user_gift'),($1,'revoked','admin_grant'),($2,'available','admin_grant')`, owner.ID, other.ID)
	require.NoError(t, err)
	rows, err := tx.Client().QueryContext(ctx, `INSERT INTO subscription_reset_events(event_type,status,effective_at,scope_type) VALUES ('global_reset','completed',NOW(),'all') RETURNING id`)
	require.NoError(t, err)
	require.True(t, rows.Next())
	var eventID int64
	require.NoError(t, rows.Scan(&eventID))
	require.NoError(t, rows.Close())
	_, err = tx.Client().ExecContext(ctx, `INSERT INTO subscription_reset_applications(reset_event_id,user_subscription_id,effective_at,status) VALUES ($1,$2,NOW(),'applied')`, eventID, sub.ID)
	require.NoError(t, err)
	repo := &subscriptionResetCardRepository{client: tx.Client()}
	result, err := repo.ListRewardArrivals(ctx, owner.ID, time.Now().Add(-time.Hour))
	require.NoError(t, err)
	require.Len(t, result, 3)
	counts := map[string]int{}
	for _, event := range result {
		counts[event.Type]++
		require.Equal(t, 1, event.Quantity)
	}
	require.Equal(t, 2, counts["reset_card_received"])
	require.Equal(t, 1, counts["global_reset_received"])
	_, err = tx.Client().ExecContext(ctx, `UPDATE subscription_reset_applications SET status='skipped' WHERE reset_event_id=$1`, eventID)
	require.NoError(t, err)
	result, err = repo.ListRewardArrivals(ctx, owner.ID, time.Now().Add(-time.Hour))
	require.NoError(t, err)
	require.Len(t, result, 2)
	result, err = repo.ListRewardArrivals(ctx, other.ID, time.Now().Add(-time.Hour))
	require.NoError(t, err)
	require.Len(t, result, 1)
}
