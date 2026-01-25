# TimeoutConfig Migration to Status - Comprehensive Impact Triage

**Date**: January 12, 2026
**Context**: Gap #8 implementation requires TimeoutConfig to be in `status`, not `spec`
**Reason**: Spec is immutable (Gateway sets at creation), Status is mutable (operators can adjust)
**Priority**: 🔴 **CRITICAL** - Blocks Gap #8 implementation

---

## 🎯 **Executive Summary**

**Current State**: `TimeoutConfig` lives in `RemediationRequest.Spec` (line 338)
**Target State**: `TimeoutConfig` should live in `RemediationRequest.Status`
**Impact**: **141 references** to `Status.TimeoutConfig` across codebase
**Effort**: **1 day** (8 hours)
**Risk**: **Medium** - Comprehensive testing required

---

## 🔍 **Architecture Analysis**

### **Current Design (WRONG)**

```go
// api/remediation/v1alpha1/remediationrequest_types.go:334-338
type RemediationRequestSpec struct {
    // ... other fields ...

    // TimeoutConfig provides fine-grained timeout configuration for this remediation.
    // Overrides controller-level defaults when specified.
    // Reference: BR-ORCH-027 (Global timeout), BR-ORCH-028 (Per-phase timeouts)
    // +optional
    TimeoutConfig *TimeoutConfig `json:"timeoutConfig,omitempty"` // ❌ WRONG LOCATION
}
```

**Problems**:
1. ❌ **Gateway must set at creation** (but Gateway has no timeout knowledge)
2. ❌ **Operators cannot adjust mid-remediation** (spec is immutable)
3. ❌ **Inflexible**: "this remediation is taking longer, extend timeout" → **IMPOSSIBLE**
4. ❌ **Wrong semantic**: TimeoutConfig is **operational state**, not **desired state**

---

### **Target Design (CORRECT)**

```go
// api/remediation/v1alpha1/remediationrequest_types.go:373+
type RemediationRequestStatus struct {
    // ... existing status fields ...

    // ========================================
    // TIMEOUT CONFIGURATION (BR-ORCH-027/028)
    // ========================================

    // TimeoutConfig provides operational timeout overrides for this remediation.
    // OWNER: Remediation Orchestrator (sets defaults on first reconcile)
    // MUTABLE BY: Operators (can adjust mid-remediation via kubectl edit)
    // Reference: BR-ORCH-027 (Global timeout), BR-ORCH-028 (Per-phase timeouts)
    // +optional
    TimeoutConfig *TimeoutConfig `json:"timeoutConfig,omitempty"` // ✅ CORRECT LOCATION
}
```

**Advantages**:
1. ✅ **Gateway creates RR with `timeoutConfig: nil`** (no timeout knowledge needed)
2. ✅ **RO sets defaults in status on first reconcile** (e.g., 1h global, 10m analyzing)
3. ✅ **Operators can adjust**: `kubectl edit rr rr-123 → status.timeoutConfig.global = 2h`
4. ✅ **RO reads from `status.timeoutConfig`** throughout reconciliation
5. ✅ **Correct semantic**: Status = **observed operational state** (mutable)

---

## 📊 **Impact Analysis**

### **Gateway Service** ✅ **NO IMPACT**

**Current Behavior**:
```go
// pkg/gateway/processing/crd_creator.go:369-403
Spec: remediationv1alpha1.RemediationRequestSpec{
    SignalFingerprint: signal.Fingerprint,
    SignalName:        signal.AlertName,
    // ... other fields ...
    // ✅ TimeoutConfig NOT SET (Gateway doesn't populate it)
},
```

**Analysis**:
- ✅ Gateway **NEVER sets TimeoutConfig** (confirmed in crd_creator.go:369-403)
- ✅ Gateway creates RR with `status.timeoutConfig = nil`
- ✅ **NO CODE CHANGES NEEDED** in Gateway service

**Confidence**: **100%** - Gateway is unaffected

---

### **Remediation Orchestrator** ⚠️ **HIGH IMPACT**

**Files Affected**: **141 references** to `Status.TimeoutConfig`

#### **1. Reconciler Core** (internal/controller/remediationorchestrator/reconciler.go)

**Impact**: **11 references** to `rr.Status.TimeoutConfig`

| Line | Function | Change Required |
|------|----------|----------------|
| 318 | `Reconcile()` | `rr.Status.TimeoutConfig` → `rr.Status.TimeoutConfig` |
| 328 | `Reconcile()` | `rr.Status.TimeoutConfig != nil` → `rr.Status.TimeoutConfig != nil` |
| 1956-1957 | `getGlobalTimeout()` | `rr.Status.TimeoutConfig.Global` → `rr.Status.TimeoutConfig.Global` |
| 1966-1978 | `getPhaseTimeout()` | `rr.Status.TimeoutConfig.Processing/Analyzing/Executing` → `rr.Status.TimeoutConfig.*` |
| 2039 | `logTimeoutConfiguration()` | `rr.Status.TimeoutConfig` → `rr.Status.TimeoutConfig` |
| 2271-2292 | `validateTimeoutConfig()` | **ENTIRE FUNCTION** needs update |

**Critical Addition**: **Initialize defaults on first reconcile**

```go
// NEW FUNCTION REQUIRED
func (r *RemediationOrchestratorReconciler) initializeTimeoutDefaults(
    ctx context.Context,
    rr *remediationv1.RemediationRequest,
) error {
    // Only initialize if status.timeoutConfig is nil (first reconcile)
    if rr.Status.TimeoutConfig != nil {
        return nil // Already initialized
    }

    // Set defaults from controller config
    rr.Status.TimeoutConfig = &remediationv1.TimeoutConfig{
        Global:     &metav1.Duration{Duration: r.timeoutConfig.GlobalTimeout},
        Processing: &metav1.Duration{Duration: r.timeoutConfig.ProcessingTimeout},
        Analyzing:  &metav1.Duration{Duration: r.timeoutConfig.AnalyzingTimeout},
        Executing:  &metav1.Duration{Duration: r.timeoutConfig.ExecutingTimeout},
    }

    // Update status
    if err := r.Status().Update(ctx, rr); err != nil {
        return fmt.Errorf("failed to initialize timeout defaults: %w", err)
    }

    r.logger.Info("Initialized timeout defaults in status",
        "name", rr.Name,
        "namespace", rr.Namespace,
        "global", r.timeoutConfig.GlobalTimeout,
        "processing", r.timeoutConfig.ProcessingTimeout,
        "analyzing", r.timeoutConfig.AnalyzingTimeout,
        "executing", r.timeoutConfig.ExecutingTimeout)

    return nil
}
```

**Call Site** (reconciler.go:~283-288):
```go
// Initialize timeout defaults on first reconcile
if err := r.initializeTimeoutDefaults(ctx, rr); err != nil {
    return ctrl.Result{}, fmt.Errorf("failed to initialize timeout defaults: %w", err)
}
```

---

#### **2. Timeout Detector** (pkg/remediationorchestrator/timeout/detector.go)

**Impact**: **8 references** to `rr.Status.TimeoutConfig`

| Line | Function | Change Required |
|------|----------|----------------|
| 83-84 | `DetectGlobalTimeout()` | `rr.Status.TimeoutConfig.Global` → `rr.Status.TimeoutConfig.Global` |
| 146-158 | `getPhaseTimeout()` | `rr.Status.TimeoutConfig.Processing/Analyzing/Executing` → `rr.Status.TimeoutConfig.*` |

**Effort**: **30 minutes** (mechanical refactor)

---

#### **3. WorkflowExecution Creator** (pkg/remediationorchestrator/creator/workflowexecution.go)

**Impact**: **1 reference** to `rr.Status.TimeoutConfig`

| Line | Function | Change Required |
|------|----------|----------------|
| 186-188 | `CreateWorkflowExecution()` | `rr.Status.TimeoutConfig.Executing` → `rr.Status.TimeoutConfig.Executing` |

**Effort**: **15 minutes**

---

#### **4. Audit Manager** (pkg/remediationorchestrator/audit/manager.go)

**Impact**: **NEW** - Gap #8 implementation

**Change**: Capture `status.timeoutConfig` instead of `status.timeoutConfig`

```go
// BuildRemediationCreatedEvent builds an audit event for a new RemediationRequest creation.
// Per BR-AUDIT-005 Gap #8, this captures the initial state of the RR, including TimeoutConfig.
func (m *Manager) BuildRemediationCreatedEvent(
    correlationID string,
    namespace string,
    rrName string,
    timeoutConfig *remediationv1.TimeoutConfig, // From status, not spec
) (*ogenclient.AuditEventRequest, error) {
    // ... implementation ...

    // Gap #8: Capture TimeoutConfig from status
    if timeoutConfig != nil {
        timeoutConfigPayload := api.TimeoutConfigPayload{}
        if timeoutConfig.Global != nil {
            timeoutConfigPayload.Global.SetTo(timeoutConfig.Global.Duration.String())
        }
        // ... other fields ...
        payload.TimeoutConfig.SetTo(timeoutConfigPayload)
    }

    // ... rest of implementation ...
}
```

**Call Site** (reconciler.go:~283-288):
```go
// Emit orchestrator.lifecycle.created audit event (Gap #8)
event, err := r.auditManager.BuildRemediationCreatedEvent(
    rr.Name, // correlationID
    rr.Namespace,
    rr.Name,
    rr.Status.TimeoutConfig, // ✅ From status, not spec
)
```

**Effort**: **1 hour**

---

### **Test Files** ⚠️ **MEDIUM IMPACT**

**Files Affected**: **~20 test files**

#### **Unit Tests**

| File | References | Effort |
|------|-----------|--------|
| `test/unit/remediationorchestrator/timeout_detector_test.go` | 1 | 15 min |
| `test/unit/remediationorchestrator/workflowexecution_creator_test.go` | Unknown | 15 min |
| `test/unit/remediationorchestrator/controller/*.go` | Multiple | 1 hour |
| `test/shared/helpers/remediation.go` | 1 | 15 min |

**Pattern**:
```go
// BEFORE
rr.Status.TimeoutConfig = &remediationv1.TimeoutConfig{
    Global: &metav1.Duration{Duration: 2 * time.Hour},
}

// AFTER
rr.Status.TimeoutConfig = &remediationv1.TimeoutConfig{
    Global: &metav1.Duration{Duration: 2 * time.Hour},
}
```

#### **Integration Tests**

| File | References | Effort |
|------|-----------|--------|
| `test/integration/remediationorchestrator/timeout_integration_test.go` | 1 | 30 min |
| `test/integration/remediationorchestrator/audit_errors_integration_test.go` | Unknown | 30 min |

**Total Test Effort**: **~3 hours**

---

### **Documentation** ⚠️ **LOW IMPACT**

**Files Affected**: **~50 documentation files**

**Pattern**: Replace all `status.timeoutConfig` → `status.timeoutConfig` in docs

**Effort**: **1 hour** (find-replace + manual review)

---

### **CRD Schema** ⚠️ **CRITICAL**

**File**: `api/remediation/v1alpha1/remediationrequest_types.go`

**Changes**:
1. **Move `TimeoutConfig` field** from `RemediationRequestSpec` (line 338) to `RemediationRequestStatus` (after line 430)
2. **Update comments** to reflect ownership (RO initializes, operators can mutate)
3. **Regenerate CRD YAML**: `make manifests`
4. **Regenerate deepcopy**: `make generate`

**Effort**: **30 minutes**

---

## 🚨 **Breaking Change Analysis**

### **API Compatibility**

**Breaking Change**: ✅ **YES** - Field moved from spec to status

**Impact on Existing RRs**:
- ❌ **Existing RRs with `status.timeoutConfig` set**: Will be IGNORED by RO (reads from status)
- ✅ **Existing RRs with `status.timeoutConfig = nil`**: NO IMPACT (RO initializes status)

**Migration Strategy**:
```bash
# Option A: Manual migration (for production RRs with custom timeouts)
kubectl get rr rr-prod-123 -o yaml | \
  yq '.status.timeoutConfig = .status.timeoutConfig | del(.status.timeoutConfig)' | \
  kubectl apply -f -

# Option B: Let RO re-initialize (for most RRs)
# RO will initialize status.timeoutConfig with defaults on next reconcile
```

**Recommendation**: **Option B** - Let RO re-initialize (most RRs use defaults anyway)

---

### **Backward Compatibility**

**Question**: Should we support reading from `status.timeoutConfig` as fallback?

**Options**:

#### **Option A: Hard Cut-Over** (RECOMMENDED)
```go
// RO reads ONLY from status.timeoutConfig
timeout := rr.Status.TimeoutConfig.Global.Duration
```

**Pros**:
- ✅ Clean design
- ✅ No technical debt
- ✅ Forces correct usage

**Cons**:
- ❌ Breaks existing RRs with status.timeoutConfig set (rare)

---

#### **Option B: Fallback Support**
```go
// RO reads from status, falls back to spec
timeout := rr.Status.TimeoutConfig
if timeout == nil {
    timeout = rr.Status.TimeoutConfig // Fallback
}
```

**Pros**:
- ✅ Backward compatible

**Cons**:
- ❌ Technical debt
- ❌ Confusing: "which field is authoritative?"
- ❌ Delays migration

**Recommendation**: **Option A** - Hard cut-over (clean design)

---

## 📋 **Implementation Plan**

### **Phase 1: API Schema Changes** (1 hour)

1. **Move TimeoutConfig field**:
   - From: `RemediationRequestSpec` (line 338)
   - To: `RemediationRequestStatus` (after line 430)

2. **Update comments**:
   ```go
   // TimeoutConfig provides operational timeout overrides for this remediation.
   // OWNER: Remediation Orchestrator (sets defaults on first reconcile)
   // MUTABLE BY: Operators (can adjust mid-remediation via kubectl edit)
   // Reference: BR-ORCH-027 (Global timeout), BR-ORCH-028 (Per-phase timeouts)
   // +optional
   TimeoutConfig *TimeoutConfig `json:"timeoutConfig,omitempty"`
   ```

3. **Regenerate CRD**:
   ```bash
   make manifests
   make generate
   ```

---

### **Phase 2: Reconciler Changes** (2 hours)

1. **Add `initializeTimeoutDefaults()` function** (reconciler.go)
2. **Call initialization on first reconcile** (reconciler.go:~283-288)
3. **Update all `Status.TimeoutConfig` → `Status.TimeoutConfig`**:
   - `getGlobalTimeout()` (line 1956-1957)
   - `getPhaseTimeout()` (line 1966-1978)
   - `validateTimeoutConfig()` (line 2271-2292)
   - `logTimeoutConfiguration()` (line 2039)

---

### **Phase 3: Timeout Detector Changes** (30 minutes)

1. **Update `pkg/remediationorchestrator/timeout/detector.go`**:
   - `DetectGlobalTimeout()` (line 83-84)
   - `getPhaseTimeout()` (line 146-158)

---

### **Phase 4: WorkflowExecution Creator Changes** (15 minutes)

1. **Update `pkg/remediationorchestrator/creator/workflowexecution.go`**:
   - Line 186-188: `rr.Status.TimeoutConfig.Executing` → `rr.Status.TimeoutConfig.Executing`

---

### **Phase 5: Test Updates** (3 hours)

1. **Unit tests**: Update all `Status.TimeoutConfig` → `Status.TimeoutConfig`
2. **Integration tests**: Update timeout test setup
3. **Test helpers**: Update `test/shared/helpers/remediation.go`

---

### **Phase 6: Documentation Updates** (1 hour)

1. **Find-replace**: `status.timeoutConfig` → `status.timeoutConfig` in docs
2. **Manual review**: Ensure semantic correctness
3. **Update BR-ORCH-027/028**: Reflect new location

---

### **Phase 7: Gap #8 Implementation** (1 hour)

1. **Implement `BuildRemediationCreatedEvent()`** in audit manager
2. **Emit event on RR creation** (reconciler.go:~283-288)
3. **Capture `status.timeoutConfig`** in audit payload

---

## ⏱️ **Effort Summary**

| Phase | Task | Effort |
|-------|------|--------|
| 1 | API Schema Changes | 1 hour |
| 2 | Reconciler Changes | 2 hours |
| 3 | Timeout Detector Changes | 30 min |
| 4 | WorkflowExecution Creator Changes | 15 min |
| 5 | Test Updates | 3 hours |
| 6 | Documentation Updates | 1 hour |
| 7 | Gap #8 Implementation | 1 hour |
| **TOTAL** | **All Phases** | **8.75 hours (~1 day)** |

---

## 🎯 **Risk Assessment**

| Risk | Likelihood | Impact | Mitigation |
|------|-----------|--------|-----------|
| **Breaking existing RRs** | Low (most use defaults) | Medium | Document migration, provide kubectl command |
| **Test failures** | High (141 references) | High | Comprehensive test updates |
| **Missed references** | Medium | High | Grep for `Status.TimeoutConfig` before commit |
| **CRD regeneration issues** | Low | High | Test CRD apply in Kind cluster |

---

## ✅ **Success Criteria**

1. ✅ **All 141 references** to `Status.TimeoutConfig` updated to `Status.TimeoutConfig`
2. ✅ **RO initializes defaults** on first reconcile
3. ✅ **Operators can mutate** `status.timeoutConfig` via `kubectl edit`
4. ✅ **All tests pass** (unit, integration, E2E)
5. ✅ **Gap #8 implemented** with correct `status.timeoutConfig` capture
6. ✅ **Documentation updated** to reflect new location

---

## 🚀 **Recommendation**

**Proceed with migration** before Gap #8 implementation:

**Rationale**:
1. ✅ **Correct architecture** from day 1
2. ✅ **No rework** (avoid implementing Gap #8 twice)
3. ✅ **Operator value** (mutability is key feature)
4. ✅ **Clean audit trail** (Gap #8 captures from correct location)

**Timeline**:
- **Day 1**: TimeoutConfig migration (8 hours)
- **Day 2**: Gap #8 implementation (2 hours)

**Total**: **1.5 days** (vs 0.5 days if we skip migration, but with technical debt)

---

## 📝 **Next Steps**

1. **User approval** for migration approach
2. **Begin Phase 1**: API schema changes
3. **Incremental commits**: One phase at a time
4. **Continuous testing**: Run tests after each phase
5. **Gap #8 implementation**: After migration complete

---

**Status**: ⏸️ **AWAITING USER APPROVAL**
**Decision Required**: Proceed with TimeoutConfig migration to status?
