## Reconciliation Architecture

**Version**: 4.0
**Last Updated**: 2025-12-02
**CRD API Group**: `workflowexecution.kubernaut.ai/v1alpha1`
**Status**: ✅ Updated for Tekton Architecture (ADR-044)

---

## Changelog

### Version 4.0 (2025-12-02)
- ✅ **Rewritten**: Complete rewrite for Tekton PipelineRun delegation
- ✅ **Removed**: All step orchestration logic (Tekton handles this)
- ✅ **Added**: Resource locking phases (DD-WE-001)

---

### Phase Transitions

**Simplified Workflow Execution**:

```
"" (new) → Pending → Running → Completed
              ↓         ↓
           Skipped   Failed
```

**Rationale**: WorkflowExecution delegates step orchestration to Tekton. The controller only:
1. Checks resource locks
2. Creates PipelineRun
3. Monitors PipelineRun status
4. Updates WorkflowExecution status

---

### Reconciliation Flow

#### 1. **Pending** Phase (Initial)

**Purpose**: Validate spec and check resource locks before execution

**Trigger**: WorkflowExecution CRD created by RemediationOrchestrator

**Actions**:

**Step 1: Spec Validation**
- Validate `spec.workflowRef.containerImage` is present
- Validate `spec.targetResource` format
- Validate `spec.parameters` against workflow schema (optional)

**Step 2: Resource Lock Check** (BR-WE-009)
- Query for Running/Pending WorkflowExecutions with same `targetResource`
- If found: Set `phase = "Skipped"`, `skipDetails.reason = "ResourceBusy"`
- Include `skipDetails.conflictingWorkflow` with blocking WFE info

**Step 3: Cooldown Check** (BR-WE-010)
- Query for recent Completed WorkflowExecutions with same `targetResource` + `workflowId`
- If within cooldown period: Set `phase = "Skipped"`, `skipDetails.reason = "RecentlyRemediated"`
- Include `skipDetails.cooldownRemaining`

**Step 4: Create PipelineRun** (BR-WE-001)
- Build PipelineRun with bundle resolver
- Set owner reference to WorkflowExecution
- Pass parameters from `spec.parameters`
- Set `phase = "Running"`

```go
func (r *WorkflowExecutionReconciler) reconcilePending(
    ctx context.Context,
    wfe *workflowexecutionv1.WorkflowExecution,
) (ctrl.Result, error) {
    log := log.FromContext(ctx)

    // Step 1: Validate spec
    if err := r.validateSpec(wfe); err != nil {
        return r.markFailed(ctx, wfe, "ValidationError", err.Error())
    }

    // Step 2: Check resource lock
    if blocked, conflicting := r.checkResourceLock(ctx, wfe); blocked {
        return r.markSkipped(ctx, wfe, "ResourceBusy", conflicting)
    }

    // Step 3: Check cooldown
    if recent := r.checkCooldown(ctx, wfe); recent != nil {
        return r.markSkipped(ctx, wfe, "RecentlyRemediated", recent)
    }

    // Step 4: Create PipelineRun
    pr := r.buildPipelineRun(wfe)
    if err := r.Create(ctx, pr); err != nil {
        log.Error(err, "Failed to create PipelineRun")
        return r.markFailed(ctx, wfe, "PipelineRunCreationFailed", err.Error())
    }

    // Transition to Running
    wfe.Status.Phase = workflowexecutionv1.PhaseRunning
    wfe.Status.StartTime = &metav1.Time{Time: time.Now()}
    wfe.Status.PipelineRunRef = &corev1.ObjectReference{
        Name:      pr.Name,
        Namespace: pr.Namespace,
    }

    return ctrl.Result{RequeueAfter: 10 * time.Second}, r.Status().Update(ctx, wfe)
}
```

**Transition Criteria**:
```
if resourceLockBlocked → phase = "Skipped" (ResourceBusy)
if recentlyRemediated → phase = "Skipped" (RecentlyRemediated)
if pipelineRunCreated → phase = "Running"
if validationFailed   → phase = "Failed"
```

---

#### 2. **Running** Phase

**Purpose**: Monitor PipelineRun status and update WorkflowExecution

**Actions**:

**Step 1: Get PipelineRun Status**
- Fetch PipelineRun by `status.pipelineRunRef`
- Handle NotFound (external deletion)

**Step 2: Map Tekton Status to WFE Status** (BR-WE-003)
- Tekton `Succeeded=True` → Phase `Completed`, Outcome `Success`
- Tekton `Succeeded=False` → Phase `Failed`, extract `FailureDetails`
- Tekton running → Requeue for status check

**Step 3: Extract Failure Details** (BR-WE-004)
- Parse Tekton condition message for task/step failure
- Build `FailureDetails` struct with reason, message, task info
- Generate `naturalLanguageSummary` for recovery context

```go
func (r *WorkflowExecutionReconciler) reconcileRunning(
    ctx context.Context,
    wfe *workflowexecutionv1.WorkflowExecution,
) (ctrl.Result, error) {
    log := log.FromContext(ctx)

    // Get PipelineRun
    var pr tektonv1.PipelineRun
    if err := r.Get(ctx, client.ObjectKey{
        Name:      wfe.Status.PipelineRunRef.Name,
        Namespace: wfe.Namespace,
    }, &pr); err != nil {
        if apierrors.IsNotFound(err) {
            // PipelineRun externally deleted
            return r.markFailed(ctx, wfe, "PipelineRunDeleted", "PipelineRun was deleted externally")
        }
        return ctrl.Result{}, err
    }

    // Check Tekton conditions
    for _, cond := range pr.Status.Conditions {
        if cond.Type == "Succeeded" {
            switch cond.Status {
            case "True":
                // Success
                return r.markCompleted(ctx, wfe, &pr)
            case "False":
                // Failure - extract details
                return r.markFailedFromPipelineRun(ctx, wfe, &pr, cond)
            default:
                // Still running - requeue
                log.V(1).Info("PipelineRun still running", "reason", cond.Reason)
                return ctrl.Result{RequeueAfter: 10 * time.Second}, nil
            }
        }
    }

    // No Succeeded condition yet - requeue
    return ctrl.Result{RequeueAfter: 10 * time.Second}, nil
}
```

**Transition Criteria**:
```
if tektonSucceeded     → phase = "Completed"
if tektonFailed        → phase = "Failed"
if tektonRunning       → requeue(10s)
if pipelineRunDeleted  → phase = "Failed"
```

---

#### 3. **Completed** Phase (Terminal)

**Purpose**: Record success and cleanup

**Actions**:
- Set `status.outcome = "Success"`
- Set `status.completionTime`
- Record audit event to Data Storage
- Emit Kubernetes event

**No Requeue** (terminal state)

---

#### 4. **Failed** Phase (Terminal)

**Purpose**: Record failure details for debugging and recovery

**Actions**:
- Set `status.outcome = "Failed"`
- Populate `status.failureDetails` with:
  - `reason`: K8s-style reason code (e.g., `TaskRunFailed`, `Timeout`)
  - `message`: Human-readable error
  - `naturalLanguageSummary`: LLM-friendly description for recovery
  - `wasExecutionFailure`: true if workflow started executing
  - `requiresManualReview`: true for non-idempotent failure scenarios
- Record audit event
- Emit Kubernetes event

**No Requeue** (terminal state)

---

#### 5. **Skipped** Phase (Terminal)

**Purpose**: Record why execution was skipped (resource lock or cooldown)

**Actions**:
- Set `status.skipDetails` with:
  - `reason`: `ResourceBusy` or `RecentlyRemediated`
  - `message`: Human-readable explanation
  - `conflictingWorkflow`: (if ResourceBusy) reference to blocking WFE
  - `skippedAt`: timestamp
- Emit Kubernetes event
- **No PipelineRun created**

**No Requeue** (terminal state)

---

### CRD-Based Coordination

#### Owner Reference Management

**This CRD (WorkflowExecution)**:
- **Owned By**: RemediationRequest (parent CRD)
- **Cascade Deletion**: Deleted automatically when RemediationRequest is deleted
- **Owns**: Tekton PipelineRun (child resource)

**Coordination Flow**:
```
RemediationOrchestrator creates WorkflowExecution
    ↓
WorkflowExecution Controller reconciles
    ↓
WorkflowExecution creates PipelineRun (owned)
    ↓
Tekton executes workflow
    ↓
WorkflowExecution syncs status from PipelineRun
    ↓
WorkflowExecution.status.phase = "Completed"
    ↓ (watch trigger)
RemediationOrchestrator detects completion
```

---

### Architecture Principles

1. **Delegation, Not Orchestration**: WorkflowExecution does NOT orchestrate steps. Tekton handles all step execution, parallelism, and dependencies.

2. **Single PipelineRun**: One WorkflowExecution creates exactly one PipelineRun. The workflow OCI bundle contains the complete pipeline definition.

3. **Status Synchronization**: WorkflowExecution status mirrors PipelineRun status. No independent state tracking.

4. **Resource Locking**: Safety mechanism to prevent parallel/redundant executions on same resource (DD-WE-001).

---

