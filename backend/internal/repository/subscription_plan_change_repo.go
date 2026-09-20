package repository

import (
	"context"
	"time"

	dbent "github.com/Wei-Shaw/sub2api/ent"
	"github.com/Wei-Shaw/sub2api/ent/apikey"
	"github.com/Wei-Shaw/sub2api/ent/subscriptionplanchange"
	"github.com/Wei-Shaw/sub2api/ent/subscriptionterm"
	"github.com/Wei-Shaw/sub2api/ent/usersubscription"
	"github.com/Wei-Shaw/sub2api/internal/service"
)

// ------------------------------------------------------------------
// subscription_terms
// ------------------------------------------------------------------

type subscriptionTermRepo struct {
	client *dbent.Client
}

func NewSubscriptionTermStore(client *dbent.Client) service.TermStore {
	return &subscriptionTermRepo{client: client}
}

func (r *subscriptionTermRepo) UnconsumedTerms(ctx context.Context, subscriptionID int64, now time.Time) ([]service.SubscriptionTermRecord, error) {
	rows, err := txClientFromContext(ctx, r.client).SubscriptionTerm.Query().
		Where(
			subscriptionterm.SubscriptionIDEQ(subscriptionID),
			subscriptionterm.TermEndGT(now),
		).
		Order(dbent.Asc(subscriptionterm.FieldTermStart)).
		All(ctx)
	if err != nil {
		return nil, err
	}
	out := make([]service.SubscriptionTermRecord, 0, len(rows))
	for _, m := range rows {
		out = append(out, service.SubscriptionTermRecord{
			SubscriptionID: m.SubscriptionID,
			OrderID:        m.OrderID,
			PlanID:         m.PlanID,
			PricePaid:      m.PricePaid,
			Currency:       m.Currency,
			Days:           m.Days,
			TermStart:      m.TermStart,
			TermEnd:        m.TermEnd,
			Source:         m.Source,
		})
	}
	return out, nil
}

func (r *subscriptionTermRepo) RecordTerm(ctx context.Context, term *service.SubscriptionTermRecord) error {
	create := txClientFromContext(ctx, r.client).SubscriptionTerm.Create().
		SetSubscriptionID(term.SubscriptionID).
		SetPricePaid(term.PricePaid).
		SetCurrency(term.Currency).
		SetDays(term.Days).
		SetTermStart(term.TermStart).
		SetTermEnd(term.TermEnd).
		SetSource(term.Source)
	if term.OrderID != nil {
		create.SetOrderID(*term.OrderID)
	}
	if term.PlanID != nil {
		create.SetPlanID(*term.PlanID)
	}
	_, err := create.Save(ctx)
	return err
}

// ------------------------------------------------------------------
// subscription_plan_changes
// ------------------------------------------------------------------

type subscriptionPlanChangeRepo struct {
	client *dbent.Client
}

func NewSubscriptionPlanChangeStore(client *dbent.Client) service.PlanChangeStore {
	return &subscriptionPlanChangeRepo{client: client}
}

func planChangeEntityToService(m *dbent.SubscriptionPlanChange) *service.PlanChangeRecord {
	return &service.PlanChangeRecord{
		ID: m.ID, UserID: m.UserID, SubscriptionID: m.SubscriptionID,
		ChangeType: m.ChangeType,
		FromPlanID: m.FromPlanID, ToPlanID: m.ToPlanID,
		FromGroupID: m.FromGroupID, ToGroupID: m.ToGroupID,
		FromTier: m.FromTier, ToTier: m.ToTier,
		OldPriceSnapshot: m.OldPriceSnapshot, NewPriceSnapshot: m.NewPriceSnapshot,
		Currency:  m.Currency,
		TermStart: m.TermStart, TermEnd: m.TermEnd,
		RemainingSeconds: m.RemainingSeconds,
		UnusedCredit:     m.UnusedCredit, ProratedCharge: m.ProratedCharge, AmountDue: m.AmountDue,
		QuoteCreatedAt: m.QuoteCreatedAt, QuoteExpiresAt: m.QuoteExpiresAt,
		EffectiveAt: m.EffectiveAt, Status: m.Status,
		CancelReason: m.CancelReason, OrderID: m.OrderID,
		IdempotencyKey: m.IdempotencyKey,
		PaidAt:         m.PaidAt, FulfilledAt: m.FulfilledAt, CancelledAt: m.CancelledAt,
		CreatedAt: m.CreatedAt, UpdatedAt: m.UpdatedAt,
	}
}

func (r *subscriptionPlanChangeRepo) CreateQuote(ctx context.Context, rec *service.PlanChangeRecord) (int64, error) {
	create := txClientFromContext(ctx, r.client).SubscriptionPlanChange.Create().
		SetUserID(rec.UserID).
		SetSubscriptionID(rec.SubscriptionID).
		SetChangeType(rec.ChangeType).
		SetToPlanID(rec.ToPlanID).
		SetToGroupID(rec.ToGroupID).
		SetFromTier(rec.FromTier).
		SetToTier(rec.ToTier).
		SetNewPriceSnapshot(rec.NewPriceSnapshot).
		SetCurrency(rec.Currency).
		SetRemainingSeconds(rec.RemainingSeconds).
		SetUnusedCredit(rec.UnusedCredit).
		SetProratedCharge(rec.ProratedCharge).
		SetAmountDue(rec.AmountDue).
		SetStatus(rec.Status).
		SetMetadata(map[string]any{})
	if rec.FromPlanID != nil {
		create.SetFromPlanID(*rec.FromPlanID)
	}
	if rec.FromGroupID != nil {
		create.SetFromGroupID(*rec.FromGroupID)
	}
	if rec.OldPriceSnapshot != nil {
		create.SetOldPriceSnapshot(*rec.OldPriceSnapshot)
	}
	if rec.TermStart != nil {
		create.SetTermStart(*rec.TermStart)
	}
	if rec.TermEnd != nil {
		create.SetTermEnd(*rec.TermEnd)
	}
	if rec.QuoteCreatedAt != nil {
		create.SetQuoteCreatedAt(*rec.QuoteCreatedAt)
	}
	if rec.QuoteExpiresAt != nil {
		create.SetQuoteExpiresAt(*rec.QuoteExpiresAt)
	}
	if rec.EffectiveAt != nil {
		create.SetEffectiveAt(*rec.EffectiveAt)
	}
	if rec.IdempotencyKey != nil {
		create.SetIdempotencyKey(*rec.IdempotencyKey)
	}
	created, err := create.Save(ctx)
	if err != nil {
		return 0, err
	}
	return created.ID, nil
}

func (r *subscriptionPlanChangeRepo) GetByID(ctx context.Context, id int64) (*service.PlanChangeRecord, error) {
	m, err := txClientFromContext(ctx, r.client).SubscriptionPlanChange.Get(ctx, id)
	if err != nil {
		return nil, service.ErrPlanChangeNotFound
	}
	return planChangeEntityToService(m), nil
}

func (r *subscriptionPlanChangeRepo) GetByOrder(ctx context.Context, orderID int64) (*service.PlanChangeRecord, error) {
	m, err := txClientFromContext(ctx, r.client).SubscriptionPlanChange.Query().
		Where(subscriptionplanchange.OrderIDEQ(orderID)).
		Only(ctx)
	if err != nil {
		return nil, service.ErrPlanChangeNotFound
	}
	return planChangeEntityToService(m), nil
}

func (r *subscriptionPlanChangeRepo) MarkPendingPayment(ctx context.Context, id, orderID int64) error {
	_, err := txClientFromContext(ctx, r.client).SubscriptionPlanChange.Update().
		Where(subscriptionplanchange.IDEQ(id), subscriptionplanchange.StatusEQ("quoted")).
		SetStatus("pending_payment").
		SetOrderID(orderID).
		Save(ctx)
	return err
}

func (r *subscriptionPlanChangeRepo) MarkPaid(ctx context.Context, id int64) error {
	_, err := txClientFromContext(ctx, r.client).SubscriptionPlanChange.Update().
		Where(subscriptionplanchange.IDEQ(id),
			subscriptionplanchange.StatusIn("quoted", "pending_payment")).
		SetStatus("paid").
		SetPaidAt(time.Now()).
		Save(ctx)
	return err
}

func (r *subscriptionPlanChangeRepo) MarkFulfilled(ctx context.Context, id int64, effectiveAt time.Time) error {
	_, err := txClientFromContext(ctx, r.client).SubscriptionPlanChange.Update().
		Where(subscriptionplanchange.IDEQ(id),
			subscriptionplanchange.StatusIn("paid", "pending_payment")).
		SetStatus("fulfilled").
		SetFulfilledAt(time.Now()).
		SetEffectiveAt(effectiveAt).
		Save(ctx)
	return err
}

func (r *subscriptionPlanChangeRepo) Cancel(ctx context.Context, id int64, reason string) error {
	_, err := txClientFromContext(ctx, r.client).SubscriptionPlanChange.Update().
		Where(subscriptionplanchange.IDEQ(id), subscriptionplanchange.StatusNEQ("fulfilled")).
		SetStatus("cancelled").
		SetCancelReason(reason).
		SetCancelledAt(time.Now()).
		Save(ctx)
	return err
}

func (r *subscriptionPlanChangeRepo) ActiveScheduledChange(ctx context.Context, subscriptionID int64) (*service.PlanChangeRecord, error) {
	m, err := txClientFromContext(ctx, r.client).SubscriptionPlanChange.Query().
		Where(
			subscriptionplanchange.SubscriptionIDEQ(subscriptionID),
			subscriptionplanchange.ChangeTypeEQ("scheduled_downgrade"),
			subscriptionplanchange.StatusEQ("scheduled"),
		).
		Only(ctx)
	if err != nil {
		return nil, nil // 无 pending：非错误
	}
	return planChangeEntityToService(m), nil
}

func (r *subscriptionPlanChangeRepo) ListBySubscription(ctx context.Context, subscriptionID int64, limit int) ([]service.PlanChangeRecord, error) {
	ms, err := txClientFromContext(ctx, r.client).SubscriptionPlanChange.Query().
		Where(subscriptionplanchange.SubscriptionIDEQ(subscriptionID)).
		Order(dbent.Desc(subscriptionplanchange.FieldID)).
		Limit(limit).
		All(ctx)
	if err != nil {
		return nil, err
	}
	out := make([]service.PlanChangeRecord, 0, len(ms))
	for _, m := range ms {
		out = append(out, *planChangeEntityToService(m))
	}
	return out, nil
}

// ------------------------------------------------------------------
// PlanService（SKU 快照读取）
// ------------------------------------------------------------------

type planSnapshotRepo struct {
	client *dbent.Client
}

func NewPlanSnapshotService(client *dbent.Client) service.PlanService {
	return &planSnapshotRepo{client: client}
}

func (r *planSnapshotRepo) GetPlan(ctx context.Context, planID int64) (*service.PlanSnapshot, error) {
	m, err := r.client.SubscriptionPlan.Get(ctx, planID)
	if err != nil {
		return nil, service.ErrPlanChangeNotFound
	}
	return &service.PlanSnapshot{
		ID: m.ID, GroupID: m.GroupID, Name: m.Name,
		Price: m.Price, Currency: m.Currency,
		ValidityDays: m.ValidityDays, ValidityUnit: m.ValidityUnit,
		TierRank: m.TierRank, ForSale: m.ForSale,
	}, nil
}

// ------------------------------------------------------------------
// UserSubscription Switcher + 映射补列
// ------------------------------------------------------------------

// SwitchPlan 事务内切换订阅的 Group/Plan（保 usage/anchor/starts/expires/fallback）。
func (r *userSubscriptionRepository) SwitchPlan(ctx context.Context, subscriptionID, groupID, planID int64) error {
	client := clientFromContext(ctx, r.client)
	_, err := client.UserSubscription.Update().
		Where(usersubscription.IDEQ(subscriptionID)).
		SetGroupID(groupID).
		SetPlanID(planID).
		Save(ctx)
	return translatePersistenceError(err, service.ErrSubscriptionNotFound, nil)
}

// SetNextPlan 设置/清除 scheduled downgrade 目标。
func (r *userSubscriptionRepository) SetNextPlan(ctx context.Context, subscriptionID int64, planID *int64) error {
	client := clientFromContext(ctx, r.client)
	update := client.UserSubscription.Update().Where(usersubscription.IDEQ(subscriptionID))
	if planID == nil {
		update = update.ClearNextPlanID()
	} else {
		update = update.SetNextPlanID(*planID)
	}
	_, err := update.Save(ctx)
	return translatePersistenceError(err, service.ErrSubscriptionNotFound, nil)
}

// ------------------------------------------------------------------
// API Key Group 迁移
// ------------------------------------------------------------------

type apiKeyGroupMigrator struct {
	client *dbent.Client
}

func NewAPIKeyGroupMigrator(client *dbent.Client) service.APIKeyGroupMigrator {
	return &apiKeyGroupMigrator{client: client}
}

func (m *apiKeyGroupMigrator) MigrateGroupForUser(ctx context.Context, userID, fromGroupID, toGroupID int64) (int64, error) {
	rows, err := txClientFromContext(ctx, m.client).APIKey.Update().
		Where(
			apikey.UserIDEQ(userID),
			apikey.GroupIDEQ(fromGroupID),
		).
		SetGroupID(toGroupID).
		Save(ctx)
	if err != nil {
		return 0, err
	}
	return int64(rows), nil
}

func (m *apiKeyGroupMigrator) CountByUserAndGroup(ctx context.Context, userID, groupID int64) (int64, error) {
	n, err := txClientFromContext(ctx, m.client).APIKey.Query().
		Where(
			apikey.UserIDEQ(userID),
			apikey.GroupIDEQ(groupID),
		).
		Count(ctx)
	return int64(n), err
}
