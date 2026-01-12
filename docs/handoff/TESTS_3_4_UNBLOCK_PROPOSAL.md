# Tests 3 & 4 Unblock Proposal
**Date**: 2025-12-12
**Author**: AI Assistant
**Status**: 🔴 AWAITING DECISION
**Priority**: P1 (BR-ORCH-028)

---

## 🎯 **Executive Summary**

Tests 3 and 4 are blocked by architectural decisions. This document proposes solutions to unblock both tests and complete BR-ORCH-028 (Per-Phase Timeout Management).

**Impact**: Completing these tests delivers:
- ✅ AC-027-4: Per-remediation timeout override
- ✅ AC-028-1 through AC-028-5: Full per-phase timeout support
- ✅ 75% → 100% BR-ORCH-027/028 coverage

---

## 🚧 **Test 3: Per-Remediation Timeout Override**

### **Current Blocker**

❌ **Schema Decision Required**: CRD needs `status.timeoutConfig` field

### **Business Requirement**

**BR-ORCH-028 AC-027-4**: Per-remediation override supported

**Use Case**: Complex workflows need longer timeouts (e.g., multi-region deployments need 2-hour timeout vs default 1 hour)

### **Proposed Solution**

#### **Option A: Simple Duration Override (RECOMMENDED)**

Add minimal field to CRD spec:

```go
// In api/remediation/v1alpha1/remediationrequest_types.go

type RemediationRequestSpec struct {
    // ... existing fields ...

    // GlobalTimeout overrides the default global timeout for this remediation.
    // Default: 1 hour (if not specified or zero)
    // Reference: BR-ORCH-028, AC-027-4
    // +optional
    // +kubebuilder:validation:Format=duration
    GlobalTimeout *metav1.Duration `json:"globalTimeout,omitempty"`
}
```

**Implementation Effort**: 30 minutes
**Test Effort**: 20 minutes
**Breaking Change**: No (optional field)

#### **Option B: Comprehensive Timeout Config (FUTURE-PROOF)**

```go
// TimeoutConfig provides fine-grained timeout configuration.
type TimeoutConfig struct {
    // Global timeout for entire remediation (overrides default 1 hour)
    // +optional
    Global *metav1.Duration `json:"global,omitempty"`

    // Per-phase timeouts (future extension for BR-ORCH-028)
    // +optional
    Processing *metav1.Duration `json:"processing,omitempty"`
    // +optional
    Analyzing *metav1.Duration `json:"analyzing,omitempty"`
    // +optional
    Executing *metav1.Duration `json:"executing,omitempty"`
}

type RemediationRequestSpec struct {
    // ... existing fields ...

    // TimeoutConfig allows per-remediation timeout customization.
    // Reference: BR-ORCH-028
    // +optional
    TimeoutConfig *TimeoutConfig `json:"timeoutConfig,omitempty"`
}
```

**Implementation Effort**: 1 hour
**Test Effort**: 45 minutes
**Breaking Change**: No (optional field)

#### **Option C: Controller-Level Only (MINIMAL)**

No CRD change - timeouts only configurable at controller level via flags/ConfigMap.

**Pros**: Zero schema change
**Cons**: ❌ Fails AC-027-4 (per-remediation requirement)
**Verdict**: ❌ Does not meet business requirement

### **Recommendation**

✅ **Option A** for Test 3 implementation (minimal, meets AC-027-4)
📋 **Option B** as future enhancement (when per-phase overrides needed)

---

## 🚧 **Test 4: Per-Phase Timeout Detection**

### **Current Blocker**

❌ **Configuration Decision Required**: How to configure phase timeouts?

### **Business Requirement**

**BR-ORCH-028**: Per-phase timeouts for faster detection

**Use Case**: Detect stuck AIAnalysis after 10 minutes (vs waiting 1 hour for global timeout)

### **Existing Infrastructure** ✅

**Good News**: Infrastructure already exists!

```go
// api/remediation/v1alpha1/remediationrequest_types.go (ALREADY EXISTS)
type RemediationRequestStatus struct {
    // Phase start time tracking (BR-ORCH-028)
    ProcessingStartTime *metav1.Time `json:"processingStartTime,omitempty"`
    AnalyzingStartTime  *metav1.Time `json:"analyzingStartTime,omitempty"`
    ExecutingStartTime  *metav1.Time `json:"executingStartTime,omitempty"`
}
```

```go
// pkg/remediationorchestrator/types.go (ALREADY EXISTS)
type PhaseTimeouts struct {
    Processing time.Duration // default: 5 minutes
    Analyzing  time.Duration // default: 10 minutes
    Executing  time.Duration // default: 30 minutes
    Global     time.Duration // default: 60 minutes
}

func DefaultPhaseTimeouts() PhaseTimeouts { /* ... */ }
```

### **Proposed Solution**

#### **Option A: Controller-Level Configuration (RECOMMENDED)**

Phase timeouts configured via command-line flags (similar to global timeout):

```go
// cmd/remediationorchestrator/main.go
func main() {
    var globalTimeout time.Duration
    var processingTimeout time.Duration
    var analyzingTimeout time.Duration
    var executingTimeout time.Duration

    flag.DurationVar(&globalTimeout, "global-timeout", 1*time.Hour, "...")
    flag.DurationVar(&processingTimeout, "processing-timeout", 5*time.Minute, "Phase timeout for SignalProcessing (BR-ORCH-028)")
    flag.DurationVar(&analyzingTimeout, "analyzing-timeout", 10*time.Minute, "Phase timeout for AIAnalysis (BR-ORCH-028)")
    flag.DurationVar(&executingTimeout, "executing-timeout", 30*time.Minute, "Phase timeout for WorkflowExecution (BR-ORCH-028)")

    reconciler := controller.NewReconciler(
        mgr.GetClient(),
        mgr.GetScheme(),
        auditStore,
        controller.TimeoutConfig{
            Global:     globalTimeout,
            Processing: processingTimeout,
            Analyzing:  analyzingTimeout,
            Executing:  executingTimeout,
        },
    )
}
```

**Controller Logic**:
```go
// pkg/remediationorchestrator/controller/reconciler.go
func (r *Reconciler) checkPhaseTimeouts(ctx context.Context, rr *RemediationRequest) error {
    // Check Processing phase timeout
    if rr.Status.OverallPhase == PhaseProcessing && rr.Status.ProcessingStartTime != nil {
        if time.Since(rr.Status.ProcessingStartTime.Time) > r.timeouts.Processing {
            return r.handlePhaseTimeout(ctx, rr, "Processing", r.timeouts.Processing)
        }
    }

    // Check Analyzing phase timeout
    if rr.Status.OverallPhase == PhaseAnalyzing && rr.Status.AnalyzingStartTime != nil {
        if time.Since(rr.Status.AnalyzingStartTime.Time) > r.timeouts.Analyzing {
            return r.handlePhaseTimeout(ctx, rr, "Analyzing", r.timeouts.Analyzing)
        }
    }

    // Check Executing phase timeout
    if rr.Status.OverallPhase == PhaseExecuting && rr.Status.ExecutingStartTime != nil {
        if time.Since(rr.Status.ExecutingStartTime.Time) > r.timeouts.Executing {
            return r.handlePhaseTimeout(ctx, rr, "Executing", r.timeouts.Executing)
        }
    }

    return nil
}
```

**Implementation Effort**: 2 hours
**Test Effort**: 1 hour
**Breaking Change**: No (new flags with defaults)

#### **Option B: Per-Remediation Phase Overrides (COMPREHENSIVE)**

Uses Option B from Test 3 (CRD `timeoutConfig` with per-phase fields):

```go
// Controller checks per-RR overrides first, falls back to controller-level defaults
func (r *Reconciler) getPhaseTimeout(rr *RemediationRequest, phase string) time.Duration {
    if rr.Status.TimeoutConfig != nil {
        switch phase {
        case "Processing":
            if rr.Status.TimeoutConfig.Processing != nil {
                return rr.Status.TimeoutConfig.Processing.Duration
            }
        case "Analyzing":
            if rr.Status.TimeoutConfig.Analyzing != nil {
                return rr.Status.TimeoutConfig.Analyzing.Duration
            }
        // ... etc
        }
    }

    // Fall back to controller-level defaults
    return r.timeouts[phase]
}
```

**Implementation Effort**: 3 hours (requires Option B from Test 3)
**Test Effort**: 1.5 hours
**Breaking Change**: No

#### **Option C: ConfigMap-Based (OPERATIONAL)**

Phase timeouts defined in ConfigMap, watched by controller:

**Pros**: Dynamic updates without restart
**Cons**: Adds operational complexity
**Verdict**: ⚠️ Over-engineered for current needs

### **Recommendation**

✅ **Option A** for Test 4 implementation (meets BR-ORCH-028, minimal complexity)
📋 **Option B** as future enhancement if per-RR phase overrides are needed

---

## 🎯 **Implementation Roadmap**

### **Phase 1: Unblock Test 3 (1 hour)**

1. ✅ Add `spec.globalTimeout` to CRD (Option A)
2. ✅ Update controller to check per-RR timeout
3. ✅ Activate and fix Test 3
4. ✅ Verify 4/4 active timeout tests passing

### **Phase 2: Unblock Test 4 (3 hours)**

1. ✅ Add phase timeout flags to main.go
2. ✅ Implement `checkPhaseTimeouts()` in reconciler
3. ✅ Implement `handlePhaseTimeout()` (similar to global timeout)
4. ✅ Activate and fix Test 4
5. ✅ Verify 5/5 active timeout tests passing

### **Phase 3: Documentation (30 minutes)**

1. ✅ Update RO_SERVICE_COMPLETE_HANDOFF.md
2. ✅ Update BR-ORCH-027/028 coverage to 100%
3. ✅ Document timeout configuration options

---

## 📊 **Impact Assessment**

### **Business Value**

| Outcome | Before | After |
|---------|--------|-------|
| **BR-ORCH-027/028 Coverage** | 75% (3/4 tests) | 100% (5/5 tests) |
| **Timeout Flexibility** | Global only | Global + per-RR |
| **Stuck Phase Detection** | 60 minutes | 5-30 minutes (per phase) |
| **MTTR Improvement** | Baseline | Up to 50% faster for phase issues |

### **Technical Debt**

✅ **Zero Technical Debt**: All options use existing Kubernetes patterns
✅ **Zero Breaking Changes**: All new fields/flags are optional
✅ **Low Maintenance**: Follows existing timeout pattern from global timeout

---

## ❓ **Decision Required**

### **Test 3 Schema Decision**

**Question**: Which CRD schema option for per-RR timeout override?

- [ ] **Option A**: Simple `spec.globalTimeout` field (RECOMMENDED, 1 hour effort)
- [ ] **Option B**: Comprehensive `status.timeoutConfig` struct (3 hours effort, future-proof)
- [ ] **Option C**: No per-RR override (FAILS AC-027-4, not recommended)

### **Test 4 Configuration Decision**

**Question**: How should phase timeouts be configured?

- [ ] **Option A**: Controller flags only (RECOMMENDED, 3 hours effort)
- [ ] **Option B**: Per-RR overrides via CRD (5 hours effort, requires Test 3 Option B)
- [ ] **Option C**: ConfigMap-based (over-engineered)

---

## 🚀 **Recommended Path Forward**

### **Minimal Implementation (4 hours total)**

✅ **Test 3**: Option A (simple `spec.globalTimeout`)
✅ **Test 4**: Option A (controller-level phase timeouts)

**Result**: Complete BR-ORCH-028, all acceptance criteria met

### **Future-Proof Implementation (8 hours total)**

✅ **Test 3**: Option B (comprehensive `status.timeoutConfig`)
✅ **Test 4**: Option B (per-RR phase timeout overrides)

**Result**: Complete BR-ORCH-028 + extensibility for future requirements

---

## 📋 **Next Steps**

**AWAITING USER DECISION**:

1. **Review this proposal**
2. **Choose Test 3 option** (A, B, or C)
3. **Choose Test 4 option** (A, B, or C)
4. **Approve implementation start**

Once decisions made, implementation can begin immediately following TDD RED → GREEN → REFACTOR methodology.

---

**Questions? Concerns? Alternative approaches?**

Please review and provide your decision on the schema and configuration approaches! 🚀


