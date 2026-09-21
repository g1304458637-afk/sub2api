package service

import (
	"context"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/pkg/pagination"
)

type UserSubscriptionRepository interface {
	Create(ctx context.Context, sub *UserSubscription) error
	GetByID(ctx context.Context, id int64) (*UserSubscription, error)
	GetByIDForUpdate(ctx context.Context, id int64) (*UserSubscription, error)
	GetByIDIncludeDeleted(ctx context.Context, id int64) (*UserSubscription, error)
	GetByUserIDAndGroupID(ctx context.Context, userID, groupID int64) (*UserSubscription, error)
	GetActiveByUserIDAndGroupID(ctx context.Context, userID, groupID int64) (*UserSubscription, error)
	Update(ctx context.Context, sub *UserSubscription) error
	Delete(ctx context.Context, id int64) error
	Restore(ctx context.Context, subscriptionID int64, restoredStatus string) (*UserSubscription, error)

	ListByUserID(ctx context.Context, userID int64) ([]UserSubscription, error)
	ListActiveByUserID(ctx context.Context, userID int64) ([]UserSubscription, error)
	ListByGroupID(ctx context.Context, groupID int64, params pagination.PaginationParams) ([]UserSubscription, *pagination.PaginationResult, error)
	List(ctx context.Context, params pagination.PaginationParams, userID, groupID *int64, status, platform, sortBy, sortOrder string) ([]UserSubscription, *pagination.PaginationResult, error)

	ExistsByUserIDAndGroupID(ctx context.Context, userID, groupID int64) (bool, error)
	ExistsActiveByUserIDAndGroupID(ctx context.Context, userID, groupID int64) (bool, error)
	ExtendExpiry(ctx context.Context, subscriptionID int64, newExpiresAt time.Time) error
	UpdateStatus(ctx context.Context, subscriptionID int64, status string) error
	UpdateNotes(ctx context.Context, subscriptionID int64, notes string) error

	// ActivateWindows 首次使用时激活用量窗口。日窗口按日历日对齐，锚点为当天 0 点
	// （dailyStart）；周/月窗口为期限对齐滚动窗口，锚点为激活时刻（periodicStart）。
	// 仅当三个窗口均未激活时生效。
	ActivateWindows(ctx context.Context, id int64, dailyStart, periodicStart time.Time) error
	// ResetUsageWindows 手动重置所选窗口的用量。日窗口锚点写入 dailyStart（当天 0 点，
	// 保持 0 点刷新节奏不漂移）；周/月窗口锚点写入 periodicStart（重置时刻）。
	ResetUsageWindows(ctx context.Context, id int64, resetDaily, resetWeekly, resetMonthly bool, dailyStart, periodicStart time.Time) error
	ResetDailyUsage(ctx context.Context, id int64, expectedWindowStart *time.Time, newWindowStart time.Time) error
	ResetWeeklyUsage(ctx context.Context, id int64, expectedWindowStart *time.Time, newWindowStart time.Time) error
	ResetMonthlyUsage(ctx context.Context, id int64, expectedWindowStart *time.Time, newWindowStart time.Time) error
	IncrementUsage(ctx context.Context, id int64, costUSD float64) error

	BatchUpdateExpiredStatus(ctx context.Context) (int64, error)
}

// SubscriptionPaygFallbackStore 是 UserSubscriptionRepository 的可选能力：
// 更新用户级 PAYG fallback 开关（生产 ent 仓储实现；测试 stub 可不实现）。
type SubscriptionPaygFallbackStore interface {
	UpdatePaygFallback(ctx context.Context, id int64, enabled bool) error
}

// UserScheduledChangeCloser 是用户级 pending 变更批量取消的可选仓储能力
// （支付履约闭包用；生产 ent 仓储实现）。
type UserScheduledChangeCloser interface {
	// CancelScheduledForUser 取消该用户全部 scheduled_downgrade 审计行，返回条数。
	CancelScheduledForUser(ctx context.Context, userID int64, reason string) (int, error)
}

// SubscriptionLatestRowReader 是用户级"最近一条订阅行（任意状态）"的可选仓储能力：
// 预付费固定周期制下，next_plan_id 挂在行上且到期后必须保留（ACTIVE=0 合法），
// 状态合同需要用户级读取（生产 ent 仓储实现；测试 stub 可不实现）。
type SubscriptionLatestRowReader interface {
	// FindLatestByUserID 返回用户最近一条未删除订阅行（expires_at DESC, id DESC）。
	FindLatestByUserID(ctx context.Context, userID int64) (*UserSubscription, error)
}

// SubscriptionSingleActiveGuard 是单主套餐不变量（产品 RULE 1）的可选仓储能力：
// 生产 ent 仓储实现；测试 stub 未实现时守卫跳过，由数据库 partial unique index
// （迁移 242）兜底。接口化是为了不破坏既有 UserSubscriptionRepository 全量 stub。
type SubscriptionSingleActiveGuard interface {
	// FindActiveByUserIDExcludingGroup 返回用户在目标组之外的任一 ACTIVE 订阅（无则 nil）。
	FindActiveByUserIDExcludingGroup(ctx context.Context, userID, groupID int64) (*UserSubscription, error)
	// ExpireLapsedByUser 把用户 status=active 但已过 expires_at 的订阅翻为 expired。
	ExpireLapsedByUser(ctx context.Context, userID int64, now time.Time) (int64, error)
}

// ScheduledChangeSuperseder 支付履约成功后的用户级"下次续费偏好"清理
// （PlanChangeService 实现，wire 注入）。Phase 11B：付费续费可能开新行
// （跨档续费），指针必须按用户清理而非按订阅行。必须在支付履约事务内调用。
type ScheduledChangeSuperseder interface {
	SupersedeScheduledChangeForUser(ctx context.Context, userID int64) error
}

// SubscriptionConcurrencyOverrideReader 是 UserSubscriptionRepository 的可选能力：
// 用户全部 active+metered 订阅分组的 concurrency_override 最大值（Phase 9 并发权益）。
type SubscriptionConcurrencyOverrideReader interface {
	GetMaxActiveGroupConcurrencyOverride(ctx context.Context, userID int64) (int, error)
}
