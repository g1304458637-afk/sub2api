-- 订阅权益发放台账（Subscription Grant Ledger）
--
-- "Grant" 回答「为什么、由谁、通过什么活动给了用户一份订阅权益」；
-- "Subscription"（user_subscriptions）回答「用户现在实际拥有什么订阅」。
-- 二者概念分离：user_subscriptions 继续由 SubscriptionService 独占管理，
-- 本表只记录权益的来源、幂等身份、生效策略与生命周期，绝不允许来源模块
-- 直接 INSERT user_subscriptions。
--
-- 幂等核心：
--   1. UNIQUE (source, source_key) WHERE source_key IS NOT NULL —— 每一个业务上
--      的"赠送动作"（学生认证 = benefit_code:normalized_email；管理员批量 =
--      请求幂等键:userID）只能存在一条台账记录；
--   2. benefit_claims 表（迁移 247）以 (benefit_code, identity_key) 与
--      (benefit_code, user_id) 双唯一约束锁定"一人一号一权益"。
-- 管理员单次赠送允许重复发放（source_key 为 NULL），由 API 层
-- IdempotencyCoordinator（Idempotency-Key 头）防止浏览器重复提交。
--
-- 撤销安全：fulfilled Grant 记录 contribution_end（本 Grant 贡献到的到期时刻），
-- 撤销时受"付费地板"保护 —— 只回收纯赠送时段，绝不缩短用户付费购买的权益
-- （见 SubscriptionGrantService.RevokeGrant）。
--
-- 收入口径：本表产生的订阅没有 payment_order / subscription_terms 付费快照，
-- 天然不进入收入统计。

CREATE TABLE IF NOT EXISTS subscription_grants (
    id                     BIGSERIAL PRIMARY KEY,
    user_id                BIGINT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    group_id               BIGINT NOT NULL,                      -- 目标分组（逻辑引用 groups，台账先于分组生命周期）
    plan_id                BIGINT,                               -- 可选：展示用套餐身份（逻辑引用）
    source                 VARCHAR(50) NOT NULL,                 -- admin_grant / student_verification / campaign / ...
    source_key             VARCHAR(200),                         -- 业务幂等键（可 NULL：管理员单次赠送允许重复）
    benefit_code           VARCHAR(64),                          -- 权益身份（如 HUBU_STUDENT_WELCOME），管理员赠送为 NULL
    identity_type          VARCHAR(50),                          -- 权益身份类型（如 hubu_email）
    identity_key           VARCHAR(200),                         -- 权益身份键（normalize 后的验证邮箱等）
    idempotency_key        VARCHAR(200),                         -- API 层幂等键（审计用途，约束在 idempotency_records）
    status                 VARCHAR(20) NOT NULL DEFAULT 'pending',   -- pending/fulfilled/expired/revoked/failed
    effective_policy       VARCHAR(20) NOT NULL DEFAULT 'immediate', -- immediate / end_of_term
    duration_days          INT NOT NULL,                         -- 赠送时长（系统原生按天语义）
    reason                 VARCHAR(500),                         -- 赠送原因（面向审计）
    notes                  TEXT,                                 -- 内部备注
    operator_user_id       BIGINT,                               -- 操作管理员（NULL = 系统自动）
    linked_subscription_id BIGINT,                               -- 激活后的订阅行（逻辑引用 user_subscriptions）
    contribution_start     TIMESTAMPTZ,                          -- 本 Grant 贡献时段起点（激活时的旧到期时刻或 now）
    contribution_end       TIMESTAMPTZ,                          -- 本 Grant 贡献时段终点（激活后的到期时刻）
    activated_at           TIMESTAMPTZ,
    revoked_at             TIMESTAMPTZ,
    revoked_by             BIGINT,
    revoke_reason          VARCHAR(500),
    failure_reason         VARCHAR(500),
    created_at             TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at             TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

-- 业务幂等：同一 (source, source_key) 永远只有一条台账（含已撤销，防止
-- "撤销后再重新领取"的循环薅取）。
CREATE UNIQUE INDEX IF NOT EXISTS subscription_grants_source_key_key
    ON subscription_grants (source, source_key)
    WHERE source_key IS NOT NULL;

CREATE INDEX IF NOT EXISTS subscription_grants_user_id_idx
    ON subscription_grants (user_id);

CREATE INDEX IF NOT EXISTS subscription_grants_status_idx
    ON subscription_grants (status);

CREATE INDEX IF NOT EXISTS subscription_grants_source_idx
    ON subscription_grants (source);

CREATE INDEX IF NOT EXISTS subscription_grants_benefit_code_idx
    ON subscription_grants (benefit_code);

CREATE INDEX IF NOT EXISTS subscription_grants_linked_subscription_idx
    ON subscription_grants (linked_subscription_id)
    WHERE linked_subscription_id IS NOT NULL;

-- pending 激活扫描：worker 每分钟拉取待激活台账
CREATE INDEX IF NOT EXISTS subscription_grants_pending_scan_idx
    ON subscription_grants (created_at, id)
    WHERE status = 'pending';

-- fulfilled 台账的贡献期结束落库（worker 把贡献期已结束的 fulfilled 置为 expired）
CREATE INDEX IF NOT EXISTS subscription_grants_fulfilled_expiry_idx
    ON subscription_grants (contribution_end)
    WHERE status = 'fulfilled';

-- 一次性权益领取台账（Benefit Claim）
--
-- 回答「这个权益身份/这个用户是否已经领取过某个 benefit」。
-- 与 subscription_grants 分离的原因：claim 是**永久事实**（即使 Grant 被撤销，
-- 领取资格也已消耗），必须独立于 Grant 生命周期存在，且撤销 Grant 不得删除 claim。
CREATE TABLE IF NOT EXISTS benefit_claims (
    id            BIGSERIAL PRIMARY KEY,
    benefit_code  VARCHAR(64) NOT NULL,                  -- 权益身份（如 HUBU_STUDENT_WELCOME）
    user_id       BIGINT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    identity_type VARCHAR(50) NOT NULL,                  -- 如 hubu_email
    identity_key  VARCHAR(200) NOT NULL,                 -- normalize 后的验证邮箱等稳定身份键
    grant_id      BIGINT,                                -- 关联的 subscription_grants.id（逻辑引用）
    created_at    TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

-- 同一权益身份（同一验证邮箱）全局只能领取一次 —— 换账号也无法重复领取。
CREATE UNIQUE INDEX IF NOT EXISTS benefit_claims_benefit_identity_key
    ON benefit_claims (benefit_code, identity_key);

-- 同一用户对同一权益只能领取一次 —— 换另一个邮箱也不能重复领取。
CREATE UNIQUE INDEX IF NOT EXISTS benefit_claims_benefit_user_key
    ON benefit_claims (benefit_code, user_id);

CREATE INDEX IF NOT EXISTS benefit_claims_user_id_idx
    ON benefit_claims (user_id);

-- 学生认证权益配置（开关与认证功能本身解耦，默认全部关闭）
INSERT INTO settings (key, value)
VALUES
    ('student_verification_enabled', 'false'),
    ('student_benefit_group_id', '0'),
    ('student_benefit_plan_id', '0'),
    ('student_benefit_validity_days', '30')
ON CONFLICT (key) DO NOTHING;
