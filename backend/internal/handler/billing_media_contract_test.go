package handler

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/config"
	middleware2 "github.com/Wei-Shaw/sub2api/internal/server/middleware"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

type mediaBillingCache struct {
	service.BillingCache
	usage, balance float64
	calls          int
}

func (b *mediaBillingCache) GetUserBalance(context.Context, int64) (float64, error) {
	b.calls++
	return b.balance, nil
}
func (b *mediaBillingCache) GetSubscriptionCache(context.Context, int64, int64) (*service.SubscriptionCacheData, error) {
	b.calls++
	return &service.SubscriptionCacheData{Status: service.SubscriptionStatusActive, ExpiresAt: time.Now().Add(time.Hour), WeeklyUsage: b.usage}, nil
}
func mediaBillingHandler(t *testing.T, cache *mediaBillingCache) *OpenAIGatewayHandler {
	t.Helper()
	cfg := &config.Config{}
	billing := service.NewBillingCacheService(cache, nil, nil, nil, nil, nil, cfg, nil)
	t.Cleanup(billing.Stop)
	return NewOpenAIGatewayHandler(&service.OpenAIGatewayService{}, service.NewConcurrencyService(nil), billing, &service.APIKeyService{}, nil, nil, nil, nil, cfg)
}
func mediaBillingContext(sub *service.UserSubscription) gin.HandlerFunc {
	return func(c *gin.Context) {
		limit := 100.0
		groupID := int64(2)
		group := &service.Group{ID: groupID, Platform: service.PlatformOpenAI, SubscriptionType: service.SubscriptionTypeStandard, AllowImageGeneration: true, WeeklyLimitUSD: &limit}
		if sub != nil {
			group.SubscriptionType = service.SubscriptionTypeSubscription
		}
		user := &service.User{ID: 1}
		c.Set(string(middleware2.ContextKeyAPIKey), &service.APIKey{ID: 3, UserID: 1, User: user, GroupID: &groupID, Group: group})
		c.Set(string(middleware2.ContextKeyUser), middleware2.AuthSubject{UserID: 1})
		if sub != nil {
			c.Set(string(middleware2.ContextKeySubscription), sub)
		}
		c.Next()
	}
}

func TestChatDrawingVoiceRejectBeforeProviderWhenWalletEmpty(t *testing.T) {
	gin.SetMode(gin.TestMode)
	for _, route := range []struct {
		name, path, body string
		handler          func(*OpenAIGatewayHandler) gin.HandlerFunc
	}{
		{"chat", "/v1/chat/completions", `{"model":"gpt-4o","messages":[{"role":"user","content":"test"}]}`, func(h *OpenAIGatewayHandler) gin.HandlerFunc { return h.ChatCompletions }},
		{"drawing", "/v1/images/generations", `{"model":"gpt-image-2","prompt":"test"}`, func(h *OpenAIGatewayHandler) gin.HandlerFunc { return h.Images }},
		{"voice", "/v1/audio/speech", `{"model":"tts-1","input":"test","voice":"alloy"}`, func(h *OpenAIGatewayHandler) gin.HandlerFunc { return h.Speech }},
	} {
		t.Run(route.name, func(t *testing.T) {
			for _, fallback := range []bool{true, false} {
				cache := &mediaBillingCache{usage: 100, balance: 0}
				h := mediaBillingHandler(t, cache)
				router := gin.New()
				router.Use(mediaBillingContext(&service.UserSubscription{ID: 4, AutoPaygFallback: fallback}))
				router.POST(route.path, route.handler(h))
				req := httptest.NewRequest(http.MethodPost, route.path, strings.NewReader(route.body))
				req.Header.Set("Content-Type", "application/json")
				recorder := httptest.NewRecorder()
				router.ServeHTTP(recorder, req)
				expected := http.StatusTooManyRequests
				if fallback {
					expected = http.StatusForbidden
				}
				require.Equal(t, expected, recorder.Code, recorder.Body.String())
				require.Positive(t, cache.calls, "the actual handler must reach billing eligibility")
			}
		})
	}
}

type contractMusicStore struct {
	mu     sync.Mutex
	record *service.MusicTaskRecord
}

func (s *contractMusicStore) Save(_ context.Context, r *service.MusicTaskRecord, _ time.Duration) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	copy := *r
	s.record = &copy
	return nil
}
func (s *contractMusicStore) Get(context.Context, string) (*service.MusicTaskRecord, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.record == nil {
		return nil, service.ErrMusicTaskNotFound
	}
	copy := *s.record
	return &copy, nil
}

func TestMusicHandlerCarriesBillingTargetIntoDetachedJob(t *testing.T) {
	gin.SetMode(gin.TestMode)
	for _, tc := range []struct {
		name           string
		usage, balance float64
		sub, fallback  bool
		status         int
		wantSub        bool
	}{
		{"available", 0, 0, true, false, 202, true}, {"last_call_99", 99, 0, true, false, 202, true},
		{"full_wallet", 100, 1, true, true, 202, false}, {"exhausted_wallet", 101, 1, true, true, 202, false},
		{"insufficient", 100, 0, true, true, 403, false}, {"opt_out", 100, 1, true, false, 429, false},
		{"standard_wallet", 0, 1, false, false, 202, false}, {"standard_empty", 0, 0, false, false, 403, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			cache := &mediaBillingCache{usage: tc.usage, balance: tc.balance}
			h := NewAsyncMusicHandler(service.NewMusicTaskService(&contractMusicStore{}), mediaBillingHandler(t, cache))
			jobs := make(chan *musicTaskJob, 1)
			h.execute = func(_ context.Context, job *musicTaskJob) *musicExecutionOutput {
				jobs <- job
				return &musicExecutionOutput{HTTPStatus: 502}
			}
			var sub *service.UserSubscription
			if tc.sub {
				sub = &service.UserSubscription{ID: 4, AutoPaygFallback: tc.fallback}
			}
			router := gin.New()
			router.Use(mediaBillingContext(sub))
			router.POST("/v1/audio/music", h.Submit)
			req := httptest.NewRequest("POST", "/v1/audio/music", strings.NewReader(`{"model":"suno-v5","prompt":"test"}`))
			req.Header.Set("Content-Type", "application/json")
			recorder := httptest.NewRecorder()
			router.ServeHTTP(recorder, req)
			require.Equal(t, tc.status, recorder.Code, recorder.Body.String())
			if tc.status == 202 {
				select {
				case job := <-jobs:
					require.Equal(t, tc.wantSub, job.Subscription != nil)
				case <-time.After(time.Second):
					t.Fatal("accepted task never executed")
				}
			} else {
				require.Empty(t, jobs)
			}
		})
	}
}
