# Zone Ownership Diagram

> **Last Updated:** 2026-06-26

## Purpose

How zones are mapped to Game Servers, the ownership table that tracks zone→server assignments (in-memory + PostgreSQL), and the claim/transfer/release/conflict-resolution operations that keep ownership consistent.

## Zone Ownership Architecture

```mermaid
graph TB
    subgraph "Zone Mapping"
        Z1[Zone 1001]
        Z2[Zone 1002]
        Z3[Zone 1003]
        Z4[Zone 1004]
        Z5[Zone 1005]
        Z6[Zone 1006]
    end

    subgraph "Game Servers"
        GS1[Game Server 1<br/>zone_ids: 1001, 1002, 1003]
        GS2[Game Server 2<br/>zone_ids: 1004, 1005]
        GS3[Game Server 3<br/>zone_ids: 1006]
    end

    subgraph "Room Service"
        RS[Room Service<br/>Ownership Table<br/>in-memory + PostgreSQL]
        OT["Ownership Table<br/>zone_id → server_id<br/>heartbeat_expires<br/>status: ACTIVE|TRANSFERRING|ORPHAN"]
    end

    subgraph "PostgreSQL"
        PG[(zone_ownership table)]
        PG_CONTENT["
            zone_id UUID PK<br/>
            server_id UUID NOT NULL<br/>
            runtime_id UUID NOT NULL<br/>
            status VARCHAR(20)<br/>
            heartbeat_expires TIMESTAMPTZ<br/>
            created_at TIMESTAMPTZ<br/>
            UNIQUE(zone_id)
        "]
    end

    Z1 --> GS1
    Z2 --> GS1
    Z3 --> GS1
    Z4 --> GS2
    Z5 --> GS2
    Z6 --> GS3

    GS1 -->|Heartbeat / Register| RS
    GS2 -->|Heartbeat / Register| RS
    GS3 -->|Heartbeat / Register| RS

    RS --> OT
    OT --> PG
    GW -->|LookupZone| RS
```

## Zone Transfer Sequence

```mermaid
sequenceDiagram
    participant RS as Room Service
    participant GS_A as Game Server A (source)
    participant GS_B as Game Server B (target)
    participant PG as PostgreSQL

    RS->>RS: Detect load imbalance
    RS->>RS: Select zone Z (least loaded zones first)
    RS->>RS: Pick GS_B as target (lowest load)

    rect rgb(255, 255, 200)
        Note over RS,PG: Phase 1: Prepare
        RS->>PG: BEGIN TRANSACTION
        RS->>PG: UPDATE zone_ownership SET status='TRANSFERRING' WHERE zone_id=Z
        RS->>GS_A: TransferZone(zone_id=Z, target=GS_B)
        GS_A->>GS_A: Pause AOI updates for zone Z
        GS_A->>GS_A: Serialize zone state (entities, positions, AOI index)
    end

    rect rgb(200, 230, 255)
        Note over RS,PG: Phase 2: Transfer
        GS_A->>GS_B: ZoneStateSync(snapshot) [direct gRPC, ServerStream]
        GS_B->>GS_B: Load snapshot into memory
        GS_B->>GS_B: Start simulation loop for zone Z
        GS_B-->>GS_A: ZoneStateSyncResponse(success)
    end

    rect rgb(200, 255, 200)
        Note over RS,PG: Phase 3: Commit
        GS_A-->>RS: TransferZoneResponse(success)
        RS->>PG: UPDATE zone_ownership SET status='ACTIVE', server_id=GS_B WHERE zone_id=Z
        RS->>PG: COMMIT TRANSACTION
        RS->>GW: Push routing update (zone Z → GS_B)
        GS_A->>GS_A: Release zone Z resources
    end
```

## Conflict Resolution

```mermaid
sequenceDiagram
    participant GS_X as Game Server X
    participant GS_Y as Game Server Y
    participant RS as Room Service
    participant PG as PostgreSQL

    Note over GS_X,PG: Split-brain scenario: both servers claim zone Z
    GS_X->>PG: INSERT INTO zone_ownership (zone_id='Z', server_id='X')
    GS_Y->>PG: INSERT INTO zone_ownership (zone_id='Z', server_id='Y')

    rect rgb(255, 200, 200)
        Note over PG: PostgreSQL UNIQUE constraint on zone_id
        PG-->>GS_X: INSERT succeeds (first wins)
        PG-->>GS_Y: ERROR: duplicate key value violates unique constraint
        GS_Y->>GS_Y: Surrender claim, log conflict
    end

    Note over GS_X: Zone Z is owned by GS_X
    Note over GS_Y: Zone Z is NOT owned by GS_Y

    rect rgb(200, 255, 200)
        Note over RS,PG: Stale heartbeat recovery
        RS->>PG: SELECT * FROM zone_ownership WHERE server_id='X' AND heartbeat_expires < NOW()
        PG-->>RS: Zone Z
        RS->>RS: Mark zone Z as orphan
        RS->>GS_X: ZoneClaimResponse(server_id=GS_X, status=ORPHAN)
        GS_X->>GS_X: Check local state, surrender zone Z
        RS->>GS_Y: TransferZone(zone_id=Z, target=GS_Y)
        GS_Y->>PG: INSERT zone_ownership (zone_id='Z', server_id='Y')
    end
```

## Ownership Operations

| Operation | Mechanism | PostgreSQL Statement |
|-----------|-----------|---------------------|
| **Claim zone** | Room Service assigns on CreateRuntime | `INSERT INTO zone_ownership (zone_id, server_id, status) VALUES ($1, $2, 'ACTIVE')` |
| **Transfer zone** | Two-phase prepare/commit | `UPDATE zone_ownership SET status='TRANSFERRING' WHERE zone_id=$1` → `UPDATE ... SET status='ACTIVE', server_id=$2` |
| **Release zone** | Room Service on DestroyRuntime | `DELETE FROM zone_ownership WHERE zone_id = $1` |
| **Recover orphan** | Heartbeat timeout detection | `UPDATE zone_ownership SET server_id=$1 WHERE heartbeat_expires < NOW() AND status != 'TRANSFERRING'` |
| **Conflict resolution** | PostgreSQL unique constraint | First INSERT wins, second gets error |

## References

- [ADR-001](../adr/001-zone-ownership.md) — Zone Ownership
- [ADR-002](../adr/002-zone-migration.md) — Zone Migration
- [ADR-011](../adr/011-failure-recovery.md) — Failure Recovery
- [Sequence Diagrams](sequences.md)
