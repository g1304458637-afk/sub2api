// Package muccode 提供 MUC 一次性授权码的 Redis 存取适配。
//
// handler 与 service 层均禁止直接依赖 redis 客户端（depguard:
// handler-no-repository / service-no-repository），故适配器置于 internal/pkg。
// 安全约定：Redis 只保存 code 的 SHA-256 哈希（TTL 由调用方给定，60s），
// GetDel 保证原子单次使用；明文 code 不落 Redis。
package muccode

import (
	"context"
	"time"

	"github.com/redis/go-redis/v9"
)

// CodeStore 是 *redis.Client 的窄封装：只暴露授权码的写入与原子取出删除。
type CodeStore struct {
	client *redis.Client
}

func NewCodeStore(client *redis.Client) *CodeStore {
	return &CodeStore{client: client}
}

func (s *CodeStore) SetCode(ctx context.Context, key string, payload []byte, ttl time.Duration) error {
	return s.client.Set(ctx, key, payload, ttl).Err()
}

func (s *CodeStore) GetDelCode(ctx context.Context, key string) (string, error) {
	return s.client.GetDel(ctx, key).Result()
}
