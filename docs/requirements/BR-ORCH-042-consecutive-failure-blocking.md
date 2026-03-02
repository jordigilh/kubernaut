# BR-ORCH-042: Consecutive Failure Blocking with Cooldown

**ID**: BR-ORCH-042
**Title**: Consecutive Failure Blocking with Automatic Cooldown
**Category**: ORCH (Remediation Orchestrator)
**Priority**: 🔴 P0 (V1.0)
**Version**: 1.1
**Date**: December 10, 2025
**Status**: 🚧 IN PROGRESS
**Related**: DD-GATEWAY-011, BR-GATEWAY-184 (superseded), BR-GATEWAY-185 (field selectors)

---

## Business Context

### Problem Statement

When a signal repeatedly fails remediation (e.g., due to missing RBAC, persistent infrastructure issues, or unresolvable problems), the system would continuously:
1. Create new RemediationRequests
2. Spawn child CRDs (SP, AI, WE)
3. Fail again
4. Repeat indefinitely

This wastes resources, creates noise, and masks the underlying issue requiring human intervention.

### Previous Design (Superseded)

BR-GATEWAY-184 placed this logic at Gateway:
- Gateway counted consecutive failures
- Gateway created RR with `OverallPhase=Blocked`
- **Problems**: Gateway made routing decisions, needed historical RR queries, mixed concerns

### New Design (This BR)

RO owns blocking logic because:
- RO knows *why* failures happened (timeout, workflow failure, approval rejection)
- RO already tracks recovery attempts
- Routing decisions are orchestration responsibility
- Gateway should be a "dumb pipe" for signal ingestion

---

## Requirements

### BR-ORCH-042.1: Consecutive Failure Detection

**MUST**: RO SHALL detect when a RemediationRequest completes as `Failed` and check if this is the 3rd or more consecutive failure for the same signal fingerprint.

**Fingerprint Lookup Strategy**:

> **IMPORTANT**: RO SHALL use **field selectors on `spec.signalFingerprint`** (not labels) for RR lookup.
>
> | Aspect | Label-Based (❌ Avoid) | Field Selector (✅ Required) |
> |--------|------------------------|------------------------------|
> | **Field** | `metadata.labels.kubernaut.ai/signal-fingerprint` | `spec.signalFingerprint` |
> | **Length** | 63 chars (K8s label limit) | **64 chars (full SHA256)** |
> | **Mutability** | Mutable (can be changed) | **Immutable** (kubebuilder validation) |
> | **Source** | Copy of fingerprint | **Authoritative source** |
>
> **Rationale**: `spec.signalFingerprint` is immutable (enforced by kubebuilder), supports the full 64-char SHA256, and is the authoritative source of truth. Labels are mutable and truncated.

**Implementation**:
```go
// SetupWithManager - create field index for O(1) fingerprint lookup
func (r *Reconciler) SetupWithManager(mgr ctrl.Manager) error {
    // BR-ORCH-042: Index on spec.signalFingerprint for consecutive failure counting
    if err := mgr.GetFieldIndexer().IndexField(
        context.Background(),
        &remediationv1.RemediationRequest{},
        "spec.signalFingerprint",
        func(obj client.Object) []string {
            rr := obj.(*remediationv1.RemediationRequest)
            return []string{rr.Spec.SignalFingerprint}
        },
    ); err != nil {
        return fmt.Errorf("failed to create field index on spec.signalFingerprint: %w", err)
    }
    // ...
}

// countConsecutiveFailures - use field selector (not labels)
func (r *Reconciler) countConsecutiveFailures(ctx context.Context, fingerprint string) int {
    rrList := &remediationv1.RemediationRequestList{}

    // Use field selector on immutable spec field (not mutable labels)
    r.client.List(ctx, rrList,
        client.MatchingFields{"spec.signalFingerprint": fingerprint}, // Full 64-char fingerprint
    )

    // Sort by creation time, count consecutive Failed phases
    // ...
}

func (r *Reconciler) transitionToFailed(ctx context.Context, rr *remediationv1.RemediationRequest, ...) {
    // After marking as Failed, check consecutive failure count
    consecutiveFailures := r.countConsecutiveFailures(ctx, rr.Spec.SignalFingerprint)

    if consecutiveFailures >= 3 {
        // Don't transition to terminal Failed - hold in Blocked with cooldown
        return r.transitionToBlocked(ctx, rr, "consecutive_failures_exceeded", 1*time.Hour)
    }

    // Normal terminal Failed transition
    // ...
}
```

**Acceptance Criteria**:
| ID | Criterion | Test |
|----|-----------|------|
| AC-042-1-1 | RO counts consecutive Failed RRs for same fingerprint | Unit |
| AC-042-1-2 | Count resets on any Completed RR | Unit |
| AC-042-1-3 | Count uses chronological order (most recent first) | Unit |
| AC-042-1-4 | RO uses field selector on `spec.signalFingerprint` (not labels) | Unit |
| AC-042-1-5 | Field index created in SetupWithManager | Unit |

---

### BR-ORCH-042.2: Blocked Phase (Non-Terminal)

**MUST**: `Blocked` SHALL be a **non-terminal** phase, preventing Gateway from creating new RRs.

**Phase Classification Update**:
```go
// Terminal phases - Gateway can create new RR
var TerminalPhases = []Phase{Completed, Failed, Timeout}

// Active phases - Gateway updates dedup, doesn't create new RR
var ActivePhases = []Phase{Pending, Processing, Analyzing, Approving, Executing, Recovering, Blocked}
```

**Acceptance Criteria**:
| ID | Criterion | Test |
|----|-----------|------|
| AC-042-2-1 | `IsTerminal(Blocked)` returns `false` | Unit |
| AC-042-2-2 | Gateway seeing active `Blocked` RR updates dedup, doesn't create new | Integration |

---

### BR-ORCH-042.3: Automatic Cooldown Expiry

**MUST**: RO SHALL automatically transition `Blocked` RRs to terminal `Failed` after the cooldown period (default: 1 hour).

**New Status Fields**:
```go
type RemediationRequestStatus struct {
    // ... existing fields ...

    // BlockedUntil specifies when this fingerprint can be retried
    // Set when OverallPhase=Blocked due to consecutive failures
    // +optional
    BlockedUntil *metav1.Time `json:"blockedUntil,omitempty"`

    // BlockReason explains why this RR is blocked
    // Values: "consecutive_failures_exceeded", "manual_block"
    // +optional
    BlockReason *string `json:"blockReason,omitempty"`
}
```

**Reconciliation Logic**:
```go
func (r *Reconciler) handleBlockedPhase(ctx context.Context, rr *remediationv1.RemediationRequest) (ctrl.Result, error) {
    if rr.Status.BlockedUntil == nil {
        // Manual block - no auto-expiry
        return ctrl.Result{}, nil
    }

    if time.Now().After(rr.Status.BlockedUntil.Time) {
        logger.Info("Blocked cooldown expired, transitioning to Failed")
        return r.transitionToFailed(ctx, rr, "blocked", "Cooldown expired after consecutive failures")
    }

    // Requeue at expiry time
    requeueAfter := time.Until(rr.Status.BlockedUntil.Time)
    return ctrl.Result{RequeueAfter: requeueAfter}, nil
}
```

**Acceptance Criteria**:
| ID | Criterion | Test |
|----|-----------|------|
| AC-042-3-1 | RO sets `BlockedUntil` = now + 1h when blocking | Unit |
| AC-042-3-2 | RO transitions to Failed when cooldown expires | Unit |
| AC-042-3-3 | RO requeues at exact expiry time (efficient) | Unit |
| AC-042-3-4 | After expiry, Gateway can create new RR for fingerprint | E2E |

---

### BR-ORCH-042.4: Manual Unblock

**SHOULD**: Operators SHALL be able to manually unblock a fingerprint by deleting the Blocked RR or updating its phase.

**Acceptance Criteria**:
| ID | Criterion | Test |
|----|-----------|------|
| AC-042-4-1 | Deleting Blocked RR allows Gateway to create new | Integration |
| AC-042-4-2 | Updating phase to Failed allows Gateway to create new | Integration |

---

### BR-ORCH-042.5: Notification on Block

**SHOULD**: RO SHALL create a NotificationRequest when blocking a signal fingerprint.

**Notification Type**: `consecutive_failures_blocked`

**Acceptance Criteria**:
| ID | Criterion | Test |
|----|-----------|------|
| AC-042-5-1 | NotificationRequest created when RR enters Blocked | Unit |
| AC-042-5-2 | Notification includes fingerprint, failure count, cooldown expiry | Unit |

---

## Metrics

```go
// New metrics for blocking feature
var (
    BlockedTotal = prometheus.NewCounterVec(
        prometheus.CounterOpts{
            Name: "remediationorchestrator_blocked_total",
            Help: "Total RemediationRequests blocked due to consecutive failures",
        },
        []string{"namespace", "reason"},
    )

    BlockedCooldownExpiredTotal = prometheus.NewCounter(
        prometheus.CounterOpts{
            Name: "remediationorchestrator_blocked_cooldown_expired_total",
            Help: "Total blocked RRs that expired and transitioned to Failed",
        },
    )

    CurrentBlockedGauge = prometheus.NewGaugeVec(
        prometheus.GaugeOpts{
            Name: "remediationorchestrator_blocked_current",
            Help: "Current number of blocked RRs per fingerprint",
        },
        []string{"namespace"},
    )
)
```

---

## Configuration

```yaml
# Configurable via environment or ConfigMap
consecutiveFailureBlocking:
  enabled: true
  threshold: 3                    # Block after N consecutive failures
  cooldownDuration: 1h            # How long to block before auto-retry
  notifyOnBlock: true             # Create NotificationRequest when blocking
```

---

## Gateway Impact (DD-GATEWAY-011 Update)

Gateway logic simplifies to:

```go
func (g *Gateway) HandleSignal(ctx context.Context, signal Signal) error {
    fingerprint := signal.Fingerprint()

    // Check for ANY active (non-terminal) RR with this fingerprint
    activeRR := g.findActiveRR(ctx, fingerprint)

    if activeRR != nil {
        // Active RR exists - update deduplication, don't create new
        return g.updateDeduplication(ctx, activeRR, signal)
    }

    // No active RR - create new one
    return g.createRemediationRequest(ctx, signal)
}

func (g *Gateway) findActiveRR(ctx context.Context, fingerprint string) *remediationv1.RemediationRequest {
    rrList := &remediationv1.RemediationRequestList{}

    // Use field selector on immutable spec.signalFingerprint (not mutable labels)
    // See BR-GATEWAY-185 v1.1 for rationale
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

**Note**: Gateway NO LONGER counts consecutive failures or creates Blocked RRs.

**Note**: Gateway SHOULD use field selectors on `spec.signalFingerprint` (not labels) per BR-GATEWAY-185 v1.1.

---

### BR-ORCH-042.5: Ineffective Remediation Chain Detection (Issue #214)

**MUST**: RO SHALL detect consecutive remediations that complete successfully but are ineffective (resource keeps reverting or health does not improve).

**Detection Algorithm** (three layers, applied in `CheckPostAnalysisConditions` after all other checks):

1. **Layer 1+2 (Hash chain + spec_drift)**: Walk DataStorage `Tier1.Chain` entries backwards. An entry is ineffective if:
   - Its `PreRemediationSpecHash` matches the current RR's `preRemediationSpecHash` (hash chain continuity -- resource reverted to same bad state), OR
   - Its `HashMatch == "preRemediation"` (regression/spec_drift detected by EffectivenessMonitor)
   - Block when consecutive ineffective entries >= `IneffectiveChainThreshold` (default: 3)

2. **Layer 3 (Safety net)**: Count total DS entries within `IneffectiveTimeWindow` (default: 4h). Block when count >= `RecurrenceCountThreshold` (default: 5), even without conclusive hash data.

**Error handling**: DataStorage query failures fail-open (log and return nil).

**Escalation**: On detection, RR transitions to `PhaseBlocked` with `BlockReasonIneffectiveChain`, `Outcome = "ManualReviewRequired"`, `RequiresManualReview = true`. `RequeueAfter` = `IneffectiveTimeWindow`.

**Pre-remediation hash**: `CapturePreRemediationHash` is called BEFORE routing. If `hashErr != nil`, the RR transitions to `Failed` (terminal). If `preHash == ""` with no error, hash-based checks are skipped but the RR proceeds.

**Acceptance Criteria**:

| ID | Criterion | Test |
|----|-----------|------|
| AC-042-5-1 | Hash chain match across N entries triggers IneffectiveChain block | UT-RO-214-001 |
| AC-042-5-2 | Spec drift (HashMatch == preRemediation) triggers IneffectiveChain block | UT-RO-214-002 |
| AC-042-5-3 | Chain broken by effective entry returns nil | UT-RO-214-003 |
| AC-042-5-4 | Missing hash data breaks chain | UT-RO-214-004 |
| AC-042-5-5 | Below threshold returns nil | UT-RO-214-005 |
| AC-042-5-6 | Safety net triggers on recurrence count | UT-RO-214-006 |
| AC-042-5-7 | Stale entries outside window ignored | UT-RO-214-007 |
| AC-042-5-8 | DS query failures fail-open | DS fail-open test |
| AC-042-5-9 | CapturePreRemediationHash hashErr terminal | UT-RO-214-010 |

---

## Supersedes

- **BR-GATEWAY-184**: Consecutive Failure Blocking (moved from Gateway to RO)

---

## Test Coverage

| Tier | Tests | Coverage |
|------|-------|----------|
| Unit | 8 | `consecutive_failure_test.go` |
| Unit | 10 | `ineffective_chain_test.go` (Issue #214) |
| Integration | 4 | `blocking_integration_test.go` |
| E2E | 2 | `blocking_e2e_test.go` |

---

## Version History

| Version | Date | Changes |
|---------|------|---------|
| 1.0 | 2025-12-10 | Initial version - moved from Gateway (BR-GATEWAY-184) to RO |
| 1.1 | 2025-12-10 | Updated to use field selector on `spec.signalFingerprint` (not labels) per BR-GATEWAY-185 v1.1. Added AC-042-1-4, AC-042-1-5. |
| 1.2 | 2026-02-28 | Added BR-ORCH-042.5: Ineffective Remediation Chain Detection (Issue #214). Three-layer detection using DataStorage audit traces. |

