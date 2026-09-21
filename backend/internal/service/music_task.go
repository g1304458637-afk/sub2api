package service

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"strings"
	"time"

	infraerrors "github.com/Wei-Shaw/sub2api/internal/pkg/errors"
	"github.com/google/uuid"
)

const (
	MusicTaskStatusPending   = "pending"
	MusicTaskStatusRunning   = "running"
	MusicTaskStatusSucceeded = "succeeded"
	MusicTaskStatusFailed    = "failed"

	defaultMusicTaskTTL              = 24 * time.Hour
	defaultMusicTaskExecutionTimeout = 10 * time.Minute
)

var (
	ErrMusicTaskNotFound    = infraerrors.New(http.StatusNotFound, "MUSIC_TASK_NOT_FOUND", "music task not found")
	ErrMusicTaskForbidden   = infraerrors.New(http.StatusForbidden, "MUSIC_TASK_FORBIDDEN", "music task does not belong to this API key")
	ErrMusicTaskUnavailable = infraerrors.New(http.StatusServiceUnavailable, "MUSIC_TASK_UNAVAILABLE", "music task storage is unavailable")
)

// MusicTaskRecord is the private Redis representation of an asynchronous music
// generation request. Ownership fields are intentionally omitted from the
// public view. Result carries the finished audio:
// {audio_url?, audio_base64?, content_type?, duration_sec?, format?, model?}.
type MusicTaskRecord struct {
	ID          string          `json:"id"`
	UserID      int64           `json:"user_id"`
	APIKeyID    int64           `json:"api_key_id"`
	Status      string          `json:"status"`
	HTTPStatus  int             `json:"http_status,omitempty"`
	Result      json.RawMessage `json:"result,omitempty"`
	Error       json.RawMessage `json:"error,omitempty"`
	CreatedAt   int64           `json:"created_at"`
	CompletedAt *int64          `json:"completed_at,omitempty"`
	ExpiresAt   int64           `json:"expires_at"`
}

// MusicTask is the API-safe task representation returned to callers.
// 成果字段按契约扁平暴露（audio_url | audio_base64、duration_sec、format、model），
// 内部嵌套的 Result 不外泄。
type MusicTask struct {
	ID          string          `json:"id"`
	TaskID      string          `json:"task_id"`
	Object      string          `json:"object"`
	Status      string          `json:"status"`
	HTTPStatus  int             `json:"http_status,omitempty"`
	AudioURL    string          `json:"audio_url,omitempty"`
	AudioBase64 string          `json:"audio_base64,omitempty"`
	DurationSec float64         `json:"duration_sec,omitempty"`
	Format      string          `json:"format,omitempty"`
	Model       string          `json:"model,omitempty"`
	Error       json.RawMessage `json:"error,omitempty"`
	CreatedAt   int64           `json:"created_at"`
	CompletedAt *int64          `json:"completed_at,omitempty"`
	ExpiresAt   int64           `json:"expires_at"`
}

type MusicTaskOwner struct {
	UserID   int64
	APIKeyID int64
}

type MusicTaskStore interface {
	Save(ctx context.Context, task *MusicTaskRecord, ttl time.Duration) error
	Get(ctx context.Context, id string) (*MusicTaskRecord, error)
}

// MusicTaskService tracks asynchronous music generation tasks in Redis.
//
// Unlike async image tasks, object storage is NOT the enablement gate: without
// an uploader the finished audio falls back to inline base64 (capped), so the
// feature works on every deployment with a reachable Redis.
type MusicTaskService struct {
	store            MusicTaskStore
	uploader         *ImageResultUploader
	resolve          ImageStorageResolver
	ttl              time.Duration
	executionTimeout time.Duration
}

func NewMusicTaskService(store MusicTaskStore) *MusicTaskService {
	return NewMusicTaskServiceWithOptions(store, defaultMusicTaskTTL, defaultMusicTaskExecutionTimeout)
}

func NewMusicTaskServiceWithOptions(store MusicTaskStore, ttl, executionTimeout time.Duration) *MusicTaskService {
	if ttl <= 0 {
		ttl = defaultMusicTaskTTL
	}
	if executionTimeout <= 0 {
		executionTimeout = defaultMusicTaskExecutionTimeout
	}
	return &MusicTaskService{store: store, ttl: ttl, executionTimeout: executionTimeout}
}

// NewMusicTaskServiceWithResolver 构造一个由 resolver 决定对象存储转存的服务：
// 上传器可用时结果转存对象存储并只保留 audio_url；不可用时回退内联 base64。
// 与异步生图不同，转存是增强而非开关——没有对象存储功能依然可用。
func NewMusicTaskServiceWithResolver(store MusicTaskStore, resolve ImageStorageResolver, ttl, executionTimeout time.Duration) *MusicTaskService {
	s := NewMusicTaskServiceWithOptions(store, ttl, executionTimeout)
	s.resolve = resolve
	return s
}

// current 返回当前生效的 uploader 与启用状态（与 ImageTaskService 同一约定：
// 注入 resolver 时以后台设置为准，可热切换）。
func (s *MusicTaskService) current() (*ImageResultUploader, bool) {
	if s == nil {
		return nil, false
	}
	if s.resolve != nil {
		return s.resolve()
	}
	return s.uploader, s.uploader != nil
}

// CurrentUploader 暴露当前生效的对象存储上传器，供执行路径转存音频。
func (s *MusicTaskService) CurrentUploader() (*ImageResultUploader, bool) {
	return s.current()
}

// Enabled 表示音乐任务功能是否可用（仅需任务存储可达）。
func (s *MusicTaskService) Enabled() bool {
	return s != nil && s.store != nil
}

// Pollable 与 Enabled 等价（音乐没有额外的存储开关），保留命名与生图对齐。
func (s *MusicTaskService) Pollable() bool {
	return s != nil && s.store != nil
}

func (s *MusicTaskService) ExecutionTimeout() time.Duration {
	if s == nil || s.executionTimeout <= 0 {
		return defaultMusicTaskExecutionTimeout
	}
	return s.executionTimeout
}

func (s *MusicTaskService) TTL() time.Duration {
	if s == nil || s.ttl <= 0 {
		return defaultMusicTaskTTL
	}
	return s.ttl
}

// Create persists a fresh pending task owned by (UserID, APIKeyID).
func (s *MusicTaskService) Create(ctx context.Context, owner MusicTaskOwner) (*MusicTask, error) {
	if s == nil || s.store == nil {
		return nil, ErrMusicTaskUnavailable
	}
	now := time.Now().UTC()
	task := &MusicTaskRecord{
		ID:        "mustask_" + strings.ReplaceAll(uuid.NewString(), "-", ""),
		UserID:    owner.UserID,
		APIKeyID:  owner.APIKeyID,
		Status:    MusicTaskStatusPending,
		CreatedAt: now.Unix(),
		ExpiresAt: now.Add(s.ttl).Unix(),
	}
	if err := s.store.Save(ctx, task, s.ttl); err != nil {
		return nil, ErrMusicTaskUnavailable.WithCause(err)
	}
	return musicTaskToPublic(task), nil
}

// Get enforces ownership: the polling key must match BOTH the user and the API
// key that created the task; mismatch reads as not-found so task IDs are not
// enumerable across callers.
func (s *MusicTaskService) Get(ctx context.Context, owner MusicTaskOwner, id string) (*MusicTask, error) {
	if s == nil || s.store == nil {
		return nil, ErrMusicTaskUnavailable
	}
	task, err := s.store.Get(ctx, strings.TrimSpace(id))
	if err != nil {
		if errors.Is(err, ErrMusicTaskNotFound) {
			return nil, ErrMusicTaskNotFound
		}
		return nil, ErrMusicTaskUnavailable.WithCause(err)
	}
	if task.UserID != owner.UserID || task.APIKeyID != owner.APIKeyID {
		// Do not reveal whether a random task ID exists for another caller.
		return nil, ErrMusicTaskNotFound
	}
	return musicTaskToPublic(task), nil
}

// MarkRunning performs the pending → running leg of the forward-only state
// machine. It is idempotent: a task that already left pending is untouched.
func (s *MusicTaskService) MarkRunning(ctx context.Context, id string) error {
	return s.transition(ctx, id, MusicTaskStatusPending, func(task *MusicTaskRecord) {
		task.Status = MusicTaskStatusRunning
	})
}

// Complete performs the running → succeeded transition and stores the result.
// A task whose MarkRunning write was lost still completes (from any
// non-terminal state); the terminal guard keeps transitions forward-only.
func (s *MusicTaskService) Complete(ctx context.Context, id string, statusCode int, result json.RawMessage) error {
	if !json.Valid(result) {
		return s.Fail(ctx, id, http.StatusBadGateway, musicTaskErrorJSON("api_error", "upstream returned a non-JSON music response"))
	}
	return s.transition(ctx, id, "", func(task *MusicTaskRecord) {
		task.Status = MusicTaskStatusSucceeded
		task.HTTPStatus = statusCode
		task.Result = result
		task.Error = nil
	})
}

// Fail performs the terminal failed transition from any non-terminal state.
func (s *MusicTaskService) Fail(ctx context.Context, id string, statusCode int, taskErr json.RawMessage) error {
	if !json.Valid(taskErr) {
		taskErr = musicTaskErrorJSON("api_error", "music generation failed")
	}
	return s.transition(ctx, id, "", func(task *MusicTaskRecord) {
		task.Status = MusicTaskStatusFailed
		task.HTTPStatus = statusCode
		task.Result = nil
		task.Error = taskErr
	})
}

// transition applies mutate to a non-terminal task and persists it. An empty
// from accepts any non-terminal source state (the terminal guard below keeps
// every path forward-only). Terminal tasks are never rewritten: the state
// machine only moves forward, first result wins.
func (s *MusicTaskService) transition(ctx context.Context, id, from string, mutate func(*MusicTaskRecord)) error {
	if s == nil || s.store == nil {
		return ErrMusicTaskUnavailable
	}
	task, err := s.store.Get(ctx, strings.TrimSpace(id))
	if err != nil {
		if errors.Is(err, ErrMusicTaskNotFound) {
			return ErrMusicTaskNotFound
		}
		return ErrMusicTaskUnavailable.WithCause(err)
	}
	if task.Status == MusicTaskStatusSucceeded || task.Status == MusicTaskStatusFailed {
		return nil
	}
	if from != "" && task.Status != from {
		return nil
	}
	now := time.Now().UTC()
	mutate(task)
	if task.Status == MusicTaskStatusSucceeded || task.Status == MusicTaskStatusFailed {
		completedAt := now.Unix()
		task.CompletedAt = &completedAt
		task.ExpiresAt = now.Add(s.ttl).Unix()
	}
	if err := s.store.Save(ctx, task, s.ttl); err != nil {
		return ErrMusicTaskUnavailable.WithCause(err)
	}
	return nil
}

func musicTaskToPublic(task *MusicTaskRecord) *MusicTask {
	if task == nil {
		return nil
	}
	audioURL, audioBase64, durationSec, format, model := musicTaskResultFields(task.Result)
	return &MusicTask{
		ID:          task.ID,
		TaskID:      task.ID,
		Object:      "music.generation.task",
		Status:      task.Status,
		HTTPStatus:  task.HTTPStatus,
		AudioURL:    audioURL,
		AudioBase64: audioBase64,
		DurationSec: durationSec,
		Format:      format,
		Model:       model,
		Error:       task.Error,
		CreatedAt:   task.CreatedAt,
		CompletedAt: task.CompletedAt,
		ExpiresAt:   task.ExpiresAt,
	}
}

// musicTaskResultFields 展开结果 JSON（{audio_url?, audio_base64?, duration_sec?, format?, model?}）。
func musicTaskResultFields(result json.RawMessage) (audioURL, audioBase64 string, durationSec float64, format, model string) {
	if len(result) == 0 || !json.Valid(result) {
		return "", "", 0, "", ""
	}
	var payload struct {
		AudioURL    string  `json:"audio_url"`
		AudioBase64 string  `json:"audio_base64"`
		DurationSec float64 `json:"duration_sec"`
		Format      string  `json:"format"`
		Model       string  `json:"model"`
	}
	if json.Unmarshal(result, &payload) != nil {
		return "", "", 0, "", ""
	}
	return strings.TrimSpace(payload.AudioURL), strings.TrimSpace(payload.AudioBase64),
		payload.DurationSec, strings.TrimSpace(payload.Format), strings.TrimSpace(payload.Model)
}

func musicTaskErrorJSON(errorType, message string) json.RawMessage {
	data, _ := json.Marshal(map[string]string{"type": errorType, "message": message})
	return data
}

// StableMusicBillingRequestID is the durable usage_logs / dedup key for one
// music generation task. Anchored on the immutable task ID so the money event
// cannot collapse under a reused client request id.
func StableMusicBillingRequestID(taskID string) string {
	taskID = strings.TrimSpace(taskID)
	if strings.HasPrefix(taskID, "music:") {
		return taskID
	}
	if taskID == "" {
		taskID = generateRequestID()
	}
	return "music:" + taskID
}
