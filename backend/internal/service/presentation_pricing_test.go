//go:build unit

package service

import (
	"context"
	"testing"

	infraerrors "github.com/Wei-Shaw/sub2api/internal/pkg/errors"
	"github.com/stretchr/testify/require"
)

// stubPresentationPricingRepo 内存仓储：驱动 service 层 CRUD 语义测试。
type stubPresentationPricingRepo struct {
	rows    map[string]*PresentationModelPricing
	nextID  int64
	delErr  error
	upserts int
	deletes []string
}

func newStubPresentationPricingRepo() *stubPresentationPricingRepo {
	return &stubPresentationPricingRepo{rows: map[string]*PresentationModelPricing{}}
}

func (r *stubPresentationPricingRepo) ListAll(ctx context.Context) ([]PresentationModelPricing, error) {
	out := make([]PresentationModelPricing, 0, len(r.rows))
	for _, v := range r.rows {
		out = append(out, *v)
	}
	return out, nil
}

func (r *stubPresentationPricingRepo) GetByModelName(ctx context.Context, modelName string) (*PresentationModelPricing, error) {
	if v, ok := r.rows[modelName]; ok {
		copied := *v
		return &copied, nil
	}
	return nil, ErrPresentationPricingNotFound
}

func (r *stubPresentationPricingRepo) Upsert(ctx context.Context, pricing *PresentationModelPricing) error {
	r.upserts++
	existing, ok := r.rows[pricing.ModelName]
	if ok {
		existing.BillingMode = pricing.BillingMode
		existing.Currency = pricing.Currency
		existing.InputPrice = pricing.InputPrice
		existing.OutputPrice = pricing.OutputPrice
		existing.CacheWritePrice = pricing.CacheWritePrice
		existing.CacheWrite1hPrice = pricing.CacheWrite1hPrice
		existing.CacheReadPrice = pricing.CacheReadPrice
		existing.PerRequestPrice = pricing.PerRequestPrice
		existing.Enabled = pricing.Enabled
		existing.Remark = pricing.Remark
		existing.UpdatedBy = pricing.UpdatedBy
		*pricing = *existing
		return nil
	}
	r.nextID++
	pricing.ID = r.nextID
	r.rows[pricing.ModelName] = pricing
	return nil
}

func (r *stubPresentationPricingRepo) DeleteByModelName(ctx context.Context, modelName string) error {
	if r.delErr != nil {
		return r.delErr
	}
	r.deletes = append(r.deletes, modelName)
	delete(r.rows, modelName)
	return nil
}

func TestPresentationPricingUpsertValidation(t *testing.T) {
	svc := NewPresentationPricingService(newStubPresentationPricingRepo())
	ctx := context.Background()

	// token 模式：至少要有一个 token 价格。
	_, err := svc.Upsert(ctx, &UpsertPresentationPricingInput{ModelName: "gpt-5.6", BillingMode: BillingModeToken, Enabled: true, UpdatedBy: 1})
	require.Error(t, err)
	require.True(t, infraerrors.IsBadRequest(err))

	// 负数价格拒绝。
	neg := -1.0
	_, err = svc.Upsert(ctx, &UpsertPresentationPricingInput{ModelName: "gpt-5.6", BillingMode: BillingModeToken, InputPrice: &neg, Enabled: true, UpdatedBy: 1})
	require.Error(t, err)
	require.True(t, infraerrors.IsBadRequest(err))

	// 非法模型名拒绝（含空格等非 canonical 字符）。
	_, err = svc.Upsert(ctx, &UpsertPresentationPricingInput{ModelName: "bad model name", BillingMode: BillingModeToken, InputPrice: testPtrFloat64(3), Enabled: true, UpdatedBy: 1})
	require.Error(t, err)

	// 空模型名拒绝。
	_, err = svc.Upsert(ctx, &UpsertPresentationPricingInput{ModelName: " ", BillingMode: BillingModeToken, InputPrice: testPtrFloat64(3), Enabled: true, UpdatedBy: 1})
	require.Error(t, err)

	// 合法保存：币种归一、updated_by 落行。
	row, err := svc.Upsert(ctx, &UpsertPresentationPricingInput{
		ModelName: "gpt-5.6", BillingMode: BillingModeToken, Currency: "usd",
		InputPrice: testPtrFloat64(1e-5), OutputPrice: testPtrFloat64(8e-5),
		Enabled: true, UpdatedBy: 42,
	})
	require.NoError(t, err)
	require.Equal(t, "USD", row.Currency)
	require.NotNil(t, row.UpdatedBy)
	require.Equal(t, int64(42), *row.UpdatedBy)

	// 幂等 upsert：同名覆盖而非新增。
	got, err := svc.GetByModelName(ctx, "gpt-5.6")
	require.NoError(t, err)
	require.Equal(t, 1e-5, *got.InputPrice)

	// per-request 模式必须有按次价。
	_, err = svc.Upsert(ctx, &UpsertPresentationPricingInput{ModelName: "dall-e-3", BillingMode: BillingModeImage, Enabled: true, UpdatedBy: 1})
	require.Error(t, err)

	// per-request 模式合法保存。
	_, err = svc.Upsert(ctx, &UpsertPresentationPricingInput{ModelName: "dall-e-3", BillingMode: BillingModeImage, PerRequestPrice: testPtrFloat64(0.04), Enabled: true, UpdatedBy: 1})
	require.NoError(t, err)
}

func TestPresentationPricingDeleteRestoresFallback(t *testing.T) {
	repo := newStubPresentationPricingRepo()
	svc := NewPresentationPricingService(repo)
	ctx := context.Background()

	_, err := svc.Upsert(ctx, &UpsertPresentationPricingInput{
		ModelName: "gpt-5.6", BillingMode: BillingModeToken,
		InputPrice: testPtrFloat64(1e-5), Enabled: true, UpdatedBy: 1,
	})
	require.NoError(t, err)

	// 清除 override 幂等：删除两次都不报错。
	require.NoError(t, svc.DeleteByModelName(ctx, "gpt-5.6"))
	require.NoError(t, svc.DeleteByModelName(ctx, "gpt-5.6"))
	require.Empty(t, repo.rows)

	// 空模型名拒绝。
	require.Error(t, svc.DeleteByModelName(ctx, " "))
}

func TestPresentationPricingMapFiltersDisabled(t *testing.T) {
	repo := newStubPresentationPricingRepo()
	svc := NewPresentationPricingService(repo)
	ctx := context.Background()
	_, err := svc.Upsert(ctx, &UpsertPresentationPricingInput{
		ModelName: "m-a", BillingMode: BillingModeToken, InputPrice: testPtrFloat64(1), Enabled: true, UpdatedBy: 1,
	})
	require.NoError(t, err)
	_, err = svc.Upsert(ctx, &UpsertPresentationPricingInput{
		ModelName: "m-b", BillingMode: BillingModeToken, InputPrice: testPtrFloat64(2), Enabled: false, UpdatedBy: 1,
	})
	require.NoError(t, err)

	m, err := svc.MapByModelName(ctx)
	require.NoError(t, err)
	require.Contains(t, m, "m-a")
	require.NotContains(t, m, "m-b", "disabled override 不参与展示价解析")
}
