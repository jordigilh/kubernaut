# Workflow Execution Service - CRD Implementation

**Service Type**: CRD Controller
**Health/Ready Port**: 8080 (`/health`, `/ready` - no auth required)
**Metrics Port**: 9090 (`/metrics` - with auth filter)
**CRD**: WorkflowExecution (workflowexecution.kubernaut.io/v1)
**Controller**: WorkflowExecutionReconciler
**Status**: ⚠️ **NEEDS IMPLEMENTATION**
**Priority**: **P0 - CRITICAL**
**Effort**: 2 weeks
**Confidence**: 90%

---

## 📚 Related Documentation

**CRD Design Specification**: [docs/design/CRD/04_WORKFLOW_EXECUTION_CRD.md](../../design/CRD/04_WORKFLOW_EXECUTION_CRD.md)

**Related Services**:
- **Creates & Watches**: KubernetesExecution (DEPRECATED - ADR-025) CRD (for step execution)
- **Integrates With**: Workflow Engine, Executor Service, Data Storage Service

**Architecture References**:
- [Multi-CRD Reconciliation Architecture](../../architecture/MULTI_CRD_RECONCILIATION_ARCHITECTURE.md)
- [Workflow Engine & Orchestration Architecture](../../architecture/WORKFLOW_ENGINE_ORCHESTRATION_ARCHITECTURE.md)
- [Service Connectivity Specification](../../architecture/SERVICE_CONNECTIVITY_SPECIFICATION.md)

---

## Business Requirements

**Primary Business Requirements**:
- **BR-WF-001 to BR-WF-040**: Core workflow engine functionality (step execution, dependency resolution, state management)
- **BR-ORCHESTRATION-001 to BR-ORCHESTRATION-045**: Adaptive orchestration capabilities
- **BR-EXECUTION-001 to BR-EXECUTION-035**: Action execution and validation
- **BR-AUTOMATION-001 to BR-AUTOMATION-030**: Intelligent automation patterns

**Secondary Business Requirements**:
- **BR-WF-041 to BR-WF-080**: Workflow template management and versioning
- **BR-WF-081 to BR-WF-120**: Safety validation and dry-run capabilities
- **BR-WF-121 to BR-WF-165**: Rollback mechanisms and recovery procedures
- **BR-MONITORING-015 to BR-MONITORING-030**: Workflow monitoring and observability

**Excluded Requirements** (Delegated to Other Services):
- **BR-EX-001 to BR-EX-020**: Kubernetes API operations - Executor Service responsibility
- **BR-AI-001 to BR-AI-050**: AI analysis and recommendations - AI Analysis Service responsibility
- **BR-ALERT-001 to BR-ALERT-030**: Alert processing and enrichment - Alert Processor responsibility

**Note**: Workflow Execution receives WorkflowExecution CRDs from AlertRemediation controller after AIAnalysis completes and recommendations are approved.

---

## Overview

**Purpose**: Orchestrates multi-step remediation workflows with adaptive execution, safety validation, and intelligent optimization.

**Core Responsibilities**:
1. Plan workflow execution based on AI recommendations (BR-WF-001, BR-WF-002)
2. Validate safety requirements and prerequisites (BR-WF-015, BR-WF-016)
3. Execute workflow steps with dependency resolution (BR-WF-010, BR-WF-011)
4. Monitor execution progress and health (BR-WF-030, BR-WF-031)
5. Handle failures with rollback and recovery (BR-WF-050, BR-WF-051)
6. Create KubernetesExecution CRDs for approved actions
7. Apply adaptive orchestration based on runtime conditions (BR-ORCHESTRATION-001)

**V1 Scope - Core Workflow Orchestration**:
- Single-workflow execution (sequential or parallel steps)
- Safety validation with dry-run capabilities
- Basic rollback strategies (manual and automatic)
- Step dependency resolution
- Real-time execution monitoring
- Kubernetes action delegation to Executor Service
- Workflow state persistence in CRD

**Future V2 Enhancements** (Out of Scope):
- Multi-workflow orchestration (compound workflows)
- Advanced machine learning for step optimization
- Cross-cluster workflow execution
- Workflow scheduling and batching
- Advanced canary and blue-green strategies

**Key Architectural Decisions**:
- **Multi-Phase State Machine**: Planning → Validating → Executing → Monitoring → Completed (5 phases)
- **Step-Based Execution**: Each step creates KubernetesExecution CRD for atomic operations
- **Adaptive Orchestration**: Runtime adjustment based on success/failure patterns
- **Safety-First Validation**: Mandatory validation phase before execution
- **Rollback Capability**: Automatic or manual rollback with state preservation
- **Watch-Based Coordination**: Monitors KubernetesExecution status for step completion
- **24-Hour Retention**: Aligned with AlertRemediation lifecycle
- **Does NOT execute K8s operations** (Executor Service responsibility)

---

## Development Methodology

**Mandatory Process**: Follow APDC-Enhanced TDD workflow per [.cursor/rules/00-core-development-methodology.mdc](../../../.cursor/rules/00-core-development-methodology.mdc)

### APDC-TDD Workflow

```
┌─────────────────────────────────────────────────────────────┐
│ ANALYSIS → PLAN → DO-RED → DO-GREEN → DO-REFACTOR → CHECK  │
└─────────────────────────────────────────────────────────────┘
```

**ANALYSIS** (5-15 min): Comprehensive context understanding
  - Search existing implementations (`codebase_search "workflow execution implementations"`)
  - Identify reusable components in `pkg/workflow/` and `pkg/orchestration/`
  - Map business requirements (BR-WF-001 to BR-WF-165, BR-ORCHESTRATION-001 to BR-ORCHESTRATION-045)
  - Identify integration points in `cmd/` for workflow execution controller

**PLAN** (10-20 min): Detailed implementation strategy
  - Define TDD phase breakdown (RED → GREEN → REFACTOR)
  - Plan integration points (WorkflowExecutionReconciler in cmd/workflowexecution/)
  - Establish success criteria (planning <5s, validation <30s, step execution <5min each)
  - Identify risks (step failures → rollback logic, K8s Executor unavailability → degraded mode)

**DO-RED** (10-15 min): Write failing tests FIRST
  - Unit tests defining business contract (70%+ coverage target)
  - Use FAKE K8s client (`sigs.k8s.io/controller-runtime/pkg/client/fake`)
  - Mock ONLY external HTTP services (none for workflow controller)
  - Use REAL workflow planning and validation business logic
  - Map tests to business requirements (BR-WF-XXX)

**DO-GREEN** (15-20 min): Minimal implementation
  - Define WorkflowExecutionReconciler interface to make tests compile
  - Minimal code to pass tests (basic planning, validation, execution)
  - **MANDATORY integration in cmd/workflowexecution/** (controller startup)
  - Add owner references to AlertRemediation CRD
  - Add KubernetesExecution CRD creation for steps

**DO-REFACTOR** (20-30 min): Enhance with sophisticated logic
  - **NO new types/interfaces/files** (enhance existing controller methods)
  - Add sophisticated workflow planning algorithms (dependency resolution, parallel execution)
  - Maintain integration with AlertRemediation orchestration
  - Add adaptive orchestration based on historical success patterns
  - Optimize step execution and monitoring

**CHECK** (5-10 min): Validation and confidence assessment
  - Business requirement verification (BR-WF-001 to BR-WF-165 addressed)
  - Integration confirmation (controller in cmd/workflowexecution/)
  - Test coverage validation (70%+ unit, 20% integration, 10% E2E)
  - Performance validation (per-step <5min, total workflow <30min)
  - Confidence assessment: 90% (high confidence, workflow orchestration pattern)

**AI Assistant Checkpoints**: See [.cursor/rules/10-ai-assistant-behavioral-constraints.mdc](../../../.cursor/rules/10-ai-assistant-behavioral-constraints.mdc)
  - **Checkpoint A**: Type Reference Validation (read WorkflowExecution CRD types before referencing)
  - **Checkpoint B**: Test Creation Validation (search existing workflow patterns)
  - **Checkpoint C**: Business Integration Validation (verify cmd/workflowexecution/ integration)
  - **Checkpoint D**: Build Error Investigation (complete dependency analysis for workflow components)

### Quick Decision Matrix

| Starting Point | Required Phase | Reference |
|----------------|---------------|-----------|
| **New WorkflowExecution controller** | Full APDC workflow | Controller pattern is new |
| **Enhance validation logic** | DO-RED → DO-REFACTOR | Existing validation is well-understood |
| **Fix rollback bugs** | ANALYSIS → DO-RED → DO-REFACTOR | Understand workflow state management first |
| **Add step execution tests** | DO-RED only | Write tests for step orchestration logic |

**Testing Strategy Reference**: [.cursor/rules/03-testing-strategy.mdc](../../../.cursor/rules/03-testing-strategy.mdc)
  - Unit Tests (70%+): test/unit/workflowexecution/ - Fake K8s client for all CRDs
  - Integration Tests (20%): test/integration/workflowexecution/ - Real K8s (KIND), cross-CRD lifecycle
  - E2E Tests (10%): test/e2e/workflowexecution/ - Complete workflow-to-completion scenario

---

## Package Structure

**Approved Structure**: `cmd/workflowexecution/` (single-word compound), `pkg/workflow/execution/` (nested allowed for business logic)

Following Go idioms and codebase patterns, the Workflow Execution service uses a single-word compound package name for cmd and nested structure for pkg:

```
cmd/workflowexecution/          → Main application entry point
  └── main.go

pkg/workflow/execution/         → Business logic (PUBLIC API)
  ├── orchestrator.go           → WorkflowOrchestrator interface
  ├── planner.go               → Workflow planning logic
  ├── validator.go             → Safety validation
  ├── executor.go              → Step execution coordination
  ├── monitor.go               → Execution monitoring
  ├── rollback.go              → Rollback strategy implementation
  └── types.go                 → Type-safe result types

internal/controller/            → CRD controller (INTERNAL)
  └── workflowexecution_controller.go
```

**Migration**: Leverage existing `pkg/workflow/` components for step management

---

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

**Trigger**: WorkflowExecution CRD created by AlertRemediation controller

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

**Step 5: Status Update**
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
  - Create KubernetesExecution CRD for step
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

**Step 4: Status Update**
- Set `status.phase = "completed"`
- Set `status.workflowResult` with outcome
- Record completion timestamp
- Calculate workflow effectiveness score

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
- Update AlertRemediation status
- Wait for 24-hour retention before cleanup

**No Timeout** (terminal state)

**Note**: WorkflowExecution CRD remains for 24 hours for review, then cleaned up by AlertRemediation lifecycle.

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

This service uses **CRD-based reconciliation** for coordination with AlertRemediation controller:

1. **Created By**: AlertRemediation controller creates WorkflowExecution CRD (with owner reference)
2. **Watch Pattern**: AlertRemediation watches WorkflowExecution status for completion
3. **Status Propagation**: Status updates trigger AlertRemediation reconciliation automatically (<1s latency)
4. **Event Emission**: Emit Kubernetes events for operational visibility

**Coordination Flow**:
```
AlertRemediation.status.overallPhase = "executing"
    ↓
AlertRemediation Controller creates WorkflowExecution CRD
    ↓
WorkflowExecution Controller reconciles (this controller)
    ↓
WorkflowExecution.status.phase = "completed"
    ↓ (watch trigger in AlertRemediation)
AlertRemediation Controller reconciles (detects completion)
    ↓
AlertRemediation Controller creates KubernetesExecution CRD
```

---

#### Owner Reference Management

**This CRD (WorkflowExecution)**:
- **Owned By**: AlertRemediation (parent CRD)
- **Owner Reference**: Set at creation by AlertRemediation controller
- **Cascade Deletion**: Deleted automatically when AlertRemediation is deleted
- **Owns**: Nothing (does NOT create KubernetesExecution)
- **Watches**: Nothing (processes own CRD only)

**Leaf Controller Pattern** (Similar to AlertProcessing):

WorkflowExecution is a **leaf controller** in the remediation workflow:
- ✅ **Clear responsibility**: Execute workflow steps, update status, done
- ✅ **No CRD creation**: Does NOT create KubernetesExecution (common misconception)
- ✅ **No watches**: Only processes its own CRD
- ✅ **Separation of concerns**: Workflow logic separate from Kubernetes execution

**Lifecycle**:
```
AlertRemediation Controller
    ↓ (creates with owner reference)
WorkflowExecution CRD
    ↓ (executes workflow steps)
WorkflowExecution.status.phase = "completed"
    ↓ (watch trigger)
AlertRemediation Controller reconciles
    ↓ (IMPORTANT: AlertRemediation creates next CRD, not WorkflowExecution)
AlertRemediation Controller creates KubernetesExecution CRD
```

---

#### Critical Clarification: NO WorkflowExecution → KubernetesExecution Creation

**Common Misconception**: WorkflowExecution creates KubernetesExecution CRD

**Actual Architecture**: AlertRemediation creates ALL service CRDs (centralized orchestration)

**Why This Matters**:
- **Separation of Concerns**: Workflow logic separate from Kubernetes execution logic
- **Centralized Orchestration**: AlertRemediation is the single orchestrator
- **Simplified Controllers**: Each controller only processes its own CRD
- **Clear Dependencies**: AlertRemediation manages the entire workflow sequence

**What WorkflowExecution Does NOT Do**:
- ❌ Create KubernetesExecution CRD (AlertRemediation does this)
- ❌ Watch KubernetesExecution status (AlertRemediation does this)
- ❌ Execute Kubernetes operations directly (KubernetesExecution does this)
- ❌ Coordinate with Kubernetes Executor Service

**What WorkflowExecution DOES Do**:
- ✅ Execute multi-step workflow logic
- ✅ Manage step dependencies and parallel execution
- ✅ Store workflow results in status (for AlertRemediation to copy)
- ✅ Update status to "completed" when workflow finishes
- ✅ Trust AlertRemediation to create KubernetesExecution

**Coordination Sequence**:
```
WorkflowExecution.status.phase = "completed"
    ↓ (WorkflowExecution controller STOPS here)
AlertRemediation detects completion via watch
    ↓
AlertRemediation extracts workflow results from WorkflowExecution.status
    ↓
AlertRemediation creates KubernetesExecution CRD with operations
```

---

#### No Direct HTTP Calls Between Controllers

**Anti-Pattern (Avoided)**: ❌ WorkflowExecution calling KubernetesExecution controller via HTTP

**Correct Pattern (Used)**: ✅ CRD status update + AlertRemediation watch-based coordination

**Why This Matters**:
- **Reliability**: CRD status persists in etcd (HTTP calls can fail silently)
- **Observability**: Status visible via `kubectl get workflowexecution` (HTTP calls are opaque)
- **Kubernetes-Native**: Leverages built-in watch/reconcile patterns (no custom HTTP infrastructure)
- **Decoupling**: WorkflowExecution doesn't need to know about KubernetesExecution existence
- **Centralized Control**: AlertRemediation manages ALL service CRD lifecycle

**Why NOT Let WorkflowExecution Create KubernetesExecution**:
- ❌ **Tight Coupling**: WorkflowExecution coupled to KubernetesExecution
- ❌ **Complex Testing**: Need to mock K8s Executor in workflow tests
- ❌ **Harder Debugging**: Workflow and execution concerns mixed
- ❌ **Distributed Orchestration**: Multiple controllers managing workflow sequence

**Why Centralized Orchestration is Better**:
- ✅ **Single Orchestrator**: AlertRemediation is the single source of truth
- ✅ **Simple Controllers**: Each controller only processes its own CRD
- ✅ **Easy Testing**: Mock only K8s client, not other controllers
- ✅ **Clear Flow**: Sequential CRD creation visible in AlertRemediation controller

---

#### Watch Configuration (Upstream)

**AlertRemediation Watches WorkflowExecution**:

```go
// In AlertRemediationReconciler.SetupWithManager()
err = c.Watch(
    &source.Kind{Type: &workflowexecutionv1.WorkflowExecution{}},
    handler.EnqueueRequestsFromMapFunc(r.workflowExecutionToRemediation),
)

// Mapping function
func (r *AlertRemediationReconciler) workflowExecutionToRemediation(obj client.Object) []ctrl.Request {
    wf := obj.(*workflowexecutionv1.WorkflowExecution)
    return []ctrl.Request{
        {
            NamespacedName: types.NamespacedName{
                Name:      wf.Spec.AlertRemediationRef.Name,
                Namespace: wf.Spec.AlertRemediationRef.Namespace,
            },
        },
    }
}
```

**Result**: Any WorkflowExecution status update triggers AlertRemediation reconciliation within ~100ms.

---

#### Coordination Benefits

**For WorkflowExecution Controller**:
- ✅ **Focused**: Only handles workflow execution logic
- ✅ **No Kubernetes Operations**: Doesn't need K8s API access for remediation
- ✅ **Testable**: Unit tests only need fake K8s client for CRD
- ✅ **Decoupled**: Doesn't know about Kubernetes Executor Service

**For AlertRemediation Controller**:
- ✅ **Visibility**: Can query WorkflowExecution status anytime
- ✅ **Control**: Decides when to create KubernetesExecution
- ✅ **Data Extraction**: Copies workflow operations to KubernetesExecution spec
- ✅ **Timeout Detection**: Can detect if WorkflowExecution takes too long

**For Operations**:
- ✅ **Debuggable**: `kubectl get workflowexecution -o yaml` shows workflow state
- ✅ **Observable**: Kubernetes events show workflow progress
- ✅ **Traceable**: CRD history shows workflow execution timeline
- ✅ **Clear Sequence**: AlertRemediation shows entire orchestration flow

---

## Current State & Migration Path

### Existing Business Logic (Verified)

**Current Location**: `pkg/workflow/` (existing workflow engine code)
**Target Location**: `pkg/workflow/execution/` (for workflow execution logic)

```
Reusable Components:
pkg/workflow/
├── engine/              → Workflow engine interfaces
├── templates/           → Workflow template management
└── steps/              → Step execution logic
```

**Existing Tests** (Verified - to be extended):
- `test/unit/workflow/` → `test/unit/workflowexecution/` - Unit tests with Ginkgo/Gomega
- `test/integration/workflow/` → `test/integration/workflowexecution/` - Integration tests

### Implementation Gap Analysis

**What Exists (Verified)**:
- ✅ Workflow engine interfaces and step definitions
- ✅ Template management and versioning
- ✅ Basic step execution logic
- ✅ Workflow state management

**What's Missing (CRD V1 Requirements)**:
- ❌ WorkflowExecution CRD schema (need to create)
- ❌ WorkflowExecutionReconciler controller (need to create)
- ❌ Multi-phase orchestration (planning, validating, executing, monitoring)
- ❌ Safety validation and dry-run capabilities
- ❌ Rollback strategy implementation
- ❌ KubernetesExecution CRD creation and monitoring
- ❌ Adaptive orchestration based on runtime conditions
- ❌ Watch-based step coordination

**Estimated Migration Effort**: 10-12 days (2 weeks)
- Day 1-2: CRD schema + controller skeleton + TDD planning
- Day 3-4: Planning and validation phases
- Day 5-7: Execution phase with step orchestration
- Day 8-9: Monitoring phase and rollback logic
- Day 10-11: Integration testing with Executor Service
- Day 12: E2E testing and documentation

---

## CRD Schema Specification

**Full Schema**: See [docs/design/CRD/04_WORKFLOW_EXECUTION_CRD.md](../../design/CRD/04_WORKFLOW_EXECUTION_CRD.md)

**Note**: The examples below show the conceptual structure. The authoritative OpenAPI v3 schema is defined in `04_WORKFLOW_EXECUTION_CRD.md`.

**Location**: `api/v1/workflowexecution_types.go`

### ✅ **TYPE SAFETY COMPLIANCE**

This CRD specification uses **fully structured types** and eliminates all `map[string]interface{}` anti-patterns:

| Type | Previous (Anti-Pattern) | Current (Type-Safe) | Benefit |
|------|------------------------|---------------------|---------|
| **WorkflowStep.Parameters** | `map[string]interface{}` | Action-specific parameter types (10+ types) | Compile-time validation, self-documenting |
| **StepStatus.Result** | `map[string]interface{}` | Action-specific result types | Type-safe execution results |
| **AIRecommendations** | `map[string]interface{}` | Structured AI response | Clear HolmesGPT contract |
| **DryRunResults** | `map[string]interface{}` | Structured validation results | Detailed dry-run analysis |
| **RollbackSpec.Parameters** | `map[string]interface{}` | Rollback-specific parameters | Type-safe rollback operations |

**Related Triage**: See `WORKFLOW_EXECUTION_TYPE_SAFETY_TRIAGE.md` for detailed analysis and remediation plan.

**Total Structured Types**: 30+ types defined for comprehensive type safety

```go
package v1

import (
    corev1 "k8s.io/api/core/v1"
    metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// WorkflowExecutionSpec defines the desired state of WorkflowExecution
type WorkflowExecutionSpec struct {
    // AlertRemediationRef references the parent AlertRemediation CRD
    AlertRemediationRef corev1.ObjectReference `json:"alertRemediationRef"`

    // WorkflowDefinition contains the workflow to execute
    WorkflowDefinition WorkflowDefinition `json:"workflowDefinition"`

    // ExecutionStrategy specifies how to execute the workflow
    ExecutionStrategy ExecutionStrategy `json:"executionStrategy"`

    // AdaptiveOrchestration enables runtime optimization
    AdaptiveOrchestration AdaptiveOrchestrationConfig `json:"adaptiveOrchestration,omitempty"`
}

// WorkflowDefinition represents the workflow to execute
type WorkflowDefinition struct {
    Name             string                  `json:"name"`
    Version          string                  `json:"version"`
    Steps            []WorkflowStep          `json:"steps"`
    Dependencies     map[string][]string     `json:"dependencies,omitempty"`
    AIRecommendations *AIRecommendations     `json:"aiRecommendations,omitempty"` // ✅ TYPE SAFE - Replaces map[string]interface{} anti-pattern
}

// WorkflowStep represents a single step in the workflow
type WorkflowStep struct {
    StepNumber     int                    `json:"stepNumber"`
    Name           string                 `json:"name"`
    Action         string                 `json:"action"` // e.g., "scale-deployment", "restart-pod"
    TargetCluster  string                 `json:"targetCluster"`
    Parameters     *StepParameters        `json:"parameters"` // ✅ TYPE SAFE - Replaces map[string]interface{} anti-pattern
    CriticalStep   bool                   `json:"criticalStep"` // Failure triggers rollback
    MaxRetries     int                    `json:"maxRetries,omitempty"`
    Timeout        string                 `json:"timeout,omitempty"` // e.g., "5m"
    DependsOn      []int                  `json:"dependsOn,omitempty"` // Step numbers
    RollbackSpec   *RollbackSpec          `json:"rollbackSpec,omitempty"`
}

// RollbackSpec defines how to rollback a step
type RollbackSpec struct {
    Action     string                 `json:"action"` // e.g., "restore-previous-config"
    Parameters *RollbackParameters    `json:"parameters"` // ✅ TYPE SAFE - Replaces map[string]interface{} anti-pattern
    Timeout    string                 `json:"timeout,omitempty"`
}

// ExecutionStrategy specifies execution behavior
type ExecutionStrategy struct {
    ApprovalRequired bool   `json:"approvalRequired"`
    DryRunFirst      bool   `json:"dryRunFirst"`
    RollbackStrategy string `json:"rollbackStrategy"` // "automatic", "manual", "none"
    MaxRetries       int    `json:"maxRetries,omitempty"`
    SafetyChecks     []SafetyCheck `json:"safetyChecks,omitempty"`
}

// SafetyCheck represents a validation requirement
type SafetyCheck struct {
    Type        string `json:"type"` // "rbac", "capacity", "health", "network"
    Description string `json:"description"`
    Required    bool   `json:"required"`
}

// AdaptiveOrchestrationConfig enables runtime optimization
type AdaptiveOrchestrationConfig struct {
    OptimizationEnabled      bool `json:"optimizationEnabled"`
    LearningFromHistory      bool `json:"learningFromHistory"`
    DynamicStepAdjustment    bool `json:"dynamicStepAdjustment"`
}

// WorkflowExecutionStatus defines the observed state
type WorkflowExecutionStatus struct {
    // Phase tracks current execution stage
    Phase string `json:"phase"` // "planning", "validating", "executing", "monitoring", "completed", "failed"

    // CurrentStep tracks progress
    CurrentStep int `json:"currentStep"`
    TotalSteps  int `json:"totalSteps"`

    // ExecutionPlan generated during planning phase
    ExecutionPlan *ExecutionPlan `json:"executionPlan,omitempty"`

    // ValidationResults from validation phase
    ValidationResults *ValidationResults `json:"validationResults,omitempty"`

    // StepStatuses tracks individual step execution
    StepStatuses []StepStatus `json:"stepStatuses,omitempty"`

    // ExecutionMetrics tracks workflow performance
    ExecutionMetrics *ExecutionMetrics `json:"executionMetrics,omitempty"`

    // AdaptiveAdjustments made during execution
    AdaptiveAdjustments []AdaptiveAdjustment `json:"adaptiveAdjustments,omitempty"`

    // WorkflowResult final outcome
    WorkflowResult *WorkflowResult `json:"workflowResult,omitempty"`

    // Phase timestamps
    PlanningStartTime    *metav1.Time `json:"planningStartTime,omitempty"`
    ValidationStartTime  *metav1.Time `json:"validationStartTime,omitempty"`
    ExecutionStartTime   *metav1.Time `json:"executionStartTime,omitempty"`
    MonitoringStartTime  *metav1.Time `json:"monitoringStartTime,omitempty"`
    CompletionTime       *metav1.Time `json:"completionTime,omitempty"`

    // Error handling
    FailureReason string `json:"failureReason,omitempty"`
    RollbackReason string `json:"rollbackReason,omitempty"`

    // Conditions for status tracking
    Conditions []metav1.Condition `json:"conditions,omitempty"`
}

// ExecutionPlan generated during planning phase
type ExecutionPlan struct {
    Strategy          string `json:"strategy"` // "sequential", "parallel", "sequential-with-parallel"
    EstimatedDuration string `json:"estimatedDuration"`
    RollbackStrategy  string `json:"rollbackStrategy"`
    SafetyChecks      []SafetyCheck `json:"safetyChecks"`
}

// ValidationResults from validation phase
type ValidationResults struct {
    SafetyChecksPassed  bool                   `json:"safetyChecksPassed"`
    DryRunPerformed     bool                   `json:"dryRunPerformed"`
    DryRunResults       *DryRunResults         `json:"dryRunResults,omitempty"` // ✅ TYPE SAFE - Replaces map[string]interface{} anti-pattern
    ApprovalReceived    bool                   `json:"approvalReceived"`
    ApprovalTimestamp   *metav1.Time           `json:"approvalTimestamp,omitempty"`
    ValidationErrors    []string               `json:"validationErrors,omitempty"`
}

// StepStatus tracks individual step execution
type StepStatus struct {
    StepNumber        int                    `json:"stepNumber"`
    Action            string                 `json:"action"`
    Status            string                 `json:"status"` // "pending", "executing", "completed", "failed", "rolled_back"
    StartTime         *metav1.Time           `json:"startTime,omitempty"`
    EndTime           *metav1.Time           `json:"endTime,omitempty"`
    Result            *StepExecutionResult   `json:"result,omitempty"` // ✅ TYPE SAFE - Replaces map[string]interface{} anti-pattern
    ErrorMessage      string                 `json:"errorMessage,omitempty"`
    RetriesAttempted  int                    `json:"retriesAttempted,omitempty"`
    K8sExecutionRef   *corev1.ObjectReference `json:"k8sExecutionRef,omitempty"`
}

// ExecutionMetrics tracks workflow performance
type ExecutionMetrics struct {
    TotalDuration      string  `json:"totalDuration"`
    StepSuccessRate    float64 `json:"stepSuccessRate"`
    RollbacksPerformed int     `json:"rollbacksPerformed"`
    ResourcesAffected  int     `json:"resourcesAffected"`
}

// AdaptiveAdjustment records runtime optimization
type AdaptiveAdjustment struct {
    Timestamp   metav1.Time `json:"timestamp"`
    Adjustment  string      `json:"adjustment"` // Description of what was adjusted
    Reason      string      `json:"reason"`
}

// WorkflowResult final outcome
type WorkflowResult struct {
    Outcome             string  `json:"outcome"` // "success", "partial_success", "failed", "unknown"
    EffectivenessScore  float64 `json:"effectivenessScore"` // 0.0-1.0
    ResourceHealth      string  `json:"resourceHealth"` // "healthy", "degraded", "unhealthy"
    NewAlertsTriggered  bool    `json:"newAlertsTriggered"`
    RecommendedActions  []string `json:"recommendedActions,omitempty"` // For partial success or failure
}

// ===================================================================
// STRUCTURED TYPES - Replacing map[string]interface{} anti-patterns
// ===================================================================

// AIRecommendations contains AI-generated workflow optimization suggestions
// ✅ TYPE SAFE - Replaces map[string]interface{} anti-pattern
type AIRecommendations struct {
    // Confidence and metadata
    OverallConfidence    float64   `json:"overallConfidence"` // 0.0-1.0
    RecommendationSource string    `json:"recommendationSource"` // "holmesgpt", "history-based"
    GeneratedAt          string    `json:"generatedAt"` // ISO 8601 timestamp

    // Step-level recommendations
    StepOptimizations    []StepOptimization    `json:"stepOptimizations,omitempty"`

    // Workflow-level recommendations
    ParallelExecutionSuggestions []ParallelExecutionGroup `json:"parallelExecutionSuggestions,omitempty"`
    SafetyImprovements          []SafetyImprovement      `json:"safetyImprovements,omitempty"`

    // Historical success data
    SimilarWorkflowSuccessRate float64 `json:"similarWorkflowSuccessRate"` // 0.0-1.0
    EstimatedDuration          string  `json:"estimatedDuration"` // e.g., "5m30s"

    // Risk assessment
    RiskFactors []RiskFactor `json:"riskFactors,omitempty"`
}

type StepOptimization struct {
    StepNumber        int     `json:"stepNumber"`
    Recommendation    string  `json:"recommendation"` // Human-readable suggestion
    Confidence        float64 `json:"confidence"` // 0.0-1.0
    ImpactLevel       string  `json:"impactLevel"` // "low", "medium", "high"
    ParameterChanges  *ParameterOptimization `json:"parameterChanges,omitempty"`
}

type ParameterOptimization struct {
    SuggestedParameters map[string]string `json:"suggestedParameters"` // Key-value pairs
    Reason              string            `json:"reason"` // Why these parameters
}

type ParallelExecutionGroup struct {
    GroupName   string `json:"groupName"`
    StepNumbers []int  `json:"stepNumbers"` // Steps that can run in parallel
    Confidence  float64 `json:"confidence"` // 0.0-1.0
}

type SafetyImprovement struct {
    Description  string `json:"description"`
    StepNumber   int    `json:"stepNumber,omitempty"` // 0 = workflow-level
    SafetyCheck  string `json:"safetyCheck"` // Suggested safety check to add
    Priority     string `json:"priority"` // "low", "medium", "high"
}

type RiskFactor struct {
    Factor      string  `json:"factor"` // Description of risk
    Severity    string  `json:"severity"` // "low", "medium", "high"
    Probability float64 `json:"probability"` // 0.0-1.0
    Mitigation  string  `json:"mitigation,omitempty"` // Suggested mitigation
}

// StepParameters is a discriminated union based on Action type
// ✅ TYPE SAFE - Replaces map[string]interface{} anti-pattern
// Only ONE of these should be populated based on the Action field
type StepParameters struct {
    // Deployment actions
    ScaleDeployment   *ScaleDeploymentParams   `json:"scaleDeployment,omitempty"`
    RestartDeployment *RestartDeploymentParams `json:"restartDeployment,omitempty"`
    UpdateImage       *UpdateImageParams       `json:"updateImage,omitempty"`

    // Pod actions
    RestartPod        *RestartPodParams        `json:"restartPod,omitempty"`
    DeletePod         *DeletePodParams         `json:"deletePod,omitempty"`

    // ConfigMap/Secret actions
    UpdateConfigMap   *UpdateConfigMapParams   `json:"updateConfigMap,omitempty"`
    UpdateSecret      *UpdateSecretParams      `json:"updateSecret,omitempty"`

    // Node actions
    CordonNode        *CordonNodeParams        `json:"cordonNode,omitempty"`
    DrainNode         *DrainNodeParams         `json:"drainNode,omitempty"`

    // Custom actions (for extensibility)
    Custom            *CustomActionParams      `json:"custom,omitempty"`
}

// Deployment action parameters
type ScaleDeploymentParams struct {
    Namespace  string `json:"namespace"`
    Deployment string `json:"deployment"`
    Replicas   int32  `json:"replicas"`
}

type RestartDeploymentParams struct {
    Namespace  string `json:"namespace"`
    Deployment string `json:"deployment"`
    GracePeriod string `json:"gracePeriod,omitempty"` // e.g., "30s"
}

type UpdateImageParams struct {
    Namespace  string `json:"namespace"`
    Deployment string `json:"deployment"`
    Container  string `json:"container"`
    NewImage   string `json:"newImage"`
}

// Pod action parameters
type RestartPodParams struct {
    Namespace  string `json:"namespace"`
    PodName    string `json:"podName,omitempty"` // Empty = all pods matching selector
    Selector   string `json:"selector,omitempty"` // e.g., "app=web"
    GracePeriod string `json:"gracePeriod,omitempty"`
}

type DeletePodParams struct {
    Namespace  string `json:"namespace"`
    PodName    string `json:"podName"`
    GracePeriod string `json:"gracePeriod,omitempty"`
}

// ConfigMap/Secret parameters
type UpdateConfigMapParams struct {
    Namespace   string            `json:"namespace"`
    Name        string            `json:"name"`
    DataUpdates map[string]string `json:"dataUpdates"` // Key-value pairs to update
}

type UpdateSecretParams struct {
    Namespace   string            `json:"namespace"`
    Name        string            `json:"name"`
    DataUpdates map[string]string `json:"dataUpdates"` // Base64 encoded values
}

// Node action parameters
type CordonNodeParams struct {
    NodeName string `json:"nodeName"`
}

type DrainNodeParams struct {
    NodeName         string `json:"nodeName"`
    GracePeriod      string `json:"gracePeriod,omitempty"`
    IgnoreDaemonSets bool   `json:"ignoreDaemonSets"`
    DeleteLocalData  bool   `json:"deleteLocalData"`
}

// Custom action for extensibility
type CustomActionParams struct {
    ActionType string            `json:"actionType"` // Custom action identifier
    Config     map[string]string `json:"config"` // String-only key-value pairs
}

// RollbackParameters is a discriminated union based on rollback action
// ✅ TYPE SAFE - Replaces map[string]interface{} anti-pattern
type RollbackParameters struct {
    // Deployment rollbacks
    RestorePreviousDeployment *RestorePreviousDeploymentParams `json:"restorePreviousDeployment,omitempty"`
    ScaleToPrevious           *ScaleToPreviousParams           `json:"scaleToPrevious,omitempty"`

    // Config rollbacks
    RestorePreviousConfig     *RestorePreviousConfigParams     `json:"restorePreviousConfig,omitempty"`

    // Node rollbacks
    UncordonNode              *UncordonNodeParams              `json:"uncordonNode,omitempty"`

    // Custom rollbacks
    Custom                    *CustomRollbackParams            `json:"custom,omitempty"`
}

type RestorePreviousDeploymentParams struct {
    Namespace  string `json:"namespace"`
    Deployment string `json:"deployment"`
    Revision   int32  `json:"revision,omitempty"` // 0 = previous revision
}

type ScaleToPreviousParams struct {
    Namespace  string `json:"namespace"`
    Deployment string `json:"deployment"`
    PreviousReplicas int32 `json:"previousReplicas"` // Captured before change
}

type RestorePreviousConfigParams struct {
    Namespace string `json:"namespace"`
    Name      string `json:"name"`
    Type      string `json:"type"` // "ConfigMap" or "Secret"
    Snapshot  string `json:"snapshot"` // Reference to saved snapshot
}

type UncordonNodeParams struct {
    NodeName string `json:"nodeName"`
}

type CustomRollbackParams struct {
    ActionType string            `json:"actionType"`
    Config     map[string]string `json:"config"`
}

// DryRunResults contains structured dry-run execution results
// ✅ TYPE SAFE - Replaces map[string]interface{} anti-pattern
type DryRunResults struct {
    OverallSuccess    bool                    `json:"overallSuccess"`
    ExecutionTime     string                  `json:"executionTime"` // Duration
    StepsSimulated    int                     `json:"stepsSimulated"`
    StepResults       []DryRunStepResult      `json:"stepResults"`
    ResourceChanges   []ResourceChange        `json:"resourceChanges,omitempty"`
    PotentialIssues   []PotentialIssue        `json:"potentialIssues,omitempty"`
}

type DryRunStepResult struct {
    StepNumber       int    `json:"stepNumber"`
    Action           string `json:"action"`
    WouldSucceed     bool   `json:"wouldSucceed"`
    SimulatedDuration string `json:"simulatedDuration"`
    ValidationErrors []string `json:"validationErrors,omitempty"`
}

type ResourceChange struct {
    ResourceType string            `json:"resourceType"` // "Deployment", "Pod", etc.
    ResourceName string            `json:"resourceName"`
    Namespace    string            `json:"namespace"`
    ChangeType   string            `json:"changeType"` // "scale", "update", "delete"
    BeforeState  map[string]string `json:"beforeState"` // String key-value pairs only
    AfterState   map[string]string `json:"afterState"`
}

type PotentialIssue struct {
    Severity    string `json:"severity"` // "low", "medium", "high"
    Description string `json:"description"`
    StepNumber  int    `json:"stepNumber,omitempty"`
    Recommendation string `json:"recommendation,omitempty"`
}

// StepExecutionResult contains structured execution results
// ✅ TYPE SAFE - Replaces map[string]interface{} anti-pattern
type StepExecutionResult struct {
    Success          bool                   `json:"success"`
    ExecutionTime    string                 `json:"executionTime"` // Duration

    // Action-specific results (discriminated union - only one populated)
    ScaleResult      *ScaleExecutionResult      `json:"scaleResult,omitempty"`
    RestartResult    *RestartExecutionResult    `json:"restartResult,omitempty"`
    UpdateResult     *UpdateExecutionResult     `json:"updateResult,omitempty"`
    CustomResult     *CustomExecutionResult     `json:"customResult,omitempty"`

    // Common result metadata
    ResourcesAffected []AffectedResource         `json:"resourcesAffected,omitempty"`
    Warnings          []string                   `json:"warnings,omitempty"`
}

type ScaleExecutionResult struct {
    PreviousReplicas int32 `json:"previousReplicas"`
    NewReplicas      int32 `json:"newReplicas"`
    ScaledNamespace  string `json:"scaledNamespace"`
    ScaledDeployment string `json:"scaledDeployment"`
}

type RestartExecutionResult struct {
    PodsRestarted    int      `json:"podsRestarted"`
    RestartedPods    []string `json:"restartedPods"` // Pod names
    Namespace        string   `json:"namespace"`
}

type UpdateExecutionResult struct {
    ResourceType   string `json:"resourceType"` // "Deployment", "ConfigMap", etc.
    ResourceName   string `json:"resourceName"`
    Namespace      string `json:"namespace"`
    UpdateType     string `json:"updateType"` // "image", "config", "spec"
    PreviousValue  string `json:"previousValue,omitempty"`
    NewValue       string `json:"newValue,omitempty"`
}

type CustomExecutionResult struct {
    ActionType string            `json:"actionType"`
    Output     map[string]string `json:"output"` // String key-value pairs only
}

type AffectedResource struct {
    ResourceType string `json:"resourceType"`
    Name         string `json:"name"`
    Namespace    string `json:"namespace"`
    ChangeType   string `json:"changeType"` // "modified", "restarted", "scaled"
}

//+kubebuilder:object:root=true
//+kubebuilder:subresource:status

// WorkflowExecution is the Schema for the workflowexecutions API
type WorkflowExecution struct {
    metav1.TypeMeta   `json:",inline"`
    metav1.ObjectMeta `json:"metadata,omitempty"`

    Spec   WorkflowExecutionSpec   `json:"spec,omitempty"`
    Status WorkflowExecutionStatus `json:"status,omitempty"`
}

//+kubebuilder:object:root=true

// WorkflowExecutionList contains a list of WorkflowExecution
type WorkflowExecutionList struct {
    metav1.TypeMeta `json:",inline"`
    metav1.ListMeta `json:"metadata,omitempty"`
    Items           []WorkflowExecution `json:"items"`
}

func init() {
    SchemeBuilder.Register(&WorkflowExecution{}, &WorkflowExecutionList{})
}
```

---

## Controller Implementation

**Location**: `internal/controller/workflowexecution_controller.go`

### Controller Configuration

**Critical Patterns from [MULTI_CRD_RECONCILIATION_ARCHITECTURE.md](../../architecture/MULTI_CRD_RECONCILIATION_ARCHITECTURE.md)**:
1. **Owner References**: WorkflowExecution CRD owned by AlertRemediation for cascade deletion
2. **Finalizers**: Cleanup coordination before deletion
3. **Watch Optimization**: Watches KubernetesExecution CRDs for step completion
4. **Timeout Handling**: Phase-level timeout detection and escalation
5. **Event Emission**: Operational visibility through Kubernetes events

```go
package controller

import (
    "context"
    "fmt"
    "time"

    corev1 "k8s.io/api/core/v1"
    apierrors "k8s.io/apimachinery/pkg/api/errors"
    apimeta "k8s.io/apimachinery/pkg/api/meta"
    metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
    "k8s.io/apimachinery/pkg/runtime"
    "k8s.io/client-go/tools/record"
    ctrl "sigs.k8s.io/controller-runtime"
    "sigs.k8s.io/controller-runtime/pkg/client"
    "sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
    "sigs.k8s.io/controller-runtime/pkg/log"

    remediationv1 "github.com/jordigilh/kubernaut/api/remediation/v1"
    executorv1 "github.com/jordigilh/kubernaut/api/executor/v1"
    workflowexecutionv1 "github.com/jordigilh/kubernaut/api/workflow/v1"
)

const (
    // Finalizer for cleanup coordination
    workflowExecutionFinalizer = "workflowexecution.kubernaut.io/finalizer"

    // Timeout configuration
    defaultPlanningTimeout   = 30 * time.Second
    defaultValidationTimeout = 5 * time.Minute
    defaultStepTimeout       = 5 * time.Minute
    defaultMonitoringTimeout = 10 * time.Minute
)

// WorkflowExecutionReconciler reconciles a WorkflowExecution object
type WorkflowExecutionReconciler struct {
    client.Client
    Scheme         *runtime.Scheme
    Recorder       record.EventRecorder  // Event emission for visibility
    WorkflowEngine WorkflowEngine        // Workflow planning and orchestration
    Validator      WorkflowValidator     // Safety validation
    Monitor        WorkflowMonitor       // Execution monitoring
}

//+kubebuilder:rbac:groups=workflowexecution.kubernaut.io,resources=workflowexecutions,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=workflowexecution.kubernaut.io,resources=workflowexecutions/status,verbs=get;update;patch
//+kubebuilder:rbac:groups=workflowexecution.kubernaut.io,resources=workflowexecutions/finalizers,verbs=update
//+kubebuilder:rbac:groups=kubernetesexecution.kubernaut.io,resources=kubernetesexecutions,verbs=create;get;list;watch
//+kubebuilder:rbac:groups=kubernaut.io,resources=alertremediations,verbs=get;list;watch
//+kubebuilder:rbac:groups=kubernaut.io,resources=alertremediations/status,verbs=get;update;patch
//+kubebuilder:rbac:groups="",resources=events,verbs=create;patch

func (r *WorkflowExecutionReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
    log := log.FromContext(ctx)

    // Fetch WorkflowExecution CRD
    var wf workflowexecutionv1.WorkflowExecution
    if err := r.Get(ctx, req.NamespacedName, &wf); err != nil {
        return ctrl.Result{}, client.IgnoreNotFound(err)
    }

    // Handle deletion with finalizer
    if !wf.DeletionTimestamp.IsZero() {
        return r.reconcileDelete(ctx, &wf)
    }

    // Add finalizer if not present
    if !controllerutil.ContainsFinalizer(&wf, workflowExecutionFinalizer) {
        controllerutil.AddFinalizer(&wf, workflowExecutionFinalizer)
        if err := r.Update(ctx, &wf); err != nil {
            return ctrl.Result{}, err
        }
    }

    // Set owner reference to AlertRemediation (for cascade deletion)
    if err := r.ensureOwnerReference(ctx, &wf); err != nil {
        log.Error(err, "Failed to set owner reference")
        r.Recorder.Event(&wf, "Warning", "OwnerReferenceFailed",
            fmt.Sprintf("Failed to set owner reference: %v", err))
        return ctrl.Result{RequeueAfter: time.Second * 30}, err
    }

    // Check for phase timeout
    if r.isPhaseTimedOut(&wf) {
        return r.handlePhaseTimeout(ctx, &wf)
    }

    // Initialize phase if empty
    if wf.Status.Phase == "" {
        wf.Status.Phase = "planning"
        wf.Status.PlanningStartTime = &metav1.Time{Time: time.Now()}
        if err := r.Status().Update(ctx, &wf); err != nil {
            return ctrl.Result{}, err
        }
    }

    // Reconcile based on current phase
    var result ctrl.Result
    var err error

    switch wf.Status.Phase {
    case "planning":
        result, err = r.reconcilePlanning(ctx, &wf)
    case "validating":
        result, err = r.reconcileValidating(ctx, &wf)
    case "executing":
        result, err = r.reconcileExecuting(ctx, &wf)
    case "monitoring":
        result, err = r.reconcileMonitoring(ctx, &wf)
    case "rolling_back":
        result, err = r.reconcileRollback(ctx, &wf)
    case "completed", "failed":
        // Terminal states - use optimized requeue strategy
        return r.determineRequeueStrategy(&wf), nil
    default:
        log.Error(nil, "Unknown phase", "phase", wf.Status.Phase)
        r.Recorder.Event(&wf, "Warning", "UnknownPhase",
            fmt.Sprintf("Unknown phase: %s", wf.Status.Phase))
        return ctrl.Result{RequeueAfter: time.Second * 30}, nil
    }

    return result, err
}

// Additional controller methods would be implemented here...
// reconcilePlanning, reconcileValidating, reconcileExecuting, reconcileMonitoring, etc.

func (r *WorkflowExecutionReconciler) SetupWithManager(mgr ctrl.Manager) error {
    return ctrl.NewControllerManagedBy(mgr).
        For(&workflowexecutionv1.WorkflowExecution{}).
        Owns(&executorv1.KubernetesExecution{}).  // Watch KubernetesExecution CRDs
        Complete(r)
}
```

---

## Finalizer Implementation

### Finalizer Name

Following Kubernetes finalizer naming convention:

```go
const workflowExecutionFinalizer = "workflowexecution.kubernaut.io/workflowexecution-cleanup"
```

**Naming Pattern**: `{domain}.kubernaut.io/{resource}-cleanup`

**Why This Pattern**:
- **Domain-Scoped**: `workflowexecution.kubernaut.io` prevents conflicts with other services
- **Resource-Specific**: `workflowexecution-cleanup` clearly indicates what's being cleaned up
- **Kubernetes Convention**: Follows standard finalizer naming (domain/action format)

---

### Complete Reconciliation Loop with Finalizer

```go
package controller

import (
    "context"
    "fmt"

    workflowexecutionv1 "github.com/jordigilh/kubernaut/api/workflow/execution/v1"

    "github.com/go-logr/logr"
    "k8s.io/apimachinery/pkg/runtime"
    "k8s.io/client-go/tools/record"
    ctrl "sigs.k8s.io/controller-runtime"
    "sigs.k8s.io/controller-runtime/pkg/client"
    "sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
)

const workflowExecutionFinalizer = "workflowexecution.kubernaut.io/workflowexecution-cleanup"

type WorkflowExecutionReconciler struct {
    client.Client
    Scheme            *runtime.Scheme
    Log               logr.Logger
    Recorder          record.EventRecorder
    WorkflowBuilder   WorkflowBuilder
    StorageClient     StorageClient
}

func (r *WorkflowExecutionReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
    var we workflowexecutionv1.WorkflowExecution
    if err := r.Get(ctx, req.NamespacedName, &we); err != nil {
        return ctrl.Result{}, client.IgnoreNotFound(err)
    }

    // ========================================
    // DELETION HANDLING WITH FINALIZER
    // ========================================
    if !we.ObjectMeta.DeletionTimestamp.IsZero() {
        if controllerutil.ContainsFinalizer(&we, workflowExecutionFinalizer) {
            // Perform cleanup before deletion
            if err := r.cleanupWorkflowExecution(ctx, &we); err != nil {
                r.Log.Error(err, "Failed to cleanup WorkflowExecution resources",
                    "name", we.Name,
                    "namespace", we.Namespace,
                )
                return ctrl.Result{}, err
            }

            // Remove finalizer to allow deletion
            controllerutil.RemoveFinalizer(&we, workflowExecutionFinalizer)
            if err := r.Update(ctx, &we); err != nil {
                return ctrl.Result{}, err
            }
        }
        return ctrl.Result{}, nil
    }

    // ========================================
    // ADD FINALIZER IF NOT PRESENT
    // ========================================
    if !controllerutil.ContainsFinalizer(&we, workflowExecutionFinalizer) {
        controllerutil.AddFinalizer(&we, workflowExecutionFinalizer)
        if err := r.Update(ctx, &we); err != nil {
            return ctrl.Result{}, err
        }
    }

    // ========================================
    // NORMAL RECONCILIATION LOGIC
    // ========================================

    // Skip if already completed or failed
    if we.Status.Phase == "completed" || we.Status.Phase == "failed" {
        return ctrl.Result{}, nil
    }

    // Execute workflow building and validation...
    // (existing reconciliation logic from previous section)

    return ctrl.Result{}, nil
}
```

---

### Cleanup Logic

**What Gets Cleaned Up** (Leaf Controller Pattern):

```go
package controller

import (
    "context"
    "fmt"
    "time"

    workflowexecutionv1 "github.com/jordigilh/kubernaut/api/workflow/execution/v1"
)

func (r *WorkflowExecutionReconciler) cleanupWorkflowExecution(
    ctx context.Context,
    we *workflowexecutionv1.WorkflowExecution,
) error {
    r.Log.Info("Cleaning up WorkflowExecution resources",
        "name", we.Name,
        "namespace", we.Namespace,
        "phase", we.Status.Phase,
    )

    // 1. Record final audit to database
    if err := r.recordFinalAudit(ctx, we); err != nil {
        r.Log.Error(err, "Failed to record final audit", "name", we.Name)
        // Don't block deletion on audit failure
        // Audit is best-effort during cleanup
    }

    // 2. Emit deletion event
    r.Recorder.Event(we, "Normal", "WorkflowExecutionDeleted",
        fmt.Sprintf("WorkflowExecution cleanup completed (phase: %s)", we.Status.Phase))

    r.Log.Info("WorkflowExecution cleanup completed successfully",
        "name", we.Name,
        "namespace", we.Namespace,
    )

    return nil
}

func (r *WorkflowExecutionReconciler) recordFinalAudit(
    ctx context.Context,
    we *workflowexecutionv1.WorkflowExecution,
) error {
    auditRecord := &AuditRecord{
        AlertFingerprint: we.Spec.AlertContext.Fingerprint,
        ServiceType:      "WorkflowExecution",
        CRDName:          we.Name,
        Namespace:        we.Namespace,
        Phase:            we.Status.Phase,
        CreatedAt:        we.CreationTimestamp.Time,
        DeletedAt:        we.DeletionTimestamp.Time,
        WorkflowSteps:    len(we.Status.Workflow.Steps),
        ValidationStatus: we.Status.ValidationStatus,
    }

    return r.StorageClient.RecordAudit(ctx, auditRecord)
}
```

**Cleanup Philosophy for WorkflowExecution** (Leaf Controller):
- ✅ **Record final audit**: Capture workflow definition (best-effort)
- ✅ **Emit deletion event**: Operational visibility
- ❌ **No external cleanup needed**: WorkflowExecution is a leaf CRD (owns nothing)
- ❌ **No child CRD cleanup**: WorkflowExecution doesn't create child CRDs
- ✅ **Non-blocking**: Audit failures don't block deletion (best-effort)

**Note**: WorkflowExecution does NOT create KubernetesExecution CRDs. AlertRemediation is responsible for creating KubernetesExecution based on the workflow generated by WorkflowExecution.

---

### Finalizer Testing

**Unit Test Pattern**:

```go
package controller_test

import (
    "context"
    "fmt"

    workflowexecutionv1 "github.com/jordigilh/kubernaut/api/workflow/execution/v1"
    "github.com/jordigilh/kubernaut/pkg/workflow/execution/controller"

    . "github.com/onsi/ginkgo/v2"
    . "github.com/onsi/gomega"
    metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
    "k8s.io/apimachinery/pkg/runtime"
    "k8s.io/client-go/kubernetes/scheme"
    ctrl "sigs.k8s.io/controller-runtime"
    "sigs.k8s.io/controller-runtime/pkg/client"
    "sigs.k8s.io/controller-runtime/pkg/client/fake"
    "sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
)

var _ = Describe("WorkflowExecution Finalizer", func() {
    var (
        ctx       context.Context
        k8sClient client.Client
        reconciler *controller.WorkflowExecutionReconciler
    )

    BeforeEach(func() {
        ctx = context.Background()
        k8sClient = fake.NewClientBuilder().
            WithScheme(scheme.Scheme).
            Build()

        reconciler = &controller.WorkflowExecutionReconciler{
            Client:        k8sClient,
            StorageClient: &mockStorageClient{},
        }
    })

    Context("when WorkflowExecution is created", func() {
        It("should add finalizer on first reconcile", func() {
            we := &workflowexecutionv1.WorkflowExecution{
                ObjectMeta: metav1.ObjectMeta{
                    Name:      "test-workflow",
                    Namespace: "default",
                },
                Spec: workflowexecutionv1.WorkflowExecutionSpec{
                    Recommendations: []workflowexecutionv1.RemediationRecommendation{
                        {Action: "restart_pod", Confidence: 0.95},
                    },
                },
            }
            Expect(k8sClient.Create(ctx, we)).To(Succeed())

            // First reconcile should add finalizer
            _, err := reconciler.Reconcile(ctx, ctrl.Request{
                NamespacedName: client.ObjectKeyFromObject(we),
            })
            Expect(err).ToNot(HaveOccurred())

            // Verify finalizer added
            Expect(k8sClient.Get(ctx, client.ObjectKeyFromObject(we), we)).To(Succeed())
            Expect(controllerutil.ContainsFinalizer(we, workflowExecutionFinalizer)).To(BeTrue())
        })
    })

    Context("when WorkflowExecution is deleted", func() {
        It("should execute cleanup and remove finalizer", func() {
            we := &workflowexecutionv1.WorkflowExecution{
                ObjectMeta: metav1.ObjectMeta{
                    Name:       "test-workflow",
                    Namespace:  "default",
                    Finalizers: []string{workflowExecutionFinalizer},
                },
                Status: workflowexecutionv1.WorkflowExecutionStatus{
                    Phase: "completed",
                },
            }
            Expect(k8sClient.Create(ctx, we)).To(Succeed())

            // Delete WorkflowExecution
            Expect(k8sClient.Delete(ctx, we)).To(Succeed())

            // Reconcile should execute cleanup
            _, err := reconciler.Reconcile(ctx, ctrl.Request{
                NamespacedName: client.ObjectKeyFromObject(we),
            })
            Expect(err).ToNot(HaveOccurred())

            // Verify finalizer removed (CRD will be deleted by Kubernetes)
            err = k8sClient.Get(ctx, client.ObjectKeyFromObject(we), we)
            Expect(err).To(HaveOccurred())
            Expect(client.IgnoreNotFound(err)).ToNot(HaveOccurred())
        })

        It("should not block deletion if audit fails", func() {
            // Mock storage client to return error
            reconciler.StorageClient = &mockStorageClient{
                recordAuditError: fmt.Errorf("database unavailable"),
            }

            we := &workflowexecutionv1.WorkflowExecution{
                ObjectMeta: metav1.ObjectMeta{
                    Name:       "test-workflow",
                    Namespace:  "default",
                    Finalizers: []string{workflowExecutionFinalizer},
                },
            }
            Expect(k8sClient.Create(ctx, we)).To(Succeed())
            Expect(k8sClient.Delete(ctx, we)).To(Succeed())

            // Cleanup should succeed even if audit fails
            _, err := reconciler.Reconcile(ctx, ctrl.Request{
                NamespacedName: client.ObjectKeyFromObject(we),
            })
            Expect(err).ToNot(HaveOccurred())

            // Verify finalizer removed despite audit failure
            err = k8sClient.Get(ctx, client.ObjectKeyFromObject(we), we)
            Expect(client.IgnoreNotFound(err)).ToNot(HaveOccurred())
        })
    })
})
```

---

## CRD Lifecycle Management

### Creation Lifecycle

**Created By**: AlertRemediation controller (centralized orchestration)

**Creation Trigger**: AIAnalysis completion (with approved recommendations)

**Sequence**:
```
AIAnalysis.status.phase = "completed"
    ↓ (watch trigger <100ms)
AlertRemediation Controller reconciles
    ↓
AlertRemediation extracts approved recommendations
    ↓
AlertRemediation Controller creates WorkflowExecution CRD
    ↓ (with owner reference)
WorkflowExecution Controller reconciles (this controller)
    ↓
WorkflowExecution builds multi-step workflow
    ↓
WorkflowExecution validates workflow steps
    ↓
WorkflowExecution.status.phase = "completed"
    ↓ (watch trigger <100ms)
AlertRemediation Controller detects completion
    ↓
AlertRemediation Controller creates KubernetesExecution CRD
```

**Owner Reference Set at Creation**:
```go
package controller

import (
    "context"
    "fmt"

    remediationv1 "github.com/jordigilh/kubernaut/api/alertremediation/v1"
    workflowexecutionv1 "github.com/jordigilh/kubernaut/api/workflow/execution/v1"

    metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
    "sigs.k8s.io/controller-runtime/pkg/client"
)

// In AlertRemediationReconciler
func (r *AlertRemediationReconciler) createWorkflowExecution(
    ctx context.Context,
    remediation *remediationv1.AlertRemediation,
    recommendations []workflowexecutionv1.RemediationRecommendation,
) error {
    workflowExecution := &workflowexecutionv1.WorkflowExecution{
        ObjectMeta: metav1.ObjectMeta{
            Name:      fmt.Sprintf("%s-workflow", remediation.Name),
            Namespace: remediation.Namespace,
            OwnerReferences: []metav1.OwnerReference{
                *metav1.NewControllerRef(remediation,
                    remediationv1.GroupVersion.WithKind("AlertRemediation")),
            },
        },
        Spec: workflowexecutionv1.WorkflowExecutionSpec{
            AlertRemediationRef: workflowexecutionv1.AlertRemediationReference{
                Name:      remediation.Name,
                Namespace: remediation.Namespace,
            },
            Recommendations: recommendations,
            AlertContext: workflowexecutionv1.AlertContext{
                Fingerprint: remediation.Spec.AlertFingerprint,
                Environment: remediation.Status.Environment,
            },
        },
    }

    return r.Create(ctx, workflowExecution)
}
```

**Result**: WorkflowExecution is owned by AlertRemediation (cascade deletion applies)

---

### Update Lifecycle

**Status Updates by WorkflowExecution Controller**:

```go
package controller

import (
    "context"
    "time"

    workflowexecutionv1 "github.com/jordigilh/kubernaut/api/workflow/execution/v1"

    metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func (r *WorkflowExecutionReconciler) updateStatusCompleted(
    ctx context.Context,
    we *workflowexecutionv1.WorkflowExecution,
    workflow workflowexecutionv1.Workflow,
    validationStatus string,
) error {
    // Controller updates own status
    we.Status.Phase = "completed"
    we.Status.CompletionTime = &metav1.Time{Time: time.Now()}
    we.Status.Workflow = workflow
    we.Status.ValidationStatus = validationStatus

    return r.Status().Update(ctx, we)
}
```

**Watch Triggers AlertRemediation Reconciliation**:

```
WorkflowExecution.status.phase = "completed"
    ↓ (watch event)
AlertRemediation watch triggers
    ↓ (<100ms latency)
AlertRemediation Controller reconciles
    ↓
AlertRemediation extracts workflow definition
    ↓
AlertRemediation creates KubernetesExecution CRD
```

**No Self-Updates After Completion**:
- WorkflowExecution does NOT update itself after `phase = "completed"`
- WorkflowExecution does NOT create other CRDs (leaf controller)
- WorkflowExecution does NOT watch other CRDs

**Critical Anti-Pattern to Avoid**:
- ❌ WorkflowExecution does NOT create KubernetesExecution CRDs
- ✅ AlertRemediation creates KubernetesExecution based on workflow output

---

### Deletion Lifecycle

**Trigger**: AlertRemediation deletion (cascade)

**Cascade Deletion Sequence**:
```
User/System deletes AlertRemediation
    ↓
Kubernetes garbage collector detects owner reference
    ↓ (parallel deletion of all owned CRDs)
WorkflowExecution.deletionTimestamp set
    ↓
WorkflowExecution Controller reconciles (detects deletion)
    ↓
Finalizer cleanup executes:
  - Record final workflow audit
  - Emit deletion event
    ↓
Finalizer removed
    ↓
Kubernetes deletes WorkflowExecution CRD
```

**Parallel Deletion**: All service CRDs (AlertProcessing, AIAnalysis, WorkflowExecution, KubernetesExecution) deleted in parallel when AlertRemediation is deleted.

**Retention**:
- **WorkflowExecution**: No independent retention (deleted with parent)
- **AlertRemediation**: 24-hour retention (parent CRD manages retention)
- **Audit Data**: 90-day retention in PostgreSQL (persisted before deletion)

---

### Lifecycle Events

**Kubernetes Events Emitted**:

```go
package controller

import (
    "fmt"
    "time"

    workflowexecutionv1 "github.com/jordigilh/kubernaut/api/workflow/execution/v1"

    "k8s.io/client-go/tools/record"
)

func (r *WorkflowExecutionReconciler) emitLifecycleEvents(
    we *workflowexecutionv1.WorkflowExecution,
    oldPhase string,
    duration time.Duration,
) {
    // Creation event
    r.Recorder.Event(we, "Normal", "WorkflowExecutionCreated",
        fmt.Sprintf("Workflow building started for %d recommendations", len(we.Spec.Recommendations)))

    // Phase transition events
    r.Recorder.Event(we, "Normal", "PhaseTransition",
        fmt.Sprintf("Phase: %s → %s", oldPhase, we.Status.Phase))

    // Workflow generated
    if we.Status.Phase == "completed" && we.Status.Workflow.Steps != nil {
        r.Recorder.Event(we, "Normal", "WorkflowGenerated",
            fmt.Sprintf("Generated workflow with %d steps", len(we.Status.Workflow.Steps)))
    }

    // Validation events
    if we.Status.ValidationStatus == "passed" {
        r.Recorder.Event(we, "Normal", "WorkflowValidated",
            "Workflow validation passed")
    } else if we.Status.ValidationStatus == "failed" {
        r.Recorder.Event(we, "Warning", "WorkflowValidationFailed",
            "Workflow validation failed - review workflow definition")
    }

    // Completion event
    r.Recorder.Event(we, "Normal", "WorkflowExecutionCompleted",
        fmt.Sprintf("Workflow building completed in %s", duration))

    // Deletion event (in cleanup function)
    r.Recorder.Event(we, "Normal", "WorkflowExecutionDeleted",
        fmt.Sprintf("WorkflowExecution cleanup completed (phase: %s)", we.Status.Phase))
}
```

**Event Visibility**:
```bash
kubectl describe workflowexecution <name>
# Shows all events in chronological order

kubectl get events --field-selector involvedObject.name=<name>
# Filter events for specific WorkflowExecution
```

---

### Lifecycle Monitoring

**Prometheus Metrics**:

```promql
# CRD creation rate
rate(workflowexecution_created_total[5m])

# CRD completion time (end-to-end)
histogram_quantile(0.95, workflowexecution_lifecycle_duration_seconds)

# Active WorkflowExecution CRDs
workflowexecution_active_total

# CRD deletion rate
rate(workflowexecution_deleted_total[5m])

# Workflow validation failure rate
rate(workflowexecution_validation_failures_total[5m])

# Workflow step count distribution
histogram_quantile(0.95, workflowexecution_workflow_steps_total)
```

**Grafana Dashboard**:
```yaml
panels:
  - title: "WorkflowExecution Lifecycle"
    targets:
      - expr: workflowexecution_active_total
        legendFormat: "Active CRDs"
      - expr: rate(workflowexecution_created_total[5m])
        legendFormat: "Creation Rate"
      - expr: rate(workflowexecution_deleted_total[5m])
        legendFormat: "Deletion Rate"

  - title: "Workflow Building Latency (P95)"
    targets:
      - expr: histogram_quantile(0.95, workflowexecution_lifecycle_duration_seconds)
        legendFormat: "P95 Duration"

  - title: "Workflow Validation Status"
    targets:
      - expr: sum(workflowexecution_validation_status{status="passed"})
        legendFormat: "Passed"
      - expr: sum(workflowexecution_validation_status{status="failed"})
        legendFormat: "Failed"
```

**Alert Rules**:

```yaml
groups:
- name: workflowexecution-lifecycle
  rules:
  - alert: WorkflowExecutionStuckInPhase
    expr: time() - workflowexecution_phase_start_timestamp > 300
    for: 5m
    labels:
      severity: warning
    annotations:
      summary: "WorkflowExecution stuck in phase for >5 minutes"
      description: "WorkflowExecution {{ $labels.name }} has been in phase {{ $labels.phase }} for over 5 minutes"

  - alert: WorkflowExecutionHighValidationFailureRate
    expr: rate(workflowexecution_validation_failures_total[5m]) > 0.1
    for: 5m
    labels:
      severity: warning
    annotations:
      summary: "High workflow validation failure rate"
      description: "Workflow validation failing for >10% of executions"

  - alert: WorkflowExecutionHighDeletionRate
    expr: rate(workflowexecution_deleted_total[5m]) > rate(workflowexecution_created_total[5m]) * 1.5
    for: 5m
    labels:
      severity: warning
    annotations:
      summary: "WorkflowExecution deletion rate exceeds creation rate"
      description: "More WorkflowExecution CRDs being deleted than created (possible cascade deletion issue)"

  - alert: WorkflowExecutionComplexWorkflows
    expr: histogram_quantile(0.95, workflowexecution_workflow_steps_total) > 10
    for: 10m
    labels:
      severity: info
    annotations:
      summary: "P95 workflow complexity exceeds 10 steps"
      description: "Generated workflows are becoming complex - review recommendation quality"
```

---

## Prometheus Metrics Implementation

### Metrics Server Setup

**Two-Port Architecture** for security separation (same as other services):

```go
// Port 8080: Health and Readiness (NO AUTH)
// Port 9090: Prometheus Metrics (WITH AUTH FILTER)
// See 01-alert-processor.md for complete metrics server setup example
```

### Service-Specific Metrics Registration

```go
// Package execution provides workflow execution orchestration business logic
// Location: pkg/workflow/execution/
package execution

import (
    "time"
    "github.com/prometheus/client_golang/prometheus"
    "github.com/prometheus/client_golang/prometheus/promauto"
)

var (
    // Counter: Total workflows processed
    WorkflowsProcessedTotal = promauto.NewCounterVec(prometheus.CounterOpts{
        Name: "kubernaut_workflowexecution_executions_total",
        Help: "Total number of workflow executions",
    }, []string{"workflow_name", "outcome"}) // outcome: success, failed, partial

    // Histogram: Workflow execution duration by phase
    WorkflowPhaseDuration = promauto.NewHistogramVec(prometheus.HistogramOpts{
        Name:    "kubernaut_workflowexecution_phase_duration_seconds",
        Help:    "Duration of workflow phases",
        Buckets: []float64{1, 5, 10, 30, 60, 120, 300, 600},
    }, []string{"phase"}) // planning, validating, executing, monitoring

    // Gauge: Current active workflow executions
    ActiveWorkflowsGauge = promauto.NewGauge(prometheus.GaugeOpts{
        Name: "kubernaut_workflowexecution_active_executions",
        Help: "Number of workflows currently executing",
    })

    // Counter: Workflow steps created
    WorkflowStepsCreatedTotal = promauto.NewCounterVec(prometheus.CounterOpts{
        Name: "kubernaut_workflowexecution_steps_created_total",
        Help: "Total workflow steps created",
    }, []string{"workflow_name", "action"})

    // Counter: Workflow steps completed
    WorkflowStepsCompletedTotal = promauto.NewCounterVec(prometheus.CounterOpts{
        Name: "kubernaut_workflowexecution_steps_completed_total",
        Help: "Total workflow steps completed",
    }, []string{"workflow_name", "action", "status"}) // status: success, failed

    // Counter: Rollbacks performed
    RollbacksPerformedTotal = promauto.NewCounterVec(prometheus.CounterOpts{
        Name: "kubernaut_workflowexecution_rollbacks_total",
        Help: "Total rollbacks performed",
    }, []string{"workflow_name", "reason"}) // reason: step_failure, validation_failure, manual

    // Histogram: Workflow effectiveness score
    WorkflowEffectivenessScore = promauto.NewHistogram(prometheus.HistogramOpts{
        Name:    "kubernaut_workflowexecution_effectiveness_score",
        Help:    "Workflow effectiveness score (0.0-1.0)",
        Buckets: []float64{0.5, 0.6, 0.7, 0.8, 0.9, 0.95, 0.99, 1.0},
    })

    // Counter: Safety validation results
    SafetyValidationTotal = promauto.NewCounterVec(prometheus.CounterOpts{
        Name: "kubernaut_workflowexecution_safety_validation_total",
        Help: "Total safety validations performed",
    }, []string{"check_type", "result"}) // result: pass, fail

    // Counter: Adaptive adjustments made
    AdaptiveAdjustmentsTotal = promauto.NewCounterVec(prometheus.CounterOpts{
        Name: "kubernaut_workflowexecution_adaptive_adjustments_total",
        Help: "Total adaptive adjustments made during execution",
    }, []string{"adjustment_type"})
)
```

---

## Testing Strategy

**Testing Framework Reference**: [.cursor/rules/03-testing-strategy.mdc](../../../.cursor/rules/03-testing-strategy.mdc)

### Pyramid Testing Approach

Following Kubernaut's defense-in-depth testing strategy:

- **Unit Tests (70%+)**: Comprehensive controller logic with mocked external dependencies
- **Integration Tests (20%)**: Cross-component CRD interactions with real K8s
- **E2E Tests (10%)**: Complete workflow execution scenarios

### Unit Tests (Primary Coverage Layer)

**Test Directory**: [test/unit/](../../../test/unit/)
**Service Tests**: Create `test/unit/workflowexecution/controller_test.go`
**Coverage Target**: 70%+ of business requirements (BR-WF-001 to BR-WF-165)
**Confidence**: 85-90%

**Test File Structure**:
```
test/unit/
├── workflowexecution/            # Matches cmd/workflowexecution/
│   ├── controller_test.go        # Main controller reconciliation tests
│   ├── planning_test.go          # Workflow planning phase tests
│   ├── validation_test.go        # Safety validation tests
│   ├── execution_test.go         # Step execution orchestration tests
│   ├── monitoring_test.go        # Effectiveness monitoring tests
│   ├── rollback_test.go          # Rollback strategy tests
│   └── suite_test.go             # Ginkgo test suite setup
```

**Example Unit Test**:
```go
package workflowexecution

import (
    . "github.com/onsi/ginkgo/v2"
    . "github.com/onsi/gomega"
    "context"
    "time"

    remediationv1 "github.com/jordigilh/kubernaut/api/remediation/v1"
    executorv1 "github.com/jordigilh/kubernaut/api/executor/v1"
    workflowexecutionv1 "github.com/jordigilh/kubernaut/api/workflow/v1"
    "github.com/jordigilh/kubernaut/internal/controller"
    "github.com/jordigilh/kubernaut/pkg/testutil"

    corev1 "k8s.io/api/core/v1"
    metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
    "k8s.io/apimachinery/pkg/runtime"
    "sigs.k8s.io/controller-runtime/pkg/client"
    "sigs.k8s.io/controller-runtime/pkg/client/fake"
)

var _ = Describe("BR-WF-001: Workflow Execution Controller", func() {
    var (
        fakeK8sClient      client.Client
        scheme             *runtime.Scheme
        reconciler         *controller.WorkflowExecutionReconciler
        ctx                context.Context
    )

    BeforeEach(func() {
        ctx = context.Background()

        scheme = runtime.NewScheme()
        _ = corev1.AddToScheme(scheme)
        _ = remediationv1.AddToScheme(scheme)
        _ = executorv1.AddToScheme(scheme)
        _ = workflowexecutionv1.AddToScheme(scheme)

        fakeK8sClient = fake.NewClientBuilder().
            WithScheme(scheme).
            Build()

        reconciler = &controller.WorkflowExecutionReconciler{
            Client:   fakeK8sClient,
            Scheme:   scheme,
            // WorkflowEngine, Validator, Monitor would be real implementations
        }
    })

    Context("BR-WF-010: Planning Phase", func() {
        It("should create execution plan with dependency resolution", func() {
            wf := &workflowexecutionv1.WorkflowExecution{
                ObjectMeta: metav1.ObjectMeta{
                    Name:      "test-workflow",
                    Namespace: "default",
                },
                Spec: workflowexecutionv1.WorkflowExecutionSpec{
                    WorkflowDefinition: workflowexecutionv1.WorkflowDefinition{
                        Name:    "scale-and-restart",
                        Version: "1.0",
                        Steps: []workflowexecutionv1.WorkflowStep{
                            {
                                StepNumber: 1,
                                Name:       "scale-deployment",
                                Action:     "scale-deployment",
                                Parameters: map[string]interface{}{
                                    "deployment": "web-app",
                                    "replicas":   5,
                                },
                                CriticalStep: true,
                            },
                            {
                                StepNumber: 2,
                                Name:       "verify-health",
                                Action:     "check-pod-health",
                                DependsOn:  []int{1},
                            },
                        },
                        Dependencies: map[string][]string{
                            "verify-health": {"scale-deployment"},
                        },
                    },
                    ExecutionStrategy: workflowexecutionv1.ExecutionStrategy{
                        ApprovalRequired: false,
                        DryRunFirst:      false,
                        RollbackStrategy: "automatic",
                    },
                },
            }

            Expect(fakeK8sClient.Create(ctx, wf)).To(Succeed())

            // Execute reconciliation
            result, err := reconciler.Reconcile(ctx, testutil.NewReconcileRequest(wf))

            // Validate planning outcome
            Expect(err).ToNot(HaveOccurred())
            Expect(result.Requeue).To(BeTrue())

            // Fetch updated status
            Expect(fakeK8sClient.Get(ctx, client.ObjectKeyFromObject(wf), wf)).To(Succeed())
            Expect(wf.Status.Phase).To(Equal("validating"))
            Expect(wf.Status.TotalSteps).To(Equal(2))
            Expect(wf.Status.ExecutionPlan).ToNot(BeNil())
            Expect(wf.Status.ExecutionPlan.Strategy).To(Equal("sequential"))
        })
    })

    Context("BR-WF-030: Executing Phase", func() {
        It("should create KubernetesExecution CRD for each step", func() {
            wf := testutil.NewWorkflowExecution("test-execution", "default")
            wf.Status.Phase = "executing"
            wf.Status.TotalSteps = 2
            wf.Status.CurrentStep = 0
            wf.Status.StepStatuses = []workflowexecutionv1.StepStatus{
                {StepNumber: 1, Status: "pending"},
                {StepNumber: 2, Status: "pending"},
            }
            wf.Status.ExecutionPlan = &workflowexecutionv1.ExecutionPlan{
                Strategy:         "sequential",
                RollbackStrategy: "automatic",
            }

            Expect(fakeK8sClient.Create(ctx, wf)).To(Succeed())

            // Execute reconciliation
            result, err := reconciler.Reconcile(ctx, testutil.NewReconcileRequest(wf))

            // Validate KubernetesExecution creation
            Expect(err).ToNot(HaveOccurred())
            Expect(result.RequeueAfter).To(Equal(10 * time.Second))

            // Verify KubernetesExecution CRD created
            k8sExecList := &executorv1.KubernetesExecutionList{}
            Expect(fakeK8sClient.List(ctx, k8sExecList, client.InNamespace("default"))).To(Succeed())
            Expect(k8sExecList.Items).To(HaveLen(1))
            Expect(k8sExecList.Items[0].Name).To(ContainSubstring("test-execution-step-1"))

            // Verify owner reference set
            Expect(k8sExecList.Items[0].OwnerReferences).ToNot(BeEmpty())
            Expect(k8sExecList.Items[0].OwnerReferences[0].Name).To(Equal("test-execution"))
        })
    })
})
```

### Mock Usage Decision Matrix

| Component | Unit Tests | Integration | E2E | Justification |
|-----------|------------|-------------|-----|---------------|
| **Kubernetes API** | **FAKE K8S CLIENT** | REAL (KIND) | REAL (OCP/KIND) | Compile-time API safety, type-safe CRD handling |
| **Workflow Engine** | REAL | REAL | REAL | Core business logic |
| **Executor Service** | **FAKE K8S CLIENT** (KubernetesExecution CRDs) | REAL | REAL | Watch-based coordination |
| **Safety Validator** | REAL | REAL | REAL | Business logic validation |
| **Metrics Recording** | REAL | REAL | REAL | Business observability |

---

## Database Integration for Audit & Tracking

### Dual Audit System

Following the [MULTI_CRD_RECONCILIATION_ARCHITECTURE.md](../../architecture/MULTI_CRD_RECONCILIATION_ARCHITECTURE.md#crd-lifecycle-management-and-cleanup) dual audit approach:

1. **CRDs**: Real-time execution state + 24-hour review window
2. **Database**: Long-term compliance + workflow learning

### Audit Data Schema

```go
package storage

type WorkflowExecutionAudit struct {
    ID               string                    `json:"id" db:"id"`
    RemediationID    string                    `json:"remediation_id" db:"remediation_id"`
    WorkflowName     string                    `json:"workflow_name" db:"workflow_name"`
    WorkflowVersion  string                    `json:"workflow_version" db:"workflow_version"`

    // Execution metrics
    TotalSteps       int                       `json:"total_steps" db:"total_steps"`
    StepsCompleted   int                       `json:"steps_completed" db:"steps_completed"`
    StepsFailed      int                       `json:"steps_failed" db:"steps_failed"`
    TotalDuration    time.Duration             `json:"total_duration" db:"total_duration"`

    // Outcome
    Outcome          string                    `json:"outcome" db:"outcome"` // success, failed, partial
    EffectivenessScore float64                 `json:"effectiveness_score" db:"effectiveness_score"`
    RollbacksPerformed int                     `json:"rollbacks_performed" db:"rollbacks_performed"`

    // Learning data
    StepExecutions   []StepExecutionAudit      `json:"step_executions" db:"step_executions"`
    AdaptiveAdjustments []AdaptiveAdjustment   `json:"adaptive_adjustments" db:"adaptive_adjustments"`

    // Metadata
    CompletedAt      time.Time                 `json:"completed_at" db:"completed_at"`
    Status           string                    `json:"status" db:"status"`
    ErrorMessage     string                    `json:"error_message,omitempty" db:"error_message"`
}

type StepExecutionAudit struct {
    StepNumber       int                       `json:"step_number"`
    Action           string                    `json:"action"`
    Duration         time.Duration             `json:"duration"`
    Status           string                    `json:"status"`
    RetriesAttempted int                       `json:"retries_attempted"`
    ErrorMessage     string                    `json:"error_message,omitempty"`
}
```

---

## Integration Points

### 1. Upstream Integration: AlertRemediation Controller

**Integration Pattern**: AlertRemediation creates WorkflowExecution after AIAnalysis completes and approval received

```go
// In AlertRemediationReconciler (Remediation Coordinator)
// Requires: import remediationv1 "github.com/jordigilh/kubernaut/api/remediation/v1"
// Requires: import aiv1 "github.com/jordigilh/kubernaut/api/ai/v1"
// Requires: import workflowexecutionv1 "github.com/jordigilh/kubernaut/api/workflow/v1"
func (r *AlertRemediationReconciler) reconcileWorkflowExecution(
    ctx context.Context,
    remediation *remediationv1.AlertRemediation,
    aiAnalysis *aiv1.AIAnalysis,
) error {
    // When AIAnalysis is approved, create WorkflowExecution
    if aiAnalysis.Status.Phase == "completed" && remediation.Status.WorkflowExecutionRef == nil {
        workflowExec := &workflowexecutionv1.WorkflowExecution{
            ObjectMeta: metav1.ObjectMeta{
                Name:      fmt.Sprintf("%s-workflow", remediation.Name),
                Namespace: remediation.Namespace,
                OwnerReferences: []metav1.OwnerReference{
                    *metav1.NewControllerRef(remediation, remediationv1.GroupVersion.WithKind("AlertRemediation")),
                },
            },
            Spec: workflowexecutionv1.WorkflowExecutionSpec{
                AlertRemediationRef: workflowexecutionv1.AlertRemediationReference{
                    Name:      remediation.Name,
                    Namespace: remediation.Namespace,
                },
                // Build workflow from AI recommendations
                WorkflowDefinition: buildWorkflowFromRecommendations(aiAnalysis.Status.Recommendations),
                ExecutionStrategy: workflowexecutionv1.ExecutionStrategy{
                    ApprovalRequired: false, // Already approved at AIAnalysis level
                    DryRunFirst:      true,  // Safety-first
                    RollbackStrategy: "automatic",
                },
            },
        }

        return r.Create(ctx, workflowExec)
    }

    return nil
}
```

### 2. Downstream Integration: Executor Service via KubernetesExecution CRDs

**Integration Pattern**: WorkflowExecution creates KubernetesExecution CRDs for each step

```go
// WorkflowExecution creates KubernetesExecution for each step
k8sExec := &executorv1.KubernetesExecution{
    ObjectMeta: metav1.ObjectMeta{
        Name:      fmt.Sprintf("%s-step-%d", wf.Name, stepNumber),
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
        Action:        step.Action,
        TargetCluster: step.TargetCluster,
        Parameters:    step.Parameters,
        SafetyChecks:  wf.Spec.ExecutionStrategy.SafetyChecks,
    },
}
r.Create(ctx, k8sExec)
```

---

## RBAC Configuration

```yaml
apiVersion: rbac.authorization.k8s.io/v1
kind: ClusterRole
metadata:
  name: workflowexecution-controller
rules:
- apiGroups: ["workflowexecution.kubernaut.io"]
  resources: ["workflowexecutions", "workflowexecutions/status"]
  verbs: ["get", "list", "watch", "update", "patch"]
- apiGroups: ["workflowexecution.kubernaut.io"]
  resources: ["workflowexecutions/finalizers"]
  verbs: ["update"]
- apiGroups: ["kubernetesexecution.kubernaut.io"]
  resources: ["kubernetesexecutions"]
  verbs: ["create", "get", "list", "watch"]
- apiGroups: ["kubernaut.io"]
  resources: ["alertremediations"]
  verbs: ["get", "list", "watch"]
- apiGroups: [""]
  resources: ["events"]
  verbs: ["create", "patch"]
```

---

## Implementation Checklist

**Note**: Follow APDC-TDD phases for each implementation step (see Development Methodology section)

### Phase 1: ANALYSIS & CRD Setup (2 days) [RED Phase Preparation]

- [ ] **ANALYSIS**: Search existing workflow implementations (`codebase_search "workflow execution implementations"`)
- [ ] **ANALYSIS**: Map business requirements (BR-WF-001 to BR-WF-165, BR-ORCHESTRATION-001 to BR-ORCHESTRATION-045)
- [ ] **ANALYSIS**: Identify integration points in cmd/workflowexecution/
- [ ] **CRD RED**: Write WorkflowExecutionReconciler tests (should fail - no controller yet)
- [ ] **CRD GREEN**: Generate CRD + controller skeleton (tests pass)
  - [ ] Create WorkflowExecution CRD schema (`api/v1/workflowexecution_types.go`)
  - [ ] Generate Kubebuilder controller scaffold
  - [ ] Implement WorkflowExecutionReconciler with finalizers
  - [ ] Configure owner references to AlertRemediation CRD
- [ ] **CRD REFACTOR**: Enhance controller with error handling
  - [ ] Add controller-specific Prometheus metrics
  - [ ] Implement cross-CRD reference validation
  - [ ] Add phase timeout detection (configurable per phase)

### Phase 2: Planning & Validation Phases (2-3 days) [RED-GREEN-REFACTOR]

- [ ] **Planning RED**: Write tests for planning phase (fail - no planning logic yet)
- [ ] **Planning GREEN**: Implement minimal planning logic (tests pass)
  - [ ] Workflow analysis and dependency resolution
  - [ ] Execution strategy planning
  - [ ] Resource planning and estimation
- [ ] **Planning REFACTOR**: Enhance with sophisticated algorithms
  - [ ] Parallel execution detection
  - [ ] Adaptive optimization based on history
- [ ] **Validation RED**: Write tests for validation phase (fail)
- [ ] **Validation GREEN**: Implement safety validation (tests pass)
  - [ ] RBAC checks
  - [ ] Resource availability validation
  - [ ] Dry-run execution (optional)
  - [ ] Approval validation
- [ ] **Validation REFACTOR**: Add sophisticated safety checks

### Phase 3: Execution & Monitoring Phases (3-4 days) [RED-GREEN-REFACTOR]

- [ ] **Execution RED**: Write tests for step execution (fail - no execution logic yet)
- [ ] **Execution GREEN**: Implement step orchestration (tests pass)
  - [ ] KubernetesExecution CRD creation per step
  - [ ] Watch-based step completion monitoring
  - [ ] Dependency handling (sequential/parallel)
  - [ ] Failure handling and retry logic
- [ ] **Execution REFACTOR**: Enhance with adaptive adjustments
  - [ ] Runtime optimization
  - [ ] Historical pattern application
- [ ] **Monitoring RED**: Write tests for effectiveness monitoring (fail)
- [ ] **Monitoring GREEN**: Implement monitoring logic (tests pass)
  - [ ] Resource health validation
  - [ ] Success criteria verification
  - [ ] Learning and optimization recording
- [ ] **Main App Integration**: Verify WorkflowExecutionReconciler instantiated in cmd/workflowexecution/ (MANDATORY)

### Phase 4: Rollback & Error Handling (2 days) [RED-GREEN-REFACTOR]

- [ ] **Rollback RED**: Write tests for rollback strategies (fail)
- [ ] **Rollback GREEN**: Implement automatic rollback (tests pass)
  - [ ] Step-by-step rollback execution
  - [ ] State restoration logic
  - [ ] Rollback verification
- [ ] **Rollback REFACTOR**: Add manual rollback support
  - [ ] Rollback approval workflow
  - [ ] Partial rollback capabilities

### Phase 5: Testing & Validation (2 days) [CHECK Phase]

- [ ] **CHECK**: Verify 70%+ unit test coverage (test/unit/workflowexecution/)
  - [ ] Planning phase tests (BR-WF-001 to BR-WF-010)
  - [ ] Validation phase tests (BR-WF-015 to BR-WF-025)
  - [ ] Execution phase tests (BR-WF-030 to BR-WF-045)
  - [ ] Monitoring phase tests (BR-WF-050 to BR-WF-060)
  - [ ] Rollback tests (BR-WF-050 to BR-WF-051)
- [ ] **CHECK**: Run integration tests - 20% coverage target (test/integration/workflowexecution/)
  - [ ] Real K8s API (KIND) CRD lifecycle tests
  - [ ] Cross-CRD coordination with KubernetesExecution
  - [ ] Watch-based step monitoring tests
- [ ] **CHECK**: Execute E2E tests - 10% coverage target (test/e2e/workflowexecution/)
  - [ ] Complete workflow-to-completion scenario
  - [ ] Multi-step workflow with dependencies
  - [ ] Rollback scenario testing
- [ ] **CHECK**: Validate business requirement coverage (BR-WF-001 to BR-WF-165)
- [ ] **CHECK**: Performance validation (per-step <5min, total <30min)
- [ ] **CHECK**: Provide confidence assessment (90% high confidence)

### Phase 6: Metrics, Audit & Deployment (1 day)

- [ ] **Metrics**: Define and implement Prometheus metrics
  - [ ] Workflow execution metrics
  - [ ] Phase duration metrics
  - [ ] Step success/failure metrics
  - [ ] Rollback metrics
  - [ ] Setup metrics server on port 9090 (with auth)
- [ ] **Audit**: Database integration for learning
  - [ ] Implement audit client
  - [ ] Record workflow executions to PostgreSQL
  - [ ] Store execution patterns to vector DB
  - [ ] Implement historical success queries
- [ ] **Deployment**: Binary and infrastructure
  - [ ] Create `cmd/workflowexecution/main.go` entry point
  - [ ] Configure Kubebuilder manager with leader election
  - [ ] Add RBAC permissions for CRD operations
  - [ ] Create Kubernetes deployment manifests

### Phase 7: Documentation (1 day)

- [ ] Update API documentation with WorkflowExecution CRD
- [ ] Document workflow planning patterns
- [ ] Add troubleshooting guide for workflow execution
- [ ] Create runbook for rollback procedures
- [ ] Document adaptive orchestration mechanisms

---

## Critical Architectural Patterns

### 1. Owner References & Cascade Deletion
**Pattern**: WorkflowExecution CRD owned by AlertRemediation
```go
controllerutil.SetControllerReference(&alertRemediation, &workflowExecution, scheme)
```
**Purpose**: Automatic cleanup when AlertRemediation is deleted (24h retention)

### 2. Finalizers for Cleanup Coordination
**Pattern**: Add finalizer before processing, remove after cleanup
```go
const workflowExecutionFinalizer = "workflowexecution.kubernaut.io/finalizer"
```
**Purpose**: Ensure audit data persisted before CRD deletion

### 3. Watch-Based Step Coordination
**Pattern**: Watch KubernetesExecution CRDs for step completion
```go
ctrl.NewControllerManagedBy(mgr).
    For(&workflowexecutionv1.WorkflowExecution{}).
    Owns(&executorv1.KubernetesExecution{}).  // Watch for step completion
    Complete(r)
```
**Purpose**: Automatic step completion detection without polling

### 4. Phase Timeout Detection & Escalation
**Pattern**: Per-phase timeout with fallback strategies
```go
defaultPlanningTimeout   = 30 * time.Second
defaultValidationTimeout = 5 * time.Minute
defaultStepTimeout       = 5 * time.Minute
defaultMonitoringTimeout = 10 * time.Minute
```
**Purpose**: Prevent stuck workflows, enable escalation

### 5. Adaptive Orchestration
**Pattern**: Runtime adjustment based on execution patterns
```go
if wf.Spec.AdaptiveOrchestration.OptimizationEnabled {
    adjustedPlan := r.WorkflowEngine.OptimizeExecution(plan, historicalData)
}
```
**Purpose**: Improve workflow success rates through learning

---

## Common Pitfalls

1. **Don't execute K8s operations directly** - Delegate to Executor Service via KubernetesExecution CRDs
2. **Approval validation timing** - Check approval before validation phase, not during execution
3. **Step dependency resolution** - Build complete dependency graph during planning
4. **Rollback state management** - Store pre-execution state for accurate rollback
5. **Missing owner references** - Always set AlertRemediation as owner for cascade deletion
6. **Finalizer cleanup** - Ensure audit persistence before removing finalizer
7. **Event emission** - Emit events for all significant state changes
8. **Phase timeouts** - Implement configurable per-phase timeout detection
9. **Watch configuration** - Properly configure Owns() for KubernetesExecution CRDs

---

## Summary

**Workflow Execution Service - V1 Design Specification (90% Complete)**

### Core Purpose
Multi-step remediation workflow orchestration with adaptive execution, safety validation, and intelligent optimization.

### Key Architectural Decisions
1. **Multi-Phase State Machine** - Planning → Validating → Executing → Monitoring → Completed (5 phases)
2. **Safety-First Validation** - Mandatory validation phase with dry-run capabilities
3. **Step-Based Execution** - Each step creates KubernetesExecution CRD for atomic operations
4. **Watch-Based Coordination** - Monitors KubernetesExecution status for step completion
5. **Adaptive Orchestration** - Runtime optimization based on historical patterns

### Integration Model
```
AlertRemediation → WorkflowExecution (this service)
                         ↓
        WorkflowExecution creates KubernetesExecution per step
                         ↓
           WorkflowExecution watches step completion
                         ↓
        WorkflowExecution.status.phase = "completed"
```

### V1 Scope Boundaries
**Included**:
- Single-workflow execution (sequential or parallel steps)
- Safety validation with dry-run
- Basic rollback strategies
- Step dependency resolution
- Real-time execution monitoring

**Excluded** (V2):
- Multi-workflow orchestration
- Advanced ML optimization
- Cross-cluster execution
- Workflow scheduling

### Business Requirements Coverage
- **BR-WF-001 to BR-WF-165**: Core workflow functionality
- **BR-ORCHESTRATION-001 to BR-ORCHESTRATION-045**: Adaptive orchestration
- **BR-EXECUTION-001 to BR-EXECUTION-035**: Action execution
- **BR-AUTOMATION-001 to BR-AUTOMATION-030**: Intelligent automation

### Implementation Status
- **Existing Code**: Workflow engine components in `pkg/workflow/` to reuse
- **Migration Effort**: 10-12 days (2 weeks)
- **CRD Controller**: New implementation following controller-runtime patterns
- **Database Schema**: Workflow audit table design complete

### Next Steps
1. ✅ **Approved Design Specification** (90% complete)
2. **CRD Schema Definition**: WorkflowExecution API types
3. **Controller Implementation**: Multi-phase reconciliation logic
4. **Integration Testing**: With Executor Service and AlertRemediation

### Critical Success Factors
- Multi-phase execution with proper timeouts
- Safety validation before execution
- Watch-based step coordination
- Proper owner references for cascade deletion
- Audit trail completeness for learning

**Design Specification Status**: Production-Ready (90% Confidence)

