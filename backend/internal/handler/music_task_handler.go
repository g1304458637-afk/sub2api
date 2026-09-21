package handler

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"

	infraerrors "github.com/Wei-Shaw/sub2api/internal/pkg/errors"
	pkghttputil "github.com/Wei-Shaw/sub2api/internal/pkg/httputil"
	"github.com/Wei-Shaw/sub2api/internal/pkg/ip"
	"github.com/Wei-Shaw/sub2api/internal/pkg/logger"
	middleware2 "github.com/Wei-Shaw/sub2api/internal/server/middleware"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
	"go.uber.org/zap"
)

// defaultMusicDownloadHTTPClient bounds a single audio download; callers cap
// the accepted size separately.
func defaultMusicDownloadHTTPClient() *http.Client {
	return &http.Client{Timeout: 120 * time.Second}
}

const (
	// maxMusicAudioDownloadBytes caps the upstream audio download so the gateway
	// can self-host the clip (upstream CDN links expire).
	maxMusicAudioDownloadBytes int64 = 20 << 20 // 20 MiB
	// maxMusicInlineBase64Bytes caps raw audio stored inline in Redis when no
	// object storage is configured; larger clips fail instead of ballooning Redis.
	maxMusicInlineBase64Bytes int64 = 10 << 20 // 10 MiB
)

// AsyncMusicHandler accepts music generation requests as asynchronous tasks and
// serves owner-bound polling. Unlike async image tasks, execution calls the
// provider adapter directly (no httptest replay) and object storage is an
// enhancement, not an enablement gate: clips fall back to inline base64.
type AsyncMusicHandler struct {
	tasks   *service.MusicTaskService
	openAI  *OpenAIGatewayHandler
	execute func(ctx context.Context, job *musicTaskJob) *musicExecutionOutput
}

func NewAsyncMusicHandler(tasks *service.MusicTaskService, openAI *OpenAIGatewayHandler) *AsyncMusicHandler {
	h := &AsyncMusicHandler{tasks: tasks, openAI: openAI}
	h.execute = h.executeGeneration
	return h
}

// musicSubmitRequest is the POST /v1/audio/music body.
type musicSubmitRequest struct {
	Model        string `json:"model"`
	Prompt       string `json:"prompt"`
	Lyrics       string `json:"lyrics"`
	Instrumental bool   `json:"instrumental"`
	Seconds      int    `json:"seconds"`
	Format       string `json:"format"`
}

// musicTaskJob carries everything the detached execution goroutine needs after
// the submit request has returned.
type musicTaskJob struct {
	TaskID       string
	ProviderReq  service.MusicGenerationRequest
	APIKey       *service.APIKey
	Subscription *service.UserSubscription
	Usage        musicUsageMeta
	StartedAt    time.Time
}

// musicUsageMeta snapshots the request-scoped fields used by the usage record.
type musicUsageMeta struct {
	InboundEndpoint    string
	UpstreamEndpoint   string
	UserAgent          string
	IPAddress          string
	SessionID          string
	RequestPayloadHash string
	QuotaPlatform      string
	ChannelFields      service.ChannelUsageFields
}

// musicExecutionOutput is the terminal outcome of one execution attempt.
type musicExecutionOutput struct {
	HTTPStatus int
	Result     json.RawMessage
	Err        json.RawMessage
	Account    *service.Account
}

// enabled reports whether music task submission is available (store reachable).
func (h *AsyncMusicHandler) enabled() bool {
	return h != nil && h.tasks != nil && h.tasks.Enabled()
}

// pollable reports whether task lookups can be served.
func (h *AsyncMusicHandler) pollable() bool {
	return h != nil && h.tasks != nil && h.tasks.Pollable()
}

// Submit validates and accepts a music generation request, returns 202 with the
// task envelope, and executes the generation in a detached goroutine.
func (h *AsyncMusicHandler) Submit(c *gin.Context) {
	if !h.enabled() {
		musicTaskJSONError(c, http.StatusNotFound, "not_found_error", "music tasks are not enabled")
		return
	}
	apiKey, ok := middleware2.GetAPIKeyFromContext(c)
	if !ok || apiKey == nil || apiKey.UserID <= 0 || apiKey.ID <= 0 {
		musicTaskError(c, service.ErrMusicTaskForbidden)
		return
	}
	if apiKey.GroupID == nil {
		// Account selection (and the group price config) need a group; the music
		// route chain does not carry RequireGroupAssignment, so guard here.
		musicTaskJSONError(c, http.StatusForbidden, "no_group_assigned", "API key is not assigned to a group")
		return
	}
	if h == nil || h.tasks == nil || h.execute == nil {
		musicTaskError(c, service.ErrMusicTaskUnavailable)
		return
	}

	body, err := pkghttputil.ReadRequestBodyWithPrealloc(c.Request)
	if err != nil {
		if maxErr, ok := extractMaxBytesError(err); ok {
			musicTaskJSONError(c, http.StatusRequestEntityTooLarge, "invalid_request_error", buildBodyTooLargeMessage(maxErr.Limit))
			return
		}
		musicTaskJSONError(c, http.StatusBadRequest, "invalid_request_error", "Failed to read request body")
		return
	}
	if len(body) == 0 {
		musicTaskJSONError(c, http.StatusBadRequest, "invalid_request_error", "Request body is empty")
		return
	}
	var req musicSubmitRequest
	if err := json.Unmarshal(body, &req); err != nil {
		musicTaskJSONError(c, http.StatusBadRequest, "invalid_request_error", "Request body is not valid JSON")
		return
	}
	req.Model = strings.TrimSpace(req.Model)
	req.Prompt = strings.TrimSpace(req.Prompt)
	if req.Model == "" {
		musicTaskJSONError(c, http.StatusBadRequest, "invalid_request_error", "model is required")
		return
	}
	if req.Prompt == "" {
		musicTaskJSONError(c, http.StatusBadRequest, "invalid_request_error", "prompt is required")
		return
	}

	subscription, _ := middleware2.GetSubscriptionFromContext(c)
	if h.openAI == nil || h.openAI.billingCacheService == nil {
		musicTaskError(c, service.ErrMusicTaskUnavailable)
		return
	}
	{
		eligibility, err := h.openAI.billingCacheService.CheckBillingEligibility(c.Request.Context(), apiKey.User, apiKey, apiKey.Group, subscription, service.QuotaPlatform(c.Request.Context(), apiKey))
		if err != nil {
			status, code, message, retryAfter := billingErrorDetails(err)
			if retryAfter > 0 {
				c.Header("Retry-After", strconv.Itoa(retryAfter))
			}
			musicTaskJSONError(c, status, code, message)
			return
		}
		subscription = eligibility.SubscriptionForBilling(subscription)
	}

	usage := musicUsageMeta{
		InboundEndpoint:    GetInboundEndpoint(c),
		UpstreamEndpoint:   GetUpstreamEndpoint(c, service.PlatformOpenAI),
		UserAgent:          c.GetHeader("User-Agent"),
		IPAddress:          ip.GetClientIP(c),
		SessionID:          service.ExtractClientSessionID(c),
		RequestPayloadHash: service.HashUsageRequestPayload(body),
		QuotaPlatform:      service.QuotaPlatform(c.Request.Context(), apiKey),
		ChannelFields:      clientRequestedUsageFields(c, service.ChannelMappingResult{}, req.Model, req.Model),
	}

	taskCtx, cancel := context.WithTimeout(context.WithoutCancel(c.Request.Context()), h.tasks.ExecutionTimeout())
	task, err := h.tasks.Create(c.Request.Context(), service.MusicTaskOwner{UserID: apiKey.UserID, APIKeyID: apiKey.ID})
	if err != nil {
		cancel()
		musicTaskError(c, err)
		return
	}

	pollURL := "/v1/audio/music/tasks/" + task.ID
	c.Header("Cache-Control", "no-store")
	c.Header("Location", pollURL)
	c.Header("Retry-After", "3")
	c.JSON(http.StatusAccepted, gin.H{
		"id":         task.ID,
		"task_id":    task.TaskID,
		"object":     task.Object,
		"status":     task.Status,
		"created_at": task.CreatedAt,
		"expires_at": task.ExpiresAt,
		"poll_url":   pollURL,
	})

	job := &musicTaskJob{
		TaskID:       task.ID,
		ProviderReq:  service.MusicGenerationRequest(req),
		APIKey:       apiKey,
		Subscription: subscription,
		Usage:        usage,
		StartedAt:    time.Now(),
	}
	go h.run(job, taskCtx, cancel)
}

// Get serves owner-bound polling: the polling key must match the creating user
// AND API key, otherwise the task reads as not-found (IDs are not enumerable).
func (h *AsyncMusicHandler) Get(c *gin.Context) {
	if !h.pollable() {
		musicTaskJSONError(c, http.StatusNotFound, "not_found_error", "music tasks are not enabled")
		return
	}
	apiKey, ok := middleware2.GetAPIKeyFromContext(c)
	if !ok || apiKey == nil || apiKey.UserID <= 0 || apiKey.ID <= 0 {
		musicTaskError(c, service.ErrMusicTaskForbidden)
		return
	}
	task, err := h.tasks.Get(c.Request.Context(), service.MusicTaskOwner{UserID: apiKey.UserID, APIKeyID: apiKey.ID}, c.Param("task_id"))
	if err != nil {
		musicTaskError(c, err)
		return
	}
	c.Header("Cache-Control", "no-store")
	if task.Status == service.MusicTaskStatusPending || task.Status == service.MusicTaskStatusRunning {
		c.Header("Retry-After", "3")
	}
	c.JSON(http.StatusOK, task)
}

// run interprets the execution outcome: completes/fails the task and bills on
// success only.
func (h *AsyncMusicHandler) run(job *musicTaskJob, execCtx context.Context, cancel context.CancelFunc) {
	defer cancel()
	defer func() {
		if recovered := recover(); recovered != nil {
			logger.L().Error("music_task.execution_panicked", zap.String("task_id", job.TaskID), zap.Any("panic", recovered))
			h.failTask(job.TaskID, http.StatusInternalServerError, musicTaskErrorPayload("api_error", "music generation task panicked"))
		}
	}()

	_ = h.tasks.MarkRunning(context.Background(), job.TaskID)
	out := h.execute(execCtx, job)
	if out == nil {
		out = &musicExecutionOutput{HTTPStatus: http.StatusInternalServerError, Err: musicTaskErrorPayload("api_error", "music generation produced no outcome")}
	}
	if execCtx.Err() != nil && out.Result == nil {
		// The execution window expired without a usable result.
		h.failTask(job.TaskID, http.StatusGatewayTimeout, musicTaskErrorPayload("timeout_error", "music generation task timed out"))
		return
	}
	if out.HTTPStatus >= http.StatusOK && out.HTTPStatus < http.StatusMultipleChoices && len(out.Result) > 0 {
		if err := h.tasks.Complete(context.Background(), job.TaskID, out.HTTPStatus, out.Result); err != nil {
			logger.L().Error("music_task.complete_store_failed", zap.String("task_id", job.TaskID), zap.Error(err))
			return
		}
		h.recordMusicUsage(job, out)
		return
	}
	status := out.HTTPStatus
	if status < 400 {
		status = http.StatusBadGateway
	}
	taskErr := out.Err
	if len(taskErr) == 0 {
		taskErr = musicTaskErrorPayload("api_error", "music generation failed")
	}
	h.failTask(job.TaskID, status, taskErr)
}

// executeGeneration resolves an account, drives the provider adapter, downloads
// and persists the audio, and produces the task result payload.
func (h *AsyncMusicHandler) executeGeneration(ctx context.Context, job *musicTaskJob) *musicExecutionOutput {
	if h.openAI == nil || h.openAI.gatewayService == nil {
		return &musicExecutionOutput{HTTPStatus: http.StatusServiceUnavailable, Err: musicTaskErrorPayload("api_error", "music gateway is unavailable")}
	}
	factory, ok := service.MusicProviderForModel(job.ProviderReq.Model)
	if !ok {
		return &musicExecutionOutput{
			HTTPStatus: http.StatusBadRequest,
			Err:        musicTaskErrorPayload("invalid_request_error", "model "+job.ProviderReq.Model+" is not a supported music generation model"),
		}
	}

	// Reuse the closest existing scheduler: an openai-platform apikey account
	// carries {base_url, access_token} credentials the adapters consume.
	selection, _, selectErr := h.openAI.gatewayService.SelectAccountWithSchedulerForCapability(
		ctx,
		job.APIKey.GroupID,
		"",
		"",
		job.ProviderReq.Model,
		nil,
		service.OpenAIUpstreamTransportHTTPSSE,
		service.OpenAIEndpointCapabilityChatCompletions,
		false,
		false,
		false,
		service.PlatformOpenAI,
	)
	if selectErr != nil || selection == nil || selection.Account == nil {
		return &musicExecutionOutput{
			HTTPStatus: http.StatusServiceUnavailable,
			Err:        musicTaskErrorPayload("api_error", "no available music generation accounts"),
		}
	}
	account := selection.Account

	provider := factory(account.GetOpenAIBaseURL(), account.GetOpenAIAccessToken(), nil)
	result, err := provider.Generate(ctx, job.ProviderReq)
	if err != nil {
		return &musicExecutionOutput{
			HTTPStatus: http.StatusBadGateway,
			Err:        musicTaskErrorPayload("api_error", err.Error()),
		}
	}
	if audioBase64 := strings.TrimSpace(result.AudioBase64); audioBase64 != "" {
		return &musicExecutionOutput{
			HTTPStatus: http.StatusOK,
			Result:     buildMusicTaskResult(result, audioBase64, "", job.ProviderReq.Model),
			Account:    account,
		}
	}
	data, contentType, err := h.downloadAudio(ctx, result.AudioURL)
	if err != nil {
		return &musicExecutionOutput{
			HTTPStatus: http.StatusBadGateway,
			Err:        musicTaskErrorPayload("api_error", err.Error()),
		}
	}
	if contentType != "" {
		result.ContentType = contentType
	}
	storeErr := h.persistAudio(ctx, job.TaskID, result, data)
	if storeErr != nil {
		return &musicExecutionOutput{
			HTTPStatus: http.StatusInternalServerError,
			Err:        musicTaskErrorPayload("api_error", storeErr.Error()),
		}
	}
	return &musicExecutionOutput{
		HTTPStatus: http.StatusOK,
		Result:     buildMusicTaskResult(result, result.AudioBase64, result.AudioURL, job.ProviderReq.Model),
		Account:    account,
	}
}

// downloadAudio self-hosts upstream audio: CDN links expire, bytes do not.
func (h *AsyncMusicHandler) downloadAudio(ctx context.Context, rawURL string) ([]byte, string, error) {
	rawURL = strings.TrimSpace(rawURL)
	if rawURL == "" {
		return nil, "", errors.New("upstream returned no audio url")
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, rawURL, nil)
	if err != nil {
		return nil, "", errors.New("build audio download request failed")
	}
	resp, err := defaultMusicDownloadHTTPClient().Do(req)
	if err != nil {
		return nil, "", errors.New("download generated audio failed")
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode < http.StatusOK || resp.StatusCode >= http.StatusMultipleChoices {
		return nil, "", errors.New("download generated audio failed with status " + strconv.Itoa(resp.StatusCode))
	}
	data, err := io.ReadAll(io.LimitReader(resp.Body, maxMusicAudioDownloadBytes+1))
	if err != nil {
		return nil, "", errors.New("read generated audio failed")
	}
	if int64(len(data)) > maxMusicAudioDownloadBytes {
		return nil, "", errors.New("generated audio exceeds the 20MB download limit")
	}
	contentType := strings.TrimSpace(strings.Split(resp.Header.Get("Content-Type"), ";")[0])
	return data, contentType, nil
}

// persistAudio prefers object storage and falls back to inline base64 when no
// uploader is enabled. Oversized clips without object storage fail the task
// rather than inflating Redis.
func (h *AsyncMusicHandler) persistAudio(ctx context.Context, taskID string, result *service.MusicGenerationResult, data []byte) error {
	if uploader, enabled := h.tasks.CurrentUploader(); enabled && uploader != nil {
		key := "music/" + taskID + musicExtensionForContentType(result.ContentType)
		url, err := uploader.Save(ctx, key, result.ContentType, data)
		if err != nil {
			return errors.New("failed to store generated audio to object storage")
		}
		result.AudioURL = url
		result.AudioBase64 = ""
		return nil
	}
	if int64(len(data)) > maxMusicInlineBase64Bytes {
		return errors.New("generated audio exceeds the 10MB inline limit and object storage is not configured")
	}
	result.AudioBase64 = base64.StdEncoding.EncodeToString(data)
	result.AudioURL = ""
	return nil
}

// recordMusicUsage bills one track via the group music price on success only.
func (h *AsyncMusicHandler) recordMusicUsage(job *musicTaskJob, out *musicExecutionOutput) {
	if h.openAI == nil || h.openAI.gatewayService == nil || job == nil || job.APIKey == nil || out.Account == nil {
		return
	}
	// Forced durable money-event id anchored on the immutable task id.
	result := &service.OpenAIForwardResult{
		RequestID: service.StableMusicBillingRequestID(job.TaskID),
		Model:     job.ProviderReq.Model,
		Duration:  time.Since(job.StartedAt),
		AudioUsage: &service.AudioUsage{
			Mode:            "music",
			DurationOrUnits: 1,
		},
	}
	usage := job.Usage
	h.openAI.submitMandatoryUsageRecordTask(context.Background(), service.UsageRecordTask(func(ctx context.Context) {
		if err := h.openAI.gatewayService.RecordUsage(ctx, &service.OpenAIRecordUsageInput{
			Result:             result,
			APIKey:             job.APIKey,
			User:               job.APIKey.User,
			Account:            out.Account,
			Subscription:       job.Subscription,
			InboundEndpoint:    usage.InboundEndpoint,
			UpstreamEndpoint:   usage.UpstreamEndpoint,
			UserAgent:          usage.UserAgent,
			IPAddress:          usage.IPAddress,
			RequestPayloadHash: usage.RequestPayloadHash,
			APIKeyService:      h.openAI.apiKeyService,
			QuotaPlatform:      usage.QuotaPlatform,
			SessionID:          usage.SessionID,
			ChannelUsageFields: usage.ChannelFields,
		}); err != nil {
			logger.L().With(
				zap.String("component", "handler.async_music"),
				zap.Int64("user_id", job.APIKey.UserID),
				zap.Int64("api_key_id", job.APIKey.ID),
				zap.String("task_id", job.TaskID),
			).Error("music_task.record_usage_failed", zap.Error(err))
		}
	}))
}

func (h *AsyncMusicHandler) failTask(taskID string, statusCode int, taskErr json.RawMessage) {
	if err := h.tasks.Fail(context.Background(), taskID, statusCode, taskErr); err != nil {
		logger.L().Error("music_task.failure_store_failed", zap.String("task_id", taskID), zap.Error(err))
	}
}

// buildMusicTaskResult assembles the public result JSON:
// {audio_url?, audio_base64?, content_type?, duration_sec?, format?, model?}.
func buildMusicTaskResult(result *service.MusicGenerationResult, audioBase64, audioURL, fallbackModel string) json.RawMessage {
	payload := gin.H{
		"content_type": result.ContentType,
		"duration_sec": result.DurationSec,
		"format":       result.Format,
		"model":        firstNonEmptyString(result.Model, fallbackModel),
	}
	if url := strings.TrimSpace(firstNonEmptyString(result.AudioURL, audioURL)); url != "" {
		payload["audio_url"] = url
	}
	if b64 := strings.TrimSpace(firstNonEmptyString(result.AudioBase64, audioBase64)); b64 != "" {
		payload["audio_base64"] = b64
	}
	data, _ := json.Marshal(payload)
	return data
}

func musicTaskErrorPayload(errorType, message string) json.RawMessage {
	data, _ := json.Marshal(gin.H{"type": errorType, "message": message})
	return data
}

func musicTaskError(c *gin.Context, err error) {
	status := infraerrors.Code(err)
	code := infraerrors.Reason(err)
	message := infraerrors.Message(err)
	if status <= 0 {
		status = http.StatusInternalServerError
	}
	if strings.TrimSpace(code) == "" {
		code = "MUSIC_TASK_ERROR"
	}
	musicTaskJSONError(c, status, code, message)
}

func musicTaskJSONError(c *gin.Context, status int, code, message string) {
	c.Header("Cache-Control", "no-store")
	c.JSON(status, gin.H{"error": gin.H{"type": code, "code": code, "message": message}})
}

func musicExtensionForContentType(contentType string) string {
	switch strings.ToLower(strings.TrimSpace(strings.Split(contentType, ";")[0])) {
	case "audio/wav", "audio/x-wav":
		return ".wav"
	case "audio/ogg":
		return ".ogg"
	case "audio/flac":
		return ".flac"
	case "audio/mp4":
		return ".m4a"
	default:
		return ".mp3"
	}
}
