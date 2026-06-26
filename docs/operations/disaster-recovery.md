# Disaster Recovery Plan

> **Last Updated:** 2026-06-26

## Purpose

Define disaster recovery objectives, failover procedures between regions/data centers, and recovery steps for each failure scenario.

## RPO / RTO Targets

| Service | RPO (Recovery Point Objective) | RTO (Recovery Time Objective) |
|---------|-------------------------------|-------------------------------|
| PostgreSQL | 5 min (WAL) / 24 hr (full dump) | 30 min |
| Redis (session cache) | 6 hours | 15 min |
| Redis (metadata cache) | 6 hours | 15 min |
| Gateway | N/A (stateless) | 5 min |
| Room Service | 5s (state in PostgreSQL) | 30s |
| Game Server | 5s (in-memory state) | 60s |

RPO is driven by WAL archiving (continuous) + periodic RDB snapshots. Acceptable data loss for Game Server in-memory state is 5s (one persistence interval).

## Failure Scenarios

### Single Node Failure (K3s Worker)

**Impact:** Services on node are unavailable. K3s reschedules pods to healthy nodes.

**Detection:** Node `NotReady` > 60s.

**Recovery:**
1. Verify node is unreachable (ping, SSH).
2. Cordoning should already be automatic via K3s.
3. Pods reschedule to remaining nodes (K3s default).
4. If persistent volume (PostgreSQL, Redis), the pod must be rescheduled on original node or PV manually reassigned.
5. If PostgreSQL was on failed node, restore from WAL archive + latest dump to new node.

### Regional / Datacenter Failure

**Architecture:** Single-region currently (MVP). Multi-region is a post-MVP enhancement.

**Current DR (single region):**
- K3s multi-node cluster provides node-level HA.
- PostgreSQL and Redis run with data replicated to S3-compatible storage (off-node).
- If entire region fails, the system is unavailable until infrastructure is reprovisioned.

**Future multi-region plan:**
```
┌─────────────────────┐    ┌─────────────────────┐
│   Primary Region    │    │  Secondary Region   │
│  ┌───────────────┐  │    │  ┌───────────────┐  │
│  │   PostgreSQL   │──┼────┼─>│  PostgreSQL   │  │
│  │   (Primary)    │  │    │  │  (Standby)    │  │
│  └───────────────┘  │    │  └───────────────┘  │
│  ┌───────────────┐  │    │  ┌───────────────┐  │
│  │    K3s +      │  │    │  │   K3s +       │  │
│  │  Services     │  │    │  │  Services     │  │
│  └───────────────┘  │    │  └───────────────┘  │
│  ┌───────────────┐  │    │  ┌───────────────┐  │
│  │    Redis      │  │    │  │    Redis      │  │
│  │   (Primary)   │  │    │  │   (Replica)   │  │
│  └───────────────┘  │    │  └───────────────┘  │
└─────────────────────┘    └─────────────────────┘
```

#### Failover to Secondary (Future)

1. Promote PostgreSQL standby to primary (`pg_ctl promote`).
2. Reconfigure Redis replica as primary (sentinel or manual).
3. Update DNS / load balancer to point to secondary region.
4. Start Gateway, Room Service, Game Server instances in secondary.
5. Verify: health checks pass, clients can connect, zone ownership is consistent.

#### Failback (Future)

1. Reprovision primary region infrastructure via Terraform.
2. Set up PostgreSQL streaming replication from secondary → primary.
3. Wait for replication lag to reach zero.
4. Promote primary, switch DNS back.
5. Verify, then decommission secondary.

### Full Infrastructure Loss (Region Destroyed)

**RTO:** 4 hours
**RPO:** 24 hours (latest full pg_dump) or 5 min (WAL if S3 survives)

**Recovery:**
1. Provision new infrastructure via Terraform (separate region or provider).
2. Restore PostgreSQL from S3 backup + WAL archive.
3. Restore Redis from S3 RDB snapshot.
4. Deploy services via Helm.
5. Validate health checks.
6. Update DNS.

### Data Corruption (Logical)

**Detection:** Alert from data integrity checks, or user reports data inconsistency.

**Recovery:**
1. Isolate the corrupted database (prevent writes, snapshot current state).
2. Identify corruption timestamp via audit logs.
3. Restore PostgreSQL to point just before corruption (PITR).
4. Verify data integrity.
5. Point services at restored database.
6. Post-mortem on root cause.

### Cascading Failure

**Scenario:** Room Service crash triggers repeated Game Server heartbeats to unknown coordinator.

**Recovery:**
1. Room Service restarts (K3s auto-restart or Lease API failover).
2. Game Servers re-register with new leader.
3. Zone ownership is rebuilt from PostgreSQL.
4. Do NOT attempt to "rescue" Game Servers — let them reconnect.

## Regional / Data Center Details

### Current Topology

| Resource | Location | Provider |
|----------|----------|----------|
| K3s cluster | Primary DC | Cloud-agnostic (Terraform managed) |
| PostgreSQL | K3s node | Kubernetes StatefulSet |
| Redis | K3s node | Kubernetes StatefulSet |
| Backups | S3-compatible | Off-cluster object storage |

### Multi-Region Pre-Requisites (Future)

- PostgreSQL streaming replication across regions
- Redis replication across regions (or sentinel-based failover)
- Global load balancer (DNS-based failover, e.g., Route53, Cloudflare)
- Cross-region backup replication (S3 CRR or equivalent)
- Terraform modules parameterized by region

## DR Drill Schedule

| Drill | Frequency | Scope |
|-------|-----------|-------|
| PostgreSQL dump restore | Weekly | Staging, restore from last backup |
| PITR test | Monthly | Staging, restore to arbitrary timestamp |
| Node failure | Quarterly | Kill a K3s worker, verify rescheduling |
| Full DR exercise | Yearly | Simulate region loss, restore from backups |

## References

- [Backup Strategy](backup.md)
- [Restore Guide](restore.md)
- [Incident Response](incident-response.md)
- [ADR-011](../adr/011-failure-recovery.md) — Failure Recovery
- [ADR-017](../adr/017-capacity-planning.md) — Capacity Planning
- [ADR-015](../adr/015-architecture-principles.md) — Architecture Principles
