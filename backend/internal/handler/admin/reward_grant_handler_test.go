package admin

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

// stubRewardGrantRepo 只覆写 AdminList，记录入参并返回预设结果。
type stubRewardGrantRepo struct {
	result *service.RewardGrantList
	filter *service.RewardGrantAdminFilter
}

func (s *stubRewardGrantRepo) InsertIdempotent(ctx context.Context, grant *service.RewardGrant) (bool, error) {
	return true, nil
}

func (s *stubRewardGrantRepo) GetByIdempotencyKey(ctx context.Context, idempotencyKey string) (*service.RewardGrant, error) {
	return nil, nil
}

func (s *stubRewardGrantRepo) GetByUserSourceCampaign(ctx context.Context, userID int64, sourceType, campaign string) ([]service.RewardGrant, error) {
	return nil, nil
}

func (s *stubRewardGrantRepo) ListByUser(ctx context.Context, userID int64, limit int) ([]service.RewardGrant, error) {
	return nil, nil
}

func (s *stubRewardGrantRepo) CountByUser(ctx context.Context, userID int64) (int64, error) {
	return 0, nil
}

func (s *stubRewardGrantRepo) GetBySource(ctx context.Context, sourceType string, sourceID int64) ([]service.RewardGrant, error) {
	return nil, nil
}

func (s *stubRewardGrantRepo) AdminList(ctx context.Context, filter *service.RewardGrantAdminFilter) (*service.RewardGrantList, error) {
	s.filter = filter
	if s.result != nil {
		return s.result, nil
	}
	return &service.RewardGrantList{Items: []service.RewardGrantAdminItem{}}, nil
}

func setupRewardGrantRouter(repo *stubRewardGrantRepo) *gin.Engine {
	gin.SetMode(gin.TestMode)
	router := gin.New()
	svc := service.NewRewardGrantService(nil, repo, nil, nil, nil, nil)
	handler := NewRewardGrantHandler(svc)
	router.GET("/api/v1/admin/reward-grants", handler.List)
	return router
}

func TestRewardGrantHandlerList_InvalidUserID(t *testing.T) {
	repo := &stubRewardGrantRepo{}
	router := setupRewardGrantRouter(repo)

	for _, q := range []string{"?user_id=abc", "?user_id=0", "?user_id=-1", "?user_id=1.5"} {
		rec := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodGet, "/api/v1/admin/reward-grants"+q, nil)
		router.ServeHTTP(rec, req)
		require.Equal(t, http.StatusBadRequest, rec.Code, q)
	}
	// 非法请求不应落到 service
	require.Nil(t, repo.filter)
}

func TestRewardGrantHandlerList_PaginatedShape(t *testing.T) {
	grantedBy := int64(7)
	sourceID := int64(887)
	createdAt := time.Date(2026, 3, 14, 8, 0, 0, 0, time.UTC)
	repo := &stubRewardGrantRepo{result: &service.RewardGrantList{
		Items: []service.RewardGrantAdminItem{{
			RewardGrant: service.RewardGrant{
				ID: 5, UserID: 42, IdempotencyKey: "student_verification:42:2026_spring",
				SourceType: "student_verification", SourceID: &sourceID, Campaign: "2026_spring",
				Amount: 20, GrantedBy: &grantedBy, CreatedAt: createdAt,
			},
			Email: "stu@example.com", Username: "stu", GrantedByEmail: "admin@example.com",
		}},
		Total: 1, Page: 2, PageSize: 50,
	}}
	router := setupRewardGrantRouter(repo)

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/admin/reward-grants?page=2&page_size=50&user_id=42&campaign=2026_spring&source_type=student_verification", nil)
	router.ServeHTTP(rec, req)
	require.Equal(t, http.StatusOK, rec.Code)

	// filter 解析：分页与精确过滤条件原样进入 service
	require.NotNil(t, repo.filter)
	require.Equal(t, 2, repo.filter.Page)
	require.Equal(t, 50, repo.filter.PageSize)
	require.NotNil(t, repo.filter.UserID)
	require.Equal(t, int64(42), *repo.filter.UserID)
	require.Equal(t, "2026_spring", repo.filter.Campaign)
	require.Equal(t, "student_verification", repo.filter.SourceType)

	var body struct {
		Code int `json:"code"`
		Data struct {
			Items []map[string]any `json:"items"`
			Total int64            `json:"total"`
			Page  int              `json:"page"`
			Size  int              `json:"page_size"`
		} `json:"data"`
	}
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &body))
	require.Equal(t, 0, body.Code)
	require.Len(t, body.Data.Items, 1)
	require.Equal(t, int64(1), body.Data.Total)
	require.Equal(t, 2, body.Data.Page)
	require.Equal(t, 50, body.Data.Size)

	item := body.Data.Items[0]
	require.Equal(t, float64(5), item["id"])
	require.Equal(t, float64(42), item["user_id"])
	require.Equal(t, "stu@example.com", item["email"])
	require.Equal(t, "stu", item["username"])
	require.Equal(t, "student_verification:42:2026_spring", item["idempotency_key"])
	require.Equal(t, "student_verification", item["source_type"])
	require.Equal(t, float64(887), item["source_id"])
	require.Equal(t, "2026_spring", item["campaign"])
	require.Equal(t, float64(20), item["amount"])
	require.Equal(t, float64(7), item["granted_by"])
	require.Equal(t, "admin@example.com", item["granted_by_email"])
	// UTC RFC3339
	require.Equal(t, "2026-03-14T08:00:00Z", item["created_at"])
}

func TestRewardGrantHandlerList_EmptyItems(t *testing.T) {
	repo := &stubRewardGrantRepo{}
	router := setupRewardGrantRouter(repo)

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/admin/reward-grants", nil)
	router.ServeHTTP(rec, req)
	require.Equal(t, http.StatusOK, rec.Code)

	var body struct {
		Data struct {
			Items []map[string]any `json:"items"`
		} `json:"data"`
	}
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &body))
	require.NotNil(t, body.Data.Items)
	require.Empty(t, body.Data.Items)
	// 缺省分页：page=1, page_size=20
	require.Equal(t, 1, repo.filter.Page)
	require.Equal(t, 20, repo.filter.PageSize)
}
