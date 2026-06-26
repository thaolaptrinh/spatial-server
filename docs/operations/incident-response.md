# Incident Response

> **Last Updated:** 2026-06-26

## Purpose

Define the incident response process including severity levels, escalation paths, communication templates, and post-mortem procedures.

## Severity Levels

| Level | Label | Definition | Response Time | Examples |
|-------|-------|------------|---------------|----------|
| SEV-1 | Critical | Complete service outage or data loss. All users affected. | 5 min | PostgreSQL crash, Gateway fully down, Game Server mass crash |
| SEV-2 | Major | Partial outage or severe degradation. Subset of users affected. | 15 min | Redis crash (degraded), Game Server single instance down, zone migration failures |
| SEV-3 | Minor | Degraded performance but functional. No user-facing impact expected. | 60 min | Tick overrun, high latency (p99 > 100ms), elevated error rates |
| SEV-4 | Informational | Non-urgent issues found during monitoring. | Next business day | Capacity nearing thresholds, outdated dependencies, warning alerts |

## Incident Response Flow

```text
┌─────────────────────┐
│  Alert Triggered    │
│  or User Report     │
└─────────┬───────────┘
          ▼
┌─────────────────────┐
│   Assess Severity   │
│   (see table above) │
└─────────┬───────────┘
          ▼
┌─────────────────────┐
│    Declare Incident  │
│  (#incidents Slack)  │
└─────────┬───────────┘
          ▼
┌─────────────────────┐
│  Assign IC (Incident │
│     Commander)      │
└─────────┬───────────┘
          ▼
┌─────────────────────┐
│   Mitigate / Fix    │
│  (see runbook.md)   │
└─────────┬───────────┘
          ▼
┌─────────────────────┐
│   Resolution         │
│   Verify & Monitor  │
└─────────┬───────────┘
          ▼
┌─────────────────────┐
│   Post-Mortem       │
│   (within 48 hours) │
└─────────────────────┘
```

### Step 1: Declare Incident

The first person who detects the incident declares it:

- **Slack:** Post in `#incidents` with severity, service, and summary.
- **On-call:** Page the on-call engineer (PagerDuty or equivalent).
- **SEV-1/2:** Also notify `#engineering` and `#leadership` channels.

### Step 2: Assign Incident Commander (IC)

- IC is the on-call engineer or the first responder.
- IC's job is coordination, not debugging. Assign a separate investigator.
- IC tracks timeline, updates comms, escalates if needed.

### Step 3: Mitigate

- Follow recovery procedures in [runbook.md](runbook.md) and [ADR-011](../adr/011-failure-recovery.md).
- If runbook does not cover the scenario, IC delegates investigation to the most qualified engineer.
- IC communicates status updates every 15 min (SEV-1) or 30 min (SEV-2).

### Step 4: Resolve

- Verify fix with health checks and monitoring.
- Confirm no residual impact.
- Post resolution summary in `#incidents`.

## Escalation Paths

| Role | Primary | Secondary | Tertiary |
|------|---------|-----------|----------|
| Incident Commander | On-call engineer | Senior engineer | Engineering manager |
| Database (PostgreSQL) | DBA / on-call | Infrastructure lead | Engineering manager |
| Game Server / Room Service | Platform team lead | Senior engineer | Engineering manager |
| Networking / Gateway | Infrastructure lead | Senior engineer | Engineering manager |
| Security incident | Security lead | Engineering manager | CTO |

Escalation flow: Primary → Secondary → Tertiary → CTO, with max 15 min per level for SEV-1.

## Communication Templates

### Incident Declaration

```
INCIDENT: [SEV-1/2/3/4]
Service: [service name]
Summary: [what is happening]
Impact: [who/what is affected]
Time: [UTC timestamp]
IC: [name]
Status: [investigating / mitigated / resolved]
```

### Status Update

```
UPDATE: [incident ID]
Time: [UTC timestamp]
Status: [investigating / mitigated / resolved]
Action taken: [what was done]
Next steps: [what remains]
ETA: [estimated resolution time, if known]
```

### Resolution

```
RESOLVED: [incident ID]
Time: [UTC timestamp]
Duration: [time from declaration to resolution]
Root cause: [brief summary]
Action taken: [summary of fix]
Post-mortem: [link to post-mortem doc]
```

## Post-Mortem Process

### Timeline

Within 48 hours of resolution:
1. IC creates a post-mortem document (`docs/post-mortems/YYYY-MM-DD-<incident>.md`).
2. Schedule a post-mortem meeting within 7 days.
3. Invite: all responders, relevant team members, engineering manager.

### Post-Mortem Template

```markdown
# Post-Mortem: [Title]

Date: YYYY-MM-DD
Severity: SEV-1/2/3
Duration: Xh Xm
Services affected: [list]

## Timeline

| UTC | Event |
|-----|-------|
| HH:MM | Alert triggered |
| HH:MM | Incident declared |
| HH:MM | Mitigation started |
| HH:MM | Service restored |
| HH:MM | Monitoring confirmed stable |

## Root Cause

[What went wrong. Focus on systems, not people.]

## Impact

- Users affected: [count or percentage]
- Downtime: [duration]
- Data loss: [yes/no, amount]

## Action Items

- [ ] Fix: [short-term fix, owner, deadline]
- [ ] Fix: [long-term fix, owner, deadline]
- [ ] Process: [process improvement, owner, deadline]

## Lessons Learned

- [What went well]
- [What went wrong]
- [What we will do differently]

## Appendix

- [Links to relevant logs, dashboards, PRs]
```

### Blameless Culture

Post-mortems are blameless. The goal is to identify systemic weaknesses, not assign fault. Every team member should feel safe reporting incidents and participating in post-mortems.

## References

- [Runbook](runbook.md)
- [Disaster Recovery](disaster-recovery.md)
- [ADR-011](../adr/011-failure-recovery.md) — Failure Recovery
- [ADR-019](../adr/019-observability.md) — Observability
