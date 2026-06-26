# Helm

> **Last Updated:** 2026-06-26

## Purpose

Package and deploy all Spatial Server components on K3s using Helm charts. Every deployable component must have a Helm chart.

## Chart Organization

All charts live in `infra/helm/`:

```
infra/helm/
├── gateway/           # WebSocket Gateway deployment
├── room-service/      # Room Service deployment
├── game-server/       # Game Server deployment
├── redis/             # Redis StatefulSet
├── postgres/          # PostgreSQL StatefulSet
└── monitoring/        # Prometheus, Grafana, Loki, Promtail
```

### Chart Structure (per service)

```
<chart>/
├── Chart.yaml           # Chart metadata
├── values.yaml          # Default values
├── values-<env>.yaml    # Environment overrides (dev, staging, production)
├── templates/
│   ├── deployment.yaml  # Deployment or StatefulSet
│   ├── service.yaml     # ClusterIP / LoadBalancer Service
│   ├── configmap.yaml   # Application config (from configs/*.yml)
│   ├── secret.yaml      # References external secret (values never inline)
│   ├── hpa.yaml         # HorizontalPodAutoscaler
│   ├── pdb.yaml         # PodDisruptionBudget
│   └── _helpers.tpl     # Named templates
└── charts/              # Subchart dependencies (if any)
```

## Values

| File | Purpose |
|------|---------|
| `values.yaml` | Sensible defaults for local dev |
| `values-staging.yaml` | Staging overrides (smaller resources, single PG replica) |
| `values-production.yaml` | Production overrides (HA, resource limits, replica counts) |

### Key Values

```yaml
replicaCount: 2
image:
  repository: ghcr.io/spatial-server/gateway
  tag: "dev-latest"
  pullPolicy: Always
resources:
  requests:
    cpu: "500m"
    memory: "256Mi"
  limits:
    cpu: "1"
    memory: "512Mi"
config:
  existingConfigMap: "gateway-config"
service:
  type: ClusterIP
  port: 9000
```

## Dependency Management

Application charts (gateway, room-service, game-server) depend on the redis and postgres charts for local dev but point to existing StatefulSets in production:

```yaml
# Chart.yaml
dependencies:
  - name: redis
    version: ">=0.1.0"
    repository: "file://../redis"
    condition: redis.enabled
  - name: postgres
    version: ">=0.1.0"
    repository: "file://../postgres"
    condition: postgres.enabled
```

In production, `redis.enabled` and `postgres.enabled` are set to `false` and external endpoints are provided via ConfigMap.

## Installation

```bash
helm install gateway ./infra/helm/gateway --values ./infra/helm/gateway/values-production.yaml -n spatial-server
```

## References

- ADR-008 — Deployment Strategy
- ADR-014 — Infrastructure Platform
- [k3s.md](k3s.md)
