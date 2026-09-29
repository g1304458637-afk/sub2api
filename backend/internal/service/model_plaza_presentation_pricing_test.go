//go:build unit

package service

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"
)

// stubPresentationServiceForPlaza 用指定 override 表构造 PresentationPricingService。
func stubPresentationServiceForPlaza(rows ...PresentationModelPricing) *PresentationPricingService {
	repo := newStubPresentationPricingRepo()
	for i := range rows {
		row := rows[i]
		row.Enabled = true
		repo.rows[row.ModelName] = &row
	}
	return NewPresentationPricingService(repo)
}

func plazaManualOverride(name string, input, output float64) PresentationModelPricing {
	return PresentationModelPricing{
		ModelName:   name,
		BillingMode: BillingModeToken,
		InputPrice:  testPtrFloat64(input),
		OutputPrice: testPtrFloat64(output),
	}
}

// TestFillPresentationPricing_ManualOverride 核心语义：manual override 是展示价的
// 最高优先级，且解析过程绝不回写 Pricing（真实计费口径不受影响）。
func TestFillPresentationPricing_ManualOverride(t *testing.T) {
	original := &ChannelModelPricing{
		BillingMode: BillingModeToken,
		InputPrice:  testPtrFloat64(3e-6),
		OutputPrice: testPtrFloat64(1.5e-5),
	}
	m := &PlazaModel{
		Name:     "gpt-5.6",
		Platform: PlatformOpenAI,
		Pricing:  original,
	}
	official := &PlazaOfficialPricing{InputPrice: testPtrFloat64(3e-5), OutputPrice: testPtrFloat64(1.2e-4)}
	m.OfficialPricing = official

	presentation := stubPresentationServiceForPlaza(plazaManualOverride("gpt-5.6", 1e-5, 8e-5))
	svc := NewModelPlazaService(nil, nil, nil, nil, nil, presentation)
	svc.fillPresentationPricing(m, nil, mustOverrideMap(t, presentation))

	require.Equal(t, PresentationSourceManual, m.PresentationSource)
	require.NotNil(t, m.DisplayPricing)
	require.InDelta(t, 1e-5, *m.DisplayPricing.InputPrice, 1e-18)
	require.InDelta(t, 8e-5, *m.DisplayPricing.OutputPrice, 1e-18)
	// 隔离硬约束：Pricing（计费口径）与 OfficialPricing 保持原样。
	require.Same(t, original, m.Pricing)
	require.InDelta(t, 3e-6, *m.Pricing.InputPrice, 1e-18)
	require.InDelta(t, 1.5e-5, *m.Pricing.OutputPrice, 1e-18)
	require.Same(t, official, m.OfficialPricing)
	// manual 展示价下分时倍率时段对用户无意义。
	require.Nil(t, m.TimePricing)
}

func TestFillPresentationPricing_OfficialFallback(t *testing.T) {
	m := &PlazaModel{
		Name:     "claude-sonnet",
		Platform: PlatformAnthropic,
		Pricing:  &ChannelModelPricing{BillingMode: BillingModeToken, InputPrice: testPtrFloat64(3e-6)},
		OfficialPricing: &PlazaOfficialPricing{
			InputPrice:  testPtrFloat64(3e-6),
			OutputPrice: testPtrFloat64(1.5e-5),
		},
	}
	presentation := stubPresentationServiceForPlaza()
	svc := NewModelPlazaService(nil, nil, nil, nil, nil, presentation)
	svc.fillPresentationPricing(m, nil, mustOverrideMap(t, presentation))

	require.Equal(t, PresentationSourceOfficial, m.PresentationSource)
	require.InDelta(t, 3e-6, *m.DisplayPricing.InputPrice, 1e-18)
	require.InDelta(t, 1.5e-5, *m.DisplayPricing.OutputPrice, 1e-18)
}

func TestFillPresentationPricing_BillingFallbackScalesByGroupRate(t *testing.T) {
	g := Group{ID: 10, Name: "g", Platform: PlatformOpenAI, RateMultiplier: 0.5}
	m := &PlazaModel{
		Name:     "gpt-no-official",
		Platform: PlatformOpenAI,
		Pricing: &ChannelModelPricing{
			BillingMode: BillingModeToken,
			InputPrice:  testPtrFloat64(3e-6),
			OutputPrice: testPtrFloat64(1.5e-5),
		},
	}
	presentation := stubPresentationServiceForPlaza()
	svc := NewModelPlazaService(nil, nil, nil, nil, nil, presentation)
	svc.fillPresentationPricing(m, &g, mustOverrideMap(t, presentation))

	require.Equal(t, PresentationSourceBilling, m.PresentationSource)
	// billing 回退展示分组默认倍率折算后的标准实付价。
	require.InDelta(t, 1.5e-6, *m.DisplayPricing.InputPrice, 1e-18)
	require.InDelta(t, 7.5e-6, *m.DisplayPricing.OutputPrice, 1e-18)
	// 计费口径原值不变。
	require.InDelta(t, 3e-6, *m.Pricing.InputPrice, 1e-18)
}

func TestFillPresentationPricing_NoneWhenNoPricingAtAll(t *testing.T) {
	m := &PlazaModel{Name: "mystery-model", Platform: PlatformOpenAI}
	presentation := stubPresentationServiceForPlaza()
	svc := NewModelPlazaService(nil, nil, nil, nil, nil, presentation)
	svc.fillPresentationPricing(m, nil, mustOverrideMap(t, presentation))

	require.Equal(t, PresentationSourceNone, m.PresentationSource)
	require.Nil(t, m.DisplayPricing, "无任何价格来源时不得猜测数字")
}

func TestFillPresentationPricing_DisabledOverrideSkipped(t *testing.T) {
	repo := newStubPresentationPricingRepo()
	repo.rows["gpt-5.6"] = &PresentationModelPricing{
		ModelName:   "gpt-5.6",
		BillingMode: BillingModeToken,
		InputPrice:  testPtrFloat64(9e-5),
		Enabled:     false,
	}
	svc := NewPresentationPricingService(repo)
	m := &PlazaModel{
		Name:     "gpt-5.6",
		Platform: PlatformOpenAI,
		Pricing:  &ChannelModelPricing{BillingMode: BillingModeToken, InputPrice: testPtrFloat64(3e-6)},
		OfficialPricing: &PlazaOfficialPricing{
			InputPrice: testPtrFloat64(3e-5), OutputPrice: testPtrFloat64(1.2e-4),
		},
	}
	svc2 := NewModelPlazaService(nil, nil, nil, nil, nil, svc)
	svc2.fillPresentationPricing(m, nil, mustOverrideMap(t, svc))

	require.Equal(t, PresentationSourceOfficial, m.PresentationSource, "disabled override 应跳过走 official")
}

// TestPlazaListGroups_PresentationOverridesEndToEnd 全链路：渠道模型 → 广场聚合 →
// 展示价解析。manual 命中、未配置走 billing 回退，二者互不影响。
func TestPlazaListGroups_PresentationOverridesEndToEnd(t *testing.T) {
	ch := plazaPricedChannel(1, "ch", []int64{10}, PlatformOpenAI, "gpt-override", "gpt-billing-fallback")
	groups := []Group{{ID: 10, Name: "g", Platform: PlatformOpenAI, RateMultiplier: 1}}

	svc := NewModelPlazaService(
		&mockChannelRepository{listAllFn: func(ctx context.Context) ([]Channel, error) { return []Channel{ch}, nil }},
		&stubGroupRepoForAvailable{activeGroups: groups},
		nil, nil, nil,
		stubPresentationServiceForPlaza(plazaManualOverride("gpt-override", 1e-5, 8e-5)),
	)
	out, err := svc.ListGroups(context.Background())
	require.NoError(t, err)
	require.Len(t, out, 1)
	require.Len(t, out[0].Models, 2)

	byName := map[string]PlazaModel{}
	for _, m := range out[0].Models {
		byName[m.Name] = m
	}
	require.Equal(t, PresentationSourceManual, byName["gpt-override"].PresentationSource)
	require.InDelta(t, 1e-5, *byName["gpt-override"].DisplayPricing.InputPrice, 1e-18)
	// 计费价保持渠道原价（3e-6），未被 override 污染。
	require.InDelta(t, 3e-6, *byName["gpt-override"].Pricing.InputPrice, 1e-18)

	require.Equal(t, PresentationSourceBilling, byName["gpt-billing-fallback"].PresentationSource)
	require.InDelta(t, 3e-6, *byName["gpt-billing-fallback"].DisplayPricing.InputPrice, 1e-18)
}

func mustOverrideMap(t *testing.T, svc *PresentationPricingService) map[string]*PresentationModelPricing {
	t.Helper()
	m, err := svc.MapByModelName(context.Background())
	require.NoError(t, err)
	return m
}

// TestFillPresentationPricing_PerUnitModelPrefersBillingOverOfficial 按图/按次模型的
// 展示价回退用同量纲的计费按次价，不用官方 token 参考价（量纲不符，§40）。
func TestFillPresentationPricing_PerUnitModelPrefersBillingOverOfficial(t *testing.T) {
	g := Group{ID: 10, Name: "g", Platform: PlatformOpenAI, RateMultiplier: 0.5, ImageRateIndependent: false}
	m := &PlazaModel{
		Name:     "gpt-image-2",
		Platform: PlatformOpenAI,
		Pricing: &ChannelModelPricing{
			BillingMode:     BillingModeImage,
			PerRequestPrice: testPtrFloat64(0.04),
		},
		OfficialPricing: &PlazaOfficialPricing{InputPrice: testPtrFloat64(5e-6), OutputPrice: testPtrFloat64(1e-5)},
	}
	presentation := stubPresentationServiceForPlaza()
	svc := NewModelPlazaService(nil, nil, nil, nil, nil, presentation)
	svc.fillPresentationPricing(m, &g, mustOverrideMap(t, presentation))

	require.Equal(t, PresentationSourceBilling, m.PresentationSource)
	require.Equal(t, BillingModeImage, m.DisplayPricing.BillingMode)
	// 0.04 × 0.5 = 0.02 按次标准价；绝不是官方 token 价
	require.InDelta(t, 0.02, *m.DisplayPricing.PerRequestPrice, 1e-12)
	require.Nil(t, m.DisplayPricing.InputPrice)
}
