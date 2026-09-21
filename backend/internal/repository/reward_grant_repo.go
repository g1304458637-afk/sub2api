package repository

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"strings"
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

// rewardGrantAdminSelectColumns 管理端列表查询列：前 10 列与 scanRewardGrantRow 对齐，
// 之后依次追加用户 email/username 与发放人 email（COALESCE 兜底空串）。
const rewardGrantAdminSelectColumns = `
  g.id,
  g.user_id,
  g.idempotency_key,
  g.source_type,
  g.source_id,
  g.campaign,
  g.amount::double precision,
  g.granted_by,
  g.metadata,
  g.created_at,
  COALESCE(u.email, ''),
  COALESCE(u.username, ''),
  COALESCE(gu.email, '')`

// AdminList 管理端分页查询发放记录（created_at 倒序）。
// user_id / campaign / source_type 均为精确等值匹配，无 LIKE 转义问题；
// COUNT 走单表（过滤条件只引用 g），明细才 JOIN users 回填邮箱/用户名。
func (r *rewardGrantRepository) AdminList(ctx context.Context, filter *service.RewardGrantAdminFilter) (*service.RewardGrantList, error) {
	if r == nil || r.client == nil {
		return nil, fmt.Errorf("nil reward grant repository")
	}
	if filter == nil {
		filter = &service.RewardGrantAdminFilter{}
	}

	page := filter.Page
	if page <= 0 {
		page = 1
	}
	pageSize := filter.PageSize
	if pageSize <= 0 {
		pageSize = 20
	}
	if pageSize > 100 {
		pageSize = 100
	}

	clauses := []string{"1=1"}
	args := make([]any, 0, 5)
	if filter.UserID != nil {
		args = append(args, *filter.UserID)
		clauses = append(clauses, "g.user_id = $"+itoa(len(args)))
	}
	if v := strings.TrimSpace(filter.Campaign); v != "" {
		args = append(args, v)
		clauses = append(clauses, "g.campaign = $"+itoa(len(args)))
	}
	if v := strings.TrimSpace(filter.SourceType); v != "" {
		args = append(args, v)
		clauses = append(clauses, "g.source_type = $"+itoa(len(args)))
	}
	where := "WHERE " + strings.Join(clauses, " AND ")

	client := clientFromContext(ctx, r.client)

	var total int64
	countArgs := make([]any, len(args))
	copy(countArgs, args)
	countRows, err := client.QueryContext(ctx,
		`SELECT COUNT(*) FROM reward_grants g `+where, countArgs...)
	if err != nil {
		return nil, err
	}
	if !countRows.Next() {
		err = countRows.Err()
		_ = countRows.Close()
		if err != nil {
			return nil, err
		}
		return nil, fmt.Errorf("count reward grants: empty result")
	}
	if err := countRows.Scan(&total); err != nil {
		_ = countRows.Close()
		return nil, err
	}
	_ = countRows.Close()

	args = append(args, pageSize, (page-1)*pageSize)
	query := `SELECT ` + rewardGrantAdminSelectColumns + `
FROM reward_grants g
LEFT JOIN users u ON u.id = g.user_id
LEFT JOIN users gu ON gu.id = g.granted_by
` + where + `
ORDER BY g.created_at DESC, g.id DESC
LIMIT $` + itoa(len(args)-1) + ` OFFSET $` + itoa(len(args))

	rows, err := client.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()

	items := make([]service.RewardGrantAdminItem, 0)
	for rows.Next() {
		item, err := scanRewardGrantAdminRow(rows)
		if err != nil {
			return nil, err
		}
		items = append(items, *item)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return &service.RewardGrantList{Items: items, Total: int(total), Page: page, PageSize: pageSize}, nil
}

// scanRewardGrantAdminRow 扫描管理端列表行：前 10 列复用 scanRewardGrantRow，再读回填列。
func scanRewardGrantAdminRow(row interface{ Scan(dest ...any) error }) (*service.RewardGrantAdminItem, error) {
	grant, err := scanRewardGrantRow(row)
	if err != nil {
		return nil, err
	}
	var item service.RewardGrantAdminItem
	item.RewardGrant = *grant
	if err := row.Scan(&item.Email, &item.Username, &item.GrantedByEmail); err != nil {
		return nil, err
	}
	return &item, nil
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
