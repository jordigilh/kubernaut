# BR-WE-014: Kubernetes Job Execution Backend

**Business Requirement ID**: BR-WE-014
**Category**: Workflow Engine Service
**Priority**: **P1 (HIGH)** - Architectural Fitness + E2E Test Enablement
**Target Version**: **V1.0** (pending WE team estimation)
**Status**: Proposed - Pending WE Team Review
**Date**: February 5, 2026
**Related ADRs**: ADR-043 (Execution Engine Schema), ADR-044 (Engine Portability), DD-WE-006 (Schema-Declared Dependencies)
**Related BRs**: BR-WE-002 (PipelineRun Creation), BR-WE-003 (Status Monitoring), BR-WE-009 (Resource Locking), BR-WORKFLOW-004 (Schema Format)
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

1. WorkflowExecution CRD requires `spec.executionEngine` field (mandatory) with values `"tekton"` or `"job"`
2. CRD validation rejects WorkflowExecution resources that omit `executionEngine`
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
     executionEngine: "tekton"  # Explicit, required field
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
    // Supported values: "tekton" (Tekton PipelineRun), "job" (Kubernetes Job).
    // This field is REQUIRED -- the CRD creator (RemediationOrchestrator) MUST
    // explicitly set the engine based on the workflow catalog entry.
    // +kubebuilder:validation:Required
    // +kubebuilder:validation:Enum=tekton;job
    ExecutionEngine string `json:"executionEngine"`
}
```

**No backward compatibility**: This is a new mandatory field. Since WorkflowExecution CRDs are created programmatically by the RemediationOrchestrator (not by humans), there is no migration concern. The RO MUST populate this field from the workflow catalog's `execution_engine` column.

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
func (r *WorkflowExecutionReconciler) getExecutor(wfe *v1alpha1.WorkflowExecution) (executor.Executor, error) {
    switch wfe.Spec.ExecutionEngine {
    case "tekton":
        return r.tektonExecutor, nil
    case "job":
        return r.jobExecutor, nil
    default:
        return nil, fmt.Errorf("unsupported execution engine: %q (must be \"tekton\" or \"job\")", wfe.Spec.ExecutionEngine)
    }
}
```

### TR-6: RO Integration -- Catalog Propagation

RemediationOrchestrator MUST propagate `execution_engine` from the workflow catalog to the WorkflowExecution CRD `spec.executionEngine` field when creating WFE resources.

**Location**: `pkg/remediationorchestrator/creator/`

**Propagation Chain Gap**: The current data flow is:

```
Catalog (has execution_engine) → HolmesGPT-API → AIAnalysis.Status.SelectedWorkflow → RO Creator → WFE CRD
```

The `SelectedWorkflow` struct (`api/aianalysis/v1alpha1/aianalysis_types.go`) currently does **not** include `ExecutionEngine`. The following changes are required to complete the propagation chain:

1. **AIAnalysis CRD**: Add `ExecutionEngine string` field to `SelectedWorkflow` struct
2. **HolmesGPT-API**: Ensure workflow selection response includes `execution_engine` from catalog
3. **AIAnalysis response processor**: Extract and store `execution_engine` in `Status.SelectedWorkflow.ExecutionEngine`
4. **RO Creator**: Read `ai.Status.SelectedWorkflow.ExecutionEngine` and set `wfe.Spec.ExecutionEngine`

**Cross-service dependencies**: AIAnalysis (CRD + processor), HolmesGPT-API (response schema), RO (creator)

### TR-7: OCI Compliance

K8s Job executor uses the container image directly from `WorkflowRef.ContainerImage`, which MUST be an OCI image reference with digest (e.g., `registry.example.com/remediation/restart:v1.2@sha256:abc...`). This ensures:
- Immutable workflow definitions tracked by OCI digest
- Audit trail records exact image executed
- Consistent with Tekton bundle approach for audit compliance

---

## Cross-Cutting Concerns

### Resource Locking (BR-WE-009)

Resource locking MUST apply regardless of execution backend. The lock check occurs BEFORE executor dispatch. However, the **current implementation** checks for existing PipelineRuns to detect conflicts. When `executionEngine: "job"`, the lock check MUST also look for existing Jobs, or the lock will be bypassed.

**Required change**: Update resource lock conflict detection to check both `PipelineRun` and `Job` existence for the same target resource.

### Cooldown (BR-WE-010)

Cooldown periods MUST apply regardless of execution backend. Cooldown check occurs before dispatch.

### Audit Trail (BR-WE-005)

**ALL** audit events across the WorkflowExecution lifecycle MUST include `execution_engine` field to differentiate Job vs Tekton executions. This applies to every event type:

| Event Type | When Emitted |
|------------|--------------|
| `workflowexecution.selection.completed` | After workflow selection from spec |
| `workflowexecution.execution.started` | After Job/PipelineRun creation |
| `workflowexecution.workflow.started` | When execution begins running |
| `workflowexecution.workflow.completed` | On successful completion |
| `workflowexecution.workflow.failed` | On failure (includes error_details per Gap #7) |

```json
{
  "event_type": "workflowexecution.execution.started",
  "event_data": {
    "execution_engine": "job",
    "workflow_name": "restart-with-increased-memory",
    "container_image": "registry.example.com/remediation/restart:v1.2@sha256:abc..."
  }
}
```

### Schema-Declared Dependencies (DD-WE-006)

Workflows may declare infrastructure dependencies (Secrets, ConfigMaps) in their schema's `dependencies` section. At execution time, the WFE controller:

1. Queries Data Storage for the workflow's dependencies using `workflowRef.workflowId`
2. Validates each dependency exists in `kubernaut-workflows` with non-empty `.data`
3. Mounts dependencies into the execution resource

**Job backend**: Dependencies are mounted as volumes at well-known paths:
- Secrets: `/run/kubernaut/secrets/<name>/<key>`
- ConfigMaps: `/run/kubernaut/configmaps/<name>/<key>`

**Tekton backend**: Dependencies are added as workspace bindings on the PipelineRun:
- Secrets: workspace `secret-<name>`
- ConfigMaps: workspace `configmap-<name>`
- The Pipeline inside the OCI bundle must declare matching workspace names

**No CRD propagation**: Dependencies are NOT stored in the WFE spec. Workflows are immutable once registered (DD-WORKFLOW-012), so the WFE queries them on demand from Data Storage.

**RBAC**: The WFE controller needs a namespace-scoped Role in `kubernaut-workflows` granting `get` on `secrets` and `configmaps` for validation.

**Validation failure**: If any dependency is missing or has empty `.data`, the WFE marks the execution as Failed with `FailureDetails.Reason: ConfigurationError` and does not create the execution resource.

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
    And audit events include execution_engine: "job" at each lifecycle stage:
      | workflowexecution.selection.completed | After workflow selection |
      | workflowexecution.execution.started   | After Job creation      |
      | workflowexecution.workflow.started    | When Job starts running  |
    When the Job completes successfully
    Then the WorkflowExecution phase transitions to Succeeded
    And a workflowexecution.workflow.completed audit event is emitted with execution_engine: "job"

  Scenario: Job backend handles failure
    Given a WorkflowExecution CRD with executionEngine: "job"
    When the created Job fails
    Then the WorkflowExecution phase transitions to Failed
    And a workflowexecution.workflow.failed audit event is emitted with execution_engine: "job"
    And failure details include Job failure reason
    And wasExecutionFailure is set to true
    And subsequent executions are blocked (BR-WE-012)

  Scenario: Tekton backend via explicit executionEngine
    Given a WorkflowExecution CRD with executionEngine: "tekton"
    When the controller reconciles the WorkflowExecution
    Then a Tekton PipelineRun is created (existing behavior)

  Scenario: Missing executionEngine is rejected by CRD validation
    Given a WorkflowExecution CRD without executionEngine field
    When the CRD is submitted to the API server
    Then the API server rejects the resource with a validation error
    And no WorkflowExecution is created

  Scenario: Invalid executionEngine is rejected by CRD validation
    Given a WorkflowExecution CRD with executionEngine: "ansible"
    When the CRD is submitted to the API server
    Then the API server rejects the resource with an enum validation error
    And no WorkflowExecution is created

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

  Scenario: Job backend mounts declared dependencies as volumes
    Given a WorkflowExecution CRD with executionEngine: "job"
    And the workflow catalog entry declares dependencies:
      | type      | name              |
      | secret    | gitea-repo-creds  |
      | configMap | remediation-config|
    And Secret "gitea-repo-creds" exists in kubernaut-workflows with non-empty data
    And ConfigMap "remediation-config" exists in kubernaut-workflows with non-empty data
    When the controller reconciles the WorkflowExecution
    Then the controller queries Data Storage for workflow dependencies
    And validates each dependency exists with non-empty data
    And creates a Job with volume mounts:
      | volume           | mountPath                                  |
      | secret-gitea-repo-creds    | /run/kubernaut/secrets/gitea-repo-creds    |
      | configmap-remediation-config | /run/kubernaut/configmaps/remediation-config |
    And the workflow container can read files from the mounted paths

  Scenario: Tekton backend mounts declared dependencies as workspaces
    Given a WorkflowExecution CRD with executionEngine: "tekton"
    And the workflow catalog entry declares dependencies:
      | type   | name             |
      | secret | gitea-repo-creds |
    And Secret "gitea-repo-creds" exists in kubernaut-workflows with non-empty data
    When the controller reconciles the WorkflowExecution
    Then the PipelineRun includes workspace bindings:
      | workspace name            | secret name      |
      | secret-gitea-repo-creds   | gitea-repo-creds |

  Scenario: Dependency validation fails -- missing Secret
    Given a WorkflowExecution CRD with executionEngine: "job"
    And the workflow catalog entry declares dependency: secret "missing-creds"
    And Secret "missing-creds" does NOT exist in kubernaut-workflows
    When the controller reconciles the WorkflowExecution
    Then WFE is marked Failed with FailureDetails.Reason=ConfigurationError and message naming the missing resource
    And no Job is created

  Scenario: Dependency validation fails -- empty Secret
    Given a WorkflowExecution CRD with executionEngine: "job"
    And the workflow catalog entry declares dependency: secret "empty-secret"
    And Secret "empty-secret" exists in kubernaut-workflows but has empty data
    When the controller reconciles the WorkflowExecution
    Then WFE is marked Failed with FailureDetails.Reason=ConfigurationError and message naming the empty resource
    And no Job is created

  Scenario: Workflow with no dependencies proceeds normally
    Given a WorkflowExecution CRD with executionEngine: "job"
    And the workflow catalog entry has no dependencies section
    When the controller reconciles the WorkflowExecution
    Then the Job is created without additional volumes
    And existing behavior is unchanged

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

## Prerequisite Refactoring

The following normalizations MUST be completed before or as part of this BR's implementation to ensure the codebase is execution-engine-agnostic.

### PR-1: Normalize Condition Type Names (BR-WE-006 Update)

Rename Tekton-specific Kubernetes condition types to generic names:

| Current (Tekton-specific) | New (Engine-agnostic) |
|---------------------------|-----------------------|
| `ConditionTektonPipelineCreated` | `ConditionExecutionCreated` |
| `ConditionTektonPipelineRunning` | `ConditionExecutionRunning` |
| `ConditionTektonPipelineComplete` | `ConditionExecutionComplete` |
| `SetTektonPipelineCreated()` | `SetExecutionCreated()` |
| `SetTektonPipelineRunning()` | `SetExecutionRunning()` |
| `SetTektonPipelineComplete()` | `SetExecutionComplete()` |

**Impact**: ~200+ occurrences across ~35 files (constants, functions, controller, tests, docs).
**Tool**: `gopls rename` for type-safe refactoring across codebase.

### PR-2: Normalize CRD Status Fields

Rename Tekton-specific status fields to generic names:

| Current (Tekton-specific) | New (Engine-agnostic) |
|---------------------------|-----------------------|
| `PipelineRunRef` | `ExecutionRef` |
| `PipelineRunStatus` | `ExecutionStatus` |
| `PipelineRunStatusSummary` (type) | `ExecutionStatusSummary` |
| `BuildPipelineRunStatusSummary()` | `BuildExecutionStatusSummary()` |

**Impact**: ~100+ occurrences across ~40+ files (types, controller, audit, tests, OpenAPI schema, generated code).
**Note**: Requires deepcopy regeneration, CRD YAML regeneration, and OpenAPI schema update for `pipelinerun_name` audit field.
**Tool**: `gopls rename` + `controller-gen` + `make generate`.

### PR-3: Normalize Prometheus Metric Names

Rename Tekton-specific metric to generic name:

| Current (Tekton-specific) | New (Engine-agnostic) |
|---------------------------|-----------------------|
| `workflowexecution_reconciler_pipelinerun_creations_total` | `workflowexecution_reconciler_execution_creations_total` |
| `MetricNamePipelineRunCreations` | `MetricNameExecutionCreations` |
| `PipelineRunCreations` (struct field) | `ExecutionCreations` |
| `RecordPipelineRunCreation()` | `RecordExecutionCreation()` |

**Impact**: ~20 occurrences across ~12 files (metrics, controller, tests, docs).
**Tool**: `gopls rename` for Go symbols; manual update for metric name string.

### PR-4: Add `execution_engine` to OpenAPI Audit Payload

Add `execution_engine` field to `WorkflowExecutionAuditPayload` schema in `api/openapi/data-storage-v1.yaml`:

```yaml
execution_engine:
  type: string
  enum: [tekton, job]
  description: "Execution backend used for this workflow execution"
```

**Impact**: Requires OpenAPI client regeneration (ogen).

### PR-5: Add `ExecutionEngineJob` Constant

Add `"job"` constant to `pkg/datastorage/models/workflow.go`:

```go
const (
    ExecutionEngineTekton ExecutionEngine = "tekton"
    ExecutionEngineJob    ExecutionEngine = "job"  // NEW
)
```

### PR-6: Update ADR-043 Execution Engine Values

Update ADR-043 to include `"job"` as a V1 execution engine value (currently only lists `"tekton"` for V1).

---

## Implementation Scope

| Area | Change | Scope | Risk |
|------|--------|-------|------|
| **CRD** | Add required `executionEngine` field to `WorkflowExecutionSpec` | Small, additive | Low -- mandatory field, kubebuilder validation |
| **Interface** | Create `Executor` interface in `pkg/workflowexecution/executor/` | New file | Low -- clean abstraction |
| **Tekton backend** | Extract existing Tekton logic into `TektonExecutor` | Refactor | Medium -- must preserve all existing behavior |
| **Job backend** | New `JobExecutor` implementing `Executor` | New file | Low -- well-defined K8s API |
| **Controller** | Dispatch to executor based on `spec.executionEngine` | ~10 lines | Low |
| **RO integration** | Pass `execution_engine` from catalog to WFE CRD | Small | Low |
| **Tests** | Unit + Integration + E2E for both backends | Significant | Medium -- Tekton refactor needs regression coverage |

---

## Deliverables

### Phase 1: Documentation (Specs)

- [ ] ADR: Multi-engine workflow execution (amend ADR-044 or new ADR)
- [ ] DD: K8s Job executor design document
- [ ] Updated CRD schema documentation
- [ ] Test plans for unit, integration, and E2E tiers

### Phase 2: Testing (TDD RED -- Tests First)

Following TDD RED-GREEN-REFACTOR, tests are written **before** implementation at each tier.

#### 2a. Unit Tests (RED)

- [ ] `Executor` interface contract tests (table-driven, both backends)
- [ ] `JobExecutor` unit tests (Create, GetStatus, Delete)
- [ ] `TektonExecutor` regression tests (post-extraction, same behavior)
- [ ] Controller dispatch unit tests (mandatory field, enum validation)
- [ ] Audit event `execution_engine` field tests

#### 2b. Integration Tests (RED)

- [ ] Job lifecycle integration tests (create -> running -> complete/failed)
- [ ] Job status mapping to WFE phases
- [ ] RO catalog -> CRD `executionEngine` propagation

#### 2c. E2E Tests (RED)

- [ ] E2E test for Job backend (single-step remediation)
- [ ] Tekton E2E regression (existing behavior preserved)
- [ ] Full platform E2E test (OOMKill scenario using Job backend)

### Phase 3: Implementation (TDD GREEN + REFACTOR)

Minimal implementation to pass each test tier, then refactor.

- [ ] CRD field addition (required, enum-validated)
- [ ] `Executor` interface + `TektonExecutor` extraction (refactor from existing code)
- [ ] `JobExecutor` implementation
- [ ] Controller dispatch logic
- [ ] RO catalog -> CRD propagation
- [ ] Audit event `execution_engine` field
- [ ] REFACTOR: Clean up, reduce duplication, improve abstractions

---

## Dependencies

| Dependency | Service | Status | Notes |
|------------|---------|--------|-------|
| Workflow catalog `execution_engine` field | DataStorage | Exists | ADR-043, `pkg/datastorage/models/workflow.go` |
| `ExecutionEngineJob` constant | DataStorage | **New (PR-5)** | Add `"job"` constant alongside `"tekton"` |
| `SelectedWorkflow.ExecutionEngine` field | AIAnalysis | **New (G1)** | Add to `api/aianalysis/v1alpha1/` CRD types |
| HolmesGPT-API `execution_engine` in response | HolmesGPT-API | **New (G1)** | Include `execution_engine` from catalog in workflow selection response |
| `batchv1` API | Kubernetes | Exists | Core API, no additional CRDs needed |
| Tekton controllers | Tekton | Exists | Only needed when `executionEngine: "tekton"` |
| WE controller RBAC | Kubernetes | Update needed | Add `batchv1/jobs` permissions |
| OpenAPI `execution_engine` in audit payload | DataStorage | **New (PR-4)** | Add to `WorkflowExecutionAuditPayload` schema |
| Condition/status/metric normalization | WE | **New (PR-1..3)** | Prerequisite refactoring for engine-agnostic code |

---

## Related Requirements

| BR ID | Description | Relationship |
|-------|-------------|--------------|
| BR-WE-002 | PipelineRun Creation | Tekton-specific; generalized by Executor interface |
| BR-WE-003 | Status Monitoring | Generalized to monitor both Job and PipelineRun |
| BR-WE-005 | Audit Events | Extended with `execution_engine` field (PR-4) |
| BR-WE-006 | Kubernetes Conditions | Condition types normalized to engine-agnostic names (PR-1) |
| BR-WE-008 | Prometheus Metrics | Extended with `execution_engine` label; metric renamed (PR-3) |
| BR-WE-009 | Resource Locking | Updated to check both PipelineRun and Job (G4) |
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
| 2.0 | 2026-02-05 | **Major update**: Added prerequisite refactoring section (PR-1..6): normalize condition types, CRD status fields, metrics to engine-agnostic names; documented execution_engine propagation chain gap (G1: AIAnalysis → RO → WFE); updated resource locking to check both PipelineRun and Job (G4); added OpenAPI schema update (PR-4); added ExecutionEngineJob constant (PR-5); ADR-043 update for "job" engine (PR-6); expanded dependencies table with cross-service requirements |
| 1.3 | 2026-02-05 | Audit trail: `execution_engine` required in ALL lifecycle events (selection, started, completed, failed), not just completion |
| 1.2 | 2026-02-05 | Deliverables reordered to TDD methodology: Documentation (specs) -> Testing (RED) -> Implementation (GREEN + REFACTOR), phased by unit/integration/E2E tiers |
| 1.1 | 2026-02-05 | `executionEngine` field changed from optional (default "tekton") to mandatory; removed backward-compatibility scenarios; added CRD validation rejection scenarios |
| 1.0 | 2026-02-05 | Initial BR: K8s Job execution backend (Issue #44) |

---

**Document Status**: Proposed -- Pending WE Team Review
**Version**: 2.0
**File**: `docs/requirements/BR-WE-014-kubernetes-job-execution-backend.md`
