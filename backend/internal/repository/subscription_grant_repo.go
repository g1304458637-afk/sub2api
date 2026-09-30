package repository

import (
	"context"
	"database/sql"
	"fmt"
	"strconv"
	"strings"
	"time"

	dbent "github.com/Wei-Shaw/sub2api/ent"
	"github.com/Wei-Shaw/sub2api/internal/service"
)

// subscriptionGrantRepository subscription_grants / benefit_claims 表的裸 SQL 实现。
//
// 与 rewardGrantRepository 同类：权益台账是金融级审计事实，SQL 即事实，
// 不建 ent schema。所有方法经 clientFromContext 感知外层事务，保证
// 「发放记录 + 订阅变更 + claim」同事务提交。
type subscriptionGrantRepository struct {
	client *dbent.Client
}

func NewSubscriptionGrantRepository(client *dbent.Client) service.SubscriptionGrantRepository {
	return &subscriptionGrantRepository{client: client}
}

// grantColumns 与 scanGrantRow 的扫描顺序严格对齐。
const grantColumns = `
  id, user_id, group_id, plan_id, source, source_key, benefit_code, identity_type,
  identity_key, idempotency_key, status, effective_policy, duration_days, reason,
  notes, operator_user_id, linked_subscription_id, contribution_start, contribution_end,
  activated_at, revoked_at, revoked_by, revoke_reason, failure_reason, created_at, updated_at`

func (r *subscriptionGrantRepository) InsertIdempotent(ctx context.Context, grant *service.SubscriptionGrant) (bool, error) {
	client := clientFromContext(ctx, r.client)
	const insertSQL = `
INSERT INTO subscription_grants (
  user_id, group_id, plan_id, source, source_key, benefit_code, identity_type,
  identity_key, idempotency_key, status, effective_policy, duration_days, reason,
  notes, operator_user_id
) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15)
ON CONFLICT (source, source_key) WHERE source_key IS NOT NULL DO NOTHING
RETURNING id, created_at`

	var (
		id        int64
		createdAt time.Time
	)
	rows, err := client.QueryContext(ctx, insertSQL,
		grant.UserID, grant.GroupID, grant.PlanID, grant.Source, grant.SourceKey,
		grant.BenefitCode, grant.IdentityType, grant.IdentityKey, grant.IdempotencyKey,
		grant.Status, grant.EffectivePolicy, grant.DurationDays, grant.Reason,
		grant.Notes, grant.OperatorUserID,
	)
	if err != nil {
		return false, err
	}
	defer func() { _ = rows.Close() }()

	if !rows.Next() {
		if err := rows.Err(); err != nil {
			return false, err
		}
		// ON CONFLICT DO NOTHING：同 (source, source_key) 已发放过
		return false, nil
	}
	if err := rows.Scan(&id, &createdAt); err != nil {
		return false, err
	}
	grant.ID = id
	grant.CreatedAt = createdAt
	return true, nil
}

func (r *subscriptionGrantRepository) GetByID(ctx context.Context, id int64) (*service.SubscriptionGrant, error) {
	client := clientFromContext(ctx, r.client)
	rows, err := client.QueryContext(ctx,
		`SELECT `+grantColumns+` FROM subscription_grants WHERE id = $1`, id)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()
	if !rows.Next() {
		if err := rows.Err(); err != nil {
			return nil, err
		}
		return nil, service.ErrGrantNotFound
	}
	return scanGrantRow(rows)
}

func (r *subscriptionGrantRepository) GetBySourceKey(ctx context.Context, source, sourceKey string) (*service.SubscriptionGrant, error) {
	client := clientFromContext(ctx, r.client)
	rows, err := client.QueryContext(ctx,
		`SELECT `+grantColumns+` FROM subscription_grants WHERE source = $1 AND source_key = $2`,
		source, sourceKey)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()
	if !rows.Next() {
		if err := rows.Err(); err != nil {
			return nil, err
		}
		return nil, service.ErrGrantNotFound
	}
	return scanGrantRow(rows)
}

func (r *subscriptionGrantRepository) GetForUpdate(ctx context.Context, id int64) (*service.SubscriptionGrant, error) {
	client := clientFromContext(ctx, r.client)
	rows, err := client.QueryContext(ctx,
		`SELECT `+grantColumns+` FROM subscription_grants WHERE id = $1 FOR UPDATE`, id)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()
	if !rows.Next() {
		if err := rows.Err(); err != nil {
			return nil, err
		}
		return nil, service.ErrGrantNotFound
	}
	return scanGrantRow(rows)
}

func (r *subscriptionGrantRepository) UpdateActivation(ctx context.Context, id int64, update service.GrantActivationUpdate) error {
	client := clientFromContext(ctx, r.client)
	res, err := client.ExecContext(ctx, `
UPDATE subscription_grants
SET status = $2, linked_subscription_id = $3, contribution_start = $4,
    contribution_end = $5, activated_at = $6, updated_at = NOW()
WHERE id = $1`, id, update.Status, update.LinkedSubscriptionID,
		update.ContributionStart, update.ContributionEnd, update.ActivatedAt)
	if err != nil {
		return err
	}
	return requireAffectedRows(res, "activate subscription grant")
}

func (r *subscriptionGrantRepository) MarkRevoked(ctx context.Context, id int64, revokedBy int64, reason string) error {
	client := clientFromContext(ctx, r.client)
	res, err := client.ExecContext(ctx, `
UPDATE subscription_grants
SET status = $2, revoked_at = NOW(), revoked_by = $3, revoke_reason = $4, updated_at = NOW()
WHERE id = $1`, id, domainSubscriptionGrantRevoked, revokedBy, reason)
	if err != nil {
		return err
	}
	return requireAffectedRows(res, "revoke subscription grant")
}

func (r *subscriptionGrantRepository) MarkFailed(ctx context.Context, id int64, reason string) error {
	client := clientFromContext(ctx, r.client)
	res, err := client.ExecContext(ctx, `
UPDATE subscription_grants
SET status = $2, failure_reason = $3, updated_at = NOW()
WHERE id = $1`, id, domainSubscriptionGrantFailed, truncateForColumn(reason, 500))
	if err != nil {
		return err
	}
	return requireAffectedRows(res, "fail subscription grant")
}

func (r *subscriptionGrantRepository) ListPending(ctx context.Context, limit int) ([]*service.SubscriptionGrant, error) {
	if limit <= 0 {
		limit = 100
	}
	client := clientFromContext(ctx, r.client)
	rows, err := client.QueryContext(ctx,
		`SELECT `+grantColumns+` FROM subscription_grants WHERE status = $1 ORDER BY created_at ASC, id ASC LIMIT $2`,
		domainSubscriptionGrantPending, limit)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()
	return scanGrantRows(rows)
}

func (r *subscriptionGrantRepository) ExpireFulfilledContribution(ctx context.Context, now time.Time) (int64, error) {
	client := clientFromContext(ctx, r.client)
	res, err := client.ExecContext(ctx, `
UPDATE subscription_grants
SET status = $2, updated_at = NOW()
WHERE status = $3 AND contribution_end IS NOT NULL AND contribution_end <= $1`,
		now, domainSubscriptionGrantExpired, domainSubscriptionGrantFulfilled)
	if err != nil {
		return 0, err
	}
	return res.RowsAffected()
}

func (r *subscriptionGrantRepository) PaidFloorForSubscription(ctx context.Context, subscriptionID int64) (*time.Time, error) {
	client := clientFromContext(ctx, r.client)
	var floor sql.NullTime
	rows, err := client.QueryContext(ctx, `
SELECT MAX(term_end) FROM subscription_terms
WHERE subscription_id = $1 AND source IN ('purchase', 'renewal', 'upgrade')`, subscriptionID)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()
	if !rows.Next() {
		if err := rows.Err(); err != nil {
			return nil, err
		}
		return nil, fmt.Errorf("load paid floor: empty result")
	}
	if err := rows.Scan(&floor); err != nil {
		return nil, err
	}
	if !floor.Valid {
		return nil, nil
	}
	t := floor.Time
	return &t, nil
}

func (r *subscriptionGrantRepository) OtherGrantFloorForSubscription(ctx context.Context, subscriptionID, excludeGrantID int64) (*time.Time, error) {
	client := clientFromContext(ctx, r.client)
	var floor sql.NullTime
	rows, err := client.QueryContext(ctx, `
SELECT MAX(contribution_end) FROM subscription_grants
WHERE linked_subscription_id = $1 AND id <> $2 AND contribution_end IS NOT NULL`,
		subscriptionID, excludeGrantID)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()
	if !rows.Next() {
		if err := rows.Err(); err != nil {
			return nil, err
		}
		return nil, fmt.Errorf("load other grant floor: empty result")
	}
	if err := rows.Scan(&floor); err != nil {
		return nil, err
	}
	if !floor.Valid {
		return nil, nil
	}
	t := floor.Time
	return &t, nil
}

// grantAdminSelectColumns 台账列表查询列：前 26 列与 scanGrantRow 对齐，
// 之后依次追加用户 email/username、操作人 email 与分组名。
const grantAdminSelectColumns = grantColumns + `,
  COALESCE(u.email, ''),
  COALESCE(u.username, ''),
  COALESCE(ou.email, ''),
  COALESCE(g.name, '')`

func (r *subscriptionGrantRepository) AdminList(ctx context.Context, filter *service.GrantAdminFilter) (*service.GrantAdminList, error) {
	if r == nil || r.client == nil {
		return nil, fmt.Errorf("nil subscription grant repository")
	}
	if filter == nil {
		filter = &service.GrantAdminFilter{}
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
	args := make([]any, 0, 6)
	if filter.UserID != nil {
		args = append(args, *filter.UserID)
		clauses = append(clauses, "s.user_id = $"+strconv.Itoa(len(args)))
	}
	if v := strings.TrimSpace(filter.Source); v != "" {
		args = append(args, v)
		clauses = append(clauses, "s.source = $"+strconv.Itoa(len(args)))
	}
	if v := strings.TrimSpace(filter.Status); v != "" {
		args = append(args, v)
		clauses = append(clauses, "s.status = $"+strconv.Itoa(len(args)))
	}
	if v := strings.TrimSpace(filter.BenefitCode); v != "" {
		args = append(args, v)
		clauses = append(clauses, "s.benefit_code = $"+strconv.Itoa(len(args)))
	}
	if filter.GroupID != nil {
		args = append(args, *filter.GroupID)
		clauses = append(clauses, "s.group_id = $"+strconv.Itoa(len(args)))
	}
	where := "WHERE " + strings.Join(clauses, " AND ")

	client := clientFromContext(ctx, r.client)

	var total int64
	countRows, err := client.QueryContext(ctx,
		`SELECT COUNT(*) FROM subscription_grants s `+where, args...)
	if err != nil {
		return nil, err
	}
	if !countRows.Next() {
		err = countRows.Err()
		_ = countRows.Close()
		if err != nil {
			return nil, err
		}
		return nil, fmt.Errorf("count subscription grants: empty result")
	}
	if err := countRows.Scan(&total); err != nil {
		_ = countRows.Close()
		return nil, err
	}
	_ = countRows.Close()

	args = append(args, pageSize, (page-1)*pageSize)
	query := `SELECT ` + grantAdminSelectColumns + `
FROM subscription_grants s
LEFT JOIN users u ON u.id = s.user_id
LEFT JOIN users ou ON ou.id = s.operator_user_id
LEFT JOIN groups g ON g.id = s.group_id
` + where + `
ORDER BY s.created_at DESC, s.id DESC
LIMIT $` + strconv.Itoa(len(args)-1) + ` OFFSET $` + strconv.Itoa(len(args))

	rows, err := client.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()

	items := make([]service.GrantAdminItem, 0)
	for rows.Next() {
		item, err := scanGrantAdminRow(rows)
		if err != nil {
			return nil, err
		}
		items = append(items, *item)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return &service.GrantAdminList{Items: items, Total: int(total), Page: page, PageSize: pageSize}, nil
}

func (r *subscriptionGrantRepository) ListByUser(ctx context.Context, userID int64, limit int) ([]*service.SubscriptionGrant, error) {
	if limit <= 0 {
		limit = 50
	}
	client := clientFromContext(ctx, r.client)
	rows, err := client.QueryContext(ctx,
		`SELECT `+grantColumns+` FROM subscription_grants WHERE user_id = $1 ORDER BY created_at DESC, id DESC LIMIT $2`,
		userID, limit)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()
	return scanGrantRows(rows)
}

func scanGrantRows(rows *sql.Rows) ([]*service.SubscriptionGrant, error) {
	out := make([]*service.SubscriptionGrant, 0)
	for rows.Next() {
		grant, err := scanGrantRow(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, grant)
	}
	return out, rows.Err()
}

func scanGrantAdminRow(row interface{ Scan(dest ...any) error }) (*service.GrantAdminItem, error) {
	var item service.GrantAdminItem
	grant, err := scanGrantRow(row, &item.UserEmail, &item.Username, &item.OperatorEmail, &item.GroupName)
	if err != nil {
		return nil, err
	}
	item.SubscriptionGrant = *grant
	return &item, nil
}

func scanGrantRow(row interface{ Scan(dest ...any) error }, extra ...any) (*service.SubscriptionGrant, error) {
	var (
		grant       service.SubscriptionGrant
		sourceKey   sql.NullString
		benefitCode sql.NullString
		identityTyp sql.NullString
		identityKey sql.NullString
		idemKey     sql.NullString
		reason      sql.NullString
		notes       sql.NullString
		operator    sql.NullInt64
		linkedSub   sql.NullInt64
		contribS    sql.NullTime
		contribE    sql.NullTime
		activatedAt sql.NullTime
		revokedAt   sql.NullTime
		revokedBy   sql.NullInt64
		revokeRsn   sql.NullString
		failureRsn  sql.NullString
		createdAt   time.Time
		updatedAt   time.Time
	)
	dest := []any{
		&grant.ID, &grant.UserID, &grant.GroupID, &grant.PlanID, &grant.Source, &sourceKey,
		&benefitCode, &identityTyp, &identityKey, &idemKey, &grant.Status, &grant.EffectivePolicy,
		&grant.DurationDays, &reason, &notes, &operator, &linkedSub, &contribS, &contribE,
		&activatedAt, &revokedAt, &revokedBy, &revokeRsn, &failureRsn, &createdAt, &updatedAt,
	}
	if err := row.Scan(append(dest, extra...)...); err != nil {
		return nil, err
	}
	grant.SourceKey = nullStringPtr(sourceKey)
	grant.BenefitCode = nullStringPtr(benefitCode)
	grant.IdentityType = nullStringPtr(identityTyp)
	grant.IdentityKey = nullStringPtr(identityKey)
	grant.IdempotencyKey = nullStringPtr(idemKey)
	grant.Reason = nullStringPtr(reason)
	grant.Notes = nullStringPtr(notes)
	grant.OperatorUserID = nullIntPtr(operator)
	grant.LinkedSubscriptionID = nullIntPtr(linkedSub)
	grant.ContributionStart = nullTimePtr(contribS)
	grant.ContributionEnd = nullTimePtr(contribE)
	grant.ActivatedAt = nullTimePtr(activatedAt)
	grant.RevokedAt = nullTimePtr(revokedAt)
	grant.RevokedBy = nullIntPtr(revokedBy)
	grant.RevokeReason = nullStringPtr(revokeRsn)
	grant.FailureReason = nullStringPtr(failureRsn)
	grant.CreatedAt = createdAt
	grant.UpdatedAt = updatedAt
	return &grant, nil
}

// =============================================================================
// Benefit Claim（一次性权益领取）
// =============================================================================

type benefitClaimRepository struct {
	client *dbent.Client
}

func NewBenefitClaimRepository(client *dbent.Client) service.BenefitClaimRepository {
	return &benefitClaimRepository{client: client}
}

func (r *benefitClaimRepository) InsertIdempotent(ctx context.Context, claim *service.BenefitClaim) (bool, error) {
	client := clientFromContext(ctx, r.client)
	const insertSQL = `
INSERT INTO benefit_claims (benefit_code, user_id, identity_type, identity_key, grant_id)
VALUES ($1, $2, $3, $4, $5)
ON CONFLICT DO NOTHING
RETURNING id, created_at`

	var (
		id        int64
		createdAt time.Time
	)
	rows, err := client.QueryContext(ctx, insertSQL,
		claim.BenefitCode, claim.UserID, claim.IdentityType, claim.IdentityKey, claim.GrantID)
	if err != nil {
		return false, err
	}
	defer func() { _ = rows.Close() }()
	if !rows.Next() {
		if err := rows.Err(); err != nil {
			return false, err
		}
		return false, nil
	}
	if err := rows.Scan(&id, &createdAt); err != nil {
		return false, err
	}
	claim.ID = id
	claim.CreatedAt = createdAt
	return true, nil
}

func (r *benefitClaimRepository) GetByBenefitAndIdentity(ctx context.Context, benefitCode, identityKey string) (*service.BenefitClaim, error) {
	client := clientFromContext(ctx, r.client)
	return scanBenefitClaim(client.QueryContext(ctx, `
SELECT id, benefit_code, user_id, identity_type, identity_key, grant_id, created_at
FROM benefit_claims WHERE benefit_code = $1 AND identity_key = $2`, benefitCode, identityKey))
}

func (r *benefitClaimRepository) GetByBenefitAndUser(ctx context.Context, benefitCode string, userID int64) (*service.BenefitClaim, error) {
	client := clientFromContext(ctx, r.client)
	return scanBenefitClaim(client.QueryContext(ctx, `
SELECT id, benefit_code, user_id, identity_type, identity_key, grant_id, created_at
FROM benefit_claims WHERE benefit_code = $1 AND user_id = $2`, benefitCode, userID))
}

func scanBenefitClaim(rows *sql.Rows, err error) (*service.BenefitClaim, error) {
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
	var (
		claim     service.BenefitClaim
		grantID   sql.NullInt64
		createdAt time.Time
	)
	if err := rows.Scan(&claim.ID, &claim.BenefitCode, &claim.UserID, &claim.IdentityType,
		&claim.IdentityKey, &grantID, &createdAt); err != nil {
		return nil, err
	}
	claim.GrantID = nullIntPtr(grantID)
	claim.CreatedAt = createdAt
	return &claim, nil
}

// requireAffectedRows 断言 UPDATE 影响行数 = 1（台账行被并发删除时快速失败）。
func requireAffectedRows(res sql.Result, action string) error {
	affected, err := res.RowsAffected()
	if err != nil {
		return err
	}
	if affected != 1 {
		return fmt.Errorf("%s: unexpected affected rows %d", action, affected)
	}
	return nil
}

func truncateForColumn(s string, max int) string {
	if len(s) <= max {
		return s
	}
	return s[:max]
}

func nullStringPtr(v sql.NullString) *string {
	if !v.Valid {
		return nil
	}
	s := v.String
	return &s
}

func nullIntPtr(v sql.NullInt64) *int64 {
	if !v.Valid {
		return nil
	}
	i := v.Int64
	return &i
}

func nullTimePtr(v sql.NullTime) *time.Time {
	if !v.Valid {
		return nil
	}
	t := v.Time
	return &t
}

// grantStatusAlias* 台账状态的 DB 字面量（subscription_grants.status 列值）。
const (
	domainSubscriptionGrantPending   = "pending"
	domainSubscriptionGrantFulfilled = "fulfilled"
	domainSubscriptionGrantExpired   = "expired"
	domainSubscriptionGrantRevoked   = "revoked"
	domainSubscriptionGrantFailed    = "failed"
)
