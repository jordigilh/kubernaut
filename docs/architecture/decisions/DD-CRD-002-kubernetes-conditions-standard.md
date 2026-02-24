# DD-CRD-002: Kubernetes Conditions Standard for All CRD Controllers

**Status**: ✅ **APPROVED** (2025-12-16)
**Priority**: 🚨 **MANDATORY FOR V1.0**
**Last Reviewed**: 2026-02-18
**Confidence**: 95%
**Owner**: Platform Team
**Applies To**: All CRD Controller Services

---

## 📋 Context & Problem

### Problem Statement

Kubernetes Conditions are a standard pattern for surfacing detailed status information in CRD resources. Currently, **4 of 6** CRD controllers have implemented Conditions infrastructure:

| CRD | Status | Gap |
|-----|--------|-----|
| AIAnalysis | ✅ Complete (2025-12-14) | - |
| WorkflowExecution | ✅ Complete (2025-12-14) | - |
| Notification | ✅ Complete (2025-12-14) | - |
| SignalProcessing | ✅ Complete (2025-12-16) | - |
| RemediationRequest | 🔴 Schema only | Infrastructure + Tests |
| RemediationApprovalRequest | 🔴 Schema only | Infrastructure + Tests |

### Why This Matters

1. **Operator Experience**: `kubectl describe` should show detailed phase status
2. **Automation**: `kubectl wait --for=condition=X` enables scripting
3. **Consistency**: All Kubernaut CRDs should follow the same patterns
4. **Debugging**: Conditions provide granular failure reasons without log access

---

## 🎯 Decision

**ALL CRD controllers MUST implement Kubernetes Conditions infrastructure by V1.0 release.**

**Excluded**: KubernetesExecution is deprecated (ADR-025) and excluded from conditions implementation.

### Requirements

1. **Ready Condition** (MANDATORY for all active CRDs):
   - Every CRD MUST implement a `Ready` condition as an aggregate status
   - **Aggregation semantics**: `Ready=True` on success terminal states; `Ready=False` on failure terminal states
   - Enables `kubectl wait --for=condition=Ready` and Reason printer column (DD-CRD-003)

2. **ObservedGeneration** (MANDATORY):
   - All condition setters MUST set `ObservedGeneration` on every condition update
   - Prevents stale condition display when spec changes but reconcile has not yet processed

3. **Schema Field** (ALREADY EXISTS for all 7 CRDs):
   ```go
   // Conditions for detailed status
   Conditions []metav1.Condition `json:"conditions,omitempty"`
   ```

4. **Infrastructure File** (`pkg/{service}/conditions.go`):
   - Condition type constants (including `Ready`)
   - Condition reason constants
   - `SetCondition()` helper function (must set `ObservedGeneration`)
   - `SetReady()` helper function
   - `GetCondition()` helper function
   - Phase-specific helper functions (e.g., `SetValidationComplete()`)

5. **Controller Integration**:
   - Set conditions during phase transitions
   - Include failure reasons in condition messages
   - Update conditions on status updates

6. **Test Coverage**:
   - Unit tests for condition helper functions
   - Integration tests verifying conditions are populated

---

## 📐 Standard Pattern

### Condition Naming Convention

| Element | Pattern | Example |
|---------|---------|---------|
| **Type** | `{Phase}Complete` or `{Feature}` | `ValidationComplete`, `AuditRecorded` |
| **Reason (Success)** | `{Phase}Succeeded` | `ValidationSucceeded` |
| **Reason (Failure)** | `{FailureType}` | `ValidationFailed`, `K8sAPITimeout`, `RBACDenied` |
| **Message** | Human-readable with context | `"Input validation passed for alert alert-123"` |

### Standard Helper Functions

Every `conditions.go` MUST include:

```go
// SetCondition sets or updates a condition on the CRD status.
// MUST set ObservedGeneration on every update (DD-CRD-002 requirement).
func SetCondition(obj *v1alpha1.YourCRD, conditionType string, status metav1.ConditionStatus, reason, message string) {
    condition := metav1.Condition{
        Type:               conditionType,
        Status:             status,
        LastTransitionTime: metav1.Now(),
        Reason:             reason,
        Message:            message,
        ObservedGeneration: obj.Generation, // REQUIRED: prevents stale condition display
    }
    meta.SetStatusCondition(&obj.Status.Conditions, condition)
}

// GetCondition returns the condition with the specified type, or nil if not found
func GetCondition(obj *v1alpha1.YourCRD, conditionType string) *metav1.Condition {
    return meta.FindStatusCondition(obj.Status.Conditions, conditionType)
}

// IsConditionTrue returns true if the condition exists and has status True
func IsConditionTrue(obj *v1alpha1.YourCRD, conditionType string) bool {
    condition := GetCondition(obj, conditionType)
    return condition != nil && condition.Status == metav1.ConditionTrue
}
```

---

## 📋 Service-Specific Requirements

### SignalProcessing (SP Team)

**Status**: ✅ **COMPLETE** (2025-12-16)

**Business Requirements Mapping**:
- BR-SP-001: Kubernetes Context Enrichment
- BR-SP-051-053: Environment Classification
- BR-SP-070-072: Priority Assignment
- BR-SP-090: Audit Trail

**Required Conditions**:

| Condition Type | Phase | Success Reason | Failure Reasons | BR Reference |
|---------------|-------|----------------|-----------------|--------------|
| `ValidationComplete` | Validating | `ValidationSucceeded` | `ValidationFailed`, `InvalidSignalFormat` | BR-SP-001 |
| `EnrichmentComplete` | Enriching | `EnrichmentSucceeded` | `EnrichmentFailed`, `K8sAPITimeout`, `ResourceNotFound` | BR-SP-001 |
| `ClassificationComplete` | Classifying | `ClassificationSucceeded` | `ClassificationFailed`, `RegoEvaluationError`, `PolicyNotFound` | BR-SP-070 |
| `ProcessingComplete` | Completed | `ProcessingSucceeded` | `ProcessingFailed` | BR-SP-090 |

**Files**:
- Infrastructure: `pkg/signalprocessing/conditions.go` (157 lines)
- Tests: `test/unit/signalprocessing/conditions_test.go` (26 specs, 100% coverage)

**Effort**: 3-4 hours ✅ **COMPLETE**

---

### RemediationRequest (RO Team)

**Business Requirements Mapping**:
- BR-RO-001: Request Validation
- BR-RO-010: Approval Workflow
- BR-RO-020: Execution Coordination

**Required Conditions**:

| Condition Type | Phase | Success Reason | Failure Reasons | BR Reference |
|---------------|-------|----------------|-----------------|--------------|
| `RequestValidated` | Validating | `ValidationSucceeded` | `ValidationFailed`, `InvalidWorkflowRef` | BR-RO-001 |
| `ApprovalResolved` | PendingApproval/Approved | `ApprovalGranted`, `AutoApproved` | `ApprovalDenied`, `ApprovalExpired` | BR-RO-010 |
| `ExecutionStarted` | Executing | `ExecutionStarted` | `ExecutionFailed`, `WorkflowNotFound` | BR-RO-020 |
| `ExecutionComplete` | Completed | `ExecutionSucceeded` | `ExecutionFailed`, `PartialSuccess` | BR-RO-020 |

**File**: `pkg/remediationorchestrator/conditions.go`
**Effort**: 3-4 hours

---

### RemediationApprovalRequest (RO Team)

**Business Requirements Mapping**:
- BR-RO-011: Approval Decision
- BR-RO-012: Approval Timeout

**Required Conditions**:

| Condition Type | Phase | Success Reason | Failure Reasons | BR Reference |
|---------------|-------|----------------|-----------------|--------------|
| `DecisionRecorded` | Approved/Rejected | `Approved`, `Rejected` | `DecisionFailed` | BR-RO-011 |
| `NotificationSent` | Any | `NotificationSucceeded` | `NotificationFailed` | BR-RO-011 |
| `TimeoutExpired` | Expired | `TimeoutExpired` | - | BR-RO-012 |

**File**: `pkg/remediationorchestrator/approval_conditions.go`
**Effort**: 2-3 hours

---

## ✅ Reference Implementations

Teams SHOULD reference these existing implementations:

### AIAnalysis (Most Comprehensive)
- **File**: `pkg/aianalysis/conditions.go` (127 lines)
- **Conditions**: 4 types, 9 reasons
- **Pattern**: Phase-aligned with investigation/analysis lifecycle

### WorkflowExecution (Most Detailed Reasons)
- **File**: `pkg/workflowexecution/conditions.go` (270 lines)
- **Conditions**: 5 types, 15 reasons
- **Pattern**: Tekton pipeline state mapping

### Notification (Minimal Pattern)
- **File**: `pkg/notification/conditions.go` (123 lines)
- **Conditions**: 1 type, 3 reasons
- **Pattern**: Single condition for routing visibility

---

## 🧪 Testing Requirements

### Unit Tests (REQUIRED)

**File**: `test/unit/{service}/conditions_test.go`

```go
var _ = Describe("Conditions", func() {
    var obj *v1alpha1.YourCRD

    BeforeEach(func() {
        obj = &v1alpha1.YourCRD{
            Status: v1alpha1.YourCRDStatus{},
        }
    })

    Context("SetCondition", func() {
        It("should set condition to True on success", func() {
            SetPhaseComplete(obj, true, "Success message")

            cond := GetCondition(obj, ConditionPhaseComplete)
            Expect(cond).ToNot(BeNil())
            Expect(cond.Status).To(Equal(metav1.ConditionTrue))
            Expect(cond.Reason).To(Equal(ReasonPhaseSucceeded))
        })

        It("should set condition to False on failure", func() {
            SetPhaseComplete(obj, false, "Failure message")

            cond := GetCondition(obj, ConditionPhaseComplete)
            Expect(cond).ToNot(BeNil())
            Expect(cond.Status).To(Equal(metav1.ConditionFalse))
        })
    })

    Context("IsConditionTrue", func() {
        It("should return true when condition is True", func() {
            SetCondition(obj, ConditionPhaseComplete, metav1.ConditionTrue, "Reason", "Message")
            Expect(IsConditionTrue(obj, ConditionPhaseComplete)).To(BeTrue())
        })

        It("should return false when condition is False", func() {
            SetCondition(obj, ConditionPhaseComplete, metav1.ConditionFalse, "Reason", "Message")
            Expect(IsConditionTrue(obj, ConditionPhaseComplete)).To(BeFalse())
        })

        It("should return false when condition does not exist", func() {
            Expect(IsConditionTrue(obj, "NonExistent")).To(BeFalse())
        })
    })
})
```

### Integration Tests (REQUIRED)

Verify conditions are populated during reconciliation:

```go
It("should set ValidationComplete condition after validation phase", func() {
    // Create CRD
    obj := createTestCRD()
    Expect(k8sClient.Create(ctx, obj)).To(Succeed())

    // Wait for condition
    Eventually(func() bool {
        var updated v1alpha1.YourCRD
        if err := k8sClient.Get(ctx, client.ObjectKeyFromObject(obj), &updated); err != nil {
            return false
        }
        cond := conditions.GetCondition(&updated, conditions.ConditionValidationComplete)
        return cond != nil && cond.Status == metav1.ConditionTrue
    }, timeout, interval).Should(BeTrue())
})
```

---

## 📅 Implementation Timeline

| Team | CRD | Status | Deadline | Owner |
|------|-----|--------|----------|-------|
| **SignalProcessing** | SignalProcessing | ✅ **COMPLETE** (2025-12-16) | Jan 3, 2026 | SP Team |
| **RemediationOrchestrator** | RemediationRequest | 🔴 Pending | Jan 3, 2026 | RO Team |
| **RemediationOrchestrator** | RemediationApprovalRequest | 🔴 Pending | Jan 3, 2026 | RO Team |

**V1.0 Release Deadline**: January 10, 2026
**Conditions Implementation Deadline**: January 3, 2026 (1 week buffer)

**Completed CRDs** (4/6):
- ✅ AIAnalysis (2025-12-14)
- ✅ WorkflowExecution (2025-12-14)
- ✅ Notification (2025-12-14)
- ✅ SignalProcessing (2025-12-16)

**Files Delivered**:
- `pkg/signalprocessing/conditions.go` (157 lines)
- `test/unit/signalprocessing/conditions_test.go` (26 tests, 100% coverage)

---

## 🚫 Anti-Patterns to Avoid

### ❌ DON'T: Generic Reasons
```go
// BAD
SetCondition(obj, "Complete", metav1.ConditionFalse, "Failed", "Something went wrong")
```

### ✅ DO: Specific Reasons
```go
// GOOD
SetCondition(obj, ConditionEnrichmentComplete, metav1.ConditionFalse,
    ReasonK8sAPITimeout, "Failed to fetch Pod details: timeout after 30s")
```

### ❌ DON'T: Missing Context in Messages
```go
// BAD
SetValidationComplete(obj, false, "Validation failed")
```

### ✅ DO: Include Actionable Context
```go
// GOOD
SetValidationComplete(obj, false, "Validation failed: signal fingerprint missing (required per BR-SP-001)")
```

---

## 📊 Success Metrics

| Metric | Target | Measurement |
|--------|--------|-------------|
| **Coverage** | 100% CRDs | 6/6 CRDs have conditions.go |
| **Unit Test Coverage** | 100% functions | All Set*/Get*/Is* functions tested |
| **Integration Coverage** | All phases | Each phase transition sets condition |
| **Operator UX** | < 1 min debug | Conditions visible in `kubectl describe` |

---

## 🔗 Related Documents

- [DD-CRD-001: CRD API Standards](./DD-CRD-001-crd-api-standards.md)
- [Kubernetes API Conventions - Conditions](https://github.com/kubernetes/community/blob/master/contributors/devel/sig-architecture/api-conventions.md#typical-status-properties)
- [AIAnalysis Conditions Implementation](../../handoff/AIANALYSIS_CONDITIONS_IMPLEMENTATION_STATUS.md)
- [WorkflowExecution Conditions Implementation](../../handoff/REQUEST_WE_KUBERNETES_CONDITIONS_IMPLEMENTATION.md)

---

## ✅ Compliance Checklist

Before V1.0 release, each team must verify:

- [ ] `pkg/{service}/conditions.go` exists with all required conditions
- [ ] All condition types map to business requirements (BR-XXX-XXX)
- [ ] Unit tests in `test/unit/{service}/conditions_test.go`
- [ ] Integration tests verify conditions are set during reconciliation
- [ ] Controller code calls condition setters during phase transitions
- [ ] `kubectl describe {crd}` shows populated Conditions section
- [ ] Documentation updated with condition reference

---

**Document Version**: 1.1
**Created**: 2025-12-16
**Last Updated**: 2026-02-18 (Issue #79: Ready condition, ObservedGeneration, KubernetesExecution excluded)
**Author**: Platform Team
**File**: `docs/architecture/decisions/DD-CRD-002-kubernetes-conditions-standard.md`

