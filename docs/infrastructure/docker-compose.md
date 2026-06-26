# Docker Compose

> **Last Updated:** 2026-06-26

## Purpose

Provide a single-host Docker Compose environment for local development, staging, and CI. Docker Compose is the local dev platform only — production always uses K3s.

## Service Definitions

| Service | Image | Replicas | Depends On |
|---------|-------|----------|------------|
| gateway | `gateway:dev` | 1 | postgres, redis, room-service |
| room-service | `room-service:dev` | 1 | postgres, redis |
| game-server | `game-server:dev` | 1 (scalable) | postgres, redis, room-service |
| postgres | `postgres:16-alpine` | 1 | — |
| redis | `redis:7-alpine` | 1 | — |

### Startup Order

```
postgres → redis → room-service → gateway, game-server
```

## Configuration

Compose file: `deploy/docker-compose/docker-compose.yml`

Overlay file for monitoring: `deploy/docker-compose/docker-compose.staging.yml` (adds Prometheus + Grafana).

Config files from `configs/*.yml` are bind-mounted into each container. Environment variables override config file values at runtime.

### Port Mappings

| Service | Host Port | Container Port | Protocol |
|---------|-----------|----------------|----------|
| gateway (WS) | 8080 | 8080 | WebSocket |
| room-service (gRPC) | 9000 | 9000 | gRPC |
| game-server (gRPC) | 9001 | 9001 | gRPC |
| postgres | 5432 | 5432 | TCP |
| redis | 6379 | 6379 | TCP |

## Health Checks

All services implement a `/health` HTTP endpoint. Compose health checks use `curl --fail http://localhost:8080/health` (gateway) or gRPC health probe (room-service, game-server). PostgreSQL and Redis use built-in `pg_isready` and `redis-cli ping`.

## Scaling

Game Server supports `--scale game-server=N` for multi-instance testing:

```bash
docker compose -f deploy/docker-compose/docker-compose.yml up --scale game-server=3
```

Gateway and Room Service are single-instance in Compose (multi-instance only in K3s).

## Volumes

- `pgdata` — persistent PostgreSQL data across restarts
- `redisdata` — persistent Redis data across restarts

Both are named volumes defined in the Compose file.

## Networks

| Network | Services |
|---------|----------|
| `spatial-net` | All services (flat network for local dev) |

Production network segmentation (public/private/database/monitoring) is not replicated in Compose. See [networking.md](networking.md).

## References

- ADR-008 — Deployment Strategy
- ADR-014 — Infrastructure Platform
- [Deployment Guide](../operations/deployment.md)
- [networking.md](networking.md)
