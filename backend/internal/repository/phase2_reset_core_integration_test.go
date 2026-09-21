//go:build integration

package repository

// Phase 2 —— 统一 Reset Core 集成测试（真实 PostgreSQL / Redis）。
//
// 覆盖：§11 事件应用崩溃窗口等价性（单事务原子性 → retry 幂等）、
// §12/§35 Reset↔Settlement 并发不变量、§14/§16 缓存两层失效（Redis L2 + DB 回读）、
// §39 AdminResetQuota 回归（详见 Phase 0 套件的 re-anchor 测试，经新 Core 路径仍 PASS）。

import (
	"context"
	"sync"
	"testing"
	"time"

	dbent "github.com/Wei-Shaw/sub2api/ent"
	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/Wei-Shaw/sub2api/internal/domain"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/stretchr/testify/require"
)

func phase2NewCoreService(t *testing.T, client *dbent.Client, billingCache *service.BillingCacheService) (*service.SubscriptionService, *userSubscriptionRepository) {
	t.Helper()
	subRepo := NewUserSubscriptionRepository(client).(*userSubscriptionRepository)
	svc := service.NewSubscriptionService(nil, subRepo, billingCache, client, nil)
	svc.SetResetApplicationRepository(NewSubscriptionResetApplicationRepository(client))
	return svc, subRepo
}

// ---- §34 事件 retry（真实 DB：单事务原子性） ----

func TestPhase2ResetCoreEventRetryIntegration(t *testing.T) {
	ctx := context.Background()
	client := testEntClient(t)
	svc, _ := phase2NewCoreService(t, client, nil)

	weeklyLimit := 100.0
	user, group, sub, _, _ := phase0MustSubscriptionStack(t, client, 0, &weeklyLimit)
	anchor := time.Now().Add(-72 * time.Hour).Truncate(time.Microsecond)
	phase0SetWeeklyWindow(t, sub.ID, anchor, 6.5)

	ev, err := client.SubscriptionResetEvent.Create().
		SetEventType(domain.ResetEventTypeGlobalReset).
		SetStatus(domain.ResetEventStatusPending).
		SetScopeType(domain.ResetEventScopeTypeSubscription).
		SetScope(map[string]any{"subscription_ids": []int64{sub.ID}}).
		SetEffectiveAt(time.Now().Add(-time.Hour).Truncate(time.Microsecond)).
		SetReason("phase2 retry test").
		SetMetadata(map[string]any{}).
		Save(ctx)
	require.NoError(t, err)
	phase1CleanupEvents(t, ev.ID)
	phase0CleanupStack(t, user.ID, group.ID, 0)

	eventID := ev.ID
	statuses := make([]service.WeeklyResetStatus, 0, 10)
	for i := 0; i < 10; i++ {
		res, err := svc.ResetSubscriptionWeeklyPeriod(ctx, &service.WeeklyResetInput{
			UserSubscriptionID: sub.ID,
			EffectiveAt:        ev.EffectiveAt,
			Source:             domain.WeeklyResetSourceGlobalReset,
			ResetEventID:       &eventID,
		})
		require.NoError(t, err)
		statuses = append(statuses, res.Status)
	}

	require.Equal(t, service.WeeklyResetApplied, statuses[0])
	for i := 1; i < 10; i++ {
		require.Equal(t, service.WeeklyResetAlreadyApplied, statuses[i],
			"retry round %d must be ALREADY_APPLIED", i)
	}

	// application 恰好一行（UNIQUE 约束 + 单事务原子性）
	var appCount int
	require.NoError(t, integrationDB.QueryRowContext(ctx,
		"SELECT COUNT(*) FROM subscription_reset_applications WHERE reset_event_id = $1 AND user_subscription_id = $2",
		ev.ID, sub.ID).Scan(&appCount))
	require.Equal(t, 1, appCount)

	usage, gotAnchor := phase0WeeklyState(t, sub.ID)
	require.InDelta(t, 0, usage, 1e-8, "reset executed exactly once")
	require.Equal(t, ev.EffectiveAt.Format(time.RFC3339Nano), gotAnchor.Format(time.RFC3339Nano))

	// 审计列：previous 值已回填
	var prevStart *time.Time
	var prevUsage *float64
	var appStatus string
	require.NoError(t, integrationDB.QueryRowContext(ctx,
		"SELECT previous_weekly_window_start, previous_weekly_usage_usd, status FROM subscription_reset_applications WHERE reset_event_id = $1 AND user_subscription_id = $2",
		ev.ID, sub.ID).Scan(&prevStart, &prevUsage, &appStatus))
	require.Equal(t, domain.ResetApplicationStatusApplied, appStatus)
	require.NotNil(t, prevStart)
	require.Equal(t, anchor.Format(time.RFC3339Nano), prevStart.Format(time.RFC3339Nano))
	require.NotNil(t, prevUsage)
	require.InDelta(t, 6.5, *prevUsage, 1e-8)
}

// ---- §12/§35 Reset ↔ Settlement 并发（service 层，行锁语义） ----

func TestPhase2ResetCoreVsSettlementConcurrentInvariant(t *testing.T) {
	ctx := context.Background()
	client := testEntClient(t)
	svc, _ := phase2NewCoreService(t, client, nil)

	rounds := 10
	for i := 0; i < rounds; i++ {
		_, _, sub, _, _ := phase0MustSubscriptionStack(t, client, 0, nil)
		oldAnchor := time.Now().Add(-24 * time.Hour).Truncate(time.Microsecond)
		effective := oldAnchor.Add(time.Hour)
		phase0SetWeeklyWindow(t, sub.ID, oldAnchor, 10)

		var wg sync.WaitGroup
		wg.Add(2)
		go func() {
			defer wg.Done()
			_ = NewUserSubscriptionRepository(client).IncrementUsage(ctx, sub.ID, 1.5)
		}()
		go func() {
			defer wg.Done()
			_, _ = svc.ResetSubscriptionWeeklyPeriod(ctx, &service.WeeklyResetInput{
				UserSubscriptionID: sub.ID,
				EffectiveAt:        effective,
				Source:             domain.WeeklyResetSourceGlobalReset,
			})
		}()
		wg.Wait()

		usage, anchor := phase0WeeklyState(t, sub.ID)
		require.Equal(t, effective.Format(time.RFC3339Nano), anchor.Format(time.RFC3339Nano),
			"round %d: reset must always move anchor to effective_at (row lock serializes)", i)
		require.Contains(t, []float64{0, 1.5}, usage,
			"round %d: usage must be 0 (settle→reset) or cost (reset→settle), got %f", i, usage)
	}
}

// ---- §14/§16 缓存失效（Redis L2 清除 + DB 回读新状态） ----

func TestPhase2ResetCoreInvalidatesSubscriptionCache(t *testing.T) {
	ctx := context.Background()
	client := testEntClient(t)

	weeklyLimit := 100.0
	user, group, sub, _, _ := phase0MustSubscriptionStack(t, client, 0, &weeklyLimit)
	anchor := time.Now().Add(-24 * time.Hour).Truncate(time.Microsecond)
	phase0SetWeeklyWindow(t, sub.ID, anchor, 4.2)

	rdb := testRedis(t)
	l2 := NewBillingCache(rdb)
	bcs := service.NewBillingCacheService(
		l2, nil, NewUserSubscriptionRepository(client),
		nil, nil, nil, &config.Config{}, nil)
	svc, _ := phase2NewCoreService(t, client, bcs)

	// 预置一条"旧状态" L2 缓存（模拟结算后写入的读数：usage=4.2）
	require.NoError(t, l2.SetSubscriptionCache(ctx, user.ID, group.ID, &service.SubscriptionCacheData{
		Status:      service.SubscriptionStatusActive,
		ExpiresAt:   time.Now().Add(24 * time.Hour),
		WeeklyUsage: 4.2,
	}))

	// Reset：effective = 现在-1h（合法过去时刻），usage 应清零、anchor 前移
	effective := time.Now().Add(-time.Hour).Truncate(time.Microsecond)
	res, err := svc.ResetSubscriptionWeeklyPeriod(ctx, &service.WeeklyResetInput{
		UserSubscriptionID: sub.ID,
		EffectiveAt:        effective,
		Source:             domain.WeeklyResetSourceAdminManual,
	})
	require.NoError(t, err)
	require.Equal(t, service.WeeklyResetApplied, res.Status)

	// L2 缓存已被失效：读不到旧值
	_, err = l2.GetSubscriptionCache(ctx, user.ID, group.ID)
	require.Error(t, err, "stale subscription cache must be invalidated by reset core")

	// 服务层下一次读取（缓存 miss → DB）：拿到新 anchor / usage
	fresh, err := bcs.GetSubscriptionStatus(ctx, user.ID, group.ID)
	require.NoError(t, err)
	require.InDelta(t, 0, fresh.WeeklyUsage, 1e-8, "next service read must see zeroed usage")

	usage, gotAnchor := phase0WeeklyState(t, sub.ID)
	require.InDelta(t, 0, usage, 1e-8)
	require.Equal(t, effective.Format(time.RFC3339Nano), gotAnchor.Format(time.RFC3339Nano))
}
