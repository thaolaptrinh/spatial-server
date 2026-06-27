# ADR 015: Architecture Principles

## Status

Accepted

## Context

Spatial Server must maintain architectural consistency across all implementation phases. Without documented principles, individual decisions may drift from the original design intent over time.

## Problem

As implementation proceeds across phases and contributors, ad-hoc architectural choices accumulate and diverge from the original design intent, leading to redesigns and inconsistencies that are expensive to correct late.

## Decision

### 1. Production First

Architecture is designed for the final production topology from day one. Implementation phases determine implementation completeness, not architectural changes. No architectural redesign should be required from single-server to hundreds-of-servers deployment.

### 2. Horizontal Scaling First

When scaling is needed, prefer adding instances over increasing CPU/RAM. Every service must be designed such that adding another replica increases total capacity.

### 3. Logical Service Independence

A service's logical definition (what it does) must be independent from its physical deployment (where it runs). Today one VM may host multiple services; tomorrow each service may move to its own node — no code changes required.

### 4. Infrastructure as Code

Everything must be reproducible from source. No manual production deployment. No SSH-based configuration. Terraform provisions infrastructure, cloud-init bootstraps nodes, Helm deploys applications.

### 5. Cloud Agnostic

No business logic may depend on a specific cloud provider. Terraform modules abstract provider details. Supported providers include AWS, GCP, Azure, Hetzner, Vultr, DigitalOcean, and on-premise.

### 6. Clean Separation: Business vs Infrastructure

Spatial Server is a reusable realtime infrastructure platform, not a business backend. All business logic (auth, users, products, meetings) belongs in external Business Backends. A feature must pass the test: "Is this realtime infrastructure or business logic?" If business logic → implement in Business Backend.

### 7. Single Source of Truth

Architecture decisions are documented in ADRs. The ADR set (`docs/adr/`) is the authoritative reference for architectural decisions. Implementation plans derive from the design, never the reverse.

### 8. Benchmark Driven

Performance targets are defined before optimization. Load tests and benchmarks validate every phase before production deployment. No performance tuning without measurement.

## Alternatives

1. **Implicit, undocumented principles**: Rely on tribal knowledge to keep decisions consistent. Rejected because it guarantees drift as the team and codebase grow.
2. **Heavyweight architecture review board**: Gate every change through a formal review process. Rejected as disproportionate overhead for the current team size; documented principles provide guidance without a bureaucratic gate.

## Tradeoffs

- Documented principles constrain implementation phases (shortcuts that violate them are disallowed) but prevent costly redesigns and keep the system horizontally scalable from single-server to hundreds of servers.
- Principles can become stale if not revisited; they must evolve alongside deliberate ADR updates.

## Consequences

- Every engineer can reference these principles to resolve design disputes.
- New features can be quickly classified: belongs in Spatial Server vs. belongs in Business Backend.
- Architecture drift is prevented by referring to documented principles.
- Implementation phases are constrained by architecture, not the other way around.

## Future Considerations

- Periodic review of these principles as the platform and team scale.
- Expanding the set with dedicated security and performance principles as those areas mature.

## Replaces

- None. These principles were previously implicit and undocumented.
