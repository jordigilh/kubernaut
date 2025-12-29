# BR-GATEWAY-181 to BR-GATEWAY-185: Shared Status Deduplication

**Service**: Gateway Service
**Category**: V1.0 Core Requirements
**Priority**: P0 (CRITICAL)
**Version**: 1.1
**Date**: 2025-12-10
**Status**: 🚧 In Progress
**Related ADR**: [ADR-001: Gateway ↔ RO Deduplication Communication](../architecture/decisions/ADR-001-gateway-ro-deduplication-communication.md)
**Related DD**: [DD-GATEWAY-011: Shared Status Deduplication](../architecture/decisions/DD-GATEWAY-011-shared-status-deduplication.md)

---

## Overview

These business requirements implement the shared status ownership pattern from ADR-001, where Gateway owns `RemediationRequest.Status.Deduplication` and updates it on duplicate signal detection.

**Key Change**: Deduplication data moves from `Spec` (immutable) to `Status` (Gateway-owned section).

---

## BR-GATEWAY-181: Status-Based Deduplication Tracking

### Description

Gateway MUST track signal deduplication by updating `RemediationRequest.Status.Deduplication` instead of `Spec.Deduplication`.

### Priority

**P0 (CRITICAL)** - Fixes spec immutability violation

### Rationale

Kubernetes best practice requires that CRD `spec` fields are immutable after creation. The previous design violated this by updating `spec.deduplication.occurrenceCount` on duplicate signals.

### Implementation

```go
// Gateway updates status.deduplication (NOT spec)
rr.Status.Deduplication.OccurrenceCount++
rr.Status.Deduplication.LastOccurrence = metav1.Now()
err := client.Status().Update(ctx, rr)
```

### Acceptance Criteria

| ID | Criterion | Test Coverage |
|----|-----------|---------------|
| AC-181-1 | Gateway NEVER updates `RR.Spec` after creation | Unit |
| AC-181-2 | Gateway updates `RR.Status.Deduplication.OccurrenceCount` on duplicates | Unit |
| AC-181-3 | Gateway updates `RR.Status.Deduplication.LastOccurrence` on duplicates | Unit |
| AC-181-4 | Gateway sets `RR.Status.Deduplication.FirstOccurrence` on first duplicate | Unit |
| AC-181-5 | Deduplication data persists with RR (visible in `kubectl get rr -o yaml`) | Integration |

---

## BR-GATEWAY-182: Optimistic Concurrency for Status Updates

### Description

Gateway MUST handle concurrent status updates using Kubernetes optimistic concurrency (resourceVersion) and retry on conflict.

### Priority

**P0 (CRITICAL)** - Required for shared status ownership

### Rationale

Since both Gateway and RO update `RemediationRequest.Status`, conflicts may occur. Gateway must use `resourceVersion` for optimistic locking and retry on HTTP 409 Conflict.

### Implementation

```go
import "k8s.io/client-go/util/retry"

func (g *Gateway) UpdateDeduplication(ctx context.Context, rr *RemediationRequest) error {
    return retry.RetryOnConflict(retry.DefaultRetry, func() error {
        // Refetch to get latest resourceVersion
        if err := g.client.Get(ctx, client.ObjectKeyFromObject(rr), rr); err != nil {
            return err
        }

        // Update ONLY Gateway-owned fields
        rr.Status.Deduplication.OccurrenceCount++
        rr.Status.Deduplication.LastOccurrence = metav1.Now()

        return g.client.Status().Update(ctx, rr)
    })
}
```

### Acceptance Criteria

| ID | Criterion | Test Coverage |
|----|-----------|---------------|
| AC-182-1 | Gateway retries status update on HTTP 409 Conflict | Unit |
| AC-182-2 | Gateway refetches RR before retry to get latest resourceVersion | Unit |
| AC-182-3 | Gateway uses exponential backoff for retries | Unit |
| AC-182-4 | Gateway preserves RO-owned status fields during update | Unit |
| AC-182-5 | Concurrent Gateway + RO updates don't lose data | Integration |

---

## BR-GATEWAY-183: Informer-Based RR Status Reads

### Description

Gateway MUST use Kubernetes informers (controller-runtime cache) for efficient reads of `RemediationRequest.Status.OverallPhase` when determining deduplication decisions.

### Priority

**P1 (HIGH)** - Performance optimization

### Rationale

During incidents, Gateway may receive 100+ signals/second. Direct API calls (~10ms each) would saturate the API server. Informer cache provides ~100ns lookups.

### Implementation

```go
// Gateway uses controller-runtime manager with informers
mgr, _ := ctrl.NewManager(cfg, ctrl.Options{
    Scheme:         scheme,
    LeaderElection: false,
})

// Wait for cache sync before accepting requests
mgr.GetCache().WaitForCacheSync(ctx)

// Use cached client for reads
rrList := &RemediationRequestList{}
g.client.List(ctx, rrList,
    client.MatchingLabels{"kubernaut.ai/fingerprint": fingerprint},
)
```

### Acceptance Criteria

| ID | Criterion | Test Coverage |
|----|-----------|---------------|
| AC-183-1 | Gateway uses controller-runtime manager with informers | Unit |
| AC-183-2 | Gateway waits for cache sync before accepting requests | Unit |
| AC-183-3 | Gateway reads RR status from informer cache (not direct API) | Unit |
| AC-183-4 | Deduplication decision latency <10ms (P99) | Integration |
| AC-183-5 | Gateway handles cache sync failure gracefully | Unit |

---

## BR-GATEWAY-184: Consecutive Failure Blocking

> ⛔ **SUPERSEDED (2025-12-10)**: This requirement has been moved to RO as **BR-ORCH-042**.
> Gateway should NOT count consecutive failures or create Blocked RRs.
> See: [BR-ORCH-042](BR-ORCH-042-consecutive-failure-blocking.md), [DD-GATEWAY-011 v1.3](../architecture/decisions/DD-GATEWAY-011-shared-status-deduplication.md)

### Description (SUPERSEDED)

~~Gateway MUST detect consecutive remediation failures for the same signal fingerprint and create new `RemediationRequest` with `OverallPhase=Blocked` when ≥3 consecutive failures are detected.~~

**New Design**: RO owns blocking logic. Gateway only checks if an active (non-terminal) RR exists.

### Priority

~~**P0 (CRITICAL)** - Prevents infinite failure loops~~
**N/A** - Superseded by BR-ORCH-042

### Rationale

If a signal repeatedly fails remediation due to external factors (e.g., missing RBAC), the system would create infinite RRs. Gateway must detect this pattern and block further attempts until manual intervention.

### Implementation

```go
func (g *Gateway) ShouldBlock(ctx context.Context, fingerprint string) bool {
    rrList := &RemediationRequestList{}
    g.client.List(ctx, rrList,
        client.MatchingLabels{"kubernaut.ai/fingerprint": fingerprint},
    )

    consecutiveFailures := 0
    for _, rr := range rrList.Items {
        switch rr.Status.OverallPhase {
        case "Failed":
            consecutiveFailures++
        case "Completed":
            consecutiveFailures = 0  // Reset on success
        case "Blocked":
            return true  // Already blocked
        }
    }

    return consecutiveFailures >= 3
}
```

### Acceptance Criteria

| ID | Criterion | Test Coverage |
|----|-----------|---------------|
| AC-184-1 | Gateway counts consecutive Failed RRs for same fingerprint | Unit |
| AC-184-2 | Gateway resets failure count on Completed RR | Unit |
| AC-184-3 | Gateway creates RR with `OverallPhase=Blocked` when ≥3 failures | Unit |
| AC-184-4 | Gateway returns 429 for signals with blocked fingerprint | Unit |
| AC-184-5 | Blocked signals are logged with fingerprint and failure count | Unit |
| AC-184-6 | Manual intervention (delete/update RR) unblocks signal | Integration |

---

## BR-GATEWAY-185: Field Selector for RR Lookup

> **⚠️ UPDATED (v1.1, 2025-12-10)**: Changed from label-based to `spec.signalFingerprint` field selector.

### Description

Gateway MUST use Kubernetes field selectors on **`spec.signalFingerprint`** (not labels) for efficient lookup of `RemediationRequest` by fingerprint.

### Priority

**P1 (HIGH)** - Performance optimization + Data integrity

### Rationale

| Aspect | Label-Based (v1.0 - ❌ Deprecated) | Field Selector (v1.1 - ✅ Required) |
|--------|-----------------------------------|-------------------------------------|
| **Field** | `metadata.labels.kubernaut.ai/signal-fingerprint` | `spec.signalFingerprint` |
| **Length** | 63 chars (K8s label limit) | **64 chars (full SHA256)** |
| **Mutability** | ⚠️ Mutable (can be changed) | ✅ **Immutable** (kubebuilder validation) |
| **Source** | Copy of fingerprint | ✅ **Authoritative source of truth** |
| **Collision Risk** | Possible if fingerprints differ in 64th char | ✅ **None** |

**Why the change?**
1. **Data integrity**: `spec.signalFingerprint` is immutable; labels are mutable
2. **Full precision**: Spec field supports full 64-char SHA256; labels truncate to 63
3. **Authoritative**: Spec is the source of truth; labels are copies that can drift
4. **Same pattern as WE**: WorkflowExecution uses `spec.targetResource` field selector

### Implementation

```go
// SetupWithManager - create field index on spec.signalFingerprint
func SetupWithManager(mgr ctrl.Manager) error {
    // BR-GATEWAY-185 v1.1: Index on spec.signalFingerprint (not labels)
    if err := mgr.GetFieldIndexer().IndexField(
        context.Background(),
        &remediationv1.RemediationRequest{},
        "spec.signalFingerprint",  // Immutable, full 64-char fingerprint
        func(obj client.Object) []string {
            rr := obj.(*remediationv1.RemediationRequest)
            return []string{rr.Spec.SignalFingerprint}
        },
    ); err != nil {
        return fmt.Errorf("failed to create field index on spec.signalFingerprint: %w", err)
    }
    // ...
}

// Use field selector for efficient O(1) lookup
func (g *Gateway) findActiveRR(ctx context.Context, fingerprint string) *remediationv1.RemediationRequest {
    rrList := &remediationv1.RemediationRequestList{}

    // BR-GATEWAY-185 v1.1: Use field selector on immutable spec field
    g.client.List(ctx, rrList,
        client.MatchingFields{"spec.signalFingerprint": fingerprint}, // Full 64-char fingerprint
    )

    for _, rr := range rrList.Items {
        if !phase.IsTerminal(phase.Phase(rr.Status.OverallPhase)) {
            return &rr
        }
    }
    return nil
}
```

### Acceptance Criteria

| ID | Criterion | Test Coverage |
|----|-----------|---------------|
| AC-185-1 | Gateway/RO sets up field index on `spec.signalFingerprint` | Unit |
| AC-185-2 | Gateway/RO uses `client.MatchingFields` (not `MatchingLabels`) | Unit |
| AC-185-3 | Lookup uses full 64-char fingerprint (no truncation) | Unit |
| AC-185-4 | Lookup returns only RRs with matching fingerprint | Unit |
| AC-185-5 | Lookup performance <5ms for 1000 RRs | Integration |

### Version History

| Version | Date | Changes |
|---------|------|---------|
| 1.0 | 2025-12-07 | Initial - label-based lookup |
| 1.1 | 2025-12-10 | Changed to `spec.signalFingerprint` field selector (immutable, full 64-char) |

---

## Test Scenarios

```gherkin
Scenario: Status-based deduplication tracking
  Given RemediationRequest "rr-1" exists with fingerprint "abc123"
  And rr-1.Status.Deduplication.OccurrenceCount = 1
  When Gateway receives duplicate signal with fingerprint "abc123"
  Then Gateway should update rr-1.Status.Deduplication.OccurrenceCount to 2
  And Gateway should update rr-1.Status.Deduplication.LastOccurrence
  And Gateway should NOT modify rr-1.Spec

Scenario: Conflict handling on concurrent updates
  Given RemediationRequest "rr-1" exists
  And RO is updating rr-1.Status.OverallPhase concurrently
  When Gateway attempts to update rr-1.Status.Deduplication
  And receives HTTP 409 Conflict
  Then Gateway should refetch rr-1
  And Gateway should retry the status update
  And Gateway should preserve RO's phase update

Scenario: Consecutive failure blocking
  Given fingerprint "abc123" has 3 consecutive Failed RemediationRequests
  When Gateway receives new signal with fingerprint "abc123"
  Then Gateway should create RemediationRequest with OverallPhase="Blocked"
  And Gateway should return 429 for subsequent signals with "abc123"

Scenario: Block reset on success
  Given fingerprint "abc123" has 2 consecutive Failed RemediationRequests
  When RemediationRequest "rr-3" with fingerprint "abc123" completes successfully
  And Gateway receives new signal with fingerprint "abc123"
  Then Gateway should create normal RemediationRequest (not blocked)
```

---

## Metrics

```prometheus
# Counter for status updates
gateway_deduplication_status_updates_total{
  result="success|conflict_retry|error"
}

# Counter for blocked signals
gateway_signals_blocked_total{
  fingerprint="<fingerprint>",
  consecutive_failures="<count>"
}

# Histogram for deduplication check latency
gateway_deduplication_check_duration_seconds{
  source="informer_cache|api_direct"
}
```

---

## Related Documents

| Document | Purpose |
|----------|---------|
| [ADR-001](../architecture/decisions/ADR-001-gateway-ro-deduplication-communication.md) | Architecture decision record |
| [DD-GATEWAY-011](../architecture/decisions/DD-GATEWAY-011-shared-status-deduplication.md) | Design decision details |
| [DD-GATEWAY-009](../architecture/decisions/DD-GATEWAY-009-state-based-deduplication.md) | Previous deduplication design (updated) |
| [NOTICE_SHARED_STATUS_OWNERSHIP](../handoff/NOTICE_SHARED_STATUS_OWNERSHIP_DD_SI_001.md) | Team notification |

---

## Changelog

| Version | Date | Changes |
|---------|------|---------|
| 1.0 | 2025-12-07 | Initial BRs created based on ADR-001 |

---

**Document Version**: 1.0
**Last Updated**: December 7, 2025
**Maintained By**: Kubernaut Architecture Team

