package routes

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/buildinfo"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

func TestHealthzReportsReleaseMetadata(t *testing.T) {
	gin.SetMode(gin.TestMode)
	buildinfo.Version = "2.0.6-rc.1"
	buildinfo.Commit = "2332c2bd4f65f91dcb1dc73a4aff86f93d12c22e"
	buildinfo.Date = "2026-09-22T00:00:00Z"
	buildinfo.Brand = "muc"
	buildinfo.MigrationBaseline = "9a0d68d1670a08fd0e04af951858ffae74424a0a"

	router := gin.New()
	RegisterCommonRoutes(router)
	response := httptest.NewRecorder()
	router.ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/healthz", nil))

	require.Equal(t, http.StatusOK, response.Code)
	var body map[string]string
	require.NoError(t, json.Unmarshal(response.Body.Bytes(), &body))
	require.Equal(t, "2.0.6-rc.1", body["version"])
	require.Equal(t, "2332c2bd4f65f91dcb1dc73a4aff86f93d12c22e", body["commit"])
	require.Equal(t, "2026-09-22T00:00:00Z", body["build_timestamp"])
	require.Equal(t, "muc", body["brand"])
	require.Equal(t, "9a0d68d1670a08fd0e04af951858ffae74424a0a", body["migration_baseline"])
}
