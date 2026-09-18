package routes

import (
	"github.com/gin-gonic/gin"

	"github.com/Wei-Shaw/sub2api/internal/handler"
	"github.com/Wei-Shaw/sub2api/internal/server/middleware"
)

// RegisterHubuRoutes HUBU Harness: 桌面客户端一键连接授权路由（湖北大学，2026-09 本地开发）。
// - POST /api/v1/hubu/connect-code：需要网站登录态，签发 60s 一次性授权码
// - POST /api/v1/hubu/exchange：公开接口，靠一次性 code 本身授权（原子单次使用）
// 与 MUC 共用 CampusConnectHandler，仅路径段与 Redis/Key 前缀不同。
func RegisterHubuRoutes(
	v1 *gin.RouterGroup,
	h *handler.Handlers,
	jwtAuth middleware.JWTAuthMiddleware,
) {
	registerCampusRoutes(v1, jwtAuth, "hubu", h.CampusConnect.Hubu)
}
