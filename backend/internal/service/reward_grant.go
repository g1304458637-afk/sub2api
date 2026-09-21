package service

import (
	"context"
	"fmt"
	"math"
	"time"

	dbent "github.com/Wei-Shaw/sub2api/ent"
	infraerrors "github.com/Wei-Shaw/sub2api/internal/pkg/errors"
)

// 奖励来源类型（reward_grants.source_type）。
// 新增来源时：加常量即可，Reward Core 不感知来源业务语义。
const (
	RewardSourceStudentVerification = "student_verification"
)

// 发放结果状态。GRANTED / ALREADY_GRANTED 都是业务上的幂等成功；
// DISABLED / INVALID_CONFIG 表示本次调用未发放（开关关闭或配置不完整）。
type RewardGrantStatus string

const (
	RewardGrantStatusGranted        RewardGrantStatus = "granted"
	RewardGrantStatusAlreadyGranted RewardGrantStatus = "already_granted"
	RewardGrantStatusDisabled       RewardGrantStatus = "disabled"
	RewardGrantStatusInvalidConfig  RewardGrantStatus = "invalid_config"
)

// RewardGrant 奖励发放记录（reward_grants 表的一行）。
type RewardGrant struct {
	ID             int64
	UserID         int64
	IdempotencyKey string // 业务奖励动作的确定性唯一标识（幂等维度）
	SourceType     string
	SourceID       *int64 // 来源业务行 ID（逻辑引用，无外键）
	Campaign       string // 活动归属（审计/运营维度，不参与唯一约束）
	Amount         float64
	GrantedBy      *int64
	Metadata       map[string]any
	CreatedAt      time.Time
}

// RewardGrantAdminFilter 管理端发放记录列表筛选（分页 + 精确等值过滤）。
type RewardGrantAdminFilter struct {
	Page     int
	PageSize int
	UserID   *int64
	Campaign string
	// SourceType 精确等值匹配（如 student_verification），空串 = 不过滤。
	SourceType string
}

// RewardGrantAdminItem 管理端发放记录列表项：
// 发放记录本体 + 用户邮箱/用户名与发放人邮箱回填（LEFT JOIN users，未命中为空串）。
type RewardGrantAdminItem struct {
	RewardGrant
	Email          string
	Username       string
	GrantedByEmail string
}

// RewardGrantList 管理端发放记录分页结果。
type RewardGrantList struct {
	Items    []RewardGrantAdminItem
	Total    int
	Page     int
	PageSize int
}

// GrantRewardCommand 通用发放命令。
// IdempotencyKey 必填：由发放方从业务事实派生的确定性 key（如学生认证 =
// "student_verification:{userID}:{campaign}"，未来 referral/补偿各派生自己的形状）。
// 金额与活动由 Reward 层决定，不允许调用方随意指定：业务封装（如
// GrantStudentVerificationReward）负责从 settings 读取后填入。
type GrantRewardCommand struct {
	UserID         int64
	IdempotencyKey string
	SourceType     string
	SourceID       *int64
	Campaign       string
	Amount         float64
	GrantedBy      *int64
	Metadata       map[string]any
}

// RewardGrantResult 发放结果。调用方据 Status 区分"本次真正发了钱"与"之前已发过"。
type RewardGrantResult struct {
	Status RewardGrantStatus
	Grant  *RewardGrant // GRANTED = 新记录；ALREADY_GRANTED = 已存在的记录（尽力回查）
}

// Granted reports whether the user's balance was credited by this reward
// campaign at any point in time (this call or an earlier one).
func (r *RewardGrantResult) Granted() bool {
	return r != nil && (r.Status == RewardGrantStatusGranted || r.Status == RewardGrantStatusAlreadyGranted)
}

var (
	ErrRewardInvalidCommand = infraerrors.BadRequest("REWARD_INVALID_COMMAND", "invalid reward grant command")
	ErrRewardGrantFailed    = infraerrors.InternalServer("REWARD_GRANT_FAILED", "reward grant failed")
)

// validateGrantRewardCommand 校验通用命令的基本形状。
func validateGrantRewardCommand(cmd GrantRewardCommand) error {
	switch {
	case cmd.UserID <= 0:
		return ErrRewardInvalidCommand
	case cmd.IdempotencyKey == "":
		return ErrRewardInvalidCommand
	case cmd.SourceType == "":
		return ErrRewardInvalidCommand
	case cmd.Campaign == "":
		return ErrRewardInvalidCommand
	case cmd.Amount <= 0 || math.IsNaN(cmd.Amount) || math.IsInf(cmd.Amount, 0):
		return ErrRewardInvalidCommand
	default:
		return nil
	}
}

// GrantReward 通用奖励发放入口。
//
// 事务与幂等语义（数据库级，调用方无需自己判断是否已发放）：
//  1. INSERT reward_grants ... ON CONFLICT (idempotency_key) DO NOTHING；
//  2. affected = 0 说明该业务奖励动作已发放过 → 直接返回 ALREADY_GRANTED，
//     绝不再加余额；
//  3. affected = 1 才执行 userRepo.AdjustBalance(+amount)，与 INSERT 同一事务提交，
//     保证"有记录必有加钱、加了钱必有记录"。
//
// 若 ctx 已携带事务（dbent.TxFromContext），则在调用方事务内联执行（不做缓存失效，
// 由调用方提交后调用 InvalidateCaches）；否则自开事务并在提交后失效余额相关缓存。
func (s *RewardGrantService) GrantReward(ctx context.Context, cmd GrantRewardCommand) (*RewardGrantResult, error) {
	if s == nil || s.rewardRepo == nil || s.userRepo == nil {
		return nil, ErrServiceUnavailable
	}
	if err := validateGrantRewardCommand(cmd); err != nil {
		return nil, err
	}

	if dbent.TxFromContext(ctx) == nil {
		// 无外层事务：自开事务（内联路径不依赖 entClient）
		if s.entClient == nil {
			return nil, ErrServiceUnavailable
		}
		tx, err := s.entClient.Tx(ctx)
		if err != nil {
			return nil, fmt.Errorf("begin reward grant transaction: %w", err)
		}
		defer func() { _ = tx.Rollback() }()

		txCtx := dbent.NewTxContext(ctx, tx)
		result, err := s.grantRewardInTx(txCtx, cmd)
		if err != nil {
			return nil, err
		}
		if err := tx.Commit(); err != nil {
			return nil, fmt.Errorf("commit reward grant transaction: %w", err)
		}

		if result.Status == RewardGrantStatusGranted {
			s.invalidateRewardCaches(ctx, cmd.UserID)
		}
		return result, nil
	}

	return s.grantRewardInTx(ctx, cmd)
}

// grantRewardInTx 在事务上下文内执行发放（调用方需保证 ctx 已含事务）。
func (s *RewardGrantService) grantRewardInTx(ctx context.Context, cmd GrantRewardCommand) (*RewardGrantResult, error) {
	grant := &RewardGrant{
		UserID:         cmd.UserID,
		IdempotencyKey: cmd.IdempotencyKey,
		SourceType:     cmd.SourceType,
		SourceID:       cmd.SourceID,
		Campaign:       cmd.Campaign,
		Amount:         cmd.Amount,
		GrantedBy:      cmd.GrantedBy,
		Metadata:       cmd.Metadata,
	}

	inserted, err := s.rewardRepo.InsertIdempotent(ctx, grant)
	if err != nil {
		return nil, fmt.Errorf("insert reward grant: %w", err)
	}
	if !inserted {
		// 唯一约束命中：该业务动作以前已发放过。回查已有记录供调用方展示，绝不再加余额。
		existing, getErr := s.rewardRepo.GetByIdempotencyKey(ctx, cmd.IdempotencyKey)
		if getErr != nil {
			return nil, fmt.Errorf("load existing reward grant: %w", getErr)
		}
		return &RewardGrantResult{Status: RewardGrantStatusAlreadyGranted, Grant: existing}, nil
	}

	// 学生奖励不是充值：必须走 AdjustBalance（不加 total_recharged）。
	if _, err := s.userRepo.AdjustBalance(ctx, cmd.UserID, cmd.Amount); err != nil {
		return nil, fmt.Errorf("adjust balance for reward: %w", err)
	}

	return &RewardGrantResult{Status: RewardGrantStatusGranted, Grant: grant}, nil
}

// invalidateRewardCaches 事务提交成功后失效鉴权缓存与余额缓存（先 Commit 后失效）。
func (s *RewardGrantService) invalidateRewardCaches(ctx context.Context, userID int64) {
	if s.authCacheInvalidator != nil {
		s.authCacheInvalidator.InvalidateAuthCacheByUserID(ctx, userID)
	}
	if s.billingCacheService != nil {
		go func() {
			cacheCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()
			_ = s.billingCacheService.InvalidateUserBalance(cacheCtx, userID)
		}()
	}
}

// InvalidateCaches 供外部事务提交方使用：当 GrantReward 以内联模式运行在调用方事务里时，
// 调用方在自己的事务 COMMIT 成功后必须调用本方法失效余额相关缓存。
// （自开事务路径已在提交后自动调用，无需外部触发。）
func (s *RewardGrantService) InvalidateCaches(ctx context.Context, userID int64) {
	if s == nil {
		return
	}
	s.invalidateRewardCaches(ctx, userID)
}
