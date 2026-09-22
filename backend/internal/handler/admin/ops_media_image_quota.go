package admin

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"strings"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/pkg/response"
	"github.com/gin-gonic/gin"
)

// 管理端只读代理：转发本地 codex-image-bridge wrapper 的 /stats。
//
//	GET /api/v1/admin/media/image-quota
//
// - 两个 env 均未配置 → {"enabled":false}
// - wrapper 不可达/超时/非 2xx → {"enabled":true,"reachable":false,"error":"<简要原因>"}
// - 成功 → 透传 wrapper JSON，顶层附加 enabled/reachable=true
const (
	mediaImageBridgeURLEnv = "MUC_IMAGE_BRIDGE_URL"
	mediaImageBridgeKeyEnv = "MUC_IMAGE_BRIDGE_KEY"

	mediaImageBridgeTimeout = 2 * time.Second
	mediaImageBridgeMaxBody = 1 << 20 // 1 MiB
)

// GetImageQuota 查询本地生图 wrapper 的额度统计（只读，不落库、不写配置）。
// GET /api/v1/admin/media/image-quota
func (h *OpsHandler) GetImageQuota(c *gin.Context) {
	// TODO: 迁 settings（当前走进程环境变量，仅作最小可用接入）
	bridgeURL := strings.TrimSpace(os.Getenv(mediaImageBridgeURLEnv))
	bridgeKey := strings.TrimSpace(os.Getenv(mediaImageBridgeKeyEnv))
	if bridgeURL == "" || bridgeKey == "" {
		response.Success(c, gin.H{"enabled": false})
		return
	}

	req, err := http.NewRequestWithContext(c.Request.Context(), http.MethodGet, strings.TrimRight(bridgeURL, "/")+"/stats", nil)
	if err != nil {
		response.Success(c, gin.H{"enabled": true, "reachable": false, "error": "invalid bridge url"})
		return
	}
	req.Header.Set("Authorization", "Bearer "+bridgeKey)

	client := &http.Client{Timeout: mediaImageBridgeTimeout}
	resp, err := client.Do(req)
	if err != nil {
		response.Success(c, gin.H{"enabled": true, "reachable": false, "error": mediaBridgeFetchErrorReason(err)})
		return
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(io.LimitReader(resp.Body, mediaImageBridgeMaxBody))
	if err != nil {
		response.Success(c, gin.H{"enabled": true, "reachable": false, "error": "read wrapper body failed"})
		return
	}
	if resp.StatusCode < http.StatusOK || resp.StatusCode >= http.StatusMultipleChoices {
		response.Success(c, gin.H{
			"enabled":   true,
			"reachable": false,
			"error":     fmt.Sprintf("wrapper http %d", resp.StatusCode),
		})
		return
	}

	var payload map[string]any
	if err := json.Unmarshal(body, &payload); err != nil {
		response.Success(c, gin.H{"enabled": true, "reachable": false, "error": "wrapper returned non-JSON payload"})
		return
	}
	payload["enabled"] = true
	payload["reachable"] = true
	response.Success(c, payload)
}

// mediaBridgeFetchErrorReason 把请求错误压缩成一行简要原因。
func mediaBridgeFetchErrorReason(err error) string {
	if err == nil {
		return "unknown error"
	}
	if errors.Is(err, context.DeadlineExceeded) {
		return "wrapper request timeout"
	}
	var urlErr *url.Error
	if errors.As(err, &urlErr) {
		if urlErr.Timeout() {
			return "wrapper request timeout"
		}
		// url.Error.Error() 形如 `Get "http://x/stats": dial tcp ...`，剥掉前缀只留根因
		return strings.TrimPrefix(urlErr.Error(), urlErr.Op+" \""+urlErr.URL+"\": ")
	}
	return err.Error()
}
