//go:build unit

package service

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"
)

// 管理员撤销校园邮箱认证：service 层参数校验与幂等语义。
// repo 的 SQL 行为（事务内清引用 + 删除并返回条数）无法用 stub 覆盖，
// 留作集成环境手工验证点。
func TestAdminRevokeUserEducationEmail_IsIdempotent(t *testing.T) {
	repo := &userRepoStub{
		user:          &User{ID: 7, Email: "student@muc.edu.cn"},
		revokedCounts: map[int64]int64{7: 2},
	}
	svc := &adminServiceImpl{userRepo: repo}

	revoked, err := svc.RevokeUserEducationEmail(context.Background(), 7)
	require.NoError(t, err)
	require.Equal(t, int64(2), revoked)

	// 重复撤销：无匹配行时返回 (0, nil)，不报错。
	revoked, err = svc.RevokeUserEducationEmail(context.Background(), 7)
	require.NoError(t, err)
	require.Equal(t, int64(0), revoked)
}

func TestAdminRevokeUserEducationEmail_ValidatesInputAndUser(t *testing.T) {
	repo := &userRepoStub{}
	svc := &adminServiceImpl{userRepo: repo}

	_, err := svc.RevokeUserEducationEmail(context.Background(), 0)
	require.Error(t, err)

	// 用户不存在时透传仓储层错误（ent NotFound 映射为 ErrUserNotFound → 404）。
	_, err = svc.RevokeUserEducationEmail(context.Background(), 42)
	require.ErrorIs(t, err, ErrUserNotFound)
}
