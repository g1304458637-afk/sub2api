package service

// Subscription V1 Phase 10 —— Plan Change Runtime（Upgrade / Scheduled Downgrade）。
//
// Reference Decision（Phase 10 Reference Gate，详见 docs/SUBSCRIPTION_BACKEND_REFERENCE_LOG.md）：
//   - Stripe `proration_behavior=always_invoice`：升级立即生效 + 未用时间 credit +
//     立即开票（对应本实现：Quote 冻结 → 支付 → 立即履约）；
//   - Stripe pending_update / Chargebee end_of_term：降级不立即执行，term 末生效
//     （对应：next_plan_id + renewal 时按目标档续费）；
//   - Chargebee update_subscription_estimate：服务端权威报价预览；
//   - OpenMeter grants / Lago 新订阅+钱包分离：entitlement 切换不触碰 Wallet。
//   不采用：客户端提交金额；用目录价充当历史实付（Gate 2）；平均化多段预付（Gate 3）。
//
// 四闸门：
//   Gate 1 plan identity：sub.plan_id 为 NULL → plan_identity_unresolved，Preview 拒绝；
//     新购买/续费/升级履约写入 plan_id 与 subscription_terms。
//   Gate 2 price truth：credit 基于该段实付（terms.price_paid），非目录价。
//   Gate 3 prepaid segments：按未消费 term 逐段折算（同价段合并结果等价）。
//   Gate 4 tier：plans.tier_rank 判定升/降级；0 档位拒绝。

import (
	"context"
	"crypto/sha256"
	"errors"
	"fmt"
	"math"
	"strings"
	"time"

	"github.com/shopspring/decimal"

	dbent "github.com/Wei-Shaw/sub2api/ent"
	"github.com/Wei-Shaw/sub2api/ent/usersubscription"
	infraerrors "github.com/Wei-Shaw/sub2api/internal/pkg/errors"
)

// Plan Change 业务错误。
var (
	ErrPlanChangeNotFound        = infraerrors.NotFound("PLAN_CHANGE_NOT_FOUND", "plan change not found")
	ErrPlanIdentityUnresolved    = infraerrors.Conflict("PLAN_IDENTITY_UNRESOLVED", "subscription has no reliable plan identity; admin resolve required")
	ErrPlanTierNotConfigured     = infraerrors.Conflict("PLAN_TIER_NOT_CONFIGURED", "plan tier_rank is not configured")
	ErrPlanNoChange              = infraerrors.Conflict("PLAN_NO_CHANGE", "target plan equals current plan")
	ErrPlanTargetGroupActive     = infraerrors.Conflict("PLAN_TARGET_GROUP_ACTIVE", "target plan group already has an active subscription")
	ErrPlanCrossCurrency         = infraerrors.BadRequest("PLAN_CROSS_CURRENCY_UNSUPPORTED", "cross-currency plan change is unsupported in V1")
	ErrPlanTermMismatch          = infraerrors.BadRequest("PLAN_TERM_MISMATCH", "different billing durations are unsupported in V1")
	ErrPlanQuoteExpired          = infraerrors.Conflict("PLAN_QUOTE_EXPIRED", "quote expired; re-quote required")
	ErrPlanQuoteStatusInvalid    = infraerrors.Conflict("PLAN_QUOTE_STATUS_INVALID", "plan change is not in a payable state")
	ErrPlanDowngradeNotTierOrder = infraerrors.BadRequest("PLAN_CHANGE_NOT_DOWNGRADE", "target tier is not lower than current tier")
)

// PlanChangeQuote 服务端权威报价（可返回给用户；含金额但不泄露内部 USD quota）。
type PlanChangeQuote struct {
	ShortRemainingPercentBefore *float64 `json:"short_remaining_percent_before,omitempty"`
	ShortRemainingPercentAfter  *float64 `json:"short_remaining_percent_after,omitempty"`
	WeeklyRemainingPercentAfter *float64 `json:"weekly_remaining_percent_after,omitempty"`

	SubscriptionID           int64       `json:"subscription_id"`
	ChangeType               string      `json:"change_type"` // upgrade / scheduled_downgrade
	FromPlanID               int64       `json:"from_plan_id"`
	ToPlanID                 int64       `json:"to_plan_id"`
	FromDisplayName          string      `json:"from_display_name"`
	ToDisplayName            string      `json:"to_display_name"`
	EffectiveAt              time.Time   `json:"effective_at"`
	CurrentExpiry            time.Time   `json:"current_expiry"`
	NewExpiry                time.Time   `json:"new_expiry"` // 升级=不变；降级=term 末
	RemainingSeconds         int64       `json:"remaining_seconds"`
	UnusedCredit             float64     `json:"unused_credit,string"`
	ProratedCharge           float64     `json:"prorated_charge,string"`
	AmountDue                float64     `json:"amount_due,string"`
	Currency                 string      `json:"currency"`
	WeeklyUsagePercentBefore *int        `json:"weekly_usage_percent_before"`
	WeeklyUsagePercentAfter  *int        `json:"weekly_usage_percent_after"` // 升级后按新额度重算（usage 不变）
	UsageStatusAfter         UsageStatus `json:"usage_status_after"`
	KeysToMigrateCount       int64       `json:"keys_to_migrate_count"`
}

// PlanChangeRecord 服务层视图。
type PlanChangeRecord struct {
	ID               int64
	UserID           int64
	SubscriptionID   int64
	ChangeType       string
	FromPlanID       *int64
	ToPlanID         int64
	FromGroupID      *int64
	ToGroupID        int64
	FromTier         int
	ToTier           int
	OldPriceSnapshot *float64
	NewPriceSnapshot float64
	Currency         string
	TermStart        *time.Time
	TermEnd          *time.Time
	RemainingSeconds int64
	UnusedCredit     float64
	ProratedCharge   float64
	AmountDue        float64
	QuoteCreatedAt   *time.Time
	QuoteExpiresAt   *time.Time
	EffectiveAt      *time.Time
	Status           string
	CancelReason     *string
	OrderID          *int64
	IdempotencyKey   *string
	PaidAt           *time.Time
	FulfilledAt      *time.Time
	CancelledAt      *time.Time
	CreatedAt        time.Time
	UpdatedAt        time.Time
}

// PlanChangeStore 持久化端口。
type PlanChangeStore interface {
	// CreateQuote 落报价行（quoted）。
	CreateQuote(ctx context.Context, rec *PlanChangeRecord) (int64, error)
	// GetByID 取审计行。
	GetByID(ctx context.Context, id int64) (*PlanChangeRecord, error)
	// GetByOrder 按 payment order 取（履约幂等入口）。
	GetByOrder(ctx context.Context, orderID int64) (*PlanChangeRecord, error)
	// MarkPendingPayment / MarkPaid / MarkFulfilled / Cancel 状态迁移（CAS）。
	MarkPendingPayment(ctx context.Context, id, orderID int64) error
	MarkPaid(ctx context.Context, id int64) error
	MarkFulfilled(ctx context.Context, id int64, effectiveAt time.Time) error
	Cancel(ctx context.Context, id int64, reason string) error
	// ActiveScheduledChange 订阅的 pending scheduled change（最多一个）。
	ActiveScheduledChange(ctx context.Context, subscriptionID int64) (*PlanChangeRecord, error)
	// ListBySubscription 审计历史。
	ListBySubscription(ctx context.Context, subscriptionID int64, limit int) ([]PlanChangeRecord, error)

	// ListAll 管理端审计：按 id 倒序 + 总数；过滤条件均可选。
	ListAll(ctx context.Context, userID, subscriptionID *int64, status *string, limit, offset int) ([]PlanChangeRecord, int64, error)
}

// TermStore 已付 term 快照端口。
type TermStore interface {
	// UnconsumedTerms now 之后仍未消费的 term（按 term_end 升序）。
	UnconsumedTerms(ctx context.Context, subscriptionID int64, now time.Time) ([]SubscriptionTermRecord, error)
	// RecordTerm 记录一段已付事实（购买/续费/升级/管理员分配）。
	RecordTerm(ctx context.Context, term *SubscriptionTermRecord) error
}

// SubscriptionTermRecord term 行。
type SubscriptionTermRecord struct {
	SubscriptionID int64
	OrderID        *int64
	PlanID         *int64
	PricePaid      float64
	Currency       string
	Days           int
	TermStart      time.Time
	TermEnd        time.Time
	Source         string
}

// PlanService Plan 读取端口（窄接口，避免依赖 payment 配置服务）。
type PlanService interface {
	GetPlan(ctx context.Context, planID int64) (*PlanSnapshot, error)
}

// PlanSnapshot Plan Change 所需的 SKU 快照。
type PlanSnapshot struct {
	ID           int64
	GroupID      int64
	Name         string
	Price        float64
	Currency     string
	ValidityDays int
	ValidityUnit string
	TierRank     int
	ForSale      bool
}

// APIKeyGroupMigrator 升级时把旧组 Key 迁到新组（同事务）。
type APIKeyGroupMigrator interface {
	// MigrateGroupForUser 将 user 名下 fromGroup 的全部 Key 迁到 toGroup；返回迁移条数。
	MigrateGroupForUser(ctx context.Context, userID, fromGroupID, toGroupID int64) (int64, error)
	CountByUserAndGroup(ctx context.Context, userID, groupID int64) (int64, error)
}

// PlanChangeService Plan Change Runtime。
type PlanChangeService struct {
	store     PlanChangeStore
	terms     TermStore
	plans     PlanService
	subRepo   UserSubscriptionRepository
	groupRepo GroupRepository
	apiKeys   APIKeyGroupMigrator
	status    *AccountStatusService
	entClient *dbent.Client
	now       func() time.Time
	quoteTTL  time.Duration

	// cacheInvalidator 提交后缓存失效回调（SubscriptionService 提供；nil = no-op）。
	cacheInvalidator func(userID, groupID int64)
}

func NewPlanChangeService(
	store PlanChangeStore,
	terms TermStore,
	plans PlanService,
	subRepo UserSubscriptionRepository,
	groupRepo GroupRepository,
	apiKeys APIKeyGroupMigrator,
	status *AccountStatusService,
	entClient *dbent.Client,
) *PlanChangeService {
	return &PlanChangeService{
		store: store, terms: terms, plans: plans, subRepo: subRepo,
		groupRepo: groupRepo, apiKeys: apiKeys, status: status,
		entClient: entClient, now: time.Now, quoteTTL: 30 * time.Minute,
	}
}

// SetNow 供测试注入。
func (s *PlanChangeService) SetNow(now func() time.Time) { s.now = now }

// SetCacheInvalidator wire 注入提交后缓存失效回调（替代旧 no-op 占位）。
func (s *PlanChangeService) SetCacheInvalidator(invalidator func(userID, groupID int64)) {
	s.cacheInvalidator = invalidator
}

func (s *PlanChangeService) invalidateCaches(ctx context.Context, userID, groupID int64) {
	if s.cacheInvalidator != nil {
		s.cacheInvalidator(userID, groupID)
	}
}

// ------------------------------------------------------------------
// Quote（服务端权威；Preview 与 Create 共用同一计算）
// ------------------------------------------------------------------

// PreviewUpgrade 生成升级报价（纯读；不落库）。
func (s *PlanChangeService) PreviewUpgrade(ctx context.Context, userID, subscriptionID, targetPlanID int64) (*PlanChangeQuote, error) {
	quote, _, err := s.buildUpgradeQuote(ctx, userID, subscriptionID, targetPlanID)
	if err != nil {
		return nil, err
	}
	return quote, nil
}

// CreateUpgradeQuote 报价落库冻结（quoted；后续创建订单只能引用此行金额）。
func (s *PlanChangeService) CreateUpgradeQuote(ctx context.Context, userID, subscriptionID, targetPlanID int64, idempotencyKey string) (*PlanChangeQuote, int64, error) {
	if strings.TrimSpace(idempotencyKey) != "" {
		digest := sha256.Sum256([]byte(idempotencyKey))
		idempotencyKey = fmt.Sprintf("user:%d:upgrade:%x", userID, digest)
		if rec, err := s.findUpgradeReplay(ctx, idempotencyKey, userID, subscriptionID, targetPlanID); err != nil {
			return nil, 0, err
		} else if rec != nil {
			return frozenUpgradeQuote(rec), rec.ID, nil
		}
	}

	quote, basis, err := s.buildUpgradeQuote(ctx, userID, subscriptionID, targetPlanID)
	if err != nil {
		return nil, 0, err
	}
	now := s.now()
	expiresAt := now.Add(s.quoteTTL)
	fromPlan := basis.fromPlan
	rec := &PlanChangeRecord{
		UserID:           userID,
		SubscriptionID:   subscriptionID,
		ChangeType:       "upgrade",
		FromPlanID:       &fromPlan.ID,
		ToPlanID:         targetPlanID,
		FromGroupID:      &basis.sub.GroupID,
		ToGroupID:        basis.toPlan.GroupID,
		FromTier:         fromPlan.TierRank,
		ToTier:           basis.toPlan.TierRank,
		OldPriceSnapshot: &basis.blendedOldPrice,
		NewPriceSnapshot: basis.toPlan.Price,
		Currency:         basis.currency,
		TermStart:        &basis.windowStart,
		TermEnd:          &basis.windowEnd,
		RemainingSeconds: quote.RemainingSeconds,
		UnusedCredit:     quote.UnusedCredit,
		ProratedCharge:   quote.ProratedCharge,
		AmountDue:        quote.AmountDue,
		QuoteCreatedAt:   &now,
		QuoteExpiresAt:   &expiresAt,
		IdempotencyKey:   nilIfEmpty(idempotencyKey),
		Status:           "quoted",
	}
	id, err := s.store.CreateQuote(ctx, rec)
	if err != nil {
		if idempotencyKey != "" {
			if replay, replayErr := s.findUpgradeReplay(ctx, idempotencyKey, userID, subscriptionID, targetPlanID); replayErr != nil {
				return nil, 0, replayErr
			} else if replay != nil {
				return frozenUpgradeQuote(replay), replay.ID, nil
			}
		}
		return nil, 0, err
	}
	return quote, id, nil
}

type upgradeQuoteBasis struct {
	sub             *UserSubscription
	fromPlan        *PlanSnapshot
	toPlan          *PlanSnapshot
	windowStart     time.Time // 未消费窗口起点（最早未消费 term 的 term_start，或 now）
	windowEnd       time.Time // = sub.ExpiresAt（升级不改变）
	blendedOldPrice float64   // 窗口内未消费段的实付合计（同 currency）
	currency        string
	unconsumedTerms []SubscriptionTermRecord
}

func (s *PlanChangeService) buildUpgradeQuote(ctx context.Context, userID, subscriptionID, targetPlanID int64) (*PlanChangeQuote, *upgradeQuoteBasis, error) {
	if s.plans == nil || s.terms == nil {
		return nil, nil, errors.New("plan change: storage unavailable")
	}
	sub, err := s.subRepo.GetByID(ctx, subscriptionID)
	if err != nil {
		return nil, nil, ErrSubscriptionNotFound
	}
	if sub.UserID != userID {
		return nil, nil, ErrSubscriptionNotFound // 不泄露存在性
	}
	if sub.Status != SubscriptionStatusActive || !sub.ExpiresAt.After(s.now()) {
		return nil, nil, ErrSubscriptionExpired
	}

	// Gate 1：plan identity
	if sub.PlanID == nil {
		return nil, nil, ErrPlanIdentityUnresolved
	}
	fromPlan, err := s.plans.GetPlan(ctx, *sub.PlanID)
	if err != nil {
		return nil, nil, err
	}
	toPlan, err := s.plans.GetPlan(ctx, targetPlanID)
	if err != nil {
		return nil, nil, err
	}

	// Gate 4：tier 必须已配置且为升级
	if fromPlan.TierRank <= 0 || toPlan.TierRank <= 0 {
		return nil, nil, ErrPlanTierNotConfigured
	}
	if fromPlan.ID == toPlan.ID {
		return nil, nil, ErrPlanNoChange
	}
	if toPlan.TierRank <= fromPlan.TierRank {
		return nil, nil, ErrPlanNoChange // 非升级走 downgrade 流程
	}
	if !sameWalletCurrency(fromPlan.Currency, toPlan.Currency) {
		return nil, nil, ErrPlanCrossCurrency
	}

	// 冲突：目标 Group 已有 active 订阅 → Preview 即拒绝（不自动 merge）
	if toPlan.GroupID != sub.GroupID {
		if existing, err := s.subRepo.GetActiveByUserIDAndGroupID(ctx, userID, toPlan.GroupID); err != nil && !errors.Is(err, ErrSubscriptionNotFound) {
			return nil, nil, err
		} else if existing != nil {
			return nil, nil, ErrPlanTargetGroupActive
		}
	}

	if s.groupRepo != nil {
		fromGroup, err := s.groupRepo.GetByID(ctx, sub.GroupID)
		if err != nil {
			return nil, nil, err
		}
		toGroup, err := s.groupRepo.GetByID(ctx, toPlan.GroupID)
		if err != nil {
			return nil, nil, err
		}
		if err := validateQuotaUpgrade(fromGroup, toGroup); err != nil {
			return nil, nil, err
		}
	}

	// Gate 2/3：价格真相 = 未消费 term 实付；逐段折算
	now := s.now()
	terms, err := s.terms.UnconsumedTerms(ctx, subscriptionID, now)
	if err != nil {
		return nil, nil, err
	}
	if len(terms) == 0 {
		return nil, nil, ErrPlanIdentityUnresolved // 无任何 term 事实：视为身份未解析
	}
	toPlanPriceCNY, err := walletAmountToCNY(toPlan.Price, toPlan.Currency)
	if err != nil {
		return nil, nil, ErrPlanCrossCurrency
	}
	canonicalToPlan := *toPlan
	canonicalToPlan.Price = toPlanPriceCNY
	canonicalToPlan.Currency = "CNY"
	toPlan = &canonicalToPlan
	currency := "CNY"
	// Gate 3 补充：V1 仅支持同 billing cadence（from/to Plan 标称周期一致；
	// 已付 term 的段长不限定——预付/赠送段按日归一计价）
	if !samePlanCadence(fromPlan, toPlan) {
		return nil, nil, ErrPlanTermMismatch
	}

	// A subscription grants one continuous interval. Terms are immutable payment
	// contributions: upgrade deltas can overlap purchase terms, but they never
	// create a second copy of the entitlement's remaining calendar time.
	credit := decimal.Zero
	windowStart := now
	toPlanNominal := planNominalDays(toPlan)
	if toPlanNominal <= 0 || toPlan.Price < 0 || math.IsNaN(toPlan.Price) || math.IsInf(toPlan.Price, 0) {
		return nil, nil, ErrPlanTermMismatch
	}
	toDaily := decimal.NewFromFloat(toPlanPriceCNY).Div(decimal.NewFromInt(int64(toPlanNominal)))
	charge := toDaily.Mul(decimal.NewFromInt(sub.ExpiresAt.Sub(now).Milliseconds())).Div(decimal.NewFromInt((24 * time.Hour).Milliseconds()))
	for i := range terms {
		term := &terms[i]
		termPriceCNY, err := walletAmountToCNY(term.PricePaid, term.Currency)
		if err != nil {
			return nil, nil, ErrPlanCrossCurrency
		}
		term.PricePaid = termPriceCNY
		term.Currency = "CNY"
		if !term.TermEnd.After(term.TermStart) || term.PricePaid < 0 || math.IsNaN(term.PricePaid) || math.IsInf(term.PricePaid, 0) {
			return nil, nil, ErrPlanIdentityUnresolved
		}
		consumeFrom := now
		if term.TermStart.After(consumeFrom) {
			consumeFrom = term.TermStart
		}
		consumeUntil := term.TermEnd
		if sub.ExpiresAt.Before(consumeUntil) {
			consumeUntil = sub.ExpiresAt
		}
		remainingMs := consumeUntil.Sub(consumeFrom).Milliseconds()
		if remainingMs <= 0 {
			continue
		}
		ratio := decimal.NewFromInt(remainingMs).Div(decimal.NewFromInt(term.TermEnd.Sub(term.TermStart).Milliseconds()))
		credit = credit.Add(decimal.NewFromFloat(term.PricePaid).Mul(ratio))
	}
	// Each displayed monetary component is rounded once before computing due.
	// Thus gross - credit == the frozen order amount, including last-day quotes.
	credit = credit.Round(2)
	charge = charge.Round(2)
	amountDue := charge.Sub(credit)
	if amountDue.IsNegative() {
		// 折算为负（理论 corner：旧实付高于新价）：最低 0，不自动退款
		amountDue = decimal.Zero
	}
	// 金额量化到货币最小单位（CNY=分；与 payment numeric(20,2) 对齐）
	q2 := func(d decimal.Decimal) float64 {
		v, _ := d.Round(2).Float64()
		return v
	}

	windowEnd := sub.ExpiresAt
	basis := &upgradeQuoteBasis{
		sub: sub, fromPlan: fromPlan, toPlan: toPlan,
		windowStart: windowStart, windowEnd: windowEnd,
		blendedOldPrice: q2(credit), // 窗口内未消费实付折算（快照展示）
		currency:        currency, unconsumedTerms: terms,
	}

	quote := &PlanChangeQuote{
		SubscriptionID:   subscriptionID,
		ChangeType:       "upgrade",
		FromPlanID:       fromPlan.ID,
		ToPlanID:         toPlan.ID,
		FromDisplayName:  fromPlan.Name,
		ToDisplayName:    toPlan.Name,
		EffectiveAt:      now,
		CurrentExpiry:    sub.ExpiresAt,
		NewExpiry:        sub.ExpiresAt, // 升级不改变到期
		RemainingSeconds: int64(sub.ExpiresAt.Sub(now).Seconds()),
		UnusedCredit:     q2(credit),
		ProratedCharge:   q2(charge),
		AmountDue:        q2(amountDue),
		Currency:         currency,
	}

	if sub.Group.UsesDualWindows() {
		copy := *sub
		sub = &copy
		projectDualWindows(sub, now)
	}
	// 升级后百分比（usage 绝对值不变，额度换新组）
	if s.status != nil {
		if st, err := s.status.GetGroupSubscriptionStatus(ctx, userID, sub.GroupID); err == nil && st != nil {
			quote.WeeklyUsagePercentBefore = st.WeeklyUsagePercent
			if st.ShortWindow != nil {
				v := st.ShortWindow.RemainingPercent
				quote.ShortRemainingPercentBefore = &v
			}
		}
		group, err := s.groupRepo.GetByID(ctx, toPlan.GroupID)
		if err == nil && group != nil && group.HasWeeklyLimit() && sub.Group != nil && sub.Group.HasWeeklyLimit() {
			raw := sub.WeeklyUsageUSD / *group.WeeklyLimitUSD * 100
			after := UserDisplayPercent(raw)
			quote.WeeklyUsagePercentAfter = &after
			quote.UsageStatusAfter = ClassifyUsageStatus(raw)
			rem := remainingPercent(sub.WeeklyUsageUSD, *group.WeeklyLimitUSD)
			quote.WeeklyRemainingPercentAfter = &rem
			if group.ValidDualLimits() {
				short := sub.ShortUsageUSD
				if sub.ShortWindowStart == nil || !now.Before(sub.ShortWindowStart.Add(5*time.Hour)) {
					short = 0
				}
				v := remainingPercent(short, *group.ShortLimitUSD)
				quote.ShortRemainingPercentAfter = &v
				quote.UsageStatusAfter = ClassifyUsageStatus(100 - math.Min(v, rem))
			}
		}
	}
	if s.apiKeys != nil {
		if n, err := s.apiKeys.CountByUserAndGroup(ctx, userID, sub.GroupID); err == nil {
			quote.KeysToMigrateCount = n
		}
	}
	return quote, basis, nil
}

func samePlanCadence(from, to *PlanSnapshot) bool {
	fromNominal, toNominal := planNominalDays(from), planNominalDays(to)
	if fromNominal <= 0 || toNominal <= 0 {
		return true // 未标称周期：跳过 cadence 校验（保守允许）
	}
	return fromNominal == toNominal
}

func planNominalDays(plan *PlanSnapshot) int {
	switch plan.ValidityUnit {
	case "week", "weeks":
		return plan.ValidityDays * 7
	case "month", "months":
		return plan.ValidityDays * 30
	default:
		return plan.ValidityDays
	}
}

func nilIfEmpty(s string) *string {
	if s == "" {
		return nil
	}
	return &s
}

// ------------------------------------------------------------------
// Fulfillment（支付成功后；单事务：订阅切组 + Key 迁移 + term + 审计 + 缓存）
// ------------------------------------------------------------------

// FulfillUpgrade 幂等履约：任何一步失败整体回滚；重复回调稳定返回。
func (s *PlanChangeService) FulfillUpgrade(ctx context.Context, changeID int64) error {
	if s.entClient == nil {
		return errors.New("plan change: storage unavailable")
	}
	change, err := s.store.GetByID(ctx, changeID)
	if err != nil {
		return err
	}
	if change.Status == "fulfilled" {
		return nil // 幂等：支付回调重放
	}
	if change.Status != "paid" {
		return ErrPlanQuoteStatusInvalid
	}

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

	// 订阅行锁 + 仍处于源组校验（防 TOCTOU：履约时订阅已被转移/撤销）
	sub, err := s.subRepo.GetByIDForUpdate(txCtx, change.SubscriptionID)
	if err != nil {
		return err
	}
	if sub.UserID != change.UserID {
		return ErrSubscriptionNotFound
	}
	// Re-read after the subscription lock. Concurrent callbacks must observe
	// the fulfilled marker before attempting a second switch or adding a term.
	change, err = s.store.GetByID(txCtx, changeID)
	if err != nil {
		return err
	}
	if change.Status == "fulfilled" {
		return nil
	}
	if change.Status != "paid" {
		return ErrPlanQuoteStatusInvalid
	}
	if change.FromPlanID != nil && (sub.PlanID == nil || *sub.PlanID != *change.FromPlanID) {
		return ErrPlanQuoteStatusInvalid
	}
	if change.TermStart == nil || change.TermEnd == nil || !change.TermEnd.Equal(sub.ExpiresAt) {
		return ErrPlanQuoteStatusInvalid
	}

	if change.FromGroupID != nil && sub.GroupID != *change.FromGroupID {
		return ErrPlanQuoteStatusInvalid // 已不在源组：不得重复迁移
	}
	if sub.Status != SubscriptionStatusActive || !sub.ExpiresAt.After(s.now()) {
		return ErrSubscriptionExpired
	}

	// 目标组冲突终检（Preview 之后可能新建了该组订阅）
	if existing, err := s.subRepo.GetActiveByUserIDAndGroupID(txCtx, change.UserID, change.ToGroupID); err != nil && !errors.Is(err, ErrSubscriptionNotFound) {
		return err
	} else if existing != nil && existing.ID != sub.ID {
		return ErrPlanTargetGroupActive
	}

	toGroup, err := s.groupRepo.GetByID(txCtx, change.ToGroupID)
	if err != nil {
		return err
	}
	if toGroup == nil || toGroup.Status != StatusActive || !toGroup.IsSubscriptionType() {
		return ErrPlanTargetGroupActive
	}

	fromGroup, err := s.groupRepo.GetByID(txCtx, sub.GroupID)
	if err != nil {
		return err
	}
	if err := validateQuotaUpgrade(fromGroup, toGroup); err != nil {
		return err
	}

	// 切组保字段：usage / anchor / starts_at / expires_at / fallback 全部不动（无隐藏 Reset）
	if err := switchSubscriptionPlan(txCtx, s.subRepo, sub.ID, change.ToGroupID, change.ToPlanID); err != nil {
		return err
	}

	// 升级段 term：实付差额及时间窗都来自冻结报价，与订单金额一致。
	now := s.now()
	orderID := change.OrderID
	toPlanID := change.ToPlanID
	if err := s.terms.RecordTerm(txCtx, &SubscriptionTermRecord{
		SubscriptionID: sub.ID,
		OrderID:        orderID,
		PlanID:         &toPlanID,
		PricePaid:      change.AmountDue,
		Currency:       change.Currency,
		Days:           int(change.TermEnd.Sub(*change.TermStart).Hours() / 24),
		TermStart:      *change.TermStart,
		TermEnd:        *change.TermEnd,
		Source:         "upgrade",
	}); err != nil {
		return err
	}

	// Key 迁移：仅该用户名下源组 Key；ID/secret 不变
	if s.apiKeys != nil {
		if _, err := s.apiKeys.MigrateGroupForUser(txCtx, change.UserID, sub.GroupID, change.ToGroupID); err != nil {
			return err
		}
	}

	// 升级取消旧 scheduled downgrade（superseded_by_upgrade）
	pending, err := s.store.ActiveScheduledChange(txCtx, sub.ID)
	if err != nil {
		return err
	}
	if pending != nil {
		if err := s.store.Cancel(txCtx, pending.ID, "superseded_by_upgrade"); err != nil {
			return err
		}
		if err := clearNextPlan(txCtx, s.subRepo, sub.ID); err != nil {
			return err
		}
	}

	if err := s.store.MarkFulfilled(txCtx, change.ID, now); err != nil {
		return err
	}
	if err := tx.Commit(); err != nil {
		return err
	}
	committed = true

	// 缓存失效（提交后）：订阅（旧+新组）+ API Key 认证缓存由 Key 迁移方失效
	s.invalidateCaches(ctx, sub.UserID, sub.GroupID)
	s.invalidateCaches(ctx, sub.UserID, change.ToGroupID)
	return nil
}

// ------------------------------------------------------------------
// Next Renewal Plan Change（预付费固定周期制：到期后下一次主动续费的默认目标）
// ------------------------------------------------------------------

// ScheduleDowngrade 记录"下次续费切换至目标档"的用户偏好（同订阅仅一个 pending）。
//
// Phase 11B 语义收紧（预付费固定周期制，永不自动续费）：
//   - 本指针只是未来续费偏好，不是自动执行指令；到期任务只负责 ACTIVE→EXPIRED，
//     绝不因 effective_at 到达而切换套餐或生成新周期；
//   - 只有三条路径会清除/取代本指针：用户付费购买/续费成功（含升级）；
//     用户主动取消；用户改选其它目标（替换）。
//   - EffectiveAt 语义 = 当前订阅 expires_at（最早续费切换点），不是自动生效时间。
func (s *PlanChangeService) ScheduleDowngrade(ctx context.Context, userID, subscriptionID, targetPlanID int64, idempotencyKey string) (*PlanChangeRecord, error) {
	sub, err := s.subRepo.GetByID(ctx, subscriptionID)
	if err != nil {
		return nil, ErrSubscriptionNotFound
	}
	if sub.UserID != userID {
		return nil, ErrSubscriptionNotFound
	}
	if sub.Status != SubscriptionStatusActive {
		return nil, ErrSubscriptionExpired
	}
	if sub.PlanID == nil {
		return nil, ErrPlanIdentityUnresolved
	}
	fromPlan, err := s.plans.GetPlan(ctx, *sub.PlanID)
	if err != nil {
		return nil, err
	}
	toPlan, err := s.plans.GetPlan(ctx, targetPlanID)
	if err != nil {
		return nil, err
	}
	if fromPlan.TierRank <= 0 || toPlan.TierRank <= 0 {
		return nil, ErrPlanTierNotConfigured
	}
	if fromPlan.ID == toPlan.ID {
		return nil, ErrPlanNoChange
	}
	if toPlan.TierRank >= fromPlan.TierRank {
		return nil, ErrPlanDowngradeNotTierOrder
	}

	// 替换语义：新 scheduled 取消旧 scheduled（保留审计）
	pending, err := s.store.ActiveScheduledChange(ctx, subscriptionID)
	if err != nil {
		return nil, err
	}
	if pending != nil {
		if err := s.store.Cancel(ctx, pending.ID, "superseded_by_reschedule"); err != nil {
			return nil, err
		}
	}
	if err := setNextPlan(ctx, s.subRepo, subscriptionID, targetPlanID); err != nil {
		return nil, err
	}

	now := s.now()
	toPlanID := targetPlanID
	fromPlanID := fromPlan.ID
	fromGroup := sub.GroupID
	rec := &PlanChangeRecord{
		UserID: userID, SubscriptionID: subscriptionID,
		ChangeType: "scheduled_downgrade",
		FromPlanID: &fromPlanID, ToPlanID: toPlanID,
		FromGroupID: &fromGroup, ToGroupID: toPlan.GroupID,
		FromTier: fromPlan.TierRank, ToTier: toPlan.TierRank,
		NewPriceSnapshot: toPlan.Price, Currency: toPlan.Currency,
		TermEnd:        &sub.ExpiresAt,
		EffectiveAt:    &sub.ExpiresAt, // term 末生效
		Status:         "scheduled",
		IdempotencyKey: nilIfEmpty(idempotencyKey),
	}
	id, err := s.store.CreateQuote(ctx, rec)
	if err != nil {
		return nil, err
	}
	_ = now
	created, err := s.store.GetByID(ctx, id)
	if err != nil {
		return nil, err
	}
	return created, nil
}

// CancelScheduledDowngrade 用户取消 pending 降级。
func (s *PlanChangeService) CancelScheduledDowngrade(ctx context.Context, userID, subscriptionID int64) error {
	sub, err := s.subRepo.GetByID(ctx, subscriptionID)
	if err != nil {
		return ErrSubscriptionNotFound
	}
	if sub.UserID != userID {
		return ErrSubscriptionNotFound
	}
	pending, err := s.store.ActiveScheduledChange(ctx, subscriptionID)
	if err != nil {
		return err
	}
	if pending == nil {
		return ErrPlanChangeNotFound
	}
	if err := s.store.Cancel(ctx, pending.ID, "user_cancel"); err != nil {
		return err
	}
	return clearNextPlan(ctx, s.subRepo, subscriptionID)
}

// GetChange 审计查询（用户仅限本人）。
func (s *PlanChangeService) GetChange(ctx context.Context, userID, changeID int64) (*PlanChangeRecord, error) {
	rec, err := s.store.GetByID(ctx, changeID)
	if err != nil {
		return nil, err
	}
	if rec.UserID != userID {
		return nil, ErrPlanChangeNotFound
	}
	return rec, nil
}

// ListChangesBySubscription 审计历史（用户仅限本人订阅）。
// PlanChangeAdminFilter 管理端审计过滤（均可选）。
type PlanChangeAdminFilter struct {
	UserID         *int64
	SubscriptionID *int64
	Status         *string
	Limit          int
	Offset         int
}

// AdminListChanges 管理端套餐变更审计（全部用户，支持过滤）。
func (s *PlanChangeService) AdminListChanges(ctx context.Context, f PlanChangeAdminFilter) ([]PlanChangeRecord, int64, error) {
	return s.store.ListAll(ctx, f.UserID, f.SubscriptionID, f.Status, f.Limit, f.Offset)
}

func (s *PlanChangeService) ListChangesBySubscription(ctx context.Context, userID, subscriptionID int64, limit int) ([]PlanChangeRecord, error) {
	sub, err := s.subRepo.GetByID(ctx, subscriptionID)
	if err != nil {
		return nil, ErrSubscriptionNotFound
	}
	if sub.UserID != userID {
		return nil, ErrSubscriptionNotFound
	}
	if limit <= 0 {
		limit = 20
	}
	return s.store.ListBySubscription(ctx, subscriptionID, limit)
}

// ---- repo-facing helpers（订阅字段切换；由具体 repo 提供事务实现）----

// SupersedeScheduledChangeForUser 支付履约成功后的用户级清理（superseded_by_renewal）：
// 清除该用户全部订阅行上的 next_plan_id 指针 + 取消全部 pending 审计行。
// 付费续费可能在新行上激活（跨档续费），因此必须按用户而非按行清理。
// 由 SubscriptionService.ClosePendingChangeOnPaidRenewal 在支付履约事务内调用；
// 到期/维护/对账等系统任务绝不允许触达本方法。
func (s *PlanChangeService) SupersedeScheduledChangeForUser(ctx context.Context, userID int64) error {
	if s.entClient != nil {
		// 事务感知：履约事务内调用时必须走 tx（直接用 entClient 会绕过回滚，
		// 造成"履约失败但指针已被清"的半提交状态）
		client := s.entClient
		if tx := dbent.TxFromContext(ctx); tx != nil {
			client = tx.Client()
		}
		if _, err := client.UserSubscription.Update().
			Where(
				usersubscription.UserIDEQ(userID),
				usersubscription.NextPlanIDNotNil(),
			).
			ClearNextPlanID().
			Save(ctx); err != nil {
			return err
		}
	}
	if closer, ok := s.store.(UserScheduledChangeCloser); ok {
		if _, err := closer.CancelScheduledForUser(ctx, userID, "superseded_by_renewal"); err != nil {
			return err
		}
	}
	return nil
}

func switchSubscriptionPlan(ctx context.Context, subRepo UserSubscriptionRepository, subscriptionID, groupID, planID int64) error {
	if sw, ok := subRepo.(SubscriptionPlanSwitcher); ok {
		return sw.SwitchPlan(ctx, subscriptionID, groupID, planID)
	}
	return errors.New("subscription repository does not support plan switch")
}

func setNextPlan(ctx context.Context, subRepo UserSubscriptionRepository, subscriptionID int64, planID int64) error {
	if sw, ok := subRepo.(SubscriptionPlanSwitcher); ok {
		return sw.SetNextPlan(ctx, subscriptionID, &planID)
	}
	return errors.New("subscription repository does not support plan switch")
}

func clearNextPlan(ctx context.Context, subRepo UserSubscriptionRepository, subscriptionID int64) error {
	if sw, ok := subRepo.(SubscriptionPlanSwitcher); ok {
		return sw.SetNextPlan(ctx, subscriptionID, nil)
	}
	return errors.New("subscription repository does not support plan switch")
}

// SubscriptionPlanSwitcher 订阅 Plan 切换的可选能力（生产 ent 仓储实现）。
type SubscriptionPlanSwitcher interface {
	SwitchPlan(ctx context.Context, subscriptionID, groupID, planID int64) error
	SetNextPlan(ctx context.Context, subscriptionID int64, planID *int64) error
}

// Optional for in-memory quote calculators; production storage implements this.
type PlanChangeIdempotencyReader interface {
	GetByIdempotencyKey(context.Context, string) (*PlanChangeRecord, error)
}

func (s *PlanChangeService) findUpgradeReplay(ctx context.Context, key string, userID, subscriptionID, targetPlanID int64) (*PlanChangeRecord, error) {
	reader, ok := s.store.(PlanChangeIdempotencyReader)
	if !ok {
		return nil, nil
	}
	rec, err := reader.GetByIdempotencyKey(ctx, key)
	if errors.Is(err, ErrPlanChangeNotFound) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	if rec.UserID != userID || rec.SubscriptionID != subscriptionID || rec.ToPlanID != targetPlanID || rec.ChangeType != "upgrade" {
		return nil, infraerrors.Conflict("IDEMPOTENCY_CONFLICT", "idempotency key was used for a different plan change")
	}
	return rec, nil
}
func frozenUpgradeQuote(rec *PlanChangeRecord) *PlanChangeQuote {
	q := &PlanChangeQuote{SubscriptionID: rec.SubscriptionID, ChangeType: rec.ChangeType, ToPlanID: rec.ToPlanID, RemainingSeconds: rec.RemainingSeconds, UnusedCredit: rec.UnusedCredit, ProratedCharge: rec.ProratedCharge, AmountDue: rec.AmountDue, Currency: rec.Currency}
	if rec.FromPlanID != nil {
		q.FromPlanID = *rec.FromPlanID
	}
	if rec.QuoteCreatedAt != nil {
		q.EffectiveAt = *rec.QuoteCreatedAt
	}
	if rec.TermEnd != nil {
		q.CurrentExpiry = *rec.TermEnd
		q.NewExpiry = *rec.TermEnd
	}
	return q
}

func validateQuotaUpgrade(from, to *Group) error {
	if from.UsesDualWindows() != to.UsesDualWindows() {
		return infraerrors.Conflict("QUOTA_POLICY_MISMATCH", "plans must use the same quota policy")
	}
	if from.UsesDualWindows() && (!from.ValidDualLimits() || !to.ValidDualLimits() || *to.ShortLimitUSD < *from.ShortLimitUSD || *to.WeeklyLimitUSD < *from.WeeklyLimitUSD) {
		return infraerrors.Conflict("QUOTA_UPGRADE_INVALID", "upgrade must not reduce either quota limit")
	}
	return nil
}
