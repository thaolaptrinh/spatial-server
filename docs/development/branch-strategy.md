# Branch Strategy

> **Last Updated:** 2026-06-26

| Branch | Purpose | Protected |
|--------|---------|-----------|
| `main` | Production-ready code | Yes |
| `develop` | Integration branch for features | Yes |
| `feature/<name>` | Feature branches, branched from `develop` | No |
| `fix/<name>` | Bugfix branches | No |
| `release/<version>` | Release preparation branches | No |

## Workflow

1. Branch from `develop`: `git checkout -b feature/my-feature develop`
2. Implement with tests
3. Run `make lint && make test`
4. Push and create PR against `develop`
5. Squash-merge after CI passes and review approved

## References

- [Contribution Guide](contributing.md)
- [Release Process](release.md)
