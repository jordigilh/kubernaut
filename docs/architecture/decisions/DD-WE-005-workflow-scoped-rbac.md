# DD-WE-005: Workflow-Scoped RBAC via Schema-Declared Permissions

**Version**: 1.0
**Date**: 2026-02-23
**Status**: ✅ APPROVED
**Author**: WorkflowExecution Team
**Reviewers**: Platform Team, Gateway Team

---

## Context

DD-WE-002 established that all workflow executions run in a dedicated `kubernaut-workflows` namespace using a single ServiceAccount (`kubernaut-workflow-runner`) with a ClusterRole granting write access to common workload types across all namespaces.

This design has served well for initial development but introduces security concerns as the platform scales:

1. **Blast radius**: Every workflow job runs with the same permissions. A workflow targeting `HPA/api-frontend` in `demo-hpa` can also patch any Deployment in any namespace.
2. **Permission creep**: Each new resource type (HPA, PDB, PVC) requires expanding the shared ClusterRole, accumulating permissions that most workflows never need.
3. **No auditability**: There is no record of what permissions a workflow *should* need versus what it *has*. Security reviews cannot verify least-privilege per workflow.
4. **No isolation**: Concurrent workflow executions share identical permissions regardless of their scope.

### Triggering Incident

During the `hpa-maxed` demo scenario, the workflow job needed `get` and `patch` on `horizontalpodautoscalers` in the `autoscaling` API group. This permission was missing from the shared ClusterRole, causing a `Forbidden` error. Adding it to the shared role would grant HPA write access to *every* workflow, even those that only need to restart a Deployment.

---

## Decision

**Workflow schemas declare their required RBAC rules. The WE controller provisions a short-lived, scoped ServiceAccount with exactly those permissions for each workflow execution.**

This replaces the shared `kubernaut-workflow-runner` ClusterRole with per-execution RBAC derived from the workflow's own schema declaration.

---

## Scenarios Evaluated

### Scenario 1: Shared ServiceAccount (Current State)

A single SA (`kubernaut-workflow-runner`) in `kubernaut-workflows` with a ClusterRole granting write access to common workload types.

```
kubernaut-workflow-runner SA
  └── ClusterRole: get/list/patch deployments, statefulsets, daemonsets (all namespaces)
      └── Used by ALL workflow jobs
```

**Pros**:
- Simple setup, single ClusterRole to manage
- No per-execution overhead

**Cons**:
- Every workflow has identical permissions regardless of need
- New resource types require expanding the shared ClusterRole
- No audit trail of intended vs actual permissions
- Blast radius is the entire cluster for supported resource types

**Decision**: Replaced by Scenario 3

---

### Scenario 2: Dynamic Target-Resource-Inferred RBAC

The WE controller infers write permissions from the `targetResource` field (`namespace/kind/name`) and provisions a scoped SA per execution with:
- Built-in `view` ClusterRole for read-only cluster access
- Namespaced Role with `get` + `patch` on the specific target resource (using `resourceNames` for instance-level scoping)

```
Per-execution SA (wfe-<hash>)
  ├── ClusterRoleBinding → built-in "view" (read-only cluster-wide)
  └── RoleBinding → Role in target namespace
      └── get, patch on <kind>/<name> only
```

**Pros**:
- Least-privilege: write access only to the specific target resource instance
- No schema changes required
- Backward compatible with existing workflows

**Cons**:
- Assumes the workflow only needs to write to its target resource, which is not always true (e.g., a workflow may need to create a ConfigMap, update a related Service, or patch multiple resources)
- The WE engine must infer the API group from the Kind, which can be ambiguous (see DD-WE-005-related issue #184)
- No audit trail of *intended* permissions -- the inference is implicit

**Decision**: Rejected in favor of Scenario 3. Inference is fragile and does not capture the workflow author's intent.

---

### Scenario 3: Schema-Declared RBAC (Selected)

The workflow schema includes an `rbac` section that explicitly declares the Kubernetes RBAC rules the workflow requires. The WE controller uses these rules to provision a scoped SA per execution.

```yaml
# workflow-schema.yaml
rbac:
  rules:
    - apiGroups: ["autoscaling"]
      resources: ["horizontalpodautoscalers"]
      verbs: ["get", "patch"]
```

```
Per-execution SA (wfe-<hash>)
  ├── ClusterRoleBinding → built-in "view" (read-only cluster-wide)
  └── RoleBinding → Role in target namespace
      └── Rules from workflow schema declaration
```

**Pros**:
- Explicit: workflow authors declare exactly what their workflow needs
- Auditable: the RBAC declaration is stored in the workflow catalog alongside the schema, visible during review and approval
- Precise: supports multi-resource workflows (e.g., patch HPA + read ConfigMap)
- Immutable: RBAC rules are registered with the schema, not embedded in the container image where they could drift
- Extensible: future policy engines can validate rules at registration time

**Cons**:
- Requires schema extension and catalog storage update
- Workflow authors must understand Kubernetes RBAC rules
- Slightly more complex registration flow

**Decision**: Selected

---

## Implementation

### Workflow Schema Extension

Add an `rbac` field to the workflow schema:

```yaml
metadata:
  workflowId: patch-hpa-v1
  version: "1.0.0"

actionType: PatchHPA

rbac:
  rules:
    - apiGroups: ["autoscaling"]
      resources: ["horizontalpodautoscalers"]
      verbs: ["get", "patch"]

labels:
  signalName: HPAMaxedOut
  severity: [low, medium, high, critical]
  # ...

execution:
  engine: job
  bundle: quay.io/kubernaut-cicd/test-workflows/patch-hpa-job@sha256:...

parameters:
  - name: TARGET_NAMESPACE
    type: string
    required: true
  - name: TARGET_HPA
    type: string
    required: true
```

### Go Types

```go
// WorkflowSchema gains an RBAC field
type WorkflowSchema struct {
    Metadata       WorkflowSchemaMetadata `yaml:"metadata" json:"metadata"`
    ActionType     string                 `yaml:"actionType" json:"actionType"`
    RBAC           *WorkflowRBAC          `yaml:"rbac,omitempty" json:"rbac,omitempty"`
    Labels         WorkflowSchemaLabels   `yaml:"labels" json:"labels"`
    // ... existing fields
}

// WorkflowRBAC declares the Kubernetes RBAC rules the workflow requires.
type WorkflowRBAC struct {
    Rules []WorkflowRBACRule `yaml:"rules" json:"rules" validate:"required,min=1,dive"`
}

// WorkflowRBACRule mirrors rbacv1.PolicyRule for workflow permission declarations.
type WorkflowRBACRule struct {
    APIGroups []string `yaml:"apiGroups" json:"apiGroups" validate:"required"`
    Resources []string `yaml:"resources" json:"resources" validate:"required"`
    Verbs     []string `yaml:"verbs" json:"verbs" validate:"required"`
}
```

### Per-Execution RBAC Lifecycle

```
WFE Pending
  │
  ├─ 1. Parse RBAC rules from WorkflowExecution spec (propagated from catalog)
  ├─ 2. Create SA "wfe-<hash>" in kubernaut-workflows
  ├─ 3. Create ClusterRoleBinding → built-in "view" for SA
  ├─ 4. Create Role in target namespace with declared rules
  ├─ 5. Create RoleBinding in target namespace binding SA → Role
  ├─ 6. Create Job/PipelineRun with SA = "wfe-<hash>"
  │
  │  ... workflow executes ...
  │
  ├─ 7. Job completes
  ├─ 8. Cleanup: delete CRB, Role, RoleBinding (SA garbage-collected via Job OwnerRef)
  │
  └─ WFE Completed/Failed
```

### RBAC Resource Naming

All per-execution RBAC resources use deterministic names derived from the WFE:

| Resource | Name | Namespace | Cleanup |
|----------|------|-----------|---------|
| ServiceAccount | `wfe-<hash>` | `kubernaut-workflows` | OwnerRef to Job |
| ClusterRoleBinding | `wfe-<hash>-reader` | cluster-scoped | Explicit delete in Cleanup() |
| Role | `wfe-<hash>-writer` | target namespace | Explicit delete in Cleanup() |
| RoleBinding | `wfe-<hash>-writer` | target namespace | Explicit delete in Cleanup() |

All resources are labeled with `kubernaut.ai/workflow-execution: <wfe-name>` for orphan detection.

### Cluster-Scoped Targets

For cluster-scoped resources (e.g., `Node/worker-1`), the write permissions use a ClusterRole + ClusterRoleBinding instead of a namespaced Role:

| Resource | Name | Cleanup |
|----------|------|---------|
| ClusterRole | `wfe-<hash>-writer` | Explicit delete in Cleanup() |
| ClusterRoleBinding | `wfe-<hash>-writer` | Explicit delete in Cleanup() |

### WE Controller Permissions

The WE controller's own ClusterRole needs permissions to manage the per-execution RBAC:

```yaml
# Additional rules for the workflowexecution-controller ClusterRole
- apiGroups: [""]
  resources: ["serviceaccounts"]
  verbs: ["create", "delete", "get"]
- apiGroups: ["rbac.authorization.k8s.io"]
  resources: ["roles", "rolebindings", "clusterroles", "clusterrolebindings"]
  verbs: ["create", "delete", "get", "bind", "escalate"]
```

### Data Flow: Schema Registration to Execution

```
1. Workflow author writes schema with rbac.rules
2. DataStorage stores RBAC rules in workflow catalog
3. HAPI selects workflow → returns RBAC rules in SelectedWorkflow
4. RO creates WFE CRD with rbacRules from SelectedWorkflow
5. WE controller reads rbacRules from WFE spec
6. WE controller provisions scoped SA + Role + RoleBinding
7. Job/PipelineRun runs with scoped SA
8. WE controller cleans up RBAC resources on completion
```

### WFE CRD Extension

```go
type WorkflowExecutionSpec struct {
    // ... existing fields

    // RBACRules declares the Kubernetes RBAC rules for this execution.
    // Propagated from the workflow schema's rbac.rules field.
    // When empty, the execution falls back to the shared kubernaut-workflow-runner SA.
    RBACRules []RBACRule `json:"rbacRules,omitempty"`
}

type RBACRule struct {
    APIGroups []string `json:"apiGroups"`
    Resources []string `json:"resources"`
    Verbs     []string `json:"verbs"`
}
```

### Backward Compatibility

Workflows without `rbac` in their schema fall back to the existing shared `kubernaut-workflow-runner` SA. This allows incremental migration: existing workflows continue to work, new workflows opt in to scoped RBAC.

### Future Enhancement: Instance-Level Scoping via `resourceNames` (Pending #184)

Once issue #184 (propagate full GVK through the pipeline) is implemented, the WE controller
can further narrow write permissions to the specific target resource instance using Kubernetes
`resourceNames` field:

```yaml
# Future: WE controller injects resourceNames for the target resource rule only
- apiGroups: ["autoscaling"]
  resources: ["horizontalpodautoscalers"]
  resourceNames: ["api-frontend"]    # injected by WE from targetResource
  verbs: ["get", "patch"]

# Non-target rules remain namespace-scoped (no resourceNames)
- apiGroups: [""]
  resources: ["configmaps"]
  verbs: ["get", "list"]
```

The matching logic requires both the API group and plural resource name from the WFE spec to
identify which rule corresponds to the target resource. This avoids false-positive matches
when multiple rules share the same resource name across different API groups.

**This enhancement is deferred until #184 provides the full GVK in the WFE spec.** Without the
API group, matching on resource name alone carries a small risk of misapplying `resourceNames`
to the wrong rule. The current design (namespace-scoped Role without `resourceNames`) provides
meaningful blast radius reduction while remaining safe.

---

## Affected Components

| Component | Team | Change |
|-----------|------|--------|
| Workflow schema types | DataStorage | Add `RBAC` field to `WorkflowSchema` |
| DataStorage API | DataStorage | Store and return RBAC rules with catalog entries |
| HAPI workflow selection | HAPI | Pass RBAC rules through `SelectedWorkflow` response |
| RO controller | RemediationOrchestrator | Propagate `rbacRules` from SelectedWorkflow to WFE spec |
| WFE CRD | WorkflowExecution | Add `rbacRules` field |
| WE controller | WorkflowExecution | Provision and clean up per-execution RBAC |
| WE Helm chart | WorkflowExecution | Update controller ClusterRole, deprecate shared SA |
| Workflow schemas | Workflow Authors | Add `rbac` section to existing schemas |

---

## Consequences

### Positive

1. **Least-privilege**: Each workflow execution runs with only the permissions it declares
2. **Auditable**: RBAC requirements are part of the workflow schema, visible during review
3. **No permission creep**: The shared ClusterRole does not grow as new resource types are added
4. **Isolation**: Concurrent executions have independent, scoped permissions
5. **Self-documenting**: The schema declares what the workflow does (parameters) and what it needs (RBAC)

### Negative

1. **Per-execution overhead**: Creating and cleaning up 4 RBAC resources per execution adds latency (~200-500ms)
   - Mitigation: Negligible compared to workflow execution time (typically 30s+)
2. **Complexity**: WE controller manages RBAC lifecycle in addition to job lifecycle
   - Mitigation: Encapsulated in a dedicated `RBACProvisioner` component
3. **Cross-namespace cleanup**: Role and RoleBinding in target namespace cannot use OwnerReferences
   - Mitigation: Explicit cleanup in `Cleanup()` + orphan detection via labels

### Neutral

1. **Workflow authors**: Must declare RBAC rules, but this aligns with the existing Kubernetes permission model they already understand
2. **Existing workflows**: Continue to work via backward-compatible fallback to shared SA

---

## Related Documents

- [DD-WE-002: Dedicated Execution Namespace](./DD-WE-002-dedicated-execution-namespace.md) -- Establishes `kubernaut-workflows` namespace pattern (this DD extends it)
- [DD-WE-003: Resource Lock Persistence](./DD-WE-003-resource-lock-persistence.md) -- Deterministic naming used for RBAC resources
- [Issue #183: EM spec hash empty for HPA](https://github.com/jordigilh/kubernaut/issues/183) -- Triggered investigation into RBAC gaps
- [Issue #184: Propagate full GVK through pipeline](https://github.com/jordigilh/kubernaut/issues/184) -- Prerequisite for `resourceNames` instance-level scoping
- [Issue #186: Implement workflow-scoped RBAC](https://github.com/jordigilh/kubernaut/issues/186) -- Implementation tracking

---

## Document Maintenance

| Date | Version | Changes |
|------|---------|---------|
| 2026-02-23 | 1.0 | Initial decision - schema-declared RBAC replacing shared SA |
