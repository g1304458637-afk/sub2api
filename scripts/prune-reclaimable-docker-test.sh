#!/usr/bin/env bash
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
tmp_dir="$(mktemp -d)"
trap 'rm -rf "$tmp_dir"' EXIT

cat > "$tmp_dir/docker" <<'MOCK'
#!/usr/bin/env bash
printf '%s\n' "$*" >> "$DOCKER_CALLS"
MOCK
chmod +x "$tmp_dir/docker"

calls_file="$tmp_dir/calls"
DOCKER_CALLS="$calls_file" PATH="$tmp_dir:$PATH" bash "$SCRIPT_DIR/prune-reclaimable-docker.sh"

expected="$tmp_dir/expected"
cat > "$expected" <<'EXPECTED'
image prune --force
builder prune --force
EXPECTED

diff -u "$expected" "$calls_file"
if grep -En '(^| )(system|volume|container) prune|--all|-a([[:space:]]|$)' "$SCRIPT_DIR/prune-reclaimable-docker.sh"; then
  echo "Unsafe prune command found" >&2
  exit 1
fi

echo "Safe Docker cleanup scope tests passed"
