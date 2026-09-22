package service

// Subscription V1 Phase 6 —— Reset Card Runtime（含 Scoped Grant）。
//
// Reset Card 是账户级一次性权益：属于 User，未消费前不绑定订阅；消费时由服务端
// 选卡并复用统一 Reset Core（ResetSubscriptionWeeklyPeriod）重置该订阅的周周期。
//
// Reference Decision（Phase 6.0）：
//   - Lago consumable credits：权益=带状态/过期的账本行；卡表即账本，不建第二 ledger。
//   - Kill Bill / Stripe：durable Idempotency-Key + stored response——复用仓库既有
//     IdempotencyCoordinator + idempotency_records（DB 持久），禁止内存幂等。
//   - PostgreSQL：SELECT ... FOR UPDATE + CAS 状态迁移 + 确定性 FIFO 选卡
//     （最早到期 → 最早创建 → 最小 id）。
//   - 不采用：内存幂等、后台批量 expire mutation（惰性有效过期：查询即判定）。
//
// 定向发卡（补充定稿）：target_mode = users / groups / all_active_users；
// 按 User 去重，quantity_per_user 控制每人张数；Grant Event 创建即 snapshot
// 目标用户集合，之后新购用户不自动获得旧活动卡。Preview 无任何写入。

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	dbent "github.com/Wei-Shaw/sub2api/ent"
	"github.com/Wei-Shaw/sub2api/internal/domain"
	infraerrors "github.com/Wei-Shaw/sub2api/internal/pkg/errors"
)

// Reset Card 业务错误。
var (
	ErrResetCardNotFound      = infraerrors.NotFound("RESET_CARD_NOT_FOUND", "reset card not found")
	ErrResetCardNoAvailable   = infraerrors.Conflict("NO_AVAILABLE_RESET_CARD", "no available reset card")
	ErrResetCardAlreadyUsed   = infraerrors.Conflict("RESET_CARD_ALREADY_USED", "reset card already used")
	ErrResetCardNotNeeded     = infraerrors.Conflict("RESET_CARD_NOT_NEEDED", "subscription weekly period does not need a reset now")
	ErrResetCardUnmetered     = infraerrors.BadRequest("RESET_CARD_UNMETERED", "subscription has no weekly limit; reset card not applicable")
	ErrResetCardInvalidExpiry = infraerrors.BadRequest("RESET_CARD_INVALID_EXPIRY", "expires_at must be in the future")
	ErrResetCardInvalidQty    = infraerrors.BadRequest("RESET_CARD_INVALID_QUANTITY", "quantity_per_user must be >= 1")
	ErrResetTargetEmpty       = infraerrors.BadRequest("RESET_TARGET_EMPTY", "selector matched no targets")
	ErrResetSelectorInvalid   = infraerrors.BadRequest("RESET_SELECTOR_INVALID", "selector target_mode is invalid or incomplete")
)

// ResetCard service 层视图。
type ResetCard struct {
	ID                 int64      `json:"id"`
	UserID             int64      `json:"user_id"`
	Status             string     `json:"status"`
	Scope              string     `json:"scope"`
	SourceType         string     `json:"source_type"`
	Campaign           *string    `json:"campaign"`
	GrantEventID       *int64     `json:"grant_event_id"`
	GrantIndex         int        `json:"grant_index"`
	GrantedAt          time.Time  `json:"granted_at"`
	ExpiresAt          *time.Time `json:"expires_at"`
	UsedAt             *time.Time `json:"used_at"`
	UsedSubscriptionID *int64     `json:"used_subscription_id"`
	CreatedBy          *int64     `json:"created_by"`
	Notes              string     `json:"notes"`
	CreatedAt          time.Time  `json:"created_at"`
	UpdatedAt          time.Time  `json:"updated_at"`
}

// ResetCardGrantSelector 定向发卡目标（按 User 去重）。
type ResetCardGrantSelector struct {
	// target_mode: users（user_ids 必填）/ groups（group_ids 必填）/ all_active_users
	Mode     string  `json:"target_mode"`
	UserIDs  []int64 `json:"user_ids,omitempty"`
	GroupIDs []int64 `json:"group_ids,omitempty"`
}

// GrantResetCardsInput 管理员发卡输入（durable 幂等：IdempotencyKey 必填）。
type GrantResetCardsInput struct {
	Selector        ResetCardGrantSelector
	QuantityPerUser int
	ExpiresAt       *time.Time
	Reason          string
	Campaign        string
	SourceType      string // 空 = admin_grant
	IdempotencyKey  string
	ActorAdminID    int64
}

// GrantResetCardsResult 发卡结果（含快照统计）。
type GrantResetCardsResult struct {
	EventID     int64
	UniqueUsers int
	TotalCards  int
	Replayed    bool
}

// ConsumeResetCardResult 消费结果。
type ConsumeResetCardResult struct {
	ShortPeriodEndsAt  *time.Time
	QuotaPolicy        string
	CardID             int64
	SubscriptionID     int64
	Applied            bool
	WeeklyPeriodEndsAt *time.Time
}

// GrantCardTarget 一张待发卡（去重后的 user + grant_index）。
type GrantCardTarget struct {
	UserID     int64
	GrantIndex int
}

// WeeklyResetCore Reset Core 的窄接口（由 *SubscriptionService 实现）。
type WeeklyResetCore interface {
	ResetSubscriptionWeeklyPeriod(ctx context.Context, input *WeeklyResetInput) (*WeeklyResetResult, error)
}

// ResetTargetResolver 定向解析（Direct Reset 与 Card Grant 共用）。
type ResetTargetResolver interface {
	// ResolveActiveMeteredSubscriptionIDs 解析 active + metered 的订阅 id 集合（snapshot）。
	ResolveActiveMeteredSubscriptionIDs(ctx context.Context, mode string, userIDs, groupIDs []int64) ([]int64, error)
	// ResolveActiveMeteredUserIDs 解析 active + metered 的去重用户集合。
	ResolveActiveMeteredUserIDs(ctx context.Context, mode string, userIDs, groupIDs []int64) ([]int64, error)
	// DescribeTargets 预览统计（订阅数 / 去重用户数 / 分组分布）。
	DescribeTargets(ctx context.Context, mode string, userIDs, groupIDs []int64) (*ResetTargetSummary, error)
}

// ResetTargetSummary preview 统计。
type ResetTargetSummary struct {
	TargetMode        string              `json:"target_mode"`
	SubscriptionCount int64               `json:"subscription_count"`
	UniqueUserCount   int64               `json:"unique_user_count"`
	GroupBreakdown    []GroupTargetStat   `json:"group_breakdown,omitempty"`
	Sample            []ResetTargetSample `json:"sample,omitempty"`
}

type GroupTargetStat struct {
	GroupID   int64  `json:"group_id"`
	Name      string `json:"name"`
	SubsCount int64  `json:"subscription_count"`
}

type ResetTargetSample struct {
	SubscriptionID int64  `json:"subscription_id"`
	UserID         int64  `json:"user_id"`
	DisplayName    string `json:"display_name"`
}

// ResetCardStore Reset Card 持久化端口。
type ResetCardOperation struct {
	Status string                  `json:"status"`
	Result *ConsumeResetCardResult `json:"result,omitempty"`
}

type ResetCardStore interface {
	LockOperation(context.Context, int64, int64, string, string) (*ResetCardOperation, error)
	CompleteOperation(context.Context, int64, string, *ConsumeResetCardResult) error
	CancelOperation(context.Context, int64, string) error
	SubscriptionResetCardReader
	GrantCards(ctx context.Context, eventID int64, targets []GrantCardTarget, expiresAt *time.Time, sourceType string, createdBy *int64, notes string) (int, error)
	GetAvailableForUserForUpdate(ctx context.Context, userID int64, now time.Time) (*ResetCard, error)
	MarkUsed(ctx context.Context, cardID, subscriptionID int64, usedAt time.Time) error
	Revoke(ctx context.Context, cardID int64, now time.Time) error
	ListByUser(ctx context.Context, userID int64, status *string, limit, offset int) ([]ResetCard, int, error)
	GetByID(ctx context.Context, cardID int64) (*ResetCard, error)
}

// ResetCardService Reset Card Runtime。
type ResetCardService struct {
	store     ResetCardStore
	subRepo   UserSubscriptionRepository
	groupRepo GroupRepository
	resetCore WeeklyResetCore
	targets   ResetTargetResolver
	entClient *dbent.Client
	now       func() time.Time
}

func NewResetCardService(
	store ResetCardStore,
	subRepo UserSubscriptionRepository,
	groupRepo GroupRepository,
	resetCore WeeklyResetCore,
	targets ResetTargetResolver,
	entClient *dbent.Client,
) *ResetCardService {
	return &ResetCardService{
		store:     store,
		subRepo:   subRepo,
		groupRepo: groupRepo,
		resetCore: resetCore,
		targets:   targets,
		entClient: entClient,
		now:       time.Now,
	}
}

// SetNow 供测试注入时钟。
func (s *ResetCardService) SetNow(now func() time.Time) { s.now = now }

// GrantResetCards 定向发卡（durable 幂等：同一 Idempotency-Key 重试绝不重复发卡）。
func (s *ResetCardService) GrantResetCards(ctx context.Context, in *GrantResetCardsInput) (*GrantResetCardsResult, error) {
	if in == nil || in.QuantityPerUser < 1 || in.QuantityPerUser > 1000 {
		return nil, ErrResetCardInvalidQty
	}
	if in.IdempotencyKey == "" {
		return nil, ErrIdempotencyKeyRequired
	}
	sourceType := in.SourceType
	if sourceType == "" {
		sourceType = domain.ResetCardSourceAdminGrant
	}

	coordinator := DefaultIdempotencyCoordinator()
	if coordinator == nil {
		return nil, errors.New("reset card: idempotency coordinator unavailable")
	}
	execRes, err := coordinator.Execute(ctx, IdempotencyExecuteOptions{
		Scope:          "admin.reset_card.grant",
		ActorScope:     fmt.Sprintf("admin:%d", in.ActorAdminID),
		Method:         "POST",
		Route:          "/admin/subscription-reset-cards/grants",
		IdempotencyKey: in.IdempotencyKey,
		Payload: map[string]any{
			"selector": in.Selector, "quantity_per_user": in.QuantityPerUser,
			"expires_at": in.ExpiresAt, "campaign": in.Campaign, "reason": in.Reason, "source_type": sourceType,
		},
		RequireKey: true,
	}, func(ctx context.Context) (any, error) {
		if in.ExpiresAt != nil && !in.ExpiresAt.After(s.now()) {
			return nil, ErrResetCardInvalidExpiry
		}
		return s.grantOnce(ctx, in, sourceType)
	})
	if err != nil {
		return nil, err
	}
	result, err := decodeSubscriptionOperationResult[GrantResetCardsResult](execRes.Data)
	if err != nil {
		return nil, err
	}
	result.Replayed = execRes.Replayed
	return result, nil
}

func (s *ResetCardService) grantOnce(ctx context.Context, in *GrantResetCardsInput, sourceType string) (*GrantResetCardsResult, error) {
	if s.entClient == nil || s.targets == nil {
		return nil, errors.New("reset card: storage unavailable")
	}
	// Snapshot：创建事件时固定目标用户（去重）；此后新购用户不自动获得本活动卡
	userIDs, err := s.targets.ResolveActiveMeteredUserIDs(ctx, in.Selector.Mode, in.Selector.UserIDs, in.Selector.GroupIDs)
	if err != nil {
		return nil, err
	}
	if len(userIDs) == 0 {
		return nil, ErrResetTargetEmpty
	}

	tx, err := s.entClient.Tx(ctx)
	if err != nil {
		return nil, err
	}
	defer func() { _ = tx.Rollback() }()
	txCtx := dbent.NewTxContext(ctx, tx)

	now := s.now()
	ev, err := tx.SubscriptionResetEvent.Create().
		SetEventType(domain.ResetEventTypeResetCardGrant).
		SetStatus(domain.ResetEventStatusCompleted).
		SetScopeType(resetScopeTypeFor(in.Selector.Mode)).
		SetScope(resetScopeJSON(in.Selector.Mode, in.Selector.UserIDs, in.Selector.GroupIDs)).
		SetEffectiveAt(now).
		SetNillableCampaign(strPtrIfSet(in.Campaign)).
		SetNillableCreatedBy(nonZero64Ptr(in.ActorAdminID)).
		SetReason(in.Reason).
		SetMetadata(map[string]any{"quantity_per_user": in.QuantityPerUser}).
		Save(txCtx)
	if err != nil {
		return nil, err
	}

	targets := make([]GrantCardTarget, 0, len(userIDs)*in.QuantityPerUser)
	for _, uid := range userIDs {
		for idx := 0; idx < in.QuantityPerUser; idx++ {
			targets = append(targets, GrantCardTarget{UserID: uid, GrantIndex: idx})
		}
	}
	granted, err := s.store.GrantCards(txCtx, ev.ID, targets, in.ExpiresAt, sourceType, nonZero64Ptr(in.ActorAdminID), in.Reason)
	if err != nil {
		return nil, err
	}
	if err := tx.Commit(); err != nil {
		return nil, err
	}
	return &GrantResetCardsResult{
		EventID:     ev.ID,
		UniqueUsers: len(userIDs),
		TotalCards:  granted,
	}, nil
}

// PreviewGrantTargets 发卡预览（无任何写入）。
func (s *ResetCardService) PreviewGrantTargets(ctx context.Context, selector ResetCardGrantSelector, quantityPerUser int) (*ResetTargetSummary, error) {
	if quantityPerUser < 1 || quantityPerUser > 1000 {
		return nil, ErrResetCardInvalidQty
	}
	summary, err := s.targets.DescribeTargets(ctx, selector.Mode, selector.UserIDs, selector.GroupIDs)
	if err != nil {
		return nil, err
	}
	summary.TargetMode = selector.Mode
	return summary, nil
}

// CountAvailableResetCards 管理端/状态端账户级可用卡计数（只读）。
func (s *ResetCardService) CountAvailableResetCards(ctx context.Context, userID int64) (int, error) {
	return s.store.CountAvailableResetCards(ctx, userID, s.now())
}

// ListResetCards 管理端按用户/状态列出卡（分页）。
func (s *ResetCardService) ListResetCards(ctx context.Context, userID int64, status *string, limit, offset int) ([]ResetCard, int, error) {
	if limit <= 0 {
		limit = 50
	}
	return s.store.ListByUser(ctx, userID, status, limit, offset)
}

// RevokeResetCard 撤销可用卡（used 禁止撤销；不物理删除，保留审计）。
func (s *ResetCardService) RevokeResetCard(ctx context.Context, cardID int64) error {
	return s.store.Revoke(ctx, cardID, s.now())
}

// ConsumeForSubscription 用户消费一张可用卡重置指定订阅的周周期（单事务：卡 CAS + Reset Core）。
func (s *ResetCardService) ConsumeForSubscription(ctx context.Context, userID, subscriptionID int64, idempotencyKey string, contractVersion ...int) (*ConsumeResetCardResult, error) {
	if idempotencyKey == "" {
		return nil, ErrIdempotencyKeyRequired
	}
	coordinator := DefaultIdempotencyCoordinator()
	if coordinator == nil {
		return nil, errors.New("reset card: idempotency coordinator unavailable")
	}
	execRes, err := coordinator.Execute(ctx, IdempotencyExecuteOptions{
		Scope:          "subscription.reset_card.consume",
		ActorScope:     fmt.Sprintf("user:%d", userID),
		Method:         "POST",
		Route:          "/subscriptions/:id/reset-with-card",
		IdempotencyKey: idempotencyKey,
		Payload:        map[string]any{"user_id": userID, "subscription_id": subscriptionID},
		RequireKey:     true,
	}, func(ctx context.Context) (any, error) {
		return s.consumeOnce(ctx, userID, subscriptionID, idempotencyKey, contractVersion...)
	})
	if err != nil {
		return nil, err
	}
	return decodeSubscriptionOperationResult[ConsumeResetCardResult](execRes.Data)
}

func (s *ResetCardService) consumeOnce(ctx context.Context, userID, subscriptionID int64, key string, contractVersion ...int) (*ConsumeResetCardResult, error) {
	if s.entClient == nil {
		return nil, errors.New("reset card: storage unavailable")
	}
	tx, err := s.entClient.Tx(ctx)
	if err != nil {
		return nil, err
	}
	defer func() { _ = tx.Rollback() }()
	txCtx := dbent.NewTxContext(ctx, tx)

	now := s.now()

	// 1) 订阅行锁 + 归属/状态/metered 校验
	sub, err := s.subRepo.GetByIDForUpdate(txCtx, subscriptionID)
	if err != nil {
		return nil, ErrSubscriptionNotFound
	}
	if sub.UserID != userID {
		return nil, ErrSubscriptionNotFound // 不向越权者泄露存在性
	}
	operation, err := s.store.LockOperation(txCtx, userID, subscriptionID, key, "pending")
	if err != nil {
		return nil, err
	}
	if operation.Status == "succeeded" {
		return operation.Result, nil
	}
	if operation.Status == "cancelled" {
		return nil, infraerrors.Conflict("RESET_OPERATION_CANCELLED", "reset operation was reconciled as not executed")
	}
	if sub.Status != SubscriptionStatusActive || !sub.ExpiresAt.After(now) {
		return nil, ErrSubscriptionExpired
	}
	group, err := s.groupRepo.GetByID(txCtx, sub.GroupID)
	if err != nil {
		return nil, err
	}
	if group == nil || !group.HasWeeklyLimit() {
		return nil, ErrResetCardUnmetered
	}

	if group.UsesDualWindows() && (len(contractVersion) == 0 || contractVersion[0] < 2) {
		return nil, infraerrors.Conflict("CLIENT_UPGRADE_REQUIRED", "请更新客户端后使用双周期重置卡")
	}
	// 2) FIFO 锁定最早可用卡（最早到期 → 最早创建 → 最小 id）
	card, err := s.store.GetAvailableForUserForUpdate(txCtx, userID, now)
	if err != nil {
		return nil, err
	}

	// 3) 卡 CAS available → used
	if err := s.store.MarkUsed(txCtx, card.ID, subscriptionID, now); err != nil {
		return nil, err
	}

	// 4) 复用 Reset Core（禁止第二次实现 weekly_usage=0）
	result, err := s.resetCore.ResetSubscriptionWeeklyPeriod(txCtx, &WeeklyResetInput{
		UserSubscriptionID: subscriptionID,
		EffectiveAt:        now,
		Source:             domain.WeeklyResetSourceResetCard,
		DualWindows:        group.UsesDualWindows(),
		ResetCardID:        &card.ID,
	})
	if err != nil {
		return nil, err
	}
	if result.Status != WeeklyResetApplied {
		// stale（锚点已 >= now）：整体回滚，卡不白烧
		return nil, ErrResetCardNotNeeded
	}
	var periodEnds *time.Time
	if result.Subscription != nil {
		periodEnds = result.Subscription.WeeklyResetTime()
	}
	receipt := &ConsumeResetCardResult{CardID: card.ID, SubscriptionID: subscriptionID, Applied: true, WeeklyPeriodEndsAt: periodEnds}
	receipt.QuotaPolicy = group.QuotaPolicy
	if group.UsesDualWindows() {
		end := now.Add(5 * time.Hour)
		if end.After(sub.ExpiresAt) {
			end = sub.ExpiresAt
		}
		receipt.ShortPeriodEndsAt = &end
	}
	if receipt.WeeklyPeriodEndsAt != nil && receipt.WeeklyPeriodEndsAt.After(sub.ExpiresAt) {
		receipt.WeeklyPeriodEndsAt = &sub.ExpiresAt
	}
	if err := s.store.CompleteOperation(txCtx, userID, key, receipt); err != nil {
		return nil, err
	}
	if err := tx.Commit(); err != nil {
		return nil, err
	}
	return receipt, nil
}

// resetScopeTypeFor：selector mode → events.scope_type
func resetScopeTypeFor(mode string) string {
	switch mode {
	case "users":
		return "user"
	case "groups":
		return "group"
	case "all_active_users", "all_active":
		return "all"
	default:
		return "user"
	}
}

// resetScopeJSON：selector → events.scope JSONB（确定性解释，见 migration 239 注释）
func resetScopeJSON(mode string, userIDs, groupIDs []int64) map[string]any {
	switch mode {
	case "users":
		return map[string]any{"user_ids": userIDs}
	case "groups":
		return map[string]any{"group_ids": groupIDs}
	default:
		return map[string]any{}
	}
}

func strPtrIfSet(s string) *string {
	if s == "" {
		return nil
	}
	return &s
}

func nonZero64Ptr(v int64) *int64 {
	if v == 0 {
		return nil
	}
	return &v
}

// ReconcileResetOperation also fences a never-executed v2 operation. A delayed POST
// with that key can then never consume a card after the client creates its next key.
func (s *ResetCardService) ReconcileResetOperation(ctx context.Context, userID, subscriptionID int64, key string) (*ResetCardOperation, error) {
	return s.resolveResetOperation(ctx, userID, subscriptionID, key, false)
}
func (s *ResetCardService) PrepareResetOperation(ctx context.Context, userID, subscriptionID int64, key string) (*ResetCardOperation, error) {
	return s.resolveResetOperation(ctx, userID, subscriptionID, key, true)
}
func (s *ResetCardService) resolveResetOperation(ctx context.Context, userID, subscriptionID int64, key string, prepare bool) (*ResetCardOperation, error) {
	if key == "" || len(key) > 128 {
		return nil, ErrIdempotencyKeyRequired
	}
	if s.entClient == nil {
		return nil, errors.New("reset card storage unavailable")
	}
	tx, err := s.entClient.Tx(ctx)
	if err != nil {
		return nil, err
	}
	defer func() { _ = tx.Rollback() }()
	txCtx := dbent.NewTxContext(ctx, tx)
	sub, err := s.subRepo.GetByIDForUpdate(txCtx, subscriptionID)
	if err != nil || sub == nil || sub.UserID != userID {
		return nil, ErrSubscriptionNotFound
	}
	initial := ""
	if strings.HasPrefix(key, "reset-v2-") {
		initial = "cancelled"
		if prepare {
			initial = "pending"
		}
	}
	operation, err := s.store.LockOperation(txCtx, userID, subscriptionID, key, initial)
	if err != nil {
		return nil, err
	}
	// Legacy clients lack durable receipts, but an intact authenticated cached
	// success can still be recovered. Absence/expiry alone never proves non-execution.
	if operation.Status == "unknown" {
		coordinator := DefaultIdempotencyCoordinator()
		if coordinator != nil && coordinator.repo != nil {
			record, lookupErr := coordinator.repo.GetByScopeAndKeyHash(txCtx, "subscription.reset_card.consume", HashIdempotencyKey(key))
			if lookupErr != nil {
				return nil, lookupErr
			}
			fingerprint, fingerprintErr := BuildIdempotencyFingerprint("POST", "/subscriptions/:id/reset-with-card", fmt.Sprintf("user:%d", userID), map[string]any{"user_id": userID, "subscription_id": subscriptionID})
			if fingerprintErr != nil {
				return nil, fingerprintErr
			}
			if record != nil && record.RequestFingerprint == fingerprint && record.Status == IdempotencyStatusSucceeded {
				data, decodeErr := coordinator.decodeStoredResponse(record.ResponseBody)
				if decodeErr != nil {
					return nil, decodeErr
				}
				result, decodeErr := decodeSubscriptionOperationResult[ConsumeResetCardResult](data)
				if decodeErr != nil {
					return nil, decodeErr
				}
				if _, err := s.store.LockOperation(txCtx, userID, subscriptionID, key, "pending"); err != nil {
					return nil, err
				}
				if err := s.store.CompleteOperation(txCtx, userID, key, result); err != nil {
					return nil, err
				}
				operation = &ResetCardOperation{Status: "succeeded", Result: result}
			}
		}
	}
	if !prepare && operation.Status == "pending" {
		if err := s.store.CancelOperation(txCtx, userID, key); err != nil {
			return nil, err
		}
		operation.Status = "cancelled"
	}
	if err := tx.Commit(); err != nil {
		return nil, err
	}
	return operation, nil
}
