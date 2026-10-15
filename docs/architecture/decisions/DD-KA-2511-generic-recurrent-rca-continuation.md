# DD-KA-2511: Generic Recurrent RCA Continuation

**Status**: Authoritative; implementation complete pending runtime E2E capacity recovery
**Date**: 2026-10-14
**Issues**: #2510, #2511
**Authority**: BR-KA-016, BR-AUDIT-005, DD-KA-016. This decision supersedes the Phase 1/RCA timing statements in DD-KA-2496 v1.2 and ADR-055 v2.1; their target-scoping, handoff and Phase 3 contracts remain authoritative.

## Decision

When RCA identifies a target with prior target-scoped remediation history, KA
automatically supplies that history before final RCA completion. The first
`submit_result` is provisional when history is available; KA adds a sanitized,
server-owned representation of that provisional RCA to the same conversation,
then continues with the target-scoped history review. If the Agent changes the
target, KA re-enriches the new target and continues. Target changes are not
escalation conditions.

The continuation uses the existing RCA `maxTurns`, per-tool anomaly budgets,
cancellation and existing validation/review gates. No fixed target-change or
investigation-step threshold is introduced. Per-investigation enrichment caching
prevents duplicate target lookups.

The contract is generic across remediation types. OOM/informer growth is only a
regression fixture. History includes server-derived applied-state/spec-change,
effectiveness, resolution, hash, health/alert/metric and prior limitation data.
The final RCA may record `causal_mechanism_status` as `verified`, `probable` or
`unknown`.

Required history acquisition failures use the existing observable review/fail-
closed path; no fabricated history or implicit successful lookup is allowed.

## Alternatives

1. **Signal-target pre-injection — rejected**: can provide the wrong resource's
   history and conflicts with ADR-055 target-scoped enrichment.
2. **Fixed two-pass/target limit — rejected**: introduces an unapproved policy and
   can stop valid investigation after a correct target change.
3. **Server-assisted continuation under existing budgets — selected**: preserves
   target correctness and existing safety limits.

## Controls

FedRAMP SI-10, AC-4, AU-2/AU-3; SOC 2 CC7.2; OWASP ASVS v5 V2.2.1–V2.3.1 and
V16.2.x. SOC 2 CC8.1 applies to the implementation change process only.

Production comments and tests implementing this decision must reference DD-KA-2511.
