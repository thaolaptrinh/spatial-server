#!/usr/bin/env bash

set -euo pipefail

echo "Starting spatial-server development environment (Postgres + Redis)..."
docker compose -f deploy/docker-compose/compose.yaml up -d
echo "PostgreSQL is ready at localhost:5432"
echo "Redis is ready at localhost:6379"
echo "Run the services locally with: go run ./apps/gateway  (etc.)"
echo "For the full containerized stack use: make dev-up-full"
