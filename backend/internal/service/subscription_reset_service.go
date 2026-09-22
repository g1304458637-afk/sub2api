package service

// Subscription V1 Phase 2 —— 统一 Weekly Period Re-Anchoring Reset Core。
//
// 唯一入口：SubscriptionService.ResetSubscriptionWeeklyPeriod。
// Admin Manual（既有 AdminResetQuota weekly 分支）/ 未来的 Global Reset worker、
// Reset Card 消费、补偿操作都必须复用本方法，禁止各自实现 "weekly_usage=0"。
//
// 语义（Phase 2 架构确认，测试锁定）：
//   - 成功 = weekly_usage_usd→0 且 weekly_window_start→effectiveAt（Re-Anchoring，
//     后续自然周期沿新锚点每 7 天推进；不回 started_at 旧轨道）；
//   - started_at / expires_at / user.balance / daily / monthly 一律不触碰；
//     Reset 永不延长订阅（最后一个周期可不足 7 天）；
//   - 统一 stale invariant：current anchor >= effectiveAt（含相等）→ SKIPPED_STALE，
//     旧事件绝不能把用户已推进的新锚点回拨；
//   - effectiveAt 由调用方显式传入（Global=事件共享时刻、Card=消费时刻、Admin=now）；
//     零值 → INVALID_EFFECTIVE_TIME；未来时刻 → NOT_YET_EFFECTIVE（V1 不做未来锚点）；
//   - 事件驱动在同一事务内 claim/finalize subscription_reset_applications：
//     崩溃窗口全部归约为「整事务提交或整事务回滚」，retry 要么 ALREADY_APPLIED
//     要么 SKIPPED_STALE，绝无第二次错误 Reset；
//   - 失败（error）= 整事务回滚 = 不留任何行；"failed application 行" 与业务事务
//     回滚的矛盾由此不存在（skipped 是成功提交的终态，不是错误）；
//   - DB commit 成功后才失效缓存（L1 ristretto + Redis pub/sub，复用 AdminResetQuota 模式）。

import (
	"context"
	"errors"
	"log"
	"time"

	dbent "github.com/Wei-Shaw/sub2api/ent"
	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/Wei-Shaw/sub2api/internal/domain"
)

// Reset Core 依赖注入：事件应用记录仓储（Phase 4 Global worker 正式消费）。
// SetResetApplicationRepository 供 wire 包装器/测试注入；nil 时事件驱动 Reset
// 返回错误，非事件路径（Admin/Card）不受影响。
func (s *SubscriptionService) SetResetApplicationRepository(repo SubscriptionResetApplicationRepository) {
	s.resetAppRepo = repo
}

// ProvideSubscriptionServiceWithReset 是 wire 使用的构造器（DI Strategy A：
// 仓储构造器直接返回接口），在既有 NewSubscriptionService 之上注入
// Reset Core 的事件应用仓储。保持 NewSubscriptionService 原签名以兼容存量调用方。
func ProvideSubscriptionServiceWithReset(
	groupRepo GroupRepository,
	userSubRepo UserSubscriptionRepository,
	billingCacheService *BillingCacheService,
	entClient *dbent.Client,
	cfg *config.Config,
	resetAppRepo SubscriptionResetApplicationRepository,
) *SubscriptionService {
	svc := NewSubscriptionService(groupRepo, userSubRepo, billingCacheService, entClient, cfg)
	svc.SetResetApplicationRepository(resetAppRepo)
	return svc
}

// ResetSubscriptionWeeklyPeriod 统一 Weekly Period Re-Anchoring Reset 入口。
func (s *SubscriptionService) ResetSubscriptionWeeklyPeriod(ctx context.Context, in *WeeklyResetInput) (*WeeklyResetResult, error) {
	if in == nil || in.UserSubscriptionID <= 0 {
		return nil, ErrInvalidInput
	}
	if in.EffectiveAt.IsZero() {
		return nil, ErrResetInvalidEffectiveTime
	}
	if in.Source == "" {
		return nil, ErrInvalidInput
	}
	now := s.now()
	// V1 不做未来锚点：Reset Core 只执行已生效的 Reset（未来事件由调度器到点后再调）
	if in.EffectiveAt.After(now) {
		return nil, ErrResetNotYetEffective
	}
	if in.ResetEventID != nil && s.resetAppRepo == nil {
		return nil, errors.New("subscription reset: event application repository is not wired")
	}

	if s.entClient != nil && dbent.TxFromContext(ctx) == nil {
		// 事件应用审计 + CAS 重置必须在同一事务内（§29 事务边界）。
		// 调用方已持有事务（如 BulkSubscriptionAction 的 withSubscriptionUpdateTx）时
		// 直接复用其事务，绝不嵌套开新事务（提交/回滚交给外层持有者）。
		tx, err := s.entClient.Tx(ctx)
		if err != nil {
			return nil, err
		}
		res, rerr := s.resetWeeklyPeriodInTx(dbent.NewTxContext(ctx, tx), in, now)
		if rerr != nil {
			_ = tx.Rollback()
			return nil, rerr
		}
		if err := tx.Commit(); err != nil {
			return nil, err
		}
		s.invalidateAfterWeeklyReset(ctx, res.Subscription)
		return res, nil
	}

	// 已在调用方事务内（复用该事务）或无 entClient（单元测试）：
	// 外层事务提交成功后才失效缓存；回滚不能广播未生效的重置。
	res, err := s.resetWeeklyPeriodInTx(ctx, in, now)
	if err != nil {
		return nil, err
	}
	if tx := dbent.TxFromContext(ctx); tx != nil {
		tx.OnCommit(func(next dbent.Committer) dbent.Committer {
			return dbent.CommitFunc(func(commitCtx context.Context, tx *dbent.Tx) error {
				if err := next.Commit(commitCtx, tx); err != nil {
					return err
				}
				s.invalidateAfterWeeklyReset(commitCtx, res.Subscription)
				return nil
			})
		})
	} else {
		s.invalidateAfterWeeklyReset(ctx, res.Subscription)
	}
	return res, nil
}

// resetWeeklyPeriodInTx 在单一事务（或无事务直连）内执行核心流程：
// 行锁读 → 生命周期校验 → （事件）claim → stale 守卫 → CAS 重置 → finalize 审计。
func (s *SubscriptionService) resetWeeklyPeriodInTx(ctx context.Context, in *WeeklyResetInput, now time.Time) (*WeeklyResetResult, error) {
	// 行锁串行化：与 IncrementUsage（结算）互斥，两种顺序结果都确定（Phase 0 已证）
	sub, err := s.userSubRepo.GetByIDForUpdate(ctx, in.UserSubscriptionID)
	if err != nil {
		return nil, err
	}

	if !in.IgnoreLifecycleCheck {
		// Reset 不复活订阅：expired/suspended 一律拒绝
		switch sub.Status {
		case SubscriptionStatusExpired:
			return nil, ErrSubscriptionExpired
		case SubscriptionStatusSuspended:
			return nil, ErrSubscriptionSuspended
		}
		if !sub.ExpiresAt.After(now) {
			return nil, ErrSubscriptionExpired
		}
	}

	// 事件驱动：claim-first（UNIQUE(reset_event_id, user_subscription_id) 兜底 retry 幂等）。
	// claim 行只在同事务提交后可见 —— 崩溃即整体回滚，不存在"半认领"状态。
	var claimedAppID int64
	if in.ResetEventID != nil {
		claimed, existingStatus, id, cerr := s.resetAppRepo.ClaimWeeklyResetApplication(
			ctx, *in.ResetEventID, in.UserSubscriptionID, in.EffectiveAt)
		if cerr != nil {
			return nil, cerr
		}
		if !claimed {
			// 同一 (event, subscription) 已有终态：applied → ALREADY_APPLIED；
			// skipped → 锚点单调递增，本次必然仍为 stale。
			status := WeeklyResetSkippedStale
			if existingStatus == domain.ResetApplicationStatusApplied {
				status = WeeklyResetAlreadyApplied
			}
			fresh, ferr := s.userSubRepo.GetByID(ctx, in.UserSubscriptionID)
			if ferr != nil {
				return nil, ferr
			}
			return &WeeklyResetResult{Status: status, Subscription: fresh, ApplicationStatus: existingStatus}, nil
		}
		claimedAppID = id
	}

	previousStart := sub.WeeklyWindowStart
	previousUsage := sub.WeeklyUsageUSD

	// 统一 stale invariant（含相等）：绝不回拨、绝不二次清零新周期内的消费
	if previousStart != nil && !previousStart.Before(in.EffectiveAt) {
		if claimedAppID != 0 {
			if ferr := s.resetAppRepo.FinalizeClaimedWeeklyResetApplication(
				ctx, claimedAppID, previousStart, &previousUsage, now,
				domain.ResetApplicationStatusSkipped); ferr != nil {
				return nil, ferr
			}
		}
		fresh, ferr := s.userSubRepo.GetByID(ctx, in.UserSubscriptionID)
		if ferr != nil {
			return nil, ferr
		}
		return &WeeklyResetResult{
			Status:            WeeklyResetSkippedStale,
			Subscription:      fresh,
			ApplicationStatus: domain.ResetApplicationStatusSkipped,
		}, nil
	}

	// CAS 重置：复用既有 repo 原子操作（Phase 0 锁定其行为），不另造无守卫 UPDATE。
	// 行锁在手，expected 即锁内快照；previousStart=nil（窗口未激活）由 repo 的
	// IS NULL 条件分支处理。
	if err := s.userSubRepo.ResetWeeklyUsage(ctx, in.UserSubscriptionID, previousStart, in.EffectiveAt); err != nil {
		return nil, err
	}

	if claimedAppID != 0 {
		if ferr := s.resetAppRepo.FinalizeClaimedWeeklyResetApplication(
			ctx, claimedAppID, previousStart, &previousUsage, now,
			domain.ResetApplicationStatusApplied); ferr != nil {
			return nil, ferr
		}
	}

	fresh, ferr := s.userSubRepo.GetByID(ctx, in.UserSubscriptionID)
	if ferr != nil {
		return nil, ferr
	}
	return &WeeklyResetResult{
		Status:            WeeklyResetApplied,
		Subscription:      fresh,
		ApplicationStatus: domain.ResetApplicationStatusApplied,
	}, nil
}

// invalidateAfterWeeklyReset 提交后失效两层订阅缓存（顺序：commit → invalidate）。
// 复用 AdminResetQuota 的成熟模式：L1 ristretto 同步失效（含 Wait）+
// Redis L2 失效并 pub/sub 广播跨实例。
func (s *SubscriptionService) invalidateAfterWeeklyReset(ctx context.Context, sub *UserSubscription) {
	if sub == nil {
		return
	}
	if err := s.invalidateSubscriptionCaches(sub.UserID, sub.GroupID); err != nil {
		log.Printf("subscription reset cache invalidation failed: user=%d group=%d err=%v", sub.UserID, sub.GroupID, err)
	}
}
