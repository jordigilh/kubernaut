# AI Analysis to Workflow Execution - Detailed Step-by-Step Flow

**Date**: October 8, 2025
**Purpose**: Detailed technical flow from AI Analysis Controller completion through Workflow Controller creating and executing workflow manifest via Executor Controller
**Sources**:
- `docs/requirements/04_WORKFLOW_ENGINE_ORCHESTRATION.md`
- `docs/services/crd-controllers/02-aianalysis/`
- `docs/services/crd-controllers/03-workflowexecution/`
- `docs/services/crd-controllers/04-kubernetesexecutor/`
- `docs/services/crd-controllers/05-remediationorchestrator/`

---

## 🎯 **HIGH-LEVEL OVERVIEW**

```
AIAnalysis Controller (Phase: completed)
    ↓ (status update triggers watch)
RemediationOrchestrator Controller (detects AIAnalysis.status.phase = "completed")
    ↓ (creates WorkflowExecution CRD)
WorkflowExecution Controller (Phase: planning → validating → executing)
    ↓ (creates KubernetesExecution (DEPRECATED - ADR-025) CRDs for each step)
KubernetesExecution Controller (executes action + validates outcome)
    ↓ (updates status with results)
WorkflowExecution Controller (monitors step completion)
    ↓ (all steps complete)
WorkflowExecution Controller (Phase: completed)
```

---

## 📋 **DETAILED STEP-BY-STEP FLOW**

---

## **PHASE 1: AI ANALYSIS COMPLETION** (AIAnalysis Controller)

### **Step 1.1: AIAnalysis Enters "recommending" Phase**

**Controller**: AIAnalysis Reconciler
**CRD**: `AIAnalysis`
**Phase**: `recommending`
**Business Requirements**: BR-AI-006, BR-AI-007, BR-AI-008, BR-AI-009, BR-AI-010

**Actions**:
1. Generate remediation recommendations from HolmesGPT analysis
2. Rank recommendations by feasibility and impact
3. Calculate historical success rates for each recommendation
4. Apply safety constraints (production/non-production)
5. Incorporate cluster capacity and resource availability

**CRD Status Update**:
```yaml
status:
  phase: recommending
  recommendations:
  - id: "rec-001"
    action: "scale_deployment"
    description: "Scale payment-api deployment from 3 to 5 replicas"
    confidence: 0.92
    historicalSuccessRate: 0.85
    feasibility: "high"
    impact: "medium"
    safetyLevel: "safe"
    parameters:
      deployment: "payment-api"
      namespace: "production"
      currentReplicas: 3
      targetReplicas: 5
  - id: "rec-002"
    action: "increase_memory_limit"
    description: "Increase memory limit from 512Mi to 1Gi"
    confidence: 0.88
    historicalSuccessRate: 0.78
    feasibility: "high"
    impact: "low"
    safetyLevel: "safe"
```

**Duration**: ~5 seconds (ranking and constraint application)

---

### **Step 1.2: AIAnalysis Enters "completed" Phase**

**Controller**: AIAnalysis Reconciler
**CRD**: `AIAnalysis`
**Phase**: `completed`
**Business Requirements**: BR-AI-014

**Actions**:
1. Select top-ranked recommendation for execution
2. Generate investigation report
3. Update CRD status to "completed"
4. Store analysis metadata in audit database
5. Emit Kubernetes event: `AIAnalysisCompleted`

**CRD Status Update**:
```yaml
status:
  phase: completed
  topRecommendation:
    id: "rec-001"
    action: "scale_deployment"
    confidence: 0.92
    parameters:
      deployment: "payment-api"
      namespace: "production"
      targetReplicas: 5
  completionTime: "2025-01-15T10:35:00Z"
  investigationReport:
    summary: "Memory pressure causing OOM events"
    rootCause: "Insufficient memory limits for current workload"
    recommendation: "Scale deployment to distribute load"
```

**Critical**: This status update triggers a Kubernetes watch event.

**Duration**: <100ms (status update propagation)

---

## **PHASE 2: ORCHESTRATOR DETECTS COMPLETION** (RemediationOrchestrator Controller)

### **Step 2.1: Watch Event Triggers RemediationOrchestrator Reconciliation**

**Controller**: RemediationOrchestrator Reconciler
**CRD**: `RemediationRequest`
**Current Phase**: `analyzing`
**Trigger**: Watch on `AIAnalysis` CRD status changes

**Watch Pattern**:
```go
// In RemediationOrchestrator controller setup
err = c.Watch(
    &source.Kind{Type: &aiv1.AIAnalysis{}},
    handler.EnqueueRequestsFromMapFunc(r.aiAnalysisToRemediationRequest),
)
```

**Reconciliation Triggered**: Watch detects `AIAnalysis.status.phase = "completed"`

**Duration**: <100ms (watch event propagation)

---

### **Step 2.2: RemediationOrchestrator Validates AIAnalysis Completion**

**Controller**: RemediationOrchestrator Reconciler
**CRD**: `RemediationRequest`
**Business Requirements**: BR-AR-062 (status aggregation)

**Validation Checks**:
```go
func (r *RemediationRequestReconciler) validateAIAnalysisComplete(
    ctx context.Context,
    remediationRequest *remediationv1.RemediationRequest,
) (bool, error) {
    // Fetch AIAnalysis CRD owned by this RemediationRequest
    aiAnalysis := &aiv1.AIAnalysis{}
    err := r.Get(ctx, client.ObjectKey{
        Name:      remediationRequest.Status.AIAnalysisRef.Name,
        Namespace: remediationRequest.Namespace,
    }, aiAnalysis)

    if err != nil {
        return false, fmt.Errorf("failed to fetch AIAnalysis: %w", err)
    }

    // Validation criteria
    checks := []bool{
        aiAnalysis.Status.Phase == "completed",
        len(aiAnalysis.Status.Recommendations) > 0,
        aiAnalysis.Status.TopRecommendation != nil,
    }

    for _, check := range checks {
        if !check {
            return false, nil
        }
    }

    return true, nil
}
```

**Duration**: ~50ms (API call to fetch AIAnalysis)

---

### **Step 2.3: RemediationOrchestrator Creates WorkflowExecution CRD**

**Controller**: RemediationOrchestrator Reconciler
**CRD**: `WorkflowExecution` (CREATED)
**Business Requirements**: BR-AR-063, BR-AR-064, BR-AR-065

**Data Snapshot Pattern**: Copy AI recommendations from AIAnalysis.status → WorkflowExecution.spec

**Creation Logic**:
```go
func (r *RemediationRequestReconciler) createWorkflowExecution(
    ctx context.Context,
    remediationRequest *remediationv1.RemediationRequest,
    aiAnalysis *aiv1.AIAnalysis,
) error {
    // Extract top recommendation
    topRec := aiAnalysis.Status.TopRecommendation

    // Build workflow steps from recommendation
    steps := buildWorkflowSteps(topRec)

    // Create WorkflowExecution CRD
    workflow := &workflowv1.WorkflowExecution{
        ObjectMeta: metav1.ObjectMeta{
            Name:      fmt.Sprintf("%s-workflow", remediationRequest.Name),
            Namespace: remediationRequest.Namespace,
            OwnerReferences: []metav1.OwnerReference{
                {
                    APIVersion: remediationv1.GroupVersion.String(),
                    Kind:       "RemediationRequest",
                    Name:       remediationRequest.Name,
                    UID:        remediationRequest.UID,
                    Controller: pointer.Bool(true),
                },
            },
        },
        Spec: workflowv1.WorkflowExecutionSpec{
            WorkflowDefinition: workflowv1.WorkflowDefinition{
                Steps: steps,
            },
            ExecutionStrategy: workflowv1.ExecutionStrategy{
                Mode:            "sequential", // or "parallel"
                DryRunFirst:     true,
                ApprovalRequired: false, // Already approved in AI phase
                RollbackOnFailure: true,
            },
            AlertContext: workflowv1.AlertContext{
                Fingerprint:  remediationRequest.Spec.AlertFingerprint,
                Severity:     remediationRequest.Spec.Severity,
                OriginalAlert: remediationRequest.Spec.OriginalPayload,
            },
            AIRecommendation: workflowv1.AIRecommendation{
                RecommendationID: topRec.ID,
                Confidence:       topRec.Confidence,
                Action:           topRec.Action,
                Parameters:       topRec.Parameters,
            },
        },
    }

    if err := r.Create(ctx, workflow); err != nil {
        return fmt.Errorf("failed to create WorkflowExecution: %w", err)
    }

    // Update RemediationRequest status
    remediationRequest.Status.Phase = "executing"
    remediationRequest.Status.WorkflowRef = &corev1.ObjectReference{
        Kind:      "WorkflowExecution",
        Name:      workflow.Name,
        Namespace: workflow.Namespace,
        UID:       workflow.UID,
    }

    return r.Status().Update(ctx, remediationRequest)
}
```

**WorkflowExecution CRD Created**:
```yaml
apiVersion: workflow.kubernaut.io/v1
kind: WorkflowExecution
metadata:
  name: rem-req-abc123-workflow
  namespace: kubernaut-system
  ownerReferences:
  - apiVersion: remediation.kubernaut.io/v1
    kind: RemediationRequest
    name: rem-req-abc123
    uid: <parent-uid>
    controller: true
spec:
  workflowDefinition:
    steps:
    - name: "scale-deployment"
      action: "scale_deployment"
      parameters:
        deployment: "payment-api"
        namespace: "production"
        replicas: 5
      dependencies: []
  executionStrategy:
    mode: "sequential"
    dryRunFirst: true
    approvalRequired: false
    rollbackOnFailure: true
  alertContext:
    fingerprint: "prom-alert-xyz789"
    severity: "critical"
  aiRecommendation:
    recommendationID: "rec-001"
    confidence: 0.92
    action: "scale_deployment"
status:
  phase: "" # Will be set by WorkflowExecution controller
```

**Duration**: ~200ms (CRD creation + API propagation)

**Critical**: WorkflowExecution CRD creation triggers a reconciliation in the WorkflowExecution Controller.

---

## **PHASE 3: WORKFLOW PLANNING** (WorkflowExecution Controller)

### **Step 3.1: WorkflowExecution Controller Detects New CRD**

**Controller**: WorkflowExecution Reconciler
**CRD**: `WorkflowExecution`
**Phase**: `""` (empty, newly created)
**Business Requirements**: BR-WF-001, BR-WF-002

**Trigger**: WorkflowExecution CRD created by RemediationOrchestrator

**Reconciliation Entry Point**:
```go
func (r *WorkflowExecutionReconciler) Reconcile(
    ctx context.Context,
    req ctrl.Request,
) (ctrl.Result, error) {
    workflow := &workflowv1.WorkflowExecution{}
    if err := r.Get(ctx, req.NamespacedName, workflow); err != nil {
        return ctrl.Result{}, client.IgnoreNotFound(err)
    }

    // Initial state - transition to planning
    if workflow.Status.Phase == "" {
        workflow.Status.Phase = "planning"
        workflow.Status.StartTime = metav1.Now()
        return ctrl.Result{}, r.Status().Update(ctx, workflow)
    }

    // Handle planning phase
    if workflow.Status.Phase == "planning" {
        return r.handlePlanningPhase(ctx, workflow)
    }

    // ... other phases
}
```

**Duration**: <50ms (initial phase transition)

---

### **Step 3.2: Planning Phase - Parse AI Recommendations**

**Controller**: WorkflowExecution Reconciler
**CRD**: `WorkflowExecution`
**Phase**: `planning`
**Business Requirements**: BR-WF-001, BR-WF-002, BR-WF-010

**Actions**:

**3.2.1: Parse Workflow Definition**
```go
func (r *WorkflowExecutionReconciler) handlePlanningPhase(
    ctx context.Context,
    workflow *workflowv1.WorkflowExecution,
) (ctrl.Result, error) {
    // Step 1: Parse AI recommendations (AUTHORITATIVE - no Context API query)
    steps := workflow.Spec.WorkflowDefinition.Steps

    log.Info("Parsing workflow steps",
        "stepCount", len(steps),
        "mode", workflow.Spec.ExecutionStrategy.Mode)

    // Step 2: Build dependency graph
    graph, err := r.buildDependencyGraph(steps)
    if err != nil {
        return r.transitionToFailed(ctx, workflow, "dependency_resolution_failed", err)
    }

    // Step 3: Calculate execution order
    executionOrder, err := r.calculateExecutionOrder(graph)
    if err != nil {
        return r.transitionToFailed(ctx, workflow, "execution_order_failed", err)
    }

    // Step 4: Validate safety constraints
    if err := r.validateSafetyConstraints(ctx, steps); err != nil {
        return r.transitionToFailed(ctx, workflow, "safety_validation_failed", err)
    }

    // Step 5: Update status with execution plan
    workflow.Status.TotalSteps = len(steps)
    workflow.Status.CurrentStep = 0
    workflow.Status.ExecutionPlan = &workflowv1.ExecutionPlan{
        Strategy:          workflow.Spec.ExecutionStrategy.Mode,
        EstimatedDuration: calculateDuration(steps),
        RollbackStrategy:  "automatic",
        ExecutionOrder:    executionOrder,
    }

    // Transition to validating phase
    workflow.Status.Phase = "validating"

    return ctrl.Result{}, r.Status().Update(ctx, workflow)
}
```

**3.2.2: Build Dependency Graph** (BR-WF-010, BR-WF-011)
```go
func (r *WorkflowExecutionReconciler) buildDependencyGraph(
    steps []workflowv1.WorkflowStep,
) (*DependencyGraph, error) {
    graph := NewDependencyGraph()

    for _, step := range steps {
        node := &GraphNode{
            StepName:     step.Name,
            Action:       step.Action,
            Dependencies: step.Dependencies,
        }

        if err := graph.AddNode(node); err != nil {
            return nil, fmt.Errorf("failed to add node %s: %w", step.Name, err)
        }
    }

    // Detect circular dependencies
    if graph.HasCycle() {
        return nil, fmt.Errorf("circular dependency detected in workflow")
    }

    return graph, nil
}
```

**3.2.3: Calculate Execution Order**
```go
func (r *WorkflowExecutionReconciler) calculateExecutionOrder(
    graph *DependencyGraph,
) ([]ExecutionBatch, error) {
    // Topological sort to determine execution order
    order := []ExecutionBatch{}

    // Group steps that can run in parallel
    for !graph.IsEmpty() {
        batch := ExecutionBatch{
            Steps:    []string{},
            Parallel: true,
        }

        // Find all nodes with no dependencies
        readyNodes := graph.GetReadyNodes()
        for _, node := range readyNodes {
            batch.Steps = append(batch.Steps, node.StepName)
            graph.RemoveNode(node.StepName)
        }

        order = append(order, batch)
    }

    return order, nil
}
```

**CRD Status Update**:
```yaml
status:
  phase: validating
  totalSteps: 1
  currentStep: 0
  executionPlan:
    strategy: "sequential"
    estimatedDuration: "5m"
    rollbackStrategy: "automatic"
    executionOrder:
    - batch: 1
      steps: ["scale-deployment"]
      parallel: false
  stepStatuses: []
```

**Duration**: ~5 seconds (parsing, graph building, validation)

---

## **PHASE 4: WORKFLOW VALIDATION** (WorkflowExecution Controller)

### **Step 4.1: Safety Checks and Pre-Flight Validation**

**Controller**: WorkflowExecution Reconciler
**CRD**: `WorkflowExecution`
**Phase**: `validating`
**Business Requirements**: BR-WF-015, BR-WF-016, BR-WF-017, BR-WF-018, BR-WF-019

**Actions**:

**4.1.1: Validate RBAC Permissions** (BR-WF-015)
```go
func (r *WorkflowExecutionReconciler) handleValidatingPhase(
    ctx context.Context,
    workflow *workflowv1.WorkflowExecution,
) (ctrl.Result, error) {
    validationResults := &workflowv1.ValidationResults{}

    // Step 1: Validate RBAC for all steps
    for _, step := range workflow.Spec.WorkflowDefinition.Steps {
        saName := getServiceAccountForAction(step.Action)

        hasPermission, err := r.validateRBAC(ctx, saName, step.Parameters)
        if err != nil || !hasPermission {
            validationResults.RBACValid = false
            validationResults.ErrorMessage = fmt.Sprintf(
                "Insufficient RBAC for step %s: %v", step.Name, err)
            return r.transitionToFailed(ctx, workflow, "rbac_validation_failed", err)
        }
    }
    validationResults.RBACValid = true

    // Step 2: Validate resource availability
    for _, step := range workflow.Spec.WorkflowDefinition.Steps {
        exists, err := r.validateResourceExists(ctx, step.Parameters)
        if err != nil || !exists {
            validationResults.ResourceAvailable = false
            validationResults.ErrorMessage = fmt.Sprintf(
                "Target resource not found for step %s", step.Name)
            return r.transitionToFailed(ctx, workflow, "resource_not_found", err)
        }
    }
    validationResults.ResourceAvailable = true

    // Step 3: Dry-run if configured
    if workflow.Spec.ExecutionStrategy.DryRunFirst {
        for _, step := range workflow.Spec.WorkflowDefinition.Steps {
            dryRunSuccess, err := r.executeDryRun(ctx, step)
            if err != nil || !dryRunSuccess {
                validationResults.DryRunPassed = false
                validationResults.ErrorMessage = fmt.Sprintf(
                    "Dry-run failed for step %s: %v", step.Name, err)
                return r.transitionToFailed(ctx, workflow, "dry_run_failed", err)
            }
        }
        validationResults.DryRunPassed = true
    }

    // Step 4: Check approval (if required)
    if workflow.Spec.ExecutionStrategy.ApprovalRequired {
        approved := r.checkApprovalAnnotation(workflow)
        if !approved {
            // Requeue until approval received
            return ctrl.Result{RequeueAfter: 30 * time.Second}, nil
        }
        validationResults.ApprovalReceived = true
    }

    // All validations passed - transition to executing
    workflow.Status.ValidationResults = validationResults
    workflow.Status.Phase = "executing"

    return ctrl.Result{}, r.Status().Update(ctx, workflow)
}
```

**CRD Status Update**:
```yaml
status:
  phase: executing
  validationResults:
    rbacValid: true
    resourceAvailable: true
    dryRunPassed: true
    approvalReceived: false # Not required
    safetyChecksComplete: true
  validationTime: "2025-01-15T10:35:10Z"
```

**Duration**: ~30 seconds (includes dry-run if configured)

---

## **PHASE 5: WORKFLOW EXECUTION** (WorkflowExecution Controller)

### **Step 5.1: Create KubernetesExecution CRD for First Step**

**Controller**: WorkflowExecution Reconciler
**CRD**: `KubernetesExecution` (CREATED)
**Business Requirements**: BR-WF-030, BR-WF-031

**Execution Logic**:
```go
func (r *WorkflowExecutionReconciler) handleExecutingPhase(
    ctx context.Context,
    workflow *workflowv1.WorkflowExecution,
) (ctrl.Result, error) {
    // Get execution order from plan
    executionPlan := workflow.Status.ExecutionPlan

    // Determine next step(s) to execute
    nextBatch := r.getNextExecutionBatch(workflow, executionPlan)

    if nextBatch == nil {
        // All steps complete - transition to monitoring
        workflow.Status.Phase = "monitoring"
        return ctrl.Result{}, r.Status().Update(ctx, workflow)
    }

    // Create KubernetesExecution CRD for each step in batch
    for _, stepName := range nextBatch.Steps {
        step := r.getStepByName(workflow, stepName)

        // Check if KubernetesExecution already created
        if r.isStepExecutionCreated(workflow, stepName) {
            continue
        }

        // Create KubernetesExecution CRD
        ke := r.buildKubernetesExecution(workflow, step)
        if err := r.Create(ctx, ke); err != nil {
            return ctrl.Result{}, fmt.Errorf(
                "failed to create KubernetesExecution for step %s: %w",
                stepName, err)
        }

        // Update step status
        workflow.Status.StepStatuses = append(workflow.Status.StepStatuses,
            workflowv1.StepStatus{
                StepName:   stepName,
                Phase:      "executing",
                StartTime:  metav1.Now(),
                ExecutionRef: &corev1.ObjectReference{
                    Kind:      "KubernetesExecution",
                    Name:      ke.Name,
                    Namespace: ke.Namespace,
                    UID:       ke.UID,
                },
            })
    }

    workflow.Status.CurrentStep = len(workflow.Status.StepStatuses)

    return ctrl.Result{}, r.Status().Update(ctx, workflow)
}
```

**Build KubernetesExecution CRD**:
```go
func (r *WorkflowExecutionReconciler) buildKubernetesExecution(
    workflow *workflowv1.WorkflowExecution,
    step workflowv1.WorkflowStep,
) *executorv1.KubernetesExecution {
    return &executorv1.KubernetesExecution{
        ObjectMeta: metav1.ObjectMeta{
            Name:      fmt.Sprintf("%s-%s", workflow.Name, step.Name),
            Namespace: workflow.Namespace,
            OwnerReferences: []metav1.OwnerReference{
                {
                    APIVersion: workflowv1.GroupVersion.String(),
                    Kind:       "WorkflowExecution",
                    Name:       workflow.Name,
                    UID:        workflow.UID,
                    Controller: pointer.Bool(true),
                },
            },
        },
        Spec: executorv1.KubernetesExecutionSpec{
            ActionType:   step.Action,
            ActionParams: step.Parameters,
            TargetCluster: executorv1.ClusterReference{
                Name:      "primary-cluster",
                Namespace: step.Parameters["namespace"].(string),
            },
            Timeout: metav1.Duration{Duration: 5 * time.Minute},
        },
    }
}
```

**KubernetesExecution CRD Created**:
```yaml
apiVersion: executor.kubernaut.io/v1
kind: KubernetesExecution
metadata:
  name: rem-req-abc123-workflow-scale-deployment
  namespace: kubernaut-system
  ownerReferences:
  - apiVersion: workflow.kubernaut.io/v1
    kind: WorkflowExecution
    name: rem-req-abc123-workflow
    uid: <workflow-uid>
    controller: true
spec:
  actionType: "scale_deployment"
  actionParams:
    deployment: "payment-api"
    namespace: "production"
    replicas: 5
  targetCluster:
    name: "primary-cluster"
    namespace: "production"
  timeout: "5m"
status:
  phase: "" # Will be set by KubernetesExecution controller
```

**WorkflowExecution Status Update**:
```yaml
status:
  phase: executing
  currentStep: 1
  totalSteps: 1
  stepStatuses:
  - stepName: "scale-deployment"
    phase: "executing"
    startTime: "2025-01-15T10:35:15Z"
    executionRef:
      kind: "KubernetesExecution"
      name: "rem-req-abc123-workflow-scale-deployment"
      namespace: "kubernaut-system"
```

**Duration**: ~200ms per step (CRD creation)

---

## **PHASE 6: KUBERNETES ACTION EXECUTION** (KubernetesExecution Controller)

### **Step 6.1: KubernetesExecution Controller Detects New CRD**

**Controller**: KubernetesExecution Reconciler
**CRD**: `KubernetesExecution`
**Phase**: `""` (empty, newly created)
**Business Requirements**: BR-EXEC-001 to BR-EXEC-030

**Reconciliation Entry Point**:
```go
func (r *KubernetesExecutionReconciler) Reconcile(
    ctx context.Context,
    req ctrl.Request,
) (ctrl.Result, error) {
    ke := &executorv1.KubernetesExecution{}
    if err := r.Get(ctx, req.NamespacedName, ke); err != nil {
        return ctrl.Result{}, client.IgnoreNotFound(err)
    }

    // Initial state - transition to validating
    if ke.Status.Phase == "" {
        ke.Status.Phase = "validating"
        ke.Status.StartTime = metav1.Now()
        return ctrl.Result{}, r.Status().Update(ctx, ke)
    }

    if ke.Status.Phase == "validating" {
        return r.handleValidatingPhase(ctx, ke)
    }

    if ke.Status.Phase == "executing" {
        return r.handleExecutingPhase(ctx, ke)
    }

    // ... other phases
}
```

---

### **Step 6.2: Pre-Execution Validation**

**Controller**: KubernetesExecution Reconciler
**Phase**: `validating`
**Business Requirements**: BR-EXEC-026, BR-EXEC-060, BR-EXEC-061, BR-EXEC-062, BR-EXEC-063

**Actions**:
```go
func (r *KubernetesExecutionReconciler) handleValidatingPhase(
    ctx context.Context,
    ke *executorv1.KubernetesExecution,
) (ctrl.Result, error) {
    validationResults := &executorv1.ValidationResults{}

    // Step 1: Validate RBAC permissions (BR-EXEC-063)
    saName := getServiceAccountForAction(ke.Spec.ActionType)
    hasPermission, err := r.validateRBAC(ctx, saName, ke.Spec.ActionParams)
    if err != nil || !hasPermission {
        validationResults.RBACValid = false
        validationResults.ErrorMessage = fmt.Sprintf("RBAC validation failed: %v", err)
        return r.transitionToFailed(ctx, ke, validationResults)
    }
    validationResults.RBACValid = true

    // Step 2: Validate target resource exists (BR-EXEC-062)
    exists, err := r.validateResourceExists(ctx, ke.Spec.TargetCluster, ke.Spec.ActionParams)
    if err != nil || !exists {
        validationResults.ResourceExists = false
        validationResults.ErrorMessage = "Target resource not found"
        return r.transitionToFailed(ctx, ke, validationResults)
    }
    validationResults.ResourceExists = true

    // Step 3: Perform dry-run (BR-EXEC-061)
    dryRunSuccess, dryRunOutput, err := r.executeDryRun(ctx, ke)
    if err != nil || !dryRunSuccess {
        validationResults.DryRunPassed = false
        validationResults.ErrorMessage = fmt.Sprintf("Dry-run failed: %v", err)
        validationResults.DryRunOutput = dryRunOutput
        return r.transitionToFailed(ctx, ke, validationResults)
    }
    validationResults.DryRunPassed = true
    validationResults.DryRunOutput = dryRunOutput

    // All validations passed - transition to executing
    ke.Status.ValidationResults = validationResults
    ke.Status.Phase = "executing"

    return ctrl.Result{}, r.Status().Update(ctx, ke)
}
```

**CRD Status Update**:
```yaml
status:
  phase: executing
  validationResults:
    rbacValid: true
    resourceExists: true
    dryRunPassed: true
    dryRunOutput: "deployment.apps/payment-api scaled (dry run)"
  validationTime: "2025-01-15T10:35:20Z"
```

**Duration**: ~5 seconds (RBAC check + resource check + dry-run)

---

### **Step 6.3: Execute Kubernetes Action**

**Controller**: KubernetesExecution Reconciler
**Phase**: `executing`
**Business Requirements**: BR-EXEC-001, BR-EXEC-002, BR-EXEC-070

**Execution via Kubernetes Job**:
```go
func (r *KubernetesExecutionReconciler) handleExecutingPhase(
    ctx context.Context,
    ke *executorv1.KubernetesExecution,
) (ctrl.Result, error) {
    // Step 1: Create Kubernetes Job for action execution
    job, err := r.createExecutionJob(ctx, ke)
    if err != nil {
        return r.transitionToFailed(ctx, ke, &executorv1.ExecutionResults{
            Success:      false,
            ErrorMessage: fmt.Sprintf("Failed to create execution job: %v", err),
        })
    }

    // Step 2: Store job reference
    ke.Status.JobRef = &corev1.ObjectReference{
        Kind:      "Job",
        Name:      job.Name,
        Namespace: job.Namespace,
        UID:       job.UID,
    }

    if err := r.Status().Update(ctx, ke); err != nil {
        return ctrl.Result{}, err
    }

    // Step 3: Watch job completion
    return ctrl.Result{RequeueAfter: 5 * time.Second}, nil
}
```

**Create Kubernetes Job**:
```go
func (r *KubernetesExecutionReconciler) createExecutionJob(
    ctx context.Context,
    ke *executorv1.KubernetesExecution,
) (*batchv1.Job, error) {
    saName := getServiceAccountForAction(ke.Spec.ActionType)

    // Build kubectl command
    kubectlCmd := buildKubectlCommand(ke.Spec.ActionType, ke.Spec.ActionParams)

    job := &batchv1.Job{
        ObjectMeta: metav1.ObjectMeta{
            Name:      fmt.Sprintf("ke-%s-%s", ke.Spec.ActionType, ke.Name[:8]),
            Namespace: "kubernaut-system",
        },
        Spec: batchv1.JobSpec{
            BackoffLimit: pointer.Int32(0), // No retries for safety
            Template: corev1.PodTemplateSpec{
                Spec: corev1.PodSpec{
                    ServiceAccountName: saName,
                    RestartPolicy:      corev1.RestartPolicyNever,
                    Containers: []corev1.Container{
                        {
                            Name:    "kubectl-executor",
                            Image:   "bitnami/kubectl:latest",
                            Command: []string{"/bin/sh", "-c"},
                            Args:    []string{kubectlCmd},
                            Resources: corev1.ResourceRequirements{
                                Requests: corev1.ResourceList{
                                    corev1.ResourceCPU:    resource.MustParse("100m"),
                                    corev1.ResourceMemory: resource.MustParse("128Mi"),
                                },
                                Limits: corev1.ResourceList{
                                    corev1.ResourceCPU:    resource.MustParse("500m"),
                                    corev1.ResourceMemory: resource.MustParse("256Mi"),
                                },
                            },
                        },
                    },
                },
            },
        },
    }

    if err := r.Create(ctx, job); err != nil {
        return nil, fmt.Errorf("failed to create job: %w", err)
    }

    return job, nil
}
```

**Example kubectl Command**:
```bash
kubectl scale deployment payment-api --namespace=production --replicas=5
```

**Job Created**:
```yaml
apiVersion: batch/v1
kind: Job
metadata:
  name: ke-scale-deployment-abc123
  namespace: kubernaut-system
spec:
  backoffLimit: 0
  template:
    spec:
      serviceAccountName: scale-deployment-sa
      restartPolicy: Never
      containers:
      - name: kubectl-executor
        image: bitnami/kubectl:latest
        command: ["/bin/sh", "-c"]
        args:
        - "kubectl scale deployment payment-api --namespace=production --replicas=5"
        resources:
          requests:
            cpu: "100m"
            memory: "128Mi"
          limits:
            cpu: "500m"
            memory: "256Mi"
```

**Duration**: ~10-30 seconds (job creation + execution)

---

### **Step 6.4: Monitor Job Completion and Validate Outcome**

**Controller**: KubernetesExecution Reconciler
**Phase**: `executing`
**Business Requirements**: BR-EXEC-027 (verify outcomes), BR-EXEC-029 (post-action health checks)

**Watch Job Status**:
```go
func (r *KubernetesExecutionReconciler) monitorJobCompletion(
    ctx context.Context,
    ke *executorv1.KubernetesExecution,
) (ctrl.Result, error) {
    // Fetch job status
    job := &batchv1.Job{}
    err := r.Get(ctx, client.ObjectKey{
        Name:      ke.Status.JobRef.Name,
        Namespace: ke.Status.JobRef.Namespace,
    }, job)

    if err != nil {
        return ctrl.Result{}, fmt.Errorf("failed to fetch job: %w", err)
    }

    // Check job status
    if job.Status.Succeeded > 0 {
        // Job succeeded - now validate expected outcome (BR-EXEC-027)
        return r.validateExpectedOutcome(ctx, ke)
    }

    if job.Status.Failed > 0 {
        // Job failed - extract error and transition to failed
        podLogs, _ := r.getJobPodLogs(ctx, job)
        return r.transitionToFailed(ctx, ke, &executorv1.ExecutionResults{
            Success:      false,
            ErrorMessage: "Execution job failed",
            JobOutput:    podLogs,
        })
    }

    // Job still running - check timeout
    if time.Since(ke.Status.StartTime.Time) > ke.Spec.Timeout.Duration {
        return r.transitionToFailed(ctx, ke, &executorv1.ExecutionResults{
            Success:      false,
            ErrorMessage: "Execution timeout exceeded",
        })
    }

    // Requeue to check again
    return ctrl.Result{RequeueAfter: 5 * time.Second}, nil
}
```

**Validate Expected Outcome** (BR-EXEC-027, BR-EXEC-029):
```go
func (r *KubernetesExecutionReconciler) validateExpectedOutcome(
    ctx context.Context,
    ke *executorv1.KubernetesExecution,
) (ctrl.Result, error) {
    // BR-EXEC-027: Verify action outcome against expected result
    switch ke.Spec.ActionType {
    case "scale_deployment":
        return r.validateScaleDeployment(ctx, ke)
    case "restart_pod":
        return r.validateRestartPod(ctx, ke)
    case "patch_configmap":
        return r.validatePatchConfigMap(ctx, ke)
    default:
        return r.transitionToFailed(ctx, ke, &executorv1.ExecutionResults{
            Success:      false,
            ErrorMessage: fmt.Sprintf("Unknown action type: %s", ke.Spec.ActionType),
        })
    }
}

func (r *KubernetesExecutionReconciler) validateScaleDeployment(
    ctx context.Context,
    ke *executorv1.KubernetesExecution,
) (ctrl.Result, error) {
    deploymentName := ke.Spec.ActionParams["deployment"].(string)
    namespace := ke.Spec.ActionParams["namespace"].(string)
    expectedReplicas := int32(ke.Spec.ActionParams["replicas"].(float64))

    // BR-EXEC-027: Verify deployment scaled to expected replicas
    deployment := &appsv1.Deployment{}
    err := r.Get(ctx, client.ObjectKey{
        Name:      deploymentName,
        Namespace: namespace,
    }, deployment)

    if err != nil {
        return r.transitionToFailed(ctx, ke, &executorv1.ExecutionResults{
            Success:      false,
            ErrorMessage: fmt.Sprintf("Failed to verify deployment: %v", err),
        })
    }

    // Verify replica count
    if *deployment.Spec.Replicas != expectedReplicas {
        return r.transitionToFailed(ctx, ke, &executorv1.ExecutionResults{
            Success:      false,
            ErrorMessage: fmt.Sprintf(
                "Validation failed: expected %d replicas, got %d",
                expectedReplicas, *deployment.Spec.Replicas),
        })
    }

    // BR-EXEC-029: Post-action health check - verify replicas are ready
    if deployment.Status.ReadyReplicas != expectedReplicas {
        // Wait for replicas to become ready
        if time.Since(ke.Status.StartTime.Time) < 2*time.Minute {
            return ctrl.Result{RequeueAfter: 10 * time.Second}, nil
        }

        return r.transitionToFailed(ctx, ke, &executorv1.ExecutionResults{
            Success:      false,
            ErrorMessage: fmt.Sprintf(
                "Health check failed: only %d of %d replicas ready",
                deployment.Status.ReadyReplicas, expectedReplicas),
        })
    }

    // Success - all replicas scaled and ready
    return r.transitionToCompleted(ctx, ke, &executorv1.ExecutionResults{
        Success:           true,
        Message:           fmt.Sprintf("Successfully scaled %s to %d replicas", deploymentName, expectedReplicas),
        ValidationResult:  fmt.Sprintf("Verified %d replicas running and ready", expectedReplicas),
        ExecutionDuration: time.Since(ke.Status.StartTime.Time),
    })
}
```

**Transition to Completed**:
```go
func (r *KubernetesExecutionReconciler) transitionToCompleted(
    ctx context.Context,
    ke *executorv1.KubernetesExecution,
    results *executorv1.ExecutionResults,
) (ctrl.Result, error) {
    ke.Status.Phase = "completed"
    ke.Status.CompletionTime = metav1.Now()
    ke.Status.ExecutionResults = results

    // Store in audit database
    r.storeExecutionAudit(ctx, ke)

    // Emit event
    r.Recorder.Event(ke, corev1.EventTypeNormal, "ExecutionCompleted",
        fmt.Sprintf("Action %s completed successfully", ke.Spec.ActionType))

    return ctrl.Result{}, r.Status().Update(ctx, ke)
}
```

**KubernetesExecution Status Update** (Final):
```yaml
status:
  phase: completed
  completionTime: "2025-01-15T10:35:45Z"
  executionResults:
    success: true
    message: "Successfully scaled payment-api to 5 replicas"
    validationResult: "Verified 5 replicas running and ready"
    executionDuration: "30s"
  jobRef:
    kind: "Job"
    name: "ke-scale-deployment-abc123"
    namespace: "kubernaut-system"
```

**Duration**: ~30 seconds (execution + validation + health check)

**Critical**: This status update triggers a watch event in the WorkflowExecution Controller.

---

## **PHASE 7: WORKFLOW MONITORING** (WorkflowExecution Controller)

### **Step 7.1: WorkflowExecution Detects Step Completion**

**Controller**: WorkflowExecution Reconciler
**CRD**: `WorkflowExecution`
**Phase**: `executing`
**Trigger**: Watch on `KubernetesExecution` status changes

**Watch Pattern**:
```go
// In WorkflowExecution controller setup
err = c.Watch(
    &source.Kind{Type: &executorv1.KubernetesExecution{}},
    handler.EnqueueRequestsFromMapFunc(r.kubernetesExecutionToWorkflow),
)
```

**Reconciliation Logic**:
```go
func (r *WorkflowExecutionReconciler) handleExecutingPhase(
    ctx context.Context,
    workflow *workflowv1.WorkflowExecution,
) (ctrl.Result, error) {
    // Check all step statuses
    allStepsComplete := true
    anyStepFailed := false

    for i, stepStatus := range workflow.Status.StepStatuses {
        // Fetch KubernetesExecution status
        ke := &executorv1.KubernetesExecution{}
        err := r.Get(ctx, client.ObjectKey{
            Name:      stepStatus.ExecutionRef.Name,
            Namespace: stepStatus.ExecutionRef.Namespace,
        }, ke)

        if err != nil {
            return ctrl.Result{}, fmt.Errorf("failed to fetch KubernetesExecution: %w", err)
        }

        // Update step status from KubernetesExecution
        workflow.Status.StepStatuses[i].Phase = ke.Status.Phase
        workflow.Status.StepStatuses[i].CompletionTime = ke.Status.CompletionTime
        workflow.Status.StepStatuses[i].ExecutionResults = ke.Status.ExecutionResults

        // Check completion
        if ke.Status.Phase != "completed" && ke.Status.Phase != "failed" {
            allStepsComplete = false
        }

        if ke.Status.Phase == "failed" {
            anyStepFailed = true
        }
    }

    if err := r.Status().Update(ctx, workflow); err != nil {
        return ctrl.Result{}, err
    }

    // All steps complete - transition to monitoring phase
    if allStepsComplete && !anyStepFailed {
        workflow.Status.Phase = "monitoring"
        return ctrl.Result{}, r.Status().Update(ctx, workflow)
    }

    // Step failed - handle failure
    if anyStepFailed {
        return r.handleStepFailure(ctx, workflow)
    }

    // Check for next batch of steps
    nextBatch := r.getNextExecutionBatch(workflow, workflow.Status.ExecutionPlan)
    if nextBatch != nil {
        // Create KubernetesExecution for next batch
        return r.createNextBatchExecutions(ctx, workflow, nextBatch)
    }

    // Requeue to check again
    return ctrl.Result{RequeueAfter: 10 * time.Second}, nil
}
```

**WorkflowExecution Status Update**:
```yaml
status:
  phase: monitoring
  currentStep: 1
  totalSteps: 1
  stepStatuses:
  - stepName: "scale-deployment"
    phase: "completed"
    startTime: "2025-01-15T10:35:15Z"
    completionTime: "2025-01-15T10:35:45Z"
    executionResults:
      success: true
      message: "Successfully scaled payment-api to 5 replicas"
      validationResult: "Verified 5 replicas running and ready"
    executionRef:
      kind: "KubernetesExecution"
      name: "rem-req-abc123-workflow-scale-deployment"
```

**Duration**: <100ms (status aggregation)

---

### **Step 7.2: Post-Workflow Monitoring**

**Controller**: WorkflowExecution Reconciler
**Phase**: `monitoring`
**Business Requirements**: BR-WF-050, BR-WF-051, BR-WF-052

**Monitoring Actions**:
```go
func (r *WorkflowExecutionReconciler) handleMonitoringPhase(
    ctx context.Context,
    workflow *workflowv1.WorkflowExecution,
) (ctrl.Result, error) {
    // Monitor workflow outcomes for 1 minute after completion
    monitoringDuration := 1 * time.Minute

    if time.Since(workflow.Status.StepStatuses[len(workflow.Status.StepStatuses)-1].CompletionTime.Time) < monitoringDuration {
        // Still monitoring - verify resources are stable
        for _, stepStatus := range workflow.Status.StepStatuses {
            stable, err := r.verifyResourceStability(ctx, stepStatus)
            if err != nil || !stable {
                // Resource became unstable - may need rollback
                return r.handlePostExecutionFailure(ctx, workflow)
            }
        }

        // Requeue to continue monitoring
        return ctrl.Result{RequeueAfter: 10 * time.Second}, nil
    }

    // Monitoring period complete - transition to completed
    workflow.Status.Phase = "completed"
    workflow.Status.CompletionTime = metav1.Now()

    // Store audit trail
    r.storeWorkflowAudit(ctx, workflow)

    // Emit event
    r.Recorder.Event(workflow, corev1.EventTypeNormal, "WorkflowCompleted",
        "Workflow execution completed successfully")

    return ctrl.Result{}, r.Status().Update(ctx, workflow)
}
```

**WorkflowExecution Status Update** (Final):
```yaml
status:
  phase: completed
  completionTime: "2025-01-15T10:36:50Z"
  totalSteps: 1
  currentStep: 1
  executionPlan:
    strategy: "sequential"
    estimatedDuration: "5m"
    actualDuration: "1m35s"
  stepStatuses:
  - stepName: "scale-deployment"
    phase: "completed"
    startTime: "2025-01-15T10:35:15Z"
    completionTime: "2025-01-15T10:35:45Z"
    executionResults:
      success: true
      message: "Successfully scaled payment-api to 5 replicas"
      validationResult: "Verified 5 replicas running and ready"
  overallSuccess: true
```

**Duration**: ~1 minute (monitoring period)

---

## **PHASE 8: ORCHESTRATOR DETECTS WORKFLOW COMPLETION**

### **Step 8.1: RemediationOrchestrator Updates RemediationRequest**

**Controller**: RemediationOrchestrator Reconciler
**CRD**: `RemediationRequest`
**Trigger**: Watch on `WorkflowExecution` status changes

**Watch Pattern**:
```go
err = c.Watch(
    &source.Kind{Type: &workflowv1.WorkflowExecution{}},
    handler.EnqueueRequestsFromMapFunc(r.workflowExecutionToRemediationRequest),
)
```

**Reconciliation Logic**:
```go
func (r *RemediationRequestReconciler) updateFromWorkflowCompletion(
    ctx context.Context,
    remediationRequest *remediationv1.RemediationRequest,
    workflow *workflowv1.WorkflowExecution,
) (ctrl.Result, error) {
    // Update RemediationRequest with workflow results
    remediationRequest.Status.Phase = "completed"
    remediationRequest.Status.CompletionTime = workflow.Status.CompletionTime
    remediationRequest.Status.WorkflowResults = &remediationv1.WorkflowResults{
        Success:       workflow.Status.OverallSuccess,
        StepsExecuted: workflow.Status.TotalSteps,
        Duration:      workflow.Status.ExecutionPlan.ActualDuration,
    }

    // Store in audit database
    r.storeRemediationAudit(ctx, remediationRequest)

    // Emit event
    r.Recorder.Event(remediationRequest, corev1.EventTypeNormal, "RemediationCompleted",
        "Alert remediation workflow completed successfully")

    // Schedule cleanup after 24 hours (BR-AR-066)
    r.scheduleCleanup(remediationRequest, 24*time.Hour)

    return ctrl.Result{}, r.Status().Update(ctx, remediationRequest)
}
```

**RemediationRequest Status Update** (Final):
```yaml
status:
  phase: completed
  completionTime: "2025-01-15T10:36:50Z"
  remediationProcessingRef:
    name: "rem-req-abc123-processing"
  aiAnalysisRef:
    name: "rem-req-abc123-analysis"
  workflowRef:
    name: "rem-req-abc123-workflow"
  workflowResults:
    success: true
    stepsExecuted: 1
    duration: "1m35s"
  cleanupScheduledAt: "2025-01-16T10:36:50Z"
```

---

## 📊 **COMPLETE TIMELINE SUMMARY**

| Phase | Controller | Duration | Business Requirements |
|-------|-----------|----------|----------------------|
| **1. AI Analysis Completion** | AIAnalysis | ~5s | BR-AI-006 to BR-AI-014 |
| **2. Orchestrator Watch** | RemediationOrchestrator | <100ms | BR-AR-062, BR-AR-063 |
| **3. Create WorkflowExecution** | RemediationOrchestrator | ~200ms | BR-AR-064, BR-AR-065 |
| **4. Workflow Planning** | WorkflowExecution | ~5s | BR-WF-001, BR-WF-002, BR-WF-010 |
| **5. Workflow Validation** | WorkflowExecution | ~30s | BR-WF-015 to BR-WF-019 |
| **6. Create KubernetesExecution** | WorkflowExecution | ~200ms | BR-WF-030, BR-WF-031 |
| **7. Executor Validation** | KubernetesExecution | ~5s | BR-EXEC-026, BR-EXEC-060 to BR-EXEC-063 |
| **8. Action Execution** | KubernetesExecution | ~30s | BR-EXEC-001, BR-EXEC-002 |
| **9. Outcome Validation** | KubernetesExecution | ~10s | BR-EXEC-027, BR-EXEC-029 |
| **10. Workflow Monitoring** | WorkflowExecution | ~1min | BR-WF-050, BR-WF-051 |
| **11. Orchestrator Update** | RemediationOrchestrator | <100ms | BR-AR-066 |

**Total Duration**: ~2 minutes 30 seconds (from AI completion to workflow completion)

---

## 🔑 **KEY ARCHITECTURAL PATTERNS**

### **1. Watch-Based Event-Driven Coordination**
- No polling or HTTP calls between controllers
- Kubernetes watch events trigger reconciliation
- Each controller watches child CRD status changes
- Parent controllers react to child completion via watches

### **2. Owner Reference Cascade Deletion**
- RemediationRequest owns WorkflowExecution
- WorkflowExecution owns KubernetesExecution
- Deleting parent auto-deletes all children
- Clean lifecycle management with 24-hour retention

### **3. Data Snapshot Pattern**
- Parent copies complete data to child spec
- Child is self-contained (no lookups needed)
- Clear data flow: status → next spec
- No circular dependencies

### **4. Validation Responsibility Chain** (ADR-016)
- Each layer trusts previous layer's data/status
- AI recommendations are authoritative (no revalidation)
- Steps validate expected outcomes
- Workflow relies on step status (no redundant checks)

### **5. Step-Level Validation with Expected Outcomes** (BR-EXEC-027, BR-EXEC-029)
- KubernetesExecution executes action + validates outcome
- Examples: Scale → verify replicas, Delete → verify gone
- WorkflowExecution monitors step status (no direct K8s queries)
- Clear separation: execution validates, orchestration monitors

---

## ✅ **BUSINESS REQUIREMENTS ALIGNMENT**

### **AI Analysis** (BR-AI-001 to BR-AI-014)
✅ Generate and rank recommendations
✅ Apply safety constraints
✅ Calculate historical success rates
✅ Transition to "completed" phase

### **Workflow Planning** (BR-WF-001 to BR-WF-019)
✅ Parse AI recommendations (authoritative)
✅ Build dependency graph
✅ Calculate execution order
✅ Validate safety constraints
✅ Perform dry-run if configured

### **Workflow Execution** (BR-WF-030 to BR-WF-050)
✅ Create KubernetesExecution CRDs
✅ Monitor step completion via watches
✅ Aggregate step results
✅ Post-workflow monitoring

### **Kubernetes Execution** (BR-EXEC-001 to BR-EXEC-030)
✅ Validate RBAC and resources (BR-EXEC-026)
✅ Execute action via Kubernetes Job
✅ Verify expected outcome (BR-EXEC-027)
✅ Perform post-action health check (BR-EXEC-029)
✅ Update status with validation results

### **Remediation Orchestration** (BR-AR-061 to BR-AR-067)
✅ Watch-based event coordination
✅ Status aggregation across CRDs
✅ Sequential CRD creation
✅ 24-hour retention with cleanup

---

## 📚 **REFERENCES**

- **Business Requirements**: `docs/requirements/04_WORKFLOW_ENGINE_ORCHESTRATION.md`
- **AI Analysis Service**: `docs/services/crd-controllers/02-aianalysis/`
- **Workflow Execution Service**: `docs/services/crd-controllers/03-workflowexecution/`
- **Kubernetes Executor Service**: `docs/services/crd-controllers/04-kubernetesexecutor/`
- **Remediation Orchestrator**: `docs/services/crd-controllers/05-remediationorchestrator/`
- **Validation Chain**: `docs/architecture/decisions/ADR-016-validation-responsibility-chain.md`

---

**Document Status**: ✅ **COMPLETE** - Comprehensive step-by-step flow from AI Analysis to Workflow Execution
