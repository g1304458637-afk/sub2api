//go:build unit

package service

import (
	"context"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/config"

	"github.com/stretchr/testify/require"
)

// TestPresentationPricingDoesNotAffectBillingIsolation 核心验收（任务 §43/§44）：
// 展示价 $100/1M、真实计费价 $3/1M 时——
//  1. 用户页面（Plaza 解析链）显示 manual override 的 $100/1M；
//  2. BillingService 对同一模型的计费仍按 $3/1M 计算，分文不差；
//  3. 修改 Billing（渠道价 $3 → $4）后：计费口径变 $4，用户页面仍显示 $100
//     （Billing 变更不覆盖 manual override）。
//
// 隔离的结构性保证：PresentationPricingService 与 BillingService 无任何共享字段 /
// 调用边（Billing 永不读展示价），本测试从数值上固化该边界。
//
// 模型用 claude-sonnet-4：BillingService 内置回退价 input $3/MTok（3e-6/token），
// 与渠道定价同值，便于对计费结果做严格断言。
func TestPresentationPricingDoesNotAffectBillingIsolation(t *testing.T) {
	ctx := context.Background()
	const model = "claude-sonnet-4"

	// 渠道定价 = 真实计费价：input 3e-6/token（$3/1M）。
	ch := plazaPricedChannel(1, "ch", []int64{10}, PlatformAnthropic, model)
	groups := []Group{{ID: 10, Name: "g", Platform: PlatformAnthropic, RateMultiplier: 1}}

	// 管理员设置展示价：input 1e-4/token（$100/1M）、output 2e-4/token。
	presentation := stubPresentationServiceForPlaza(plazaManualOverride(model, 1e-4, 2e-4))

	plaza := NewModelPlazaService(
		&mockChannelRepository{listAllFn: func(ctx context.Context) ([]Channel, error) { return []Channel{ch}, nil }},
		&stubGroupRepoForAvailable{activeGroups: groups},
		nil, nil, nil, presentation,
	)
	out, err := plaza.ListGroups(ctx)
	require.NoError(t, err)
	require.Len(t, out, 1)
	require.Len(t, out[0].Models, 1)
	model0 := out[0].Models[0]

	// 1) 用户页面展示 manual 标准价 $100/1M。
	require.Equal(t, PresentationSourceManual, model0.PresentationSource)
	require.InDelta(t, 1e-4, *model0.DisplayPricing.InputPrice, 1e-18)

	// 2) 真实计费仍按内置回退价 $3/1M：1M input tokens → cost 3.0（非 100）。
	billing := NewBillingService(&config.Config{}, nil)
	tokens := UsageTokens{InputTokens: 1_000_000, OutputTokens: 0}
	cost, err := billing.CalculateCost(model, tokens, 1.0)
	require.NoError(t, err)
	require.InDelta(t, 3.0, cost.InputCost, 1e-9,
		"真实计费必须按 $3/1M，绝不受展示价 $100/1M 影响")
	require.InDelta(t, 3.0, cost.TotalCost, 1e-9)

	// 3) 修改渠道计费价 $3 → $4 后重建广场数据：计费口径变 $4，展示仍 $100。
	chUpdated := plazaPricedChannel(1, "ch", []int64{10}, PlatformAnthropic, model)
	chUpdated.ModelPricing[0].InputPrice = testPtrFloat64(4e-6)
	plaza2 := NewModelPlazaService(
		&mockChannelRepository{listAllFn: func(ctx context.Context) ([]Channel, error) { return []Channel{chUpdated}, nil }},
		&stubGroupRepoForAvailable{activeGroups: groups},
		nil, nil, nil, presentation,
	)
	out2, err := plaza2.ListGroups(ctx)
	require.NoError(t, err)
	model1 := out2[0].Models[0]
	require.Equal(t, PresentationSourceManual, model1.PresentationSource)
	require.InDelta(t, 1e-4, *model1.DisplayPricing.InputPrice, 1e-18,
		"Billing 变更不得覆盖 manual 展示价")
	require.InDelta(t, 4e-6, *model1.Pricing.InputPrice, 1e-18,
		"计费口径已更新为 $4/1M")
}
