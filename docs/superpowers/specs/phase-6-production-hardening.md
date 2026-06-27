# Phase 6 — Production Hardening + Sign-off

> **Last Updated:** 2026-06-27
> **Status:** Draft

## Purpose

Phase 5 delivered reproducible infrastructure and observability, but left four production gates open: transport security is TLS-ready but not enforced, JWT validation still uses a shared HMAC secret, autoscaling is not wired to real spatial metrics, and none of the [ADR-017](../../adr/017-capacity-planning.md) capacity targets or [ADR-020](../../adr/020-benchmark-strategy.md) performance budgets have been measured against a real load.

Phase 6 is a hardening + sign-off phase. It enforces TLS 1.3 at the edge, migrates auth to asymmetric keys with rotation, wires autoscaling to `websocket_connections` and `entity_count`, then validates the platform against the full benchmark, chaos, capacity, and security checklists. A freeze on new features applies for the duration — this phase changes behavior only to fix what the validation exposes.

Exit criteria: every ADR-020 target met, every [ADR-011](../../adr/011-failure-recovery.md) failure mode recovered within budget, p95 < 100ms, 10K connections per Gateway, 5K entities per Game Server, and a signed-off security audit.

## Scope

- TLS 1.3 termination at Gateway (WSS) + cert-manager integration for K3s (Let's Encrypt or internal CA)
- Optional internal mTLS between Gateway ↔ Room Service ↔ Game Server gRPC
- JWT migration: HMAC → EdDSA (Ed25519); JWKS endpoint for key rotation; Gateway fetches public keys every 5 min
- Autoscaling: HPA with custom metrics (`websocket_connections`, `entity_count`); Compose scale hook for dev
- Benchmark suite ([ADR-020](../../adr/020-benchmark-strategy.md)): light, medium, heavy, burst, zone-transfer, stability (24h soak)
- p95 < 100ms round-trip validation (client → server → client); pprof CPU/heap flame graphs
- Chaos testing for every [ADR-011](../../adr/011-failure-recovery.md) failure mode: Game Server crash mid-tick, Room Service leader failover, network partition, Redis loss, PostgreSQL pool exhaustion
- Capacity validation: 10K concurrent WebSocket connections per Gateway, 5K entities per Game Server ([ADR-017](../../adr/017-capacity-planning.md))
- Security audit checklist: OWASP Top 10, packet fuzzing, JWT tampering, rate-limit bypass

**Out of scope:**
- New product features (feature freeze for hardening)
- Predictive/ML autoscaling ([ADR-007](../../adr/007-autoscaling.md) future work)
- Multi-region disaster recovery

## Architecture

```
                 Client  (WSS :443 — TLS 1.3 required, [ADR-018])
                              │
                  ┌───────────▼────────────┐
                  │ Traefik Ingress        │ ◄── cert-manager ClusterIssuer
                  │ cert: gateway-tls      │     (infra/k3s/issuer.yaml)
                  └───────────┬────────────┘
                              │  (mTLS optional, internal gRPC)
        ┌─────────────────────▼─────────────────────┐
        │  Gateway                                 │
        │  JWKS verifier — internal/gateway/jwks.go        │  fetches public keys every 5 min
        │  EdDSA verify — internal/gateway/jwt.go          │  rejects HMAC tokens
        └────┬──────────────────────────┬──────────┘
        mTLS │ gRPC                mTLS  │ gRPC
        ┌────▼──────────┐         ┌──────▼───────┐
        │ Room Service  │         │  Game Server │
        │ (Lease leader)│         │  (N replicas)│
        └────┬──────────┘         └──────┬───────┘
             │                           │
        ┌────▼─────────┐          ┌──────▼───────┐
        │ PostgreSQL   │          │    Redis     │
        │ RDS (TLS)    │          │  ElastiCache │
        └──────────────┘          └──────────────┘

   Autoscaling — infra/k3s/prometheus-adapter.yaml:
        HPA(Gateway)     ← websocket_connections  ([ADR-007](../../adr/007-autoscaling.md), [ADR-017](../../adr/017-capacity-planning.md))
        HPA(Game Server) ← entity_count

   Validation harness — benchmarks/ + tools/chaos/:
   ┌────────────────────────────────────────────────────┐
   │ Simulation framework → light/medium/heavy/burst/   │
   │   zone-transfer/stability(24h)  ([ADR-020](../../adr/020-benchmark-strategy.md))      │
   │ pprof flame graphs (CPU/heap)                      │
   │ Chaos → every [ADR-011](../../adr/011-failure-recovery.md) failure mode             │
   │ Sign-off: 10K conn, 5K entities, p95 < 100ms       │
   └────────────────────────────────────────────────────┘
```

## Components

### 1. TLS 1.3 + cert-manager at Gateway

Enforces [ADR-018](../../adr/018-security.md): external TLS 1.3 required (WSS), certs via cert-manager or LB termination.

| File | Action | Responsibility |
|------|--------|----------------|
| `infra/k3s/issuer.yaml` | Create | cert-manager `ClusterIssuer` — Let's Encrypt (staging+prod) or internal CA; selectable via value |
| `infra/helm/gateway/templates/certificate.yaml` | Create | `Certificate` resource for the Gateway hostname |
| `infra/helm/gateway/templates/ingress.yaml` | Modify | Wire TLS ref + HSTS header; Phase 5 placeholder → real cert |
| `infra/helm/gateway/values.yaml` | Modify | `tls.enabled`, `tls.hostName`, `tls.issuer` |
| `infra/k3s/cert-manager.yaml` | Create | cert-manager install manifest (or note: install via Helm) |

Min TLS version pinned to 1.3 at Ingress; legacy TLS disabled. cert-manager auto-renews before expiry.

### 2. Optional internal mTLS

[ADR-018](../../adr/018-security.md) treats internal mTLS as deferred/future; Phase 6 makes it **optional** (toggle in chart values). When enabled:

| File | Action | Responsibility |
|------|--------|----------------|
| `pkg/mtls/mtls.go` | Create | Build `tls.Config` for gRPC server (require+verify client cert) and client (present cert); load cert/key from mounted files |
| `infra/k3s/mtls-ca.yaml` | Create | Internal CA `Certificate` (cert-manager `SelfSigned` → `ClusterCA`) issuing per-service client+server certs |
| `infra/helm/{gateway,room-service,game-server}/templates/` | Modify | Mount certs, set `SPATIAL_GRPC__TLS_ENABLED`, gate behind `mtls.enabled` |

Default `mtls.enabled: false` to preserve private-network trust model ([ADR-018](../../adr/018-security.md)); enable for regulated deployments.

### 3. JWT migration — HMAC → EdDSA + JWKS

Replaces the current shared HMAC secret (`SPATIAL_GATEWAY__JWT_SECRET`) with asymmetric Ed25519 verification per [ADR-018](../../adr/018-security.md) ("validate using Business Backend's public key").

| File | Action | Responsibility |
|------|--------|----------------|
| `internal/gateway/jwt.go` | Modify | Add `EdDSA` (Ed25519) verification path; reject HMAC tokens; keep HMAC behind a deprecated flag for migration window |
| `internal/gateway/jwks.go` | Create | JWKS fetcher: GET `<issuer>/.well-known/jwks.json`, cache keys by `kid`, background refresh every 5 min, fail-closed if refresh fails past TTL |
| `apps/gateway/main.go` | Modify | Wire `JWKSProvider` into auth middleware instead of static secret |
| `configs/gateway.yml` | Modify | `jwt.jwks_url`, `jwt.refresh_interval: 5m`, `jwt.issuer` (deprecate `jwt.secret`) |

Migration path: deploy with both verifiers, cut Business Backend over to Ed25519 signing, then remove HMAC. Key rotation = Business Backend publishes a new `kid`; Gateway picks it up within 5 min — no redeploy.

### 4. Autoscaling (HPA custom metrics + Compose hook)

Wires [ADR-007](../../adr/007-autoscaling.md) production path (K3s HPA + Prometheus adapter) to spatial metrics, with thresholds from [ADR-017](../../adr/017-capacity-planning.md).

| File | Action | Responsibility |
|------|--------|----------------|
| `infra/k3s/prometheus-adapter.yaml` | Create | Map `gateway_connections_active` → custom metric `websocket_connections`; `game_server_entities_total` → `entity_count` |
| `infra/helm/gateway/templates/hpa.yaml` | Modify | Scale on `websocket_connections` > 8000 (80% of 10K, [ADR-017](../../adr/017-capacity-planning.md)) |
| `infra/helm/game-server/templates/hpa.yaml` | Modify | Scale on `entity_count` > 4000 + CPU > 70% |
| `deploy/docker-compose/docker-compose.yml` | Modify | Document `docker compose up --scale game-server=N` as the dev scale hook ([ADR-008](../../adr/008-deployment.md)) |
| `scripts/dev-scale.sh` | Create | Helper: scale game-server in compose and verify room-service rebalances |

### 5. Benchmark suite ([ADR-020](../../adr/020-benchmark-strategy.md))

Standalone Go simulation framework under `benchmarks/` (separate from production code per [ADR-020](../../adr/020-benchmark-strategy.md)): virtual WebSocket clients, movement patterns, AOI verification, per-RPC histograms, automated reports to `benchmarks/reports/`.

| Scenario | Clients | Entities | Zones | Duration | Target |
|----------|---------|----------|-------|----------|--------|
| Light | 100 | 1,000 | 1 | 5 min | AOI correctness |
| Medium | 1,000 | 5,000 | 10 | 10 min | p95 < 100ms |
| Heavy | 5,000 | 10,000 | 50 | 10 min | No dropped connections, 5K entities/Game Server |
| Burst | 500 (connect in 1s) | — | — | 2 min | Connection-rate handling |
| Zone transfer | (moving across zones) | — | many | 10 min | State sync time < 1s ([ADR-002](../../adr/002-zone-migration.md)) |
| Stability | 2,000 | — | — | 24 h soak | No memory leak |

> These extend the [ADR-020](../../adr/020-benchmark-strategy.md) baseline (stability scaled to 24h, burst tightened to 500-in-1s, heavy to 10K entities) per its "Future Considerations" (continuous soak, chaos-injected load).

| File | Action |
|------|--------|
| `benchmarks/framework/client.go` | Create — virtual WebSocket client, movement patterns, latency capture |
| `benchmarks/framework/report.go` | Create — histogram (p50/p95/p99/p99.9), pass/fail, comparison to prior run |
| `benchmarks/scenarios/{light,medium,heavy,burst,zone_transfer,stability}.go` | Create — one runner per scenario |
| `benchmarks/main.go` | Create — CLI: `benchmarks -scenario=medium -addr=...` |

CI gate: light scenario on every PR (per [ADR-020](../../adr/020-benchmark-strategy.md)); full suite pre-release.

### 6. p95 < 100ms validation + pprof

Round-trip = client sends timestamped packet → Gateway → Game Server → Gateway → client. Measured client-side by the framework; target p95 < 100ms e2e, < 5ms p99 internal RPC, < 50ms p99 tick ([ADR-020](../../adr/020-benchmark-strategy.md)).

| File | Action | Responsibility |
|------|--------|----------------|
| `benchmarks/framework/latency.go` | Create | Client↔server timestamp echo, p95 gate |
| `Makefile` | Modify | `pprof-cpu` / `pprof-heap` targets: enable `net/http/pprof` on Game Server debug port, capture flame graphs |
| `apps/game-server/main.go` | Modify | Attach pprof to debug listener (off-by-default in prod, gated by flag) |

pprof flame graphs + heap snapshots are attached to each benchmark report in `benchmarks/reports/`.

### 7. Chaos testing — every [ADR-011](../../adr/011-failure-recovery.md) failure mode

Each failure mode has a test asserting the recovery contract from [ADR-011](../../adr/011-failure-recovery.md):

| Failure | Test | Expected recovery |
|---------|------|-------------------|
| Game Server crash mid-tick | `tools/chaos/game-server-crash.go` | Heartbeat timeout 15s → zones ORPHAN → reassigned → state from PostgreSQL (≤5s loss) |
| Room Service leader failover | `tools/chaos/leader-failover.go` | Kill leader pod → follower acquires Lease → reads ownership from PostgreSQL within seconds |
| Network partition between zones | `tools/chaos/network-partition.go` | Split-brain prevented via PostgreSQL advisory lock; stale owner surrenders zone within ~15s |
| Redis connection loss | `tools/chaos/redis-loss.go` | Graceful degrade to PostgreSQL; session lookups slower, no data loss |
| PostgreSQL pool exhaustion | `tools/chaos/pg-pool.go` | Writes queued in bounded buffer; new zone transfers blocked; no crash |

| File | Action |
|------|--------|
| `tools/chaos/{game-server-crash,leader-failover,network-partition,redis-loss,pg-pool}.go` | Create — fault injectors (kubectl exec / network-emulation / chaos-mesh) |
| `test/chaos/chaos_test.go` | Create — orchestrates injectors + assertions via the benchmark framework |
| `docs/ops/runbooks/chaos.md` | Create — how to run, expected pass criteria |

### 8. Capacity validation ([ADR-017](../../adr/017-capacity-planning.md))

Two gate scenarios proving the ADR-017 targets:

- **Gateway**: 10,000 concurrent WebSocket connections on a single instance — verified by the heavy/burst scenarios against one Gateway replica. Monitors file-descriptor/goroutine limits and per-connection memory (~50 KB).
- **Game Server**: 5,000 entities (100/zone × 50 zones) on a single instance — verified by a dedicated capacity scenario; asserts tick stays < 50ms p99 and memory ~5 KB/entity.

Node baseline assumption: 2 vCPU / 4 GB / 100 Mbps ([ADR-017](../../adr/017-capacity-planning.md)). Results recorded in `benchmarks/reports/capacity-signoff.md` and used to revise thresholds if targets miss.

### 9. Security audit checklist

| File | Action | Responsibility |
|------|--------|----------------|
| `docs/ops/security-audit-checklist.md` | Create | OWASP Top 10 review, dependency scan (`govulncheck`), secret scan |
| `test/fuzz/packet_fuzz_test.go` | Create | `go test -fuzz` on packet decoder ([ADR-010](../../adr/010-packet-protocol.md)); malformed/oversized packets silently dropped, no panic |
| `test/security/jwt_tamper_test.go` | Create | Forged/expired/wrong-key EdDSA tokens rejected; replay (old sequence) rejected ([ADR-018](../../adr/018-security.md)) |
| `test/security/rate_limit_bypass_test.go` | Create | 100 msg/s per-conn and 500 msg/s per-IP enforced; bypass attempts terminated after 3 violations |

Audit checklist items: TLS 1.3 enforced (no downgrade), no internal port public except Gateway WSS + Room Service mgmt, Secrets not in Git, JWKS rotation works, rate limits hold under burst, packet fuzz finds no crashes.

## Deployment / Validation Flow

```
1. Phase 5 infra already live (K3s, Helm, monitoring)
2. cert-manager install + issuer + gateway certificate  → TLS 1.3 active
3. (optional) mtls.enabled=true + internal CA            → mTLS on gRPC
4. Deploy JWKS-enabled Gateway + cut Business Backend    → EdDSA tokens
5. Apply prometheus-adapter + HPA custom metrics          → autoscaling live
6. Run benchmark suite: light → medium → heavy → burst   → capture p95/pprof
7. Run zone-transfer + 24h stability                      → memory/sync gates
8. Run chaos suite (5 ADR-011 modes)                      → recovery contracts
9. Capacity sign-off: 10K conn / 5K entities              → fill reports
10. Security audit + fuzz + pentest checklist             → sign-off doc
```

Exit gate: all green → production sign-off recorded in `docs/ops/production-signoff.md`.

## Files Changed

| File / Directory | Action |
|------------------|--------|
| `infra/k3s/{issuer,cert-manager,mtls-ca,prometheus-adapter}.yaml` | Create |
| `infra/helm/gateway/templates/{certificate,ingress}.yaml` | Modify (wire TLS) |
| `infra/helm/{gateway,game-server}/templates/hpa.yaml` | Modify (custom metrics) |
| `infra/helm/{gateway,room-service,game-server}/templates/*` | Modify (mTLS mount, gated) |
| `pkg/mtls/mtls.go` | Create |
| `internal/gateway/jwt.go` | Modify (EdDSA) |
| `internal/gateway/jwks.go` | Create |
| `apps/{gateway,game-server}/main.go` | Modify (JWKS, pprof) |
| `configs/gateway.yml` | Modify (jwks config) |
| `benchmarks/framework/{client,report,latency}.go` | Create |
| `benchmarks/scenarios/{light,medium,heavy,burst,zone_transfer,stability}.go` | Create |
| `benchmarks/main.go` | Create |
| `tools/chaos/{game-server-crash,leader-failover,network-partition,redis-loss,pg-pool}.go` | Create |
| `test/chaos/chaos_test.go` | Create |
| `test/fuzz/packet_fuzz_test.go` | Create |
| `test/security/{jwt_tamper,rate_limit_bypass}_test.go` | Create |
| `scripts/dev-scale.sh` | Create |
| `docs/ops/{security-audit-checklist,production-signoff}.md` | Create |
| `docs/ops/runbooks/chaos.md` | Create |
| `Makefile` | Modify (`benchmark`, `chaos`, `pprof-cpu`, `pprof-heap` targets) |

## References

- [ADR-007 Autoscaling](../../adr/007-autoscaling.md) — HPA + Prometheus adapter, scale flows
- [ADR-008 Deployment](../../adr/008-deployment.md) — K3s, Compose scale hook
- [ADR-010 Packet Protocol](../../adr/010-packet-protocol.md) — packet schema for fuzz targets
- [ADR-011 Failure Recovery](../../adr/011-failure-recovery.md) — chaos recovery contracts
- [ADR-017 Capacity Planning](../../adr/017-capacity-planning.md) — 10K/5K targets, thresholds
- [ADR-018 Security](../../adr/018-security.md) — TLS 1.3, JWT public-key validation, rate limits, replay
- [ADR-019 Observability](../../adr/019-observability.md) — metrics feeding HPA + dashboards
- [ADR-020 Benchmark Strategy](../../adr/020-benchmark-strategy.md) — scenarios, targets, CI gate
- [ADR-002 Zone Migration](../../adr/002-zone-migration.md) — zone-transfer latency budget
- [Master Phase Roadmap](./master-phase-roadmap.md)
- [Phase 5 — Infra-as-Code](./phase-5-infra-as-code.md)
