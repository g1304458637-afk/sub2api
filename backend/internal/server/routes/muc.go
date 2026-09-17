package routes

import (
	"github.com/gin-gonic/gin"

	"github.com/Wei-Shaw/sub2api/internal/handler"
	"github.com/Wei-Shaw/sub2api/internal/server/middleware"
)

// RegisterMucRoutes MUC Harness: 桌面客户端一键连接授权路由。
// - POST /api/v1/muc/connect-code：需要网站登录态，签发 60s 一次性授权码
// - POST /api/v1/muc/exchange：公开接口，靠一次性 code 本身授权（原子单次使用）
func RegisterMucRoutes(
	v1 *gin.RouterGroup,
	h *handler.Handlers,
	jwtAuth middleware.JWTAuthMiddleware,
) {
	authenticated := v1.Group("")
	authenticated.Use(gin.HandlerFunc(jwtAuth))
	{
		authenticated.POST("/muc/connect-code", h.MucConnect.ConnectCode)
	}

	v1.POST("/muc/exchange", h.MucConnect.Exchange)
}
