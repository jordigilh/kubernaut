# REQUEST: RemediationOrchestrator - Kubernetes Conditions Implementation

**Date**: 2025-12-11
**Version**: 1.0
**From**: AIAnalysis Team
**To**: RemediationOrchestrator Team
**Status**: ⏳ **PENDING RESPONSE**
**Priority**: 🔥 **HIGH** (Orchestration Visibility)

---

## 📋 Request Summary

**Request**: Implement Kubernetes Conditions for RemediationOrchestrator CRD to provide visibility into child CRD orchestration state.

**Background**: AIAnalysis has implemented full Conditions support. RO, as the **orchestration controller**, has the HIGHEST priority for Conditions because it needs to surface the state of child CRDs (AIAnalysis, WorkflowExecution).

**Why HIGH Priority**: RemediationOrchestrator coordinates multiple child CRDs. Conditions are essential for operators to understand orchestration state without inspecting child resources individually.

---

## 🔥 **Current Gap - HIGH PRIORITY**

### RemediationOrchestrator Status

| Aspect | Current | Required | Gap |
|--------|---------|----------|-----|
| **Conditions Field** | ❌ Not in CRD schema | ✅ `Conditions []metav1.Condition` | 🔴 **Missing** |
| **Conditions Infrastructure** | ❌ No `conditions.go` | ✅ Helper functions | 🔴 **Missing** |
| **Handler Integration** | ❌ No conditions set | ✅ Set in phase handlers | 🔴 **Missing** |
| **Test Coverage** | ❌ No condition tests | ✅ Unit + integration tests | 🔴 **Missing** |

---

## 🎯 **Recommended Conditions for RemediationOrchestrator**

Based on your orchestration flow and child CRD lifecycle:

### **Condition 1: AIAnalysisReady**

**Type**: `AIAnalysisReady`
**When**: After AIAnalysis CRD created
**Success Reason**: `AIAnalysisCreated`
**Failure Reason**: `AIAnalysisCreationFailed`

**Example**:
```
Status: True
Reason: AIAnalysisCreated
Message: AIAnalysis CRD aianalysis-123 created successfully
```

**Why Important**: Operators can see when AI analysis is initiated without querying child CRD

---

### **Condition 2: AIAnalysisComplete**

**Type**: `AIAnalysisComplete`
**When**: After AIAnalysis transitions to `Completed` phase
**Success Reason**: `AIAnalysisSucceeded`
**Failure Reason**: `AIAnalysisFailed`, `AIAnalysisTimeout`

**Example**:
```
Status: True
Reason: AIAnalysisSucceeded
Message: AIAnalysis completed with workflow wf-restart-pod (confidence 0.85)
```

**Why Important**: Shows AI investigation result without checking child CRD

---

### **Condition 3: WorkflowExecutionReady**

**Type**: `WorkflowExecutionReady`
**When**: After WorkflowExecution CRD created
**Success Reason**: `WorkflowExecutionCreated`
**Failure Reason**: `WorkflowExecutionCreationFailed`, `ApprovalPending`

**Example**:
```
Status: True
Reason: WorkflowExecutionCreated
Message: WorkflowExecution CRD we-123 created for workflow wf-restart-pod
```

**Why Important**: Tracks workflow execution initiation

---

### **Condition 4: WorkflowExecutionComplete**

**Type**: `WorkflowExecutionComplete`
**When**: After WorkflowExecution finishes
**Success Reason**: `WorkflowSucceeded`
**Failure Reason**: `WorkflowFailed`, `WorkflowTimeout`

**Example**:
```
Status: True
Reason: WorkflowSucceeded
Message: Workflow wf-restart-pod completed successfully (exit code: 0)
```

**Why Important**: Shows execution result without checking child CRD

---

### **Condition 5: RecoveryComplete** [Deprecated - Issue #180]

**Type**: `RecoveryComplete` [Deprecated - Issue #180]
**When**: Overall remediation finished (success or failure)
**Success Reason**: `RecoverySucceeded`
**Failure Reason**: `RecoveryFailed`, `MaxAttemptsReached`

**Example**:
```
Status: True
Reason: RecoverySucceeded
Message: Remediation completed successfully after 1 attempt
```

**Why Important**: Overall remediation status at a glance

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

## 🛠️ **Implementation Steps for RemediationOrchestrator**

### **Step 1: Create Infrastructure** (~1.5 hours)

**File**: `pkg/remediationorchestrator/conditions.go`

```go
package remediationorchestrator

import (
    "k8s.io/apimachinery/pkg/api/meta"
    metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

    rov1 "github.com/jordigilh/kubernaut/api/remediationorchestrator/v1alpha1"
)

// Condition types for RemediationOrchestrator
const (
    ConditionAIAnalysisReady          = "AIAnalysisReady"
    ConditionAIAnalysisComplete       = "AIAnalysisComplete"
    ConditionWorkflowExecutionReady   = "WorkflowExecutionReady"
    ConditionWorkflowExecutionComplete = "WorkflowExecutionComplete"
    ConditionRecoveryComplete         = "RecoveryComplete" // [Deprecated - Issue #180]
)

// Condition reasons
const (
    // AIAnalysis conditions
    ReasonAIAnalysisCreated         = "AIAnalysisCreated"
    ReasonAIAnalysisCreationFailed  = "AIAnalysisCreationFailed"
    ReasonAIAnalysisSucceeded       = "AIAnalysisSucceeded"
    ReasonAIAnalysisFailed          = "AIAnalysisFailed"
    ReasonAIAnalysisTimeout         = "AIAnalysisTimeout"

    // WorkflowExecution conditions
    ReasonWorkflowExecutionCreated        = "WorkflowExecutionCreated"
    ReasonWorkflowExecutionCreationFailed = "WorkflowExecutionCreationFailed"
    ReasonWorkflowSucceeded               = "WorkflowSucceeded"
    ReasonWorkflowFailed                  = "WorkflowFailed"
    ReasonWorkflowTimeout                 = "WorkflowTimeout"
    ReasonApprovalPending                 = "ApprovalPending"

    // Recovery conditions
    ReasonRecoverySucceeded      = "RecoverySucceeded"
    ReasonRecoveryFailed         = "RecoveryFailed"
    ReasonMaxAttemptsReached     = "MaxAttemptsReached"
)

// SetCondition sets or updates a condition
func SetCondition(ro *rov1.RemediationOrchestrator, conditionType string, status metav1.ConditionStatus, reason, message string) {
    condition := metav1.Condition{
        Type:               conditionType,
        Status:             status,
        LastTransitionTime: metav1.Now(),
        Reason:             reason,
        Message:            message,
    }
    meta.SetStatusCondition(&ro.Status.Conditions, condition)
}

// GetCondition returns the condition with the specified type
func GetCondition(ro *rov1.RemediationOrchestrator, conditionType string) *metav1.Condition {
    return meta.FindStatusCondition(ro.Status.Conditions, conditionType)
}

// SetAIAnalysisReady sets the AIAnalysisReady condition
func SetAIAnalysisReady(ro *rov1.RemediationOrchestrator, ready bool, reason, message string) {
    status := metav1.ConditionTrue
    if !ready {
        status = metav1.ConditionFalse
    }
    SetCondition(ro, ConditionAIAnalysisReady, status, reason, message)
}

// SetAIAnalysisComplete sets the AIAnalysisComplete condition
func SetAIAnalysisComplete(ro *rov1.RemediationOrchestrator, succeeded bool, message string) {
    status := metav1.ConditionTrue
    reason := ReasonAIAnalysisSucceeded
    if !succeeded {
        status = metav1.ConditionFalse
        reason = ReasonAIAnalysisFailed
    }
    SetCondition(ro, ConditionAIAnalysisComplete, status, reason, message)
}

// SetWorkflowExecutionReady sets the WorkflowExecutionReady condition
func SetWorkflowExecutionReady(ro *rov1.RemediationOrchestrator, ready bool, reason, message string) {
    status := metav1.ConditionTrue
    if !ready {
        status = metav1.ConditionFalse
    }
    SetCondition(ro, ConditionWorkflowExecutionReady, status, reason, message)
}

// SetWorkflowExecutionComplete sets the WorkflowExecutionComplete condition
func SetWorkflowExecutionComplete(ro *rov1.RemediationOrchestrator, succeeded bool, message string) {
    status := metav1.ConditionTrue
    reason := ReasonWorkflowSucceeded
    if !succeeded {
        status = metav1.ConditionFalse
        reason = ReasonWorkflowFailed
    }
    SetCondition(ro, ConditionWorkflowExecutionComplete, status, reason, message)
}

// SetRecoveryComplete sets the RecoveryComplete condition
// [Deprecated - Issue #180: RecoveryComplete removed]
func SetRecoveryComplete(ro *rov1.RemediationOrchestrator, succeeded bool, reason, message string) {
    status := metav1.ConditionTrue
    if !succeeded {
        status = metav1.ConditionFalse
    }
    SetCondition(ro, ConditionRecoveryComplete, status, reason, message)
}
```

**Lines**: ~130 lines

---

### **Step 2: Update CRD Schema** (~15 minutes)

**File**: `api/remediationorchestrator/v1alpha1/remediationorchestrator_types.go`

```go
// RemediationOrchestratorStatus defines the observed state of RemediationOrchestrator
type RemediationOrchestratorStatus struct {
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

---

### **Step 3: Update Controller Logic** (~2-3 hours)

**Key Integration Points**:

**After creating AIAnalysis CRD**:
```go
// pkg/remediationorchestrator/handlers/analyzing.go or similar
aiAnalysisCRD := &aianalysisv1.AIAnalysis{...}
err := r.Client.Create(ctx, aiAnalysisCRD)
if err != nil {
    ro.SetAIAnalysisReady(remediation, false, ro.ReasonAIAnalysisCreationFailed, err.Error())
    return ctrl.Result{}, err
}

ro.SetAIAnalysisReady(remediation, true, ro.ReasonAIAnalysisCreated,
    fmt.Sprintf("AIAnalysis CRD %s created successfully", aiAnalysisCRD.Name))
```

**When AIAnalysis completes**:
```go
// Watch AIAnalysis status
if aiAnalysisCRD.Status.Phase == aianalysis.PhaseCompleted {
    ro.SetAIAnalysisComplete(remediation, true,
        fmt.Sprintf("AIAnalysis completed with workflow %s", aiAnalysisCRD.Status.SelectedWorkflow.WorkflowID))
}
```

**Similar patterns for WorkflowExecution and overall recovery**

---

### **Step 4: Add Tests** (~1-2 hours)

**Create**: `test/unit/remediationorchestrator/conditions_test.go`

**Add to integration tests**: Verify conditions track child CRD state

---

### **Step 5: Update Documentation** (~30 minutes)

**Files to Update**:
1. `docs/services/crd-controllers/05-remediationorchestrator/crd-schema.md`
2. `docs/services/crd-controllers/05-remediationorchestrator/IMPLEMENTATION_PLAN_*.md`
3. `docs/services/crd-controllers/05-remediationorchestrator/testing-strategy.md`

---

## 📊 **Effort Estimate for RemediationOrchestrator**

| Task | Time | Difficulty |
|------|------|------------|
| Create `conditions.go` | 1.5 hours | Medium (5 conditions + reasons) |
| Update CRD schema | 15 min | Easy |
| Update controller logic | 2-3 hours | Medium (multiple watch points) |
| Add tests | 1-2 hours | Medium |
| Update documentation | 30 min | Easy |
| **Total** | **4-6 hours** | **Medium-High** |

**Why More Effort**: RO is orchestration controller with more complex state tracking

---

## ✅ **Benefits for RemediationOrchestrator**

### **Orchestration Visibility**

**Before** (no conditions):
```bash
$ kubectl describe remediationorchestrator ro-123
Status:
  Phase: WaitingForAnalysis
  # No visibility into AIAnalysis state without querying it separately
```

**After** (with conditions):
```bash
$ kubectl describe remediationorchestrator ro-123
Status:
  Phase: WaitingForAnalysis
  Conditions:
    Type:     AIAnalysisReady
    Status:   True
    Reason:   AIAnalysisCreated
    Message:  AIAnalysis CRD aianalysis-123 created successfully

    Type:     AIAnalysisComplete
    Status:   False
    Reason:   InProgress
    Message:  Waiting for AIAnalysis investigation to complete
```

### **Single Resource View**

Operators can see entire remediation state from RO without:
- ❌ `kubectl get aianalysis aianalysis-123`
- ❌ `kubectl get workflowexecution we-123`
- ✅ Just `kubectl describe ro ro-123` shows everything!

---

## 📚 **Reference Materials**

### **AIAnalysis Implementation** (Your Reference)

1. **Main Infrastructure**: `pkg/aianalysis/conditions.go` (127 lines)
2. **Handler Integration**: `pkg/aianalysis/handlers/analyzing.go`
3. **Documentation**: `docs/handoff/AIANALYSIS_CONDITIONS_IMPLEMENTATION_STATUS.md`

---

## 🗳️ **Response Requested**

Please respond to this request by updating the section below:

---

## 📝 **RemediationOrchestrator Team Response**

**Date**: 2025-12-11
**Status**: ✅ **APPROVED**
**Responded By**: RemediationOrchestrator Team

### **Decision**

- [x] ✅ **APPROVED** - Will implement Conditions
- [ ] ⏸️ **DEFERRED** - Will defer to V1.1/V2.0 (provide reason)
- [ ] ❌ **DECLINED** - Will not implement (provide reason)

### **Implementation Plan** (if approved)

**Target Version**: V1.2 (after BR-ORCH-042)
**Target Date**: 2025-12-13
**Estimated Effort**: 5-6 hours

**Conditions to Implement**:
- [x] SignalProcessingReady (RO-specific addition)
- [x] SignalProcessingComplete (RO-specific addition)
- [x] AIAnalysisReady
- [x] AIAnalysisComplete
- [x] WorkflowExecutionReady
- [x] WorkflowExecutionComplete
- [x] RecoveryComplete [Deprecated - Issue #180]

**Implementation Approach**:
Follow AIAnalysis pattern with RO-specific adaptations:
- Create `pkg/remediationorchestrator/conditions.go` (~150 lines, 7 conditions)
- Add `Conditions []metav1.Condition` to CRD schema
- Integrate at child CRD creation/completion points
- Add 35+ unit tests + 5-7 integration tests
- Update documentation (4 files)

**See Full Details**: `docs/handoff/RESPONSE_RO_CONDITIONS_IMPLEMENTATION.md`

### **Questions or Concerns**

**Clarification**: Should we add conditions for RemediationApprovalRequest in V1.2?
**Recommendation**: Start with 7 conditions, add approval conditions in V1.3 if needed.

---

## 📊 **Why RO Has HIGHEST Priority**

| Reason | Impact |
|--------|--------|
| **Orchestration Controller** | Shows state of 2+ child CRDs in one place |
| **Operator Experience** | Single `kubectl describe` shows full remediation state |
| **Debugging** | Faster troubleshooting without multiple queries |
| **Automation** | Scripts can wait for `RecoveryComplete` condition [Deprecated] |
| **Production Readiness** | Essential for production operations |

**Recommendation**: Prioritize RO for V1.1 implementation

---

## 📚 **Additional Resources**

- **Kubernetes API Conventions**: https://github.com/kubernetes/community/blob/master/contributors/devel/sig-architecture/api-conventions.md
- **AIAnalysis Code**: `pkg/aianalysis/conditions.go`
- **Best Practices**: Orchestration controllers benefit most from Conditions

---

**Next Steps**:
1. RemediationOrchestrator team reviews this request
2. Fill in "RemediationOrchestrator Team Response" section above
3. Commit response to this file
4. If approved, create implementation plan and execute

---

**Document Status**: ✅ **RESPONDED** - RO Team Approved Implementation
**Response File**: `docs/handoff/RESPONSE_RO_CONDITIONS_IMPLEMENTATION.md`
**Created**: 2025-12-11
**Responded**: 2025-12-11 (same day)
**From**: AIAnalysis Team
**Priority**: 🔥 **HIGH** (Orchestration visibility is critical)
**File**: `docs/handoff/REQUEST_RO_KUBERNETES_CONDITIONS_IMPLEMENTATION.md`

