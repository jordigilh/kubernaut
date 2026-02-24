# TRIAGE: DD-CRD-002 Kubernetes Conditions Standard Implementation

**Date**: December 16, 2025
**Decision Document**: DD-CRD-002
**Priority**: 🚨 **MANDATORY FOR V1.0**
**Deadline**: January 3, 2026 (1 week buffer before V1.0 release)
**Status**: 📋 Triage Complete - Implementation Plan Ready

---

## 📋 Executive Summary

**Requirement**: All 7 CRD controllers must implement Kubernetes Conditions infrastructure by V1.0 release.

**Current Status**: 3 of 7 complete (43%)
- ✅ AIAnalysis - Complete
- ✅ WorkflowExecution - Complete
- ✅ Notification - Complete
- 🔴 SignalProcessing - Schema only, infrastructure needed
- 🔴 RemediationRequest - Schema only, infrastructure needed
- 🔴 RemediationApprovalRequest - Schema only, infrastructure needed
- 🔴 KubernetesExecution (DEPRECATED - ADR-025) - Schema only, infrastructure needed

**Total Effort**: 10-14 hours across 3 teams
**Business Value**: Critical for operator UX, automation, and debugging

---

## 🎯 Triage Assessment

### What Are Kubernetes Conditions?

Kubernetes Conditions are a **standard pattern** for surfacing detailed status information in CRD resources. They enable:

1. **Operator Experience**: `kubectl describe` shows granular phase status
2. **Automation**: `kubectl wait --for=condition=X` enables scripting
3. **Debugging**: Detailed failure reasons without log access
4. **Consistency**: All Kubernaut CRDs follow same patterns

### Why This Is Critical for V1.0

**Without Conditions**:
```bash
# Operator sees this
$ kubectl describe remediationrequest rr-001
...
Status:
  Phase: Failed
  # No details about WHY it failed!
```

**With Conditions**:
```bash
# Operator sees this
$ kubectl describe remediationrequest rr-001
...
Status:
  Phase: Failed
  Conditions:
    Type:              ValidationComplete
    Status:            False
    Reason:            ValidationFailed
    Message:           Workflow reference 'invalid-workflow' not found in namespace 'kubernaut-system'
    Last Transition:   2025-12-16T10:00:00Z
```

**Impact**: Operators can debug issues without accessing logs or calling support.

---

## 📊 Affected Teams & Services

### Team Impact Analysis

| Team | CRDs Affected | Total Effort | Files to Create | Tests to Write | Priority |
|------|--------------|--------------|-----------------|----------------|----------|
| **SignalProcessing** | 1 (SignalProcessing) | 3-4 hours | 2 files | ~50 specs | High |
| **RemediationOrchestrator** | 2 (RemediationRequest, ApprovalRequest) | 5-7 hours | 4 files | ~80 specs | High |
| **WorkflowExecution** | 1 (KubernetesExecution) | 2-3 hours | 2 files | ~40 specs | Medium |
| **Gateway** | 0 (Not a CRD controller) | 0 hours | 0 files | 0 specs | N/A |
| **Notification** | 0 (Already complete) | 0 hours | 0 files | 0 specs | N/A |
| **AIAnalysis** | 0 (Already complete) | 0 hours | 0 files | 0 specs | N/A |

**Total Project Effort**: 10-14 hours across 3 teams

---

## 🔍 Detailed Gap Analysis

### 1. SignalProcessing (SP Team)

**Current State**: ❌ Schema field exists, infrastructure missing

**Required Implementation**:

**Files to Create**:
1. `pkg/signalprocessing/conditions.go` (~150 lines)
2. `test/unit/signalprocessing/conditions_test.go` (~200 lines)

**Conditions Needed** (4 types):
- `ValidationComplete` - Input validation status
- `EnrichmentComplete` - K8s context enrichment status
- `ClassificationComplete` - Environment/priority classification status
- `ProcessingComplete` - Overall processing status

**Business Requirements Mapped**:
- BR-SP-001: Kubernetes Context Enrichment
- BR-SP-051-053: Environment Classification
- BR-SP-070-072: Priority Assignment
- BR-SP-090: Audit Trail

**Controller Integration Points**:
- `Reconcile()` function - 4 locations (after each phase)
- Status update calls - integrate condition setters

**Effort**: 3-4 hours
**Complexity**: Medium (4 phases to track)

---

### 2. RemediationRequest (RO Team)

**Current State**: ❌ Schema field exists, infrastructure missing

**Required Implementation**:

**Files to Create**:
1. `pkg/remediationorchestrator/conditions.go` (~180 lines)
2. `test/unit/remediationorchestrator/conditions_test.go` (~250 lines)

**Conditions Needed** (4 types):
- `RequestValidated` - Request validation status
- `ApprovalResolved` - Approval workflow status
- `ExecutionStarted` - Execution initiation status
- `ExecutionComplete` - Execution completion status

**Business Requirements Mapped**:
- BR-RO-001: Request Validation
- BR-RO-010: Approval Workflow
- BR-RO-020: Execution Coordination

**Controller Integration Points**:
- `ReconcileRemediationRequest()` - 5 locations (validation, approval, execution start/end, completion)
- Approval decision handling - 2 locations
- Execution status updates - 2 locations

**Effort**: 3-4 hours
**Complexity**: High (complex lifecycle with approval branching)

---

### 3. RemediationApprovalRequest (RO Team)

**Current State**: ❌ Schema field exists, infrastructure missing

**Required Implementation**:

**Files to Create**:
1. `pkg/remediationorchestrator/approval_conditions.go` (~120 lines)
2. `test/unit/remediationorchestrator/approval_conditions_test.go` (~150 lines)

**Conditions Needed** (3 types):
- `DecisionRecorded` - Approval decision status
- `NotificationSent` - Notification delivery status
- `TimeoutExpired` - Timeout expiration status

**Business Requirements Mapped**:
- BR-RO-011: Approval Decision
- BR-RO-012: Approval Timeout

**Controller Integration Points**:
- `ReconcileApprovalRequest()` - 3 locations (decision, notification, timeout)
- Approval decision handler - 1 location
- Timeout check - 1 location

**Effort**: 2-3 hours
**Complexity**: Low (simple lifecycle)

---

### 4. KubernetesExecution (WE Team)

**Current State**: ❌ Schema field exists, infrastructure missing

**Required Implementation**:

**Files to Create**:
1. `pkg/kubernetesexecution/conditions.go` (~130 lines)
2. `test/unit/kubernetesexecution/conditions_test.go` (~160 lines)

**Conditions Needed** (3 types):
- `JobCreated` - Kubernetes Job creation status
- `JobRunning` - Job execution status
- `JobComplete` - Job completion status

**Business Requirements Mapped**:
- BR-WE-010: Kubernetes Job Execution
- BR-WE-011: Job Status Tracking

**Controller Integration Points**:
- `ReconcileKubernetesExecution()` - 3 locations (creation, running, completion)
- Job status polling - 2 locations

**Effort**: 2-3 hours
**Complexity**: Low (straightforward Job lifecycle)

---

## 🚫 Services NOT Affected

### Gateway Service - No Action Required

**Why Not Affected**:
- Gateway is a **stateless HTTP service**, NOT a CRD controller
- Gateway does NOT manage any CRD lifecycle
- Gateway only **creates** RemediationRequest CRDs (fire-and-forget)
- Conditions are set by the **RemediationOrchestrator controller**, not Gateway

**Gateway's Role**:
```
Signal → Gateway HTTP Handler → Create RemediationRequest CRD → Done
                                 ↓
                        RemediationOrchestrator Controller
                        ↓ (Sets Conditions here)
                        Updates RemediationRequest.Status.Conditions
```

**Confirmation**: Gateway team has **zero implementation work** for DD-CRD-002.

---

## 📐 Standard Implementation Pattern

All teams MUST follow this pattern (see reference implementations):

### File Structure

```
pkg/{service}/
├── conditions.go          # Condition infrastructure (NEW)
│   ├── Condition type constants
│   ├── Condition reason constants
│   ├── SetCondition() helper
│   ├── GetCondition() helper
│   ├── IsConditionTrue() helper
│   └── Phase-specific helpers (e.g., SetValidationComplete())
│
test/unit/{service}/
├── conditions_test.go     # Unit tests for conditions (NEW)
│   ├── SetCondition tests
│   ├── GetCondition tests
│   ├── IsConditionTrue tests
│   └── Phase-specific helper tests
```

### Code Pattern (Copy-Paste from Reference)

```go
// pkg/{service}/conditions.go

package {service}

import (
    metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
    "k8s.io/apimachinery/pkg/api/meta"
    "github.com/jordigilh/kubernaut/api/{service}/v1alpha1"
)

// Condition types
const (
    ConditionValidationComplete  = "ValidationComplete"
    ConditionEnrichmentComplete  = "EnrichmentComplete"
    // ... more conditions ...
)

// Condition reasons (success)
const (
    ReasonValidationSucceeded  = "ValidationSucceeded"
    ReasonEnrichmentSucceeded  = "EnrichmentSucceeded"
    // ... more reasons ...
)

// Condition reasons (failure)
const (
    ReasonValidationFailed     = "ValidationFailed"
    ReasonK8sAPITimeout        = "K8sAPITimeout"
    ReasonResourceNotFound     = "ResourceNotFound"
    // ... more reasons ...
)

// SetCondition sets or updates a condition
func SetCondition(obj *v1alpha1.YourCRD, conditionType string, status metav1.ConditionStatus, reason, message string) {
    condition := metav1.Condition{
        Type:               conditionType,
        Status:             status,
        LastTransitionTime: metav1.Now(),
        Reason:             reason,
        Message:            message,
    }
    meta.SetStatusCondition(&obj.Status.Conditions, condition)
}

// GetCondition returns the condition with the specified type
func GetCondition(obj *v1alpha1.YourCRD, conditionType string) *metav1.Condition {
    return meta.FindStatusCondition(obj.Status.Conditions, conditionType)
}

// IsConditionTrue returns true if condition exists and is True
func IsConditionTrue(obj *v1alpha1.YourCRD, conditionType string) bool {
    condition := GetCondition(obj, conditionType)
    return condition != nil && condition.Status == metav1.ConditionTrue
}

// Phase-specific helper
func SetValidationComplete(obj *v1alpha1.YourCRD, success bool, message string) {
    status := metav1.ConditionTrue
    reason := ReasonValidationSucceeded
    if !success {
        status = metav1.ConditionFalse
        reason = ReasonValidationFailed
    }
    SetCondition(obj, ConditionValidationComplete, status, reason, message)
}
```

---

## 📋 Implementation Checklist per Team

### SignalProcessing Team

**Files to Create**:
- [ ] `pkg/signalprocessing/conditions.go` (~150 lines)
- [ ] `test/unit/signalprocessing/conditions_test.go` (~200 lines)

**Conditions to Implement**:
- [ ] `ValidationComplete` (BR-SP-001)
- [ ] `EnrichmentComplete` (BR-SP-001)
- [ ] `ClassificationComplete` (BR-SP-070)
- [ ] `ProcessingComplete` (BR-SP-090)

**Controller Integration**:
- [ ] Add condition setter after validation phase
- [ ] Add condition setter after enrichment phase
- [ ] Add condition setter after classification phase
- [ ] Add condition setter at processing completion

**Testing**:
- [ ] Unit tests for all condition helpers (~50 specs)
- [ ] Integration tests verify conditions are set during reconciliation (~20 specs)

**Reference Implementation**: `pkg/aianalysis/conditions.go` (similar 4-phase lifecycle)

---

### RemediationOrchestrator Team (RemediationRequest)

**Files to Create**:
- [ ] `pkg/remediationorchestrator/conditions.go` (~180 lines)
- [ ] `test/unit/remediationorchestrator/conditions_test.go` (~250 lines)

**Conditions to Implement**:
- [ ] `RequestValidated` (BR-RO-001)
- [ ] `ApprovalResolved` (BR-RO-010)
- [ ] `ExecutionStarted` (BR-RO-020)
- [ ] `ExecutionComplete` (BR-RO-020)

**Controller Integration**:
- [ ] Add condition setter after request validation
- [ ] Add condition setter when approval is granted/denied/expired
- [ ] Add condition setter when execution starts
- [ ] Add condition setter when execution completes/fails

**Testing**:
- [ ] Unit tests for all condition helpers (~60 specs)
- [ ] Integration tests verify conditions during lifecycle (~30 specs)

**Reference Implementation**: `pkg/workflowexecution/conditions.go` (complex lifecycle)

---

### RemediationOrchestrator Team (RemediationApprovalRequest)

**Files to Create**:
- [ ] `pkg/remediationorchestrator/approval_conditions.go` (~120 lines)
- [ ] `test/unit/remediationorchestrator/approval_conditions_test.go` (~150 lines)

**Conditions to Implement**:
- [ ] `DecisionRecorded` (BR-RO-011)
- [ ] `NotificationSent` (BR-RO-011)
- [ ] `TimeoutExpired` (BR-RO-012)

**Controller Integration**:
- [ ] Add condition setter when approval decision is recorded
- [ ] Add condition setter after notification attempt
- [ ] Add condition setter when timeout expires

**Testing**:
- [ ] Unit tests for all condition helpers (~40 specs)
- [ ] Integration tests verify conditions during approval flow (~20 specs)

**Reference Implementation**: `pkg/notification/conditions.go` (minimal pattern)

---

### WorkflowExecution Team (KubernetesExecution)

**Files to Create**:
- [ ] `pkg/kubernetesexecution/conditions.go` (~130 lines)
- [ ] `test/unit/kubernetesexecution/conditions_test.go` (~160 lines)

**Conditions to Implement**:
- [ ] `JobCreated` (BR-WE-010)
- [ ] `JobRunning` (BR-WE-010)
- [ ] `JobComplete` (BR-WE-011)

**Controller Integration**:
- [ ] Add condition setter after Job creation
- [ ] Add condition setter when Job starts running
- [ ] Add condition setter when Job completes/fails

**Testing**:
- [ ] Unit tests for all condition helpers (~40 specs)
- [ ] Integration tests verify conditions during Job lifecycle (~20 specs)

**Reference Implementation**: `pkg/notification/conditions.go` (simple 3-state pattern)

---

## 📅 Proposed Implementation Timeline

**Total Timeline**: 2 weeks (Dec 16 - Jan 3, 2026)
**Deadline**: January 3, 2026 (1 week buffer before V1.0 release on Jan 10)

### Week 1: Dec 16-20, 2025

| Team | Days | Deliverable |
|------|------|-------------|
| **SignalProcessing** | Dec 16-17 | `conditions.go` + unit tests |
| **RemediationOrchestrator** | Dec 16-18 | RemediationRequest `conditions.go` + unit tests |
| **WorkflowExecution** | Dec 16 | KubernetesExecution `conditions.go` + unit tests |

### Week 2: Dec 23-27, 2025 (Holiday Week)

| Team | Days | Deliverable |
|------|------|-------------|
| **SignalProcessing** | Dec 23-24 | Controller integration + integration tests |
| **RemediationOrchestrator** | Dec 23-26 | RemediationApprovalRequest `conditions.go` + unit tests + controller integration |
| **WorkflowExecution** | Dec 23 | Controller integration + integration tests |

### Buffer Week: Dec 30 - Jan 3, 2026

- Final testing and validation
- Cross-team integration verification
- Documentation updates

---

## 🧪 Testing Strategy

### Unit Tests (Per Team)

**Pattern**: Test all helper functions

```go
// test/unit/{service}/conditions_test.go

var _ = Describe("Conditions", func() {
    var obj *v1alpha1.YourCRD

    BeforeEach(func() {
        obj = &v1alpha1.YourCRD{
            Status: v1alpha1.YourCRDStatus{},
        }
    })

    Context("SetCondition", func() {
        It("should set condition to True on success", func() {
            SetValidationComplete(obj, true, "Success")

            cond := GetCondition(obj, ConditionValidationComplete)
            Expect(cond).ToNot(BeNil())
            Expect(cond.Status).To(Equal(metav1.ConditionTrue))
            Expect(cond.Reason).To(Equal(ReasonValidationSucceeded))
        })

        It("should set condition to False on failure", func() {
            SetValidationComplete(obj, false, "Failure")

            cond := GetCondition(obj, ConditionValidationComplete)
            Expect(cond).ToNot(BeNil())
            Expect(cond.Status).To(Equal(metav1.ConditionFalse))
            Expect(cond.Reason).To(Equal(ReasonValidationFailed))
        })
    })
})
```

**Coverage Target**: 100% of condition helper functions

---

### Integration Tests (Per Team)

**Pattern**: Verify conditions are set during reconciliation

```go
It("should set ValidationComplete condition after validation", func() {
    // Create test CRD
    obj := createTestCRD()
    Expect(k8sClient.Create(ctx, obj)).To(Succeed())

    // Wait for condition to be set
    Eventually(func() bool {
        var updated v1alpha1.YourCRD
        if err := k8sClient.Get(ctx, client.ObjectKeyFromObject(obj), &updated); err != nil {
            return false
        }
        return conditions.IsConditionTrue(&updated, conditions.ConditionValidationComplete)
    }, timeout, interval).Should(BeTrue())
})
```

**Coverage Target**: All phase transitions set appropriate conditions

---

## 📊 Success Metrics

| Metric | Target | Verification Method |
|--------|--------|---------------------|
| **CRD Coverage** | 7/7 CRDs | `find pkg/ -name conditions.go` shows 7 files |
| **Unit Test Coverage** | 100% functions | All Set*/Get*/Is* functions tested |
| **Integration Coverage** | All phases | Each phase transition sets condition |
| **Operator UX** | < 1 min debug time | `kubectl describe` shows detailed failure reasons |
| **Code Quality** | 0 linter errors | `make lint` passes |

---

## 🚀 Quick Start Guide for Teams

### Step 1: Copy Reference Implementation

Choose a reference based on your service's complexity:

- **Simple lifecycle** (3 conditions): Use `pkg/notification/conditions.go` as template
- **Medium lifecycle** (4 conditions): Use `pkg/aianalysis/conditions.go` as template
- **Complex lifecycle** (5+ conditions): Use `pkg/workflowexecution/conditions.go` as template

### Step 2: Customize Conditions

1. Replace condition type constants with your service's phases
2. Add success/failure reason constants
3. Update CRD type references (`v1alpha1.YourCRD`)
4. Create phase-specific helper functions

### Step 3: Write Unit Tests

1. Copy test structure from reference implementation
2. Test all helper functions (Set*, Get*, Is*)
3. Test phase-specific helpers
4. Verify condition message formatting

### Step 4: Integrate with Controller

1. Identify phase transition points in `Reconcile()` function
2. Add condition setter calls after each phase
3. Include detailed failure messages with context
4. Ensure status updates include condition changes

### Step 5: Add Integration Tests

1. Create test CRD instances
2. Verify conditions are set during reconciliation
3. Test both success and failure paths
4. Verify condition transitions (False → True, etc.)

---

## 🔗 Reference Implementations

### Copy These Files as Templates

| Your Service Complexity | Use This Template | Lines |
|-------------------------|------------------|-------|
| **Simple** (3 conditions) | `pkg/notification/conditions.go` | 123 |
| **Medium** (4 conditions) | `pkg/aianalysis/conditions.go` | 127 |
| **Complex** (5+ conditions) | `pkg/workflowexecution/conditions.go` | 270 |

**Unit Test Template**:
- `test/unit/notification/conditions_test.go` - Simple pattern
- `test/unit/aianalysis/conditions_test.go` - Medium pattern
- `test/unit/workflowexecution/conditions_test.go` - Complex pattern

---

## ⚠️ Critical Considerations

### 1. Business Requirement Mapping

**MANDATORY**: Every condition type MUST map to a business requirement (BR-XXX-XXX).

✅ **Good**:
```go
// ValidationComplete tracks BR-SP-001: Input Validation
const ConditionValidationComplete = "ValidationComplete"
```

❌ **Bad**:
```go
// Generic condition with no BR mapping
const ConditionComplete = "Complete"
```

### 2. Failure Reason Specificity

**MANDATORY**: Use specific failure reasons, not generic "Failed".

✅ **Good**:
```go
const (
    ReasonK8sAPITimeout     = "K8sAPITimeout"
    ReasonResourceNotFound  = "ResourceNotFound"
    ReasonRBACDenied        = "RBACDenied"
)
```

❌ **Bad**:
```go
const ReasonFailed = "Failed"  // Too generic!
```

### 3. Message Context

**MANDATORY**: Include actionable context in condition messages.

✅ **Good**:
```go
SetValidationComplete(obj, false,
    "Validation failed: workflow reference 'invalid-workflow' not found in namespace 'kubernaut-system' (BR-RO-001)")
```

❌ **Bad**:
```go
SetValidationComplete(obj, false, "Validation failed")
```

---

## 📞 Support & Questions

### Resources

1. **Reference Implementations**: See "Reference Implementations" section above
2. **Kubernetes Docs**: [API Conventions - Conditions](https://github.com/kubernetes/community/blob/master/contributors/devel/sig-architecture/api-conventions.md#typical-status-properties)
3. **Decision Document**: `docs/architecture/decisions/DD-CRD-002-kubernetes-conditions-standard.md`

### Get Help

- **Slack**: #conditions-implementation
- **Questions**: Comment on DD-CRD-002 document
- **Code Reviews**: Tag @platform-team

---

## ✅ Team Acknowledgment

**Please acknowledge your team's commitment to implement by Jan 3, 2026.**

### Acknowledgment Format

```markdown
- [x] Team Name - @lead - YYYY-MM-DD - "Reviewed. Commit to X hours implementation by deadline."
```

### Acknowledged By

- [ ] **SignalProcessing Team** - @sp-lead - _Pending_ - "Reviewed. Commit to 3-4 hours implementation."
- [ ] **RemediationOrchestrator Team** - @ro-lead - _Pending_ - "Reviewed. Commit to 5-7 hours implementation (2 CRDs)."
- [ ] **WorkflowExecution Team** - @we-lead - _Pending_ - "Reviewed. Commit to 2-3 hours implementation."
- [x] **Gateway Team** - @jgil - 2025-12-16 - "Reviewed. Confirmed Gateway is NOT affected (not a CRD controller). No action required. ✅"

---

## 📋 Summary

### What This Means

- **Requirement**: Implement Kubernetes Conditions for 4 CRDs by Jan 3, 2026
- **Affected Teams**: SignalProcessing (1 CRD), RemediationOrchestrator (2 CRDs), WorkflowExecution (1 CRD)
- **NOT Affected**: Gateway (stateless HTTP service), Notification/AIAnalysis (already complete)
- **Total Effort**: 10-14 hours across 3 teams
- **Business Value**: Critical for operator UX, automation, and V1.0 production readiness

### Next Steps

1. ✅ **Acknowledge**: Each team confirms commitment in section above
2. 📅 **Schedule**: Teams allocate 2-7 hours in Dec 16 - Jan 3 timeline
3. 📚 **Reference**: Copy from existing implementations (AIAnalysis, WorkflowExecution, Notification)
4. 🧪 **Validate**: Ensure unit + integration tests pass
5. ✅ **Complete**: Verify with `kubectl describe` showing populated Conditions

---

**Triage Status**: ✅ Complete - Implementation plan ready
**Action Required**: Team acknowledgment + scheduled implementation
**Deadline**: January 3, 2026 (18 days from now)

---

**Document Version**: 1.0
**Created**: December 16, 2025
**Author**: Platform Team / AI Assistant
**File**: `docs/handoff/TRIAGE_DD_CRD_002_CONDITIONS_IMPLEMENTATION.md`


