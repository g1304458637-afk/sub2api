package service

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

const (
	// sunoGeneratePath creates a generation job; sunoRecordInfoPath polls it.
	sunoGeneratePath   = "/api/v1/generate"
	sunoRecordInfoPath = "/api/v1/generate/record-info"

	// sunoPollInterval is the fixed polling cadence for record-info.
	sunoPollInterval = 5 * time.Second

	// defaultMusicAudioFormat is what Suno relays produce unless told otherwise.
	defaultMusicAudioFormat = "mp3"
)

// SunoAdapter talks to a Suno-style relay exposed through an OpenAI-protocol
// apikey account (base_url + access token credentials).
type SunoAdapter struct {
	baseURL      string
	apiKey       string
	httpClient   *http.Client
	pollInterval time.Duration
}

// NewSunoAdapter builds the adapter; httpClient nil falls back to the default.
// It matches MusicProviderFactory.
func NewSunoAdapter(baseURL, apiKey string, httpClient *http.Client) MusicProvider {
	return &SunoAdapter{
		baseURL:      strings.TrimRight(strings.TrimSpace(baseURL), "/"),
		apiKey:       strings.TrimSpace(apiKey),
		httpClient:   defaultMusicProviderHTTPClientOn(httpClient),
		pollInterval: sunoPollInterval,
	}
}

func defaultMusicProviderHTTPClientOn(client *http.Client) *http.Client {
	if client != nil {
		return client
	}
	return defaultMusicProviderHTTPClient()
}

type sunoGenerateResponse struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
	Data    struct {
		TaskID string `json:"taskId"`
	} `json:"data"`
}

type sunoRecordInfoResponse struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
	Data    struct {
		Status        string `json:"status"`
		TaskStatus    string `json:"taskStatus"`
		FailMsg       string `json:"failMsg"`
		ErrorResponse *struct {
			Message string `json:"message"`
		} `json:"error_response"`
		Response struct {
			SunoData []struct {
				AudioURL   string  `json:"audio_url"`
				AudioURL2  string  `json:"audioUrl"`
				SourceType string  `json:"source_type"`
				Duration   float64 `json:"duration"`
				ModelName  string  `json:"model_name"`
			} `json:"sunoData"`
		} `json:"response"`
	} `json:"data"`
}

// Generate submits the generation job and polls record-info until the clip is
// ready, the upstream reports a failure, or ctx expires (the handler bounds the
// whole window at 10 minutes).
func (s *SunoAdapter) Generate(ctx context.Context, req MusicGenerationRequest) (result *MusicGenerationResult, err error) {
	// The overall generation deadline also covers in-flight HTTP requests and
	// body reads, not just the wait between polls.
	defer func() {
		if err != nil && ctx.Err() == context.DeadlineExceeded {
			err = &MusicProviderError{Message: "music generation timed out"}
		}
	}()
	taskID, err := s.createTask(ctx, req)
	if err != nil {
		return nil, err
	}
	return s.awaitResult(ctx, taskID, req)
}

func (s *SunoAdapter) createTask(ctx context.Context, req MusicGenerationRequest) (string, error) {
	payload := map[string]any{
		"prompt":       req.Prompt,
		"customMode":   false,
		"instrumental": req.Instrumental,
		"model":        req.Model,
		// callBackUrl deliberately omitted: results are pulled via record-info.
	}
	if strings.TrimSpace(req.Lyrics) != "" {
		payload["lyrics"] = req.Lyrics
	}
	if req.Seconds > 0 {
		payload["seconds"] = req.Seconds
	}
	body, err := json.Marshal(payload)
	if err != nil {
		return "", fmt.Errorf("encode suno generate request: %w", err)
	}
	raw, err := s.doJSON(ctx, http.MethodPost, s.baseURL+sunoGeneratePath, body)
	if err != nil {
		return "", err
	}
	var parsed sunoGenerateResponse
	if err := json.Unmarshal(raw, &parsed); err != nil {
		return "", &MusicProviderError{Message: "suno upstream returned a non-JSON generate response"}
	}
	if parsed.Code != 200 {
		return "", &MusicProviderError{Message: firstNonEmpty(parsed.Message, "suno generate request failed")}
	}
	taskID := strings.TrimSpace(parsed.Data.TaskID)
	if taskID == "" {
		return "", &MusicProviderError{Message: "suno generate response is missing taskId"}
	}
	return taskID, nil
}

func (s *SunoAdapter) awaitResult(ctx context.Context, taskID string, req MusicGenerationRequest) (*MusicGenerationResult, error) {
	interval := s.pollInterval
	if interval <= 0 {
		interval = sunoPollInterval
	}
	for {
		record, err := s.fetchRecordInfo(ctx, taskID)
		if err != nil {
			return nil, err
		}
		switch sunoStatusOf(record) {
		case "SUCCESS":
			if len(record.Data.Response.SunoData) == 0 {
				return nil, &MusicProviderError{Message: "suno upstream reported SUCCESS without audio data"}
			}
			clip := record.Data.Response.SunoData[0]
			audioURL := strings.TrimSpace(clip.AudioURL)
			if audioURL == "" {
				audioURL = strings.TrimSpace(clip.AudioURL2)
			}
			if audioURL == "" {
				return nil, &MusicProviderError{Message: "suno upstream returned no audio url"}
			}
			format := strings.TrimSpace(req.Format)
			if format == "" {
				format = defaultMusicAudioFormat
			}
			model := strings.TrimSpace(clip.ModelName)
			if model == "" {
				model = strings.TrimSpace(req.Model)
			}
			return &MusicGenerationResult{
				AudioURL:    audioURL,
				ContentType: musicContentTypeForFormat(format),
				DurationSec: clip.Duration,
				Format:      format,
				Model:       model,
			}, nil
		case "CREATE_TASK_FAILED", "GENERATE_AUDIO_FAILED", "SENSITIVE_WORD_ERROR":
			failMsg := record.Data.FailMsg
			if record.Data.ErrorResponse != nil {
				failMsg = firstNonEmpty(failMsg, record.Data.ErrorResponse.Message)
			}
			msg := firstNonEmpty(failMsg, record.Message, "suno generation failed")
			return nil, &MusicProviderError{Message: msg}
		default:
			// PENDING / TEXT_SUCCESS / FIRST_SUCCESS / unknown relay status: keep polling.
		}
		select {
		case <-ctx.Done():
			return nil, &MusicProviderError{Message: "music generation timed out"}
		case <-time.After(interval):
		}
	}
}

func sunoStatusOf(record *sunoRecordInfoResponse) string {
	status := strings.ToUpper(strings.TrimSpace(record.Data.Status))
	if status == "" {
		status = strings.ToUpper(strings.TrimSpace(record.Data.TaskStatus))
	}
	return status
}

func (s *SunoAdapter) fetchRecordInfo(ctx context.Context, taskID string) (*sunoRecordInfoResponse, error) {
	target := s.baseURL + sunoRecordInfoPath + "?taskId=" + url.QueryEscape(taskID)
	raw, err := s.doJSON(ctx, http.MethodGet, target, nil)
	if err != nil {
		return nil, err
	}
	var parsed sunoRecordInfoResponse
	if err := json.Unmarshal(raw, &parsed); err != nil {
		return nil, &MusicProviderError{Message: "suno upstream returned a non-JSON record-info response"}
	}
	if parsed.Code != 200 {
		return nil, &MusicProviderError{Message: firstNonEmpty(parsed.Message, fmt.Sprintf("suno record-info failed with code %d", parsed.Code))}
	}
	return &parsed, nil
}

func (s *SunoAdapter) doJSON(ctx context.Context, method, target string, body []byte) ([]byte, error) {
	var reader io.Reader
	if body != nil {
		reader = bytes.NewReader(body)
	}
	req, err := http.NewRequestWithContext(ctx, method, target, reader)
	if err != nil {
		return nil, fmt.Errorf("build suno request: %w", err)
	}
	req.Header.Set("Accept", "application/json")
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	if s.apiKey != "" {
		req.Header.Set("Authorization", "Bearer "+s.apiKey)
	}
	resp, err := s.httpClient.Do(req)
	if err != nil {
		return nil, &MusicProviderError{Message: "suno upstream is unreachable: " + err.Error()}
	}
	defer func() { _ = resp.Body.Close() }()
	data, err := io.ReadAll(io.LimitReader(resp.Body, 8<<20))
	if err != nil {
		return nil, &MusicProviderError{Message: "read suno upstream response: " + err.Error()}
	}
	if resp.StatusCode < http.StatusOK || resp.StatusCode >= http.StatusMultipleChoices {
		msg := strings.TrimSpace(string(bytes.TrimSpace(data)))
		if len(msg) > 512 {
			msg = msg[:512]
		}
		return nil, &MusicProviderError{Message: fmt.Sprintf("suno upstream returned status %d: %s", resp.StatusCode, msg)}
	}
	return data, nil
}

// musicContentTypeForFormat maps an audio container name to its MIME type.
func musicContentTypeForFormat(format string) string {
	switch strings.ToLower(strings.TrimSpace(format)) {
	case "wav":
		return "audio/wav"
	case "ogg":
		return "audio/ogg"
	case "flac":
		return "audio/flac"
	case "mp4", "m4a":
		return "audio/mp4"
	default:
		return "audio/mpeg"
	}
}
