package admin

import (
	"context"
	"fmt"
	"strconv"
	"strings"

	"github.com/Wei-Shaw/sub2api/internal/domain"
	infraerrors "github.com/Wei-Shaw/sub2api/internal/pkg/errors"
	"github.com/Wei-Shaw/sub2api/internal/pkg/response"
	middleware2 "github.com/Wei-Shaw/sub2api/internal/server/middleware"
	"github.com/Wei-Shaw/sub2api/internal/service"

	"github.com/gin-gonic/gin"
)

// 订阅权益发放（Subscription Grant）管理端接口：
//   - POST /api/v1/admin/subscription-grants/preview   发放预览（无副作用）
//   - POST /api/v1/admin/subscription-grants           单用户赠送（Idempotency-Key 必带）
//   - POST /api/v1/admin/subscription-grants/bulk      批量赠送（每用户独立结果）
//   - GET  /api/v1/admin/subscription-grants           台账列表（user/source/status/benefit 过滤）
//   - POST /api/v1/admin/subscription-grants/:id/revoke 撤销（受付费地板保护）
//
// 全部挂在 admin 组：AuditLogMiddleware 自动审计 + step-up/admin-compliance 生效。
// 幂等：写入端点统一走 executeAdminIdempotentJSON（IdempotencyCoordinator），
// 并以 Idempotency-Key 派生台账级 source_key（business 幂等，超出协调器 TTL 依然安全）。

// SubscriptionGrantHandler 订阅权益发放管理端 handler
type SubscriptionGrantHandler struct {
	grantService *service.SubscriptionGrantService
}

// NewSubscriptionGrantHandler 创建订阅权益发放管理端 handler
func NewSubscriptionGrantHandler(grantService *service.SubscriptionGrantService) *SubscriptionGrantHandler {
	return &SubscriptionGrantHandler{grantService: grantService}
}

// CreateSubscriptionGrantRequest 单用户赠送请求
type CreateSubscriptionGrantRequest struct {
	UserID          int64  `json:"user_id" binding:"required,gt=0"`
	GroupID         int64  `json:"group_id" binding:"required,gt=0"`
	PlanID          *int64 `json:"plan_id"`
	DurationDays    int    `json:"duration_days" binding:"required,gt=0,lte=36500"`
	EffectivePolicy string `json:"effective_policy" binding:"required,oneof=immediate end_of_term"`
	Source          string `json:"source" binding:"required"`
	Reason          string `json:"reason"`
	Notes           string `json:"notes"`
}

// BulkCreateSubscriptionGrantsRequest 批量赠送请求
type BulkCreateSubscriptionGrantsRequest struct {
	UserIDs         []int64 `json:"user_ids" binding:"required,min=1,max=100,dive,gt=0"`
	GroupID         int64   `json:"group_id" binding:"required,gt=0"`
	PlanID          *int64  `json:"plan_id"`
	DurationDays    int     `json:"duration_days" binding:"required,gt=0,lte=36500"`
	EffectivePolicy string  `json:"effective_policy" binding:"required,oneof=immediate end_of_term"`
	Source          string  `json:"source" binding:"required"`
	Reason          string  `json:"reason"`
	Notes           string  `json:"notes"`
}

// RevokeSubscriptionGrantRequest 撤销请求
type RevokeSubscriptionGrantRequest struct {
	Reason string `json:"reason" binding:"required,max=500"`
}

// adminValidGrantSources 管理端允许指定的来源（不包含系统自动来源）。
var adminValidGrantSources = map[string]bool{
	domain.SubscriptionGrantSourceAdminGrant:          true,
	domain.SubscriptionGrantSourceCompensation:        true,
	domain.SubscriptionGrantSourceCampaign:            true,
	domain.SubscriptionGrantSourceSchoolBulk:          true,
	domain.SubscriptionGrantSourceTeacherVerification: true,
	domain.SubscriptionGrantSourceInvitation:          true,
	domain.SubscriptionGrantSourceInternal:            true,
	domain.SubscriptionGrantSourceOther:               true,
}

// Preview 发放预览：管理员确认前看到本次操作将如何改变用户权益
// GET/POST /api/v1/admin/subscription-grants/preview
func (h *SubscriptionGrantHandler) Preview(c *gin.Context) {
	var req CreateSubscriptionGrantRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, "Invalid request: "+err.Error())
		return
	}
	if !adminValidGrantSources[req.Source] {
		response.BadRequest(c, "Invalid source")
		return
	}
	preview, err := h.grantService.PreviewGrant(c.Request.Context(), &service.CreateSubscriptionGrantCommand{
		UserID:          req.UserID,
		GroupID:         req.GroupID,
		PlanID:          req.PlanID,
		Source:          req.Source,
		EffectivePolicy: req.EffectivePolicy,
		DurationDays:    req.DurationDays,
	})
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	response.Success(c, preview)
}

// Create 单用户赠送
// POST /api/v1/admin/subscription-grants
func (h *SubscriptionGrantHandler) Create(c *gin.Context) {
	var req CreateSubscriptionGrantRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, "Invalid request: "+err.Error())
		return
	}
	if !adminValidGrantSources[req.Source] {
		response.BadRequest(c, "Invalid source")
		return
	}

	payload := map[string]any{
		"user_id":          req.UserID,
		"group_id":         req.GroupID,
		"duration_days":    req.DurationDays,
		"effective_policy": req.EffectivePolicy,
		"source":           req.Source,
	}

	operatorID := currentAdminID(c)
	cmd := &service.CreateSubscriptionGrantCommand{
		UserID:          req.UserID,
		GroupID:         req.GroupID,
		PlanID:          req.PlanID,
		Source:          req.Source,
		EffectivePolicy: req.EffectivePolicy,
		DurationDays:    req.DurationDays,
		Reason:          req.Reason,
		Notes:           req.Notes,
		OperatorUserID:  &operatorID,
		// 台账级幂等：同一 Idempotency-Key 重试（即使超出协调器 TTL）不会重复发放。
		SourceKey:      fmt.Sprintf("admin:%s:%d", idempotencyKeyOf(c), req.UserID),
		IdempotencyKey: idempotencyKeyOf(c),
	}

	executeAdminIdempotentJSON(c, "admin:subscription-grant:create", payload, service.DefaultWriteIdempotencyTTL(), func(ctx context.Context) (any, error) {
		execution, err := h.grantService.CreateGrant(ctx, cmd)
		if err != nil {
			return nil, err
		}
		return grantExecutionResponse(execution), nil
	})
}

// Bulk 批量赠送：每个用户独立结果，失败用户明确报错，重试不重复发放。
// POST /api/v1/admin/subscription-grants/bulk
func (h *SubscriptionGrantHandler) Bulk(c *gin.Context) {
	var req BulkCreateSubscriptionGrantsRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, "Invalid request: "+err.Error())
		return
	}
	if !adminValidGrantSources[req.Source] {
		response.BadRequest(c, "Invalid source")
		return
	}

	operatorID := currentAdminID(c)
	idemKey := idempotencyKeyOf(c)

	payload := map[string]any{
		"user_count":       len(req.UserIDs),
		"group_id":         req.GroupID,
		"duration_days":    req.DurationDays,
		"effective_policy": req.EffectivePolicy,
		"source":           req.Source,
	}

	executeAdminIdempotentJSON(c, "admin:subscription-grant:bulk", payload, service.DefaultWriteIdempotencyTTL(), func(ctx context.Context) (any, error) {
		type bulkItem struct {
			UserID    int64   `json:"user_id"`
			Success   bool    `json:"success"`
			Action    string  `json:"action,omitempty"`
			GrantID   int64   `json:"grant_id,omitempty"`
			ExpiresAt *string `json:"expires_at,omitempty"`
			Error     string  `json:"error,omitempty"`
		}
		items := make([]bulkItem, 0, len(req.UserIDs))
		successCount := 0
		for _, userID := range req.UserIDs {
			cmd := &service.CreateSubscriptionGrantCommand{
				UserID:          userID,
				GroupID:         req.GroupID,
				PlanID:          req.PlanID,
				Source:          req.Source,
				EffectivePolicy: req.EffectivePolicy,
				DurationDays:    req.DurationDays,
				Reason:          req.Reason,
				Notes:           req.Notes,
				OperatorUserID:  &operatorID,
				SourceKey:       fmt.Sprintf("admin-bulk:%s:%d", idemKey, userID),
				IdempotencyKey:  idemKey,
			}
			execution, err := h.grantService.CreateGrant(ctx, cmd)
			if err != nil {
				items = append(items, bulkItem{UserID: userID, Success: false, Error: infraerrors.Message(err)})
				continue
			}
			successCount++
			item := bulkItem{
				UserID:  userID,
				Success: true,
				Action:  execution.Outcome.Action,
				GrantID: execution.Grant.ID,
			}
			if execution.Outcome.ExpiresAt != nil {
				v := execution.Outcome.ExpiresAt.UTC().Format(timeRFC3339)
				item.ExpiresAt = &v
			}
			items = append(items, item)
		}
		return gin.H{
			"items":         items,
			"total":         len(req.UserIDs),
			"success_count": successCount,
			"failed_count":  len(req.UserIDs) - successCount,
		}, nil
	})
}

// List 台账分页查询
// GET /api/v1/admin/subscription-grants?page=&page_size=&user_id=&source=&status=&benefit_code=&group_id=
func (h *SubscriptionGrantHandler) List(c *gin.Context) {
	page, pageSize := response.ParsePagination(c)
	if pageSize > 100 {
		pageSize = 100
	}
	filter := &service.GrantAdminFilter{
		Page:        page,
		PageSize:    pageSize,
		Source:      strings.TrimSpace(c.Query("source")),
		Status:      strings.TrimSpace(c.Query("status")),
		BenefitCode: strings.TrimSpace(c.Query("benefit_code")),
	}
	if v := strings.TrimSpace(c.Query("user_id")); v != "" {
		id, err := strconv.ParseInt(v, 10, 64)
		if err != nil || id <= 0 {
			response.BadRequest(c, "Invalid user_id")
			return
		}
		filter.UserID = &id
	}
	if v := strings.TrimSpace(c.Query("group_id")); v != "" {
		id, err := strconv.ParseInt(v, 10, 64)
		if err != nil || id <= 0 {
			response.BadRequest(c, "Invalid group_id")
			return
		}
		filter.GroupID = &id
	}

	result, err := h.grantService.AdminListGrants(c.Request.Context(), filter)
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	response.Success(c, result)
}

// Revoke 撤销赠送
// POST /api/v1/admin/subscription-grants/:id/revoke
func (h *SubscriptionGrantHandler) Revoke(c *gin.Context) {
	grantID, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil || grantID <= 0 {
		response.BadRequest(c, "Invalid grant ID")
		return
	}
	var req RevokeSubscriptionGrantRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, "Invalid request: "+err.Error())
		return
	}
	operatorID := currentAdminID(c)

	payload := map[string]any{"grant_id": grantID, "reason": req.Reason}
	executeAdminIdempotentJSON(c, "admin:subscription-grant:revoke", payload, service.DefaultWriteIdempotencyTTL(), func(ctx context.Context) (any, error) {
		grant, err := h.grantService.RevokeGrant(ctx, grantID, operatorID, req.Reason)
		if err != nil {
			return nil, err
		}
		return grant, nil
	})
}

// currentAdminID 提取当前管理员 ID（admin 组中间件保证已认证）。
func currentAdminID(c *gin.Context) int64 {
	if subject, ok := middleware2.GetAuthSubjectFromContext(c); ok {
		return subject.UserID
	}
	return 0
}

func idempotencyKeyOf(c *gin.Context) string {
	return strings.TrimSpace(c.GetHeader("Idempotency-Key"))
}

const timeRFC3339 = "2006-01-02T15:04:05Z07:00"

func grantExecutionResponse(execution *service.SubscriptionGrantExecution) gin.H {
	outcome := execution.Outcome
	outcomeMap := gin.H{"action": outcome.Action, "grant_id": outcome.GrantID, "message": outcome.Message}
	if outcome.SubscriptionID > 0 {
		outcomeMap["subscription_id"] = outcome.SubscriptionID
	}
	if outcome.PreviousExpires != nil {
		outcomeMap["previous_expires"] = outcome.PreviousExpires.UTC().Format(timeRFC3339)
	}
	if outcome.ExpiresAt != nil {
		outcomeMap["expires_at"] = outcome.ExpiresAt.UTC().Format(timeRFC3339)
	}
	return gin.H{
		"grant":   execution.Grant,
		"outcome": outcomeMap,
	}
}
