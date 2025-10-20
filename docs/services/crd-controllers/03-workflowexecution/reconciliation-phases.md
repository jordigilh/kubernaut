## Reconciliation Architecture

### Phase Transitions

**Multi-Phase Workflow Orchestration**:

```
"" (new) → planning → validating → executing → monitoring → completed
              ↓          ↓            ↓           ↓
           (5s)       (30s)     (5min/step)    (1min)
```

**Rationale**: Workflow execution requires distinct phases for safety validation, step-by-step execution, and health monitoring before completion.

### Reconciliation Flow

#### 1. **planning** Phase (BR-WF-001 to BR-WF-010)

**Purpose**: Analyze AI recommendations and create executable workflow plan

**Trigger**: WorkflowExecution CRD created by RemediationRequest controller

**Actions** (executed synchronously):

**Step 1: Workflow Analysis** (BR-WF-001, BR-WF-002)
- Parse AI recommendations from spec.workflowDefinition
- Identify workflow steps and dependencies
- Determine execution order (sequential vs parallel)
- Calculate estimated execution time

**Step 2: Dependency Resolution** (BR-WF-010, BR-WF-011)
- Build dependency graph for workflow steps
- Identify parallel execution opportunities
- Resolve step prerequisites and conditions
- Validate dependency chain completeness

**Upstream Validation** (AIAnalysis Service):
Dependencies are pre-validated by AIAnalysis service before reaching WorkflowExecution:
- ✅ **BR-AI-051**: All dependency IDs reference valid recommendations (completeness check)
- ✅ **BR-AI-052**: No circular dependencies in graph (cycle detection via topological sort)
- ✅ **BR-AI-053**: Missing dependencies defaulted to empty array (graceful handling)

**WorkflowExecution Additional Validation**:
WorkflowExecution performs workflow-specific validation beyond AIAnalysis checks:
- Verify step dependencies are within workflow bounds (step numbers are valid)
- Validate no cross-workflow dependencies (all dependencies are in same workflow)
- Confirm all referenced steps exist in workflow definition
- Validate execution order is achievable (no impossible constraints)

**Step 3: Resource Planning** (BR-WF-015)
- Identify target Kubernetes resources for each step
- Check resource availability and accessibility
- Plan resource allocation and cleanup
- Estimate resource usage and impact

**Step 4: Execution Strategy** (BR-WF-020)
- Determine rollback strategy (automatic vs manual)
- Configure retry policies per step
- Set execution timeouts and deadlines
- Define success/failure criteria

**Step 5: Status Update**
- Set `status.phase = "validating"`
- Set `status.totalSteps` with count
- Set `status.executionPlan` with detailed plan
- Record planning completion timestamp

**Transition Criteria**:
```go
if planningComplete && dependenciesResolved {
    phase = "validating"
    // Proceed to safety validation
} else if planningError {
    phase = "failed"
    reason = "planning_error"
}
```

**Timeout**: 30 seconds (fast planning required)

**Example CRD Update**:
```yaml
status:
  phase: validating
  totalSteps: 5
  currentStep: 0
  executionPlan:
    strategy: "sequential-with-parallel"
    estimatedDuration: "15m"
    rollbackStrategy: "automatic"
  stepStatuses: []
```

#### 2. **validating** Phase (BR-WF-015 to BR-WF-025)

**Purpose**: Validate safety requirements and perform dry-run if configured

**Actions**:

**Step 1: Safety Checks** (BR-WF-015, BR-WF-016)
- Validate RBAC permissions for all steps
- Check resource availability and health
- Verify cluster capacity and constraints
- Validate network connectivity to target resources

**Step 2: Dry-Run Execution** (BR-WF-017) [Optional]
- Execute dry-run for all steps if spec.executionStrategy.dryRunFirst = true
- Validate Kubernetes API responses
- Check for conflicting operations
- Verify no unexpected side effects

**Step 3: Approval Validation** (BR-WF-018)
- Check if manual approval required (spec.executionStrategy.approvalRequired)
- If approval required, wait for approval annotation
- Validate approval authority and RBAC
- Record approval metadata

**Step 4: Pre-Execution Validation** (BR-WF-019)
- Validate workflow definition completeness
- Check all step configurations are valid
- Verify rollback procedures are defined
- Confirm monitoring endpoints are accessible

**Step 5: Step-Level Precondition Planning** (BR-WF-016, BR-WF-053) [NEW - DD-002]
- Identify steps with preConditions defined
- Validate precondition Rego policies are syntactically correct
- Plan precondition evaluation strategy (which conditions are required vs warnings)
- Record which steps require precondition checks
- Load condition policies from ConfigMap if using policy references

**Step 6: Status Update**
- Set `status.phase = "executing"`
- Set `status.validationResults` with checks
- Record validation timestamp

**Transition Criteria**:
```go
if allSafetyChecksPassed && (approvalReceived || !approvalRequired) {
    phase = "executing"
    // Begin step execution
} else if safetyChecksFailed {
    phase = "failed"
    reason = "safety_validation_failed"
} else if awaitingApproval {
    // Requeue until approval received
    return ctrl.Result{RequeueAfter: 30 * time.Second}, nil
}
```

**Timeout**: 5 minutes (includes approval wait time)

#### 3. **executing** Phase (BR-WF-030 to BR-WF-045)

**Purpose**: Execute workflow steps by creating KubernetesExecution CRDs

**Actions**:

**Step 1: Step Execution Loop**
- For each step in execution order:
  - **Step 1a: Precondition Validation** (BR-WF-016) [NEW - DD-002]
    - Evaluate all `step.preConditions[]` using Rego policy engine
    - Query current cluster state for condition input (e.g., deployment status, pod count, resource availability)
    - For each precondition:
      - Execute Rego policy with cluster state as input
      - If `condition.required=true` and evaluation fails: Block execution, mark step as "blocked", update `status.stepStatuses[n].preConditionResults`, do NOT create KubernetesExecution CRD
      - If `condition.required=false` and evaluation fails: Log warning, update `status.stepStatuses[n].preConditionResults`, continue execution
    - Wait up to `condition.timeout` for async precondition checks
  - Create KubernetesExecution CRD for step (only if all required preconditions passed)
  - Set owner reference to WorkflowExecution
  - Configure step-specific parameters
  - Record step start time

**Step 2: Execution Monitoring** (BR-WF-031)
- Watch KubernetesExecution CRD status updates
- Track step completion and success/failure
- Update `status.stepStatuses` with progress
- Calculate execution metrics (duration, success rate)

**Step 3: Dependency Handling** (BR-WF-011)
- Wait for dependent steps to complete before proceeding
- Check dependency success status
- Execute parallel steps concurrently
- Handle step failures based on retry policy

**Step 4: Adaptive Adjustments** (BR-ORCHESTRATION-001 to BR-ORCHESTRATION-010)
- Monitor step execution patterns
- Adjust execution strategy based on failures
- Apply historical success patterns
- Optimize remaining step execution

**Step 5: Status Update**
- Set `status.currentStep` with progress
- Update `status.stepStatuses` with results
- Record execution metrics
- Set `status.phase = "monitoring"` when all steps completed

**Transition Criteria**:
```go
if allStepsCompleted && allStepsSuccessful {
    phase = "monitoring"
    // Verify workflow effectiveness
} else if criticalStepFailed {
    // Initiate rollback if automatic
    if spec.executionStrategy.rollbackStrategy == "automatic" {
        phase = "rolling_back"
    } else {
        phase = "failed"
        reason = "step_execution_failed"
    }
} else if stepInProgress {
    // Continue monitoring
    return ctrl.Result{RequeueAfter: 10 * time.Second}, nil
}
```

**Timeout**: 5 minutes per step (configurable)

**Example Execution Code**:
```go
func (r *WorkflowExecutionReconciler) reconcileExecuting(
    ctx context.Context,
    wf *workflowexecutionv1.WorkflowExecution,
) (ctrl.Result, error) {
    log := log.FromContext(ctx)

    // Get current execution plan
    plan := wf.Status.ExecutionPlan

    // Find next step to execute
    nextStep := findNextStep(wf.Status.StepStatuses, plan)
    if nextStep == nil {
        // All steps executed, transition to monitoring
        wf.Status.Phase = "monitoring"
        wf.Status.MonitoringStartTime = &metav1.Time{Time: time.Now()}
        return ctrl.Result{Requeue: true}, r.Status().Update(ctx, wf)
    }

    // Check if step already has KubernetesExecution CRD
    k8sExec := &executorv1.KubernetesExecution{}
    err := r.Get(ctx, client.ObjectKey{
        Name:      fmt.Sprintf("%s-step-%d", wf.Name, nextStep.StepNumber),
        Namespace: wf.Namespace,
    }, k8sExec)

    if apierrors.IsNotFound(err) {
        // Create KubernetesExecution CRD for this step
        k8sExec = &executorv1.KubernetesExecution{
            ObjectMeta: metav1.ObjectMeta{
                Name:      fmt.Sprintf("%s-step-%d", wf.Name, nextStep.StepNumber),
                Namespace: wf.Namespace,
                OwnerReferences: []metav1.OwnerReference{
                    *metav1.NewControllerRef(wf, workflowexecutionv1.GroupVersion.WithKind("WorkflowExecution")),
                },
            },
            Spec: executorv1.KubernetesExecutionSpec{
                WorkflowExecutionRef: corev1.ObjectReference{
                    Name:      wf.Name,
                    Namespace: wf.Namespace,
                },
                Action:        nextStep.Action,
                TargetCluster: nextStep.TargetCluster,
                Parameters:    nextStep.Parameters,
                SafetyChecks:  plan.SafetyChecks,
                RollbackSpec:  nextStep.RollbackSpec,
            },
        }

        if err := r.Create(ctx, k8sExec); err != nil {
            log.Error(err, "Failed to create KubernetesExecution", "step", nextStep.StepNumber)
            ErrorsTotal.WithLabelValues("k8s_exec_creation_failed", "executing").Inc()
            return ctrl.Result{RequeueAfter: 30 * time.Second}, err
        }

        log.Info("Created KubernetesExecution for step", "step", nextStep.StepNumber)
        WorkflowStepsCreatedTotal.WithLabelValues(wf.Name, nextStep.Action).Inc()

        // Update step status
        wf.Status.StepStatuses[nextStep.StepNumber].Status = "executing"
        wf.Status.StepStatuses[nextStep.StepNumber].StartTime = &metav1.Time{Time: time.Now()}
        return ctrl.Result{RequeueAfter: 10 * time.Second}, r.Status().Update(ctx, wf)
    } else if err != nil {
        return ctrl.Result{}, err
    }

    // KubernetesExecution exists, check its status
    stepStatus := &wf.Status.StepStatuses[nextStep.StepNumber]

    switch k8sExec.Status.Phase {
    case "completed":
        stepStatus.Status = "completed"
        stepStatus.EndTime = k8sExec.Status.CompletionTime
        stepStatus.Result = k8sExec.Status.ExecutionResult
        log.Info("Step completed successfully", "step", nextStep.StepNumber)
        WorkflowStepsCompletedTotal.WithLabelValues(wf.Name, nextStep.Action, "success").Inc()

    case "failed":
        stepStatus.Status = "failed"
        stepStatus.EndTime = &metav1.Time{Time: time.Now()}
        stepStatus.ErrorMessage = k8sExec.Status.ErrorMessage
        log.Error(nil, "Step failed", "step", nextStep.StepNumber, "error", k8sExec.Status.ErrorMessage)
        WorkflowStepsCompletedTotal.WithLabelValues(wf.Name, nextStep.Action, "failed").Inc()

        // Check if step is critical
        if nextStep.CriticalStep {
            // Initiate rollback if automatic
            if wf.Spec.ExecutionStrategy.RollbackStrategy == "automatic" {
                wf.Status.Phase = "rolling_back"
                wf.Status.RollbackReason = fmt.Sprintf("Critical step %d failed", nextStep.StepNumber)
            } else {
                wf.Status.Phase = "failed"
                wf.Status.FailureReason = fmt.Sprintf("Critical step %d failed, manual intervention required", nextStep.StepNumber)
            }
            return ctrl.Result{Requeue: true}, r.Status().Update(ctx, wf)
        }

        // Non-critical step failed, continue with next step

    case "executing":
        // Step still in progress, continue monitoring
        stepStatus.Status = "executing"
        log.Info("Step still executing", "step", nextStep.StepNumber)

    default:
        log.Info("Step in unknown state", "step", nextStep.StepNumber, "state", k8sExec.Status.Phase)
    }

    // Update status and requeue
    return ctrl.Result{RequeueAfter: 10 * time.Second}, r.Status().Update(ctx, wf)
}
```

#### 4. **monitoring** Phase (BR-WF-050 to BR-WF-060)

**Purpose**: Monitor workflow effectiveness and verify success

**Actions**:

**Step 1: Effectiveness Monitoring** (BR-WF-050)
- Monitor target resources for expected changes
- Verify remediation goals achieved
- Track resource health metrics
- Measure workflow impact

**Step 2: Success Validation** (BR-WF-051)
- Validate workflow success criteria met
- Check for unexpected side effects
- Verify no new alerts triggered
- Confirm resource stability

**Step 3: Learning & Optimization** (BR-ORCHESTRATION-020 to BR-ORCHESTRATION-030)
- Record workflow execution metrics
- Store success patterns to database
- Update historical success rates
- Generate optimization recommendations

**Step 4: Postcondition Verification** (BR-WF-052) [NEW - DD-002]
- After all steps complete, evaluate all `step.postConditions[]` for completed steps
- Query cluster state to verify intended outcomes achieved
- For each postcondition:
  - Execute Rego policy with post-execution cluster state as input
  - Wait up to `condition.timeout` for async verification (e.g., pods starting, deployment stabilizing)
  - If `condition.required=true` and verification fails: Mark step as "failed", update `status.stepStatuses[n].postConditionResults`, trigger rollback if `rollbackStrategy=automatic`
  - If `condition.required=false` and verification fails: Log warning, update `status.stepStatuses[n].postConditionResults`, mark as partial success
- Aggregate postcondition results across all steps
- Use postcondition results to inform workflow effectiveness score

**Step 5: Status Update**
- Set `status.phase = "completed"`
- Set `status.workflowResult` with outcome (consider postcondition results)
- Record completion timestamp
- Calculate workflow effectiveness score (postcondition success rate contributes to score)

**Transition Criteria**:
```go
if workflowEffective && successValidated {
    phase = "completed"
    result = "success"
    // Workflow succeeded
} else if monitoringTimeout {
    phase = "completed"
    result = "unknown"
    reason = "monitoring_timeout_cannot_verify_success"
} else if newAlertsTriggered {
    phase = "failed"
    result = "ineffective"
    reason = "workflow_triggered_new_alerts"
}
```

**Timeout**: 10 minutes (post-execution monitoring)

**Example CRD Update**:
```yaml
status:
  phase: completed
  totalSteps: 5
  currentStep: 5
  stepStatuses:
  - stepNumber: 1
    action: "scale-deployment"
    status: "completed"
    startTime: "2025-01-15T10:00:00Z"
    endTime: "2025-01-15T10:02:00Z"
    result:
      resourcesAffected: ["deployment/web-app"]
      changesApplied: ["replicas: 3 → 5"]
  # ... more steps
  workflowResult:
    outcome: "success"
    effectivenessScore: 0.95
    resourceHealth: "healthy"
    newAlertsTriggered: false
  executionMetrics:
    totalDuration: "15m23s"
    stepSuccessRate: 1.0
    rollbacksPerformed: 0
  completionTime: "2025-01-15T10:15:23Z"
```

#### 5. **completed** Phase (Terminal State)

**Purpose**: Record workflow completion and cleanup

**Actions**:
- Record workflow execution to PostgreSQL audit table
- Store workflow patterns to vector database for ML
- Emit Kubernetes event: `WorkflowExecutionCompleted`
- Update RemediationRequest status
- Wait for 24-hour retention before cleanup

**No Timeout** (terminal state)

**Note**: WorkflowExecution CRD remains for 24 hours for review, then cleaned up by RemediationRequest lifecycle.

#### 6. **failed** Phase (Terminal State)

**Purpose**: Record failure for debugging

**Actions**:
- Log failure reason and context
- Emit Kubernetes event: `WorkflowExecutionFailed`
- Record failure to audit database
- Store failure patterns for learning
- Escalate to manual intervention if rollback failed

**No Requeue** (terminal state - requires manual intervention)

---

### CRD-Based Coordination Patterns

#### Event-Driven Coordination

This service uses **CRD-based reconciliation** for coordination with RemediationRequest controller:

1. **Created By**: RemediationRequest controller creates WorkflowExecution CRD (with owner reference)
2. **Watch Pattern**: RemediationRequest watches WorkflowExecution status for completion
3. **Status Propagation**: Status updates trigger RemediationRequest reconciliation automatically (<1s latency)
4. **Event Emission**: Emit Kubernetes events for operational visibility

**Coordination Flow**:
```
RemediationRequest.status.overallPhase = "executing"
    ↓
RemediationRequest Controller creates WorkflowExecution CRD
    ↓
WorkflowExecution Controller reconciles (this controller)
    ↓
WorkflowExecution.status.phase = "completed"
    ↓ (watch trigger in RemediationRequest)
RemediationRequest Controller reconciles (detects completion)
    ↓
RemediationRequest Controller creates KubernetesExecution CRD
```

---

#### Owner Reference Management

**This CRD (WorkflowExecution)**:
- **Owned By**: RemediationRequest (parent CRD)
- **Owner Reference**: Set at creation by RemediationRequest controller
- **Cascade Deletion**: Deleted automatically when RemediationRequest is deleted
- **Owns**: Nothing (does NOT create KubernetesExecution)
- **Watches**: Nothing (processes own CRD only)

**Leaf Controller Pattern** (Similar to RemediationProcessing):

WorkflowExecution is a **leaf controller** in the remediation workflow:
- ✅ **Clear responsibility**: Execute workflow steps, update status, done
- ✅ **No CRD creation**: Does NOT create KubernetesExecution (common misconception)
- ✅ **No watches**: Only processes its own CRD
- ✅ **Separation of concerns**: Workflow logic separate from Kubernetes execution

**Lifecycle**:
```
RemediationRequest Controller
    ↓ (creates with owner reference)
WorkflowExecution CRD
    ↓ (executes workflow steps)
WorkflowExecution.status.phase = "completed"
    ↓ (watch trigger)
RemediationRequest Controller reconciles
    ↓ (IMPORTANT: RemediationRequest creates next CRD, not WorkflowExecution)
RemediationRequest Controller creates KubernetesExecution CRD
```

---

#### Critical Clarification: NO WorkflowExecution → KubernetesExecution Creation

**Common Misconception**: WorkflowExecution creates KubernetesExecution CRD

**Actual Architecture**: RemediationRequest creates ALL service CRDs (centralized orchestration)

**Why This Matters**:
- **Separation of Concerns**: Workflow logic separate from Kubernetes execution logic
- **Centralized Orchestration**: RemediationRequest is the single orchestrator
- **Simplified Controllers**: Each controller only processes its own CRD
- **Clear Dependencies**: RemediationRequest manages the entire workflow sequence

**What WorkflowExecution Does NOT Do**:
- ❌ Create KubernetesExecution CRD (RemediationRequest does this)
- ❌ Watch KubernetesExecution status (RemediationRequest does this)
- ❌ Execute Kubernetes operations directly (KubernetesExecution does this)
- ❌ Coordinate with Kubernetes Executor Service

**What WorkflowExecution DOES Do**:
- ✅ Execute multi-step workflow logic
- ✅ Manage step dependencies and parallel execution
- ✅ Store workflow results in status (for RemediationRequest to copy)
- ✅ Update status to "completed" when workflow finishes
- ✅ Trust RemediationRequest to create KubernetesExecution

**Coordination Sequence**:
```
WorkflowExecution.status.phase = "completed"
    ↓ (WorkflowExecution controller STOPS here)
RemediationRequest detects completion via watch
    ↓
RemediationRequest extracts workflow results from WorkflowExecution.status
    ↓
RemediationRequest creates KubernetesExecution CRD with operations
```

---

#### No Direct HTTP Calls Between Controllers

**Anti-Pattern (Avoided)**: ❌ WorkflowExecution calling KubernetesExecution controller via HTTP

**Correct Pattern (Used)**: ✅ CRD status update + RemediationRequest watch-based coordination

**Why This Matters**:
- **Reliability**: CRD status persists in etcd (HTTP calls can fail silently)
- **Observability**: Status visible via `kubectl get workflowexecution` (HTTP calls are opaque)
- **Kubernetes-Native**: Leverages built-in watch/reconcile patterns (no custom HTTP infrastructure)
- **Decoupling**: WorkflowExecution doesn't need to know about KubernetesExecution existence
- **Centralized Control**: RemediationRequest manages ALL service CRD lifecycle

**Why NOT Let WorkflowExecution Create KubernetesExecution**:
- ❌ **Tight Coupling**: WorkflowExecution coupled to KubernetesExecution
- ❌ **Complex Testing**: Need to mock K8s Executor in workflow tests
- ❌ **Harder Debugging**: Workflow and execution concerns mixed
- ❌ **Distributed Orchestration**: Multiple controllers managing workflow sequence

**Why Centralized Orchestration is Better**:
- ✅ **Single Orchestrator**: RemediationRequest is the single source of truth
- ✅ **Simple Controllers**: Each controller only processes its own CRD
- ✅ **Easy Testing**: Mock only K8s client, not other controllers
- ✅ **Clear Flow**: Sequential CRD creation visible in RemediationRequest controller

---

#### Watch Configuration (Upstream)

**RemediationRequest Watches WorkflowExecution**:

```go
// In RemediationRequestReconciler.SetupWithManager()
err = c.Watch(
    &source.Kind{Type: &workflowexecutionv1.WorkflowExecution{}},
    handler.EnqueueRequestsFromMapFunc(r.workflowExecutionToRemediation),
)

// Mapping function
func (r *RemediationRequestReconciler) workflowExecutionToRemediation(obj client.Object) []ctrl.Request {
    wf := obj.(*workflowexecutionv1.WorkflowExecution)
    return []ctrl.Request{
        {
            NamespacedName: types.NamespacedName{
                Name:      wf.Spec.RemediationRequestRef.Name,
                Namespace: wf.Spec.RemediationRequestRef.Namespace,
            },
        },
    }
}
```

**Result**: Any WorkflowExecution status update triggers RemediationRequest reconciliation within ~100ms.

---

#### Coordination Benefits

**For WorkflowExecution Controller**:
- ✅ **Focused**: Only handles workflow execution logic
- ✅ **No Kubernetes Operations**: Doesn't need K8s API access for remediation
- ✅ **Testable**: Unit tests only need fake K8s client for CRD
- ✅ **Decoupled**: Doesn't know about Kubernetes Executor Service

**For RemediationRequest Controller**:
- ✅ **Visibility**: Can query WorkflowExecution status anytime
- ✅ **Control**: Decides when to create KubernetesExecution
- ✅ **Data Extraction**: Copies workflow operations to KubernetesExecution spec
- ✅ **Timeout Detection**: Can detect if WorkflowExecution takes too long

**For Operations**:
- ✅ **Debuggable**: `kubectl get workflowexecution -o yaml` shows workflow state
- ✅ **Observable**: Kubernetes events show workflow progress
- ✅ **Traceable**: CRD history shows workflow execution timeline
- ✅ **Clear Sequence**: RemediationRequest shows entire orchestration flow

---

