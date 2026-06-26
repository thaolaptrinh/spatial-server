# ADR 013: Platform Boundary

## Status

Approved

## Context

Spatial Server started with a design that included business concepts: users, rooms, products, meetings. This created coupling between the realtime infrastructure and application-specific business logic. Each new application (Showroom, Meeting, Event, Digital Twin) would require changes to Spatial Server's schema and APIs.

The team recognized that Spatial Server should be a **reusable realtime infrastructure platform**, not a business backend. Each application has its own backend for business logic.

## Decision

### Core Principle

Spatial Server is **NOT** a business backend. It is a reusable realtime infrastructure platform.

### Boundary Definition

| Owned by Business Backend | Owned by Spatial Server |
|---|---|
| User accounts, profiles | Runtime instances |
| Organizations, companies | Connected players |
| Products, showrooms, meetings | Entity state (positions, attributes) |
| Room metadata (name, settings) | Zone ownership |
| Access control, permissions | AOI (Area of Interest) |
| Payments, subscriptions | Game Server load balancing |
| REST API, Admin dashboard | Gateway connection routing |

### Terminology

| Old Term (Removed) | New Term |
|---|---|
| Room (business concept) | Runtime (infrastructure concept) |
| Room ID → Runtime ID | A runtime is an instantiated realtime session |
| User (in Spatial Server) | Player (identified by external player_id) |
| User data (in PostgreSQL) | Removed — belongs in Business Backend |

### Data Flow

```
Business Backend                    Spatial Server
─────────────────                   ────────────────
Create Room (business data)         
  │                                  
  ├──→ REST/gRPC: CreateRuntime()    
  │       │                          
  │       ├── Allocate zones         
  │       ├── Assign Game Servers    
  │       ├── Return runtime_id      
  │                                  
  └──→ Client receives runtime_id   
       │                            
       ├──→ WebSocket Connect       
       │    (with runtime token)     
       │                            
       └──→ Gateway validates token 
            → Proxy to Game Server  
```

### Runtime Lifecycle

1. **Business Backend** creates the room/showroom/etc. in its own database.
2. **Business Backend** calls Spatial Server: `CreateRuntime(runtime_id, zone_count)`.
3. **Spatial Server** allocates zones, assigns Game Servers, creates runtime metadata.
4. **Business Backend** returns runtime connection info to client.
5. **Client** connects to Gateway with a runtime token issued by Business Backend.
6. **Gateway** validates the token (JWT signed by Business Backend).
7. **Client** joins the runtime and participates in realtime simulation.
8. **Business Backend** calls `DestroyRuntime(runtime_id)` when the room/showroom ends.

### Authentication

- Spatial Server **never issues JWT**.
- Spatial Server **never manages users**.
- Business Backend issues runtime tokens (JWT containing: `player_id`, `runtime_id`, `exp`).
- Gateway validates token signature using Business Backend's public key.
- If token validation needs to be more dynamic, Gateway calls Business Backend's gRPC `ValidateToken(token)`.

### Schema Changes

- Remove `users` table from Spatial Server.
- Remove `rooms` table (business concept).
- Rename `rooms` → `runtimes` (infrastructure concept).

### API Surface (Spatial Server → Business Backend)

```protobuf
service SpatialServerAPI {
  // Business Backend → Spatial Server
  rpc CreateRuntime(CreateRuntimeRequest) returns (CreateRuntimeResponse);
  rpc DestroyRuntime(DestroyRuntimeRequest) returns (DestroyRuntimeResponse);
  rpc GetRuntimeInfo(GetRuntimeInfoRequest) returns (GetRuntimeInfoResponse);
  rpc GetRuntimeMetrics(GetRuntimeMetricsRequest) returns (GetRuntimeMetricsResponse);
  rpc ListRuntimes(ListRuntimesRequest) returns (ListRuntimesResponse);
}
```

### Business Backend Integration

- Business Backend communicates via gRPC (or REST gateway if preferred).
- Business Backend provides JWT public key for token validation (config).
- Spatial Server is stateless with respect to business logic — no caching of business data.
- Spatial Server can serve multiple Business Backends simultaneously.

## Consequences

- Spatial Server is now **completely reusable** across applications.
- No business logic ever leaks into Spatial Server.
- Business Backend and Spatial Server can evolve independently.
- Database schema is minimal (only infrastructure metadata).
- New business applications don't require Spatial Server changes.
- Engineering rule: "Is this business logic or realtime infrastructure?" must be asked for every new feature.

## Replaces

- Previous design included `users`, `rooms`, `products` tables in Spatial Server schema.
- Previous Gateway design included user authentication (now just token validation).
- Previous Room Service included room CRUD (now just runtime lifecycle).
