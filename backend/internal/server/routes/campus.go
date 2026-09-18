package routes

import (
	"github.com/gin-gonic/gin"

	"github.com/Wei-Shaw/sub2api/internal/handler"
	"github.com/Wei-Shaw/sub2api/internal/server/middleware"
)

// registerCampusRoutes 各品牌一键连接路由的共用注册逻辑。
// 路由段与 handler 的品牌前缀（Redis key / API Key 名）由注册方决定，
// 授权码机制（60s TTL、SHA-256 存储、GETDEL 单次使用）完全一致。
func registerCampusRoutes(
	v1 *gin.RouterGroup,
	jwtAuth middleware.JWTAuthMiddleware,
	pathSegment string,
	h *handler.CampusConnectHandler,
) {
	authenticated := v1.Group("")
	authenticated.Use(gin.HandlerFunc(jwtAuth))
	{
		authenticated.POST("/"+pathSegment+"/connect-code", h.ConnectCode)
	}

	v1.POST("/"+pathSegment+"/exchange", h.Exchange)
}
