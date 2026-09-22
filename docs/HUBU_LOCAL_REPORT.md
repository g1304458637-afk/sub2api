# HUBU LOCAL REPORT — 湖北大学 AI Harness（本地开发版）

> 交付日期：2026-09-19 · 分支：sub2api `hubu-local` / opencode-muc `hubu`
> 范围：**仅本机**。全程未 SSH、未触碰 admin.wuxuexi.top / 112.125.88.123 / 生产 DB·Redis·Docker·Nginx·GitHub secrets、未部署。
> 计划书：`docs/HUBU_LOCAL_IMPLEMENTATION_PLAN.md`

---

## 1. Architecture

```
                       ┌────────────────────────────── 单 Core 共用 ──────────────────────────────┐
 HUBU AI Desktop ──────▶ sub2api(hubu-local 分支构建, :8081)                                      │
 (dist/HUBU AI.app)    │  ├─ 前端 BRAND=hubu 构建（绿金调色板 + /hubu 页）                          │
  hubu://connect?code  │  ├─ /api/v1/{muc|hubu}/connect-code → 60s TTL · SHA-256 · GETDEL 单次使用 │
  │ safeStorage 凭据   │  ├─ /api/v1/{muc|hubu}/exchange → per-device API Key（"HUBU <device>"）    │
  ▼ HUBU_API_KEY(env)  │  └─ GET /v1/models → 按 Key/分组动态返回（无任何写死模型）                 │
 opencode core ◀───────┘            │                                                            │
  dynamicModels:true                ▼                                                            │
  gateway-only 过滤          mock 上游(127.0.0.1:9999)：/v1/models + /v1/chat/completions          │
                                                                                                  └────────────────────────────┘
 品牌数据唯一事实源：
  - opencode-muc：packages/brand（@opencode-ai/brand，brands/{muc,hubu}/brand.json 镜像 + 漂移测试）
  - sub2api：backend/internal/pkg/campus（Go Brand 注册表）+ frontend/src/brand（VITE_BRAND 构建期注入）
```

- 切换机制：**BRAND=hubu**（或 OPENCODE_CHANNEL=hubu）。桌面主进程把 BRAND 传给内嵌 opencode sidecar。
- 未来第三所学校 = 一个 brand 目录 + 图片 + 配置，不 fork 任何代码。

## 2. Brand System

| 项 | MUC（保持不变） | HUBU（新增） |
|---|---|---|
| 品牌档案 | `packages/brand/brands/muc/brand.json` | `packages/brand/brands/hubu/brand.json` |
| 中文名/英文名 | 中央民族大学 / Minzu University of China | 湖北大学 / Hubei University |
| 产品名 | MUC AI Harness（mucode） | **HUBU AI** |
| 校训 | 美美与共，知行合一 | **日思日睿，笃志笃行** |
| 建校 | 1941 | **1931** |
| 深链 | muc:// | **hubu://** |
| 主色 | 民大红 #AC0E0F | **深湖大绿 #135440** |
| 辅色 | 金 #D9A94E | **青铜金 #BC9D53**（楚文化饰线） |
| 深色底 | #0d0a0b | **墨绿黑 #0B1F17** |
| TUI 主题 | minzu | **hubu**（theme/assets/hubu.json） |
| 网关默认 | http://admin.wuxuexi.top | **http://localhost:8081**（HUBU_GATEWAY_URL 可覆盖） |
| Key 环境变量 | MUC_API_KEY | HUBU_API_KEY |
| App ID | cn.edu.muc.harness | cn.edu.hubu.harness |
| 安装包名 | mucode-mac-arm64.dmg 等 | hubu-ai-mac-arm64.dmg 等 |

**颜色出处声明**：HUBU 色值采样自用户提供的湖北大学主视觉图 `assets/brands/hubu/hubu-hero.png`（横幅深绿 #17503A/#104B37、校徽绿 #094E37、饰带金 #BC9D53），**非湖北大学官方 VI 标准色**。

主视觉 `hubu-hero.png` 已入仓（sub2api `assets/brands/hubu/` 与 `frontend/src/assets/hubu/`，opencode-muc desktop/app assets），所有使用处 `object-cover` + 深色渐变遮罩，不拉伸。

## 3. Changed Files（要点，完整见两仓库提交记录）

**sub2api（分支 hubu-local，5 commits）**
- 新增：`backend/internal/pkg/campus/brand.go`、`backend/internal/server/routes/{hubu,campus}.go`、`frontend/src/brand/index.ts`、`frontend/src/api/campus.ts`、`frontend/src/components/campus/CampusConnectView.vue`、`frontend/src/views/user/HubuView.vue`、`frontend/src/assets/hubu/hubu-hero.png`、`assets/brands/hubu/hubu-hero.png`、`scripts/dev-hubu.sh`、`docs/HUBU_LOCAL_{IMPLEMENTATION_PLAN,REPORT}.md`
- 修改：`muc_connect_handler.go`（参数化 CampusConnectHandler）、`routes/muc.go`（路径逐字节不变）、`router.go`（hubu 路由 + DOWNLOADS_DIR）、`handler.go`/`wire.go`/`wire_gen.go`、`tailwind.config.js`（BRAND 调色板 + bronze 辅色）、`vite.config.ts`（VITE_BRAND define）、`AuthLayout`/`LoginView`/`AppHeader`/`AppSidebar`（品牌驱动）、`api/muc.ts`（修复既有双前缀隐患）

**opencode-muc（分支 hubu，4 commits）**
- 新增：`packages/brand/**`（注册表+镜像+测试）、`packages/tui/src/theme/assets/hubu.json`、`packages/desktop/{icons,resources}/hubu/*`、`src/renderer/assets/hubu-hero.png`、`packages/app/src/assets/hubu/`
- 修改：desktop `constants.ts`、`main/index.ts`（APP_NAMES/IDS、BRAND 传递、协议注册）、`migrate.ts`、`muc/{deep-link,gateway,connect,controller,secret-store,ipc}.ts`、`muc-gate.tsx`（品牌门）、`electron-builder.config.ts`（hubu case）、`electron.vite.config.ts` + `app/vite.js`（channel 判定）、`prebuild/copy-icons/utils`；opencode `provider/muc.ts`（品牌网关/Key env）；tui `logo.ts`、`component/logo.tsx`、`util/presentation.ts`（去重）、`theme/index.ts`、`context/theme.tsx`；ui `logo.tsx`（民/湖 Mark）、`wordmark-v2.tsx`（HUBU AI 字标）、`v2/styles/theme.css`（data-brand 覆盖块）；app `new-session-view.tsx`、`entry.tsx`、`env.d.ts`、`titlebar.tsx`、`workspace-controller.ts`

## 4. MUC/HUBU Shared Core（共用清单，均未复制）

| Core | 位置 | 复用方式 |
|---|---|---|
| 动态模型发现 | sub2api `gateway_handler.go Models` | 原样共用 |
| connect-code/exchange 机制 | `muc_connect_handler.go` | CampusConnectHandler 参数化（60s/SHA-256/GETDEL 不变） |
| 一键连接桌面流程 | desktop `main/muc/*` | 品牌参数（scheme/exchangePath/keyEnv/gateway） |
| provider 注入 | opencode `provider/muc.ts` + `provider.ts` | resolveBrand() 切换，注入 id 仍为 sub2api |
| safeStorage 凭据仓 | desktop `secret-store.ts` | 文件名按品牌（muc 保持历史文件名） |
| TUI 框架/主题系统 | tui theme/index | hubu 主题注册 + 默认主题按品牌 |
| 构建/打包 | electron-builder + electron-vite | hubu case 与 muc case 并列，字段取 brand.json |

**MUC 零回归证据**：后端 `go test ./internal/handler/ -run "TestMuc|TestHubu"` 全绿（MUC 原用例未改语义）；desktop `bun test` 78/78 通过；默认（无 BRAND）前端构建 CSS 仅含 #ac0e0f、无 #135440；8080 预构建实例全程健康（截图 muc-dashboard-regression.png）。

## 5. Local Ports

| 服务 | 端口 | 说明 |
|---|---|---|
| HUBU 网站/网关 | **http://localhost:8081** | hubu-server（embed 前端） |
| HUBU Postgres | 127.0.0.1:5433 | 容器 sub2api-hubu-postgres |
| HUBU Redis | 127.0.0.1:6380 | 容器 sub2api-hubu-redis |
| mock 上游 | 127.0.0.1:9999 | bun mock-upstream.mjs（hubu-mock-lite/pro） |
| MUC（未动） | 8080 | 预构建镜像 sub2api-muc:latest |

若 8081 被占用：`HUBU_PORT=xxxx scripts/dev-hubu.sh` 即可换端口启动（脚本记录于输出横幅）。

## 6. Docker Topology

```
colima (docker)
├─ MUC 栈（未动）：sub2api / sub2api-postgres / sub2api-redis @ sub2api-network
└─ HUBU 栈（新增）：name: sub2api-hubu（共用/sub2api-deploy-hubu/docker-compose.yml）
   ├─ sub2api-hubu-postgres   (postgres:18-alpine, 127.0.0.1:5433)
   ├─ sub2api-hubu-redis      (redis:8-alpine,    127.0.0.1:6380)
   ├─ network: sub2api-hubu-network
   └─ volumes: sub2api-hubu-postgres-data / sub2api-hubu-redis-data
```

注意：postgres:18+ 镜像挂载点为 `/var/lib/postgresql`（非旧版 `/data` 子目录）。HUBU 网关本体是本机 go 二进制（hubu-server），不在容器内，便于快速迭代。

## 7. Data Isolation

- HUBU 数据目录：`共用/sub2api-deploy-hubu/`（`data/`、compose、env、seed；目录不在任何 git 仓库内）
- 三层隔离：**MUC 本地**（sub2api-deploy/ + 旧容器/卷）｜**HUBU 本地**（sub2api-deploy-hubu/ + 新容器/具名卷/网络）｜**生产**（112.125.88.123，全程未触）
- HUBU Redis 前缀 `hubu:code:` 与 MUC `muc:code:` 互不读取（有单测保障：TestHubuCodeIsolatedFromMucPrefix）
- 本地测试账户：`admin@hubu.local` / `demo@hubu.local`（demo_user + $100 本地余额 + "HUBU Demo Key"）。口令仅在 `sub2api-deploy-hubu/.env`（chmod 600，不入 git）；Demo Key 明文仅 `data/demo_api_key.txt`（0600）
- 真实上游 Key 预留 `sub2api-deploy-hubu/.env.local`（当前未用——上游为本地 mock）

## 8. Deep Link

`hubu://connect?code=XXXX`（code 32 字节随机 base64url，60s TTL，GETDEL 单次使用，URL 不含 API Key）：

1. 登录 http://localhost:8081 → /hubu → 「一键连接 HUBU」→ `POST /api/v1/hubu/connect-code`（200）
2. `hubu://connect?code=…` → HUBU AI.app（Info.plist CFBundleURLSchemes=[hubu, opencode]）
3. `POST /api/v1/hubu/exchange` → `{gateway, api_key, key_name:"HUBU <device>", user}`（信封兼容 {code,data}）
4. 凭据 safeStorage 加密落 `~/Library/Application Support/cn.edu.hubu.harness/hubu-credential.bin`（0600）
5. 重放同 code → 404 code_not_found（已实测）

## 9. Dynamic Models

`GET /v1/models`（Bearer demo key）实测返回 **`["hubu-mock-lite","hubu-mock-pro"]`** —— 来自 mock 上游账号的 model_mapping 白名单（分组 platform=openai）。客户端零写死：HUBU 客户端代码中无 GLM/Claude/GPT/Qwen/Kimi 任何模型名；模型元数据由 `mucDynamicModel` 按需合成（claude* → 200k，其余 128k，与 MUC 同策略）。完整请求链路实测：demo key → 网关 8081 → mock 上游 9999 → 对话回包（usage 正常计费扣减）。

## 10. Credential Security

- code：Redis 只存 SHA-256（60s TTL），GETDEL 原子单次；日志无 code 明文
- API Key：响应仅出现一次；安全存储为 Electron safeStorage（macOS Keychain 背书）密文；core 仅经进程内存 env `HUBU_API_KEY`
- 扫描结果：两仓库 git 追踪文件无真实密钥；`hubu-server` 内嵌前端无密钥；HUBU AI.app asar 无密钥（sk-prompt-* 为 feature flag 名非密钥）；server.log 无明文 Key；.env / demo_api_key.txt 均 0600 且不入 git

## 11. Screenshots

存于 `共用/sub2api-deploy-hubu/data/screenshots/`：
- `hubu-login.png` 登录页（hero 主视觉 + 校徽金环 Logo + 绿色 Sign In + 校训标语）
- `hubu-page.png` /hubu 页（Hero「HUBU AI / 湖北大学 HUBEI UNIVERSITY · 日思日睿 笃志笃行 · 1931」+ 一键连接 + 三平台下载卡 + 侧栏「下载 HUBU」）
- `hubu-desktop-gate.png` HUBU AI.app 启动门（主视觉 cover + 湖北大学 / HUBEI UNIVERSITY / 1931 / 校训 / 深绿按钮）
- `hubu-desktop-connect-success.png` 连接成功态（✓ 账户连接成功 / ✓ 凭据已安全保存 / ✓ 已同步模型）
- `hubu-icon.png` 应用图标（校徽 + 青铜金环 + 深绿圆角）
- `muc-dashboard-regression.png` MUC 8080 回归（民大红 mucode 原貌）

## 12. Test Results

| # | 项 | 结果 |
|---|---|---|
| A | MUC 本地原环境 | ✅ 8080 healthz 200，控制台视觉原貌 |
| B | HUBU 独立启动 | ✅ dev-hubu.sh / 手动均可，8081 ready |
| C | 8080/8081 共存 | ✅ 同机并行，容器/网络/卷互不相交 |
| D | HUBU 页面品牌 | ✅ 登录页 + /hubu 全绿金品牌（截图） |
| E | MUC 页面未改坏 | ✅ 预构建实例原样 + 默认源码构建无 HUBU 色 + 测试全绿 |
| F | Desktop 文案 | ✅ 湖北大学 / HUBEI UNIVERSITY / 日思日睿，笃志笃行 / 1931 / Powered by HUBU AI Harness |
| G | TUI 独立主题 | ✅ hubu 主题注册；epilogue 绿(46;144;112)+青铜金(188;157;83)，无民大红；BRAND=muc 回归民大红金原样 |
| H | hubu:// 协议 | ✅ Info.plist 注册 + `open "hubu://connect?code=…"` 实测唤起 |
| I | connect-code/exchange | ✅ 200 签发 → 200 交换（key "HUBU HUBU-d219f57a"）→ 重放 404 |
| J | 模型来自 /v1/models | ✅ 精确返回 mock 上游两模型 |
| K | 密钥不泄露 | ✅ 源码/git/URL/日志/asar 五处扫描通过 |
| L | 数据隔离 | ✅ 容器/网络/卷/端口/Redis 前缀/凭据文件全独立（含隔离单测） |
| M | 登录→建 Key→一键连接→发现模型→测试请求 | ✅（上游为本地 mock，见已知限制 #4） |

## 13. How To Start

```bash
cd /Users/cccc/Desktop/共用/sub2api
./scripts/dev-hubu.sh            # 数据层容器 → 二进制(缺失才重编) → 网关+mock → seed → 打印 ready
./scripts/dev-hubu.sh --rebuild  # 强制重编前端(BRAND=hubu)+后端二进制
open "/Users/cccc/Desktop/共用/dist/HUBU AI.app"   # 桌面端（hubu:// 已注册）
```
输出就绪横幅：Website http://localhost:8081 ｜ Admin /admin ｜ HUBU /hubu。

## 14. How To Stop

```bash
./scripts/dev-hubu.sh --stop   # 停网关+mock（容器保留，数据不丢）
./scripts/dev-hubu.sh --down   # 再停数据容器（具名卷保留）
```

## 15. Known Limitations

1. **exchange 直建 Key 未绑分组**：桌面端 exchange 创建的 per-device Key 无 group_id，本地 ingress 以 `group_unassigned` 拒绝其 `/v1/models`（成功页模型计数因此可能缺失）。网站 UI/seed 建的 Key 不受影响。修复需改共用 handler（绑定默认分组）→ 影响线上 MUC，留待与 MUC 一起决策。
2. **生产 connect 前缀重复**：与 MUC 同模式，Key 名形如 "HUBU HUBU-xxxx"（客户端 `HUBU-<deviceId>` + 服务端 `"HUBU "` 前缀），行为与 MUC 逐字一致，未单独"修复"。
3. TUI 交互式启动依赖终端应答（opentui 设备查询），headless PTY 下无法截全屏；已用纯函数级断言验证品牌画/颜色/主题注册。
4. 端到端对话停在 **mock provider**（本地 bun 服务），未用任何真实第三方 Key（符合验收豁免条款）。
5. HUBU 色值为采样值非官方 VI；两品牌页路由共存导致 MUC 默认构建也内嵌 hubu-hero.png（+2.6MB 安装体积），如需精简可改为构建期裁剪。
6. Windows NSIS/DMG 正式签名未做（与 MUC 同为 ad-hoc；xattr 提示见品牌页说明）。

## 16. Future Production Deployment

1. 服务端镜像：复用 CI/CD 管线新增 `BRAND=hubu` 构建档（或同镜像 + `BRAND` env 运行时路由），先跑 `production-audit.sh` 评估（服务器当前冻结中）。
2. 域名/协议：为 hubu:// 提交 macOS LaunchServices 类型声明（打包件已含）；HUBU_GATEWAY_URL 指向正式域名并启用 HTTPS。
3. 数据：独立生产 DB/Redis（命名 hubu_ 前缀），AUTO_SETUP 后先改默认口令 + 合规确认。
4. exchange Key 分组策略（见 #15.1）需先定夺。
5. 分发：安装包（hubu-ai-mac-arm64.dmg 等）放入生产 data/downloads 即经 /downloads 生效，无需重建镜像。
