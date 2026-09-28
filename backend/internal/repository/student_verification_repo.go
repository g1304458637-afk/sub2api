package repository

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"

	dbent "github.com/Wei-Shaw/sub2api/ent"
	"github.com/Wei-Shaw/sub2api/internal/service"
)

// studentVerificationRepository student_verifications 表的裸 SQL 实现。
// 认证记录是审计事实（同一用户允许多次认证留痕），不做唯一约束；
// 「一人一号一权益」由 benefit_claims 双唯一约束兜底。
type studentVerificationRepository struct {
	client *dbent.Client
}

func NewStudentVerificationRepository(client *dbent.Client) service.StudentVerificationRepository {
	return &studentVerificationRepository{client: client}
}

const studentVerificationColumns = `
  id, user_id, provider, email, status, benefit_grant_id, ip, user_agent,
  notes, verified_at, revoked_at, revoked_by, created_at`

func (r *studentVerificationRepository) Insert(ctx context.Context, v *service.StudentVerification) (int64, error) {
	client := clientFromContext(ctx, r.client)
	var (
		id        int64
		verifiedAt time.Time
	)
	rows, err := client.QueryContext(ctx, `
INSERT INTO student_verifications (user_id, provider, email, status, benefit_grant_id, ip, user_agent, notes, verified_at)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8, NOW())
RETURNING id, verified_at`,
		v.UserID, v.Provider, v.Email, v.Status, v.BenefitGrantID, v.IP, v.UserAgent, v.Notes,
	)
	if err != nil {
		return 0, err
	}
	defer rows.Close()
	if !rows.Next() {
		if rowsErr := rows.Err(); rowsErr != nil {
			return 0, rowsErr
		}
		return 0, errors.New("student_verifications insert returned no rows")
	}
	if err := rows.Scan(&id, &verifiedAt); err != nil {
		return 0, err
	}
	v.ID = id
	v.VerifiedAt = verifiedAt
	return id, nil
}

func (r *studentVerificationRepository) GetByID(ctx context.Context, id int64) (*service.StudentVerification, error) {
	client := clientFromContext(ctx, r.client)
	rows, err := client.QueryContext(ctx,
		`SELECT `+studentVerificationColumns+` FROM student_verifications WHERE id = $1`, id)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()
	if !rows.Next() {
		if err := rows.Err(); err != nil {
			return nil, err
		}
		return nil, fmt.Errorf("student verification %d not found", id)
	}
	return scanStudentVerification(rows)
}

func (r *studentVerificationRepository) LatestVerifiedByUser(ctx context.Context, userID int64) (*service.StudentVerification, error) {
	client := clientFromContext(ctx, r.client)
	rows, err := client.QueryContext(ctx,
		`SELECT `+studentVerificationColumns+` FROM student_verifications
WHERE user_id = $1 AND status = $2 ORDER BY verified_at DESC, id DESC LIMIT 1`,
		userID, "verified")
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
	return scanStudentVerification(rows)
}

func (r *studentVerificationRepository) LinkBenefitGrant(ctx context.Context, id, grantID int64) error {
	client := clientFromContext(ctx, r.client)
	res, err := client.ExecContext(ctx,
		`UPDATE student_verifications SET benefit_grant_id = $2 WHERE id = $1`, id, grantID)
	if err != nil {
		return err
	}
	affected, err := res.RowsAffected()
	if err != nil {
		return err
	}
	if affected != 1 {
		return fmt.Errorf("link student verification benefit grant: unexpected affected rows %d", affected)
	}
	return nil
}

func (r *studentVerificationRepository) RevokeByUser(ctx context.Context, userID int64, revokedBy int64, reason string) (int64, error) {
	client := clientFromContext(ctx, r.client)
	res, err := client.ExecContext(ctx, `
UPDATE student_verifications
SET status = $3, revoked_at = NOW(), revoked_by = $4, notes = COALESCE(notes, '') || $5
WHERE user_id = $1 AND status = $2`,
		userID, "verified", "revoked", revokedBy, "\n[revoke] "+reason)
	if err != nil {
		return 0, err
	}
	return res.RowsAffected()
}

func (r *studentVerificationRepository) AdminList(ctx context.Context, filter *service.StudentVerificationAdminFilter) (*service.StudentVerificationAdminList, error) {
	if r == nil || r.client == nil {
		return nil, fmt.Errorf("nil student verification repository")
	}
	if filter == nil {
		filter = &service.StudentVerificationAdminFilter{}
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
		clauses = append(clauses, "v.user_id = $"+strconv.Itoa(len(args)))
	}
	if v := strings.TrimSpace(filter.Provider); v != "" {
		args = append(args, v)
		clauses = append(clauses, "v.provider = $"+strconv.Itoa(len(args)))
	}
	if v := strings.TrimSpace(filter.Status); v != "" {
		args = append(args, v)
		clauses = append(clauses, "v.status = $"+strconv.Itoa(len(args)))
	}
	if v := strings.TrimSpace(filter.Email); v != "" {
		args = append(args, strings.ToLower(v)+"%")
		clauses = append(clauses, "v.email LIKE $"+strconv.Itoa(len(args)))
	}
	where := "WHERE " + strings.Join(clauses, " AND ")

	client := clientFromContext(ctx, r.client)

	var total int64
	countRows, err := client.QueryContext(ctx,
		`SELECT COUNT(*) FROM student_verifications v `+where, args...)
	if err != nil {
		return nil, err
	}
	if !countRows.Next() {
		err = countRows.Err()
		_ = countRows.Close()
		if err != nil {
			return nil, err
		}
		return nil, fmt.Errorf("count student verifications: empty result")
	}
	if err := countRows.Scan(&total); err != nil {
		_ = countRows.Close()
		return nil, err
	}
	_ = countRows.Close()

	args = append(args, pageSize, (page-1)*pageSize)
	query := `SELECT
  v.id, v.user_id, v.provider, v.email, v.status, v.benefit_grant_id, v.ip, v.user_agent,
  v.notes, v.verified_at, v.revoked_at, v.revoked_by, v.created_at,
  COALESCE(u.email, ''), COALESCE(u.username, '')
FROM student_verifications v
LEFT JOIN users u ON u.id = v.user_id
` + where + `
ORDER BY v.created_at DESC, v.id DESC
LIMIT $` + strconv.Itoa(len(args)-1) + ` OFFSET $` + strconv.Itoa(len(args))

	rows, err := client.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()

	items := make([]service.StudentVerificationAdminItem, 0)
	for rows.Next() {
		var item service.StudentVerificationAdminItem
		v, err := scanStudentVerification(rows, &item.UserEmail, &item.Username)
		if err != nil {
			return nil, err
		}
		item.StudentVerification = *v
		items = append(items, item)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return &service.StudentVerificationAdminList{Items: items, Total: int(total), Page: page, PageSize: pageSize}, nil
}

func scanStudentVerification(row interface{ Scan(dest ...any) error }, extra ...any) (*service.StudentVerification, error) {
	var (
		v          service.StudentVerification
		benefitGID sql.NullInt64
		notes      sql.NullString
		revokedAt  sql.NullTime
		revokedBy  sql.NullInt64
		verifiedAt time.Time
		createdAt  time.Time
	)
	dest := []any{&v.ID, &v.UserID, &v.Provider, &v.Email, &v.Status, &benefitGID,
		&v.IP, &v.UserAgent, &notes, &verifiedAt, &revokedAt, &revokedBy, &createdAt}
	if err := row.Scan(append(dest, extra...)...); err != nil {
		return nil, err
	}
	v.BenefitGrantID = nullIntPtr(benefitGID)
	v.Notes = nullStringPtr(notes)
	v.RevokedAt = nullTimePtr(revokedAt)
	v.RevokedBy = nullIntPtr(revokedBy)
	v.VerifiedAt = verifiedAt
	v.CreatedAt = createdAt
	return &v, nil
}
