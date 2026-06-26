# Standards

> **Last Updated:** 2026-06-26

## Purpose

Standards and conventions for the Spatial Server platform. These documents define the rules every engineer must follow when writing code, designing APIs, and configuring infrastructure.

## Index

| Document | Scope |
|----------|-------|
| [Coding](coding.md) | Go code style, linters, formatting, commit conventions |
| [Naming](naming.md) | Naming conventions for Go, protobuf, config, infrastructure, database |
| [Dependency Rules](dependency-rules.md) | Layer architecture, per-package allowed dependencies, enforcement, prohibited patterns |
| [Error Handling](error-handling.md) | Error creation, wrapping, propagation, gRPC mapping, panic handling |
| [Logging](logging.md) | slog conventions, standard fields, structured logging patterns |
| [Versioning](versioning.md) | Semantic versioning for releases, protocol, and API versions |
| [Configuration](configuration.md) | Configuration loading, environment variables, secrets, koanf conventions |
| [Metrics](metrics.md) | Prometheus metric naming, labels, histogram buckets, alerting rules |
| [API Convention](api-convention.md) | gRPC API design conventions, error codes, field rules |
| [gRPC Convention](grpc-convention.md) | gRPC service design, streaming patterns, deadline propagation |
| [Protobuf Convention](protobuf-convention.md) | Protobuf file layout, field numbering, message patterns, package naming |

## Reading Order

1. Start with [Coding](coding.md) and [Naming](naming.md) for day-to-day conventions.
2. Read [Dependency Rules](dependency-rules.md) and [Error Handling](error-handling.md) for architecture-level rules.
3. Review [Logging](logging.md), [Configuration](configuration.md), and [Metrics](metrics.md) for operational conventions.
4. Study [Versioning](versioning.md) for release and API version management.
5. Read [API Convention](api-convention.md), [gRPC Convention](grpc-convention.md), and [Protobuf Convention](protobuf-convention.md) when designing new services.

## References

- [Coding Standards](../standards/coding.md) — Quick start for common conventions
- [Architecture Principles](../architecture/principles.md) — Design principles governing standards

