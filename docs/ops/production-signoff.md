> **Last Updated:** 2026-06-27

## Purpose

Production sign-off verification against all quality ADRs.

## References

- [ADR-011 Zone Ownership](../adr/011-zone-ownership.md)
- [ADR-017 Performance Budget](../adr/017-performance-budget.md)
- [ADR-018 Security](../adr/018-security.md)
- [ADR-020 Testing Strategy](../adr/020-testing-strategy.md)

## Sign-off

| ADR | Measure | Target | Measured | Status | Signatory |
|-----|---------|--------|----------|--------|-----------|
| ADR-011 | Crash recovery | ≤15s zone reassign | — | [ ] | — |
| ADR-011 | State loss | ≤5s | — | [ ] | — |
| ADR-017 | Conn/Gateway | 10,000 | — | [ ] | — |
| ADR-017 | Entities/GS | 5,000 | — | [ ] | — |
| ADR-018 | TLS 1.3 | enforced | — | [ ] | — |
| ADR-018 | JWT | EdDSA + JWKS | — | [ ] | — |
| ADR-020 | p95 e2e | < 100ms | — | [ ] | — |
| ADR-020 | Fuzz | No crashes | — | [ ] | — |
