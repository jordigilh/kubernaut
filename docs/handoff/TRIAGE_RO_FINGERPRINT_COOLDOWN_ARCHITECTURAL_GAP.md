# TRIAGE: RO Fingerprint Cooldown - Critical Architectural Gap

**Date**: December 14, 2025
**Priority**: 🔴 P0 CRITICAL (Resource Waste + Architectural Correctness)
**Status**: 🚨 DESIGN GAP DISCOVERED
**Impact**: MODERATE (wastes SP/AI/WE resources, but system still functions)

---

## 🚨 Problem Statement

### Current Behavior (WASTEFUL)

When a signal with the **same fingerprint** arrives after successful remediation:

```
Gateway: RR already terminal (Completed) → Create NEW RR with same fingerprint
    ↓
RO (Pending phase): ✅ Create SignalProcessing CRD
    ↓
SP completes: Enriches signal data
    ↓
RO (Processing phase): ✅ Create AIAnalysis CRD
    ↓
AI completes: Analyzes context (might say "cannot reproduce problem")
    ↓
RO (Analyzing phase): ✅ Create WorkflowExecution CRD
    ↓
WE (Pending phase): 🚫 Check cooldown → Skip with "RecentlyRemediated"
    ↓
RO: Mark RR as Skipped

❌ WASTED: SP resources, AI API calls, WE creation
```

### Desired Behavior (EFFICIENT)

```
Gateway: RR already terminal (Completed) → Create NEW RR with same fingerprint
    ↓
RO (Pending phase): 🔍 Check recent terminal RRs with same fingerprint
    ↓
    Found within cooldown (5 min) → 🚫 Skip immediately
    ↓
RR.status.phase = "Skipped"
RR.status.skipReason = "RecentlyRemediated"
RR.status.cooldownRemaining = "3m15s"
RR.status.duplicateOf = "rr-abc123-previous"

✅ SAVED: No SP, no AI, no WE created
```

---

## 📊 Resource Impact Analysis

### Per Wasteful Remediation

| Resource | Current (Wasteful) | Proposed (Efficient) | Savings |
|----------|-------------------|---------------------|---------|
| **SignalProcessing** | 1 CRD created | 0 CRDs created | **100%** |
| **AIAnalysis** | 1 CRD + AI API call | 0 CRDs + 0 API calls | **100%** |
| **WorkflowExecution** | 1 CRD created | 0 CRDs created | **100%** |
| **etcd writes** | ~6-8 writes | ~2 writes | **75%** |
| **Controller reconciliations** | ~15-20 | ~3-5 | **70%** |

### Projected Volume

**Scenario**: High-frequency flapping alert (e.g., memory pressure oscillating)

```
Alert fires every 30s while condition persists (Prometheus default)
Remediation completes successfully in 2 minutes
Alert continues for 5 minutes (10 total firings)

Current Flow:
  - Gateway deduplicates 9/10 → 1 RR created during remediation
  - After completion (next 5 min): 10 more firings
  - Gateway creates 10 new RRs (all terminal → not duplicates)
  - RO creates: 10 SP + 10 AI + 10 WE
  - WE skips all 10 with "RecentlyRemediated"
  - WASTE: 10 SP, 10 AI, 10 WE (all useless)

Proposed Flow:
  - Gateway creates 10 new RRs (same as current)
  - RO checks fingerprint cooldown → Skip all 10 immediately
  - SAVED: 10 SP, 10 AI, 10 WE
```

**Estimated Savings**: 30-40% reduction in downstream CRD creation for flapping alerts

---

## 🏗️ Architecture Analysis

### Why WE's Cooldown Check is Wrong Place

| Factor | WE Cooldown Check (Current) | RO Cooldown Check (Proposed) |
|--------|----------------------------|------------------------------|
| **Timing** | After SP + AI work done | Before any downstream work |
| **Resource Waste** | ❌ High (SP, AI, WE all created) | ✅ None (skip immediately) |
| **Routing Decision** | ❌ Wrong layer (WE is executor) | ✅ Correct layer (RO is orchestrator) |
| **Fingerprint Awareness** | ❌ WE doesn't have fingerprint | ✅ RO has `spec.signalFingerprint` |
| **Same Signal Logic** | ❌ Checks `targetResource + workflowId` | ✅ Checks `fingerprint` (signal identity) |

### Authority Analysis

**DD-WE-001** (Resource Locking Safety):
> "WE checks cooldown for **same workflow + same target resource**"
> - **Purpose**: Prevent concurrent executions on same resource
> - **Mechanism**: `targetResource` + `workflowId` lookup

**BR-WE-010** (Cooldown - Prevent Redundant Execution):
> "Skip if same workflow executed within 5 minutes on same target"
> - **Scope**: Workflow-level cooldown (execution safety)
> - **Goal**: Resource stabilization time

**Missing Document**: **DD-RO-XXX (Signal-Level Cooldown)**
> "RO should check cooldown for **same fingerprint** before creating SP"
> - **Purpose**: Prevent wasteful re-processing of same signal
> - **Mechanism**: `spec.signalFingerprint` lookup
> - **Goal**: Resource efficiency + same-signal deduplication

---

## 🔍 Key Insight: Two Types of Cooldown

### Type 1: Signal-Level Cooldown (RO Responsibility - MISSING)

**What**: Same **signal fingerprint** within cooldown window
**Who**: Remediation Orchestrator (RO)
**When**: Pending phase (before creating SP)
**Why**: Same signal → same analysis → same workflow → waste of resources
**Check**: `spec.signalFingerprint` against recent terminal RRs

**Example**:
```
Signal: pod/myapp-xyz crash loop (fingerprint: abc123)
  ↓
RR-1: Successfully remediated (increased memory limit)
  ↓
5 minutes later...
  ↓
Signal: pod/myapp-xyz crash loop (fingerprint: abc123)  ← SAME FINGERPRINT
  ↓
RO: "Same signal recently remediated successfully → Skip SP/AI/WE"
```

### Type 2: Workflow-Level Cooldown (WE Responsibility - EXISTS)

**What**: Same **workflow + target resource** within cooldown window
**Who**: WorkflowExecution (WE)
**When**: Pending phase (before creating PipelineRun)
**Why**: Resource needs stabilization time after workflow execution
**Check**: `workflowId + targetResource` against recent terminal WEs

**Example**:
```
Signal A: node/worker-1 disk pressure (fingerprint: aaa)
  ↓
WE-1: Executes node-disk-cleanup successfully
  ↓
5 minutes later...
  ↓
Signal B: node/worker-1 memory pressure (fingerprint: bbb)  ← DIFFERENT FINGERPRINT
  ↓ (Different signal, different AI analysis, different workflow)
RO: "Different signal → Create SP/AI"
  ↓
AI: Recommends same workflow (node-disk-cleanup)
  ↓
RO: "Create WE"
  ↓
WE: "Same workflow + target within cooldown → Skip"
```

---

## 🎯 Proposed Solution

### Implementation: RO Pending Phase Enhancement

**Location**: `internal/controller/remediationorchestrator/remediationrequest_controller.go`

**Pseudocode**:
```go
func (r *Reconciler) reconcilePending(
    ctx context.Context,
    rr *remediationv1.RemediationRequest,
) (ctrl.Result, error) {
    log := log.FromContext(ctx)

    // NEW: Check signal-level cooldown BEFORE creating SP
    inCooldown, parentRR, cooldownRemaining, err := r.checkSignalCooldown(ctx, rr)
    if err != nil {
        return ctrl.Result{}, fmt.Errorf("failed to check signal cooldown: %w", err)
    }

    if inCooldown {
        // Same signal recently remediated → Skip immediately
        log.Info("Signal within cooldown period, skipping SP/AI/WE creation",
            "fingerprint", rr.Spec.SignalFingerprint,
            "parentRR", parentRR.Name,
            "cooldownRemaining", cooldownRemaining)

        // Update RR status
        rr.Status.OverallPhase = "Skipped"
        rr.Status.SkipReason = "RecentlyRemediated"
        rr.Status.DuplicateOf = parentRR.Name
        rr.Status.Message = fmt.Sprintf(
            "Same signal recently remediated successfully. Cooldown: %s remaining",
            cooldownRemaining,
        )

        if err := r.Status().Update(ctx, rr); err != nil {
            return ctrl.Result{}, err
        }

        // Track duplicate on parent RR
        if err := r.trackDuplicateOnParent(ctx, parentRR.Name, rr.Name); err != nil {
            log.Error(err, "Failed to track duplicate", "parent", parentRR.Name)
            // Non-fatal
        }

        // Send notification (optional)
        if r.Config.NotifySkippedDuplicates {
            r.createSkippedNotification(ctx, rr, parentRR)
        }

        // Requeue at cooldown expiration
        return ctrl.Result{RequeueAfter: cooldownRemaining}, nil
    }

    // NOT in cooldown → proceed with normal flow (create SP)
    return r.createSignalProcessing(ctx, rr)
}

func (r *Reconciler) checkSignalCooldown(
    ctx context.Context,
    rr *remediationv1.RemediationRequest,
) (inCooldown bool, parentRR *remediationv1.RemediationRequest, remaining time.Duration, err error) {
    fingerprint := rr.Spec.SignalFingerprint
    if fingerprint == "" {
        return false, nil, 0, nil // No fingerprint → allow
    }

    // Query for recent terminal RRs with same fingerprint
    rrList := &remediationv1.RemediationRequestList{}
    listOpts := []client.ListOption{
        client.InNamespace(rr.Namespace),
        client.MatchingFields{"spec.signalFingerprint": fingerprint},
    }

    if err := r.List(ctx, rrList, listOpts...); err != nil {
        return false, nil, 0, fmt.Errorf("failed to list RRs: %w", err)
    }

    // Find most recent successfully completed RR
    var mostRecent *remediationv1.RemediationRequest
    for i := range rrList.Items {
        item := &rrList.Items[i]

        // Skip self
        if item.Name == rr.Name {
            continue
        }

        // Only consider successfully completed
        if item.Status.OverallPhase != "Completed" {
            continue
        }

        // Track most recent
        if mostRecent == nil || item.Status.CompletedAt.After(mostRecent.Status.CompletedAt.Time) {
            mostRecent = item
        }
    }

    if mostRecent == nil {
        return false, nil, 0, nil // No recent completion → allow
    }

    // Check cooldown window (default: 5 minutes, matches WE cooldown)
    cooldownWindow := r.Config.SignalCooldownDuration // Default: 5 * time.Minute
    timeSinceCompletion := time.Since(mostRecent.Status.CompletedAt.Time)

    if timeSinceCompletion < cooldownWindow {
        remaining := cooldownWindow - timeSinceCompletion
        return true, mostRecent, remaining, nil
    }

    return false, nil, 0, nil // Cooldown expired → allow
}
```

### Configuration

**New Config Field** (`internal/controller/remediationorchestrator/config.go`):
```go
type Config struct {
    // ... existing fields ...

    // SignalCooldownDuration is the cooldown period for same-fingerprint signals
    // after successful remediation. Default: 5 minutes (matches WE cooldown).
    SignalCooldownDuration time.Duration `json:"signalCooldownDuration"`

    // NotifySkippedDuplicates controls whether to send notifications for
    // signal-level cooldown skips. Default: false (rely on bulk notification).
    NotifySkippedDuplicates bool `json:"notifySkippedDuplicates"`
}

func DefaultConfig() *Config {
    return &Config{
        // ... existing defaults ...
        SignalCooldownDuration:  5 * time.Minute,
        NotifySkippedDuplicates: false,
    }
}
```

### Field Selector Index (Required)

**Add to controller setup** (`internal/controller/remediationorchestrator/remediationrequest_controller.go`):
```go
func (r *RemediationRequestReconciler) SetupWithManager(mgr ctrl.Manager) error {
    // Index RemediationRequests by spec.signalFingerprint for efficient lookup
    if err := mgr.GetFieldIndexer().IndexField(
        context.Background(),
        &remediationv1.RemediationRequest{},
        "spec.signalFingerprint",
        func(obj client.Object) []string {
            rr := obj.(*remediationv1.RemediationRequest)
            if rr.Spec.SignalFingerprint == "" {
                return nil
            }
            return []string{rr.Spec.SignalFingerprint}
        },
    ); err != nil {
        return err
    }

    // ... existing setup ...
}
```

---

## 📋 Decision Matrix

### Option A: Keep Current Behavior (WE Cooldown Only)

**Pros**:
- ✅ No code changes needed
- ✅ Simple (one cooldown mechanism)

**Cons**:
- ❌ Wastes SP resources (1 CRD per duplicate signal)
- ❌ Wastes AI API calls ($$$)
- ❌ Wastes WE creation (1 CRD per duplicate signal)
- ❌ Routing decision at wrong layer (WE shouldn't know about signals)
- ❌ AI might return "cannot reproduce" (waste of analysis)

**Verdict**: ❌ Not recommended - architectural correctness issue

---

### Option B: Add RO Signal-Level Cooldown (RECOMMENDED)

**Pros**:
- ✅ Prevents wasteful downstream CRD creation
- ✅ Routing decision at correct layer (RO owns orchestration)
- ✅ Matches user's architectural expectations
- ✅ Saves SP/AI/WE resources (30-40% reduction for flapping alerts)
- ✅ Same fingerprint = same signal = logically correct to skip

**Cons**:
- ⚠️ Requires new RO logic (~200 lines of code)
- ⚠️ Requires field selector index
- ⚠️ Requires new config parameter

**Verdict**: ✅ **RECOMMENDED** - correct architectural layer + resource efficiency

---

## 🔗 Related Documents

### Existing Design Decisions

| Document | Relevance |
|----------|-----------|
| **DD-WE-001** | Resource locking (workflow-level cooldown) |
| **BR-WE-010** | Workflow cooldown requirements |
| **DD-GATEWAY-011** | Phase-based deduplication (Gateway → RO) |
| **DD-RO-001** | Resource lock deduplication handling |
| **BR-ORCH-042** | Consecutive failure blocking (fingerprint tracking) |

### New Design Decision Required

**DD-RO-XXX: Signal-Level Cooldown for Resource Efficiency**
- **Status**: MISSING (gap discovered)
- **Purpose**: Define RO's responsibility for same-fingerprint cooldown
- **Scope**: Pending phase validation before SP creation
- **Integration**: Complements WE's workflow-level cooldown

---

## 🎯 Recommendation

**APPROVE** Option B: Add RO Signal-Level Cooldown

**Justification**:
1. **Architectural Correctness**: RO owns routing decisions, not WE
2. **Resource Efficiency**: 30-40% reduction in downstream CRDs for flapping alerts
3. **User Expectation**: Matches user's intuition ("stop it at GW → RO → X")
4. **Same Signal Logic**: Same fingerprint = same problem = skip immediately

**Implementation Priority**: P1 (V1.1 enhancement, not V1.0 blocker)

**Rationale for P1**:
- System functions correctly without it (WE cooldown catches it eventually)
- Resource waste is moderate, not catastrophic
- Can be added incrementally without breaking changes
- V1.0 focus is on correctness, V1.1 can optimize efficiency

---

## 📊 Success Metrics

### Before (Current)

```
Scenario: 10 duplicate signals within 5-minute cooldown

Metrics:
  - SignalProcessing CRDs created: 10
  - AIAnalysis CRDs created: 10
  - WorkflowExecution CRDs created: 10
  - WE skipped with "RecentlyRemediated": 10
  - Total etcd writes: ~60-80
```

### After (Proposed)

```
Scenario: 10 duplicate signals within 5-minute cooldown

Metrics:
  - SignalProcessing CRDs created: 0
  - AIAnalysis CRDs created: 0
  - WorkflowExecution CRDs created: 0
  - RR skipped with "RecentlyRemediated": 10
  - Total etcd writes: ~20
  - Savings: 70-75% etcd writes, 100% SP/AI/WE creation
```

---

**Document Version**: 1.0
**Last Updated**: December 14, 2025
**Status**: 🚨 AWAITING DECISION
**Next Steps**:
1. User approval for Option B
2. Create DD-RO-XXX design decision
3. Add to V1.1 implementation plan
4. Update RO reconciliation phases documentation

