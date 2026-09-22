#!/usr/bin/env bash
set -euo pipefail
docker exec sub2api-postgres sh -c 'psql -v ON_ERROR_STOP=1 -U "$POSTGRES_USER" -d "$POSTGRES_DB" -At -c "COPY (SELECT filename, checksum FROM schema_migrations ORDER BY filename) TO STDOUT"'
