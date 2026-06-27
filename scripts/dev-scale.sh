#!/usr/bin/env bash
set -euo pipefail
N="${1:-3}"
docker compose -p spatial -f deploy/docker-compose/docker-compose.yml up -d --scale game-server="$N" --no-deps game-server
echo "game-server scaled to N=$N; watch room-service logs for zone rebalance."
