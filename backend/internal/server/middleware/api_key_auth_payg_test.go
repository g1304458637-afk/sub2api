//go:build unit

package middleware

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

func TestAPIKeyAuthExhaustionDefersOptedInPAYGToBilling(t *testing.T) {
	gin.SetMode(gin.TestMode)
	for _, usage := range []float64{99, 100, 101} {
		for _, fallback := range []bool{false, true} {
			t.Run(fmtPaygCase(usage, fallback), func(t *testing.T) {
				limit := 100.0
				now := time.Now()
				group := &service.Group{ID: 42, Status: service.StatusActive, Hydrated: true, SubscriptionType: service.SubscriptionTypeSubscription, WeeklyLimitUSD: &limit}
				user := &service.User{ID: 7, Status: service.StatusActive, Balance: 10}
				key := &service.APIKey{ID: 1, UserID: 7, Key: "test-key", Status: service.StatusActive, User: user, Group: group, GroupID: &group.ID}
				keyRepo := &stubApiKeyRepo{getByKey: func(context.Context, string) (*service.APIKey, error) { c := *key; return &c, nil }}
				sub := &service.UserSubscription{ID: 2, UserID: 7, GroupID: 42, Status: service.SubscriptionStatusActive, ExpiresAt: now.Add(time.Hour), DailyWindowStart: &now, WeeklyWindowStart: &now, MonthlyWindowStart: &now, WeeklyUsageUSD: usage, AutoPaygFallback: fallback}
				subRepo := &stubUserSubscriptionRepo{getActive: func(context.Context, int64, int64) (*service.UserSubscription, error) { c := *sub; return &c, nil }}
				cfg := &config.Config{RunMode: config.RunModeStandard}
				keys := service.NewAPIKeyService(keyRepo, nil, nil, nil, nil, nil, cfg)
				subs := service.NewSubscriptionService(nil, subRepo, nil, nil, cfg)
				t.Cleanup(subs.Stop)
				router := gin.New()
				router.Use(gin.HandlerFunc(NewAPIKeyAuthMiddleware(keys, subs, cfg)))
				router.GET("/t", func(c *gin.Context) {
					got, ok := GetSubscriptionFromContext(c)
					require.True(t, ok)
					require.Equal(t, sub.ID, got.ID)
					c.Status(200)
				})
				w := httptest.NewRecorder()
				req := httptest.NewRequest(http.MethodGet, "/t", nil)
				req.Header.Set("x-api-key", "test-key")
				router.ServeHTTP(w, req)
				expected := 200
				if usage >= 100 && !fallback {
					expected = 429
				}
				require.Equal(t, expected, w.Code, w.Body.String())
			})
		}
	}
}
func fmtPaygCase(usage float64, fallback bool) string {
	if usage < 100 {
		if fallback {
			return "99_on"
		}
		return "99_off"
	}
	if usage == 100 {
		if fallback {
			return "100_on"
		}
		return "100_off"
	}
	if fallback {
		return "101_on"
	}
	return "101_off"
}

func TestResetAndMusicReadRoutesRemainAvailableWithoutBalance(t *testing.T) {
	require.True(t, isSubscriptionResetRequest(http.MethodPost, "/v1/muc/reset-with-card/1"))
	require.False(t, isSubscriptionResetRequest(http.MethodGet, "/v1/muc/reset-with-card/1"))
	require.False(t, isSubscriptionResetRequest(http.MethodPost, "/muc/reset-with-card-evil/1"))
	require.True(t, isAsyncImageTaskRead(http.MethodGet, "/v1/audio/music/tasks/task1"))
	require.False(t, isAsyncImageTaskRead(http.MethodPost, "/v1/audio/music/tasks/task1"))
}

func TestExhaustedResetRouteBypassesOnlyBilling(t *testing.T) {
	gin.SetMode(gin.TestMode)
	limit := 100.0
	now := time.Now()
	group := &service.Group{ID: 42, Status: service.StatusActive, Hydrated: true, SubscriptionType: service.SubscriptionTypeSubscription, WeeklyLimitUSD: &limit}
	user := &service.User{ID: 7, Status: service.StatusActive, Balance: 0}
	key := &service.APIKey{ID: 1, UserID: 7, Key: "test-key", Status: service.StatusActive, User: user, Group: group, GroupID: &group.ID}
	keyRepo := &stubApiKeyRepo{getByKey: func(context.Context, string) (*service.APIKey, error) { copy := *key; return &copy, nil }}
	sub := &service.UserSubscription{ID: 2, UserID: 7, GroupID: 42, Status: service.SubscriptionStatusActive, ExpiresAt: now.Add(time.Hour), DailyWindowStart: &now, WeeklyWindowStart: &now, MonthlyWindowStart: &now, WeeklyUsageUSD: 100, AutoPaygFallback: false}
	subRepo := &stubUserSubscriptionRepo{getActive: func(context.Context, int64, int64) (*service.UserSubscription, error) {
		copy := *sub
		return &copy, nil
	}}
	cfg := &config.Config{RunMode: config.RunModeStandard}
	keys := service.NewAPIKeyService(keyRepo, nil, nil, nil, nil, nil, cfg)
	subs := service.NewSubscriptionService(nil, subRepo, nil, nil, cfg)
	t.Cleanup(subs.Stop)
	router := gin.New()
	router.Use(gin.HandlerFunc(NewAPIKeyAuthMiddleware(keys, subs, cfg)))
	reached := 0
	router.POST("/v1/muc/reset-with-card/:id", func(c *gin.Context) { reached++; c.Status(204) })
	router.POST("/v1/messages", func(c *gin.Context) { t.Error("exhausted model request reached handler"); c.Status(204) })
	for _, tc := range []struct {
		path string
		want int
	}{
		{"/v1/muc/reset-with-card/2", 204},
		{"/v1/messages", 429},
	} {
		req := httptest.NewRequest(http.MethodPost, tc.path, nil)
		req.Header.Set("x-api-key", "test-key")
		recorder := httptest.NewRecorder()
		router.ServeHTTP(recorder, req)
		require.Equal(t, tc.want, recorder.Code, recorder.Body.String())
	}
	require.Equal(t, 1, reached)
	for _, path := range []string{"/muc/reset-with-card/2", "/v1/muc/reset-with-card/", "/v1/muc/reset-with-card/0", "/v1/muc/reset-with-card/2/messages", "/v1/muc/reset-with-card/abc"} {
		require.False(t, isSubscriptionResetRequest(http.MethodPost, path), path)
	}
}
