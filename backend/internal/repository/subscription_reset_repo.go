package repository

import (
	"context"
	"errors"
	"time"

	dbent "github.com/Wei-Shaw/sub2api/ent"
	"github.com/Wei-Shaw/sub2api/ent/subscriptionresetapplication"
	"github.com/Wei-Shaw/sub2api/internal/domain"
	"github.com/Wei-Shaw/sub2api/internal/service"
)

// subscriptionResetApplicationRepository 实现 service.SubscriptionResetApplicationRepository。
// 所有方法经 clientFromContext 支持事务传播（与 Reset Core 同事务执行）。
type subscriptionResetApplicationRepository struct {
	client *dbent.Client
}

func NewSubscriptionResetApplicationRepository(client *dbent.Client) service.SubscriptionResetApplicationRepository {
	return &subscriptionResetApplicationRepository{client: client}
}

// ClaimWeeklyResetApplication 认领 (event, subscription) 应用记录。
// 先查询后插入：并发冲突由 UNIQUE(reset_event_id, user_subscription_id) 兜底，
// 插入撞唯一约束时回读既有行（事务内 retry-safe）。
func (r *subscriptionResetApplicationRepository) ClaimWeeklyResetApplication(
	ctx context.Context, resetEventID, userSubscriptionID int64, effectiveAt time.Time,
) (claimed bool, existingStatus string, appID int64, err error) {
	client := clientFromContext(ctx, r.client)

	existing, qerr := client.SubscriptionResetApplication.Query().
		Where(
			subscriptionresetapplication.ResetEventIDEQ(resetEventID),
			subscriptionresetapplication.UserSubscriptionIDEQ(userSubscriptionID),
		).
		Only(ctx)
	if qerr == nil {
		return false, existing.Status, existing.ID, nil
	}
	if !dbent.IsNotFound(qerr) {
		return false, "", 0, qerr
	}

	created, cerr := client.SubscriptionResetApplication.Create().
		SetResetEventID(resetEventID).
		SetUserSubscriptionID(userSubscriptionID).
		SetEffectiveAt(effectiveAt).
		SetStatus(domain.ResetApplicationStatusApplied).
		SetMetadata(map[string]any{}).
		Save(ctx)
	if cerr != nil {
		// 并发认领撞唯一约束：回读既有行（同事务内另一 worker 已提交或本事务可见）
		var constraintErr *dbent.ConstraintError
		if errors.As(cerr, &constraintErr) {
			existing, qerr = client.SubscriptionResetApplication.Query().
				Where(
					subscriptionresetapplication.ResetEventIDEQ(resetEventID),
					subscriptionresetapplication.UserSubscriptionIDEQ(userSubscriptionID),
				).
				Only(ctx)
			if qerr != nil {
				return false, "", 0, qerr
			}
			return false, existing.Status, existing.ID, nil
		}
		return false, "", 0, cerr
	}
	return true, created.Status, created.ID, nil
}

// FinalizeClaimedWeeklyResetApplication 回填审计值并落最终 status（同事务内）。
func (r *subscriptionResetApplicationRepository) FinalizeClaimedWeeklyResetApplication(
	ctx context.Context, id int64, previousStart *time.Time, previousUsage *float64, appliedAt time.Time, finalStatus string,
) error {
	client := clientFromContext(ctx, r.client)
	update := client.SubscriptionResetApplication.UpdateOneID(id).
		SetNillablePreviousWeeklyWindowStart(previousStart).
		SetNillablePreviousWeeklyUsageUsd(previousUsage).
		SetAppliedAt(appliedAt).
		SetStatus(finalStatus)
	if _, err := update.Save(ctx); err != nil {
		return translatePersistenceError(err, service.ErrSubscriptionNotFound, nil)
	}
	return nil
}
