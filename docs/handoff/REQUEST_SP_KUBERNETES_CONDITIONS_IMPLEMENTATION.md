# REQUEST: SignalProcessing - Kubernetes Conditions Implementation

**Date**: 2025-12-11
**Version**: 1.0
**From**: AIAnalysis Team
**To**: SignalProcessing Team
**Status**: ⏳ **PENDING RESPONSE**
**Priority**: MEDIUM

---

## 📋 Request Summary

**Request**: Implement Kubernetes Conditions for SignalProcessing CRD to improve operator experience and follow Kubernetes API conventions.

**Background**: AIAnalysis has implemented full Conditions support. Other CRD controllers should follow this pattern for consistency and better observability.

---

## 🟡 **Current Gap**

### SignalProcessing Status

| Aspect | Current | Required | Gap |
|--------|---------|----------|-----|
| **Conditions Field** | ❌ Not in CRD schema | ✅ `Conditions []metav1.Condition` | 🟡 Missing |
| **Conditions Infrastructure** | ❌ No `conditions.go` | ✅ Helper functions | 🟡 Missing |
| **Handler Integration** | ❌ No conditions set | ✅ Set in phase handlers | 🟡 Missing |
| **Test Coverage** | ❌ No condition tests | ✅ Unit + integration tests | 🟡 Missing |

---

## 🎯 **Recommended Conditions for SignalProcessing**

Based on your 4-phase flow (`Pending → Validating → Enriching → Classifying → Completed`):

### **Condition 1: ValidationComplete**

**Type**: `ValidationComplete`
**When**: After `Validating` phase
**Success Reason**: `ValidationSucceeded`
**Failure Reason**: `ValidationFailed`

**Example**:
```
Status: True
Reason: ValidationSucceeded
Message: Input validation passed for alert alert-123
```

---

### **Condition 2: EnrichmentComplete**

**Type**: `EnrichmentComplete`
**When**: After `Enriching` phase
**Success Reason**: `EnrichmentSucceeded`
**Failure Reason**: `EnrichmentFailed`, `K8sAPITimeout`, `ResourceNotFound`

**Example**:
```
Status: True
Reason: EnrichmentSucceeded
Message: Successfully enriched with Pod, Node, and Deployment context
```

---

### **Condition 3: ClassificationComplete**

**Type**: `ClassificationComplete`
**When**: After `Classifying` phase
**Success Reason**: `ClassificationSucceeded`
**Failure Reason**: `ClassificationFailed`, `RegoEvaluationError`

**Example**:
```
Status: True
Reason: ClassificationSucceeded
Message: Signal classified with priority P1, category Infrastructure
```

---

### **Condition 4: ProcessingComplete**

**Type**: `ProcessingComplete`
**When**: Transition to `Completed` phase
**Success Reason**: `ProcessingSucceeded`
**Failure Reason**: `ProcessingFailed`

**Example**:
```
Status: True
Reason: ProcessingSucceeded
Message: SignalProcessing completed successfully, ready for remediation orchestration
```

---

## 📚 **Reference Implementation: AIAnalysis**

### **Files to Review**

| File | Purpose | Lines |
|------|---------|-------|
| `pkg/aianalysis/conditions.go` | Infrastructure + helpers | 127 |
| `api/aianalysis/v1alpha1/aianalysis_types.go:450` | CRD schema field | 1 |
| `pkg/aianalysis/handlers/investigating.go:421` | Handler usage example | 1 |
| `pkg/aianalysis/handlers/analyzing.go` | Multiple condition examples | 6 usages |

**Full Documentation**: `docs/handoff/AIANALYSIS_CONDITIONS_IMPLEMENTATION_STATUS.md`

---

## 🛠️ **Implementation Steps for SignalProcessing**

### **Step 1: Create Infrastructure** (~1 hour)

**File**: `pkg/signalprocessing/conditions.go`

```go
package signalprocessing

import (
    "k8s.io/apimachinery/pkg/api/meta"
    metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

    spv1 "github.com/jordigilh/kubernaut/api/signalprocessing/v1alpha1"
)

// Condition types
const (
    ConditionValidationComplete    = "ValidationComplete"
    ConditionEnrichmentComplete    = "EnrichmentComplete"
    ConditionClassificationComplete = "ClassificationComplete"
    ConditionProcessingComplete    = "ProcessingComplete"
)

// Condition reasons
const (
    ReasonValidationSucceeded    = "ValidationSucceeded"
    ReasonValidationFailed       = "ValidationFailed"
    ReasonEnrichmentSucceeded    = "EnrichmentSucceeded"
    ReasonEnrichmentFailed       = "EnrichmentFailed"
    ReasonClassificationSucceeded = "ClassificationSucceeded"
    ReasonClassificationFailed   = "ClassificationFailed"
    ReasonProcessingSucceeded    = "ProcessingSucceeded"
    ReasonProcessingFailed       = "ProcessingFailed"
)

// SetCondition sets or updates a condition
func SetCondition(sp *spv1.SignalProcessing, conditionType string, status metav1.ConditionStatus, reason, message string) {
    condition := metav1.Condition{
        Type:               conditionType,
        Status:             status,
        LastTransitionTime: metav1.Now(),
        Reason:             reason,
        Message:            message,
    }
    meta.SetStatusCondition(&sp.Status.Conditions, condition)
}

// GetCondition returns the condition with the specified type
func GetCondition(sp *spv1.SignalProcessing, conditionType string) *metav1.Condition {
    return meta.FindStatusCondition(sp.Status.Conditions, conditionType)
}

// SetValidationComplete sets the ValidationComplete condition
func SetValidationComplete(sp *spv1.SignalProcessing, succeeded bool, message string) {
    status := metav1.ConditionTrue
    reason := ReasonValidationSucceeded
    if !succeeded {
        status = metav1.ConditionFalse
        reason = ReasonValidationFailed
    }
    SetCondition(sp, ConditionValidationComplete, status, reason, message)
}

// SetEnrichmentComplete sets the EnrichmentComplete condition
func SetEnrichmentComplete(sp *spv1.SignalProcessing, succeeded bool, message string) {
    status := metav1.ConditionTrue
    reason := ReasonEnrichmentSucceeded
    if !succeeded {
        status = metav1.ConditionFalse
        reason = ReasonEnrichmentFailed
    }
    SetCondition(sp, ConditionEnrichmentComplete, status, reason, message)
}

// SetClassificationComplete sets the ClassificationComplete condition
func SetClassificationComplete(sp *spv1.SignalProcessing, succeeded bool, message string) {
    status := metav1.ConditionTrue
    reason := ReasonClassificationSucceeded
    if !succeeded {
        status = metav1.ConditionFalse
        reason = ReasonClassificationFailed
    }
    SetCondition(sp, ConditionClassificationComplete, status, reason, message)
}

// SetProcessingComplete sets the ProcessingComplete condition
func SetProcessingComplete(sp *spv1.SignalProcessing, succeeded bool, message string) {
    status := metav1.ConditionTrue
    reason := ReasonProcessingSucceeded
    if !succeeded {
        status = metav1.ConditionFalse
        reason = ReasonProcessingFailed
    }
    SetCondition(sp, ConditionProcessingComplete, status, reason, message)
}
```

**Lines**: ~100 lines

---

### **Step 2: Update CRD Schema** (~15 minutes)

**File**: `api/signalprocessing/v1alpha1/signalprocessing_types.go`

```go
// SignalProcessingStatus defines the observed state of SignalProcessing
type SignalProcessingStatus struct {
    // ... existing fields ...

    // Conditions represent the latest available observations of the resource's state
    // +optional
    // +patchMergeKey=type
    // +patchStrategy=merge
    // +listType=map
    // +listMapKey=type
    Conditions []metav1.Condition `json:"conditions,omitempty" patchStrategy:"merge" patchMergeKey:"type"`
}
```

**Regenerate CRDs**:
```bash
make manifests
```

---

### **Step 3: Update Handlers** (~1-2 hours)

**Example**: `pkg/signalprocessing/handlers/validating.go`

```go
import (
    sp "github.com/jordigilh/kubernaut/pkg/signalprocessing"
)

func (h *ValidatingHandler) Handle(ctx context.Context, signal *spv1.SignalProcessing) (ctrl.Result, error) {
    // ... existing validation logic ...

    if validationErr != nil {
        sp.SetValidationComplete(signal, false, "Validation failed: "+validationErr.Error())
        return ctrl.Result{}, validationErr
    }

    // Set ValidationComplete condition
    sp.SetValidationComplete(signal, true, "Input validation passed")

    // Transition to Enriching
    signal.Status.Phase = sp.PhaseEnriching
    return ctrl.Result{}, nil
}
```

**Apply to all handlers**:
- `validating.go` → Set `ValidationComplete`
- `enriching.go` → Set `EnrichmentComplete`
- `classifying.go` → Set `ClassificationComplete`
- Final transition → Set `ProcessingComplete`

---

### **Step 4: Add Tests** (~1-2 hours)

**Create**: `test/unit/signalprocessing/conditions_test.go`

```go
var _ = Describe("SignalProcessing Conditions", func() {
    var signal *spv1.SignalProcessing

    BeforeEach(func() {
        signal = &spv1.SignalProcessing{
            Status: spv1.SignalProcessingStatus{},
        }
    })

    Context("SetValidationComplete", func() {
        It("should set condition to True on success", func() {
            sp.SetValidationComplete(signal, true, "Success")

            cond := sp.GetCondition(signal, sp.ConditionValidationComplete)
            Expect(cond).ToNot(BeNil())
            Expect(cond.Status).To(Equal(metav1.ConditionTrue))
            Expect(cond.Reason).To(Equal(sp.ReasonValidationSucceeded))
        })

        It("should set condition to False on failure", func() {
            sp.SetValidationComplete(signal, false, "Failure")

            cond := sp.GetCondition(signal, sp.ConditionValidationComplete)
            Expect(cond).ToNot(BeNil())
            Expect(cond.Status).To(Equal(metav1.ConditionFalse))
            Expect(cond.Reason).To(Equal(sp.ReasonValidationFailed))
        })
    })

    // Add similar tests for other conditions
})
```

**Add to integration tests**: Verify conditions are populated during reconciliation

---

### **Step 5: Update Documentation** (~30 minutes)

**Files to Update**:
1. `docs/services/crd-controllers/01-signalprocessing/crd-schema.md`
   - Document `conditions` field
   - List all 4 condition types and reasons

2. `docs/services/crd-controllers/01-signalprocessing/IMPLEMENTATION_PLAN_*.md`
   - Add "Conditions Implementation" section

3. `docs/services/crd-controllers/01-signalprocessing/testing-strategy.md`
   - Document condition test patterns

---

## 📊 **Effort Estimate for SignalProcessing**

| Task | Time | Difficulty |
|------|------|------------|
| Create `conditions.go` | 1 hour | Easy (copy from AIAnalysis) |
| Update CRD schema | 15 min | Easy |
| Update handlers (4 phases) | 1-2 hours | Medium |
| Add tests | 1-2 hours | Medium |
| Update documentation | 30 min | Easy |
| **Total** | **3-4 hours** | **Medium** |

---

## ✅ **Benefits for SignalProcessing**

### **Better Operator Experience**

**Before** (no conditions):
```bash
$ kubectl describe signalprocessing sp-123
Status:
  Phase: Enriching
  # No clear indication of what completed or why
```

**After** (with conditions):
```bash
$ kubectl describe signalprocessing sp-123
Status:
  Phase: Enriching
  Conditions:
    Type:     ValidationComplete
    Status:   True
    Reason:   ValidationSucceeded
    Message:  Input validation passed for alert alert-123

    Type:     EnrichmentComplete
    Status:   False
    Reason:   K8sAPITimeout
    Message:  Failed to fetch Pod details: timeout after 30s
```

### **Automation-Friendly**

```bash
# Wait for specific condition in scripts/CI
kubectl wait --for=condition=ProcessingComplete signalprocessing/sp-123 --timeout=60s

# Check condition status programmatically
kubectl get signalprocessing sp-123 -o jsonpath='{.status.conditions[?(@.type=="ValidationComplete")].status}'
```

---

## 📚 **Reference Materials**

### **AIAnalysis Implementation** (Your Reference)

1. **Main Infrastructure**: `pkg/aianalysis/conditions.go` (127 lines)
   - Copy this file and adapt for SignalProcessing
   - Replace `aianalysis` with `signalprocessing`
   - Update condition types and reasons

2. **Handler Integration**:
   - `pkg/aianalysis/handlers/investigating.go:421`
   - `pkg/aianalysis/handlers/analyzing.go:80,97,116,119,123,128`

3. **Documentation**: `docs/handoff/AIANALYSIS_CONDITIONS_IMPLEMENTATION_STATUS.md`

---

## 🗳️ **Response Requested**

Please respond to this request by updating the section below:

---

## 📝 **SignalProcessing Team Response**

**Date**: _[FILL IN]_
**Status**: ⏳ **PENDING**
**Responded By**: _[TEAM MEMBER NAME]_

### **Decision**

- [ ] ✅ **APPROVED** - Will implement Conditions
- [ ] ⏸️ **DEFERRED** - Will defer to V1.1/V2.0 (provide reason)
- [ ] ❌ **DECLINED** - Will not implement (provide reason)

### **Implementation Plan** (if approved)

**Target Version**: _[e.g., V1.1, V2.0]_
**Target Date**: _[YYYY-MM-DD]_
**Estimated Effort**: _[hours]_

**Conditions to Implement**:
- [ ] ValidationComplete
- [ ] EnrichmentComplete
- [ ] ClassificationComplete
- [ ] ProcessingComplete
- [ ] Other: _[specify if adding more]_

**Implementation Approach**:
_[Brief description of how you'll implement - copy from AIAnalysis, adapt, etc.]_

### **Questions or Concerns**

_[Any questions about the implementation or concerns about the approach]_

---

## 📊 **Effort Breakdown for SignalProcessing**

| Task | Estimated Time |
|------|----------------|
| Study AIAnalysis implementation | 30 min |
| Create `pkg/signalprocessing/conditions.go` | 1 hour |
| Update CRD schema (`api/signalprocessing/v1alpha1/`) | 15 min |
| Regenerate CRD manifests (`make manifests`) | 5 min |
| Update `ValidatingHandler` | 30 min |
| Update `EnrichingHandler` | 30 min |
| Update `ClassifyingHandler` | 30 min |
| Add completion logic in final phase | 30 min |
| Create `test/unit/signalprocessing/conditions_test.go` | 1 hour |
| Update integration tests | 30 min |
| Update documentation (crd-schema.md, etc.) | 30 min |
| **Total** | **~3-4 hours** |

---

## 📚 **Additional Resources**

- **Kubernetes API Conventions**: https://github.com/kubernetes/community/blob/master/contributors/devel/sig-architecture/api-conventions.md#typical-status-properties
- **AIAnalysis Code**: `pkg/aianalysis/conditions.go`
- **AIAnalysis Tests**: `test/integration/aianalysis/reconciliation_test.go`

---

**Next Steps**:
1. SignalProcessing team reviews this request
2. Fill in "SignalProcessing Team Response" section above
3. Commit response to this file
4. If approved, create implementation plan and execute

---

**Document Status**: ⏳ Awaiting SignalProcessing Team Response
**Created**: 2025-12-11
**From**: AIAnalysis Team
**File**: `docs/handoff/REQUEST_SP_KUBERNETES_CONDITIONS_IMPLEMENTATION.md`

