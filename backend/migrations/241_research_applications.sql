-- 科研优惠登记（Research Application）
--
-- 功能语义：用户提交科研身份证明（文字说明 + 附件图片/PDF）→ 管理员在管理端审核；
-- 通过时后端自动给该用户静默发放等额余额兑换券（redeem_codes + RedeemForAdminFulfillment）；
-- 驳回必须填写备注。两张表：
--   1. research_applications         申请主表（附件元数据冗余存 attachments jsonb，不含物理路径）
--   2. research_attachment_uploads   附件"待绑定"上传记录（物理路径 storage_path 只存本表；
--      下载时由 DB 取 path + DATA_DIR 拼绝对路径，禁止用户输入参与路径拼接，防穿越）
--
-- 字段与 ent schema（ent/schema/research_application.go、research_attachment_upload.go）
-- 完全一致；故意不建外键（与 reward_grants 迁移做法一致，避免跨模块强耦合）。
-- 幂等：CREATE TABLE IF NOT EXISTS / CREATE INDEX IF NOT EXISTS。

CREATE TABLE IF NOT EXISTS research_applications (
    id                   BIGSERIAL PRIMARY KEY,
    created_at           TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at           TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    user_id              BIGINT NOT NULL,                    -- 申请人
    description          TEXT NOT NULL,                      -- 科研身份说明（1..2000 字）
    status               VARCHAR(20) NOT NULL DEFAULT 'pending', -- pending/approved/rejected
    reviewer_id          BIGINT,                             -- 审核管理员（NULL = 未审核）
    review_notes         TEXT,                               -- 审核备注（驳回时必填）
    reward_amount        DOUBLE PRECISION,                   -- 通过时发放的余额金额
    issued_redeem_code_id BIGINT,                            -- 发放的 redeem_codes.id（静默到账记录）
    reviewed_at          TIMESTAMPTZ,                        -- 审核时间
    attachments          JSONB                               -- [{id,name,mime,size}] 附件元数据（无 path）
);

CREATE INDEX IF NOT EXISTS researchapplication_user_id
    ON research_applications (user_id);

CREATE INDEX IF NOT EXISTS researchapplication_status
    ON research_applications (status);

CREATE TABLE IF NOT EXISTS research_attachment_uploads (
    id             VARCHAR(36) PRIMARY KEY,                   -- UUID 字符串主键
    created_at     TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at     TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    user_id        BIGINT NOT NULL,                           -- 上传者（下载权限 = 仅本人/管理员）
    original_name  TEXT NOT NULL,                             -- 原始文件名（仅用于下载头，RFC 5987）
    mime           VARCHAR(100) NOT NULL,                     -- 白名单内的 MIME 类型
    size           BIGINT NOT NULL,                           -- 字节数（≤5MB）
    storage_path   TEXT NOT NULL,                             -- 相对存储路径（uploads/research/<uuid><ext>）
    application_id BIGINT                                     -- 绑定的申请 ID（NULL = 未绑定/待绑定）
);

CREATE INDEX IF NOT EXISTS researchattachmentupload_user_id
    ON research_attachment_uploads (user_id);

CREATE INDEX IF NOT EXISTS researchattachmentupload_application_id
    ON research_attachment_uploads (application_id);
