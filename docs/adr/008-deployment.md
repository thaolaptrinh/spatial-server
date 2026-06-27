# ADR 008: Deployment

## Status

Approved

## Context

The platform must support local development, Docker Compose for staging/CI, and K3s (lightweight Kubernetes) for production.

## Problem

The deployment strategy must support local development, staging/CI, and production without diverging configurations or complex tooling. Each environment has different requirements for simplicity, reproducibility, and scalability.

## Decision

> **Note:** [ADR-014](014-infrastructure-platform.md) supersedes the Helm portion of this decision. Helm charts ARE required per [ADR-014](014-infrastructure-platform.md). The "no Helm for MVP" stance below is superseded.

### Development (Local)

- Single machine, no containerization.
- `go run apps/<service>/main.go` (separate terminals or Makefile targets).
- PostgreSQL and Redis via local install or quick Docker containers.
- Config files in `configs/` with localhost endpoints.

### Docker Compose (Staging + CI)

- Single `docker-compose.yml` with all services.
- `docker-compose.staging.yml` adds monitoring (Prometheus + Grafana).
- Service dependencies: postgres → redis → room-service → gateway, game-server.
- Game Server can scale via `docker-compose up --scale game-server=N`.
- Health checks for all services.

### K3s / Kubernetes (Production)

- Target: K3s.
- Declarative manifests (not Helm charts for MVP).
- Namespace: `spatial-server`.
- Deployments: Gateway (stateless), Room Service (2 replicas, leader via Lease API), Game Server (N replicas, stateful).
- Services: ClusterIP for internal gRPC, LoadBalancer for WebSocket.
- ConfigMaps + Secrets for configuration.
- HPA for Game Server and Gateway.
- Ingress for WebSocket with TLS termination.

### Container Image Strategy

- Multi-stage Dockerfiles.
- Distroless base images (non-root).
- Single binary per service (no dependencies in image).
- Tag: `dev-<sha>` for dev, `v<semver>` for releases.

## Alternatives

1. **Kubernetes-only**: Skip Docker Compose, use Minikube/K3s for local dev. Higher dev environment setup cost and resource usage.
2. **Helm charts from the start**: Package everything in Helm. More complex upfront for a small team; adds learning curve.
3. **PaaS (Fly.io, Railway)**: Deploy without managing infrastructure. Less operational control and potential vendor lock-in.

## Tradeoffs

- Three deployment targets means maintaining three configurations — more work but matches each environment's needs.
- Multi-stage Dockerfiles produce minimal, secure images but add build pipeline complexity.
- No Helm keeps things simple for MVP but may require migration to Helm as the deployment grows.

## Consequences

- Development setup is simple (no Docker overhead).
- Docker Compose and K3s share the same images — easy transition.
- K3s manifests are reference-quality but not Helm-packaged (simpler for small team).

## Future Considerations

- Helm chart for production deployment.
- GitOps workflow (ArgoCD) for K3s deployments.
- Blue-green or rolling deployment strategy for zero-downtime updates.

## Replaces

None.
