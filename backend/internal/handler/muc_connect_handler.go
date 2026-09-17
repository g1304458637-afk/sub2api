// MUC Harness: 一次性授权码 → per-device API Key。
//
// 安全约定（硬性约束）：
// - Redis 只保存 code 的 SHA-256 哈希（60s TTL），不保存明文 code；
// - GETDEL 保证原子单次使用（用过即失效）；
// - exchange 以绑定的用户身份创建独立 Key（名 "MUC <device>"），可单独撤销；
// - 任何日志不得出现 code 或 API Key 明文；
// - 客户端只拿到该用户自己的调用凭据，绝不接触管理员/上游凭据。

package handler

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/redis/go-redis/v9"

	middleware2 "github.com/Wei-Shaw/sub2api/internal/server/middleware"
	"github.com/Wei-Shaw/sub2api/internal/pkg/response"
	"github.com/Wei-Shaw/sub2api/internal/service"
)

const (
	mucCodeTTL        = 60 * time.Second
	mucCodeKeyPrefix  = "muc:code:"
	mucMaxCodeLength  = 128
	mucMaxDeviceName  = 64
	mucDefaultKeyHint = "MUC Desktop"
)

// 窄接口：仅依赖创建 Key 与查用户两个能力，便于单测打桩
type mucKeyCreator interface {
	Create(ctx context.Context, userID int64, req service.CreateAPIKeyRequest) (*service.APIKey, error)
}

type mucUserLookup interface {
	GetByID(ctx context.Context, id int64) (*service.User, error)
}

type MucConnectHandler struct {
	redisClient   *redis.Client
	apiKeyCreator mucKeyCreator
	userLookup    mucUserLookup
}

func NewMucConnectHandler(redisClient *redis.Client, apiKeyService *service.APIKeyService, userService *service.UserService) *MucConnectHandler {
	return &MucConnectHandler{
		redisClient:   redisClient,
		apiKeyCreator: apiKeyService,
		userLookup:    userService,
	}
}

type mucCodePayload struct {
	UserID int64 `json:"user_id"`
}

// ConnectCode 为已登录用户签发一次性授权码（TTL 60s）。
// POST /api/v1/muc/connect-code   （需网站登录态）
func (h *MucConnectHandler) ConnectCode(c *gin.Context) {
	subject, ok := middleware2.GetAuthSubjectFromContext(c)
	if !ok {
		response.Unauthorized(c, "User not authenticated")
		return
	}

	buf := make([]byte, 24)
	if _, err := rand.Read(buf); err != nil {
		response.ErrorFrom(c, err)
		return
	}
	code := base64.RawURLEncoding.EncodeToString(buf)

	sum := sha256.Sum256([]byte(code))
	payload, err := json.Marshal(mucCodePayload{UserID: subject.UserID})
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	if err := h.redisClient.Set(c.Request.Context(), mucCodeKeyPrefix+hex.EncodeToString(sum[:]), payload, mucCodeTTL).Err(); err != nil {
		response.ErrorFrom(c, err)
		return
	}

	response.Success(c, gin.H{
		"code":       code,
		"expires_in": int(mucCodeTTL.Seconds()),
	})
}

// Exchange 用一次性授权码换取 per-device API Key（公开接口，靠 code 本身授权）。
// POST /api/v1/muc/exchange   body: {"code": "...", "device_name": "..."}
func (h *MucConnectHandler) Exchange(c *gin.Context) {
	var req struct {
		Code       string `json:"code"`
		DeviceName string `json:"device_name"`
	}
	if err := c.ShouldBindJSON(&req); err != nil || req.Code == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "bad_request"})
		return
	}
	if len(req.Code) > mucMaxCodeLength {
		c.JSON(http.StatusBadRequest, gin.H{"error": "bad_request"})
		return
	}

	sum := sha256.Sum256([]byte(req.Code))
	// GETDEL：原子取出并删除 —— 单次使用（验收 D/E）
	payload, err := h.redisClient.GetDel(c.Request.Context(), mucCodeKeyPrefix+hex.EncodeToString(sum[:])).Result()
	if err != nil {
		// 不存在（未签发/已过期/已使用）统一 404，不泄露具体原因
		c.JSON(http.StatusNotFound, gin.H{"error": "code_not_found"})
		return
	}
	var payloadData mucCodePayload
	if err := json.Unmarshal([]byte(payload), &payloadData); err != nil || payloadData.UserID <= 0 {
		c.JSON(http.StatusNotFound, gin.H{"error": "code_not_found"})
		return
	}

	deviceName := mucDefaultKeyHint
	if req.DeviceName != "" {
		if len(req.DeviceName) > mucMaxDeviceName {
			req.DeviceName = req.DeviceName[:mucMaxDeviceName]
		}
		deviceName = "MUC " + req.DeviceName
	}

	// 以绑定用户身份创建 per-device Key（明文只在创建响应中出现一次）
	key, err := h.apiKeyCreator.Create(c.Request.Context(), payloadData.UserID, service.CreateAPIKeyRequest{
		Name: deviceName,
	})
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}

	userDisplay := ""
	if h.userLookup != nil {
		if user, err := h.userLookup.GetByID(c.Request.Context(), payloadData.UserID); err == nil && user != nil {
			userDisplay = user.Email
		}
	}

	response.Success(c, gin.H{
		"gateway":  mucPublicGatewayURL(c),
		"api_key":  key.Key,
		"key_name": key.Name,
		"user":     userDisplay,
	})
}

// 网关对外地址：优先环境变量，其次从请求推导（反代场景跟随 X-Forwarded-Proto）。
func mucPublicGatewayURL(c *gin.Context) string {
	scheme := "https"
	if c.Request.TLS == nil {
		if proto := c.GetHeader("X-Forwarded-Proto"); proto != "" {
			scheme = proto
		} else {
			scheme = "http"
		}
	}
	return scheme + "://" + c.Request.Host
}
