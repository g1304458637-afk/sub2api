#!/usr/bin/env bash
# Strict health check for the isolated HUBU production stack.
set -euo pipefail

EXPECT_COMMIT=""
RETRIES=6
INTERVAL=5

while [ "$#" -gt 0 ]; do
  case "$1" in
    --expect-commit) EXPECT_COMMIT="${2:?}"; shift 2 ;;
    --retries) RETRIES="${2:?}"; shift 2 ;;
    --interval) INTERVAL="${2:?}"; shift 2 ;;
    *) echo "unknown argument: $1" >&2; exit 1 ;;
  esac
done

printf '%s' "$EXPECT_COMMIT" | grep -Eq '^[0-9a-f]{40}$' || {
  echo "HEALTHCHECK FAIL: an exact commit SHA is required" >&2
  exit 1
}

check_container() {
  local name="$1" state
  state=$(docker inspect -f '{{.State.Running}} {{if .State.Health}}{{.State.Health.Status}}{{else}}none{{end}}' "$name" 2>/dev/null || echo 'missing none')
  echo "$name: $state"
  [ "$state" = "true healthy" ]
}

attempt=0
while :; do
  attempt=$((attempt + 1))
  echo "== HUBU health check $attempt/$RETRIES =="
  ok=1
  check_container sub2api-hubu || ok=0
  check_container sub2api-hubu-postgres || ok=0
  check_container sub2api-hubu-redis || ok=0

  body=$(curl -fsS --max-time 10 http://127.0.0.1:8081/healthz 2>/dev/null || true)
  if [ -n "$body" ] && printf '%s' "$body" | python3 -c '
import json, sys
try:
    data=json.load(sys.stdin)
except Exception:
    sys.exit(1)
sys.exit(0 if data.get("status")=="ok" and data.get("brand")=="hubu" and data.get("commit")==sys.argv[1] else 1)
' "$EXPECT_COMMIT"; then
    echo "HUBU /healthz: status=ok brand=hubu commit=$EXPECT_COMMIT"
  else
    echo "HUBU /healthz did not report the expected status, brand, and commit"
    ok=0
  fi

  [ "$ok" = "1" ] && { echo "HUBU HEALTHCHECK PASS"; exit 0; }
  [ "$attempt" -ge "$RETRIES" ] && { echo "HUBU HEALTHCHECK FAIL after $RETRIES attempts" >&2; exit 1; }
  sleep "$INTERVAL"
done
