package repository

import (
	"context"
	"encoding/json"
	"strings"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/redis/go-redis/v9"
)

const musicTaskKeyPrefix = "music_task:"

type musicTaskStore struct {
	rdb *redis.Client
}

func NewMusicTaskStore(rdb *redis.Client) service.MusicTaskStore {
	return &musicTaskStore{rdb: rdb}
}

func (s *musicTaskStore) Save(ctx context.Context, task *service.MusicTaskRecord, ttl time.Duration) error {
	data, err := json.Marshal(task)
	if err != nil {
		return err
	}
	return s.rdb.Set(ctx, musicTaskKey(task.ID), data, ttl).Err()
}

func (s *musicTaskStore) Get(ctx context.Context, id string) (*service.MusicTaskRecord, error) {
	data, err := s.rdb.Get(ctx, musicTaskKey(id)).Bytes()
	if err != nil {
		if err == redis.Nil {
			return nil, service.ErrMusicTaskNotFound
		}
		return nil, err
	}
	var task service.MusicTaskRecord
	if err := json.Unmarshal(data, &task); err != nil {
		return nil, err
	}
	return &task, nil
}

func musicTaskKey(id string) string {
	return musicTaskKeyPrefix + strings.TrimSpace(id)
}
