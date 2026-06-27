# Protobuf Convention Standards

> **Last Updated:** 2026-06-26

## Purpose

Define consistent protobuf conventions for all `.proto` files in the Spatial Server platform, including package naming, message style, field numbering, imports, and code generation configuration.

## Package Naming

```protobuf
package spatialserver.v1;
```

| Component | Convention | Example |
|-----------|------------|---------|
| Root | `spatialserver` | All proto files share this root |
| Version | `v{N}` | `v1`, `v2` — increment on breaking change |
| Sub-package | Optional for internal organization | `spatialserver.v1.gateway` (reserved for future use) |

**Rules:**

- Single flat package `spatialserver.v1` for all service definitions.
- Sub-packages only introduced when namespace collision is unavoidable.
- Package name matches the go_package suffix.

## File Naming and Layout

```
proto/
├── spatialserver/
│   └── v1/
│       ├── spatial_server_api.proto   # Business Backend API
│       ├── gateway.proto              # Gateway internal service
│       ├── room_service.proto         # Room Service definitions
│       ├── game_server.proto          # Game Server definitions
│       └── common.proto               # Shared types (EntityID, Vector3, ZoneID)
└── gen/                               # Generated output (gitignored)
    └── spatialserver/v1/*.pb.go
```

**Rules:**

- Files are nested under `proto/spatialserver/v1/` to match the `spatialserver.v1` package path.
- One file per service domain.
- `common.proto` for types shared across services.
- File names are snake_case, matching the primary service or domain.

## go_package Convention

```protobuf
option go_package = "github.com/thaolaptrinh/spatial-server/proto/gen/spatialserver/v1;spatialserverv1";
```

| Component | Rule |
|-----------|------|
| Module path | `github.com/thaolaptrinh/spatial-server` (matches `go.mod`) |
| Directory | `proto/gen/spatialserver/v1` |
| Package suffix | `spatialserverv1` (lowercase, version suffix) |

**Rules:**

- Generated Go code goes in `proto/gen/` (gitignored, generated at build time).
- All services in the same Go package (`spatialserverv1`).
- Never split generated code into separate Go packages per proto file.

## Message Style

### Naming

| Element | Convention | Good | Bad |
|---------|------------|------|-----|
| Message | PascalCase | `EntitySnapshot` | `entity_snapshot` |
| Field | snake_case | `zone_id` | `zoneID`, `ZoneId` |
| Enum type | PascalCase | `RuntimeStatus` | `runtime_status` |
| Enum value | UPPER_SNAKE_CASE | `RUNTIME_ACTIVE` | `RuntimeActive` |
| Oneof | PascalCase | `routing_rule` | `RoutingRule` |

### Message Structure

```protobuf
// EntitySnapshot represents the full state of an entity for zone transfer.
message EntitySnapshot {
  // Required fields (1-10)
  string entity_id = 1;            // UUIDv7
  string type = 2;                 // Entity type name
  Vector3 position = 3;            // Current world position

  // Optional fields (11-20)
  map<string, bytes> attributes = 11;  // Custom entity attributes

  // Reserved
  reserved 4, 5;
  reserved "old_field";
}
```

**Rules:**

- Required fields first, then optional, then metadata.
- Every field has a comment explaining its purpose (not its type).
- `map<string, bytes>` for extensible attributes — allows type-specific data without schema changes.
- Never use `google.protobuf.Any` — use explicit types or `map<string, bytes>`.

## Field Numbering

| Range | Usage | Notes |
|-------|-------|-------|
| 1 | Resource ID | Always the primary identifier |
| 2-10 | Required fields | Core data |
| 11-20 | Optional fields | Extended data |
| 21-50 | Metadata | Timestamps, status, etc. |
| 51-500 | Service-specific | Per-message fields |
| 500-19000 | Reserved | Reserved for system use |
| 19000-19999 | (reserved by protobuf) | Never use |
| 20000+ | Reserved for future | Large messages, extensions |

**Rules:**

- Field 1 reserved for the resource ID across all messages.
- Deleted fields use `reserved` with both number and name.
- Never reuse a field number — it breaks wire compatibility.
- Leave gaps for future fields (don't number sequentially 1, 2, 3).

## Imports

```protobuf
syntax = "proto3";

package spatialserver.v1;

import "spatialserver/v1/common.proto";
import "google/protobuf/timestamp.proto";
import "google/protobuf/empty.proto";
```

**Rules:**

- Internal imports use the package-relative path with the `spatialserver/v1/` prefix (`import "spatialserver/v1/common.proto"`).
- Google imports use full path (`import "google/protobuf/timestamp.proto"`).
- Third-party protos (grpc-gateway, validate) use full module path.
- Imports are grouped: standard protos first, then internal, then third-party.
- Only import what is used — no unused imports.

## Enum Convention

```protobuf
enum RuntimeStatus {
  RUNTIME_STATUS_UNSPECIFIED = 0;
  RUNTIME_STATUS_CREATING = 1;
  RUNTIME_STATUS_ACTIVE = 2;
  RUNTIME_STATUS_DRAINING = 3;
  RUNTIME_STATUS_DESTROYED = 4;
}
```

**Rules:**

- Enum values are prefixed with the enum name to avoid collision (`RUNTIME_STATUS_` prefix).
- Value 0 is `_UNSPECIFIED` (protobuf default), not a valid state.
- Never delete enum values — mark as `reserved` if no longer used.
- New values are appended, never inserted.

## Oneof Convention

```protobuf
message ZoneEvent {
  string zone_id = 1;

  oneof event {
    EntitySpawnData entity_spawn = 10;
    EntityDespawnData entity_despawn = 11;
    ZoneTransferData zone_transfer = 12;
  }
}
```

**Rules:**

- Oneof fields numbered in the 10+ range (after required fields).
- Oneof field names are snake_case matching the event type.
- Use oneof for mutually exclusive data, not for optional fields.

## Comments

```protobuf
// A ZoneID uniquely identifies a zone within the spatial grid.
message ZoneID {
  string room_id = 1;  // UUIDv7 — the runtime/room this zone belongs to
  int32 grid_x = 2;    // Grid column index (0-based)
  int32 grid_y = 3;    // Grid row index (0-based)
}
```

**Rules:**

- Every message has a file-level or message-level comment.
- Every field has an inline comment explaining semantics.
- Comments describe the "why" not the "what" — unit is useful (`// in milliseconds`).

## Code Generation

Code is generated with [buf](https://buf.build) (not raw `protoc`), driven by `buf.yaml` (module + lint/breaking config) and `buf.gen.yaml` (plugin config) at the repository root.

```yaml
# buf.gen.yaml
version: v2
plugins:
  - local: protoc-gen-go
    out: proto/gen
    opt: paths=source_relative
  - local: protoc-gen-go-grpc
    out: proto/gen
    opt: paths=source_relative
```

```makefile
# Makefile target — run with `make proto`
proto:
	buf generate --path proto/spatialserver/v1
```

| Setting | Value |
|---------|-------|
| Generator | `buf generate` |
| Go plugins | `protoc-gen-go`, `protoc-gen-go-grpc` (declared in `buf.gen.yaml`) |
| Output dir | `proto/gen/` |
| Module mode | `paths=source_relative` |
| Lint config | `buf.yaml` (uses `STANDARD` lint, `FILE` breaking) |
| Generated files | `*.pb.go`, `*_grpc.pb.go` under `proto/gen/spatialserver/v1/` |
| Check into VCS | No (gitignored, generated by `make proto`) |

## References

- [ADR-009](../adr/009-rpc-contract.md) — RPC Contract (proto definitions reference)
- [ADR-010](../adr/010-packet-protocol.md) — Packet Protocol (client binary protocol, not proto)
- [API Convention](api-convention.md) — RPC naming and message patterns
- [gRPC Convention](grpc-convention.md) — Runtime behavior of protobuf-generated services
