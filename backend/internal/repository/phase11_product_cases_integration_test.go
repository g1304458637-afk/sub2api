//go:build integration

package repository

// Phase 11 —— 产品规则验收旅程（Phase 9 Case A–G 的后端语义等价 E2E）。
//
// 一条用户旅程串起全部产品规则（RULE 1–7），替代需要完整 UI 栈的手工
// pricinguser 流程；前端呈现由组件测试 + 状态合同覆盖。
//
//	A 购买 Basic → Basic ACTIVE 且唯一（setup + 单一 ACTIVE 断言）
//	B Basic→Pro 立即升级 + 剩余周期折抵 + Basic 不再 ACTIVE
//	C Pro 预约 Basic：立即不生效、Group 不变、不退款（无支付动作）
//	D 状态可从服务合同完整恢复（重登/刷新语义：GetAccountStatus 重读）
//	E term 末到点引擎：Pro→Basic 生效、续费后新周期以 Basic 开始
//	F Max 起点的预约可替换：pending Basic → 改为 pending Pro，只有一条 pending
//	G 有 pending 时升级：取消 pending、立即生效、pending 清空
//
// 注：F/G 需要当前档为 Max（顶档替换/次顶档升级+pending），旅程顺序 B→G(C)→F。

import (
	"context"
	"testing"
	"time"

	dbent "github.com/Wei-Shaw/sub2api/ent"
	"github.com/Wei-Shaw/sub2api/ent/subscriptionplan"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/stretchr/testify/require"
)

func phase11Upgrade(t *testing.T, s *phase10Stack, subID, toPlanID int64, key string) {
	t.Helper()
	_, changeID, err := s.svc.CreateUpgradeQuote(context.Background(), s.user.ID, subID, toPlanID, key)
	require.NoError(t, err)
	require.NoError(t, s.changes.MarkPaid(context.Background(), changeID))
	require.NoError(t, s.svc.FulfillUpgrade(context.Background(), changeID))
}

func phase11ActiveGroup(t *testing.T, _ *dbent.Client, subID int64) int64 {
	t.Helper()
	var gid int64
	row := integrationDB.QueryRow("SELECT group_id FROM user_subscriptions WHERE id = $1", subID)
	require.NoError(t, row.Scan(&gid))
	return gid
}

func TestPhase11ProductCasesJourney(t *testing.T) {
	client := testEntClient(t)
	ctx := context.Background()

	// ── Case A：购买 Basic → Basic ACTIVE 且唯一 ──
	s := phase10Setup(t, client, 39, 99, 0, 30) // Basic 30d，全剩余
	require.Equal(t, s.basicG.ID, phase11ActiveGroup(t, client, s.basicSub.ID))
	subs, err := NewUserSubscriptionRepository(client).ListActiveByUserID(ctx, s.user.ID)
	require.NoError(t, err)
	require.Len(t, subs, 1)

	statusSvc := service.NewAccountStatusService(
		NewUserRepository(client, integrationDB), NewUserSubscriptionRepository(client),
		NewGroupRepository(client, integrationDB), nil, nil, true)

	// ── Case B：Basic → Pro 立即升级 ──
	phase11Upgrade(t, s, s.basicSub.ID, s.proPlan.ID, "p11-journey-b")
	require.Equal(t, s.proG.ID, phase11ActiveGroup(t, client, s.basicSub.ID), "upgrade applies immediately")
	subs, err = NewUserSubscriptionRepository(client).ListActiveByUserID(ctx, s.user.ID)
	require.NoError(t, err)
	require.Len(t, subs, 1, "exactly one ACTIVE after upgrade")
	var status string
	require.NoError(t, integrationDB.QueryRowContext(ctx,
		`SELECT status FROM user_subscriptions WHERE id=$1`, s.basicSub.ID).Scan(&status))
	require.Equal(t, service.SubscriptionStatusActive, status, "same row, not a second subscription")

	// ── Case C：Pro 预约 Basic —— 立即不生效、Group 不变、无退款动作 ──
	_, err = s.svc.ScheduleDowngrade(ctx, s.user.ID, s.basicSub.ID, s.basicPlan.ID, "p11-c")
	require.NoError(t, err)
	require.Equal(t, s.proG.ID, phase11ActiveGroup(t, client, s.basicSub.ID), "downgrade must not apply early")
	pending, err := s.changes.ActiveScheduledChange(ctx, s.basicSub.ID)
	require.NoError(t, err)
	require.NotNil(t, pending)
	require.Equal(t, "scheduled", pending.Status)

	// ── Case G：有 pending 时升级 Max → 取消 pending、立即生效 ──
	maxPlan, err := client.SubscriptionPlan.Query().
		Where(subscriptionplan.GroupIDEQ(s.maxG.ID)).
		Only(ctx)
	require.NoError(t, err)
	maxPlanID := maxPlan.ID
	phase11Upgrade(t, s, s.basicSub.ID, maxPlanID, "p11-journey-g")
	require.Equal(t, s.maxG.ID, phase11ActiveGroup(t, client, s.basicSub.ID))
	afterUpgrade, err := s.changes.ActiveScheduledChange(ctx, s.basicSub.ID)
	require.NoError(t, err)
	require.Nil(t, afterUpgrade, "pending downgrade cancelled by upgrade")
	subs, err = NewUserSubscriptionRepository(client).ListActiveByUserID(ctx, s.user.ID)
	require.NoError(t, err)
	require.Len(t, subs, 1)

	// ── Case F：Max 起点预约 Basic → 改为 Pro：只有一条 pending（Pro） ──
	_, err = s.svc.ScheduleDowngrade(ctx, s.user.ID, s.basicSub.ID, s.basicPlan.ID, "p11-f-1")
	require.NoError(t, err)
	_, err = s.svc.ScheduleDowngrade(ctx, s.user.ID, s.basicSub.ID, s.proPlan.ID, "p11-f-2")
	require.NoError(t, err)
	onlyPending, err := s.changes.ActiveScheduledChange(ctx, s.basicSub.ID)
	require.NoError(t, err)
	require.NotNil(t, onlyPending)
	require.Equal(t, s.proPlan.ID, onlyPending.ToPlanID, "replacement pending points at Pro")
	var pendingRows int
	require.NoError(t, integrationDB.QueryRowContext(ctx,
		`SELECT COUNT(*) FROM subscription_plan_changes
		 WHERE subscription_id=$1 AND change_type='scheduled_downgrade' AND status='scheduled'`,
		s.basicSub.ID).Scan(&pendingRows))
	require.Equal(t, 1, pendingRows, "at most one pending change")

	// ── Case E：term 末到点 → 切到 Pro 档行；续费后新周期以 Pro 开始 ──
	now := time.Now()
	_, err = integrationDB.ExecContext(ctx,
		`UPDATE user_subscriptions SET expires_at=$1 WHERE id=$2`, now.Add(-time.Hour), s.basicSub.ID)
	require.NoError(t, err)
	_, err = integrationDB.ExecContext(ctx,
		`UPDATE subscription_plan_changes SET effective_at=$1 WHERE subscription_id=$2 AND status='scheduled'`,
		now.Add(-30*time.Minute), s.basicSub.ID)
	require.NoError(t, err)
	applied, err := s.svc.ApplyDueScheduledDowngrades(ctx, now, 100)
	require.NoError(t, err)
	require.Equal(t, 1, applied)
	require.Equal(t, s.proG.ID, phase11ActiveGroup(t, client, s.basicSub.ID))
	require.NoError(t, integrationDB.QueryRowContext(ctx,
		`SELECT status FROM user_subscriptions WHERE id=$1`, s.basicSub.ID).Scan(&status))
	require.Equal(t, service.SubscriptionStatusExpired, status,
		"prepaid model: term ended and user has not renewed - no free Pro period")

	// ── Case D：刷新/重登 —— 状态合同重读与库内事实一致（此刻：已到期、无预约） ──
	full, err := statusSvc.GetAccountStatus(ctx, s.user.ID)
	require.NoError(t, err)
	require.Len(t, full.Subscriptions, 0, "nothing active between term end and renewal")
	require.Nil(t, full.PendingChange, "fulfilled downgrade no longer pending")

	// 按目标档（Pro）续费 → 同一行复活，新周期 Pro
	subSvc := service.NewSubscriptionService(
		NewGroupRepository(client, integrationDB), NewUserSubscriptionRepository(client), nil, client, nil)
	renewed, isRenewal, err := subSvc.AssignOrExtendSubscription(ctx, &service.AssignSubscriptionInput{
		UserID: s.user.ID, GroupID: s.proG.ID, ValidityDays: 30, PlanID: &s.proPlan.ID,
	})
	require.NoError(t, err)
	require.True(t, isRenewal)
	require.Equal(t, s.basicSub.ID, renewed.ID, "renewal reuses the same subscription row")
	require.Equal(t, service.SubscriptionStatusActive, renewed.Status)
	require.True(t, renewed.ExpiresAt.After(now), "new period starts from renewal")

	// 续费后恢复单一 ACTIVE（Case A 不变量的闭环）
	subs, err = NewUserSubscriptionRepository(client).ListActiveByUserID(ctx, s.user.ID)
	require.NoError(t, err)
	require.Len(t, subs, 1)
	require.Equal(t, s.proG.ID, subs[0].GroupID)
}
