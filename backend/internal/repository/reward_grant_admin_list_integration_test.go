//go:build integration

// 奖励发放记录管理端分页查询（AdminList）集成测试。
// 需要真实 PostgreSQL（reward_grants 表，迁移 240），本地无 DB 时无法运行：
//
//	go test -tags integration ./internal/repository/ -run TestRewardGrantRepository_AdminList
package repository

import (
	"context"
	"fmt"
	"testing"
	"time"

	dbent "github.com/Wei-Shaw/sub2api/ent"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/stretchr/testify/require"
)

func TestRewardGrantRepository_AdminList(t *testing.T) {
	ctx := context.Background()
	tx := testEntTx(t)
	txCtx := dbent.NewTxContext(ctx, tx)
	client := tx.Client()

	repo := NewRewardGrantRepository(client)

	suffix := time.Now().UnixNano()
	u1 := mustCreateUser(t, client, &service.User{
		Email:        fmt.Sprintf("reward-admin-u1-%d@example.com", suffix),
		PasswordHash: "hash",
		Role:         service.RoleUser,
		Status:       service.StatusActive,
		Balance:      0,
		Concurrency:  5,
	})
	u2 := mustCreateUser(t, client, &service.User{
		Email:        fmt.Sprintf("reward-admin-u2-%d@example.com", suffix),
		PasswordHash: "hash",
		Role:         service.RoleUser,
		Status:       service.StatusActive,
		Balance:      0,
		Concurrency:  5,
	})
	adminUser := mustCreateUser(t, client, &service.User{
		Email:        fmt.Sprintf("reward-admin-op-%d@example.com", suffix),
		PasswordHash: "hash",
		Role:         service.RoleAdmin,
		Status:       service.StatusActive,
		Balance:      0,
		Concurrency:  5,
	})

	adminID := adminUser.ID
	sourceID := int64(887)
	// 三条记录：u1 两条（同一来源/活动），u2 一条（granted_by = NULL → 系统自动）。
	for _, g := range []service.RewardGrant{
		{UserID: u1.ID, IdempotencyKey: fmt.Sprintf("it_adminlist:%d:1", suffix), SourceType: service.RewardSourceStudentVerification, SourceID: &sourceID, Campaign: "2026_spring", Amount: 20, GrantedBy: &adminID},
		{UserID: u1.ID, IdempotencyKey: fmt.Sprintf("it_adminlist:%d:2", suffix), SourceType: service.RewardSourceStudentVerification, SourceID: &sourceID, Campaign: "2026_spring", Amount: 10, GrantedBy: &adminID},
		{UserID: u2.ID, IdempotencyKey: fmt.Sprintf("it_adminlist:%d:3", suffix), SourceType: service.RewardSourceStudentVerification, Campaign: "2025_winter", Amount: 5},
	} {
		cmd := g
		inserted, err := repo.InsertIdempotent(txCtx, &cmd)
		require.NoError(t, err)
		require.True(t, inserted)
	}

	// 全量：COUNT 与分页
	page1, err := repo.AdminList(txCtx, &service.RewardGrantAdminFilter{Page: 1, PageSize: 2})
	require.NoError(t, err)
	require.Equal(t, 3, page1.Total)
	require.Len(t, page1.Items, 2)
	require.Equal(t, 1, page1.Page)
	require.Equal(t, 2, page1.PageSize)

	page2, err := repo.AdminList(txCtx, &service.RewardGrantAdminFilter{Page: 2, PageSize: 2})
	require.NoError(t, err)
	require.Len(t, page2.Items, 1)

	// 排序：created_at DESC, id DESC（同刻插入时 id 兜底 → 最后插入的排最前）
	require.Equal(t, fmt.Sprintf("it_adminlist:%d:3", suffix), page1.Items[0].IdempotencyKey)
	require.Equal(t, fmt.Sprintf("it_adminlist:%d:2", suffix), page1.Items[1].IdempotencyKey)
	require.Equal(t, fmt.Sprintf("it_adminlist:%d:1", suffix), page2.Items[0].IdempotencyKey)

	// JOIN 回填：email/username/granted_by_email；NULL granted_by → 空串
	require.Equal(t, u2.Email, page1.Items[0].Email)
	require.Equal(t, u2.Username, page1.Items[0].Username)
	require.Empty(t, page1.Items[0].GrantedByEmail)
	require.Nil(t, page1.Items[0].GrantedBy)
	require.Equal(t, adminUser.Email, page1.Items[1].GrantedByEmail)
	require.NotNil(t, page1.Items[1].SourceID)
	require.Equal(t, sourceID, *page1.Items[1].SourceID)

	// 过滤：user_id 精确等值
	uid := u1.ID
	byUser, err := repo.AdminList(txCtx, &service.RewardGrantAdminFilter{UserID: &uid, PageSize: 100})
	require.NoError(t, err)
	require.Equal(t, 2, byUser.Total)
	for _, item := range byUser.Items {
		require.Equal(t, u1.ID, item.UserID)
	}

	// 过滤：campaign 精确等值
	byCampaign, err := repo.AdminList(txCtx, &service.RewardGrantAdminFilter{Campaign: "2025_winter"})
	require.NoError(t, err)
	require.Equal(t, 1, byCampaign.Total)
	require.Equal(t, u2.ID, byCampaign.Items[0].UserID)
	require.InDelta(t, 5, byCampaign.Items[0].Amount, 1e-9)

	// 过滤：source_type + 分页防御（page<=0 → 1；pageSize 超上限截断 100）
	bySource, err := repo.AdminList(txCtx, &service.RewardGrantAdminFilter{Page: 0, PageSize: 500, SourceType: service.RewardSourceStudentVerification})
	require.NoError(t, err)
	require.Equal(t, 3, bySource.Total)
	require.Equal(t, 1, bySource.Page)
	require.Equal(t, 100, bySource.PageSize)
}

func TestRewardGrantRepository_AdminList_Empty(t *testing.T) {
	tx := testEntTx(t)
	txCtx := dbent.NewTxContext(context.Background(), tx)

	repo := NewRewardGrantRepository(tx.Client())
	// 不存在的用户 → 空结果（避免依赖全表无数据）
	missing := int64(9_999_999_999)
	result, err := repo.AdminList(txCtx, &service.RewardGrantAdminFilter{UserID: &missing})
	require.NoError(t, err)
	require.NotNil(t, result)
	require.Empty(t, result.Items)
	require.Equal(t, 1, result.Page)
	require.Equal(t, 20, result.PageSize)

	// nil filter 走默认分页（防御路径）
	result, err = repo.AdminList(txCtx, nil)
	require.NoError(t, err)
	require.NotNil(t, result)
	require.Equal(t, 1, result.Page)
	require.Equal(t, 20, result.PageSize)
}
