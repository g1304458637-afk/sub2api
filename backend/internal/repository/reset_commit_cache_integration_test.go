//go:build integration

package repository

import (
	"context"
	"sync/atomic"
	"testing"
	"time"

	dbent "github.com/Wei-Shaw/sub2api/ent"
	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/Wei-Shaw/sub2api/internal/domain"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/stretchr/testify/require"
)

type resetCommitCache struct {
	service.BillingCache
	invalidations atomic.Int64
}

func (c *resetCommitCache) InvalidateSubscriptionCache(context.Context, int64, int64) error {
	c.invalidations.Add(1)
	return nil
}
func TestConsolidationResetCacheInvalidatesAfterOuterCommit(t *testing.T) {
	ctx := context.Background()
	client := testEntClient(t)
	limit := 10.0
	user, group, sub := phase4MustStack(t, client, &limit, 8, -24*time.Hour)
	phase0CleanupStack(t, user.ID, group.ID, 0)
	cache := &resetCommitCache{}
	billing := service.NewBillingCacheService(cache, nil, nil, nil, nil, nil, &config.Config{}, nil)
	t.Cleanup(billing.Stop)
	core := service.NewSubscriptionService(nil, NewUserSubscriptionRepository(client), billing, client, nil)
	t.Cleanup(core.Stop)
	for _, commit := range []bool{false, true} {
		tx, err := client.Tx(ctx)
		require.NoError(t, err)
		_, err = core.ResetSubscriptionWeeklyPeriod(dbent.NewTxContext(ctx, tx), &service.WeeklyResetInput{UserSubscriptionID: sub.ID, EffectiveAt: time.Now(), Source: domain.WeeklyResetSourceResetCard})
		require.NoError(t, err)
		require.Zero(t, cache.invalidations.Load(), "uncommitted reset must not invalidate a live cache")
		if commit {
			require.NoError(t, tx.Commit())
		} else {
			require.NoError(t, tx.Rollback())
		}
	}
	require.Equal(t, int64(1), cache.invalidations.Load())
}
