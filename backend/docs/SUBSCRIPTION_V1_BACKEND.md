# Subscription V1 Backend

> 状态：**BACKEND READY**（Phase 0-11 全部完成；Full Backend E2E 通过）。
> 剩余工作仅为最终 Website / MUCODE 前端。

## 1. Architecture

```
Plan (SKU: 价格/周期/for_sale)          ── 销售身份
  └─ Group (runtime entitlement)        ── 权益身份：subscription_type / weekly_limit /
        │                                  model_allowlist / rate_multiplier / concurrency_override
        └─ UserSubscription (User×Group) ── 权益实例：生命周期 + weekly_usage + anchor

Wallet (users.balance, USD, NUMERIC(20,8)) ── PAYG 余额，与订阅额度严格分离

Reset System
├── Reset Core: ResetSubscriptionWeeklyPeriod（唯一 weekly_usage=0 实现；
│   stale guard: anchor >= effectiveAt → skip）
├── Reset Card（账户级一次性权益）：scoped grant → hold → user consume（FIFO 选卡，
│   单事务卡 CAS + Reset Core；durable idempotency）
└── Direct Reset（立即执行，scoped/batch）：event → snapshot subscription_ids →
    SKIP LOCKED worker → 每 application 一事务 Reset Core

Billing: UsageBillingCommand{SubscriptionCost | BalanceCost} 单事务结算（dedup 幂等）
AccountStatusService: Website/MUCODE 共享状态合同（wallet + 整数百分比 + reset_cards）
PAYG Fallback: 限额闸门降级为钱包准入；请求级 subscription 清空 → BalanceCost
Concurrency: effective = max(users.concurrency, max active metered group override)；<=0 = unlimited
```

## 2. 语义定稿（冻结）

- 百分比：`raw>=100→100` 否则 `floor(raw)`；usage_status 按 raw 分档（<70 normal / <90 high / <100 near_limit / >=100 exhausted；NULL limit = unmetered）
- 周周期：anchored 7-day（锚点=weekly_window_start）；`weekly_period_ends_at = min(anchor+7d, expires_at)`；Reset 不改 started_at/expires_at/balance
- Reset 来源审计：`admin_direct / batch_direct / reset_card / compensation`
- Direct Reset ≠ Reset Card（立即执行 vs 可保存权益）；两者都必须支持定向；均复用 Reset Core
- 0 并发 = unlimited（override 只抬底不封顶）
- PAYG fallback：单请求单模式（禁止拆单）；fallback 不绕过任何权限闸门

## 3. API Map

### User
| 方法 | 路径 | 说明 |
| --- | --- | --- |
| GET | /api/v1/subscriptions/status | 统一账户状态（wallet + reset_cards + 全部活跃订阅净化视图） |
| POST | /api/v1/subscriptions/:id/reset-with-card | 消费重置卡（Idempotency-Key 必填；服务端选卡） |
| PATCH | /api/v1/subscriptions/:id/payg-fallback | 切换 PAYG fallback |
| GET | /api/v1/subscriptions（legacy） | 兼容面（DEPRECATED COMPATIBILITY SURFACE） |
| GET | /subscriptions/active/summary/progress（legacy） | 同上 |

### Admin（均在 adminAuth + auditLog + compliance guard 组内）
| 方法 | 路径 | 说明 |
| --- | --- | --- |
| POST | /admin/subscription-resets | 创建 Direct Reset 事件（subscription_ids/users/groups/all_active） |
| POST | /admin/subscription-resets/preview | 目标预览（无写入） |
| GET | /admin/subscription-resets | 事件列表 |
| GET | /admin/subscription-resets/:id | 事件 + 进度统计 |
| POST | /admin/subscription-resets/:id/retry | failed → pending 重试 |
| POST | /admin/subscription-reset-cards/grants | 定向发卡（users/groups/all_active_users + quantity_per_user；Idempotency-Key 必填） |
| POST | /admin/subscription-reset-cards/grants/preview | 发卡预览（无写入） |
| GET | /admin/subscription-reset-cards | 按用户/状态列卡 |
| GET | /admin/subscription-reset-cards/count | 可用卡计数 |
| POST | /admin/subscription-reset-cards/:id/revoke | 撤销可用卡 |
| POST/PUT/DELETE | /admin/payment/plans（既有） | Plan CRUD |
| POST/PUT | /admin/groups（既有，含 concurrency_override） | Group CRUD |
| POST | /admin/subscriptions/:id/reset-quota（既有） | 单订阅手动重置（走 Reset Core） |

### Plan Change（用户）
| 方法 | 路径 | 说明 |
| --- | --- | --- |
| POST | /api/v1/subscriptions/:id/change/preview | 升级报价（服务端 proration；纯读） |
| POST | /api/v1/subscriptions/:id/upgrade | 创建升级订单（金额只来自冻结报价；Idempotency-Key） |
| POST | /api/v1/subscriptions/:id/schedule-downgrade | 计划降级（term 末生效；替换语义） |
| DELETE | /api/v1/subscriptions/:id/schedule-downgrade | 取消计划降级 |
| GET | /api/v1/subscriptions/:id/changes | Plan Change 审计历史 |

### MUCODE（API Key scope）
| 方法 | 路径 | 说明 |
| --- | --- | --- |
| GET | /v1/usage | wallet + reset_cards + subscription_status（新合同）+ legacy 字段（兼容期） |

## 4. Migration Map

| # | 内容 |
| --- | --- |
| 239 | subscription_v1_fallback_and_resets：user_subscriptions.auto_payg_fallback、groups.concurrency_override、subscription_reset_events/applications/cards |
| 240 | reward_grants（Reward Track） |
| 241 | Phase 10 Plan Change：subscription_plans.tier_rank、user_subscriptions.plan_id/next_plan_id、subscription_terms（已付 term 快照）、subscription_plan_changes（报价冻结+审计）、payment_orders.plan_change_id |

## 5. Failure / Retry Semantics

- 结算幂等：usage_billing_dedup(request_id, api_key_id) + sha256 指纹；usage_logs ON CONFLICT
- Reset Core：CAS；输=幂等 no-op；事件驱动路径有 applications UNIQUE 兜底
- 卡消费：拒绝路径整体回滚（卡不白烧）；同 Key 重放返回已持久化结果
- Direct Reset worker：认领独立提交，执行事务显式 COMMIT；崩溃行停在 applying 由 retry 收尾；expired→skipped；stale→skipped（卡赢）
- Fallback：准入时定案（单模式），结算层无第二套事务

## 6. Known Technical Debt

- integration 冷启动偶发 FAIL：colima 资源压力下 PG 测试容器自身进入 recovery（`pq: database system is in recovery mode`）——基础设施非代码；缓解=控制并行/清理残留容器（本轮已清 134 个），长期=独立 PG 或 readiness 重试
- B1 限额边界语义（auth `<=` vs 预检 `>=`）；B4 usage_billing_dedup 清理任务
- legacy 兼容面（/v1/usage legacy、/subscriptions/active 等）待客户端覆盖率后独立删除
