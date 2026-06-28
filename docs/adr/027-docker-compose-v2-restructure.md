# ADR 027: Docker Compose v2 Restructure

> **Last Updated:** 2026-06-28

## Status

Accepted / Implemented.

## Purpose

Restructure the local-development Docker Compose files into a clean, layered
**Compose v2** set (`compose.*.yaml`) that is self-standing, correctly named,
and free of the structural defects present in the original `docker-compose*.yml`
layout. This ADR records *why* the layered design and its specific mechanics
were chosen.

## Context

The original deployment-compose layout was a single `docker-compose.yml` plus
three sibling files. A docker-expert audit surfaced seven structural problems:

1. `docker-compose.yml` mixed **infrastructure** (Postgres, Redis) with **app
   services** (room-service, game-server, gateway). `make dev-up` documented
   itself as "Postgres + Redis only" but actually built and started the whole
   stack — there was no fast DB-only path for local `go run ./apps/...`.
2. `docker-compose.2node.yml` and `docker-compose.multinode.yml` **duplicated**
   the same two-node game-server topology via different mechanisms
   (`extends:` vs inline build); neither was obviously canonical.
3. `docker-compose.staging.yml` was **mislabeled** — it is the observability
   stack (Prometheus + Grafana + Loki), not a "staging" application environment.
4. Overlays were **not self-standing**: `2node`/`multinode` declared
   `depends_on: postgres` / `room-service` without defining them, so the
   documented standalone command failed.
5. `staging.yml` mounted a **non-existent** `./prometheus.yml`, leaving
   Prometheus broken on startup.
6. Secrets were **hardcoded** literals (`JWT_SECRET`, `POSTGRES_PASSWORD`,
   Grafana `admin`).
7. A missing `.dockerignore` shipped the entire repo (`.git/`, `docs/`, tests)
   as build context on every `docker build`.

## Problem

The layout had to support four distinct, cleanly separable use cases on a single
host:

- **DB-only dev** — Postgres + Redis, to run services via `go run`.
- **Full demo stack** — infra + all app services + a test client.
- **Multi-node validation** — infra + room-service + gateway + two game-server
   nodes for ownership-transfer / entity-migration testing.
- **Observability** — Prometheus + Grafana + Loki + Promtail + Alertmanager.

…and it had to do so without the duplication, mislabeling, broken overlays,
missing config, or secret leakage above — while preserving the project's
established multi-node correctness model.

### Key constraint: multi-node routing

Game-server identity is a **per-process UUIDv7** (`internal/types/id.go`,
`apps/game-server/main.go`), so two replicas always get distinct identities.
However, the *advertised address* is `SPATIAL_GRPC__HOST`. With plain
`docker compose up --scale game-server=N`, every replica inherits the same
`SPATIAL_GRPC__HOST` (`game-server`), and Docker DNS round-robins that name
across replicas. Room Service then returns the same `game-server:9000` for every
node, and the gateway may dial the **wrong replica** for a zone it does not own.
Genuine multi-node therefore requires **explicitly-named nodes with distinct
hostnames** — `--scale` is only safe in the degenerate case where any replica
can serve any zone.

## Decision

Adopt a **layered Compose v2** file set in `deploy/docker-compose/`, composed
with top-level `include:`:

| File | Responsibility |
|------|----------------|
| `compose.yaml` | Base infra: Postgres + Redis (healthchecks, `pgdata` volume). |
| `compose.app.yaml` | App services (room-service + gateway + single game-server); `include`s `compose.yaml`. |
| `compose.scaling.yaml` | Multi-node: two named `game-server-1` / `game-server-2` nodes; `include`s `compose.yaml`. |
| `compose.obs.yaml` | Observability overlay; run on top of the app stack. |

Supporting files: `prometheus.yml`, `promtail.yml` (in the same directory);
`.env.example` and `.dockerignore` at the repo root.

Specific mechanics:

- **`include:` over `extends:`.** `include:` is the idiomatic Compose v2
  composition primitive; `extends:` was only re-added in Compose 2.24 (Mar 2024)
  and carries a version caveat. Obsolete `version: "3.9"` is dropped.
- **Named nodes via a top-level extension anchor.** `compose.scaling.yaml`
  declares `x-game-server-node: &game-server-node` (a top-level extension field,
  ignored by Compose but its anchor is resolvable) and merges it into
  `game-server-1` / `game-server-2` with `<<:`, overriding `hostname` and
  `SPATIAL_GRPC__HOST` per node. `environment` is written per node because YAML
  merge keys do not deep-merge maps.
- **Secrets as `${VAR:-default}`.** Hardcoded literals are replaced with
  `${VAR:-devdefault}` so out-of-the-box dev still works while production
  overrides via a root `.env`. Compose v2 loads `.env` from the **CWD (repo
  root)**, not the `-f` directory — documented in `.env.example`. The
  env→config mapping follows `internal/config/config.go` (prefix `SPATIAL_`,
  `__` → `.`).
- **`.dockerignore`** excludes `.git/`, `docs/`, `tests/`, `infra/`, `deploy/`,
  test files, and local env — keeping only what the Go build + runtime image need.
- **Metrics** are scraped on container port `9090`
  (`configs/defaults.yml`: `metrics.port`).
- New Make targets: `dev-up-full`, `scale-up`/`scale-down`, `obs-up`/`obs-down`;
  `dev-up` is now genuinely DB-only; `demo` uses `compose.yaml` + `compose.app.yaml`.

## Alternatives Considered

- **`extends:` (the original `2node.yml` mechanism).** Rejected: version caveat
  (Compose < 2.24) and it re-declared `depends_on`/env identically to the base,
  partially defeating reuse.
- **Compose `profiles` to share `room-service`+`gateway` between `compose.app.yaml`
  and `compose.scaling.yaml`.** Rejected: adds `--profile` flags to common
  commands and produces surprising partial states; the small deliberate
  duplication of two service definitions is simpler and guarantees
  `compose.scaling.yaml` starts exactly two game-server nodes.
- **Plain `--scale game-server=N` for multi-node.** Rejected for correctness
  (see Problem): it aliases replicas to one round-robined address and breaks
  per-node routing. Retained only on the single `game-server` in
  `compose.app.yaml` for the degenerate any-replica case (with a documented caveat).
- **Keep Dockerfiles in `deploy/docker/` (as some stale docs implied).** Not
  adopted: Dockerfiles stay in `build/docker/` (the documented, correct
  location); only the compose files were restructured.

## Tradeoffs

- `room-service` + `gateway` definitions appear in both `compose.app.yaml` and
  `compose.scaling.yaml`. This is deliberate (simplicity + exact node count) at
  the cost of ~14 duplicated lines.
- `.env` must live at the repo root, which is mildly surprising vs. co-locating
  it with the compose files. Mitigated by `.env.example` documentation.
- App services still use `depends_on: service_started` (not `service_healthy`)
  because the Dockerfiles do not yet define `HEALTHCHECK`. See Future.

## Consequences

- `make dev-up` is fast and DB-only, matching its documentation.
- Multi-node validation runs against a correct 2-node topology with distinct
  routable addresses.
- The observability overlay starts functional (real `prometheus.yml` /
  `promtail.yml`, persistent volumes).
- Secrets are no longer committed as literals.
- All historical references under `docs/superpowers/plans|specs/` and the
  immutable ADR-008 intentionally retain the old filenames as historical record.

## Future

- **Dockerfile hardening (next pass):** non-root `USER`, `HEALTHCHECK`
  (`grpc_health_probe` for room-service/game-server; `wget` for the gateway
  `/health` endpoint), build-args + `org.opencontainers.image.*` labels, then
  upgrade compose `depends_on` for app services to `service_healthy`. Optional
  `distroless` base (the binaries are already `CGO_ENABLED=0`).
- Network segmentation (frontend/backend `internal:`) — not replicated in Compose
  today; production uses K3s NetworkPolicies.
