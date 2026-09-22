-- 242: Subscription V1 单主套餐硬约束（产品 RULE 1）。
--
-- 背景：user_subscriptions 的既有唯一约束是 (user_id, group_id) WHERE deleted_at IS NULL
-- （迁移 016），防的是同组重复；跨组购买/兑换/赠送会为同一用户制造第二条 ACTIVE
-- 订阅（例如 Basic ACTIVE + Pro ACTIVE 同时存在），违反「同一时间一个用户只能有
-- 一个 MUC 主套餐生效」的产品不变量。
--
-- 当前系统内全部订阅组均属 MUCODE 单一产品族，因此 user_id 维度的唯一性
-- 即等价于 (user_id, product_family) 维度的不变量；未来出现第二个产品族时，
-- 应增加 product_family 列并将本索引进化为 (user_id, product_family)。
--
-- 本迁移在事务内执行（runner 逐文件事务化）：
--   ① 幂等 repair 脏数据：同一用户多条 ACTIVE 时，保留 expires_at 最大者
--     （并列取 id 最大，即最近一次履约），其余翻 expired 并追加审计备注；
--     不触碰 payment_orders / subscription_terms / usage_logs（历史与价格真相完整保留）。
--   ② 建 partial unique index 硬化不变量：此后任何路径（购买履约 / 兑换码 /
--     注册赠送 / Admin 分配）再制造第二条 ACTIVE 都会在数据库层被拒绝。

-- ① repair：收敛每个用户的多条 ACTIVE 订阅
WITH ranked AS (
    SELECT id,
           ROW_NUMBER() OVER (
               PARTITION BY user_id
               ORDER BY expires_at DESC, id DESC
           ) AS rn
    FROM user_subscriptions
    WHERE deleted_at IS NULL AND status = 'active'
)
UPDATE user_subscriptions us
SET status = 'expired',
    notes = CASE
        WHEN us.notes IS NULL OR us.notes = '' THEN 'PLAN_UNIFY_242(prev_status=active)'
        ELSE us.notes || E'\nPLAN_UNIFY_242(prev_status=active)'
    END,
    updated_at = NOW()
FROM ranked r
WHERE us.id = r.id
  AND r.rn > 1;

-- ② 硬约束：每用户至多一条 ACTIVE 订阅（软删与历史行不受影响）
CREATE UNIQUE INDEX IF NOT EXISTS uq_user_subscriptions_single_active
    ON user_subscriptions(user_id)
    WHERE deleted_at IS NULL AND status = 'active';
