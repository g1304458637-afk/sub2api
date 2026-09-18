#!/usr/bin/env bash
# =============================================================================
# production-audit.sh — 只读生产状态审计（不做任何修改）
#
# 用法:
#   本机执行:  bash production-audit.sh [user@host] [-i keyfile] [-p port]
#   服务器上:  sudo bash production-audit.sh
#
# 输出: 架构/容器/镜像/compose 状态/.env.deploy/磁盘/路由探测。
# 绝不输出 .env 的值（只列键名）；绝不读取容器日志内容；绝不写服务器任何文件。
# =============================================================================
set -euo pipefail

REMOTE_ARGS=()
if [ "${1:-}" != "" ] && [[ "$1" != -* ]]; then REMOTE_HOST="$1"; shift; fi
while [ $# -gt 0 ]; do
  case "$1" in
    -i) REMOTE_ARGS+=(-i "${2:?}"); shift 2 ;;
    -p) REMOTE_ARGS+=(-p "${2:?}"); shift 2 ;;
    *) echo "未知参数: $1" >&2; exit 1 ;;
  esac
done

readonly REPORT='
echo "===== 1. 系统 ====="
uname -m
head -2 /etc/os-release | tr "\n" " "; echo
docker --version; docker compose version 2>/dev/null | head -1
df -h /srv | tail -1

echo "===== 2. 容器（全部状态，含已退出）====="
docker ps -a --format "table {{.Names}}\t{{.Image}}\t{{.Status}}" | grep -E "NAMES|sub2api|postgres|redis"

echo "===== 3. 本地镜像 ====="
docker images --format "{{.Repository}}:{{.Tag}} {{.ID}} {{.CreatedAt}}" | grep -Ei "sub2api|postgres|redis" | head -10

echo "===== 4. compose 镜像行 ====="
sed -n "/^  sub2api:/,/container_name/p" /srv/sub2api/docker-compose.yml | grep "image:"

echo "===== 5. compose 插值校验（双 env 文件）====="
docker compose -f /srv/sub2api/docker-compose.yml --env-file /srv/sub2api/.env --env-file /srv/sub2api/.env.deploy config 2>&1 | grep -E "^    image:|error" | head -5

echo "===== 6. .env.deploy（部署状态，设计上无 secret）====="
cat /srv/sub2api/.env.deploy 2>/dev/null || echo "(不存在)"

echo "===== 7. .env 键名清单（只列键，绝不输出值）====="
grep -oE "^[A-Z_0-9]+=" /srv/sub2api/.env | tr -d "=" | tr "\n" " "; echo

echo "===== 8. env 与 original 的键名差异（防配置漂移，只列键名）====="
diff <(grep -oE "^[A-Z_0-9]+=" /srv/sub2api/.env.original 2>/dev/null | sort) <(grep -oE "^[A-Z_0-9]+=" /srv/sub2api/.env | sort) || true

echo "===== 9. authorized_keys 指纹（确认部署 key 在列）====="
ssh-keygen -lf ~/.ssh/authorized_keys 2>/dev/null || sudo ssh-keygen -lf /home/admin/.ssh/authorized_keys 2>/dev/null || echo "(无法读取)"

echo "===== 10. nginx upstream（admin.wuxuexi.top）====="
nginx -T 2>/dev/null | grep -B8 "proxy_pass http://127.0.0.1:8080" | grep -E "server_name|proxy_pass" | head -4 || sudo nginx -T 2>/dev/null | grep -E "server_name admin|8080" | head -4

echo "===== 11. 路由探测（服务器本机）====="
for probe in "GET /healthz" "GET /health" "POST /api/v1/muc/connect-code" "POST /api/v1/muc/exchange" "GET /v1/models"; do
  m=${probe%% *}; p=${probe#* }
  code=$(curl -s -o /dev/null -w "%{http_code}" --max-time 8 -X "$m" -H "Content-Type: application/json" -d "{}" "http://127.0.0.1:8080$p" 2>/dev/null || echo "ERR")
  echo "$m $p -> $code"
done
curl -s --max-time 8 http://127.0.0.1:8080/healthz 2>/dev/null | head -c 200; echo

echo "===== 12. MUC 安装包目录 ====="
ls -la /srv/sub2api/data/downloads/ 2>/dev/null | head -8 || echo "(data/downloads 不存在)"

echo "===== 审计结束（只读，未做任何修改）====="
'

if [ -n "${REMOTE_HOST:-}" ]; then
  echo ">> 通过 SSH 只读审计 $REMOTE_HOST（不修改任何内容）"
  ssh -o BatchMode=yes -o AddressFamily=inet "${REMOTE_ARGS[@]}" "$REMOTE_HOST" "sudo bash -c '$REPORT'"
else
  bash -c "$REPORT"
fi
