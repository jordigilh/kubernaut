# ADR-006: AF-to-KA Communication Pattern

**Status:** Accepted
**Date:** 2026-05-02
**Deciders:** AF team
**Source:** Preflight check (KA REST API analysis)

## Context

AF needs to communicate with KA to initiate investigations and track progress. KA exposes a REST API for session management. We need to decide how AF consumes KA's investigation progress.

## Decision

AF communicates with KA via REST API polling:
- `POST /api/v1/incident/analyze` — start investigation
- `GET /api/v1/incident/session/{id}` — poll status
- `GET /api/v1/incident/session/{id}/result` — get completed result

AF synthesizes SSE events for clients from a combination of:
1. KA REST poll responses (investigation progress)
2. CRD watches on RR/AA (pipeline state changes)

## Consequences

- Simple integration: standard HTTP client, no protocol complexity
- AF controls poll frequency (configurable, circuit breaker on failures)
- SSE events to clients are synthesized by AF (not forwarded from KA)
- Slight latency added by polling interval (mitigated by CRD watch for state changes)
- No dependency on KA implementing SSE (which is deferred in KA)
- Circuit breaker protects AF from KA unavailability

## Alternatives Considered

| Alternative | Why rejected |
|-------------|-------------|
| KA SSE streaming | KA doesn't implement SSE yet (deferred); would tightly couple AF to KA's streaming format |
| Webhook (KA → AF) | Requires KA to know AF's address; complicates NetworkPolicy; inverts control |
| CRD watch only | AA CRD doesn't have fine-grained tool-call-level progress; only phase transitions |
| gRPC streaming | KA is REST-only; adding gRPC increases complexity on both sides |
