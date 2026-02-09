# BR-ORCH-042: Consecutive Failure Blocking - Implementation Plan

**BR**: [BR-ORCH-042](../../../../requirements/BR-ORCH-042-consecutive-failure-blocking.md)
**Version**: 1.3
**Created**: December 10, 2025
**Status**: 🚧 IN PROGRESS (Day 1-3 Complete, Day 4 Pending)
**Estimated Effort**: 3-4 days
**Priority**: P0 (V1.0)

---

## ⚠️ TDD Deviation Notice

**IMPORTANT**: This implementation deviated from TDD methodology. Implementation (Day 1-2) was completed BEFORE tests (Day 3).

**What Happened**:
1. Day 1: Schema + phase classification implemented
2. Day 2: Blocking logic + reconciler + metrics implemented
3. Day 3: Unit tests written **AFTER** implementation (violation)

**Corrective Action**:
- Unit tests written retroactively to validate business requirements
- All 229 unit tests pass, including 19 new BR-ORCH-042 tests
- Future implementations MUST follow TDD (RED → GREEN → REFACTOR)

**Lesson Learned**: Per `TESTING_GUIDELINES.md`, tests should be written FIRST to define the business contract, not after implementation.

---

## 📋 Executive Summary

Implement consecutive failure blocking at the RO level to prevent infinite failure loops. When a signal fingerprint fails ≥3 consecutive times, RO holds the RR in a non-terminal `Blocked` phase for 1 hour before allowing retry.

**Key Architectural Change**: This logic was originally planned for Gateway (BR-GATEWAY-184) but moved to RO for cleaner separation of concerns. Gateway now only checks if an active RR exists.

---

## 🎯 Implementation Scope

### In Scope
- [x] Add `BlockedUntil`, `BlockReason`, `ConsecutiveFailureCount` fields to RR status ✅ **DONE**
- [x] Add `Blocked` phase to phase constants ✅ **DONE**
- [x] Make `Blocked` a non-terminal phase (NOT in `IsTerminal()`) ✅ **DONE**
- [x] Update `ValidTransitions` for Failed → Blocked and Blocked → Failed ✅ **DONE**
- [x] Update timeout detector to skip Blocked phase ✅ **DONE**
- [x] Regenerate CRD manifests ✅ **DONE**
- [x] Implement `countConsecutiveFailures()` helper ✅ **DONE**
- [x] Implement `handleBlockedPhase()` handler ✅ **DONE**
- [x] Update `transitionToFailed()` to check for blocking ✅ **DONE**
- [x] Add metrics for blocking ✅ **DONE**
- [x] Set up field index on `spec.signalFingerprint` in `SetupWithManager()` ✅ **DONE**
- [x] Unit tests (19 tests - exceeded plan) ✅ **DONE** (⚠️ TDD deviation - written after impl)
- [ ] Integration tests (4 tests) ⏳ **PENDING**

### Out of Scope
- Gateway changes (handled by Gateway team per DD-GATEWAY-011 v1.3)
- Notification on block (deferred to V1.1)

---

## 📁 Files to Modify/Create

### Schema Changes

| File | Change | Priority | Status |
|------|--------|----------|--------|
| `api/remediation/v1alpha1/remediationrequest_types.go` | Add `BlockedUntil`, `BlockReason`, `ConsecutiveFailureCount` fields | P0 | ✅ **DONE** |
| `config/crd/bases/remediation.kubernaut.ai_remediationrequests.yaml` | Regenerate CRD | P0 | ✅ **DONE** |

### Phase Classification

| File | Change | Priority | Status |
|------|--------|----------|--------|
| `pkg/remediationorchestrator/phase/types.go` | Add `Blocked` phase constant, update `Validate()`, `ValidTransitions` | P0 | ✅ **DONE** |
| `pkg/remediationorchestrator/timeout/detector.go` | Add `Blocked` to skip list | P0 | ✅ **DONE** |

### Controller Logic

| File | Change | Priority |
|------|--------|----------|
| `pkg/remediationorchestrator/controller/reconciler.go` | Add `handleBlockedPhase()`, update `transitionToFailed()`, add field index in `SetupWithManager()` | P0 |
| `pkg/remediationorchestrator/controller/blocking.go` | **NEW**: `countConsecutiveFailures()`, `shouldBlockSignal()`, use `client.MatchingFields` (not labels) | P0 |

### Field Index Setup (BR-GATEWAY-185 v1.1)

> **⚠️ CRITICAL**: RO MUST set up a field index on `spec.signalFingerprint` for O(1) lookup.

| File | Change | Priority |
|------|--------|----------|
| `pkg/remediationorchestrator/controller/reconciler.go` | Add `mgr.GetFieldIndexer().IndexField()` in `SetupWithManager()` | P0 |

**Rationale**: Uses immutable `spec.signalFingerprint` (64 chars) instead of mutable labels (63 chars max).

### Metrics

| File | Change | Priority |
|------|--------|----------|
| `pkg/remediationorchestrator/metrics/prometheus.go` | Add blocking metrics | P1 |

### Tests

| File | Change | Priority |
|------|--------|----------|
| `test/unit/remediationorchestrator/blocking_test.go` | **NEW**: 8 unit tests | P0 |
| `test/integration/remediationorchestrator/blocking_integration_test.go` | **NEW**: 4 integration tests | P1 |

---

## 🔧 Implementation Details

### 1. Schema Changes (`remediationrequest_types.go`)

```go
type RemediationRequestStatus struct {
    // ... existing fields ...

    // ========================================
    // BLOCKING (BR-ORCH-042)
    // ========================================

    // BlockedUntil specifies when this fingerprint can be retried.
    // Set when OverallPhase=Blocked due to consecutive failures.
    // After this time, RO transitions to Failed (terminal), allowing Gateway
    // to create new RRs for the same fingerprint.
    // +optional
    BlockedUntil *metav1.Time `json:"blockedUntil,omitempty"`

    // BlockReason explains why this RR is blocked.
    // Values: "consecutive_failures_exceeded", "manual_block"
    // +optional
    BlockReason *string `json:"blockReason,omitempty"`
}
```

### 2. Phase Classification (`phase.go`)

```go
const (
    // ... existing phases ...
    Blocked Phase = "Blocked" // NEW: Non-terminal blocking phase
)

// IsTerminal returns true if the phase is terminal.
// Key change: Blocked is NOT terminal (BR-ORCH-042)
func IsTerminal(p Phase) bool {
    switch p {
    case Completed, Failed, Timeout:
        return true
    case Pending, Processing, Analyzing, Approving, Executing, Recovering, Blocked:
        return false
    default:
        return false
    }
}
```

### 3. Blocking Logic (`blocking.go`)

> **⚠️ UPDATED (v1.2)**: Uses field selector on `spec.signalFingerprint` (not labels) per BR-GATEWAY-185 v1.1.

```go
package controller

import (
    "context"
    "fmt"
    "sort"
    "time"

    remediationv1 "github.com/jordigilh/kubernaut/api/remediation/v1alpha1"
    "sigs.k8s.io/controller-runtime/pkg/client"
)

const (
    DefaultBlockThreshold    = 3
    DefaultCooldownDuration  = 1 * time.Hour
)

// countConsecutiveFailures counts consecutive Failed RRs for a fingerprint.
// Uses field selector on spec.signalFingerprint (immutable, full 64-char).
// Stops counting on first Completed RR (resets the counter).
//
// Reference: BR-ORCH-042.1, BR-GATEWAY-185 v1.1
func (r *Reconciler) countConsecutiveFailures(ctx context.Context, fingerprint string) int {
    rrList := &remediationv1.RemediationRequestList{}

    // BR-GATEWAY-185 v1.1: Use field selector on immutable spec field (not mutable labels)
    // Full 64-char fingerprint (no truncation)
    if err := r.client.List(ctx, rrList,
        client.MatchingFields{"spec.signalFingerprint": fingerprint},
    ); err != nil {
        return 0 // Conservative: don't block on error
    }

    // Sort by creation timestamp, newest first
    sort.Slice(rrList.Items, func(i, j int) bool {
        return rrList.Items[i].CreationTimestamp.After(rrList.Items[j].CreationTimestamp.Time)
    })

    consecutiveFailures := 0
    for _, rr := range rrList.Items {
        switch rr.Status.OverallPhase {
        case "Failed":
            // Failed RR - increment counter
            // Note: TimedOut not counted per BR-ORCH-042 (only "Failed RRs")
            consecutiveFailures++
        case "Completed":
            // Success resets the counter
            return consecutiveFailures
        case "Blocked":
            // Already blocked - don't double-count
            continue
        case "Skipped":
            // Skipped due to resource lock - not a remediation failure
            continue
        default:
            // Active/in-progress - skip
            continue
        }
    }

    return consecutiveFailures
}

// shouldBlockSignal determines if a signal should be blocked based on failure history.
func (r *Reconciler) shouldBlockSignal(ctx context.Context, fingerprint string) (bool, string) {
    consecutiveFailures := r.countConsecutiveFailures(ctx, fingerprint)

    if consecutiveFailures >= DefaultBlockThreshold {
        return true, "consecutive_failures_exceeded"
    }
    return false, ""
}

// transitionToBlocked transitions the RR to Blocked phase with cooldown.
func (r *Reconciler) transitionToBlocked(ctx context.Context, rr *remediationv1.RemediationRequest, reason string, cooldown time.Duration) (ctrl.Result, error) {
    logger := log.FromContext(ctx).WithValues("remediationRequest", rr.Name)

    blockedUntil := metav1.NewTime(time.Now().Add(cooldown))

    err := retry.RetryOnConflict(retry.DefaultRetry, func() error {
        if err := r.client.Get(ctx, client.ObjectKeyFromObject(rr), rr); err != nil {
            return err
        }

        rr.Status.OverallPhase = string(phase.Blocked)
        rr.Status.BlockedUntil = &blockedUntil
        rr.Status.BlockReason = &reason
        rr.Status.Message = fmt.Sprintf("Signal blocked due to %s. Will unblock at %s",
            reason, blockedUntil.Format(time.RFC3339))

        return r.client.Status().Update(ctx, rr)
    })
    if err != nil {
        logger.Error(err, "Failed to transition to Blocked")
        return ctrl.Result{}, err
    }

    // Record metric
    metrics.BlockedTotal.WithLabelValues(rr.Namespace, reason).Inc()
    metrics.CurrentBlockedGauge.WithLabelValues(rr.Namespace).Inc()

    logger.Info("Signal blocked due to consecutive failures",
        "reason", reason,
        "blockedUntil", blockedUntil.Format(time.RFC3339),
        "cooldownDuration", cooldown,
    )

    // Requeue at exactly blockedUntil
    return ctrl.Result{RequeueAfter: cooldown}, nil
}
```

### 4. Handle Blocked Phase (`reconciler.go`)

```go
// In Reconcile() switch statement:
case phase.Blocked:
    return r.handleBlockedPhase(ctx, rr)

// handleBlockedPhase handles the Blocked phase.
// Checks if cooldown has expired and transitions to Failed if so.
func (r *Reconciler) handleBlockedPhase(ctx context.Context, rr *remediationv1.RemediationRequest) (ctrl.Result, error) {
    logger := log.FromContext(ctx).WithValues("remediationRequest", rr.Name)

    if rr.Status.BlockedUntil == nil {
        // Manual block without cooldown - remain blocked
        logger.V(1).Info("RR is manually blocked, no auto-expiry")
        return ctrl.Result{}, nil
    }

    if time.Now().After(rr.Status.BlockedUntil.Time) {
        logger.Info("Blocked cooldown expired, transitioning to Failed")

        // Decrement gauge
        metrics.CurrentBlockedGauge.WithLabelValues(rr.Namespace).Dec()
        metrics.BlockedCooldownExpiredTotal.Inc()

        // Transition to terminal Failed
        return r.transitionToFailed(ctx, rr, "blocked",
            fmt.Sprintf("Cooldown expired after blocking due to %s", *rr.Status.BlockReason))
    }

    // Still in cooldown - requeue at expiry
    requeueAfter := time.Until(rr.Status.BlockedUntil.Time)
    logger.V(1).Info("Still blocked, requeueing at expiry",
        "blockedUntil", rr.Status.BlockedUntil.Format(time.RFC3339),
        "requeueAfter", requeueAfter,
    )
    return ctrl.Result{RequeueAfter: requeueAfter}, nil
}
```

### 5. Update `transitionToFailed()`

```go
// transitionToFailed - updated to check for blocking
func (r *Reconciler) transitionToFailed(ctx context.Context, rr *remediationv1.RemediationRequest, failurePhase, failureReason string) (ctrl.Result, error) {
    logger := log.FromContext(ctx).WithValues("remediationRequest", rr.Name)

    // BR-ORCH-042: Check if this failure triggers blocking
    // Skip if already transitioning from Blocked phase (cooldown expiry)
    if failurePhase != "blocked" {
        consecutiveFailures := r.countConsecutiveFailures(ctx, rr.Spec.SignalFingerprint)
        // +1 for this failure (not yet recorded)
        if consecutiveFailures+1 >= DefaultBlockThreshold {
            logger.Info("Consecutive failure threshold reached, blocking signal",
                "consecutiveFailures", consecutiveFailures+1,
                "threshold", DefaultBlockThreshold,
            )
            return r.transitionToBlocked(ctx, rr, "consecutive_failures_exceeded", DefaultCooldownDuration)
        }
    }

    // Normal terminal Failed transition
    // ... existing logic ...
}
```

### 6. Metrics (`prometheus.go`)

```go
var (
    // BlockedTotal counts RRs blocked due to consecutive failures
    BlockedTotal = prometheus.NewCounterVec(
        prometheus.CounterOpts{
            Namespace: namespace,
            Subsystem: subsystem,
            Name:      "blocked_total",
            Help:      "Total RemediationRequests blocked due to consecutive failures",
        },
        []string{"namespace", "reason"},
    )

    // BlockedCooldownExpiredTotal counts blocked RRs that expired
    BlockedCooldownExpiredTotal = prometheus.NewCounter(
        prometheus.CounterOpts{
            Namespace: namespace,
            Subsystem: subsystem,
            Name:      "blocked_cooldown_expired_total",
            Help:      "Total blocked RRs that expired and transitioned to Failed",
        },
    )

    // CurrentBlockedGauge tracks current blocked RR count
    CurrentBlockedGauge = prometheus.NewGaugeVec(
        prometheus.GaugeOpts{
            Namespace: namespace,
            Subsystem: subsystem,
            Name:      "blocked_current",
            Help:      "Current number of blocked RRs",
        },
        []string{"namespace"},
    )
)
```

---

## 🧪 Test Plan

### Unit Tests (`blocking_test.go`)

| # | Test | BR | Assertion |
|---|------|-----|-----------|
| 1 | `countConsecutiveFailures returns 0 for empty list` | BR-ORCH-042 | Count == 0 |
| 2 | `countConsecutiveFailures counts consecutive Failed` | BR-ORCH-042 | Count == 3 |
| 3 | `countConsecutiveFailures resets on Completed` | BR-ORCH-042 | Count == 1 (after success) |
| 4 | `shouldBlockSignal returns true at threshold` | BR-ORCH-042 | blocked == true at 3 |
| 5 | `shouldBlockSignal returns false below threshold` | BR-ORCH-042 | blocked == false at 2 |
| 6 | `transitionToBlocked sets correct fields` | BR-ORCH-042 | Phase, BlockedUntil, BlockReason set |
| 7 | `handleBlockedPhase transitions after cooldown` | BR-ORCH-042 | Phase → Failed after expiry |
| 8 | `handleBlockedPhase requeues before cooldown` | BR-ORCH-042 | RequeueAfter = remaining time |

### Integration Tests (`blocking_integration_test.go`)

| # | Test | BR | Assertion |
|---|------|-----|-----------|
| 1 | `Third consecutive failure triggers Blocked phase` | BR-ORCH-042 | RR enters Blocked, not Failed |
| 2 | `Gateway sees Blocked RR as active (no new RR)` | BR-ORCH-042 | Simulated - dedup update |
| 3 | `Blocked RR transitions to Failed after 1 hour` | BR-ORCH-042 | Use fake clock |
| 4 | `Success resets failure counter` | BR-ORCH-042 | 4th RR not blocked after success |

---

## 📅 Implementation Timeline

| Day | Tasks | Deliverables | Status |
|-----|-------|--------------|--------|
| **Day 1** | Schema changes, phase classification | `remediationrequest_types.go`, `types.go`, `detector.go` | ✅ **DONE** |
| **Day 2** | Blocking logic, reconciler updates, field index, metrics | `blocking.go`, `reconciler.go`, `prometheus.go`, CRD manifest | ✅ **DONE** |
| **Day 3** | Unit tests | `blocking_test.go` (19 tests), `phase_test.go` updates | ✅ **DONE** (⚠️ TDD deviation) |
| **Day 4** | Integration tests, documentation | `blocking_integration_test.go` (4 tests), update BR_MAPPING.md | ⏳ **NEXT** |

---

## 🔗 Dependencies

### Blocked By
- None (can proceed independently)

### Blocks
- Gateway team implementation of DD-GATEWAY-011 v1.3 (but they can start in parallel)

### Related Work
- Gateway team: Update Gateway to remove failure counting logic
- Gateway team: Treat `Blocked` as non-terminal (update dedup, don't create new RR)

---

## ✅ Acceptance Criteria

| ID | Criterion | Test Coverage |
|----|-----------|---------------|
| AC-042-1 | RO counts consecutive Failed RRs for same fingerprint | Unit |
| AC-042-2 | RO resets failure count on Completed RR | Unit |
| AC-042-3 | RO transitions to Blocked at ≥3 failures | Unit, Integration |
| AC-042-4 | BlockedUntil set to now + 1 hour | Unit |
| AC-042-5 | RO transitions Blocked → Failed after cooldown | Unit, Integration |
| AC-042-6 | `IsTerminal(Blocked)` returns false | Unit |
| AC-042-7 | Metrics recorded for blocking | Unit |

---

## 📊 Risk Assessment

| Risk | Likelihood | Impact | Mitigation |
|------|------------|--------|------------|
| Gateway not updated | Medium | High | Clear communication via DD-GATEWAY-011 v1.3 |
| Performance (list RRs) | Low | Medium | Add fingerprint index, limit to recent RRs |
| Clock skew | Low | Low | Use server time consistently |

---

## 📝 Notes

- **Configuration**: Threshold (3) and cooldown (1h) are hardcoded for V1.0. Make configurable in V1.1.
- **Notification**: BR-ORCH-042.5 (notify on block) deferred to V1.1.
- **Manual unblock**: Operators can delete Blocked RR to allow immediate retry.

---

**Document Version**: 1.3
**Created**: December 10, 2025
**Last Updated**: December 10, 2025
**Maintained By**: Remediation Orchestrator Team

---

## 📋 Change Log

| Version | Date | Changes |
|---------|------|---------|
| 1.0 | 2025-12-10 | Initial plan |
| 1.1 | 2025-12-10 | Day 1 complete: schema fields (`BlockedUntil`, `BlockReason`, `ConsecutiveFailureCount`), phase constants (`Blocked`), timeout skip, CRD manifest regenerated |
| 1.2 | 2025-12-10 | **BREAKING**: Changed from label-based to field selector on `spec.signalFingerprint` per BR-GATEWAY-185 v1.1. Updated blocking logic to use `client.MatchingFields`. Added field index setup in `SetupWithManager()`. Only count `Failed` phase (not `TimedOut` per BR-ORCH-042 text). |
| 1.3 | 2025-12-10 | Day 2-3 complete: Blocking logic (`blocking.go`), reconciler updates, metrics, unit tests (19 tests). **TDD DEVIATION DOCUMENTED**: Implementation completed before tests - corrective action taken. |

