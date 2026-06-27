# System Context Diagram

> **Last Updated:** 2026-06-26

## Purpose

High-level C4-style system context showing Spatial Server as a black-box platform bounded by its external actors (clients, Business Backend, monitoring stack, external services) and the public/private network boundaries between them.

```mermaid
graph TB
    subgraph "External Actors"
        BB[("Business Backend<br/>REST API Server")]
        MON["Monitoring Stack<br/>Prometheus + Grafana + Loki"]
        EXT_SVC["External Services<br/>DNS · Let's Encrypt · OTel"]
    end

    subgraph "Spatial Server"
        subgraph "Public Boundary"
            GW[Gateway<br/>WebSocket :443<br/>JWT Auth · Rate Limiting]
        end
        subgraph "Private Network"
            RS[Room Service<br/>Zone Ownership · HA]
            GS[Game Servers<br/>Entity Simulation · AOI]
        end
        subgraph "Data Stores"
            PG[(PostgreSQL<br/>Operational State)]
            RDS[(Redis<br/>Cache · Pub/Sub)]
        end
    end

    subgraph "Client Types"
        C1[3D Showroom Client<br/>WebSocket]
        C2[Virtual Office Client<br/>WebSocket]
        C3[Digital Twin Client<br/>WebSocket]
        C4[Event Platform Client<br/>WebSocket]
    end

    C1 -->|WSS :443| GW
    C2 -->|WSS :443| GW
    C3 -->|WSS :443| GW
    C4 -->|WSS :443| GW

    BB -->|CreateRuntime / DestroyRuntime<br/>GetRuntimeInfo| RS
    BB -->|Issue JWT Tokens| C1
    BB -->|Issue JWT Tokens| C2
    BB -->|Issue JWT Tokens| C3
    BB -->|Issue JWT Tokens| C4

    GW -->|gRPC :9000| RS
    GW -->|gRPC :9000| GS
    RS -->|gRPC :9000| GS
    GS <-->|gRPC P2P :9000| GS

    GS -->|:5432| PG
    RS -->|:5432| PG
    GS -->|:6379| RDS
    RS -->|:6379| RDS

    GW -->|/metrics :9090| MON
    RS -->|/metrics :9090| MON
    GS -->|/metrics :9090| MON
    PG -.->|logs| MON
    RDS -.->|logs| MON

    MON -->|Alertmanager| EXT_SVC
```

## Description

This C4-style system context diagram shows Spatial Server as a black-box platform bounded by four external actor groups:

- **Clients** — Four distinct client types (3D Showroom, Virtual Office, Digital Twin, Event Platform) all connecting via WebSocket over WSS :443. Each receives a JWT from the Business Backend before connecting.
- **Business Backend** — The authoritative external system that creates/destroys runtimes via Room Service and issues JWT tokens to clients. Spatial Server never contains business logic.
- **Monitoring Stack** — Prometheus collects metrics from all services; Promtail ships logs to Loki; Grafana provides dashboards.
- **External Services** — DNS resolution, Let's Encrypt TLS certificates, OpenTelemetry tracing backend.

Data flows show clear network boundary separation: clients hit the public Gateway, Gateway proxies to the private network, and data stores are accessed only by internal services.

## References

- [Architecture Overview](../architecture/overview.md)
- [ADR-015](../adr/015-architecture-principles.md) — Architecture Principles
- [ADR-013](../adr/013-platform-boundary.md) — Platform Boundary
