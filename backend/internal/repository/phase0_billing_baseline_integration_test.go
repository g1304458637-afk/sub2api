//go:build integration

package repository

// Phase 0 — Existing Billing Baseline Lock.
//
// 本文件只锁定现有计费/订阅/并发行为，作为 Subscription V1 改造的回归基线。
// 不修改任何生产代码；所有断言描述的是当前仓库的真实行为。
//
// 锁定范围：
//   §1  PAYG settlement（BalanceCost → users.balance 扣减）
//   §2  Subscription settlement（SubscriptionCost → user_subscriptions 用量累加，balance 不变）
//   §3  billing dedup（同 request_id+api_key_id 二次结算不重复落账）
//   §7  ResetWeeklyUsage CAS（命中 / stale no-op）
//   §8  Reset ↔ Settlement 两种顺序 + 并发不变量（禁止 lost update 覆盖新周期）
//   §10 Weekly limit overshoot（settlement 无条件累加，越线后下一请求被拒）
//   §11 并发 overshoot（并发全通过预检时的最大超额测量）
//   §14 用户并发槽（limit=2：acquire/acquire/reject/release）
//   §15 多 API Key / 多分组共用同一用户并发池（同 user ZSET 证据）
//   §16 多订阅并存（每分组独立 anchor 与用量）

import (
	"context"
	"fmt"
	"sync"
	"testing"
	"time"

	dbent "github.com/Wei-Shaw/sub2api/ent"
	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"github.com/Wei-Shaw/sub2api/internal/service"
)

// ---- helpers ----

// phase0CleanupStack 登记测试数据清理：testEntClient 的写入不会自动回滚，
// 而 package 内存量 List 类测试对共享库做无过滤断言，Phase 0 数据必须事后删除。
func phase0CleanupStack(t *testing.T, userID, groupID, accountID int64) {
	t.Helper()
	t.Cleanup(func() {
		ctx := context.Background()
		_, _ = integrationDB.ExecContext(ctx,
			"DELETE FROM usage_billing_dedup WHERE api_key_id IN (SELECT id FROM api_keys WHERE user_id = $1)", userID)
		_, _ = integrationDB.ExecContext(ctx, "DELETE FROM user_subscriptions WHERE user_id = $1", userID)
		_, _ = integrationDB.ExecContext(ctx, "DELETE FROM api_keys WHERE user_id = $1", userID)
		_, _ = integrationDB.ExecContext(ctx, "DELETE FROM accounts WHERE id = $1", accountID)
		_, _ = integrationDB.ExecContext(ctx, "DELETE FROM users WHERE id = $1", userID)
		_, _ = integrationDB.ExecContext(ctx, "DELETE FROM groups WHERE id = $1", groupID)
	})
}

func phase0MustStack(
	t *testing.T,
	client *dbent.Client,
	subscriptionType string,
	balance float64,
	weeklyLimit *float64,
) (*service.User, *service.Group, *service.UserSubscription, *service.APIKey, *service.Account) {
	t.Helper()

	user := mustCreateUser(t, client, &service.User{
		Email:        fmt.Sprintf("phase0-sub-%d@example.com", time.Now().UnixNano()),
		PasswordHash: "hash",
		Balance:      balance,
	})
	group := mustCreateGroup(t, client, &service.Group{
		Name:             "phase0-sub-group-" + uuid.NewString(),
		SubscriptionType: subscriptionType,
		WeeklyLimitUSD:   weeklyLimit,
	})
	var sub *service.UserSubscription
	if subscriptionType == service.SubscriptionTypeSubscription {
		sub = mustCreateSubscription(t, client, &service.UserSubscription{
			UserID:  user.ID,
			GroupID: group.ID,
		})
	}
	apiKey := mustCreateApiKey(t, client, &service.APIKey{
		UserID:  user.ID,
		GroupID: &group.ID,
		Key:     "sk-phase0-sub-" + uuid.NewString(),
		Name:    "phase0",
	})
	account := mustCreateAccount(t, client, &service.Account{
		Name: "phase0-sub-account-" + uuid.NewString(),
		Type: service.AccountTypeAPIKey,
	})
	phase0CleanupStack(t, user.ID, group.ID, account.ID)
	return user, group, sub, apiKey, account
}

func phase0MustSubscriptionStack(
	t *testing.T,
	client *dbent.Client,
	balance float64,
	weeklyLimit *float64,
) (*service.User, *service.Group, *service.UserSubscription, *service.APIKey, *service.Account) {
	t.Helper()
	return phase0MustStack(t, client, service.SubscriptionTypeSubscription, balance, weeklyLimit)
}

func phase0QueryFloat(t *testing.T, query string, args ...any) float64 {
	t.Helper()
	var v float64
	require.NoError(t, integrationDB.QueryRow(query, args...).Scan(&v))
	return v
}

func phase0SetWeeklyWindow(t *testing.T, subscriptionID int64, anchor time.Time, usage float64) {
	t.Helper()
	_, err := integrationDB.Exec(
		"UPDATE user_subscriptions SET weekly_window_start = $1, weekly_usage_usd = $2 WHERE id = $3",
		anchor, usage, subscriptionID)
	require.NoError(t, err)
}

func phase0WeeklyState(t *testing.T, subscriptionID int64) (usage float64, anchor time.Time) {
	t.Helper()
	require.NoError(t, integrationDB.QueryRow(
		"SELECT weekly_usage_usd, weekly_window_start FROM user_subscriptions WHERE id = $1",
		subscriptionID).Scan(&usage, &anchor))
	return usage, anchor
}

// ---- §1 PAYG Billing Baseline ----

func TestPhase0PAYGSettlementDeductsBalanceOnly(t *testing.T) {
	ctx := context.Background()
	client := testEntClient(t)
	repo := NewUsageBillingRepository(client, integrationDB)

	user := mustCreateUser(t, client, &service.User{
		Email:        fmt.Sprintf("phase0-payg-%d@example.com", time.Now().UnixNano()),
		PasswordHash: "hash",
		Balance:      100,
	})
	group := mustCreateGroup(t, client, &service.Group{
		Name: "phase0-payg-group-" + uuid.NewString(),
	})
	apiKey := mustCreateApiKey(t, client, &service.APIKey{
		UserID:  user.ID,
		GroupID: &group.ID,
		Key:     "sk-phase0-payg-" + uuid.NewString(),
		Name:    "phase0",
	})
	account := mustCreateAccount(t, client, &service.Account{
		Name: "phase0-payg-account-" + uuid.NewString(),
		Type: service.AccountTypeAPIKey,
	})
	phase0CleanupStack(t, user.ID, group.ID, account.ID)

	cmd := &service.UsageBillingCommand{
		RequestID:        uuid.NewString(),
		APIKeyID:         apiKey.ID,
		UserID:           user.ID,
		AccountID:        account.ID,
		AccountType:      service.AccountTypeAPIKey,
		BalanceCost:      0.5,
		SubscriptionID:   nil,
		SubscriptionCost: 0,
	}

	result, err := repo.Apply(ctx, cmd)
	require.NoError(t, err)
	require.True(t, result.Applied)
	require.NotNil(t, result.NewBalance)
	require.InDelta(t, 99.5, *result.NewBalance, 1e-8)
	require.False(t, result.BalanceOverdrafted)

	balance := phase0QueryFloat(t, "SELECT balance FROM users WHERE id = $1", user.ID)
	require.InDelta(t, 99.5, balance, 1e-8, "PAYG settlement must deduct users.balance")

	var dedupCount int
	require.NoError(t, integrationDB.QueryRow(
		"SELECT COUNT(*) FROM usage_billing_dedup WHERE request_id = $1 AND api_key_id = $2",
		cmd.RequestID, apiKey.ID).Scan(&dedupCount))
	require.Equal(t, 1, dedupCount, "exactly one dedup claim per (request_id, api_key_id)")
}

// ---- §2 Subscription Billing Baseline ----

func TestPhase0SubscriptionSettlementAddsUsageWithoutTouchingBalance(t *testing.T) {
	ctx := context.Background()
	client := testEntClient(t)
	repo := NewUsageBillingRepository(client, integrationDB)

	weeklyLimit := 20.0
	user, group, sub, apiKey, account := phase0MustSubscriptionStack(t, client, 50, &weeklyLimit)
	_ = group

	anchor := time.Now().Add(-24 * time.Hour).Truncate(time.Microsecond)
	phase0SetWeeklyWindow(t, sub.ID, anchor, 1.0)

	subID := sub.ID
	cmd := &service.UsageBillingCommand{
		RequestID:        uuid.NewString(),
		APIKeyID:         apiKey.ID,
		UserID:           user.ID,
		AccountID:        account.ID,
		AccountType:      service.AccountTypeAPIKey,
		SubscriptionID:   &subID,
		SubscriptionCost: 1.25,
		BalanceCost:      0,
	}

	result, err := repo.Apply(ctx, cmd)
	require.NoError(t, err)
	require.True(t, result.Applied)
	require.Nil(t, result.NewBalance, "subscription settlement must not deduct balance")

	usage, gotAnchor := phase0WeeklyState(t, sub.ID)
	require.InDelta(t, 2.25, usage, 1e-8, "weekly_usage_usd += ActualCost")
	require.Equal(t, anchor.Format(time.RFC3339Nano), gotAnchor.Format(time.RFC3339Nano),
		"settlement must not move the weekly anchor")

	daily := phase0QueryFloat(t, "SELECT daily_usage_usd FROM user_subscriptions WHERE id = $1", sub.ID)
	monthly := phase0QueryFloat(t, "SELECT monthly_usage_usd FROM user_subscriptions WHERE id = $1", sub.ID)
	require.InDelta(t, 1.25, daily, 1e-8)
	require.InDelta(t, 1.25, monthly, 1e-8)

	balance := phase0QueryFloat(t, "SELECT balance FROM users WHERE id = $1", user.ID)
	require.InDelta(t, 50, balance, 1e-8, "subscription settlement must leave users.balance untouched")

	var dedupCount int
	require.NoError(t, integrationDB.QueryRow(
		"SELECT COUNT(*) FROM usage_billing_dedup WHERE request_id = $1 AND api_key_id = $2",
		cmd.RequestID, apiKey.ID).Scan(&dedupCount))
	require.Equal(t, 1, dedupCount)
}

// ---- §3 Billing Dedup（订阅侧） ----

func TestPhase0SubscriptionSettlementDeduplicates(t *testing.T) {
	ctx := context.Background()
	client := testEntClient(t)
	repo := NewUsageBillingRepository(client, integrationDB)

	weeklyLimit := 100.0
	user, _, sub, apiKey, _ := phase0MustSubscriptionStack(t, client, 50, &weeklyLimit)
	anchor := time.Now().Add(-time.Hour).Truncate(time.Microsecond)
	phase0SetWeeklyWindow(t, sub.ID, anchor, 0)

	subID := sub.ID
	accountID := mustCreateAccount(t, client, &service.Account{Name: "phase0-dedup-" + uuid.NewString(), Type: service.AccountTypeAPIKey}).ID
	cmd := &service.UsageBillingCommand{
		RequestID:        uuid.NewString(),
		APIKeyID:         apiKey.ID,
		UserID:           user.ID,
		AccountID:        accountID,
		AccountType:      service.AccountTypeAPIKey,
		SubscriptionID:   &subID,
		SubscriptionCost: 2.0,
	}

	r1, err := repo.Apply(ctx, cmd)
	require.NoError(t, err)
	require.True(t, r1.Applied)

	r2, err := repo.Apply(ctx, cmd)
	require.NoError(t, err)
	require.False(t, r2.Applied, "second settlement with identical fingerprint must be idempotently skipped")

	usage, _ := phase0WeeklyState(t, sub.ID)
	require.InDelta(t, 2.0, usage, 1e-8, "usage incremented exactly once")

	balance := phase0QueryFloat(t, "SELECT balance FROM users WHERE id = $1", user.ID)
	require.InDelta(t, 50, balance, 1e-8)
}

// ---- §7 ResetWeeklyUsage CAS ----

func TestPhase0ResetWeeklyUsageCAS(t *testing.T) {
	client := testEntClient(t)
	repo := NewUserSubscriptionRepository(client)

	t.Run("CaseA expected anchor matches → reset succeeds", func(t *testing.T) {
		_, _, sub, _, _ := phase0MustSubscriptionStack(t, client, 0, nil)
		oldAnchor := time.Now().Add(-48 * time.Hour).Truncate(time.Microsecond)
		phase0SetWeeklyWindow(t, sub.ID, oldAnchor, 12.34)

		newAnchor := time.Now().Truncate(time.Microsecond)
		require.NoError(t, repo.ResetWeeklyUsage(context.Background(), sub.ID, &oldAnchor, newAnchor))

		usage, anchor := phase0WeeklyState(t, sub.ID)
		require.InDelta(t, 0, usage, 1e-8, "CAS reset must zero weekly usage")
		require.Equal(t, newAnchor.Format(time.RFC3339Nano), anchor.Format(time.RFC3339Nano))
	})

	t.Run("CaseB expected anchor stale → CAS no-op, window untouched", func(t *testing.T) {
		_, _, sub, _, _ := phase0MustSubscriptionStack(t, client, 0, nil)
		realAnchor := time.Now().Add(-24 * time.Hour).Truncate(time.Microsecond)
		phase0SetWeeklyWindow(t, sub.ID, realAnchor, 7.5)

		staleExpected := time.Now().Add(-72 * time.Hour).Truncate(time.Microsecond)
		newAnchor := time.Now().Truncate(time.Microsecond)
		// stale reset 是预期内的幂等 no-op（不报错）
		require.NoError(t, repo.ResetWeeklyUsage(context.Background(), sub.ID, &staleExpected, newAnchor))

		usage, anchor := phase0WeeklyState(t, sub.ID)
		require.InDelta(t, 7.5, usage, 1e-8, "stale CAS must not clobber current period usage")
		require.Equal(t, realAnchor.Format(time.RFC3339Nano), anchor.Format(time.RFC3339Nano),
			"stale CAS must not move the anchor")
	})
}

// ---- §8 Reset ↔ Settlement 顺序与并发 ----

func TestPhase0ResetVsSettlementSequences(t *testing.T) {
	client := testEntClient(t)
	subRepo := NewUserSubscriptionRepository(client)
	billingRepo := NewUsageBillingRepository(client, integrationDB)

	newStack := func(t *testing.T) (*service.UserSubscription, *service.APIKey, *service.User) {
		user, _, sub, apiKey, _ := phase0MustSubscriptionStack(t, client, 0, nil)
		return sub, apiKey, user
	}

	t.Run("SequenceA settlement then reset → usage=0, anchor=new", func(t *testing.T) {
		sub, apiKey, user := newStack(t)
		accountID := mustCreateAccount(t, client, &service.Account{Name: "phase0-seqa-" + uuid.NewString(), Type: service.AccountTypeAPIKey}).ID
		oldAnchor := time.Now().Add(-24 * time.Hour).Truncate(time.Microsecond)
		phase0SetWeeklyWindow(t, sub.ID, oldAnchor, 10)
		subID := sub.ID

		_, err := billingRepo.Apply(context.Background(), &service.UsageBillingCommand{
			RequestID:        uuid.NewString(),
			APIKeyID:         apiKey.ID,
			UserID:           user.ID,
			AccountID:        accountID,
			AccountType:      service.AccountTypeAPIKey,
			SubscriptionID:   &subID,
			SubscriptionCost: 1.5,
		})
		require.NoError(t, err)

		newAnchor := time.Now().Truncate(time.Microsecond)
		require.NoError(t, subRepo.ResetWeeklyUsage(context.Background(), sub.ID, &oldAnchor, newAnchor))

		usage, anchor := phase0WeeklyState(t, sub.ID)
		require.InDelta(t, 0, usage, 1e-8)
		require.Equal(t, newAnchor.Format(time.RFC3339Nano), anchor.Format(time.RFC3339Nano))
	})

	t.Run("SequenceB reset then settlement → usage=cost, anchor=new", func(t *testing.T) {
		sub, apiKey, user := newStack(t)
		accountID := mustCreateAccount(t, client, &service.Account{Name: "phase0-seqb-" + uuid.NewString(), Type: service.AccountTypeAPIKey}).ID
		oldAnchor := time.Now().Add(-24 * time.Hour).Truncate(time.Microsecond)
		phase0SetWeeklyWindow(t, sub.ID, oldAnchor, 10)
		subID := sub.ID

		newAnchor := time.Now().Truncate(time.Microsecond)
		require.NoError(t, subRepo.ResetWeeklyUsage(context.Background(), sub.ID, &oldAnchor, newAnchor))

		_, err := billingRepo.Apply(context.Background(), &service.UsageBillingCommand{
			RequestID:        uuid.NewString(),
			APIKeyID:         apiKey.ID,
			UserID:           user.ID,
			AccountID:        accountID,
			AccountType:      service.AccountTypeAPIKey,
			SubscriptionID:   &subID,
			SubscriptionCost: 1.5,
		})
		require.NoError(t, err)

		usage, anchor := phase0WeeklyState(t, sub.ID)
		require.InDelta(t, 1.5, usage, 1e-8, "settlement after reset lands in the new period")
		require.Equal(t, newAnchor.Format(time.RFC3339Nano), anchor.Format(time.RFC3339Nano))
	})
}

// TestPhase0ResetVsSettlementConcurrentInvariant 并发锤击：证明不存在
// "Reset 已推进锚点，旧周期用量又被累加回新周期" 的 lost update。
//
// 不变量：初始 (anchor=T0, usage=10)，并发执行 IncrementUsage(1.5) 与
// ResetWeeklyUsage(T0→T1) 各一次，最终只允许三种结局：
//
//	(usage=11.5, anchor=T0)  reset 的 CAS 输了（合法：另一请求已推进）
//	(usage=0,    anchor=T1)  settlement 先提交
//	(usage=1.5,  anchor=T1)  reset 先提交
//
// 被禁止：usage=11.5 且 anchor=T1（旧周期用量被叠进新周期）。
func TestPhase0ResetVsSettlementConcurrentInvariant(t *testing.T) {
	client := testEntClient(t)
	subRepo := NewUserSubscriptionRepository(client)
	ctx := context.Background()

	rounds := 20
	for i := 0; i < rounds; i++ {
		_, _, sub, _, _ := phase0MustSubscriptionStack(t, client, 0, nil)
		oldAnchor := time.Now().Add(-24 * time.Hour).Truncate(time.Microsecond)
		newAnchor := oldAnchor.Add(time.Hour)
		phase0SetWeeklyWindow(t, sub.ID, oldAnchor, 10)

		var wg sync.WaitGroup
		wg.Add(2)
		go func() {
			defer wg.Done()
			_ = subRepo.IncrementUsage(ctx, sub.ID, 1.5)
		}()
		go func() {
			defer wg.Done()
			_ = subRepo.ResetWeeklyUsage(ctx, sub.ID, &oldAnchor, newAnchor)
		}()
		wg.Wait()

		usage, anchor := phase0WeeklyState(t, sub.ID)
		resetWon := anchor.Equal(newAnchor)
		switch {
		case resetWon:
			require.Contains(t, []float64{0, 1.5}, usage,
				"round %d: with new anchor, usage must be 0 or cost, got %f (lost update)", i, usage)
		default:
			require.True(t, anchor.Equal(oldAnchor), "round %d: unexpected anchor", i)
			require.InDelta(t, 11.5, usage, 1e-8,
				"round %d: reset lost → usage = pre + cost on the old period", i)
		}
	}
}

// ---- §10 Weekly Limit Overshoot ----

func TestPhase0WeeklyLimitOvershootThenRejection(t *testing.T) {
	ctx := context.Background()
	client := testEntClient(t)
	subRepo := NewUserSubscriptionRepository(client)

	weeklyLimit := 20.0
	_, group, sub, _, _ := phase0MustSubscriptionStack(t, client, 0, &weeklyLimit)
	anchor := time.Now().Add(-24 * time.Hour).Truncate(time.Microsecond)
	phase0SetWeeklyWindow(t, sub.ID, anchor, 19.8)

	// 请求开始时 eligibility 通过（19.8 < 20）；settlement 无条件累加 1.5。
	require.NoError(t, subRepo.IncrementUsage(ctx, sub.ID, 1.5))
	usage, _ := phase0WeeklyState(t, sub.ID)
	require.InDelta(t, 21.3, usage, 1e-8, "settlement overshoot past the limit must be recorded")

	// 下一请求 eligibility 失败。
	svcSub, err := subRepo.GetByID(ctx, sub.ID)
	require.NoError(t, err)
	require.False(t, svcSub.CheckWeeklyLimit(group, 0), "next request must be rejected after overshoot")
}

// ---- §11 Concurrent Weekly Overshoot ----

func TestPhase0ConcurrentWeeklyOvershootMagnitude(t *testing.T) {
	ctx := context.Background()
	client := testEntClient(t)
	subRepo := NewUserSubscriptionRepository(client)

	weeklyLimit := 20.0
	_, group, sub, _, _ := phase0MustSubscriptionStack(t, client, 0, &weeklyLimit)
	anchor := time.Now().Add(-24 * time.Hour).Truncate(time.Microsecond)
	phase0SetWeeklyWindow(t, sub.ID, anchor, 19.8)

	const concurrency = 8
	const perRequestCost = 1.5

	var wg sync.WaitGroup
	wg.Add(concurrency)
	for i := 0; i < concurrency; i++ {
		go func() {
			defer wg.Done()
			_ = subRepo.IncrementUsage(ctx, sub.ID, perRequestCost)
		}()
	}
	wg.Wait()

	usage, _ := phase0WeeklyState(t, sub.ID)
	expected := 19.8 + concurrency*perRequestCost
	require.InDelta(t, expected, usage, 1e-6,
		"all concurrently-admitted settlements must land (baseline risk: limit + concurrency × cost)")

	// 用量记录完整性：并发后全部被拒绝
	svcSub, err := subRepo.GetByID(ctx, sub.ID)
	require.NoError(t, err)
	require.False(t, svcSub.CheckWeeklyLimit(group, 0))
}

// ---- §14 用户并发槽 + §15 多 Key 共用同一用户池 ----

func TestPhase0UserConcurrencySlotBaseline(t *testing.T) {
	cache := NewConcurrencyCache(testRedis(t), 10, 60)
	ctx := context.Background()
	userID := time.Now().UnixNano()

	// limit=2：acquire, acquire, reject, release, acquire
	ok1, err := cache.AcquireUserSlot(ctx, userID, 2, "req-1")
	require.NoError(t, err)
	require.True(t, ok1)

	ok2, err := cache.AcquireUserSlot(ctx, userID, 2, "req-2")
	require.NoError(t, err)
	require.True(t, ok2)

	ok3, err := cache.AcquireUserSlot(ctx, userID, 2, "req-3")
	require.NoError(t, err)
	require.False(t, ok3, "third concurrent request must be rejected at limit=2")

	require.NoError(t, cache.ReleaseUserSlot(ctx, userID, "req-1"))
	ok4, err := cache.AcquireUserSlot(ctx, userID, 2, "req-4")
	require.NoError(t, err)
	require.True(t, ok4, "released slot must be acquirable")

	count, err := cache.GetUserConcurrency(ctx, userID)
	require.NoError(t, err)
	require.Equal(t, 2, count)
}

func TestPhase0UserConcurrencyPoolSharedAcrossApiKeys(t *testing.T) {
	// §15 证据：并发池按 userID 单一 ZSET，与 API Key / Group 无关。
	// 同一用户两把 Key（不同分组）各 acquire 一次 → 同池计数为 2。
	cache := NewConcurrencyCache(testRedis(t), 10, 60)
	ctx := context.Background()
	userID := time.Now().UnixNano()

	okA, err := cache.AcquireUserSlot(ctx, userID, 2, "key-a-req-1")
	require.NoError(t, err)
	require.True(t, okA)

	okB, err := cache.AcquireUserSlot(ctx, userID, 2, "key-b-req-1")
	require.NoError(t, err)
	require.True(t, okB)

	okC, err := cache.AcquireUserSlot(ctx, userID, 2, "key-a-req-2")
	require.NoError(t, err)
	require.False(t, okC, "same-user different-key requests share one user ZSET pool")

	count, err := cache.GetUserConcurrency(ctx, userID)
	require.NoError(t, err)
	require.Equal(t, 2, count)
}

// ---- §21 AdminResetQuota re-anchor 基线 ----

func TestPhase0AdminResetQuotaReAnchorsWeeklyPeriod(t *testing.T) {
	ctx := context.Background()
	client := testEntClient(t)
	subRepo := NewUserSubscriptionRepository(client)
	// billingCacheService=nil 时 InvalidateSubCacheSync/InvalidateSubscription 均为 no-op，
	// 与生产路径的缓存语义差异由 Phase 0 报告单独记录。
	subSvc := service.NewSubscriptionService(nil, subRepo, nil, client, nil)

	startsAt := time.Now().Add(-10 * 24 * time.Hour).Truncate(time.Microsecond)
	expiresAt := time.Now().Add(20 * 24 * time.Hour).Truncate(time.Microsecond)
	_, _, sub, _, _ := phase0MustSubscriptionStack(t, client, 0, nil)
	// 固化生命周期字段（fixture 默认值不可控）
	_, err := integrationDB.Exec(
		"UPDATE user_subscriptions SET starts_at = $1, expires_at = $2 WHERE id = $3",
		startsAt, expiresAt, sub.ID)
	require.NoError(t, err)

	anchor := time.Now().Add(-5 * 24 * time.Hour).Truncate(time.Microsecond)
	phase0SetWeeklyWindow(t, sub.ID, anchor, 13.5)

	before, err := subRepo.GetByID(ctx, sub.ID)
	require.NoError(t, err)

	resetAt := time.Now()
	// AdminResetQuota 内部使用 time.Now() 作为新锚点（service.now 未导出，
	// 集成测试以 2 秒容差断言 re-anchor 到 reset 时刻）。
	updated, err := subSvc.AdminResetQuota(ctx, sub.ID, false, true, false)
	require.NoError(t, err)

	usage, newAnchor := phase0WeeklyState(t, sub.ID)
	require.InDelta(t, 0, usage, 1e-8, "AdminResetQuota must zero weekly usage")
	require.False(t, newAnchor.Equal(anchor), "AdminResetQuota must re-anchor the weekly window")
	require.WithinDuration(t, resetAt, newAnchor, 2*time.Second,
		"new anchor = reset time (re-anchoring semantics)")

	// 下一次自然重置 = 新锚点 + 7 天
	require.Equal(t, newAnchor.Add(7*24*time.Hour).Format(time.RFC3339Nano),
		updated.WeeklyResetTime().Format(time.RFC3339Nano))

	// 生命周期字段不可被 Reset 改动
	require.Equal(t, before.StartsAt.Format(time.RFC3339Nano), updated.StartsAt.Format(time.RFC3339Nano),
		"reset must not touch started_at")
	require.Equal(t, before.ExpiresAt.Format(time.RFC3339Nano), updated.ExpiresAt.Format(time.RFC3339Nano),
		"reset must not touch expires_at")
	require.Equal(t, service.SubscriptionStatusActive, updated.Status)

	// daily / monthly 未被请求重置
	daily := phase0QueryFloat(t, "SELECT daily_usage_usd FROM user_subscriptions WHERE id = $1", sub.ID)
	monthly := phase0QueryFloat(t, "SELECT monthly_usage_usd FROM user_subscriptions WHERE id = $1", sub.ID)
	require.InDelta(t, before.DailyUsageUSD, daily, 1e-8)
	require.InDelta(t, before.MonthlyUsageUSD, monthly, 1e-8)
}

// ---- §16 多订阅并存 ----

func TestPhase0MultipleSubscriptionsIndependentWindows(t *testing.T) {
	ctx := context.Background()
	client := testEntClient(t)
	subRepo := NewUserSubscriptionRepository(client)

	user := mustCreateUser(t, client, &service.User{
		Email:        fmt.Sprintf("phase0-multi-%d@example.com", time.Now().UnixNano()),
		PasswordHash: "hash",
	})
	groupPro := mustCreateGroup(t, client, &service.Group{
		Name:             "phase0-pro-" + uuid.NewString(),
		SubscriptionType: service.SubscriptionTypeSubscription,
	})
	groupMax := mustCreateGroup(t, client, &service.Group{
		Name:             "phase0-max-" + uuid.NewString(),
		SubscriptionType: service.SubscriptionTypeSubscription,
	})
	phase0CleanupStack(t, user.ID, groupPro.ID, 0)
	t.Cleanup(func() {
		_, _ = integrationDB.Exec("DELETE FROM groups WHERE id = $1", groupMax.ID)
	})

	anchorPro := time.Now().Add(-72 * time.Hour).Truncate(time.Microsecond)
	anchorMax := time.Now().Add(-2 * time.Hour).Truncate(time.Microsecond)
	subPro := mustCreateSubscription(t, client, &service.UserSubscription{UserID: user.ID, GroupID: groupPro.ID})
	subMax := mustCreateSubscription(t, client, &service.UserSubscription{UserID: user.ID, GroupID: groupMax.ID})
	phase0SetWeeklyWindow(t, subPro.ID, anchorPro, 5)
	phase0SetWeeklyWindow(t, subMax.ID, anchorMax, 1)

	// 两订阅并存且 anchor / usage 完全独立
	active, err := subRepo.ListActiveByUserID(ctx, user.ID)
	require.NoError(t, err)
	require.Len(t, active, 2, "Pro and Max subscriptions coexist in parallel")

	byPro, err := subRepo.GetActiveByUserIDAndGroupID(ctx, user.ID, groupPro.ID)
	require.NoError(t, err)
	require.InDelta(t, 5, byPro.WeeklyUsageUSD, 1e-8)
	require.Equal(t, anchorPro.Format(time.RFC3339Nano), byPro.WeeklyWindowStart.Format(time.RFC3339Nano))

	byMax, err := subRepo.GetActiveByUserIDAndGroupID(ctx, user.ID, groupMax.ID)
	require.NoError(t, err)
	require.InDelta(t, 1, byMax.WeeklyUsageUSD, 1e-8)
	require.Equal(t, anchorMax.Format(time.RFC3339Nano), byMax.WeeklyWindowStart.Format(time.RFC3339Nano))

	// 增量只落在对应分组的订阅上
	require.NoError(t, subRepo.IncrementUsage(ctx, subPro.ID, 2))
	byPro, _ = subRepo.GetActiveByUserIDAndGroupID(ctx, user.ID, groupPro.ID)
	byMax, _ = subRepo.GetActiveByUserIDAndGroupID(ctx, user.ID, groupMax.ID)
	require.InDelta(t, 7, byPro.WeeklyUsageUSD, 1e-8)
	require.InDelta(t, 1, byMax.WeeklyUsageUSD, 1e-8)
}
