//go:build integration

package repository_test

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/Wei-Shaw/sub2api/internal/domain"
	"github.com/Wei-Shaw/sub2api/internal/handler"
	"github.com/Wei-Shaw/sub2api/internal/repository"
	"github.com/Wei-Shaw/sub2api/internal/server/middleware"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

func TestRCResetExhaustedRouteAndDurableRecovery(t *testing.T) {
	ctx := context.Background()
	client, integrationDB, user, group, sub, cards, subs := repository.RCResetFixture(t)
	_, err := integrationDB.ExecContext(ctx, "UPDATE users SET balance=0 WHERE id=$1", user.ID)
	require.NoError(t, err)
	_, err = integrationDB.ExecContext(ctx, "UPDATE user_subscriptions SET auto_payg_fallback=false WHERE id=$1", sub.ID)
	require.NoError(t, err)
	t.Cleanup(subs.Stop)
	_, err = cards.GrantResetCards(ctx, &service.GrantResetCardsInput{Selector: service.ResetCardGrantSelector{Mode: domain.ResetTargetModeUsers, UserIDs: []int64{user.ID}}, QuantityPerUser: 1, IdempotencyKey: fmt.Sprintf("rc-grant-%d", user.ID)})
	require.NoError(t, err)
	secret := fmt.Sprintf("rc-test-key-%d", user.ID)
	key, err := client.APIKey.Create().SetUserID(user.ID).SetGroupID(group.ID).SetKey(secret).SetName("RC regression").SetStatus(service.StatusActive).Save(ctx)
	require.NoError(t, err)
	t.Cleanup(func() { _, _ = integrationDB.Exec("DELETE FROM api_keys WHERE id=$1", key.ID) })
	cfg := &config.Config{RunMode: config.RunModeStandard}
	keys := service.NewAPIKeyService(repository.NewAPIKeyRepository(client, integrationDB), nil, nil, nil, nil, nil, cfg)
	h := handler.NewSubscriptionHandler(subs, nil, cards)
	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.Use(gin.HandlerFunc(middleware.NewAPIKeyAuthMiddleware(keys, subs, cfg)))
	router.POST("/v1/muc/reset-with-card/:id", h.ResetWithCard)
	router.POST("/v1/muc/reset-with-card/:id/reconcile", h.ReconcileResetCard)
	router.POST("/v1/messages", func(c *gin.Context) { t.Error("exhausted model request bypassed billing"); c.Status(204) })
	post := func(path, key string) *httptest.ResponseRecorder {
		req := httptest.NewRequest(http.MethodPost, path, nil)
		req.Header.Set("Authorization", "Bearer "+secret)
		req.Header.Set("Idempotency-Key", key)
		w := httptest.NewRecorder()
		router.ServeHTTP(w, req)
		return w
	}
	require.Equal(t, 429, post("/v1/messages", "").Code)
	operation := "reset-v2-rc-integration"
	path := fmt.Sprintf("/v1/muc/reset-with-card/%d", sub.ID)
	for i := 0; i < 2; i++ {
		w := post(path, operation)
		require.Equal(t, 200, w.Code, w.Body.String())
	}
	var usage float64
	require.NoError(t, integrationDB.QueryRowContext(ctx, "SELECT weekly_usage_usd FROM user_subscriptions WHERE id=$1", sub.ID).Scan(&usage))
	require.Zero(t, usage)
	count, err := cards.CountAvailableResetCards(ctx, user.ID)
	require.NoError(t, err)
	require.Zero(t, count)
	// Remove the TTL cache to prove durable receipts, not the coordinator, recover success.
	_, err = integrationDB.ExecContext(ctx, "DELETE FROM idempotency_records WHERE scope=$1", "subscription.reset_card.consume")
	require.NoError(t, err)
	recovered, err := cards.ReconcileResetOperation(ctx, user.ID, sub.ID, operation)
	require.NoError(t, err)
	require.Equal(t, "succeeded", recovered.Status)
	require.NotNil(t, recovered.Result)
	replay, err := cards.ConsumeForSubscription(ctx, user.ID, sub.ID, operation)
	require.NoError(t, err)
	require.Equal(t, recovered.Result.CardID, replay.CardID)
	cancelled, err := cards.ReconcileResetOperation(ctx, user.ID, sub.ID, "reset-v2-never-sent")
	require.NoError(t, err)
	require.Equal(t, "cancelled", cancelled.Status)
	_, err = cards.ConsumeForSubscription(ctx, user.ID, sub.ID, "reset-v2-never-sent")
	require.Error(t, err)
	unknown, err := cards.ReconcileResetOperation(ctx, user.ID, sub.ID, "legacy-unknown")
	require.NoError(t, err)
	require.Equal(t, "unknown", unknown.Status)
	prepared, err := cards.PrepareResetOperation(ctx, user.ID, sub.ID, "reset-v2-prepared")
	require.NoError(t, err)
	require.Equal(t, "pending", prepared.Status)
	cancelled, err = cards.ReconcileResetOperation(ctx, user.ID, sub.ID, "reset-v2-prepared")
	require.NoError(t, err)
	require.Equal(t, "cancelled", cancelled.Status)
	// A reconciliation request never changes usage or spends a card.
	require.WithinDuration(t, time.Now().Add(7*24*time.Hour), *recovered.Result.WeeklyPeriodEndsAt, time.Minute)
}

func TestRCResetReconcileSerializesWithConsumption(t *testing.T) {
	ctx := context.Background()
	_, _, user, _, sub, cards, subs := repository.RCResetFixture(t)
	t.Cleanup(subs.Stop)
	_, err := cards.GrantResetCards(ctx, &service.GrantResetCardsInput{Selector: service.ResetCardGrantSelector{Mode: domain.ResetTargetModeUsers, UserIDs: []int64{user.ID}}, QuantityPerUser: 1, IdempotencyKey: fmt.Sprintf("rc-race-grant-%d", user.ID)})
	require.NoError(t, err)
	key := fmt.Sprintf("reset-v2-race-%d", user.ID)
	start := make(chan struct{})
	var workers sync.WaitGroup
	for i := 0; i < 12; i++ {
		workers.Add(1)
		go func(index int) {
			defer workers.Done()
			<-start
			if index%2 == 0 {
				_, _ = cards.ConsumeForSubscription(ctx, user.ID, sub.ID, key)
			} else {
				_, _ = cards.ReconcileResetOperation(ctx, user.ID, sub.ID, key)
			}
		}(i)
	}
	close(start)
	workers.Wait()
	result, err := cards.ReconcileResetOperation(ctx, user.ID, sub.ID, key)
	require.NoError(t, err)
	count, err := cards.CountAvailableResetCards(ctx, user.ID)
	require.NoError(t, err)
	switch result.Status {
	case "succeeded":
		require.Zero(t, count)
		require.NotNil(t, result.Result)
	case "cancelled":
		require.Equal(t, 1, count)
	default:
		t.Fatalf("unresolved operation: %s", result.Status)
	}
}
