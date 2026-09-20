package service

// Subscription V1 Phase 4 —— 统一"用户账户状态"数据层（Wallet + Subscription Status）。
//
// Website（GET /api/v1/subscriptions/status）与 MUCODE（GET /v1/usage 增量字段）
// 共用同一个 Builder：百分比、usage_status、周期、钱包语义只有这一份实现，
// 两个端点不允许各自计算。
//
// 领域分离（对齐 Lago/OpenMeter/Kill Bill 的 wallet ≠ entitlement 原则）：
//   - Wallet：users.balance（USD 账本，decimal(20,8)），对用户完全透明；
//   - Subscription：内部 USD 额度（weekly_limit_usd 等）绝不进入普通用户 DTO，
//     用户只消费服务端算好的 weekly_usage_percent（clamp 0..100）与 usage_status；
//   - Admin 端继续使用既有完整 DTO（AdminUserSubscription + Progress），互不影响。
//
// 周期语义：
//   - weekly_period_started_at = weekly_window_start（锚点，未激活时为 null）；
//   - weekly_period_ends_at    = min(锚点+7天, expires_at)——订阅到期优先，
//     最后一个周期可以不足 7 天（Reset Core 的既有不变式）。

import (
	"context"
	"errors"
	"fmt"
	"math"
	"time"
)

// UsageStatus 普通用户视角的用量状态（后端唯一权威定义，客户端不得自行计算）。
type UsageStatus string

const (
	UsageStatusUnmetered UsageStatus = "unmetered"  // 分组未配置 weekly_limit（NULL/0）= 该维度不限
	UsageStatusNormal    UsageStatus = "normal"     // 0–69
	UsageStatusHigh      UsageStatus = "high"       // 70–89
	UsageStatusNearLimit UsageStatus = "near_limit" // 90–99
	UsageStatusExhausted UsageStatus = "exhausted"  // >= 100
)

// SubscriptionWindowMaintainer 窗口惰性维护（自然重置）由 SubscriptionService 提供。
type SubscriptionWindowMaintainer interface {
	EnsureWindowMaintenance(ctx context.Context, sub *UserSubscription) (*UserSubscription, error)
}

// SubscriptionResetCardReader Reset Card 只读统计（发卡/消费 Runtime 未实现；
// Phase 4.1 仅提供账户级可用卡计数，不做任何状态修改）。
type SubscriptionResetCardReader interface {
	// CountAvailableResetCards 统计用户当前可用的 Reset Card 数量。
	// 可用定义：status='available' AND (expires_at IS NULL OR expires_at > now)。
	// 过期边界：expires_at == now 不可用（严格大于）。
	CountAvailableResetCards(ctx context.Context, userID int64, now time.Time) (int, error)
}

// AccountWalletStatus 钱包状态。canonical 账本为 users.balance（USD，NUMERIC(20,8)），
// 金额以 8 位小数字符串表达以保证小数保真；CNY 等展示折算由客户端基于既有汇率逻辑完成。
type AccountWalletStatus struct {
	Balance           string `json:"balance"`
	CanonicalCurrency string `json:"canonical_currency"`
}

// AccountSubscriptionStatus 单条订阅的净化状态（无任何内部 USD 数值）。
type AccountSubscriptionStatus struct {
	ID          int64  `json:"id"`
	GroupID     int64  `json:"group_id"`
	DisplayName string `json:"display_name"`

	// 整数百分比 0..100（floor，raw>=100 钳为 100）；unmetered 时为 null。
	// usage_status 由 raw 分档，与本整数控解耦（99.99 → 99 / near_limit）。
	WeeklyUsagePercent    *int        `json:"weekly_usage_percent"`
	UsageStatus           UsageStatus `json:"usage_status"`
	WeeklyPeriodStartedAt *time.Time  `json:"weekly_period_started_at"` // 未激活（尚无请求）时为 null
	WeeklyPeriodEndsAt    *time.Time  `json:"weekly_period_ends_at"`    // min(锚点+7d, expires_at)；未激活时为 null
	ExpiresAt             time.Time   `json:"expires_at"`
	PaygFallback          bool        `json:"payg_fallback"`
}

// AccountResetCardsStatus 账户级 Reset Card 计数。
// Reset Card 属于 User（未消费前不绑定订阅，Phase 1 定稿）：账户级一份，
// 不随订阅条目重复——避免 Pro+Max 用户看起来"每个套餐各有一张卡"。
type AccountResetCardsStatus struct {
	Available int `json:"available"`
}

// AccountStatus 用户账户统一状态。
type AccountStatus struct {
	Wallet        AccountWalletStatus         `json:"wallet"`
	ResetCards    AccountResetCardsStatus     `json:"reset_cards"`
	Subscriptions []AccountSubscriptionStatus `json:"subscriptions"`
}

// UserDisplayPercent 普通用户展示百分比（整数合同，Phase 4.1 定稿）：
// raw >= 100 → 100；否则 floor(raw)。99.99 → 99（显示 100% 会与"eligibility 尚未
// exhausted"矛盾）；106.5 → 100。usage_status 仍按 raw 分档，不得由本整数反推。
func UserDisplayPercent(rawPercent float64) int {
	if rawPercent >= 100 {
		return 100
	}
	if rawPercent < 0 {
		return 0
	}
	return int(math.Floor(rawPercent))
}

// FormatWalletBalance 把 users.balance（NUMERIC(20,8)）格式化为 8 位小数字符串。
func FormatWalletBalance(balance float64) string {
	return fmt.Sprintf("%.8f", balance)
}

// ClassifyUsageStatus 按原始百分比（可 >100）划档；阈值唯一权威定义在此。
func ClassifyUsageStatus(rawPercent float64) UsageStatus {
	switch {
	case rawPercent >= 100:
		return UsageStatusExhausted
	case rawPercent >= 90:
		return UsageStatusNearLimit
	case rawPercent >= 70:
		return UsageStatusHigh
	default:
		return UsageStatusNormal
	}
}

// ClampUsagePercent 普通用户展示百分比：clamp(raw, 0, 100)。内部可 106.5%，用户只见 100%。
func ClampUsagePercent(rawPercent float64) float64 {
	if rawPercent < 0 {
		return 0
	}
	if rawPercent > 100 {
		return 100
	}
	return rawPercent
}

// AccountStatusService Website 与 MUCODE 共享的状态构建器（服务端唯一权威）。
type AccountStatusService struct {
	userRepo    UserRepository
	subRepo     UserSubscriptionRepository
	groupRepo   GroupRepository
	maintainer  SubscriptionWindowMaintainer
	resetCards  SubscriptionResetCardReader
	monitorOnly bool // true = 只读监控视图（不执行窗口维护写入）
	now         func() time.Time
}

func NewAccountStatusService(
	userRepo UserRepository,
	subRepo UserSubscriptionRepository,
	groupRepo GroupRepository,
	maintainer SubscriptionWindowMaintainer,
	resetCards SubscriptionResetCardReader,
	monitorOnly bool,
) *AccountStatusService {
	return &AccountStatusService{
		userRepo:    userRepo,
		subRepo:     subRepo,
		groupRepo:   groupRepo,
		maintainer:  maintainer,
		resetCards:  resetCards,
		monitorOnly: monitorOnly,
		now:         time.Now,
	}
}

// SetNow 供测试注入时钟。
func (s *AccountStatusService) SetNow(now func() time.Time) { s.now = now }

// CountAvailableResetCards 账户级可用 Reset Card 数（只读；发卡 Runtime 未实现，恒 0 直到入口存在）。
func (s *AccountStatusService) CountAvailableResetCards(ctx context.Context, userID int64) int {
	if s.resetCards == nil {
		return 0
	}
	n, err := s.resetCards.CountAvailableResetCards(ctx, userID, s.now())
	if err != nil {
		// 只读统计失败不应打断状态返回；按 0 处理（与 wallet best-effort 同级）
		return 0
	}
	return n
}

// GetAccountStatus Website 视角：Wallet + 账户级 Reset Cards + 全部 active subscriptions。
func (s *AccountStatusService) GetAccountStatus(ctx context.Context, userID int64) (*AccountStatus, error) {
	user, err := s.userRepo.GetByID(ctx, userID)
	if err != nil {
		return nil, err
	}
	subs, err := s.subRepo.ListActiveByUserID(ctx, userID)
	if err != nil {
		return nil, err
	}
	statuses := make([]AccountSubscriptionStatus, 0, len(subs))
	for i := range subs {
		st, err := s.buildStatus(ctx, &subs[i])
		if err != nil {
			return nil, err
		}
		statuses = append(statuses, *st)
	}
	return &AccountStatus{
		Wallet: AccountWalletStatus{
			Balance:           FormatWalletBalance(user.Balance),
			CanonicalCurrency: "USD",
		},
		ResetCards:    AccountResetCardsStatus{Available: s.CountAvailableResetCards(ctx, userID)},
		Subscriptions: statuses,
	}, nil
}

// GetGroupSubscriptionStatus MUCODE 视角：当前 API Key 所属 Group 的订阅状态。
// 该 Key 对应分组无有效订阅时返回 nil（调用方据此省略字段，PAYG 钱包模式不受影响）。
func (s *AccountStatusService) GetGroupSubscriptionStatus(ctx context.Context, userID, groupID int64) (*AccountSubscriptionStatus, error) {
	sub, err := s.subRepo.GetActiveByUserIDAndGroupID(ctx, userID, groupID)
	if err != nil {
		if errors.Is(err, ErrSubscriptionNotFound) {
			return nil, nil
		}
		return nil, err
	}
	return s.buildStatus(ctx, sub)
}

// GetWallet 仅供仅需要钱包余额的调用方（与 GetAccountStatus 同源同值）。
func (s *AccountStatusService) GetWallet(ctx context.Context, userID int64) (AccountWalletStatus, error) {
	user, err := s.userRepo.GetByID(ctx, userID)
	if err != nil {
		return AccountWalletStatus{}, err
	}
	return AccountWalletStatus{
		Balance:           FormatWalletBalance(user.Balance),
		CanonicalCurrency: "USD",
	}, nil
}

// buildStatus 构建单条订阅的净化状态。先做窗口维护（自然重置）保证读数是
// 当前周期的权威值，再基于分组限额计算百分比与状态。
func (s *AccountStatusService) buildStatus(ctx context.Context, sub *UserSubscription) (*AccountSubscriptionStatus, error) {
	if !s.monitorOnly && s.maintainer != nil {
		refreshed, err := s.maintainer.EnsureWindowMaintenance(ctx, sub)
		if err != nil {
			return nil, err
		}
		sub = refreshed
	}
	group := sub.Group
	if group == nil && s.groupRepo != nil {
		g, err := s.groupRepo.GetByID(ctx, sub.GroupID)
		if err == nil {
			group = g
		}
	}

	st := &AccountSubscriptionStatus{
		ID:           sub.ID,
		GroupID:      sub.GroupID,
		DisplayName:  sub.Group.Name,
		ExpiresAt:    sub.ExpiresAt,
		PaygFallback: sub.AutoPaygFallback,
	}
	if group != nil {
		st.DisplayName = group.Name
	}

	// 周期与百分比：锚点为空 = 尚未激活（还没有任何请求），百分比记 0、周期起点/终点未知
	if sub.WeeklyWindowStart != nil {
		start := *sub.WeeklyWindowStart
		st.WeeklyPeriodStartedAt = &start
		periodEnd := start.Add(7 * 24 * time.Hour)
		// 订阅到期优先：最后一个周期可以不足 7 天（不得展示一个订阅已死之后的"恢复时间"）
		if !sub.ExpiresAt.Before(periodEnd) {
			st.WeeklyPeriodEndsAt = &periodEnd
		} else {
			expires := sub.ExpiresAt
			st.WeeklyPeriodEndsAt = &expires
		}
	}

	if group != nil && group.HasWeeklyLimit() && *group.WeeklyLimitUSD > 0 {
		raw := sub.WeeklyUsageUSD / *group.WeeklyLimitUSD * 100
		display := UserDisplayPercent(raw)
		st.WeeklyUsagePercent = &display
		st.UsageStatus = ClassifyUsageStatus(raw)
	} else {
		// weekly_limit 未配置 = 该维度不受限；不得返回 0% 让用户误读为"有 0 额度"
		st.UsageStatus = UsageStatusUnmetered
	}
	return st, nil
}
