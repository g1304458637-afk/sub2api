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
	"errors"
	"fmt"
	"html"
	"net/http"
	"net/url"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/gin-gonic/gin"

	"github.com/Wei-Shaw/sub2api/internal/pkg/campus"
	"github.com/Wei-Shaw/sub2api/internal/pkg/muccode"
	"github.com/Wei-Shaw/sub2api/internal/pkg/response"
	middleware2 "github.com/Wei-Shaw/sub2api/internal/server/middleware"
	"github.com/Wei-Shaw/sub2api/internal/service"
)

const (
	mucCodeTTL        = 60 * time.Second
	mucCodeKeyPrefix  = "muc:code:"
	mucMaxCodeLength  = 128
	mucMaxDeviceRunes = 64
	mucMaxStoredName  = 255 // api_keys.name 列宽；轮换/截断都按转义后的落库名称对齐
)

// 窄接口：handler 层禁止直接依赖 redis 客户端（depguard: handler-no-repository）。
// 生产由 internal/pkg/muccode.CodeStore 适配 *redis.Client；测试用 miniredis 适配桩。
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
	GetUserGroupVisibility(ctx context.Context, userID int64) (map[int64]struct{}, bool, error)
}

type mucGroupLookup interface {
	GetByID(context.Context, int64) (*service.Group, error)
}
type mucSubscriptionLookup interface {
	ListActiveUserSubscriptions(context.Context, int64) ([]service.UserSubscription, error)
}

type MucConnectHandler struct {
	brand         campus.Brand
	codes         mucCodeStore
	keys          mucKeyManager
	userLookup    mucUserLookup
	groupLookup   mucGroupLookup
	subscriptions mucSubscriptionLookup
	paygGroupID   int64
}

func NewMucConnectHandler(codeStore *muccode.CodeStore, apiKeyService *service.APIKeyService, userService *service.UserService, groupService *service.GroupService, subscriptionService *service.SubscriptionService) *MucConnectHandler {
	paygGroupID := int64(0)
	if configured := strings.TrimSpace(os.Getenv("CAMPUS_PAYG_GROUP_ID")); configured != "" {
		parsed, err := strconv.ParseInt(configured, 10, 64)
		if err != nil || parsed <= 0 {
			panic("invalid CAMPUS_PAYG_GROUP_ID")
		}
		paygGroupID = parsed
	}
	return &MucConnectHandler{
		brand:         campus.Current(),
		codes:         codeStore,
		keys:          apiKeyService,
		userLookup:    userService,
		groupLookup:   groupService,
		subscriptions: subscriptionService,
		paygGroupID:   paygGroupID,
	}
}

type mucCodePayload struct {
	Brand    string `json:"brand"`
	Audience string `json:"audience"`
	UserID   int64  `json:"user_id"`
}

// ConnectCode 为已登录用户签发一次性授权码（TTL 60s）。
// POST /api/v1/muc/connect-code   （需网站登录态）
func (h *MucConnectHandler) ConnectCode(c *gin.Context) {
	brand := h.campusBrand()
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
	payload, err := json.Marshal(mucCodePayload{UserID: subject.UserID, Brand: brand.ID, Audience: brand.Audience()})
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	if err := h.codes.SetCode(c.Request.Context(), brand.RedisPrefix+hex.EncodeToString(sum[:]), payload, mucCodeTTL); err != nil {
		response.ErrorFrom(c, err)
		return
	}

	response.Success(c, gin.H{
		"code":       code,
		"expires_in": int(mucCodeTTL.Seconds()),
		"brand":      brand.ID, "audience": brand.Audience(), "scheme": brand.ProtocolScheme,
	})
}

// Exchange 用一次性授权码换取 per-device API Key（公开接口，靠 code 本身授权）。
// POST /api/v1/muc/exchange   body: {"code": "...", "device_name": "..."}
func (h *MucConnectHandler) Exchange(c *gin.Context) {
	brand := h.campusBrand()
	var req struct {
		Code       string `json:"code"`
		Brand      string `json:"brand"`
		Audience   string `json:"audience"`
		DeviceName string `json:"device_name"`
	}
	if err := c.ShouldBindJSON(&req); err != nil || req.Code == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "bad_request"})
		return
	}
	if (req.Brand != "" && req.Brand != brand.ID) || (req.Audience != "" && req.Audience != brand.Audience()) {
		c.JSON(http.StatusForbidden, gin.H{"error": "brand_mismatch"})
		return
	}
	if len(req.Code) > mucMaxCodeLength {
		c.JSON(http.StatusBadRequest, gin.H{"error": "bad_request"})
		return
	}

	sum := sha256.Sum256([]byte(req.Code))
	// GETDEL：原子取出并删除 —— 单次使用（验收 D/E）
	payload, err := h.codes.GetDelCode(c.Request.Context(), brand.RedisPrefix+hex.EncodeToString(sum[:]))
	if err != nil {
		// 不存在的 code（未签发/已过期/已使用）统一 404，不泄露具体原因；
		// 其余错误是 Redis 基础设施故障，必须与 404 区分，否则故障被伪装成"码无效"。
		if errors.Is(err, muccode.ErrCodeNotFound) {
			c.JSON(http.StatusNotFound, gin.H{"error": "code_not_found"})
			return
		}
		response.ErrorFrom(c, fmt.Errorf("muc exchange: getdel code: %w", err))
		return
	}
	var payloadData mucCodePayload
	if err := json.Unmarshal([]byte(payload), &payloadData); err != nil || payloadData.UserID <= 0 || payloadData.Brand != brand.ID || payloadData.Audience != brand.Audience() {
		c.JSON(http.StatusNotFound, gin.H{"error": "code_not_found"})
		return
	}

	gateway, err := campusGatewayForRequest(c, brand)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "gateway_origin_mismatch"})
		return
	}
	deviceName := brand.DeviceHint
	if req.DeviceName != "" {
		deviceName = brand.KeyPrefix + mucTruncateDeviceName(req.DeviceName)
	}
	// service.CreateAPIKey 落库时会做 html.EscapeString；轮换必须按转义后的
	// 完整名称精确匹配，按子串匹配会误删其他设备/用户手建的同前缀 Key。
	storedName := html.EscapeString(deviceName)

	// 以绑定用户身份创建 per-device Key（明文只在创建响应中出现一次）。
	// 先建后删：创建失败时旧 Key 仍然有效，设备不致凭据全失。
	//
	keyGroupID, err := h.resolveDesktopGroup(c.Request.Context(), payloadData.UserID)
	if err != nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": "desktop_group_unavailable"})
		return
	}
	key, err := h.keys.Create(c.Request.Context(), payloadData.UserID, service.CreateAPIKeyRequest{
		Name:    deviceName,
		GroupID: keyGroupID,
	})
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}

	// MUC Harness: 轮换——删除该设备此前的旧 Key（转义后名称精确相等，排除刚创建的）。
	// 搜索失败视为轮换失败但不阻断换取（旧 Key 可另行清理）。
	if olds, err := h.keys.SearchAPIKeys(c.Request.Context(), payloadData.UserID, brand.KeyPrefix, 50); err == nil {
		for _, old := range olds {
			if old.ID == key.ID || old.Name != storedName {
				continue
			}
			_ = h.keys.Delete(c.Request.Context(), old.ID, payloadData.UserID)
		}
	}

	userDisplay := ""
	if h.userLookup != nil {
		if user, err := h.userLookup.GetByID(c.Request.Context(), payloadData.UserID); err == nil && user != nil {
			userDisplay = mucMaskEmail(user.Email)
		}
	}

	response.Success(c, gin.H{
		"brand": brand.ID, "audience": brand.Audience(),
		"gateway":  gateway,
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

// mucTruncateDeviceName 按 rune 截断设备名（保证 UTF-8 完整，中文不被切碎），
// 并确保 html.EscapeString 转义后的长度不超过 api_keys.name 列宽。
func mucTruncateDeviceName(name string) string {
	runes := []rune(name)
	if len(runes) > mucMaxDeviceRunes {
		runes = runes[:mucMaxDeviceRunes]
	}
	for len(html.EscapeString(string(runes))) > mucMaxStoredName && len(runes) > 0 {
		runes = runes[:len(runes)-1]
	}
	return string(runes)
}

// mucMaskEmail 兑换响应会连同授权码一起留在客户端与浏览器历史里，
// 邮箱只回显足以辨识的脱敏形式。
func mucMaskEmail(email string) string {
	at := strings.LastIndex(email, "@")
	if at <= 0 {
		return "***"
	}
	local, domain := email[:at], email[at+1:]
	if len(local) > 1 {
		local = local[:1] + "***"
	} else {
		local = "***"
	}
	if domain == "" {
		return local
	}
	return local + "@" + domain
}

func (h *MucConnectHandler) campusBrand() campus.Brand {
	if h.brand.ID == "" {
		return campus.Current()
	}
	return h.brand
}

// resolveDesktopGroup never guesses a billing group. The consolidated schema permits
// one active primary subscription; ambiguous legacy rows are rejected until repaired.
func (h *MucConnectHandler) resolveDesktopGroup(ctx context.Context, userID int64) (*int64, error) {
	if h.keys == nil || h.userLookup == nil || h.subscriptions == nil || h.groupLookup == nil {
		return nil, errors.New("desktop group resolver unavailable")
	}
	allowed, restricted, err := h.keys.GetUserGroupVisibility(ctx, userID)
	if err != nil {
		return nil, err
	}
	user, err := h.userLookup.GetByID(ctx, userID)
	if err != nil {
		return nil, err
	}
	if user == nil || !user.IsActive() {
		return nil, errors.New("inactive user")
	}
	subscriptions, err := h.subscriptions.ListActiveUserSubscriptions(ctx, userID)
	if err != nil {
		return nil, err
	}
	var groupID int64
	for _, sub := range subscriptions {
		if sub.UserID != userID || !sub.IsActive() || sub.StartsAt.After(time.Now()) {
			return nil, errors.New("invalid active subscription")
		}
		if groupID != 0 {
			return nil, errors.New("ambiguous primary subscription")
		}
		groupID = sub.GroupID
	}
	hasSubscription := len(subscriptions) > 0
	if !hasSubscription {
		// Wallet-only access is opt-in through an explicitly configured standard group.
		if h.paygGroupID <= 0 || user.Balance <= 0 {
			return nil, errors.New("wallet access unavailable")
		}
		groupID = h.paygGroupID
	}
	if groupID <= 0 {
		return nil, errors.New("missing desktop group")
	}
	if restricted {
		if _, ok := allowed[groupID]; !ok {
			return nil, errors.New("desktop group not visible")
		}
	}
	group, err := h.groupLookup.GetByID(ctx, groupID)
	if err != nil {
		return nil, err
	}
	if group == nil || group.ID != groupID || group.Status != service.StatusActive || group.IsSubscriptionType() != hasSubscription {
		return nil, errors.New("invalid desktop billing group")
	}
	if !hasSubscription && !user.CanBindGroup(group.ID, group.IsExclusive) {
		return nil, errors.New("wallet group not permitted")
	}
	return &groupID, nil
}

func campusGatewayForRequest(c *gin.Context, brand campus.Brand) (string, error) {
	requested, err := url.Parse(mucPublicGatewayURL(c))
	if err != nil || requested.Host == "" || requested.User != nil || requested.RawQuery != "" || requested.Fragment != "" {
		return "", errors.New("invalid request origin")
	}
	local := os.Getenv("CAMPUS_LOCAL_BUILD") == "1" && (requested.Hostname() == "localhost" || requested.Hostname() == "127.0.0.1" || requested.Hostname() == "::1")
	if requested.Scheme != "https" && (!local || requested.Scheme != "http") {
		return "", errors.New("HTTPS required")
	}
	origin := requested.Scheme + "://" + requested.Host
	if brand.GatewayURL != "" && strings.TrimRight(brand.GatewayURL, "/") != origin {
		return "", errors.New("gateway origin mismatch")
	}
	return origin, nil
}
