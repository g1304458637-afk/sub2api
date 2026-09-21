package routes

import (
	"github.com/Wei-Shaw/sub2api/internal/handler"
	"github.com/Wei-Shaw/sub2api/internal/server/middleware"
	"github.com/Wei-Shaw/sub2api/internal/service"

	"github.com/gin-gonic/gin"
)

// RegisterWebChatRoutes 注册网页聊天/网页绘图路由（面板 JWT 鉴权，中间件链与
// routes/user.go 保持一致：JWT + BackendModeUserGuard + 面板限流 + 审计）。
func RegisterWebChatRoutes(
	v1 *gin.RouterGroup,
	h *handler.Handlers,
	jwtAuth middleware.JWTAuthMiddleware,
	auditLog middleware.AuditLogMiddleware,
	settingService *service.SettingService,
	panelRateLimiter *middleware.PanelRateLimiter,
) {
	authenticated := v1.Group("")
	authenticated.Use(gin.HandlerFunc(jwtAuth))
	authenticated.Use(middleware.BackendModeUserGuard(settingService))
	// 面板全局按用户限流：聊天/绘图代理回环进入网关前先挡住高频请求
	authenticated.Use(panelRateLimiter.Global())
	// 与用户管理面一致，变更/代理调用入审计
	authenticated.Use(gin.HandlerFunc(auditLog))
	{
		webChat := authenticated.Group("/web-chat")
		{
			webChat.GET("/config", h.WebChat.GetConfig)
			webChat.POST("/chat/completions", h.WebChat.ChatCompletions)
			webChat.POST("/images/generations", h.WebChat.ImagesGenerations)
			webChat.POST("/images/edits", h.WebChat.ImagesEdits)
			webChat.POST("/audio/speech", h.WebChat.AudioSpeech)
			webChat.POST("/music/generations", h.WebChat.MusicGenerations)
			webChat.GET("/music/tasks/:task_id", h.WebChat.MusicTaskGet)
		}
	}
}
