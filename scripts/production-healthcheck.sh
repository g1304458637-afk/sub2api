#!/usr/bin/env bash
# =============================================================================
# production-healthcheck.sh — 生产 Sub2API 健康检查（在服务器上执行）
#
# 用法: sudo bash production-healthcheck.sh [--expect-commit <sha>] [--lenient]
#                                           [--retries N] [--interval N]
#
# 严格模式（默认）: 容器 healthy + /healthz 200 且 JSON status=ok (+commit 匹配)
#                   + 关键路由语义化状态码全部正确。
# 宽松模式 --lenient: 回滚到旧版镜像（尚无 /healthz JSON）时使用：
#                   容器 healthy + /health 200 + 关键路由状态码正确。
#
# 退出码: 0 = 通过; 1 = 失败。绝不输出任何 secret。
# =============================================================================
set -euo pipefail

BASE_URL="http://127.0.0.1:8080"
CONTAINER="sub2api"
EXPECT_COMMIT=""
RETRIES=6
INTERVAL=5
LENIENT=0

while [ $# -gt 0 ]; do
  case "$1" in
    --expect-commit) EXPECT_COMMIT="${2:?}"; shift 2 ;;
    --lenient)       LENIENT=1; shift ;;
    --retries)       RETRIES="${2:?}"; shift 2 ;;
    --interval)      INTERVAL="${2:?}"; shift 2 ;;
    *) echo "未知参数: $1" >&2; exit 1 ;;
  esac
done

fail() { echo "HEALTHCHECK FAIL: $*" >&2; exit 1; }
curl_code() { curl -s -o /dev/null -w '%{http_code}' --max-time 10 "$@"; }

attempt=0
while :; do
  attempt=$((attempt + 1))
  echo "== 健康检查 第 ${attempt}/${RETRIES} 轮 =="
  ok=1

  # 1) 容器运行且 docker 层 healthy
  state=$(docker inspect -f '{{.State.Running}} {{if .State.Health}}{{.State.Health.Status}}{{else}}none{{end}}' "$CONTAINER" 2>/dev/null || echo "missing none")
  echo "container: $state"
  [ "$state" = "true healthy" ] || { ok=0; echo "容器未运行或未 healthy"; }

  # 2) /healthz：严格模式要求 200 + JSON status=ok + commit 匹配
  hz_code=$(curl_code "$BASE_URL/healthz")
  hz_body=$(curl -s --max-time 10 "$BASE_URL/healthz" || true)
  echo "/healthz -> $hz_code $hz_body"
  if [ "$LENIENT" = "1" ]; then
    code=$(curl_code "$BASE_URL/health")
    echo "/health  -> $code"
    [ "$code" = "200" ] || { ok=0; echo "期望 /health 200"; }
  else
    if [ "$hz_code" = "200" ] && echo "$hz_body" | grep -q '"status":"ok"'; then
      if [ -n "$EXPECT_COMMIT" ]; then
        if echo "$hz_body" | grep -q "\"commit\":\"$EXPECT_COMMIT\""; then
          echo "commit 匹配: $EXPECT_COMMIT"
        else
          ok=0; echo "commit 不匹配（期望 $EXPECT_COMMIT）—— 新镜像可能未生效"
        fi
      fi
    else
      ok=0; echo "期望 /healthz 200 且 JSON status=ok"
    fi
  fi

  # 3) 关键路由存在性（语义化状态码即认为路由存在）
  code=$(curl_code -X POST "$BASE_URL/api/v1/muc/connect-code")
  echo "POST /api/v1/muc/connect-code (未登录) -> $code"
  [ "$code" = "401" ] || { ok=0; echo "期望 401"; }

  code=$(curl_code -X POST -H 'Content-Type: application/json' -d '{}' "$BASE_URL/api/v1/muc/exchange")
  echo "POST /api/v1/muc/exchange (空参)   -> $code"
  [ "$code" = "400" ] || { ok=0; echo "期望 400"; }

  code=$(curl_code "$BASE_URL/v1/models")
  echo "GET  /v1/models (无 key)          -> $code"
  [ "$code" = "401" ] || { ok=0; echo "期望 401"; }

  [ "$ok" = "1" ] && { echo "HEALTHCHECK PASS"; exit 0; }

  [ "$attempt" -ge "$RETRIES" ] && fail "重试 ${RETRIES} 轮后仍未通过"
  sleep "$INTERVAL"
done
