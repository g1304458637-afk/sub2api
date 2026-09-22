package handler

import (
	"strconv"
	"strings"

	"github.com/Wei-Shaw/sub2api/internal/handler/dto"
	infraerrors "github.com/Wei-Shaw/sub2api/internal/pkg/errors"
	"github.com/Wei-Shaw/sub2api/internal/pkg/response"
	middleware2 "github.com/Wei-Shaw/sub2api/internal/server/middleware"
	"github.com/Wei-Shaw/sub2api/internal/service"

	"github.com/gin-gonic/gin"
)

// SubscriptionSummaryItem represents a subscription item in summary
type SubscriptionSummaryItem struct {
	ID              int64   `json:"id"`
	GroupID         int64   `json:"group_id"`
	GroupName       string  `json:"group_name"`
	Status          string  `json:"status"`
	DailyUsedUSD    float64 `json:"daily_used_usd,omitempty"`
	DailyLimitUSD   float64 `json:"daily_limit_usd,omitempty"`
	WeeklyUsedUSD   float64 `json:"weekly_used_usd,omitempty"`
	WeeklyLimitUSD  float64 `json:"weekly_limit_usd,omitempty"`
	MonthlyUsedUSD  float64 `json:"monthly_used_usd,omitempty"`
	MonthlyLimitUSD float64 `json:"monthly_limit_usd,omitempty"`
	ExpiresAt       *string `json:"expires_at,omitempty"`
}

// SubscriptionProgressInfo represents subscription with progress info
type SubscriptionProgressInfo struct {
	Subscription *dto.UserSubscription         `json:"subscription"`
	Progress     *service.SubscriptionProgress `json:"progress"`
}

// SubscriptionHandler handles user subscription operations
type SubscriptionHandler struct {
	subscriptionService *service.SubscriptionService
	accountStatus       *service.AccountStatusService
	resetCards          *service.ResetCardService
}

// NewSubscriptionHandler creates a new user subscription handler
func NewSubscriptionHandler(subscriptionService *service.SubscriptionService, accountStatus *service.AccountStatusService, resetCards *service.ResetCardService) *SubscriptionHandler {
	return &SubscriptionHandler{
		subscriptionService: subscriptionService,
		accountStatus:       accountStatus,
		resetCards:          resetCards,
	}
}

// UpdatePaygFallbackRequest PATCH /api/v1/subscriptions/:id/payg-fallback
type UpdatePaygFallbackRequest struct {
	Enabled *bool `json:"enabled" binding:"required"`
}

// UpdatePaygFallback 切换用户级 PAYG fallback（仅归属校验；Runtime 后续接线）。
func (h *SubscriptionHandler) UpdatePaygFallback(c *gin.Context) {
	subject, ok := middleware2.GetAuthSubjectFromContext(c)
	if !ok {
		response.Unauthorized(c, "User not found in context")
		return
	}
	subscriptionID, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil || subscriptionID <= 0 {
		response.BadRequest(c, "Invalid subscription id")
		return
	}
	var req UpdatePaygFallbackRequest
	if err := c.ShouldBindJSON(&req); err != nil || req.Enabled == nil {
		response.BadRequest(c, "enabled is required")
		return
	}
	if err := h.subscriptionService.UpdatePaygFallback(c.Request.Context(), subject.UserID, subscriptionID, *req.Enabled); err != nil {
		response.ErrorFrom(c, err)
		return
	}
	response.Success(c, gin.H{"subscription_id": subscriptionID, "payg_fallback": *req.Enabled})
}

// ResetWithCard 用户消费一张可用 Reset Card 重置指定订阅的周周期。
// 幂等：必须携带 Idempotency-Key（双击/重试/超时重放只消费一张卡）。
// POST /api/v1/subscriptions/:id/reset-with-card
func (h *SubscriptionHandler) ResetWithCard(c *gin.Context) {
	subject, ok := middleware2.GetAuthSubjectFromContext(c)
	if !ok {
		response.Unauthorized(c, "User not found in context")
		return
	}
	subscriptionID, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil || subscriptionID <= 0 {
		response.BadRequest(c, "Invalid subscription id")
		return
	}
	if h.resetCards == nil {
		response.ErrorFrom(c, infraerrors.InternalServer("RESET_CARD_UNAVAILABLE", "reset card service is unavailable"))
		return
	}
	result, err := h.resetCards.ConsumeForSubscription(
		c.Request.Context(), subject.UserID, subscriptionID,
		c.GetHeader("Idempotency-Key"), resetContractVersion(c.GetHeader("X-Quota-Contract")),
	)
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	response.Success(c, gin.H{
		"subscription_id":       result.SubscriptionID,
		"weekly_period_ends_at": result.WeeklyPeriodEndsAt,
		"short_period_ends_at":  result.ShortPeriodEndsAt,
		"quota_policy":          result.QuotaPolicy,
	})
}

// GetStatus 返回统一账户状态：Wallet + 全部 active subscriptions（净化视图，
// 无任何内部 USD 额度/倍率；百分比与状态由服务端权威计算）。
// GET /api/v1/subscriptions/status
func (h *SubscriptionHandler) GetStatus(c *gin.Context) {
	subject, ok := middleware2.GetAuthSubjectFromContext(c)
	if !ok {
		response.Unauthorized(c, "User not found in context")
		return
	}
	if h.accountStatus == nil {
		response.ErrorFrom(c, infraerrors.InternalServer("ACCOUNT_STATUS_UNAVAILABLE", "account status service is unavailable"))
		return
	}
	status, err := h.accountStatus.GetAccountStatus(c.Request.Context(), subject.UserID)
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	response.Success(c, status)
}

// List handles listing current user's subscriptions
// GET /api/v1/subscriptions
func (h *SubscriptionHandler) List(c *gin.Context) {
	subject, ok := middleware2.GetAuthSubjectFromContext(c)
	if !ok {
		response.Unauthorized(c, "User not found in context")
		return
	}

	subscriptions, err := h.subscriptionService.ListUserSubscriptions(c.Request.Context(), subject.UserID)
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}

	out := make([]dto.UserSubscription, 0, len(subscriptions))
	for i := range subscriptions {
		out = append(out, *dto.UserSubscriptionFromService(&subscriptions[i]))
	}
	response.Success(c, out)
}

// GetActive handles getting current user's active subscriptions
// GET /api/v1/subscriptions/active
func (h *SubscriptionHandler) GetActive(c *gin.Context) {
	subject, ok := middleware2.GetAuthSubjectFromContext(c)
	if !ok {
		response.Unauthorized(c, "User not found in context")
		return
	}

	subscriptions, err := h.subscriptionService.ListActiveUserSubscriptions(c.Request.Context(), subject.UserID)
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}

	out := make([]dto.UserSubscription, 0, len(subscriptions))
	for i := range subscriptions {
		out = append(out, *dto.UserSubscriptionFromService(&subscriptions[i]))
	}
	response.Success(c, out)
}

// GetProgress handles getting subscription progress for current user
// GET /api/v1/subscriptions/progress
func (h *SubscriptionHandler) GetProgress(c *gin.Context) {
	subject, ok := middleware2.GetAuthSubjectFromContext(c)
	if !ok {
		response.Unauthorized(c, "User not found in context")
		return
	}

	// Get all active subscriptions with progress
	subscriptions, err := h.subscriptionService.ListActiveUserSubscriptions(c.Request.Context(), subject.UserID)
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}

	result := make([]SubscriptionProgressInfo, 0, len(subscriptions))
	for i := range subscriptions {
		sub := &subscriptions[i]
		progress, err := h.subscriptionService.GetSubscriptionProgress(c.Request.Context(), sub.ID)
		if err != nil {
			// Skip subscriptions with errors
			continue
		}
		result = append(result, SubscriptionProgressInfo{
			Subscription: dto.UserSubscriptionFromService(sub),
			Progress:     progress,
		})
	}

	response.Success(c, result)
}

// GetSummary handles getting a summary of current user's subscription status
// GET /api/v1/subscriptions/summary
func (h *SubscriptionHandler) GetSummary(c *gin.Context) {
	subject, ok := middleware2.GetAuthSubjectFromContext(c)
	if !ok {
		response.Unauthorized(c, "User not found in context")
		return
	}

	// Get all active subscriptions
	subscriptions, err := h.subscriptionService.ListActiveUserSubscriptions(c.Request.Context(), subject.UserID)
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}

	var totalUsed float64
	items := make([]SubscriptionSummaryItem, 0, len(subscriptions))

	for _, sub := range subscriptions {
		item := SubscriptionSummaryItem{
			ID:             sub.ID,
			GroupID:        sub.GroupID,
			Status:         sub.Status,
			DailyUsedUSD:   sub.DailyUsageUSD,
			WeeklyUsedUSD:  sub.WeeklyUsageUSD,
			MonthlyUsedUSD: sub.MonthlyUsageUSD,
		}

		// Add group info if preloaded
		if sub.Group != nil {
			item.GroupName = sub.Group.Name
			if sub.Group.DailyLimitUSD != nil {
				item.DailyLimitUSD = *sub.Group.DailyLimitUSD
			}
			if sub.Group.WeeklyLimitUSD != nil {
				item.WeeklyLimitUSD = *sub.Group.WeeklyLimitUSD
			}
			if sub.Group.MonthlyLimitUSD != nil {
				item.MonthlyLimitUSD = *sub.Group.MonthlyLimitUSD
			}
		}

		// Format expiration time
		if !sub.ExpiresAt.IsZero() {
			formatted := sub.ExpiresAt.Format("2006-01-02T15:04:05Z07:00")
			item.ExpiresAt = &formatted
		}

		// Track total usage (use monthly as the most comprehensive)
		totalUsed += sub.MonthlyUsageUSD

		items = append(items, item)
	}

	summary := struct {
		ActiveCount   int                       `json:"active_count"`
		TotalUsedUSD  float64                   `json:"total_used_usd"`
		Subscriptions []SubscriptionSummaryItem `json:"subscriptions"`
	}{
		ActiveCount:   len(subscriptions),
		TotalUsedUSD:  totalUsed,
		Subscriptions: items,
	}

	response.Success(c, summary)
}

func (h *SubscriptionHandler) ReconcileResetCard(c *gin.Context) {
	subject, ok := middleware2.GetAuthSubjectFromContext(c)
	if !ok {
		response.Unauthorized(c, "User not found in context")
		return
	}
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil || id <= 0 {
		response.BadRequest(c, "Invalid subscription id")
		return
	}
	if h.resetCards == nil {
		response.ErrorFrom(c, infraerrors.InternalServer("RESET_CARD_UNAVAILABLE", "reset card service unavailable"))
		return
	}
	var operation *service.ResetCardOperation
	if strings.HasSuffix(c.FullPath(), "/prepare") {
		operation, err = h.resetCards.PrepareResetOperation(c.Request.Context(), subject.UserID, id, c.GetHeader("Idempotency-Key"))
	} else {
		operation, err = h.resetCards.ReconcileResetOperation(c.Request.Context(), subject.UserID, id, c.GetHeader("Idempotency-Key"))
	}
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	result := gin.H{"status": operation.Status}
	if operation.Result != nil {
		result["weekly_period_ends_at"] = operation.Result.WeeklyPeriodEndsAt
		result["short_period_ends_at"] = operation.Result.ShortPeriodEndsAt
		result["quota_policy"] = operation.Result.QuotaPolicy
	}
	response.Success(c, result)
}

func resetContractVersion(value string) int {
	if value == "2" {
		return 2
	}
	return 1
}
