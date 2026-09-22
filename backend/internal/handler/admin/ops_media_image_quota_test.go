//go:build unit

package admin

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

const (
	mediaQuotaTestURLEnv = "MUC_IMAGE_BRIDGE_URL"
	mediaQuotaTestKeyEnv = "MUC_IMAGE_BRIDGE_KEY"
)

// serveMediaQuotaStats 起一个假 codex-image-bridge upstream，返回收到的 Authorization 头。
func serveMediaQuotaStats(t *testing.T, status int, body string) (*httptest.Server, *string) {
	t.Helper()
	var authHeader string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		authHeader = r.Header.Get("Authorization")
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(status)
		_, _ = w.Write([]byte(body))
	}))
	t.Cleanup(srv.Close)
	return srv, &authHeader
}

func doGetMediaImageQuota(t *testing.T) *httptest.ResponseRecorder {
	t.Helper()
	gin.SetMode(gin.TestMode)
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest(http.MethodGet, "/api/v1/admin/media/image-quota", nil)
	h := &OpsHandler{}
	h.GetImageQuota(c)
	return w
}

func decodeMediaQuotaData(t *testing.T, w *httptest.ResponseRecorder) map[string]any {
	t.Helper()
	require.Equal(t, http.StatusOK, w.Code)
	var envelope struct {
		Code int            `json:"code"`
		Data map[string]any `json:"data"`
	}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &envelope))
	require.NotNil(t, envelope.Data)
	return envelope.Data
}

func TestAdminMediaImageQuotaPassthroughOnSuccess(t *testing.T) {
	srv, authHeader := serveMediaQuotaStats(t, http.StatusOK,
		`{"daily_limit":500,"daily_used":123,"model":"gpt-image-1","reset_at":"2026-01-01T00:00:00Z"}`)
	t.Setenv(mediaQuotaTestURLEnv, srv.URL)
	t.Setenv(mediaQuotaTestKeyEnv, "test-bridge-key")

	w := doGetMediaImageQuota(t)
	data := decodeMediaQuotaData(t, w)

	require.Equal(t, true, data["enabled"])
	require.Equal(t, true, data["reachable"])
	// wrapper 字段原样保留
	require.Equal(t, float64(500), data["daily_limit"])
	require.Equal(t, float64(123), data["daily_used"])
	require.Equal(t, "gpt-image-1", data["model"])
	// 转发请求带 Bearer 凭证
	require.Equal(t, "Bearer test-bridge-key", *authHeader)
}

func TestAdminMediaImageQuotaConnectionRefused(t *testing.T) {
	// 先起再关，拿到一个确定拒绝连接的地址
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {}))
	srv.Close()
	t.Setenv(mediaQuotaTestURLEnv, srv.URL)
	t.Setenv(mediaQuotaTestKeyEnv, "test-bridge-key")

	w := doGetMediaImageQuota(t)
	data := decodeMediaQuotaData(t, w)

	require.Equal(t, true, data["enabled"])
	require.Equal(t, false, data["reachable"])
	require.NotEmpty(t, data["error"])
}

func TestAdminMediaImageQuotaDisabledWhenEnvMissing(t *testing.T) {
	t.Setenv(mediaQuotaTestURLEnv, "")
	t.Setenv(mediaQuotaTestKeyEnv, "")

	w := doGetMediaImageQuota(t)
	data := decodeMediaQuotaData(t, w)

	require.Equal(t, false, data["enabled"])
	_, hasReachable := data["reachable"]
	require.False(t, hasReachable)
}

func TestAdminMediaImageQuotaUpstreamErrorStatus(t *testing.T) {
	srv, _ := serveMediaQuotaStats(t, http.StatusInternalServerError, `{"error":"boom"}`)
	t.Setenv(mediaQuotaTestURLEnv, srv.URL)
	t.Setenv(mediaQuotaTestKeyEnv, "test-bridge-key")

	w := doGetMediaImageQuota(t)
	data := decodeMediaQuotaData(t, w)

	require.Equal(t, true, data["enabled"])
	require.Equal(t, false, data["reachable"])
	require.Contains(t, data["error"], "wrapper http 500")
}

func TestAdminMediaImageQuotaRejectsRedirect(t *testing.T) {
	var received bool
	target := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { received = true }))
	defer target.Close()
	bridge := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { http.Redirect(w, r, target.URL, http.StatusFound) }))
	defer bridge.Close()
	t.Setenv(mediaQuotaTestURLEnv, bridge.URL)
	t.Setenv(mediaQuotaTestKeyEnv, "test-key")
	data := decodeMediaQuotaData(t, doGetMediaImageQuota(t))
	require.Equal(t, false, data["reachable"])
	require.False(t, received, "bridge credentials must not follow redirects")
}

func TestAdminMediaImageQuotaRejectsInvalidConfiguration(t *testing.T) {
	for _, endpoint := range []string{"file:///etc/passwd", "http://user:pass@localhost", "http://", "http://localhost?target=x", "http://localhost#fragment"} {
		t.Run(endpoint, func(t *testing.T) {
			t.Setenv(mediaQuotaTestURLEnv, endpoint)
			t.Setenv(mediaQuotaTestKeyEnv, "test-key")
			data := decodeMediaQuotaData(t, doGetMediaImageQuota(t))
			require.Equal(t, false, data["reachable"])
			require.Equal(t, "invalid bridge url", data["error"])
		})
	}
}
