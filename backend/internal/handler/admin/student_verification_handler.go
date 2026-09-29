package admin

import (
	"context"
	"strconv"
	"strings"

	"github.com/Wei-Shaw/sub2api/internal/pkg/response"
	middleware2 "github.com/Wei-Shaw/sub2api/internal/server/middleware"
	"github.com/Wei-Shaw/sub2api/internal/service"

	"github.com/gin-gonic/gin"
)

// 学生认证记录管理端接口（挂 routes/admin.go 管理链，自动审计）：
//   - GET  /api/v1/admin/student-verifications?page=&user_id=&provider=&status=&email=
//   - GET  /api/v1/admin/student-verifications/:id
//   - POST /api/v1/admin/student-verifications/:id/revoke
//
// 注意：撤销认证记录只回写认证状态（HUBU_EMAIL_VERIFIED 失效），
// 不自动撤销已发放的订阅权益 —— 权益撤销走 subscription-grants/:id/revoke，
// 两个动作解耦，避免误伤用户付费权益。

// StudentVerificationHandler 学生认证记录管理端 handler
type StudentVerificationHandler struct {
	verificationService *service.StudentVerificationService
}

// NewStudentVerificationHandler 创建学生认证记录管理端 handler
func NewStudentVerificationHandler(verificationService *service.StudentVerificationService) *StudentVerificationHandler {
	return &StudentVerificationHandler{verificationService: verificationService}
}

// ListVerificationRecords 分页查询认证记录
// GET /api/v1/admin/student-verifications
func (h *StudentVerificationHandler) ListVerificationRecords(c *gin.Context) {
	page, pageSize := response.ParsePagination(c)
	if pageSize > 100 {
		pageSize = 100
	}
	filter := &service.StudentVerificationAdminFilter{
		Page:     page,
		PageSize: pageSize,
		Provider: strings.TrimSpace(c.Query("provider")),
		Status:   strings.TrimSpace(c.Query("status")),
		Email:    strings.TrimSpace(c.Query("email")),
	}
	if v := strings.TrimSpace(c.Query("user_id")); v != "" {
		id, err := strconv.ParseInt(v, 10, 64)
		if err != nil || id <= 0 {
			response.BadRequest(c, "Invalid user_id")
			return
		}
		filter.UserID = &id
	}
	result, err := h.verificationService.AdminListVerifications(c.Request.Context(), filter)
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	response.Success(c, result)
}

// GetVerification 认证记录详情
// GET /api/v1/admin/student-verifications/:id
func (h *StudentVerificationHandler) GetVerification(c *gin.Context) {
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil || id <= 0 {
		response.BadRequest(c, "Invalid verification ID")
		return
	}
	verification, err := h.verificationService.GetVerification(c.Request.Context(), id)
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	response.Success(c, verification)
}

// RevokeVerificationRequest 撤销认证请求
type RevokeVerificationRequest struct {
	Reason string `json:"reason" binding:"required,max=500"`
}

// Revoke 撤销认证记录（回写状态，不动权益台账）
// POST /api/v1/admin/student-verifications/:id/revoke
func (h *StudentVerificationHandler) Revoke(c *gin.Context) {
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil || id <= 0 {
		response.BadRequest(c, "Invalid verification ID")
		return
	}
	var req RevokeVerificationRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, "Invalid request")
		return
	}
	operatorID := int64(0)
	if subject, ok := middleware2.GetAuthSubjectFromContext(c); ok {
		operatorID = subject.UserID
	}
	payload := map[string]any{"verification_id": id, "reason": req.Reason}
	executeAdminIdempotentJSON(c, "admin:student-verification:revoke", payload, service.DefaultWriteIdempotencyTTL(), func(ctx context.Context) (any, error) {
		revoked, err := h.verificationService.AdminRevokeVerification(ctx, id, operatorID, req.Reason)
		if err != nil {
			return nil, err
		}
		return revoked, nil
	})
}
