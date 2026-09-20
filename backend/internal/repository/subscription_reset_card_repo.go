package repository

import (
	"context"
	"time"

	dbent "github.com/Wei-Shaw/sub2api/ent"
	"github.com/Wei-Shaw/sub2api/ent/subscriptionresetcard"
	"github.com/Wei-Shaw/sub2api/internal/domain"
	"github.com/Wei-Shaw/sub2api/internal/service"
)

// subscriptionResetCardRepository 实现 service.SubscriptionResetCardReader。
// Phase 4.1 仅提供只读统计：可用卡 = status='available' 且未过期。
// 过期即不可用（expires_at > now 严格判断，== now 视为过期）；不做惰性状态修改
// ——status 的批量 expire 归属未来 Reset Card Runtime，与本读路径解耦。
type subscriptionResetCardRepository struct {
	client *dbent.Client
}

func NewSubscriptionResetCardRepository(client *dbent.Client) service.SubscriptionResetCardReader {
	return &subscriptionResetCardRepository{client: client}
}

func (r *subscriptionResetCardRepository) CountAvailableResetCards(ctx context.Context, userID int64, now time.Time) (int, error) {
	return r.client.SubscriptionResetCard.Query().
		Where(
			subscriptionresetcard.UserIDEQ(userID),
			subscriptionresetcard.StatusEQ(domain.ResetCardStatusAvailable),
			subscriptionresetcard.Or(
				subscriptionresetcard.ExpiresAtIsNil(),
				subscriptionresetcard.ExpiresAtGT(now),
			),
		).
		Count(ctx)
}
