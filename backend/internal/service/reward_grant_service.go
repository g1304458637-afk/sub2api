package service

import (
	"context"
	"fmt"
	"log/slog"

	dbent "github.com/Wei-Shaw/sub2api/ent"
)

// StudentRewardConfigReader 学生奖励配置读取接口（由 *SettingService 实现）。
// 收窄成接口便于单测注入假实现。
type StudentRewardConfigReader interface {
	IsStudentVerificationRewardEnabled(ctx context.Context) bool
	GetStudentVerificationRewardAmount(ctx context.Context) float64
	GetStudentVerificationRewardCampaign(ctx context.Context) string
}

// RewardGrantService 系统奖励发放服务。
//
// 所有"系统自动给用户加余额"的场景（学生认证奖励、未来的注册奖励/邀请奖励等）
// 都必须经过本服务，不允许来源模块自己 AdjustBalance：金额、活动、幂等、事务
// 全部封装在这里。
type RewardGrantService struct {
	entClient            *dbent.Client
	rewardRepo           RewardGrantRepository
	userRepo             UserRepository
	settingReader        StudentRewardConfigReader
	authCacheInvalidator APIKeyAuthCacheInvalidator
	billingCacheService  *BillingCacheService
}

func NewRewardGrantService(
	entClient *dbent.Client,
	rewardRepo RewardGrantRepository,
	userRepo UserRepository,
	settingReader StudentRewardConfigReader,
	authCacheInvalidator APIKeyAuthCacheInvalidator,
	billingCacheService *BillingCacheService,
) *RewardGrantService {
	return &RewardGrantService{
		entClient:            entClient,
		rewardRepo:           rewardRepo,
		userRepo:             userRepo,
		settingReader:        settingReader,
		authCacheInvalidator: authCacheInvalidator,
		billingCacheService:  billingCacheService,
	}
}

// GrantStudentVerificationReward 学生认证通过后的奖励发放（业务封装）。
//
// 学生认证模块在"认证状态 pending → approved"成功落库之后调用本方法：
//   - 开关关闭 → 返回 DISABLED（不发钱，也不是错误）；
//   - 开关打开但金额/活动配置不完整 → 返回 INVALID_CONFIG（不发钱，需管理员修配置）；
//   - 其余情况委托 GrantReward，重复调用/并发/重试由 reward_grants 的唯一约束兜底。
//
// 金额与 campaign 一律从 settings 读取，调用方不传金额。
// verificationID 是学生认证业务表的行 ID（逻辑引用，本模块不建外键）。
// grantedBy 是触发审核的管理员 ID（系统自动场景传 nil）。
func (s *RewardGrantService) GrantStudentVerificationReward(
	ctx context.Context,
	userID int64,
	verificationID int64,
	grantedBy *int64,
) (*RewardGrantResult, error) {
	if s == nil || s.settingReader == nil {
		return nil, ErrServiceUnavailable
	}
	if userID <= 0 || verificationID <= 0 {
		return nil, ErrRewardInvalidCommand
	}

	if !s.settingReader.IsStudentVerificationRewardEnabled(ctx) {
		return &RewardGrantResult{Status: RewardGrantStatusDisabled}, nil
	}

	amount := s.settingReader.GetStudentVerificationRewardAmount(ctx)
	campaign := s.settingReader.GetStudentVerificationRewardCampaign(ctx)
	if amount <= 0 || campaign == "" {
		slog.Warn("student verification reward misconfigured; skipping grant",
			"user_id", userID, "verification_id", verificationID,
			"amount", amount, "campaign", campaign)
		return &RewardGrantResult{Status: RewardGrantStatusInvalidConfig}, nil
	}

	return s.GrantReward(ctx, GrantRewardCommand{
		UserID: userID,
		// 确定性幂等键：同一用户 + 同一 campaign 只能领取一次；
		// 换 campaign（新一轮活动）即得到新 key，可再次发放。
		IdempotencyKey: StudentVerificationRewardIdempotencyKey(userID, campaign),
		SourceType:     RewardSourceStudentVerification,
		SourceID:       &verificationID,
		Campaign:       campaign,
		Amount:         amount,
		GrantedBy:      grantedBy,
	})
}

// StudentVerificationRewardIdempotencyKey 派生学生认证奖励的确定性幂等键。
// 公开导出：认证模块如需在自己的审计/对账逻辑里预告 key，可用同一函数生成。
func StudentVerificationRewardIdempotencyKey(userID int64, campaign string) string {
	return fmt.Sprintf("student_verification:%d:%s", userID, campaign)
}

// ListRewardsByUser 查询某用户的奖励发放记录（余额历史与后续 admin 查询用）。
func (s *RewardGrantService) ListRewardsByUser(ctx context.Context, userID int64, limit int) ([]RewardGrant, error) {
	if s == nil || s.rewardRepo == nil || userID <= 0 {
		return nil, nil
	}
	if limit <= 0 {
		limit = 100
	}
	return s.rewardRepo.ListByUser(ctx, userID, limit)
}

// GetRewardBySource 按来源查询奖励发放记录（如学生认证模块回查某次认证是否已发奖）。
func (s *RewardGrantService) GetRewardBySource(ctx context.Context, sourceType string, sourceID int64) ([]RewardGrant, error) {
	if s == nil || s.rewardRepo == nil || sourceType == "" || sourceID <= 0 {
		return nil, nil
	}
	return s.rewardRepo.GetBySource(ctx, sourceType, sourceID)
}

// AdminListRewardGrants 管理端分页查询发放记录（只读台账）。
// filter 为 nil 或 repo 不可用时返回空列表（防御式，不报错），与 ListRewardsByUser 口径一致。
func (s *RewardGrantService) AdminListRewardGrants(ctx context.Context, filter *RewardGrantAdminFilter) (*RewardGrantList, error) {
	if s == nil || s.rewardRepo == nil {
		return &RewardGrantList{Items: []RewardGrantAdminItem{}}, nil
	}
	return s.rewardRepo.AdminList(ctx, filter)
}

// compile-time 接口满足性检查：*SettingService 必须实现 StudentRewardConfigReader。
var _ StudentRewardConfigReader = (*SettingService)(nil)
