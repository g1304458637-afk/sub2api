package handler

import (
	"context"
	"strings"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/pkg/response"
	middleware2 "github.com/Wei-Shaw/sub2api/internal/server/middleware"
	"github.com/Wei-Shaw/sub2api/internal/service"

	"github.com/gin-gonic/gin"
)

// StudentVerificationHandler 学生邮箱认证（Campus Identity）用户端接口。
//
// 路由挂在品牌路径下（/api/v1/{brand.PathSegment}/student-verification/*），
// 中间件栈与 muc connect-code 一致：JWT + BackendModeUserGuard + 面板限流 + 审计；
// send-code 叠加按 IP 的 fail-close 限流。
//
// 防枚举：send-code 对「邮箱域不合法 / 频控」之外的失败一律返回通用成功语义，
// 不区分邮箱是否存在。
type StudentVerificationHandler struct {
	verificationService *service.StudentVerificationService
}

// NewStudentVerificationHandler 创建学生认证 handler
func NewStudentVerificationHandler(verificationService *service.StudentVerificationService) *StudentVerificationHandler {
	return &StudentVerificationHandler{verificationService: verificationService}
}

// SendCodeRequest 发送验证码请求
type SendCodeRequest struct {
	Email string `json:"email" binding:"required,max=254"`
}

// VerifyRequest 提交验证码请求
type VerifyRequest struct {
	Email string `json:"email" binding:"required,max=254"`
	Code  string `json:"code" binding:"required,max=10"`
}

// SendCode 发送学生邮箱验证码
// POST /api/v1/{brand}/student-verification/send-code
func (h *StudentVerificationHandler) SendCode(c *gin.Context) {
	var req SendCodeRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, "Invalid request")
		return
	}
	userID := currentAuthUserID(c)
	if userID <= 0 {
		response.Unauthorized(c, "authentication required")
		return
	}
	if err := h.verificationService.SendVerificationCode(c.Request.Context(), userID, strings.TrimSpace(req.Email)); err != nil {
		response.ErrorFrom(c, err)
		return
	}
	response.Success(c, gin.H{"sent": true})
}

// Verify 提交验证码：认证 + 自动发放学生权益（一步完成，无需再点领取）
// POST /api/v1/{brand}/student-verification/verify
func (h *StudentVerificationHandler) Verify(c *gin.Context) {
	var req VerifyRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, "Invalid request")
		return
	}
	userID := currentAuthUserID(c)
	if userID <= 0 {
		response.Unauthorized(c, "authentication required")
		return
	}

	payload := map[string]any{"email": strings.ToLower(strings.TrimSpace(req.Email))}
	executeUserIdempotentJSON(c, "student-verification:verify", payload, service.DefaultWriteIdempotencyTTL(), func(ctx context.Context) (any, error) {
		result, err := h.verificationService.VerifyEmail(
			ctx, userID,
			strings.TrimSpace(req.Email), strings.TrimSpace(req.Code),
			c.ClientIP(), c.Request.UserAgent(),
		)
		if err != nil {
			return nil, err
		}
		return gin.H{
			"verified":      true,
			"email":         result.Verification.Email,
			"verified_at":   result.Verification.VerifiedAt,
			"grant_status":  result.Grant.Status,
			"outcome":       result.Outcome,
			"benefit_code":  derefString(result.Grant.BenefitCode),
			"duration_days": result.Grant.DurationDays,
			"expires_at":    timeStringUTC(result.Grant.ContributionEnd),
			"activated_at":  timeStringUTC(result.Grant.ActivatedAt),
			"message":       result.Outcome.Message,
		}, nil
	})
}

// Status 认证状态（个人资料卡片轮询/回显）
// GET /api/v1/{brand}/student-verification/status
func (h *StudentVerificationHandler) Status(c *gin.Context) {
	userID := currentAuthUserID(c)
	if userID <= 0 {
		response.Unauthorized(c, "authentication required")
		return
	}
	status, err := h.verificationService.GetStatus(c.Request.Context(), userID)
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	response.Success(c, status)
}

// currentAuthUserID 提取当前认证用户 ID。
func currentAuthUserID(c *gin.Context) int64 {
	if subject, ok := middleware2.GetAuthSubjectFromContext(c); ok {
		return subject.UserID
	}
	return 0
}

func derefString(s *string) string {
	if s == nil {
		return ""
	}
	return *s
}

const timeRFC3339 = "2006-01-02T15:04:05Z07:00"

func timeStringUTC(t *time.Time) *string {
	if t == nil {
		return nil
	}
	v := t.UTC().Format(timeRFC3339)
	return &v
}
