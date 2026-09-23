#!/usr/bin/env bash
# Remove only resources Docker classifies as unused and regenerable.
# Never prune named images, containers, volumes, or the Docker system as a whole.
set -euo pipefail

echo "Docker image prune: dangling images only"
docker image prune --force

echo "Docker builder prune: cache unused for at least 7 days"
docker builder prune --force --filter 'until=168h'
