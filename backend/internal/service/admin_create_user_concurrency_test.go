//go:build unit

package service

// Phase 3D —— Admin Create / Edit 用户并发语义回归。
//
// 锁定语义（GitHub 上游 Issue #5977 同义）：
//   - 字段省略（nil）→ default_concurrency 设置（与注册/OAuth 路径统一）；
//   - 显式 0 → unlimited（Runtime 既有语义，AcquireUserSlot <=0 → bypass）；
//   - 显式 N>0 → 上限 N；
//   - 负数 → handler 绑定层 400（gte=0），不落库；
//   - 编辑其他字段不得改动已存在的 0 值。

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/Wei-Shaw/sub2api/internal/config"
)

func phase3bConcurrencySettingService(t *testing.T, settingValue string) *SettingService {
	t.Helper()
	cfg := &config.Config{Default: config.DefaultConfig{UserConcurrency: 5}}
	return NewSettingService(&settingRepoStub{values: map[string]string{
		SettingKeyDefaultConcurrency: settingValue,
	}}, cfg)
}

// 省略字段 → default_concurrency 设置
func TestPhase3D_AdminCreateUser_OmittedConcurrencyUsesSetting(t *testing.T) {
	repo := &userRepoStub{nextID: 30}
	svc := &adminServiceImpl{userRepo: repo, settingService: phase3bConcurrencySettingService(t, "5")}

	user, err := svc.CreateUser(context.Background(), &CreateUserInput{
		Email:    "omit@test.com",
		Password: "strong-pass",
	})
	require.NoError(t, err)
	require.Equal(t, 5, user.Concurrency, "omitted field must fall back to default_concurrency setting")
	require.Equal(t, 5, repo.created[0].Concurrency)
}

// 设置缺失/非法 → 回退 config 默认（GetDefaultConcurrency 自带兜底）
func TestPhase3D_AdminCreateUser_OmittedConcurrencyFallsBackToConfig(t *testing.T) {
	repo := &userRepoStub{nextID: 31}
	svc := &adminServiceImpl{userRepo: repo, settingService: phase3bConcurrencySettingService(t, "")}

	user, err := svc.CreateUser(context.Background(), &CreateUserInput{
		Email:    "fallback@test.com",
		Password: "strong-pass",
	})
	require.NoError(t, err)
	require.Equal(t, 5, user.Concurrency, "missing setting must fall back to config default.user_concurrency")
}

// 显式 0 = 管理员明确要求 unlimited（合法，必须保留 0 而非被设置覆盖）
func TestPhase3D_AdminCreateUser_ExplicitZeroMeansUnlimited(t *testing.T) {
	repo := &userRepoStub{nextID: 32}
	svc := &adminServiceImpl{userRepo: repo, settingService: phase3bConcurrencySettingService(t, "5")}
	zero := 0

	user, err := svc.CreateUser(context.Background(), &CreateUserInput{
		Email:       "zero@test.com",
		Password:    "strong-pass",
		Concurrency: &zero,
	})
	require.NoError(t, err)
	require.Equal(t, 0, user.Concurrency, "explicit 0 must be preserved (unlimited), not replaced by setting")
	require.Equal(t, 0, repo.created[0].Concurrency)
}

// 显式正数 = 上限
func TestPhase3D_AdminCreateUser_ExplicitPositive(t *testing.T) {
	repo := &userRepoStub{nextID: 33}
	svc := &adminServiceImpl{userRepo: repo, settingService: phase3bConcurrencySettingService(t, "5")}
	n := 3

	user, err := svc.CreateUser(context.Background(), &CreateUserInput{
		Email:       "pos@test.com",
		Password:    "strong-pass",
		Concurrency: &n,
	})
	require.NoError(t, err)
	require.Equal(t, 3, user.Concurrency)
}

// 编辑其他字段不得改动已存在的 0 值（0 = unlimited 可长期存续）
func TestPhase3D_AdminEditUser_KeepsZeroConcurrency(t *testing.T) {
	repo := &userRepoStub{usersByID: map[int64]*User{
		42: {ID: 42, Email: "zero-user@test.com", Concurrency: 0, Status: StatusActive},
	}}
	invalidator := &authCacheInvalidatorStub{}
	svc := &adminServiceImpl{
		userRepo:             repo,
		redeemCodeRepo:       &redeemRepoStub{},
		authCacheInvalidator: invalidator,
	}
	notes := "updated notes"

	updated, err := svc.UpdateUser(context.Background(), 42, &UpdateUserInput{Notes: &notes})
	require.NoError(t, err)
	require.NotNil(t, updated)
	require.Equal(t, 0, updated.Concurrency, "editing other fields must keep concurrency=0 (unlimited)")
	require.Equal(t, notes, updated.Notes)
}
