-- 展示价（Presentation Pricing）独立层
--
-- 事实边界：本表只决定用户端「模型与价格」页面看到什么标准价格，不参与任何
-- 真实计费。真实扣费（订阅额度 / 钱包余额 / Usage charge）仍完全由现有
-- Billing 体系（渠道定价、分组模型定价、倍率、分时、长上下文阶梯）决定。
-- 依赖方向：展示层可读取 Billing 价格作为 fallback；Billing 永不读取本表。
--
-- model_name 绑定渠道 SupportedModels 的 canonical 模型名（API 调用名），
-- 与 display_name / 中文别名 / provider 标签无关，改名不会导致配置丢失。
-- 价格以账本币种（USD per token / USD per 次）存储；前端展示层按现有
-- 币种切换逻辑换算，不引入第二套汇率。
--
-- 回滚：本表为纯新增表，整表移除即可（幂等方式），不触碰任何既有
-- pricing / usage / subscription / wallet 数据。

CREATE TABLE IF NOT EXISTS presentation_model_pricing (
    id                   BIGSERIAL PRIMARY KEY,
    model_name           VARCHAR(255) NOT NULL UNIQUE,      -- canonical model key
    billing_mode         VARCHAR(20)  NOT NULL DEFAULT 'token',  -- token / image / per_request / video
    currency             VARCHAR(10)  NOT NULL DEFAULT 'USD',
    input_price          DOUBLE PRECISION,                  -- USD / token
    output_price         DOUBLE PRECISION,                  -- USD / token
    cache_write_price    DOUBLE PRECISION,                  -- USD / token（5m 缓存写入）
    cache_write_1h_price DOUBLE PRECISION,                  -- USD / token（1h 缓存写入）
    cache_read_price     DOUBLE PRECISION,                  -- USD / token（缓存读取）
    per_request_price    DOUBLE PRECISION,                  -- USD / 次（按次、按图、按视频）
    enabled              BOOLEAN      NOT NULL DEFAULT TRUE,
    remark               TEXT,
    updated_by           BIGINT,                            -- 最后修改的管理员
    created_at           TIMESTAMPTZ  NOT NULL DEFAULT NOW(),
    updated_at           TIMESTAMPTZ  NOT NULL DEFAULT NOW()
);

COMMENT ON TABLE presentation_model_pricing IS
    '展示价层：仅影响「模型与价格」页面展示，不参与真实计费。manual override 为空时按 manual → official → billing 顺序回退。';
