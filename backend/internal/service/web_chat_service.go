package service

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"sort"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/config"
	infraerrors "github.com/Wei-Shaw/sub2api/internal/pkg/errors"
	"github.com/Wei-Shaw/sub2api/internal/pkg/pagination"

	"github.com/gin-gonic/gin"
)

// 网页聊天/网页绘图：面板用户通过 JWT 鉴权后，由本服务代理到本机网关
// /v1/chat/completions 与 /v1/images/generations（HTTP 回环转发），完整复用网关
// 中间件链——计费、限额、分组 allowlist、用量日志全部原路生效。
//
// 鉴权采用"网页聊天"专用 API Key（按用户 get-or-create）。关于免分组 Key 的调查结论
// （为什么必须绑定具体分组，而不是建 GroupID=nil 的 Key）：
//  1. 鉴权层 RequireGroupAssignment 中间件对未分组 Key 直接拦截，站点开关
//     allow_ungrouped_key_scheduling 默认 false（fail-closed），回环请求会拿到 403；
//  2. 即便开关打开，调度层 GatewayService.isAccountInGroup 对 nil 分组只匹配
//     "未分组账号"（len(AccountGroups)==0），并非用户可见分组的模型并集，
//     "免分组 Key = 可用面为并集"的假设在本网关不成立。
// 因此退而求其次：Key 绑定到用户可见（可绑定）分组之一——优先选可用模型包含
// web_chat_default_model 的第一个分组，无默认模型或无法枚举（GetAvailableModels
// 返回 nil 表示该分组未配置 model_mapping）时取第一个分组。

const (
	// WebChatAPIKeyName 网页聊天专用 API Key 名称（按用户 get-or-create）
	WebChatAPIKeyName = "网页聊天"

	// WebChatChatTotalTimeout 聊天转发整体超时（含 SSE 全程）
	WebChatChatTotalTimeout = 10 * time.Minute
	// WebChatChatIdleTimeout SSE 空闲读超时：超过该时长上游没有任何新数据则中断
	WebChatChatIdleTimeout = 5 * time.Minute
	// WebChatImageTimeout 绘图转发（非流式）超时
	WebChatImageTimeout = 300 * time.Second
	// webChatUpstreamErrorBodyLimit 上游流开始前错误体的透传上限
	webChatUpstreamErrorBodyLimit = 1 << 20
	// webChatSSEMaxLineSize SSE 单行上限（chunk 中可能携带较大 delta/base64）
	webChatSSEMaxLineSize = 4 << 20
	// webChatBinaryResponseLimit 语音/绘图等二进制或 JSON 响应的缓冲上限（32MB）
	webChatBinaryResponseLimit = 32 << 20
)

// WebChatModelType 模型类型：网页聊天 / 网页绘图 / 网页语音合成 / 网页音乐
const (
	WebChatModelTypeChat  = "chat"
	WebChatModelTypeImage = "image"
	WebChatModelTypeTTS   = "tts"
	WebChatModelTypeMusic = "music"
)

// WebChatModel config 端点返回的单条模型描述（冻结契约字段）
type WebChatModel struct {
	Model       string `json:"model"`
	DisplayName string `json:"display_name"`
	Vendor      string `json:"vendor"`
	Description string `json:"description"`
	Type        string `json:"type"`     // "chat" | "image" | "tts" | "music"
	APIOnly     bool   `json:"api_only"` // true = 仅 API 使用，网页聊天不可选
}

// WebChatConfig GET /api/v1/web-chat/config 的 data
type WebChatConfig struct {
	Enabled      bool           `json:"enabled"`
	DefaultModel string         `json:"default_model"`
	Models       []WebChatModel `json:"models"`
}

// WebChatMessage 聊天请求消息（content 以原始 JSON 透传，避免有损转换）
type WebChatMessage struct {
	Role    string          `json:"role"`
	Content json.RawMessage `json:"content"`
}

// WebChatImageGenerationRequest POST /api/v1/web-chat/images/generations 请求体
type WebChatImageGenerationRequest struct {
	Model  string `json:"model"`
	Prompt string `json:"prompt"`
	Size   string `json:"size,omitempty"`
	N      int    `json:"n,omitempty"`
}

// WebChatAudioSpeechRequest POST /api/v1/web-chat/audio/speech 请求体
type WebChatAudioSpeechRequest struct {
	Model string `json:"model"`
	Input string `json:"input"`
}

// WebChatImageEditRequest POST /api/v1/web-chat/images/edits 请求体。
// Image 为 data URL 数组，转发时逐条包装为网关 JSON 编辑形状
// {"images":[{"image_url":"<data URL>"}]}（见 ParseOpenAIImagesRequest）。
type WebChatImageEditRequest struct {
	Model  string   `json:"model"`
	Prompt string   `json:"prompt"`
	Image  []string `json:"image"`
	Size   string   `json:"size,omitempty"`
	N      int      `json:"n,omitempty"`
}

// WebChatMusicGenerationRequest POST /api/v1/web-chat/music/generations 请求体
type WebChatMusicGenerationRequest struct {
	Model        string `json:"model"`
	Prompt       string `json:"prompt"`
	Lyrics       string `json:"lyrics,omitempty"`
	Instrumental bool   `json:"instrumental,omitempty"`
	Seconds      int    `json:"seconds,omitempty"`
}

// WebChatUserGroupReader 网页聊天依赖的用户/Key 能力（*APIKeyService 实现）
type WebChatUserGroupReader interface {
	GetUserGroupVisibility(ctx context.Context, userID int64) (map[int64]struct{}, bool, error)
	List(ctx context.Context, userID int64, params pagination.PaginationParams, filters APIKeyListFilters) ([]APIKey, *pagination.PaginationResult, error)
	Create(ctx context.Context, userID int64, req CreateAPIKeyRequest) (*APIKey, error)
}

// WebChatGroupLister 分组列表能力（*GroupService 实现）
type WebChatGroupLister interface {
	ListActive(ctx context.Context) ([]Group, error)
}

// WebChatModelLister 分组可用模型能力（*GatewayService 实现）
type WebChatModelLister interface {
	GetAvailableModels(ctx context.Context, groupID *int64, platform string) []string
}

// WebChatService 网页聊天/绘图代理服务
type WebChatService struct {
	settingService *SettingService
	apiKeyService  WebChatUserGroupReader
	gatewayService WebChatModelLister
	groupService   WebChatGroupLister
	cfg            *config.Config

	httpClient *http.Client

	// getOrCreateMu 串行化同一用户的 Key get-or-create，避免并发首请求重复建 Key。
	// 网页聊天非性能敏感路径，进程内全局锁即可。
	getOrCreateMu sync.Mutex
}

// NewWebChatService 创建网页聊天代理服务
func NewWebChatService(
	settingService *SettingService,
	apiKeyService *APIKeyService,
	gatewayService *GatewayService,
	groupService *GroupService,
	cfg *config.Config,
) *WebChatService {
	return &WebChatService{
		settingService: settingService,
		apiKeyService:  apiKeyService,
		gatewayService: gatewayService,
		groupService:   groupService,
		cfg:            cfg,
		httpClient:     &http.Client{},
	}
}

// parseWebChatModels 解析 web_chat_models 设置（JSON 数组）。容错语义：
// 整体非法 → 返回空（回退模式语义由调用方按"非空判断"处理）；单条坏条目忽略，
// 不影响其余条目。model 为空或 type 非法视为坏条目；display_name 缺省回退为 model。
func parseWebChatModels(raw string) []WebChatModel {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return nil
	}
	var items []json.RawMessage
	if err := json.Unmarshal([]byte(raw), &items); err != nil {
		return nil
	}
	models := make([]WebChatModel, 0, len(items))
	for _, item := range items {
		var m WebChatModel
		if err := json.Unmarshal(item, &m); err != nil {
			continue
		}
		m.Model = strings.TrimSpace(m.Model)
		m.Type = strings.ToLower(strings.TrimSpace(m.Type))
		if m.Model == "" {
			continue
		}
		if !webChatModelTypeSupported(m.Type) {
			continue
		}
		m.DisplayName = strings.TrimSpace(m.DisplayName)
		if m.DisplayName == "" {
			m.DisplayName = m.Model
		}
		// vendor 缺省时按模型前缀推断，与管理端展示保持一致
		m.Vendor = strings.TrimSpace(m.Vendor)
		if m.Vendor == "" {
			m.Vendor = WebChatVendorForModel(m.Model)
		}
		models = append(models, m)
	}
	return models
}

// webChatModelTypeSupported 判定模型类型是否被网页端支持（parse 阶段的准入集）
func webChatModelTypeSupported(modelType string) bool {
	switch modelType {
	case WebChatModelTypeChat, WebChatModelTypeImage, WebChatModelTypeTTS, WebChatModelTypeMusic:
		return true
	default:
		return false
	}
}

// WebChatVendorForModel 按模型 ID 前缀推断厂商展示名（未命中返回空串）
func WebChatVendorForModel(model string) string {
	model = strings.ToLower(strings.TrimSpace(model))
	switch {
	case model == "":
		return ""
	case strings.HasPrefix(model, "glm"):
		return "智谱"
	case strings.HasPrefix(model, "claude"):
		return "Anthropic"
	case strings.HasPrefix(model, "gpt"), strings.HasPrefix(model, "o1"), strings.HasPrefix(model, "o3"):
		return "OpenAI"
	case strings.HasPrefix(model, "gemini"):
		return "Google"
	case strings.HasPrefix(model, "grok"):
		return "xAI"
	case strings.HasPrefix(model, "deepseek"):
		return "DeepSeek"
	case strings.HasPrefix(model, "qwen"):
		return "通义千问"
	case strings.HasPrefix(model, "doubao"):
		return "字节跳动"
	case strings.HasPrefix(model, "kimi"), strings.HasPrefix(model, "moonshot"):
		return "月之暗面"
	default:
		return ""
	}
}

// WebChatAllowedModelSet 从候选模型中筛出网页端可选集：type 匹配且非 api_only。
// 返回的 set 用于服务端强制白名单判定（请求 model 不在集合内一律 400）。
func WebChatAllowedModelSet(models []WebChatModel, modelType string) map[string]struct{} {
	set := make(map[string]struct{}, len(models))
	for _, m := range models {
		if m.Type != modelType || m.APIOnly {
			continue
		}
		set[m.Model] = struct{}{}
	}
	return set
}

// WebChatModelAllowed 判定 model 是否在可选集内
func WebChatModelAllowed(models []WebChatModel, modelType, model string) bool {
	_, ok := WebChatAllowedModelSet(models, modelType)[model]
	return ok
}

// webChatVisibleGroups 计算用户可见分组。
// GetUserGroupVisibility 返回 user_allowed_groups 授权 + 有效订阅的分组集合，以及
// 用户是否开启了"公开分组限制"；非专属（公开）分组默认全员可见，受限用户则必须
// 落在集合内。仅统计活跃分组。
func webChatVisibleGroups(groups []Group, allowed map[int64]struct{}, restrictPublicGroups bool) []Group {
	visible := make([]Group, 0, len(groups))
	for _, g := range groups {
		if _, ok := allowed[g.ID]; ok {
			visible = append(visible, g)
			continue
		}
		if !g.IsExclusive && !restrictPublicGroups {
			visible = append(visible, g)
		}
	}
	return visible
}

// webChatUnionModels 多分组可用模型并集（有序去重）
func webChatUnionModels(modelsByGroup [][]string) []string {
	seen := make(map[string]struct{})
	union := make([]string, 0)
	for _, models := range modelsByGroup {
		for _, m := range models {
			if m == "" {
				continue
			}
			if _, ok := seen[m]; ok {
				continue
			}
			seen[m] = struct{}{}
			union = append(union, m)
		}
	}
	sort.Strings(union)
	return union
}

// fallbackModels 回退模式：取当前用户可见分组的可用模型并集，每条生成
// {model, display_name=model, vendor=前缀推断, description="", type="chat", api_only=false}。
func (s *WebChatService) fallbackModels(ctx context.Context, userID int64) []WebChatModel {
	if s.apiKeyService == nil || s.gatewayService == nil {
		return []WebChatModel{}
	}
	allowed, restrictPublicGroups, err := s.apiKeyService.GetUserGroupVisibility(ctx, userID)
	if err != nil {
		return []WebChatModel{}
	}
	if s.groupService == nil {
		return []WebChatModel{}
	}
	groups, err := s.groupService.ListActive(ctx)
	if err != nil {
		return []WebChatModel{}
	}
	visible := webChatVisibleGroups(groups, allowed, restrictPublicGroups)
	perGroup := make([][]string, 0, len(visible))
	for _, g := range visible {
		perGroup = append(perGroup, s.gatewayService.GetAvailableModels(ctx, &g.ID, ""))
	}
	union := webChatUnionModels(perGroup)
	models := make([]WebChatModel, 0, len(union))
	for _, m := range union {
		models = append(models, WebChatModel{
			Model:       m,
			DisplayName: m,
			Vendor:      WebChatVendorForModel(m),
			Type:        WebChatModelTypeChat,
		})
	}
	return models
}

// GetConfig 网页聊天配置（config 端点）。enabled=false 时 models 返回空数组，
// default_model 原样回传设置值。
func (s *WebChatService) GetConfig(ctx context.Context, userID int64) (*WebChatConfig, error) {
	settings, err := s.GetWebChatSettings(ctx)
	if err != nil {
		return nil, err
	}
	models := []WebChatModel{}
	if m := s.configModels(ctx, settings, userID); len(m) > 0 {
		models = m
	}
	return &WebChatConfig{
		Enabled:      settings.WebChatEnabled,
		DefaultModel: settings.WebChatDefaultModel,
		Models:       models,
	}, nil
}

// AdminAvailableModels 管理端：所有活跃分组可用模型 ID 并集（供管理员配置
// web_chat_models 时选取）。
func (s *WebChatService) AdminAvailableModels(ctx context.Context) ([]string, error) {
	if s.groupService == nil || s.gatewayService == nil {
		return []string{}, nil
	}
	groups, err := s.groupService.ListActive(ctx)
	if err != nil {
		return nil, fmt.Errorf("list active groups: %w", err)
	}
	perGroup := make([][]string, 0, len(groups))
	for _, g := range groups {
		group := g
		perGroup = append(perGroup, s.gatewayService.GetAvailableModels(ctx, &group.ID, ""))
	}
	union := webChatUnionModels(perGroup)
	if union == nil {
		union = []string{}
	}
	return union, nil
}

// GetWebChatSettings 读取并解析网页聊天相关设置
func (s *WebChatService) GetWebChatSettings(ctx context.Context) (*SystemSettings, error) {
	settings, err := s.settingService.GetAllSettings(ctx)
	if err != nil {
		return nil, fmt.Errorf("get web chat settings: %w", err)
	}
	return settings, nil
}

// configModels 计算给定设置下的候选模型（enabled=false → 空）
func (s *WebChatService) configModels(ctx context.Context, settings *SystemSettings, userID int64) []WebChatModel {
	if settings == nil || !settings.WebChatEnabled {
		return nil
	}
	if parsed := parseWebChatModels(settings.WebChatModels); len(parsed) > 0 {
		return parsed
	}
	if strings.TrimSpace(settings.WebChatModels) != "" {
		// 配置了但整体解析失败：不回退（fail-closed），网页端无可选模型
		return nil
	}
	return s.fallbackModels(ctx, userID)
}

// checkModelAllowedWithSettings 基于已读取的设置做白名单判定
func (s *WebChatService) checkModelAllowedWithSettings(ctx context.Context, settings *SystemSettings, userID int64, model, modelType string) error {
	if settings == nil || !settings.WebChatEnabled {
		return infraerrors.Forbidden("WEB_CHAT_DISABLED", "Web chat is disabled")
	}
	if !WebChatModelAllowed(s.configModels(ctx, settings, userID), modelType, model) {
		return infraerrors.BadRequest("WEB_CHAT_MODEL_NOT_ALLOWED", fmt.Sprintf("Model %q is not available for web chat", model))
	}
	return nil
}

// webChatLoopbackBaseURL 回环转发目标：本机监听端口（固定 127.0.0.1，
// 不依赖 Server.Host——它可能是 0.0.0.0 等通配地址）。
func (s *WebChatService) webChatLoopbackBaseURL() string {
	port := 0
	if s.cfg != nil {
		port = s.cfg.Server.Port
	}
	return fmt.Sprintf("http://127.0.0.1:%d", port)
}

// getOrCreateAPIKey 取（或创建）当前用户的"网页聊天"专用 Key，返回明文 Key。
// API Key 在库中即明文存储（apiKeyEntityToService 直接回带 Key），列表路径即可读取。
func (s *WebChatService) getOrCreateAPIKey(ctx context.Context, userID int64, defaultModel string) (string, error) {
	s.getOrCreateMu.Lock()
	defer s.getOrCreateMu.Unlock()

	filters := APIKeyListFilters{Search: WebChatAPIKeyName, Status: StatusActive}
	keys, _, err := s.apiKeyService.List(ctx, userID, pagination.PaginationParams{
		Page:      1,
		PageSize:  100,
		SortOrder: pagination.SortOrderAsc,
	}, filters)
	if err == nil {
		for i := range keys {
			if keys[i].Name != WebChatAPIKeyName || keys[i].Key == "" {
				continue
			}
			if !keys[i].IsActive() || keys[i].IsExpired() {
				continue
			}
			return keys[i].Key, nil
		}
	}

	// 选择绑定分组（决策见文件头注释）
	groupID, err := s.pickGroupID(ctx, userID, defaultModel)
	if err != nil {
		return "", err
	}
	apiKey, err := s.apiKeyService.Create(ctx, userID, CreateAPIKeyRequest{
		Name:    WebChatAPIKeyName,
		GroupID: groupID,
	})
	if err != nil {
		return "", fmt.Errorf("create web chat api key: %w", err)
	}
	return apiKey.Key, nil
}

// pickGroupID 从用户可见分组中选择 Key 绑定分组：优先可用模型包含 defaultModel
// 的第一个分组；无默认模型 / 分组模型不可枚举时取第一个可见分组。
func (s *WebChatService) pickGroupID(ctx context.Context, userID int64, defaultModel string) (*int64, error) {
	allowed, restrictPublicGroups, err := s.apiKeyService.GetUserGroupVisibility(ctx, userID)
	if err != nil {
		return nil, fmt.Errorf("get user group visibility: %w", err)
	}
	if s.groupService == nil {
		return nil, fmt.Errorf("group service unavailable")
	}
	groups, err := s.groupService.ListActive(ctx)
	if err != nil {
		return nil, fmt.Errorf("list active groups: %w", err)
	}
	visible := webChatVisibleGroups(groups, allowed, restrictPublicGroups)
	if len(visible) == 0 {
		return nil, infraerrors.Forbidden("WEB_CHAT_NO_AVAILABLE_GROUP", "No available group for web chat")
	}

	defaultModel = strings.TrimSpace(defaultModel)
	if defaultModel != "" {
		for i := range visible {
			g := visible[i]
			models := s.gatewayService.GetAvailableModels(ctx, &g.ID, "")
			for _, m := range models {
				if m == defaultModel {
					id := g.ID
					return &id, nil
				}
			}
		}
	}
	id := visible[0].ID
	return &id, nil
}

// newLoopbackRequest 构造回环 POST 请求（带专用 Key 鉴权头）
func (s *WebChatService) newLoopbackRequest(ctx context.Context, path string, body []byte, apiKey string) (*http.Request, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, s.webChatLoopbackBaseURL()+path, bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("build loopback request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+apiKey)
	return req, nil
}

// writeUpstreamError 上游流开始前的 4xx/5xx：状态码与错误 JSON 原样透传给前端。
func writeUpstreamError(c *gin.Context, resp *http.Response) {
	body, _ := io.ReadAll(io.LimitReader(resp.Body, webChatUpstreamErrorBodyLimit))
	contentType := resp.Header.Get("Content-Type")
	if contentType == "" {
		contentType = "application/json"
	}
	c.Data(resp.StatusCode, contentType, body)
}

// ProxyChatCompletions 聊天回环转发（SSE）。
//
// 白名单校验通过后，以"网页聊天"专用 Key POST 本机 /v1/chat/completions
// （stream 强制 true），上游 SSE 逐行复制到客户端并每行 Flush；客户端断开
// （c.Request.Context() 取消）传播到上游请求。上游 4xx/5xx（流开始前）原样透传。
//
// 返回 error 仅表示流开始前的本地失败（由 handler 统一转 {code,message} 形状）；
// 一旦进入转发阶段，响应已直接写回 c。
func (s *WebChatService) ProxyChatCompletions(c *gin.Context, userID int64, model string, messages []WebChatMessage) error {
	settings, err := s.GetWebChatSettings(c.Request.Context())
	if err != nil {
		return err
	}
	if err := s.checkModelAllowedWithSettings(c.Request.Context(), settings, userID, model, WebChatModelTypeChat); err != nil {
		return err
	}
	apiKey, err := s.getOrCreateAPIKey(c.Request.Context(), userID, settings.WebChatDefaultModel)
	if err != nil {
		return err
	}

	payload, err := json.Marshal(map[string]any{
		"model":    model,
		"messages": messages,
		"stream":   true, // 强制流式
	})
	if err != nil {
		return fmt.Errorf("marshal web chat request: %w", err)
	}

	// 整体超时 10 分钟；父 ctx 为客户端请求 ctx，客户端断开自动传播到上游
	ctx, cancel := context.WithTimeout(c.Request.Context(), WebChatChatTotalTimeout)
	defer cancel()
	req, err := s.newLoopbackRequest(ctx, "/v1/chat/completions", payload, apiKey)
	if err != nil {
		return err
	}
	req.Header.Set("Accept", "text/event-stream")

	resp, err := s.httpClient.Do(req)
	if err != nil {
		return infraerrors.New(http.StatusBadGateway, "WEB_CHAT_UPSTREAM_UNAVAILABLE", "Web chat upstream is unavailable")
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		writeUpstreamError(c, resp)
		return nil
	}

	// 空闲读超时（5 分钟）：watchdog 周期检查最近一次读到数据的时间
	var lastActivity atomic.Int64
	lastActivity.Store(time.Now().UnixNano())
	wdCtx, wdCancel := context.WithCancel(ctx)
	defer wdCancel()
	go func() {
		ticker := time.NewTicker(1 * time.Second)
		defer ticker.Stop()
		for {
			select {
			case <-wdCtx.Done():
				return
			case <-ticker.C:
				if time.Since(time.Unix(0, lastActivity.Load())) > WebChatChatIdleTimeout {
					cancel() // 触发上游请求取消
					return
				}
			}
		}
	}()

	// SSE 响应头
	c.Writer.Header().Set("Content-Type", "text/event-stream")
	c.Writer.Header().Set("Cache-Control", "no-cache")
	c.Writer.Header().Set("X-Accel-Buffering", "no")
	c.Writer.WriteHeader(http.StatusOK)

	scanner := bufio.NewScanner(resp.Body)
	scanner.Buffer(make([]byte, 0, 64*1024), webChatSSEMaxLineSize)
	for scanner.Scan() {
		lastActivity.Store(time.Now().UnixNano())
		if _, werr := c.Writer.WriteString(scanner.Text() + "\n"); werr != nil {
			cancel() // 客户端断开 → 传播到上游
			return nil
		}
		c.Writer.Flush()
	}
	return nil
}

// ProxyImagesGenerations 绘图回环转发（非流式，缓冲响应原样回传，客户端超时 300s）。
func (s *WebChatService) ProxyImagesGenerations(c *gin.Context, userID int64, reqBody WebChatImageGenerationRequest) error {
	settings, err := s.GetWebChatSettings(c.Request.Context())
	if err != nil {
		return err
	}
	if err := s.checkModelAllowedWithSettings(c.Request.Context(), settings, userID, reqBody.Model, WebChatModelTypeImage); err != nil {
		return err
	}
	apiKey, err := s.getOrCreateAPIKey(c.Request.Context(), userID, settings.WebChatDefaultModel)
	if err != nil {
		return err
	}

	payload, err := json.Marshal(reqBody)
	if err != nil {
		return fmt.Errorf("marshal web chat image request: %w", err)
	}

	ctx, cancel := context.WithTimeout(c.Request.Context(), WebChatImageTimeout)
	defer cancel()
	httpReq, err := s.newLoopbackRequest(ctx, "/v1/images/generations", payload, apiKey)
	if err != nil {
		return err
	}
	resp, err := s.httpClient.Do(httpReq)
	if err != nil {
		return infraerrors.New(http.StatusBadGateway, "WEB_CHAT_UPSTREAM_UNAVAILABLE", "Web chat upstream is unavailable")
	}
	defer func() { _ = resp.Body.Close() }()

	body, readErr := io.ReadAll(io.LimitReader(resp.Body, 32<<20)) // 绘图响应缓冲上限 32MB
	if readErr != nil {
		return infraerrors.New(http.StatusBadGateway, "WEB_CHAT_UPSTREAM_READ_FAILED", "Failed to read upstream response")
	}
	contentType := resp.Header.Get("Content-Type")
	if contentType == "" {
		contentType = "application/json"
	}
	c.Data(resp.StatusCode, contentType, body)
	return nil
}

// ProxyImagesEdits 绘图编辑回环转发（非流式，缓冲响应原样回传，客户端超时 300s）。
//
// 网页端请求 {model, prompt, image: ["data:..."], size?, n?} 被改写为网关
// /v1/images/edits 的 JSON 编辑形状：images 数组逐条包装为
// {"image_url": "<data URL>"}（与 ParseOpenAIImagesRequest 的 JSON 解析字段一致，
// data URL 原样透传）。
func (s *WebChatService) ProxyImagesEdits(c *gin.Context, userID int64, reqBody WebChatImageEditRequest) error {
	settings, err := s.GetWebChatSettings(c.Request.Context())
	if err != nil {
		return err
	}
	if err := s.checkModelAllowedWithSettings(c.Request.Context(), settings, userID, reqBody.Model, WebChatModelTypeImage); err != nil {
		return err
	}
	apiKey, err := s.getOrCreateAPIKey(c.Request.Context(), userID, settings.WebChatDefaultModel)
	if err != nil {
		return err
	}

	images := make([]map[string]string, 0, len(reqBody.Image))
	for _, img := range reqBody.Image {
		img = strings.TrimSpace(img)
		if img == "" {
			continue
		}
		images = append(images, map[string]string{"image_url": img})
	}
	payloadMap := map[string]any{
		"model":  reqBody.Model,
		"prompt": reqBody.Prompt,
		"images": images,
	}
	if strings.TrimSpace(reqBody.Size) != "" {
		payloadMap["size"] = strings.TrimSpace(reqBody.Size)
	}
	if reqBody.N > 0 {
		payloadMap["n"] = reqBody.N
	}
	payload, err := json.Marshal(payloadMap)
	if err != nil {
		return fmt.Errorf("marshal web chat image edit request: %w", err)
	}

	ctx, cancel := context.WithTimeout(c.Request.Context(), WebChatImageTimeout)
	defer cancel()
	httpReq, err := s.newLoopbackRequest(ctx, "/v1/images/edits", payload, apiKey)
	if err != nil {
		return err
	}
	resp, err := s.httpClient.Do(httpReq)
	if err != nil {
		return infraerrors.New(http.StatusBadGateway, "WEB_CHAT_UPSTREAM_UNAVAILABLE", "Web chat upstream is unavailable")
	}
	defer func() { _ = resp.Body.Close() }()

	body, readErr := io.ReadAll(io.LimitReader(resp.Body, webChatBinaryResponseLimit))
	if readErr != nil {
		return infraerrors.New(http.StatusBadGateway, "WEB_CHAT_UPSTREAM_READ_FAILED", "Failed to read upstream response")
	}
	contentType := resp.Header.Get("Content-Type")
	if contentType == "" {
		contentType = "application/json"
	}
	c.Data(resp.StatusCode, contentType, body)
	return nil
}

// ProxyAudioSpeech 语音合成回环转发（非流式二进制，缓冲响应原样回传，客户端超时 300s）。
// 上游响应（含错误状态）不做 JSON 解析，状态码 + Content-Type + 原始字节直接透传。
func (s *WebChatService) ProxyAudioSpeech(c *gin.Context, userID int64, reqBody WebChatAudioSpeechRequest) error {
	settings, err := s.GetWebChatSettings(c.Request.Context())
	if err != nil {
		return err
	}
	if err := s.checkModelAllowedWithSettings(c.Request.Context(), settings, userID, reqBody.Model, WebChatModelTypeTTS); err != nil {
		return err
	}
	apiKey, err := s.getOrCreateAPIKey(c.Request.Context(), userID, settings.WebChatDefaultModel)
	if err != nil {
		return err
	}

	payload, err := json.Marshal(map[string]any{
		"model": reqBody.Model,
		"input": reqBody.Input,
	})
	if err != nil {
		return fmt.Errorf("marshal web chat speech request: %w", err)
	}

	ctx, cancel := context.WithTimeout(c.Request.Context(), WebChatImageTimeout)
	defer cancel()
	httpReq, err := s.newLoopbackRequest(ctx, "/v1/audio/speech", payload, apiKey)
	if err != nil {
		return err
	}
	resp, err := s.httpClient.Do(httpReq)
	if err != nil {
		return infraerrors.New(http.StatusBadGateway, "WEB_CHAT_UPSTREAM_UNAVAILABLE", "Web chat upstream is unavailable")
	}
	defer func() { _ = resp.Body.Close() }()

	body, readErr := io.ReadAll(io.LimitReader(resp.Body, webChatBinaryResponseLimit))
	if readErr != nil {
		return infraerrors.New(http.StatusBadGateway, "WEB_CHAT_UPSTREAM_READ_FAILED", "Failed to read upstream response")
	}
	contentType := resp.Header.Get("Content-Type")
	if contentType == "" {
		contentType = "application/octet-stream"
	}
	c.Data(resp.StatusCode, contentType, body)
	return nil
}

// ProxyMusicGenerations 音乐生成回环转发（异步任务提交，202 任务 JSON 原样回传，客户端超时 300s）。
func (s *WebChatService) ProxyMusicGenerations(c *gin.Context, userID int64, reqBody WebChatMusicGenerationRequest) error {
	settings, err := s.GetWebChatSettings(c.Request.Context())
	if err != nil {
		return err
	}
	if err := s.checkModelAllowedWithSettings(c.Request.Context(), settings, userID, reqBody.Model, WebChatModelTypeMusic); err != nil {
		return err
	}
	apiKey, err := s.getOrCreateAPIKey(c.Request.Context(), userID, settings.WebChatDefaultModel)
	if err != nil {
		return err
	}

	payloadMap := map[string]any{
		"model":  reqBody.Model,
		"prompt": reqBody.Prompt,
	}
	if lyrics := strings.TrimSpace(reqBody.Lyrics); lyrics != "" {
		payloadMap["lyrics"] = lyrics
	}
	if reqBody.Instrumental {
		payloadMap["instrumental"] = true
	}
	if reqBody.Seconds > 0 {
		payloadMap["seconds"] = reqBody.Seconds
	}
	payload, err := json.Marshal(payloadMap)
	if err != nil {
		return fmt.Errorf("marshal web chat music request: %w", err)
	}

	ctx, cancel := context.WithTimeout(c.Request.Context(), WebChatImageTimeout)
	defer cancel()
	httpReq, err := s.newLoopbackRequest(ctx, "/v1/audio/music", payload, apiKey)
	if err != nil {
		return err
	}
	return webChatJSONPassthrough(c, s.httpClient, httpReq)
}

// ProxyMusicTaskGet 音乐任务轮询回环转发（GET，任务归属由网关按 API Key 双绑定校验）。
func (s *WebChatService) ProxyMusicTaskGet(c *gin.Context, userID int64, taskID string) error {
	settings, err := s.GetWebChatSettings(c.Request.Context())
	if err != nil {
		return err
	}
	apiKey, err := s.getOrCreateAPIKey(c.Request.Context(), userID, settings.WebChatDefaultModel)
	if err != nil {
		return err
	}

	ctx, cancel := context.WithTimeout(c.Request.Context(), WebChatImageTimeout)
	defer cancel()
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodGet, s.webChatLoopbackBaseURL()+"/v1/audio/music/tasks/"+url.PathEscape(taskID), nil)
	if err != nil {
		return fmt.Errorf("build loopback request: %w", err)
	}
	httpReq.Header.Set("Authorization", "Bearer "+apiKey)
	return webChatJSONPassthrough(c, s.httpClient, httpReq)
}

// webChatJSONPassthrough 缓冲上游 JSON 响应并原样透传状态码与响应体。
func webChatJSONPassthrough(c *gin.Context, client *http.Client, req *http.Request) error {
	resp, err := client.Do(req)
	if err != nil {
		return infraerrors.New(http.StatusBadGateway, "WEB_CHAT_UPSTREAM_UNAVAILABLE", "Web chat upstream is unavailable")
	}
	defer func() { _ = resp.Body.Close() }()

	body, readErr := io.ReadAll(io.LimitReader(resp.Body, webChatBinaryResponseLimit))
	if readErr != nil {
		return infraerrors.New(http.StatusBadGateway, "WEB_CHAT_UPSTREAM_READ_FAILED", "Failed to read upstream response")
	}
	contentType := resp.Header.Get("Content-Type")
	if contentType == "" {
		contentType = "application/json"
	}
	c.Data(resp.StatusCode, contentType, body)
	return nil
}
