#!/usr/bin/env bash
# =============================================================================
# production-rollback.sh — 手动回滚（在服务器上以 root 执行）
#
# 用法:
#   sudo bash production-rollback.sh              # 回滚到 .env.deploy 中的 PREVIOUS_IMAGE
#   sudo bash production-rollback.sh <镜像ref>    # 显式指定目标镜像（如 sha- 旧版本）
#
# 退出码: 0=回滚成功  2=CRITICAL 回滚失败  3=无可用回滚点
# =============================================================================
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
ENV_DEPLOY="/srv/sub2api/.env.deploy"

if [ $# -ge 1 ]; then
  TARGET_IMAGE="$1"
else
  TARGET_IMAGE=$([ -f "$ENV_DEPLOY" ] && grep '^PREVIOUS_IMAGE=' "$ENV_DEPLOY" | head -1 | cut -d= -f2- || true)
  [ -n "$TARGET_IMAGE" ] || { echo "PREFLIGHT FAIL: 无 PREVIOUS_IMAGE，且未显式指定镜像" >&2; exit 3; }
fi

echo "回滚目标: $TARGET_IMAGE"
exec bash "$SCRIPT_DIR/production-deploy.sh" --rollback-only "$TARGET_IMAGE"
