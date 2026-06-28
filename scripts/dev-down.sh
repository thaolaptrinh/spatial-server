#!/usr/bin/env bash

set -euo pipefail

echo "Stopping spatial-server development environment..."
docker compose -f deploy/docker-compose/compose.yaml -f deploy/docker-compose/compose.app.yaml down
echo "All services stopped."
