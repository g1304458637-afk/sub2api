package handler

// Phase 10 —— Plan Change 用户端（preview / upgrade order / scheduled downgrade /
// cancel scheduled / 审计查询）。金额全部服务端权威。

import (
	"strconv"

	"github.com/Wei-Shaw/sub2api/internal/pkg/response"
	middleware2 "github.com/Wei-Shaw/sub2api/internal/server/middleware"
	"github.com/Wei-Shaw/sub2api/internal/service"

	"github.com/gin-gonic/gin"
)

// PlanChangeHandler 用户 Plan Change 端点。
type PlanChangeHandler struct {
	planChanges *service.PlanChangeService
	payments    *service.PaymentService
}

func NewPlanChangeHandler(planChanges *service.PlanChangeService, payments *service.PaymentService) *PlanChangeHandler {
	return &PlanChangeHandler{planChanges: planChanges, payments: payments}
}

type planChangeTargetRequest struct {
	TargetPlanID int64 `json:"target_plan_id" binding:"required"`
}

// PreviewUpgrade POST /api/v1/subscriptions/:id/change/preview（纯读）
func (h *PlanChangeHandler) PreviewUpgrade(c *gin.Context) {
	subject, ok := middleware2GetSubject(c)
	if !ok {
		return
	}
	subscriptionID, ok := parseSubID(c)
	if !ok {
		return
	}
	var req planChangeTargetRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, "target_plan_id is required")
		return
	}
	quote, err := h.planChanges.PreviewUpgrade(c.Request.Context(), subject.UserID, subscriptionID, req.TargetPlanID)
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	response.Success(c, quote)
}

// CreateUpgrade POST /api/v1/subscriptions/:id/upgrade
// body: target_plan_id + Idempotency-Key（冻结报价行）；支付金额只来自报价。
func (h *PlanChangeHandler) CreateUpgrade(c *gin.Context) {
	subject, ok := middleware2GetSubject(c)
	if !ok {
		return
	}
	subscriptionID, ok := parseSubID(c)
	if !ok {
		return
	}
	var req struct {
		TargetPlanID int64  `json:"target_plan_id" binding:"required"`
		PaymentType  string `json:"payment_type" binding:"required"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, "target_plan_id and payment_type are required")
		return
	}
	ctx := c.Request.Context()
	_, changeID, err := h.planChanges.CreateUpgradeQuote(ctx, subject.UserID, subscriptionID, req.TargetPlanID, c.GetHeader("Idempotency-Key"))
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	// 创建支付订单：金额唯一来源 = 冻结报价行（客户端不传 amount）
	order, err := h.payments.CreateOrder(ctx, service.CreateOrderRequest{
		UserID:       subject.UserID,
		PaymentType:  req.PaymentType,
		OrderType:    "plan_change",
		PlanChangeID: changeID,
		ClientIP:     c.ClientIP(),
		SrcHost:      c.Request.Host,
	})
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	response.Success(c, gin.H{
		"order_id":       order.OrderID,
		"plan_change_id": changeID,
		"amount":         order.Amount,
		"pay_amount":     order.PayAmount,
		"payment_type":   order.PaymentType,
		"status":         order.Status,
		"pay_url":        order.PayURL,
		"qr_code":        order.QRCode,
	})
}

// ScheduleDowngrade POST /api/v1/subscriptions/:id/schedule-downgrade
func (h *PlanChangeHandler) ScheduleDowngrade(c *gin.Context) {
	subject, ok := middleware2GetSubject(c)
	if !ok {
		return
	}
	subscriptionID, ok := parseSubID(c)
	if !ok {
		return
	}
	var req planChangeTargetRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, "target_plan_id is required")
		return
	}
	rec, err := h.planChanges.ScheduleDowngrade(c.Request.Context(), subject.UserID, subscriptionID, req.TargetPlanID, c.GetHeader("Idempotency-Key"))
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	response.Success(c, rec)
}

// CancelScheduledDowngrade DELETE /api/v1/subscriptions/:id/schedule-downgrade
func (h *PlanChangeHandler) CancelScheduledDowngrade(c *gin.Context) {
	subject, ok := middleware2GetSubject(c)
	if !ok {
		return
	}
	subscriptionID, ok := parseSubID(c)
	if !ok {
		return
	}
	if err := h.planChanges.CancelScheduledDowngrade(c.Request.Context(), subject.UserID, subscriptionID); err != nil {
		response.ErrorFrom(c, err)
		return
	}
	response.Success(c, gin.H{"subscription_id": subscriptionID, "scheduled": false})
}

// GetChanges GET /api/v1/subscriptions/:id/changes
func (h *PlanChangeHandler) GetChanges(c *gin.Context) {
	subject, ok := middleware2GetSubject(c)
	if !ok {
		return
	}
	subscriptionID, ok := parseSubID(c)
	if !ok {
		return
	}
	records, err := h.planChanges.ListChangesBySubscription(c.Request.Context(), subject.UserID, subscriptionID, 20)
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	response.Success(c, records)
}

func middleware2GetSubject(c *gin.Context) (subject middleware2.AuthSubject, ok bool) {
	return middleware2.GetAuthSubjectFromContext(c)
}

func parseSubID(c *gin.Context) (int64, bool) {
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil || id <= 0 {
		response.BadRequest(c, "Invalid subscription id")
		return 0, false
	}
	return id, true
}
