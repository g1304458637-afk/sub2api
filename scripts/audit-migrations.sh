#!/usr/bin/env bash
# =============================================================================
# audit-migrations.sh — 数据库 migration 安全审计
#
# 用法: bash audit-migrations.sh <BASE_SHA> <HEAD_SHA>
#
# 检查 BASE..HEAD 之间新增的 backend/migrations/*.sql 是否包含破坏性操作。
# 政策（第十二节）: 只允许 ADD TABLE / ADD COLUMN / 安全 index / 数据回填(INSERT/UPDATE)。
# 发现 DROP / RENAME / 改变数据类型 / TRUNCATE / DELETE 时判定为不可逆，退出 1。
# =============================================================================
set -euo pipefail

BASE=${1:?用法: audit-migrations.sh <BASE_SHA> <HEAD_SHA>}
HEAD=${2:?用法: audit-migrations.sh <BASE_SHA> <HEAD_SHA>}

cd "$(git rev-parse --show-toplevel)"

git cat-file -e "$BASE^{commit}"
git cat-file -e "$HEAD^{commit}"
removed=$(git diff --name-only --diff-filter=D "$BASE" "$HEAD" -- 'backend/migrations/*.sql')
[ -z "$removed" ] || { echo "MIGRATION AUDIT FAIL: removed migration files: $removed" >&2; exit 1; }
new_migrations=$(git diff --name-only --diff-filter=AM "$BASE" "$HEAD" -- 'backend/migrations/*.sql')
if [ -z "$new_migrations" ]; then
  echo "MIGRATION AUDIT PASS: 本次无新增 migration 文件"
  exit 0
fi

echo "新增 migration 文件:"
echo "$new_migrations"

# 不可逆/破坏性语句关键字
pattern='DROP[[:space:]]+(TABLE|COLUMN|INDEX|CONSTRAINT|SCHEMA|DATABASE)|RENAME|ALTER[[:space:]]+COLUMN|USING[[:space:]]+.*::|TRUNCATE|DELETE[[:space:]]+FROM'

violation=0
while IFS= read -r f; do
  content=$(git show "$HEAD:$f")
  hits=$(printf '%s\n' "$content" | grep -inE "$pattern" || true)
  if [ -n "$hits" ]; then
    violation=1
    echo "::error::发现潜在不可逆 migration: $f"
    echo "$hits"
  fi
done <<EOF
$new_migrations
EOF

if [ "$violation" = "1" ]; then
  echo "MIGRATION AUDIT FAIL: 存在潜在不可逆 migration，生产部署已阻止。" >&2
  echo "如确需此类变更：人工评审并走显式豁免流程，不得自动放行。" >&2
  exit 1
fi
echo "MIGRATION AUDIT PASS"
