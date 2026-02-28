## Current State & Migration Path

> **DEPRECATED**: KubernetesExecution CRD and KubernetesExecutor service were eliminated by ADR-025 and replaced by Tekton TaskRun via WorkflowExecution. This documentation is retained for historical reference only. API types and CRD manifests have been removed from the codebase.

### Existing Business Logic (Verified)

**Current Location**: `pkg/platform/executor/` (implementation exists)
**Target Location**: `pkg/kubernetesexecution/` (after refactor)

**Existing Components**:
```
pkg/platform/executor/
├── executor.go (245 lines)          ✅ Action execution interface
├── kubernetes_executor.go (418 lines) ✅ K8s action implementations
└── actions.go (182 lines)           ✅ Action type definitions
```

**Existing Tests**:
- `test/unit/platform/executor/` → `test/unit/kubernetesexecution/`
- `test/integration/kubernetes_operations/` → `test/integration/kubernetesexecution/`

### Component Reuse Mapping

| Existing Component | CRD Controller Usage | Reusability | Migration Effort | Notes |
|-------------------|---------------------|-------------|-----------------|-------|
| **Action Interface** | Action type definitions | 85% | Low | ✅ Adapt to typed parameters |
| **K8s Executor** | Job creation and monitoring | 60% | Medium | ⚠️ Refactor for Job-based execution |
| **Action Implementations** | Command building logic | 90% | Low | ✅ Reuse kubectl command generation |
| **Validation Logic** | Pre-execution validation | 75% | Medium | ✅ Add Rego policy integration |

### Implementation Gap Analysis

**What Exists (Verified)**:
- ✅ Basic action execution framework (pkg/platform/executor/)
- ✅ Kubernetes client integration
- ✅ Action type definitions
- ✅ Command building for common actions

**What's Missing (CRD V1 Requirements)**:
- ❌ KubernetesExecution CRD schema
- ❌ KubernetesExecutionReconciler controller
- ❌ Native Kubernetes Job creation and lifecycle management
- ❌ Per-action ServiceAccount creation and RBAC
- ❌ Rego policy integration for safety validation
- ❌ Dry-run validation Jobs
- ❌ Rollback information extraction and storage
- ❌ Approval gate handling
- ❌ Comprehensive audit trail to database

**Code Quality Issues to Address**:
- ⚠️ **Refactor for Job-Based Execution**: Current implementation uses direct kubectl calls
  - Need to wrap all actions in Kubernetes Job specifications
  - Add Job monitoring and status tracking
  - Implement Job cleanup with TTL
  - Estimated effort: 3-4 days

**Estimated Migration Effort**: 10-15 days (2-3 weeks)
- Day 1-2: CRD schema + controller skeleton
- Day 3-5: Job creation and monitoring logic
- Day 6-8: Rego policy integration + validation
- Day 9-10: Per-action ServiceAccounts + RBAC
- Day 11-12: Testing and refinement
- Day 13-15: Integration with WorkflowExecution + audit

---

## Detailed Component Analysis

### 1. Existing Action Interface

**Current Implementation**: `pkg/platform/executor/executor.go:13-45`

```go
// Current interface (direct execution)
type ActionExecutor interface {
    Execute(ctx context.Context, action Action) (*ActionResult, error)
    Validate(ctx context.Context, action Action) error
    DryRun(ctx context.Context, action Action) (*DryRunResult, error)
}

type Action struct {
    Type       string
    Target     Target
    Parameters map[string]interface{} // ⚠️ Untyped parameters
}

type Target struct {
    Namespace    string
    ResourceKind string
    ResourceName string
}
```

**Reusability**: 85% - Structure is solid, needs typing improvements

**Migration Path**:
```go
// New CRD-based interface
type KubernetesExecutor interface {
    // Creates and monitors Job for action execution
    CreateExecutionJob(ctx context.Context, execution *executionv1.KubernetesExecution) (*batchv1.Job, error)
    MonitorJobStatus(ctx context.Context, job *batchv1.Job) (*JobStatus, error)
    CollectJobResult(ctx context.Context, job *batchv1.Job) (*ExecutionResult, error)

    // Reuses existing validation logic
    ValidateAction(ctx context.Context, actionType string, parameters json.RawMessage) error

    // Reuses existing dry-run logic
    CreateDryRunJob(ctx context.Context, execution *executionv1.KubernetesExecution) (*batchv1.Job, error)
}

// Typed action parameters (replaces map[string]interface{})
type ScaleDeploymentParams struct {
    Replicas int32 `json:"replicas" validate:"required,min=0,max=100"`
}

type RestartPodParams struct {
    GracePeriodSeconds int64 `json:"gracePeriodSeconds" validate:"min=0,max=300"`
}
```

**Changes Required**:
1. **Add JSON RawMessage for parameters**: Replace `map[string]interface{}` with typed structs
2. **Add Job creation logic**: Wrap kubectl commands in Job specs
3. **Add ServiceAccount handling**: Specify per-action ServiceAccount
4. **Add RBAC validation**: Check permissions before Job creation

**Estimated Effort**: 2-3 days

---

### 2. Kubernetes Executor Implementation

**Current Implementation**: `pkg/platform/executor/kubernetes_executor.go:23-418`

```go
// Current direct execution pattern
type KubernetesExecutor struct {
    clientset kubernetes.Interface
    config    *rest.Config
}

func (e *KubernetesExecutor) Execute(ctx context.Context, action Action) (*ActionResult, error) {
    switch action.Type {
    case "scale-deployment":
        return e.scaleDeployment(ctx, action.Target, action.Parameters)
    case "restart-pod":
        return e.restartPod(ctx, action.Target)
    case "rollback-deployment":
        return e.rollbackDeployment(ctx, action.Target)
    // ... more actions
    }
}

// Direct kubectl-style execution
func (e *KubernetesExecutor) scaleDeployment(ctx context.Context, target Target, params map[string]interface{}) (*ActionResult, error) {
    // Direct API call
    deployment, err := e.clientset.AppsV1().Deployments(target.Namespace).Get(ctx, target.ResourceName, metav1.GetOptions{})
    if err != nil {
        return nil, err
    }

    replicas := int32(params["replicas"].(float64))
    deployment.Spec.Replicas = &replicas

    _, err = e.clientset.AppsV1().Deployments(target.Namespace).Update(ctx, deployment, metav1.UpdateOptions{})
    return &ActionResult{Success: true}, err
}
```

**Reusability**: 60% - Command logic reusable, execution pattern needs refactoring

**Migration Path**:
```go
// New Job-based execution pattern
type JobBasedExecutor struct {
    client    client.Client
    scheme    *runtime.Scheme
    jobImages map[string]string // Action type -> Docker image
}

func (e *JobBasedExecutor) CreateExecutionJob(ctx context.Context, execution *executionv1.KubernetesExecution) (*batchv1.Job, error) {
    // Reuse existing command building logic
    command, args := e.buildActionCommand(execution.Spec.Action)

    job := &batchv1.Job{
        ObjectMeta: metav1.ObjectMeta{
            Name:      fmt.Sprintf("%s-job", execution.Name),
            Namespace: execution.Namespace,
            Labels: map[string]string{
                "kubernetes-execution": execution.Name,
                "action-type":          execution.Spec.Action.Type,
            },
            OwnerReferences: []metav1.OwnerReference{
                *metav1.NewControllerRef(execution, executionv1.GroupVersion.WithKind("KubernetesExecution")),
            },
        },
        Spec: batchv1.JobSpec{
            Template: corev1.PodTemplateSpec{
                Spec: corev1.PodSpec{
                    ServiceAccountName: e.getServiceAccountForAction(execution.Spec.Action.Type),
                    Containers: []corev1.Container{
                        {
                            Name:    "executor",
                            Image:   e.jobImages[execution.Spec.Action.Type],
                            Command: command,
                            Args:    args,
                            Env:     e.buildEnvVars(execution),
                        },
                    },
                    RestartPolicy: corev1.RestartPolicyNever,
                },
            },
            BackoffLimit: ptr.To(int32(0)), // Controller handles retries
        },
    }

    return job, e.client.Create(ctx, job)
}

// Reuse existing command building logic (minimal changes)
func (e *JobBasedExecutor) buildActionCommand(action executionv1.ActionSpec) ([]string, []string) {
    switch action.Type {
    case "scale-deployment":
        return []string{"kubectl"}, []string{
            "scale", "deployment",
            action.Target.ResourceName,
            "--replicas", string(action.Parameters["replicas"]),
            "-n", action.Target.Namespace,
        }
    case "restart-pod":
        return []string{"kubectl"}, []string{
            "delete", "pod",
            action.Target.ResourceName,
            "-n", action.Target.Namespace,
            "--grace-period", string(action.Parameters["gracePeriodSeconds"]),
        }
    // ... reuse existing command building logic
    }
}
```

**Changes Required**:
1. **Wrap in Jobs**: Create Job specs instead of direct API calls
2. **Add Job monitoring**: Watch Job status and collect results
3. **Add per-action images**: Map action types to Docker images
4. **Add ServiceAccount mapping**: Specify ServiceAccount per action type
5. **Preserve command logic**: Reuse existing kubectl command generation

**Estimated Effort**: 3-4 days

---

### 3. Action Type Definitions

**Current Implementation**: `pkg/platform/executor/actions.go:15-182`

```go
// Current action catalog
const (
    ActionScaleDeployment     = "scale-deployment"
    ActionRestartPod          = "restart-pod"
    ActionRollbackDeployment  = "rollback-deployment"
    ActionCordonNode          = "cordon-node"
    ActionDrainNode           = "drain-node"
    ActionCollectLogs         = "collect-logs"
    ActionApplyManifest       = "apply-manifest"
    ActionDeleteResource      = "delete-resource"
)

// Action parameter schemas (informal, needs formalization)
var ActionSchemas = map[string]map[string]string{
    ActionScaleDeployment: {
        "replicas": "int32, required, min=0, max=100",
    },
    ActionRestartPod: {
        "gracePeriodSeconds": "int64, optional, default=30",
    },
    // ... more schemas
}
```

**Reusability**: 90% - Excellent foundation, needs JSON schema formalization

**Migration Path**:
```go
// Formalize with typed structs and validation tags
package actions

// Predefined action types (keep existing constants)
const (
    ActionScaleDeployment     = "scale-deployment"
    ActionRestartPod          = "restart-pod"
    ActionRollbackDeployment  = "rollback-deployment"
    ActionCordonNode          = "cordon-node"
    ActionCollectLogs         = "collect-logs"
)

// Typed parameter structs with validation
type ScaleDeploymentParams struct {
    Replicas int32 `json:"replicas" validate:"required,min=0,max=100"`
}

type RestartPodParams struct {
    GracePeriodSeconds int64 `json:"gracePeriodSeconds" validate:"min=0,max=300"`
}

type RollbackDeploymentParams struct {
    ToRevision int64 `json:"toRevision" validate:"min=1"`
}

// Action metadata for Job creation
type ActionMetadata struct {
    Type            string
    Image           string
    ServiceAccount  string
    DefaultTimeout  metav1.Duration
    RBACPermissions []rbacv1.PolicyRule
}

var ActionCatalog = map[string]ActionMetadata{
    ActionScaleDeployment: {
        Type:           ActionScaleDeployment,
        Image:          "kubernaut/action-scale:v1",
        ServiceAccount: "action-scale-deployment",
        DefaultTimeout: metav1.Duration{Duration: 30 * time.Second},
        RBACPermissions: []rbacv1.PolicyRule{
            {
                APIGroups: []string{"apps"},
                Resources: []string{"deployments", "deployments/scale"},
                Verbs:     []string{"get", "patch", "update"},
            },
        },
    },
    ActionRestartPod: {
        Type:           ActionRestartPod,
        Image:          "kubernaut/action-restart:v1",
        ServiceAccount: "action-restart-pod",
        DefaultTimeout: metav1.Duration{Duration: 60 * time.Second},
        RBACPermissions: []rbacv1.PolicyRule{
            {
                APIGroups: []string{""},
                Resources: []string{"pods", "pods/eviction"},
                Verbs:     []string{"get", "delete", "create"},
            },
        },
    },
    // ... more actions
}
```

**Changes Required**:
1. **Add typed parameter structs**: Replace informal schemas
2. **Add validation tags**: Use `validate` struct tags
3. **Add action metadata**: Docker images, ServiceAccounts, RBAC, timeouts
4. **Formalize catalog**: Machine-readable action definitions

**Estimated Effort**: 1-2 days

---

### 4. Validation Logic

**Current Implementation**: Various validation scattered across files

```go
// Current ad-hoc validation
func (e *KubernetesExecutor) Validate(ctx context.Context, action Action) error {
    // Basic checks
    if action.Type == "" {
        return fmt.Errorf("action type is required")
    }
    if action.Target.Namespace == "" {
        return fmt.Errorf("target namespace is required")
    }

    // Type-specific validation (inconsistent)
    switch action.Type {
    case "scale-deployment":
        replicas, ok := action.Parameters["replicas"]
        if !ok {
            return fmt.Errorf("replicas parameter required")
        }
        if r, ok := replicas.(float64); ok && (r < 0 || r > 100) {
            return fmt.Errorf("replicas must be 0-100")
        }
    }

    return nil
}
```

**Reusability**: 75% - Logic is sound, needs formalization

**Migration Path**:
```go
// Centralized validation with validator library
import "github.com/go-playground/validator/v10"

type ActionValidator struct {
    validate *validator.Validate
}

func (v *ActionValidator) ValidateAction(actionType string, parameters json.RawMessage) error {
    metadata, ok := ActionCatalog[actionType]
    if !ok {
        return fmt.Errorf("unknown action type: %s", actionType)
    }

    // Unmarshal into typed struct
    var params interface{}
    switch actionType {
    case ActionScaleDeployment:
        params = &ScaleDeploymentParams{}
    case ActionRestartPod:
        params = &RestartPodParams{}
    // ... more types
    }

    if err := json.Unmarshal(parameters, params); err != nil {
        return fmt.Errorf("invalid parameters: %w", err)
    }

    // Validate using struct tags
    if err := v.validate.Struct(params); err != nil {
        return fmt.Errorf("validation failed: %w", err)
    }

    return nil
}

// Add Rego policy validation for safety
func (v *ActionValidator) ValidateSafety(ctx context.Context, execution *executionv1.KubernetesExecution) error {
    input := map[string]interface{}{
        "action_type":     execution.Spec.Action.Type,
        "target_namespace": execution.Spec.Action.Target.Namespace,
        "parameters":      execution.Spec.Action.Parameters,
    }

    allowed, err := v.regoEngine.Query(ctx, "data.actions.allow", input)
    if err != nil {
        return fmt.Errorf("safety validation failed: %w", err)
    }

    if !allowed.(bool) {
        return fmt.Errorf("action blocked by safety policy")
    }

    return nil
}
```

**Changes Required**:
1. **Add validator library**: Use `go-playground/validator`
2. **Centralize validation**: Single validation function
3. **Add Rego integration**: Safety policy evaluation
4. **Add production namespace protection**: Prevent dangerous actions

**Estimated Effort**: 2-3 days

---

## Migration Strategy

### Phase 1: Foundation (Days 1-3)

**Goal**: Create CRD and controller skeleton

1. **Generate CRD**:
   ```bash
   kubebuilder create api --group kubernetesexecution --version v1alpha1 --kind KubernetesExecution
   ```

2. **Define CRD schema** based on existing `Action` type
3. **Create controller skeleton** with basic reconciliation loop
4. **Add owner references** to WorkflowExecution CRD

**Deliverables**:
- `api/kubernetesexecution/v1alpha1/kubernetesexecution_types.go`
- `internal/controller/kubernetesexecution/kubernetesexecution_controller.go`
- Basic unit tests

---

### Phase 2: Job-Based Execution (Days 4-6)

**Goal**: Refactor direct execution to Job-based pattern

1. **Create `JobBasedExecutor`** struct
2. **Migrate command building logic** from `kubernetes_executor.go`
3. **Implement Job creation** with per-action images
4. **Add Job monitoring** and status tracking
5. **Implement result collection** from Job pod logs

**Deliverables**:
- `pkg/kubernetesexecution/job_executor.go` (reuses existing logic)
- `pkg/kubernetesexecution/actions.go` (migrated from `pkg/platform/executor/`)
- Integration tests with Kind cluster

---

### Phase 3: RBAC & Safety (Days 7-9)

**Goal**: Add per-action ServiceAccounts and Rego validation

1. **Create action metadata catalog** with RBAC definitions
2. **Generate per-action ServiceAccounts** dynamically
3. **Implement Rego policy evaluation**
4. **Add production namespace protection**
5. **Implement dry-run validation Jobs**

**Deliverables**:
- `pkg/kubernetesexecution/rbac.go` (ServiceAccount management)
- `pkg/kubernetesexecution/validation.go` (Rego integration)
- RBAC manifests in `config/rbac/`

---

### Phase 4: Integration & Audit (Days 10-12)

**Goal**: Connect with WorkflowExecution and add audit trail

1. **Configure `SetupWithManager`** to watch owned Jobs
2. **Implement status propagation** to WorkflowExecution
3. **Add audit persistence** to Storage Service
4. **Implement Prometheus metrics**

**Deliverables**:
- Full CRD integration with WorkflowExecution
- Audit storage implementation
- Metrics dashboards

---

### Phase 5: Testing & Refinement (Days 13-15)

**Goal**: Comprehensive testing and documentation

1. **Unit tests**: 70%+ coverage target
2. **Integration tests**: Real Kubernetes cluster scenarios
3. **E2E tests**: Complete workflow-to-execution tests
4. **Performance testing**: Measure execution duration
5. **Documentation updates**: Runbook, troubleshooting guide

**Deliverables**:
- Complete test suite (unit, integration, e2e)
- Updated documentation
- Performance benchmarks

---

## Code Reuse Summary

| Component | Existing Lines | Reusable % | New Lines Needed | Total Lines |
|-----------|---------------|------------|------------------|-------------|
| Action definitions | 182 | 90% | 100 | 282 |
| Command building | 200 | 85% | 50 | 250 |
| Validation logic | 150 | 75% | 100 | 250 |
| Kubernetes client | 418 | 60% | 300 | 718 |
| **Total** | **950** | **75%** | **550** | **1,500** |

**Overall Reusability**: 75% of existing code is reusable with refactoring
**Estimated New Code**: 550 lines (controller + Job management + RBAC)
**Total Code**: ~1,500 lines for complete implementation

---

## Risk Assessment

### High Risk

- **Job-based refactoring complexity**: Moving from direct execution to Jobs requires significant architectural change
  - **Mitigation**: Incremental migration, keep existing executor for testing comparison

### Medium Risk

- **Rego policy integration**: New dependency, learning curve
  - **Mitigation**: Start with simple policies, gradually add complexity

- **Per-action RBAC**: Many ServiceAccounts to manage
  - **Mitigation**: Automate ServiceAccount generation, use naming conventions

### Low Risk

- **CRD controller**: Standard Kubebuilder pattern
  - **Mitigation**: Follow existing controller patterns from other services

- **Audit storage**: Well-defined integration point
  - **Mitigation**: Reuse patterns from RemediationProcessing controller

---

## Success Criteria

- [ ] All existing action types migrated to Job-based execution
- [ ] 70%+ code coverage with unit tests
- [ ] Integration tests passing on Kind cluster
- [ ] Per-action RBAC implemented and tested
- [ ] Rego policy validation functional
- [ ] Audit trail persisting to Storage Service
- [ ] Documentation complete and accurate
- [ ] Performance meets SLOs (<30s per action execution)

---

## References

- **Existing Code**: `pkg/platform/executor/` (current implementation)
- **Target Structure**: `pkg/kubernetesexecution/` (new package)
- **Integration Points**: [integration-points.md](./integration-points.md)
- **Testing Strategy**: [testing-strategy.md](./testing-strategy.md)
- **RBAC Design**: [security-configuration.md](./security-configuration.md)
