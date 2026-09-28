package routes

import (
	"strings"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/handler"
	ratelimit "github.com/Wei-Shaw/sub2api/internal/middleware"
	"github.com/Wei-Shaw/sub2api/internal/pkg/campus"
	"github.com/Wei-Shaw/sub2api/internal/server/middleware"
	"github.com/Wei-Shaw/sub2api/internal/service"

	"github.com/gin-gonic/gin"
	"github.com/redis/go-redis/v9"
)

// RegisterStudentVerificationRoutes 学生邮箱认证（Campus Identity）路由。
// 挂在品牌路径段下：HUBU 部署 = /api/v1/hubu/student-verification/*。
// 未配置学生邮箱域名的品牌（EducationDomain 为空）自动跳过注册（fail-closed）。
//
// 中间件栈与 routes/user.go 对齐：JWT + BackendModeUserGuard + 面板按用户限流 + 审计；
// send-code 叠加按 IP 的 fail-close 限流（验证码接口防刷、防枚举）。
func RegisterStudentVerificationRoutes(
	v1 *gin.RouterGroup,
	h *handler.Handlers,
	jwtAuth middleware.JWTAuthMiddleware,
	auditLog middleware.AuditLogMiddleware,
	redisClient *redis.Client,
	settingService *service.SettingService,
	panelRateLimiter *middleware.PanelRateLimiter,
) {
	brand := campus.Current()
	if h.StudentVerification == nil {
		return
	}
	if strings.TrimSpace(brand.EducationDomain) == "" {
		// 品牌未配置学生邮箱域：不注册路由，避免暴露不可用功能。
		return
	}

	rateLimiter := ratelimit.NewRateLimiter(redisClient)

	group := v1.Group("/" + brand.PathSegment + "/student-verification")
	group.Use(gin.HandlerFunc(jwtAuth))
	group.Use(middleware.BackendModeUserGuard(settingService))
	group.Use(panelRateLimiter.Global())
	group.Use(gin.HandlerFunc(auditLog))
	{
		// 验证码发送：IP 级 fail-close（Redis 故障拒绝而非放行）+ 用户级频控在服务层。
		group.POST("/send-code", rateLimiter.LimitWithOptions(brand.ID+"-student-verification-send", 5, time.Minute, ratelimit.RateLimitOptions{
			FailureMode: ratelimit.RateLimitFailClose,
		}), h.StudentVerification.SendCode)
		// 验证提交：服务端幂等（IdempotencyCoordinator）+ OTP 单次消费。
		group.POST("/verify", panelRateLimiter.Heavy(), h.StudentVerification.Verify)
		group.GET("/status", h.StudentVerification.Status)
	}
}
