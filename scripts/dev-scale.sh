#!/usr/bin/env bash
# Scale the single game-server in the app stack to N replicas.
#
# CAVEAT: `--scale` is only safe when ANY replica can serve ANY zone — all
# replicas share the advertised address `game-server:9000`, so room-service
# round-robins lookups across them and a zone owned by replica A may have its
# traffic delivered to replica B. For genuine per-node zone ownership /
# migration testing (distinct routable addresses), use `make scale-up` instead
# (compose.scaling.yaml defines named game-server-1 / game-server-2 nodes).
# See ADR-027.
set -euo pipefail
N="${1:-3}"
docker compose -f deploy/docker-compose/compose.yaml \
               -f deploy/docker-compose/compose.app.yaml \
               up -d --scale game-server="$N" --no-deps game-server
echo "game-server scaled to N=$N (degenerate any-replica mode); watch room-service logs."
