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

## Detailed Component Analysis

### 1. Existing Workflow Engine Interfaces

**Current Implementation**: `pkg/workflow/engine/interfaces.go`

```go
// Current workflow engine interface (reusable)
type WorkflowEngine interface {
    CreateWorkflow(ctx context.Context, request WorkflowRequest) (*Workflow, error)
    ExecuteWorkflow(ctx context.Context, workflow *Workflow) error
    ValidateWorkflow(ctx context.Context, workflow *Workflow) error
}

type Workflow struct {
    Name    string
    Version string
    Steps   []WorkflowStep
}

type WorkflowStep struct {
    Name       string
    Action     string
    Parameters map[string]interface{}
    Timeout    time.Duration
}
```

**Reusability**: 85% - Core structures are solid, need CRD integration

**Migration Path**:
```go
// New CRD-based workflow execution
type WorkflowExecutionReconciler struct {
    client.Client
    Scheme         *runtime.Scheme
    WorkflowEngine engine.WorkflowEngine // Reuse existing engine
}

func (r *WorkflowExecutionReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
    var workflowExec workflowv1.WorkflowExecution
    if err := r.Get(ctx, req.NamespacedName, &workflowExec); err != nil {
        return ctrl.Result{}, client.IgnoreNotFound(err)
    }

    // Reuse existing workflow engine for step logic
    workflow := convertCRDToWorkflow(&workflowExec)
    
    // Execute via existing engine (reusable code)
    if err := r.WorkflowEngine.ExecuteWorkflow(ctx, workflow); err != nil {
        return ctrl.Result{}, err
    }

    return ctrl.Result{}, nil
}

// Convert CRD to engine workflow (adapter pattern)
func convertCRDToWorkflow(workflowExec *workflowv1.WorkflowExecution) *engine.Workflow {
    workflow := &engine.Workflow{
        Name:    workflowExec.Name,
        Version: workflowExec.Spec.WorkflowVersion,
        Steps:   make([]engine.WorkflowStep, len(workflowExec.Spec.Steps)),
    }
    
    for i, crdStep := range workflowExec.Spec.Steps {
        workflow.Steps[i] = engine.WorkflowStep{
            Name:       crdStep.Name,
            Action:     crdStep.Action.Type,
            Parameters: crdStep.Action.Parameters,
            Timeout:    crdStep.Timeout.Duration,
        }
    }
    
    return workflow
}
```

**Changes Required**:
1. **Add CRD adapter**: Convert WorkflowExecution CRD to engine.Workflow
2. **Add step orchestration**: Create KubernetesExecution CRDs per step
3. **Add status tracking**: Watch KubernetesExecution status and update workflow status
4. **Add rollback handling**: Execute rollback steps on failure

**Estimated Effort**: 2-3 days

---

### 2. Workflow Template Management

**Current Implementation**: `pkg/workflow/templates/template_manager.go`

```go
// Current template manager (highly reusable)
type TemplateManager interface {
    GetTemplate(ctx context.Context, name string, version string) (*WorkflowTemplate, error)
    ValidateTemplate(template *WorkflowTemplate) error
}

type WorkflowTemplate struct {
    Name    string
    Version string
    Steps   []StepTemplate
}

type StepTemplate struct {
    Name           string
    ActionType     string
    ParameterSchema map[string]ParameterDefinition
    Timeout        time.Duration
    Retryable      bool
    Rollback       *RollbackConfig
}
```

**Reusability**: 90% - Excellent template structure, minimal changes needed

**Migration Path**:
```go
// Integrate template manager into controller
func (r *WorkflowExecutionReconciler) loadWorkflowFromAIAnalysis(
    ctx context.Context,
    analysis *aianalysisv1.AIAnalysis,
) (*workflowv1.WorkflowExecutionSpec, error) {
    // AIAnalysis provides recommended workflow name + version
    templateName := analysis.Status.RecommendedWorkflow.Name
    templateVersion := analysis.Status.RecommendedWorkflow.Version

    // Reuse existing template manager
    template, err := r.TemplateManager.GetTemplate(ctx, templateName, templateVersion)
    if err != nil {
        return nil, fmt.Errorf("failed to load workflow template: %w", err)
    }

    // Convert template to CRD spec
    spec := &workflowv1.WorkflowExecutionSpec{
        WorkflowName:    template.Name,
        WorkflowVersion: template.Version,
        Steps:           make([]workflowv1.WorkflowStep, len(template.Steps)),
    }

    for i, stepTemplate := range template.Steps {
        spec.Steps[i] = workflowv1.WorkflowStep{
            Name: stepTemplate.Name,
            Action: workflowv1.ActionSpec{
                Type:       stepTemplate.ActionType,
                Parameters: fillParametersFromAnalysis(stepTemplate, analysis),
            },
            Timeout:   metav1.Duration{Duration: stepTemplate.Timeout},
            Retryable: stepTemplate.Retryable,
        }
    }

    return spec, nil
}

// Fill step parameters from AI analysis (targeting data)
func fillParametersFromAnalysis(stepTemplate *StepTemplate, analysis *aianalysisv1.AIAnalysis) json.RawMessage {
    params := make(map[string]interface{})
    
    // Extract target resource from AI analysis
    if analysis.Status.TargetResource != nil {
        params["namespace"] = analysis.Status.TargetResource.Namespace
        params["resourceKind"] = analysis.Status.TargetResource.Kind
        params["resourceName"] = analysis.Status.TargetResource.Name
    }
    
    // Add action-specific parameters from template defaults
    for key, paramDef := range stepTemplate.ParameterSchema {
        if paramDef.DefaultValue != nil {
            params[key] = paramDef.DefaultValue
        }
    }
    
    paramsJSON, _ := json.Marshal(params)
    return paramsJSON
}
```

**Changes Required**:
1. **Add CRD integration**: Load templates based on AIAnalysis recommendations
2. **Add parameter filling**: Populate parameters from AIAnalysis targeting data
3. **Preserve template logic**: Reuse existing validation and versioning

**Estimated Effort**: 1-2 days

---

### 3. Step Execution Logic

**Current Implementation**: `pkg/workflow/steps/executor.go`

```go
// Current step executor (needs refactoring for CRD-based execution)
type StepExecutor interface {
    ExecuteStep(ctx context.Context, step *WorkflowStep) (*StepResult, error)
    ValidateStep(ctx context.Context, step *WorkflowStep) error
}

// Direct execution pattern (needs to be Job-based)
func (e *DefaultStepExecutor) ExecuteStep(ctx context.Context, step *WorkflowStep) (*StepResult, error) {
    switch step.Action {
    case "scale-deployment":
        return e.scaleDeployment(ctx, step)
    case "restart-pod":
        return e.restartPod(ctx, step)
    // ... more actions
    }
}
```

**Reusability**: 60% - Step orchestration logic reusable, but execution pattern needs refactoring

**Migration Path**:
```go
// New CRD-based step orchestration
func (r *WorkflowExecutionReconciler) executeStep(
    ctx context.Context,
    workflow *workflowv1.WorkflowExecution,
    step *workflowv1.WorkflowStep,
    stepIndex int,
) error {
    // Create KubernetesExecution CRD for this step
    execution := &kubernetesexecutionv1.KubernetesExecution{
        ObjectMeta: metav1.ObjectMeta{
            Name:      fmt.Sprintf("%s-step-%d", workflow.Name, stepIndex),
            Namespace: workflow.Namespace,
            Labels: map[string]string{
                "workflow-execution": workflow.Name,
                "step-index":         fmt.Sprintf("%d", stepIndex),
            },
            OwnerReferences: []metav1.OwnerReference{
                *metav1.NewControllerRef(workflow, workflowv1.GroupVersion.WithKind("WorkflowExecution")),
            },
        },
        Spec: kubernetesexecutionv1.KubernetesExecutionSpec{
            WorkflowExecutionRef: kubernetesexecutionv1.WorkflowExecutionReference{
                Name:      workflow.Name,
                Namespace: workflow.Namespace,
            },
            StepIndex: stepIndex,
            Action:    step.Action,
            Timeout:   step.Timeout,
        },
    }

    // Create KubernetesExecution CRD (delegates to Kubernetes Executor)
    return r.Create(ctx, execution)
}

// Watch KubernetesExecution status and update workflow
func (r *WorkflowExecutionReconciler) watchStepExecution(
    ctx context.Context,
    workflow *workflowv1.WorkflowExecution,
    stepIndex int,
) error {
    executionName := fmt.Sprintf("%s-step-%d", workflow.Name, stepIndex)
    
    var execution kubernetesexecutionv1.KubernetesExecution
    if err := r.Get(ctx, client.ObjectKey{
        Name:      executionName,
        Namespace: workflow.Namespace,
    }, &execution); err != nil {
        return err
    }

    // Update step status based on execution status
    step := &workflow.Status.Steps[stepIndex]
    step.Phase = convertExecutionPhase(execution.Status.Phase)
    step.StartedAt = execution.Status.StartedAt
    step.CompletedAt = execution.Status.CompletedAt

    // Handle completion
    if execution.Status.Phase == "completed" {
        if execution.Status.Result.Success {
            // Step succeeded - proceed to next step
            return r.executeNextStep(ctx, workflow, stepIndex+1)
        } else {
            // Step failed - handle rollback or retry
            return r.handleStepFailure(ctx, workflow, stepIndex, execution.Status.Result.Error)
        }
    }

    // Still executing - requeue
    return nil
}
```

**Changes Required**:
1. **Refactor to CRD orchestration**: Create KubernetesExecution CRDs instead of direct execution
2. **Add status watching**: Monitor KubernetesExecution status
3. **Add step coordination**: Sequence steps based on dependencies
4. **Preserve retry logic**: Reuse existing retry patterns

**Estimated Effort**: 3-4 days

---

### 4. Rollback Strategy Implementation

**Current Implementation**: Partial rollback logic exists in `pkg/workflow/rollback/`

```go
// Current rollback interface (needs CRD integration)
type RollbackManager interface {
    PrepareRollback(ctx context.Context, workflow *Workflow, failedStepIndex int) (*RollbackPlan, error)
    ExecuteRollback(ctx context.Context, plan *RollbackPlan) error
}

type RollbackPlan struct {
    StepsToRollback []RollbackStep
}

type RollbackStep struct {
    OriginalStepIndex int
    Action            string
    Parameters        map[string]interface{}
}
```

**Reusability**: 75% - Rollback planning logic reusable, execution needs CRD integration

**Migration Path**:
```go
// CRD-based rollback execution
func (r *WorkflowExecutionReconciler) handleStepFailure(
    ctx context.Context,
    workflow *workflowv1.WorkflowExecution,
    failedStepIndex int,
    errorMessage string,
) error {
    // Check if rollback is enabled
    if !workflow.Spec.RollbackEnabled {
        workflow.Status.Phase = "failed"
        workflow.Status.Error = errorMessage
        return r.Status().Update(ctx, workflow)
    }

    // Reuse existing rollback manager to generate plan
    plan, err := r.RollbackManager.PrepareRollback(ctx, convertCRDToWorkflow(workflow), failedStepIndex)
    if err != nil {
        return fmt.Errorf("failed to prepare rollback: %w", err)
    }

    // Execute rollback steps via KubernetesExecution CRDs
    for i, rollbackStep := range plan.StepsToRollback {
        execution := &kubernetesexecutionv1.KubernetesExecution{
            ObjectMeta: metav1.ObjectMeta{
                Name:      fmt.Sprintf("%s-rollback-%d", workflow.Name, i),
                Namespace: workflow.Namespace,
                Labels: map[string]string{
                    "workflow-execution": workflow.Name,
                    "rollback":           "true",
                },
            },
            Spec: kubernetesexecutionv1.KubernetesExecutionSpec{
                Action: kubernetesexecutionv1.ActionSpec{
                    Type:       rollbackStep.Action,
                    Parameters: mustMarshalJSON(rollbackStep.Parameters),
                },
            },
        }

        if err := r.Create(ctx, execution); err != nil {
            return fmt.Errorf("failed to create rollback execution: %w", err)
        }
    }

    workflow.Status.Phase = "rolling-back"
    return r.Status().Update(ctx, workflow)
}
```

**Changes Required**:
1. **Add CRD-based rollback execution**: Create KubernetesExecution CRDs for rollback steps
2. **Preserve rollback planning**: Reuse existing rollback plan generation
3. **Add rollback monitoring**: Watch rollback execution status

**Estimated Effort**: 2-3 days

---

## Migration Strategy

### Phase 1: Foundation (Days 1-3)

**Goal**: Create CRD and controller skeleton with existing engine integration

1. **Generate CRD**:
   ```bash
   kubebuilder create api --group workflowexecution --version v1alpha1 --kind WorkflowExecution
   ```

2. **Define CRD schema** based on existing `Workflow` type
3. **Create controller skeleton** with basic reconciliation loop
4. **Integrate existing WorkflowEngine** via adapter pattern
5. **Add owner references** to WorkflowExecution CRD

**Deliverables**:
- `api/workflowexecution/v1alpha1/workflowexecution_types.go`
- `internal/controller/workflowexecution/workflowexecution_controller.go`
- Adapter functions (`convertCRDToWorkflow`, `convertWorkflowToCRD`)
- Basic unit tests

---

### Phase 2: Template Integration (Days 4-5)

**Goal**: Integrate template manager and parameter filling from AIAnalysis

1. **Add TemplateManager integration**
2. **Implement parameter filling from AIAnalysis**
3. **Add template validation**
4. **Test template loading and parameter filling**

**Deliverables**:
- `loadWorkflowFromAIAnalysis` function
- `fillParametersFromAnalysis` function
- Integration tests with AIAnalysis CRD

---

### Phase 3: Step Orchestration (Days 6-8)

**Goal**: Implement step-by-step execution via KubernetesExecution CRDs

1. **Implement `executeStep`** (create KubernetesExecution CRDs)
2. **Implement `watchStepExecution`** (monitor step status)
3. **Add step coordination** (sequential and parallel execution)
4. **Add retry logic** (configurable per step)
5. **Add timeout handling**

**Deliverables**:
- Step orchestration logic
- Status watching and updates
- Integration tests with KubernetesExecution CRD

---

### Phase 4: Rollback & Safety (Days 9-10)

**Goal**: Implement rollback logic and safety validation

1. **Integrate RollbackManager**
2. **Implement CRD-based rollback execution**
3. **Add rollback monitoring**
4. **Implement dry-run validation**
5. **Add production namespace protection**

**Deliverables**:
- Rollback execution logic
- Safety validation
- Rollback integration tests

---

### Phase 5: Testing & Documentation (Days 11-12)

**Goal**: Comprehensive testing and documentation

1. **Unit tests**: 70%+ coverage
2. **Integration tests**: Full workflow scenarios
3. **E2E tests**: Complete remediation flow (AIAnalysis → WorkflowExecution → KubernetesExecution)
4. **Performance tests**: Workflow execution duration
5. **Documentation updates**: Runbook, troubleshooting guide

**Deliverables**:
- Complete test suite
- Updated documentation
- Performance benchmarks

---

## Code Reuse Summary

| Component | Existing Lines | Reusable % | New Lines Needed | Total Lines |
|-----------|---------------|------------|------------------|-------------|
| Workflow engine interfaces | 200 | 85% | 50 | 250 |
| Template management | 300 | 90% | 100 | 400 |
| Step execution logic | 400 | 60% | 300 | 700 |
| Rollback logic | 200 | 75% | 150 | 350 |
| **Total** | **1,100** | **75%** | **600** | **1,700** |

**Overall Reusability**: 75% of existing code is reusable with CRD integration
**Estimated New Code**: 600 lines (controller + CRD orchestration + status watching)
**Total Code**: ~1,700 lines for complete implementation

---

## Risk Assessment

### High Risk

- **CRD orchestration complexity**: Sequential step execution with status watching requires careful coordination
  - **Mitigation**: Incremental development, extensive integration testing

### Medium Risk

- **Rollback reliability**: Rollback steps must be idempotent and validated
  - **Mitigation**: Comprehensive rollback testing, dry-run capability

- **Template parameter filling**: AIAnalysis targeting data must map correctly to step parameters
  - **Mitigation**: Schema validation, parameter validation tests

### Low Risk

- **Engine integration**: Adapter pattern is straightforward
  - **Mitigation**: Standard adapter pattern implementation

- **Status watching**: Standard Kubernetes watch pattern
  - **Mitigation**: Follow existing controller patterns

---

## Success Criteria

- [ ] WorkflowExecution CRD creates KubernetesExecution CRDs per step
- [ ] Status watching updates workflow phase based on step execution
- [ ] Sequential step execution with proper coordination
- [ ] Rollback execution on step failure (if enabled)
- [ ] Template loading from AIAnalysis recommendations
- [ ] Parameter filling from AIAnalysis targeting data
- [ ] 70%+ code coverage with unit tests
- [ ] Integration tests validate full workflow orchestration
- [ ] E2E tests confirm end-to-end remediation flow
- [ ] Documentation complete and accurate

---

## References

- **Existing Code**: `pkg/workflow/` (current implementation)
- **Target Structure**: `pkg/workflow/execution/` (new CRD controller)
- **Integration Points**: [integration-points.md](./integration-points.md)
- **Testing Strategy**: [testing-strategy.md](./testing-strategy.md)
- **Database Integration**: [database-integration.md](./database-integration.md)
- **Architecture**: [Multi-CRD Reconciliation](../../../architecture/MULTI_CRD_RECONCILIATION_ARCHITECTURE.md)
