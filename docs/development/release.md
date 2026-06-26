# Release Process

> **Last Updated:** 2026-06-26

## Versioning

Spatial Server follows [Semantic Versioning](https://semver.org/):
- `vMAJOR.MINOR.PATCH` (e.g., `v1.0.0`)

## Release Checklist

1. Create `release/v<version>` branch from `develop`
2. Run full test suite: `make lint && make test`
3. Update documentation if needed
4. Create release tag: `git tag v<version>`
5. Push tag: `git push origin v<version>`
6. CI builds Docker images with tag
7. Merge release branch to `main`
8. Deploy to staging for validation
9. Deploy to production after validation

## Tag Strategy

| Tag | Purpose |
|-----|---------|
| `dev` | Latest on main branch |
| `staging` | Git tag with `-staging` suffix |
| `production` | Semantic version (`v1.0.0`, `v1.1.0`) |

## References

- [Branch Strategy](branch-strategy.md)
- [Contribution Guide](contributing.md)
