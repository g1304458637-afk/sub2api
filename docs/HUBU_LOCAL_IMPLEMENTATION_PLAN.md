# HUBU LOCAL IMPLEMENTATION PLAN

> 湖北大学 HUBU AI — 本地开发版实施计划（2026-09-19）
> 约束：仅本机。不 SSH 生产、不动 admin.wuxuexi.top / 112.125.88.123 / 生产 DB·Redis·Docker·Nginx·GitHub secrets、不部署。
> 现有 MUC 本地实例（localhost:8080，`sub2api-muc:latest` 预构建镜像）运行中，全程不可破坏。

## 0. Phase 0 审计结论（核心 vs 品牌硬编码）

### sub2api（Go + Vue，main 分支）
- **Core（共用，不动逻辑）**：网关与 `GET /v1/models` 动态发现（`gateway_handler.go Models`）、API Key 体系、用户/分组、connect-code 机制本体（`muc_connect_handler.go`：60s TTL、SHA-256 入 Redis、GETDEL 单次使用）、AUTO_SETUP 引导（`setup/setup.go`）、前端嵌入（`-tags embed`）。
- **MUC 品牌硬编码（需参数化）**：
  - `muc_connect_handler.go`：Redis 前缀 `muc:code:`、Key 名前缀 `"MUC "`、设备提示 `MUC Desktop`
  - `routes/muc.go`：`/api/v1/muc/*` 路由路径
  - `router.go`：下载目录 env `MUC_DOWNLOADS_DIR`
  - 前端：`tailwind.config.js` 全站民大红 primary；`MucView.vue`（/muc 页）；`AuthLayout.vue` 登录背景/校训；`LoginView.vue` 登录后跳 `/muc`；`AppHeader/AppSidebar`「下载 MUC」；`assets/muc/campus.png`
- **品牌抽象：不存在。** 仅有动态 `site_name`/`site_logo` 站点设置。

### opencode-muc（TS monorepo，muc-harness 分支）
- **Core（共用）**：opencode 内核、TUI 框架、桌面壳（Electron 42 + electron-builder）、`safeStorage` 凭据仓、动态模型注入（`provider/muc.ts` + `provider.ts` dynamicModels 展开与 gateway-only 过滤）、desktop channel 机制（`OPENCODE_CHANNEL`，union `dev|beta|prod|muc`）。
- **MUC 品牌硬编码（分散 5 包，无注册表）**：
  - desktop：`APP_NAMES/APP_IDS`（`main/index.ts`）、`muc-gate.tsx`（启动门，红金 + admin.wuxuexi.top）、`main/muc/*`（deep-link/gateway/connect/secret-store）、`electron-builder.config.ts` muc case、`icons/muc`、`resources/muc`
  - opencode：`provider/muc.ts`（MUC 常量 + `DEFAULT_GATEWAY=http://admin.wuxuexi.top` + `MUC_API_KEY`）
  - tui：`logo.ts`（民大 ASCII 画）、`component/logo.tsx`（红金硬编码）、`util/presentation.ts`（ANSI 色）、`theme/assets/minzu.json` + 默认主题回退 minzu
  - ui / app：`logo.tsx` Mark/民 字、`wordmark-v2.tsx` mucode 字标、`v2/styles/theme.css` campus 调色板、`new-session-view.tsx` 校门 hero、`index.html` 标题 mucode
- 品牌缝隙：desktop channel union（5 处重复定义）是最接近注册表的现有机制。

## 1. 目标架构（单 Core + 品牌注册表）

### sub2api 侧
- `backend/internal/pkg/campus/brand.go`：`Brand` 结构 + `MUC`/`HUBU` 注册表（id、中英文名、motto、founded、协议 scheme、Redis 前缀、Key 名前缀、设备提示、API 路径段）。
- `muc_connect_handler.go` → 参数化为 campus handler（构造时注入 Brand）；`routes/muc.go` 保持 `/api/v1/muc/*` 字节不变；新增 `routes/hubu.go` 注册 `/api/v1/hubu/*`。MUC 行为零变化（测试保障）。
- `router.go` 下载目录解析顺序：`DOWNLOADS_DIR` > `MUC_DOWNLOADS_DIR` > 默认（向后兼容）。
- 前端：`src/brand/`（index.ts + muc.ts + hubu.ts），`VITE_BRAND`（构建期，默认 muc）驱动：tailwind primary 调色板、AuthLayout、Login 跳转、Header/Sidebar 文案；`/muc` 页保留原样，新增 `/hubu` 页 + HubuView.vue + `api/campus.ts`（按品牌调 connect-code）。
- 品牌资产：`frontend/src/assets/hubu/hubu-hero.png`（主视觉，另在 `assets/brands/hubu/hubu-hero.png` 存一份 canonical 副本）。

### opencode-muc 侧
- 新包 `packages/brand`（`@opencode/brand`）：`brands/{muc,hubu}/brand.json` + `src/index.ts`（类型、注册表、`resolveBrand(channel|env)`）+ `src/logos.ts`（两校 ASCII 画与颜色）。五个包统一从注册表取品牌数据，消灭重复硬编码。
- desktop：channel union 增加 `hubu`；APP_NAMES `HUBU AI`、APP_ID `cn.edu.hubu.harness`、协议 `hubu`（+opencode）、`HUBU_GATEWAY_URL`（默认 `http://localhost:8081`）、`HUBU_API_KEY` env、gate 页绿金 + hero；electron-builder hubu case（productName `HUBU AI`、artifact `hubu-ai-*`、icons `resources/hubu`）；prebuild sidecar 支持 hubu。
- opencode core：`provider/muc.ts` 品牌化（gateway/exchange 路径/key env 按 brand），muc 行为不变。
- tui：新增 `theme/assets/hubu.json`（深绿+青铜金）并注册；默认主题按品牌回退；logo/尾声横幅按品牌取画与色。
- ui/app：Mark/字标/启动 hero/标题按品牌。
- 资产：`icons/hubu/`、`resources/hubu/`、renderer & app hubu hero。

## 2. 本地拓扑与隔离

| | MUC（现状，不动） | HUBU（新建） |
|---|---|---|
| 网站 | http://localhost:8080 | http://localhost:8081 |
| 进程 | docker `sub2api`（预构建镜像） | 本机 go 二进制（embed 前端，BRAND=hubu 构建） |
| Postgres | 容器 `sub2api-postgres`（无宿主端口） | 容器 `sub2api-hubu-postgres`（127.0.0.1:5433） |
| Redis | 容器 `sub2api-redis`（无宿主端口） | 容器 `sub2api-hubu-redis`（127.0.0.1:6380） |
| network | sub2api-network | sub2api-hubu-network |
| volume | bind ./postgres_data 等 | 具名卷 sub2api-hubu-postgres-data / sub2api-hubu-redis-data |
| 数据目录 | sub2api-deploy/ | sub2api-deploy-hubu/（独立，不入 git） |
| 协议 | muc:// | hubu:// |
| Redis 前缀 | muc:code: | hubu:code: |

- 编排文件：`共用/sub2api-deploy-hubu/docker-compose.yml`（模仿 sub2api-deploy 模式，独立目录不进 git）。
- 启动器：`sub2api/scripts/dev-hubu.sh`（构建前端→go build embed→起 pg/redis→起 server→seed→打印 ready 信息）。
- Mock provider：本机小服务（Bun）`127.0.0.1:9999` 模拟 OpenAI 兼容上游，供账号接入与测试请求，绝不使用真实第三方 Key。

## 3. 安全规则（与 MUC 一致）
- code 60s / GETDEL 单次使用 / URL 只带 code 不带 Key / 日志无 code、Key 明文。
- 凭据：desktop safeStorage（Keychain）；core 只经进程 env。
- 真实上游 Key 只允许放 `sub2api-deploy-hubu/.env.local`（gitignore）；源码、git、URL、日志、asar 零真实 Key。
- hubu dist 与二进制为构建产物，不提交 dist 到 git（提交前恢复）。

## 4. 分支
- sub2api：`main` → 新分支 `hubu-local`
- opencode-muc：`muc-harness` → 新分支 `hubu`

## 5. HUBU 品牌规范
- 名称：湖北大学 / Hubei University / HUBU / 产品 HUBU AI；校训「日思日睿，笃志笃行」；1931。
- 视觉：深湖大绿 + 青铜金 + 墨绿黑底 + 楚文化意象（编钟/凤鸟纹）。颜色一律从用户提供的主视觉图采样生成（非官方 VI 标准色，报告中如实标注），anchor 由采样脚本从 hubu-hero.png 提取。
- 主视觉：hubu-hero.png（cover + 深色渐变遮罩，保证 UI 可读，禁止拉伸变形）。

## 6. 验收对照（用户 A–M 全覆盖）
A/B/C 共存与独立启动；D /hubu 品牌；E /muc 不变（线上跑预构建镜像 + 源码默认 muc 不动 + 构建验证）；F Desktop 文案；G TUI 主题；H hubu://；I connect-code/exchange；J /v1/models；K 密钥扫描（源码/git/URL/日志/asar）；L 数据隔离（容器/卷/端口/前缀）；M 登录→建 Key→一键连接→模型发现→测试请求（mock 上游）。

## 7. 停下来问用户的情形（本次预期均不触发）
破坏 MUC / 接触生产 / 无主视觉图 / 需真实三方 Key / 重大命名分歧。主视觉图已提供（Downloads/ChatGPT Image 2026年9月19日 01_28_50.png），产品名采用用户建议 HUBU AI。
