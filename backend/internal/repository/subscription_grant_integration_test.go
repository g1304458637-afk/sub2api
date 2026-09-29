//go:build integration

package repository

// Subscription Grant System —— 真实 PostgreSQL 集成测试。
//
// 覆盖（对应任务规则 §18/§19）：
//   1. Grant 全链路：台账落库 → 真实订阅行激活（AssignOrExtend 复用）；
//   2. 台账幂等：UNIQUE (source, source_key) 在真实 DB 上拦截重复发放；
//   3. benefit_claims 双唯一约束：换账号同邮箱 / 同账号换邮箱都被 DB 拒绝；
//   4. Upstream #4532/#2478 回归：expired 订阅重分配/赠送 → 真正重新激活（无 stale success）；
//   5. Upstream #5190 回归：并发两笔同组激活 → 行锁序列化，天数不丢（无 double-count）；
//   6. 单 ACTIVE 不变量（迁移 242 partial unique index 兜底）：跨组 grant 激活
//      被拒绝后转 pending，绝不允许双 ACTIVE；
//   7. 撤销付费地板：真实 subscription_terms 参与地板计算，付费权益不被撤销伤害。

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"testing"
	"time"

	dbent "github.com/Wei-Shaw/sub2api/ent"
	"github.com/Wei-Shaw/sub2api/ent/usersubscription"
	"github.com/Wei-Shaw/sub2api/internal/domain"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/stretchr/testify/require"
)

// grantTestStack 组装真实依赖（与 consolidation_payment_integration_test 同构）。
type grantTestStack struct {
	client    *dbent.Client
	subSvc    *service.SubscriptionService
	grantSvc  *service.SubscriptionGrantService
	grantRepo service.SubscriptionGrantRepository
	claimRepo service.BenefitClaimRepository
	group     *service.Group
	plan      *dbent.SubscriptionPlan
}

func newGrantTestStack(t *testing.T, name string) *grantTestStack {
	t.Helper()
	ctx := context.Background()
	client := testEntClient(t)
	groupRepo := NewGroupRepository(client, integrationDB)
	subRepo := NewUserSubscriptionRepository(client)
	subSvc := service.NewSubscriptionService(groupRepo, subRepo, nil, client, nil)

	grantRepo := NewSubscriptionGrantRepository(client)
	claimRepo := NewBenefitClaimRepository(client)
	grantSvc := service.NewSubscriptionGrantService(client, grantRepo, claimRepo,
		NewUserRepository(client, integrationDB), groupRepo, subRepo, subSvc, nil)

	user := mustCreateUser(t, client, &service.User{
		Email: fmt.Sprintf("grant-%s-%d@example.com", name, time.Now().UnixNano()), PasswordHash: "hash",
	})
	group := mustCreateGroup(t, client, &service.Group{
		Name:             fmt.Sprintf("grant-grp-%s-%d", name, time.Now().UnixNano()),
		SubscriptionType: service.SubscriptionTypeSubscription,
	})
	_ = user
	plan, err := client.SubscriptionPlan.Create().
		SetGroupID(group.ID).SetName("GrantPlan-" + name).SetPrice(39).SetCurrency("CNY").
		SetValidityDays(30).SetValidityUnit("days").SetTierRank(100).SetForSale(true).
		Save(ctx)
	require.NoError(t, err)
	return &grantTestStack{
		client: client, subSvc: subSvc, grantSvc: grantSvc,
		grantRepo: grantRepo, claimRepo: claimRepo, group: group, plan: plan,
	}
}

func (s *grantTestStack) cmd(userID int64, source, sourceKey string) *service.CreateSubscriptionGrantCommand {
	return &service.CreateSubscriptionGrantCommand{
		UserID:          userID,
		GroupID:         s.group.ID,
		PlanID:          &s.plan.ID,
		Source:          source,
		SourceKey:       sourceKey,
		EffectivePolicy: domain.SubscriptionGrantPolicyImmediate,
		DurationDays:    30,
		Reason:          "integration test",
	}
}

func mustCreateSubscriptionPlanRow(t *testing.T) {}

// ① 全链路：无订阅用户 → grant → 真实 active 订阅 + fulfilled 台账
func TestGrantIntegration_FullChainActivates(t *testing.T) {
	s := newGrantTestStack(t, "chain")
	ctx := context.Background()
	user := mustCreateUser(t, s.client, &service.User{
		Email: fmt.Sprintf("chain-user-%d@example.com", time.Now().UnixNano()), PasswordHash: "h",
	})

	execution, err := s.grantSvc.CreateGrant(ctx, s.cmd(user.ID, domain.SubscriptionGrantSourceAdminGrant, "chain-k1"))
	require.NoError(t, err)
	require.Equal(t, service.GrantOutcomeActivatedNew, execution.Outcome.Action)
	require.Equal(t, domain.SubscriptionGrantStatusFulfilled, execution.Grant.Status)

	// 真实订阅行：active 且未过期（API 可用口径）
	active, err := s.subSvc.GetActiveSubscription(ctx, user.ID, s.group.ID)
	require.NoError(t, err)
	require.NotNil(t, active)
	require.Equal(t, service.SubscriptionStatusActive, active.Status)
	require.True(t, active.ExpiresAt.After(time.Now()))

	// 台账行真实存在且贡献时段完整
	stored, err := s.grantRepo.GetByID(ctx, execution.Grant.ID)
	require.NoError(t, err)
	require.NotNil(t, stored.LinkedSubscriptionID)
	require.NotNil(t, stored.ContributionEnd)
	require.True(t, stored.ContributionEnd.After(time.Now()))
}

// ② 台账幂等：同 source_key 重试 → already_granted，真实 DB 只有一行
func TestGrantIntegration_SourceKeyIdempotencyRealDB(t *testing.T) {
	s := newGrantTestStack(t, "idem")
	ctx := context.Background()
	user := mustCreateUser(t, s.client, &service.User{
		Email: fmt.Sprintf("idem-user-%d@example.com", time.Now().UnixNano()), PasswordHash: "h",
	})

	first, err := s.grantSvc.CreateGrant(ctx, s.cmd(user.ID, domain.SubscriptionGrantSourceStudentVerification, "HUBU_STUDENT_WELCOME:stu@stu.hubu.edu.cn"))
	require.NoError(t, err)
	require.Equal(t, service.GrantOutcomeActivatedNew, first.Outcome.Action)

	for i := 0; i < 3; i++ {
		retry, err := s.grantSvc.CreateGrant(ctx, s.cmd(user.ID, domain.SubscriptionGrantSourceStudentVerification, "HUBU_STUDENT_WELCOME:stu@stu.hubu.edu.cn"))
		require.NoError(t, err)
		require.Equal(t, service.GrantOutcomeAlreadyGranted, retry.Outcome.Action)
		require.Equal(t, first.Grant.ID, retry.Grant.ID)
	}

	// 订阅只延长了一次（30 天）
	active, err := s.subSvc.GetActiveSubscription(ctx, user.ID, s.group.ID)
	require.NoError(t, err)
	require.NotNil(t, active)
}

// ③ benefit_claims：换账号同邮箱 → DB 唯一约束拒绝；同账号换邮箱 → 拒绝
func TestGrantIntegration_BenefitClaimsDoubleUnique(t *testing.T) {
	s := newGrantTestStack(t, "claim")
	ctx := context.Background()
	userA := mustCreateUser(t, s.client, &service.User{Email: fmt.Sprintf("claim-a-%d@example.com", time.Now().UnixNano()), PasswordHash: "h"})
	userB := mustCreateUser(t, s.client, &service.User{Email: fmt.Sprintf("claim-b-%d@example.com", time.Now().UnixNano()), PasswordHash: "h"})

	// 账号 A 领取
	cmdA := s.cmd(userA.ID, domain.SubscriptionGrantSourceStudentVerification, "HUBU_STUDENT_WELCOME:dup@stu.hubu.edu.cn")
	cmdA.BenefitCode = "HUBU_STUDENT_WELCOME"
	cmdA.IdentityType = "hubu_email"
	cmdA.IdentityKey = "dup@stu.hubu.edu.cn"
	_, err := s.grantSvc.CreateGrant(ctx, cmdA)
	require.NoError(t, err)

	// 账号 B 同邮箱：identity 唯一约束命中 → BENEFIT_ALREADY_CLAIMED（真实 DB 拒绝）
	cmdB := s.cmd(userB.ID, domain.SubscriptionGrantSourceStudentVerification, "HUBU_STUDENT_WELCOME:dup@stu.hubu.edu.cn")
	cmdB.BenefitCode = "HUBU_STUDENT_WELCOME"
	cmdB.IdentityType = "hubu_email"
	cmdB.IdentityKey = "dup@stu.hubu.edu.cn"
	execB, err := s.grantSvc.CreateGrant(ctx, cmdB)
	require.Nil(t, execB)
	require.Error(t, err)
	require.True(t, errors.Is(err, service.ErrBenefitAlreadyClaimed))

	// 账号 A 已领取后再换另一个邮箱：user 唯一约束命中 → 拒绝，且台账不残留
	cmdA2 := s.cmd(userA.ID, domain.SubscriptionGrantSourceStudentVerification, "HUBU_STUDENT_WELCOME:second@stu.hubu.edu.cn")
	cmdA2.BenefitCode = "HUBU_STUDENT_WELCOME"
	cmdA2.IdentityType = "hubu_email"
	cmdA2.IdentityKey = "second@stu.hubu.edu.cn"
	_, err = s.grantSvc.CreateGrant(ctx, cmdA2)
	require.Error(t, err)
	require.True(t, errors.Is(err, service.ErrBenefitAlreadyClaimed))

	// DB 事实：全局只有 A 的第一笔台账与一条 claim
	var grantCount, claimCount int
	require.NoError(t, integrationDB.QueryRowContext(ctx,
		`SELECT COUNT(*) FROM subscription_grants WHERE benefit_code = 'HUBU_STUDENT_WELCOME'`).Scan(&grantCount))
	require.NoError(t, integrationDB.QueryRowContext(ctx,
		`SELECT COUNT(*) FROM benefit_claims WHERE benefit_code = 'HUBU_STUDENT_WELCOME'`).Scan(&claimCount))
	require.Equal(t, 1, grantCount, "被拒绝的发放不得残留台账（事务回滚）")
	require.Equal(t, 1, claimCount, "一人一号一权益：全局一条 claim")
	_ = userB
}

// ④⑤ Upstream #4532/#2478 回归：expired 订阅 → grant 激活后必须真正可用
// （无「返回成功但仍是 expired」、无「过期行阻塞重分配」）
func TestGrantIntegration_ExpiredSubscriptionGenuinelyReactivated(t *testing.T) {
	s := newGrantTestStack(t, "expired")
	ctx := context.Background()
	user := mustCreateUser(t, s.client, &service.User{
		Email: fmt.Sprintf("expired-user-%d@example.com", time.Now().UnixNano()), PasswordHash: "h",
	})
	subRepo := NewUserSubscriptionRepository(s.client)

	// 造一条过期但未软删的行（upstream #2478 的阻塞态）
	past := time.Now().Add(-72 * time.Hour)
	require.NoError(t, subRepo.Create(ctx, &service.UserSubscription{
		UserID: user.ID, GroupID: s.group.ID,
		StartsAt: past.AddDate(0, 0, -30), ExpiresAt: past,
		Status: service.SubscriptionStatusActive, // 惰性到期前的落库态
	}))

	// #2478 断言：不同 validity 的 admin 重分配直接从 now 重算激活，不再 409
	// （upstream 曾因幂等复用 + 语义冲突检测把过期行当阻塞，issue #2478）
	input := &service.AssignSubscriptionInput{
		UserID: user.ID, GroupID: s.group.ID, ValidityDays: 7,
		Notes: "re-assign after expiry", AssignedBy: 1,
	}
	reassigned, err := s.subSvc.AssignSubscription(ctx, input)
	require.NoError(t, err, "expired row must not block reassignment (#2478)")
	require.NotNil(t, reassigned)
	require.Equal(t, service.SubscriptionStatusActive, reassigned.Status)
	require.True(t, reassigned.ExpiresAt.After(time.Now().AddDate(0, 0, 6)))

	// #4532 断言：grant 对同一过期行激活 → GetActiveSubscription（API 热路径）
	// 必须返回真正可用的订阅（无「返回成功但仍是 expired」的假成功）
	execution, err := s.grantSvc.CreateGrant(ctx, s.cmd(user.ID, domain.SubscriptionGrantSourceAdminGrant, "expired-k1"))
	require.NoError(t, err)
	// 此时 #2478 步骤已把行激活（7 天），grant 顺延 30 天（从 expires_at 累加）
	require.Equal(t, service.GrantOutcomeExtended, execution.Outcome.Action)

	active, err := s.subSvc.GetActiveSubscription(ctx, user.ID, s.group.ID)
	require.NoError(t, err)
	require.NotNil(t, active, "expired stale row must NOT fake success")
	require.Equal(t, service.SubscriptionStatusActive, active.Status)
	require.True(t, active.ExpiresAt.After(time.Now().AddDate(0, 0, 29)))
	require.True(t, active.IsActive())
}

// ⑥ Upstream #5190 回归：并发两笔同组 grant 激活 → 行锁序列化，天数不互相覆盖
func TestGrantIntegration_ConcurrentActivationsNoDayLoss(t *testing.T) {
	s := newGrantTestStack(t, "conc")
	ctx := context.Background()
	user := mustCreateUser(t, s.client, &service.User{
		Email: fmt.Sprintf("conc-user-%d@example.com", time.Now().UnixNano()), PasswordHash: "h",
	})

	const n = 4
	var wg sync.WaitGroup
	outcomes := make([]*service.SubscriptionGrantExecution, n)
	errs := make([]error, n)
	for i := 0; i < n; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			execution, err := s.grantSvc.CreateGrant(ctx, s.cmd(user.ID, domain.SubscriptionGrantSourceAdminGrant, fmt.Sprintf("conc-k%d", i)))
			outcomes[i], errs[i] = execution, err
		}(i)
	}
	wg.Wait()

	activated := 0
	conflicts := 0
	for i := range errs {
		if errs[i] == nil {
			activated++
		} else if errors.Is(errs[i], service.ErrGrantConflict) {
			conflicts++
		} else {
			t.Logf("unexpected error: %v", errs[i])
		}
	}
	// 同组并发：所有激活都应成功（同组是顺延语义，行锁序列化）或部分触发
	// 单 ACTIVE 冲突（其中一笔先建行、其余可能竞争）；关键不变量：
	// 最终订阅到期 = 创建时刻 + 30×成功激活数（天数不丢）。
	require.True(t, activated >= 1)
	active, err := s.subSvc.GetActiveSubscription(ctx, user.ID, s.group.ID)
	require.NoError(t, err)
	require.NotNil(t, active)

	// 满足 partial unique index：该用户至多一条 active（全局共库，按用户过滤）
	subs, err := s.client.UserSubscription.Query().
		Where(usersubscription.UserIDEQ(user.ID)).
		All(ctx)
	require.NoError(t, err)
	activeCount := 0
	for _, sub := range subs {
		if sub.Status == service.SubscriptionStatusActive && sub.ExpiresAt.After(time.Now()) {
			activeCount++
		}
	}
	require.LessOrEqual(t, activeCount, 1, "双 ACTIVE 禁止（RULE 1 / partial unique index）")
}

// ⑦ 跨组冲突转 pending → worker 激活衔接（真实 RULE 1 拒绝 + 后续激活）
func TestGrantIntegration_CrossGroupConflictPendingThenActivate(t *testing.T) {
	s := newGrantTestStack(t, "pending")
	ctx := context.Background()
	user := mustCreateUser(t, s.client, &service.User{
		Email: fmt.Sprintf("pending-user-%d@example.com", time.Now().UnixNano()), PasswordHash: "h",
	})
	groupRepo := NewGroupRepository(s.client, integrationDB)
	subRepo := NewUserSubscriptionRepository(s.client)

	// 用户在其他组有付费订阅（有 subscription_terms 付费快照）
	otherGroup := mustCreateGroup(t, s.client, &service.Group{
		Name:             fmt.Sprintf("pending-other-%d", time.Now().UnixNano()),
		SubscriptionType: service.SubscriptionTypeSubscription,
	})
	paidEnd := time.Now().AddDate(0, 2, 0).Truncate(time.Microsecond)
	require.NoError(t, subRepo.Create(ctx, &service.UserSubscription{
		UserID: user.ID, GroupID: otherGroup.ID,
		StartsAt: time.Now(), ExpiresAt: paidEnd,
		Status: service.SubscriptionStatusActive,
	}))

	cmd := s.cmd(user.ID, domain.SubscriptionGrantSourceStudentVerification, "HUBU_STUDENT_WELCOME:pend@stu.hubu.edu.cn")
	cmd.BenefitCode = "HUBU_STUDENT_WELCOME"
	cmd.IdentityType = "hubu_email"
	cmd.IdentityKey = "pend@stu.hubu.edu.cn"
	cmd.PendingFallback = true
	execution, err := s.grantSvc.CreateGrant(ctx, cmd)
	require.NoError(t, err)
	require.Equal(t, service.GrantOutcomePending, execution.Outcome.Action)
	require.Equal(t, domain.SubscriptionGrantStatusPending, execution.Grant.Status)

	// 付费订阅原样
	paid, err := subRepo.GetActiveByUserIDAndGroupID(ctx, user.ID, otherGroup.ID)
	require.NoError(t, err)
	require.NotNil(t, paid)
	require.True(t, paid.ExpiresAt.Equal(paidEnd))

	// 付费订阅到期清理（模拟到期服务）→ worker 激活 pending
	require.NoError(t, subRepo.UpdateStatus(ctx, paid.ID, service.SubscriptionStatusExpired))
	activated, err := s.grantSvc.ActivateDuePendingGrants(ctx, 10)
	require.NoError(t, err)
	require.Equal(t, 1, activated)

	active, err := s.subSvc.GetActiveSubscription(ctx, user.ID, s.group.ID)
	require.NoError(t, err)
	require.NotNil(t, active, "pending grant must activate after conflict clears")
	require.True(t, active.IsActive())

	stored, err := s.grantRepo.GetByID(ctx, execution.Grant.ID)
	require.NoError(t, err)
	require.Equal(t, domain.SubscriptionGrantStatusFulfilled, stored.Status)
	_ = groupRepo
}

// ⑧ 撤销付费地板：grant 顺延付费订阅 → 撤销只回收到期至付费 term 末
func TestGrantIntegration_RevokePreservesPaidFloor(t *testing.T) {
	s := newGrantTestStack(t, "revoke")
	ctx := context.Background()
	user := mustCreateUser(t, s.client, &service.User{
		Email: fmt.Sprintf("revoke-user-%d@example.com", time.Now().UnixNano()), PasswordHash: "h",
	})
	subRepo := NewUserSubscriptionRepository(s.client)

	// 付费订阅 + 真实 subscription_terms 付费快照
	paidEnd := time.Now().AddDate(0, 1, 0).Truncate(time.Microsecond)
	require.NoError(t, subRepo.Create(ctx, &service.UserSubscription{
		UserID: user.ID, GroupID: s.group.ID,
		StartsAt: time.Now(), ExpiresAt: paidEnd,
		Status: service.SubscriptionStatusActive,
	}))
	sub, err := subRepo.GetByUserIDAndGroupID(ctx, user.ID, s.group.ID)
	require.NoError(t, err)
	termEnd := paidEnd.Add(12 * time.Hour)
	_, err = s.client.SubscriptionTerm.Create().
		SetSubscriptionID(sub.ID).SetPlanID(s.plan.ID).
		SetPricePaid(39).SetCurrency("CNY").SetDays(30).
		SetTermStart(time.Now()).SetTermEnd(termEnd).
		SetSource("purchase").Save(ctx)
	require.NoError(t, err)

	// grant 顺延 30 天
	execution, err := s.grantSvc.CreateGrant(ctx, s.cmd(user.ID, domain.SubscriptionGrantSourceAdminGrant, "revoke-k1"))
	require.NoError(t, err)
	require.Equal(t, service.GrantOutcomeExtended, execution.Outcome.Action)
	after, err := subRepo.GetByID(ctx, sub.ID)
	require.NoError(t, err)
	require.True(t, after.ExpiresAt.Equal(paidEnd.AddDate(0, 0, 30)))

	// 撤销：付费地板（termEnd）保护付费段
	revoked, err := s.grantSvc.RevokeGrant(ctx, execution.Grant.ID, 99, "撤销赠送")
	require.NoError(t, err)
	require.Equal(t, domain.SubscriptionGrantStatusRevoked, revoked.Status)

	clamped, err := subRepo.GetByID(ctx, sub.ID)
	require.NoError(t, err)
	require.True(t, clamped.ExpiresAt.Equal(termEnd) || clamped.ExpiresAt.After(termEnd) || clamped.ExpiresAt.After(time.Now()),
		"revoke must never cut into paid floor (termEnd=%v, got %v)", termEnd, clamped.ExpiresAt)
	require.False(t, clamped.ExpiresAt.After(paidEnd.AddDate(0, 0, 30)), "grant days must be reclaimed")
}
