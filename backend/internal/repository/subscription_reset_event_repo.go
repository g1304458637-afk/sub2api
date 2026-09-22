package repository

import (
	"context"
	"fmt"
	"time"

	dbent "github.com/Wei-Shaw/sub2api/ent"
	"github.com/Wei-Shaw/sub2api/ent/subscriptionresetevent"
	"github.com/Wei-Shaw/sub2api/internal/domain"
	"github.com/Wei-Shaw/sub2api/internal/service"
)

// subscriptionResetEventRepo 实现 service.ResetEventStore（Phase 7 Direct Reset Runtime）。
//
// Job 模型：applications 行即任务；claim 使用 FOR UPDATE SKIP LOCKED 原子认领，
// 认领 + Reset Core + 终态在单事务内 —— 崩溃自动回到 pending，多 worker 安全。
type subscriptionResetEventRepo struct {
	client *dbent.Client
}

func NewSubscriptionResetEventStore(client *dbent.Client) service.ResetEventStore {
	return &subscriptionResetEventRepo{client: client}
}

func (r *subscriptionResetEventRepo) CreateEventWithApplications(ctx context.Context, ev *service.ResetEventRecord, applications []int64) (int64, error) {
	tx, err := txClientFromContext(ctx, r.client).Tx(ctx)
	if err != nil {
		return 0, err
	}
	defer func() { _ = tx.Rollback() }()

	created, err := tx.SubscriptionResetEvent.Create().
		SetEventType(ev.EventType).
		SetStatus(ev.Status).
		SetScopeType(ev.ScopeType).
		SetScope(ev.Scope).
		SetEffectiveAt(ev.EffectiveAt).
		SetReason(ev.Reason).
		SetMetadata(map[string]any{}).
		Save(ctx)
	if err != nil {
		return 0, err
	}
	for _, subID := range applications {
		if _, err := tx.SubscriptionResetApplication.Create().
			SetResetEventID(created.ID).
			SetUserSubscriptionID(subID).
			SetEffectiveAt(ev.EffectiveAt).
			SetStatus(domain.ResetApplicationStatusPending).
			SetMetadata(map[string]any{}).
			Save(ctx); err != nil {
			return 0, err
		}
	}
	if err := tx.Commit(); err != nil {
		return 0, err
	}
	return created.ID, nil
}

func (r *subscriptionResetEventRepo) GetDueEventIDs(ctx context.Context, now time.Time, limit int) ([]int64, error) {
	return r.client.SubscriptionResetEvent.Query().
		Where(
			subscriptionresetevent.StatusIn(domain.ResetEventStatusPending, domain.ResetEventStatusRunning),
			subscriptionresetevent.EffectiveAtLTE(now),
		).
		Order(dbent.Asc(subscriptionresetevent.FieldEffectiveAt)).
		Limit(limit).
		IDs(ctx)
}

func (r *subscriptionResetEventRepo) ClaimRunning(ctx context.Context, eventID int64) error {
	_, err := txExecContext(ctx, r.client, `
		UPDATE subscription_reset_events
		SET status = 'running', started_at = COALESCE(started_at, NOW())
		WHERE id = $1 AND status IN ('pending', 'running')
	`, eventID)
	return err
}

func (r *subscriptionResetEventRepo) GetEvent(ctx context.Context, eventID int64) (*service.ResetEventRecord, error) {
	m, err := r.client.SubscriptionResetEvent.Get(ctx, eventID)
	if err != nil {
		return nil, service.ErrResetEventNotFound
	}
	rec := &service.ResetEventRecord{
		ID:          m.ID,
		EventType:   m.EventType,
		Status:      m.Status,
		EffectiveAt: m.EffectiveAt,
		ScopeType:   m.ScopeType,
		Scope:       m.Scope,
		Reason:      m.Reason,
		StartedAt:   m.StartedAt,
		CompletedAt: m.CompletedAt,
		CreatedAt:   m.CreatedAt,
		UpdatedAt:   m.UpdatedAt,
	}
	return rec, nil
}

func (r *subscriptionResetEventRepo) ListEvents(ctx context.Context, limit, offset int) ([]*service.ResetEventRecord, error) {
	ms, err := r.client.SubscriptionResetEvent.Query().
		Order(dbent.Desc(subscriptionresetevent.FieldID)).
		Limit(limit).
		Offset(offset).
		All(ctx)
	if err != nil {
		return nil, err
	}
	out := make([]*service.ResetEventRecord, 0, len(ms))
	for _, m := range ms {
		out = append(out, &service.ResetEventRecord{
			ID: m.ID, EventType: m.EventType, Status: m.Status,
			EffectiveAt: m.EffectiveAt, ScopeType: m.ScopeType, Scope: m.Scope,
			Reason:    m.Reason,
			StartedAt: m.StartedAt, CompletedAt: m.CompletedAt,
			CreatedAt: m.CreatedAt, UpdatedAt: m.UpdatedAt,
		})
	}
	return out, nil
}

func (r *subscriptionResetEventRepo) ApplicationStats(ctx context.Context, eventID int64) (map[string]int64, error) {
	rows, err := txClientFromContext(ctx, r.client).QueryContext(ctx, `
		SELECT status, COUNT(*) FROM subscription_reset_applications
		WHERE reset_event_id = $1 GROUP BY status
	`, eventID)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()
	stats := map[string]int64{}
	for rows.Next() {
		var status string
		var count int64
		if err := rows.Scan(&status, &count); err != nil {
			return nil, err
		}
		stats[status] = count
	}
	return stats, rows.Err()
}

// ClaimApplicationBatch SKIP LOCKED 原子认领一批 pending application。
func (r *subscriptionResetEventRepo) ClaimApplicationBatch(ctx context.Context, eventID int64, limit int) ([]service.ResetApplicationClaim, error) {
	claimed := make([]service.ResetApplicationClaim, 0, limit)
	for i := 0; i < limit; i++ {
		rows, err := txClientFromContext(ctx, r.client).QueryContext(ctx, `
			UPDATE subscription_reset_applications
			SET status = 'applying', applied_at = NOW()
			WHERE id = (
				SELECT id FROM subscription_reset_applications
				WHERE reset_event_id = $1 AND (status = 'pending' OR (status = 'applying' AND applied_at < NOW() - INTERVAL '5 minutes'))
				ORDER BY id
				FOR UPDATE SKIP LOCKED
				LIMIT 1
			)
			RETURNING id, user_subscription_id
		`, eventID)
		if err != nil {
			return claimed, err
		}
		var appID, subID int64
		has := rows.Next()
		if has {
			err = rows.Scan(&appID, &subID)
		}
		closeErr := rows.Close()
		if err != nil {
			return claimed, err
		}
		if closeErr != nil {
			return claimed, closeErr
		}
		if !has {
			break
		}
		claimed = append(claimed, service.ResetApplicationClaim{ApplicationID: appID, UserSubscriptionID: subID})
	}
	return claimed, nil
}

func (r *subscriptionResetEventRepo) GetApplicationForUpdate(ctx context.Context, appID int64) (*service.ResetApplicationRecord, error) {
	rows, err := txClientFromContext(ctx, r.client).QueryContext(ctx, `
		SELECT id, reset_event_id, user_subscription_id, effective_at,
		       previous_weekly_window_start, previous_weekly_usage_usd,
		       applied_at, status, COALESCE((metadata->>'attempts')::int, 0)
		FROM subscription_reset_applications
		WHERE id = $1
		FOR UPDATE
	`, appID)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()
	if !rows.Next() {
		return nil, service.ErrResetEventNotFound
	}
	rec := &service.ResetApplicationRecord{}
	if err := rows.Scan(
		&rec.ID, &rec.ResetEventID, &rec.UserSubscriptionID, &rec.EffectiveAt,
		&rec.PreviousWeeklyWindowStart, &rec.PreviousWeeklyUsageUSD,
		&rec.AppliedAt, &rec.Status, &rec.Attempts,
	); err != nil {
		return nil, err
	}
	return rec, rows.Err()
}

func (r *subscriptionResetEventRepo) FinalizeApplication(ctx context.Context, appID int64, prevStart *time.Time, prevUsage *float64, finalStatus string) error {
	res, err := txExecContext(ctx, r.client, `
		UPDATE subscription_reset_applications
		SET status = $2,
		    previous_weekly_window_start = COALESCE(previous_weekly_window_start, $3),
		    previous_weekly_usage_usd = COALESCE(previous_weekly_usage_usd, $4)
		WHERE id = $1
	`, appID, finalStatus, prevStart, prevUsage)
	if err != nil {
		return err
	}
	if affected, _ := res.RowsAffected(); affected == 0 {
		return fmt.Errorf("application %d not found", appID)
	}
	return nil
}

// MarkApplicationFailed 独立事务（调用方已回滚业务事务）落失败账 + attempts++。
func (r *subscriptionResetEventRepo) MarkApplicationFailed(ctx context.Context, appID int64, reason string) error {
	_, err := txExecContext(ctx, r.client, `
		UPDATE subscription_reset_applications
		SET status = 'failed',
		    metadata = COALESCE(metadata, '{}'::jsonb)
		              || jsonb_build_object('attempts',
		                  COALESCE((metadata->>'attempts')::int, 0) + 1,
		                  'last_error', $2)
		WHERE id = $1 AND status = 'applying'
	`, appID, reason)
	return err
}

// RequeueFailed failed → pending（retry）。
func (r *subscriptionResetEventRepo) RequeueFailed(ctx context.Context, eventID int64) (int64, error) {
	rows, err := txClientFromContext(ctx, r.client).QueryContext(ctx, `
 WITH parent AS (
   SELECT id FROM subscription_reset_events WHERE id = $1 FOR UPDATE
 ), requeued AS (
   UPDATE subscription_reset_applications a SET status = 'pending'
   FROM parent WHERE a.reset_event_id = parent.id AND a.status = 'failed'
   RETURNING a.id
 ), reopened AS (
   UPDATE subscription_reset_events SET status = 'pending', completed_at = NULL
   WHERE id = $1 AND EXISTS (SELECT 1 FROM requeued) RETURNING id
 ) SELECT COUNT(*) FROM requeued`, eventID)
	if err != nil {
		return 0, err
	}
	defer func() { _ = rows.Close() }()
	var count int64
	if rows.Next() {
		if err := rows.Scan(&count); err != nil {
			return 0, err
		}
	}
	return count, rows.Err()
}

// CompleteIfDrained 全部 application 到终态时收口事件状态。
func (r *subscriptionResetEventRepo) CompleteIfDrained(ctx context.Context, eventID int64) (string, error) {
	rows, err := txClientFromContext(ctx, r.client).QueryContext(ctx, `
		UPDATE subscription_reset_events e
		SET status = CASE
		        WHEN stats.failed > 0 AND stats.applied + stats.skipped = 0 THEN 'failed'
		        WHEN stats.failed > 0 THEN 'partial_failed'
		        ELSE 'completed'
		    END,
		    completed_at = NOW()
		FROM (
		    SELECT
		        COUNT(*) FILTER (WHERE status = 'applied') AS applied,
		        COUNT(*) FILTER (WHERE status = 'skipped') AS skipped,
		        COUNT(*) FILTER (WHERE status = 'failed') AS failed,
		        COUNT(*) FILTER (WHERE status = 'pending' OR status = 'applying') AS open
		    FROM subscription_reset_applications
		    WHERE reset_event_id = $1
		) stats
		WHERE e.id = $1 AND stats.open = 0
		RETURNING e.status
	`, eventID)
	if err != nil {
		return "", err
	}
	defer func() { _ = rows.Close() }()
	final := ""
	if rows.Next() {
		if err := rows.Scan(&final); err != nil {
			return "", err
		}
	}
	return final, rows.Err()
}
