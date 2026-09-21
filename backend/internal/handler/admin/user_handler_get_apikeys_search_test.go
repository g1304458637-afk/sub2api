package admin

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

// getUserAPIKeysSearchStub 捕获传入 GetUserAPIKeys 的 search，其余 AdminService 方法走 baseline stub。
type getUserAPIKeysSearchStub struct {
	service.AdminService
	capturedSearch string
	calls          int
}

func (s *getUserAPIKeysSearchStub) GetUserAPIKeys(_ context.Context, _ int64, _, _ int, _, _, search string) ([]service.APIKey, int64, error) {
	s.capturedSearch = search
	s.calls++
	return []service.APIKey{}, 0, nil
}

func TestAdminUserGetAPIKeys_SearchPassthrough(t *testing.T) {
	gin.SetMode(gin.TestMode)
	cases := []struct {
		name  string
		query string
		want  string
	}{
		// handler 参照用户侧行为 TrimSpace：前端传 "MUC "，服务层收到 "MUC"，
		// NameContainsFold("MUC") 仍可命中所有 "MUC " 前缀的设备 Key
		{"mucode prefix", "?search=MUC%20", "MUC"},
		{"plain keyword", "?search=foo", "foo"},
		{"missing", "", ""},
		{"empty", "?search=", ""},
		{"whitespace trimmed", "?search=%20bar%20", "bar"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			stub := &getUserAPIKeysSearchStub{AdminService: newStubAdminService()}
			r := gin.New()
			h := NewUserHandler(stub, nil, nil, nil, nil, nil, nil)
			r.GET("/admin/users/:id/api-keys", h.GetUserAPIKeys)

			w := httptest.NewRecorder()
			req, _ := http.NewRequest(http.MethodGet, "/admin/users/1/api-keys"+tc.query, nil)
			r.ServeHTTP(w, req)

			require.Equal(t, http.StatusOK, w.Code)
			require.Equal(t, 1, stub.calls)
			require.Equal(t, tc.want, stub.capturedSearch)
		})
	}
}

// 超长 search 截断到 100 字符，与 api_key_handler 的用户侧行为一致。
func TestAdminUserGetAPIKeys_SearchTruncatedTo100(t *testing.T) {
	gin.SetMode(gin.TestMode)
	stub := &getUserAPIKeysSearchStub{AdminService: newStubAdminService()}
	r := gin.New()
	h := NewUserHandler(stub, nil, nil, nil, nil, nil, nil)
	r.GET("/admin/users/:id/api-keys", h.GetUserAPIKeys)

	long := make([]byte, 150)
	for i := range long {
		long[i] = 'x'
	}
	w := httptest.NewRecorder()
	req, _ := http.NewRequest(http.MethodGet, "/admin/users/1/api-keys?search="+string(long), nil)
	r.ServeHTTP(w, req)

	require.Equal(t, http.StatusOK, w.Code)
	require.Len(t, stub.capturedSearch, 100)
}
