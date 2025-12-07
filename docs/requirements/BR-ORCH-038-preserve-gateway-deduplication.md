# BR-ORCH-038: Preserve Gateway Deduplication Data

**Service**: RemediationOrchestrator Controller
**Category**: V1.0 Core Requirements
**Priority**: P0 (CRITICAL)
**Version**: 1.0
**Date**: 2025-12-07
**Status**: 🚧 Planned
**Related ADR**: [ADR-001: Gateway ↔ RO Deduplication Communication](../architecture/decisions/ADR-001-gateway-ro-deduplication-communication.md)
**Related DD**: [DD-GATEWAY-011: Shared Status Deduplication](../architecture/decisions/DD-GATEWAY-011-shared-status-deduplication.md)
**Related Gateway BRs**: BR-GATEWAY-181 to BR-GATEWAY-185

---

## Overview

This business requirement ensures that Remediation Orchestrator preserves Gateway-owned `Status.Deduplication` data when updating its own status fields. This is critical for the shared status ownership pattern defined in ADR-001.

---

## BR-ORCH-038: Preserve Gateway Deduplication Data

### Description

RemediationOrchestrator MUST preserve `RemediationRequest.Status.Deduplication` when updating its owned status fields (`OverallPhase`, child refs, timestamps, etc.).

### Priority

**P0 (CRITICAL)** - Required for shared status ownership pattern

### Rationale

The shared status ownership pattern (ADR-001) divides `RemediationRequest.Status` between two controllers:

| Status Section | Owner |
|----------------|-------|
| `status.deduplication.*` | Gateway |
| `status.overallPhase`, `status.*Ref`, `status.*Time` | RO |

When RO updates its fields, it must **never overwrite** Gateway's deduplication data. This is ensured by:
1. Using `client.Status().Update()` which merges, not replaces
2. Refetching the RR before updates to get latest deduplication
3. Only modifying RO-owned fields

### Implementation

```go
func (r *Reconciler) UpdatePhase(ctx context.Context, rr *RemediationRequest, phase string) error {
    return retry.RetryOnConflict(retry.DefaultRetry, func() error {
        // CRITICAL: Refetch to get latest (including Gateway's deduplication updates)
        if err := r.client.Get(ctx, client.ObjectKeyFromObject(rr), rr); err != nil {
            return err
        }

        // Update ONLY RO-owned fields
        // Gateway's status.deduplication is preserved automatically
        rr.Status.OverallPhase = phase
        rr.Status.ObservedGeneration = rr.Generation

        // Set phase timestamp
        now := metav1.Now()
        switch phase {
        case "Processing":
            rr.Status.ProcessingStartTime = &now
        // ...
        }

        // IMPORTANT: Do NOT modify rr.Status.Deduplication
        return r.client.Status().Update(ctx, rr)
    })
}
```

### Acceptance Criteria

| ID | Criterion | Test Coverage |
|----|-----------|---------------|
| AC-038-1 | RO refetches RR before every status update | Unit |
| AC-038-2 | RO NEVER modifies `status.deduplication.*` fields | Unit |
| AC-038-3 | Gateway's deduplication data survives RO phase transitions | Integration |
| AC-038-4 | RO uses optimistic concurrency (retry on conflict) | Unit |
| AC-038-5 | Concurrent Gateway + RO updates preserve both data sets | Integration |
| AC-038-6 | Final RR shows correct `occurrenceCount` after full lifecycle | E2E |

---

## Test Scenarios

```gherkin
Scenario: RO preserves Gateway deduplication during phase transition
  Given RemediationRequest "rr-1" exists with:
    | status.deduplication.occurrenceCount | 5 |
    | status.overallPhase | Pending |
  When RO updates phase to "Processing"
  Then status.deduplication.occurrenceCount should still be 5
  And status.overallPhase should be "Processing"

Scenario: Concurrent updates preserve both data sets
  Given RemediationRequest "rr-1" exists with:
    | status.deduplication.occurrenceCount | 3 |
    | status.overallPhase | Processing |
  When Gateway updates deduplication (occurrenceCount → 4)
  And RO updates phase (Processing → Analyzing) concurrently
  Then final status should have:
    | status.deduplication.occurrenceCount | 4 |
    | status.overallPhase | Analyzing |

Scenario: Full lifecycle preserves deduplication history
  Given signal arrives and Gateway creates RR
  And 3 duplicate signals arrive during processing
  When RO completes full lifecycle (Processing → Analyzing → Executing → Completed)
  Then final RR.Status.Deduplication.OccurrenceCount should be 4
  And RR.Status.Deduplication.FirstOccurrence should be original timestamp
  And RR.Status.Deduplication.LastOccurrence should be last duplicate timestamp
```

---

## Anti-Patterns to Avoid

### ❌ DON'T: Overwrite entire status

```go
// WRONG: This would lose Gateway's deduplication data
rr.Status = RemediationRequestStatus{
    OverallPhase: "Completed",
}
client.Status().Update(ctx, rr)
```

### ❌ DON'T: Use stale data

```go
// WRONG: Using stale RR without refetch
rr.Status.OverallPhase = "Analyzing"
client.Status().Update(ctx, rr)  // May conflict or overwrite recent Gateway updates
```

### ✅ DO: Refetch and modify only owned fields

```go
// CORRECT: Refetch, modify only RO-owned fields
client.Get(ctx, key, rr)  // Get latest including Gateway's updates
rr.Status.OverallPhase = "Analyzing"
client.Status().Update(ctx, rr)
```

---

## Observability

### Logs

```json
{
  "level": "debug",
  "msg": "Preserving Gateway deduplication data during status update",
  "rr": "rr-abc123",
  "occurrenceCount": 5,
  "newPhase": "Analyzing"
}
```

### Metrics

```prometheus
# Counter for status updates that preserved deduplication
ro_status_updates_total{
  preserved_deduplication="true|false"
}

# Counter for conflict retries
ro_status_update_conflicts_total{
  phase="Processing|Analyzing|..."
}
```

---

## Related Documents

| Document | Purpose |
|----------|---------|
| [ADR-001](../architecture/decisions/ADR-001-gateway-ro-deduplication-communication.md) | Architecture decision |
| [DD-GATEWAY-011](../architecture/decisions/DD-GATEWAY-011-shared-status-deduplication.md) | Design details |
| [BR-GATEWAY-181-185](./BR-GATEWAY-181-185-shared-status-deduplication.md) | Gateway's side of the pattern |
| [BR-ORCH-036](./BR-ORCH-036-manual-review-notification.md) | Manual review on failures |

---

## Changelog

| Version | Date | Changes |
|---------|------|---------|
| 1.0 | 2025-12-07 | Initial BR created based on ADR-001 |

---

**Document Version**: 1.0
**Last Updated**: December 7, 2025
**Maintained By**: Kubernaut Architecture Team

