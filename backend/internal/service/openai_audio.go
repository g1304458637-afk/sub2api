package service

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/util/responseheaders"
	"github.com/gin-gonic/gin"
	"github.com/tidwall/gjson"
	"github.com/tidwall/sjson"
)

// OpenAI 兼容 /v1/audio/speech（TTS）转发：仅支持 APIKEY 类型账号的直连透传。
// OAuth/ChatGPT 账号没有等价的 TTS 上游端点，转发层显式拒绝。
const (
	openAIAudioSpeechEndpoint = "/v1/audio/speech"
	openAIAudioSpeechURL      = "https://api.openai.com/v1/audio/speech"

	// openAIAudioSpeechMaxResponseBytes 音频二进制响应缓冲上限（与网页端代理一致）。
	openAIAudioSpeechMaxResponseBytes int64 = 32 << 20
)

// ForwardSpeech forwards an OpenAI-shaped POST /v1/audio/speech request to an
// OpenAI-compatible upstream with the account's API key. The response is the
// raw audio bytes; they are passed through to the client unmodified.
//
// billing: TTS is billed per million input characters (same unit semantics as
// the Grok TTS path), carried in OpenAIForwardResult.AudioUsage with mode "tts".
func (s *OpenAIGatewayService) ForwardSpeech(
	ctx context.Context,
	c *gin.Context,
	account *Account,
	body []byte,
	input string,
	requestModel string,
	channelMappedModel string,
) (*OpenAIForwardResult, error) {
	if account == nil {
		return nil, fmt.Errorf("speech account is required")
	}
	// v1 仅支持 APIKEY 直连透传；OAuth/SetupToken 账号由 handler 侧排除，
	// 这里保持同形防御（与 forwardOpenAIImagesAPIKey 的账号类型分发一致）。
	if account.Type != AccountTypeAPIKey {
		return nil, fmt.Errorf("audio/speech only supports %s accounts, got %s", AccountTypeAPIKey, account.Type)
	}
	startTime := time.Now()
	requestModel = strings.TrimSpace(requestModel)
	if mapped := strings.TrimSpace(channelMappedModel); mapped != "" {
		requestModel = mapped
	}
	if requestModel == "" {
		requestModel = strings.TrimSpace(gjson.GetBytes(body, "model").String())
	}
	if requestModel == "" {
		return nil, fmt.Errorf("audio/speech requires a model")
	}
	upstreamModel := account.GetMappedModel(requestModel)
	if upstreamModel == "" {
		upstreamModel = requestModel
	}
	SetOpsUpstreamModel(c, upstreamModel)
	// 沿用 Images JSON 路径的 model 改写：forward body 始终是 JSON。
	forwardBody, err := sjson.SetBytes(body, "model", upstreamModel)
	if err != nil {
		return nil, fmt.Errorf("rewrite speech request model: %w", err)
	}
	// TTS 是上游侧已产生实际成本的耗时操作：客户端中途断开不应连带取消上游请求
	// （与 forwardOpenAIImagesAPIKey / Grok 媒体路径的 detach 语义对齐）。
	upstreamCtx, releaseUpstreamCtx := detachUpstreamContext(ctx)
	defer releaseUpstreamCtx()

	token, _, err := s.GetAccessToken(upstreamCtx, account)
	if err != nil {
		return nil, err
	}
	upstreamReq, err := s.buildOpenAIAudioSpeechRequest(upstreamCtx, c, account, forwardBody, token)
	if err != nil {
		return nil, err
	}

	proxyURL := ""
	if account.ProxyID != nil && account.Proxy != nil {
		proxyURL = account.Proxy.URL()
	}
	upstreamStart := time.Now()
	resp, err := s.doOpenAIUpstream(upstreamReq, proxyURL, account)
	SetOpsLatencyMs(c, OpsUpstreamLatencyMsKey, time.Since(upstreamStart).Milliseconds())
	if err != nil {
		safeErr := sanitizeUpstreamErrorMessage(err.Error())
		setOpsUpstreamError(c, 0, safeErr, "")
		appendOpsUpstreamError(c, OpsUpstreamErrorEvent{
			ProxyID:            opsUpstreamProxyID(account),
			ProxyName:          opsUpstreamProxyName(account),
			Platform:           account.Platform,
			AccountID:          account.ID,
			AccountName:        account.Name,
			UpstreamStatusCode: 0,
			UpstreamURL:        safeUpstreamURL(upstreamReq.URL.String()),
			Kind:               "request_error",
			Message:            safeErr,
		})
		return nil, fmt.Errorf("upstream request failed: %s", safeErr)
	}
	if resp.StatusCode >= 400 {
		respBody := s.readUpstreamErrorBody(resp)
		_ = resp.Body.Close()
		respBody = s.redactAgentIdentitySensitiveBody(upstreamCtx, account, respBody)
		resp.Body = io.NopCloser(bytes.NewReader(respBody))
		upstreamMsg := strings.TrimSpace(extractUpstreamErrorMessage(respBody))
		upstreamMsg = sanitizeUpstreamErrorMessage(upstreamMsg)
		if s.shouldFailoverOpenAIUpstreamResponse(account, resp.StatusCode, upstreamMsg, respBody) {
			appendOpsUpstreamError(c, OpsUpstreamErrorEvent{
				ProxyID:            opsUpstreamProxyID(account),
				ProxyName:          opsUpstreamProxyName(account),
				Platform:           account.Platform,
				AccountID:          account.ID,
				AccountName:        account.Name,
				UpstreamStatusCode: resp.StatusCode,
				UpstreamRequestID:  resp.Header.Get("x-request-id"),
				UpstreamURL:        safeUpstreamURL(upstreamReq.URL.String()),
				Kind:               "failover",
				Message:            upstreamMsg,
			})
			shouldDisable := s.handleFailoverSideEffects(upstreamCtx, resp, account, respBody, upstreamModel)
			retryableOnSameAccount := !shouldDisable && account.IsPoolMode() && account.IsPoolModeRetryableStatus(resp.StatusCode)
			if isOpenAIHTTPUpstreamAccessStateError(resp.StatusCode, upstreamMsg, respBody) {
				return nil, newOpenAIUpstreamFailoverError(resp.StatusCode, resp.Header, respBody, upstreamMsg, retryableOnSameAccount)
			}
			return nil, &UpstreamFailoverError{StatusCode: resp.StatusCode, ResponseBody: respBody, RetryableOnSameAccount: retryableOnSameAccount}
		}
		// 非 failover 错误：沿用 Images 端点的上游错误透传（真实状态码 + 错误体）。
		return s.handleOpenAIImagesErrorResponse(upstreamCtx, resp, c, account, upstreamModel)
	}
	defer func() { _ = resp.Body.Close() }()

	data, err := readUpstreamResponseBodyLimited(resp.Body, openAIAudioSpeechMaxResponseBytes)
	if err != nil {
		if errors.Is(err, ErrUpstreamResponseBodyTooLarge) {
			setOpsUpstreamError(c, http.StatusBadGateway, "upstream response too large", "")
			openAITooLargeError(c)
		}
		return nil, err
	}
	writeOpenAIAudioSpeechResponse(c, resp, data, s.responseHeaderFilter)

	audioUsage := estimateOpenAISpeechAudioUsage(input, body)
	return &OpenAIForwardResult{
		// Forced durable money-event id so usage_billing_dedup cannot collapse
		// under a reused client id (same approach as the Grok voice HTTP path).
		RequestID:       StableGrokAudioBillingRequestID(firstNonEmpty(resp.Header.Get("x-request-id"), resp.Header.Get("xai-request-id"))),
		UpstreamHeaders: resp.Header,
		Model:           requestModel,
		UpstreamModel:   upstreamModel,
		Duration:        time.Since(startTime),
		AudioUsage:      audioUsage,
	}, nil
}

// buildOpenAIAudioSpeechRequest mirrors buildOpenAIImagesRequest for the
// /v1/audio/speech endpoint: custom base URL support, OpenAI auth headers,
// passthrough allowlist and per-account header overrides.
func (s *OpenAIGatewayService) buildOpenAIAudioSpeechRequest(
	ctx context.Context,
	c *gin.Context,
	account *Account,
	body []byte,
	token string,
) (*http.Request, error) {
	targetURL := openAIAudioSpeechURL
	baseURL := account.GetOpenAIBaseURL()
	if baseURL != "" {
		validatedURL, err := s.validateUpstreamBaseURL(baseURL)
		if err != nil {
			return nil, err
		}
		targetURL = buildOpenAIEndpointURL(validatedURL, openAIAudioSpeechEndpoint)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, targetURL, bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	req = req.WithContext(WithHTTPUpstreamProfile(req.Context(), HTTPUpstreamProfileOpenAI))
	authHeaders, err := s.buildOpenAIAuthenticationHeaders(ctx, account, token)
	if err != nil {
		return nil, fmt.Errorf("build openai authentication headers: %w", err)
	}
	for key, values := range authHeaders {
		for _, value := range values {
			req.Header.Add(key, value)
		}
	}
	for key, values := range c.Request.Header {
		if !openaiPassthroughAllowedHeaders[strings.ToLower(key)] {
			continue
		}
		for _, value := range values {
			req.Header.Add(key, value)
		}
	}
	if customUA := account.GetOpenAIUserAgent(); customUA != "" {
		req.Header.Set("User-Agent", customUA)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json, audio/*")
	// 账号级请求头覆写（仅 openai api_key 账号启用时生效）
	account.ApplyHeaderOverrides(req.Header)
	return req, nil
}

// writeOpenAIAudioSpeechResponse passes the raw audio bytes through, keeping the
// upstream Content-Type (default application/octet-stream when missing).
func writeOpenAIAudioSpeechResponse(c *gin.Context, resp *http.Response, body []byte, filter *responseheaders.CompiledHeaderFilter) {
	if c == nil || resp == nil {
		return
	}
	writeOpenAIPassthroughResponseHeaders(c.Writer.Header(), resp.Header, filter)
	contentType := strings.TrimSpace(resp.Header.Get("Content-Type"))
	if contentType == "" {
		contentType = "application/octet-stream"
	}
	c.Data(resp.StatusCode, contentType, body)
}

// estimateOpenAISpeechAudioUsage bills TTS per million input characters,
// mirroring estimateGrokVoiceAudioUsage's tts branch: prefer the parsed input,
// fall back to the JSON input/text/prompt fields, then to the raw body length.
func estimateOpenAISpeechAudioUsage(input string, reqBody []byte) *AudioUsage {
	chars := len([]rune(strings.TrimSpace(input)))
	if chars <= 0 && gjson.ValidBytes(reqBody) {
		for _, key := range []string{"input", "text", "prompt"} {
			if v := strings.TrimSpace(gjson.GetBytes(reqBody, key).String()); v != "" {
				chars = len([]rune(v))
				break
			}
		}
	}
	if chars <= 0 {
		chars = len(reqBody)
	}
	if chars <= 0 {
		return nil
	}
	return &AudioUsage{Mode: "tts", DurationOrUnits: float64(chars) / 1_000_000.0}
}
