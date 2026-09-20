package routes

import (
	"time"

	"github.com/Wei-Shaw/sub2api/internal/handler"
	ratelimit "github.com/Wei-Shaw/sub2api/internal/middleware"
	"github.com/Wei-Shaw/sub2api/internal/server/middleware"
	"github.com/Wei-Shaw/sub2api/internal/service"

	"github.com/gin-gonic/gin"
	"github.com/redis/go-redis/v9"
)

// RegisterMucRoutes MUC Harness: 桌面客户端一键连接授权路由。
// - POST /api/v1/muc/connect-code：需要网站登录态，签发 60s 一次性授权码
// - POST /api/v1/muc/exchange：公开接口，靠一次性 code 本身授权（原子单次使用）
//
// 中间件栈与 routes/user.go 对齐：后台模式限制 + 面板按用户限流 + 审计；
// 公开的 exchange 叠加按 IP 的兜底限流（Redis 故障时 fail-close）。
func RegisterMucRoutes(
	v1 *gin.RouterGroup,
	h *handler.Handlers,
	jwtAuth middleware.JWTAuthMiddleware,
	auditLog middleware.AuditLogMiddleware,
	redisClient *redis.Client,
	settingService *service.SettingService,
	panelRateLimiter *middleware.PanelRateLimiter,
) {
	rateLimiter := ratelimit.NewRateLimiter(redisClient)

	authenticated := v1.Group("")
	authenticated.Use(gin.HandlerFunc(jwtAuth))
	authenticated.Use(middleware.BackendModeUserGuard(settingService))
	authenticated.Use(panelRateLimiter.Global())
	authenticated.Use(gin.HandlerFunc(auditLog))
	{
		authenticated.POST("/muc/connect-code", h.MucConnect.ConnectCode)
	}

	v1.POST("/muc/exchange", rateLimiter.LimitWithOptions("muc-exchange", 10, time.Minute, ratelimit.RateLimitOptions{
		FailureMode: ratelimit.RateLimitFailClose,
	}), h.MucConnect.Exchange)
}
