# Phase 1E — Runtime Features

> **Last Updated:** 2026-06-26
> **Status:** Approved

## Purpose

Integrate AOI into the Game Server so entities only receive updates about visible entities. Add zone boundary detection with ghost entity support (same-server only).

## Architecture

3 sequential milestones, each independently testable:

| Milestone | Scope | Files |
|-----------|-------|-------|
| 1E.1 | AOI integration: `aoi.Enter`/`Leave`/`Move` in `pkg/game` | `pkg/game/game.go` |
| 1E.2 | Entity visibility: AOI query per tick, spawn/despawn/move packets per client, full snapshot on AOI enter | `pkg/game/game.go` |
| 1E.3 | Zone boundary detection, ghost entity with TTL, outbound filter | `pkg/game/game.go` |

### 1E.1 — AOI Integration

- `Game.AddEntity` → `g.aoi.Enter(e.ID, e.Position)`
- `Game.RemoveEntity` → `g.aoi.Leave(e.ID)`
- `Game.tick` → drain inbound → if PositionUpdate → `g.aoi.Move(e.ID, newPos)`
- Game struct gains `aoi *aoi.AOI` field, initialized in `New()` with DefaultCellSize/DefaultAOIRadius

### 1E.2 — Entity Visibility

Each tick after drain inbound:
1. For each entity: `g.aoi.EntitiesInRange(entity.Position, aoiRadius)`
2. Compare current AOI set with previous tick's set
3. On new entity in AOI: build full snapshot packet (`EntitySpawn`) → push to Outbox
4. On entity left AOI: build `EntityDespawn` → push to Outbox
5. On position change within AOI: build `EntityMove` → push to Outbox
6. Store per-entity AOI set for next tick diff

### 1E.3 — Zone Boundary + Outbound Filter

- Each tick: check entity grid coordinate via `g.aoi.cellKeyFor(pos)` vs previous
- On grid change: get old cell entities, spawn ghost with TTL (5s)
- Ghost entities: stored in separate `ghosts map[EntityID]*ghostEntry` with expiry time
- Each tick: sweep expired ghosts → `aoi.Leave` → notify clients in old zone
- Outbound filter: only push to Outbox if recipient entity is in sender's AOI (reverse check)

## Non-goals

- Cross-GameServer zone transfer
- Ghost entity migration (ghost stays in place, despawns on TTL)
- Attribute delta sync (position only)
- Persistent entity state

## Definition of Done

- Entity enter/leave AOI produces spawn/despawn packets
- Entity moving within AOI produces move packets
- Entity crossing zone boundary creates ghost in old zone
- Ghost entity despawns after 5s TTL
- Outbound only reaches clients in sender's AOI
- All unit tests pass

## References

- [Phase 1D Spec](2026-06-26-phase1d-runtime.md)
- [ADR-003](../adr/003-aoi-strategy.md) — AOI Strategy
