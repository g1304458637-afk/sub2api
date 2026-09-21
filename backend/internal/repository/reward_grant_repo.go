package repository

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"time"

	dbent "github.com/Wei-Shaw/sub2api/ent"
	"github.com/Wei-Shaw/sub2api/internal/service"
)

// rewardGrantRepository reward_grants 表的裸 SQL 实现。
//
// 与 user_affiliate_ledger 同类：金融流水表不建 ent schema，SQL 即事实。
// 所有方法经 clientFromContext 感知外层事务，保证与余额变更同事务。

type rewardGrantRepository struct {
	client *dbent.Client
}

func NewRewardGrantRepository(client *dbent.Client) service.RewardGrantRepository {
	return &rewardGrantRepository{client: client}
}

func (r *rewardGrantRepository) InsertIdempotent(ctx context.Context, grant *service.RewardGrant) (bool, error) {
	metadata := []byte("{}")
	if len(grant.Metadata) > 0 {
		raw, err := json.Marshal(grant.Metadata)
		if err != nil {
			return false, fmt.Errorf("marshal reward grant metadata: %w", err)
		}
		metadata = raw
	}

	client := clientFromContext(ctx, r.client)
	const insertSQL = `
INSERT INTO reward_grants (user_id, idempotency_key, source_type, source_id, campaign, amount, granted_by, metadata)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8)
ON CONFLICT (idempotency_key) DO NOTHING
RETURNING id, created_at`

	var (
		id        int64
		createdAt time.Time
	)
	rows, err := client.QueryContext(ctx, insertSQL,
		grant.UserID, grant.IdempotencyKey, grant.SourceType, grant.SourceID, grant.Campaign,
		grant.Amount, grant.GrantedBy, metadata,
	)
	if err != nil {
		return false, err
	}
	defer func() { _ = rows.Close() }()

	if !rows.Next() {
		if err := rows.Err(); err != nil {
			return false, err
		}
		// ON CONFLICT DO NOTHING：该幂等键已发放过
		return false, nil
	}
	if err := rows.Scan(&id, &createdAt); err != nil {
		return false, err
	}
	grant.ID = id
	grant.CreatedAt = createdAt
	return true, nil
}

func (r *rewardGrantRepository) GetByIdempotencyKey(ctx context.Context, idempotencyKey string) (*service.RewardGrant, error) {
	client := clientFromContext(ctx, r.client)
	const querySQL = `
SELECT id, user_id, idempotency_key, source_type, source_id, campaign, amount::double precision, granted_by, metadata, created_at
FROM reward_grants
WHERE idempotency_key = $1`

	rows, err := client.QueryContext(ctx, querySQL, idempotencyKey)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()

	if !rows.Next() {
		if err := rows.Err(); err != nil {
			return nil, err
		}
		return nil, nil
	}
	grant, err := scanRewardGrantRow(rows)
	if err != nil {
		return nil, err
	}
	return grant, nil
}

func (r *rewardGrantRepository) GetByUserSourceCampaign(ctx context.Context, userID int64, sourceType, campaign string) ([]service.RewardGrant, error) {
	client := clientFromContext(ctx, r.client)
	const querySQL = `
SELECT id, user_id, idempotency_key, source_type, source_id, campaign, amount::double precision, granted_by, metadata, created_at
FROM reward_grants
WHERE user_id = $1 AND source_type = $2 AND campaign = $3
ORDER BY created_at DESC, id DESC`

	rows, err := client.QueryContext(ctx, querySQL, userID, sourceType, campaign)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()

	out := make([]service.RewardGrant, 0)
	for rows.Next() {
		grant, err := scanRewardGrantRow(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, *grant)
	}
	return out, rows.Err()
}

func (r *rewardGrantRepository) ListByUser(ctx context.Context, userID int64, limit int) ([]service.RewardGrant, error) {
	if limit <= 0 {
		limit = 100
	}
	client := clientFromContext(ctx, r.client)
	const querySQL = `
SELECT id, user_id, idempotency_key, source_type, source_id, campaign, amount::double precision, granted_by, metadata, created_at
FROM reward_grants
WHERE user_id = $1
ORDER BY created_at DESC, id DESC
LIMIT $2`

	rows, err := client.QueryContext(ctx, querySQL, userID, limit)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()

	out := make([]service.RewardGrant, 0)
	for rows.Next() {
		grant, err := scanRewardGrantRow(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, *grant)
	}
	return out, rows.Err()
}

func (r *rewardGrantRepository) CountByUser(ctx context.Context, userID int64) (int64, error) {
	client := clientFromContext(ctx, r.client)
	rows, err := client.QueryContext(ctx,
		`SELECT COUNT(*) FROM reward_grants WHERE user_id = $1`, userID)
	if err != nil {
		return 0, err
	}
	defer func() { _ = rows.Close() }()

	if !rows.Next() {
		return 0, rows.Err()
	}
	var total int64
	if err := rows.Scan(&total); err != nil {
		return 0, err
	}
	return total, rows.Err()
}

func (r *rewardGrantRepository) GetBySource(ctx context.Context, sourceType string, sourceID int64) ([]service.RewardGrant, error) {
	client := clientFromContext(ctx, r.client)
	const querySQL = `
SELECT id, user_id, idempotency_key, source_type, source_id, campaign, amount::double precision, granted_by, metadata, created_at
FROM reward_grants
WHERE source_type = $1 AND source_id = $2
ORDER BY created_at DESC, id DESC`

	rows, err := client.QueryContext(ctx, querySQL, sourceType, sourceID)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()

	out := make([]service.RewardGrant, 0)
	for rows.Next() {
		grant, err := scanRewardGrantRow(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, *grant)
	}
	return out, rows.Err()
}

// ListAll 管理端发放记录查询：created_at 倒序 + 总数；userID 为 nil 时不过滤。
func (r *rewardGrantRepository) ListAll(ctx context.Context, userID *int64, limit, offset int) ([]service.RewardGrant, int64, error) {
	if limit <= 0 {
		limit = 20
	}
	client := clientFromContext(ctx, r.client)

	where := "WHERE TRUE"
	args := []any{}
	if userID != nil {
		args = append(args, *userID)
		where += fmt.Sprintf(" AND user_id = $%d", len(args))
	}

	var total int64
	countSQL := "SELECT COUNT(*) FROM reward_grants " + where
	countRows, err := client.QueryContext(ctx, countSQL, args...)
	if err != nil {
		return nil, 0, err
	}
	if countRows.Next() {
		if err := countRows.Scan(&total); err != nil {
			_ = countRows.Close()
			return nil, 0, err
		}
	}
	_ = countRows.Close()

	args = append(args, limit, offset)
	querySQL := `
SELECT id, user_id, idempotency_key, source_type, source_id, campaign, amount::double precision, granted_by, metadata, created_at
FROM reward_grants ` + where + `
ORDER BY created_at DESC, id DESC
LIMIT $` + fmt.Sprint(len(args)-1) + ` OFFSET $` + fmt.Sprint(len(args))

	rows, err := client.QueryContext(ctx, querySQL, args...)
	if err != nil {
		return nil, 0, err
	}
	defer func() { _ = rows.Close() }()

	out := make([]service.RewardGrant, 0)
	for rows.Next() {
		grant, err := scanRewardGrantRow(rows)
		if err != nil {
			return nil, 0, err
		}
		out = append(out, *grant)
	}
	return out, total, rows.Err()
}

// StatsRange 统计 [from, to) 内发放笔数与总金额；userID 为 nil 时全局。
func (r *rewardGrantRepository) StatsRange(ctx context.Context, userID *int64, from, to time.Time) (int64, float64, error) {
	client := clientFromContext(ctx, r.client)
	where := "WHERE created_at >= $1 AND created_at < $2"
	args := []any{from, to}
	if userID != nil {
		args = append(args, *userID)
		where += fmt.Sprintf(" AND user_id = $%d", len(args))
	}
	var count int64
	var amount float64
	rows, err := client.QueryContext(ctx,
		"SELECT COUNT(*), COALESCE(SUM(amount), 0)::double precision FROM reward_grants "+where,
		args...)
	if err != nil {
		return 0, 0, err
	}
	defer func() { _ = rows.Close() }()
	if rows.Next() {
		if err := rows.Scan(&count, &amount); err != nil {
			return 0, 0, err
		}
	}
	return count, amount, rows.Err()
}

// scanRewardGrantRow 扫描单行；source_id / granted_by 可空，metadata 为 JSONB。
func scanRewardGrantRow(row interface{ Scan(dest ...any) error }) (*service.RewardGrant, error) {
	var (
		grant          service.RewardGrant
		idempotencyKey string
		sourceID       sql.NullInt64
		grantedBy      sql.NullInt64
		metadata       []byte
		createdAt      time.Time
	)
	if err := row.Scan(&grant.ID, &grant.UserID, &idempotencyKey, &grant.SourceType, &sourceID,
		&grant.Campaign, &grant.Amount, &grantedBy, &metadata, &createdAt); err != nil {
		return nil, err
	}
	grant.IdempotencyKey = idempotencyKey
	if sourceID.Valid {
		v := sourceID.Int64
		grant.SourceID = &v
	}
	if grantedBy.Valid {
		v := grantedBy.Int64
		grant.GrantedBy = &v
	}
	grant.CreatedAt = createdAt
	if len(metadata) > 0 {
		md := map[string]any{}
		if err := json.Unmarshal(metadata, &md); err == nil {
			grant.Metadata = md
		}
	}
	return &grant, nil
}
