package service

import "context"

// RewardGrantRepository reward_grants 表的持久化接口。
type RewardGrantRepository interface {
	// InsertIdempotent 幂等插入一条发放记录。
	// 命中 UNIQUE(idempotency_key) 时返回 inserted=false 且不报错；
	// 插入成功时会把数据库生成的 ID / CreatedAt 回填进 grant。
	InsertIdempotent(ctx context.Context, grant *RewardGrant) (inserted bool, err error)

	// GetByIdempotencyKey 按幂等键查询已有发放记录（未命中返回 nil, nil）。
	GetByIdempotencyKey(ctx context.Context, idempotencyKey string) (*RewardGrant, error)

	// GetByUserSourceCampaign 按 (user, source, campaign) 查询发放记录（审计用，可能多条历史）。
	GetByUserSourceCampaign(ctx context.Context, userID int64, sourceType, campaign string) ([]RewardGrant, error)

	// ListByUser 按用户查询发放记录，created_at 倒序（余额历史归并用）。
	ListByUser(ctx context.Context, userID int64, limit int) ([]RewardGrant, error)

	// CountByUser 统计某用户发放记录总数。
	CountByUser(ctx context.Context, userID int64) (int64, error)

	// GetBySource 按来源（source_type + source_id）查询发放记录。
	GetBySource(ctx context.Context, sourceType string, sourceID int64) ([]RewardGrant, error)

	// AdminList 管理端分页查询发放记录（created_at 倒序，回填用户与发放人邮箱/用户名）。
	AdminList(ctx context.Context, filter *RewardGrantAdminFilter) (*RewardGrantList, error)
}
