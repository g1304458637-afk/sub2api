package service

import (
	"context"
	"fmt"
	"math"
	"regexp"
	"strings"
	"time"

	infraerrors "github.com/Wei-Shaw/sub2api/internal/pkg/errors"
)

// PresentationPriceSource 展示价来源（resolved display pricing 的 provenance）。
//
// manual：管理员手工设置的展示价 override；
// official：无 override 时回退到官方目录价（与计费同源：LiteLLM → 内置兜底 → 模型策略）；
// billing：无 override、官方目录也未覆盖时，回退到计费价（分组实收口径的标准价）；
// none：三者皆无，页面显示「价格暂未公布」，绝不猜测数字。
const (
	PresentationSourceManual   = "manual"
	PresentationSourceOfficial = "official"
	PresentationSourceBilling  = "billing"
	PresentationSourceNone     = "none"
)

// ErrPresentationPricingNotFound 指定模型无 override。
var ErrPresentationPricingNotFound = infraerrors.NotFound("PRESENTATION_PRICING_NOT_FOUND", "presentation pricing not found")

// presentationModelNamePattern canonical 模型名约束：API 调用名的常见形态
// （字母数字 . _ - : / @）。展示价按此名绑定，display_name 变化不影响配置。
var presentationModelNamePattern = regexp.MustCompile(`^[A-Za-z0-9._:/@-]{1,255}$`)

// presentationMaxPrice 单价上限（USD/token 或 USD/次）。防止管理员误输入
// 天文数字直接投放到用户页面；真实计费不受本表约束，无需对齐 Billing 校验。
const presentationMaxPrice = 100000.0

// PresentationModelPricing 单个模型的手工展示价 override（可选字段为 nil 表示该项不展示）。
type PresentationModelPricing struct {
	ID                int64
	ModelName         string
	BillingMode       BillingMode
	Currency          string
	InputPrice        *float64
	OutputPrice       *float64
	CacheWritePrice   *float64
	CacheWrite1hPrice *float64
	CacheReadPrice    *float64
	PerRequestPrice   *float64
	Enabled           bool
	Remark            string
	UpdatedBy         *int64
	CreatedAt         time.Time
	UpdatedAt         time.Time
}

// UpsertPresentationPricingInput 管理员保存展示价的入参。
type UpsertPresentationPricingInput struct {
	ModelName         string
	BillingMode       BillingMode
	Currency          string
	InputPrice        *float64
	OutputPrice       *float64
	CacheWritePrice   *float64
	CacheWrite1hPrice *float64
	CacheReadPrice    *float64
	PerRequestPrice   *float64
	Enabled           bool
	Remark            string
	UpdatedBy         int64
}

// PresentationPricingRepository 展示价 override 的持久化接口。
type PresentationPricingRepository interface {
	ListAll(ctx context.Context) ([]PresentationModelPricing, error)
	GetByModelName(ctx context.Context, modelName string) (*PresentationModelPricing, error)
	Upsert(ctx context.Context, pricing *PresentationModelPricing) error
	DeleteByModelName(ctx context.Context, modelName string) error
}

// PresentationPricingService 展示价（Presentation Pricing）服务。
//
// 职责边界（与 Billing 的依赖隔离是硬约束）：
//   - 只负责用户端「模型与价格」页面的标准价格展示配置；
//   - 不参与订阅额度、钱包余额、Usage charge 的任何计算；
//   - BillingService 不得依赖本服务；本服务也不修改任何 Billing 配置。
type PresentationPricingService struct {
	repo PresentationPricingRepository
}

// NewPresentationPricingService 创建展示价服务。
func NewPresentationPricingService(repo PresentationPricingRepository) *PresentationPricingService {
	return &PresentationPricingService{repo: repo}
}

// ListAll 返回全部 override（表极小，直接全量读取，不做缓存：
// 管理员保存后用户页面立即生效，天然满足缓存一致性要求）。
func (s *PresentationPricingService) ListAll(ctx context.Context) ([]PresentationModelPricing, error) {
	return s.repo.ListAll(ctx)
}

// GetByModelName 查询单个 override；不存在返回 ErrPresentationPricingNotFound。
func (s *PresentationPricingService) GetByModelName(ctx context.Context, modelName string) (*PresentationModelPricing, error) {
	return s.repo.GetByModelName(ctx, modelName)
}

// Upsert 校验并保存（创建或更新）展示价 override。
func (s *PresentationPricingService) Upsert(ctx context.Context, input *UpsertPresentationPricingInput) (*PresentationModelPricing, error) {
	if err := validateUpsertPresentationInput(input); err != nil {
		return nil, err
	}
	row := &PresentationModelPricing{
		ModelName:         strings.TrimSpace(input.ModelName),
		BillingMode:       input.BillingMode,
		Currency:          normalizePresentationCurrency(input.Currency),
		InputPrice:        clonePricePtr(input.InputPrice),
		OutputPrice:       clonePricePtr(input.OutputPrice),
		CacheWritePrice:   clonePricePtr(input.CacheWritePrice),
		CacheWrite1hPrice: clonePricePtr(input.CacheWrite1hPrice),
		CacheReadPrice:    clonePricePtr(input.CacheReadPrice),
		PerRequestPrice:   clonePricePtr(input.PerRequestPrice),
		Enabled:           input.Enabled,
		Remark:            strings.TrimSpace(input.Remark),
		UpdatedBy:         &input.UpdatedBy,
	}
	if err := s.repo.Upsert(ctx, row); err != nil {
		return nil, fmt.Errorf("upsert presentation pricing: %w", err)
	}
	return row, nil
}

// DeleteByModelName 清除 override，恢复回退链（manual → official → billing）。
// 幂等：不存在时静默成功。
func (s *PresentationPricingService) DeleteByModelName(ctx context.Context, modelName string) error {
	name := strings.TrimSpace(modelName)
	if name == "" {
		return infraerrors.BadRequest("PRESENTATION_PRICING_INVALID_MODEL", "model name is required")
	}
	return s.repo.DeleteByModelName(ctx, name)
}

// MapByModelName 返回 model_name → enabled override 的映射（展示层解析用）。
func (s *PresentationPricingService) MapByModelName(ctx context.Context) (map[string]*PresentationModelPricing, error) {
	rows, err := s.repo.ListAll(ctx)
	if err != nil {
		return nil, err
	}
	out := make(map[string]*PresentationModelPricing, len(rows))
	for i := range rows {
		if rows[i].Enabled {
			out[rows[i].ModelName] = &rows[i]
		}
	}
	return out, nil
}

// validateUpsertPresentationInput 入参校验：模型名、计价形态、币种、价格非负且
// 精度合法（≤3 位小数由前端约束，后端只挡非负 / 非有限 / 超上限 / 全空价格）。
func validateUpsertPresentationInput(input *UpsertPresentationPricingInput) error {
	name := strings.TrimSpace(input.ModelName)
	if name == "" {
		return infraerrors.BadRequest("PRESENTATION_PRICING_INVALID_MODEL", "model name is required")
	}
	if !presentationModelNamePattern.MatchString(name) {
		return infraerrors.BadRequest("PRESENTATION_PRICING_INVALID_MODEL", "model name contains unsupported characters")
	}
	if !input.BillingMode.IsValid() {
		return infraerrors.BadRequest("PRESENTATION_PRICING_INVALID_BILLING_MODE", "unsupported billing mode")
	}
	if input.BillingMode == "" {
		input.BillingMode = BillingModeToken
	}
	for _, p := range []*float64{
		input.InputPrice, input.OutputPrice,
		input.CacheWritePrice, input.CacheWrite1hPrice, input.CacheReadPrice,
		input.PerRequestPrice,
	} {
		if p == nil {
			continue
		}
		if math.IsNaN(*p) || math.IsInf(*p, 0) || *p < 0 {
			return infraerrors.BadRequest("PRESENTATION_PRICING_INVALID_PRICE", "price must be a non-negative finite number")
		}
		if *p > presentationMaxPrice {
			return infraerrors.BadRequest("PRESENTATION_PRICING_PRICE_TOO_LARGE", "price exceeds the allowed maximum")
		}
	}
	token := input.BillingMode == BillingModeToken
	hasTokenPrice := anyPriceSet(input.InputPrice, input.OutputPrice)
	hasRequestPrice := anyPriceSet(input.PerRequestPrice)
	if token && !hasTokenPrice {
		return infraerrors.BadRequest("PRESENTATION_PRICING_PRICE_REQUIRED", "at least one of input/output price is required for token pricing")
	}
	if !token && !hasRequestPrice {
		return infraerrors.BadRequest("PRESENTATION_PRICING_PRICE_REQUIRED", "per-request price is required for per-request/image/video pricing")
	}
	if len(strings.TrimSpace(input.Currency)) > 10 {
		return infraerrors.BadRequest("PRESENTATION_PRICING_INVALID_CURRENCY", "currency code is too long")
	}
	return nil
}

func anyPriceSet(values ...*float64) bool {
	for _, v := range values {
		if v != nil && *v > 0 {
			return true
		}
	}
	return false
}

func normalizePresentationCurrency(raw string) string {
	c := strings.ToUpper(strings.TrimSpace(raw))
	if c == "" {
		return "USD"
	}
	return c
}

func clonePricePtr(v *float64) *float64 {
	if v == nil {
		return nil
	}
	c := *v
	return &c
}
