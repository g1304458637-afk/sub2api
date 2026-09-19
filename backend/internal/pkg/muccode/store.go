// Package muccode 提供 MUC 一次性授权码的 Redis 存取适配。
//
// handler 与 service 层均禁止直接依赖 redis 客户端（depguard:
// handler-no-repository / service-no-repository），故适配器置于 internal/pkg。
// 安全约定：Redis 只保存 code 的 SHA-256 哈希（TTL 由调用方给定，60s），
// GetDel 保证原子单次使用；明文 code 不落 Redis。
package muccode

import (
	"context"
	"errors"
	"time"

	"github.com/redis/go-redis/v9"
)

// ErrCodeNotFound 表示 code 不存在（未签发/已过期/已使用）；
// 调用方必须把它与 Redis 基础设施故障（网络/超时等）区分处理。
var ErrCodeNotFound = errors.New("muc code not found")

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
	v, err := s.client.GetDel(ctx, key).Result()
	if errors.Is(err, redis.Nil) {
		return "", ErrCodeNotFound
	}
	return v, err
}
