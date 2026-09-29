package repository

import (
	"context"

	dbent "github.com/Wei-Shaw/sub2api/ent"
	"github.com/Wei-Shaw/sub2api/ent/presentationmodelpricing"
	"github.com/Wei-Shaw/sub2api/internal/service"
)

type presentationPricingRepository struct {
	client *dbent.Client
}

// NewPresentationPricingRepository 创建展示价（Presentation Pricing）仓储。
func NewPresentationPricingRepository(client *dbent.Client) service.PresentationPricingRepository {
	return &presentationPricingRepository{client: client}
}

func (r *presentationPricingRepository) ListAll(ctx context.Context) ([]service.PresentationModelPricing, error) {
	rows, err := r.client.PresentationModelPricing.Query().
		Order(dbent.Asc(presentationmodelpricing.FieldModelName)).
		All(ctx)
	if err != nil {
		return nil, err
	}
	out := make([]service.PresentationModelPricing, 0, len(rows))
	for i := range rows {
		out = append(out, presentationPricingEntityToService(rows[i]))
	}
	return out, nil
}

func (r *presentationPricingRepository) GetByModelName(ctx context.Context, modelName string) (*service.PresentationModelPricing, error) {
	m, err := r.client.PresentationModelPricing.Query().
		Where(presentationmodelpricing.ModelNameEQ(modelName)).
		Only(ctx)
	if err != nil {
		return nil, translatePersistenceError(err, service.ErrPresentationPricingNotFound, nil)
	}
	row := presentationPricingEntityToService(m)
	return &row, nil
}

// Upsert 按 model_name 幂等保存。updated_at 由 ent UpdateDefault 维护。
func (r *presentationPricingRepository) Upsert(ctx context.Context, pricing *service.PresentationModelPricing) error {
	created, err := r.client.PresentationModelPricing.Create().
		SetModelName(pricing.ModelName).
		SetBillingMode(string(pricing.BillingMode)).
		SetCurrency(pricing.Currency).
		SetNillableInputPrice(pricing.InputPrice).
		SetNillableOutputPrice(pricing.OutputPrice).
		SetNillableCacheWritePrice(pricing.CacheWritePrice).
		SetNillableCacheWrite1hPrice(pricing.CacheWrite1hPrice).
		SetNillableCacheReadPrice(pricing.CacheReadPrice).
		SetNillablePerRequestPrice(pricing.PerRequestPrice).
		SetEnabled(pricing.Enabled).
		SetRemark(pricing.Remark).
		SetNillableUpdatedBy(pricing.UpdatedBy).
		OnConflictColumns(presentationmodelpricing.FieldModelName).
		UpdateNewValues().
		ID(ctx)
	if err != nil {
		return err
	}
	// Upsert 冲突分支不会回填 created_at 等字段，读一次保证返回完整行。
	saved, err := r.client.PresentationModelPricing.Query().
		Where(presentationmodelpricing.IDEQ(created)).
		Only(ctx)
	if err != nil {
		return err
	}
	*pricing = presentationPricingEntityToService(saved)
	return nil
}

func (r *presentationPricingRepository) DeleteByModelName(ctx context.Context, modelName string) error {
	_, err := r.client.PresentationModelPricing.Delete().
		Where(presentationmodelpricing.ModelNameEQ(modelName)).
		Exec(ctx)
	return err
}

func presentationPricingEntityToService(m *dbent.PresentationModelPricing) service.PresentationModelPricing {
	return service.PresentationModelPricing{
		ID:                m.ID,
		ModelName:         m.ModelName,
		BillingMode:       service.BillingMode(m.BillingMode),
		Currency:          m.Currency,
		InputPrice:        m.InputPrice,
		OutputPrice:       m.OutputPrice,
		CacheWritePrice:   m.CacheWritePrice,
		CacheWrite1hPrice: m.CacheWrite1hPrice,
		CacheReadPrice:    m.CacheReadPrice,
		PerRequestPrice:   m.PerRequestPrice,
		Enabled:           m.Enabled,
		Remark:            m.Remark,
		UpdatedBy:         m.UpdatedBy,
		CreatedAt:         m.CreatedAt,
		UpdatedAt:         m.UpdatedAt,
	}
}
