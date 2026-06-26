# Changelog

All notable changes to the Spatial Server documentation and architecture.

## [Unreleased]

### Architecture Freeze v1 — 2026-06-26

#### Added
- Architecture Freeze v1 review completed: 14 criteria validated across ~110 documents.
- Comprehensive terminology audit: zero prohibited World/MMO references found.

#### Fixed
- `docs/infrastructure/overview.md`: Room Service ↔ Game Server gRPC port corrected from :9000 to :9001.
- `docs/infrastructure/networking.md`: Room Service ↔ Game Server gRPC port corrected from :9000 to :9001.
- `docs/operations/deployment.md`: Docker Compose CI command updated from v1 (`docker-compose`) to v2 (`docker compose`).

#### Changed
- `docs/documentation-review-report.md`: Updated with comprehensive Architecture Freeze v1 findings, including diagram consistency audit, navigation gaps, and infrastructure consistency checks. Overall score revised to 85/100 with approved decision.

#### Known Issues
- 3/7 diagram files have RPC direction errors that contradict ADR-009 (runtime-lifecycle, ownership, rpc-flow).
- 3 RPCs used in diagrams are not defined in ADR-009 (AssignZone, ReleaseZone, ReportMetrics, SendEntityUpdate, QueryEntities).
- Legacy `docs/ADR.md` contains conflicting packet format with spurious "Packet Length" field.
- 10/14 docs subdirectories missing README.md files.
- No automated link checker or Mermaid syntax verification in CI.
