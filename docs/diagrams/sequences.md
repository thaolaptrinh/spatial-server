# Sequence Diagrams

> **Last Updated:** 2026-06-26

## Client Connection Flow

```mermaid
sequenceDiagram
    participant C as Client
    participant BB as Business Backend
    participant GW as Gateway
    participant RS as Room Service
    participant GS as Game Server

    C->>BB: Request room join
    BB->>BB: Create runtime (if not exists)
    BB->>RS: CreateRuntime(runtime_id, zone_count)
    RS->>RS: Allocate zones, assign Game Servers
    RS-->>BB: gateway_addr, runtime info
    BB-->>C: gateway_addr, runtime token (JWT)

    C->>GW: WebSocket Connect (wss://...?token=JWT)
    GW->>GW: Validate JWT signature
    GW->>RS: LookupZone(zone_id)
    RS-->>GW: GameServer address
    GW->>GS: Open gRPC forwarding session
    GS-->>GW: Session established
    GW-->>C: WebSocket Accepted
    Note over C,GS: Bidirectional communication begins
```

## Zone Transfer Flow

```mermaid
sequenceDiagram
    participant RS as Room Service
    participant GS_A as Game Server A (source)
    participant GS_B as Game Server B (target)

    RS->>RS: Detect load imbalance
    RS->>RS: Select zone Z for transfer
    RS->>GS_A: TransferZone(zoneID, targetID)
    GS_A->>GS_A: Pause zone Z (stop AOI updates)
    GS_A->>GS_A: Serialize zone state (entities, positions, AOI)
    GS_A->>GS_B: ZoneStateSync(snapshot) [direct gRPC]
    GS_B->>GS_B: Load snapshot, start simulation
    GS_B-->>GS_A: ZoneStateSyncResponse(success)
    GS_A-->>RS: TransferZoneResponse(success)
    RS->>RS: Update ownership table (PostgreSQL)
    RS->>GW: Push routing update (or Gateway polls)
    GS_A->>GS_A: Release zone Z resources
```

## Heartbeat and Crash Recovery

```mermaid
sequenceDiagram
    participant RS as Room Service
    participant GS1 as Game Server 1
    participant GS2 as Game Server 2
    participant PG as PostgreSQL

    loop Every 5 seconds
        GS1->>RS: Heartbeat(load)
        RS->>PG: UPDATE last_heartbeat
        RS-->>GS1: HeartbeatResponse(ack)
    end

    Note over GS1: Game Server 1 crashes
    rect rgb(255, 200, 200)
        Note over RS: 15 seconds of missed heartbeats
        RS->>RS: Detect stale heartbeat
        RS->>PG: SELECT zones WHERE server_id=GS1
        PG-->>RS: Zone A, Zone B
        RS->>RS: Mark zones as orphan
        RS->>GS2: TransferZone(zone A)
        GS2->>GS2: Load zone from PostgreSQL
        GS2-->>RS: TransferZoneResponse(success)
        RS->>PG: UPDATE zone_ownership SET server_id=GS2
    end
```

## AOI Update Flow

```mermaid
sequenceDiagram
    participant C1 as Client 1
    participant GW as Gateway
    participant GS as Game Server (Zone A)
    participant GS_B as Game Server (Zone B)

    C1->>GW: PositionUpdate(x, y, z)
    GW->>GS: Forward position update
    GS->>GS: Update entity position in zone A
    GS->>GS: Query AOI index for interested entities

    alt Entity crosses zone boundary
        GS->>GS_B: NotifyEntityEnter(entity_id, zone_b_id)
        GS_B->>GS_B: Add entity to AOI index for zone B
        GS_B-->>GS: NotifyResponse(ack)
        GS->>C1: EntityDespawn(entity leaving zone A)
    end

    GS->>GW: EntityMove (for all interested clients)
    GW->>C1: EntityMove
    GW->>C2: EntityMove
```

## Service Discovery Flow

```mermaid
sequenceDiagram
    participant GS as Game Server
    participant DNS as DNS Server
    participant RS as Room Service
    participant PG as PostgreSQL

    GS->>DNS: Resolve room-service hostname
    DNS-->>GS: IP address
    GS->>RS: Register(server_id, host, port, capacity)
    RS->>PG: INSERT INTO game_servers
    PG-->>RS: OK
    RS-->>GS: RegisterResponse(success)

    loop Every 5 seconds
        GS->>RS: Heartbeat(load)
        RS->>PG: UPDATE game_servers SET last_heartbeat=NOW()
        RS-->>GS: HeartbeatResponse(ack)
    end
```

## References

- [Architecture Overview](../architecture/overview.md)
- [Component Diagram](component.md)
