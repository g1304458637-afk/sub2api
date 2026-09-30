//go:build unit

package admin

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/server/middleware"
	"github.com/Wei-Shaw/sub2api/internal/service"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

// stubPresentationPricingRepo 内存仓储（admin handler 测试用）。
type stubAdminPresentationPricingRepo struct {
	rows map[string]*service.PresentationModelPricing
}

func (r *stubAdminPresentationPricingRepo) ListAll(ctx context.Context) ([]service.PresentationModelPricing, error) {
	out := make([]service.PresentationModelPricing, 0, len(r.rows))
	for _, v := range r.rows {
		out = append(out, *v)
	}
	return out, nil
}

func (r *stubAdminPresentationPricingRepo) GetByModelName(ctx context.Context, modelName string) (*service.PresentationModelPricing, error) {
	if v, ok := r.rows[modelName]; ok {
		copied := *v
		return &copied, nil
	}
	return nil, service.ErrPresentationPricingNotFound
}

func (r *stubAdminPresentationPricingRepo) Upsert(ctx context.Context, pricing *service.PresentationModelPricing) error {
	existing, ok := r.rows[pricing.ModelName]
	if !ok {
		existing = &service.PresentationModelPricing{}
		r.rows[pricing.ModelName] = existing
	}
	*existing = *pricing
	return nil
}

func (r *stubAdminPresentationPricingRepo) DeleteByModelName(ctx context.Context, modelName string) error {
	delete(r.rows, modelName)
	return nil
}

func newAdminPresentationPricingTestRouter() *gin.Engine {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	h := NewPresentationPricingHandler(service.NewPresentationPricingService(
		&stubAdminPresentationPricingRepo{rows: map[string]*service.PresentationModelPricing{}}))
	r.Use(func(c *gin.Context) {
		c.Set(string(middleware.ContextKeyUser), middleware.AuthSubject{UserID: 7})
		c.Next()
	})
	r.GET("/api/v1/admin/model-presentation-pricing", h.List)
	r.PUT("/api/v1/admin/model-presentation-pricing", h.Upsert)
	r.DELETE("/api/v1/admin/model-presentation-pricing", h.Delete)
	return r
}

func TestAdminPresentationPricingHandler_CRUD(t *testing.T) {
	r := newAdminPresentationPricingTestRouter()

	// 校验失败：token 模式无任何价格 → 400。
	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPut, "/api/v1/admin/model-presentation-pricing",
		strings.NewReader(`{"model_name":"gpt-5.6","billing_mode":"token","enabled":true}`))
	r.ServeHTTP(w, req)
	require.Equal(t, http.StatusBadRequest, w.Code)

	// 负数价格 → 400（binding min=0）。
	w = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodPut, "/api/v1/admin/model-presentation-pricing",
		strings.NewReader(`{"model_name":"gpt-5.6","billing_mode":"token","input_price":-1,"enabled":true}`))
	r.ServeHTTP(w, req)
	require.Equal(t, http.StatusBadRequest, w.Code)

	// 合法保存 → 200 + 回显。
	w = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodPut, "/api/v1/admin/model-presentation-pricing",
		strings.NewReader(`{"model_name":"gpt-5.6","billing_mode":"token","input_price":0.0001,"output_price":0.0002,"enabled":true,"remark":"launch price"}`))
	r.ServeHTTP(w, req)
	require.Equal(t, http.StatusOK, w.Code)
	var saved struct {
		Data presentationPricingItem `json:"data"`
	}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &saved))
	require.Equal(t, "gpt-5.6", saved.Data.ModelName)
	require.InDelta(t, 0.0001, *saved.Data.InputPrice, 1e-12)
	require.NotNil(t, saved.Data.UpdatedBy)
	require.Equal(t, int64(7), *saved.Data.UpdatedBy, "updated_by 取认证上下文，不接受客户端伪造")

	// 列表可见。
	w = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodGet, "/api/v1/admin/model-presentation-pricing", nil)
	r.ServeHTTP(w, req)
	require.Equal(t, http.StatusOK, w.Code)
	var list struct {
		Data struct {
			Items []presentationPricingItem `json:"items"`
		} `json:"data"`
	}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &list))
	require.Len(t, list.Data.Items, 1)

	// 清除 override → 200；再删幂等。
	for i := 0; i < 2; i++ {
		w = httptest.NewRecorder()
		req = httptest.NewRequest(http.MethodDelete, "/api/v1/admin/model-presentation-pricing?model_name=gpt-5.6", nil)
		r.ServeHTTP(w, req)
		require.Equal(t, http.StatusOK, w.Code)
	}

	// 缺 model_name 的删除 → 400。
	w = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodDelete, "/api/v1/admin/model-presentation-pricing", nil)
	r.ServeHTTP(w, req)
	require.Equal(t, http.StatusBadRequest, w.Code)
}
