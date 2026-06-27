# Component Diagram

> **Last Updated:** 2026-06-26

## Purpose

Component-level view of Spatial Server's internal services (Gateway, Room Service, Game Servers) and data stores (PostgreSQL, Redis), showing responsibilities and the gRPC/read-write edges between them.

```mermaid
graph TB
    subgraph "Public Network"
        CLIENTS[Clients<br/>WebSocket Clients]
    end

    subgraph "Edge"
        LB[Load Balancer<br/>L4 TCP]
        GW[Gateway<br/>WebSocket Termination<br/>JWT Auth<br/>Rate Limiting]
    end

    subgraph "Coordinator"
        RS[Room Service<br/>Zone Ownership<br/>Load Balancing<br/>Service Discovery]
    end

    subgraph "Game Layer"
        GS1[Game Server 1<br/>Zone A, Zone B<br/>Entity Simulation<br/>AOI Queries]
        GS2[Game Server 2<br/>Zone C, Zone D<br/>Entity Simulation<br/>AOI Queries]
        GSN[Game Server N<br/>Zone E, Zone F<br/>Entity Simulation<br/>AOI Queries]
    end

    subgraph "Data Layer"
        PG[(PostgreSQL<br/>Runtime Metadata<br/>Zone Ownership<br/>Game Server Registry)]
        REDIS[(Redis<br/>Session Cache<br/>Metadata Cache)]
    end

    CLIENTS -->|WSS :443| LB
    LB --> GW
    GW -->|gRPC Lookup| RS
    GW -->|gRPC Proxy| GS1
    GW -->|gRPC Proxy| GS2
    RS -->|gRPC Control| GS1
    RS -->|gRPC Control| GS2
    RS -->|gRPC Control| GSN
    GS1 <-->|gRPC P2P| GS2
    GS1 <-->|gRPC P2P| GSN
    GS2 <-->|gRPC P2P| GSN
    GS1 -->|Read/Write| PG
    GS2 -->|Read/Write| PG
    RS -->|Read/Write| PG
    GW -->|Read| REDIS
    RS -->|Read/Write| REDIS
```

## Component Responsibilities

| Component | Responsibility |
|-----------|---------------|
| **Clients** | Connect via WebSocket. Send position updates, receive entity states. |
| **Load Balancer** | L4 TCP load balancing. Distributes WebSocket connections across Gateway instances. |
| **Gateway** | Terminates WebSocket connections. Validates JWT. Enforces rate limits. Routes packets to correct Game Server. |
| **Room Service** | Maintains zone ownership table. Registers Game Servers. Handles heartbeat monitoring. Reassigns zones on failure. |
| **Game Server** | Runs simulation loop. Manages entities. Processes AOI queries. Handles zone transfers. Replicates state to clients. |
| **PostgreSQL** | Stores runtime metadata, zone ownership records, Game Server registry. Source of truth for operational data. |
| **Redis** | Caches session data and metadata lookups. Pub/sub for non-realtime domain events. |

## References

- [Architecture Overview](../architecture/overview.md)
- [Sequence Diagrams](sequences.md)
