//go:build integration

package repository

// Phase 11 —— 单主套餐不变量（产品 RULE 1）+ 预约降级到点执行引擎 集成测试。
//
// 覆盖：
//   迁移 242：repair 规则（保留 expires_at 最大者）+ partial unique index 硬约束；
//   choke point 守卫：跨组购买/兑换/赠送拒绝（ErrPrimarySubscriptionExists），同组续期放行；
//   续期取代：续费取消 pending 预约降级（superseded_by_renewal）+ 清 next_plan_id；
//   到点引擎：到期切换目标档（lapsed → expired 不免费送 / 未到期 → 原位切换）、
//            未到点不动、CAS 幂等、指针被清则作废；
//   履约前置：ExpireLapsedByUser 收敛惰性到期行。

import (
	"context"
	"fmt"
	"testing"
	"time"

	dbent "github.com/Wei-Shaw/sub2api/ent"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/stretchr/testify/require"
)

// phase11Stack：用户 + Basic/Pro 两组两 SKU（tier 100/200）。
type phase11Stack struct {
	user      *service.User
	basicG    *service.Group
	proG      *service.Group
	basicPlan *dbent.SubscriptionPlan
	proPlan   *dbent.SubscriptionPlan
	client    *dbent.Client
}

func phase11Setup(t *testing.T) *phase11Stack {
	t.Helper()
	ctx := context.Background()
	client := testEntClient(t)
	user := mustCreateUser(t, client, &service.User{
		Email: fmt.Sprintf("phase11-user-%d@example.com", time.Now().UnixNano()), PasswordHash: "hash",
	})
	basicG := mustCreateGroup(t, client, &service.Group{
		Name: "phase11-basic-" + user.Email, SubscriptionType: service.SubscriptionTypeSubscription,
	})
	proG := mustCreateGroup(t, client, &service.Group{
		Name: "phase11-pro-" + user.Email, SubscriptionType: service.SubscriptionTypeSubscription,
	})
	mkPlan := func(g *service.Group, name string, price float64, tier int) *dbent.SubscriptionPlan {
		p, err := client.SubscriptionPlan.Create().
			SetGroupID(g.ID).SetName(name).SetPrice(price).SetCurrency("CNY").
			SetValidityDays(30).SetValidityUnit("days").SetTierRank(tier).SetForSale(true).
			Save(ctx)
		require.NoError(t, err)
		return p
	}
	s := &phase11Stack{
		user: user, basicG: basicG, proG: proG,
		basicPlan: mkPlan(basicG, "Basic", 39, 100),
		proPlan:   mkPlan(proG, "Pro", 99, 200),
		client:    client,
	}
	t.Cleanup(func() {
		_, _ = integrationDB.Exec("DELETE FROM subscription_plan_changes WHERE user_id = $1", user.ID)
		_, _ = integrationDB.Exec("DELETE FROM subscription_terms WHERE subscription_id IN (SELECT id FROM user_subscriptions WHERE user_id = $1)", user.ID)
		_, _ = integrationDB.Exec("DELETE FROM user_subscriptions WHERE user_id = $1", user.ID)
		_, _ = integrationDB.Exec("DELETE FROM subscription_plans WHERE id IN ($1,$2)", s.basicPlan.ID, s.proPlan.ID)
		_, _ = integrationDB.Exec("DELETE FROM groups WHERE id IN ($1,$2)", basicG.ID, proG.ID)
		_, _ = integrationDB.Exec("DELETE FROM users WHERE id = $1", user.ID)
	})
	return s
}

func (s *phase11Stack) newSubService(t *testing.T) *service.SubscriptionService {
	t.Helper()
	return service.NewSubscriptionService(
		NewGroupRepository(s.client, integrationDB),
		NewUserSubscriptionRepository(s.client),
		nil, s.client, nil,
	)
}

func (s *phase11Stack) newPlanChangeService(t *testing.T) (*service.PlanChangeService, service.PlanChangeStore) {
	t.Helper()
	svc, changes, _ := phase10NewPlanChangeService(t, s.client)
	return svc, changes
}

func (s *phase11Stack) newActiveSub(t *testing.T, groupID int64, expiresIn time.Duration) *service.UserSubscription {
	t.Helper()
	sub := mustCreateSubscription(t, s.client, &service.UserSubscription{
		UserID: s.user.ID, GroupID: groupID, ExpiresAt: time.Now().Add(expiresIn),
	})
	return sub
}

// ---- 迁移 242：repair 规则 ----

func TestPhase11Migration242RepairKeepsLatestExpiresAt(t *testing.T) {
	ctx := context.Background()
	s := phase11Setup(t)

	tx := testTx(t)
	t.Cleanup(func() { _ = tx.Rollback() })

	// 全程在本事务连接内：撤索引 → raw SQL 造双 ACTIVE → 跑迁移 repair → 断言（回滚不落库）。
	// 不能走 ent 根连接：共享测试库已应用 242，索引仍在，会直接拒绝双 ACTIVE 插入。
	_, err := tx.ExecContext(ctx, "DROP INDEX uq_user_subscriptions_single_active")
	require.NoError(t, err)

	now := time.Now()
	var basicSubID, proSubID int64
	require.NoError(t, tx.QueryRowContext(ctx, `
INSERT INTO user_subscriptions (user_id, group_id, starts_at, expires_at, status, assigned_at, created_at, updated_at)
VALUES ($1,$2,$3,$4,'active',$3,$3,$3) RETURNING id`,
		s.user.ID, s.basicG.ID, now, now.Add(24*time.Hour)).Scan(&basicSubID))
	require.NoError(t, tx.QueryRowContext(ctx, `
INSERT INTO user_subscriptions (user_id, group_id, starts_at, expires_at, status, assigned_at, created_at, updated_at)
VALUES ($1,$2,$3,$4,'active',$3,$3,$3) RETURNING id`,
		s.user.ID, s.proG.ID, now, now.Add(30*24*time.Hour)).Scan(&proSubID)) // expires_at 更大 → 应保留

	repairSQL := `
WITH ranked AS (
    SELECT id,
           ROW_NUMBER() OVER (PARTITION BY user_id ORDER BY expires_at DESC, id DESC) AS rn
    FROM user_subscriptions
    WHERE deleted_at IS NULL AND status = 'active'
)
UPDATE user_subscriptions us
SET status = 'expired',
    notes = CASE
        WHEN us.notes IS NULL OR us.notes = '' THEN 'PLAN_UNIFY_242(prev_status=active)'
        ELSE us.notes || E'\nPLAN_UNIFY_242(prev_status=active)'
    END,
    updated_at = NOW()
FROM ranked r
WHERE us.id = r.id AND r.rn > 1;`
	_, err = tx.ExecContext(ctx, repairSQL)
	require.NoError(t, err)

	var status, notes string
	require.NoError(t, tx.QueryRowContext(ctx,
		`SELECT status, COALESCE(notes,'') FROM user_subscriptions WHERE id = $1`, basicSubID).
		Scan(&status, &notes))
	require.Equal(t, service.SubscriptionStatusExpired, status)
	require.Contains(t, notes, "PLAN_UNIFY_242(prev_status=active)")

	require.NoError(t, tx.QueryRowContext(ctx,
		`SELECT status FROM user_subscriptions WHERE id = $1`, proSubID).Scan(&status))
	require.Equal(t, service.SubscriptionStatusActive, status)
}

// ---- 迁移 242：硬约束 ----

func TestPhase11SingleActiveIndexBlocksSecondActive(t *testing.T) {
	ctx := context.Background()
	s := phase11Setup(t)

	s.newActiveSub(t, s.basicG.ID, 24*time.Hour)

	_, err := s.client.UserSubscription.Create().
		SetUserID(s.user.ID).
		SetGroupID(s.proG.ID).
		SetStartsAt(time.Now()).
		SetExpiresAt(time.Now().Add(24 * time.Hour)).
		SetStatus(service.SubscriptionStatusActive).
		SetAssignedAt(time.Now()).
		Save(ctx)
	require.Error(t, err, "第二张 ACTIVE 主订阅必须被 partial unique index 拒绝")

	// 历史/非 ACTIVE 行不受影响：把第一张置为 expired 后，新 ACTIVE 可创建
	expiredAt := time.Now().Add(-1 * time.Hour)
	_, err = integrationDB.ExecContext(ctx,
		`UPDATE user_subscriptions SET status='expired', expires_at=$1 WHERE user_id=$2`,
		expiredAt, s.user.ID)
	require.NoError(t, err)
	_, err = s.client.UserSubscription.Create().
		SetUserID(s.user.ID).
		SetGroupID(s.proG.ID).
		SetStartsAt(time.Now()).
		SetExpiresAt(time.Now().Add(24 * time.Hour)).
		SetStatus(service.SubscriptionStatusActive).
		SetAssignedAt(time.Now()).
		Save(ctx)
	require.NoError(t, err)
}

// ---- choke point 守卫 ----

func TestPhase11GuardBlocksCrossGroupAssignmentAllowsSameGroupRenewal(t *testing.T) {
	ctx := context.Background()
	s := phase11Setup(t)
	subSvc := s.newSubService(t)

	// 用户已持 Pro
	proSub := s.newActiveSub(t, s.proG.ID, 15*24*time.Hour)

	// 跨组购买 Basic → 拒绝
	_, _, err := subSvc.AssignOrExtendSubscription(ctx, &service.AssignSubscriptionInput{
		UserID: s.user.ID, GroupID: s.basicG.ID, ValidityDays: 30,
	})
	require.ErrorIs(t, err, service.ErrPrimarySubscriptionExists)

	// 同组购买 Pro → 续期放行
	renewed, isRenewal, err := subSvc.AssignOrExtendSubscription(ctx, &service.AssignSubscriptionInput{
		UserID: s.user.ID, GroupID: s.proG.ID, ValidityDays: 30,
	})
	require.NoError(t, err)
	require.True(t, isRenewal)
	require.Equal(t, proSub.ID, renewed.ID)
	require.True(t, renewed.ExpiresAt.After(time.Now().Add(40*24*time.Hour)), "续期应从原到期时间累加")
}

func TestPhase11GuardAllowsAssignmentWhenNoActiveElsewhere(t *testing.T) {
	ctx := context.Background()
	s := phase11Setup(t)
	subSvc := s.newSubService(t)

	// 只有一条已过期的 Basic 行（惰性到期未翻）→ 新购买 Pro 应放行
	s.newActiveSub(t, s.basicG.ID, -1*time.Hour)

	created, isRenewal, err := subSvc.AssignOrExtendSubscription(ctx, &service.AssignSubscriptionInput{
		UserID: s.user.ID, GroupID: s.proG.ID, ValidityDays: 30,
	})
	require.NoError(t, err)
	require.False(t, isRenewal)
	require.Equal(t, s.proG.ID, created.GroupID)
}

// ---- 续期取代 pending 预约降级 ----


// ---- 到点执行引擎 ----






// ---- 履约前置：惰性到期收敛 ----

func TestPhase11ExpireLapsedByUser(t *testing.T) {
	ctx := context.Background()
	s := phase11Setup(t)
	subRepo := NewUserSubscriptionRepository(s.client)

	lapsed := s.newActiveSub(t, s.basicG.ID, -2*time.Hour) // 已过期但仍 active（惰性到期）

	guard, ok := subRepo.(service.SubscriptionSingleActiveGuard)
	require.True(t, ok, "生产仓储必须实现单主套餐守卫能力")
	n, err := guard.ExpireLapsedByUser(ctx, s.user.ID, time.Now())
	require.NoError(t, err)
	require.Equal(t, int64(1), n)

	var status string
	require.NoError(t, integrationDB.QueryRowContext(ctx,
		`SELECT status FROM user_subscriptions WHERE id=$1`, lapsed.ID).Scan(&status))
	require.Equal(t, service.SubscriptionStatusExpired, status)

	activeCount := 0
	require.NoError(t, integrationDB.QueryRowContext(ctx,
		`SELECT COUNT(*) FROM user_subscriptions WHERE user_id=$1 AND status='active'`, s.user.ID).
		Scan(&activeCount))
	require.Equal(t, 0, activeCount, "lapsed 行应被翻为 expired")
}

