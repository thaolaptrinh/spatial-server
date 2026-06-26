# K3s

> **Last Updated:** 2026-06-26

## Purpose

Run Spatial Server services in production using K3s, a lightweight certified Kubernetes distribution. K3s is the sole production container orchestrator.

## Node Requirements

| Role | Spec | Services |
|------|------|----------|
| Server (control plane) | 2 vCPU, 4 GB RAM | etcd, API server, scheduler, controller manager |
| Worker | 4 vCPU, 8 GB RAM | Gateway, Room Service, Game Server, PostgreSQL, Redis |

All nodes run Ubuntu 24.04 LTS with Docker as the container runtime. Nodes are provisioned via Terraform + cloud-init (see [terraform.md](terraform.md) and [cloud-init.md](cloud-init.md)).

## Installation

K3s is installed by cloud-init on first boot:

```bash
curl -sfL https://get.k3s.io | sh -s - server --cluster-init  # server nodes
curl -sfL https://get.k3s.io | K3S_URL=https://<server>:6443 K3S_TOKEN=<token> sh -  # worker nodes
```

The K3s token is generated on the server node and stored in `/var/lib/rancher/k3s/server/node-token`.

## Cluster Configuration

- **CNI:** Flannel (default, K3s-bundled)
- **Ingress Controller:** Traefik (default, K3s-bundled) for WebSocket TLS termination
- **Service Load Balancer:** Klipper (default, K3s-bundled) for LoadBalancer-type Services
- **Storage Class:** Local Path Provisioner (default) for PostgreSQL and Redis PVCs
- **TLS:** cert-manager + Let's Encrypt for Ingress TLS certificates

## Namespaces

| Namespace | Purpose |
|-----------|---------|
| `spatial-server` | All Spatial Server workloads |
| `monitoring` | Prometheus, Grafana, Loki, Promtail |
| `cert-manager` | TLS certificate management |

## Resource Management

### Resource Requests and Limits

| Service | CPU Request | CPU Limit | Memory Request | Memory Limit |
|---------|-------------|-----------|----------------|--------------|
| Gateway | 500m | 1 | 256 Mi | 512 Mi |
| Room Service | 500m | 1 | 256 Mi | 512 Mi |
| Game Server | 1 | 2 | 512 Mi | 1 Gi |
| PostgreSQL | 1 | 2 | 1 Gi | 2 Gi |
| Redis | 500m | 1 | 512 Mi | 1 Gi |

### Horizontal Pod Autoscaler

| Service | Metric | Threshold | Min Replicas | Max Replicas |
|---------|--------|-----------|-------------|-------------|
| Gateway | CPU | >70% for 30s | 2 | 10 |
| Game Server | CPU | >70% for 30s | 1 | 20 |
| Game Server | Memory | >80% for 30s | 1 | 20 |

## PodDisruptionBudgets

| Service | Min Available | Max Unavailable |
|---------|--------------|-----------------|
| Gateway | 1 | — |
| Room Service | 1 | — |
| Game Server | 1 | — |

PDBs ensure zone ownership is never fully disrupted during voluntary cluster operations (node drains, upgrades).

## Node Affinity and Anti-Affinity

### Game Server Anti-Affinity

Game Server pods must NOT co-locate on the same worker node (fault isolation per ADR-008 / ADR-014):

```yaml
affinity:
  podAntiAffinity:
    preferredDuringSchedulingIgnoredDuringExecution:
    - weight: 100
      podAffinityTerm:
        labelSelector:
          matchLabels:
            app: game-server
        topologyKey: kubernetes.io/hostname
```

### Node Affinity

Game Server pods prefer dedicated game-server nodes when labeled:

```yaml
affinity:
  nodeAffinity:
    preferredDuringSchedulingIgnoredDuringExecution:
    - weight: 50
      preference:
        matchExpressions:
        - key: node-type
          operator: In
          values:
          - game-server
```

## References

- ADR-008 — Deployment Strategy
- ADR-014 — Infrastructure Platform
- [cloud-init.md](cloud-init.md)
- [terraform.md](terraform.md)
- [helm.md](helm.md)
