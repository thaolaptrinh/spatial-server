# Coding Standards

> **Last Updated:** 2026-06-26

## Purpose

Define consistent coding conventions across all Spatial Server codebases.

## Scope

All Go code within the `spatial-server` repository.

This document references dedicated standards for detailed conventions. Each subtopic below links to its full specification.

## Package Layout

```
pkg/<domain>/
├── <domain>.go        # Main types + interface
├── <domain>_test.go   # Unit tests
├── impl/              # Implementations (if interface is in parent package)
│   └── <variant>.go
or
└── <variant>.go       # Single implementation file
```

## Interface Rules

- Define interfaces in the consumer package (not implementor)
- Prefer small interfaces (1-3 methods)
- Accept interfaces, return concrete types

## Detailed Standards

| Topic | Document |
|-------|----------|
| Naming conventions | [naming.md](naming.md) |
| Dependency rules | [dependency-rules.md](dependency-rules.md) |
| Error handling | [error-handling.md](error-handling.md) |
| Logging | [logging.md](logging.md) |
| API design | [api-convention.md](api-convention.md) |
| gRPC conventions | [grpc-convention.md](grpc-convention.md) |
| Protobuf conventions | [protobuf-convention.md](protobuf-convention.md) |
| Configuration | [configuration.md](configuration.md) |
| Metrics | [metrics.md](metrics.md) |
| Versioning | [versioning.md](versioning.md) |

## References

- [ADR-015](../adr/015-architecture-principles.md) — Architecture Principles
- [Dependency Rules](dependency-rules.md)
- [Architecture Repository Structure](../architecture/repository-structure.md)
