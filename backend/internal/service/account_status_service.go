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
	"time"
)

// UsageStatus 普通用户视角的用量状态（后端唯一权威定义，客户端不得自行计算）。
type UsageStatus string

const (
	UsageStatusUnmetered  UsageStatus = "unmetered"  // 分组未配置 weekly_limit（NULL/0）= 该维度不限
	UsageStatusNormal     UsageStatus = "normal"     // 0–69
	UsageStatusHigh       UsageStatus = "high"       // 70–89
	UsageStatusNearLimit  UsageStatus = "near_limit" // 90–99
	UsageStatusExhausted  UsageStatus = "exhausted"  // >= 100
)

// SubscriptionWindowMaintainer 窗口惰性维护（自然重置）由 SubscriptionService 提供。
type SubscriptionWindowMaintainer interface {
	EnsureWindowMaintenance(ctx context.Context, sub *UserSubscription) (*UserSubscription, error)
}

// AccountWalletStatus 钱包状态。canonical 账本为 users.balance（USD，NUMERIC(20,8)），
// 金额以 8 位小数字符串表达以保证小数保真；CNY 等展示折算由客户端基于既有汇率逻辑完成。
type AccountWalletStatus struct {
	Balance           string `json:"balance"`
	CanonicalCurrency string `json:"canonical_currency"`
}

// AccountSubscriptionStatus 单条订阅的净化状态（无任何内部 USD 数值）。
type AccountSubscriptionStatus struct {
	ID                    int64      `json:"id"`
	GroupID               int64      `json:"group_id"`
	Name                  string     `json:"name"`
	WeeklyUsagePercent    *float64   `json:"weekly_usage_percent"` // clamp 0..100；unmetered 时为 null
	UsageStatus           UsageStatus `json:"usage_status"`
	WeeklyPeriodStartedAt *time.Time `json:"weekly_period_started_at"` // 未激活（尚无请求）时为 null
	WeeklyPeriodEndsAt    *time.Time `json:"weekly_period_ends_at"`    // min(锚点+7d, expires_at)；未激活时为 null
	ExpiresAt             time.Time  `json:"expires_at"`
	PaygFallback          bool       `json:"payg_fallback"`
	// Reset Card Runtime（发卡/消费）尚未实现；Reset Card 表已存在但无任何入口，
	// 因此恒为 0 —— 不伪造可用功能。
	ResetCardsAvailable int `json:"reset_cards_available"`
}

// AccountStatus 用户账户统一状态。
type AccountStatus struct {
	Wallet        AccountWalletStatus         `json:"wallet"`
	Subscriptions []AccountSubscriptionStatus `json:"subscriptions"`
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
	monitorOnly bool // true = 只读监控视图（不执行窗口维护写入）
	now         func() time.Time
}

func NewAccountStatusService(
	userRepo UserRepository,
	subRepo UserSubscriptionRepository,
	groupRepo GroupRepository,
	maintainer SubscriptionWindowMaintainer,
	monitorOnly bool,
) *AccountStatusService {
	return &AccountStatusService{
		userRepo:    userRepo,
		subRepo:     subRepo,
		groupRepo:   groupRepo,
		maintainer:  maintainer,
		monitorOnly: monitorOnly,
		now:         time.Now,
	}
}

// SetNow 供测试注入时钟。
func (s *AccountStatusService) SetNow(now func() time.Time) { s.now = now }

// GetAccountStatus Website 视角：Wallet + 全部 active subscriptions。
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
		ID:          sub.ID,
		GroupID:     sub.GroupID,
		Name:        sub.Group.Name,
		ExpiresAt:   sub.ExpiresAt,
		PaygFallback: sub.AutoPaygFallback,
	}
	if group != nil {
		st.Name = group.Name
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
		display := ClampUsagePercent(raw)
		st.WeeklyUsagePercent = &display
		st.UsageStatus = ClassifyUsageStatus(raw)
	} else {
		// weekly_limit 未配置 = 该维度不受限；不得返回 0% 让用户误读为"有 0 额度"
		st.UsageStatus = UsageStatusUnmetered
	}
	return st, nil
}
