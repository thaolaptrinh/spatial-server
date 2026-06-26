# API Documentation

> **Last Updated:** 2026-06-26

## Purpose

Define the API contracts for Spatial Server — both the Business Backend-facing gRPC API and the operator admin API.

## Contents

| File | Description |
|------|-------------|
| [spatial-server.md](spatial-server.md) | Business Backend gRPC API — runtime lifecycle (CreateRuntime, DestroyRuntime, GetRuntimeInfo, GetRuntimeMetrics, ListRuntimes) |
| [admin-api.md](admin-api.md) | Operator admin API — system management, diagnostics, and operational controls |

## Reading Order

1. Start with [spatial-server.md](spatial-server.md) for the primary Business Backend integration API.
2. Review [admin-api.md](admin-api.md) for operator capabilities.

## Related Documents

- [ADR-016](../adr/016-runtime-lifecycle.md) — Runtime Lifecycle
- [Protocol](../protocol/websocket.md) — Client-facing WebSocket protocol
