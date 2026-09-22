#!/usr/bin/env bash
# HUBU（湖北大学）本地开发环境一键启动器。
#
# 用法：
#   ./scripts/dev-hubu.sh              # 启动（数据层容器 + 网关 8081 + mock 上游 + seed）
#   ./scripts/dev-hubu.sh --rebuild    # 强制重编前端（BRAND=hubu）+ 后端二进制
#   ./scripts/dev-hubu.sh --stop       # 停止网关与 mock（数据容器保留）
#   ./scripts/dev-hubu.sh --down       # --stop 并停止数据容器（卷保留，数据不丢）
#
# 隔离承诺：容器 sub2api-hubu-{postgres,redis}、网络 sub2api-hubu-network、
# 卷 sub2api-hubu-*-data、数据目录 ../sub2api-deploy-hubu/、端口 8081。
# 与 MUC 本地（8080）和生产环境完全隔离；本脚本绝不触远端。
set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
HUBU_DIR="${HUBU_DIR:-$ROOT_DIR/../sub2api-deploy-hubu}"
PORT="${HUBU_PORT:-8081}"
MOCK_PORT="${HUBU_MOCK_PORT:-9999}"
BRAND="hubu"

log()  { printf '\033[32;1m[HUBU]\033[0m %s\n' "$*"; }
warn() { printf '\033[33;1m[HUBU]\033[0m %s\n' "$*"; }
die()  { printf '\033[31;1m[HUBU]\033[0m %s\n' "$*" >&2; exit 1; }

[[ -d "$HUBU_DIR" ]] || die "找不到 $HUBU_DIR（请先创建 HUBU 本地部署目录）"
[[ -f "$HUBU_DIR/.env" ]] || die "缺少 $HUBU_DIR/.env（含本地随机口令，不入 git）"

stop_procs() {
  # 匹配任意启动形式（绝对路径或 ./hubu-server），以 hubu-server 为特征串
  pkill -f "hubu-server" 2>/dev/null || true
  pkill -f "deploy-hubu/mock-upstream.mjs" 2>/dev/null || true
  sleep 1
}

case "${1:-}" in
  --stop) stop_procs; log "已停止 HUBU 网关与 mock 上游（数据容器保留，重启用 docker compose -f $HUBU_DIR/docker-compose.yml up -d）"; exit 0 ;;
  --down) stop_procs; (cd "$HUBU_DIR" && docker compose down); log "已停止并移除容器（卷保留）"; exit 0 ;;
esac

REBUILD=""
[[ "${1:-}" == "--rebuild" ]] && REBUILD=1

command -v docker >/dev/null || die "需要 docker（colima）"
docker info >/dev/null 2>&1 || die "docker 不可用，请先启动 colima"

# 1) 数据层（隔离容器）
log "启动数据层（sub2api-hubu-postgres / sub2api-hubu-redis）"
(cd "$HUBU_DIR" && docker compose up -d)
for i in $(seq 1 30); do
  pg=$(docker inspect -f '{{.State.Health.Status}}' sub2api-hubu-postgres 2>/dev/null || echo none)
  rd=$(docker inspect -f '{{.State.Health.Status}}' sub2api-hubu-redis 2>/dev/null || echo none)
  [[ "$pg" == "healthy" && "$rd" == "healthy" ]] && break
  [[ $i -eq 30 ]] && die "数据层未就绪"
  sleep 2
done
log "数据层 healthy"

# 2) 二进制（缺失或 --rebuild 时重编）
if [[ -n "$REBUILD" || ! -x "$HUBU_DIR/hubu-server" ]]; then
  log "构建前端（BRAND=$BRAND）…"
  (cd "$ROOT_DIR/frontend" && BRAND=$BRAND pnpm build) >>"$HUBU_DIR/data/build.log" 2>&1 ||
    die "前端构建失败（见 $HUBU_DIR/data/build.log）"
  log "构建后端（embed）…"
  (cd "$ROOT_DIR/backend" && CGO_ENABLED=0 go build -tags embed -o "$HUBU_DIR/hubu-server" ./cmd/server) ||
    die "后端构建失败"
fi

# 3) 启动网关 + mock 上游
stop_procs
log "启动 HUBU 网关（:$PORT）与 mock 上游（:$MOCK_PORT）"
set -a; source "$HUBU_DIR/.env"; set +a
DATA_DIR="$HUBU_DIR/data" \
AUTO_SETUP=true \
ADMIN_EMAIL="$HUBU_ADMIN_EMAIL" ADMIN_PASSWORD="$HUBU_ADMIN_PASSWORD" \
DATABASE_HOST=127.0.0.1 DATABASE_PORT=5433 DATABASE_USER="$HUBU_PG_USER" \
DATABASE_PASSWORD="$HUBU_PG_PASSWORD" DATABASE_DBNAME="$HUBU_PG_DB" DATABASE_SSLMODE=disable \
REDIS_HOST=127.0.0.1 REDIS_PORT=6380 \
SERVER_PORT="$PORT" DOWNLOADS_DIR="$HUBU_DIR/data/downloads" TZ=Asia/Shanghai \
nohup "$HUBU_DIR/hubu-server" >>"$HUBU_DIR/data/server.log" 2>&1 &
disown || true
HUBU_MOCK_PORT="$MOCK_PORT" HUBU_MOCK_UPSTREAM_KEY="${HUBU_MOCK_UPSTREAM_KEY:-sk-hubu-local-mock}" \
  nohup bun "$HUBU_DIR/mock-upstream.mjs" >>"$HUBU_DIR/data/mock.log" 2>&1 &
disown || true

for i in $(seq 1 30); do
  curl -sf -m 2 "http://localhost:$PORT/healthz" >/dev/null && break
  [[ $i -eq 30 ]] && { tail -20 "$HUBU_DIR/data/server.log"; die "网关未就绪"; }
  sleep 2
done

# 4) Seed（幂等：合规确认/品牌设置/分组/账号/demo 用户/测试 Key）
log "执行 seed（幂等）"
(cd "$HUBU_DIR" && python3 seed.py) || warn "seed 未全部成功（可能已初始化过，检查上方输出）"

cat <<EOF

==================================================================
  HUBU local environment ready

  Website : http://localhost:$PORT
  Admin   : http://localhost:$PORT/admin
  HUBU    : http://localhost:$PORT/hubu

  深链协议 : hubu://connect?code=...
  Mock 上游: http://127.0.0.1:$MOCK_PORT/v1（hubu-mock-lite / hubu-mock-pro）
  Demo Key : $HUBU_DIR/data/demo_api_key.txt
  管理员   : $HUBU_ADMIN_EMAIL（口令见 $HUBU_DIR/.env）

  停止     : ./scripts/dev-hubu.sh --stop
  停止+容器: ./scripts/dev-hubu.sh --down
==================================================================
EOF
