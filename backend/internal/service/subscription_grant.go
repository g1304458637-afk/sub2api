package service

import (
	"context"
	"time"

	dbent "github.com/Wei-Shaw/sub2api/ent"
	"github.com/Wei-Shaw/sub2api/internal/domain"
	infraerrors "github.com/Wei-Shaw/sub2api/internal/pkg/errors"
)

// =============================================================================
// Subscription Grant（订阅权益发放台账）
//
// Grant 回答「为什么、由谁、通过什么活动给了用户一份订阅权益」；
// user_subscriptions 回答「用户现在实际拥有什么订阅」。二者概念分离：
// 所有来源（学生认证、管理员赠送、未来的活动/邀请/补偿）都必须经过
// SubscriptionGrantService → SubscriptionService（AssignOrExtendSubscription），
// 绝不允许来源模块自己 INSERT user_subscriptions。
// =============================================================================

// SubscriptionGrant 台账记录（subscription_grants 表的一行）。
type SubscriptionGrant struct {
	ID                   int64
	UserID               int64
	GroupID              int64
	PlanID               *int64
	Source               string
	SourceKey            *string
	BenefitCode          *string
	IdentityType         *string
	IdentityKey          *string
	IdempotencyKey       *string
	Status               string
	EffectivePolicy      string
	DurationDays         int
	Reason               *string
	Notes                *string
	OperatorUserID       *int64
	LinkedSubscriptionID *int64
	ContributionStart    *time.Time
	ContributionEnd      *time.Time
	ActivatedAt          *time.Time
	RevokedAt            *time.Time
	RevokedBy            *int64
	RevokeReason         *string
	FailureReason        *string
	CreatedAt            time.Time
	UpdatedAt            time.Time

	// prevExpiresCache 激活前同组订阅的到期时刻（进程内推导 outcome 用，不落库；
	// 同一事实在台账列 contribution_start 已有承载）。
	prevExpiresCache *time.Time
}

// BenefitClaim 一次性权益领取记录（benefit_claims 表的一行）。
// claim 是永久事实：即使 Grant 被撤销，领取资格也已消耗，不得重新领取。
type BenefitClaim struct {
	ID           int64
	BenefitCode  string
	UserID       int64
	IdentityType string
	IdentityKey  string
	GrantID      *int64
	CreatedAt    time.Time
}

// CreateSubscriptionGrantCommand 发放命令。
//   - BenefitCode 非空 = 一次性权益：必须携带 IdentityType/IdentityKey，
//     由 benefit_claims 双唯一约束锁定"一人一号一权益"；
//   - SourceKey 非空 = 业务幂等键（subscription_grants 唯一部分索引兜底）；
//   - 管理员单次赠送两者皆可空（API 层 IdempotencyCoordinator 防重复提交）。
type CreateSubscriptionGrantCommand struct {
	UserID          int64
	GroupID         int64
	PlanID          *int64
	Source          string
	SourceKey       string
	BenefitCode     string
	IdentityType    string
	IdentityKey     string
	IdempotencyKey  string
	EffectivePolicy string
	DurationDays    int
	Reason          string
	Notes           string
	OperatorUserID  *int64
	// PendingFallback 学生认证等自动触发场景：跨组冲突时不报错，转入 pending
	// 在当前订阅结束后衔接（安全策略）；管理员赠送保持 false，由预览接口引导。
	PendingFallback bool
}

// GrantOutcomeAction 发放动作结果。
const (
	GrantOutcomeActivatedNew   = "activated_new"   // 新建订阅并激活
	GrantOutcomeExtended       = "extended"        // 同组已有订阅，从 expires_at 顺延
	GrantOutcomePending        = "pending"         // 跨组冲突，转入待生效
	GrantOutcomeAlreadyGranted = "already_granted" // 幂等命中：之前已发放过
)

// GrantOutcome 一次发放的执行结果（前端预览/结果展示用）。
type GrantOutcome struct {
	Action          string     `json:"action"`
	GrantID         int64      `json:"grant_id"`
	SubscriptionID  int64      `json:"subscription_id"`
	PreviousExpires *time.Time `json:"previous_expires,omitempty"`
	ExpiresAt       *time.Time `json:"expires_at,omitempty"`
	PlanName        string     `json:"plan_name,omitempty"`
	Message         string     `json:"message,omitempty"`
}

// Granted reports whether the user's subscription was (or had already been)
// activated or extended by this grant at any point in time.
func (o *GrantOutcome) Granted() bool {
	return o != nil && (o.Action == GrantOutcomeActivatedNew ||
		o.Action == GrantOutcomeExtended ||
		o.Action == GrantOutcomeAlreadyGranted)
}

// GrantAdminFilter 管理端台账筛选。
type GrantAdminFilter struct {
	Page        int
	PageSize    int
	UserID      *int64
	Source      string
	Status      string
	BenefitCode string
	GroupID     *int64
}

// GrantAdminItem 管理端台账列表项（回填用户与操作人邮箱）。
type GrantAdminItem struct {
	SubscriptionGrant
	UserEmail     string `json:"user_email"`
	Username      string `json:"username"`
	OperatorEmail string `json:"operator_email"`
	GroupName     string `json:"group_name"`
}

// GrantAdminList 台账分页结果。
type GrantAdminList struct {
	Items    []GrantAdminItem `json:"items"`
	Total    int              `json:"total"`
	Page     int              `json:"page"`
	PageSize int              `json:"page_size"`
}

// GrantPreview 发放预览（管理员确认前看到将会发生什么）。
type GrantPreview struct {
	// Outcome: will_activate_new / will_extend / will_be_pending / conflict
	Outcome         string     `json:"outcome"`
	CurrentGroupID  *int64     `json:"current_group_id,omitempty"`
	CurrentPlanName string     `json:"current_plan_name,omitempty"`
	CurrentExpires  *time.Time `json:"current_expires,omitempty"`
	// PredictedExpires 立即生效/顺延时的预计到期时间；pending 为 nil。
	PredictedExpires *time.Time `json:"predicted_expires,omitempty"`
	// ExtensionBase 顺延时基于哪个时刻累加（当前订阅 expires_at）。
	ExtensionBase *time.Time `json:"extension_base,omitempty"`
	Message       string     `json:"message,omitempty"`
}

// 预览结论常量。
const (
	GrantPreviewWillActivateNew = "will_activate_new"
	GrantPreviewWillExtend      = "will_extend"
	GrantPreviewWillBePending   = "will_be_pending"
	GrantPreviewConflict        = "conflict"
)

// GrantSubscriptionAssigner 订阅激活原语（由 *SubscriptionService 实现）。
// Grant 层不直接改写 user_subscriptions，统一复用 Assign-or-Extend 语义：
// 同组已有订阅 → 从 expires_at 顺延（过期则从 now 重算并激活）；
// 无订阅 → 创建；其他组已有 active 主订阅 → ErrPrimarySubscriptionExists。
type GrantSubscriptionAssigner interface {
	AssignOrExtendSubscriptionDeferredCache(ctx context.Context, input *AssignSubscriptionInput) (*UserSubscription, bool, error)
	InvalidateSubCache(userID, groupID int64)
}

var (
	ErrGrantInvalidCommand = infraerrors.BadRequest("GRANT_INVALID_COMMAND", "invalid subscription grant command")
	// ErrGrantConflict 目标用户存在其他分组的有效主订阅，立即生效会覆盖/降级
	// 现有付费权益（RULE 1）。管理员可改选 end_of_term。
	ErrGrantConflict = infraerrors.Conflict("GRANT_CONFLICT", "user has an active subscription on another group; choose end-of-term scheduling instead")
	// ErrBenefitAlreadyClaimed 一次性权益已被领取（同一验证邮箱或同一用户）。
	ErrBenefitAlreadyClaimed = infraerrors.Conflict("BENEFIT_ALREADY_CLAIMED", "this benefit has already been claimed")
	// ErrGrantNotRevokable 台账当前状态不可撤销。
	ErrGrantNotRevokable = infraerrors.BadRequest("GRANT_NOT_REVOKABLE", "grant cannot be revoked in its current state")
	// ErrGrantNotFound 台账记录不存在。
	ErrGrantNotFound = infraerrors.NotFound("GRANT_NOT_FOUND", "subscription grant not found")
)

func validateCreateSubscriptionGrantCommand(cmd *CreateSubscriptionGrantCommand) error {
	switch {
	case cmd.UserID <= 0:
		return ErrGrantInvalidCommand
	case cmd.GroupID <= 0:
		return ErrGrantInvalidCommand
	case !domain.ValidSubscriptionGrantSources[cmd.Source]:
		return ErrGrantInvalidCommand
	case cmd.DurationDays <= 0 || cmd.DurationDays > MaxValidityDays:
		return ErrGrantInvalidCommand
	case cmd.EffectivePolicy != domain.SubscriptionGrantPolicyImmediate && cmd.EffectivePolicy != domain.SubscriptionGrantPolicyEndOfTerm:
		return ErrGrantInvalidCommand
	case cmd.BenefitCode != "" && cmd.IdentityKey == "":
		return ErrGrantInvalidCommand
	default:
		return nil
	}
}

// SubscriptionGrantRepository 台账仓储（裸 SQL；经 clientFromContext 感知外层事务）。
type SubscriptionGrantRepository interface {
	// InsertIdempotent INSERT ... ON CONFLICT (source, source_key) DO NOTHING。
	// 返回 false 表示同 (source, source_key) 已发放过（幂等命中）。
	InsertIdempotent(ctx context.Context, grant *SubscriptionGrant) (bool, error)
	GetByID(ctx context.Context, id int64) (*SubscriptionGrant, error)
	GetBySourceKey(ctx context.Context, source, sourceKey string) (*SubscriptionGrant, error)
	// GetForUpdate 行锁读取（撤销/激活竞态防护）。
	GetForUpdate(ctx context.Context, id int64) (*SubscriptionGrant, error)
	// UpdateActivation 激活落库（status/contribution/link）。
	UpdateActivation(ctx context.Context, id int64, update GrantActivationUpdate) error
	// MarkRevoked 撤销落库。
	MarkRevoked(ctx context.Context, id int64, revokedBy int64, reason string) error
	MarkFailed(ctx context.Context, id int64, reason string) error
	// ListPending 分页取待激活台账（created_at 升序，FIFO 衔接）。
	ListPending(ctx context.Context, limit int) ([]*SubscriptionGrant, error)
	// ExpireFulfilledContribution 贡献期已结束的 fulfilled → expired（返回行数）。
	ExpireFulfilledContribution(ctx context.Context, now time.Time) (int64, error)
	// PaidFloorForSubscription 付费地板：MAX(term_end) over paid sources。
	PaidFloorForSubscription(ctx context.Context, subscriptionID int64) (*time.Time, error)
	// OtherGrantFloorForSubscription 同订阅上其他 Grant 的贡献终点地板。
	OtherGrantFloorForSubscription(ctx context.Context, subscriptionID, excludeGrantID int64) (*time.Time, error)
	// AdminList 管理端分页查询（回填用户/操作人邮箱与分组名）。
	AdminList(ctx context.Context, filter *GrantAdminFilter) (*GrantAdminList, error)
	// ListByUser 用户维度台账（个人/管理端抽屉用）。
	ListByUser(ctx context.Context, userID int64, limit int) ([]*SubscriptionGrant, error)
}

// GrantActivationUpdate 激活落库内容。
type GrantActivationUpdate struct {
	Status               string
	LinkedSubscriptionID int64
	ContributionStart    time.Time
	ContributionEnd      time.Time
	ActivatedAt          time.Time
}

// BenefitClaimRepository 一次性权益领取仓储。
type BenefitClaimRepository interface {
	// InsertIdempotent INSERT ... ON CONFLICT DO NOTHING（双唯一约束：
	// (benefit_code, identity_key) 与 (benefit_code, user_id)）。
	// 返回 false 表示该权益已被领取。
	InsertIdempotent(ctx context.Context, claim *BenefitClaim) (bool, error)
	// GetByBenefitAndIdentity / GetByBenefitAndUser 幂等命中后回查（区分冲突维度）。
	GetByBenefitAndIdentity(ctx context.Context, benefitCode, identityKey string) (*BenefitClaim, error)
	GetByBenefitAndUser(ctx context.Context, benefitCode string, userID int64) (*BenefitClaim, error)
}

// StudentVerificationRepository 学生认证记录仓储。
// 认证记录是审计事实（允许同一用户多次认证留痕），不做唯一约束；
// 「一人一号一权益」由 benefit_claims 双唯一约束兜底。
type StudentVerificationRepository interface {
	Insert(ctx context.Context, v *StudentVerification) (int64, error)
	GetByID(ctx context.Context, id int64) (*StudentVerification, error)
	LatestVerifiedByUser(ctx context.Context, userID int64) (*StudentVerification, error)
	// LinkBenefitGrant 回填触发的权益台账 ID。
	LinkBenefitGrant(ctx context.Context, id, grantID int64) error
	// RevokeByUser 管理员撤销某用户的学生认证（仅状态回写，不动 Grant）。
	RevokeByUser(ctx context.Context, userID int64, revokedBy int64, reason string) (int64, error)
	AdminList(ctx context.Context, filter *StudentVerificationAdminFilter) (*StudentVerificationAdminList, error)
}

// StudentVerification 学生认证记录。
type StudentVerification struct {
	ID             int64
	UserID         int64
	Provider       string
	Email          string
	Status         string
	BenefitGrantID *int64
	IP             string
	UserAgent      string
	Notes          *string
	VerifiedAt     time.Time
	RevokedAt      *time.Time
	RevokedBy      *int64
	CreatedAt      time.Time
}

// StudentVerificationAdminFilter 管理端认证记录筛选。
type StudentVerificationAdminFilter struct {
	Page     int
	PageSize int
	UserID   *int64
	Provider string
	Status   string
	Email    string
}

// StudentVerificationAdminItem 管理端认证记录列表项。
type StudentVerificationAdminItem struct {
	StudentVerification
	UserEmail string `json:"user_email"`
	Username  string `json:"username"`
}

// StudentVerificationAdminList 管理端认证记录分页结果。
type StudentVerificationAdminList struct {
	Items    []StudentVerificationAdminItem `json:"items"`
	Total    int                            `json:"total"`
	Page     int                            `json:"page"`
	PageSize int                            `json:"page_size"`
}

// GrantServiceDependencies Grant 服务依赖（wire 注入 + 单测替换）。
type GrantServiceDependencies struct {
	EntClient           *dbent.Client
	GrantRepo           SubscriptionGrantRepository
	ClaimRepo           BenefitClaimRepository
	UserRepo            UserRepository
	GroupRepo           GroupRepository
	Assigner            GrantSubscriptionAssigner
	BillingCacheService *BillingCacheService
}
