package admin

import (
	"context"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

type resetPreviewTargets struct{ service.ResetTargetResolver }

func (resetPreviewTargets) DescribeTargets(context.Context, string, []int64, []int64) (*service.ResetTargetSummary, error) {
	return &service.ResetTargetSummary{UniqueUserCount: 2, SubscriptionCount: 2}, nil
}
func TestResetGrantPreviewWithoutIdempotencyKey(t *testing.T) {
	gin.SetMode(gin.TestMode)
	svc := service.NewResetCardService(nil, nil, nil, nil, resetPreviewTargets{}, nil)
	h := NewAdminSubscriptionResetHandler(svc)
	router := gin.New()
	router.POST("/preview", h.PreviewGrantResetCards)
	recorder := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/preview", strings.NewReader(`{"target_mode":"all_active_users","quantity_per_user":3}`))
	req.Header.Set("Content-Type", "application/json")
	router.ServeHTTP(recorder, req)
	require.Equal(t, 200, recorder.Code, recorder.Body.String())
	require.Contains(t, recorder.Body.String(), `"total_cards":6`)
	require.Contains(t, recorder.Body.String(), `"unique_users":2`)
}
