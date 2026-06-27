> **Last Updated:** 2026-06-27

## Purpose

Chaos testing recovery procedures for ADR-011 failure modes.

## References

- [ADR-011](../adr/011-zone-ownership.md)
- [Phase 6 Production Hardening](../../superpowers/plans/phase-6-production-hardening.md)

## Failure Modes

### Game Server Crash
- **Inject:** `kubectl delete pod -l app=game-server --grace-period=0 --wait=false`
- **Recovery:** Zone ORPHAN → reassigned ≤15s. State from PostgreSQL ≤5s loss.
- **Rollback:** Zone repopulated from Room Service ownership map.

### Leader Failover
- **Inject:** `kubectl delete pod -l app=room-service --field-selector=status.phase=Running`
- **Recovery:** Follower acquires Lease within seconds.
- **Rollback:** PostgreSQL ownership table is source of truth.

### Network Partition
- **Inject:** `iptables DROP` between two game-server pods
- **Recovery:** Split-brain prevented via PostgreSQL advisory lock. Stale owner surrenders zone ≤15s.
- **Rollback:** Remove iptables rules: `iptables -F`

### Redis Loss
- **Inject:** Delete redis pod / block port 6379
- **Recovery:** Graceful degrade to PostgreSQL. Session lookups slower, no data loss.
- **Rollback:** Redis recovery from AOF snapshot.

### PG Pool Exhaustion
- **Inject:** Repeated `SELECT pg_sleep(30)` to exhaust connections
- **Recovery:** Writes queued in bounded buffer. New zone transfers blocked. No crash.
- **Rollback:** Terminate sleeping queries: `SELECT pg_terminate_backend(pid)`
