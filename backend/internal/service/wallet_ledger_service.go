package service

import (
	"context"
	"fmt"
	"time"

	dbent "github.com/Wei-Shaw/sub2api/ent"
)

// WalletLedgerService 统一钱包流水（Final Frontend CLOSURE：解除 BLOCKED #1/#2）。
//
// 组合既有事实表，不新建账本、不改余额语义：
//   - recharge  充值订单（payment_orders，order_type=balance，COMPLETED）
//   - redeem    兑换码（redeem_codes，status=used）
//   - reward    系统奖励（reward_grants：学生认证/活动等）
//   - payg_day  按量消费日聚合（usage_logs.actual_cost 按天汇总；订阅扣减额度，
//     钱包扣费的真实金额以此为准）
type WalletLedgerService struct {
	entClient  *dbent.Client
	rewardRepo RewardGrantRepository
}

func NewWalletLedgerService(entClient *dbent.Client, rewardRepo RewardGrantRepository) *WalletLedgerService {
	return &WalletLedgerService{entClient: entClient, rewardRepo: rewardRepo}
}

// WalletLedgerEntry 单条流水。
type WalletLedgerEntry struct {
	Type      string    `json:"type"` // recharge | redeem | reward | payg_day
	Amount    float64   `json:"amount"`
	Ref       string    `json:"ref,omitempty"` // 单号 / 兑换码 / 活动 / 日期
	CreatedAt time.Time `json:"created_at"`
}

// ListUserLedger 用户侧钱包流水（各来源限额后合并，created_at 倒序）。
func (s *WalletLedgerService) ListUserLedger(ctx context.Context, userID int64, limit int) ([]WalletLedgerEntry, error) {
	if limit <= 0 {
		limit = 50
	}
	perSource := limit
	entries := make([]WalletLedgerEntry, 0, limit)

	// recharge
	rows, err := s.entClient.QueryContext(ctx, `
SELECT pay_amount, out_trade_no, created_at
FROM payment_orders
WHERE user_id = $1 AND order_type = 'balance' AND status = 'COMPLETED'
ORDER BY created_at DESC LIMIT $2`, userID, perSource)
	if err != nil {
		return nil, fmt.Errorf("ledger recharge: %w", err)
	}
	for rows.Next() {
		var amount float64
		var ref string
		var at time.Time
		if err := rows.Scan(&amount, &ref, &at); err != nil {
			_ = rows.Close()
			return nil, err
		}
		entries = append(entries, WalletLedgerEntry{Type: "recharge", Amount: amount, Ref: ref, CreatedAt: at})
	}
	_ = rows.Close()

	// redeem
	rows, err = s.entClient.QueryContext(ctx, `
SELECT value, code, used_at
FROM redeem_codes
WHERE used_by = $1 AND status = 'used'
ORDER BY used_at DESC LIMIT $2`, userID, perSource)
	if err != nil {
		return nil, fmt.Errorf("ledger redeem: %w", err)
	}
	for rows.Next() {
		var amount float64
		var code string
		var at time.Time
		if err := rows.Scan(&amount, &code, &at); err != nil {
			_ = rows.Close()
			return nil, err
		}
		entries = append(entries, WalletLedgerEntry{Type: "redeem", Amount: amount, Ref: code, CreatedAt: at})
	}
	_ = rows.Close()

	// reward
	grants, err := s.rewardRepo.ListByUser(ctx, userID, perSource)
	if err != nil {
		return nil, fmt.Errorf("ledger reward: %w", err)
	}
	for _, g := range grants {
		ref := g.Campaign
		if ref == "" {
			ref = g.SourceType
		}
		entries = append(entries, WalletLedgerEntry{Type: "reward", Amount: g.Amount, Ref: ref, CreatedAt: g.CreatedAt})
	}

	// payg_day（按量消费日聚合）
	rows, err = s.entClient.QueryContext(ctx, `
SELECT date_trunc('day', created_at) AS d, SUM(actual_cost)
FROM usage_logs
WHERE user_id = $1
GROUP BY d
ORDER BY d DESC LIMIT $2`, userID, perSource)
	if err != nil {
		return nil, fmt.Errorf("ledger payg: %w", err)
	}
	for rows.Next() {
		var day time.Time
		var amount float64
		if err := rows.Scan(&day, &amount); err != nil {
			_ = rows.Close()
			return nil, err
		}
		entries = append(entries, WalletLedgerEntry{
			Type:      "payg_day",
			Amount:    -amount,
			Ref:       day.Format("2006-01-02"),
			CreatedAt: day,
		})
	}
	_ = rows.Close()

	// 合并倒序截断
	for i := 0; i < len(entries); i++ {
		for j := i + 1; j < len(entries); j++ {
			if entries[j].CreatedAt.After(entries[i].CreatedAt) {
				entries[i], entries[j] = entries[j], entries[i]
			}
		}
	}
	if len(entries) > limit {
		entries = entries[:limit]
	}
	return entries, nil
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
