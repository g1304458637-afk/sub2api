# Subscription V1 Backend Reference Log

> Mature Reference Gate 记录：每个子系统编码前调研了什么、采用了什么、为什么、没采用什么、为什么。
> 阶段性补充会随实现持续追加。

---

## Phase 2 — Unified Weekly Re-Anchoring Reset Core

**调研**：Kill Bill entitlement lifecycle（block/phase 状态机）、Lago subscription reset 语义、PostgreSQL CAS（`UPDATE ... WHERE expected`）与 `SELECT FOR UPDATE` 惯例；本仓库既有 `ResetWeeklyUsage(expected, new)` CAS、`IncrementUsage` 原地自增、`GetByIDForUpdate` 行锁。

**采用**：
- 复用仓库既有 CAS reset 与行锁作为唯一 Reset 实现（源码级证明 Reset↔Settlement 无 lost update）；
- claim-first + 同事务 finalize 的 application 模式（UNIQUE(event, subscription) 兜底 retry 幂等）；
- 事务复用检测（`dbent.TxFromContext`）避免嵌套事务。

**未采用**：重建第二套 reset SQL；外部 workflow 引擎；SKIP LOCKED（单订阅重置是点操作，不是队列）。

## Phase 4/4.1 — Account Status Contract

**调研**：Lago（wallet/customer 分离、金额透明）、OpenMeter（entitlement 由服务端计算成状态，客户端不接触 limit）、Kill Bill（user/admin 视图分离）；Stripe 金额序列化（minor units / decimal 字符串）。

**采用**：单一 `AccountStatusService` 作为 Website/MUCODE 共享事实源；整数百分比合同（floor + >=100 钳制，状态按 raw 分档）；`display_name` = Group 名（entitlement 身份，不伪造 SKU 身份）；金额 8 位小数字符串对齐 NUMERIC(20,8)。

**未采用**：第二钱包/币种换算进账本；客户端百分比计算；给 UserSubscription 伪造 plan_id。

## Phase 6 — Reset Card Runtime (+Scoped Grant)

**调研**：Lago consumable credits（带状态/过期账本行）、Kill Bill / Stripe durable Idempotency-Key + stored response（复用仓库 `IdempotencyCoordinator` + `idempotency_records`）、PostgreSQL FOR UPDATE + FIFO 确定性选卡。

**采用**：卡表即账本（不建第二 ledger）；durable 幂等（grant/consume 双入口）；FIFO 选卡（最早到期→最早创建→最小 id）；单事务"卡 CAS + Reset Core"，失败整体回滚（卡不白烧）；惰性过期（查询即判定，无批量 mutation）。

**未采用**：内存幂等；后台 expire 扫描；按 Group 发 scoped card（V1 卡是账户级）。

## Phase 7 — Scoped / Batch Direct Reset Runtime

**调研**：graphile-worker / PgBoss 的 SKIP LOCKED job-claim 模式（DB 即队列）；OpenMeter batch 重算 / Lago batch invoice finalize 的批处理+计数+可重试设计；本仓库 ent/tx/raw SQL 设施。

**采用**：DB-backed worker（applications 行即任务）；`FOR UPDATE SKIP LOCKED` 单行认领（多 worker 安全）；认领与执行分连接（认领独立提交为 applying，执行事务显式 COMMIT——缺失 COMMIT 是本轮真实根因 bug）；snapshot 于事件创建时固定目标集合；stale guard 保留 Reset Card 赢过旧批次的锚点。

**未采用**：Redis 队列/外部调度器；worker 运行时重新 SELECT 目标（防漂移）；一次性大事务批量 reset（锁面过大）。

## Phase 8 — PAYG Fallback Runtime

**调研**：Lago allowances/overage（额度尽→计费降级而非拒绝）、OpenMeter usage balance/overage、hybrid subscription+PAYG 计费模式；本仓库 `UsageBillingCommand` 双字段（SubscriptionCost/BalanceCost）与 ops-fallback nil 传播先例（`gateway_handler.go:1018`）。

**采用**：集中式裁决在 `CheckBillingEligibility`（返回 fallback bool），所有 handler 以"清空请求级 subscription"传播模式——复用 Phase 0 锁定的 nil→BalanceCost 结算路径；platform quota 对 fallback 生效（视同 standard）；fallback 请求 `billing_type=balance`、`subscription_id=NULL` 天然可归因。

**未采用**：拆单计费（V1 明确禁止）；第二套 billing transaction；在多个 handler 分散判断限额。

## Phase 9 — Concurrency Entitlement

**调研**：entitlement precedence / plan override 聚合模式（Stripe Billing usage limits per subscription、Kong rate-limit aggregation）；本仓库 auth cache 快照 + `users.concurrency <= 0 = unlimited`（Phase 3D 拍板）+ Redis ZSET 用户并发池（Phase 0 实证全 Key 共池）。

**采用**：`effective = max(users.concurrency, max active metered group override)`；在 auth lookup 处一次解析（缓存 TTL 吸收新鲜度，购买/过期/管理更新走既有失效）；窄可选接口（Reader/Store）避免污染大接口与几十个测试 stub。

**未采用**：每个请求实时聚合（热路径成本）；按当前请求 Group 的 override（Phase 0 证明同池不同限语义不自洽）；负值/0 覆盖（0=unlimited 不得被限）。

## Phase 10 — Plan Change Runtime

**调研**：Stripe `proration_behavior=always_invoice`（升级立即生效 + 未用时间 credit + 立即开票）、Stripe `pending_update` / Chargebee `end_of_term`（降级 term 末生效）、Chargebee `update_subscription_estimate`（服务端权威报价预览）、OpenMeter grants / Lago 订阅+钱包分离（entitlement 切换不触碰 Wallet）；本仓库 payment_orders（plan_id/amount/subscription_days 真实支付事实）与 assignOrExtend 履约链。

**采用**：
- Quote 冻结（30 分钟）：金额/双价快照/剩余窗口在报价行落库，创建订单只读冻结行（客户端 amount 一律忽略；过期重新报价）——Stripe quote / Chargebee estimate 模式；
- 立即升级 proration：按未消费 term 逐段 decimal 折算（毫秒级 ratio；未消费起点 = max(now, termStart) 防未来段双重计入——45/60 天测试抓出的真实 bug）；credit 不为负；
- 履约单事务：订阅行锁 + 源组校验 + 目标组冲突终检 + SwitchPlan（保 usage/anchor/starts/expires/fallback）+ 升级 term + Key 组迁移（ID/secret 不变）+ 取消旧 scheduled downgrade + 提交后缓存失效；
- Scheduled Downgrade：next_plan_id + 审计行；当前 term 权益不变；Renewal 时按目标档报价执行；升级 supersede 旧降级（superseded_by_upgrade）；
- 四闸门：plan_id identity（历史 NULL=unresolved 拒绝）、term 快照价格真相（非目录价）、逐段预付折算、tier_rank（≠sort_order）。

**未采用**：客户端提交金额；目录价充当历史实付；多段平均化；自动 merge 目标组已有订阅；跨币种/跨 cadence（V1 显式拒绝）；独立 invoice 系统。
