> **Last Updated:** 2026-06-27

## Purpose

Security audit checklist for production hardening sign-off.

## References

- [ADR-018](../adr/018-security.md)
- [Phase 6 Production Hardening](../../superpowers/plans/phase-6-production-hardening.md)

## Checklist

| Check | Method | Status |
|-------|--------|--------|
| OWASP Top 10 review | Manual review | [ ] |
| Dependency scan | `govulncheck ./...` | [ ] |
| Secret scan | gitleaks | [ ] |
| TLS 1.3-only (no downgrade) | `curl --tls-max 1.2` must fail | [ ] |
| No internal port public | nmap scan from outside VPC | [ ] |
| Secrets not in Git | check no config values in commits | [ ] |
| JWKS rotation works | deploy new key pair, verify | [ ] |
| Rate limits hold under burst | benchmark test | [ ] |
| Fuzz finds no crashes | `go test -fuzz` 30 min | [ ] |
