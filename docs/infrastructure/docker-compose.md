# Docker Compose

> **Last Updated:** 2026-06-28

## Purpose

Provide a single-host Docker Compose environment for local development, demos,
and CI. Docker Compose is the **local dev platform only** — production always
uses K3s (see `infra/k3s/`).

## Layered file structure (Compose v2)

Compose files live in `deploy/docker-compose/` and are layered with `include:`
rather than packed into one file. Each layer has a single responsibility:

| File | Role | Make target |
|------|------|-------------|
| `compose.yaml` | Base **infrastructure**: Postgres + Redis | `make dev-up` |
| `compose.app.yaml` | **App** services: room-service + gateway + a single game-server (`include`s `compose.yaml`) | `make dev-up-full`, `make demo` |
| `compose.scaling.yaml` | **Multi-node**: 2 named game-server nodes for ownership/migration testing (`include`s `compose.yaml`) | `make scale-up` |
| `compose.obs.yaml` | **Observability** overlay: Prometheus + Grafana + Loki + Promtail + Alertmanager | `make obs-up` |

Supporting files in the same directory: `prometheus.yml`, `promtail.yml`.

### Common invocations

```bash
make dev-up        # Postgres + Redis only (fast; run services via `go run ./apps/...`)
make dev-up-full   # infra + app (room-service + gateway + game-server)
make demo          # infra + app, --build + force-recreate + test client (auto-teardown)
make scale-up      # infra + 2 named game-server nodes
make obs-up        # infra + app + observability stack
```

> `.env` (gitignored) is read from the **repo root** (the CWD where Compose
> runs), not from `deploy/docker-compose/`. See `/.env.example`. Secrets use
> `${VAR:-default}` so dev works out of the box; override for prod via `.env`.

## Service Definitions

| Service | Built from | Replicas | Depends On |
|---------|------------|----------|------------|
| gateway | `build/docker/gateway.Dockerfile` | 1 | room-service, redis |
| room-service | `build/docker/room-service.Dockerfile` | 1 | postgres, redis |
| game-server | `build/docker/game-server.Dockerfile` | 1 (or 2 named in scaling) | postgres, room-service |
| postgres | `postgres:16-alpine` | 1 | — |
| redis | `redis:7-alpine` | 1 | — |

### Startup order

```
postgres → redis → room-service → gateway, game-server
```

Postgres/Redis use `service_healthy` (built-in healthchecks). App services use
`service_started` today — wiring `service_healthy` for them requires Dockerfile
`HEALTHCHECK` (planned next pass, see ADR-027).

### Port mappings

| Service | Host:Container | Protocol |
|---------|----------------|----------|
| gateway (WS) | 8080:8080 | WebSocket |
| room-service (gRPC) | 9001:9000 | gRPC |
| game-server (gRPC) | not host-exposed (internal only) | gRPC |
| postgres | 5432:5432 | TCP |
| redis | 6379:6379 | TCP |

Each service also exposes Prometheus metrics internally on container port 9090
(`configs/defaults.yml`: `metrics.port`) — scraped by Prometheus in `compose.obs.yaml`.

## Configuration

`configs/*.yml` and `internal/storage/migrations/` are **COPY'd into each image**
at build time (self-contained images). Runtime overrides come via environment
variables using the loader convention in `internal/config/config.go`: prefix
`SPATIAL_`, `__` as the nested-key separator (e.g. `SPATIAL_GATEWAY__JWT_SECRET`
→ `gateway.jwt_secret`).

## Scaling

Two distinct modes — choose by intent:

1. **Degenerate `--scale`** (any replica can serve any zone):
   ```bash
   make dev-up-full
   docker compose -f deploy/docker-compose/compose.yaml \
                  -f deploy/docker-compose/compose.app.yaml \
                  up -d --scale game-server=3 --no-deps game-server
   ```
   ⚠ All replicas share the advertised address `game-server:9000`, so
   room-service round-robins lookups across them — a zone owned by replica A may
   receive traffic on replica B. Safe only when any replica can serve any zone.

2. **True multi-node** (distinct per-node ownership / migration testing):
   ```bash
   make scale-up   # compose.scaling.yaml — named game-server-1 / game-server-2
   ```
   Each node has a distinct routable hostname + `SPATIAL_GRPC__HOST`. Identity
   itself is a per-process UUIDv7 (`internal/types/id.go`), so nodes are always
   distinct; the distinct *address* is what `--scale` cannot provide.

Gateway and Room Service are single-instance in Compose (multi-instance only in K3s).

## Volumes

- `pgdata` — persistent PostgreSQL data across restarts.
- `prometheus_data`, `grafana_data`, `loki_data` — observability persistence
  (defined in `compose.obs.yaml`).

## Networks

All services share the Compose **default** bridge network (flat, for local dev).
Production network segmentation (public/private/database/monitoring) is not
replicated in Compose — see [networking.md](networking.md) and K3s NetworkPolicies.

## References

- ADR-008 — Deployment Strategy
- ADR-027 — Docker Compose v2 restructure
- [Deployment Guide](../operations/deployment.md)
- [networking.md](networking.md)
