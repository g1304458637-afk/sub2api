package service

// Subscription V1 Phase 2 —— 统一 Weekly Period Re-Anchoring Reset Core 的端口与类型。
//
// 所有 Reset 入口（Admin Manual / Global Reset / Reset Card / Compensation）必须复用
// SubscriptionService.ResetSubscriptionWeeklyPeriod，禁止各自实现 "weekly_usage=0" 逻辑。
// Global Reset worker / Reset Card 消费在后续 Phase 接入；本 Phase 完成
// AdminResetQuota(weekly) 的接线和事件应用（application）原子原语。

import (
	"context"
	"time"

	infraerrors "github.com/Wei-Shaw/sub2api/internal/pkg/errors"
)

// WeeklyResetStatus 表示一次 Reset 调用的最终结局。
type WeeklyResetStatus string

const (
	// WeeklyResetApplied 本次真正执行了 Reset（usage→0，anchor→effectiveAt）。
	WeeklyResetApplied WeeklyResetStatus = "applied"
	// WeeklyResetSkippedStale 当前锚点 >= effectiveAt（含相等，见架构 §33）：
	// 事件驱动场景下绝不回拨用户更新的锚点；usage/anchor 均未改动。
	WeeklyResetSkippedStale WeeklyResetStatus = "skipped_stale"
	// WeeklyResetAlreadyApplied 同一 (event, subscription) 的 application 已存在且
	// 最终态为 applied（worker retry 幂等路径）。
	WeeklyResetAlreadyApplied WeeklyResetStatus = "already_applied"
)

// Reset Core 错误（项目 infraerrors 风格）。
var (
	// ErrResetInvalidEffectiveTime effectiveAt 为零值等非法输入。
	ErrResetInvalidEffectiveTime = infraerrors.BadRequest("RESET_INVALID_EFFECTIVE_TIME", "effective_at is required")
	// ErrResetNotYetEffective effectiveAt 在未来：Reset Core 只执行已生效的 Reset，
	// 未来事件由调度器到点后再调用（V1 不做未来锚点）。
	ErrResetNotYetEffective = infraerrors.BadRequest("RESET_NOT_YET_EFFECTIVE", "reset effective_at is in the future")
)

// WeeklyResetInput 统一 Reset 的输入。
type WeeklyResetInput struct {
	AuditEventID       *int64 // event already claimed by batch executor
	DualWindows        bool   // explicit scope; historical events remain weekly-only
	UserSubscriptionID int64

	// EffectiveAt 由调用方显式传入（必填）：
	//   - Global Reset：事件内全体目标共享的同一时刻
	//   - Reset Card：实际成功消费时刻
	//   - Admin Manual：当前时刻
	// Reset Core 不自行取 time.Now()。
	EffectiveAt time.Time

	// Source 取 domain.WeeklyResetSource* 常量，写入审计 metadata。
	Source string

	// ResetEventID 事件驱动 Reset（Global Reset 等）：填写则在同一事务内
	// claim/finalize subscription_reset_applications（retry-safe + 审计）。
	// 非事件 Reset（Admin/Card）留空，不写 application。
	ResetEventID *int64

	// ResetCardID / ActorID / Metadata 仅进入审计 metadata（V1）。
	ResetCardID *int64
	ActorID     *int64
	Metadata    map[string]any

	// IgnoreLifecycleCheck 跳过 status/expiry 校验。
	// 仅 Admin Manual 兼容路径使用（存量 AdminResetQuota 允许对已过期订阅清零，
	// 外部行为保持不变）；事件/卡片路径默认 false——Reset 不复活订阅。
	IgnoreLifecycleCheck bool
}

// WeeklyResetResult 统一 Reset 的结果。
type WeeklyResetResult struct {
	Status WeeklyResetStatus

	// Subscription 为操作后从 DB 回读的最新快照（applied/skipped 均返回；
	// 仅在订阅存在时可读时非 nil）。
	Subscription *UserSubscription

	// ApplicationStatus 事件驱动时 application 行的最终 status（applied/skipped）。
	ApplicationStatus string
}

// SubscriptionResetApplicationRepository 事件应用记录的原子原语（Phase 4 Global worker 复用）。
type SubscriptionResetApplicationRepository interface {
	// ClaimWeeklyResetApplication 认领 (event, subscription) 应用记录：
	// 不存在则插入（status=claimed 初始态）返回 claimed=true；
	// 已存在则返回 claimed=false 与既有行 status。同事务内 retry-safe。
	ClaimWeeklyResetApplication(ctx context.Context, resetEventID, userSubscriptionID int64, effectiveAt time.Time) (claimed bool, existingStatus string, appID int64, err error)

	// FinalizeClaimedWeeklyResetApplication 在同一事务内回填审计值与最终 status。
	FinalizeClaimedWeeklyResetApplication(ctx context.Context, id int64, previousStart *time.Time, previousUsage *float64, appliedAt time.Time, finalStatus string) error
}
