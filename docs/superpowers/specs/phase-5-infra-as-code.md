# Phase 5 — Infra-as-Code

> **Last Updated:** 2026-06-27
> **Note:** The `k3s/ingress.yaml` manifest uses `kubernetes.io/ingress.class: traefik`. K3s bundles Traefik by default — do NOT disable it in cloud-init. If an alternative ingress controller is preferred (e.g., nginx-ingress), disable Traefik via `disable: [traefik]` in cloud-init AND update ingressClassName accordingly.
> **Status:** Draft

## Purpose

Phases 1–4 delivered a functionally complete platform — multi-server zone distribution, session resumption, and backpressure — but it ships only via `deploy/docker-compose/docker-compose.yml`. There is no reproducible path from source to a production cluster, no observability stack, and no distributed tracing across the gRPC path.

Phase 5 closes these gaps. After this phase, `terraform apply` provisions the cloud, `cloud-init` bootstraps K3s nodes, `helm install` deploys every service plus a full monitoring stack, and a CI pipeline builds images and packages charts. Operators gain Prometheus metrics, Loki logs, Grafana dashboards, Alertmanager rules, and OpenTelemetry traces spanning Gateway → Room Service → Game Server.

All infrastructure decisions follow [ADR-014](../../adr/014-infrastructure-platform.md) (platform stack + `infra/` layout) and [ADR-008](../../adr/008-deployment.md) (K3s target).

## Scope

- `infra/` directory per [ADR-014](../../adr/014-infrastructure-platform.md):
  - `infra/terraform/` — cloud-specific providers under `providers/<cloud>/` ([ADR-024](../../adr/024-multicloud-provider-abstraction.md)), networking, K3s cluster, node pools, Cloudflare DNS; HCP Terraform remote state (Postgres/Redis run in-cluster, see §4)
  - `infra/cloud-init/` — K3s server + agent node bootstrap scripts
  - `infra/helm/` — charts: `gateway`, `room-service`, `game-server`, `postgres`, `redis`, `monitoring`
  - `infra/k3s/` — cluster manifests: Namespace, Ingress (TLS-ready), HPA, PDB, ConfigMaps, Secrets
- `deploy/docker-compose/docker-compose.staging.yml` — extends base compose with Prometheus + Grafana + Loki + Promtail
- OpenTelemetry Collector with distributed tracing across gRPC (Gateway ↔ Room Service ↔ Game Server); trace context propagation via gRPC metadata
- Grafana dashboards: realtime connections, packets/sec, entity count, tick duration, gRPC latency p50/p95/p99, zone migration time
- Alertmanager rules: connection drop rate, heartbeat failures, entity count spike, tick duration > 100ms
- CI/CD: Docker image build + push to registry, Helm chart packaging (GitHub Actions per [ADR-014](../../adr/014-infrastructure-platform.md))

**Out of scope (deferred to [Phase 6](./phase-6-production-hardening.md)):**
- TLS certificate issuance / cert-manager wiring (Ingress is TLS-ready but certs land in Phase 6)
- Internal mTLS between services
- JWT migration to asymmetric keys + JWKS
- Autoscaling threshold tuning and custom-metric HPA
- Chaos testing and capacity sign-off

## Architecture

```
                          Internet
                              │
                       ┌──────▼───────┐
                        │ DNS Cloudflare│ ◄── infra/terraform/modules/dns
                       └──────┬───────┘
                              │ WSS :443 (cert in Phase 6)
            ┌─────────────────▼──────────────────┐
            │  K3s Cluster  (infra/terraform/      │
            │                modules/k3s + node_pool)│
            │                                        │
            │   ┌─────────────────────────────────┐ │
            │   │ Ingress (Traefik, infra/k3s/)    │ │
            │   └────────────────┬─────────────────┘ │
            │                    │ LB                 │
            │   ┌────────────────▼─────────────────┐ │
            │   │  Gateway   (infra/helm/gateway)   │ │  stateless, HPA, PDB
            │   └────┬──────────────────────┬──────┘ │
            │   gRPC │                      │ gRPC   │
            │   ┌────▼──────────┐   ┌───────▼──────┐ │
            │   │ Room Service  │   │  Game Server │ │  N replicas (stateful)
            │   │ 2 replicas +  │   │  (N, HPA)    │ │
            │   │ Lease leader  │   │              │ │
            │   └──────┬────────┘   └──────┬───────┘ │
            └──────────┼───────────────────┼─────────┘
                       │                   │
           ┌────────────▼─────┐      ┌──────▼──────────┐
           │ CloudNativePG    │      │ Bitnami Redis    │ ◄── in-cluster (K3s)
           │ Postgres         │      │ (Helm chart)     │    per ADR-024
           └──────────────────┘      └──────────────────┘

   Observability — infra/helm/monitoring:
   ┌───────────────────────────────────────────────────────┐
   │ Prometheus ◄── /metrics (Gateway, Room, Game)         │
   │ Grafana   ──► dashboards (connections, tick, RPC p95) │
   │ Promtail  ──► Loki  (DaemonSet tails container logs)  │
   │ OTel Collector ◄── OTLP/gRPC traces (1% prod sampling)│
   │ Alertmanager ──► Slack / PagerDuty                    │
   └───────────────────────────────────────────────────────┘
```

## Components

### 1. Terraform modules (`infra/terraform/`)

Cloud-agnostic provisioning per [ADR-014](../../adr/014-infrastructure-platform.md): Terraform provisions VMs, networking, DNS, volumes, load balancers — but **never installs applications** (that is K3s/Helm's job).

| Module | Files | Responsibility |
|--------|-------|----------------|
| `providers/<cloud>/` | per-cloud `main.tf`, `variables.tf`, `outputs.tf` | Cloud-specific IaaS (network, servers, load balancer) behind a shared provider contract. **Hetzner is the staging provider**; Sakura Internet / AWS are future candidates ([ADR-024](../../adr/024-multicloud-provider-abstraction.md)) |
| `modules/vpc/` | `main.tf`, `variables.tf`, `outputs.tf` | VPC, public/private subnets, NAT, security groups |
| `modules/k3s/` | `main.tf`, `cloud-init.tf`, `outputs.tf` | K3s server node (control plane), injects server cloud-init token |
| `modules/node_pool/` | `main.tf`, `variables.tf` | K3s agent (worker) pool, autoscaling group, joins cluster via token |
| `modules/dns/` | `main.tf`, `variables.tf` | Cloudflare DNS record → Ingress LB |
| `environments/staging/` | `main.tf`, `variables.tf`, `terraform.tfvars.example`, `backend.tf` | Composes `module "cloud" = providers/hcloud`; remote state in HCP Terraform |

Data services (PostgreSQL, Redis) are **not** Terraform modules — they run in-cluster as CloudNativePG Postgres + Bitnami Redis (see [ADR-024](../../adr/024-multicloud-provider-abstraction.md) and §4).

State is remote (HCP Terraform). `terraform.tfvars.example` documents every variable; secrets never appear — they come from `kubectl`-created Secrets or CI-injected env at apply time.

### 2. cloud-init node bootstrap (`infra/cloud-init/`)

Per [ADR-014](../../adr/014-infrastructure-platform.md): cloud-init installs Docker, K3s, SSH hardening, and cluster join on first boot. SSH is debug-only, never a deploy channel.

| File | Purpose |
|------|---------|
| `infra/cloud-init/k3s-server.yaml` | Installs Docker + K3s server (`curl -sfL https://get.k3s.io \| sh -s - server`), writes kubeconfig, exposes node token to user-data output for agents |
| `infra/cloud-init/k3s-agent.yaml` | Installs Docker + K3s agent, joins cluster using server token + server private IP (templated by `modules/node_pool`) |
| `infra/cloud-init/common.yaml` | Shared: non-root `spatial` user, SSH key-only login, fail2ban, ulimits (`nofile 1048576` for 10K connections per [ADR-017](../../adr/017-capacity-planning.md)) |

Files are consumed by Terraform via `templatefile()` and passed as EC2 `user_data`.

### 3. Helm charts — services (`infra/helm/{gateway,room-service,game-server}`)

[ADR-014](../../adr/014-infrastructure-platform.md) mandates a Helm chart per deployable component. Each service chart follows the same layout:

```
infra/helm/<service>/
├── Chart.yaml
├── values.yaml
└── templates/
    ├── deployment.yaml
    ├── service.yaml
    ├── hpa.yaml            # gateway + game-server only
    ├── pdb.yaml            # minAvailable guarantees
    ├── configmap.yaml      # non-secret config
    ├── secret.yaml         # sealed/templated refs
    ├── servicemonitor.yaml # Prometheus scrape (CRD)
    └── NOTES.txt
```

Chart-specifics:

| Chart | Replicas | Service type | Key resources/limits |
|-------|----------|--------------|----------------------|
| `gateway` | 2+ (HPA) | `LoadBalancer` (WSS :443/:8080) + `ClusterIP` (gRPC :9000) | 512Mi–1Gi, driven by 10K conns ([ADR-017](../../adr/017-capacity-planning.md)) |
| `room-service` | 2 (Lease leader, [ADR-011](../../adr/011-failure-recovery.md)) | `ClusterIP` :9000 | 256Mi–512Mi |
| `game-server` | N (HPA, stateful) | `ClusterIP` :9000, `Headless` for sticky routing | 1Gi–2Gi, 5K entities/server ([ADR-017](../../adr/017-capacity-planning.md)) |

Images use tags `dev-<sha>` (dev) / `v<semver>` (release) per [ADR-008](../../adr/008-deployment.md). Env vars follow the existing `SPATIAL_` + `__` nesting convention (e.g. `SPATIAL_POSTGRES__DSN`, `SPATIAL_REDIS__ADDR`).

### 4. Helm charts — stateful services (`infra/helm/postgres`, `redis`)

Per [ADR-024](../../adr/024-multicloud-provider-abstraction.md), data services run in-cluster rather than as cloud-managed offerings — no RDS/ElastiCache from Terraform. The `postgres` chart installs CloudNativePG (native failover, PITR); the `redis` chart uses the Bitnami chart. For dev/CI parity the charts can fall back to the plain containers matching `deploy/docker-compose/docker-compose.yml` (`postgres:16-alpine`, `redis:7-alpine`). PersistentVolumeClaims use the cluster's CSI StorageClass (`local-path` for dev only).

### 5. Monitoring Helm chart (`infra/helm/monitoring`)

Single chart bundling the [ADR-019](../../adr/019-observability.md) stack:

| Sub-component | Template | Source |
|---------------|----------|--------|
| Prometheus | `templates/prometheus.yaml` (Deployment + ConfigMap scrape config) | Scrapes `/metrics` on every service via `ServiceMonitor` |
| Grafana | `templates/grafana.yaml` | Pre-provisioned datasource (Prometheus + Loki) + dashboard ConfigMaps |
| Promtail | `templates/promtail.yaml` (`DaemonSet`) | Tails container stdout JSON logs ([ADR-019](../../adr/019-observability.md) log schema) |
| Loki | `templates/loki.yaml` | Log aggregation, Prometheus-compatible labels |
| OTel Collector | `templates/otel-collector.yaml` | Receives OTLP/gRPC, exports to backend (Tempo/Jaeger) |
| Alertmanager | `templates/alertmanager.yaml` | Routes to Slack/PagerDuty webhooks (Secret-stored) |
| PrometheusRule | `templates/rules.yaml` | Alert rules (see §9) |

**Dashboards** (JSON in `infra/helm/monitoring/dashboards/`, loaded as ConfigMap): realtime connections, packets/sec, entity count, tick duration, gRPC latency p50/p95/p99, zone migration time — all sourced from the metrics named in [ADR-019](../../adr/019-observability.md) (`gateway_connections_active`, `game_server_tick_duration_ms`, `room_service_rpc_duration_ms`, etc.).

### 6. K3s cluster manifests (`infra/k3s/`)

Cluster-level resources not owned by a service chart:

| File | Purpose |
|------|---------|
| `infra/k3s/namespace.yaml` | `spatial-server` namespace ([ADR-008](../../adr/008-deployment.md)) |
| `infra/k3s/ingress.yaml` | Traefik `Ingress` routing `:443` → Gateway (TLS cert ref is a placeholder, wired in Phase 6) |
| `infra/k3s/lease-rbac.yaml` | RBAC for Room Service K3s `coordination.k8s.io` Lease leader election ([ADR-011](../../adr/011-failure-recovery.md)) |
| `infra/k3s/priority-classes.yaml` | `game-server-critical`, `room-service-high` (evict monitoring before gameplay) |
| `infra/k3s/network-policies.yaml` | Enforce [ADR-018](../../adr/018-security.md) isolation: only Gateway + Room Service mgmt port external; DB/Redis private-only |

HPA/PDB live inside service charts (§3) for cohesion.

### 7. `docker-compose.staging.yml`

Extends `deploy/docker-compose/docker-compose.yml` (Compose `extends:` / `override`) to bring the [ADR-019](../../adr/019-observability.md) stack to staging/CI without a cluster:

```yaml
services:
  prometheus:    # scrapes gateway/room-service/game-server :9000/metrics
  grafana:       # provisioned with same dashboard JSON as the Helm chart
  loki:
  promtail:      # tails the compose container logs
  alertmanager:
```

Kept alongside the existing base compose (where the `Makefile` already references it) rather than under `infra/` so `make dev-up` continues to work unchanged.

### 8. OpenTelemetry distributed tracing

Trace context propagates end-to-end: client request → Gateway → Game Server tick / Room Service lookup. Per [ADR-019](../../adr/019-observability.md): W3C `traceparent` carried in gRPC metadata, OTLP/gRPC exporter to the Collector, 1% production sampling / 100% staging.

| File | Action | Responsibility |
|------|--------|----------------|
| `pkg/observability/otel.go` | Create | Init provider (`otel.SetTracerProvider`), OTLP/gRPC exporter, graceful shutdown |
| `pkg/observability/grpc.go` | Create | gRPC interceptors: `otelgrpc.StatsHandler` on every client + server; `trace_id`/`request_id` into `slog` context |
| `apps/gateway/main.go` | Modify | Register interceptors on Room Service client + Relay stream |
| `apps/room-service/main.go` | Modify | Server interceptors + Game Server client interceptors |
| `apps/game-server/main.go` | Modify | Server interceptors on Relay/AssignZone/etc. |

Every log line carries `trace_id` + `request_id` ([ADR-019](../../adr/019-observability.md) log schema), enabling Grafana jump-from-trace-to-log.

### 9. Alertmanager rules

`infra/helm/monitoring/templates/rules.yaml` (`PrometheusRule` CRD):

| Alert | Condition | Severity |
|-------|-----------|----------|
| ConnectionDropRateHigh | `rate(gateway_connections_total - gateway_connections_active)[5m]` spike > threshold | warning |
| HeartbeatFailures | `rate(game_server_heartbeat_missed_total)[1m]` > 0 → [ADR-011](../../adr/011-failure-recovery.md) 15s timeout | critical |
| EntityCountSpike | `game_server_entities_total` delta > 3× baseline in 60s | warning |
| TickDurationCritical | `game_server_tick_duration_ms` > 100ms for 10 ticks | critical |

These complement the [ADR-019](../../adr/019-observability.md) base rules (`GatewayDown`, `GameServerDown`, `TickOverrun`, `HighLatency`, `ConnectionSaturation`).

### 10. CI/CD pipeline (`.github/workflows/`)

Per [ADR-014](../../adr/014-infrastructure-platform.md) (GitHub Actions):

| Workflow | Trigger | Steps |
|----------|---------|-------|
| `release-images.yml` | tag `v*` / `main` push | Build 3 multi-stage images (reuse `build/docker/*.Dockerfile`), tag `dev-<sha>`/`v<semver>`, push to registry (GHCR), `cosign sign` |
| `helm-package.yml` | tag `v*` | `helm lint` + `helm package` each chart in `infra/helm/`, push to GHCR OCI registry |
| `terraform-plan.yml` | PR touching `infra/terraform/` | `terraform fmt -check`, `terraform validate`, `terraform plan` (comment on PR) |

## Deployment Flow

```
1. terraform init/apply   → network, K3s server + agent nodes, load balancer, Cloudflare DNS
2. cloud-init (first boot)→ Docker + K3s install, agents join via server token
3. helm install postgres  → managed: skip / else postgres chart
4. helm install redis     → managed: skip / else redis chart
5. helm install room-service → 2 replicas, Lease RBAC, DB/Redis DSN from Secret
6. helm install game-server  → N replicas, registers with Room Service on start
7. helm install gateway      → LB service, Ingress, JWT secret from Secret
8. helm install monitoring   → Prometheus/Grafana/Loki/Promtail/OTel/Alertmanager
9. kubectl apply infra/k3s/  → namespace, Ingress, NetworkPolicies, Lease RBAC
10. CI on tag               → build images, push, package + publish Helm charts
```

Validation gate: all service pods `Ready`, Prometheus targets up, OTel collector receiving spans, Grafana dashboards populated, `room-service` Lease leader elected.

## Files Changed

| File / Directory | Action |
|------------------|--------|
| `infra/terraform/providers/<cloud>/*` (+ `providers/shared/cloudinit/*`) | Create |
| `infra/terraform/modules/vpc/*` | Create |
| `infra/terraform/modules/k3s/*` | Create |
| `infra/terraform/modules/node_pool/*` | Create |
| `infra/terraform/modules/dns/*` | Create |
| `infra/terraform/environments/staging/*` | Create |
| `infra/cloud-init/{k3s-server,k3s-agent,common}.yaml` | Create |
| `infra/helm/gateway/{Chart.yaml,values.yaml,templates/*}` | Create |
| `infra/helm/room-service/{Chart.yaml,values.yaml,templates/*}` | Create |
| `infra/helm/game-server/{Chart.yaml,values.yaml,templates/*}` | Create |
| `infra/helm/postgres/{Chart.yaml,values.yaml,templates/*}` | Create |
| `infra/helm/redis/{Chart.yaml,values.yaml,templates/*}` | Create |
| `infra/helm/monitoring/{Chart.yaml,values.yaml,templates/*}` | Create |
| `infra/helm/monitoring/dashboards/*.json` | Create |
| `infra/k3s/{namespace,ingress,lease-rbac,priority-classes,network-policies}.yaml` | Create |
| `deploy/docker-compose/docker-compose.staging.yml` | Create |
| `pkg/observability/{otel,grpc}.go` | Create |
| `apps/{gateway,room-service,game-server}/main.go` | Modify (wire OTel interceptors) |
| `.github/workflows/{release-images,helm-package,terraform-plan}.yml` | Create |
| `Makefile` | Modify (add `helm-lint`, `terraform-*`, `k3s-apply` targets) |

## References

- [ADR-008 Deployment](../../adr/008-deployment.md) — K3s target, namespace, deployment topology
- [ADR-014 Infrastructure Platform](../../adr/014-infrastructure-platform.md) — `infra/` layout, stack, immutability principles
- [ADR-011 Failure Recovery](../../adr/011-failure-recovery.md) — Room Service Lease leader, alert thresholds
- [ADR-017 Capacity Planning](../../adr/017-capacity-planning.md) — resource limits driving chart values
- [ADR-018 Security](../../adr/018-security.md) — NetworkPolicy isolation, Secret management
- [ADR-019 Observability](../../adr/019-observability.md) — metrics, logs, traces, dashboards, alert rules
- [Master Phase Roadmap](./master-phase-roadmap.md)
- [Phase 6 — Production Hardening](./phase-6-production-hardening.md)
