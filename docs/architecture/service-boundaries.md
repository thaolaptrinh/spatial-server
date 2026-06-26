# Service Boundaries

> **Last Updated:** 2026-06-26

## Purpose

Define the clear boundary between what Spatial Server owns versus what belongs to external systems (especially the Business Backend), and the boundaries between each internal service.

## Platform Boundary: Spatial Server vs Business Backend

This is the most important boundary in the system. Violating it is the primary architectural risk.

### Spatial Server Owns (Realtime Infrastructure)

| Domain | Details |
|--------|---------|
| Runtime lifecycle | Creating, managing, destroying realtime sessions |
| Zone ownership | Which Game Server owns which zone grid cell |
| Entity simulation | Position, attributes, movement, physics (basic) |
| AOI queries | What each player can see and interact with |
| Player presence | Connect/disconnect, session management |
| Real-time state replication | Broadcasting state changes to interested clients |
| Gateway routing | Client → Game Server request forwarding |
| Reconnection | Session tokens, state recovery after disconnect |

### Business Backend Owns (Business Logic)

| Domain | Details |
|--------|---------|
| User management | Registration, profiles, authentication |
| Authorization | Permission checks, access control |
| Room/showroom/meeting metadata | Name, settings, configuration |
| JWT token issuance | Runtime tokens for client authentication |
| REST/Admin API | Business-facing APIs |
| Payments, subscriptions | Billing and commerce |
| Analytics dashboards | Business metrics and reporting |

### The Boundary Rule

> If the feature relates to realtime spatial infrastructure, it belongs in Spatial Server.
> If the feature relates to business logic, user data, or application-specific rules, it belongs in the Business Backend.

**Spatial Server never:**
- Stores user profiles, email addresses, or PII
- Makes authorization decisions (beyond JWT validation)
- Contains business-specific game logic (scoring, matchmaking, inventory)
- Calls the Business Backend during gameplay
- Issues JWT tokens or manages authentication flows

## Internal Service Boundaries

### Gateway

| Boundary | Includes | Excludes |
|----------|----------|----------|
| Network | Public (WSS) + Private (gRPC) | Database network |
| State | In-memory connection state, cached routing table | Persistent state, zone ownership |
| Data seen | Binary protobuf packets (opaque), JWT claims | Parsed game state, entity data |
| Failure scope | Losing gateway drops its client connections | No data loss, clients reconnect |

### Room Service

| Boundary | Includes | Excludes |
|----------|----------|----------|
| Network | Private (gRPC) + Database (PostgreSQL) | Public network, WSS |
| State | Zone ownership table, Game Server registry | Entity state, AOI index, player sessions |
| Data seen | Metadata (zone_id, server_id, runtime_id) | Game state payloads, client packets |
| Authority | Zone assignment, load balancing decisions | Game simulation, entity lifecycle |
| Failure scope | Brief outage (cache covers lookups) | Gameplay continues, new connections fail |

### Game Server

| Boundary | Includes | Excludes |
|----------|----------|----------|
| Network | Private (gRPC) + Database (PostgreSQL) | Public network, client WSS directly |
| State | In-memory entity + AOI (primary), PostgreSQL (persistence) | Business metadata, user profiles |
| Data seen | Full entity state, position, attributes | JWT contents (beyond validation), business data |
| Authority | Entity simulation, AOI queries | Zone ownership decisions |
| Failure scope | Zone state loss (≤5s), players disconnected | Zones reassigned, state recovered from DB |

## Inter-Service Dependency Direction

```
Business Backend ──→ Room Service (gRPC)
                          │
Gateway ──→ Room Service (gRPC, lookup)
Gateway ──→ Game Server (gRPC, proxy)

Room Service ──→ Game Server (gRPC, control plane)
Game Server ←──→ Game Server (gRPC, P2P data plane)

All Services ──→ PostgreSQL (pgx)
Gateway, RS, GS ──→ Redis (go-redis)
```

### Prohibited Dependencies

| Dependency | Why Forbidden |
|------------|---------------|
| Gateway → PostgreSQL | Violates stateless design. Gateway must be horizontally scalable without connection pool limits. |
| Game Server → Business Backend | Creates hot-path dependency on external system. JWT pre-validation removes need. |
| Business Backend → Game Server (direct) | All communication goes through Room Service. Game Servers are transient and addressable only via Room Service. |
| Redis → realtime data path | Redis pub/sub lacks delivery guarantees. AOI and entity state are in-memory only. |

## References

- [ADR-013](../adr/013-platform-boundary.md) — Platform Boundary
- [Architecture Overview](overview.md)
- [System Context](system-context.md)
- [Component Responsibilities](component-responsibilities.md)
- [Communication Patterns](communication.md)
