package repository

import (
	"context"
	"database/sql"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/service"
)

// Existing committed rows are the event source. Card consumption and natural rollover
// cannot produce a reward. Filtering is always by the authenticated user's id.
const rewardArrivalQuery = `
SELECT id, type, quantity, occurred_at, subscription_id FROM (
 SELECT 'card:' || c.id::text AS id,
   'reset_card_received' AS type,
   1 AS quantity, c.granted_at AS occurred_at, NULL::bigint AS subscription_id
 FROM subscription_reset_cards c
 WHERE c.user_id=$1 AND c.deleted_at IS NULL AND c.status IN ('available','used')
   AND (c.expires_at IS NULL OR c.expires_at > NOW()) AND c.granted_at >= $2
 UNION ALL
 SELECT 'reset:' || a.id::text AS id,
   'global_reset_received' AS type,
   1 AS quantity, a.applied_at AS occurred_at, a.user_subscription_id AS subscription_id
 FROM subscription_reset_applications a
 JOIN user_subscriptions s ON s.id=a.user_subscription_id
 JOIN subscription_reset_events e ON e.id=a.reset_event_id
 WHERE s.user_id=$1 AND s.deleted_at IS NULL AND a.status='applied'
   AND e.event_type='global_reset' AND a.applied_at >= $2
) arrivals ORDER BY occurred_at DESC, id DESC LIMIT 100`

func (r *subscriptionResetCardRepository) ListRewardArrivals(ctx context.Context, userID int64, since time.Time) ([]service.RewardArrival, error) {
	rows, err := r.client.QueryContext(ctx, rewardArrivalQuery, userID, since)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	result := []service.RewardArrival{}
	for rows.Next() {
		var arrival service.RewardArrival
		var subscription sql.NullInt64
		if err := rows.Scan(&arrival.ID, &arrival.Type, &arrival.Quantity, &arrival.OccurredAt, &subscription); err != nil {
			return nil, err
		}
		if subscription.Valid {
			arrival.SubscriptionID = &subscription.Int64
		}
		result = append(result, arrival)
	}
	return result, rows.Err()
}
