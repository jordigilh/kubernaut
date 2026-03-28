# DD-WE-005: Per-Workflow ServiceAccount Reference

**Version**: 2.0
**Date**: 2026-03-04
**Status**: ✅ APPROVED (v2.0 supersedes v1.x schema-declared RBAC approach)
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

**Workflow schemas reference an optional, pre-existing ServiceAccount. The WE controller uses that SA for the Job/PipelineRun. If no SA is specified, Kubernetes assigns the namespace's `default` SA.**

This replaces both the shared `kubernaut-workflow-runner` SA (v1.0) and the schema-declared RBAC provisioning approach (v1.x) with a simpler, user-managed SA reference that follows the standard Kubernetes pattern used by Tekton, Argo Workflows, Flux CD, and Knative.

### Superseded Approach (v1.x): Schema-Declared RBAC

v1.0-1.2 of this document proposed that workflow schemas declare RBAC rules (`spec.rbac.rules`) and the WE controller dynamically provisions per-execution SA + Role + RoleBinding. This was rejected because:

- The WE controller required `escalate`/`bind` privileges, enlarging its threat vector
- Per-execution RBAC lifecycle (create + cleanup 4 resources) added latency and complexity
- Cross-namespace orphan cleanup required label-based sweeping
- Schema deny-list validation was needed to prevent dangerous permissions

The SA-reference approach eliminates all of these concerns. See issues #185, #186, #187 (closed, superseded by #481).

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

### Scenario 3: Schema-Declared RBAC (Superseded by Scenario 4)

The workflow schema includes an `rbac` section that explicitly declares the Kubernetes RBAC rules the workflow requires. The WE controller uses these rules to provision a scoped SA per execution.

**Pros**:
- Explicit RBAC declaration, auditable, precise for multi-resource workflows

**Cons**:
- WE controller requires `escalate`/`bind` privileges (enlarged threat vector)
- Per-execution RBAC lifecycle adds latency and complexity (4 resources to create/cleanup)
- Cross-namespace orphan cleanup required
- Schema deny-list validation needed

**Decision**: Superseded by Scenario 4

---

### Scenario 4: Per-Workflow ServiceAccount Reference (Selected)

The workflow schema includes an optional `serviceAccountName` field in the `execution` section. Operators pre-create ServiceAccounts with appropriate RBAC in the execution namespace. The WE controller uses the referenced SA for the Job/PipelineRun without any RBAC management.

```yaml
spec:
  execution:
    engine: job
    serviceAccountName: "hpa-workflow-sa"  # pre-created by operator
    bundle: "ghcr.io/kubernaut-cicd/test-workflows/patch-hpa-job@sha256:..."
```

```
Operator pre-creates:
  ├── ServiceAccount "hpa-workflow-sa" in kubernaut-workflows
  ├── Role in target namespace (get, patch HPA)
  └── RoleBinding binding SA → Role

WFE execution:
  └── Job runs as "hpa-workflow-sa" (from wfe.spec.executionConfig.serviceAccountName)
```

**Pros**:
- WE controller requires zero RBAC management privileges (no `escalate`/`bind`)
- Standard Kubernetes pattern (Tekton, Argo Workflows, Flux CD, Knative all use this)
- Users manage their own security posture; platform does not dictate allowed permissions
- No per-execution overhead (SA reused across executions of the same workflow)
- No orphan sweep, deny-list validation, or SSAR preflight needed
- Decouples SA provisioning from workflow registration

**Cons**:
- Operators must create SAs separately (not automated by the platform)
- No schema-level audit of intended permissions (trade-off for simplicity)
- If SA is missing, Job creation fails at K8s level (clear error, but not caught at registration time)

**Decision**: Selected

---

## Implementation (v2.0)

### Workflow Schema Extension

Add an optional `serviceAccountName` field to the `execution` section:

```yaml
apiVersion: kubernaut.ai/v1alpha1
kind: RemediationWorkflow
metadata:
  name: patch-hpa-v1
spec:
  version: "1.0.0"
  actionType: PatchHPA

  labels:
    severity: [low, medium, high, critical]
    # ...

  execution:
    engine: job
    serviceAccountName: "hpa-workflow-sa"  # pre-created by operator
    bundle: quay.io/kubernaut-cicd/test-workflows/patch-hpa-job@sha256:...

  parameters:
    - name: TARGET_NAMESPACE
      type: string
      required: true
    - name: TARGET_HPA
      type: string
      required: true
```

### Behavior

- If `serviceAccountName` is specified: Job/PipelineRun runs as that SA
- If `serviceAccountName` is absent: K8s assigns the namespace's `default` SA (standard K8s behavior)
- **Ansible engine (Issue #501)**: When `serviceAccountName` is specified, the AnsibleExecutor uses the K8s TokenRequest API to obtain a short-lived bearer token scoped to that SA. The token is injected into AWX as an ephemeral credential. When absent, the controller's own in-cluster SA credentials are used (Issue #500 fallback). If the API server shortens the granted token TTL below the WFE execution timeout, a `TokenTTLInsufficient` condition is set on the WFE and a `TokenTTLShortened` K8s warning event is emitted.
- WE controller has zero SA management responsibilities
- The shared `kubernaut-workflow-runner` SA and its ClusterRole/ClusterRoleBinding are removed

### Data Flow: Schema Registration to Execution

```
1. Workflow author writes schema with execution.serviceAccountName (optional)
2. DS parser extracts serviceAccountName; DS stores in service_account_name column
3. HAPI selects workflow → DS response includes serviceAccountName
4. HAPI validator extracts service_account_name → injects into selected_workflow
5. AA response processor maps service_account_name → SelectedWorkflow.ServiceAccountName
6. RO creator propagates ServiceAccountName → WFE.Spec.ServiceAccountName (top-level, engine-agnostic)
7. WE executor reads Spec.ServiceAccountName:
   - Job: sets PodSpec.ServiceAccountName
   - Tekton: sets TaskRunTemplate.ServiceAccountName
   - Ansible: calls TokenRequest API for a short-lived bearer token → injects into AWX as credential
8. K8s runs Pod as specified SA (Job/Tekton), or AWX playbook authenticates with the per-workflow token (Ansible)
```

### SA Lifecycle (Operator-Managed)

Operators create SAs, Roles, and RoleBindings independently:

```yaml
# ServiceAccount in execution namespace
apiVersion: v1
kind: ServiceAccount
metadata:
  name: hpa-workflow-sa
  namespace: kubernaut-workflows

# Role in target namespace
apiVersion: rbac.authorization.k8s.io/v1
kind: Role
metadata:
  name: hpa-workflow-role
  namespace: demo-hpa
rules:
  - apiGroups: ["autoscaling"]
    resources: ["horizontalpodautoscalers"]
    verbs: ["get", "patch"]

# RoleBinding
apiVersion: rbac.authorization.k8s.io/v1
kind: RoleBinding
metadata:
  name: hpa-workflow-binding
  namespace: demo-hpa
subjects:
  - kind: ServiceAccount
    name: hpa-workflow-sa
    namespace: kubernaut-workflows
roleRef:
  kind: Role
  name: hpa-workflow-role
  apiGroup: rbac.authorization.k8s.io
```

### WFE CRD (No Change Required)

`ExecutionConfig.ServiceAccountName` already exists on the WFE CRD. This feature activates it:

```go
type ExecutionConfig struct {
    Timeout            *metav1.Duration `json:"timeout,omitempty"`
    ServiceAccountName string           `json:"serviceAccountName,omitempty"`
}
```

### WE Controller Changes

- Remove `DefaultServiceAccountName` constant (`kubernaut-workflow-runner`)
- Remove `ServiceAccountName` field from reconciler struct
- Executors read SA from `wfe.Spec.ExecutionConfig.ServiceAccountName` instead of struct field
- If SA is empty, field is omitted from PodSpec/TaskRunTemplate (K8s assigns namespace default)
- No `escalate`/`bind` permissions needed on the WE controller ClusterRole

---

## Affected Components

| Component | Team | Change |
|-----------|------|--------|
| Workflow schema types | DataStorage | Add `ServiceAccountName` to `WorkflowExecution` struct |
| DS parser | DataStorage | `ExtractServiceAccountName()` extraction function |
| DS DB | DataStorage | `service_account_name TEXT` column in catalog |
| DS API/OAS | DataStorage | `serviceAccountName` field in REST response |
| HAPI validator | HAPI | Extract `service_account_name` from DS response |
| HAPI result parser | HAPI | Inject `service_account_name` into `selected_workflow` |
| AA response processor | AIAnalysis | Map to `SelectedWorkflow.ServiceAccountName` |
| RO creator | RemediationOrchestrator | Propagate SA to `WFE.Spec.ExecutionConfig.ServiceAccountName` |
| WFE CRD | WorkflowExecution | No change (field already exists) |
| WE executors | WorkflowExecution | Read SA from WFE spec, remove hardcoded default |
| WE controller | WorkflowExecution | Remove `ServiceAccountName` field, `DefaultServiceAccountName` |
| WE config | WorkflowExecution | Remove `ServiceAccount` from config struct |
| WE Helm chart | WorkflowExecution | Remove `kubernaut-workflow-runner` SA, ClusterRole, CRB |
| RW CRD types | RemediationWorkflow | Add `ServiceAccountName` to execution section |
| Demo workflows | Demo Team | Add `serviceAccountName` + operator SA manifests |

---

## Consequences

### Positive

1. **Minimal threat vector**: WE controller requires zero RBAC management privileges (no `escalate`/`bind`)
2. **Standard pattern**: Follows Tekton, Argo Workflows, Flux CD, and Knative SA assignment
3. **User-managed security**: Operators control exactly what each workflow can do
4. **No per-execution overhead**: SA is reused across executions of the same workflow
5. **Simple implementation**: No RBACProvisioner, orphan sweep, deny-list, or SSAR preflight
6. **Decoupled provisioning**: SA lifecycle is independent of workflow registration

### Negative

1. **Manual SA creation**: Operators must create SAs, Roles, and RoleBindings before workflow registration
   - Mitigation: Documentation with copy-paste manifests per demo scenario
2. **No registration-time validation**: Missing SA is caught at Job creation time, not registration
   - Mitigation: K8s provides clear error; operators can test SA existence with `kubectl auth can-i`
3. **No schema-level audit**: Permissions are not declared in the workflow schema
   - Mitigation: Operators can use `kubectl describe rolebinding` to audit

### Neutral

1. **Existing workflows**: `kubernaut-workflow-runner` is removed; workflows without SA use K8s `default` SA
2. **Ansible engine (Issue #501)**: `serviceAccountName` triggers a TokenRequest for per-workflow credentials; when absent, the controller SA is used as fallback (#500)

---

## Related Documents

- [DD-WE-002: Dedicated Execution Namespace](./DD-WE-002-dedicated-execution-namespace.md) -- Establishes `kubernaut-workflows` namespace pattern (SA lives here)
- [Issue #481: Per-workflow ServiceAccount reference](https://github.com/jordigilh/kubernaut/issues/481) -- Implementation tracking
- [Issue #185, #186, #187](https://github.com/jordigilh/kubernaut/issues/185) -- Superseded RBAC stanza issues (closed)
- [Test Plan](../../tests/481/TEST_PLAN.md) -- 23 test scenarios (16 unit + 7 integration)

---

## Document Maintenance

| Date | Version | Changes |
|------|---------|---------|
| 2026-02-23 | 1.0 | Initial decision - schema-declared RBAC replacing shared SA |
| 2026-03-02 | 1.1 | Added `schemaVersion: "1.1"` to RBAC schema examples (#255) |
| 2026-03-11 | 1.2 | Updated schema structure per #329: metadata.name, spec.version, spec.description, spec.maintainers; removed WorkflowSchemaMetadata |
| 2026-03-04 | 2.0 | **Major revision**: Replaced schema-declared RBAC (Scenario 3) with per-workflow SA reference (Scenario 4). Supersedes #185/#186/#187 in favor of #481. Removes all RBAC provisioning, orphan sweep, deny-list. WE controller no longer needs escalate/bind. |
