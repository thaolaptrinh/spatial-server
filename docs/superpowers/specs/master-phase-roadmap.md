# Spatial Server — Master Phase Roadmap

> **Last Updated:** 2026-06-27
> **Status:** Active

## Purpose

Defines the scope, dependencies, and deliverables for all remaining phases (1Finish through 6). Each phase has a dedicated spec and implementation plan.

## Phase Overview

| Phase | Name | Status | Depends On | Est. Effort |
|-------|------|--------|------------|-------------|
| 1A-1G | Core Scaffolding + Demo | ✅ Complete | — | Done |
| **1Finish** | **Hardened Vertical Slice** | 📋 Planned | 1G | Medium |
| **2** | **Realtime Features** | 📋 Planned | 1Finish | Medium |
| **3** | **Distributed Scaling** | 📋 Planned | 2 | Large |
| **4** | **Session + Backpressure** | 📋 Planned | 3 | Medium |
| **5** | **Infra-as-Code** | 📋 Planned | 1Finish (partial) | Medium |
| **6** | **Production Hardening** | 📋 Planned | 4, 5 | Large |

## Current State (Post Phase 1G)

**Working:** Single-server vertical slice — client connects via WebSocket → Gateway validates JWT → Relay gRPC stream to Game Server → entity spawn/move/despawn via AOI. `make demo` works end-to-end.

**Key gaps:** In-memory state (restart = data loss), 5/23 RPCs implemented, no metrics, no zone migration, no session resumption, no infra.

---

## Phase 1Finish — Hardened Vertical Slice

**Goal:** Make the single-server slice production-grade: persistent state, correct packet protocol, business API, and real integration tests.

**Scope:**
- Wire Room Service ownership/registry to PostgreSQL (replace in-memory maps)
- Align packet protocol header with ADR-010 (version + sequence + msgType)
- Implement SpatialServerAPI (CreateRuntime/DestroyRuntime/GetRuntimeInfo/ListRuntimes)
- Real integration tests via Testcontainers (replace skipped test)
- Gateway hardening: `/ready`+`/live` endpoints, packet size cap, graceful drain
- Use `pkg/config` consistently across all services (remove inline koanf)

**Out of scope:** Multi-server, zone migration, metrics, TLS

**Spec:** `docs/superpowers/specs/phase-1finish-hardened-vertical-slice.md`

---

## Phase 2 — Realtime Features

**Goal:** Complete the single-server realtime experience — NPC simulation, periodic state persistence, and observability.

**Scope:**
- NPC simulation loop (configurable behavior: patrol, idle, wander)
- EntityAction/EntityState packet handling (client → server commands)
- Periodic zone-state snapshots to PostgreSQL (`zone_state` table, configurable interval)
- `/metrics` endpoint + Prometheus scrape + basic Grafana dashboard
- gRPC interceptors (recovery, logging, metrics)
- Entity lifecycle hooks (spawn/simulate/despawn callbacks)

**Out of scope:** Cross-zone, migration, session resumption

**Spec:** `docs/superpowers/specs/phase-2-realtime-features.md`

---

## Phase 3 — Distributed Scaling

**Goal:** Transform single-server into multi-server — zones distributed across Game Servers with cross-zone AOI and migration.

**Scope:**
- Implement GameServer RPCs: AssignZone, ReleaseZone, QueryEntities, NotifyEntityEnter/Leave, SendEntityUpdate
- Cross-zone AOI subscription + ghost-entity handoff
- Zone migration: PrepareTransfer + TransferZone + ZoneStateSync streaming (ADR-002)
- Entity cross-zone migration (MigrateEntity)
- Room Service HA: K3s Lease leader election
- Heartbeat-timeout sweeper + orphan-zone reassignment (ADR-011)
- Push-based routing-cache invalidation (Gateway subscribes to ownership changes)

**Out of scope:** Session resumption, TLS, autoscaling

**Spec:** `docs/superpowers/specs/phase-3-distributed-scaling.md`

---

## Phase 4 — Session Resumption + Backpressure

**Goal:** Production-grade durability and flow control.

**Scope:**
- ADR-022: Redis-backed session tokens, 30s reconnection window
- Game Server delta ring-buffer replay on reconnect
- Backpressure: write deadlines, bounded-buffer policies, graceful drain
- Per-connection rate limiting (token bucket: 100 msg/s)
- Per-IP rate limiting (500 msg/s aggregate)
- Dynamic rate-limit config

**Out of scope:** TLS, autoscaling, chaos testing

**Spec:** `docs/superpowers/specs/phase-4-session-backpressure.md`

---

## Phase 5 — Infra-as-Code

**Goal:** Complete infrastructure pipeline from code to production K3s.

**Scope:**
- Helm charts for all services (gateway, room-service, game-server, postgres, redis, monitoring)
- K3s manifests (Deployments, Services, HPA, ConfigMaps, Secrets, PDB, Ingress)
- Terraform modules (cloud provider provisioning)
- cloud-init scripts (node bootstrap)
- docker-compose.staging.yml (Prometheus + Grafana + Loki)
- OpenTelemetry collector + distributed tracing across gRPC
- Full monitoring stack: Prometheus, Grafana, Loki + Promtail, Alertmanager

**Out of scope:** TLS cert management, autoscaling tuning, chaos testing

**Spec:** `docs/superpowers/specs/phase-5-infra-as-code.md`

---

## Phase 6 — Production Hardening + Sign-off

**Goal:** Validate production readiness against ADR targets.

**Scope:**
- TLS 1.3 (WSS) + cert-manager integration
- Optional mTLS internal (Gateway ↔ Room Service ↔ Game Server)
- Autoscaling wired (HPA custom metrics, Compose scale hook)
- Full benchmark suite: light/medium/heavy/burst/zone-transfer/stability scenarios
- p95 < 100ms validation, pprof flame graphs
- Chaos testing for every ADR-011 failure mode
- Capacity validation: 10K conns/Gateway, 5K entities/Game Server (ADR-017)
- JWT migration to asymmetric keys (EdDSA + JWKS)
- Security audit + penetration testing checklist

**Out of scope:** New features (freeze for hardening)

**Spec:** `docs/superpowers/specs/phase-6-production-hardening.md`

---

## References

- [ADR set](../adr/) — 23 architectural decisions
- [Phase 1F spec](./2026-06-26-phase1f-realtime-data-path.md)
- [Phase 1G spec](./2026-06-26-phase1g-make-it-demoable.md)
- [Repository structure](../architecture/repository-structure.md)
