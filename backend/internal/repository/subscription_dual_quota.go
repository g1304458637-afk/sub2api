package repository

import (
	"context"
	"errors"
	dbent "github.com/Wei-Shaw/sub2api/ent"
	"github.com/Wei-Shaw/sub2api/internal/pkg/timezone"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"time"
)

func (r *userSubscriptionRepository) MaintainDualWindows(ctx context.Context, id int64, now time.Time, activate bool) (*service.UserSubscription, error) {
	client := clientFromContext(ctx, r.client)
	if _, err := client.ExecContext(ctx, "SELECT campus_advance_dual_windows($1,$2,$3,$4)", id, now, activate, timezone.StartOfDay(now)); err != nil {
		return nil, err
	}
	return r.GetByID(ctx, id)
}

func (r *userSubscriptionRepository) ResetDualWindows(ctx context.Context, in *service.WeeklyResetInput) error {
	if dbent.TxFromContext(ctx) == nil {
		return errors.New("dual reset requires locked transaction")
	}
	id, at := in.UserSubscriptionID, in.EffectiveAt
	eventID := in.ResetEventID
	if eventID == nil {
		eventID = in.AuditEventID
	}
	if _, err := clientFromContext(ctx, r.client).ExecContext(ctx, `INSERT INTO subscription_dual_reset_audit(subscription_id,effective_at,source,card_id,event_id,actor_id,previous_short_usage,previous_weekly_usage,previous_short_start,previous_weekly_start) SELECT id,$2,$3,$4,$5,$6,short_usage_usd,weekly_usage_usd,short_window_start,weekly_window_start FROM user_subscriptions WHERE id=$1`, id, at, in.Source, in.ResetCardID, eventID, in.ActorID); err != nil {
		return err
	}

	_, err := clientFromContext(ctx, r.client).ExecContext(ctx, `UPDATE user_subscriptions SET short_usage_usd=0, weekly_usage_usd=0, short_window_start=$2, weekly_window_start=$2, updated_at=NOW() WHERE id=$1 AND deleted_at IS NULL`, id, at)
	return err
}
