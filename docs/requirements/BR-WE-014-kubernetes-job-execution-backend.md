# BR-WE-014: Kubernetes Job Execution Backend

**Business Requirement ID**: BR-WE-014
**Category**: Workflow Engine Service
**Priority**: **P1 (HIGH)** - Architectural Fitness + E2E Test Enablement
**Target Version**: **V1.0** (pending WE team estimation)
**Status**: Proposed - Pending WE Team Review
**Date**: February 5, 2026
**Related ADRs**: ADR-043 (Execution Engine Schema), ADR-044 (Engine Portability)
**Related BRs**: BR-WE-002 (PipelineRun Creation), BR-WE-003 (Status Monitoring), BR-WE-009 (Resource Locking)
**GitHub Issue**: [#44](https://github.com/jordigilh/kubernaut/issues/44)

---

## Business Need

### Problem Statement

The WorkflowExecution controller currently hardcodes Tekton as the sole execution backend. This creates two concrete business problems:

**1. Architectural Over-Engineering for Simple Remediations**

Many real-world remediations are single-step operations ("restart pod", "scale deployment", "increase memory limit") that do not require Tekton's multi-step pipeline machinery. Forcing all workflow executions through Tekton adds:
- Unnecessary startup latency (~15-30s for PipelineRun scheduling vs ~3-5s for Job)
- Heavyweight infrastructure dependency (Tekton controllers: ~500-800MB RAM)
- Operational complexity (Tekton CRDs, resolvers, bundles) for operations that are fundamentally a single container execution

**2. Full Platform E2E Test Blocked by CI Memory Constraints**

A complete end-to-end platform test (K8s Event -> Gateway -> SignalProcessing -> AIAnalysis -> RemediationOrchestrator -> WorkflowExecution -> Notification) currently requires Tekton controllers, pushing total Kind cluster memory to ~5-6GB and exceeding the 6GB CI runner limit. This means the platform's full remediation pipeline has **never been tested end-to-end in CI**.

**Impact Without This BR**:
- Single-step remediations carry Tekton overhead unnecessarily
- Full platform E2E test impossible in CI (6GB limit)
- `execution_engine` field in workflow catalog (ADR-043) is dead code
- No path to supporting alternative execution backends (K8s Jobs, future Ansible/AAP per Issue #45)

---

## Business Objective

**WorkflowExecution Controller SHALL support Kubernetes Jobs as a second execution backend alongside Tekton, using a Strategy pattern to dispatch execution based on `spec.executionEngine`.**

### Success Criteria

1. WorkflowExecution CRD accepts `spec.executionEngine` field with values `"tekton"` (default) and `"job"`
2. Existing WorkflowExecution CRs without `executionEngine` continue using Tekton (backward-compatible)
3. When `executionEngine: "job"`, controller creates a `batchv1.Job` instead of a Tekton PipelineRun
4. Job executor maps Job conditions (`Complete`/`Failed`) to WFE phases identically to Tekton mapping
5. Resource locking (BR-WE-009), cooldown (BR-WE-010), and audit trail (BR-WE-005) apply regardless of backend
6. RemediationOrchestrator propagates `execution_engine` from workflow catalog to WorkflowExecution CRD
7. Full platform E2E test (OOMKill scenario) passes in CI within 6GB memory using Job backend
8. Tekton E2E tests remain unchanged and pass

---

## Use Cases

### Use Case 1: Single-Step OOMKill Remediation via K8s Job

**Scenario**: Pod `my-app` is OOMKilled. Kubernaut detects the event, analyzes it, selects a "restart-with-increased-memory" workflow cataloged with `execution_engine: "job"`.

```
1. K8s Event: Pod OOMKilled
2. Gateway receives event, creates RemediationRequest
3. SignalProcessing classifies signal, triggers AIAnalysis
4. AIAnalysis produces RCA, RemediationOrchestrator selects workflow
5. RO creates WorkflowExecution CRD:
   spec:
     executionEngine: "job"  # Propagated from catalog
     workflowRef:
       name: restart-with-increased-memory
       containerImage: registry.example.com/remediation/restart:v1.2@sha256:abc...
     parameters:
       podName: my-app
       namespace: production
       memoryLimit: 1Gi
6. WE Controller dispatches to JobExecutor
7. JobExecutor creates batchv1.Job:
   - Container: registry.example.com/remediation/restart:v1.2@sha256:abc...
   - Env vars from parameters
   - ServiceAccount with scoped RBAC
8. Job completes -> WFE phase: Succeeded
9. Notification sent to operators
```

**Execution time**: ~5s (vs ~20-30s with Tekton pipeline overhead)

### Use Case 2: Multi-Step Deployment via Tekton (Unchanged)

**Scenario**: Complex deployment requiring pre-checks, canary rollout, health validation, and rollback capability.

```
1. WorkflowExecution CRD created:
   spec:
     executionEngine: "tekton"  # (or empty = default)
     workflowRef:
       name: canary-deployment
       bundleRef: oci://registry.example.com/pipelines/canary:v2.0
2. WE Controller dispatches to TektonExecutor (existing behavior)
3. Tekton PipelineRun created with multi-step tasks
4. PipelineRun completes -> WFE phase: Succeeded
```

**No change from current behavior.**

### Use Case 3: Full Platform E2E Test in CI

**Scenario**: CI pipeline runs complete OOMKill remediation test within 6GB.

```
1. Kind cluster starts with: Gateway, SP, AIAnalysis, RO, WE, Notification, DataStorage
   Total memory: ~3-4GB (no Tekton controllers needed)
2. Test triggers OOMKill event
3. Full pipeline executes using Job backend
4. Verification: notification received, audit trail complete
5. CI passes within 6GB runner limit
```

---

## Technical Requirements

### TR-1: CRD Schema Extension

Add `executionEngine` field to `WorkflowExecutionSpec`:

```go
// WorkflowExecutionSpec defines the desired state of WorkflowExecution
type WorkflowExecutionSpec struct {
    // ... existing fields ...

    // ExecutionEngine specifies which backend to use for this execution.
    // Supported values: "tekton" (default), "job" (Kubernetes Job).
    // When empty or omitted, defaults to "tekton" for backward compatibility.
    // +kubebuilder:default="tekton"
    // +kubebuilder:validation:Enum=tekton;job
    // +optional
    ExecutionEngine string `json:"executionEngine,omitempty"`
}
```

**Backward Compatibility**: Empty/omitted `executionEngine` defaults to `"tekton"`. All existing CRs continue to use Tekton.

### TR-2: Executor Interface (Strategy Pattern)

```go
// pkg/workflowexecution/executor/executor.go
type Executor interface {
    // Create creates the execution primitive (Job or PipelineRun)
    Create(ctx context.Context, wfe *v1alpha1.WorkflowExecution) error

    // GetStatus reads the current status of the execution primitive
    // and maps it to a WorkflowExecution phase
    GetStatus(ctx context.Context, wfe *v1alpha1.WorkflowExecution) (Phase, error)

    // Delete removes the execution primitive (respects cooldown)
    Delete(ctx context.Context, wfe *v1alpha1.WorkflowExecution) error
}
```

### TR-3: TektonExecutor (Refactor, No Behavior Change)

Extract existing Tekton-specific logic from the reconciler into a `TektonExecutor` struct implementing the `Executor` interface. This is a pure refactor -- no changes to Tekton behavior.

### TR-4: JobExecutor (New)

```go
// pkg/workflowexecution/executor/job_executor.go
type JobExecutor struct {
    Client client.Client
}
```

**Behavior**:
- `Create`: Creates `batchv1.Job` with:
  - Container image from `WorkflowRef.ContainerImage`
  - Parameters injected as environment variables
  - ServiceAccount from spec (scoped RBAC)
  - Owner reference to WorkflowExecution CRD (garbage collection)
  - `backoffLimit: 0` (no automatic retry -- retry managed by WE controller per BR-WE-012)
- `GetStatus`: Maps Job conditions:
  - `type: Complete, status: True` -> WFE Phase `Succeeded`
  - `type: Failed, status: True` -> WFE Phase `Failed` with failure details
  - Neither -> WFE Phase `Running`
- `Delete`: Deletes the Job with `PropagationPolicy: Background`

### TR-5: Controller Dispatch

```go
func (r *WorkflowExecutionReconciler) getExecutor(wfe *v1alpha1.WorkflowExecution) executor.Executor {
    switch wfe.Spec.ExecutionEngine {
    case "job":
        return r.jobExecutor
    default: // "tekton" or empty
        return r.tektonExecutor
    }
}
```

### TR-6: RO Integration -- Catalog Propagation

RemediationOrchestrator MUST propagate `execution_engine` from the workflow catalog to the WorkflowExecution CRD `spec.executionEngine` field when creating WFE resources.

**Location**: `pkg/remediationorchestrator/creator/`

### TR-7: OCI Compliance

K8s Job executor uses the container image directly from `WorkflowRef.ContainerImage`, which MUST be an OCI image reference with digest (e.g., `registry.example.com/remediation/restart:v1.2@sha256:abc...`). This ensures:
- Immutable workflow definitions tracked by OCI digest
- Audit trail records exact image executed
- Consistent with Tekton bundle approach for audit compliance

---

## Cross-Cutting Concerns

### Resource Locking (BR-WE-009)

Resource locking MUST apply regardless of execution backend. The lock check occurs BEFORE executor dispatch, so no changes needed.

### Cooldown (BR-WE-010)

Cooldown periods MUST apply regardless of execution backend. Cooldown check occurs before dispatch.

### Audit Trail (BR-WE-005)

Audit events MUST include `executionEngine` field to differentiate Job vs Tekton executions:

```json
{
  "event_type": "workflowexecution.started",
  "event_data": {
    "execution_engine": "job",
    "workflow_name": "restart-with-increased-memory",
    "container_image": "registry.example.com/remediation/restart:v1.2@sha256:abc..."
  }
}
```

### Block Clearing (BR-WE-013)

Block clearing mechanism is execution-engine-agnostic. No changes needed.

### Metrics (BR-WE-008)

Existing metrics MUST include `execution_engine` label:

```go
workflowExecutionDuration.WithLabelValues(executionEngine, outcome).Observe(duration)
```

---

## Acceptance Criteria

```gherkin
Feature: Kubernetes Job Execution Backend

  Background:
    Given WorkflowExecution controller is running
    And Executor interface with TektonExecutor and JobExecutor are registered

  Scenario: Job backend creates and monitors K8s Job
    Given a WorkflowExecution CRD with executionEngine: "job"
    And workflowRef.containerImage: "registry.example.com/remediation/restart:v1.2"
    When the controller reconciles the WorkflowExecution
    Then a batchv1.Job is created (not a PipelineRun)
    And the Job container uses the specified image
    And parameters are injected as environment variables
    And the Job has an owner reference to the WorkflowExecution
    When the Job completes successfully
    Then the WorkflowExecution phase transitions to Succeeded
    And an audit event is emitted with execution_engine: "job"

  Scenario: Job backend handles failure
    Given a WorkflowExecution CRD with executionEngine: "job"
    When the created Job fails
    Then the WorkflowExecution phase transitions to Failed
    And failure details include Job failure reason
    And wasExecutionFailure is set to true
    And subsequent executions are blocked (BR-WE-012)

  Scenario: Backward compatibility -- empty executionEngine defaults to Tekton
    Given a WorkflowExecution CRD without executionEngine field
    When the controller reconciles the WorkflowExecution
    Then a Tekton PipelineRun is created (existing behavior)
    And no batchv1.Job is created

  Scenario: Backward compatibility -- explicit Tekton
    Given a WorkflowExecution CRD with executionEngine: "tekton"
    When the controller reconciles the WorkflowExecution
    Then a Tekton PipelineRun is created (existing behavior)

  Scenario: Resource locking applies to Job backend
    Given a WorkflowExecution CRD with executionEngine: "job"
    And another WorkflowExecution for the same targetResource is running
    When the controller reconciles the second WorkflowExecution
    Then execution is blocked by resource lock (BR-WE-009)
    And no Job is created

  Scenario: RO propagates execution_engine from catalog
    Given a workflow in the catalog with execution_engine: "job"
    When RemediationOrchestrator creates a WorkflowExecution CRD
    Then spec.executionEngine is set to "job"

  Scenario: Full platform E2E -- OOMKill with Job backend
    Given a Kind cluster with all services (no Tekton controllers)
    And total cluster memory under 4GB
    When an OOMKill event is triggered
    Then the full pipeline executes: Gateway -> SP -> AI -> RO -> WE -> Notification
    And WorkflowExecution uses Job backend
    And the remediation completes successfully
    And notification is delivered
    And audit trail is complete
```

---

## Implementation Scope

| Area | Change | Scope | Risk |
|------|--------|-------|------|
| **CRD** | Add `executionEngine` field to `WorkflowExecutionSpec` | Small, additive | Low -- backward-compatible default |
| **Interface** | Create `Executor` interface in `pkg/workflowexecution/executor/` | New file | Low -- clean abstraction |
| **Tekton backend** | Extract existing Tekton logic into `TektonExecutor` | Refactor | Medium -- must preserve all existing behavior |
| **Job backend** | New `JobExecutor` implementing `Executor` | New file | Low -- well-defined K8s API |
| **Controller** | Dispatch to executor based on `spec.executionEngine` | ~10 lines | Low |
| **RO integration** | Pass `execution_engine` from catalog to WFE CRD | Small | Low |
| **Tests** | Unit + Integration + E2E for both backends | Significant | Medium -- Tekton refactor needs regression coverage |

---

## Deliverables

### Phase 1: Documentation (Before Implementation)

- [ ] ADR: Multi-engine workflow execution (amend ADR-044 or new ADR)
- [ ] DD: K8s Job executor design document
- [ ] Updated CRD schema documentation

### Phase 2: Implementation

- [ ] `Executor` interface + `TektonExecutor` extraction (refactor)
- [ ] `JobExecutor` implementation
- [ ] CRD field addition + defaulting webhook
- [ ] Controller dispatch logic
- [ ] RO catalog -> CRD propagation
- [ ] Audit event `execution_engine` field

### Phase 3: Testing

- [ ] Unit tests (both backends, table-driven)
- [ ] Integration tests (Job lifecycle, status mapping)
- [ ] E2E tests for Job backend
- [ ] Regression tests for Tekton backend (post-refactor)
- [ ] Full platform E2E test (OOMKill scenario using Job backend)

---

## Dependencies

| Dependency | Service | Status | Notes |
|------------|---------|--------|-------|
| Workflow catalog `execution_engine` field | DataStorage | Exists | ADR-043, `pkg/datastorage/models/workflow.go` |
| `batchv1` API | Kubernetes | Exists | Core API, no additional CRDs needed |
| Tekton controllers | Tekton | Exists | Only needed when `executionEngine: "tekton"` |
| WE controller RBAC | Kubernetes | Update needed | Add `batchv1/jobs` permissions |

---

## Related Requirements

| BR ID | Description | Relationship |
|-------|-------------|--------------|
| BR-WE-002 | PipelineRun Creation | Tekton-specific; generalized by Executor interface |
| BR-WE-003 | Status Monitoring | Generalized to monitor both Job and PipelineRun |
| BR-WE-005 | Audit Events | Extended with `execution_engine` field |
| BR-WE-008 | Prometheus Metrics | Extended with `execution_engine` label |
| BR-WE-009 | Resource Locking | Engine-agnostic; applies to both backends |
| BR-WE-010 | Cooldown Period | Engine-agnostic; applies to both backends |
| BR-WE-012 | Exponential Backoff | Engine-agnostic; applies to both backends |
| BR-WE-013 | Block Clearing | Engine-agnostic; no changes needed |

---

## Estimation Request

**@team: workflowexecution** -- Please review and provide:

1. **Feasibility**: Any concerns with the Strategy pattern or interface design?
2. **Scope estimate**: Achievable for v1.0, or target v1.1?
3. **Design refinements**: Edge cases in Job lifecycle (TTL, backoffLimit, pod failure)?
4. **Tekton extraction risk**: Concerns about refactoring Tekton-coupled code?

---

## Changelog

| Version | Date | Changes |
|---------|------|---------|
| 1.0 | 2026-02-05 | Initial BR: K8s Job execution backend (Issue #44) |

---

**Document Status**: Proposed -- Pending WE Team Review
**Version**: 1.0
**File**: `docs/requirements/BR-WE-014-kubernetes-job-execution-backend.md`
