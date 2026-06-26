# Architecture Principles

> **Last Updated:** 2026-06-26

## Purpose

Eight fundamental principles that govern all architectural decisions in Spatial Server. Every design decision must be consistent with these principles.

## Principles

### 1. Production First

Architecture is designed for the final production topology from day one. Implementation phases determine implementation completeness, not architectural changes. No architectural redesign should be required from single-server to hundreds-of-servers deployment. Development phases scope what gets built, not how it's architected.

### 2. Horizontal Scaling First

When scaling is needed, prefer adding instances over increasing CPU/RAM. Every service must be designed such that adding another replica increases total capacity. Stateless services (Gateway) are trivially scalable; stateful services (Game Server) require explicit partition mechanisms (zone ownership).

### 3. Logical Service Independence

A service's logical definition (what it does) must be independent from its physical deployment (where it runs). Today one VM may host multiple services; tomorrow each service may move to its own node — no code changes required. This is enforced by clean interfaces (gRPC protobuf) and no shared in-memory state between services.

### 4. Infrastructure as Code

Everything must be reproducible from source. No manual production deployment. No SSH-based configuration. The infrastructure pipeline is:

```
Terraform → cloud-init → Docker → K3s → Helm → Services
```

- Terraform provisions VMs, networking, DNS, volumes, load balancers (never installs apps)
- cloud-init bootstraps Docker, K3s, and cluster join
- Helm charts deploy every component (gateway, room-service, game-server, redis, postgres, monitoring)

### 5. Cloud Agnostic

No business logic may depend on a specific cloud provider. Terraform modules abstract provider details. Supported providers include AWS, GCP, Azure, Hetzner, Vultr, DigitalOcean, and on-premise. Provider-specific features (e.g., DynamoDB, SQS) must never be required — use portable alternatives (PostgreSQL, Redis pub/sub).

### 6. Clean Separation: Business vs Infrastructure

Spatial Server is a reusable realtime infrastructure platform, not a business backend. All business logic (auth, users, products, meetings) belongs in external Business Backends. Every new feature must pass the test: *"Is this realtime infrastructure or business logic?"* If business logic → implement in Business Backend.

| Belongs in Business Backend | Belongs in Spatial Server |
|---|---|
| User accounts, profiles | Runtime instances |
| Organizations, products, meetings | Connected players |
| Room metadata (name, settings) | Entity state (positions, attributes) |
| Access control, permissions | Zone ownership |
| Payments, subscriptions | AOI and entity replication |
| REST API, Admin dashboard | Gateway connection routing |

See ADR-013 for the full platform boundary definition.

### 7. Single Source of Truth

Architecture decisions are documented in ADRs. The `docs/adr/` directory is the authoritative reference. Implementation plans derive from the design, never the reverse. When a design dispute arises, the ADR is the final arbiter.

### 8. Benchmark Driven

Performance targets are defined before optimization. Load tests and benchmarks validate every phase before production deployment. No performance tuning without measurement. Key benchmarks include:

- Latency histograms (p50, p95, p99, p99.9) per RPC type
- Connection rate and capacity per Gateway instance
- AOI query performance at entity density limits
- Zone transfer duration at various state sizes
- Memory profiling per entity

## Rationale

These principles exist to prevent architectural drift. Without documented principles, individual decisions may incrementally diverge from the original design intent. They serve as a decision-making framework for engineers evaluating trade-offs.

## References

- [ADR-015](../adr/015-architecture-principles.md) — Architecture Principles (source ADR)
- [ADR-013](../adr/013-platform-boundary.md) — Platform Boundary
- [Overview](overview.md) — Architecture overview
