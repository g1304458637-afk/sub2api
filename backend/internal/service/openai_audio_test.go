//go:build unit

package service

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestEstimateOpenAISpeechAudioUsage(t *testing.T) {
	// 优先使用解析后的 input（按 rune 计字符，百万字符为一个计费单位）
	usage := estimateOpenAISpeechAudioUsage("你好世界", []byte(`{"model":"tts-1","input":"ignored-longer-text"}`))
	require.NotNil(t, usage)
	require.Equal(t, "tts", usage.Mode)
	require.InDelta(t, 4.0/1_000_000.0, usage.DurationOrUnits, 1e-12)

	// input 为空时回退到 JSON input/text/prompt 字段
	usage = estimateOpenAISpeechAudioUsage("", []byte(`{"input":"abc"}`))
	require.NotNil(t, usage)
	require.InDelta(t, 3.0/1_000_000.0, usage.DurationOrUnits, 1e-12)

	// 再回退到原始 body 字节长度
	raw := []byte(`{"x":1}`)
	usage = estimateOpenAISpeechAudioUsage("   ", raw)
	require.NotNil(t, usage)
	require.InDelta(t, float64(len(raw))/1_000_000.0, usage.DurationOrUnits, 1e-18)

	// 完全空 → nil（不记音频用量）
	require.Nil(t, estimateOpenAISpeechAudioUsage("", nil))
}

func TestStableSpeechBillingRequestID(t *testing.T) {
	// Speech 复用 grok_audio forced 前缀，保证 usage_billing_dedup 的
	// forced-id 语义（客户端重用的 request id 不会折叠计费事件）。
	require.True(t, strings.HasPrefix(StableGrokAudioBillingRequestID(""), "grok_audio:"))
	require.Equal(t, "grok_audio:up-1", StableGrokAudioBillingRequestID("up-1"))
	require.Equal(t, "grok_audio:up-1", StableGrokAudioBillingRequestID("grok_audio:up-1"))
}
