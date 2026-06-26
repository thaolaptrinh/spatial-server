# API Convention Standards

> **Last Updated:** 2026-06-26

## Purpose

Define consistent API design conventions for all Spatial Server interfaces: gRPC service definitions, method naming, error handling, and request/response patterns.

## gRPC Service Design

### Naming

| Element | Convention | Example |
|---------|------------|---------|
| Service | PascalCase, suffixed with domain | `SpatialServerAPI`, `RoomService` |
| RPC | PascalCase, verb + noun | `CreateRuntime`, `LookupZone` |
| Request | PascalCase, RPC name + `Request` | `CreateRuntimeRequest` |
| Response | PascalCase, RPC name + `Response` | `CreateRuntimeResponse` |
| Package | `spatialserver.v1` | `spatialserver.v1` |
| Proto file | snake_case, matching service | `room_service.proto` |

### Method Patterns

| Pattern | When to Use | Example |
|---------|-------------|---------|
| Create\* | Create a new resource | `CreateRuntime`, `CreateSession` |
| Get\* / Lookup\* | Read a resource by ID | `GetRuntimeInfo`, `LookupZone` |
| List\* | List resources with pagination | `ListRuntimes` |
| Destroy\* / Delete\* | Remove a resource | `DestroyRuntime` |
| Report\* | Push data (fire-and-forget) | `ReportMetrics` |
| Prepare\* | Initiate a multi-step operation | `PrepareShutdown`, `PrepareTransfer` |
| Notify\* | Inform another service | `NotifyEntityEnter`, `NotifyEntityLeave` |

## Request/Response Conventions

### Request Messages

```protobuf
message CreateRuntimeRequest {
  string runtime_id = 1;    // Required — UUIDv7
  int32 zone_count = 2;     // Required — positive integer
  double zone_size = 3;     // Optional — uses default if unset
  string game_server_type = 4; // Optional — pin to specific type
}
```

**Rules:**

- Required fields documented in comments.
- Optional fields use proto3 zero-value defaults.
- Pagination follows `page_size` + `page_token` pattern for List RPCs.
- Resource identifiers are UUIDv7 strings (not auto-increment integers).
- Timestamps use `google.protobuf.Timestamp` — never string or int64.

### Response Messages

```protobuf
message CreateRuntimeResponse {
  string runtime_id = 1;
  string gateway_addr = 2;
  int32 zone_count = 3;
  repeated ZoneInfo zones = 4;
}
```

**Rules:**

- Responses return only the data requested — no envelope wrappers.
- Success is implied by absence of error — no `bool success` field.
- Empty responses use `google.protobuf.Empty` or a dedicated message for future extensibility.
- Repeated fields are never `optional` — always empty list instead of null.

## Error Code Convention

All gRPC errors use a custom error model with the following structure:

| Code | gRPC Status | When Used |
|------|-------------|-----------|
| `ZONE_NOT_FOUND` | `NotFound` | Zone ID does not exist |
| `ZONE_NOT_OWNED` | `FailedPrecondition` | Server does not own this zone |
| `SERVER_NOT_FOUND` | `NotFound` | Server ID not registered |
| `SERVER_DRAINING` | `Unavailable` | Server is in DRAINING state |
| `TRANSFER_IN_PROGRESS` | `Aborted` | Zone is already being transferred |
| `RATE_LIMITED` | `ResourceExhausted` | Client is rate limited |
| `RUNTIME_NOT_FOUND` | `NotFound` | Runtime ID does not exist |
| `RUNTIME_ALREADY_EXISTS` | `AlreadyExists` | Runtime ID already exists |
| `NO_SERVER_AVAILABLE` | `ResourceExhausted` | No Game Server available |

**Rules:**

- Error codes are UPPER_SNAKE_CASE strings in the gRPC status details.
- Every error includes a human-readable message in the gRPC status description.
- Internal errors (unexpected failures) use standard gRPC `Internal` status — never expose stack traces.
- Validation errors (`InvalidArgument`) include field-level details in status details.

## Field Numbering

```protobuf
message Example {
  // Field 1 reserved for the resource identifier
  string id = 1;

  // Fields 2-10: required input fields
  string name = 2;
  int32 count = 3;

  // Fields 11-20: optional input fields
  string optional_label = 11;
  double optional_value = 12;

  // Fields 21+: output-only fields
  string created_at = 21;
}
```

**Rules:**

- Field 1 is always the resource ID (if applicable).
- Required fields first (2-10), then optional (11-20), then output-only (21+).
- Reserve deleted fields: `reserved 5, 7, 9; reserved "old_field";`.
- Never reuse a field number after deletion.

## References

- [ADR-009](../adr/009-rpc-contract.md) — RPC Contract (service definitions, error codes)
- [gRPC Convention](grpc-convention.md) — Timeouts, retries, streaming patterns
- [Protobuf Convention](protobuf-convention.md) — .proto file style, imports, go_package
