package admin

import (
	"log/slog"
	"strings"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/pkg/response"
	"github.com/Wei-Shaw/sub2api/internal/server/middleware"
	"github.com/Wei-Shaw/sub2api/internal/service"

	"github.com/gin-gonic/gin"
)

// PresentationPricingHandler 管理员「模型展示价格」管理。
//
// 边界提示（UI 与 API 注释反复强调）：本能力只影响用户端「模型与价格」页面的
// 标准价格展示，不影响任何真实计费（订阅额度 / 钱包 / Usage charge）。
type PresentationPricingHandler struct {
	service *service.PresentationPricingService
}

// NewPresentationPricingHandler 创建展示价管理 handler。
func NewPresentationPricingHandler(service *service.PresentationPricingService) *PresentationPricingHandler {
	return &PresentationPricingHandler{service: service}
}

// presentationPricingPayload 展示价保存入参（价格以 USD per token 存储，与渠道
// 定价同一量纲；「每 1M token」的输入换算由前端复用 mTokToPerToken 完成）。
type presentationPricingPayload struct {
	ModelName         string    `json:"model_name" binding:"required"`
	BillingMode       string    `json:"billing_mode"`
	Currency          string    `json:"currency"`
	InputPrice        *float64  `json:"input_price" binding:"omitempty,min=0"`
	OutputPrice       *float64  `json:"output_price" binding:"omitempty,min=0"`
	CacheWritePrice   *float64  `json:"cache_write_price" binding:"omitempty,min=0"`
	CacheWrite1hPrice *float64  `json:"cache_write_1h_price" binding:"omitempty,min=0"`
	CacheReadPrice    *float64  `json:"cache_read_price" binding:"omitempty,min=0"`
	PerRequestPrice   *float64  `json:"per_request_price" binding:"omitempty,min=0"`
	Enabled           *bool     `json:"enabled"`
	Remark            string    `json:"remark"`
	UpdatedBy         *int64    `json:"-"` // 从认证上下文取，不接受客户端伪造
}

// presentationPricingItem override 行（含操作人）。
type presentationPricingItem struct {
	ID                 int64     `json:"id"`
	ModelName          string    `json:"model_name"`
	BillingMode        string    `json:"billing_mode"`
	Currency           string    `json:"currency"`
	InputPrice         *float64  `json:"input_price"`
	OutputPrice        *float64  `json:"output_price"`
	CacheWritePrice    *float64  `json:"cache_write_price"`
	CacheWrite1hPrice  *float64  `json:"cache_write_1h_price"`
	CacheReadPrice     *float64  `json:"cache_read_price"`
	PerRequestPrice    *float64  `json:"per_request_price"`
	Enabled            bool      `json:"enabled"`
	Remark             string    `json:"remark"`
	UpdatedBy          *int64    `json:"updated_by"`
	UpdatedAt          time.Time `json:"updated_at"`
}

// List 列出全部展示价 override。
// GET /api/v1/admin/model-presentation-pricing
func (h *PresentationPricingHandler) List(c *gin.Context) {
	rows, err := h.service.ListAll(c.Request.Context())
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	out := make([]presentationPricingItem, 0, len(rows))
	for i := range rows {
		out = append(out, toPresentationPricingItem(&rows[i]))
	}
	response.Success(c, gin.H{"items": out})
}

// Upsert 创建或更新指定模型的展示价 override（按 model_name 幂等）。
// PUT /api/v1/admin/model-presentation-pricing
func (h *PresentationPricingHandler) Upsert(c *gin.Context) {
	var payload presentationPricingPayload
	if err := c.ShouldBindJSON(&payload); err != nil {
		response.BadRequest(c, err.Error())
		return
	}
	subject, _ := middleware.GetAuthSubjectFromContext(c)

	input := &service.UpsertPresentationPricingInput{
		ModelName:         strings.TrimSpace(payload.ModelName),
		BillingMode:       service.BillingMode(strings.TrimSpace(payload.BillingMode)),
		Currency:          payload.Currency,
		InputPrice:        payload.InputPrice,
		OutputPrice:       payload.OutputPrice,
		CacheWritePrice:   payload.CacheWritePrice,
		CacheWrite1hPrice: payload.CacheWrite1hPrice,
		CacheReadPrice:    payload.CacheReadPrice,
		PerRequestPrice:   payload.PerRequestPrice,
		Enabled:           payload.Enabled == nil || *payload.Enabled,
		Remark:            payload.Remark,
		UpdatedBy:         subject.UserID,
	}
	before, _ := h.service.GetByModelName(c.Request.Context(), input.ModelName)
	row, err := h.service.Upsert(c.Request.Context(), input)
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}

	// 展示价是业务重要配置：除审计中间件的请求级记录外，落一条变更明细
	// （谁、何时、哪个模型、旧值 → 新值），供管理员排查「用户看到的价格」。
	logPresentationPricingChange(c, "update", input.ModelName, before, row)
	response.Success(c, toPresentationPricingItem(row))
}

// Delete 清除指定模型的 override，恢复回退链（manual → official → billing）。幂等。
// DELETE /api/v1/admin/model-presentation-pricing?model_name=xxx
func (h *PresentationPricingHandler) Delete(c *gin.Context) {
	modelName := strings.TrimSpace(c.Query("model_name"))
	if modelName == "" {
		response.BadRequest(c, "model_name is required")
		return
	}
	before, _ := h.service.GetByModelName(c.Request.Context(), modelName)
	if err := h.service.DeleteByModelName(c.Request.Context(), modelName); err != nil {
		response.ErrorFrom(c, err)
		return
	}
	logPresentationPricingChange(c, "delete", modelName, before, nil)
	response.Success(c, gin.H{"deleted": true})
}

func toPresentationPricingItem(row *service.PresentationModelPricing) presentationPricingItem {
	return presentationPricingItem{
		ID:                 row.ID,
		ModelName:          row.ModelName,
		BillingMode:        string(row.BillingMode),
		Currency:           row.Currency,
		InputPrice:         row.InputPrice,
		OutputPrice:        row.OutputPrice,
		CacheWritePrice:    row.CacheWritePrice,
		CacheWrite1hPrice:  row.CacheWrite1hPrice,
		CacheReadPrice:     row.CacheReadPrice,
		PerRequestPrice:    row.PerRequestPrice,
		Enabled:            row.Enabled,
		Remark:             row.Remark,
		UpdatedBy:          row.UpdatedBy,
		UpdatedAt:          row.UpdatedAt,
	}
}

// logPresentationPricingChange 记录展示价变更明细（旧值 → 新值摘要）。
func logPresentationPricingChange(c *gin.Context, action, modelName string, before *service.PresentationModelPricing, after *service.PresentationModelPricing) {
	subject, _ := middleware.GetAuthSubjectFromContext(c)
	attrs := []any{
		"audit", true,
		"action", action,
		"model_name", modelName,
		"user_id", subject.UserID,
	}
	if before != nil {
		attrs = append(attrs, "before", presentationPricingSummary(before))
	}
	if after != nil {
		attrs = append(attrs, "after", presentationPricingSummary(after))
	}
	slog.Info("presentation_pricing_changed", attrs...)
}

func presentationPricingSummary(p *service.PresentationModelPricing) gin.H {
	return gin.H{
		"billing_mode":        string(p.BillingMode),
		"input_price":         p.InputPrice,
		"output_price":        p.OutputPrice,
		"cache_write_price":   p.CacheWritePrice,
		"cache_write_1h":      p.CacheWrite1hPrice,
		"cache_read_price":    p.CacheReadPrice,
		"per_request_price":   p.PerRequestPrice,
		"enabled":             p.Enabled,
	}
}
