package admin

// Phase 7 —— Scoped / Batch Direct Reset 管理端（create / preview / list / get / retry）。

import (
	"time"

	"github.com/Wei-Shaw/sub2api/internal/pkg/response"
	"github.com/Wei-Shaw/sub2api/internal/service"

	"github.com/gin-gonic/gin"
)

// AdminResetEventHandler Direct Reset 事件管理端。
type AdminResetEventHandler struct {
	resetEvents *service.ResetEventService
	resetCards  *service.ResetCardService
}

func NewAdminResetEventHandler(resetEvents *service.ResetEventService, resetCards *service.ResetCardService) *AdminResetEventHandler {
	return &AdminResetEventHandler{resetEvents: resetEvents, resetCards: resetCards}
}

// CreateResetEventRequest POST /admin/subscription-resets
type CreateResetEventRequest struct {
	TargetMode      string  `json:"target_mode" binding:"required,oneof=subscription_ids users groups all_active"`
	SubscriptionIDs []int64 `json:"subscription_ids"`
	UserIDs         []int64 `json:"user_ids"`
	GroupIDs        []int64 `json:"group_ids"`
	EffectiveAt     *string `json:"effective_at"`
	Reason          string  `json:"reason"`
	IdempotencyKey  string  `json:"idempotency_key" binding:"required"`
}

func (h *AdminResetEventHandler) CreateResetEvent(c *gin.Context) {
	var req CreateResetEventRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, "Invalid request body: "+err.Error())
		return
	}
	var effectiveAt *time.Time
	if req.EffectiveAt != nil && *req.EffectiveAt != "" {
		parsed, err := time.Parse(time.RFC3339, *req.EffectiveAt)
		if err != nil {
			response.BadRequest(c, "effective_at must be RFC3339")
			return
		}
		effectiveAt = &parsed
	}
	summary, err := h.resetEvents.CreateResetEvent(c.Request.Context(), &service.CreateResetEventInput{
		Selector: service.DirectResetSelector{
			TargetMode:      req.TargetMode,
			SubscriptionIDs: req.SubscriptionIDs,
			UserIDs:         req.UserIDs,
			GroupIDs:        req.GroupIDs,
		},
		EffectiveAt:    effectiveAt,
		Reason:         req.Reason,
		IdempotencyKey: req.IdempotencyKey,
		ActorAdminID:   getAdminIDFromContext(c),
	})
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	response.Success(c, summary)
}

// PreviewResetTargets POST /admin/subscription-resets/preview（无写入）
type PreviewResetTargetsRequest struct {
	TargetMode      string  `json:"target_mode" binding:"required"`
	SubscriptionIDs []int64 `json:"subscription_ids"`
	UserIDs         []int64 `json:"user_ids"`
	GroupIDs        []int64 `json:"group_ids"`
}

func (h *AdminResetEventHandler) PreviewResetTargets(c *gin.Context) {
	var req PreviewResetTargetsRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, "Invalid request body: "+err.Error())
		return
	}
	summary, err := h.resetEvents.PreviewResetTargets(c.Request.Context(), service.DirectResetSelector{
		TargetMode:      req.TargetMode,
		SubscriptionIDs: req.SubscriptionIDs,
		UserIDs:         req.UserIDs,
		GroupIDs:        req.GroupIDs,
	})
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	response.Success(c, summary)
}

// ListResetEvents GET /admin/subscription-resets?page=&page_size=
func (h *AdminResetEventHandler) ListResetEvents(c *gin.Context) {
	var q struct {
		Page     int `form:"page,default=1"`
		PageSize int `form:"page_size,default=50"`
	}
	if err := c.ShouldBindQuery(&q); err != nil {
		response.BadRequest(c, "Invalid query: "+err.Error())
		return
	}
	events, err := h.resetEvents.ListResetEvents(c.Request.Context(), q.PageSize, (q.Page-1)*q.PageSize)
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	response.Success(c, gin.H{"events": events})
}

// GetResetEvent GET /admin/subscription-resets/:id（含进度统计）
func (h *AdminResetEventHandler) GetResetEvent(c *gin.Context) {
	eventID, ok := parsePositiveIDParam(c, "id")
	if !ok {
		return
	}
	summary, err := h.resetEvents.GetResetEvent(c.Request.Context(), eventID)
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	response.Success(c, summary)
}

// RetryResetEvent POST /admin/subscription-resets/:id/retry（failed → pending）
func (h *AdminResetEventHandler) RetryResetEvent(c *gin.Context) {
	eventID, ok := parsePositiveIDParam(c, "id")
	if !ok {
		return
	}
	requeued, err := h.resetEvents.RetryFailedApplications(c.Request.Context(), eventID)
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	response.Success(c, gin.H{"requeued": requeued})
}

// PreviewGrantCards POST /admin/subscription-reset-cards/grants/preview 复用（发卡预览）
func (h *AdminResetEventHandler) PreviewGrantCardsUnimplemented() {}
