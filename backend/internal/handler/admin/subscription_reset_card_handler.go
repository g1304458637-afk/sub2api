package admin

// Phase 6 —— Reset Card 管理端（grant / list / revoke / preview）。
//
// 发卡为定向批量能力：users / groups / all_active_users 三种 target_mode，
// 按 User 去重 + quantity_per_user；durable 幂等（Idempotency-Key，重试不重复发卡）。
// Preview 无任何写入。不做最终 Admin UI。

import (
	"strings"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/pkg/response"
	"github.com/Wei-Shaw/sub2api/internal/service"

	"github.com/gin-gonic/gin"
)

// AdminSubscriptionResetHandler Reset Card 管理端（grant / list / revoke / preview）。
type AdminSubscriptionResetHandler struct {
	resetCards *service.ResetCardService
}

func NewAdminSubscriptionResetHandler(resetCards *service.ResetCardService) *AdminSubscriptionResetHandler {
	return &AdminSubscriptionResetHandler{resetCards: resetCards}
}

// GrantResetCardsRequest POST /admin/subscription-reset-cards/grants
type GrantResetCardsRequest struct {
	TargetMode      string  `json:"target_mode" binding:"required,oneof=users groups all_active_users"`
	UserIDs         []int64 `json:"user_ids"`
	GroupIDs        []int64 `json:"group_ids"`
	QuantityPerUser int     `json:"quantity_per_user" binding:"gte=1"`
	ExpiresAt       *string `json:"expires_at"`
	Reason          string  `json:"reason"`
	Campaign        string  `json:"campaign"`
	SourceType      string  `json:"source_type"`
	IdempotencyKey  string  `json:"idempotency_key"`
}

func (h *AdminSubscriptionResetHandler) GrantResetCards(c *gin.Context) {
	var req GrantResetCardsRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, "Invalid request body: "+err.Error())
		return
	}
	var expiresAt *time.Time
	if req.ExpiresAt != nil && strings.TrimSpace(*req.ExpiresAt) != "" {
		parsed, err := time.Parse(time.RFC3339, *req.ExpiresAt)
		if err != nil {
			response.BadRequest(c, "expires_at must be RFC3339")
			return
		}
		expiresAt = &parsed
	}
	if strings.TrimSpace(req.IdempotencyKey) == "" {
		req.IdempotencyKey = c.GetHeader("Idempotency-Key")
	}
	result, err := h.resetCards.GrantResetCards(c.Request.Context(), &service.GrantResetCardsInput{
		Selector: service.ResetCardGrantSelector{
			Mode:     req.TargetMode,
			UserIDs:  req.UserIDs,
			GroupIDs: req.GroupIDs,
		},
		QuantityPerUser: req.QuantityPerUser,
		ExpiresAt:       expiresAt,
		Reason:          strings.TrimSpace(req.Reason),
		Campaign:        strings.TrimSpace(req.Campaign),
		SourceType:      strings.TrimSpace(req.SourceType),
		IdempotencyKey:  strings.TrimSpace(req.IdempotencyKey),
		ActorAdminID:    getAdminIDFromContext(c),
	})
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	response.Success(c, result)
}

// PreviewGrantResetCards POST /admin/subscription-reset-cards/grants/preview（无写入）
func (h *AdminSubscriptionResetHandler) PreviewGrantResetCards(c *gin.Context) {
	var req GrantResetCardsRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, "Invalid request body: "+err.Error())
		return
	}
	summary, err := h.resetCards.PreviewGrantTargets(c.Request.Context(), service.ResetCardGrantSelector{
		Mode:     req.TargetMode,
		UserIDs:  req.UserIDs,
		GroupIDs: req.GroupIDs,
	}, req.QuantityPerUser)
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	response.Success(c, gin.H{"unique_users": summary.UniqueUserCount, "total_cards": summary.UniqueUserCount * int64(req.QuantityPerUser), "target_mode": summary.TargetMode, "subscription_count": summary.SubscriptionCount, "group_breakdown": summary.GroupBreakdown})
}

// ListResetCards GET /admin/subscription-reset-cards?user_id=&status=&page=&page_size=
func (h *AdminSubscriptionResetHandler) ListResetCards(c *gin.Context) {
	var q struct {
		UserID   int64  `form:"user_id"`
		Status   string `form:"status"`
		Page     int    `form:"page,default=1" binding:"gte=1"`
		PageSize int    `form:"page_size,default=50" binding:"gte=1,lte=100"`
	}
	if err := c.ShouldBindQuery(&q); err != nil {
		response.BadRequest(c, "Invalid query: "+err.Error())
		return
	}
	if q.UserID <= 0 {
		response.BadRequest(c, "user_id is required")
		return
	}
	var statusPtr *string
	if q.Status != "" {
		statusPtr = &q.Status
	}
	cards, total, err := h.resetCards.ListResetCards(c.Request.Context(), q.UserID, statusPtr, q.PageSize, (q.Page-1)*q.PageSize)
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	response.Success(c, gin.H{
		"cards": cards,
		"items": cards,
		"total": total,
	})
}

// RevokeResetCard POST /admin/subscription-reset-cards/:id/revoke（available → revoked）
func (h *AdminSubscriptionResetHandler) RevokeResetCard(c *gin.Context) {
	cardID, ok := parsePositiveIDParam(c, "id")
	if !ok {
		return
	}
	if err := h.resetCards.RevokeResetCard(c.Request.Context(), cardID); err != nil {
		response.ErrorFrom(c, err)
		return
	}
	response.Success(c, gin.H{"id": cardID, "status": "revoked"})
}

// CountAvailableResetCards GET /admin/subscription-reset-cards/count?user_id=
func (h *AdminSubscriptionResetHandler) CountAvailableResetCards(c *gin.Context) {
	var q struct {
		UserID int64 `form:"user_id"`
	}
	if err := c.ShouldBindQuery(&q); err != nil || q.UserID <= 0 {
		response.BadRequest(c, "user_id is required")
		return
	}
	count, err := h.resetCards.CountAvailableResetCards(c.Request.Context(), q.UserID)
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	response.Success(c, gin.H{"available": count})
}
