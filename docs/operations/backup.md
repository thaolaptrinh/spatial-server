# Backup Strategy

> **Last Updated:** 2026-06-26

## Purpose

Define backup schedules, retention policies, and verification procedures for PostgreSQL and Redis to ensure data recoverability.

## PostgreSQL Backups

### Logical Dumps (pg_dump)

| Attribute | Value |
|-----------|-------|
| Tool | `pg_dump` (custom format) |
| Schedule | Daily at 03:00 UTC (cron) |
| Retention | 30 days daily, 12 monthly, 3 yearly |
| Encryption | GPG with backup key |
| Storage | Object storage (S3-compatible) + local copy |
| Compression | Custom format (default zlib) |

```bash
pg_dump -h localhost -U spatial -Fc spatialdb \
  | gpg --encrypt --recipient backup-key \
  | aws s3 cp - s3://spatial-backups/postgres/daily/$(date +%Y-%m-%d).dump.gpg
```

### WAL Archiving (Point-in-Time Recovery)

| Attribute | Value |
|-----------|-------|
| Mode | `archive_mode = on` |
| Archive command | `cp %p /var/lib/postgresql/wal_archive/%f` |
| WAL retention | 7 days (continuous) |
| Archive destination | S3 + local filesystem |

Requires `archive_mode` and `archive_command` in `postgresql.conf`:

```
wal_level = replica
archive_mode = on
archive_command = 'aws s3 cp %p s3://spatial-backups/postgres/wal/%f && cp %p /var/lib/postgresql/wal_archive/%f'
archive_timeout = 300
```

### Backup Schedule Summary

| Type | Frequency | Retention | Recovery Point |
|------|-----------|-----------|----------------|
| Full pg_dump | Daily 03:00 UTC | 30 days | Time of dump |
| WAL archive | Continuous | 7 days | Any point within 7 days |
| Monthly | 1st of month | 12 months | Time of dump |
| Yearly | Jan 1 | 3 years | Time of dump |

## Redis Backups

### RDB Snapshots

| Attribute | Value |
|-----------|-------|
| Mechanism | Redis `SAVE` / `BGSAVE` |
| Schedule | Every 6 hours |
| Retention | 7 days |
| Location | `/var/lib/redis/dump.rdb` |
| Offsite copy | Uploaded to S3 after each snapshot |

### AOF (Append-Only File)

| Attribute | Value |
|-----------|-------|
| Mode | `appendfsync everysec` |
| Rewrite | Auto-triggered at 100% growth |
| Rewrite schedule | Via `BGREWRITEAOF` |

### Redis Backup Command

```bash
redis-cli BGSAVE && \
aws s3 cp /var/lib/redis/dump.rdb \
  s3://spatial-backups/redis/$(date +%Y-%m-%d_%H).rdb
```

## Verification Procedures

### Automated Verification (Post-Backup)

1. Backup script runs integrity check immediately after dump.
2. Logs success/failure to structured log (service: backup).
3. Failure triggers alert via monitoring (severity: major).

### Weekly Restore Test (Staging)

Every Sunday 05:00 UTC in staging environment:

```bash
pg_restore -h staging-db -U spatial -d spatialdb_test \
  --exit-on-error s3://spatial-backups/postgres/daily/latest.dump && \
echo "BACKUP_VERIFIED" | alert
```

### Monthly Restore Test (Full)

First Saturday of each month: restore latest full backup + replay WAL to verify PITR works end-to-end.

## Monitoring & Alerting

| Check | Condition | Severity |
|-------|-----------|----------|
| Backup completion | No backup in 26 hours | critical |
| Backup size delta | Size change > 20% vs 7-day avg | warning |
| WAL archiving lag | Archive command failing > 5 min | critical |
| Redis BGSAVE | BGSAVE takes > 5 min | warning |
| Restore test | Restore test fails | critical |

## Retention Cleanup

Automated via lifecycle policy on S3 bucket `spatial-backups`:

| Pattern | Retention |
|---------|-----------|
| `postgres/daily/*` | 30 days |
| `postgres/monthly/*` | 12 months |
| `postgres/yearly/*` | 3 years |
| `postgres/wal/*` | 7 days |
| `redis/*.rdb` | 7 days |

## References

- [Restore Guide](restore.md)
- [Disaster Recovery](disaster-recovery.md)
- [ADR-011](../adr/011-failure-recovery.md) — Failure Recovery
- [ADR-017](../adr/017-capacity-planning.md) — Capacity Planning
