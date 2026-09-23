package service

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	dbent "github.com/Wei-Shaw/sub2api/ent"
)

// WalletLedgerService reads wallet movements from their authoritative sources.
// Subscription entitlements and non-balance redemption never enter this view.
type WalletLedgerService struct {
	entClient  *dbent.Client
	rewardRepo RewardGrantRepository
}

func NewWalletLedgerService(entClient *dbent.Client, rewardRepo RewardGrantRepository) *WalletLedgerService {
	return &WalletLedgerService{entClient: entClient, rewardRepo: rewardRepo}
}

// WalletLedgerEntry is a signed balance movement with an explicit source currency.
type WalletLedgerEntry struct {
	ID        string    `json:"id"`
	Type      string    `json:"type"`
	Amount    float64   `json:"amount"`
	Currency  string    `json:"currency"`
	Ref       string    `json:"ref,omitempty"`
	CreatedAt time.Time `json:"created_at"`
}

// All sources are combined before pagination; tied timestamps use source IDs.
// Refund amounts use the recorded wallet deduction, never the gateway amount.
const walletLedgerSources = `WITH cutover AS (
 SELECT COALESCE((SELECT NULLIF(value, '')::timestamptz FROM settings WHERE key = 'wallet_currency_cutover_at'), 'epoch'::timestamptz) AS at
), entries AS (
 SELECT 'redeem:' || r.id AS id,
 CASE WHEN r.type = 'admin_balance' THEN 'manual_adjustment'
      WHEN EXISTS (SELECT 1 FROM payment_orders p WHERE p.recharge_code = r.code AND p.user_id = r.used_by AND p.order_type = 'balance') THEN 'wallet_recharge'
      ELSE 'redeem_balance' END AS type,
 r.value AS amount, 'CNY' AS currency, r.code AS ref, r.used_at AS created_at
 FROM redeem_codes r
 CROSS JOIN cutover c
 WHERE r.used_by = $1 AND r.status = 'used' AND r.used_at IS NOT NULL
   AND r.type IN ('balance', 'admin_balance') AND r.value <> 0
 UNION ALL
 SELECT 'reward:' || r.id, 'reward', r.amount, 'CNY', COALESCE(NULLIF(r.campaign, ''), r.source_type), r.created_at
 FROM reward_grants r CROSS JOIN cutover c WHERE r.user_id = $1 AND r.amount <> 0
 UNION ALL
 SELECT 'affiliate:' || a.id, 'reward', a.amount, 'CNY', 'affiliate_transfer', a.created_at
 FROM user_affiliate_ledger a CROSS JOIN cutover c WHERE a.user_id = $1 AND a.action = 'transfer' AND a.amount <> 0
 UNION ALL
 SELECT 'usage:' || to_char(u.created_at AT TIME ZONE 'UTC', 'YYYY-MM-DD'), 'payg_usage',
 -SUM(u.actual_cost * 6.7), 'CNY',
 to_char(u.created_at AT TIME ZONE 'UTC', 'YYYY-MM-DD'), MAX(u.created_at)
 FROM usage_logs u CROSS JOIN cutover c WHERE u.user_id = $1 AND u.billing_type = 0 AND u.actual_cost > 0
 GROUP BY to_char(u.created_at AT TIME ZONE 'UTC', 'YYYY-MM-DD')
 UNION ALL
 SELECT 'refund:' || a.id, 'refund', -((a.detail::jsonb->>'balanceDeducted')::numeric * CASE WHEN a.created_at < c.at THEN 6.7 ELSE 1 END), 'CNY',
 p.id::text, a.created_at
 FROM payment_audit_logs a JOIN payment_orders p ON p.id::text = a.order_id
 CROSS JOIN cutover c
 WHERE p.user_id = $1 AND p.order_type = 'balance' AND a.action = 'REFUND_SUCCESS'
   AND (a.detail::jsonb->>'balanceDeducted')::numeric > 0
)`

func (s *WalletLedgerService) ListUserLedger(ctx context.Context, userID int64, limit, offset int) ([]WalletLedgerEntry, int64, error) {
	if limit <= 0 || limit > 100 {
		limit = 50
	}
	if offset < 0 {
		offset = 0
	}
	rows, err := s.entClient.QueryContext(ctx, walletLedgerSources+`,
 page AS (SELECT * FROM entries ORDER BY created_at DESC, id DESC LIMIT $2 OFFSET $3)
 SELECT totals.total, page.id, page.type, page.amount, page.currency, page.ref, page.created_at
 FROM (SELECT COUNT(*) AS total FROM entries) totals LEFT JOIN page ON TRUE
 ORDER BY page.created_at DESC, page.id DESC`, userID, limit, offset)
	if err != nil {
		return nil, 0, fmt.Errorf("wallet ledger: %w", err)
	}
	defer func() { _ = rows.Close() }()
	entries := make([]WalletLedgerEntry, 0, limit)
	var total int64
	for rows.Next() {
		var id, typ, currency, ref sql.NullString
		var amount sql.NullFloat64
		var at sql.NullTime
		if err := rows.Scan(&total, &id, &typ, &amount, &currency, &ref, &at); err != nil {
			return nil, 0, err
		}
		if id.Valid {
			entries = append(entries, WalletLedgerEntry{ID: id.String, Type: typ.String, Amount: amount.Float64, Currency: currency.String, Ref: ref.String, CreatedAt: at.Time})
		}
	}
	return entries, total, rows.Err()
}

// RewardGrantView 管理端发放记录视图。
type RewardGrantView struct {
	ID         int64     `json:"id"`
	UserID     int64     `json:"user_id"`
	SourceType string    `json:"source_type"`
	Campaign   string    `json:"campaign"`
	Amount     float64   `json:"amount"`
	GrantedBy  *int64    `json:"granted_by,omitempty"`
	CreatedAt  time.Time `json:"created_at"`
}

// AdminRewardList 管理端发放记录（分页）。
func (s *WalletLedgerService) AdminRewardList(ctx context.Context, userID *int64, limit, offset int) ([]RewardGrantView, int64, error) {
	grants, total, err := s.rewardRepo.ListAll(ctx, userID, limit, offset)
	if err != nil {
		return nil, 0, err
	}
	out := make([]RewardGrantView, 0, len(grants))
	for _, g := range grants {
		out = append(out, RewardGrantView{
			ID: g.ID, UserID: g.UserID, SourceType: g.SourceType,
			Campaign: g.Campaign, Amount: g.Amount, GrantedBy: g.GrantedBy, CreatedAt: g.CreatedAt,
		})
	}
	return out, total, nil
}

// RewardStats 发放统计。
type RewardStats struct {
	Count int64   `json:"count"`
	Sum   float64 `json:"sum"`
}

// DayStats 今日/本月发放统计（userID 为 nil 时全局）。
func (s *WalletLedgerService) RewardDayStats(ctx context.Context, userID *int64, now time.Time) (today RewardStats, month RewardStats, err error) {
	todayStart := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, now.Location())
	monthStart := time.Date(now.Year(), now.Month(), 1, 0, 0, 0, 0, now.Location())
	today.Count, today.Sum, err = s.rewardRepo.StatsRange(ctx, userID, todayStart, todayStart.AddDate(0, 0, 1))
	if err != nil {
		return today, month, err
	}
	month.Count, month.Sum, err = s.rewardRepo.StatsRange(ctx, userID, monthStart, monthStart.AddDate(0, 1, 0))
	return today, month, err
}
