# Restore Procedures

> **Last Updated:** 2026-06-26

## Purpose

Document step-by-step procedures for restoring PostgreSQL and Redis from backups, including point-in-time recovery for PostgreSQL.

## PostgreSQL Restore

### Prerequisites

- Access to the backup S3 bucket (`spatial-backups`)
- GPG private key for backup decryption
- Target PostgreSQL instance running with same major version
- Sufficient disk space for restored data + WAL replay

### Full Restore from Latest Dump

```bash
# 1. Download and decrypt latest backup
aws s3 cp s3://spatial-backups/postgres/daily/latest.dump /tmp/restore.dump.gpg
gpg --decrypt /tmp/restore.dump.gpg > /tmp/restore.dump

# 2. Drop and recreate target database
psql -h target-host -U spatial -d postgres -c "DROP DATABASE IF EXISTS spatialdb;"
psql -h target-host -U spatial -d postgres -c "CREATE DATABASE spatialdb;"

# 3. Restore
pg_restore -h target-host -U spatial -d spatialdb \
  --exit-on-error --jobs=4 /tmp/restore.dump

# 4. Verify
psql -h target-host -U spatial -d spatialdb -c "SELECT count(*) FROM zones;"
```

### Point-in-Time Recovery (PITR)

Use when restoring to a specific timestamp (e.g., just before a corrupting event).

#### Step 1: Prepare Recovery Environment

```bash
# Ensure PostgreSQL is stopped
systemctl stop postgresql

# Move current data directory aside (keep as fallback)
mv /var/lib/postgresql/16/main /var/lib/postgresql/16/main.corrupted

# Create fresh data directory
pg_createcluster 16 main --start
systemctl stop postgresql
```

#### Step 2: Configure Recovery

Edit `/etc/postgresql/16/main/postgresql.conf`:

```
restore_command = 'aws s3 cp s3://spatial-backups/postgres/wal/%f %p'
recovery_target_time = '2026-06-25 14:30:00 UTC'
recovery_target_action = 'promote'
```

#### Step 3: Create Recovery Signal File

```bash
touch /var/lib/postgresql/16/main/recovery.signal
```

#### Step 4: Start PostgreSQL

```bash
systemctl start postgresql
journalctl -u postgresql -f  # Monitor recovery progress
```

PostgreSQL replays WAL from archive up to the target time, then promotes itself to normal operation.

#### Step 5: Verify Recovery Point

```bash
psql -U spatial -d spatialdb -c "SELECT pg_last_wal_replay_lsn(), pg_is_in_recovery();"
psql -U spatial -d spatialdb -c "SELECT count(*) FROM zones;"
```

Expected: `pg_is_in_recovery` = `f` (promoted), data reflects target timestamp.

#### Step 6: Re-enable WAL Archiving

Restore the original `archive_mode = on` and `archive_command`, then:

```bash
systemctl restart postgresql
```

### Latest Dump vs. PITR: When to Use

| Scenario | Method | Rationale |
|----------|--------|-----------|
| Accidental table drop | PITR | Recover to just before the DROP |
| Corrupted data (batch update) | PITR | Recover to just before the update |
| Full instance loss | Latest dump + PITR | Restore from latest dump, replay WAL to desired point |
| Staging refresh | Latest dump | No PITR needed |
| DR failover | Latest dump | Speed over precision |

## Redis Restore

### Full Restore from RDB

```bash
# 1. Download latest RDB snapshot
aws s3 cp s3://spatial-backups/redis/latest.rdb /var/lib/redis/dump.rdb

# 2. Set correct permissions
chown redis:redis /var/lib/redis/dump.rdb

# 3. Restart Redis to load the RDB
systemctl restart redis

# 4. Verify keys are loaded
redis-cli DBSIZE
redis-cli KEYS 'session:*' | wc -l
```

### Point-in-Time Recovery

Redis RDB snapshots are point-in-time by nature (state at `BGSAVE` time). For finer granularity, replay AOF:

```bash
# 1. Configure AOF append-only replay
redis-cli CONFIG SET appendonly yes

# 2. Shutdown and restart to replay AOF
redis-cli SHUTDOWN
systemctl start redis
```

Note: AOF replay recovers to the last write before crash. If AOF is corrupted, fall back to RDB.

## Verification

After any restore:

1. Run health checks on all services.
2. Verify data integrity with known queries.
3. Confirm zone ownership table consistency.
4. Check replication status (if replica).
5. Run connection tests from Gateway, Room Service, Game Server.

## Rollback

If restore produces unexpected results:

```bash
# PostgreSQL: restore original data directory
systemctl stop postgresql
rm -rf /var/lib/postgresql/16/main
mv /var/lib/postgresql/16/main.corrupted /var/lib/postgresql/16/main
systemctl start postgresql
```

## References

- [Backup Strategy](backup.md)
- [Disaster Recovery](disaster-recovery.md)
- [ADR-011](../adr/011-failure-recovery.md) — Failure Recovery
