-- 奖励发放流水表（Reward Grant）
--
-- 所有"系统奖励余额"（学生认证奖励、未来的注册奖励/邀请奖励等）的唯一事实源。
-- 奖励不是充值：本表金额不得计入 total_recharged / 累计充值口径。
--
-- 幂等核心：UNIQUE(idempotency_key) —— 每一个业务上的"奖励动作"必须提供一个确定性
-- idempotency_key（由发放方从业务事实派生，如学生认证 = student_verification:{userID}:{campaign}，
-- 未来 referral / 补偿各有自己的 key 形状）。发放走 INSERT ... ON CONFLICT (idempotency_key)
-- DO NOTHING，affected = 0 即已发放，不得再次加余额。
-- campaign 保留为活动归属字段（不参与唯一约束）；source_type / source_id 保留为业务来源审计
-- 字段；source_id 是对来源业务表（如未来的 student_verifications）的逻辑引用，故意不建外键：
-- 来源 schema 不属于奖励模块。

CREATE TABLE IF NOT EXISTS reward_grants (
    id              BIGSERIAL PRIMARY KEY,
    user_id         BIGINT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    idempotency_key VARCHAR(200) NOT NULL,               -- 幂等键：业务动作的确定性唯一标识
    source_type     VARCHAR(50) NOT NULL,                -- 发放来源，如 'student_verification'
    source_id       BIGINT,                              -- 来源业务行 ID（逻辑引用，无外键）
    campaign        VARCHAR(64) NOT NULL DEFAULT '',     -- 活动归属标识（审计/运营维度，非唯一）
    amount          DECIMAL(20, 8) NOT NULL DEFAULT 0,   -- 奖励金额（USD，系统内部余额单位）
    granted_by      BIGINT,                              -- 触发的管理员 ID（NULL = 系统自动）
    metadata        JSONB NOT NULL DEFAULT '{}',         -- 扩展信息（来源模块自定义）
    created_at      TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

-- 幂等约束：同一个业务奖励动作（确定性 key）只能存在一条发放记录
CREATE UNIQUE INDEX IF NOT EXISTS reward_grants_idempotency_key_key
    ON reward_grants (idempotency_key);

CREATE INDEX IF NOT EXISTS reward_grants_user_id_idx
    ON reward_grants (user_id);

-- (user, source, campaign) 组合保留为非唯一索引：审计与"某活动给某用户发过几笔"查询用
CREATE INDEX IF NOT EXISTS reward_grants_user_source_campaign_idx
    ON reward_grants (user_id, source_type, campaign);

CREATE INDEX IF NOT EXISTS reward_grants_source_idx
    ON reward_grants (source_type, source_id);

-- 学生认证奖励配置（开关与认证功能本身解耦，默认全部关闭）
INSERT INTO settings (key, value)
VALUES
    ('student_verification_reward_enabled', 'false'),
    ('student_verification_reward_amount', '0.00000000'),
    ('student_verification_reward_campaign', '')
ON CONFLICT (key) DO NOTHING;
