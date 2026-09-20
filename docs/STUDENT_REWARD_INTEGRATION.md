# Student Verification Reward — 集成契约

> 给 Student Verification 模块开发者的接入说明。
> Reward Track 已提供安全、幂等、可审计的奖励发放能力；认证模块**只负责告诉 Reward Service "这个用户通过了学生认证"**。

## 一句话接入

在学生认证 pending → approved 的处理中调用：

```go
result, err := rewardService.GrantStudentVerificationReward(ctx, userID, verificationID, reviewerID)
//                                          事务ctx  用户ID  认证记录ID(>0)  审核管理员ID(系统自动传 nil)
```

返回值 `*service.RewardGrantResult`，用 `Status` 区分：

| Status | 含义 | 调用方该做什么 |
| -- | -- | -- |
| `granted` | 本次真正发了钱 | 无需处理（可记业务日志） |
| `already_granted` | 该业务动作之前已发过（重试/重复审批） | 视为幂等成功，无需处理 |
| `disabled` | 奖励开关关闭（settings） | 正常继续认证流程，不发放 |
| `invalid_config` | 开关已开但金额/活动未配置 | 认证流程照常继续；记录告警日志，通知管理员修配置 |

辅助方法：`result.Granted() bool` —— 本次或之前任何一次发放过即为 true（`granted` ∪ `already_granted`）。

`err != nil` 仅代表基础设施故障（数据库/事务失败）。此时按所选集成模式处理（见下文）。

## 不要做的事

```text
不要 自己 AdjustBalance / AddBalance / UpdateBalance
不要 自己读取奖励金额（student_verification_reward_amount）
不要 自己判断 campaign
不要 自己查"是否已发过"再决定发不发
不要 把金额作为参数传进奖励接口
```

以上全部由 Reward Service 封装。金额、活动、幂等、事务、缓存失效都不属于认证模块的职责。

## 幂等保证（你不需要写任何防重代码）

`reward_grants` 表以 **`idempotency_key` 唯一约束**为幂等核心。学生认证奖励的幂等键由 Reward Service 确定性派生（可用 `service.StudentVerificationRewardIdempotencyKey(userID, campaign)` 预告）：

```text
student_verification:{userID}:{campaign}
```

同一用户 + 同一活动（campaign）**无论调用多少次、多少个并发请求、服务是否重启，最多只发放一次、余额只加一次**。重复调用返回 `already_granted`，属于业务成功。

发放记录 + 余额增加在**同一个数据库事务**内：不会出现"有记录没加钱"或"加了钱没记录"。

## 集成方式（按优先级）

### 方式一（推荐）：认证状态变更与奖励发放同一事务

如果 Student Verification 与 Reward Service 共享同一个 PostgreSQL 事务（认证模块使用 ent 事务即可），把事务 ctx 直接传入：

```text
BEGIN                                          （认证模块自己的 ent 事务）
  CAS: UPDATE student_verifications
       SET status='approved', reviewed_by=$, reviewed_at=NOW()
       WHERE id=$1 AND status='pending'        （affected=0 → 已审过，走提交/返回）
  rewardService.GrantStudentVerificationReward(txCtx, userID, verificationID, reviewerID)
  COMMIT
  rewardService.InvalidateCaches(ctx, userID)  ← COMMIT 成功之后必须调用
```

```go
tx, _ := authEntClient.Tx(ctx)               // 认证模块自己的事务
defer tx.Rollback()
txCtx := dbent.NewTxContext(ctx, tx)
// ... CAS 审核状态变更 ...
result, err := rewardService.GrantStudentVerificationReward(txCtx, userID, verificationID, reviewerID)
// 仅 infraerrors 故障才需要回滚；disabled / invalid_config 不是错误，照常提交
if err != nil { return err }
if err := tx.Commit(); err != nil { return err }
rewardService.InvalidateCaches(ctx, userID)   // 先 Commit，后失效缓存
```

效果：`verification approved + reward_grant + balance increase` 三者原子——要么一起生效，要么一起回滚，不存在"认证通过但发奖失败"的中间态。Reward Service 检测到 ctx 内已有事务时会内联执行（不自开事务、不自己失效缓存），这就是为什么 COMMIT 后必须由调用方调用 `InvalidateCaches`。

注意：方式一下，`invalid_config`（配置缺失）也会导致整个审核事务回滚。若希望"配置不完整时审核照常通过、只是不发钱"，请在调用前先检查 `IsStudentVerificationRewardEnabled` + 金额/活动是否配置，或者改用方式二。

### 方式二（仅当无法共享事务时）：先提交认证，再调用奖励

如果模块边界/基础设施暂时无法共享事务：

```text
COMMIT verification approved          （认证状态先行生效）
↓
rewardService.GrantStudentVerificationReward(ctx, userID, verificationID, reviewerID)
```

必须遵守：

1. **Reward Service 本身是数据库级幂等的**——任何重试都不会重复发钱；
2. **调用方必须具有可靠 retry**——调用失败（网络/数据库故障）后应重试（重试队列、下次登录补偿、管理员手动重放均可），因为唯一约束保证重试安全；
3. **不能因为一次调用失败就永久放弃奖励**——`err != nil` 时奖励尚未发放，用户应最终拿到钱；只有返回 `disabled` / `invalid_config`（明确不发放的状态）才允许放弃。

此方式下 Reward Service 自开事务并在提交后自动失效缓存，调用方无需调用 `InvalidateCaches`。

> V1 不要求实现 Outbox；按上述契约正确集成即可。

## 配置（管理员在 后台设置 → 用户默认设置 中修改）

| settings key | 默认 | 说明 |
| -- | -- | -- |
| `student_verification_reward_enabled` | `false` | 认证通过后是否自动发奖励（与认证功能开关解耦） |
| `student_verification_reward_amount` | `0` | 奖励金额，**单位 USD（系统内部余额记账单位，与充值/余额一致）**；后台输入框已标注单位并按展示汇率给 ≈¥ 折算提示；不计入累计充值 |
| `student_verification_reward_campaign` | `""` | 活动标识（幂等键派生维度，如 `2026_fall`）；更换即开启新一轮活动 |

`enabled=true` 但金额 ≤ 0 或 campaign 为空时：每次调用返回 `invalid_config`，**不会**发放异常奖励。

## 奖励记录与对账

- 事实源：`reward_grants` 表（`user_id / idempotency_key / source_type='student_verification' / source_id=你的认证记录ID / campaign / amount / granted_by / metadata / created_at`）。
- `source_id` 存你传入的 verification ID（逻辑引用，无外键）。
- 管理员在该用户的「余额历史」中能看到类型为 `余额（系统奖励）` 的条目（金额 + 来源 + campaign + 时间），redeem_codes 表不会产生任何伪行。
- 回查接口：`rewardService.GetRewardBySource(ctx, "student_verification", verificationID)`、`ListRewardsByUser(ctx, userID, limit)`。

## 撤销（V1 未实现）

已发放的奖励不会因修改/删除 reward_grants 行而自动扣回。未来如需 Reward Reversal，将新增独立的负向 reversal 事件（带自己的幂等键），不改历史行。
