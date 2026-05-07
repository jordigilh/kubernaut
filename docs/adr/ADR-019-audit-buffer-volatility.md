# ADR-019: Audit Buffer Volatility (Known Limitation)

**Status:** Accepted
**Date:** 2026-05-07
**Context:** FedRAMP AU-9 (Protection of Audit Information)

## Decision

The `BufferedEmitter` uses a 4096-event in-memory channel as its staging buffer.
If the process terminates abnormally (OOM kill, SIGKILL, kernel panic) before the
flush loop writes events to the Data Storage backend, those buffered events are
irrecoverably lost.

## Rationale

For the current maturity level (v1.5), the in-memory buffer provides:
- Zero-latency non-blocking writes from the request path
- Graceful shutdown flushes all buffered events (covers SIGTERM/SIGINT)
- Overflow metrics and logging provide visibility into buffer pressure

A durable WAL (write-ahead log) or filesystem staging area would address SIGKILL
scenarios but introduces:
- Disk I/O latency on the hot path (~1-5ms per fsync)
- Complexity of replay/deduplication on restart
- Storage volume provisioning and monitoring requirements

## Consequences

- **Accepted risk**: In the event of an ungraceful process termination, up to
  4096 audit events (representing ~20 seconds of sustained activity at flush
  interval) may be lost.
- **Mitigation**: Pod restart policy + health probes minimize the window of
  exposure. The `apifrontend_audit_buffer_overflow_total` metric alerts on
  buffer pressure before loss occurs.
- **Future hardening (FedRAMP Moderate GA)**: Implement WAL-backed staging when
  the service transitions to FedRAMP Moderate authorization boundary. Track in
  issue backlog as a post-GA enhancement.

## References

- FedRAMP Moderate: AU-9 (Protection of Audit Information)
- `internal/audit/buffered.go` — BufferedEmitter implementation
- `deploy/prometheus-rules.yaml` — ApifrontendAuditBufferOverflow alert
