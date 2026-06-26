#!/usr/bin/env bash

set -euo pipefail

echo "Stopping spatial-server development environment..."
docker compose -f deploy/docker-compose/docker-compose.yml down
echo "All services stopped."
