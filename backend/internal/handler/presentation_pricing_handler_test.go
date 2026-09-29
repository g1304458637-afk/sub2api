//go:build unit

package handler

import (
	"encoding/json"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/service"

	"github.com/stretchr/testify/require"
)

// TestToModelPlazaGroupDTO_DisplayPricing 用户端 DTO 必须同时携带：
// display_pricing（标准展示价）+ presentation_source（来源），
// 且 pricing（计费口径）原样保留 —— 页面展示价 ≠ 实际扣费价的 DTO 基础。
func TestToModelPlazaGroupDTO_DisplayPricing(t *testing.T) {
	g := service.PlazaGroup{
		ID: 1, Name: "g", Platform: "openai", SubscriptionType: "standard", RateMultiplier: 1,
		Models: []service.PlazaModel{{
			Name:     "gpt-5.6",
			Platform: "openai",
			// 计费口径：¥3/1M（3e-6/token）
			Pricing: &service.ChannelModelPricing{
				BillingMode: service.BillingModeToken,
				InputPrice:  testPtr(3e-6),
				OutputPrice: testPtr(1e-5),
			},
			// 展示口径：管理员手工标准价 ¥100/1M（1e-4/token）
			DisplayPricing: &service.ChannelModelPricing{
				BillingMode: service.BillingModeToken,
				InputPrice:  testPtr(1e-4),
				OutputPrice: testPtr(2e-4),
			},
			PresentationSource: service.PresentationSourceManual,
		}, {
			Name:               "mystery",
			Platform:           "openai",
			PresentationSource: service.PresentationSourceNone,
		}},
	}

	raw, err := json.Marshal(toModelPlazaGroupDTO(&g, nil))
	require.NoError(t, err)
	var decoded map[string]any
	require.NoError(t, json.Unmarshal(raw, &decoded))

	models := decoded["models"].([]any)
	first := models[0].(map[string]any)
	display := first["display_pricing"].(map[string]any)
	require.InDelta(t, 1e-4, display["input_price"].(float64), 1e-18)
	require.Equal(t, "manual", first["presentation_source"])
	// 计费口径字段仍在（同一响应可对照，不因展示价而失真）。
	billing := first["pricing"].(map[string]any)
	require.InDelta(t, 3e-6, billing["input_price"].(float64), 1e-18)

	none := models[1].(map[string]any)
	require.Nil(t, none["display_pricing"], "无任何价格来源时 display_pricing 为 null")
	require.Equal(t, "none", none["presentation_source"])
}
