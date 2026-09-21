package repository

import (
	"context"
	stdsql "database/sql"
	"time"

	dbent "github.com/Wei-Shaw/sub2api/ent"
	"github.com/Wei-Shaw/sub2api/ent/subscriptionresetcard"
	"github.com/Wei-Shaw/sub2api/internal/domain"
	"github.com/Wei-Shaw/sub2api/internal/service"
)

// subscriptionResetCardRepository 实现 service.SubscriptionResetCardReader 与
// service.ResetCardStore（Phase 6 Reset Card Runtime）。
//
// 过期语义：可用 = status='available' AND (expires_at IS NULL OR expires_at > now)，
// == now 视为过期（严格大于）。不做惰性状态修改：expired 是"有效状态"，
// status 列的批量归档归属 Reset Card Runtime 的运营操作，与本读写路径解耦。
//
// 并发：GetAvailableForUserForUpdate 使用 SELECT ... FOR UPDATE（行锁），
// 排序 = 最早到期（NULLS LAST）→ 最早创建 → 最小 id（确定性 FIFO 选卡）。
type subscriptionResetCardRepository struct {
	client *dbent.Client
}

func NewSubscriptionResetCardRepository(client *dbent.Client) service.SubscriptionResetCardReader {
	return &subscriptionResetCardRepository{client: client}
}

// NewSubscriptionResetCardStore 返回完整读写 Store（Phase 6 Runtime 使用）。
func NewSubscriptionResetCardStore(client *dbent.Client) service.ResetCardStore {
	return &subscriptionResetCardRepository{client: client}
}

func (r *subscriptionResetCardRepository) CountAvailableResetCards(ctx context.Context, userID int64, now time.Time) (int, error) {
	return r.client.SubscriptionResetCard.Query().
		Where(
			subscriptionresetcard.UserIDEQ(userID),
			subscriptionresetcard.StatusEQ(domain.ResetCardStatusAvailable),
			subscriptionresetcard.Or(
				subscriptionresetcard.ExpiresAtIsNil(),
				subscriptionresetcard.ExpiresAtGT(now),
			),
		).
		Count(ctx)
}

// ---- Phase 6 Store ----

func (r *subscriptionResetCardRepository) GrantCards(ctx context.Context, eventID int64, targets []service.GrantCardTarget, expiresAt *time.Time, sourceType string, createdBy *int64, notes string) (int, error) {
	tx := txClientFromContext(ctx, r.client)
	granted := 0
	for _, tgt := range targets {
		create := tx.SubscriptionResetCard.Create().
			SetUserID(tgt.UserID).
			SetStatus(domain.ResetCardStatusAvailable).
			SetScope(domain.ResetCardScopeWeekly).
			SetSourceType(sourceType).
			SetGrantEventID(eventID).
			SetGrantIndex(tgt.GrantIndex).
			SetNotes(notes).
			SetMetadata(map[string]any{})
		if expiresAt != nil {
			create.SetExpiresAt(*expiresAt)
		}
		if createdBy != nil {
			create.SetNillableCreatedBy(createdBy)
		}
		if _, err := create.Save(ctx); err != nil {
			return granted, err
		}
		granted++
	}
	return granted, nil
}

func (r *subscriptionResetCardRepository) GetAvailableForUserForUpdate(ctx context.Context, userID int64, now time.Time) (*service.ResetCard, error) {
	// FIFO：最早到期（NULL 最后）→ 最早创建 → 最小 id；行锁防并发双消费。
	// 软删除由 ent 拦截器自动过滤。
	const query = `
		SELECT id, user_id, status, scope, source_type, campaign, grant_event_id, grant_index,
		       granted_at, expires_at, used_at, used_subscription_id, created_by, notes,
		       created_at, updated_at
		FROM subscription_reset_cards
		WHERE user_id = $1
		  AND status = 'available'
		  AND (expires_at IS NULL OR expires_at > $2)
		  AND deleted_at IS NULL
		ORDER BY expires_at ASC NULLS LAST, granted_at ASC, id ASC
		LIMIT 1
		FOR UPDATE
	`
	rows, err := txClientFromContext(ctx, r.client).QueryContext(ctx, query, userID, now)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()
	if !rows.Next() {
		if rows.Err() != nil {
			return nil, rows.Err()
		}
		return nil, service.ErrResetCardNoAvailable
	}
	return scanResetCard(rows)
}

type resetCardRow interface{ Scan(dest ...any) error }

func scanResetCard(row resetCardRow) (*service.ResetCard, error) {
	var c service.ResetCard
	var campaign, notes *string
	err := row.Scan(
		&c.ID, &c.UserID, &c.Status, &c.Scope, &c.SourceType, &campaign,
		&c.GrantEventID, &c.GrantIndex, &c.GrantedAt, &c.ExpiresAt, &c.UsedAt,
		&c.UsedSubscriptionID, &c.CreatedBy, &notes, &c.CreatedAt, &c.UpdatedAt,
	)
	if err != nil {
		return nil, err
	}
	if campaign != nil {
		c.Campaign = campaign
	}
	if notes != nil {
		c.Notes = *notes
	}
	return &c, nil
}

func (r *subscriptionResetCardRepository) MarkUsed(ctx context.Context, cardID, subscriptionID int64, usedAt time.Time) error {
	res, err := txExecContext(ctx, r.client, `
		UPDATE subscription_reset_cards
		SET status = 'used', used_at = $1, used_subscription_id = $2, updated_at = NOW()
		WHERE id = $3 AND status = 'available'
	`, usedAt, subscriptionID, cardID)
	if err != nil {
		return err
	}
	if affected, _ := res.RowsAffected(); affected == 0 {
		return service.ErrResetCardNoAvailable
	}
	return nil
}

func (r *subscriptionResetCardRepository) Revoke(ctx context.Context, cardID int64, now time.Time) error {
	res, err := txExecContext(ctx, r.client, `
		UPDATE subscription_reset_cards
		SET status = 'revoked', updated_at = NOW()
		WHERE id = $1 AND status = 'available'
	`, cardID)
	if err != nil {
		return err
	}
	if affected, _ := res.RowsAffected(); affected == 0 {
		return service.ErrResetCardNoAvailable
	}
	return nil
}

func (r *subscriptionResetCardRepository) ListByUser(ctx context.Context, userID int64, status *string, limit, offset int) ([]service.ResetCard, int, error) {
	query := r.client.SubscriptionResetCard.Query().
		Where(subscriptionresetcard.UserIDEQ(userID)).
		Order(dbent.Desc(subscriptionresetcard.FieldGrantedAt))
	if status != nil && *status != "" {
		query = query.Where(subscriptionresetcard.StatusEQ(*status))
	}
	total, err := query.Count(ctx)
	if err != nil {
		return nil, 0, err
	}
	cards, err := query.
		Limit(limit).
		Offset(offset).
		Order(dbent.Desc(subscriptionresetcard.FieldGrantedAt)).
		All(ctx)
	if err != nil {
		return nil, 0, err
	}
	out := make([]service.ResetCard, 0, len(cards))
	for _, m := range cards {
		c := service.ResetCard{
			ID:                 m.ID,
			UserID:             m.UserID,
			Status:             m.Status,
			Scope:              m.Scope,
			SourceType:         m.SourceType,
			GrantIndex:         m.GrantIndex,
			GrantedAt:          m.GrantedAt,
			UsedAt:             m.UsedAt,
			UsedSubscriptionID: m.UsedSubscriptionID,
			Notes:              m.Notes,
			CreatedAt:          m.CreatedAt,
			UpdatedAt:          m.UpdatedAt,
		}
		out = append(out, c)
	}
	return out, total, nil
}

func (r *subscriptionResetCardRepository) GetByID(ctx context.Context, cardID int64) (*service.ResetCard, error) {
	m, err := r.client.SubscriptionResetCard.Get(ctx, cardID)
	if err != nil {
		return nil, service.ErrResetCardNotFound
	}
	c := &service.ResetCard{
		ID:                 m.ID,
		UserID:             m.UserID,
		Status:             m.Status,
		Scope:              m.Scope,
		SourceType:         m.SourceType,
		GrantIndex:         m.GrantIndex,
		GrantedAt:          m.GrantedAt,
		UsedAt:             m.UsedAt,
		UsedSubscriptionID: m.UsedSubscriptionID,
		Notes:              m.Notes,
		CreatedAt:          m.CreatedAt,
		UpdatedAt:          m.UpdatedAt,
	}
	if m.Campaign != nil {
		c.Campaign = m.Campaign
	}
	if m.ExpiresAt != nil {
		c.ExpiresAt = m.ExpiresAt
	}
	if m.GrantEventID != nil {
		c.GrantEventID = m.GrantEventID
	}
	if m.CreatedBy != nil {
		c.CreatedBy = m.CreatedBy
	}
	return c, nil
}

// txClientFromContext 返回事务内 client 或默认 client（与 clientFromContext 同语义）。
func txClientFromContext(ctx context.Context, def *dbent.Client) *dbent.Client {
	if tx := dbent.TxFromContext(ctx); tx != nil {
		return tx.Client()
	}
	return def
}

// txExecContext 在事务（或默认连接）上执行 raw SQL。
func txExecContext(ctx context.Context, def *dbent.Client, query string, args ...any) (stdsql.Result, error) {
	return txClientFromContext(ctx, def).ExecContext(ctx, query, args...)
}
