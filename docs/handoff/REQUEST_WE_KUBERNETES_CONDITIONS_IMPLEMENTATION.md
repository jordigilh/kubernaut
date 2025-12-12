# REQUEST: WorkflowExecution - Kubernetes Conditions Implementation

**Date**: 2025-12-11
**Version**: 1.1 (Responded)
**From**: AIAnalysis Team
**To**: WorkflowExecution Team
**Status**: ✅ **APPROVED - IMPLEMENTATION PLANNED**
**Priority**: HIGH

---

## 📋 Request Summary

**Request**: Implement Kubernetes Conditions for WorkflowExecution CRD to track Tekton pipeline execution state.

**Background**: AIAnalysis has implemented full Conditions support. WorkflowExecution should implement Conditions to surface Tekton PipelineRun state to operators.

---

## 🟡 **Current Gap (Updated Assessment)**

### WorkflowExecution Status

| Aspect | Current | Required | Gap |
|--------|---------|----------|-----|
| **Conditions Field** | ✅ Present in CRD schema (line 173-174) | ✅ `Conditions []metav1.Condition` | ✅ Complete |
| **Conditions Infrastructure** | ❌ No `conditions.go` | ✅ Helper functions | 🟡 Missing |
| **Handler Integration** | ❌ No conditions set | ✅ Set in phase handlers | 🟡 Missing |
| **Test Coverage** | ❌ No condition tests | ✅ Unit + integration tests | 🟡 Missing |

**Key Finding**: The Conditions field already exists in the CRD schema but is not being populated! This is HIGH PRIORITY to fix.

---

## ✅ **VALIDATION: Conditions Aligned with Authoritative Specs**

### **Authoritative Sources Reviewed**

| Source | Version | Key Findings |
|--------|---------|--------------|
| `api/workflowexecution/v1alpha1/workflowexecution_types.go` | v4.0 | Phases: Pending, Running, Completed, Failed, Skipped (lines 101-356) |
| `BR-WE-005` | P0 | Audit events MUST be emitted for lifecycle transitions (lines 171-191) |
| `DD-CONTRACT-001` | v1.4 | Conditions field present in schema (line 173-174) |
| `DD-WE-004` | Approved | Exponential backoff for pre-execution failures |
| `DD-WE-001/003` | Approved | Resource locking prevents parallel execution |
| `DD-AUDIT-003` | Approved | WorkflowExecution is P0 for audit traces |

### **Validation Matrix**

| Proposed Condition | Phase Alignment | BR/DD Support | Status |
|-------------------|-----------------|---------------|--------|
| TektonPipelineCreated | Pending → Running | ✅ Implicit in phase transitions | ✅ Valid |
| TektonPipelineRunning | Running | ✅ Maps to PhaseRunning (line 346) | ✅ Valid |
| TektonPipelineComplete | Completed/Failed | ✅ Maps to PhaseCompleted/PhaseFailed (lines 348-351) | ✅ Valid |
| AuditRecorded | All phases | ✅ BR-WE-005 (P0 requirement, lines 171-191) | ✅ Valid |
| **ResourceLocked** (NEW) | Skipped | ✅ DD-WE-001/003 + PhaseSkipped (line 355) | ✅ Valid |

### **Critical Finding: Missing Condition Type**

**Gap Identified**: The CRD schema defines `PhaseSkipped` (line 355) for resource locking, but the original proposal didn't include a dedicated condition for this.

**Correction**: Add `ResourceLocked` as a 5th condition (approved in WE team response).

---

## 🎯 **Validated Conditions for WorkflowExecution**

Based on authoritative specs and CRD schema:

### **Condition 1: TektonPipelineCreated**

**Type**: `TektonPipelineCreated`
**Phase Alignment**: Pending → Running transition
**Authoritative Source**: CRD schema Phase validation (line 103)
**Success Reason**: `PipelineCreated`
**Failure Reason**: `PipelineCreationFailed`

**When Set**:
- ✅ **True**: After successful `tektonClient.Create(ctx, pipelineRun)`
- ❌ **False**: Quota exceeded, RBAC errors, image pull failures

**Example Success**:
```yaml
Type: TektonPipelineCreated
Status: True
Reason: PipelineCreated
Message: PipelineRun workflow-exec-abc123 created in kubernaut-workflows namespace
LastTransitionTime: 2025-12-11T10:15:00Z
```

**Example Failure**:
```yaml
Type: TektonPipelineCreated
Status: False
Reason: QuotaExceeded
Message: Failed to create PipelineRun: pods "workflow-exec-123" is forbidden: exceeded quota
LastTransitionTime: 2025-12-11T10:15:00Z
```

---

### **Condition 2: TektonPipelineRunning**

**Type**: `TektonPipelineRunning`
**Phase Alignment**: PhaseRunning (line 346)
**Authoritative Source**: PipelineRun status sync
**Success Reason**: `PipelineStarted`
**Failure Reason**: `PipelineFailedToStart`

**When Set**:
- ✅ **True**: PipelineRun.Status.Conditions[Succeeded].Reason == "Running"
- ❌ **False**: Pipeline stuck in pending (node pressure, image pull, etc.)

**Example**:
```yaml
Type: TektonPipelineRunning
Status: True
Reason: PipelineStarted
Message: Pipeline executing task 2 of 5 (apply-memory-increase)
LastTransitionTime: 2025-12-11T10:16:00Z
```

---

### **Condition 3: TektonPipelineComplete**

**Type**: `TektonPipelineComplete`
**Phase Alignment**: PhaseCompleted OR PhaseFailed (lines 348-351)
**Authoritative Source**: PipelineRun completion status
**Success Reason**: `PipelineSucceeded`
**Failure Reasons**: Maps to CRD FailureReason constants (lines 385-410):
- `TaskFailed` - Task execution failure (line 406)
- `DeadlineExceeded` - Timeout (line 390)
- `OOMKilled` - Out of memory (line 387)

**Example Success**:
```yaml
Type: TektonPipelineComplete
Status: True
Reason: PipelineSucceeded
Message: All 5 tasks completed successfully in 45s
LastTransitionTime: 2025-12-11T10:17:00Z
```

**Example Failure**:
```yaml
Type: TektonPipelineComplete
Status: False
Reason: TaskFailed
Message: Task apply-memory-increase failed: kubectl apply failed with exit code 1
LastTransitionTime: 2025-12-11T10:17:00Z
```

---

### **Condition 4: AuditRecorded**

**Type**: `AuditRecorded`
**Phase Alignment**: All phases (cross-cutting concern)
**Authoritative Source**: BR-WE-005 (P0 requirement, lines 171-191)
**Success Reason**: `AuditSucceeded`
**Failure Reason**: `AuditFailed`

**When Set**:
- ✅ **True**: `r.AuditStore.StoreAudit(ctx, event)` succeeds
- ❌ **False**: Data Storage unavailable, network error

**Events Tracked** (per BR-WE-005):
- `workflowexecution.workflow.started`
- `workflowexecution.workflow.completed`
- `workflowexecution.workflow.failed`
- `workflowexecution.workflow.skipped`

**Example**:
```yaml
Type: AuditRecorded
Status: True
Reason: AuditSucceeded
Message: Audit event workflowexecution.workflow.completed recorded to DataStorage
LastTransitionTime: 2025-12-11T10:17:05Z
```

---

### **Condition 5: ResourceLocked (NEW)**

**Type**: `ResourceLocked`
**Phase Alignment**: PhaseSkipped (line 355)
**Authoritative Source**: DD-WE-001/003 + SkipDetails schema (lines 177-227)
**Success Reason**: `TargetResourceBusy`, `RecentlyRemediated`
**Failure Reason**: N/A (lock check succeeded, execution skipped by design)

**When Set**:
- ✅ **True**: Lock detected, Phase set to Skipped
- Reason maps to SkipReason constants (lines 360-382):
  - `SkipReasonParallelExecution` (line 364)
  - `SkipReasonCooldownActive` (line 368)

**Example**:
```yaml
Type: ResourceLocked
Status: True
Reason: TargetResourceBusy
Message: Another workflow (workflow-exec-xyz789) is currently executing on target deployment/payment-service
LastTransitionTime: 2025-12-11T10:15:00Z
```

---

## ✅ **Validation Summary**

### **Conditions Fully Aligned with Authoritative Specs**

All 5 proposed conditions are validated against:

| Validation Criteria | Status | Evidence |
|---------------------|--------|----------|
| **CRD Phase Alignment** | ✅ Complete | All 5 phases covered (Pending, Running, Completed, Failed, Skipped) |
| **Business Requirements** | ✅ Complete | BR-WE-005 audit requirement satisfied |
| **Design Decisions** | ✅ Complete | DD-WE-001/003 (locking), DD-WE-004 (backoff) |
| **Failure Reason Constants** | ✅ Complete | Maps to CRD FailureReason enum (lines 385-410) |
| **Skip Reason Constants** | ✅ Complete | Maps to SkipReason enum (lines 360-382) |
| **Contract Compliance** | ✅ Complete | DD-CONTRACT-001 v1.4 - Conditions field present |

### **Coverage Analysis**

```
WorkflowExecution Lifecycle → Condition Mapping

1. WFE Created
   ↓
2. Check Resource Lock → [ResourceLocked condition]
   ├─ Locked → Phase: Skipped, exit
   └─ Available → Continue
       ↓
3. Create PipelineRun → [TektonPipelineCreated condition]
   ↓
4. PipelineRun Starts → [TektonPipelineRunning condition]
   ↓
5. PipelineRun Completes → [TektonPipelineComplete condition]
   ↓
6. Emit Audit Event → [AuditRecorded condition]
   ↓
7. WFE Complete

✅ Full lifecycle coverage with 5 conditions
✅ All phases represented
✅ Cross-cutting concerns (audit) tracked
```

### **Kubernetes API Conventions Compliance**

Per [Kubernetes API Conventions](https://github.com/kubernetes/community/blob/master/contributors/devel/sig-architecture/api-conventions.md#typical-status-properties):

| Convention | Compliance | Notes |
|------------|------------|-------|
| `type` field | ✅ | Unique identifier for condition |
| `status` field | ✅ | True/False/Unknown |
| `reason` field | ✅ | CamelCase, machine-readable |
| `message` field | ✅ | Human-readable details |
| `lastTransitionTime` | ✅ | metav1.Time automatic |
| Positive polarity | ✅ | Conditions express desired state |

---

## 📚 **Reference Implementation: AIAnalysis**

### **Files to Review**

| File | Purpose | Lines |
|------|---------|-------|
| `pkg/aianalysis/conditions.go` | Infrastructure + helpers | 127 |
| `api/aianalysis/v1alpha1/aianalysis_types.go:450` | CRD schema field | 1 |
| `pkg/aianalysis/handlers/investigating.go:421` | Handler usage example | 1 |

**Full Documentation**: `docs/handoff/AIANALYSIS_CONDITIONS_IMPLEMENTATION_STATUS.md`

---

## 🛠️ **Implementation Steps for WorkflowExecution**

### **Step 1: Create Infrastructure** (~1 hour)

**File**: `pkg/workflowexecution/conditions.go`

**Template**: Similar to AIAnalysis, with 4 conditions + helper functions

**Lines**: ~100-120 lines

---

### **Step 2: Update CRD Schema** (~15 minutes)

**File**: `api/workflowexecution/v1alpha1/workflowexecution_types.go`

```go
// WorkflowExecutionStatus defines the observed state of WorkflowExecution
type WorkflowExecutionStatus struct {
    // ... existing fields ...

    // Conditions
    Conditions []metav1.Condition `json:"conditions,omitempty" patchStrategy:"merge" patchMergeKey:"type"`
}
```

---

### **Step 3: Update Handlers** (~1-2 hours)

**Integration Points**:

1. **After PipelineRun creation**:
```go
we.SetTektonPipelineCreated(execution, true, we.ReasonPipelineCreated,
    "Tekton PipelineRun created successfully")
```

2. **When pipeline starts**:
```go
we.SetTektonPipelineRunning(execution, true, we.ReasonPipelineStarted,
    "Pipeline execution started")
```

3. **When pipeline completes**:
```go
if pipelineRun.Status.Succeeded() {
    we.SetTektonPipelineComplete(execution, true, "Pipeline succeeded")
} else {
    we.SetTektonPipelineComplete(execution, false, "Pipeline failed: "+err.Error())
}
```

4. **After audit recorded**:
```go
we.SetAuditRecorded(execution, true, we.ReasonAuditSucceeded, "Audit event recorded")
```

---

### **Step 4: Add Tests** (~1-2 hours)

**Create**: `test/unit/workflowexecution/conditions_test.go`

**Add to integration tests**: Verify conditions during Tekton pipeline execution

---

### **Step 5: Update Documentation** (~30 minutes)

**Files to Update**:
1. `docs/services/crd-controllers/04-workflowexecution/crd-schema.md`
2. `docs/services/crd-controllers/04-workflowexecution/IMPLEMENTATION_PLAN_*.md`
3. `docs/services/crd-controllers/04-workflowexecution/testing-strategy.md`

---

## 📊 **Effort Estimate for WorkflowExecution**

| Task | Time | Difficulty |
|------|------|------------|
| Create `conditions.go` | 1 hour | Easy (copy from AIAnalysis) |
| Update CRD schema | 15 min | Easy |
| Update handlers | 1-2 hours | Medium (Tekton watch points) |
| Add tests | 1-2 hours | Medium |
| Update documentation | 30 min | Easy |
| **Total** | **3-4 hours** | **Medium** |

---

## ✅ **Benefits for WorkflowExecution**

### **Tekton Pipeline Visibility**

**Without Conditions**: Must query Tekton PipelineRun directly
**With Conditions**: All state visible in WorkflowExecution CRD

```bash
$ kubectl describe workflowexecution we-123
Status:
  Phase: Executing
  Conditions:
    Type:     TektonPipelineCreated
    Status:   True
    Message:  Tekton PipelineRun workflow-exec-123 created

    Type:     TektonPipelineRunning
    Status:   True
    Message:  Pipeline executing task step-1 of 3
```

---

## 📚 **Reference Materials**

- **AIAnalysis Implementation**: `docs/handoff/AIANALYSIS_CONDITIONS_IMPLEMENTATION_STATUS.md`
- **AIAnalysis Code**: `pkg/aianalysis/conditions.go`
- **Kubernetes API Conventions**: https://github.com/kubernetes/community/blob/master/contributors/devel/sig-architecture/api-conventions.md

---

## 🗳️ **Response Requested**

Please respond by updating the section below:

---

## 📝 **WorkflowExecution Team Response**

**Date**: 2025-12-11
**Status**: ✅ **APPROVED - HIGH PRIORITY**
**Responded By**: WorkflowExecution Team

### **Decision**

- [x] ✅ **APPROVED** - Will implement Conditions
- [ ] ⏸️ **DEFERRED** - Will defer to V1.1/V2.0 (provide reason)
- [ ] ❌ **DECLINED** - Will not implement (provide reason)

### **Current State Analysis**

**Good News**: CRD schema ALREADY HAS Conditions field!

```go
// From api/workflowexecution/v1alpha1/workflowexecution_types.go:173-174
// Conditions provide detailed status information
// +optional
Conditions []metav1.Condition `json:"conditions,omitempty"`
```

**Gap Analysis**:

| Component | Status | Action Needed |
|-----------|--------|---------------|
| CRD Schema Field | ✅ Present (line 173-174) | None |
| `conditions.go` Infrastructure | ❌ Missing | Create |
| Helper Functions | ❌ Missing | Implement |
| Controller Integration | ❌ Not setting conditions | Integrate |
| Unit Tests | ❌ Missing | Add |
| Integration Tests | ❌ Missing | Add |

**Priority Justification**: HIGH - Schema field exists but is unused. This creates:
- **User Confusion**: Field visible in CRD but always empty
- **Observability Gap**: Operators can't see Tekton pipeline state via `kubectl describe`
- **Contract Incompleteness**: RemediationOrchestrator expects rich status (DD-CONTRACT-001)

### **Implementation Plan** (Approved)

**Target Version**: V4.2 (Current Sprint)
**Target Date**: 2025-12-13 (2 days)
**Estimated Effort**: 4-5 hours

**Conditions to Implement** (Post-Validation):
- [x] TektonPipelineCreated
- [x] TektonPipelineRunning
- [x] TektonPipelineComplete
- [x] AuditRecorded
- [x] ResourceLocked (**ADDED after validation** - PhaseSkipped requires dedicated condition per CRD schema line 355)

**Implementation Approach**:

#### **Phase 1: Infrastructure** (1.5 hours)

Create `pkg/workflowexecution/conditions.go`:
- Copy structure from `pkg/aianalysis/conditions.go`
- Define 4 condition types as constants
- Implement helper functions:
  - `SetTektonPipelineCreated()`
  - `SetTektonPipelineRunning()`
  - `SetTektonPipelineComplete()`
  - `SetAuditRecorded()`
  - `GetCondition()`
  - `IsConditionTrue()`

#### **Phase 2: Controller Integration** (2 hours)

Update `internal/controller/workflowexecution/workflowexecution_controller.go`:

1. **After PipelineRun created** (Reconcile, after CreatePipelineRun):
```go
workflowexecution.SetTektonPipelineCreated(wfe, true,
    workflowexecution.ReasonPipelineCreated,
    fmt.Sprintf("PipelineRun %s created", pr.Name))
```

2. **When syncing PipelineRun status** (SyncPipelineRunStatus):
```go
if pr.Status.IsRunning() {
    workflowexecution.SetTektonPipelineRunning(wfe, true,
        workflowexecution.ReasonPipelineStarted,
        "Pipeline execution in progress")
}

if pr.Status.IsCompleted() {
    if pr.Status.IsSuccessful() {
        workflowexecution.SetTektonPipelineComplete(wfe, true,
            workflowexecution.ReasonPipelineSucceeded,
            "Pipeline completed successfully")
    } else {
        workflowexecution.SetTektonPipelineComplete(wfe, false,
            workflowexecution.ReasonPipelineFailed,
            fmt.Sprintf("Pipeline failed: %s", failureReason))
    }
}
```

3. **After audit event** (emitAudit):
```go
if err := r.AuditStore.StoreAudit(ctx, event); err != nil {
    workflowexecution.SetAuditRecorded(wfe, false,
        workflowexecution.ReasonAuditFailed, err.Error())
} else {
    workflowexecution.SetAuditRecorded(wfe, true,
        workflowexecution.ReasonAuditSucceeded, "Audit event recorded")
}
```

#### **Phase 3: Testing** (1-1.5 hours)

1. **Unit Tests**: `test/unit/workflowexecution/conditions_test.go`
   - Test each helper function
   - Test condition transitions
   - Validate reason/message population

2. **Integration Tests**: Add to existing integration tests
   - Verify conditions set during reconciliation
   - Validate condition history (transitions)
   - Test failure scenarios

#### **Phase 4: Validation** (30 min)

- Run `make generate` to regenerate CRDs
- Run full test suite
- Manual validation: `kubectl describe workflowexecution`

### **Success Criteria**

1. ✅ All 4 conditions implemented and documented
2. ✅ Conditions visible in `kubectl describe workflowexecution`
3. ✅ Unit test coverage for conditions infrastructure
4. ✅ Integration tests verify conditions during reconciliation
5. ✅ No breaking changes to existing status fields

### **Questions or Concerns**

**Q1**: Should we add `ResourceLocked` condition for when Phase=Skipped due to resource locking?

**A1**: YES - Good idea. This would help operators understand why a workflow was skipped. Add as 5th condition:
- Type: `ResourceLocked`
- Reason: `TargetResourceBusy` / `RecentlyRemediated`
- Message: Details from SkipDetails

**Q2**: Should conditions be backfilled for existing WorkflowExecutions?

**A2**: NO - Conditions will be populated going forward. Existing WFEs will have empty conditions array (not breaking).

### **Dependencies**

- None - All infrastructure exists in AIAnalysis for reference
- No external API changes required

### **Rollout Plan**

1. Implement and test in development
2. Deploy to staging for validation
3. Monitor first 10-20 executions for condition accuracy
4. Production rollout (non-breaking - additive field)

---

**Document Status**: ✅ APPROVED - Implementation Plan Documented
**Created**: 2025-12-11
**Responded**: 2025-12-11
**From**: AIAnalysis Team
**Responded By**: WorkflowExecution Team
**Target**: V4.2 (2025-12-13)
**File**: `docs/handoff/REQUEST_WE_KUBERNETES_CONDITIONS_IMPLEMENTATION.md`

