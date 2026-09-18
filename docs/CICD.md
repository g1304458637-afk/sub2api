# CI/CD — Sub2API 生产部署（admin.wuxuexi.top）

> 建立于 2026-09-18。本文档描述 Sub2API/MUC 项目的持续集成与生产部署体系。

## 1. 架构总览

```
git push main (或手动 workflow_dispatch)
   │
   ├─ ci.yml（push/PR 触发）
   │    frontend: lint → i18n → typecheck → vitest
   │    backend:  unit → integration
   │    build-test: docker build（不推送）
   │
   └─ deploy-production.yml（workflow_dispatch 手动触发）
        test 门禁 → migration 审计 → 构建 linux/amd64 镜像
        → 推送 ghcr.io/g1304458637-afk/sub2api/sub2api:sha-<commit>
        → SSH 到生产 → 记录 PREVIOUS_IMAGE → pull → up -d --no-deps sub2api
        → 健康检查（/healthz JSON + MUC 路由 + /v1/models）
        → 失败自动回滚上一镜像并复检
        → 独立复核 → 外网探测 → GHCR 旧版本清理（保留最近 3 个）
```

## 2. 生产拓扑

| 项 | 值 |
|---|---|
| 服务器 | `112.125.88.123`（Alibaba Cloud Linux 3，**x86_64 → linux/amd64**） |
| SSH | 用户 `admin`（免密 sudo），端口 22 |
| compose 目录 | `/srv/sub2api`（`docker-compose.yml` + `.env` + `.env.deploy`） |
| 服务 | `sub2api`（应用）/ `sub2api-postgres`（postgres:18-alpine）/ `sub2api-redis`（redis:8-alpine） |
| 数据 | 本地目录挂载 `./data`、`./postgres_data`、`./redis_data`（部署不触碰） |
| nginx | `admin.wuxuexi.top` → `127.0.0.1:8080`（HTTP） |
| 镜像 | `ghcr.io/g1304458637-afk/sub2api/sub2api`，tag：`sha-<完整commit>`（不可变）+ `production`（移动指针） |

## 3. 镜像与版本策略

- 每次部署使用不可变 tag `sha-<完整 git sha>`；`production` 仅作为"当前生产"的移动指针。
- **禁止用 `latest` 部署**。生产 compose 已改为：

  ```yaml
  image: ${SUB2API_IMAGE:?SUB2API_IMAGE is required - see .env.deploy}
  ```

  缺失 `.env.deploy` 时 compose 直接报错，杜绝误跑 `latest`。

## 4. 部署状态文件 `/srv/sub2api/.env.deploy`

由 `scripts/production-deploy.sh` 自动维护，**不含任何 secret**：

```
SUB2API_IMAGE=<当前运行镜像>        # 部署目标 / 回滚后的当前值
PREVIOUS_IMAGE=<回滚目标>           # 每次部署前自动记录
DEPLOY_COMMIT=<当前镜像对应 commit>  # migration 审计基线 + healthz 比对
PREVIOUS_COMMIT=<回滚目标对应 commit>
DEPLOYED_AT=<UTC 时间>
ROLLBACK=<false|true>
```

## 5. GitHub Environment 与 Secrets

Environment：`production`。

| Secret | 值 | 说明 |
|---|---|---|
| `PROD_HOST` | `112.125.88.123` | 生产服务器 |
| `PROD_PORT` | `22` | SSH 端口 |
| `PROD_USER` | `admin` | SSH 用户 |
| `PROD_SSH_KEY` | 专用部署私钥 | `~/.ssh/sub2api-deploy/id_ed25519`（ed25519，公钥已装服务器 `authorized_keys`；与个人 key 相互独立，可单独吊销） |
| `GHCR_PULL_USER` / `GHCR_PULL_TOKEN` | （可选，暂未配置） | 若提供最小权限 `read:packages` PAT，服务器拉取将优先使用它；未配置时回退为**本次运行临时的 `GITHUB_TOKEN`**（job 结束自动失效，只进 600 临时文件、用后即焚，不落盘） |

Actions 推送 GHCR 使用工作流内置 `GITHUB_TOKEN`，权限最小化：`contents: read` + `packages: write`。
任何 secret 不会被 echo；GitHub 日志对 secret 值自动打码。

## 6. 服务器端脚本（`scripts/`，随工作流上传 `/tmp/sub2api-deploy/` 执行）

| 脚本 | 用途 | 退出码 |
|---|---|---|
| `production-deploy.sh` | 预检（磁盘≥2GB）→ 记录回滚点 → pull → `up -d --no-deps sub2api` → 严格健康检查；失败自动回滚 | 0 成功；1 部署失败但回滚成功；2 **CRITICAL 回滚也失败**；3 预检失败（未变更） |
| `production-healthcheck.sh` | 容器 healthy + `/healthz` 200 且 JSON `status=ok`（`--expect-commit` 校验 commit）+ MUC 路由 401/400 + `/v1/models` 401。`--lenient` 宽松模式用于回滚旧版镜像 | 0/1 |
| `production-rollback.sh` | 手动回滚：`sudo bash production-rollback.sh [镜像ref]`（缺省回 `.env.deploy` 的 PREVIOUS_IMAGE） | 0/2/3 |
| `audit-migrations.sh` | migration 安全审计（BASE..HEAD 新增 SQL 含 DROP/RENAME/改类型/TRUNCATE/DELETE 即拦截） | 0/1 |

所有脚本 `set -euo pipefail`；禁止 `compose down`、`volume rm`、`system prune -a`。

## 7. 健康检查端点

| 端点 | 语义 | 通过条件 |
|---|---|---|
| `GET /healthz` | 极简存活探针（2026-09-18 新增，不查 DB/上游） | 200 + `{"status":"ok","version":...,"commit":<sha>}` |
| `GET /health` | 上游自带探针 | 200 |
| `POST /api/v1/muc/connect-code`（无登录态） | MUC 路由存在性 | 401 |
| `POST /api/v1/muc/exchange`（空参） | MUC 路由存在性 | 400 |
| `GET /v1/models`（无 key） | 网关路由存在性 | 401 |

> 注意：SPA 兜底会把未知路径渲染成 index.html（HTTP 200）。`/healthz` 已加入 `shouldBypassEmbeddedFrontend` 白名单——若未来镜像缺失该路由将返回 404（正确的失败信号），不再出现 200 HTML 假阳性。

## 8. 数据库 migration

- 应用启动时自动执行嵌入式 SQL migration（`backend/internal/repository/ent.go`，失败进程退出 → 容器 unhealthy）。
- 部署前 `preflight` job 自动对比**上次生产部署 commit → 本次 commit** 的新增 migration，发现不可逆语句（DROP/RENAME/类型变更/TRUNCATE/DELETE）即终止部署。
- 数据库永远不会因代码部署被自动删除或覆盖。

## 9. 正常部署（自动）

```bash
git push origin main   # push main 自动部署尚未开启（见 §12 rollout 计划）
```

当前（第一阶段）通过手动触发：GitHub → Actions → Deploy Production → Run workflow。

## 10. 手动部署 / 手动回滚（服务器上）

```bash
ssh admin@112.125.88.123
sudo bash /tmp/sub2api-deploy/production-rollback.sh            # 回滚到上一镜像
sudo bash /tmp/sub2api-deploy/production-rollback.sh ghcr.io/g1304458637-afk/sub2api/sub2api:sha-<旧commit>  # 显式版本
sudo docker compose -f /srv/sub2api/docker-compose.yml --env-file /srv/sub2api/.env --env-file /srv/sub2api/.env.deploy ps
sudo cat /srv/sub2api/.env.deploy                                # 查看当前部署状态
```

## 11. 故障处理

| 现象 | 含义 | 动作 |
|---|---|---|
| Action 显示 `DEPLOYMENT FAILED / ROLLBACK SUCCESSFUL` | 新镜像不健康，已自动恢复 | 查日志定位原因，修复后重新部署 |
| Action 显示 `CRITICAL: ROLLBACK FAILED` | 回滚也失败 | **立即人工 SSH 介入**；禁止盲目删除性操作 |
| `PREFLIGHT FAILED` | 磁盘不足/compose 异常 | 服务器未做任何变更，处理后重试 |
| migration 审计拦截 | 新增不可逆 migration | 人工评审，走显式豁免，不得自动放行 |
| GHCR 拉取 403 | 包未关联/凭据问题 | 检查包的 repo 关联；或配置 read-only PAT |

## 12. Rollout 计划（第十六节）

第一阶段（当前）：仅 `workflow_dispatch` 手动部署。要求连续稳定验证 2~3 次（含一次受控回滚演练）后，再在 `deploy-production.yml` 追加：

```yaml
on:
  push:
    branches: [main]
```

### 12.1 首次 rollout 前的强制重审计

> 2026-09-18 起生产状态可能被并行的手工部署/故障恢复任务改变。**任何 rollout 开始前必须重新执行只读审计**，不得假设服务器仍是本文档记录的状态：

```bash
bash scripts/production-audit.sh admin@112.125.88.123 \
  -i ~/.ssh/sub2api-deploy/id_ed25519   # 只读，不改任何东西
```

核对项（任何一项不符，先修正再 rollout）：

1. 架构仍为 `x86_64`（workflow 固定构建 linux/amd64）
2. `/srv/sub2api/docker-compose.yml` 的 sub2api 服务 image 行仍为 `${SUB2API_IMAGE:?...}`
3. `.env.deploy` 存在且 `SUB2API_IMAGE` 等于**当前实际运行镜像**（若另一任务手工切换过镜像，需先同步该文件）
4. `.env` 键名与 `.env.original` 无意外增删（防配置被覆盖）
5. 部署公钥仍在 `authorized_keys`（指纹核对）
6. 磁盘可用 ≥ 2GB
7. DB/Redis 容器健康、数据目录未变

### 12.2 首次 rollout 验收清单

CI 绿、镜像推送成功、生产拉取成功、容器更新、DB/Redis/volumes 未动、站点可用、API key 可用、余额未变、MUC 端点正常、真实推理成功、**回滚演练成功（rollback 后须恢复到 sha- 镜像而非旧版 latest，演练完再部署回最新）**。

## 13. 边界约定

- **MUC 安装包**（`MUC-mac-arm64.dmg` 等）继续放服务器持久化目录，经 `/downloads/*` 静态读取（`MUC_DOWNLOADS_DIR`）。更新安装包只需上传文件，**不要**为此重新部署 Sub2API；后续可加独立 `release-muc.yml`。
- **业务内容**（公告/价格/模型开关/版本提示/余额）存数据库与后台设置，后台保存即时生效；只有代码结构/UI/API 变化才走 CI/CD。
- GHCR 私有包配额有限（免费档 500MB 存储/月），工作流自动清理旧版本仅保留最近 3 个；若配额不足再考虑付费或调整保留数。
