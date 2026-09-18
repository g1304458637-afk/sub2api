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

	"github.com/Wei-Shaw/sub2api/internal/pkg/response"
	middleware2 "github.com/Wei-Shaw/sub2api/internal/server/middleware"
	"github.com/Wei-Shaw/sub2api/internal/service"
)

const (
	mucCodeTTL        = 60 * time.Second
	mucCodeKeyPrefix  = "muc:code:"
	mucMaxCodeLength  = 128
	mucMaxDeviceName  = 64
	mucDefaultKeyHint = "MUC Desktop"
)

// 窄接口：handler 层禁止直接依赖 redis 客户端（depguard: handler-no-repository）。
// 生产由 service.MucCodeStore 适配 *redis.Client；测试用 miniredis 适配桩。
type mucCodeStore interface {
	SetCode(ctx context.Context, key string, payload []byte, ttl time.Duration) error
	GetDelCode(ctx context.Context, key string) (string, error)
}

type mucUserLookup interface {
	GetByID(ctx context.Context, id int64) (*service.User, error)
}

type mucKeyManager interface {
	Create(ctx context.Context, userID int64, req service.CreateAPIKeyRequest) (*service.APIKey, error)
	Delete(ctx context.Context, id int64, userID int64) error
	SearchAPIKeys(ctx context.Context, userID int64, keyword string, limit int) ([]service.APIKey, error)
}

type MucConnectHandler struct {
	codes      mucCodeStore
	keys       mucKeyManager
	userLookup mucUserLookup
}

func NewMucConnectHandler(codeStore *service.MucCodeStore, apiKeyService *service.APIKeyService, userService *service.UserService) *MucConnectHandler {
	return &MucConnectHandler{
		codes:      codeStore,
		keys:       apiKeyService,
		userLookup: userService,
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
	if err := h.codes.SetCode(c.Request.Context(), mucCodeKeyPrefix+hex.EncodeToString(sum[:]), payload, mucCodeTTL); err != nil {
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
	payload, err := h.codes.GetDelCode(c.Request.Context(), mucCodeKeyPrefix+hex.EncodeToString(sum[:]))
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

	// MUC Harness: 轮换——先删除该设备此前的旧 Key 再创建。
	// 同步顺序保证不会误删后续连接创建的新 Key（先建后删在并发重连时会互删），
	// 搜索失败视为轮换失败但不阻断换取（旧 Key 可另行清理）。
	olds, err := h.keys.SearchAPIKeys(c.Request.Context(), payloadData.UserID, deviceName, 50)
	if err == nil {
		for _, old := range olds {
			_ = h.keys.Delete(c.Request.Context(), old.ID, payloadData.UserID)
		}
	}

	// 以绑定用户身份创建 per-device Key（明文只在创建响应中出现一次）
	key, err := h.keys.Create(c.Request.Context(), payloadData.UserID, service.CreateAPIKeyRequest{
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
