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

// registerCampusRoutes 各品牌一键连接路由的共用注册逻辑。
// 路由段与 handler 的品牌前缀（Redis key / API Key 名）由注册方决定，
// 授权码机制（60s TTL、SHA-256 存储、GETDEL 单次使用）完全一致。
// 中间件栈与 routes/user.go 对齐：后台模式限制 + 面板按用户限流 + 审计；
// 公开的 exchange 叠加按 IP 的兜底限流（Redis 故障时 fail-close，随主分支加固引入）。
func registerCampusRoutes(
	v1 *gin.RouterGroup,
	jwtAuth middleware.JWTAuthMiddleware,
	auditLog middleware.AuditLogMiddleware,
	redisClient *redis.Client,
	settingService *service.SettingService,
	panelRateLimiter *middleware.PanelRateLimiter,
	pathSegment string,
	h *handler.CampusConnectHandler,
) {
	rateLimiter := ratelimit.NewRateLimiter(redisClient)

	authenticated := v1.Group("")
	authenticated.Use(gin.HandlerFunc(jwtAuth))
	authenticated.Use(middleware.BackendModeUserGuard(settingService))
	authenticated.Use(panelRateLimiter.Global())
	authenticated.Use(gin.HandlerFunc(auditLog))
	{
		authenticated.POST("/"+pathSegment+"/connect-code", h.ConnectCode)
	}

	v1.POST("/"+pathSegment+"/exchange", rateLimiter.LimitWithOptions(pathSegment+"-exchange", 10, time.Minute, ratelimit.RateLimitOptions{
		FailureMode: ratelimit.RateLimitFailClose,
	}), h.Exchange)
}
