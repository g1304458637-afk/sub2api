//go:build unit

package service

// Phase 4 —— 状态计算纯函数单元测试（阈值唯一权威定义在 account_status_service.go）。

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestPhase4ClassifyUsageStatus(t *testing.T) {
	t.Parallel()
	cases := []struct {
		percent float64
		want    UsageStatus
	}{
		{0, UsageStatusNormal},
		{63, UsageStatusNormal},
		{69.99, UsageStatusNormal},
		{70, UsageStatusHigh},
		{82, UsageStatusHigh},
		{89.99, UsageStatusHigh},
		{90, UsageStatusNearLimit},
		{99.99, UsageStatusNearLimit},
		{100, UsageStatusExhausted},
		{106.5, UsageStatusExhausted},
	}
	for _, c := range cases {
		require.Equal(t, c.want, ClassifyUsageStatus(c.percent), "percent=%v", c.percent)
	}
}

func TestPhase4ClampUsagePercent(t *testing.T) {
	t.Parallel()
	require.InDelta(t, 0.0, ClampUsagePercent(0), 1e-9)
	require.InDelta(t, 63.0, ClampUsagePercent(63), 1e-9)
	require.InDelta(t, 100.0, ClampUsagePercent(100), 1e-9)
	require.InDelta(t, 100.0, ClampUsagePercent(106.5), 1e-9, "overshoot clamps to 100 for users")
	require.InDelta(t, 0.0, ClampUsagePercent(-3), 1e-9)
}

func TestPhase4FormatWalletBalance(t *testing.T) {
	t.Parallel()
	// canonical 账本 NUMERIC(20,8)：字符串 8 位小数保真，避免浮点尾差进入展示
	require.Equal(t, "12.48000000", FormatWalletBalance(12.48))
	require.Equal(t, "100.00000000", FormatWalletBalance(100))
	require.Equal(t, "0.00007813", FormatWalletBalance(0.00007813))
}
