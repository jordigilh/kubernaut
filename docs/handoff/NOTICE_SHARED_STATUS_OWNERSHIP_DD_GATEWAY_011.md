# NOTICE: Shared Status Ownership Pattern (DD-GATEWAY-011 v1.1)

**Date**: December 7, 2025
**From**: Architecture Team
**To**: Gateway Service Team, Remediation Orchestrator Team
**Priority**: 🔴 HIGH (Architectural Decision - Requires Implementation Changes)
**Status**: ✅ **APPROVED**

---

## 📋 Summary

This notice describes an approved architectural pattern for Gateway ↔ RO communication regarding deduplication and storm aggregation tracking.

**Key Decision**: Use a **shared status ownership pattern** where Gateway and RO each own distinct sections of `RemediationRequest.Status`. This follows industry-standard Kubernetes patterns (Node, Ingress, Argo, Knative).

**Key Outcome**: This pattern enables **Redis deprecation** for Gateway (see [PROPOSAL_GATEWAY_REDIS_DEPRECATION.md](PROPOSAL_GATEWAY_REDIS_DEPRECATION.md)).

---

## 🔄 What Changed

### Previous Design (Problematic)

```yaml
spec:
  deduplication:
    occurrenceCount: 5  # ❌ Gateway updated this after creation
    lastOccurrence: "..."  # ❌ VIOLATES SPEC IMMUTABILITY
```

### Approved Design (DD-GATEWAY-011 v1.1)

```yaml
spec:
  # IMMUTABLE - set by Gateway at creation, never updated
  fingerprint: "abc123"
  signalName: "KubePodCrashLooping"
  # ...

status:
  # === GATEWAY-OWNED SECTION ===
  deduplication:
    occurrenceCount: 5
    firstOccurrence: "2025-12-07T10:00:00Z"
    lastOccurrence: "2025-12-07T10:05:00Z"

  stormAggregation:                              # NEW in v1.1
    isStorm: true
    stormType: "rate"
    aggregatedCount: 15
    windowStart: "2025-12-07T10:00:00Z"
    windowEnd: "2025-12-07T10:01:00Z"

  # === RO-OWNED SECTION ===
  overallPhase: "Processing"
  signalProcessingRef: { ... }
  aiAnalysisRef: { ... }
  # ...
```

---

## 🎯 Why This Approach

| Factor | SignalIngestion CRD | Shared Status (Approved) |
|--------|---------------------|--------------------------|
| **Additional CRDs** | +1 per signal | 0 |
| **etcd Impact** | ❌ Increases pressure | ✅ No change |
| **Industry Pattern** | Less common | ✅ Used by Node, Ingress, Argo |
| **Implementation Effort** | Days (new CRD, controller) | Hours (status field move) |
| **Audit Trail** | In separate CRD | ✅ In RR (visible, persists) |
| **Redis Dependency** | Still needed | ✅ Enables Redis removal |

---

## 📊 Ownership Model

| Status Field | Owner | When Updated |
|--------------|-------|--------------|
| `status.deduplication.*` | **Gateway** | On duplicate signal detection |
| `status.stormAggregation.*` | **Gateway** | On storm detection |
| `status.overallPhase` | **RO** | On lifecycle transitions |
| `status.*Ref` | **RO** | When child CRDs created |
| `status.*Time` | **RO** | On phase transitions |
| `status.requiresManualReview` | **RO** | On failure conditions |
| `status.outcome` | **RO** | On completion |

**Rule**: Each controller ONLY updates its owned fields. Use `resourceVersion` for optimistic concurrency.

---

## 🛠️ Gateway Team: Required Changes

### 1. Move Deduplication to Status

**Before** (remove):
```go
// ❌ Don't update spec
rr.Spec.Deduplication.OccurrenceCount++
err := client.Update(ctx, rr)
```

**After** (implement):
```go
// ✅ Update status.deduplication only
rr.Status.Deduplication.OccurrenceCount++
rr.Status.Deduplication.LastOccurrence = metav1.Now()
err := client.Status().Update(ctx, rr)
```

### 2. Move Storm Aggregation to Status (NEW)

**Before** (remove):
```go
// ❌ Don't store in Redis
redis.Set("storm:"+fingerprint, stormData, TTL)
```

**After** (implement):
```go
// ✅ Store in RR status
if rr.Status.StormAggregation == nil {
    rr.Status.StormAggregation = &StormAggregationStatus{
        IsStorm:     true,
        StormType:   "rate",
        WindowStart: metav1.Now(),
    }
}
rr.Status.StormAggregation.AggregatedCount++
rr.Status.StormAggregation.WindowEnd = metav1.Now()
err := client.Status().Update(ctx, rr)
```

### 3. Handle Conflicts with Retry

```go
import "k8s.io/client-go/util/retry"

func (g *Gateway) UpdateDeduplication(ctx context.Context, rr *RemediationRequest) error {
    return retry.RetryOnConflict(retry.DefaultRetry, func() error {
        // Refetch to get latest resourceVersion
        if err := g.client.Get(ctx, client.ObjectKeyFromObject(rr), rr); err != nil {
            return err
        }

        // Update ONLY Gateway-owned fields
        if rr.Status.Deduplication == nil {
            rr.Status.Deduplication = &DeduplicationStatus{
                OccurrenceCount: 1,
                FirstOccurrence: metav1.Now(),
            }
        }
        rr.Status.Deduplication.OccurrenceCount++
        rr.Status.Deduplication.LastOccurrence = metav1.Now()

        return g.client.Status().Update(ctx, rr)
    })
}
```

### 4. Check RR Phase for Deduplication Decision

```go
func (g *Gateway) ShouldDeduplicate(ctx context.Context, fingerprint string) (bool, *RemediationRequest) {
    // List RRs by fingerprint from informer cache
    rrList := &RemediationRequestList{}
    g.client.List(ctx, rrList,
        client.MatchingLabels{"kubernaut.ai/fingerprint": fingerprint},
    )

    for _, rr := range rrList.Items {
        switch rr.Status.OverallPhase {
        case "Completed", "Failed", "Cancelled":
            continue  // Terminal → check next
        case "Blocked":
            return true, &rr  // Blocked → deduplicate
        default:
            return true, &rr  // In-progress → deduplicate
        }
    }

    return false, nil  // No in-progress RR → create new
}
```

### 5. Blocking Logic (Consecutive Failures)

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

---

## 🛠️ RO Team: Required Changes

### 1. Update API Types ✅ **DONE** (2025-12-07)

**COMPLETED**: Types added to `api/remediation/v1alpha1/remediationrequest_types.go`

---

## ✅ RO Team Acknowledgment for Gateway (2025-12-07)

### API Types - AUTHORITATIVE IMPLEMENTATION

**Location**: `api/remediation/v1alpha1/remediationrequest_types.go`

```go
// RemediationRequestStatus - Gateway-owned fields at top
type RemediationRequestStatus struct {
    // ╔════════════════════════════════════════════════════════════════╗
    // ║  GATEWAY-OWNED SECTION (DD-GATEWAY-011)                        ║
    // ║  Gateway Service has exclusive write access to these fields    ║
    // ╚════════════════════════════════════════════════════════════════╝

    // Deduplication tracks signal occurrence for this remediation.
    // OWNER: Gateway Service (exclusive write access)
    // +optional
    Deduplication *DeduplicationStatus `json:"deduplication,omitempty"`

    // StormAggregation tracks storm detection for this remediation.
    // OWNER: Gateway Service (exclusive write access)
    // +optional
    StormAggregation *StormAggregationStatus `json:"stormAggregation,omitempty"`

    // ╔════════════════════════════════════════════════════════════════╗
    // ║  RO-OWNED SECTION                                              ║
    // ╚════════════════════════════════════════════════════════════════╝
    OverallPhase string `json:"overallPhase,omitempty"`
    // ... other RO fields ...
}

// DeduplicationStatus tracks signal occurrence for deduplication.
// OWNER: Gateway Service (exclusive write access)
// Reference: DD-GATEWAY-011, BR-GATEWAY-181
type DeduplicationStatus struct {
    // FirstSeenAt is when this signal fingerprint was first observed
    // +optional
    FirstSeenAt *metav1.Time `json:"firstSeenAt,omitempty"`
    // LastSeenAt is when this signal fingerprint was last observed
    // +optional
    LastSeenAt *metav1.Time `json:"lastSeenAt,omitempty"`
    // OccurrenceCount tracks how many times this signal has been seen
    // +optional
    OccurrenceCount int32 `json:"occurrenceCount,omitempty"`
}

// StormAggregationStatus tracks storm detection for this remediation.
// OWNER: Gateway Service (exclusive write access)
// Reference: DD-GATEWAY-011, DD-GATEWAY-008 v2.0
type StormAggregationStatus struct {
    // IsPartOfStorm indicates if this signal is part of a detected storm
    // +optional
    IsPartOfStorm bool `json:"isPartOfStorm,omitempty"`
    // StormID is the unique identifier for the storm this signal belongs to
    // +optional
    StormID string `json:"stormId,omitempty"`
    // AggregatedCount is the number of signals aggregated in this storm
    // +optional
    AggregatedCount int32 `json:"aggregatedCount,omitempty"`
    // StormDetectedAt is when the storm was first detected
    // +optional
    StormDetectedAt *metav1.Time `json:"stormDetectedAt,omitempty"`
}
```

### Gateway Usage Instructions

**Import path**: `github.com/jordigilh/kubernaut/api/remediation/v1alpha1`

**Example - Update deduplication**:
```go
import (
    remediationv1 "github.com/jordigilh/kubernaut/api/remediation/v1alpha1"
    metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
    "sigs.k8s.io/controller-runtime/pkg/client"
    "k8s.io/client-go/util/retry"
)

func (g *Gateway) UpdateDeduplication(ctx context.Context, rr *remediationv1.RemediationRequest) error {
    return retry.RetryOnConflict(retry.DefaultRetry, func() error {
        // Refetch to get latest resourceVersion
        if err := g.client.Get(ctx, client.ObjectKeyFromObject(rr), rr); err != nil {
            return err
        }

        now := metav1.Now()
        if rr.Status.Deduplication == nil {
            rr.Status.Deduplication = &remediationv1.DeduplicationStatus{
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

### RO Commitments

| Commitment | Status |
|------------|--------|
| API types available for import | ✅ **READY** |
| RO preserves Gateway-owned fields on status updates | ✅ **Committed** (using retry.RetryOnConflict with refetch) |
| 24-hour RR retention before GC | ✅ **Committed** |
| Shorter backoff (10ms) approved for Gateway | ✅ **Approved** |

### Questions? Contact RO Team

---

### 2. Preserve Gateway-Owned Fields on Status Updates

```go
func (r *Reconciler) UpdatePhase(ctx context.Context, rr *RemediationRequest, phase string) error {
    return retry.RetryOnConflict(retry.DefaultRetry, func() error {
        // Refetch to get latest (including Gateway's deduplication/storm updates)
        if err := r.client.Get(ctx, client.ObjectKeyFromObject(rr), rr); err != nil {
            return err
        }

        // Update ONLY RO-owned fields (Gateway fields preserved automatically)
        rr.Status.OverallPhase = phase
        rr.Status.ObservedGeneration = rr.Generation

        return r.client.Status().Update(ctx, rr)
    })
}
```

### 3. Regenerate CRD Manifests

```bash
make generate manifests
```

---

## 📅 Timeline

| Task | Owner | Target |
|------|-------|--------|
| Update RR API types (add DeduplicationStatus, StormAggregationStatus) | RO Team | ✅ **DONE** (2025-12-07) |
| Regenerate CRD manifests | RO Team | Day 7 |
| Move deduplication from spec to status | Gateway Team | Day 7-8 |
| Move storm aggregation from Redis to status | Gateway Team | Day 7-8 |
| Implement conflict retry logic | Gateway Team | Day 7-8 |
| Update blocking logic | Gateway Team | Day 8 |
| Remove Redis dependency (if approved) | Gateway Team | Day 9-10 |
| Integration testing | Both | Day 11-12 |

---

## ✅ Acknowledgment Required

Please acknowledge receipt and confirm implementation plan:

| Team | Acknowledged | Date | Notes |
|------|--------------|------|-------|
| Gateway Service | ✅ **ACKNOWLEDGED** | 2025-12-07 | See detailed response below |
| Remediation Orchestrator | ✅ **ACKNOWLEDGED** | 2025-12-07 | Q3-Q5 answered, ADR-049 compatible |

---

## 🛠️ Gateway Team Response (2025-12-07)

### ✅ **Acknowledgment**

The Gateway team **acknowledges receipt** of DD-GATEWAY-011 v1.1 and **agrees with the proposed architectural pattern**.

**Key Points of Agreement**:
1. ✅ **Spec Immutability**: Current Gateway implementation violates spec immutability by updating `spec.Deduplication` - this should be fixed
2. ✅ **Shared Status Pattern**: Industry-standard pattern (Node, Ingress, Argo, Knative) - good alignment
3. ✅ **Redis Deprecation Potential**: Attractive for reducing infrastructure complexity

---

### 📊 Impact Assessment

#### **Files Requiring Changes**

| File | Current State | Required Change | Effort |
|------|---------------|-----------------|--------|
| `pkg/gateway/processing/crd_updater.go` | Updates `spec.Deduplication.OccurrenceCount` (L109-110) | Change to `status.Deduplication` via `Status().Update()` | 2h |
| `pkg/gateway/processing/deduplication.go` | Reads `spec.Deduplication` (L284-286) | Change to read from `status.Deduplication` | 2h |
| `pkg/gateway/processing/crd_creator.go` | Sets initial `spec.Deduplication` | Initialize `status.Deduplication` on creation | 1h |
| `pkg/gateway/processing/storm_detector.go` | Uses Redis for storm counters | Move to `status.StormAggregation` (if Redis deprecated) | 4h |
| `pkg/gateway/processing/storm_aggregator.go` | Uses Redis for aggregation state | Move to `status.StormAggregation` (if Redis deprecated) | 4h |
| Unit tests | Assert on `spec.Deduplication` | Update to assert on `status.Deduplication` | 3h |
| Integration tests | Verify `spec` updates | Update to verify `status` updates | 3h |

**Total Estimated Effort**: ~19 hours (2-3 days)

---

### ⚠️ Questions Requiring Clarification

#### **Q1: Informer Setup for Status Reads**

**Context**: Gateway is currently a stateless HTTP server using direct K8s client calls. The proposed pattern uses informer-based reads:

```go
// Current: Direct client call
rr, err := client.Get(ctx, key, &RemediationRequest{})

// Proposed: Informer cache read
rr, err := informerCache.Get(ctx, key, &RemediationRequest{})
```

**Question**: Is a full controller-runtime manager required, or can Gateway use a standalone informer? Standalone informer would be simpler:

```go
// Standalone informer (preferred for Gateway's stateless design)
informer := cache.NewSharedIndexInformer(
    &cache.ListWatch{ListFunc: ..., WatchFunc: ...},
    &RemediationRequest{},
    0, // No resync
    cache.Indexers{cache.NamespaceIndex: cache.MetaNamespaceIndexFunc},
)
```

**Impact**: Full manager = significant architectural change; Standalone informer = minimal change.

---

#### **Q2: Conflict Rate and Latency Impact**

**Context**: Gateway has a p95 latency target of <50ms. `retry.RetryOnConflict` adds latency on conflicts.

**Questions**:
1. What's the expected conflict rate between Gateway status updates and RO status updates?
2. Is there telemetry from similar patterns (Node status, Ingress status) we can reference?
3. Should Gateway use a shorter retry backoff for latency sensitivity?

```go
// Custom retry config for latency-sensitive Gateway
gatewayRetry := wait.Backoff{
    Steps:    3,
    Duration: 10 * time.Millisecond,  // Start at 10ms (not default 100ms)
    Factor:   2.0,
    Jitter:   0.1,
}
```

---

#### **Q3: RR Lifecycle and Deduplication Window**

**Context**: Current Gateway uses Redis TTL (5 minutes) for deduplication window. Status-based approach ties deduplication to RR lifecycle.

**Questions**:
1. What's the expected RR retention period before cleanup?
2. How does RO handle RR garbage collection?
3. Should Gateway check `status.OverallPhase` for terminal states, or should RO set a `status.deduplicationExpired: true` flag?

**Current logic**:
```go
// Redis TTL-based (current)
if redis.Exists("dedup:" + fingerprint) {
    return DUPLICATE
}

// RR lifecycle-based (proposed)
rrList := client.List(ctx, &RRList{}, MatchingLabels{"fingerprint": fp})
for _, rr := range rrList.Items {
    if !isTerminalPhase(rr.Status.OverallPhase) {
        return DUPLICATE, &rr
    }
}
return NEW_SIGNAL, nil
```

---

#### **Q4: Storm Aggregation - First Alert Handling**

**Context**: DD-GATEWAY-008 specifies that for rate-based storms (>10 alerts/minute), the first alert creates an RR immediately, then subsequent alerts update `status.StormAggregation`.

**Question**: Confirm this flow:
1. Alert 1: Create RR with `status.StormAggregation.AggregatedCount = 1`
2. Alerts 2-10: Update `status.StormAggregation.AggregatedCount++`
3. Alert 11: Storm detected → Set `status.StormAggregation.IsStorm = true`
4. Alerts 12+: Continue incrementing until storm window closes

**Or** should we:
1. Buffer alerts in Redis until storm threshold
2. Create single aggregated RR when storm confirmed
3. Then use status for subsequent tracking

---

### 📅 Gateway Implementation Plan

**✅ UNBLOCKED**: RO team has added API types (`DeduplicationStatus`, `StormAggregationStatus`) to `RemediationRequestStatus` (2025-12-07)

| Phase | Task | Days | Dependency |
|-------|------|------|------------|
| **Phase 1** | Move deduplication from spec to status | Day 7-8 | RO API types ready |
| **Phase 2** | Implement conflict retry logic | Day 8 | Phase 1 |
| **Phase 3** | Update unit tests | Day 8-9 | Phase 2 |
| **Phase 4** | Move storm aggregation to status | Day 9-10 | Phase 1 |
| **Phase 5** | Update integration tests | Day 10-11 | Phase 4 |
| **Phase 6** | Redis deprecation (if approved) | Day 12-15 | All phases + Redis deprecation approval |

**Note**: Redis deprecation is a separate decision (see [PROPOSAL_GATEWAY_REDIS_DEPRECATION](PROPOSAL_GATEWAY_REDIS_DEPRECATION.md)). Phases 1-5 can proceed independently.

---

### 🔄 Redis Deprecation Review

Gateway team will review [PROPOSAL_GATEWAY_REDIS_DEPRECATION](PROPOSAL_GATEWAY_REDIS_DEPRECATION.md) and provide response by **Day 8**.

**Initial Assessment**:
- ✅ **Pro**: Reduces infrastructure complexity (no Redis HA required)
- ✅ **Pro**: Deduplication data persists in RR (audit trail)
- ⚠️ **Con**: K8s API dependency for every deduplication check
- ⚠️ **Con**: Storm detection real-time counters may have higher latency
- ❓ **TBD**: Performance impact on high-volume deployments

---

## 🔗 Related Documents

| Document | Purpose |
|----------|---------|
| [ADR-001](../architecture/decisions/ADR-001-gateway-ro-deduplication-communication.md) | Full architectural decision record |
| [DD-GATEWAY-011](../architecture/decisions/DD-GATEWAY-011-shared-status-deduplication.md) | Design decision details |
| [PROPOSAL_GATEWAY_REDIS_DEPRECATION](PROPOSAL_GATEWAY_REDIS_DEPRECATION.md) | Redis removal proposal |
| [BR-GATEWAY-181-185](../requirements/BR-GATEWAY-181-185-shared-status-deduplication.md) | Gateway BRs |
| [BR-ORCH-038](../requirements/BR-ORCH-038-preserve-gateway-deduplication.md) | RO must preserve Gateway data |

---

## ❓ Questions

Please add questions here:

1. **Gateway Team**: Any concerns about informer setup for status reads?
   - **Gateway Response (2025-12-07)**: See Q1 in Gateway Team Response section. Preference for standalone informer over full controller-runtime manager.
   - **Architecture Response (2025-12-07)**: ✅ **ANSWERED** - Standalone informer approved for V1.0. See Architecture Team Response section.

2. **Gateway Team**: See [PROPOSAL_GATEWAY_REDIS_DEPRECATION](PROPOSAL_GATEWAY_REDIS_DEPRECATION.md) - please review and respond.
   - **Gateway Response (2025-12-07)**: Will review and respond by Day 8. Initial assessment in Gateway Team Response section.

3. **RO Team**: Any concerns about preserving Gateway's deduplication/storm data?
   - **RO Response (2025-12-07)**: ✅ **No concerns**. RO uses `retry.RetryOnConflict` with refetch pattern, which automatically preserves Gateway's fields. See Architecture Team's answer to Q2 for the pattern.

4. **Gateway Team → RO Team**: What is the expected RR retention period before garbage collection? (Affects deduplication window)
   - **RO Response (2025-12-07)**: ✅ **24 hours** (configurable per environment). Source: `finalizers-lifecycle.md`. RR is deleted after `CompletionTime + 24h`. Gateway should check `status.OverallPhase` for terminal states (`Completed`, `Failed`, `Cancelled`) to determine if new RR should be created.

5. **Gateway Team → RO Team**: For conflict retry, should Gateway use shorter backoff (10ms vs 100ms default) for latency sensitivity?
   - **RO Response (2025-12-07)**: ✅ **Yes, shorter backoff approved**. Gateway's p95 <50ms target is reasonable. 10ms initial backoff with 3 steps (10ms → 20ms → 40ms = 70ms worst case) fits within Gateway's latency budget. RO has no concerns.

6. **Gateway Team → Architecture Team**: Confirm storm aggregation flow - create RR on first alert, or buffer until storm confirmed?
   - **Architecture Response (2025-12-07)**: ✅ **ANSWERED** - See Architecture Team Response section below

---

## 🏛️ Architecture Team Response (2025-12-07)

### Answer to Q1: Informer Setup for Status Reads

**Recommendation**: ✅ **Standalone informer is acceptable for V1.0**

**Rationale**:
- Gateway is a stateless HTTP server, not a reconciliation controller
- Full controller-runtime manager would add unnecessary complexity
- Standalone informer provides the cache benefits without controller overhead

**Approved Pattern**:
```go
// Standalone informer (APPROVED for Gateway)
import (
    "k8s.io/client-go/informers"
    "k8s.io/client-go/tools/cache"
)

func NewGatewayInformer(clientset *kubernetes.Clientset) cache.SharedIndexInformer {
    factory := informers.NewSharedInformerFactoryWithOptions(
        clientset,
        time.Minute*5, // Resync every 5 minutes
        informers.WithNamespace(""), // All namespaces
    )

    // Add field index for fingerprint lookup
    informer := factory.Core().V1().ConfigMaps().Informer() // Example - use RR informer
    informer.AddIndexers(cache.Indexers{
        "fingerprint": func(obj interface{}) ([]string, error) {
            rr := obj.(*RemediationRequest)
            return []string{rr.Labels["kubernaut.ai/fingerprint"]}, nil
        },
    })

    return informer
}
```

**Migration Path (Optional)**:
- V1.0: Standalone informer
- V2.0+: Consider controller-runtime if Gateway needs reconciliation patterns

---

### Answer to Q2: Conflict Rate and Latency Impact

**Expected Conflict Rate**: **<1%** in normal operation

**Analysis**:
| Scenario | Gateway Updates | RO Updates | Conflict Risk |
|----------|-----------------|------------|---------------|
| New signal | `status.deduplication` (create) | None | **0%** |
| Duplicate signal | `status.deduplication.occurrenceCount++` | `status.overallPhase` (may coincide) | **<1%** |
| Storm | `status.stormAggregation` (frequent) | `status.overallPhase` (rare during storm) | **<2%** |

**Industry Reference**:
- Node status: kubelet updates every 10s, NodeController updates conditions - conflicts rare
- Ingress status: ingress controller + cloud controller both update - conflicts handled gracefully

**Backoff Recommendation**: ✅ **Custom shorter backoff APPROVED**

```go
// Gateway-specific retry config (APPROVED)
var GatewayRetry = wait.Backoff{
    Steps:    3,
    Duration: 10 * time.Millisecond,  // Start at 10ms (not 100ms default)
    Factor:   2.0,
    Jitter:   0.1,
}

// Usage:
retry.RetryOnConflict(GatewayRetry, func() error {
    // ... status update
})
```

**Worst Case Latency**:
- No conflict: 0ms overhead
- 1 retry: +10ms
- 2 retries: +30ms (10ms + 20ms)
- 3 retries: +70ms (10ms + 20ms + 40ms) → **within 50ms p99 budget with margin**

---

### Answer to Q3: RR Lifecycle and Deduplication Window

**RR Retention Period**: **24 hours** (configurable per environment)

**Source**: [finalizers-lifecycle.md](../services/crd-controllers/05-remediationorchestrator/finalizers-lifecycle.md#deletion-lifecycle)

```go
// From authoritative documentation
retentionPeriod := r.getRetentionPeriod() // Default: 24 hours, configurable

// Retention check
if remediation.Status.RetentionExpiryTime != nil {
    if time.Now().After(remediation.Status.RetentionExpiryTime.Time) {
        return ctrl.Result{}, r.Delete(ctx, remediation)
    }
}
```

**Deduplication Window Behavior**:

| RR Phase | Gateway Behavior |
|----------|------------------|
| `Pending`, `Processing`, `Analyzing`, `Executing` | **Deduplicate** → Update `status.deduplication` |
| `Blocked` | **Deduplicate** → Update `status.deduplication` |
| `Completed`, `Failed`, `Cancelled` | **Allow new RR** (terminal state) |

**Recommendation**: ✅ **Check `status.OverallPhase` for terminal states** (no additional flag needed)

```go
// APPROVED deduplication logic
func isTerminalPhase(phase string) bool {
    return phase == "Completed" || phase == "Failed" || phase == "Cancelled"
}

func (g *Gateway) ShouldDeduplicate(ctx context.Context, fingerprint string) (bool, *RemediationRequest) {
    rrList := &RemediationRequestList{}
    g.client.List(ctx, rrList,
        client.MatchingLabels{"kubernaut.ai/fingerprint": fingerprint},
    )

    for _, rr := range rrList.Items {
        if !isTerminalPhase(rr.Status.OverallPhase) {
            return true, &rr  // Active RR exists → deduplicate
        }
    }
    return false, nil  // No active RR → create new
}
```

**Key Difference from Redis TTL**:
- **Old (Redis)**: Fixed 5-minute TTL regardless of RR state
- **New (Status)**: Dedup tied to RR lifecycle - dedup active while RR is in-flight, allows new RR when terminal

---

### Answer to Q4: Storm Aggregation Flow

**Authoritative Decision**: ✅ **Buffer until storm confirmed** (Alternative 2)

**Source**: [DD-GATEWAY-008](../architecture/decisions/DD-GATEWAY-008-storm-aggregation-first-alert-handling.md)

**APPROVED Flow**:
```
Alert 1    → Buffer starts (no RR created yet)
Alert 2    → Buffer continues
Alert 3    → Buffer continues
Alert 4    → Buffer continues
Alert 5    → Storm threshold reached (threshold=5)
            → Continue buffering until inactivity timeout
T+60s      → No new alerts for 60s
            → Create SINGLE aggregated RR with all buffered alerts
            → status.StormAggregation.IsStorm = true
            → status.StormAggregation.AggregatedCount = 5+
```

**NOT the other flow** (create RR on first alert). The Gateway team's Q4 description was incorrect.

**Key Parameters** (from DD-GATEWAY-008):
| Parameter | Value | Description |
|-----------|-------|-------------|
| `threshold` | 5 (default) | Alerts before storm detection |
| `inactivity_timeout` | 60 seconds | Window resets on each alert |
| `max_window_duration` | 5 minutes | Absolute maximum window |
| `max_alerts_per_window` | 1000 | Prevents memory exhaustion |

**Sliding Window Behavior**:
```
T=0s:   Alert 1 → Window starts, closes at T=60s
T=10s:  Alert 2 → Window RESETS, closes at T=70s
T=30s:  Alert 3 → Window RESETS, closes at T=90s
T=90s:  No alerts for 60s → Window closes → Create aggregated RR
```

**Impact on Status-Based Design**:
- **During buffering**: No RR exists, state held in memory (or Redis if kept)
- **After window closes**: Single RR created with `status.StormAggregation` populated
- **Subsequent alerts** (same storm): Update existing RR's `status.StormAggregation.AggregatedCount++`

**Note**: The buffering state (before RR creation) would still need to be held somewhere during the window. Options:
1. Keep Redis for buffering only (deprecated after V1.0)
2. In-memory buffer (lost on Gateway crash - acceptable per DD-GATEWAY-008)

---

## 🔄 Gateway Team Follow-Up Response (2025-12-07)

### ⚠️ **Q4 Correction Required: Storm Aggregation Flow**

The Architecture Team's Q4 answer references DD-GATEWAY-008, which was written **before** DD-GATEWAY-011's status-based approach was approved. We believe the answer needs revision based on:

1. **DD-ORCHESTRATOR-001** (Point-in-Time Snapshot Pattern)
2. **DD-GATEWAY-011** (Status-Based Persistence)
3. **Redis Deprecation Goal** (Zero external dependencies)

---

### 📋 **Finding: Storm Data is NOT Blocking for RCA**

From **DD-ORCHESTRATOR-001** (lines 168-187):

```
T0: Gateway creates RemediationRequest
    - IsStorm: false       ← Storm not yet confirmed
    - OccurrenceCount: 1

T1: RemediationOrchestrator creates AIAnalysis
    - Snapshot: IsStorm=false, OccurrenceCount=1  ← AI proceeds without storm context

T2: Gateway detects storm, updates RemediationRequest
    - IsStorm: true
    - StormAlertCount: 15

T3: AIAnalysis completes (still uses T1 snapshot)
    - LLM saw: IsStorm=false  ← First analysis proceeds normally

T4: If remediation fails, new AIAnalysis created
    - New snapshot: IsStorm=true, StormAlertCount=15  ← Retry gets full context
```

**Key Insight**: The existing design **already assumes** RR can be created before storm is confirmed. AI analysis proceeds with whatever data is available at snapshot time. Storm context is "nice to have" for first analysis, not blocking.

---

### 🚫 **DD-GATEWAY-008 Contradiction**

DD-GATEWAY-008 (Alternative 2) says:
> "Buffer first N alerts in Redis, create NO CRDs until storm threshold is reached"

This contradicts:
1. **DD-GATEWAY-011's goal**: Enable Redis deprecation
2. **DD-ORCHESTRATOR-001's pattern**: Point-in-time snapshots (RR can exist before storm confirmed)
3. **Business goal**: Zero external infrastructure dependencies

---

### ✅ **Proposed Revision: Async Storm Aggregation (Redis-Free)**

**Key Insight**: Storm data is NOT blocking for RCA (per DD-ORCHESTRATOR-001). We can process immediately and update storm context asynchronously.

| Phase | DD-GATEWAY-008 (Old) | Proposed (Redis-Free) |
|-------|---------------------|----------------------|
| Alert 1 | Buffer in Redis (no RR) | **Create RR immediately** with `phase="Pending"` |
| Alerts 2-4 | Continue buffering | **Update RR** `status.stormAggregation.aggregatedCount++` |
| Alert 5 (threshold) | Retrieve buffer, start window | **Update RR** `isStorm=true` (RO already processing) |
| After window | Create aggregated RR | **No action** (RR already exists and processing) |

**Flow**:
```
Alert 1 → Create RR with:
            status.phase = "Pending"              ← RO processes IMMEDIATELY
            status.stormAggregation.isStorm = false
            status.stormAggregation.aggregatedCount = 1
          → RO reconciles → Creates AIAnalysis → AI starts analysis

Alert 2 → Dedup check: RR exists (fingerprint match)
          → Update RR: aggregatedCount = 2
          → RO already processing (uses point-in-time snapshot)

Alert 5 → Dedup check: RR exists
          → Threshold reached! Update RR:
            status.stormAggregation.isStorm = true
            status.stormAggregation.aggregatedCount = 5
          → RO/AI already processing with original snapshot (THAT'S FINE per DD-ORCHESTRATOR-001)

If first remediation ineffective:
          → Retry creates new AIAnalysis
          → New snapshot includes: isStorm=true, aggregatedCount=5+
          → Coordinated remediation recommended
```

**Result**:
- ✅ **1 CRD per signal/storm** (deduplication prevents multiple CRDs)
- ✅ **No waiting** (RO processes first alert immediately)
- ✅ **No Redis** (storm state in RR status)
- ✅ **Storm context available for retry** (if first remediation ineffective)

---

### 📊 **Comparison**

| Aspect | DD-GATEWAY-008 (Sync) | Async Storm Aggregation |
|--------|----------------------|-------------------------|
| Redis dependency | ❌ Required | ✅ **None** |
| First alert delay | ⚠️ Wait for window (up to 5min) | ✅ **Immediate** |
| Multi-replica safe | Via Redis | ✅ Via K8s API |
| Crash recovery | Redis TTL | ✅ RR persists in etcd |
| Single CRD guarantee | ✅ Yes | ✅ Yes |
| Storm context for first RCA | ✅ Yes (waited) | ⚠️ No (but retry gets it) |

**Trade-off**: First RCA may not have storm context, but retry will. This is acceptable because:
1. DD-ORCHESTRATOR-001 already assumes point-in-time snapshots
2. Storm context is "nice to have", not blocking for RCA
3. Retry mechanism exists for ineffective remediations

---

### 📅 **Proposed Update to DD-GATEWAY-008**

Gateway team will update DD-GATEWAY-008 to:
1. Mark Alternative 2 (sync buffering) as **SUPERSEDED** by DD-GATEWAY-011
2. Document new **Async Storm Aggregation** approach
3. Remove Redis dependency entirely
4. Clarify that storm detection's purpose is **preventing multiple CRDs**, not blocking RCA

**This enables complete Redis deprecation for Gateway in V1.0.**

---

## 🏛️ Architecture Team Approval of Async Storm Aggregation (2025-12-07)

### ✅ **APPROVED**: Async Storm Aggregation (Redis-Free)

The Architecture Team **approves** the Gateway team's proposed revision to DD-GATEWAY-008.

**Rationale**:

| Factor | Assessment |
|--------|------------|
| **DD-ORCHESTRATOR-001 Alignment** | ✅ Point-in-time snapshot pattern already supports this |
| **Redis Deprecation Goal** | ✅ Enables complete Redis removal |
| **Business Value** | ✅ Lower latency (immediate RR creation vs. 5min wait) |
| **Trade-off Acceptability** | ✅ First RCA without storm context is acceptable (retry gets it) |
| **Single CRD Guarantee** | ✅ Maintained via deduplication |

**Key Insight Validation**:

The Gateway team correctly identified that DD-GATEWAY-008's synchronous buffering was designed **before** the shared status pattern was adopted. The async approach:
- Is more aligned with K8s-native patterns
- Eliminates external infrastructure dependency
- Reduces first-alert latency from ~5min to immediate
- Maintains all business guarantees

### 📋 **Action Items**

| Task | Owner | Status |
|------|-------|--------|
| Update DD-GATEWAY-008 to mark sync buffering as SUPERSEDED | Gateway Team | ⏳ Pending |
| Document Async Storm Aggregation in DD-GATEWAY-008 | Gateway Team | ⏳ Pending |
| Update Redis Deprecation Proposal status to APPROVED | Architecture Team | ✅ Done |
| Remove Redis from Gateway implementation plan | Gateway Team | ⏳ Pending |
| Update Gateway Helm chart to remove Redis dependency | Gateway Team | ⏳ Pending |

### 📊 **Final Architecture Summary**

```
┌─────────────────────────────────────────────────────────────────────┐
│                    GATEWAY (Redis-Free V1.0)                        │
├─────────────────────────────────────────────────────────────────────┤
│  Signal Ingestion                                                   │
│       │                                                             │
│       ▼                                                             │
│  ┌─────────────────────────────────────────────────────────────┐   │
│  │ Deduplication (K8s Informer)                                │   │
│  │   • Check RR exists by fingerprint label                    │   │
│  │   • If exists + non-terminal → Update status.deduplication  │   │
│  │   • If not exists → Create new RR                           │   │
│  └─────────────────────────────────────────────────────────────┘   │
│       │                                                             │
│       ▼                                                             │
│  ┌─────────────────────────────────────────────────────────────┐   │
│  │ Storm Aggregation (Async, K8s Status)                       │   │
│  │   • Alert 1: Create RR, aggregatedCount=1                   │   │
│  │   • Alert 2-4: Update aggregatedCount++                     │   │
│  │   • Alert 5+: Set isStorm=true, continue incrementing       │   │
│  │   • RO processes immediately (point-in-time snapshot)       │   │
│  └─────────────────────────────────────────────────────────────┘   │
│       │                                                             │
│       ▼                                                             │
│  ┌─────────────────────────────────────────────────────────────┐   │
│  │ Rate Limiting (Proxy - ADR-048)                             │   │
│  │   • Nginx Ingress (K8s) / HAProxy Router (OCP)              │   │
│  │   • Global cluster-wide enforcement                         │   │
│  │   • Zero Gateway code required                              │   │
│  └─────────────────────────────────────────────────────────────┘   │
│       │                                                             │
│       ▼                                                             │
│  RemediationRequest Created/Updated                                 │
│       │                                                             │
│       ▼                                                             │
│  RO Reconciles (uses point-in-time snapshot per DD-ORCHESTRATOR-001)│
└─────────────────────────────────────────────────────────────────────┘

External Dependencies: NONE (Redis removed)
```

### 🎯 **Redis Deprecation: APPROVED for V1.0**

| Component | Before | After | Reference |
|-----------|--------|-------|-----------|
| **Deduplication** | Redis key-value | K8s RR Status + Informer | DD-GATEWAY-011 |
| **Storm Aggregation** | Redis buffering (sync) | K8s RR Status (async) | DD-GATEWAY-008 v2.0 |
| **Rate Limiting** | Redis counters | **Proxy (Ingress/Route)** | [ADR-048](../architecture/decisions/ADR-048-rate-limiting-proxy-delegation.md) |
| **Redis Dependency** | Required | **COMPLETELY REMOVED** | - |

---

## 🏛️ NEW: RemediationRequest CRD Ownership Clarification (ADR-049)

**Date Added**: 2025-12-07
**Status**: ✅ **APPROVED**
**Reference**: [ADR-049](../architecture/decisions/ADR-049-remediationrequest-crd-ownership.md)

### Summary

**Remediation Orchestrator (RO) owns the RemediationRequest CRD schema definition.**

This resolves the previous ambiguity where documentation stated both "Owner: Central Controller Service" and "Created By: Gateway Service".

### What This Means for Gateway Team

| Aspect | Impact |
|--------|--------|
| **Instance Creation** | ✅ **No change** - Gateway still creates RR instances |
| **Status Updates** | ✅ **No change** - DD-GATEWAY-011 still valid |
| **Type Imports** | ⚠️ Import RR types from RO package |
| **Documentation** | ⚠️ Remove RR schema definitions, reference RO as authoritative |

### Code Change (if needed)

```go
// Before (if Gateway defined types)
import remediationv1 "github.com/jordigilh/kubernaut/api/gateway/v1alpha1"

// After (import from RO)
import remediationv1 "github.com/jordigilh/kubernaut/api/remediation/v1alpha1"
```

### Compatibility

| Pattern | Compatible? |
|---------|-------------|
| **DD-GATEWAY-011 (Shared Status)** | ✅ Yes - Gateway still owns `status.deduplication`, `status.stormAggregation` |
| **Redis Deprecation** | ✅ Yes - No impact |
| **Async Storm Aggregation** | ✅ Yes - No impact |

### Rationale

| Factor | Rationale |
|--------|-----------|
| **K8s Pattern** | Controller that reconciles a CRD should own its schema |
| **Domain Ownership** | RR represents remediation lifecycle = RO's domain |
| **Clean Dependencies** | Gateway → RO types (not reverse) |

### Gateway Team Acknowledgment

| Acknowledged | Date | Notes |
|--------------|------|-------|
| ✅ **ACKNOWLEDGED** | 2025-12-07 | **No code changes required**. Gateway already imports from `api/remediation/v1alpha1/`. DD-GATEWAY-011 shared status pattern confirmed compatible. |

---

**Issued By**: Architecture Team
**Date**: December 7, 2025

---

## 🚨 UPDATE: DD-GATEWAY-011 v1.3 - Consecutive Failure Blocking Moved to RO (2025-12-10)

### Summary

**BR-GATEWAY-184 is SUPERSEDED by BR-ORCH-042**.

Consecutive failure blocking logic has been moved from Gateway to Remediation Orchestrator for cleaner separation of concerns.

### What Changed

| Aspect | Previous (v1.2) | New (v1.3) |
|--------|-----------------|------------|
| **Failure counting** | Gateway counted consecutive failures | **RO counts failures** |
| **Blocked RR creation** | Gateway created RR with `phase=Blocked` | **RO holds RR in Blocked phase** |
| **`Blocked` phase** | Unclear if terminal | **Non-terminal** (prevents new RR creation) |
| **Cooldown** | Not specified | **1 hour** (configurable) |

### Gateway Team: Required Changes

**REMOVE** from Gateway:
```go
// ❌ REMOVE - No longer Gateway's responsibility
func (g *Gateway) ShouldBlock(ctx context.Context, fingerprint string) bool {
    // Don't count consecutive failures
    // Don't create RR with phase=Blocked
}
```

**SIMPLIFIED Gateway Logic**:
```go
// ✅ Gateway only checks if active RR exists
func (g *Gateway) HandleSignal(ctx context.Context, signal Signal) error {
    fingerprint := signal.Fingerprint()

    // Check for ANY active (non-terminal) RR with this fingerprint
    activeRR := g.findActiveRR(ctx, fingerprint)

    if activeRR != nil {
        // Active RR exists - update deduplication, don't create new
        // NOTE: Blocked is now non-terminal, so blocked RRs are "active"
        return g.updateDeduplication(ctx, activeRR, signal)
    }

    // No active RR - create new one
    return g.createRemediationRequest(ctx, signal)
}

// Updated: Blocked is NOT terminal
func isTerminalPhase(phase string) bool {
    return phase == "Completed" || phase == "Failed" || phase == "Timeout"
    // NOTE: "Blocked" is NOT in this list - it's non-terminal
}
```

### Phase Classification Update

| Phase | Terminal? | Gateway Behavior |
|-------|-----------|------------------|
| Pending | No | Update deduplication |
| Processing | No | Update deduplication |
| Analyzing | No | Update deduplication |
| Approving | No | Update deduplication |
| Executing | No | Update deduplication |
| Recovering | No | Update deduplication |
| **Blocked** | **No (CHANGED)** | Update deduplication |
| Completed | Yes | Create new RR |
| Failed | Yes | Create new RR |
| Timeout | Yes | Create new RR |

### RO Responsibility (BR-ORCH-042)

RO now handles:
1. **Counting consecutive failures** for a fingerprint
2. **Transitioning to Blocked** when ≥3 consecutive failures
3. **Setting `BlockedUntil`** = now + 1 hour
4. **Auto-expiring** Blocked → Failed after cooldown

### New RR Status Fields (RO-Owned)

```go
// Added to RemediationRequestStatus
BlockedUntil *metav1.Time `json:"blockedUntil,omitempty"`  // When cooldown expires
BlockReason  *string      `json:"blockReason,omitempty"`   // Why blocked
```

### References

| Document | Purpose |
|----------|---------|
| [DD-GATEWAY-011 v1.3](../architecture/decisions/DD-GATEWAY-011-shared-status-deduplication.md) | Updated design decision |
| [BR-ORCH-042](../requirements/BR-ORCH-042-consecutive-failure-blocking.md) | New RO business requirement |
| [BR-GATEWAY-184](../requirements/BR-GATEWAY-181-185-shared-status-deduplication.md) | **SUPERSEDED** |
| [BR-ORCH-042 Implementation Plan](../services/crd-controllers/05-remediationorchestrator/implementation/BR-ORCH-042_IMPLEMENTATION_PLAN.md) | RO implementation plan |

### Gateway Team Action Required

| # | Task | Priority |
|---|------|----------|
| 1 | Remove `ShouldBlock()` function | P0 |
| 2 | Remove consecutive failure counting | P0 |
| 3 | Update `isTerminalPhase()` - exclude `Blocked` | P0 |
| 4 | Update unit tests | P1 |

### Questions?

Contact RO Team.

---

## 📝 Document History

| Version | Date | Author | Changes |
|---------|------|--------|---------|
| v1.0 | 2025-12-07 | Architecture Team | Initial notice |
| v1.1 | 2025-12-07 | Gateway Team | Added acknowledgment and response |
| v1.2 | 2025-12-07 | Architecture Team | Answered Q1-Q4 with authoritative documentation references |
| v1.3 | 2025-12-07 | Gateway Team | Q4 correction: DD-GATEWAY-008 sync buffering superseded by async storm aggregation (Redis-free) |
| v1.4 | 2025-12-07 | Architecture Team | **APPROVED** async storm aggregation and Redis deprecation for V1.0 |
| v1.5 | 2025-12-07 | Gateway Team | **ADR-048**: Rate limiting delegated to Ingress/Route proxy. Gateway rate limiting code DEPRECATED. |
| v1.6 | 2025-12-07 | Architecture Team | **ADR-049**: RO owns RemediationRequest CRD schema. Gateway imports RO types. |
| v1.7 | 2025-12-07 | RO Team | **ACKNOWLEDGED**: Answered Q3-Q5, confirmed 24h retention, approved shorter backoff |
| v1.8 | 2025-12-07 | RO Team | **UNBLOCKED GATEWAY**: Added `DeduplicationStatus` and `StormAggregationStatus` to RR API types |
| v1.9 | 2025-12-07 | RO Team | **ACK FOR GATEWAY**: Added authoritative implementation, usage instructions, and RO commitments |
| v1.10 | 2025-12-07 | Gateway Team | **ADR-049 ACK**: Gateway acknowledges RO owns RR schema. No code changes needed - already imports from `api/remediation/v1alpha1/` |
| v1.11 | 2025-12-10 | RO Team | **DD-GATEWAY-011 v1.3**: Consecutive failure blocking moved from Gateway to RO (BR-ORCH-042). Gateway simplified: no failure counting, `Blocked` is non-terminal. BR-GATEWAY-184 superseded. |
| v1.12 | 2025-12-10 | RO Team | **BR-GATEWAY-185 v1.1**: Changed fingerprint lookup from labels to field selector on `spec.signalFingerprint`. Labels are mutable and truncated to 63 chars; spec field is immutable and supports full 64-char SHA256. Both Gateway and RO should use `client.MatchingFields{"spec.signalFingerprint": fingerprint}`. |
| v1.12 | 2025-12-10 | Gateway Team | **DD-GATEWAY-011 v1.3 ACK**: All required changes implemented as part of DD-GATEWAY-012 (Redis removal). `IsTerminalPhase()` updated: `Completed, Failed, Timeout` are terminal; `Blocked` is non-terminal. Gateway never had `ShouldBlock()` or failure counting. Tests pass. |
| v1.13 | 2025-12-11 | Gateway Team | **BR-GATEWAY-185 v1.1 IMPLEMENTED**: Changed fingerprint lookup from labels to field selector on `spec.signalFingerprint`. Gateway now uses cached client with field index for O(1) lookups. No more 63-char truncation - full 64-char SHA256 supported. Unit tests updated with field index. |

