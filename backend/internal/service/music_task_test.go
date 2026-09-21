package service

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

type musicTaskMemoryStore struct {
	task    *MusicTaskRecord
	ttl     time.Duration
	saveErr error
	getErr  error
}

func (s *musicTaskMemoryStore) Save(_ context.Context, task *MusicTaskRecord, ttl time.Duration) error {
	if s.saveErr != nil {
		return s.saveErr
	}
	copy := *task
	s.task = &copy
	s.ttl = ttl
	return nil
}

func (s *musicTaskMemoryStore) Get(_ context.Context, _ string) (*MusicTaskRecord, error) {
	if s.getErr != nil {
		return nil, s.getErr
	}
	if s.task == nil {
		return nil, ErrMusicTaskNotFound
	}
	copy := *s.task
	return &copy, nil
}

func TestMusicTaskServiceLifecycleAndOwnership(t *testing.T) {
	store := &musicTaskMemoryStore{}
	svc := NewMusicTaskServiceWithOptions(store, time.Hour, 10*time.Minute)
	owner := MusicTaskOwner{UserID: 7, APIKeyID: 9}

	created, err := svc.Create(context.Background(), owner)
	require.NoError(t, err)
	require.Equal(t, MusicTaskStatusPending, created.Status)
	require.Equal(t, "music.generation.task", created.Object)
	require.True(t, created.ID == created.TaskID)
	require.True(t, len(created.ID) > len("mustask_") && created.ID[:len("mustask_")] == "mustask_")
	require.NotContains(t, created.ID, "-")
	require.Equal(t, time.Hour, store.ttl)
	require.Equal(t, owner.UserID, store.task.UserID)
	require.Equal(t, owner.APIKeyID, store.task.APIKeyID)

	// A different API key of the same user must not see the task.
	_, err = svc.Get(context.Background(), MusicTaskOwner{UserID: 7, APIKeyID: 10}, created.ID)
	require.ErrorIs(t, err, ErrMusicTaskNotFound)
	// A different user must not see it either.
	_, err = svc.Get(context.Background(), MusicTaskOwner{UserID: 8, APIKeyID: 9}, created.ID)
	require.ErrorIs(t, err, ErrMusicTaskNotFound)

	require.NoError(t, svc.MarkRunning(context.Background(), created.ID))
	require.Equal(t, MusicTaskStatusRunning, store.task.Status)

	result := json.RawMessage(`{"audio_url":"https://example.test/clip.mp3","format":"mp3"}`)
	require.NoError(t, svc.Complete(context.Background(), created.ID, http.StatusOK, result))

	completed, err := svc.Get(context.Background(), owner, created.ID)
	require.NoError(t, err)
	require.Equal(t, MusicTaskStatusSucceeded, completed.Status)
	require.Equal(t, http.StatusOK, completed.HTTPStatus)
	require.Equal(t, "https://example.test/clip.mp3", completed.AudioURL)
	require.Empty(t, completed.AudioBase64)
	require.Equal(t, "mp3", completed.Format)
	require.NotNil(t, completed.CompletedAt)
}

func TestMusicTaskServiceTerminalStateIsImmutable(t *testing.T) {
	store := &musicTaskMemoryStore{}
	svc := NewMusicTaskServiceWithOptions(store, time.Hour, time.Minute)
	created, err := svc.Create(context.Background(), MusicTaskOwner{UserID: 1, APIKeyID: 2})
	require.NoError(t, err)

	result := json.RawMessage(`{"audio_url":"https://example.test/a.mp3"}`)
	require.NoError(t, svc.Complete(context.Background(), created.ID, http.StatusOK, result))
	// A late duplicate write (retry, panic path) must not overwrite the result.
	require.NoError(t, svc.Fail(context.Background(), created.ID, http.StatusBadGateway, musicTaskErrorJSON("api_error", "late failure")))

	got, err := svc.Get(context.Background(), MusicTaskOwner{UserID: 1, APIKeyID: 2}, created.ID)
	require.NoError(t, err)
	require.Equal(t, MusicTaskStatusSucceeded, got.Status)
	require.Equal(t, http.StatusOK, got.HTTPStatus)
	require.Empty(t, got.Error)
}

func TestMusicTaskServiceInvalidResultBecomesFailed(t *testing.T) {
	store := &musicTaskMemoryStore{}
	svc := NewMusicTaskServiceWithOptions(store, time.Hour, time.Minute)
	created, err := svc.Create(context.Background(), MusicTaskOwner{UserID: 1, APIKeyID: 2})
	require.NoError(t, err)

	require.NoError(t, svc.Complete(context.Background(), created.ID, http.StatusOK, json.RawMessage(`not-json`)))
	got, err := svc.Get(context.Background(), MusicTaskOwner{UserID: 1, APIKeyID: 2}, created.ID)
	require.NoError(t, err)
	require.Equal(t, MusicTaskStatusFailed, got.Status)
	require.Equal(t, http.StatusBadGateway, got.HTTPStatus)
	require.Contains(t, string(got.Error), "non-JSON")
}

func TestMusicTaskServiceMapsStoreFailures(t *testing.T) {
	store := &musicTaskMemoryStore{saveErr: errors.New("redis down")}
	svc := NewMusicTaskService(store)

	_, err := svc.Create(context.Background(), MusicTaskOwner{UserID: 1, APIKeyID: 2})
	require.ErrorIs(t, err, ErrMusicTaskUnavailable)
}

func TestMusicTaskServiceWorksWithoutUploader(t *testing.T) {
	// Unlike image tasks, music stays enabled with no object storage: inline
	// base64 fallback is the enablement story.
	svc := NewMusicTaskServiceWithOptions(&musicTaskMemoryStore{}, time.Hour, time.Minute)
	require.True(t, svc.Enabled())
	_, enabled := svc.CurrentUploader()
	require.False(t, enabled)
}

func TestMusicTaskServiceResolverControlsUploader(t *testing.T) {
	store := &musicTaskMemoryStore{}
	uploader := NewImageResultUploader(nil, "music/", 0, nil)
	svc := NewMusicTaskServiceWithResolver(store, func() (*ImageResultUploader, bool) {
		return uploader, true
	}, time.Hour, time.Minute)
	got, enabled := svc.CurrentUploader()
	require.True(t, enabled)
	require.Same(t, uploader, got)
}

func TestStableMusicBillingRequestID(t *testing.T) {
	require.Equal(t, "music:mustask_abc", StableMusicBillingRequestID("mustask_abc"))
	require.Equal(t, "music:mustask_abc", StableMusicBillingRequestID("music:mustask_abc"))
	require.NotEmpty(t, StableMusicBillingRequestID(""))
}
