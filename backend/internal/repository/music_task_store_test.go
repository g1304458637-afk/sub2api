package repository

import (
	"context"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/alicebob/miniredis/v2"
	"github.com/redis/go-redis/v9"
	"github.com/stretchr/testify/require"
)

func TestMusicTaskStoreRoundTripAndTTL(t *testing.T) {
	mr := miniredis.RunT(t)
	rdb := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	t.Cleanup(func() { _ = rdb.Close() })
	store := NewMusicTaskStore(rdb)
	task := &service.MusicTaskRecord{
		ID:        "mustask_123",
		UserID:    7,
		APIKeyID:  9,
		Status:    service.MusicTaskStatusPending,
		CreatedAt: 100,
		ExpiresAt: 200,
	}

	require.NoError(t, store.Save(context.Background(), task, 24*time.Hour))
	got, err := store.Get(context.Background(), task.ID)
	require.NoError(t, err)
	require.Equal(t, task, got)
	require.Equal(t, 24*time.Hour, mr.TTL(musicTaskKey(task.ID)))
	// Redis key contract: music_task:<id>.
	require.True(t, mr.Exists(musicTaskKeyPrefix+"mustask_123"))
}

func TestMusicTaskStoreMissing(t *testing.T) {
	mr := miniredis.RunT(t)
	rdb := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	t.Cleanup(func() { _ = rdb.Close() })
	store := NewMusicTaskStore(rdb)

	_, err := store.Get(context.Background(), "mustask_missing")
	require.ErrorIs(t, err, service.ErrMusicTaskNotFound)
}

func TestMusicTaskStoreOverwrite(t *testing.T) {
	mr := miniredis.RunT(t)
	rdb := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	t.Cleanup(func() { _ = rdb.Close() })
	store := NewMusicTaskStore(rdb)
	ctx := context.Background()

	task := &service.MusicTaskRecord{ID: "mustask_overwrite", UserID: 1, APIKeyID: 2, Status: service.MusicTaskStatusPending}
	require.NoError(t, store.Save(ctx, task, time.Hour))
	task.Status = service.MusicTaskStatusRunning
	require.NoError(t, store.Save(ctx, task, time.Hour))

	got, err := store.Get(ctx, task.ID)
	require.NoError(t, err)
	require.Equal(t, service.MusicTaskStatusRunning, got.Status)
}
