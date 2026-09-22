//go:build integration

package repository

// Phase 1 — Subscription V1 Data Model 的 schema/migration 测试。
//
// 验证 migration 239 的行为契约：
//   - additive：存量订阅/分组行新列取默认值（fallback=false / override=NULL），
//     weekly usage / anchor / expiry 不受影响；
//   - reset events / applications / cards 可创建且默认值正确；
//   - applications UNIQUE(reset_event_id, user_subscription_id)（worker retry-safe 基石）；
//   - 发卡幂等 UNIQUE(grant_event_id, user_id, grant_index)，quantity=N 时 0..N-1 共存；
//   - FK 删除语义：事件被引用 RESTRICT；应用随订阅 CASCADE；
//     卡的 used_subscription_id / created_by 随引用对象 SET NULL；卡随用户 CASCADE。
//
// 本 Phase 无运行时行为：所有写入通过 ent client / raw SQL，不引入 service 层。

import (
	"context"
	"testing"
	"time"

	dbent "github.com/Wei-Shaw/sub2api/ent"
	"github.com/Wei-Shaw/sub2api/ent/subscriptionresetapplication"
	"github.com/Wei-Shaw/sub2api/ent/subscriptionresetcard"
	"github.com/Wei-Shaw/sub2api/ent/subscriptionresetevent"
	"github.com/Wei-Shaw/sub2api/internal/domain"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
)

// phase1CleanupEvents 清理事件及其应用/卡（须在 phase0CleanupStack 之前注册：
// t.Cleanup LIFO，保证先删 reset 表、后删用户/分组栈）。
func phase1CleanupEvents(t *testing.T, eventIDs ...int64) {
	t.Helper()
	t.Cleanup(func() {
		ctx := context.Background()
		for _, evID := range eventIDs {
			_, _ = integrationDB.ExecContext(ctx, "DELETE FROM subscription_reset_cards WHERE grant_event_id = $1", evID)
			_, _ = integrationDB.ExecContext(ctx, "DELETE FROM subscription_reset_applications WHERE reset_event_id = $1", evID)
			_, _ = integrationDB.ExecContext(ctx, "DELETE FROM subscription_reset_events WHERE id = $1", evID)
		}
	})
}

func phase1MustEvent(t *testing.T, client *dbent.Client, createdBy *int64, eventType string) *dbent.SubscriptionResetEvent {
	t.Helper()
	create := client.SubscriptionResetEvent.Create().
		SetEventType(eventType).
		SetScopeType(domain.ResetEventScopeTypeAll).
		SetScope(map[string]any{}).
		SetEffectiveAt(time.Now().Add(24 * time.Hour).Truncate(time.Microsecond)).
		SetMetadata(map[string]any{})
	if createdBy != nil {
		create.SetNillableCreatedBy(createdBy)
	}
	ev, err := create.Save(context.Background())
	require.NoError(t, err)
	return ev
}

// ---- additive 默认值：存量数据行为不变 ----

func TestPhase1ExistingRowsKeepBaselineDefaults(t *testing.T) {
	ctx := context.Background()
	client := testEntClient(t)

	user, group, sub, _, _ := phase0MustSubscriptionStack(t, client, 0, nil)

	anchor := time.Now().Add(-24 * time.Hour).Truncate(time.Microsecond)
	phase0SetWeeklyWindow(t, sub.ID, anchor, 8.8)
	_, err := integrationDB.ExecContext(ctx,
		"UPDATE user_subscriptions SET starts_at = $1, expires_at = $2 WHERE id = $3",
		anchor.Add(-24*time.Hour), anchor.Add(30*24*time.Hour), sub.ID)
	require.NoError(t, err)

	got, err := client.UserSubscription.Get(ctx, sub.ID)
	require.NoError(t, err)
	require.False(t, got.AutoPaygFallback,
		"existing subscription row must default to auto_payg_fallback=false (baseline behavior)")
	require.InDelta(t, 8.8, got.WeeklyUsageUsd, 1e-8, "weekly usage untouched by migration")
	require.Equal(t, anchor.Format(time.RFC3339Nano), got.WeeklyWindowStart.Format(time.RFC3339Nano),
		"weekly anchor untouched")
	require.WithinDuration(t, anchor.Add(30*24*time.Hour), got.ExpiresAt, time.Second, "expiry untouched")

	grp, err := client.Group.Get(ctx, group.ID)
	require.NoError(t, err)
	require.Nil(t, grp.ConcurrencyOverride,
		"existing group must default to concurrency_override=NULL (no runtime change)")

	// 新建行同样取默认值（NOT NULL DEFAULT false）。
	// 注：service.UserSubscription 结构体刻意不在 Phase 1 增加 fallback 字段
	// （无运行时消费）；这里直接经 ent 读回验证 DB 层默认值。
	// 每用户只允许一条 ACTIVE；第二条使用历史订阅以验证字段默认值。
	group2 := mustCreateGroup(t, client, &service.Group{
		Name:             "phase0-sub-group-2-" + uuid.NewString(),
		SubscriptionType: service.SubscriptionTypeSubscription,
	})
	t.Cleanup(func() {
		_, _ = integrationDB.Exec("DELETE FROM groups WHERE id = $1", group2.ID)
	})
	sub2 := mustCreateSubscription(t, client, &service.UserSubscription{UserID: user.ID, GroupID: group2.ID, Status: service.SubscriptionStatusExpired})
	sub2Ent, err := client.UserSubscription.Get(ctx, sub2.ID)
	require.NoError(t, err)
	require.False(t, sub2Ent.AutoPaygFallback)
}

// ---- Reset Event：可创建 + 默认值 + 必填校验 ----

func TestPhase1ResetEventCreateDefaultsAndValidation(t *testing.T) {
	ctx := context.Background()
	client := testEntClient(t)

	user, group, _, _, _ := phase0MustSubscriptionStack(t, client, 0, nil)
	before := time.Now().Add(-time.Microsecond)
	ev := phase1MustEvent(t, client, &user.ID, domain.ResetEventTypeGlobalReset)
	after := time.Now().Add(time.Microsecond)
	phase1CleanupEvents(t, ev.ID)
	phase0CleanupStack(t, user.ID, group.ID, 0)

	got, err := client.SubscriptionResetEvent.Query().
		Where(subscriptionresetevent.IDEQ(ev.ID)).
		Only(ctx)
	require.NoError(t, err)
	require.Equal(t, domain.ResetEventStatusPending, got.Status, "event defaults to pending")
	require.NotNil(t, got.CreatedBy)
	require.Equal(t, user.ID, *got.CreatedBy)
	require.NotZero(t, got.CreatedAt)
	// Ent evaluates the two time.Now defaults independently; PostgreSQL stores microseconds.
	for _, timestamp := range []time.Time{got.CreatedAt, got.UpdatedAt} {
		require.False(t, timestamp.Before(before), "default timestamp precedes creation")
		require.False(t, timestamp.After(after), "default timestamp follows creation")
	}

	// 缺 effective_at（NOT NULL）→ 拒绝
	_, err = client.SubscriptionResetEvent.Create().
		SetEventType(domain.ResetEventTypeGlobalReset).
		SetScopeType(domain.ResetEventScopeTypeAll).
		SetScope(map[string]any{}).
		Save(ctx)
	require.Error(t, err, "effective_at is required")

	// 缺 scope_type（NotEmpty）→ 拒绝
	_, err = client.SubscriptionResetEvent.Create().
		SetEventType(domain.ResetEventTypeResetCardGrant).
		SetScope(map[string]any{}).
		SetEffectiveAt(time.Now().
			Truncate(time.Microsecond)).
		Save(ctx)
	require.Error(t, err, "scope_type is required")
}

// ---- Reset Application：UNIQUE(reset_event_id, user_subscription_id) ----

func TestPhase1ResetApplicationUniqueConstraint(t *testing.T) {
	ctx := context.Background()
	client := testEntClient(t)

	user, group, sub, _, _ := phase0MustSubscriptionStack(t, client, 0, nil)
	ev := phase1MustEvent(t, client, nil, domain.ResetEventTypeGlobalReset)
	phase1CleanupEvents(t, ev.ID)
	phase0CleanupStack(t, user.ID, group.ID, 0)

	prevStart := time.Now().Add(-48 * time.Hour).Truncate(time.Microsecond)
	app, err := client.SubscriptionResetApplication.Create().
		SetResetEventID(ev.ID).
		SetUserSubscriptionID(sub.ID).
		SetEffectiveAt(ev.EffectiveAt).
		SetNillablePreviousWeeklyWindowStart(&prevStart).
		SetPreviousWeeklyUsageUsd(12.34).
		SetMetadata(map[string]any{}).
		Save(ctx)
	require.NoError(t, err)
	require.Equal(t, domain.ResetApplicationStatusApplied, app.Status, "application defaults to applied")
	require.Equal(t, ev.EffectiveAt.Format(time.RFC3339Nano), app.EffectiveAt.Format(time.RFC3339Nano))

	// 同一 (event, subscription) 第二次插入 → 违反唯一约束
	_, err = client.SubscriptionResetApplication.Create().
		SetResetEventID(ev.ID).
		SetUserSubscriptionID(sub.ID).
		SetEffectiveAt(ev.EffectiveAt).
		SetMetadata(map[string]any{}).
		Save(ctx)
	require.Error(t, err, "duplicate (reset_event_id, user_subscription_id) must violate the unique constraint")
	var constraintErr *dbent.ConstraintError
	require.ErrorAs(t, err, &constraintErr, "error must be a constraint violation")
}

// ---- Reset Card：状态机字段 + 消费绑定 ----

func TestPhase1ResetCardStatusLifecycle(t *testing.T) {
	ctx := context.Background()
	client := testEntClient(t)

	user, group, sub, _, _ := phase0MustSubscriptionStack(t, client, 0, nil)
	phase0CleanupStack(t, user.ID, group.ID, 0)

	card, err := client.SubscriptionResetCard.Create().
		SetUserID(user.ID).
		SetSourceType(domain.ResetCardSourceAdminGrant).
		SetNotes("phase1 lifecycle").
		SetMetadata(map[string]any{}).
		Save(ctx)
	require.NoError(t, err)
	t.Cleanup(func() {
		_, _ = integrationDB.ExecContext(ctx, "DELETE FROM subscription_reset_cards WHERE id = $1", card.ID)
	})

	require.Equal(t, domain.ResetCardStatusAvailable, card.Status)
	require.Equal(t, domain.ResetCardScopeWeekly, card.Scope, "V1 scope defaults to weekly")
	require.Zero(t, card.GrantIndex)
	require.Nil(t, card.GrantEventID)
	require.Nil(t, card.UsedAt)
	require.Nil(t, card.UsedSubscriptionID)

	// 消费：available → used，绑定目标订阅
	usedAt := time.Now().Truncate(time.Microsecond)
	updated, err := client.SubscriptionResetCard.UpdateOneID(card.ID).
		SetStatus(domain.ResetCardStatusUsed).
		SetUsedAt(usedAt).
		SetUsedSubscriptionID(sub.ID).
		Save(ctx)
	require.NoError(t, err)
	require.Equal(t, domain.ResetCardStatusUsed, updated.Status)
	require.NotNil(t, updated.UsedSubscriptionID)
	require.Equal(t, sub.ID, *updated.UsedSubscriptionID)
}

// ---- 批量发卡幂等：UNIQUE(grant_event_id, user_id, grant_index) ----

func TestPhase1GrantCardIdempotencyAndQuantity(t *testing.T) {
	ctx := context.Background()
	client := testEntClient(t)

	user, group, _, _, _ := phase0MustSubscriptionStack(t, client, 0, nil)
	ev := phase1MustEvent(t, client, nil, domain.ResetEventTypeResetCardGrant)
	phase1CleanupEvents(t, ev.ID)
	phase0CleanupStack(t, user.ID, group.ID, 0)

	newCard := func(index int) (*dbent.SubscriptionResetCard, error) {
		return client.SubscriptionResetCard.Create().
			SetUserID(user.ID).
			SetGrantEventID(ev.ID).
			SetGrantIndex(index).
			SetSourceType(domain.ResetCardSourceCampaign).
			SetCampaign("phase1_launch").
			SetExpiresAt(time.Now().Add(14 * 24 * time.Hour)).
			SetMetadata(map[string]any{}).
			Save(ctx)
	}

	_, err := newCard(0)
	require.NoError(t, err)

	// 同 (event, user, index) 重复 → 唯一约束（活动 retry 不重复发卡）
	_, err = newCard(0)
	require.Error(t, err, "duplicate grant (event, user, index) must violate the unique index")
	var constraintErr *dbent.ConstraintError
	require.ErrorAs(t, err, &constraintErr)

	// quantity=3：index 0/1/2 共存
	_, err = newCard(1)
	require.NoError(t, err)
	_, err = newCard(2)
	require.NoError(t, err)

	count, err := client.SubscriptionResetCard.Query().
		Where(subscriptionresetcard.GrantEventIDEQ(ev.ID), subscriptionresetcard.UserIDEQ(user.ID)).
		Count(ctx)
	require.NoError(t, err)
	require.Equal(t, 3, count, "grant_index 0/1/2 must coexist for quantity=3")
}

// ---- FK 删除语义 ----

func TestPhase1FKDeleteSemantics(t *testing.T) {
	ctx := context.Background()
	client := testEntClient(t)

	user, group, sub, _, _ := phase0MustSubscriptionStack(t, client, 0, nil)
	group2 := mustCreateGroup(t, client, &service.Group{
		Name:             "phase0-sub-group-2-" + uuid.NewString(),
		SubscriptionType: service.SubscriptionTypeSubscription,
	})
	t.Cleanup(func() {
		_, _ = integrationDB.Exec("DELETE FROM groups WHERE id = $1", group2.ID)
	})
	sub2 := mustCreateSubscription(t, client, &service.UserSubscription{UserID: user.ID, GroupID: group2.ID, Status: service.SubscriptionStatusExpired})
	ev := phase1MustEvent(t, client, &user.ID, domain.ResetEventTypeGlobalReset)
	phase1CleanupEvents(t, ev.ID)
	phase0CleanupStack(t, user.ID, group.ID, 0)

	prevStart := time.Now().Add(-24 * time.Hour).Truncate(time.Microsecond)
	_, err := client.SubscriptionResetApplication.Create().
		SetResetEventID(ev.ID).
		SetUserSubscriptionID(sub.ID).
		SetEffectiveAt(ev.EffectiveAt).
		SetNillablePreviousWeeklyWindowStart(&prevStart).
		SetPreviousWeeklyUsageUsd(3.5).
		SetMetadata(map[string]any{}).
		Save(ctx)
	require.NoError(t, err)
	card, err := client.SubscriptionResetCard.Create().
		SetUserID(user.ID).
		SetGrantEventID(ev.ID).
		SetGrantIndex(0).
		SetSourceType(domain.ResetCardSourceAdminGrant).
		SetUsedSubscriptionID(sub2.ID).
		SetMetadata(map[string]any{}).
		Save(ctx)
	require.NoError(t, err)

	// 1) 事件被 application / grant card 引用 → RESTRICT，不可删除
	err = client.SubscriptionResetEvent.DeleteOneID(ev.ID).Exec(ctx)
	require.Error(t, err, "event referenced by applications/cards must be RESTRICTed from delete")

	// 2) 订阅被 application 引用 → 硬删订阅 CASCADE 掉 application，事件保留
	require.NoError(t, hardDeleteSubscription(t, sub.ID))
	appCount, err := client.SubscriptionResetApplication.Query().
		Where(subscriptionresetapplication.ResetEventIDEQ(ev.ID), subscriptionresetapplication.UserSubscriptionIDEQ(sub.ID)).
		Count(ctx)
	require.NoError(t, err)
	require.Zero(t, appCount, "application must cascade away with its subscription")
	evExists, err := client.SubscriptionResetEvent.Query().Where(subscriptionresetevent.IDEQ(ev.ID)).Exist(ctx)
	require.NoError(t, err)
	require.True(t, evExists, "event itself must survive subscription hard delete")

	// 3) 订阅被 card.used_subscription_id 引用 → 订阅删除，卡保留且引用置 NULL
	require.NoError(t, hardDeleteSubscription(t, sub2.ID))
	cardAfter, err := client.SubscriptionResetCard.Query().
		Where(subscriptionresetcard.IDEQ(card.ID)).
		Only(ctx)
	require.NoError(t, err)
	require.Nil(t, cardAfter.UsedSubscriptionID,
		"card must survive subscription hard delete with used_subscription_id set to NULL")
	require.Equal(t, user.ID, cardAfter.UserID)

	// 4) 用户硬删 → 卡随用户 CASCADE
	require.NoError(t, hardDeleteUser(t, user.ID))
	cardCount, err := client.SubscriptionResetCard.Query().
		Where(subscriptionresetcard.IDEQ(card.ID)).
		Count(ctx)
	require.NoError(t, err)
	require.Zero(t, cardCount, "cards must cascade with their owner user")
}

func hardDeleteSubscription(t *testing.T, id int64) error {
	t.Helper()
	_, err := integrationDB.ExecContext(context.Background(),
		"DELETE FROM user_subscriptions WHERE id = $1", id)
	return err
}

func hardDeleteUser(t *testing.T, id int64) error {
	t.Helper()
	_, err := integrationDB.ExecContext(context.Background(),
		"DELETE FROM users WHERE id = $1", id)
	return err
}
