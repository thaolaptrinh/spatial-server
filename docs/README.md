# Spatial Server Documentation

> **Last Updated:** 2026-06-26

## Purpose

This directory is the single source of truth for the Spatial Server platform. Every architectural decision, standard, process, and operational procedure is documented here.

## Structure

| Directory | Purpose |
|-----------|---------|
| [architecture/](architecture/) | System architecture, principles, context, service boundaries, communication matrix, runtime model, component interaction, scaling, data model, AOI, deployment topology, repository structure, performance budgets |
| [adr/](adr/) | Architecture Decision Records (001–026) |
| [standards/](standards/) | Coding, naming, dependency rules, error handling, logging, versioning, configuration, metrics, API / gRPC / protobuf conventions |
| [development/](development/) | Local dev setup, contribution guide, branch strategy, release process |
| [testing/](testing/) | Testing strategy, unit/integration/load/simulation/chaos testing, benchmark scenarios |
| [operations/](operations/) | Deployment guide, runbook, backup/restore, disaster recovery, incident response, scaling guide |
| [security/](security/) | Security model, threat analysis, authentication, authorization |
| [api/](api/) | External API contracts (Spatial Server API, Admin API) |
| [protocol/](protocol/) | Wire protocol specification (WebSocket binary, versioning, compression, serialization, heartbeat, reconnect) |
| [deployment/](deployment/) | Deployment configurations and environment summary |
| [infrastructure/](infrastructure/) | Docker Compose, K3s, Terraform, Helm, cloud-init, secrets, networking |
| [roadmap/](roadmap/) | Development phases and timeline |
| [diagrams/](diagrams/) | Mermaid diagrams (system context, components, sequences, deployment, runtime lifecycle, ownership, RPC flow) |

## Quick Links

- [Architecture Overview](architecture/overview.md)
- [Architecture Principles](architecture/principles.md)
- [System Context](architecture/system-context.md)
- [ADR Index](adr/README.md)
- [Coding Standards](standards/coding.md)
- [Testing Strategy](testing/strategy.md)
- [Repository Structure](architecture/repository-structure.md)
- [Service Boundaries](architecture/service-boundaries.md)
- [Communication Matrix](architecture/communication-matrix.md)
- [Runtime Model](architecture/runtime-model.md)
- [Standards Index](standards/README.md)
- [Glossary](glossary.md)
- [Getting Started](../README.md)

## Reading Order

1. **Start here** — Understand the directory structure.
2. **Architecture** → [architecture/](architecture/) — System design, principles, and runtime model.
3. **ADRs** → [adr/](adr/) — Key architectural decisions.
4. **Standards** → [standards/](standards/) — Coding and design conventions.
5. **Infrastructure** → [infrastructure/](infrastructure/) — Deployment infrastructure.
6. **Operations** → [operations/](operations/) — Runbook and operational procedures.
7. **Testing** → [testing/](testing/) — Testing strategy and scenarios.
8. **Protocol** → [protocol/](protocol/) — Wire protocol specification.
9. **API** → [api/](api/) — External API contracts.
10. **Diagrams** → [diagrams/](diagrams/) — Visual architecture diagrams.

## Related Documents

- [Getting Started](../README.md) — Project overview and quick start
- [Glossary](glossary.md) — Terminology reference
- [CHANGELOG](../CHANGELOG.md) — Version history and changes

## Documentation Principles

1. **Single Source of Truth** — Documentation always reflects the current state of the system.
2. **Architecture Before Code** — No implementation without documented architecture.
3. **ADR for Every Decision** — Every architectural decision requires an ADR.
4. **Diagrams as Code** — All diagrams maintained as Mermaid.
5. **Review Before Release** — Documentation review is part of the release process.
