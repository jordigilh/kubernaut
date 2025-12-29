# WorkflowExecution Controller - Implementation Plan

**Filename**: `IMPLEMENTATION_PLAN_V3.7.md`
**Version**: v3.7 - Day 6 Extension with BR-WE-012 Implementation Tasks
**Last Updated**: 2025-12-06
**Timeline**: 12 days (Day 6 extended by 4-6h)
**Status**: ✅ VALIDATED - Ready for Implementation
**Confidence**: 95%

**Change Log**:
- **v3.7** (2025-12-06): Day 6 Extension with detailed BR-WE-012 implementation tasks
  - ✅ **Added**: Day 6 Extension section with 6 tasks for exponential backoff
  - ✅ **Added**: TDD test examples for CheckCooldown, MarkFailed, MarkCompleted
  - ✅ **Added**: Implementation code snippets aligned with DD-WE-004
  - ✅ **Added**: Day 6 Extension EOD Checklist
  - ✅ **Updated**: Timeline note (Day 6 extended by 4-6h)
- **v3.6** (2025-12-06): Exponential Backoff Cooldown per DD-WE-004 v1.1 and BR-WE-012
  - ✅ **Added**: BR-WE-012 business requirement for exponential backoff (pre-execution failures only)
  - ✅ **Added**: DD-WE-004 v1.1 design decision document
  - ✅ **CRITICAL**: Backoff ONLY applies to pre-execution failures (`wasExecutionFailure: false`)
  - ✅ **CRITICAL**: Execution failures (`wasExecutionFailure: true`) block ALL retries with `PreviousExecutionFailed`
  - ✅ **Added**: `ConsecutiveFailures` and `NextAllowedExecution` status fields
  - ✅ **Added**: `ExhaustedRetries` and `PreviousExecutionFailed` skip reasons
  - ✅ **Added**: `workflowexecution_backoff_skip_total` and `workflowexecution_consecutive_failures` metrics
  - ✅ **Updated**: Day 9 to include exponential backoff unit tests with wasExecutionFailure distinction
  - ✅ **Updated**: BR Coverage Matrix with BR-WE-012
- **v3.5** (2025-12-06): Day 9 Triage - Revised testing tasks, aligned with 03-testing-strategy.mdc
  - ✅ **Updated**: Integration coverage from ~20% to >50% (microservices mandate)
  - ✅ **Revised**: Day 9 tasks (most unit tests already written in TDD during Days 1-8)
  - ✅ **Added**: DescribeTable/Entry() requirement for edge case testing
  - ✅ **Added**: Makefile targets (parallel execution by default with 4 procs)
  - ✅ **Removed**: lint-workflowexecution target (lint should be for all code)
- **v3.4** (2025-12-05): Day 7 Triage - Clarified Day 5/7 overlap, business-value metrics only
  - ✅ **Clarified**: Day 5 already implemented FailureDetails basics, Day 7 is TaskRun-specific only
  - ✅ **Updated**: Day 7 metrics to BR-WE-008 business-value metrics only (4 metrics)
  - ✅ **Deferred**: Debug-only metrics (phase_transition, lock_check_duration, active_total)
  - ✅ **Added**: `workflowexecution_skip_total{reason}` for DD-WE-001 visibility
- **v3.3** (2025-12-05): Day 6 Cooldown & Cleanup alignment with authoritative docs
  - ✅ **Updated**: Finalizer name to `kubernaut.ai/workflowexecution-cleanup` per finalizers-lifecycle.md
  - ✅ **Updated**: `reconcileDelete()` to use deterministic PipelineRun name per DD-WE-003
  - ✅ **Added**: `WorkflowExecutionDeleted` event emission per finalizers-lifecycle.md
  - ✅ **Aligned**: `reconcileTerminal()` implementation with DD-WE-003 spec
  - ✅ **Added**: Lock release event after cooldown expiry
- **v3.2** (2025-12-05): Day 5 Status Synchronization implementation
  - ✅ **Added**: TaskRun RBAC for FailureDetails extraction from failed tasks
  - ✅ **Added**: PipelineRunStatusSummary population during Running phase
  - ✅ **Clarified**: Duration calculation on Completed/Failed transition
  - ✅ **Added**: Use `knative.dev/pkg/apis` for Tekton condition checking
- **v3.1** (2025-12-05): Cross-namespace watch implementation refinement
  - ✅ **Changed**: Replaced namespace-scoped cache with predicate filter for PipelineRun watch
  - ✅ **Rationale**: Predicate filter is simpler and achieves same goal
  - ✅ **Added**: `kind` parameter to bundle resolver (required by Tekton)
  - ✅ **Fixed**: ServiceAccountName location (TaskRunTemplate per Tekton v1 API)
- **v3.0** (2025-12-03): Complete rewrite for Tekton delegation architecture
  - ✅ Tekton PipelineRun delegation (ADR-044)
  - ✅ Dedicated execution namespace (DD-WE-002)
  - ✅ Resource lock persistence (DD-WE-003)
  - ✅ Cross-team validation complete
  - ✅ All BR-WE-001 to BR-WE-011 mapped
- **v2.0** (2025-12-02): Updated for `.ai` API group, BR-WE-* prefix
- **v1.0** (2025-11-28): Initial plan

---

## 🎯 Quick Reference

| Property | Value |
|----------|-------|
| **Service Type** | CRD Controller |
| **CRD API Group** | `kubernaut.ai/v1alpha1` |
| **Controller** | WorkflowExecutionReconciler |
| **Health Port** | 8081 |
| **Metrics Port** | 9090 |
| **Execution Namespace** | `kubernaut-workflows` |
| **Tekton Version** | Latest stable |
| **Test Environment** | KIND + Tekton Pipelines |
| **E2E NodePort** | 30085 (DD-TEST-001) |
| **E2E Metrics NodePort** | 30185 (DD-TEST-001) |
| **E2E Host Port** | 8085 |
| **E2E Metrics Host** | 9185 |

**Methodology**: APDC-TDD with Defense-in-Depth Testing
**Parallel Execution**: 4 concurrent processes for all test tiers

---

## 📑 Table of Contents

| Section | Purpose |
|---------|---------|
| [Prerequisites](#prerequisites-checklist) | Pre-Day 1 requirements |
| [Cross-Team Validation](#-cross-team-validation) | Multi-team dependency sign-off |
| [Design Decisions](#-design-decisions) | Architectural choices |
| [Risk Assessment](#️-risk-assessment-matrix) | Risk identification and mitigation |
| **Day-by-Day Breakdown** | |
| ├─ [Days 1-2](#days-1-2-foundation--crd-setup) | Foundation + CRD Setup |
| ├─ [Days 3-4](#days-3-4-resource-locking--tekton-integration) | Resource Locking + Tekton Integration |
| ├─ [Days 5-6](#days-5-6-pipelinerun-creation--status-sync) | PipelineRun Creation + Status Sync |
| ├─ [Days 7-8](#days-7-8-failure-handling--audit) | Failure Handling + Audit |
| ├─ [Days 9-10](#days-9-10-testing) | Testing (Unit + Integration + E2E) |
| ├─ [Day 11](#day-11-documentation) | Documentation |
| └─ [Day 12](#day-12-production-readiness) | Production Readiness |
| [BR Coverage Matrix](#br-coverage-matrix) | Business requirement test mapping |
| [Appendix A: Code Examples](#appendix-a-code-examples) | Complete code patterns |
| [Appendix B: CRD Controller Patterns](#appendix-b-crd-controller-patterns) | Reconciliation patterns |
| **Quality & Operations** | |
| ├─ [Performance Targets](#performance-targets) | Latency, throughput metrics |
| ├─ [Common Pitfalls](#common-pitfalls-to-avoid) | Do's and don'ts |
| ├─ [Success Criteria](#success-criteria) | Completion checklist |
| └─ [Makefile Targets](#makefile-targets) | Development commands |
| **Supplementary Documents (8 Appendices)** | |
| ├─ [Appendix A: Error Handling](./APPENDIX_A_ERROR_HANDLING.md) | 5 error categories |
| ├─ [Appendix B: Runbooks](./APPENDIX_B_PRODUCTION_RUNBOOKS.md) | 5 production runbooks |
| ├─ [Appendix C: Test Scenarios](./APPENDIX_C_TEST_SCENARIOS.md) | ~100 test scenarios |
| ├─ [Appendix D: EOD Templates](./APPENDIX_D_EOD_TEMPLATES.md) | Daily progress templates |
| ├─ [Appendix E: Metrics](./APPENDIX_E_METRICS.md) | Complete metrics code |
| ├─ [Appendix F: Integration Tests](./APPENDIX_F_INTEGRATION_TESTS.md) | Complete test code |
| ├─ [Appendix G: BR Coverage](./APPENDIX_G_BR_COVERAGE_MATRIX.md) | Coverage matrix template |
| └─ [Appendix H: E2E Setup](./APPENDIX_H_E2E_TEST_SETUP.md) | E2E test infrastructure |

---

## Prerequisites Checklist

Before starting Day 1, ensure:

### Universal Standards
- [x] DD-005: Observability Standards (metrics/logging)
- [x] DD-007: Kubernetes-Aware Graceful Shutdown
- [x] DD-014: Binary Version Logging
- [x] DD-TEST-001: Port Allocation (8081 health, 9090 metrics)

### CRD Controller Standards
- [x] DD-006: Controller Scaffolding
- [x] DD-CRD-001: API Group `.ai` domain
- [x] ADR-004: Fake K8s Client for unit tests

### Audit Standards
- [x] DD-AUDIT-003: Service Audit Trace Requirements
- [x] ADR-032: Data Access Layer Isolation
- [x] ADR-034: Unified Audit Table Design

### Service-Specific
- [x] ADR-044: Workflow Execution Engine Delegation (Tekton)
- [x] ADR-043: Workflow Schema Definition Standard (OCI bundles)
- [x] DD-WE-001: Resource Locking Safety
- [x] DD-WE-002: Dedicated Execution Namespace
- [x] DD-WE-003: Resource Lock Persistence
- [x] DD-CONTRACT-001: AIAnalysis ↔ WorkflowExecution Contract

### Infrastructure
- [x] Tekton Pipelines installed in test cluster
- [x] KIND cluster with Tekton available
- [x] `kubernaut-workflows` namespace created
- [x] `kubernaut-workflow-runner` ServiceAccount + ClusterRole

### Template Sections Reviewed
- [x] Error Handling Philosophy Template
- [x] BR Coverage Matrix Methodology
- [x] CRD Controller Variant (Appendix B)
- [x] Confidence Assessment Methodology

---

## 🤝 Cross-Team Validation

**Validation Status**: ✅ VALIDATED

| Team | Validation Topic | Status | Record |
|------|------------------|--------|--------|
| RemediationOrchestrator | WFE creation contract, targetResource | ✅ Complete | DD-CONTRACT-001 v1.4 |
| AIAnalysis | SelectedWorkflow → WorkflowRef mapping | ✅ Complete | DD-CONTRACT-001 v1.3 |
| HolmesGPT-API | Parameter validation ownership | ✅ Complete | DD-HAPI-002 v1.1 |
| Gateway | TargetResource format validation | ✅ Complete | Q-GW-01 Response |
| Notification | Failure notification format | ✅ Complete | QUESTIONS_FROM_WORKFLOW_ENGINE_TEAM.md |
| Data Storage | Audit event schema | ✅ Complete | database-integration.md |

### Pre-Implementation Validation Gate ✅

- [x] All upstream data contracts validated (RO creates WFE)
- [x] All downstream data contracts validated (WFE → Tekton PipelineRun)
- [x] Shared type definitions aligned (FailureDetails, SkipDetails)
- [x] Naming conventions agreed (kebab-case for K8s resources)
- [x] Field paths confirmed (`spec.workflowRef`, `spec.targetResource`)
- [x] Integration points documented with examples

---

## 🎯 Design Decisions

### Approved Design Decisions (2025-12-03)

| ID | Decision | Document |
|----|----------|----------|
| DD-WE-001 | Resource Locking Safety | [Link](../../../../architecture/decisions/DD-WE-001-resource-locking-safety.md) |
| DD-WE-002 | Dedicated Execution Namespace | [Link](../../../../architecture/decisions/DD-WE-002-dedicated-execution-namespace.md) |
| DD-WE-003 | Lock Persistence (Deterministic Name) | [Link](../../../../architecture/decisions/DD-WE-003-resource-lock-persistence.md) |
| DD-WE-004 | Exponential Backoff Cooldown | [Link](../../../../architecture/decisions/DD-WE-004-exponential-backoff-cooldown.md) |
| ADR-044 | Tekton PipelineRun Delegation | [Link](../../../../architecture/decisions/ADR-044-workflow-execution-engine-delegation.md) |

### Key Design Choices

#### DD-1: PipelineRun Execution Namespace

**Decision**: ALL PipelineRuns execute in dedicated `kubernaut-workflows` namespace.

**Rationale**:
- Single ServiceAccount with ClusterRoleBinding
- All remediation activity in one place (audit clarity)
- Easy cleanup and resource quota management
- Industry pattern (Crossplane, AWX, Argo)

**Alternatives Rejected**:
- Target namespace (requires per-namespace SA setup)
- Hybrid (complex, inconsistent behavior)

#### DD-2: Lock Persistence Strategy

**Decision**: Deterministic PipelineRun name + Indexed CRD query (belt-and-suspenders).

**Rationale**:
- Zero race conditions (Kubernetes object uniqueness)
- Survives controller restart
- Works with multiple replicas
- No external dependencies (no Redis)

**Implementation**:
```go
func pipelineRunName(targetResource string) string {
    h := sha256.Sum256([]byte(targetResource))
    return fmt.Sprintf("wfe-%s", hex.EncodeToString(h[:])[:16])
}
```

#### DD-3: Tekton Dependency Handling

**Decision**: Controller crashes at startup if Tekton CRDs not found (ADR-030).

**Rationale**:
- Fail-fast behavior
- Clear error for operators
- Kubernetes restarts pod, AlertManager detects CrashLoopBackOff

---

## ⚠️ Risk Assessment Matrix

### Risk Categories

| ID | Risk | Probability | Impact | Mitigation | Day |
|----|------|-------------|--------|------------|-----|
| R1 | Race condition in lock acquisition | Low | High | Deterministic PipelineRun name (DD-WE-003) | 3-4 |
| R2 | Controller restart loses lock state | Medium | High | Lock = PipelineRun existence | 3-4 |
| R3 | Cross-namespace watch fails | Medium | Medium | Namespace-scoped cache config | 5-6 |
| R4 | PipelineRun stuck forever | Low | Medium | Tekton timeout + TTL cleanup | 5-6 |
| R5 | Tekton not installed | Low | Critical | Crash at startup (ADR-030) | 1 |
| R6 | ClusterRole too broad | Low | Medium | Security audit before prod | 12 |
| R7 | Finalizer fails to delete PR | Low | Low | Retry + Tekton TTL backup | 7-8 |

### Risk Mitigation Status

| Risk | Status | Validation |
|------|--------|------------|
| R1 | ✅ Mitigated | DD-WE-003 documented, code example provided |
| R2 | ✅ Mitigated | Lock persistence via PipelineRun existence |
| R3 | ⏳ Day 5-6 | Cache configuration in SetupWithManager |
| R4 | ✅ Mitigated | Tekton handles timeout, TTL for cleanup |
| R5 | ✅ Mitigated | CheckTektonAvailable() in controller-implementation.md |
| R6 | ⏳ Day 12 | Security review before production |
| R7 | ✅ Mitigated | Finalizer + TTL belt-and-suspenders |

---

## 📋 Files Affected

### New Files

| Path | Purpose |
|------|---------|
| `api/workflowexecution/v1alpha1/workflowexecution_types.go` | CRD types |
| `api/workflowexecution/v1alpha1/groupversion_info.go` | API group |
| `api/workflowexecution/v1alpha1/zz_generated.deepcopy.go` | Generated |
| `internal/controller/workflowexecution_controller.go` | Reconciler |
| `cmd/workflowexecution/main.go` | Entry point |
| `test/unit/workflowexecution/controller_test.go` | Unit tests |
| `test/integration/workflowexecution/lifecycle_test.go` | Integration tests |
| `test/e2e/workflowexecution/workflow_test.go` | E2E tests |
| `config/crd/bases/kubernaut.ai_workflowexecutions.yaml` | CRD YAML |
| `config/rbac/workflowexecution_role.yaml` | RBAC |

### Modified Files

| Path | Changes |
|------|---------|
| `Makefile` | Add workflowexecution targets |
| `PROJECT` | Register new controller |

---

## Day-by-Day Breakdown

---

## Days 1-2: Foundation + CRD Setup

**Focus**: Project structure, CRD types, controller skeleton

### Day 1: Project Setup + CRD Types (8h)

#### Morning (4h): Project Structure

- [ ] Create `cmd/workflowexecution/` directory
- [ ] Copy `main.go` template from remediationorchestrator
- [ ] Update package imports for WorkflowExecutionReconciler
- [ ] Verify build: `go build -o bin/workflow-execution ./cmd/workflowexecution`
- [ ] Add Tekton startup check (ADR-030)

```go
// cmd/workflowexecution/main.go
func main() {
    // ... setup manager ...

    // REQUIRED: Validate Tekton is installed (ADR-030)
    if err := controller.CheckTektonAvailable(ctx, mgr.GetRESTMapper()); err != nil {
        setupLog.Error(err, "Required dependency check failed")
        os.Exit(1)  // CRASH - Tekton is required
    }

    // ... start manager ...
}
```

#### Afternoon (4h): CRD Types

- [ ] Create `api/workflowexecution/v1alpha1/` directory
- [ ] Implement `workflowexecution_types.go` per crd-schema.md
- [ ] Add groupversion_info.go with `.ai` domain
- [ ] Generate deepcopy and CRD manifest
- [ ] Register types in scheme

```go
// api/workflowexecution/v1alpha1/workflowexecution_types.go
// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:resource:shortName=wfe
// +kubebuilder:printcolumn:name="Phase",type="string",JSONPath=".status.phase"
// +kubebuilder:printcolumn:name="Target",type="string",JSONPath=".spec.targetResource"
type WorkflowExecution struct {
    metav1.TypeMeta   `json:",inline"`
    metav1.ObjectMeta `json:"metadata,omitempty"`
    Spec              WorkflowExecutionSpec   `json:"spec,omitempty"`
    Status            WorkflowExecutionStatus `json:"status,omitempty"`
}
```

### Day 1 EOD Checklist

- [ ] `go build ./cmd/workflowexecution` succeeds
- [ ] CRD types compile
- [ ] `make generate` succeeds
- [ ] CRD YAML generated in `config/crd/bases/`

---

### Day 2: Controller Skeleton + Finalizers (8h)

#### Morning (4h): Controller Scaffold

- [ ] Generate controller with kubebuilder
- [ ] Implement basic Reconcile loop
- [ ] Add finalizer handling per finalizers-lifecycle.md
- [ ] Configure owner references

```go
// internal/controller/workflowexecution_controller.go
const (
    // FinalizerName per finalizers-lifecycle.md (v3.3)
    FinalizerName = "kubernaut.ai/workflowexecution-cleanup"

    DefaultCooldownPeriod = 5 * time.Minute
)

type WorkflowExecutionReconciler struct {
    client.Client
    Scheme             *runtime.Scheme
    Recorder           record.EventRecorder
    CooldownPeriod     time.Duration
    ExecutionNamespace string  // "kubernaut-workflows" (DD-WE-002)
    ServiceAccountName string  // "kubernaut-workflow-runner"
}
```

#### Afternoon (4h): RBAC + Phase Transitions

- [ ] Add RBAC markers per security-configuration.md
- [ ] Implement phase transition logic
- [ ] Add Kubernetes event emission
- [ ] Write first unit test (controller exists)

```go
//+kubebuilder:rbac:groups=kubernaut.ai,resources=workflowexecutions,verbs=get;list;watch;update;patch
//+kubebuilder:rbac:groups=kubernaut.ai,resources=workflowexecutions/status,verbs=get;update;patch
//+kubebuilder:rbac:groups=kubernaut.ai,resources=workflowexecutions/finalizers,verbs=update
//+kubebuilder:rbac:groups=tekton.dev,resources=pipelineruns,verbs=create;get;list;watch;delete
//+kubebuilder:rbac:groups="",resources=events,verbs=create;patch
```

### Day 2 EOD Checklist

- [ ] Controller reconciles (empty loop)
- [ ] Finalizer added on create
- [ ] RBAC generated
- [ ] 1+ unit test passes

---

## Days 3-4: Resource Locking + Tekton Integration

**Focus**: DD-WE-001, DD-WE-003 implementation

### Day 3: Resource Lock Implementation (8h)

#### Morning (4h): Field Index + Lock Check

- [ ] Add field index on `spec.targetResource` (DD-WE-003)
- [ ] Implement `checkResourceLock()` - Layer 1
- [ ] Implement indexed query for Running WFEs
- [ ] Add cooldown check

```go
func (r *WorkflowExecutionReconciler) SetupWithManager(mgr ctrl.Manager) error {
    // Create index on targetResource for O(1) lock check (DD-WE-003)
    if err := mgr.GetFieldIndexer().IndexField(
        context.Background(),
        &workflowexecutionv1.WorkflowExecution{},
        "spec.targetResource",
        func(obj client.Object) []string {
            wfe := obj.(*workflowexecutionv1.WorkflowExecution)
            return []string{wfe.Spec.TargetResource}
        },
    ); err != nil {
        return err
    }
    // ...
}
```

#### Afternoon (4h): Deterministic Naming

- [ ] Implement `pipelineRunName()` - Layer 2 (DD-WE-003)
- [ ] Handle `AlreadyExists` error (race condition caught)
- [ ] Implement `markSkipped()` for blocked executions
- [ ] Add SkipDetails to status

```go
func pipelineRunName(targetResource string) string {
    h := sha256.Sum256([]byte(targetResource))
    return fmt.Sprintf("wfe-%s", hex.EncodeToString(h[:])[:16])
}
```

### Day 3 EOD Checklist

- [ ] Field index created
- [ ] Lock check returns blocked for Running WFE
- [ ] Cooldown check works
- [ ] Deterministic name generates correctly

---

### Day 4: Tekton PipelineRun Creation (8h)

#### Morning (4h): buildPipelineRun

- [ ] Implement `buildPipelineRun()` per controller-implementation.md
- [ ] Use bundle resolver for OCI bundles
- [ ] Pass parameters from spec
- [ ] Set labels for cross-namespace tracking

```go
func (r *WorkflowExecutionReconciler) BuildPipelineRun(
    wfe *workflowexecutionv1.WorkflowExecution,
) *tektonv1.PipelineRun {
    return &tektonv1.PipelineRun{
        ObjectMeta: metav1.ObjectMeta{
            Name:      PipelineRunName(wfe.Spec.TargetResource),
            Namespace: r.ExecutionNamespace,  // "kubernaut-workflows"
            Labels: map[string]string{
                "kubernaut.ai/workflow-execution": wfe.Name,
                "kubernaut.ai/workflow-id":        wfe.Spec.WorkflowRef.WorkflowID,
                "kubernaut.ai/target-resource":    wfe.Spec.TargetResource,
                "kubernaut.ai/source-namespace":   wfe.Namespace,
            },
        },
        Spec: tektonv1.PipelineRunSpec{
            PipelineRef: &tektonv1.PipelineRef{
                ResolverRef: tektonv1.ResolverRef{
                    Resolver: "bundles",
                    Params: []tektonv1.Param{
                        {Name: "bundle", Value: tektonv1.ParamValue{StringVal: wfe.Spec.WorkflowRef.ContainerImage}},
                        {Name: "name", Value: tektonv1.ParamValue{StringVal: "workflow"}},
                        {Name: "kind", Value: tektonv1.ParamValue{StringVal: "pipeline"}},  // Required by Tekton (v3.1)
                    },
                },
            },
            Params: r.ConvertParameters(wfe.Spec.Parameters),
            TaskRunTemplate: tektonv1.PipelineTaskRunTemplate{
                ServiceAccountName: r.ServiceAccountName,  // Tekton v1 API location (v3.1)
            },
        },
    }
}
```

#### Afternoon (4h): Cross-Namespace Watch

- [ ] Implement `findWFEForPipelineRun()` mapper
- [ ] Add predicate filter for our PipelineRuns (filters by `kubernaut.ai/workflow-execution` label)
- [ ] Configure `Watches()` in SetupWithManager with predicate
- [ ] Test watch triggers reconcile

> **Note (v3.1)**: Uses predicate filter instead of namespace-scoped cache. Predicate filter is simpler
> and achieves the same goal - only watching PipelineRuns that have our tracking label.

### Day 4 EOD Checklist

- [ ] PipelineRun created in `kubernaut-workflows`
- [ ] Parameters passed correctly
- [ ] Watch configured for cross-namespace
- [ ] Unit tests for buildPipelineRun

---

## Days 5-6: PipelineRun Creation + Status Sync

**Focus**: Complete execution flow, status synchronization

### Day 5: Status Synchronization (8h)

#### Morning (4h): reconcileRunning

- [ ] Add TaskRun RBAC marker for FailureDetails extraction (v3.2)
- [ ] Implement `reconcileRunning()` per reconciliation-phases.md
- [ ] Fetch PipelineRun from execution namespace (DD-WE-002)
- [ ] Update `PipelineRunStatusSummary` for task progress visibility (v3.2)
- [ ] Map Tekton status to WFE phase using `knative.dev/pkg/apis` (v3.2)
- [ ] Handle PipelineRun completion

```go
func (r *WorkflowExecutionReconciler) reconcileRunning(
    ctx context.Context,
    wfe *workflowexecutionv1.WorkflowExecution,
) (ctrl.Result, error) {
    // Get PipelineRun from execution namespace
    var pr tektonv1.PipelineRun
    if err := r.Get(ctx, client.ObjectKey{
        Name:      pipelineRunName(wfe.Spec.TargetResource),
        Namespace: r.ExecutionNamespace,
    }, &pr); err != nil {
        // Handle not found (deleted externally)
    }

    // Map status
    switch {
    case pr.Status.GetCondition(apis.ConditionSucceeded).IsTrue():
        return r.markCompleted(ctx, wfe)
    case pr.Status.GetCondition(apis.ConditionSucceeded).IsFalse():
        return r.markFailed(ctx, wfe, &pr)
    default:
        // Still running, requeue
        return ctrl.Result{RequeueAfter: 10 * time.Second}, nil
    }
}
```

#### Afternoon (4h): Phase Transitions

- [ ] Implement `markCompleted()` with Duration calculation (v3.2)
- [ ] Implement `markFailed()` with FailureDetails extraction
- [ ] Implement `extractFailureDetails()` helper (reads TaskRuns)
- [ ] Implement `buildPipelineRunStatusSummary()` helper (v3.2)
- [ ] Implement `generateNaturalLanguageSummary()` for LLM context
- [ ] Add completion time tracking
- [ ] Emit Kubernetes events

### Day 5 EOD Checklist

- [ ] TaskRun RBAC added (v3.2)
- [ ] Status syncs from PipelineRun
- [ ] PipelineRunStatusSummary populated during Running (v3.2)
- [ ] Duration calculated on completion (v3.2)
- [ ] Completed/Failed phases work
- [ ] FailureDetails populated on failure
- [ ] NaturalLanguageSummary generated
- [ ] Events emitted

---

### Day 6: Cooldown + Cleanup (8h)

#### Morning (4h): Cooldown Enforcement (reconcileTerminal)

**Reference**: DD-WE-003 Section "4. Lock Lifecycle (Deletion)" lines 342-382

- [ ] Update finalizer constant to `kubernaut.ai/workflowexecution-cleanup` (v3.3)
- [ ] Implement `reconcileTerminal()` per DD-WE-003:
  - [ ] Check `CompletionTime` is set (guard clause)
  - [ ] Calculate elapsed time since completion
  - [ ] If `elapsed < CooldownPeriod`: requeue with remaining duration
  - [ ] If `elapsed >= CooldownPeriod`: delete PipelineRun using **deterministic name**
- [ ] Emit `LockReleased` event after PipelineRun deletion
- [ ] Write TDD tests for cooldown enforcement

```go
// DD-WE-003 aligned implementation
func (r *WorkflowExecutionReconciler) reconcileTerminal(
    ctx context.Context,
    wfe *workflowexecutionv1.WorkflowExecution,
) (ctrl.Result, error) {
    log := log.FromContext(ctx)

    if wfe.Status.CompletionTime == nil {
        return ctrl.Result{}, nil  // Guard: no completion time
    }

    elapsed := time.Since(wfe.Status.CompletionTime.Time)

    // Wait for cooldown before releasing lock
    if elapsed < r.CooldownPeriod {
        remaining := r.CooldownPeriod - elapsed
        log.V(1).Info("Waiting for cooldown", "remaining", remaining)
        return ctrl.Result{RequeueAfter: remaining}, nil
    }

    // Cooldown expired - delete PipelineRun to release lock
    prName := PipelineRunName(wfe.Spec.TargetResource)  // DETERMINISTIC NAME (DD-WE-003)
    pr := &tektonv1.PipelineRun{
        ObjectMeta: metav1.ObjectMeta{
            Name:      prName,
            Namespace: r.ExecutionNamespace,
        },
    }

    if err := r.Delete(ctx, pr); err != nil && !apierrors.IsNotFound(err) {
        log.Error(err, "Failed to delete PipelineRun")
        return ctrl.Result{}, err
    }

    log.Info("Lock released after cooldown", "targetResource", wfe.Spec.TargetResource)
    r.Recorder.Event(wfe, "Normal", "LockReleased",
        fmt.Sprintf("Lock released for %s after cooldown", wfe.Spec.TargetResource))

    return ctrl.Result{}, nil
}
```

#### Afternoon (4h): Enhance reconcileDelete (Finalizer Cleanup)

**Reference**: DD-WE-003 Section "4. Lock Lifecycle (Deletion)" lines 384-415, finalizers-lifecycle.md

- [ ] Update `reconcileDelete()` to use **deterministic name** for PipelineRun deletion (v3.3)
- [ ] Add `WorkflowExecutionDeleted` event emission per finalizers-lifecycle.md (v3.3)
- [ ] Handle deletion during Running phase (cancels execution)
- [ ] Write TDD tests for finalizer cleanup

```go
// DD-WE-003 + finalizers-lifecycle.md aligned implementation
func (r *WorkflowExecutionReconciler) reconcileDelete(
    ctx context.Context,
    wfe *workflowexecutionv1.WorkflowExecution,
) (ctrl.Result, error) {
    log := log.FromContext(ctx)

    if !controllerutil.ContainsFinalizer(wfe, FinalizerName) {
        return ctrl.Result{}, nil
    }

    // Delete associated PipelineRun using DETERMINISTIC NAME (DD-WE-003)
    prName := PipelineRunName(wfe.Spec.TargetResource)
    pr := &tektonv1.PipelineRun{
        ObjectMeta: metav1.ObjectMeta{
            Name:      prName,
            Namespace: r.ExecutionNamespace,
        },
    }

    if err := r.Delete(ctx, pr); err != nil && !apierrors.IsNotFound(err) {
        log.Error(err, "Failed to delete PipelineRun during finalization")
        return ctrl.Result{}, err
    }

    log.Info("Finalizer: deleted associated PipelineRun", "pipelineRun", prName)

    // Emit deletion event (finalizers-lifecycle.md)
    r.Recorder.Event(wfe, "Normal", "WorkflowExecutionDeleted",
        fmt.Sprintf("WorkflowExecution cleanup completed (phase: %s)", wfe.Status.Phase))

    // Remove finalizer
    controllerutil.RemoveFinalizer(wfe, FinalizerName)
    if err := r.Update(ctx, wfe); err != nil {
        return ctrl.Result{}, err
    }

    return ctrl.Result{}, nil
}
```

### Day 6 EOD Checklist (Original)

- [ ] Finalizer name updated to match spec (v3.3)
- [ ] Cooldown enforced (5 min default)
- [ ] PipelineRun deleted using deterministic name (v3.3)
- [ ] `LockReleased` event emitted after cooldown (v3.3)
- [ ] `WorkflowExecutionDeleted` event emitted on deletion (v3.3)
- [ ] Finalizer cleanup works
- [ ] Error handling complete
- [ ] TDD tests for cooldown and cleanup

---

### Day 6 Extension: Exponential Backoff (BR-WE-012, DD-WE-004) (4-6h)

**Reference**: DD-WE-004 (Exponential Backoff Cooldown)
**Business Requirement**: BR-WE-012

> **v3.7 Addition**: This extension implements exponential backoff for pre-execution failures
> and permanent blocking for execution failures (`wasExecutionFailure: true`).

#### Task 1: Add Reconciler Configuration Fields (30min)

- [ ] Add backoff configuration fields to `WorkflowExecutionReconciler`:

```go
type WorkflowExecutionReconciler struct {
    client.Client
    Scheme                   *runtime.Scheme
    Recorder                 record.EventRecorder
    CooldownPeriod           time.Duration
    ExecutionNamespace       string
    ServiceAccountName       string
    AuditStore               audit.AuditStore
    // BR-WE-012: Exponential Backoff Configuration (DD-WE-004)
    BaseCooldownPeriod       time.Duration  // Default: 1 minute
    MaxCooldownPeriod        time.Duration  // Default: 10 minutes
    MaxBackoffExponent       int            // Default: 4
    MaxConsecutiveFailures   int            // Default: 5
}
```

#### Task 2: Enhance CheckCooldown with Exponential Backoff (2h)

**TDD RED**: Write failing tests first

```go
var _ = Describe("CheckCooldown with Exponential Backoff (BR-WE-012)", func() {
    Context("when previous WFE has wasExecutionFailure: true", func() {
        It("should block with PreviousExecutionFailed (not backoff)", func() {
            // Previous WFE ran and failed - blocks ALL retries
        })
    })

    Context("when previous WFE has wasExecutionFailure: false", func() {
        It("should apply exponential backoff based on ConsecutiveFailures", func() {
            // Pre-execution failures get exponential backoff
        })

        It("should cap backoff at MaxCooldownPeriod", func() {
            // Cooldown = min(Base * 2^(failures-1), Max)
        })

        It("should skip with ExhaustedRetries after MaxConsecutiveFailures", func() {
            // After 5 failures, skip with ExhaustedRetries
        })
    })

    Context("when previous WFE completed successfully", func() {
        It("should allow execution (ConsecutiveFailures reset)", func() {
            // Success resets failure counter
        })
    })
})
```

**TDD GREEN**: Implement enhanced `CheckCooldown()`

```go
// CheckCooldown checks cooldown with exponential backoff (DD-WE-004)
// Priority:
//   1. wasExecutionFailure: true → PreviousExecutionFailed (blocks ALL)
//   2. ConsecutiveFailures >= Max → ExhaustedRetries
//   3. time.Now() < NextAllowedExecution → RecentlyRemediated
func (r *WorkflowExecutionReconciler) CheckCooldown(
    ctx context.Context,
    wfe *workflowexecutionv1alpha1.WorkflowExecution,
) (bool, *workflowexecutionv1alpha1.SkipDetails, error) {
    // Find most recent terminal WFE for same target
    recentWFE := r.findMostRecentTerminalWFE(ctx, wfe)
    if recentWFE == nil {
        return false, nil, nil // No history, allow
    }

    // DD-WE-004-1: Check execution failure FIRST (blocks ALL retries)
    if recentWFE.Status.Phase == PhaseFailed &&
       recentWFE.Status.FailureDetails != nil &&
       recentWFE.Status.FailureDetails.WasExecutionFailure {
        return true, &SkipDetails{
            Reason:  SkipReasonPreviousExecutionFailed,
            Message: fmt.Sprintf("Previous execution failed during workflow run. Manual intervention required."),
            // ...
        }, nil
    }

    // DD-WE-004-3: Check exhausted retries
    if recentWFE.Status.ConsecutiveFailures >= int32(r.MaxConsecutiveFailures) {
        return true, &SkipDetails{
            Reason:  SkipReasonExhaustedRetries,
            Message: fmt.Sprintf("Max consecutive failures (%d) reached. Manual intervention required.", r.MaxConsecutiveFailures),
            // ...
        }, nil
    }

    // DD-WE-004-2: Calculate exponential backoff
    if recentWFE.Status.NextAllowedExecution != nil {
        if time.Now().Before(recentWFE.Status.NextAllowedExecution.Time) {
            return true, &SkipDetails{
                Reason:  SkipReasonRecentlyRemediated,
                Message: fmt.Sprintf("Backoff active. Next allowed: %v", recentWFE.Status.NextAllowedExecution.Time),
                // ...
            }, nil
        }
    }

    return false, nil, nil
}
```

#### Task 3: Enhance MarkFailed with ConsecutiveFailures Tracking (1h)

**TDD RED**: Write failing tests

```go
var _ = Describe("MarkFailed with ConsecutiveFailures (BR-WE-012)", func() {
    Context("when wasExecutionFailure is false (pre-execution failure)", func() {
        It("should increment ConsecutiveFailures", func() {})
        It("should calculate NextAllowedExecution with exponential backoff", func() {})
    })

    Context("when wasExecutionFailure is true (execution failure)", func() {
        It("should NOT increment ConsecutiveFailures", func() {})
        It("should NOT set NextAllowedExecution", func() {})
    })
})
```

**TDD GREEN**: Enhance `MarkFailed()`

```go
func (r *WorkflowExecutionReconciler) MarkFailed(...) (ctrl.Result, error) {
    // ... existing failure details extraction ...

    // BR-WE-012: Track consecutive failures for pre-execution failures only
    if !details.WasExecutionFailure {
        wfe.Status.ConsecutiveFailures++

        // Calculate exponential backoff: Base * 2^(min(failures-1, maxExponent))
        exponent := min(int(wfe.Status.ConsecutiveFailures)-1, r.MaxBackoffExponent)
        backoff := r.BaseCooldownPeriod * time.Duration(1<<exponent)
        if backoff > r.MaxCooldownPeriod {
            backoff = r.MaxCooldownPeriod
        }

        nextAllowed := metav1.NewTime(time.Now().Add(backoff))
        wfe.Status.NextAllowedExecution = &nextAllowed
    }
    // Note: For execution failures, ConsecutiveFailures is NOT incremented
    // The PreviousExecutionFailed check in CheckCooldown blocks ALL retries

    // ... rest of existing logic ...
}
```

#### Task 4: Enhance MarkCompleted with Counter Reset (30min)

**TDD RED**: Write failing test

```go
It("should reset ConsecutiveFailures to 0 on success", func() {
    // Previous WFE had ConsecutiveFailures = 3
    // Current WFE completes successfully
    // ConsecutiveFailures should be 0
})
```

**TDD GREEN**: Enhance `MarkCompleted()`

```go
func (r *WorkflowExecutionReconciler) MarkCompleted(...) (ctrl.Result, error) {
    // ... existing logic ...

    // BR-WE-012: Reset failure counter on success (DD-WE-004-5)
    wfe.Status.ConsecutiveFailures = 0
    wfe.Status.NextAllowedExecution = nil

    // ... rest of existing logic ...
}
```

#### Task 5: Add Backoff Metrics (30min)

**Reference**: metrics-slos.md v3.1

- [ ] Add `workflowexecution_backoff_skip_total{reason}` counter
- [ ] Add `workflowexecution_consecutive_failures{target_resource}` gauge

```go
// In metrics.go
var (
    BackoffSkipTotal = prometheus.NewCounterVec(
        prometheus.CounterOpts{
            Name: "workflowexecution_backoff_skip_total",
            Help: "Total workflows skipped due to backoff",
        },
        []string{"reason"},  // ExhaustedRetries, PreviousExecutionFailed
    )

    ConsecutiveFailuresGauge = prometheus.NewGaugeVec(
        prometheus.GaugeOpts{
            Name: "workflowexecution_consecutive_failures",
            Help: "Current consecutive failure count per target resource",
        },
        []string{"target_resource"},
    )
)
```

#### Task 6: Add Helper Function (30min)

```go
// findMostRecentTerminalWFE finds the most recent Completed/Failed WFE for the same target
func (r *WorkflowExecutionReconciler) findMostRecentTerminalWFE(
    ctx context.Context,
    wfe *workflowexecutionv1alpha1.WorkflowExecution,
) *workflowexecutionv1alpha1.WorkflowExecution {
    var wfeList workflowexecutionv1alpha1.WorkflowExecutionList
    if err := r.List(ctx, &wfeList, client.MatchingFields{
        "spec.targetResource": wfe.Spec.TargetResource,
    }); err != nil {
        return nil
    }

    var mostRecent *workflowexecutionv1alpha1.WorkflowExecution
    for i := range wfeList.Items {
        existing := &wfeList.Items[i]
        if existing.UID == wfe.UID {
            continue
        }
        if existing.Status.Phase != PhaseCompleted && existing.Status.Phase != PhaseFailed {
            continue
        }
        if existing.Status.CompletionTime == nil {
            continue
        }
        if mostRecent == nil || existing.Status.CompletionTime.After(mostRecent.Status.CompletionTime.Time) {
            mostRecent = existing
        }
    }
    return mostRecent
}
```

### Day 6 Extension EOD Checklist (BR-WE-012)

- [ ] Reconciler has backoff configuration fields
- [ ] `CheckCooldown()` checks `wasExecutionFailure` FIRST
- [ ] `CheckCooldown()` returns `PreviousExecutionFailed` for execution failures
- [ ] `CheckCooldown()` returns `ExhaustedRetries` after max failures
- [ ] `CheckCooldown()` applies exponential backoff for pre-execution failures
- [ ] `MarkFailed()` increments `ConsecutiveFailures` (pre-execution only)
- [ ] `MarkFailed()` calculates `NextAllowedExecution` with exponential formula
- [ ] `MarkCompleted()` resets `ConsecutiveFailures` to 0
- [ ] Backoff metrics registered and recorded
- [ ] All TDD tests pass (RED → GREEN)
- [ ] BR-WE-012 coverage: ✅ Unit, ⬜ Integration, ⬜ E2E

---

## Days 7-8: TaskRun Details + Metrics + Audit

**Focus**: TaskRun-specific failure info, business-value metrics, audit trail

> **Note (v3.4)**: Day 5 already implemented basic FailureDetails extraction, reason mapping,
> and NaturalLanguageSummary. Day 7 morning focuses ONLY on TaskRun-specific fields
> (`FailedTaskName`, `FailedTaskIndex`, `ExitCode`) that require fetching the actual TaskRun.

### Day 7: TaskRun Extraction + Metrics (8h)

#### Morning (4h): TaskRun-Specific Failure Details

**What Day 5 already implemented** (DO NOT DUPLICATE):
- ✅ `ExtractFailureDetails()` - basic structure, Reason, Message
- ✅ `mapTektonReasonToFailureReason()` - reason code mapping
- ✅ `GenerateNaturalLanguageSummary()` - LLM-friendly summary

**Day 7 adds** (TaskRun-specific fields from crd-schema.md):
- [ ] Implement `findFailedTaskRun()` helper
  - Iterate `pr.Status.ChildReferences` to find failed TaskRun
  - Fetch TaskRun via K8s client (we have RBAC from Day 5)
- [ ] Update `ExtractFailureDetails()` to populate:
  - `FailedTaskName` - name of failed TaskRun
  - `FailedTaskIndex` - index in pipeline
  - `ExitCode` - container exit code (if available)
- [ ] Handle TaskRun already deleted (short-lived)
- [ ] TDD tests for TaskRun extraction

```go
// Day 7 addition: Find and fetch failed TaskRun
func (r *WorkflowExecutionReconciler) findFailedTaskRun(
    ctx context.Context,
    pr *tektonv1.PipelineRun,
) (*tektonv1.TaskRun, int, error) {
    for i, ref := range pr.Status.ChildReferences {
        if ref.Kind == "TaskRun" {
            var tr tektonv1.TaskRun
            if err := r.Get(ctx, client.ObjectKey{
                Name:      ref.Name,
                Namespace: pr.Namespace,
            }, &tr); err != nil {
                continue // TaskRun may be deleted
            }

            cond := tr.Status.GetCondition(apis.ConditionSucceeded)
            if cond != nil && cond.IsFalse() {
                return &tr, i, nil
            }
        }
    }
    return nil, -1, nil // No failed TaskRun found
}
```

#### Afternoon (4h): Business-Value Metrics (BR-WE-008)

**Only implement metrics with business value** (per BR-WE-008):

- [ ] Create `internal/controller/workflowexecution/metrics.go`
- [ ] `workflowexecution_total{outcome}` - Counter (success/failed)
- [ ] `workflowexecution_duration_seconds{outcome}` - Histogram
- [ ] `workflowexecution_pipelinerun_creation_total` - Counter
- [ ] `workflowexecution_skip_total{reason}` - Counter (DD-WE-001 visibility)
- [ ] Instrument controller methods:
  - `MarkCompleted()`: Increment total, record duration
  - `MarkFailed()`: Increment total, record duration
  - `MarkSkipped()`: Increment skip counter with reason
  - `reconcilePending()`: Increment PR creation on success
- [ ] TDD tests for metrics

**Deferred metrics** (not business-critical, can add later):
- `workflowexecution_active_total` - Tekton provides equivalent
- `workflowexecution_phase_transition_total` - Debug-only
- `workflowexecution_resource_lock_check_duration_seconds` - Debug-only

### Day 7 EOD Checklist

- [ ] `FailedTaskName` populated from TaskRun
- [ ] `FailedTaskIndex` populated
- [ ] `ExitCode` populated (when available)
- [ ] Metrics exposed on :9090
- [ ] 4 business-value metrics implemented
- [ ] TDD tests for TaskRun extraction
- [ ] TDD tests for metrics

---

### Day 8: Audit Trail (8h)

#### Morning (4h): Audit Client

- [ ] Implement audit client per database-integration.md
- [ ] Record execution start
- [ ] Record completion/failure/skip
- [ ] Include all relevant fields

```go
type WorkflowExecutionAudit struct {
    WorkflowExecutionName string
    WorkflowID            string
    TargetResource        string
    Phase                 string
    PipelineRunName       string
    StartTime             time.Time
    CompletionTime        *time.Time
    FailureReason         *string
    SkipReason            *string
}
```

#### Afternoon (4h): Spec Validation

- [ ] Implement `validateSpec()` per controller-implementation.md
- [ ] Validate targetResource format
- [ ] Validate workflowRef fields
- [ ] Return ConfigurationError on failure

### Day 8 EOD Checklist

- [ ] Audit events sent to Data Storage
- [ ] Spec validation catches invalid input
- [ ] ConfigurationError reason used
- [ ] All error paths have audit

---

## Days 9-10: Testing

**Focus**: Test refinement, Integration, E2E tests per testing-strategy.md v5.1

> **Note (v3.5)**: Most unit tests were already written during TDD implementation (Days 1-8).
> Day 9 focuses on refinement, DescribeTable patterns, and Makefile targets.

### Day 9: Unit Test Refinement + Makefile (4h)

**Status of Unit Tests**: ✅ 101 tests already exist from TDD (Days 1-8)

#### Day 9 Refined Tasks

| Task | Priority | Rationale |
|------|----------|-----------|
| Add `DescribeTable` for pipelineRunName tests | P1 | Edge case coverage per 03-testing-strategy.mdc |
| Add `DescribeTable` for ExtractFailureDetails reason codes | P1 | 12 failure reasons need systematic coverage |
| Add `DescribeTable` for ValidateSpec edge cases | P1 | Multiple validation scenarios |
| Verify Makefile targets work | P1 | Required infrastructure |
| Generate coverage report | P2 | Validate 70%+ coverage |

#### DescribeTable Pattern Example (Required)

```go
// Per 03-testing-strategy.mdc: Use DescribeTable for similar scenarios
DescribeTable("pipelineRunName determinism",
    func(targetResource string, expectedPrefix string) {
        name1 := PipelineRunName(targetResource)
        name2 := PipelineRunName(targetResource)

        // Deterministic: Same input → same output
        Expect(name1).To(Equal(name2))
        Expect(name1).To(HavePrefix(expectedPrefix))
        Expect(len(name1)).To(Equal(20)) // wfe- + 16 hex chars
    },
    Entry("standard deployment", "prod/deployment/app", "wfe-"),
    Entry("statefulset", "prod/statefulset/db", "wfe-"),
    Entry("namespace with dash", "ns-1/deployment/app-2", "wfe-"),
    Entry("long resource name", "very-long-namespace/deployment/very-long-deployment-name", "wfe-"),
)

DescribeTable("ExtractFailureDetails reason mapping",
    func(tektonReason string, expectedReason FailureReason) {
        pr := createFailedPipelineRun(tektonReason)
        details := reconciler.ExtractFailureDetails(ctx, pr, nil)
        Expect(details.Reason).To(Equal(expectedReason))
    },
    Entry("OOM killed", "OOMKilled", FailureReasonOOMKilled),
    Entry("Permission denied", "Forbidden", FailureReasonForbidden),
    Entry("Deadline exceeded", "DeadlineExceeded", FailureReasonDeadlineExceeded),
    Entry("Image pull error", "ImagePullBackOff", FailureReasonImagePullBackOff),
    // ... all 12 reason codes
)
```

### Day 9 EOD Checklist (Revised v3.5)

- [ ] `DescribeTable` added for pipelineRunName (determinism, uniqueness)
- [ ] `DescribeTable` added for ExtractFailureDetails (all 12 reason codes)
- [ ] `DescribeTable` added for ValidateSpec (all validation scenarios)
- [ ] `make test-unit-workflowexecution` passes (4 parallel procs)
- [ ] `make test-coverage-workflowexecution` generates report
- [ ] Coverage report shows 70%+ unit coverage

---

### Day 10: Integration + E2E Tests (8h)

#### Morning (4h): Integration Tests

- [ ] CRD lifecycle with EnvTest
- [ ] Resource lock verification
- [ ] Cross-namespace PipelineRun creation

```go
var _ = Describe("Integration: Resource Locking", func() {
    It("should block second WFE on same target", func() {
        // Create first WFE
        wfe1 := createWFE("wfe-1", "production/deployment/app")
        Eventually(wfePhase(wfe1)).Should(Equal("Running"))

        // Create second WFE - should be Skipped
        wfe2 := createWFE("wfe-2", "production/deployment/app")
        Eventually(wfePhase(wfe2)).Should(Equal("Skipped"))
        Expect(wfe2.Status.SkipDetails.Reason).To(Equal("ResourceBusy"))
    })
})
```

#### Afternoon (4h): E2E Tests

- [ ] KIND cluster with Tekton
- [ ] Complete workflow execution
- [ ] Failure scenario

### Day 10 EOD Checklist

- [ ] Integration tests pass with EnvTest
- [ ] E2E tests pass with KIND + Tekton
- [ ] Resource locking verified
- [ ] `make test-integration test-e2e` passes

---

## Day 11: Documentation

**Focus**: User-facing docs, troubleshooting guide

### Deliverables

- [ ] Update main README.md
- [ ] Troubleshooting guide
- [ ] Runbook for common issues
- [ ] Workflow Author's Guide skeleton

### Documentation Updates

| Document | Updates |
|----------|---------|
| README.md | Link to WE service |
| docs/guides/workflow-authoring.md | How to create workflows |
| docs/operations/workflowexecution-runbook.md | Operational procedures |

### Day 11 EOD Checklist

- [ ] README updated
- [ ] Troubleshooting guide created
- [ ] Workflow Author's Guide skeleton
- [ ] All internal docs reviewed

---

## Day 12: Production Readiness

**Focus**: Security review, performance validation, handoff

### Production Readiness Checklist

#### Functional Validation (30%)

- [ ] All BR-WE-001 to BR-WE-011 covered
- [ ] Resource locking prevents parallel execution
- [ ] Cooldown prevents redundant execution
- [ ] Failure details extracted correctly

#### Operational Validation (25%)

- [ ] Metrics exposed and documented
- [ ] Structured logging with correlation IDs
- [ ] Health checks work (`/healthz`, `/readyz`)
- [ ] Graceful shutdown tested

#### Security Validation (15%)

- [ ] ClusterRole reviewed for least privilege
- [ ] ServiceAccount permissions documented
- [ ] No secrets in logs

#### Performance Validation (15%)

- [ ] PipelineRun creation < 5s
- [ ] Status sync < 10s
- [ ] No memory leaks under load

#### Documentation Validation (15%)

- [ ] All DDs referenced and current
- [ ] Runbook complete
- [ ] Handoff notes prepared

### Confidence Assessment

| Category | Score | Notes |
|----------|-------|-------|
| Functional | 95% | All BRs implemented |
| Operational | 92% | Metrics and logging complete |
| Security | 90% | RBAC reviewed |
| Performance | 88% | Tested in KIND, needs prod validation |
| Documentation | 95% | Complete spec docs |
| **Overall** | **92%** | Ready for production |

### Day 12 EOD Checklist

- [ ] Security review complete
- [ ] Performance benchmarks recorded
- [ ] Handoff notes written
- [ ] PR ready for review

---

## BR Coverage Matrix

**Coverage Targets** (per [03-testing-strategy.mdc](../../../../.cursor/rules/03-testing-strategy.mdc)):
- **Unit Tests**: 70%+ (all unit-testable BRs)
- **Integration Tests**: >50% (microservices mandate - CRD coordination, watch patterns)
- **E2E Tests**: 10-15% (critical user journeys)

### Coverage Summary

| Category | BRs | Unit | Integration | E2E | Coverage |
|----------|-----|------|-------------|-----|----------|
| Core Execution | BR-WE-001 to BR-WE-008 | ✅ | ✅ | ✅ | 100% |
| Resource Locking | BR-WE-009 to BR-WE-011 | ✅ | ✅ | ⬜ | 90% |
| **Total** | 11 | 11 | 11 | 8 | 97% |

### Per-BR Coverage

| BR | Description | Unit | Integration | E2E |
|----|-------------|------|-------------|-----|
| BR-WE-001 | Create PipelineRun from OCI Bundle | ✅ | ✅ | ✅ |
| BR-WE-002 | Pass Parameters to Execution Engine | ✅ | ✅ | ✅ |
| BR-WE-003 | Monitor Execution Status | ✅ | ✅ | ✅ |
| BR-WE-004 | Owner Reference for Cascade Deletion | ✅ | ✅ | ✅ |
| BR-WE-005 | Audit Events for Execution Lifecycle | ✅ | ✅ | ⬜ |
| BR-WE-006 | ServiceAccount Configuration | ✅ | ✅ | ⬜ |
| BR-WE-007 | Handle Externally Deleted PipelineRun | ✅ | ✅ | ⬜ |
| BR-WE-008 | Prometheus Metrics for Execution Outcomes | ✅ | ✅ | ⬜ |
| BR-WE-009 | Prevent Parallel Execution | ✅ | ✅ | ✅ |
| BR-WE-010 | Cooldown Period | ✅ | ✅ | ✅ |
| BR-WE-011 | Target Resource Identification | ✅ | ✅ | ✅ |
| BR-WE-012 | Exponential Backoff Cooldown | ⬜ | ⬜ | ⬜ |

---

## Appendix A: Code Examples

### Complete Reconcile Loop

```go
func (r *WorkflowExecutionReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
    log := log.FromContext(ctx)

    var wfe workflowexecutionv1.WorkflowExecution
    if err := r.Get(ctx, req.NamespacedName, &wfe); err != nil {
        return ctrl.Result{}, client.IgnoreNotFound(err)
    }

    // Handle deletion
    if !wfe.DeletionTimestamp.IsZero() {
        return r.reconcileDelete(ctx, &wfe)
    }

    // Add finalizer
    if !controllerutil.ContainsFinalizer(&wfe, workflowExecutionFinalizer) {
        controllerutil.AddFinalizer(&wfe, workflowExecutionFinalizer)
        if err := r.Update(ctx, &wfe); err != nil {
            return ctrl.Result{}, err
        }
    }

    // Reconcile based on phase
    switch wfe.Status.Phase {
    case "", workflowexecutionv1.PhasePending:
        return r.reconcilePending(ctx, &wfe)
    case workflowexecutionv1.PhaseRunning:
        return r.reconcileRunning(ctx, &wfe)
    case workflowexecutionv1.PhaseCompleted, workflowexecutionv1.PhaseFailed:
        return r.reconcileTerminal(ctx, &wfe)
    case workflowexecutionv1.PhaseSkipped:
        return ctrl.Result{}, nil
    default:
        log.Error(nil, "Unknown phase", "phase", wfe.Status.Phase)
        return ctrl.Result{RequeueAfter: 30 * time.Second}, nil
    }
}
```

---

## Appendix B: CRD Controller Patterns

### CRD API Group Standard (DD-CRD-001)

```go
// api/workflowexecution/v1alpha1/groupversion_info.go
package v1alpha1

import (
    "k8s.io/apimachinery/pkg/runtime/schema"
    "sigs.k8s.io/controller-runtime/pkg/scheme"
)

var (
    // GroupVersion is group version used to register these objects
    GroupVersion = schema.GroupVersion{Group: "kubernaut.ai", Version: "v1alpha1"}

    // SchemeBuilder is used to add go types to the GroupVersionKind scheme
    SchemeBuilder = &scheme.Builder{GroupVersion: GroupVersion}

    // AddToScheme adds the types in this group-version to the given scheme.
    AddToScheme = SchemeBuilder.AddToScheme
)
```

### Owner Reference Pattern

```go
// WorkflowExecution is owned by RemediationRequest
// PipelineRun cannot use OwnerReference (cross-namespace)
// Use labels + finalizer for cleanup instead

Labels: map[string]string{
    "kubernaut.ai/workflow-execution": wfe.Name,
    "kubernaut.ai/source-namespace":   wfe.Namespace,
}
```

---

## Performance Targets

| Metric | Target | Measurement |
|--------|--------|-------------|
| PipelineRun creation latency (P95) | < 5s | `pipelinerun_creation_duration_seconds` |
| Status sync latency | < 10s | Time from PR status change to WFE update |
| Lock check latency (P99) | < 50ms | Indexed CRD query |
| Memory usage | < 256MB | Per replica |
| CPU usage | < 0.5 cores | Average |
| Reconcile throughput | > 100/min | Under normal load |

---

## Common Pitfalls to Avoid

### ❌ Don't Do This:

1. **Skip resource locking tests**: Causes duplicate remediations in production
2. **Hardcode execution namespace**: Use configurable `ExecutionNamespace`
3. **Ignore AlreadyExists error**: Must handle race condition by marking Skipped
4. **Delete PR before cooldown**: Allows wasteful sequential executions
5. **Retry execution failures**: Cluster state unknown, requires manual review
6. **Use OwnerReference for PR**: Cross-namespace, use labels + finalizer instead
7. **Skip Tekton availability check**: Controller should crash-if-missing (ADR-030)
8. **Ignore WasExecutionFailure flag**: Critical for recovery decisions

### ✅ Do This Instead:

1. **Test all locking scenarios**: ResourceBusy, RecentlyRemediated, race conditions
2. **Configure namespace via ConfigMap**: Per ADR-030 configuration standards
3. **Handle AlreadyExists gracefully**: Mark WFE as Skipped with reason
4. **Wait for cooldown expiry**: Then delete PR to release lock
5. **Mark execution failures as terminal**: Set WasExecutionFailure=true
6. **Use labels for cross-namespace tracking**: `kubernaut.ai/workflow-execution`
7. **Check Tekton at startup**: Use `CheckTektonAvailable()` function
8. **Include WasExecutionFailure in FailureDetails**: For RO/AIAnalysis decisions

---

## Success Criteria

### Implementation Complete When:

- [ ] All BR-WE-001 to BR-WE-011 implemented
- [ ] Build passes without errors
- [ ] Zero lint errors
- [ ] Unit test coverage > 70%
- [ ] Integration tests pass with EnvTest
- [ ] E2E tests pass with KIND + Tekton
- [ ] All 10+ metrics exposed
- [ ] Health checks functional
- [ ] Documentation complete
- [ ] Production readiness validated (Day 12)

### Quality Indicators:

- **Code Quality**: No lint errors, follows Go idioms, consistent patterns
- **Test Quality**: Table-driven tests, clear assertions, BR mapping
- **Test Organization**: Unit/Integration/E2E separation per testing-strategy.md
- **Documentation Quality**: All spec docs current, cross-references valid
- **Production Readiness**: Score > 95/109 on Day 12 assessment

---

## Makefile Targets

**Note (v3.5)**: All test targets use 4 parallel processors by default per [03-testing-strategy.mdc](../../../../.cursor/rules/03-testing-strategy.mdc).
Lint targets are NOT service-specific - use `make lint` for all code.

```makefile
# Testing (4 parallel processors by default)
.PHONY: test-unit-workflowexecution
test-unit-workflowexecution: ## Run WorkflowExecution unit tests (4 parallel procs)
	ginkgo -v --timeout=5m --procs=4 ./test/unit/workflowexecution/...

.PHONY: test-integration-workflowexecution
test-integration-workflowexecution: ## Run WorkflowExecution integration tests (4 parallel procs, EnvTest)
	ginkgo -v --timeout=15m --procs=4 ./test/integration/workflowexecution/...

.PHONY: test-e2e-workflowexecution
test-e2e-workflowexecution: ## Run WorkflowExecution E2E tests (4 parallel procs, Kind + Tekton)
	ginkgo -v --timeout=15m --procs=4 ./test/e2e/workflowexecution/...

.PHONY: test-workflowexecution-all
test-workflowexecution-all: ## Run ALL WorkflowExecution tests (unit + integration + e2e, 4 parallel each)
	# Runs all 3 tiers with proper error handling

.PHONY: test-coverage-workflowexecution
test-coverage-workflowexecution: ## Run WorkflowExecution unit tests with coverage report
	go test -cover -coverprofile=coverage-workflowexecution.out -p 4 ./internal/controller/workflowexecution/... ./test/unit/workflowexecution/...
	go tool cover -html=coverage-workflowexecution.out -o coverage-workflowexecution.html

# Build
.PHONY: build-workflowexecution
build-workflowexecution: ## Build WorkflowExecution controller binary
	go build -o bin/workflowexecution-controller ./cmd/workflowexecution

# NOTE: No lint-workflowexecution target - use `make lint` for all code
```

---

## 📚 Appendices (Supplementary Documents)

This implementation plan is split into multiple files for manageability (~4,500 lines total). All appendices are cross-referenced:

| Appendix | File | Lines | Purpose |
|----------|------|-------|---------|
| **A** | [APPENDIX_A_ERROR_HANDLING.md](./APPENDIX_A_ERROR_HANDLING.md) | ~300 | Error Handling Philosophy (5 categories) |
| **B** | [APPENDIX_B_PRODUCTION_RUNBOOKS.md](./APPENDIX_B_PRODUCTION_RUNBOOKS.md) | ~400 | Production Runbooks (5 runbooks) |
| **C** | [APPENDIX_C_TEST_SCENARIOS.md](./APPENDIX_C_TEST_SCENARIOS.md) | ~400 | Test Scenarios (~100 tests detailed) |
| **D** | [APPENDIX_D_EOD_TEMPLATES.md](./APPENDIX_D_EOD_TEMPLATES.md) | ~350 | EOD Documentation Templates |
| **E** | [APPENDIX_E_METRICS.md](./APPENDIX_E_METRICS.md) | ~500 | Complete Metrics Code + Cardinality Audit |
| **F** | [APPENDIX_F_INTEGRATION_TESTS.md](./APPENDIX_F_INTEGRATION_TESTS.md) | ~600 | Complete Integration Test Examples |
| **G** | [APPENDIX_G_BR_COVERAGE_MATRIX.md](./APPENDIX_G_BR_COVERAGE_MATRIX.md) | ~400 | BR Coverage Matrix Template |
| **H** | [APPENDIX_H_E2E_TEST_SETUP.md](./APPENDIX_H_E2E_TEST_SETUP.md) | ~500 | E2E Test Setup + Helpers |

### Appendix Summary

- **Appendix A**: Defines 5 error categories (Validation, External, Permission, Execution, System) with code examples, retry strategies, metrics, and circuit breaker patterns
- **Appendix B**: Contains 5 production runbooks (RB-WE-001 to RB-WE-005) with alerts, diagnosis, resolution steps, and rollback plan
- **Appendix C**: Details 65 unit tests, 25 integration tests, and 10 E2E tests with table-driven patterns and test ID system
- **Appendix D**: Provides Day 1, Day 4, Day 7, and Day 12 EOD documentation templates plus Lessons Learned template
- **Appendix E**: Complete metrics implementation with 15 Prometheus metrics, recording patterns, cardinality audit, and testing examples
- **Appendix F**: 3 complete integration test files with ~400 lines of production-ready test code (Lifecycle, Locking, Finalizer)
- **Appendix G**: BR Coverage Matrix with calculation methodology, per-BR breakdown, gap analysis, and test file index
- **Appendix H**: E2E test infrastructure setup with Kind config, NodePort pattern, helpers, and business outcome tests

---

## References

### Specification Documents

| Document | Purpose |
|----------|---------|
| [overview.md](../overview.md) | Service architecture |
| [crd-schema.md](../crd-schema.md) | CRD type definitions |
| [controller-implementation.md](../controller-implementation.md) | Reconciler code |
| [testing-strategy.md](../testing-strategy.md) | Test patterns |
| [security-configuration.md](../security-configuration.md) | RBAC |
| [BUSINESS_REQUIREMENTS.md](../BUSINESS_REQUIREMENTS.md) | BR-WE-001 to BR-WE-011 |

### Architecture Decisions

| Document | Purpose |
|----------|---------|
| [DD-WE-001](../../../../architecture/decisions/DD-WE-001-resource-locking-safety.md) | Resource Locking |
| [DD-WE-002](../../../../architecture/decisions/DD-WE-002-dedicated-execution-namespace.md) | Execution Namespace |
| [DD-WE-003](../../../../architecture/decisions/DD-WE-003-resource-lock-persistence.md) | Lock Persistence |
| [ADR-044](../../../../architecture/decisions/ADR-044-workflow-execution-engine-delegation.md) | Tekton Delegation |
| [ADR-030](../../../../architecture/decisions/ADR-030-service-configuration-management.md) | Configuration Standards |

### Cross-Team

| Document | Purpose |
|----------|---------|
| [DD-CONTRACT-001](../../../../architecture/decisions/DD-CONTRACT-001-aianalysis-workflowexecution-alignment.md) | AIAnalysis ↔ WE Contract |
| [CONFIG_STANDARDS.md](../../../../configuration/CONFIG_STANDARDS.md) | Centralized Configuration |

### Template

| Document | Purpose |
|----------|---------|
| [SERVICE_IMPLEMENTATION_PLAN_TEMPLATE.md](../../../../services/SERVICE_IMPLEMENTATION_PLAN_TEMPLATE.md) | Implementation Plan Template (v3.0) |

---

## Revision History

| Version | Date | Changes | Author |
|---------|------|---------|--------|
| v3.1 | 2025-12-04 | Added 4 new appendices (E-H): Metrics, Integration Tests, BR Matrix, E2E Setup | AI Assistant |
| v3.0 | 2025-12-03 | Complete rewrite for Tekton + Resource Locking | AI Assistant |
| v2.0 | 2025-12-02 | Updated for `.ai` API group, BR-WE-* prefix | AI Assistant |
| v1.0 | 2025-11-28 | Initial plan | AI Assistant |

---

**Implementation Plan Status**: ✅ Ready for Implementation

**Template Compliance**: 100% aligned with [SERVICE_IMPLEMENTATION_PLAN_TEMPLATE.md](../../../../services/SERVICE_IMPLEMENTATION_PLAN_TEMPLATE.md) v3.0

**Document Structure** (9 documents, ~4,500 lines):
| Document | Lines | Content |
|----------|-------|---------|
| Main plan | ~1,050 | Core 12-day breakdown, code examples |
| Appendix A | ~300 | Error handling (5 categories, code) |
| Appendix B | ~400 | Runbooks (5 alerts, diagnosis, resolution) |
| Appendix C | ~400 | Test scenarios (~100 tests detailed) |
| Appendix D | ~350 | EOD templates (Day 1/4/7/12) |
| Appendix E | ~500 | Metrics implementation (15 metrics, code) |
| Appendix F | ~600 | Integration tests (3 complete test files) |
| Appendix G | ~400 | BR coverage matrix (11 BRs, per-BR breakdown) |
| Appendix H | ~500 | E2E test setup (Kind, NodePort, helpers) |
| **Total** | **~4,500** | **Comprehensive implementation guide** |

**Confidence Assessment**: 95%
- Complete code examples for all major components
- All 11 BRs mapped to specific tests
- Production runbooks with alerts
- Metrics implementation with cardinality audit

**Next Steps**:
1. Review with team
2. Create PR for Day 1 deliverables
3. Begin implementation following APDC-TDD methodology
4. Update EOD templates daily
5. Populate BR Coverage Matrix as tests are written


