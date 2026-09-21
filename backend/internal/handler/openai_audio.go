package handler

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"strconv"
	"strings"
	"time"

	pkghttputil "github.com/Wei-Shaw/sub2api/internal/pkg/httputil"
	"github.com/Wei-Shaw/sub2api/internal/pkg/ip"
	"github.com/Wei-Shaw/sub2api/internal/pkg/logger"
	middleware2 "github.com/Wei-Shaw/sub2api/internal/server/middleware"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
	"go.uber.org/zap"
)

// openAISpeechGatewayRequest is the OpenAI POST /v1/audio/speech JSON body.
// Only the fields we validate are declared; the raw body bytes are forwarded
// upstream so unknown/optional fields (e.g. voice options) survive untouched.
type openAISpeechGatewayRequest struct {
	Model          string   `json:"model"`
	Input          string   `json:"input"`
	Voice          string   `json:"voice"`
	ResponseFormat string   `json:"response_format"`
	Speed          *float64 `json:"speed"`
}

// Speech handles OpenAI-compatible text-to-speech requests.
// POST /v1/audio/speech
//
// Grok-platform groups are dispatched to the existing GrokVoice handler at the
// route layer; this handler serves PlatformOpenAI (and OpenAI-compatible API
// key accounts) with a simple non-streaming binary passthrough.
func (h *OpenAIGatewayHandler) Speech(c *gin.Context) {
	streamStarted := false
	defer h.recoverResponsesPanic(c, &streamStarted)

	requestStart := time.Now()

	apiKey, ok := middleware2.GetAPIKeyFromContext(c)
	if !ok {
		h.errorResponse(c, http.StatusUnauthorized, "authentication_error", "Invalid API key")
		return
	}

	subject, ok := middleware2.GetAuthSubjectFromContext(c)
	if !ok {
		h.errorResponse(c, http.StatusInternalServerError, "api_error", "User context not found")
		return
	}
	reqLog := requestLogger(
		c,
		"handler.openai_gateway.speech",
		zap.Int64("user_id", subject.UserID),
		zap.Int64("api_key_id", apiKey.ID),
		zap.Any("group_id", apiKey.GroupID),
	)
	if !h.ensureResponsesDependencies(c, reqLog) {
		return
	}

	body, err := pkghttputil.ReadRequestBodyWithPrealloc(c.Request)
	if err != nil {
		if maxErr, ok := extractMaxBytesError(err); ok {
			h.errorResponse(c, http.StatusRequestEntityTooLarge, "invalid_request_error", buildBodyTooLargeMessage(maxErr.Limit))
			return
		}
		h.errorResponse(c, http.StatusBadRequest, "invalid_request_error", "Failed to read request body")
		return
	}
	if len(body) == 0 {
		h.errorResponse(c, http.StatusBadRequest, "invalid_request_error", "Request body is empty")
		return
	}

	var req openAISpeechGatewayRequest
	if err := json.Unmarshal(body, &req); err != nil {
		h.errorResponse(c, http.StatusBadRequest, "invalid_request_error", "Failed to parse request body")
		return
	}
	req.Model = strings.TrimSpace(req.Model)
	if req.Model == "" {
		h.errorResponse(c, http.StatusBadRequest, "invalid_request_error", "model is required")
		return
	}
	if strings.TrimSpace(req.Input) == "" {
		h.errorResponse(c, http.StatusBadRequest, "invalid_request_error", "input is required")
		return
	}
	if req.Speed != nil && (*req.Speed < 0.25 || *req.Speed > 4.0) {
		h.errorResponse(c, http.StatusBadRequest, "invalid_request_error", "speed must be between 0.25 and 4.0")
		return
	}

	setOpsRequestContext(c, req.Model, false)

	requestModel := req.Model
	ensureCompositeTargetPlatform(c, apiKey, requestModel)
	clientRequestModel := clientRequestedModel(c, requestModel)
	routingModel := requestModel
	if resolvedModel, ok := service.ResolvedUpstreamModelFromContext(c.Request.Context()); ok {
		routingModel = resolvedModel
	}
	if !compositeTargetPlatformAllowed(c, apiKey, requestModel, service.PlatformOpenAI) {
		h.errorResponse(c, http.StatusBadRequest, "invalid_request_error", "Model is not supported by this OpenAI-compatible endpoint for composite groups")
		return
	}

	reqLog = reqLog.With(
		zap.String("model", clientRequestModel),
		zap.String("routing_model", routingModel),
	)

	// TTS bodies carry the spoken text in "input". Normalize to chat messages so
	// content moderation extractors see it (same approach as the Grok TTS path).
	auditBody := body
	if b, err := json.Marshal(map[string]any{
		"messages": []map[string]any{{"role": "user", "content": req.Input}},
	}); err == nil {
		auditBody = b
	}
	if decision := h.checkSecurityAudit(c, reqLog, apiKey, subject, service.ContentModerationProtocolOpenAIChat, requestModel, auditBody); decision != nil && !decision.AllowNextStage {
		h.openAISecurityAuditError(c, decision)
		return
	}

	setOpsRequestContext(c, clientRequestModel, false)
	setOpsEndpointContext(c, "", int16(service.RequestTypeFromLegacy(false, false)))

	channelMapping, _ := h.gatewayService.ResolveChannelMappingAndRestrict(c.Request.Context(), apiKey.GroupID, routingModel)

	if h.errorPassthroughService != nil {
		service.BindErrorPassthroughService(c, h.errorPassthroughService)
	}

	subscription, _ := middleware2.GetSubscriptionFromContext(c)

	service.SetOpsLatencyMs(c, service.OpsAuthLatencyMsKey, time.Since(requestStart).Milliseconds())
	routingStart := time.Now()

	userReleaseFunc, acquired := h.acquireResponsesUserSlot(c, subject.UserID, subject.Concurrency, false, &streamStarted, reqLog)
	if !acquired {
		return
	}
	if userReleaseFunc != nil {
		defer userReleaseFunc()
	}

	if err := h.billingCacheService.CheckBillingEligibility(c.Request.Context(), apiKey.User, apiKey, apiKey.Group, subscription, service.QuotaPlatform(c.Request.Context(), apiKey)); err != nil {
		reqLog.Info("openai.speech.billing_eligibility_check_failed", zap.Error(err))
		status, code, message, retryAfter := billingErrorDetails(err)
		if retryAfter > 0 {
			c.Header("Retry-After", strconv.Itoa(retryAfter))
		}
		h.handleStreamingAwareError(c, status, code, message, streamStarted)
		return
	}

	// Account scheduling mirrors GrokVoice: a small bounded retry loop over the
	// scheduler with an excluded set. Speech v1 is apikey passthrough only, so
	// OAuth/setup-token selections are skipped like any other failed candidate.
	// Images' scheduler API is reused unchanged: it selects PlatformOpenAI
	// accounts without imposing a text-endpoint capability filter.
	failed := map[int64]struct{}{}
	var last *service.UpstreamFailoverError
	sawUnsupportedType := false

	for attempts := 0; attempts < 4; attempts++ {
		selection, _, selectErr := h.gatewayService.SelectAccountWithSchedulerForImages(
			c.Request.Context(),
			apiKey.GroupID,
			"",
			routingModel,
			failed,
			service.OpenAIImagesCapabilityBasic,
		)
		if selectErr != nil || selection == nil || selection.Account == nil {
			switch {
			case last != nil:
				h.handleFailoverExhausted(c, last, streamStarted)
			case sawUnsupportedType:
				h.errorResponse(c, http.StatusNotImplemented, "invalid_request_error", "audio/speech is only supported for OpenAI-compatible API key accounts")
			default:
				h.errorResponse(c, http.StatusServiceUnavailable, "api_error", "No available accounts")
			}
			return
		}
		account := selection.Account
		if account.Type != service.AccountTypeAPIKey {
			sawUnsupportedType = true
			failed[account.ID] = struct{}{}
			continue
		}
		setOpsSelectedAccount(c, account.ID, account.Platform)

		var started bool
		release, slotStatus := h.acquireResponsesAccountSlot(c, apiKey.GroupID, "", selection, false, &started, reqLog)
		if slotStatus == openAISlotAcquireProfitVetoed {
			failed[account.ID] = struct{}{}
			continue
		}
		if slotStatus != openAISlotAcquireOK {
			// Slot path already wrote the error response (or transient reject).
			if slotStatus == openAISlotAcquireFailed && len(failed) == 0 {
				return
			}
			failed[account.ID] = struct{}{}
			continue
		}

		service.SetOpsLatencyMs(c, service.OpsRoutingLatencyMsKey, time.Since(routingStart).Milliseconds())
		result, forwardErr := func() (*service.OpenAIForwardResult, error) {
			defer func() {
				if release != nil {
					release()
				}
			}()
			return h.gatewayService.ForwardSpeech(c.Request.Context(), c, account, body, req.Input, requestModel, channelMapping.MappedModel)
		}()
		if forwardErr == nil {
			h.recordSpeechUsage(c, apiKey, account, subscription, req.Input, body, result)
			return
		}
		var failoverErr *service.UpstreamFailoverError
		if errors.As(forwardErr, &failoverErr) && failoverErr.ShouldRetryNextAccount() {
			failed[account.ID] = struct{}{}
			last = failoverErr
			continue
		}
		// Non-failover error: the service already wrote the upstream error
		// response for terminal paths; guarantee a fallback otherwise.
		if !c.Writer.Written() {
			h.ensureForwardErrorResponse(c, streamStarted)
		}
		reqLog.Warn("openai.speech.forward_failed",
			zap.Int64("account_id", account.ID),
			zap.Error(forwardErr),
		)
		return
	}
	if last != nil {
		h.handleFailoverExhausted(c, last, streamStarted)
	} else if sawUnsupportedType {
		h.errorResponse(c, http.StatusNotImplemented, "invalid_request_error", "audio/speech is only supported for OpenAI-compatible API key accounts")
	} else {
		h.errorResponse(c, http.StatusServiceUnavailable, "api_error", "No available accounts")
	}
}

// recordSpeechUsage bills TTS via group audio prices when AudioUsage is set
// (same money-path semantics as recordGrokVoiceUsage).
func (h *OpenAIGatewayHandler) recordSpeechUsage(
	c *gin.Context,
	apiKey *service.APIKey,
	account *service.Account,
	subscription *service.UserSubscription,
	input string,
	body []byte,
	result *service.OpenAIForwardResult,
) {
	if h == nil || c == nil || apiKey == nil || account == nil || result == nil {
		return
	}
	if result.AudioUsage == nil {
		return
	}
	// Ensure forced durable request ids even if callers forget (tts money path).
	result.RequestID = service.StableGrokAudioBillingRequestID(result.RequestID)
	userAgent := c.GetHeader("User-Agent")
	clientIP := ip.GetClientIP(c)
	sessionID := service.ExtractClientSessionID(c)
	requestPayloadHash := service.HashUsageRequestPayload(body)
	if requestPayloadHash == "" {
		requestPayloadHash = service.HashUsageRequestPayload([]byte(input))
	}
	inboundEndpoint := GetInboundEndpoint(c)
	upstreamEndpoint := GetUpstreamEndpoint(c, account.Platform)
	quotaPlatform := service.QuotaPlatform(c.Request.Context(), apiKey)
	model := strings.TrimSpace(result.Model)
	if model == "" {
		model = "tts"
	}

	h.submitMandatoryUsageRecordTask(c.Request.Context(), func(ctx context.Context) {
		if err := h.gatewayService.RecordUsage(ctx, &service.OpenAIRecordUsageInput{
			Result:             result,
			APIKey:             apiKey,
			User:               apiKey.User,
			Account:            account,
			Subscription:       subscription,
			InboundEndpoint:    inboundEndpoint,
			UpstreamEndpoint:   upstreamEndpoint,
			UserAgent:          userAgent,
			IPAddress:          clientIP,
			RequestPayloadHash: requestPayloadHash,
			APIKeyService:      h.apiKeyService,
			QuotaPlatform:      quotaPlatform,
			SessionID:          sessionID,
			ChannelUsageFields: clientRequestedUsageFields(c, service.ChannelMappingResult{}, model, result.UpstreamModel),
		}); err != nil {
			logger.L().With(
				zap.String("component", "handler.openai_gateway.speech"),
				zap.Int64("user_id", apiKey.User.ID),
				zap.Int64("api_key_id", apiKey.ID),
				zap.Any("group_id", apiKey.GroupID),
				zap.String("model", model),
				zap.Int64("account_id", account.ID),
			).Error("speech.record_usage_failed", zap.Error(err))
		}
	})
}
