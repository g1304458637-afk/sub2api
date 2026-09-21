-- Subscription V1 Phase 10: Plan Change Runtime（tier_rank / plan identity /
-- term snapshots / plan change audit）。
--
-- 全部 additive；存量行为不变。
--
-- 四闸门落地：
--   Gate 1 plan identity：user_subscriptions.plan_id（新购买/续费/升级起写入；
--     历史行保持 NULL = plan_identity_unresolved，Upgrade Preview 明确拒绝）
--   Gate 2 price truth：subscription_terms 保存每段已付事实（价格快照来自
--     payment_orders.amount，非目录价）
--   Gate 3 prepaid segments：每次购买/续费/升级履约各生成一行 term；
--     升级按未消费 segment 逐段折算，不平均
--   Gate 4 tier：subscription_plans.tier_rank（entitlement 档位，非 sort_order）

-- ① Plan 档位（0 = 未设置；涉及 0 档位的 Plan Change 一律拒绝，等待管理员配置）
ALTER TABLE subscription_plans ADD COLUMN IF NOT EXISTS tier_rank INT NOT NULL DEFAULT 0;

-- ② 订阅的 Plan 身份 + 计划降级目标（scheduled downgrade：term 结束后的下一档）
ALTER TABLE user_subscriptions ADD COLUMN IF NOT EXISTS plan_id BIGINT REFERENCES subscription_plans(id) ON DELETE SET NULL;
ALTER TABLE user_subscriptions ADD COLUMN IF NOT EXISTS next_plan_id BIGINT REFERENCES subscription_plans(id) ON DELETE SET NULL;

-- ③ 已付 term 快照（每段购买/续费/升级一行；升级不延长 term_end）
CREATE TABLE IF NOT EXISTS subscription_terms (
    id              BIGSERIAL PRIMARY KEY,
    subscription_id BIGINT NOT NULL REFERENCES user_subscriptions(id) ON DELETE CASCADE,
    -- 产生该 term 的订单与 SKU（admin 手动分配可为 NULL 订单）
    order_id        BIGINT,
    plan_id         BIGINT REFERENCES subscription_plans(id) ON DELETE SET NULL,
    -- 实付价格快照（来源 payment_orders.amount / 升级 amount_due；非目录价）
    price_paid      DECIMAL(20, 2) NOT NULL DEFAULT 0,
    currency        VARCHAR(8) NOT NULL DEFAULT '',
    days            INT NOT NULL DEFAULT 0,
    term_start      TIMESTAMPTZ NOT NULL,
    term_end        TIMESTAMPTZ NOT NULL,
    source          VARCHAR(20) NOT NULL DEFAULT 'purchase', -- purchase/renewal/upgrade/admin_assign
    created_at      TIMESTAMPTZ NOT NULL DEFAULT NOW()
);
CREATE INDEX IF NOT EXISTS idx_subscription_terms_subscription ON subscription_terms(subscription_id);
CREATE INDEX IF NOT EXISTS idx_subscription_terms_order ON subscription_terms(order_id);

-- ④ Plan Change 审计（Quote 冻结 + 履约链路；不重复建支付 ledger，
--    支付仍走 payment_orders.order_type='plan_change'，此处保存裁决快照）
CREATE TABLE IF NOT EXISTS subscription_plan_changes (
    id              BIGSERIAL PRIMARY KEY,
    user_id         BIGINT NOT NULL,
    subscription_id BIGINT NOT NULL REFERENCES user_subscriptions(id) ON DELETE CASCADE,

    change_type     VARCHAR(20) NOT NULL,  -- upgrade / scheduled_downgrade / cancel_scheduled_change
    from_plan_id    BIGINT,
    to_plan_id      BIGINT NOT NULL,
    from_group_id   BIGINT,
    to_group_id     BIGINT NOT NULL,
    from_tier       INT NOT NULL DEFAULT 0,
    to_tier         INT NOT NULL DEFAULT 0,

    -- 报价冻结（quote 创建时快照，防 TOCTOU；客户端不可篡改）
    old_price_snapshot  DECIMAL(20, 2),
    new_price_snapshot  DECIMAL(20, 2) NOT NULL DEFAULT 0,
    currency            VARCHAR(8) NOT NULL DEFAULT '',
    term_start          TIMESTAMPTZ,
    term_end            TIMESTAMPTZ,
    remaining_seconds   BIGINT NOT NULL DEFAULT 0,
    unused_credit       DECIMAL(20, 2) NOT NULL DEFAULT 0,
    prorated_charge     DECIMAL(20, 2) NOT NULL DEFAULT 0,
    amount_due          DECIMAL(20, 2) NOT NULL DEFAULT 0,
    quote_created_at    TIMESTAMPTZ,
    quote_expires_at    TIMESTAMPTZ,

    effective_at    TIMESTAMPTZ,
    status          VARCHAR(20) NOT NULL DEFAULT 'quoted',
                    -- quoted/pending_payment/paid/fulfilled/scheduled/cancelled/failed/expired
    cancel_reason   VARCHAR(64),          -- superseded_by_upgrade / user_cancel / admin_cancel
    order_id        BIGINT,               -- 关联 payment_orders.id（升级支付）
    idempotency_key VARCHAR(128),
    metadata        JSONB NOT NULL DEFAULT '{}'::jsonb,

    created_at      TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at      TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    paid_at         TIMESTAMPTZ,
    fulfilled_at    TIMESTAMPTZ,
    cancelled_at    TIMESTAMPTZ
);
CREATE INDEX IF NOT EXISTS idx_plan_changes_subscription ON subscription_plan_changes(subscription_id);
CREATE INDEX IF NOT EXISTS idx_plan_changes_user ON subscription_plan_changes(user_id);
CREATE INDEX IF NOT EXISTS idx_plan_changes_status ON subscription_plan_changes(status);
ALTER TABLE subscription_plan_changes ADD COLUMN IF NOT EXISTS deleted_at TIMESTAMPTZ;
CREATE UNIQUE INDEX IF NOT EXISTS uq_plan_changes_idempotency
    ON subscription_plan_changes(idempotency_key)
    WHERE idempotency_key IS NOT NULL AND deleted_at IS NULL;

-- ⑤ 升级订单引用（支付链路：order_type='plan_change' 时金额来自冻结报价）
ALTER TABLE payment_orders ADD COLUMN IF NOT EXISTS plan_change_id BIGINT;
CREATE INDEX IF NOT EXISTS idx_payment_orders_plan_change ON payment_orders(plan_change_id);
