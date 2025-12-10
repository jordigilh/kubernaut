# DD-GATEWAY-011: Shared Status Ownership for Deduplication & Storm Aggregation

**Version**: 1.3
**Created**: 2025-12-07
**Status**: ✅ **APPROVED**
**Confidence**: 95%
**Related ADR**: [ADR-001: Gateway ↔ RO Deduplication Communication](ADR-001-gateway-ro-deduplication-communication.md)
**Related Proposal**: [PROPOSAL: Gateway Redis Deprecation](../../handoff/PROPOSAL_GATEWAY_REDIS_DEPRECATION.md)
**Related BR**: [BR-ORCH-042: Consecutive Failure Blocking](../../requirements/BR-ORCH-042-consecutive-failure-blocking.md)

---

## Executive Summary

This design decision establishes a **shared status ownership pattern** for the `RemediationRequest` CRD, where Gateway owns deduplication and storm aggregation tracking (`status.deduplication`, `status.stormAggregation`) and Remediation Orchestrator owns lifecycle management (`status.overallPhase`, child refs, etc.). This follows the industry-standard Kubernetes pattern used by Node, Pod, Ingress, and many ecosystem projects.

**Key Outcome**: This pattern enables **Redis deprecation** for Gateway, as deduplication and storm state moves to K8s-native RR Status.

---

## Context & Problem

### Original Problem

1. **Spec mutability violation**: Gateway updated `RR.Spec.Deduplication.OccurrenceCount` after creation
2. **Mixed ownership**: RR spec contained both Gateway concerns (dedup) and RO concerns (remediation)
3. **Infinite failure loops**: No mechanism to block repeated failed remediations

### Constraints

| Constraint | Implication |
|------------|-------------|
| **etcd capacity** | Cannot add more CRDs per signal |
| **Signal volume** | Dozens to hundreds during incidents |
| **Audit requirement** | Dedup data must be visible in RR, not hidden in Redis |
| **Current CRD count** | Already 5-6 CRDs per signal (RR, SP, AI, WE, NR) |

---

## Decision

**Move deduplication and storm aggregation from `Spec`/Redis to `Status`** with clear ownership:

| Status Section | Owner | Updates |
|----------------|-------|---------|
| `status.deduplication.*` | **Gateway** | On duplicate signals |
| `status.stormAggregation.*` | **Gateway** | On storm detection |
| `status.overallPhase`, `status.*Ref`, `status.timestamps` | **RO** | On lifecycle transitions |

**This follows the industry-standard Kubernetes pattern and enables Redis deprecation.**

---

## Industry Precedent

| Project | CRD | Multiple Status Owners | Pattern |
|---------|-----|------------------------|---------|
| **Kubernetes** | Node | kubelet, cloud-controller, node-controller | Different `status.*` fields |
| **Kubernetes** | Pod | kubelet, scheduler | Different `status.*` fields |
| **nginx-ingress** | Ingress | User creates, controller updates status | Status owned by controller |
| **cert-manager** | Certificate | cert-manager, ACME solver | Different conditions |
| **Argo Workflows** | Workflow | workflow-controller, executor | Controller → phase, Executor → nodes |
| **Knative** | Service | serving, activator, autoscaler | Different conditions |

---

## Architecture

### Data Flow

```
┌─────────────────────────────────────────────────────────────────────┐
│  GATEWAY SERVICE                                                    │
│                                                                     │
│  1. Signal arrives via webhook                                      │
│  2. Check informer cache: RR with fingerprint exists?               │
│     ├── Yes + in-progress → Update RR.Status.Deduplication         │
│     ├── Yes + blocked → Return 429 (signal blocked)                │
│     ├── Yes + terminal → Check consecutive failures                │
│     │   ├── ≥3 failures → Create RR with phase=Blocked             │
│     │   └── <3 failures → Create new RR                            │
│     └── No → Create new RR                                         │
│                                                                     │
│  OWNS: status.deduplication.*                                       │
│  READS: status.overallPhase (to determine if in-progress/terminal) │
│                                                                     │
└─────────────────────────────────────────────────────────────────────┘
                              │
                              │ Creates RR / Updates status.deduplication
                              ▼
┌─────────────────────────────────────────────────────────────────────┐
│  REMEDIATION ORCHESTRATOR                                           │
│                                                                     │
│  1. Watches RR for new/updated resources                           │
│  2. Creates child CRDs (SP, AI, WE, NR)                            │
│  3. Updates RR.Status (phase, refs, timestamps)                    │
│  4. Sets phase=Blocked if consecutive failures detected            │
│                                                                     │
│  OWNS: status.overallPhase, status.*Ref, status.timestamps, etc.   │
│  READS: status.deduplication (for logging/metrics)                 │
│                                                                     │
└─────────────────────────────────────────────────────────────────────┘
```

---

## Schema Definition

### RemediationRequest Status (Updated)

```yaml
apiVersion: kubernaut.ai/v1alpha1
kind: RemediationRequest
metadata:
  name: rr-abc123-1733578200
  namespace: production
spec:
  # === IMMUTABLE (set by Gateway at creation) ===
  fingerprint: "abc123def456..."
  signalName: "KubePodCrashLooping"
  # ... other spec fields

status:
  # ╔════════════════════════════════════════════════════════════════╗
  # ║  GATEWAY-OWNED SECTION                                         ║
  # ║  Updated by: Gateway Service                                   ║
  # ║  When: On duplicate signal detection                          ║
  # ╚════════════════════════════════════════════════════════════════╝
  deduplication:
    occurrenceCount: 5
    firstOccurrence: "2025-12-07T10:00:00Z"
    lastOccurrence: "2025-12-07T10:05:00Z"

  stormAggregation:                              # NEW - also Gateway-owned
    isStorm: true
    stormType: "rate"                            # "rate" or "pattern"
    aggregatedCount: 15
    windowStart: "2025-12-07T10:00:00Z"
    windowEnd: "2025-12-07T10:01:00Z"
    aggregatedFingerprints: []                   # For pattern storms

  # ╔════════════════════════════════════════════════════════════════╗
  # ║  RO-OWNED SECTION                                              ║
  # ║  Updated by: Remediation Orchestrator                         ║
  # ║  When: On lifecycle transitions                               ║
  # ╚════════════════════════════════════════════════════════════════╝
  overallPhase: "Processing"
  outcome: ""
  message: ""
  requiresManualReview: false

  # Child CRD references
  signalProcessingRef: { name: "sp-abc123", namespace: "production" }
  aiAnalysisRef: { name: "ai-abc123", namespace: "production" }
  workflowExecutionRef: { name: "we-abc123", namespace: "production" }
  notificationRequestRefs: []

  # Phase timestamps
  processingStartTime: "2025-12-07T10:00:05Z"
  analyzingStartTime: null
  executingStartTime: null
  completionTime: null

  # Observability
  observedGeneration: 1
```

### Go Types

```go
// RemediationRequestStatus defines the observed state
type RemediationRequestStatus struct {
    // ╔════════════════════════════════════════════════════════════════╗
    // ║  GATEWAY-OWNED SECTION                                         ║
    // ╚════════════════════════════════════════════════════════════════╝

    // Deduplication tracks signal occurrence for this remediation.
    // OWNER: Gateway Service
    // +optional
    Deduplication *DeduplicationStatus `json:"deduplication,omitempty"`

    // StormAggregation tracks storm detection for this remediation.
    // OWNER: Gateway Service
    // +optional
    StormAggregation *StormAggregationStatus `json:"stormAggregation,omitempty"`

    // ╔════════════════════════════════════════════════════════════════╗
    // ║  RO-OWNED SECTION                                              ║
    // ╚════════════════════════════════════════════════════════════════╝

    // OverallPhase indicates the current lifecycle phase.
    // OWNER: Remediation Orchestrator
    OverallPhase string `json:"overallPhase,omitempty"`

    // ... other RO-owned fields ...

    // ObservedGeneration reflects the generation observed by the controller.
    ObservedGeneration int64 `json:"observedGeneration,omitempty"`
}

// DeduplicationStatus tracks signal deduplication.
// OWNER: Gateway Service (exclusive write access)
// AUTHORITATIVE: api/remediation/v1alpha1/remediationrequest_types.go
type DeduplicationStatus struct {
    FirstSeenAt     *metav1.Time `json:"firstSeenAt,omitempty"`
    LastSeenAt      *metav1.Time `json:"lastSeenAt,omitempty"`
    OccurrenceCount int32        `json:"occurrenceCount,omitempty"`
}

// StormAggregationStatus tracks storm detection.
// OWNER: Gateway Service (exclusive write access)
// AUTHORITATIVE: api/remediation/v1alpha1/remediationrequest_types.go
type StormAggregationStatus struct {
    // IsPartOfStorm indicates if this signal is part of a detected storm
    IsPartOfStorm bool `json:"isPartOfStorm,omitempty"`

    // StormID is the unique identifier for the storm this signal belongs to
    StormID string `json:"stormId,omitempty"`

    // AggregatedCount is the number of signals aggregated in this storm
    AggregatedCount int32 `json:"aggregatedCount,omitempty"`

    // StormDetectedAt is when the storm was first detected
    StormDetectedAt *metav1.Time `json:"stormDetectedAt,omitempty"`

    // AggregatedFingerprints lists unique fingerprints for pattern storms
    // +optional
    AggregatedFingerprints []string `json:"aggregatedFingerprints,omitempty"`
}
```

---

## Implementation

### Gateway: Deduplication Update

```go
func (g *Gateway) UpdateDeduplication(ctx context.Context, rr *RemediationRequest) error {
    return retry.RetryOnConflict(retry.DefaultRetry, func() error {
        // Refetch to get latest resourceVersion
        if err := g.client.Get(ctx, client.ObjectKeyFromObject(rr), rr); err != nil {
            return err
        }

        // Update ONLY Gateway-owned fields
        now := metav1.Now()
        if rr.Status.Deduplication == nil {
            rr.Status.Deduplication = &DeduplicationStatus{
                FirstSeenAt:     &now,
                OccurrenceCount: 1,
            }
        } else {
            rr.Status.Deduplication.OccurrenceCount++
        }
        rr.Status.Deduplication.LastSeenAt = &now

        return g.client.Status().Update(ctx, rr)
    })
}
```

### RO: Lifecycle Update (Preserving Gateway Data)

```go
func (r *Reconciler) UpdatePhase(ctx context.Context, rr *RemediationRequest, phase string) error {
    return retry.RetryOnConflict(retry.DefaultRetry, func() error {
        // Refetch to get latest (including Gateway's deduplication)
        if err := r.client.Get(ctx, client.ObjectKeyFromObject(rr), rr); err != nil {
            return err
        }

        // Update ONLY RO-owned fields (deduplication preserved automatically)
        rr.Status.OverallPhase = phase
        rr.Status.ObservedGeneration = rr.Generation

        return r.client.Status().Update(ctx, rr)
    })
}
```

---

## Consequences

### Positive

| Benefit | Impact |
|---------|--------|
| **Zero additional CRDs** | No etcd pressure increase |
| **Industry-standard pattern** | Follows Node, Ingress, Argo, Knative |
| **Spec immutability** | Gateway never updates spec after creation |
| **Audit trail** | Dedup + storm data visible in RR |
| **Redis deprecation** | Enables removal of Redis dependency |
| **Minimal change** | Validates existing architecture is sound |

### Neutral

| Change | Impact |
|--------|--------|
| **Gateway needs informers** | Required for efficient dedup checks |
| **Conflict retries** | Standard K8s pattern |

---

## Related Documents

| Document | Purpose |
|----------|---------|
| [ADR-001](ADR-001-gateway-ro-deduplication-communication.md) | Full decision record with all options |
| [BR-GATEWAY-181-185](../../requirements/BR-GATEWAY-181-185-shared-status-deduplication.md) | Gateway business requirements |
| [BR-ORCH-038](../../requirements/BR-ORCH-038-preserve-gateway-deduplication.md) | RO business requirement |
| [DD-GATEWAY-009](DD-GATEWAY-009-state-based-deduplication.md) | Previous deduplication design |
| [Implementation Plan](../../services/stateless/gateway-service/implementation/plans/DD_GATEWAY_011_IMPLEMENTATION_PLAN_V1.0.md) | 8-day implementation plan |

---

## Changelog

| Version | Date | Changes |
|---------|------|---------|
| 1.2 | 2025-12-07 | Added link to implementation plan |
| 1.1 | 2025-12-07 | Added storm aggregation to Gateway-owned status section |
| 1.0 | 2025-12-07 | Initial design (replaces rejected SignalIngestion approach) |

