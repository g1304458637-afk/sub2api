package service

// Subscription V1 Phase 7 —— Scoped / Batch Direct Reset Runtime。
//
// 产品语义（补充定稿）：
//   Direct Reset = 管理员立即替目标订阅执行 Reset（不产生/不消费 Reset Card、
//   不动 Wallet、不延长订阅）。Global Reset 只是 target_mode=all_active 的一种。
//
//   - Snapshot：事件创建即解析 selector → 固定 subscription_ids（applications 行）；
//     之后新建的订阅不会被旧事件波及。
//   - effective_at：创建时生成一次，全体目标共享；worker 处理时刻不参与。
//   - Stale safety：锚点 >= effective_at → skipped（Reset Card 永远赢过旧 batch）。
//   - 幂等：创建走 durable Idempotency-Key（重试返回同一事件）；
//     (event, subscription) 唯一约束保证 application 至多一次。
//   - Worker：DB 即 job store（applications = 任务）；FOR UPDATE SKIP LOCKED 认领，
//     单订阅单事务（认领 + Reset Core + 终态），崩溃自动回到 pending。
//
// Reference Decision（Phase 7.0）：
//   - graphile-worker / PgBoss 的 SKIP LOCKED job claim 模式（DB 即队列，无第二套
//     job system——复用仓库既有 ent/tx/raw SQL 基础设施）。
//   - OpenMeter 的 entitlement batch 重算 + Lago 的 batch invoice finalize：
//     批处理 + 状态计数 + 可重试失败的作业设计。
//   - 不采用：Redis 队列/外部调度器/内存 job registry。

import (
	"context"
	"errors"
	"fmt"
	"time"

	dbent "github.com/Wei-Shaw/sub2api/ent"
	"github.com/Wei-Shaw/sub2api/internal/domain"
	infraerrors "github.com/Wei-Shaw/sub2api/internal/pkg/errors"
)

// Direct Reset 事件业务错误。
var (
	ErrResetEventNotFound   = infraerrors.NotFound("RESET_EVENT_NOT_FOUND", "reset event not found")
	ErrResetEventNotDue     = infraerrors.BadRequest("RESET_EVENT_NOT_DUE", "event effective_at is in the future")
	ErrResetEventNotRunning = infraerrors.Conflict("RESET_EVENT_NOT_RUNNING", "event is not in a retryable state")
)

// DirectResetSelector 定向选择器（snapshot 解析为 subscription_ids）。
type DirectResetSelector struct {
	// target_mode: subscription_ids / users / groups / all_active
	TargetMode      string  `json:"target_mode"`
	SubscriptionIDs []int64 `json:"subscription_ids,omitempty"`
	UserIDs         []int64 `json:"user_ids,omitempty"`
	GroupIDs        []int64 `json:"group_ids,omitempty"`
}

// CreateResetEventInput 创建 Direct Reset 事件。
type CreateResetEventInput struct {
	Selector       DirectResetSelector
	EffectiveAt    *time.Time // nil = 立即（now）
	Reason         string
	IdempotencyKey string
	ActorAdminID   int64
}

// ResetEventSummary 事件概要（含进度统计）。
type ResetEventSummary struct {
	ID            int64      `json:"id"`
	Status        string     `json:"status"`
	TargetMode    string     `json:"target_mode"`
	EffectiveAt   time.Time  `json:"effective_at"`
	Reason        string     `json:"reason,omitempty"`
	TotalTargeted int64      `json:"total_targeted"`
	AppliedCount  int64      `json:"applied_count"`
	SkippedCount  int64      `json:"skipped_count"`
	FailedCount   int64      `json:"failed_count"`
	PendingCount  int64      `json:"pending_count"`
	CreatedAt     time.Time  `json:"created_at"`
	StartedAt     *time.Time `json:"started_at,omitempty"`
	CompletedAt   *time.Time `json:"completed_at,omitempty"`
}

// ResetEventStore Direct Reset 事件持久化端口。
type ResetEventStore interface {
	// CreateEventWithApplications 创建事件 + snapshot applications（单事务）。
	CreateEventWithApplications(ctx context.Context, ev *ResetEventRecord, applications []int64) (int64, error)
	// GetDueEventIDs 取到期事件（pending / running）。
	GetDueEventIDs(ctx context.Context, now time.Time, limit int) ([]int64, error)
	// ClaimRunning 事件 → running（首次置 started_at）。
	ClaimRunning(ctx context.Context, eventID int64) error
	// GetEvent 读取事件。
	GetEvent(ctx context.Context, eventID int64) (*ResetEventRecord, error)
	// ListEvents 分页列出事件。
	ListEvents(ctx context.Context, limit, offset int) ([]*ResetEventRecord, error)
	// ApplicationStats 按状态统计。
	ApplicationStats(ctx context.Context, eventID int64) (map[string]int64, error)
	// ClaimApplicationBatch 认领一批 pending application（SKIP LOCKED，原子）。
	ClaimApplicationBatch(ctx context.Context, eventID int64, limit int) ([]ResetApplicationClaim, error)
	// GetApplicationForUpdate 锁定 application 行（同事务内）。
	GetApplicationForUpdate(ctx context.Context, appID int64) (*ResetApplicationRecord, error)
	// FinalizeApplication 落终态与审计值。
	FinalizeApplication(ctx context.Context, appID int64, prevStart *time.Time, prevUsage *float64, finalStatus string) error
	// MarkApplicationFailed 独立事务的失败落账（attempts++）。
	MarkApplicationFailed(ctx context.Context, appID int64, reason string) error
	// RequeueFailed failed → pending（retry），返回条数。
	RequeueFailed(ctx context.Context, eventID int64) (int64, error)
	// CompleteIfDrained 全部终态时收口事件状态。
	CompleteIfDrained(ctx context.Context, eventID int64) (finalStatus string, err error)
}

// ResetEventRecord 事件行。
type ResetEventRecord struct {
	ID            int64
	EventType     string
	Status        string
	EffectiveAt   time.Time
	ScopeType     string
	Scope         map[string]any
	Reason        string
	TotalTargeted int64
	StartedAt     *time.Time
	CompletedAt   *time.Time
	CreatedAt     time.Time
	UpdatedAt     time.Time
}

// ResetApplicationClaim 认领结果。
type ResetApplicationClaim struct {
	ApplicationID      int64
	UserSubscriptionID int64
}

// ResetApplicationRecord application 行（审计）。
type ResetApplicationRecord struct {
	ID                        int64
	ResetEventID              int64
	UserSubscriptionID        int64
	EffectiveAt               time.Time
	PreviousWeeklyWindowStart *time.Time
	PreviousWeeklyUsageUSD    *float64
	AppliedAt                 time.Time
	Status                    string
	Attempts                  int
}

// ResetEventService Direct Reset Runtime。
type ResetEventService struct {
	store     ResetEventStore
	targets   ResetTargetResolver
	resetCore WeeklyResetCore
	subRepo   UserSubscriptionRepository
	entClient *dbent.Client
	now       func() time.Time
	batchSize int
}

func NewResetEventService(
	store ResetEventStore,
	targets ResetTargetResolver,
	resetCore WeeklyResetCore,
	subRepo UserSubscriptionRepository,
	entClient *dbent.Client,
) *ResetEventService {
	return &ResetEventService{
		store:     store,
		targets:   targets,
		resetCore: resetCore,
		subRepo:   subRepo,
		entClient: entClient,
		now:       time.Now,
		batchSize: 50,
	}
}

// SetNow 供测试注入时钟。
func (s *ResetEventService) SetNow(now func() time.Time) { s.now = now }

// StartWorker 启动 Direct Reset 事件后台 worker（30s 默认节奏；生产由 wire 调用）。
// 崩溃安全：application 认领/执行单事务，崩溃自动回到 pending。
func (s *ResetEventService) StartWorker(ctx context.Context, interval time.Duration) {
	if interval <= 0 {
		interval = 30 * time.Second
	}
	go func() {
		defer func() {
			if r := recover(); r != nil {
				fmt.Printf("reset event worker panic: %v\n", r)
			}
		}()
		ticker := time.NewTicker(interval)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				_, _ = s.ProcessDueEvents(ctx)
			}
		}
	}()
}

// CreateResetEvent 创建 Direct Reset 事件（snapshot + durable 幂等）。
func (s *ResetEventService) CreateResetEvent(ctx context.Context, in *CreateResetEventInput) (*ResetEventSummary, error) {
	if in == nil || in.Selector.TargetMode == "" {
		return nil, ErrResetSelectorInvalid
	}
	if in.IdempotencyKey == "" {
		return nil, ErrIdempotencyKeyRequired
	}
	coordinator := DefaultIdempotencyCoordinator()
	if coordinator == nil {
		return nil, errors.New("reset event: idempotency coordinator unavailable")
	}
	execRes, err := coordinator.Execute(ctx, IdempotencyExecuteOptions{
		Scope:          "admin.subscription_reset.create",
		ActorScope:     fmt.Sprintf("admin:%d", in.ActorAdminID),
		Method:         "POST",
		Route:          "/admin/subscription-resets",
		IdempotencyKey: in.IdempotencyKey,
		Payload:        map[string]any{"selector": in.Selector, "effective_at": in.EffectiveAt, "reason": in.Reason},
		RequireKey:     true,
	}, func(ctx context.Context) (any, error) {
		return s.createOnce(ctx, in)
	})
	if err != nil {
		return nil, err
	}
	return decodeSubscriptionOperationResult[ResetEventSummary](execRes.Data)
}

func (s *ResetEventService) createOnce(ctx context.Context, in *CreateResetEventInput) (*ResetEventSummary, error) {
	if s.store == nil || s.targets == nil {
		return nil, errors.New("reset event: storage unavailable")
	}
	effectiveAt := s.now()
	if in.EffectiveAt != nil {
		effectiveAt = *in.EffectiveAt
	}
	// subscription_ids 模式：user_ids 位携带显式订阅 id（resolver 统一按 target_mode 解释）
	// 参数按 target_mode 语义传位：users → UserIDs；groups → GroupIDs；
	// subscription_ids → SubscriptionIDs（占 userIDs 位，resolver 按 mode 解释）
	// resolver 签名 (mode, userIDs, groupIDs)：users 模式传 UserIDs；
	// subscription_ids 模式把显式订阅 id 经 userIDs 位传入（resolver 按 mode 解释）。
	userArg := in.Selector.UserIDs
	if in.Selector.TargetMode == domain.ResetTargetModeSubscriptionIDs {
		userArg = in.Selector.SubscriptionIDs
	}
	subIDs, err := s.targets.ResolveActiveMeteredSubscriptionIDs(
		ctx, in.Selector.TargetMode, userArg, in.Selector.GroupIDs)
	if err != nil {
		return nil, err
	}
	if len(subIDs) == 0 {
		return nil, ErrResetTargetEmpty
	}
	eventID, err := s.store.CreateEventWithApplications(ctx, &ResetEventRecord{
		EventType:     domain.ResetEventTypeGlobalReset,
		Status:        domain.ResetEventStatusPending,
		EffectiveAt:   effectiveAt,
		ScopeType:     directScopeTypeFor(in.Selector.TargetMode),
		Scope:         directScopeJSON(in.Selector),
		Reason:        in.Reason,
		TotalTargeted: int64(len(subIDs)),
	}, subIDs)
	if err != nil {
		return nil, err
	}
	summary, err := s.GetResetEvent(ctx, eventID)
	if err != nil {
		return nil, err
	}
	return summary, nil
}

// PreviewResetTargets Direct Reset 预览（无任何写入）。
func (s *ResetEventService) PreviewResetTargets(ctx context.Context, selector DirectResetSelector) (*ResetTargetSummary, error) {
	// users 模式按 user_ids 过滤、subscription_ids 模式按订阅 id 过滤：
	// repo 层 DescribeTargets 的第二个参数语义是「订阅 id / 用户 id（依 mode 而定）」，
	// 这里按 mode 选择正确的 id 集合（修复 users 模式预览恒报 user_ids required 的问题）。
	idsForTarget := selector.UserIDs
	if selector.TargetMode == domain.ResetTargetModeSubscriptionIDs {
		idsForTarget = selector.SubscriptionIDs
	}
	summary, err := s.targets.DescribeTargets(ctx, selector.TargetMode, idsForTarget, selector.GroupIDs)
	if err != nil {
		return nil, err
	}
	// users 模式的 user_ids 语义
	if selector.TargetMode == domain.ResetTargetModeUsers {
		summary.TargetMode = selector.TargetMode
	} else {
		summary.TargetMode = selector.TargetMode
	}
	return summary, nil
}

// GetResetEvent 事件 + 进度统计。
func (s *ResetEventService) GetResetEvent(ctx context.Context, eventID int64) (*ResetEventSummary, error) {
	ev, err := s.store.GetEvent(ctx, eventID)
	if err != nil {
		return nil, err
	}
	stats, err := s.store.ApplicationStats(ctx, eventID)
	if err != nil {
		return nil, err
	}
	count := func(status string) int64 { return stats[status] }
	return &ResetEventSummary{
		ID:          ev.ID,
		Status:      ev.Status,
		EffectiveAt: ev.EffectiveAt,
		Reason:      ev.Reason,
		TotalTargeted: count(domain.ResetApplicationStatusApplied) + count(domain.ResetApplicationStatusSkipped) +
			count(domain.ResetApplicationStatusFailed) + count(domain.ResetApplicationStatusPending) +
			count(domain.ResetApplicationStatusApplying),
		AppliedCount: count(domain.ResetApplicationStatusApplied),
		SkippedCount: count(domain.ResetApplicationStatusSkipped),
		FailedCount:  count(domain.ResetApplicationStatusFailed),
		PendingCount: count(domain.ResetApplicationStatusPending) + count(domain.ResetApplicationStatusApplying),
		CreatedAt:    ev.CreatedAt,
		StartedAt:    ev.StartedAt,
		CompletedAt:  ev.CompletedAt,
	}, nil
}

// ListResetEvents 事件列表（含统计）。
func (s *ResetEventService) ListResetEvents(ctx context.Context, limit, offset int) ([]ResetEventSummary, error) {
	if limit <= 0 {
		limit = 50
	}
	evs, err := s.store.ListEvents(ctx, limit, offset)
	if err != nil {
		return nil, err
	}
	out := make([]ResetEventSummary, 0, len(evs))
	for _, ev := range evs {
		summary, err := s.GetResetEvent(ctx, ev.ID)
		if err != nil {
			return nil, err
		}
		out = append(out, *summary)
	}
	return out, nil
}

// RetryFailedEvents failed application → pending；事件回 running 由 worker 认领。
func (s *ResetEventService) RetryFailedApplications(ctx context.Context, eventID int64) (int64, error) {
	return s.store.RequeueFailed(ctx, eventID)
}

// ProcessDueEvents worker 主循环一步：认领到期事件 → 批量处理 applications。
// 返回本次处理的 application 数（0 = 无到期工作）。
func (s *ResetEventService) ProcessDueEvents(ctx context.Context) (int, error) {
	if s.store == nil || s.entClient == nil {
		return 0, errors.New("reset event: storage unavailable")
	}
	now := s.now()
	eventIDs, err := s.store.GetDueEventIDs(ctx, now, 10)
	if err != nil {
		return 0, err
	}
	processed := 0
	for _, eventID := range eventIDs {
		ev, err := s.store.GetEvent(ctx, eventID)
		if err != nil {
			continue
		}
		if err := s.store.ClaimRunning(ctx, eventID); err != nil {
			continue
		}
		for {
			claims, err := s.store.ClaimApplicationBatch(ctx, eventID, s.batchSize)
			if err != nil {
				break
			}
			if len(claims) == 0 {
				break
			}
			for _, claim := range claims {
				if err := s.applyOne(ctx, eventID, ev.EffectiveAt, claim); err != nil {
					_ = s.store.MarkApplicationFailed(ctx, claim.ApplicationID, err.Error())
					continue
				}
				processed++
			}
		}
		if _, err := s.store.CompleteIfDrained(ctx, eventID); err != nil {
			continue
		}
	}
	return processed, nil
}

// applyOne 单订阅单事务：认领态校验 → 订阅锁 → 生命周期过滤 → Reset Core → 终态 → COMMIT。
// 认领（ClaimApplicationBatch）与执行在不同连接：认领先独立提交为 applying，
// 本事务完成全部业务写入后必须显式 Commit，否则全部回滚、行停在 applying。
func (s *ResetEventService) applyOne(ctx context.Context, eventID int64, effectiveAt time.Time, claim ResetApplicationClaim) error {
	tx, err := s.entClient.Tx(ctx)
	if err != nil {
		return err
	}
	committed := false
	defer func() {
		if !committed {
			_ = tx.Rollback()
		}
	}()
	txCtx := dbent.NewTxContext(ctx, tx)

	// finalize 落终态并提交本事务；任何错误路径走 defer 回滚。
	finalize := func(prevStart *time.Time, prevUsage *float64, status string) error {
		if err := s.store.FinalizeApplication(txCtx, claim.ApplicationID, prevStart, prevUsage, status); err != nil {
			return err
		}
		if err := tx.Commit(); err != nil {
			return err
		}
		committed = true
		return nil
	}

	app, err := s.store.GetApplicationForUpdate(txCtx, claim.ApplicationID)
	if err != nil {
		return err
	}
	if app.Status != domain.ResetApplicationStatusApplying {
		// 已被并发处理/终态：幂等 no-op（无需写入，直接提交空事务）
		if err := tx.Commit(); err != nil {
			return err
		}
		committed = true
		return nil
	}

	sub, err := s.subRepo.GetByIDForUpdate(txCtx, claim.UserSubscriptionID)
	if err != nil {
		// 订阅已删除：记 skipped（不阻塞批次）
		return finalize(nil, nil, domain.ResetApplicationStatusSkipped)
	}
	// 生命周期：expired / 非 active → skipped（Reset 不复活订阅）
	now := s.now()
	if sub.Status != SubscriptionStatusActive || !sub.ExpiresAt.After(now) {
		return finalize(sub.WeeklyWindowStart, resetFloatPtr(sub.WeeklyUsageUSD),
			domain.ResetApplicationStatusSkipped)
	}

	result, err := s.resetCore.ResetSubscriptionWeeklyPeriod(txCtx, &WeeklyResetInput{
		UserSubscriptionID: claim.UserSubscriptionID,
		EffectiveAt:        effectiveAt,
		Source:             domain.WeeklyResetSourceBatchDirect,
	})
	if err != nil {
		return err
	}
	finalStatus := domain.ResetApplicationStatusApplied
	if result.Status == WeeklyResetSkippedStale || result.Status == WeeklyResetAlreadyApplied {
		finalStatus = domain.ResetApplicationStatusSkipped
	}
	return finalize(sub.WeeklyWindowStart, resetFloatPtr(sub.WeeklyUsageUSD), finalStatus)
}

func resetFloatPtr(v float64) *float64 { return &v }

func directScopeTypeFor(mode string) string {
	switch mode {
	case domain.ResetTargetModeSubscriptionIDs:
		return "subscription"
	case domain.ResetTargetModeUsers:
		return "user"
	case domain.ResetTargetModeGroups:
		return "group"
	case domain.ResetTargetModeAllActive:
		return "all"
	default:
		return "all"
	}
}

func directScopeJSON(selector DirectResetSelector) map[string]any {
	switch selector.TargetMode {
	case domain.ResetTargetModeSubscriptionIDs:
		return map[string]any{"subscription_ids": selector.SubscriptionIDs}
	case domain.ResetTargetModeUsers:
		return map[string]any{"user_ids": selector.UserIDs}
	case domain.ResetTargetModeGroups:
		return map[string]any{"group_ids": selector.GroupIDs}
	default:
		return map[string]any{}
	}
}
