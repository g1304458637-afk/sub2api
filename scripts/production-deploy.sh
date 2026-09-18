#!/usr/bin/env bash
# =============================================================================
# production-deploy.sh — 生产部署（在服务器上以 root 执行）
#
# 用法:
#   sudo bash production-deploy.sh --image <ghcr.io/.../sub2api:sha-xxxx> \
#                                  --commit <git-sha> \
#                                  [--token-file <ghcr读取令牌文件>] [--ghcr-user <user>]
# 回滚(内部入口，保持 .env.deploy 其余记录):
#   sudo bash production-deploy.sh --rollback-only <镜像ref> [--commit <sha>]
#
# 流程: 预检 → 记录当前镜像(PREVIOUS_IMAGE) → 写 .env.deploy → pull → up -d --no-deps sub2api
#       → 健康检查；失败自动回滚到 PREVIOUS_IMAGE 并再次健康检查。
#
# 退出码: 0=成功  1=部署失败但回滚成功  2=CRITICAL 回滚也失败  3=预检失败(未做任何变更)
# 绝不执行: compose down / volume rm / system prune -a。绝不输出 secret。
# =============================================================================
set -euo pipefail

COMPOSE_DIR="/srv/sub2api"
CONTAINER="sub2api"
ENV_DEPLOY="$COMPOSE_DIR/.env.deploy"
COMPOSE=(docker compose -f "$COMPOSE_DIR/docker-compose.yml" --env-file "$COMPOSE_DIR/.env" --env-file "$ENV_DEPLOY")
SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"

NEW_IMAGE="" NEW_COMMIT="" TOKEN_FILE="" GHCR_USER="" ROLLBACK_MODE=0
while [ $# -gt 0 ]; do
  case "$1" in
    --image)        NEW_IMAGE="${2:?}"; shift 2 ;;
    --commit)       NEW_COMMIT="${2:?}"; shift 2 ;;
    --token-file)   TOKEN_FILE="${2:?}"; shift 2 ;;
    --ghcr-user)    GHCR_USER="${2:?}"; shift 2 ;;
    --rollback-only) ROLLBACK_MODE=1; NEW_IMAGE="${2:?}"; shift 2 ;;
    *) echo "未知参数: $1" >&2; exit 3 ;;
  esac
done
[ -n "$NEW_IMAGE" ] || { echo "缺少 --image" >&2; exit 3; }

cleanup() { [ -n "$TOKEN_FILE" ] && [ -f "$TOKEN_FILE" ] && rm -f "$TOKEN_FILE" || true; }
trap cleanup EXIT

fail_preflight() { echo "PREFLIGHT FAIL: $*" >&2; exit 3; }

state_field() { # $1=字段名 → 输出 .env.deploy 中该字段当前值
  [ -f "$ENV_DEPLOY" ] && grep "^$1=" "$ENV_DEPLOY" | head -1 | cut -d= -f2- || true
}

write_env_deploy() { # $1=SUB2API_IMAGE $2=PREVIOUS_IMAGE $3=DEPLOY_COMMIT $4=PREVIOUS_COMMIT $5=ROLLBACK
  cat > "$ENV_DEPLOY" <<EOF
# 由 production-deploy.sh 维护的部署状态文件。不含任何 secret。
SUB2API_IMAGE=$1
PREVIOUS_IMAGE=$2
DEPLOY_COMMIT=$3
PREVIOUS_COMMIT=$4
DEPLOYED_AT=$(date -u +%Y-%m-%dT%H:%M:%SZ)
ROLLBACK=$5
EOF
  chown admin:admin "$ENV_DEPLOY"
  chmod 600 "$ENV_DEPLOY"
}

# ---------------------------------------------------------------------------
if [ "$ROLLBACK_MODE" = "1" ]; then
  # 回滚模式：镜像切回指定版本并重启应用容器，健康检查失败直接退出 2
  OLD_IMAGE=$(state_field "SUB2API_IMAGE"); OLD_IMAGE=${OLD_IMAGE:-$NEW_IMAGE}
  OLD_PREV_COMMIT=$(state_field "PREVIOUS_COMMIT"); OLD_PREV_COMMIT=${OLD_PREV_COMMIT:-unknown}
  write_env_deploy "$NEW_IMAGE" "$OLD_IMAGE" "$OLD_PREV_COMMIT" "" "true"
  "${COMPOSE[@]}" up -d --no-deps "$CONTAINER"
  # 宽松模式：回滚目标可能是旧版镜像（尚无 /healthz JSON），只要求容器健康+真实探针+路由存在
  if bash "$SCRIPT_DIR/production-healthcheck.sh" --lenient --retries 12 --interval 5; then
    echo "ROLLBACK_OK image=$NEW_IMAGE commit=$OLD_PREV_COMMIT"
    exit 0
  fi
  echo "CRITICAL: ROLLBACK FAILED — 请立即人工介入，切勿执行任何删除性操作" >&2
  exit 2
fi

# ---------------------------------------------------------------------------
echo "== 预检 =="
[ -f "$COMPOSE_DIR/docker-compose.yml" ] || fail_preflight "compose 文件不存在"
[ -f "$COMPOSE_DIR/.env" ] || fail_preflight ".env 不存在"

avail_kb=$(df --output=avail -k "$COMPOSE_DIR" | tail -1)
echo "可用磁盘: $((avail_kb / 1024)) MB"
[ "$avail_kb" -gt 2097152 ] || fail_preflight "磁盘可用空间不足 2GB"

# 回滚点：优先取 .env.deploy 中已部署记录，其次容器实际镜像
if [ -f "$ENV_DEPLOY" ] && grep -q '^SUB2API_IMAGE=' "$ENV_DEPLOY"; then
  PREVIOUS_IMAGE=$(grep '^SUB2API_IMAGE=' "$ENV_DEPLOY" | head -1 | cut -d= -f2-)
  PREVIOUS_COMMIT=$(grep '^DEPLOY_COMMIT=' "$ENV_DEPLOY" | head -1 | cut -d= -f2-)
else
  PREVIOUS_IMAGE=$(docker inspect -f '{{.Config.Image}}' "$CONTAINER" 2>/dev/null || echo "")
  PREVIOUS_COMMIT="unknown"
fi
[ -n "$PREVIOUS_IMAGE" ] || PREVIOUS_IMAGE="weishaw/sub2api:latest"
echo "回滚点 PREVIOUS_IMAGE=$PREVIOUS_IMAGE (commit ${PREVIOUS_COMMIT:-unknown})"
echo "目标镜像 NEW_IMAGE=$NEW_IMAGE (commit ${NEW_COMMIT:-n/a})"

GHCR_CONFIG_TMP=""
login_ghcr() {
  { [ -n "$TOKEN_FILE" ] && [ -s "$TOKEN_FILE" ]; } || return 0
  GHCR_CONFIG_TMP=$(mktemp -d)
  GHCR_USER="${GHCR_USER:-oauth2}"
  if ! docker --config "$GHCR_CONFIG_TMP" login ghcr.io -u "$GHCR_USER" --password-stdin < "$TOKEN_FILE"; then
    rm -rf "$GHCR_CONFIG_TMP"; GHCR_CONFIG_TMP=""
    return 1
  fi
}
logout_ghcr() {
  if [ -n "$GHCR_CONFIG_TMP" ]; then
    docker --config "$GHCR_CONFIG_TMP" logout ghcr.io >/dev/null 2>&1 || true
    rm -rf "$GHCR_CONFIG_TMP"; GHCR_CONFIG_TMP=""
  fi
}

deploy_fail() {
  echo "DEPLOYMENT FAILED: $*" >&2
  echo "== 自动回滚到 $PREVIOUS_IMAGE =="
  if bash "$SCRIPT_DIR/production-deploy.sh" --rollback-only "$PREVIOUS_IMAGE"; then
    echo "ROLLBACK SUCCESSFUL (已恢复 $PREVIOUS_IMAGE)"
    exit 1
  fi
  echo "CRITICAL: ROLLBACK FAILED — 请立即人工介入，切勿执行任何删除性操作" >&2
  exit 2
}

echo "== 写部署状态 =="
write_env_deploy "$NEW_IMAGE" "$PREVIOUS_IMAGE" "$NEW_COMMIT" "${PREVIOUS_COMMIT:-unknown}" "false"

echo "== 拉取镜像 =="
login_ghcr || deploy_fail "GHCR 登录失败"
PULL_CONFIG="${GHCR_CONFIG_TMP:-/root/.docker}"
if ! docker --config "$PULL_CONFIG" pull "$NEW_IMAGE"; then
  logout_ghcr
  deploy_fail "镜像拉取失败"
fi
logout_ghcr

echo "== 更新应用容器（仅 sub2api，不动 db/redis/volumes）=="
if ! "${COMPOSE[@]}" up -d --no-deps "$CONTAINER"; then
  deploy_fail "compose up 失败"
fi

echo "== 健康检查 =="
if bash "$SCRIPT_DIR/production-healthcheck.sh" --expect-commit "$NEW_COMMIT" --retries 24 --interval 5; then
  echo "DEPLOY OK image=$NEW_IMAGE commit=$NEW_COMMIT previous=$PREVIOUS_IMAGE"
  exit 0
fi
deploy_fail "健康检查未通过"
