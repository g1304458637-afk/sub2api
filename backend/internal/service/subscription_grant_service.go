package service

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"time"

	dbent "github.com/Wei-Shaw/sub2api/ent"
	"github.com/Wei-Shaw/sub2api/internal/domain"
)

// SubscriptionGrantService 统一订阅权益发放服务。
//
// 所有「给用户一份订阅权益」的场景（学生认证、管理员赠送、未来的活动/邀请/
// 补偿/教师权益）都必须经过本服务，不允许来源模块自己 INSERT user_subscriptions：
//   - Grant = 为什么、由谁、通过什么活动给了用户一份权益（本服务 + 台账表）；
//   - Subscription = 用户现在实际拥有什么订阅（SubscriptionService 独占管理）。
//
// 激活统一复用 Assign-or-Extend 原语：同组已有订阅 → 从 expires_at 顺延
// （过期则从 now 重算并激活）；无订阅 → 创建；其他组已有 active 主订阅
// （RULE 1）→ ErrPrimarySubscriptionExists，绝不允许第二份 ACTIVE。
//
// 幂等三层：
//  1. benefit_claims 双唯一约束（benefit_code+identity_key / benefit_code+user_id）
//     锁定一次性权益的「一人一号一权益」；
//  2. subscription_grants UNIQUE (source, source_key) 部分唯一索引锁定业务动作；
//  3. API 层 IdempotencyCoordinator（admin: Idempotency-Key 头）防浏览器重复提交。
type SubscriptionGrantService struct {
	entClient           *dbent.Client
	grantRepo           SubscriptionGrantRepository
	claimRepo           BenefitClaimRepository
	userRepo            UserRepository
	groupRepo           GroupRepository
	userSubRepo         UserSubscriptionRepository
	assigner            GrantSubscriptionAssigner
	billingCacheService *BillingCacheService

	now func() time.Time
}

func NewSubscriptionGrantService(
	entClient *dbent.Client,
	grantRepo SubscriptionGrantRepository,
	claimRepo BenefitClaimRepository,
	userRepo UserRepository,
	groupRepo GroupRepository,
	userSubRepo UserSubscriptionRepository,
	assigner GrantSubscriptionAssigner,
	billingCacheService *BillingCacheService,
) *SubscriptionGrantService {
	return &SubscriptionGrantService{
		entClient:           entClient,
		grantRepo:           grantRepo,
		claimRepo:           claimRepo,
		userRepo:            userRepo,
		groupRepo:           groupRepo,
		userSubRepo:         userSubRepo,
		assigner:            assigner,
		billingCacheService: billingCacheService,
		now:                 time.Now,
	}
}

// SetNowFunc 单测注入时钟。
func (s *SubscriptionGrantService) SetNowFunc(fn func() time.Time) {
	if s != nil && fn != nil {
		s.now = fn
	}
}

// CreateGrant 发放订阅权益（自管事务；管理员路径）。
// 返回的 Outcome 区分 activated_new / extended / pending / already_granted。
func (s *SubscriptionGrantService) CreateGrant(ctx context.Context, cmd *CreateSubscriptionGrantCommand) (*SubscriptionGrantExecution, error) {
	if s == nil || s.entClient == nil || s.grantRepo == nil || s.assigner == nil {
		return nil, ErrServiceUnavailable
	}
	if dbent.TxFromContext(ctx) != nil {
		// 防御：调用方已带事务时应走 CreateGrantInTx。
		return s.CreateGrantInTx(ctx, cmd)
	}

	tx, err := s.entClient.Tx(ctx)
	if err != nil {
		return nil, fmt.Errorf("begin subscription grant transaction: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	txCtx := dbent.NewTxContext(ctx, tx)
	execution, err := s.CreateGrantInTx(txCtx, cmd)
	if err != nil {
		return nil, err
	}
	if err := tx.Commit(); err != nil {
		return nil, fmt.Errorf("commit subscription grant transaction: %w", err)
	}

	if execution.Outcome != nil && execution.Outcome.Granted() {
		s.InvalidateGrantCaches(ctx, execution.Grant.UserID, execution.Grant.GroupID)
	}
	return execution, nil
}

// CreateGrantInTx 在调用方事务内联发放（学生认证路径）。调用方提交成功后必须
// 调用 InvalidateGrantCaches（缓存失效必须在 COMMIT 之后，避免旧值回填）。
func (s *SubscriptionGrantService) CreateGrantInTx(ctx context.Context, cmd *CreateSubscriptionGrantCommand) (*SubscriptionGrantExecution, error) {
	if s == nil || s.grantRepo == nil || s.assigner == nil {
		return nil, ErrServiceUnavailable
	}
	if err := validateCreateSubscriptionGrantCommand(cmd); err != nil {
		return nil, err
	}
	if err := s.validateTargetUserAndGroup(ctx, cmd); err != nil {
		return nil, err
	}

	grant := &SubscriptionGrant{
		UserID:          cmd.UserID,
		GroupID:         cmd.GroupID,
		PlanID:          cmd.PlanID,
		Source:          cmd.Source,
		Status:          domain.SubscriptionGrantStatusPending,
		EffectivePolicy: cmd.EffectivePolicy,
		DurationDays:    cmd.DurationDays,
	}
	if cmd.SourceKey != "" {
		v := cmd.SourceKey
		grant.SourceKey = &v
	}
	if cmd.BenefitCode != "" {
		v := cmd.BenefitCode
		grant.BenefitCode = &v
	}
	if cmd.IdentityType != "" {
		v := cmd.IdentityType
		grant.IdentityType = &v
	}
	if cmd.IdentityKey != "" {
		v := cmd.IdentityKey
		grant.IdentityKey = &v
	}
	if cmd.IdempotencyKey != "" {
		v := cmd.IdempotencyKey
		grant.IdempotencyKey = &v
	}
	if cmd.Reason != "" {
		v := truncateForColumn(cmd.Reason, 500)
		grant.Reason = &v
	}
	if cmd.Notes != "" {
		v := cmd.Notes
		grant.Notes = &v
	}
	grant.OperatorUserID = cmd.OperatorUserID

	// ① 台账落库（业务幂等第一层：UNIQUE (source, source_key)）
	inserted, err := s.grantRepo.InsertIdempotent(ctx, grant)
	if err != nil {
		return nil, fmt.Errorf("insert subscription grant: %w", err)
	}
	if !inserted {
		// 幂等命中。对一次性权益先甄别冲突维度：同一身份键已被其他账号领取时
		// 必须报错（否则新账号会误以为领取成功）；本人重放才是幂等成功。
		if cmd.BenefitCode != "" {
			byIdentity, diagErr := s.claimRepo.GetByBenefitAndIdentity(ctx, cmd.BenefitCode, cmd.IdentityKey)
			if diagErr == nil && byIdentity != nil && byIdentity.UserID != cmd.UserID {
				return nil, ErrBenefitAlreadyClaimed.WithMetadata(map[string]string{"reason": "identity_claimed_by_other_user"})
			}
		}
		existing, getErr := s.grantRepo.GetBySourceKey(ctx, cmd.Source, cmd.SourceKey)
		if getErr != nil {
			return nil, fmt.Errorf("load existing subscription grant: %w", getErr)
		}
		return &SubscriptionGrantExecution{
			Grant:   existing,
			Outcome: &GrantOutcome{Action: GrantOutcomeAlreadyGranted, GrantID: existing.ID, Message: "该赠送动作之前已执行（幂等命中）"},
		}, nil
	}

	// ② 一次性权益领取（业务幂等第二层：benefit_claims 双唯一约束）。
	//    claim 是永久事实，与 Grant 同事务：claim 冲突 → 整体回滚，不留半截状态。
	if cmd.BenefitCode != "" {
		claim := &BenefitClaim{
			BenefitCode:  cmd.BenefitCode,
			UserID:       cmd.UserID,
			IdentityType: cmd.IdentityType,
			IdentityKey:  cmd.IdentityKey,
			GrantID:      &grant.ID,
		}
		claimInserted, err := s.claimRepo.InsertIdempotent(ctx, claim)
		if err != nil {
			return nil, fmt.Errorf("insert benefit claim: %w", err)
		}
		if !claimInserted {
			return nil, s.benefitClaimConflict(ctx, cmd)
		}
	}

	// ③ 激活：immediate 尝试当场生效（跨组冲突 → 报错或按 PendingFallback 转
	//    pending）；end_of_term 无冲突时立即生效、有冲突才登记 pending 由 worker
	//    衔接（与预览口径一致，避免「无订阅却等到下一分钟」）。
	outcome := &GrantOutcome{GrantID: grant.ID}
	shouldActivate := cmd.EffectivePolicy == domain.SubscriptionGrantPolicyImmediate
	if !shouldActivate {
		conflict, findErr := s.findActiveConflict(ctx, cmd.UserID, cmd.GroupID)
		if findErr != nil {
			return nil, findErr
		}
		shouldActivate = conflict == nil
	}
	if shouldActivate {
		if err := s.activateGrantInTx(ctx, grant); err != nil {
			if errors.Is(err, ErrGrantConflict) {
				if cmd.PendingFallback {
					outcome.Action = GrantOutcomePending
					outcome.Message = "当前已有其他分组的有效订阅，权益已登记待生效，将在其结束后自动衔接"
					return &SubscriptionGrantExecution{Grant: grant, Outcome: outcome}, nil
				}
				return nil, ErrGrantConflict
			}
			return nil, err
		}
		outcome.Action = grant.outcomeAction()
		outcome.SubscriptionID = derefInt64(grant.LinkedSubscriptionID)
		outcome.PreviousExpires = grant.prevExpiresCache
		outcome.ExpiresAt = grant.ContributionEnd
	} else {
		outcome.Action = GrantOutcomePending
		outcome.Message = "权益已登记，将在当前订阅结束后自动生效"
	}

	return &SubscriptionGrantExecution{Grant: grant, Outcome: outcome}, nil
}

// benefitClaimConflict 幂等冲突诊断：区分「邮箱已被其他账号领取」与
// 「该账号已领取过该权益」。
func (s *SubscriptionGrantService) benefitClaimConflict(ctx context.Context, cmd *CreateSubscriptionGrantCommand) error {
	byIdentity, err := s.claimRepo.GetByBenefitAndIdentity(ctx, cmd.BenefitCode, cmd.IdentityKey)
	if err == nil && byIdentity != nil && byIdentity.UserID != cmd.UserID {
		return ErrBenefitAlreadyClaimed.WithMetadata(map[string]string{"reason": "identity_claimed_by_other_user"})
	}
	byUser, err := s.claimRepo.GetByBenefitAndUser(ctx, cmd.BenefitCode, cmd.UserID)
	if err == nil && byUser != nil {
		return ErrBenefitAlreadyClaimed.WithMetadata(map[string]string{"reason": "user_already_claimed"})
	}
	return ErrBenefitAlreadyClaimed
}

// activateGrantInTx 激活单条台账（调用方需保证在事务上下文内）。
// 成功后回写台账的 fulfilled/贡献时段/关联订阅；RULE 1 冲突返回 ErrGrantConflict
// （由调用方决定 pending 还是报错）。
func (s *SubscriptionGrantService) activateGrantInTx(ctx context.Context, grant *SubscriptionGrant) error {
	now := s.now()

	// 记录激活前的到期时间（顺延场景的贡献时段起点）。
	var prevExpires *time.Time
	if prev, err := s.userSubRepo.GetByUserIDAndGroupID(ctx, grant.UserID, grant.GroupID); err == nil {
		prevExpires = &prev.ExpiresAt
	} else if !errors.Is(err, ErrSubscriptionNotFound) {
		return fmt.Errorf("load previous subscription: %w", err)
	}

	notes := fmt.Sprintf("subscription grant #%d (source=%s)", grant.ID, grant.Source)
	input := &AssignSubscriptionInput{
		UserID:       grant.UserID,
		GroupID:      grant.GroupID,
		ValidityDays: grant.DurationDays,
		PlanID:       grant.PlanID,
		Notes:        notes,
	}
	sub, _, err := s.assigner.AssignOrExtendSubscriptionDeferredCache(ctx, input)
	if err != nil {
		if errors.Is(err, ErrPrimarySubscriptionExists) {
			// RULE 1：其他分组已有 active 主订阅。绝不降级/覆盖现有权益。
			return ErrGrantConflict
		}
		return fmt.Errorf("assign grant subscription: %w", err)
	}

	// 贡献时段：顺延场景 = [旧到期时刻, 新到期时刻]；新建/过期重激活 = [now, 新到期时刻]。
	contributionStart := now
	if prevExpires != nil && prevExpires.After(now) {
		contributionStart = *prevExpires
	}
	activatedAt := now
	grant.Status = domain.SubscriptionGrantStatusFulfilled
	grant.LinkedSubscriptionID = &sub.ID
	grant.ContributionStart = &contributionStart
	grant.ContributionEnd = &sub.ExpiresAt
	grant.ActivatedAt = &activatedAt
	grant.prevExpiresCache = prevExpires

	update := GrantActivationUpdate{
		Status:               grant.Status,
		LinkedSubscriptionID: sub.ID,
		ContributionStart:    contributionStart,
		ContributionEnd:      sub.ExpiresAt,
		ActivatedAt:          activatedAt,
	}
	if err := s.grantRepo.UpdateActivation(ctx, grant.ID, update); err != nil {
		return fmt.Errorf("record grant activation: %w", err)
	}
	return nil
}

// ActivateDuePendingGrants 扫描待激活台账并尝试激活（worker 调用）。
// 返回 (激活数, 错误)。单条失败不影响其余；RULE 1 冲突视为「尚未到时机」。
func (s *SubscriptionGrantService) ActivateDuePendingGrants(ctx context.Context, limit int) (int, error) {
	if s == nil || s.grantRepo == nil || s.assigner == nil {
		return 0, ErrServiceUnavailable
	}
	if limit <= 0 {
		limit = 200
	}
	grants, err := s.grantRepo.ListPending(ctx, limit)
	if err != nil {
		return 0, fmt.Errorf("list pending grants: %w", err)
	}

	activated := 0
	for _, pending := range grants {
		tx, err := s.entClient.Tx(ctx)
		if err != nil {
			return activated, fmt.Errorf("begin grant activation transaction: %w", err)
		}
		txCtx := dbent.NewTxContext(ctx, tx)

		grant, err := s.grantRepo.GetForUpdate(txCtx, pending.ID)
		if err != nil {
			_ = tx.Rollback()
			return activated, fmt.Errorf("lock pending grant %d: %w", pending.ID, err)
		}
		if grant.Status != domain.SubscriptionGrantStatusPending {
			_ = tx.Rollback() // 已被并发激活/撤销，跳过
			continue
		}

		if err := s.validateTargetUserAndGroup(txCtx, &CreateSubscriptionGrantCommand{
			UserID: grant.UserID, GroupID: grant.GroupID,
		}); err != nil {
			_ = tx.Rollback()
			// 配置被删等终态失败：标记 failed，避免无限重试。
			if markErr := s.grantRepo.MarkFailed(ctx, grant.ID, err.Error()); markErr != nil {
				return activated, fmt.Errorf("mark grant %d failed: %w", grant.ID, markErr)
			}
			slog.Warn("subscription grant marked failed (target invalid)", "grant_id", grant.ID, "error", err)
			continue
		}

		err = s.activateGrantInTx(txCtx, grant)
		if err != nil {
			_ = tx.Rollback()
			if errors.Is(err, ErrGrantConflict) {
				continue // 尚未到衔接时机，保持 pending
			}
			slog.Error("activate pending grant failed; will retry next tick", "grant_id", grant.ID, "error", err)
			continue
		}
		if err := tx.Commit(); err != nil {
			slog.Error("commit grant activation failed; will retry next tick", "grant_id", grant.ID, "error", err)
			continue
		}
		activated++
		s.InvalidateGrantCaches(ctx, grant.UserID, grant.GroupID)
	}
	return activated, nil
}

// ExpireFulfilledGrants 贡献期自然结束的 fulfilled 台账 → expired。
// 这是 Grant 自身生命周期的归档，不影响订阅行（订阅到期由 SubscriptionExpiryService 管理）。
func (s *SubscriptionGrantService) ExpireFulfilledGrants(ctx context.Context) (int64, error) {
	if s == nil || s.grantRepo == nil {
		return 0, ErrServiceUnavailable
	}
	return s.grantRepo.ExpireFulfilledContribution(ctx, s.now())
}

// RevokeGrant 撤销赠送（管理员操作）。
//
// 状态机：
//   - pending/failed → revoked（未生效权益安全取消，不触碰任何订阅行）；
//   - fulfilled      → 受「付费地板」保护回收赠送时段（见 clampRevokeTarget）。
//
// 绝不缩短用户付费购买的权益：付费 term 快照（purchase/renewal/upgrade）的
// term_end 与其他 Grant 的贡献终点构成地板，回收只会把到期时间钳制到
// max(地板, now)，单向只缩不涨。
func (s *SubscriptionGrantService) RevokeGrant(ctx context.Context, grantID int64, operatorID int64, reason string) (*SubscriptionGrant, error) {
	if s == nil || s.entClient == nil || s.grantRepo == nil {
		return nil, ErrServiceUnavailable
	}
	if grantID <= 0 || operatorID <= 0 {
		return nil, ErrGrantInvalidCommand
	}
	reason = truncateForColumn(reason, 500)

	var (
		revokedGroupID int64
		revokedUserID  int64
		affectedSub    bool
	)
	tx, err := s.entClient.Tx(ctx)
	if err != nil {
		return nil, fmt.Errorf("begin grant revoke transaction: %w", err)
	}
	defer func() { _ = tx.Rollback() }()
	txCtx := dbent.NewTxContext(ctx, tx)

	grant, err := s.grantRepo.GetForUpdate(txCtx, grantID)
	if err != nil {
		return nil, err
	}
	if grant.Status == domain.SubscriptionGrantStatusRevoked {
		return nil, ErrGrantNotRevokable
	}
	revokedGroupID = grant.GroupID
	revokedUserID = grant.UserID

	switch grant.Status {
	case domain.SubscriptionGrantStatusPending, domain.SubscriptionGrantStatusFailed:
		// 未生效：安全取消，无订阅副作用。
	case domain.SubscriptionGrantStatusFulfilled:
		affectedSub, err = s.clampGrantContributionOnRevoke(txCtx, grant)
		if err != nil {
			return nil, err
		}
	case domain.SubscriptionGrantStatusExpired:
		// 贡献期已自然结束，无可回收时段；仅归档撤销事实。
	default:
		return nil, ErrGrantNotRevokable
	}

	if err := s.grantRepo.MarkRevoked(txCtx, grant.ID, operatorID, reason); err != nil {
		return nil, fmt.Errorf("mark grant revoked: %w", err)
	}
	if err := tx.Commit(); err != nil {
		return nil, fmt.Errorf("commit grant revoke: %w", err)
	}

	if affectedSub {
		s.InvalidateGrantCaches(ctx, revokedUserID, revokedGroupID)
	}
	revoked, err := s.grantRepo.GetByID(ctx, grant.ID)
	if err != nil {
		return grant, nil
	}
	return revoked, nil
}

// clampGrantContributionOnRevoke 回收 fulfilled Grant 的赠送时段（需行锁）。
// 返回是否有订阅行被修改。
func (s *SubscriptionGrantService) clampGrantContributionOnRevoke(ctx context.Context, grant *SubscriptionGrant) (bool, error) {
	if grant.LinkedSubscriptionID == nil {
		return false, nil
	}
	sub, err := s.userSubRepo.GetByIDForUpdate(ctx, *grant.LinkedSubscriptionID)
	if err != nil {
		if errors.Is(err, ErrSubscriptionNotFound) {
			return false, nil // 订阅行已不存在（如被软删），无可回收
		}
		return false, fmt.Errorf("lock subscription for revoke: %w", err)
	}
	if sub.Status != SubscriptionStatusActive || !sub.ExpiresAt.After(s.now()) {
		return false, nil // 订阅已不活跃/已过期，无可回收时段
	}

	floor, err := s.revokeFloor(ctx, grant)
	if err != nil {
		return false, err
	}
	now := s.now()
	// 回收目标 = min(当前到期, max(地板, now))：
	//   无地板（纯赠送订阅）→ 收到 now（赠送时段全部可回收）；
	//   付费地板在赠送段内 → 收到地板（付费权益完整保留）；
	//   地板已覆盖当前到期 → 无可回收时段。
	target := now
	if floor != nil && floor.After(target) {
		target = *floor
	}
	if target.After(sub.ExpiresAt) {
		target = sub.ExpiresAt
	}
	if !target.Before(sub.ExpiresAt) {
		return false, nil // 无可回收时段（地板已覆盖当前到期）
	}

	if !target.After(now) {
		// 赠送时段已全部或部分被消费：直接到期并收敛状态。
		if err := s.userSubRepo.UpdateStatus(ctx, sub.ID, SubscriptionStatusExpired); err != nil {
			return false, fmt.Errorf("expire subscription on revoke: %w", err)
		}
		if err := s.userSubRepo.ExtendExpiry(ctx, sub.ID, now); err != nil {
			return false, fmt.Errorf("clamp subscription expiry on revoke: %w", err)
		}
		return true, nil
	}
	// 回收到期时间至地板（保持 active，用户保留付费/后续权益覆盖的时段）。
	if err := s.userSubRepo.ExtendExpiry(ctx, sub.ID, target); err != nil {
		return false, fmt.Errorf("clamp subscription expiry on revoke: %w", err)
	}
	return true, nil
}

// revokeFloor 撤销地板：付费 term 快照 + 同订阅其他 Grant 的贡献终点 +
// 本 Grant 激活前的到期时刻（contribution_start，保护被顺延的原有权益）。
func (s *SubscriptionGrantService) revokeFloor(ctx context.Context, grant *SubscriptionGrant) (*time.Time, error) {
	var floor *time.Time
	raise := func(t *time.Time) {
		if t == nil {
			return
		}
		if floor == nil || t.After(*floor) {
			floor = t
		}
	}

	paid, err := s.grantRepo.PaidFloorForSubscription(ctx, *grant.LinkedSubscriptionID)
	if err != nil {
		return nil, fmt.Errorf("load paid floor: %w", err)
	}
	raise(paid)

	other, err := s.grantRepo.OtherGrantFloorForSubscription(ctx, *grant.LinkedSubscriptionID, grant.ID)
	if err != nil {
		return nil, fmt.Errorf("load other grant floor: %w", err)
	}
	raise(other)

	// contribution_start 仅在「顺延既有订阅」场景构成地板（激活前已存在的到期时刻）。
	// 新建场景 contribution_start == activated_at（订阅本身由本 Grant 创建），
	// 不能作为地板，否则撤销纯赠送订阅会被它自己挡住。
	if grant.ContributionStart != nil && grant.ActivatedAt != nil && grant.ContributionStart.After(*grant.ActivatedAt) {
		raise(grant.ContributionStart)
	}
	return floor, nil
}

// PreviewGrant 发放预览（管理员确认前看到将会发生什么；只读、无副作用）。
func (s *SubscriptionGrantService) PreviewGrant(ctx context.Context, cmd *CreateSubscriptionGrantCommand) (*GrantPreview, error) {
	if s == nil || s.userSubRepo == nil {
		return nil, ErrServiceUnavailable
	}
	if cmd.UserID <= 0 || cmd.GroupID <= 0 || cmd.DurationDays <= 0 || cmd.DurationDays > MaxValidityDays {
		return nil, ErrGrantInvalidCommand
	}
	if err := s.validateTargetUserAndGroup(ctx, cmd); err != nil {
		return nil, err
	}

	now := s.now()
	preview := &GrantPreview{}

	existing, err := s.userSubRepo.GetByUserIDAndGroupID(ctx, cmd.UserID, cmd.GroupID)
	if err != nil && !errors.Is(err, ErrSubscriptionNotFound) {
		return nil, fmt.Errorf("load subscription for preview: %w", err)
	}

	sameGroupUsable := existing != nil && existing.Status == SubscriptionStatusActive && existing.ExpiresAt.After(now)
	if sameGroupUsable {
		preview.Outcome = GrantPreviewWillExtend
		preview.CurrentGroupID = &existing.GroupID
		preview.CurrentExpires = &existing.ExpiresAt
		preview.ExtensionBase = &existing.ExpiresAt
		predicted := existing.ExpiresAt.AddDate(0, 0, cmd.DurationDays)
		if predicted.After(MaxExpiresAt) {
			predicted = MaxExpiresAt
		}
		preview.PredictedExpires = &predicted
		preview.Message = fmt.Sprintf("当前同套餐订阅有效期至 %s，赠送将从该时刻顺延 %d 天", existing.ExpiresAt.Format("2006-01-02 15:04"), cmd.DurationDays)
		return preview, nil
	}

	conflict, err := s.findActiveConflict(ctx, cmd.UserID, cmd.GroupID)
	if err != nil {
		return nil, err
	}
	if conflict != nil {
		preview.Outcome = GrantPreviewWillBePending
		preview.CurrentGroupID = &conflict.GroupID
		preview.CurrentPlanName = s.groupNameForPreview(ctx, conflict)
		preview.CurrentExpires = &conflict.ExpiresAt
		preview.Message = fmt.Sprintf("用户当前持有「%s」有效订阅（至 %s）。为避免覆盖现有权益，赠送将登记为待生效，在该订阅结束后自动激活 %d 天",
			preview.CurrentPlanName, conflict.ExpiresAt.Format("2006-01-02 15:04"), cmd.DurationDays)
		if cmd.EffectivePolicy == domain.SubscriptionGrantPolicyImmediate && !cmd.PendingFallback {
			preview.Outcome = GrantPreviewConflict
			preview.Message = fmt.Sprintf("立即生效会与用户当前的「%s」订阅（至 %s）冲突，且可能覆盖现有付费权益。请改选「当前订阅结束后生效」，或为用户办理升级/降级",
				preview.CurrentPlanName, conflict.ExpiresAt.Format("2006-01-02 15:04"))
		}
		return preview, nil
	}

	preview.Outcome = GrantPreviewWillActivateNew
	if existing != nil {
		// 目标组存在过期行：激活将从 now 重算（ expired → active，迁移 242 修复语义）。
		preview.Message = fmt.Sprintf("用户在目标分组已有过期订阅，赠送将重新激活该订阅并从现在起算 %d 天", cmd.DurationDays)
	} else {
		preview.Message = fmt.Sprintf("用户当前无有效订阅，赠送将立即生效，有效期 %d 天", cmd.DurationDays)
	}
	predicted := now.AddDate(0, 0, cmd.DurationDays)
	preview.PredictedExpires = &predicted
	return preview, nil
}

// groupNameForPreview 预览文案用分组名（守卫查询不预加载 Group，需回查；失败回退 "#id"）。
func (s *SubscriptionGrantService) groupNameForPreview(ctx context.Context, sub *UserSubscription) string {
	if sub.Group != nil && sub.Group.Name != "" {
		return sub.Group.Name
	}
	if s.groupRepo != nil {
		if group, err := s.groupRepo.GetByID(ctx, sub.GroupID); err == nil && group != nil {
			return group.Name
		}
	}
	return fmt.Sprintf("#%d", sub.GroupID)
}

// findActiveConflict 查询用户在目标组之外的 active 主订阅（RULE 1 口径）。
func (s *SubscriptionGrantService) findActiveConflict(ctx context.Context, userID, groupID int64) (*UserSubscription, error) {
	guard, ok := s.userSubRepo.(SubscriptionSingleActiveGuard)
	if !ok {
		return nil, nil
	}
	conflict, err := guard.FindActiveByUserIDExcludingGroup(ctx, userID, groupID)
	if err != nil {
		return nil, fmt.Errorf("check single-active invariant: %w", err)
	}
	return conflict, nil
}

func (s *SubscriptionGrantService) validateTargetUserAndGroup(ctx context.Context, cmd *CreateSubscriptionGrantCommand) error {
	if s.userRepo != nil {
		user, err := s.userRepo.GetByID(ctx, cmd.UserID)
		if err != nil || user == nil {
			return ErrGrantInvalidCommand.WithMetadata(map[string]string{"reason": "user_not_found"})
		}
	}
	if s.groupRepo != nil {
		group, err := s.groupRepo.GetByID(ctx, cmd.GroupID)
		if err != nil || group == nil {
			return ErrGrantInvalidCommand.WithMetadata(map[string]string{"reason": "group_not_found"})
		}
		if !group.IsSubscriptionType() {
			return ErrGroupNotSubscriptionType
		}
	}
	return nil
}

// InvalidateGrantCaches Grant 激活/撤销提交后由调用方调用（COMMIT 之后）。
func (s *SubscriptionGrantService) InvalidateGrantCaches(ctx context.Context, userID, groupID int64) {
	if s == nil || s.assigner == nil {
		return
	}
	s.assigner.InvalidateSubCache(userID, groupID)
	if s.billingCacheService != nil {
		go func() {
			cacheCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()
			_ = s.billingCacheService.InvalidateSubscription(cacheCtx, userID, groupID)
		}()
	}
}

// AdminListGrants 管理端台账分页查询。
func (s *SubscriptionGrantService) AdminListGrants(ctx context.Context, filter *GrantAdminFilter) (*GrantAdminList, error) {
	if s == nil || s.grantRepo == nil {
		return nil, ErrServiceUnavailable
	}
	return s.grantRepo.AdminList(ctx, filter)
}

// GetGrantByID 台账详情。
func (s *SubscriptionGrantService) GetGrantByID(ctx context.Context, id int64) (*SubscriptionGrant, error) {
	if s == nil || s.grantRepo == nil {
		return nil, ErrServiceUnavailable
	}
	return s.grantRepo.GetByID(ctx, id)
}

// ListGrantsByUser 用户维度台账。
func (s *SubscriptionGrantService) ListGrantsByUser(ctx context.Context, userID int64, limit int) ([]*SubscriptionGrant, error) {
	if s == nil || s.grantRepo == nil {
		return nil, ErrServiceUnavailable
	}
	return s.grantRepo.ListByUser(ctx, userID, limit)
}

// outcomeAction 根据台账落库状态推导结果动作。
func (g *SubscriptionGrant) outcomeAction() string {
	if g == nil {
		return ""
	}
	// 顺延（激活前已有未到期同组订阅）与新建（无订阅/过期重激活）在
	// activateGrantInTx 中通过 prevExpiresCache 区分。
	if g.prevExpiresCache != nil && g.ActivatedAt != nil && g.prevExpiresCache.After(*g.ActivatedAt) {
		return GrantOutcomeExtended
	}
	return GrantOutcomeActivatedNew
}

// SubscriptionGrantExecution 一次发放的完整执行结果。
type SubscriptionGrantExecution struct {
	Grant   *SubscriptionGrant `json:"grant"`
	Outcome *GrantOutcome      `json:"outcome"`
}

// derefInt64 可空 int64 安全解引用。
func derefInt64(v *int64) int64 {
	if v == nil {
		return 0
	}
	return *v
}

// truncateForColumn 字符串截断到列宽（台账 VARCHAR 列防溢出）。
func truncateForColumn(s string, max int) string {
	if len(s) <= max {
		return s
	}
	return s[:max]
}

// prevExpiresCache 激活前同组订阅的到期时刻（进程内推导 outcome 用，不落库）。
// 放在结构体私有字段而非台账列：台账已有 contribution_start 承载同一事实。
