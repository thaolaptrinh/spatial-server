#!/usr/bin/env bash

set -euo pipefail

echo "Starting spatial-server development environment..."
docker compose -f deploy/docker-compose/docker-compose.yml up -d
echo "PostgreSQL is ready at localhost:5432"
echo "Redis is ready at localhost:6379"
