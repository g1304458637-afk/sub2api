-- 学生认证记录（Student Verification）
--
-- 事实边界：本表只证明「用户控制一个有效的校园学生邮箱」，不证明学籍状态。
-- provider 取值预留 HUBU_EMAIL / HUBU_SSO / MUC_EMAIL / MUC_SSO 等，
-- 首版仅实现 HUBU_EMAIL（邮箱 OTP）。
--
-- 与 benefit_claims（迁移 246）的分工：本表是认证行为的审计记录（谁、何时、
-- 用哪个邮箱通过了哪类认证），benefit_claims 是权益领取的幂等事实源。
-- 邮箱统一 normalize（trim + 小写）后存储；email_normalized 参与查询，
-- 不建唯一约束（换邮箱重新认证合法，重复领取由 benefit_claims 拦截）。

CREATE TABLE IF NOT EXISTS student_verifications (
    id               BIGSERIAL PRIMARY KEY,
    user_id          BIGINT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    provider         VARCHAR(50) NOT NULL,                 -- HUBU_EMAIL（首版）；预留 HUBU_SSO / MUC_EMAIL / MUC_SSO
    email            VARCHAR(254) NOT NULL,                -- normalize 后的验证邮箱
    status           VARCHAR(20) NOT NULL DEFAULT 'verified',  -- verified / revoked
    benefit_grant_id BIGINT,                               -- 触发的订阅权益台账（逻辑引用 subscription_grants）
    ip               VARCHAR(64),
    user_agent       VARCHAR(255),
    notes            TEXT,
    verified_at      TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    revoked_at       TIMESTAMPTZ,
    revoked_by       BIGINT,
    created_at       TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS student_verifications_user_id_idx
    ON student_verifications (user_id);

CREATE INDEX IF NOT EXISTS student_verifications_email_idx
    ON student_verifications (email);

CREATE INDEX IF NOT EXISTS student_verifications_provider_status_idx
    ON student_verifications (provider, status);
