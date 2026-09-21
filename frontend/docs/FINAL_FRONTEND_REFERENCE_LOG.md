# FINAL FRONTEND — Reference Decision Log

> 记录每个大 UI 子系统「参考了什么 / 采用什么 / 不采用什么 / 为什么」。
> 视觉基准：用户提供的 Forma AI Pricing Prompt + 2026-09-21 补充的 MUCODE 民大版视觉规范。

## 全局

- **参考**：用户 MUC 民大版视觉规范（红黑金比例 70/20/8/2、品牌红≠状态色、Design Tokens CSS 变量驱动）。
- **采用**：`src/components/pricing/muc-tokens.css` 集中 token（`.muc-scope` class 作用域，Teleport 浮层同挂）；状态色分层 normal=柔白 / high=暖黄 / near_limit=橙 / exhausted=#FF4550 警示红（≠品牌红）/ reward=暖金。
- **不采用**：cyan/aqua 科技蓝；大面积纯红页面；把业务状态渲染成品牌红。
- **动效**：统一 easing `cubic-bezier(0.22,1,0.36,1)`；fast 150-220ms / normal 280-400ms / hero 500-750ms / special success 900-1700ms。全部 HTML/CSS/SVG/RAF 实现，不引入动画库（项目未装 Framer Motion）。

## 1. Pricing（已交付 Step 1，本轮审计补齐）

- **参考**：Forma AI Pricing（背景视频 + boomerang seek + 巨大渐变 watermark + 玻璃卡）、Vercel Pricing（层级清晰的价格卡 + CTA 状态机）、Stripe Pricing（报价明细的 server-authoritative 呈现）。
- **采用**：三档玻璃卡（Basic 黑灰 / Pro 红强调+Popular / Max 黑红+少量金）；CTA 全状态机（当前套餐 aria-disabled / 升级到 X / 下周期切换 / 立即购买）；金额全部来自服务端冻结报价行（`unused_credit` / `prorated_charge` / `amount_due`，decimal 字符串）；移动端横向 scroll-snap。
- **不采用**：Vercel 的月/年切换（后端仅 30 天 cadence）；前端 proration 计算（违反 Source of Truth）。
- **背景视频**：Forma 原视频资产（用户提供 CloudFront URL，16.5MB）入库 `src/assets/muc/pricing-bg.mp4`；`PricingBackground` 实现 throttled boomerang seek（direction/currentTarget/seekPending/lastTs/rafId，防 seek 堆积；卸载时 cancelAnimationFrame + removeEventListener；视频加载失败回退 CSS 极光，功能不受影响；mobile 关闭 grain 噪点层）。

## 2. 我的订阅

- **参考**：Stripe Customer Portal（订阅主卡 + 状态 + 周期信息）、ChatGPT Plus 用量页（周额度进度 + reset 语义）。
- **采用**：深色渐变背景 + 黑红玻璃卡（比 Pricing 克制，无 watermark）；主订阅卡（套餐名/周用量整数百分比条/状态/下次恢复/套餐到期/scheduled change+取消）；重置卡块（可用张数 + 使用 + 确认）；PAYG fallback 用户文案「额度用完后继续使用」+ 开启首次确认（展示当前钱包余额）；钱包摘要卡（余额 + 充值入口，完整流水在 Wallet 页）。
- **Reset 成功动画语义**：后端指标保持 `weekly_usage_percent`（reset 后=0）；动画层使用派生「AVAILABLE QUOTA = 100 − used」，从 current → 100% 的数字 tween + 圆环填充 + 红色 glow 扩散，纯 RAF/CSS，1200-1700ms，`prefers-reduced-motion` 直接淡入完成态。**必须先 API 成功再播动画。**
- **不采用**：把后端百分比改成"剩余额度"字段（后端语义不动）。

## 3. Wallet

- **参考**：Lago / OpenMeter 的 credits UI、智谱"赠送额度"卡片仪式感。
- **采用**：单一钱包（不拆充值/学生/活动钱包），顶部余额 hero + 充值入口；MucRewardGiftCard（黑红玻璃+暖金边+light sweep+红金小粒子，纯 CSS/HTML，金额 count-up，**无「领取」按钮**——Reward 已自动入账，只有「查看钱包/知道了」）。
- **Gift Card 去重**：以 reward grant 标识做 localStorage acknowledgement（`reward_seen:{grant_id}`），仅 UI 层，不触碰后端幂等。当前后端无用户侧 reward 记录端点 → 见 BLOCKED 清单，静态样式先行。
- **不采用**：拆分多钱包 tab；领取式（claim）交互。

## 4. 订单 / 记录

- **参考**：Stripe Dashboard Payments 列表（类型徽章 + 状态 + 详情 Drawer）。
- **采用**：现有 UserOrdersView 渐进增强（不新建页面）：订单类型分类（购买/续费/升级/钱包充值）+ 套餐变更记录区（来自 `/subscriptions/:id/changes`，scheduled downgrade 不是支付订单，单独呈现）。

## 5. API Key 页

- **参考**：现状已完善，仅做一致性视觉 + 权益状态增强（当前权益/本周使用/状态；升级后 Key 自动迁移文案，绝不做「请重新生成 Key」暗示；不泄露完整 Secret）。

## 6. MUCODE 客户端

- **参考**：现有悬浮球 + 展开卡（Phase 5 status contract）；Steam/游戏 overlay 的紧凑信息层级。
- **采用**：悬浮球**恒显钱包余额**（语义稳定，不因订阅/按量切换）；展开卡=订阅块（套餐/本周%/恢复日/重置卡×N[使用]）+ 钱包块（余额/继续使用 ON）+ 用量块（今日/累计/Top Model）+ 刷新；Reset 复用与 Website 相同「AVAILABLE QUOTA → 100%」动画语义（共享 helper，小卡尺寸）；Settings > Subscription 只读 + [管理套餐] 打开 Website Pricing（不在客户端做支付）。

## 7. Admin

- **参考**：Stripe Dashboard（数据密度 + Drawer 详情）、Lago Admin（订单/流水）、现有 sub2api admin 框架。
- **采用**：同一 token 家族但克制（深黑后台+玻璃/深灰+民大红 Active+白数据+暖金 Reward；无背景视频、无 watermark）；Dashboard 核心数字 + 需要处理区（支付成功未履约/批量重置失败项，点击跳对应过滤）；用户详情 Customer 360（Wallet/Subscription/ResetCards/APIKeys 四摘要 + 快捷操作，内部 USD 字段默认折叠）；Reset Center 双 Tab（直接重置≠重置卡，Preview→Execute→Progress→Retry 失败项）；Reward Center 仅管配置+记录（不碰学生认证流程）；Finance 三 Tab（订单/套餐变更/钱包流水）+ 重新履约按钮（明确不再收款）。
- **不采用**：向运营暴露 idempotency_key 等技术字段；Admin 庆祝类动画（批量重置只做专业 progress）。

## FRONTEND_BLOCKED_BY_API 清单（不自行改后端）

1. **用户端钱包流水**：无用户侧 balance ledger 端点（admin 侧 `/users/:id/balance-history` 仅为充值码历史）。→ Wallet 页先做余额+充值+奖励卡片记录区（静态 Gift Card 样式记录），流水等后端端点。
2. **Reward 记录查询**：`reward_grants` 无任何用户/管理端列表端点。→ Admin Reward Center 先做配置（settings API 已有 `student_verification_reward_*`）+ 记录区留空态说明；User Detail 的 Rewards 区同样留接口位。
3. **Plan tier_rank 管理**：`CreatePlanRequest/UpdatePlanRequest` 无 tier_rank 字段。→ Admin 套餐管理展示 tier_rank（若列表返回），编辑留 API 位。
4. **订阅健康聚合/今日支付等 Dashboard 聚合端点**：无专用聚合 API。→ Dashboard 用现有列表端点客户端聚合计数（usage_status 分档用后端返回值，不自行算阈值）。
