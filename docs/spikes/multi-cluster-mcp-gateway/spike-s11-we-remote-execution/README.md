# Spike S11: WE Remote Execution via MCP Gateway

**Date**: 2026-06-22
**Status**: Proposed
**Objective**: Validate that WE can recreate its local workflow execution model on remote clusters
through the MCP Gateway, and identify the prerequisite infrastructure that must be pre-provisioned
by GitOps or fleet management reconciliation.
**Authority**: ADR-068 design decision #9 (unified MCP Gateway chokepoint), Issue #54

## Problem Statement

WE currently executes remediation workflows (K8s Jobs, Tekton PipelineRuns, Ansible/AWX jobs) on
the local hub cluster using direct `client-go` API calls. For multi-cluster federation, WE must
execute these same workflows on remote clusters through the MCP Gateway.

Three key architectural decisions constrain the solution:

1. **WE is a workflow submitter, not an infrastructure provisioner.** WE creates Jobs and
   PipelineRuns but does NOT create RBAC infrastructure (ServiceAccounts, ClusterRoles,
   ClusterRoleBindings). This infrastructure must pre-exist on the target cluster, provisioned
   by the cluster administrator via GitOps (ArgoCD, Flux), fleet management (ACM, Rancher),
   or manual setup.

2. **The MCP Gateway is the single chokepoint.** WE connects to the MCP Gateway, not directly
   to remote K8s API servers. All remote operations go through the K8s MCP Server's tool
   interface (`resources_create_or_update`, `tekton_pipeline_start`, etc.).

3. **Runtime dependency validation replaces pre-flight checks.** The existing two-phase
   dependency validation (DS at registration, WE at execution) is removed in favor of
   K8s-native runtime validation. See [Dependency Validation Model Change](#dependency-validation-model-change).

## Spike Questions

| # | Question | Priority |
|---|----------|----------|
| Q1 | Can `resources_create_or_update` create a `batch/v1 Job` with the same spec WE uses locally? | P0 |
| Q2 | Can `tekton_pipeline_start` create a PipelineRun with SA, params, and workspace bindings matching WE's local spec? | P0 |
| Q3 | Can `resources_get` poll Job/PipelineRun status to drive WE's reconcile loop? | P0 |
| Q4 | Can `resources_create_or_update` delete a completed Job/PipelineRun (cleanup)? Or is a separate `resources_delete` needed? | P1 |
| Q5 | What is the latency overhead of MCP tool calls vs direct client-go for workflow lifecycle operations? | P1 |
| Q6 | Does K8s produce actionable error messages when a Job fails due to missing Secret/ConfigMap volume mounts? | P1 |
| Q7 | Can WE's Ansible executor mint a SA token on the remote cluster via MCP, or must the token be pre-provisioned? | P2 |

## Local Execution Model: What WE Does Today

### Category A: What WE Creates Per Workflow (must work remotely)

**K8s Job** (`JobExecutor.Create()`):
```yaml
apiVersion: batch/v1
kind: Job
metadata:
  name: wfe-<sha256[:16]>          # deterministic, idempotent
  namespace: kubernaut-workflows   # execution namespace
  labels:
    kubernaut.ai/workflow-execution: <wfe-name>
    kubernaut.ai/managed: "true"
spec:
  backoffLimit: 0
  ttlSecondsAfterFinished: 600
  template:
    spec:
      serviceAccountName: <from-catalog>   # SA must pre-exist
      restartPolicy: Never
      containers:
        - name: workflow
          image: <from-catalog>
          env:
            - name: TARGET_RESOURCE
              value: <ns/kind/name>
            # + all schema-filtered parameters as env vars
          volumeMounts:
            - name: secret-<dep>
              mountPath: /run/kubernaut/secrets/<dep>
            - name: configmap-<dep>
              mountPath: /run/kubernaut/configmaps/<dep>
      volumes:
        - name: secret-<dep>
          secret:
            secretName: <dep-secret>       # must pre-exist
        - name: configmap-<dep>
          configMap:
            name: <dep-configmap>          # must pre-exist
```

**Tekton PipelineRun** (`TektonExecutor.Create()`):
```yaml
apiVersion: tekton.dev/v1
kind: PipelineRun
metadata:
  name: wfe-<sha256[:16]>
  namespace: kubernaut-workflows
  labels:
    kubernaut.ai/workflow-execution: <wfe-name>
spec:
  pipelineRef:
    resolver: bundles                      # Tekton bundle resolver
    params:
      - name: bundle
        value: <bundle-image>
      - name: name
        value: <pipeline-name>
  taskRunTemplate:
    serviceAccountName: <from-catalog>     # SA must pre-exist
  params:
    - name: TARGET_RESOURCE
      value: <ns/kind/name>
    # + schema-filtered parameters
  workspaces:
    - name: secret-<dep>
      secret:
        secretName: <dep-secret>
    - name: configmap-<dep>
      configMap:
        name: <dep-configmap>
```

**Ansible/AWX** (`AnsibleExecutor`):
- Creates NO K8s resource -- calls AWX REST API directly
- Mints a short-lived K8s token via `TokenRequest` for the workflow SA
- Injects token as AWX credential so Ansible playbooks can call K8s API
- Out of scope for this spike (AAP MCP Server handles this independently)

### Category B: What Must Pre-Exist (cluster admin responsibility)

| Resource | Name | Namespace | Purpose | Provisioned By |
|----------|------|-----------|---------|----------------|
| Namespace | `kubernaut-workflows` | — | Execution namespace for all workflow pods | Helm / GitOps |
| ServiceAccount | Per-workflow (from catalog) | `kubernaut-workflows` | Identity for workflow runner pods | Helm / GitOps |
| ClusterRole | `kubernaut-workflow-runner` | — | RBAC ceiling for remediation actions (scale, restart, patch, drain, etc.) | Helm / GitOps |
| ClusterRoleBinding | Binds SA → ClusterRole | — | Grants workflow runner permissions | Helm / GitOps |
| Secret(s) | Dependency secrets | `kubernaut-workflows` | Credentials, tokens, configs injected into workflow pods | Operator / GitOps |
| ConfigMap(s) | Dependency configmaps | `kubernaut-workflows` | Configuration data injected into workflow pods | Operator / GitOps |
| Tekton CRDs | Pipeline, Task definitions | — | Required if using Tekton engine (installed via Tekton operator) | Cluster admin |
| Pipeline(s) | Referenced by PipelineRun | `kubernaut-workflows` | The actual pipeline definitions (or use bundle resolver) | GitOps |

## K8s MCP Server Tool Coverage for WE

### Available Tools (toolsets: `core` + `tekton`)

| WE Operation | MCP Tool | Toolset | Coverage | Notes |
|-------------|----------|---------|----------|-------|
| Create Job | `resources_create_or_update` | `core` | **FULL** | Accepts arbitrary K8s resource YAML/JSON via `apiVersion` + `kind` + `resource` body |
| Create PipelineRun | `tekton_pipeline_start` | `tekton` | **PARTIAL** | Creates PipelineRun from Pipeline reference + params. Does NOT support: bundle resolver, workspace bindings, SA override, custom labels. See gap analysis. |
| Create PipelineRun (alt) | `resources_create_or_update` | `core` | **FULL** | Can create PipelineRun with full spec (bundles, workspaces, SA). Fallback if `tekton_pipeline_start` is insufficient. |
| Get Job status | `resources_get` | `core` | **FULL** | Returns full Job spec+status including `.status.conditions`, `.status.succeeded`, `.status.failed` |
| Get PipelineRun status | `resources_get` | `core` | **FULL** | Returns full PipelineRun spec+status |
| List Jobs by label | `resources_list` + labelSelector | `core` | **FULL** | Supports `labelSelector=kubernaut.ai/workflow-execution=<name>` |
| Delete Job | `resources_delete` | `core` | **FULL** | Supports delete with propagation policy |
| Delete PipelineRun | `resources_delete` | `core` | **FULL** | Supports delete |
| Get TaskRun (status) | `resources_get` | `core` | **FULL** | For Tekton step-level status |
| TaskRun logs | `tekton_taskrun_logs` | `tekton` | **FULL** | Domain-specific log retrieval for Tekton |

### Gap Analysis: `tekton_pipeline_start` Limitations

The `tekton_pipeline_start` tool creates a PipelineRun by referencing a Pipeline name + params.
WE's local Tekton executor uses features that `tekton_pipeline_start` may not support:

| Feature | WE Local | `tekton_pipeline_start` | `resources_create_or_update` |
|---------|----------|------------------------|------------------------------|
| Pipeline reference (name) | Yes | Yes | Yes |
| Pipeline reference (bundle resolver) | Yes | **Unknown** | Yes |
| Params | Yes | Yes | Yes |
| `taskRunTemplate.serviceAccountName` | Yes | **Unknown** | Yes |
| Workspace bindings (secrets, configmaps) | Yes | **No** | Yes |
| Custom labels | Yes | **No** | Yes |
| Custom name (deterministic) | Yes | **No** (auto-generated) | Yes |

**Recommendation**: Use `resources_create_or_update` for PipelineRun creation instead of
`tekton_pipeline_start`. This gives WE full control over the PipelineRun spec, including
bundle resolver, workspace bindings, SA, labels, and deterministic naming. The `tekton`
toolset remains useful for `tekton_taskrun_logs` and status queries.

## Validation Plan

### Prerequisites

- OCP 4.21 cluster (or Kind with Tekton Pipelines installed)
- K8s MCP Server deployed with `--toolsets=core,tekton` (no `--read-only`)
- MCP Gateway deployed (or direct MCP client connection for spike)
- Pre-provisioned infrastructure:
  - Namespace `kubernaut-workflows`
  - ServiceAccount `spike-workflow-runner` with remediation ClusterRole
  - Test Secret `spike-dep-secret` with dummy data
  - Test ConfigMap `spike-dep-config` with dummy data
  - Test Tekton Pipeline `spike-remediation-pipeline` (if testing Tekton)

### Test Cases

| # | Test | Tool | Validates |
|---|------|------|-----------|
| S11-001 | Create a `batch/v1 Job` via `resources_create_or_update` with SA, env vars, volume mounts for secrets/configmaps | `resources_create_or_update` | Q1: Job creation parity with local |
| S11-002 | Poll Job status via `resources_get` until completion | `resources_get` | Q3: Status polling |
| S11-003 | Delete completed Job via `resources_delete` | `resources_delete` | Q4: Cleanup |
| S11-004 | Create a `tekton.dev/v1 PipelineRun` via `resources_create_or_update` with SA, params, workspace bindings | `resources_create_or_update` | Q2: PipelineRun creation parity |
| S11-005 | Poll PipelineRun status via `resources_get`, including child TaskRun status | `resources_get` | Q3: Tekton status polling |
| S11-006 | Retrieve TaskRun logs | `tekton_taskrun_logs` | Tekton observability |
| S11-007 | Delete completed PipelineRun | `resources_delete` | Q4: Tekton cleanup |
| S11-008 | Verify Job fails correctly when SA does not exist — extract error from `.status.conditions` | `resources_get` | Pre-provisioning validation |
| S11-009 | Verify Job fails with actionable error when dependency Secret does not exist (volume mount failure) | `resources_get` | Q6: Runtime dep validation error quality |
| S11-010 | Verify Job fails with actionable error when dependency ConfigMap does not exist | `resources_get` | Q6: Runtime dep validation error quality |
| S11-011 | Measure round-trip latency: create Job → poll status → delete | — | Q5: Latency overhead |
| S11-012 | Create Job via MCP Gateway (not direct) with OAuth2 auth | MCP Gateway | End-to-end auth chain |

### Latency Expectations

| Operation | Local (client-go) | Expected via MCP | Acceptable? |
|-----------|-------------------|------------------|-------------|
| Create Job | <10ms | 50-200ms | Yes (one-time per workflow) |
| Get Job status | <5ms | 20-100ms | Yes (polling interval is 5-30s) |
| Delete Job | <10ms | 50-200ms | Yes (one-time at cleanup) |

WE's reconcile loop polls at 5-30s intervals. MCP overhead of 50-200ms per status check is
negligible relative to the poll interval. Workflow creation and deletion are one-time operations
per workflow execution, not latency-critical.

## Pre-Provisioning Manifest (Remote Cluster)

This is the infrastructure that must exist on each remote cluster before WE can submit workflows.
Managed by GitOps, ACM PolicyGenerator, Rancher Fleet, or manual setup.

```yaml
apiVersion: v1
kind: Namespace
metadata:
  name: kubernaut-workflows
  labels:
    kubernaut.ai/managed: "true"
---
apiVersion: v1
kind: ServiceAccount
metadata:
  name: kubernaut-workflow-runner
  namespace: kubernaut-workflows
---
apiVersion: rbac.authorization.k8s.io/v1
kind: ClusterRole
metadata:
  name: kubernaut-workflow-runner
rules:
  - apiGroups: ["apps"]
    resources: ["deployments", "statefulsets", "daemonsets"]
    verbs: ["get", "list", "patch", "update"]
  - apiGroups: ["apps"]
    resources: ["replicasets"]
    verbs: ["get", "list", "watch"]
  - apiGroups: [""]
    resources: ["pods"]
    verbs: ["get", "list", "delete", "watch"]
  - apiGroups: [""]
    resources: ["pods/eviction"]
    verbs: ["create"]
  - apiGroups: [""]
    resources: ["configmaps"]
    verbs: ["get", "list", "create", "update", "patch"]
  - apiGroups: [""]
    resources: ["nodes"]
    verbs: ["get", "list", "patch", "update"]
  - apiGroups: [""]
    resources: ["namespaces"]
    verbs: ["get", "list"]
  - apiGroups: [""]
    resources: ["services"]
    verbs: ["get", "list", "create", "update", "patch"]
  - apiGroups: [""]
    resources: ["secrets"]
    verbs: ["get", "list", "create", "delete", "patch", "update"]
  - apiGroups: ["policy"]
    resources: ["poddisruptionbudgets"]
    verbs: ["get", "list", "patch"]
  - apiGroups: ["autoscaling"]
    resources: ["horizontalpodautoscalers"]
    verbs: ["get", "list", "patch"]
  - apiGroups: ["networking.k8s.io"]
    resources: ["networkpolicies"]
    verbs: ["get", "list", "create", "update", "patch", "delete"]
  - apiGroups: [""]
    resources: ["persistentvolumeclaims"]
    verbs: ["get", "list", "create", "update", "patch", "delete"]
  - apiGroups: ["batch"]
    resources: ["jobs"]
    verbs: ["get", "list", "create", "delete"]
---
apiVersion: rbac.authorization.k8s.io/v1
kind: ClusterRoleBinding
metadata:
  name: kubernaut-workflow-runner
roleRef:
  apiGroup: rbac.authorization.k8s.io
  kind: ClusterRole
  name: kubernaut-workflow-runner
subjects:
  - kind: ServiceAccount
    name: kubernaut-workflow-runner
    namespace: kubernaut-workflows
```

## Dependency Validation Model Change

### Problem: TOCTOU and Multi-Cluster Invalidity

The current dependency validation (DD-WE-006) has two phases:

1. **Registration-time** (DataStorage): When a workflow is registered via `POST /api/v1/workflows`,
   DS calls `DependencyValidator.ValidateDependencies()` to check that declared Secrets/ConfigMaps
   exist in `kubernaut-workflows` on the hub cluster. If missing, HTTP 400 rejects the registration.

2. **Execution-time** (WE controller): When a WFE enters `Pending`, `resolveSchemaMetadata()`
   re-validates dependencies using an informer-backed K8s client. If missing, the WFE is marked
   `Failed` permanently.

Both phases suffer from **TOCTOU (Time-of-Check/Time-of-Use)**: a dependency can disappear between
the validation check and the point where the Job pod actually mounts the volume. For remote
clusters, this problem compounds:

- **DS registration-time check is meaningless for remote execution.** DS validates deps on the hub,
  but the workflow runs on a remote cluster where those deps may or may not exist. DS has no
  access to remote clusters.
- **WE pre-flight check via MCP adds latency and TOCTOU.** Each dep check would require an MCP
  round-trip (`resources_get` via gateway), and deps could disappear between the check and the
  `resources_create_or_update` call that creates the Job.
- **WE has no informer on remote clusters.** The informer-backed cached client only watches the
  hub cluster. For remote clusters, every check is a point-in-time API call.

### Decision: Runtime Validation via K8s Scheduler

**Remove the `DependencyValidator` pre-flight check from both DS and WE, for both local and
remote workflows.** Normalize behavior so workflows work the same way regardless of target
cluster.

The workflow schema continues to declare dependencies (for volume mounting). K8s validates
that the referenced Secrets/ConfigMaps exist at pod scheduling time. If a dependency is
missing, K8s fails the pod with an actionable error that WE surfaces in the WFE status.

**Rationale:**
- Eliminates TOCTOU everywhere (local and remote)
- Consistent behavior: same workflow, same error path, regardless of cluster
- Simplifies the codebase: removes `DependencyValidator` interface, `K8sDependencyValidator`,
  and the validation slot in DS's `validateExternalChecks`
- Removes `datastorage-dep-reader` and `workflowexecution-dep-reader` Roles from RBAC surface
- Aligns with the "WE is a workflow submitter" principle: WE trusts that the execution
  environment is ready, just like it trusts that SAs and ClusterRoles are pre-provisioned

### What Changes

| Component | Current | After |
|-----------|---------|-------|
| DS `HandleCreateWorkflow` | Slot 2 calls `DependencyValidator.ValidateDependencies()` → HTTP 400 if missing | Slot 2 removed. Schema structural validation (`WorkflowDependencies.ValidateDependencies()`) still runs (non-empty names, uniqueness). |
| WE `resolveSchemaMetadata` | Calls `r.DependencyValidator.ValidateDependencies()` → WFE `Failed` if missing | Removed. Dependencies flow directly to executor's `buildDependencyVolumes()` / `buildDependencyWorkspaces()`. |
| `DependencyValidator` interface | Exists in `pkg/datastorage/validation/` | Removed |
| `K8sDependencyValidator` | Exists in `pkg/datastorage/validation/` | Removed |
| `datastorage-dep-reader` Role | RBAC in `charts/kubernaut/templates/rbac/` (verbs: `get`) | Removed |
| `workflowexecution-dep-reader` Role | RBAC in `charts/kubernaut/templates/rbac/` (verbs: `get`, `list`, `watch`) | Removed |
| WFE failure on missing dep | Pre-flight: `ConfigurationError` reason, no Job created | Runtime: Job created, pod fails to schedule, WFE marked `Failed` with K8s error extracted from Job `.status.conditions` |

### Error Experience

**Before (pre-flight validation):**
```
WFE Status:
  phase: Failed
  reason: ConfigurationError
  message: "Schema-declared dependency not satisfied: Secret 'my-creds' not found in namespace 'kubernaut-workflows'"
```

**After (runtime K8s validation):**
```
WFE Status:
  phase: Failed
  reason: ExecutionFailed
  message: "Job wfe-a1b2c3d4 failed: pod scheduling error: secret 'my-creds' not found"
```

The error is still actionable -- it names the missing resource and the namespace. The difference
is timing: the error comes from the K8s scheduler after Job creation instead of from a pre-flight
check before Job creation. For operators, the remediation is the same: provision the missing
Secret and re-trigger the workflow.

### Impact on Workflow Registration

Removing DS's dep validation means a workflow can be registered even if its declared dependencies
don't exist yet. This is **intentional** for multi-cluster:

- A workflow is registered once in the DS catalog
- It can execute on any managed cluster where the deps are provisioned
- The deps don't need to exist on the hub at registration time
- If deps are missing on the target cluster, the WFE fails at runtime with a clear error

This aligns with the GitOps model: workflows and their dependencies are deployed independently.
A workflow definition can be pushed before the dependency Secrets are provisioned, and the
execution will succeed as soon as the deps are available.

## Architectural Impact

### WE Controller Changes

WE's executors (`JobExecutor`, `TektonExecutor`) currently use `client-go` directly. For remote
execution, they need an adapter that translates K8s API calls into MCP tool calls:

```
Local path:   WE Controller → client-go → Hub K8s API
Remote path:  WE Controller → MCP client → MCP Gateway → K8s MCP Server → Remote K8s API
```

The adapter decision (interface-level abstraction vs executor fork vs MCP-native executor) is
out of scope for this spike. This spike validates that the MCP tool surface supports the
operations WE needs. The implementation approach is a separate design decision.

### K8s MCP Server Deployment Changes

The MCP server deployment must enable the `tekton` toolset and remove `--read-only`:

```yaml
args:
  - "--port=9090"
  - "--stateless"
  - "--toolsets=core,config,tekton"    # add tekton
  - "--log-level=2"
  - "--disable-multi-cluster"
```

### Separation of Concerns

| Concern | Owner | Mechanism |
|---------|-------|-----------|
| RBAC infrastructure (SA, ClusterRole, CRB) | Cluster admin | GitOps / fleet management / manual |
| Dependency secrets and configmaps | Cluster admin / operator | GitOps / sealed-secrets / external-secrets |
| Tekton Pipeline/Task definitions | Cluster admin | GitOps / Tekton bundle registry |
| Workflow submission (Job/PipelineRun) | WE controller | MCP Gateway → K8s MCP Server |
| Workflow status polling | WE controller | MCP Gateway → K8s MCP Server |
| Workflow cleanup | WE controller | MCP Gateway → K8s MCP Server |
| MCP Gateway auth (JWT/OPA) | Platform team | Authorino / Keycloak |
| K8s MCP Server RBAC | Cluster admin | ClusterRole `mcp-operator` |

## Exit Criteria

| Criterion | Confidence Target |
|-----------|-------------------|
| Job creation via `resources_create_or_update` matches local spec fidelity | 95% |
| PipelineRun creation via `resources_create_or_update` matches local spec fidelity | 95% |
| Status polling via `resources_get` provides sufficient data for WE reconcile loop | 95% |
| K8s produces actionable errors for missing deps (volume mount failures) extractable from Job `.status.conditions` | 90% |
| Pre-provisioning manifest is complete and documented | 90% |
| Latency overhead is acceptable for WE's polling-based reconcile model | 90% |
| **Overall spike confidence** | **90%+** |

If confidence reaches 90%+, proceed to implementation. If below, document blockers.

## References

- ADR-068: Fleet Federation Architecture (design decision #9)
- DD-WE-006: Workflow Dependency Validation (superseded by runtime validation for multi-cluster)
- Spike S1: OCP MCP Server Tool Coverage Matrix
- Spike S8: Real K8s MCP Server with envtest
- [kubernetes-mcp-server `core` toolset](https://github.com/containers/kubernetes-mcp-server)
- [kubernetes-mcp-server `tekton` toolset](https://github.com/containers/kubernetes-mcp-server/pull/892)
