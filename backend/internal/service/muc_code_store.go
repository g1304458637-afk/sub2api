package service

// MUC Harness: 一次性授权码的 Redis 存取适配器。
//
// handler 层禁止直接依赖 redis 客户端（depguard: handler-no-repository），
// 故由本适配器把 *redis.Client 收窄为 Set/GetDel 两个能力。
// 安全约定（与 handler 侧一致）：Redis 只保存 code 的 SHA-256 哈希，TTL 60s；
// GetDel 保证原子单次使用；明文 code 不落 Redis。

import (
	"context"
	"time"

	"github.com/redis/go-redis/v9"
)

type MucCodeStore struct {
	client *redis.Client
}

func NewMucCodeStore(client *redis.Client) *MucCodeStore {
	return &MucCodeStore{client: client}
}

func (s *MucCodeStore) SetCode(ctx context.Context, key string, payload []byte, ttl time.Duration) error {
	return s.client.Set(ctx, key, payload, ttl).Err()
}

func (s *MucCodeStore) GetDelCode(ctx context.Context, key string) (string, error) {
	return s.client.GetDel(ctx, key).Result()
}
