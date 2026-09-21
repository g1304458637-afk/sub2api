package handler

import (
	"strings"

	"github.com/Wei-Shaw/sub2api/internal/pkg/response"
	"github.com/Wei-Shaw/sub2api/internal/server/middleware"
	"github.com/Wei-Shaw/sub2api/internal/service"

	"github.com/gin-gonic/gin"
)

// WebChatHandler 网页聊天/网页绘图面板代理 handler。
// 路由挂在 JWT 面板鉴权链下（见 routes/web_chat.go），代理转发与白名单
// 由 service.WebChatService 实现。
type WebChatHandler struct {
	webChatService *service.WebChatService
}

// NewWebChatHandler 创建网页聊天 handler
func NewWebChatHandler(webChatService *service.WebChatService) *WebChatHandler {
	return &WebChatHandler{webChatService: webChatService}
}

// webChatUserID 从 JWT 面板上下文取当前用户 ID
func webChatUserID(c *gin.Context) (int64, bool) {
	subject, ok := middleware.GetAuthSubjectFromContext(c)
	if !ok || subject.UserID <= 0 {
		return 0, false
	}
	return subject.UserID, true
}

// respondWebChatError 流开始前的本地错误统一转面板错误形状 {code, message}
func respondWebChatError(c *gin.Context, err error) {
	response.ErrorFrom(c, err)
}

// GetConfig 获取网页聊天配置
// GET /api/v1/web-chat/config
func (h *WebChatHandler) GetConfig(c *gin.Context) {
	userID, ok := webChatUserID(c)
	if !ok {
		response.Unauthorized(c, "User not authenticated")
		return
	}
	cfg, err := h.webChatService.GetConfig(c.Request.Context(), userID)
	if err != nil {
		respondWebChatError(c, err)
		return
	}
	response.Success(c, cfg)
}

// webChatChatRequest POST /api/v1/web-chat/chat/completions 请求体
type webChatChatRequest struct {
	Model    string                   `json:"model"`
	Messages []service.WebChatMessage `json:"messages"`
}

// ChatCompletions 网页聊天代理（OpenAI chat.completion.chunk SSE 流式回传）
// POST /api/v1/web-chat/chat/completions
func (h *WebChatHandler) ChatCompletions(c *gin.Context) {
	userID, ok := webChatUserID(c)
	if !ok {
		response.Unauthorized(c, "User not authenticated")
		return
	}
	var req webChatChatRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, "Invalid request: "+err.Error())
		return
	}
	req.Model = strings.TrimSpace(req.Model)
	if req.Model == "" {
		response.BadRequest(c, "model is required")
		return
	}
	if len(req.Messages) == 0 {
		response.BadRequest(c, "messages is required")
		return
	}
	for _, m := range req.Messages {
		if strings.TrimSpace(m.Role) == "" || len(m.Content) == 0 {
			response.BadRequest(c, "each message requires role and content")
			return
		}
	}
	if err := h.webChatService.ProxyChatCompletions(c, userID, req.Model, req.Messages); err != nil {
		respondWebChatError(c, err)
	}
}

// ImagesGenerations 网页绘图代理（OpenAI images 格式）
// POST /api/v1/web-chat/images/generations
func (h *WebChatHandler) ImagesGenerations(c *gin.Context) {
	userID, ok := webChatUserID(c)
	if !ok {
		response.Unauthorized(c, "User not authenticated")
		return
	}
	var req service.WebChatImageGenerationRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, "Invalid request: "+err.Error())
		return
	}
	req.Model = strings.TrimSpace(req.Model)
	if req.Model == "" {
		response.BadRequest(c, "model is required")
		return
	}
	if strings.TrimSpace(req.Prompt) == "" {
		response.BadRequest(c, "prompt is required")
		return
	}
	if req.N < 0 {
		req.N = 0
	}
	if err := h.webChatService.ProxyImagesGenerations(c, userID, req); err != nil {
		respondWebChatError(c, err)
	}
}

// AdminAvailableModels 管理端：所有分组可用模型 ID 并集
// GET /api/v1/admin/web-chat/available-models
func (h *WebChatHandler) AdminAvailableModels(c *gin.Context) {
	models, err := h.webChatService.AdminAvailableModels(c.Request.Context())
	if err != nil {
		respondWebChatError(c, err)
		return
	}
	response.Success(c, gin.H{"models": models})
}
