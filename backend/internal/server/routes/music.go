package routes

import (
	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/Wei-Shaw/sub2api/internal/handler"
	"github.com/Wei-Shaw/sub2api/internal/server/middleware"
	"github.com/Wei-Shaw/sub2api/internal/service"

	"github.com/gin-gonic/gin"
)

// RegisterMusicRoutes 注册异步音乐生成网关路由（/v1/audio/music）。
//
// 与 RegisterGatewayRoutes 的 /v1 组共用同一条中间件链（bodyLimit →
// clientRequestID → opsErrorLogger → endpointNorm → apiKeyAuth →
// groupModelAllowlist），独立成组以避免触碰网关路由文件。任务轮询是 GET，
// 不携带模型名，因此组模型白名单对轮询请求天然放行。
func RegisterMusicRoutes(
	r *gin.Engine,
	h *handler.Handlers,
	apiKeyAuth middleware.APIKeyAuthMiddleware,
	opsService *service.OpsService,
	cfg *config.Config,
) {
	bodyLimit := middleware.RequestBodyLimit(cfg.Gateway.MaxBodySize)
	clientRequestID := middleware.ClientRequestID()
	opsErrorLogger := handler.OpsErrorLoggerMiddleware(opsService)
	endpointNorm := handler.InboundEndpointMiddleware()
	groupModelAllowlist := middleware.GroupModelAllowlist()

	music := r.Group("/v1")
	music.Use(bodyLimit)
	music.Use(clientRequestID)
	music.Use(opsErrorLogger)
	music.Use(endpointNorm)
	music.Use(gin.HandlerFunc(apiKeyAuth))
	music.Use(groupModelAllowlist)
	{
		music.POST("/audio/music", h.MusicTask.Submit)
		music.GET("/audio/music/tasks/:task_id", h.MusicTask.Get)
	}
}
