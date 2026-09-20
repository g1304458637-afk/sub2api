package admin

// Phase 3D —— Admin Create User 并发字段的 handler 层校验回归。
//
// 锁定：
//   - concurrency = -1 → 绑定层 400（gte=0），service 不被调用，负数不落库；
//   - concurrency 省略 → service 收到 nil（由 service 层回退 default_concurrency 设置）；
//   - concurrency = 0  → service 收到指向 0 的指针（显式 unlimited）。

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

type createUserConcurrencyStub struct {
	*stubAdminService
	calls []*service.CreateUserInput
}

func (s *createUserConcurrencyStub) CreateUser(_ context.Context, input *service.CreateUserInput) (*service.User, error) {
	s.calls = append(s.calls, input)
	user := service.User{ID: 100, Email: input.Email, Status: service.StatusActive}
	return &user, nil
}

func setupCreateUserRouter(serviceStub service.AdminService) *gin.Engine {
	gin.SetMode(gin.TestMode)
	router := gin.New()
	handler := NewUserHandler(serviceStub, nil, nil, nil, nil, nil, nil)
	router.POST("/api/v1/admin/users", handler.Create)
	return router
}

func postCreateUser(t *testing.T, router *gin.Engine, body map[string]any) *httptest.ResponseRecorder {
	t.Helper()
	raw, err := json.Marshal(body)
	require.NoError(t, err)
	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodPost, "/api/v1/admin/users", bytes.NewReader(raw))
	request.Header.Set("Content-Type", "application/json")
	router.ServeHTTP(recorder, request)
	return recorder
}

func TestPhase3D_CreateUser_NegativeConcurrencyRejected(t *testing.T) {
	stub := &createUserConcurrencyStub{stubAdminService: &stubAdminService{}}
	router := setupCreateUserRouter(stub)

	recorder := postCreateUser(t, router, map[string]any{
		"email":       "neg@test.com",
		"password":    "strong-pass",
		"concurrency": -1,
	})
	require.Equal(t, http.StatusBadRequest, recorder.Code,
		"negative concurrency must be rejected by binding (gte=0)")
	require.Empty(t, stub.calls, "service must not be invoked on validation failure")
}

func TestPhase3D_CreateUser_OmittedConcurrencyPassesNil(t *testing.T) {
	stub := &createUserConcurrencyStub{stubAdminService: &stubAdminService{}}
	router := setupCreateUserRouter(stub)

	recorder := postCreateUser(t, router, map[string]any{
		"email":    "omit@test.com",
		"password": "strong-pass",
	})
	require.Equal(t, http.StatusOK, recorder.Code)
	require.Len(t, stub.calls, 1)
	require.Nil(t, stub.calls[0].Concurrency,
		"omitted field must reach the service as nil so it can fall back to default_concurrency")
}

func TestPhase3D_CreateUser_ExplicitZeroPassesPointerToZero(t *testing.T) {
	stub := &createUserConcurrencyStub{stubAdminService: &stubAdminService{}}
	router := setupCreateUserRouter(stub)

	recorder := postCreateUser(t, router, map[string]any{
		"email":       "zero@test.com",
		"password":    "strong-pass",
		"concurrency": 0,
	})
	require.Equal(t, http.StatusOK, recorder.Code)
	require.Len(t, stub.calls, 1)
	require.NotNil(t, stub.calls[0].Concurrency,
		"explicit 0 must be distinguishable from omission")
	require.Equal(t, 0, *stub.calls[0].Concurrency)
}
