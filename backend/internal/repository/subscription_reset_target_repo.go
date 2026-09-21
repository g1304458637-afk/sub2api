package repository

// Reset 目标解析器（Phase 6/7 共用）：把 Direct Reset / Card Grant 的 selector
// 解析为确定性目标集合。统一口径：
//   - 目标订阅 = active + 未过期 + weekly metered（groups.weekly_limit_usd IS NOT NULL）
//   - 目标用户 = 拥有至少一个上述订阅的去重用户
//   - unmetered（weekly_limit NULL）永不进入 Direct Reset / Card Grant 目标
//
// 数组以内联 SQL 文本表达（元素为服务端解析后的 int64，无注入面）。

import (
	"context"
	"fmt"
	"strings"

	dbent "github.com/Wei-Shaw/sub2api/ent"
	"github.com/Wei-Shaw/sub2api/internal/domain"
	"github.com/Wei-Shaw/sub2api/internal/service"
)

type subscriptionResetTargetRepo struct {
	client *dbent.Client
}

func NewSubscriptionResetTargetRepo(client *dbent.Client) service.ResetTargetResolver {
	return &subscriptionResetTargetRepo{client: client}
}

func pqInt64Array(ids []int64) string {
	parts := make([]string, 0, len(ids))
	for _, id := range ids {
		parts = append(parts, fmt.Sprintf("%d", id))
	}
	return "ARRAY[" + strings.Join(parts, ",") + "]::bigint[]"
}

func targetWhere(mode string, userIDs, groupIDs []int64) (string, error) {
	where := []string{
		"us.deleted_at IS NULL",
		"us.status = 'active'",
		"us.expires_at > NOW()",
		"g.deleted_at IS NULL",
		"g.weekly_limit_usd IS NOT NULL",
		"g.weekly_limit_usd > 0",
	}
	switch mode {
	case domain.ResetTargetModeSubscriptionIDs:
		if len(userIDs) == 0 {
			return "", fmt.Errorf("subscription_ids is required")
		}
		where = append(where, "us.id = ANY("+pqInt64Array(userIDs)+")")
	case domain.ResetTargetModeUsers:
		if len(userIDs) == 0 {
			return "", fmt.Errorf("user_ids is required")
		}
		where = append(where, "us.user_id = ANY("+pqInt64Array(userIDs)+")")
		if len(groupIDs) > 0 {
			where = append(where, "us.group_id = ANY("+pqInt64Array(groupIDs)+")")
		}
	case domain.ResetTargetModeGroups:
		if len(groupIDs) == 0 {
			return "", fmt.Errorf("group_ids is required")
		}
		where = append(where, "us.group_id = ANY("+pqInt64Array(groupIDs)+")")
	case domain.ResetTargetModeAllActive, "all_active_users":
	default:
		return "", fmt.Errorf("unsupported target_mode %q", mode)
	}
	return strings.Join(where, " AND "), nil
}

const targetJoin = " FROM user_subscriptions us JOIN groups g ON g.id = us.group_id WHERE "

func (r *subscriptionResetTargetRepo) ResolveActiveMeteredSubscriptionIDs(ctx context.Context, mode string, userIDs, groupIDs []int64) ([]int64, error) {
	where, err := targetWhere(mode, userIDs, groupIDs)
	if err != nil {
		return nil, err
	}
	rows, err := r.client.QueryContext(ctx,
		"SELECT us.id"+targetJoin+where+" ORDER BY us.id")
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()
	ids := make([]int64, 0, 16)
	for rows.Next() {
		var id int64
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		ids = append(ids, id)
	}
	return ids, rows.Err()
}

func (r *subscriptionResetTargetRepo) ResolveActiveMeteredUserIDs(ctx context.Context, mode string, userIDs, groupIDs []int64) ([]int64, error) {
	where, err := targetWhere(mode, userIDs, groupIDs)
	if err != nil {
		return nil, err
	}
	rows, err := r.client.QueryContext(ctx,
		"SELECT DISTINCT us.user_id"+targetJoin+where+" ORDER BY us.user_id")
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()
	ids := make([]int64, 0, 16)
	for rows.Next() {
		var id int64
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		ids = append(ids, id)
	}
	return ids, rows.Err()
}

func (r *subscriptionResetTargetRepo) DescribeTargets(ctx context.Context, mode string, userIDs, groupIDs []int64) (*service.ResetTargetSummary, error) {
	where, err := targetWhere(mode, userIDs, groupIDs)
	if err != nil {
		return nil, err
	}
	summary := &service.ResetTargetSummary{TargetMode: mode}

	countRows, err := r.client.QueryContext(ctx,
		"SELECT COUNT(*), COUNT(DISTINCT us.user_id)"+targetJoin+where)
	if err != nil {
		return nil, err
	}
	if countRows.Next() {
		if err := countRows.Scan(&summary.SubscriptionCount, &summary.UniqueUserCount); err != nil {
			_ = countRows.Close()
			return nil, err
		}
	}
	if err := countRows.Err(); err != nil {
		_ = countRows.Close()
		return nil, err
	}
	if err := countRows.Close(); err != nil {
		return nil, err
	}

	breakdownRows, err := r.client.QueryContext(ctx,
		"SELECT g.id, g.name, COUNT(*)"+targetJoin+where+" GROUP BY g.id, g.name ORDER BY g.id")
	if err != nil {
		return nil, err
	}
	for breakdownRows.Next() {
		var st service.GroupTargetStat
		if err := breakdownRows.Scan(&st.GroupID, &st.Name, &st.SubsCount); err != nil {
			_ = breakdownRows.Close()
			return nil, err
		}
		summary.GroupBreakdown = append(summary.GroupBreakdown, st)
	}
	if err := breakdownRows.Err(); err != nil {
		_ = breakdownRows.Close()
		return nil, err
	}
	if err := breakdownRows.Close(); err != nil {
		return nil, err
	}

	sampleRows, err := r.client.QueryContext(ctx,
		"SELECT us.id, us.user_id, g.name"+targetJoin+where+" ORDER BY us.id LIMIT 10")
	if err != nil {
		return nil, err
	}
	defer func() { _ = sampleRows.Close() }()
	for sampleRows.Next() {
		var subID, userID int64
		var groupName string
		if err := sampleRows.Scan(&subID, &userID, &groupName); err != nil {
			return nil, err
		}
		summary.Sample = append(summary.Sample, service.ResetTargetSample{
			SubscriptionID: subID, UserID: userID, DisplayName: groupName,
		})
	}
	return summary, sampleRows.Err()
}
