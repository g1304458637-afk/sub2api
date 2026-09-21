#!/usr/bin/env bash
# subscription-single-active-dry-run.sh — 单主套餐不变量（迁移 242）脏数据 dry-run 检查。
#
# 用途：在应用迁移 242 之前，输出与迁移内置 repair 完全相同判定逻辑的修复计划，
# 供人工审计。只读（全部 SELECT），不做任何写操作。
#
# 用法：
#   ./subscription-single-active-dry-run.sh [连接串]
#   连接串缺省 = $DATABASE_URL 或 postgresql://sub2api:sub2api@localhost:5432/sub2api
#
# 判定规则（与 migrations/242_user_subscriptions_single_active.sql 逐字一致）：
#   同一 user_id 的多条 ACTIVE（deleted_at IS NULL AND status='active'）中，
#   保留 expires_at DESC, id DESC 第一名；其余翻 expired + notes 追加
#   PLAN_UNIFY_242(prev_status=active)。payment_orders / subscription_terms /
#   usage_logs 不触碰。
set -euo pipefail

DB="${1:-${DATABASE_URL:-postgresql://sub2api:sub2api@localhost:5432/sub2api}}"

echo "== 单主套餐不变量 dry-run（只读） =="
echo "== 连接: ${DB%%\?*}" | sed -E 's#://[^@]+@#://***@#'

PSQL="psql ${DB} -v ON_ERROR_STOP=1 -P pager=off"

echo
echo "== [1] 违规用户汇总（将被 repair 的用户数） =="
$PSQL -c "
SELECT user_id,
       COUNT(*)                       AS active_rows,
       MAX(expires_at)                AS keep_expires_at,
       MIN(expires_at)                AS drop_min_expires_at
FROM user_subscriptions
WHERE deleted_at IS NULL AND status = 'active'
GROUP BY user_id
HAVING COUNT(*) > 1
ORDER BY user_id;"

echo
echo "== [2] 逐行修复计划：keep = 保留行；drop = 将翻 expired 的行（附订单/金额/周期证据） =="
$PSQL -c "
WITH ranked AS (
    SELECT id, user_id,
           ROW_NUMBER() OVER (
               PARTITION BY user_id
               ORDER BY expires_at DESC, id DESC
           ) AS rn
    FROM user_subscriptions
    WHERE deleted_at IS NULL AND status = 'active'
)
SELECT r.rn = 1 AS keep,
       us.id            AS subscription_id,
       us.user_id,
       us.group_id,
       g.name           AS group_name,
       us.plan_id,
       us.starts_at,
       us.expires_at,
       us.weekly_usage_usd,
       o.id             AS last_order_id,
       o.amount         AS last_order_amount,
       o.status         AS last_order_status,
       us.notes
FROM ranked r
JOIN user_subscriptions us ON us.id = r.id
LEFT JOIN groups g         ON g.id = us.group_id
LEFT JOIN LATERAL (
    SELECT po.id, po.amount, po.status
    FROM payment_orders po
    WHERE po.user_id = us.user_id
      AND po.subscription_group_id = us.group_id
    ORDER BY po.id DESC
    LIMIT 1
) o ON TRUE
WHERE r.rn = 1 OR EXISTS (
    SELECT 1 FROM ranked r2
    WHERE r2.user_id = r.user_id AND r2.rn > 1
)
ORDER BY us.user_id, keep DESC, us.expires_at DESC;"

echo
echo "== [3] 预约降级一致性抽查（scheduled 行与其订阅指针） =="
$PSQL -c "
SELECT spc.id AS change_id, spc.subscription_id, spc.status, spc.effective_at,
       us.next_plan_id, us.status AS sub_status, us.expires_at
FROM subscription_plan_changes spc
LEFT JOIN user_subscriptions us ON us.id = spc.subscription_id
WHERE spc.change_type = 'scheduled_downgrade'
ORDER BY spc.id DESC
LIMIT 50;"

echo
echo "== dry-run 结束：未修改任何数据。确认 [2] 的 keep/drop 划分符合预期后，"
echo "== 直接启动新版本后端即由迁移 242 自动执行相同规则 repair 并建硬约束。 =="
