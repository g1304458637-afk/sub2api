-- Subscription V1 Phase 1: PAYG fallback flag, group concurrency override,
-- and the reset event/application/card audit tables.
--
-- 严格 additive：不修改任何现有列，不重置任何订阅数据。
-- 迁移后旧行为与迁移前完全一致：
--   - user_subscriptions.auto_payg_fallback = false（现状：超限拒绝，不转余额）
--   - groups.concurrency_override = NULL（现状：并发仍取 users.concurrency）
--   - weekly_usage_usd / weekly_window_start / expires_at 全部不触碰
--
-- Re-Anchoring 仲裁 invariant（未来 worker 必须遵守，schema 为其提供基础）：
--   1. subscription_reset_applications UNIQUE(reset_event_id, user_subscription_id)
--      保证同一事件对同一订阅至多应用一次（worker retry-safe）。
--   2. 未来 Global Reset worker 更新锚点时必须携带守卫
--      `weekly_window_start IS NULL OR weekly_window_start < :effective_at`：
--      用户在事件之后用 Reset Card 推进过的新锚点绝不允许被旧事件回拨。
--   3. weekly_window_start 是唯一周期锚点（不新增 anchor 列），
--      任何 Reset 的成功都体现为「usage 清零 + anchor = 事件 effective_at」。

-- ── 1) 用户级 PAYG fallback 开关（个人选择，非分组属性） ──
ALTER TABLE user_subscriptions ADD COLUMN IF NOT EXISTS auto_payg_fallback BOOLEAN NOT NULL DEFAULT false;

-- ── 2) 分组级并发权益覆盖（可空；NULL = 不提供额外并发权益） ──
ALTER TABLE groups ADD COLUMN IF NOT EXISTS concurrency_override INTEGER;
-- 运行时语义（Phase 7 实现，此处仅约束数据）：
--   effective_concurrency = max(users.concurrency, 该用户全部有效订阅分组的 concurrency_override)
ALTER TABLE groups ADD CONSTRAINT chk_groups_concurrency_override_positive
    CHECK (concurrency_override IS NULL OR concurrency_override > 0);

-- ── 3) 批量 Reset / 发卡事件（系统与管理员动作的唯一事实源） ──
CREATE TABLE IF NOT EXISTS subscription_reset_events (
    id            BIGSERIAL PRIMARY KEY,
    event_type    VARCHAR(32) NOT NULL,                    -- global_reset / reset_card_grant
    status        VARCHAR(20) NOT NULL DEFAULT 'pending',  -- pending/running/completed/failed/canceled
    -- Global Reset 的权威时间：worker 实际处理时刻不参与锚点计算，
    -- 所有目标订阅的新锚点一律取该值。
    effective_at  TIMESTAMPTZ NOT NULL,
    scope_type    VARCHAR(32) NOT NULL,                    -- all/group/plan/user/subscription
    -- scope 过滤条件（确定性解释，禁止 nullable 列组合）：
    --   {"group_ids": [1,2]} | {"plan_tier": "pro"} | {"user_ids": [7]}
    --   | {"subscription_ids": [9]} | {}
    scope         JSONB NOT NULL DEFAULT '{}'::jsonb,
    reason        TEXT NOT NULL DEFAULT '',
    campaign      VARCHAR(64),
    created_by    BIGINT REFERENCES users(id) ON DELETE SET NULL,
    started_at    TIMESTAMPTZ,
    completed_at  TIMESTAMPTZ,
    metadata      JSONB NOT NULL DEFAULT '{}'::jsonb,
    created_at    TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at    TIMESTAMPTZ NOT NULL DEFAULT NOW()
);
CREATE INDEX IF NOT EXISTS idx_subscription_reset_events_status ON subscription_reset_events(status);
CREATE INDEX IF NOT EXISTS idx_subscription_reset_events_effective_at ON subscription_reset_events(effective_at);

-- ── 4) 事件 → 订阅 的逐条应用记录（retry-safe + 可审计） ──
CREATE TABLE IF NOT EXISTS subscription_reset_applications (
    id                            BIGSERIAL PRIMARY KEY,
    reset_event_id                BIGINT NOT NULL REFERENCES subscription_reset_events(id) ON DELETE RESTRICT,
    user_subscription_id          BIGINT NOT NULL REFERENCES user_subscriptions(id) ON DELETE CASCADE,
    -- 冗余自事件，使单行即可回答「应用时承诺的新锚点是何时」
    effective_at                  TIMESTAMPTZ NOT NULL,
    previous_weekly_window_start  TIMESTAMPTZ,
    previous_weekly_usage_usd     DECIMAL(20, 10),
    applied_at                    TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    status                        VARCHAR(20) NOT NULL DEFAULT 'applied',  -- applied/skipped/failed
    metadata                      JSONB NOT NULL DEFAULT '{}'::jsonb,
    -- retry-safe 基石：同事件对同订阅至多一行
    CONSTRAINT uq_subscription_reset_applications_event_sub UNIQUE (reset_event_id, user_subscription_id)
);
CREATE INDEX IF NOT EXISTS idx_subscription_reset_applications_subscription
    ON subscription_reset_applications(user_subscription_id);

-- ── 5) Reset Card（一次性可消费的周期重置权益，逐卡一行） ──
CREATE TABLE IF NOT EXISTS subscription_reset_cards (
    id                    BIGSERIAL PRIMARY KEY,
    user_id               BIGINT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    status                VARCHAR(20) NOT NULL DEFAULT 'available',  -- available/used/expired/revoked
    scope                 VARCHAR(20) NOT NULL DEFAULT 'weekly',     -- V1 仅 weekly
    source_type           VARCHAR(32) NOT NULL DEFAULT 'admin_grant',
    campaign              VARCHAR(64),
    -- 活动发卡的事件回链；管理员单发可为 NULL
    grant_event_id        BIGINT REFERENCES subscription_reset_events(id) ON DELETE RESTRICT,
    -- 活动内序号（0 起）：支持同一活动每人 N 张的幂等
    grant_index           INTEGER NOT NULL DEFAULT 0,
    granted_at            TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    expires_at            TIMESTAMPTZ,
    used_at               TIMESTAMPTZ,
    -- 消费时才绑定的目标订阅（用户可同时持有多分组订阅，卡发放时不预设）
    used_subscription_id  BIGINT REFERENCES user_subscriptions(id) ON DELETE SET NULL,
    created_by            BIGINT REFERENCES users(id) ON DELETE SET NULL,
    notes                 TEXT NOT NULL DEFAULT '',
    metadata              JSONB NOT NULL DEFAULT '{}'::jsonb,
    created_at            TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at            TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    deleted_at            TIMESTAMPTZ
);
CREATE INDEX IF NOT EXISTS idx_subscription_reset_cards_user_status ON subscription_reset_cards(user_id, status);
CREATE INDEX IF NOT EXISTS idx_subscription_reset_cards_expires_at ON subscription_reset_cards(expires_at);
CREATE INDEX IF NOT EXISTS idx_subscription_reset_cards_grant_event ON subscription_reset_cards(grant_event_id);
-- 批量发卡幂等：同一活动同一用户同一序号至多一张（活动 retry 不会重复发卡）；
-- 仅约束活动卡（grant_event_id 非空），管理员单发不受影响。
CREATE UNIQUE INDEX IF NOT EXISTS uq_subscription_reset_cards_grant_idempotency
    ON subscription_reset_cards(grant_event_id, user_id, grant_index)
    WHERE grant_event_id IS NOT NULL AND deleted_at IS NULL;
