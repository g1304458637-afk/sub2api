#!/usr/bin/env bash
# =============================================================================
# production-deploy.sh — 生产部署（在服务器上以 root 执行）
#
# 用法:
#   sudo bash production-deploy.sh --image <ghcr.io/.../sub2api:sha-xxxx> \
#                                  --commit <git-sha> \
#                                  --version <version> --build-timestamp <RFC3339> \
#                                  --migration-baseline <git-sha> --brand muc \
#                                  [--token-file <ghcr读取令牌文件>] [--ghcr-user <user>]
# 回滚(内部入口，保持 .env.deploy 其余记录):
#   sudo bash production-deploy.sh --rollback-only <镜像ref> [--commit <sha>]
#
# 流程: 预检 → 记录当前镜像(PREVIOUS_IMAGE) → 写 .env.deploy → pull → up -d --no-deps sub2api
#       → 健康检查；失败自动回滚到 PREVIOUS_IMAGE 并再次健康检查。
#
# 退出码: 0=成功  1=部署失败但回滚成功  2=CRITICAL 回滚也失败  3=部署预检失败  4=可回收空间清理失败
# 绝不执行: compose down / volume rm / system prune -a。绝不输出 secret。
# =============================================================================
set -euo pipefail

COMPOSE_DIR="/srv/sub2api"
CONTAINER="sub2api"
ENV_DEPLOY="$COMPOSE_DIR/.env.deploy"
COMPOSE=(docker compose -f "$COMPOSE_DIR/docker-compose.yml" --env-file "$COMPOSE_DIR/.env" --env-file "$ENV_DEPLOY")
SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"

NEW_IMAGE="" NEW_COMMIT="" NEW_VERSION="" BUILD_TIMESTAMP="" MIGRATION_BASELINE="" BRAND="" TOKEN_FILE="" GHCR_USER="" ROLLBACK_MODE=0 PRUNE_RECLAIMABLE_DOCKER_DATA=0
EXPECTED_LEGACY_IMAGE="" EXPECTED_LEGACY_LEDGER=""
while [ $# -gt 0 ]; do
  case "$1" in
    --cleanup-reclaimable-docker-data) PRUNE_RECLAIMABLE_DOCKER_DATA=1; shift ;;
    --image)        NEW_IMAGE="${2:?}"; shift 2 ;;
    --commit)       NEW_COMMIT="${2:?}"; shift 2 ;;
    --version)      NEW_VERSION="${2:?}"; shift 2 ;;
    --build-timestamp) BUILD_TIMESTAMP="${2:?}"; shift 2 ;;
    --migration-baseline) MIGRATION_BASELINE="${2:?}"; shift 2 ;;
    --brand)        BRAND="${2:?}"; shift 2 ;;
    --expected-legacy-image) EXPECTED_LEGACY_IMAGE="${2-}"; shift 2 ;;
    --expected-legacy-ledger) EXPECTED_LEGACY_LEDGER="${2-}"; shift 2 ;;
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

write_env_deploy() { # image, previous image, commit, previous commit, rollback, version, build time, migration baseline, brand
  cat > "$ENV_DEPLOY" <<EOF
# 由 production-deploy.sh 维护的部署状态文件。不含任何 secret。
SUB2API_IMAGE=$1
PREVIOUS_IMAGE=$2
DEPLOY_COMMIT=$3
PREVIOUS_COMMIT=$4
DEPLOYED_AT=$(date -u +%Y-%m-%dT%H:%M:%SZ)
ROLLBACK=$5
VERSION=${6:-unknown}
BUILD_TIMESTAMP=${7:-unknown}
MIGRATION_BASELINE=${8:-unknown}
BRAND=${9:-unknown}
EOF
  chown admin:admin "$ENV_DEPLOY"
  chmod 600 "$ENV_DEPLOY"
}

# ---------------------------------------------------------------------------
if [ "$ROLLBACK_MODE" = "1" ]; then
  # 回滚模式：镜像切回指定版本并重启应用容器，健康检查失败直接退出 2
  OLD_IMAGE=$(state_field "SUB2API_IMAGE"); OLD_IMAGE=${OLD_IMAGE:-$NEW_IMAGE}
  OLD_PREV_COMMIT=$(state_field "PREVIOUS_COMMIT"); OLD_PREV_COMMIT=${OLD_PREV_COMMIT:-unknown}
  write_env_deploy "$NEW_IMAGE" "$OLD_IMAGE" "$OLD_PREV_COMMIT" "" "true" \
    "$(state_field VERSION)" "$(state_field BUILD_TIMESTAMP)" "$(state_field MIGRATION_BASELINE)" "$(state_field BRAND)"
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
printf '%s' "$NEW_COMMIT" | grep -Eq '^[0-9a-f]{40}$' || fail_preflight "--commit 必须是完整 40 位 Git SHA"
printf '%s' "$MIGRATION_BASELINE" | grep -Eq '^[0-9a-f]{40}$' || fail_preflight "--migration-baseline 必须是完整 40 位 Git SHA"
[ -n "$NEW_VERSION" ] || fail_preflight "缺少 --version"
[ -n "$BUILD_TIMESTAMP" ] || fail_preflight "缺少 --build-timestamp"
[ "$BRAND" = "muc" ] || fail_preflight "生产只允许 brand=muc"

if [ "$PRUNE_RECLAIMABLE_DOCKER_DATA" = "1" ]; then
  echo "== 部署前清理可回收 Docker 数据（仅悬空镜像和当前未被构建使用的缓存）=="
  before_kb=$(df --output=avail -k "$COMPOSE_DIR" | tail -1)
  echo "清理前可用磁盘: $((before_kb / 1024)) MB"
  if ! bash "$SCRIPT_DIR/prune-reclaimable-docker.sh"; then
    echo "PREFLIGHT CLEANUP FAIL: 可回收 Docker 数据清理失败；应用容器未切换" >&2
    exit 4
  fi
fi

avail_kb=$(df --output=avail -k "$COMPOSE_DIR" | tail -1)
echo "可用磁盘: $((avail_kb / 1024)) MB"
if [ "$avail_kb" -le 2097152 ]; then
  echo "== 磁盘占用诊断（只读）==" >&2
  df -h "$COMPOSE_DIR" >&2 || true
  docker system df >&2 || true
  echo "本机 Docker 镜像清单（只读）:" >&2
  docker image ls --format 'table {{.ID}}\t{{.Repository}}:{{.Tag}}\t{{.Size}}' >&2 || true
  if command -v du >/dev/null 2>&1; then
    echo "系统日志、缓存、临时目录和 /srv 一级占用（只读）:" >&2
    du -xhd1 /var/log /var/cache /tmp /srv 2>/dev/null | sort -h | tail -n 40 >&2 || true
  fi
  fail_preflight "磁盘可用空间不足 2GB"
fi

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

if [ -n "$EXPECTED_LEGACY_IMAGE" ] || [ -n "$EXPECTED_LEGACY_LEDGER" ]; then
  printf '%s' "$EXPECTED_LEGACY_IMAGE" | grep -Eq '^sha256:[0-9a-f]{64}$' || fail_preflight "invalid legacy image identity"
  printf '%s' "$EXPECTED_LEGACY_LEDGER" | grep -Eq '^[0-9a-f]{64}$' || fail_preflight "invalid legacy ledger identity"
  [ "$(docker inspect sub2api --format '{{.Image}}')" = "$EXPECTED_LEGACY_IMAGE" ] || fail_preflight "legacy image changed since audit"
  ledger_sha=$(bash "$SCRIPT_DIR/production-migration-ledger.sh" | sha256sum | cut -d' ' -f1)
  [ "$ledger_sha" = "$EXPECTED_LEGACY_LEDGER" ] || fail_preflight "migration ledger changed since audit"
  # Record the real rollback image; a migration-only SHA is not the legacy app SHA.
  PREVIOUS_IMAGE="$EXPECTED_LEGACY_IMAGE"
  PREVIOUS_COMMIT=unknown
fi

# A fresh remote recovery point precedes changes to deployment state or the app.
umask 077
BACKUP_DIR="/srv/backups/sub2api-predeploy-$(date -u +%Y%m%dT%H%M%SZ)-${NEW_COMMIT:0:12}"
mkdir -m 700 "$BACKUP_DIR"
docker exec sub2api-postgres sh -c 'pg_dump -U "$POSTGRES_USER" -d "$POSTGRES_DB" --no-owner --no-acl -Fc' > "$BACKUP_DIR/database.dump"
docker exec -i sub2api-postgres pg_restore --list < "$BACKUP_DIR/database.dump" > "$BACKUP_DIR/database.list"
CNY_CONTRACT_BEFORE=$(docker exec sub2api-postgres sh -c 'psql -U "$POSTGRES_USER" -d "$POSTGRES_DB" -Atqc "SELECT EXISTS (SELECT 1 FROM settings WHERE key = '\''wallet_currency_contract'\'' AND value = '\''CNY_V1'\'')"')
case "$CNY_CONTRACT_BEFORE" in t|f) ;; *) fail_preflight "无法确认 CNY 钱包迁移前状态" ;; esac
cp -p "$COMPOSE_DIR/docker-compose.yml" "$COMPOSE_DIR/.env" "$BACKUP_DIR/"
[ ! -f "$ENV_DEPLOY" ] || cp -p "$ENV_DEPLOY" "$BACKUP_DIR/"
printf '%s\n' "$PREVIOUS_IMAGE" > "$BACKUP_DIR/rollback-image.txt"
sha256sum "$BACKUP_DIR/database.dump" > "$BACKUP_DIR/SHA256SUMS"
echo "Verified readable database recovery archive: $BACKUP_DIR"

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
  # The one-time CNY conversion changes stored wallet units. If this release
  # applied it and the app must roll back, restore the verified predeploy DB
  # snapshot before starting the older USD-denominated binary.
  if [ "$CNY_CONTRACT_BEFORE" = "f" ]; then
    contract_after=$(docker exec sub2api-postgres sh -c 'psql -U "$POSTGRES_USER" -d "$POSTGRES_DB" -Atqc "SELECT EXISTS (SELECT 1 FROM settings WHERE key = '\''wallet_currency_contract'\'' AND value = '\''CNY_V1'\'')"' 2>/dev/null) || contract_after=unknown
    if [ "$contract_after" = "t" ]; then
      echo "== Restore pre-CNY database before app rollback ==" >&2
      if ! "${COMPOSE[@]}" stop "$CONTAINER" || ! docker exec -i sub2api-postgres sh -c 'pg_restore --clean --if-exists --no-owner --no-acl -U "$POSTGRES_USER" -d "$POSTGRES_DB"' < "$BACKUP_DIR/database.dump"; then
        echo "CRITICAL: CNY database restore failed; application remains stopped for recovery" >&2
        exit 2
      fi
    elif [ "$contract_after" != "f" ]; then
      echo "CRITICAL: cannot determine whether CNY migration committed; application remains on the attempted release" >&2
      exit 2
    fi
  fi
  echo "== 自动回滚到 $PREVIOUS_IMAGE =="
  if bash "$SCRIPT_DIR/production-deploy.sh" --rollback-only "$PREVIOUS_IMAGE"; then
    echo "ROLLBACK SUCCESSFUL (已恢复 $PREVIOUS_IMAGE)"
    exit 1
  fi
  echo "CRITICAL: ROLLBACK FAILED — 请立即人工介入，切勿执行任何删除性操作" >&2
  exit 2
}

echo "== 写部署状态 =="
write_env_deploy "$NEW_IMAGE" "$PREVIOUS_IMAGE" "$NEW_COMMIT" "${PREVIOUS_COMMIT:-unknown}" "false" \
  "$NEW_VERSION" "$BUILD_TIMESTAMP" "$MIGRATION_BASELINE" "$BRAND"

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
