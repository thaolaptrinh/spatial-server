# Runtime Lifecycle Diagram

> **Last Updated:** 2026-06-26

## Runtime State Machine

```mermaid
stateDiagram-v2
    [*] --> creating : CreateRuntime() called
    creating --> active : Zones allocated\nGame Servers assigned
    creating --> destroyed : Allocation failure\nor timeout
    active --> draining : DestroyRuntime() called
    active --> active : Rebalance zones\n(no state change)
    draining --> destroyed : All players disconnected\nResources released
    draining --> destroyed : Force destroy timeout
    destroyed --> [*]

    note right of creating
        Business Backend initiates
        Room Service allocates zones,
        assigns Game Servers,
        stores metadata in PostgreSQL
    end note

    note right of active
        Players can connect
        Entity simulation running
        Zone transfers possible
        Heartbeat active
    end note

    note right of draining
        New connections rejected
        Players notified of shutdown
        Zone state persisted
        Grace period for reconnection
    end note
```

## Runtime Lifecycle Swimlane

```mermaid
sequenceDiagram
    participant BB as Business Backend
    participant RS as Room Service
    participant GS as Game Server
    participant GW as Gateway
    participant PG as PostgreSQL

    rect rgb(200, 230, 255)
        Note over BB,PG: CREATING
        BB->>RS: CreateRuntime(runtime_id, zone_count)
        RS->>RS: Allocate zone IDs
        RS->>PG: INSERT runtime (status=creating)
        RS->>RS: Select Game Servers for zones
        RS->>GS: AssignZone(zone_id, runtime_id)
        GS->>GS: Initialize zone state
        GS-->>RS: AssignZoneResponse(ok)
        RS->>PG: INSERT zone_ownership records
        RS->>PG: UPDATE runtime SET status=active
        RS-->>BB: CreateRuntimeResponse(gateway_addr, runtime_info)
    end

    rect rgb(230, 255, 230)
        Note over BB,PG: ACTIVE
        loop Tick every 50ms
            GS->>GS: Process entity updates
            GS->>GS: Run AOI queries
            GS->>GW: Push entity updates to clients
        end
        loop Heartbeat every 5s
            GS->>RS: Heartbeat(load)
            RS->>PG: UPDATE last_heartbeat
            RS-->>GS: HeartbeatResponse(ack)
        end
    end

    rect rgb(255, 230, 200)
        Note over BB,PG: DRAINING
        BB->>RS: DestroyRuntime(runtime_id)
        RS->>RS: Mark runtime as draining
        RS->>GW: DisconnectPlayers(runtime_id)
        GW->>GW: Send shutdown notification to clients
        GW->>GW: Close WebSocket connections
        RS->>GS: PrepareShutdown(zone_ids)
        GS->>GS: Persist zone state to PostgreSQL
        GS->>GS: Stop simulation loops
        GS-->>RS: PrepareShutdownResponse(ok)
        RS->>PG: UPDATE zone_ownership SET server_id=NULL
        RS->>PG: UPDATE runtime SET status=destroyed
    end

    rect rgb(255, 200, 200)
        Note over BB,PG: DESTROYED
        RS->>GS: ReleaseZone(zone_id)
        GS->>GS: Free zone resources
        RS->>PG: DELETE zone_ownership WHERE runtime_id=?
        RS->>PG: DELETE runtime WHERE runtime_id=?
    end
```

## Description

The runtime lifecycle is modeled as a four-state state machine: `creating → active → draining → destroyed`.

**Creating:** Business Backend calls `CreateRuntime()`. Room Service allocates zone IDs, selects optimal Game Servers via load-aware assignment, persists ownership records, and returns the Gateway address to the Business Backend.

**Active:** The runtime is live. Players connect via Gateway, entity simulation runs at 20 Hz, AOI queries process, and Game Servers send heartbeats every 5 seconds. Zone transfers and rebalancing occur within this state without transitioning out.

**Draining:** Business Backend calls `DestroyRuntime()`. Room Service signals Gateway to disconnect all players with a shutdown notification. Game Servers persist any dirty state, stop simulation, and acknowledge shutdown. A configurable grace period allows for reconnection attempts.

> **Note:** The `PrepareShutdown` RPC during draining (RS→GS) differs in direction from `PrepareShutdown` in ADR-009 (GS→RS, under `service RoomService`). The draining flow requires Room Service to initiate shutdown orchestration. This is tracked as a known inconsistency — ADR-009 should be updated to include a separate `DrainZones(zoneIDs)` RPC (RS→GS) for the draining path.

**Destroyed:** All resources are released: zone ownership records deleted, runtime metadata cleaned from PostgreSQL. The runtime ID becomes available for reuse.

## References

- [ADR-016](../adr/016-runtime-lifecycle.md) — Runtime Lifecycle
- [ADR-009](../adr/009-rpc-contract.md) — RPC Contract
- [Sequence Diagrams](sequences.md)
