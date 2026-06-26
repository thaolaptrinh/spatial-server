# Diagrams

> **Last Updated:** 2026-06-26

## Purpose

Visual diagrams of the Spatial Server architecture — system context, component relationships, deployment topology, runtime lifecycle, ownership, and RPC flows.

## Contents

| File | Description |
|------|-------------|
| [system-context.md](system-context.md) | C4 system context — external actors and system boundary |
| [component.md](component.md) | C4 container diagram — internal services and their connections |
| [deployment.md](deployment.md) | Production K3s deployment topology with resource mapping |
| [sequences.md](sequences.md) | Key sequence diagrams — connection, zone transfer, heartbeat, AOI, service discovery |
| [runtime-lifecycle.md](runtime-lifecycle.md) | Runtime state machine and lifecycle swimlane |
| [ownership.md](ownership.md) | Zone ownership, transfer sequence, and conflict resolution |
| [rpc-flow.md](rpc-flow.md) | All inter-service RPC flows with properties |

## Reading Order

1. Start with [system-context.md](system-context.md) for the big picture.
2. Review [component.md](component.md) and [deployment.md](deployment.md) for internal structure.
3. Study [runtime-lifecycle.md](runtime-lifecycle.md) and [ownership.md](ownership.md) for core concepts.
4. Read [sequences.md](sequences.md) and [rpc-flow.md](rpc-flow.md) for detailed interaction flows.

## Related Documents

- [Architecture](../architecture/README.md) — Detailed architecture documentation
- [ADRs](../adr/README.md) — Architecture Decision Records
