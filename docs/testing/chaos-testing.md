# Chaos Testing

> **Last Updated:** 2026-06-28

## Purpose

Define the chaos testing approach for validating Spatial Server resilience under network partitions, node failures, and latency injection.

## Principles

- Chaos tests run against a staging environment, never production.
- Each test has a clear hypothesis and measurable success criteria.
- The blast radius is constrained (single service, single AZ initially).
- Tests are automated and repeatable via CI pipeline.

## Scenarios

### Network Partition (Gateway Isolation)

**Hypothesis:** When a Gateway is partitioned from Room Service, it continues serving existing connections and re-syncs when connectivity is restored.

**Procedure:**
1. Establish 500 client connections through Gateway A.
2. Block all traffic between Gateway A and Room Service via iptables.
3. Wait 30s — Gateway A should continue proxying Game Server traffic.
4. Restore connectivity after 60s.
5. Verify Gateway A re-registers with Room Service and all 500 clients remain connected.

**Success Criteria:**
- Zero client disconnections during partition.
- Gateway re-registers within 5s of restoration.
- No duplicate zone assignments.

### Game Server Crash Recovery

**Hypothesis:** When a Game Server crashes, Room Service detects the failure and reassigns its zones to healthy Game Servers within the timeout window.

**Procedure:**
1. Start 3 Game Servers with 2 zones each (6 zones total).
2. Kill Game Server B (SIGKILL).
3. Wait for Room Service heartbeat timeout (default: 15s).
4. Verify zones from Game Server B are reassigned to A or C.

**Success Criteria:**
- All zones reassigned within 30s of crash.
- Players in affected zones receive `ZoneTransfer` notification.
- No data loss: entity state is recovered from PostgreSQL.

### Latency Injection (High RTT)

**Hypothesis:** Internal RPCs with >100ms latency do not cause cascading failures (timeouts, connection leaks).

**Procedure:**
1. Add 50ms ± 20ms latency on all inter-Game Server RPC traffic using `tc netem`.
2. Run medium-load simulation (1,000 clients, 10 min).
3. Verify RPC timeouts and retry counts remain within bounds.

**Success Criteria:**
- RPC error rate <1% despite added latency.
- No goroutine leak (goroutine count stable over 10 min).
- Client-facing latency remains <200ms p95.

### Leader Election Failure

**Hypothesis:** Room Service leader election (PostgreSQL advisory lock) recovers cleanly when the leader crashes.

**Procedure:**
1. Verify Room Service A holds the leader lock.
2. Kill Room Service A.
3. Wait for Room Service B to acquire the lock.

**Success Criteria:**
- New leader elected within 10s.
- No concurrent leadership (two services holding the lock simultaneously).
- Runtime CRUD operations succeed after failover.

### Cascading Zone Transfer Storm

**Hypothesis:** Triggering zone transfers for 50% of zones simultaneously does not overwhelm Room Service.

**Procedure:**
1. Create 100 zones across 5 Game Servers.
2. Issue zone transfer commands for 50 zones simultaneously.
3. Monitor Room Service queue depth and transfer completion time.

**Success Criteria:**
- All 50 transfers complete within 60s.
- Room Service CPU stays below 80%.
- No transfers fail (all eventually succeed).

## Tools

| Tool | Purpose | Installation |
|------|---------|-------------|
| iptables | Network partition simulation | Built-in (Linux) |
| tc/netem | Latency and packet loss injection | `iproute2` package |
| pumba | Docker container chaos | `docker run gaiaadm/pumba` |
| Custom chaos scripts | Automated scenario execution | `test/chaos/` directory |

## Scripted Chaos Test Example

```bash
#!/bin/bash
# test/chaos/partition-gateway.sh

GATEWAY_IP=$1
ROOM_SERVICE_IP=$2

echo "=== Chaos: Isolating Gateway $GATEWAY_IP from Room Service ==="

# Block outbound
ssh "$GATEWAY_IP" "iptables -A OUTPUT -d $ROOM_SERVICE_IP -j DROP"

echo "Partition started at $(date)"
sleep 60

# Restore
ssh "$GATEWAY_IP" "iptables -D OUTPUT -d $ROOM_SERVICE_IP -j DROP"

echo "Partition ended at $(date)"
```

## CI Integration

- Chaos tests run on a dedicated staging cluster, not shared CI infrastructure.
- Nightly chaos pipeline (post-merge, pre-release).
- Any chaos test failure blocks the release candidate.
- Test reports include: hypothesis, procedure, observations, pass/fail per criterion.

## References

- [ADR-011](../adr/011-failure-recovery.md) — Failure Recovery
- [ADR-012](../adr/012-networking.md) — Networking
- [Testing Strategy](strategy.md)
- [Simulation Testing](simulation-testing.md)
