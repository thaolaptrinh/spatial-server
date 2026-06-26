# Contribution Guide

> **Last Updated:** 2026-06-26

## Purpose

Standardize contribution workflows for the Spatial Server repository.

## Branch Strategy

| Branch | Purpose |
|--------|---------|
| `main` | Production-ready code. Protected. |
| `develop` | Integration branch for features. |
| `feature/<name>` | Feature branches, branched from `develop`. |
| `fix/<name>` | Bugfix branches. |
| `release/<version>` | Release preparation branches. |

## Workflow

1. Create a feature branch from `develop`: `git checkout -b feature/my-feature develop`
2. Implement changes with tests
3. Run `make lint && make test` to verify
4. Push branch and create Pull Request against `develop`
5. PR must pass CI and receive at least one approval
6. Squash-merge into `develop`

## Commit Message Convention

Follow [Conventional Commits](https://www.conventionalcommits.org/):

```
feat: add zone transfer mechanism
fix: correct AOI query bounds check
docs: update architecture overview
refactor: extract entity validation
test: add zone ownership unit tests
```

## PR Requirements

- All tests pass
- Lint passes
- Documentation updated (if applicable)
- ADR created (if architectural decision)
- At least one reviewer approved

## References

- [Branch Strategy](branch-strategy.md)
- [Release Process](release.md)
