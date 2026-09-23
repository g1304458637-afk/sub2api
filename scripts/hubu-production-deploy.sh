#!/usr/bin/env bash
# Update only the HUBU application container; keep its database, cache and volumes intact.
set -euo pipefail

COMPOSE_DIR="/srv/sub2api-hubu"
COMPOSE_FILE="$COMPOSE_DIR/compose.yml"
RELEASE_FILE="$COMPOSE_DIR/release.json"
BACKUP_ROOT="/srv/backups/hubu-production"
SERVICE="backend"
CONTAINER="sub2api-hubu"
DB_CONTAINER="sub2api-hubu-postgres"
PROJECT="campus-hubu"
SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
COMPOSE_TOOL="$SCRIPT_DIR/hubu-compose-image.py"

NEW_IMAGE="" NEW_COMMIT="" NEW_VERSION="" BUILD_TIMESTAMP="" MIGRATION_BASELINE="" TOKEN_FILE="" GHCR_USER=""
while [ "$#" -gt 0 ]; do
  case "$1" in
    --image) NEW_IMAGE="${2:?}"; shift 2 ;;
    --commit) NEW_COMMIT="${2:?}"; shift 2 ;;
    --version) NEW_VERSION="${2:?}"; shift 2 ;;
    --build-timestamp) BUILD_TIMESTAMP="${2:?}"; shift 2 ;;
    --migration-baseline) MIGRATION_BASELINE="${2:?}"; shift 2 ;;
    --token-file) TOKEN_FILE="${2:?}"; shift 2 ;;
    --ghcr-user) GHCR_USER="${2:?}"; shift 2 ;;
    *) echo "unknown argument: $1" >&2; exit 3 ;;
  esac
done

fail_preflight() { echo "HUBU PREFLIGHT FAIL: $*" >&2; exit 3; }
valid_sha() { printf '%s' "$1" | grep -Eq '^[0-9a-f]{40}$'; }
valid_sha "$NEW_COMMIT" || fail_preflight "--commit must be a full Git SHA"
valid_sha "$MIGRATION_BASELINE" || fail_preflight "--migration-baseline must be a full Git SHA"
[ -n "$NEW_IMAGE" ] && [ -n "$NEW_VERSION" ] && [ -n "$BUILD_TIMESTAMP" ] || fail_preflight "image and release metadata are required"
[[ "$NEW_IMAGE" == ghcr.io/*:hubu-sha-"$NEW_COMMIT" ]] || fail_preflight "image must be the immutable HUBU image for this commit"
[[ "$NEW_VERSION" =~ ^[A-Za-z0-9_.-]+$ ]] || fail_preflight "version contains unsupported characters"
[ -f "$COMPOSE_FILE" ] || fail_preflight "HUBU compose file is missing"
[ -f "$COMPOSE_DIR/.env" ] || fail_preflight "HUBU environment file is missing"
[ -f "$RELEASE_FILE" ] || fail_preflight "HUBU release metadata is missing"
[ -n "$TOKEN_FILE" ] && [ -s "$TOKEN_FILE" ] || fail_preflight "GHCR token file is missing"

PREVIOUS_COMMIT="$MIGRATION_BASELINE"

wallet_contract_state() {
  docker exec "$DB_CONTAINER" sh -c 'psql -U "$POSTGRES_USER" -d "$POSTGRES_DB" -Atqc "SELECT EXISTS (SELECT 1 FROM settings WHERE key = '\''wallet_currency_contract'\'' AND value = '\''CNY_V1'\'')"'
}

check_compose_image() { python3 "$COMPOSE_TOOL" get "$COMPOSE_FILE"; }

PREVIOUS_IMAGE=$(docker inspect -f '{{.Config.Image}}' "$CONTAINER" 2>/dev/null) || fail_preflight "HUBU application container is missing"
[ "$(check_compose_image "$PREVIOUS_IMAGE")" = "$PREVIOUS_IMAGE" ] || fail_preflight "compose image mismatch"

for name in "$CONTAINER" sub2api-hubu-postgres sub2api-hubu-redis; do
  state=$(docker inspect -f '{{.State.Running}} {{if .State.Health}}{{.State.Health.Status}}{{else}}none{{end}}' "$name" 2>/dev/null || echo 'missing none')
  [ "$state" = "true healthy" ] || fail_preflight "$name is not healthy"
done
bash "$SCRIPT_DIR/hubu-production-healthcheck.sh" --expect-commit "$PREVIOUS_COMMIT" --retries 1 --interval 1 || fail_preflight "current HUBU deployment is not healthy"

CNY_CONTRACT_BEFORE=$(wallet_contract_state) || fail_preflight "could not determine HUBU wallet currency contract"
case "$CNY_CONTRACT_BEFORE" in t|f) ;; *) fail_preflight "unexpected HUBU wallet currency contract state" ;; esac

BACKUP_DIR="$BACKUP_ROOT/$NEW_COMMIT-$(date -u +%Y%m%dT%H%M%SZ)"
[ ! -e "$BACKUP_DIR" ] || fail_preflight "timestamped backup path already exists"
install -d -m 700 "$BACKUP_DIR"
cp -a "$COMPOSE_FILE" "$BACKUP_DIR/compose.yml"
cp -a "$RELEASE_FILE" "$BACKUP_DIR/release.json"
docker exec "$DB_CONTAINER" sh -c 'pg_dump -U "$POSTGRES_USER" -d "$POSTGRES_DB" --no-owner --no-acl -Fc' > "$BACKUP_DIR/database.dump"
docker exec -i "$DB_CONTAINER" pg_restore --list < "$BACKUP_DIR/database.dump" > "$BACKUP_DIR/database.list"
sha256sum "$BACKUP_DIR/database.dump" > "$BACKUP_DIR/SHA256SUMS"
printf '%s\n' "$PREVIOUS_IMAGE" > "$BACKUP_DIR/previous-image.txt"
printf '%s\n' "$PREVIOUS_COMMIT" > "$BACKUP_DIR/previous-commit.txt"
chmod 600 "$BACKUP_DIR/database.dump" "$BACKUP_DIR/database.list" "$BACKUP_DIR/SHA256SUMS" "$BACKUP_DIR/previous-image.txt" "$BACKUP_DIR/previous-commit.txt"

TOKEN_CONFIG=$(mktemp -d)
cleanup() {
  [ -n "${TOKEN_CONFIG:-}" ] && { docker --config "$TOKEN_CONFIG" logout ghcr.io >/dev/null 2>&1 || true; rm -rf "$TOKEN_CONFIG"; }
  [ -n "$TOKEN_FILE" ] && [ -f "$TOKEN_FILE" ] && rm -f "$TOKEN_FILE" || true
}
trap cleanup EXIT
GHCR_USER="${GHCR_USER:-oauth2}"
docker --config "$TOKEN_CONFIG" login ghcr.io -u "$GHCR_USER" --password-stdin < "$TOKEN_FILE" >/dev/null
DOCKER_CONFIG="$TOKEN_CONFIG" docker pull "$NEW_IMAGE"

replace_compose_image() {
  python3 "$COMPOSE_TOOL" set "$COMPOSE_FILE" "$1" "$2"
}

rollback() {
  echo "== Restore the prior HUBU app image and configuration ==" >&2
  if [ "$CNY_CONTRACT_BEFORE" = "f" ]; then
    contract_after=$(wallet_contract_state 2>/dev/null) || contract_after=unknown
    if [ "$contract_after" = "t" ]; then
      echo "== Restore pre-CNY database before app rollback ==" >&2
      if ! (cd "$COMPOSE_DIR" && docker compose --project-name "$PROJECT" -f "$COMPOSE_FILE" --env-file "$COMPOSE_DIR/.env" stop "$SERVICE") || \
         ! docker exec -i "$DB_CONTAINER" sh -c 'pg_restore --clean --if-exists --no-owner --no-acl -U "$POSTGRES_USER" -d "$POSTGRES_DB"' < "$BACKUP_DIR/database.dump"; then
        echo "CRITICAL: HUBU CNY database restore failed; application remains stopped for recovery" >&2
        exit 2
      fi
    elif [ "$contract_after" != "f" ]; then
      echo "CRITICAL: cannot determine whether HUBU CNY migration committed; application remains on the attempted release" >&2
      exit 2
    fi
  fi
  cp -a "$BACKUP_DIR/compose.yml" "$COMPOSE_FILE"
  cp -a "$BACKUP_DIR/release.json" "$RELEASE_FILE"
  if (cd "$COMPOSE_DIR" && docker compose --project-name "$PROJECT" -f "$COMPOSE_FILE" --env-file "$COMPOSE_DIR/.env" up -d --no-deps "$SERVICE") && \
     bash "$SCRIPT_DIR/hubu-production-healthcheck.sh" --expect-commit "$PREVIOUS_COMMIT" --retries 12 --interval 5; then
    echo "HUBU ROLLBACK OK commit=$PREVIOUS_COMMIT image=$PREVIOUS_IMAGE" >&2
    exit 1
  fi
  echo "CRITICAL: HUBU rollback failed; database and cache were not intentionally modified" >&2
  exit 2
}

echo "== Update only the HUBU backend service =="
replace_compose_image "$PREVIOUS_IMAGE" "$NEW_IMAGE" || fail_preflight "compose image update was refused"
if ! (cd "$COMPOSE_DIR" && DOCKER_CONFIG="$TOKEN_CONFIG" docker compose --project-name "$PROJECT" -f "$COMPOSE_FILE" --env-file "$COMPOSE_DIR/.env" pull "$SERVICE"); then
  rollback
fi
if ! (cd "$COMPOSE_DIR" && DOCKER_CONFIG="$TOKEN_CONFIG" docker compose --project-name "$PROJECT" -f "$COMPOSE_FILE" --env-file "$COMPOSE_DIR/.env" up -d --no-deps "$SERVICE"); then
  rollback
fi
if ! bash "$SCRIPT_DIR/hubu-production-healthcheck.sh" --expect-commit "$NEW_COMMIT" --retries 24 --interval 5; then
  rollback
fi

python3 - "$RELEASE_FILE" "$NEW_COMMIT" "$NEW_IMAGE" "$NEW_VERSION" "$BUILD_TIMESTAMP" "$MIGRATION_BASELINE" <<'PY' || rollback
import json, os, stat, sys, tempfile, time
path, commit, image, version, built, baseline = sys.argv[1:]
with open(path, encoding="utf-8") as f: data=json.load(f)
data.update({"source_commit":commit,"image":image,"version":version,"build_timestamp":built,"migration_baseline":baseline,"deployed_at":time.strftime("%Y-%m-%dT%H:%M:%SZ",time.gmtime())})
mode=stat.S_IMODE(os.stat(path).st_mode); st=os.stat(path)
fd,tmp=tempfile.mkstemp(prefix=".release.json.",dir=os.path.dirname(path),text=True)
try:
    with os.fdopen(fd,"w",encoding="utf-8") as f:
        json.dump(data,f,ensure_ascii=False,indent=2); f.write("\n")
    os.chmod(tmp,mode); os.chown(tmp,st.st_uid,st.st_gid); os.replace(tmp,path)
except Exception:
    try: os.unlink(tmp)
    except FileNotFoundError: pass
    raise
PY

echo "HUBU DEPLOY OK brand=hubu commit=$NEW_COMMIT version=$NEW_VERSION backup=$BACKUP_DIR"
