# DD-WE-006: Schema-Declared Infrastructure Dependencies (Secrets, ConfigMaps)

**Version**: 2.0
**Date**: 2026-02-24
**Status**: APPROVED
**Author**: WorkflowExecution Team
**Reviewers**: Platform Team, HAPI Team

---

## Context

Workflows that interact with external systems (Git repositories, APIs, databases) need
credentials and configuration that are provisioned at deployment time. Currently there is
no mechanism for a workflow to declare these dependencies:

1. **P17 (cert-failure-gitops)**: The workflow schema declares `GIT_USERNAME` and `GIT_PASSWORD`
   as LLM-provided parameters. The LLM guesses the password, causing authentication failures.
2. **P10 (gitops-drift)**: The `GIT_REPO_URL` parameter contains embedded credentials
   (`http://user:pass@host/repo.git`), relying on the LLM to construct the authenticated URL.

Both patterns violate a fundamental security boundary: **the LLM must not handle credentials**.
Credentials are an infrastructure concern, provisioned by operators and consumed by workflows.

### Relationship to DD-WE-005

DD-WE-005 established schema-declared RBAC: workflows declare the Kubernetes API permissions
they need, and the WFE provisions scoped ServiceAccounts at runtime. This decision extends
the same principle to infrastructure resources (Secrets, ConfigMaps) that workflows consume.

| Concern | DD-WE-005 | DD-WE-006 |
|---------|-----------|-----------|
| What | Kubernetes API permissions | Infrastructure resources (Secrets, ConfigMaps) |
| Declared in | `rbac.rules` | `dependencies.secrets`, `dependencies.configMaps` |
| Provisioned by | WFE (creates SA + Role) | Operator (creates Secret/ConfigMap at deploy time) |
| Validated by | WFE (before Job creation) | DS at registration + WFE at execution (dual validation) |
| Consumed via | ServiceAccount on Job | Volume mounts (Job) / workspace bindings (Tekton) |

### Triggering Incident

Issue #233: During P17 re-validation, the LLM correctly selected `GitRevertCommit` (confirming
the #219 prompt fix), but the WFE job failed on `git push` because the LLM populated
`GIT_PASSWORD=kubernaut-token` instead of the actual Gitea credential.

---

## Decision

**Workflow schemas declare infrastructure dependencies (Secrets, ConfigMaps) in a `dependencies`
section. Dependencies are validated at two points: by Data Storage at registration time (existence
+ non-empty data) and by the WFE at execution time (defense in depth). Dependencies are mounted
as volumes (Job executor) or workspace bindings (Tekton executor).**

**Dependencies are NOT propagated through CRDs.** Workflows are immutable once registered
(DD-WORKFLOW-012). The WFE queries Data Storage on demand using the workflow ID to retrieve
dependencies at execution time.

---

## Scenarios Evaluated

### Scenario 1: LLM-Provided Credentials (Current P17)

The workflow schema declares credential parameters (`GIT_USERNAME`, `GIT_PASSWORD`) that the
LLM must populate.

**Pros**: No schema extension needed.
**Cons**: Security violation (LLM guesses credentials), runtime failures, no validation.
**Decision**: Rejected. LLM must not handle credentials.

### Scenario 2: kubectl get secret in remediate.sh (Workaround)

The workflow script reads credentials from a well-known Secret via `kubectl get secret`.

**Pros**: No Go code changes, no schema extension.
**Cons**: Implicit dependency (hardcoded in script), no pre-flight validation, requires Secrets RBAC.
**Decision**: Rejected. Dependencies should be explicit and validated before execution.

### Scenario 3: EnvFrom Injection

Dependencies injected as environment variables via `EnvFrom` on the Job container.

**Pros**: Simple consumption in scripts (`echo $password`).
**Cons**: Doesn't work for binary data (TLS certs), naming collisions with parameters, less secure (env vars in `/proc`), doesn't support config files.
**Decision**: Rejected. Volume mount is more general and safer.

### Scenario 4: Volume Mount with On-Demand DS Query (Selected)

Dependencies declared in schema, validated by DS at registration and WFE at execution,
mounted as volumes (Job) or workspace bindings (Tekton). WFE queries DS on demand rather
than propagating through CRDs.

**Pros**: Explicit, validated at two points, secure (file-based), works for all data types
(credentials, certs, config files), no CRD propagation overhead, standard Kubernetes patterns.
**Cons**: Requires schema extension, DS needs k8s client, mount path convention needed.
**Decision**: Selected.

---

## Implementation

### Workflow Schema Extension

Add a `dependencies` field to the workflow schema (BR-WORKFLOW-004):

```yaml
metadata:
  workflowId: fix-certificate-gitops-v1
  version: "1.0.0"
  description:
    what: "Reverts a bad Git commit that broke a cert-manager ClusterIssuer"
    whenToUse: "When a GitOps-managed cert-manager Certificate is stuck NotReady"

actionType: GitRevertCommit

dependencies:
  secrets:
    - name: gitea-repo-creds
  configMaps: []

labels:
  signalName: CertManagerCertNotReady
  severity: [critical, high]
  environment: ["*"]
  component: "*"
  priority: "*"

execution:
  engine: job
  bundle: quay.io/kubernaut-cicd/test-workflows/fix-certificate-gitops-job:demo-v1.3

parameters:
  - name: GIT_REPO_URL
    type: string
    required: true
    description: "URL of the Git repository (without credentials)"
  - name: GIT_BRANCH
    type: string
    required: false
    description: "Branch to revert on"
    default: "main"
  - name: TARGET_NAMESPACE
    type: string
    required: true
    description: "Namespace of the affected Certificate"
  - name: TARGET_RESOURCE_NAME
    type: string
    required: true
    description: "Name of the affected Certificate (for verification)"
```

Note: `GIT_USERNAME` and `GIT_PASSWORD` are removed from parameters. The `gitea-repo-creds`
Secret provides them via volume mount.

### Go Types (Schema)

```go
type WorkflowDependencies struct {
    Secrets    []ResourceDependency `yaml:"secrets,omitempty" json:"secrets,omitempty"`
    ConfigMaps []ResourceDependency `yaml:"configMaps,omitempty" json:"configMaps,omitempty"`
}

type ResourceDependency struct {
    Name string `yaml:"name" json:"name" validate:"required"`
}
```

### Mount Path Convention

**Job executor**: Dependencies are mounted at well-known paths under `/run/kubernaut/`:

```
/run/kubernaut/
  secrets/
    gitea-repo-creds/
      username          # file containing "kubernaut"
      password          # file containing "kubernaut123"
  configmaps/
    remediation-config/
      threshold         # file containing "0.8"
```

**Tekton executor**: Dependencies are provided as workspace bindings. The Pipeline/Task
definition inside the OCI bundle controls the mount path (Tekton default:
`/workspace/<workspace-name>/`).

### Job Executor: Volume Mounting

The `buildJob` function adds `Volumes` and `VolumeMounts` for each declared dependency:

```go
const (
    SecretMountBasePath    = "/run/kubernaut/secrets"
    ConfigMapMountBasePath = "/run/kubernaut/configmaps"
)

func (j *JobExecutor) buildDependencyVolumes(deps *models.WorkflowDependencies) ([]corev1.Volume, []corev1.VolumeMount) {
    var volumes []corev1.Volume
    var mounts []corev1.VolumeMount

    if deps == nil {
        return volumes, mounts
    }

    for _, s := range deps.Secrets {
        volName := "secret-" + s.Name
        volumes = append(volumes, corev1.Volume{
            Name: volName,
            VolumeSource: corev1.VolumeSource{
                Secret: &corev1.SecretVolumeSource{SecretName: s.Name},
            },
        })
        mounts = append(mounts, corev1.VolumeMount{
            Name:      volName,
            MountPath: filepath.Join(SecretMountBasePath, s.Name),
            ReadOnly:  true,
        })
    }

    for _, cm := range deps.ConfigMaps {
        volName := "configmap-" + cm.Name
        volumes = append(volumes, corev1.Volume{
            Name: volName,
            VolumeSource: corev1.VolumeSource{
                ConfigMap: &corev1.ConfigMapVolumeSource{
                    LocalObjectReference: corev1.LocalObjectReference{Name: cm.Name},
                },
            },
        })
        mounts = append(mounts, corev1.VolumeMount{
            Name:      volName,
            MountPath: filepath.Join(ConfigMapMountBasePath, cm.Name),
            ReadOnly:  true,
        })
    }

    return volumes, mounts
}
```

### Tekton Executor: Workspace Bindings

The Tekton executor adds workspace bindings to the PipelineRunSpec. The Pipeline inside the
OCI bundle must declare matching workspace names.

**Workspace naming convention** (prefixed to avoid collisions):
- Secrets: `secret-<name>` (e.g., `secret-gitea-repo-creds`)
- ConfigMaps: `configmap-<name>` (e.g., `configmap-remediation-config`)

```go
func (t *TektonExecutor) buildDependencyWorkspaces(deps *models.WorkflowDependencies) []tektonv1.WorkspaceBinding {
    var workspaces []tektonv1.WorkspaceBinding

    if deps == nil {
        return workspaces
    }

    for _, s := range deps.Secrets {
        workspaces = append(workspaces, tektonv1.WorkspaceBinding{
            Name:   "secret-" + s.Name,
            Secret: &corev1.SecretVolumeSource{SecretName: s.Name},
        })
    }

    for _, cm := range deps.ConfigMaps {
        workspaces = append(workspaces, tektonv1.WorkspaceBinding{
            Name: "configmap-" + cm.Name,
            ConfigMap: &corev1.ConfigMapVolumeSource{
                LocalObjectReference: corev1.LocalObjectReference{Name: cm.Name},
            },
        })
    }

    return workspaces
}
```

**Pipeline author responsibility**: The Pipeline definition inside the OCI bundle must:
1. Declare workspaces matching the prefixed names
2. Mount them in Task steps at the desired path

Example Pipeline workspace declaration:
```yaml
workspaces:
  - name: secret-gitea-repo-creds
    description: "Git credentials for repository access"
```

Example Task step reading credentials:
```sh
GIT_USERNAME=$(cat $(workspaces.secret-gitea-repo-creds.path)/username)
GIT_PASSWORD=$(cat $(workspaces.secret-gitea-repo-creds.path)/password)
```

### Executor Interface: CreateOptions

The `Executor.Create()` method accepts a `CreateOptions` struct to pass dependencies
without breaking the interface for future extensions:

```go
type CreateOptions struct {
    Dependencies *models.WorkflowDependencies
}

type Executor interface {
    Create(ctx context.Context, wfe *v1alpha1.WorkflowExecution, namespace string, opts CreateOptions) (string, error)
    // ... existing methods unchanged
}
```

### Dual Validation

Dependencies are validated at two points for defense in depth:

**1. Data Storage at registration time** (Level 1 + Level 2):
- Level 1 (parser): Structural validation -- non-empty names, unique within category
- Level 2 (k8s client): Each Secret/ConfigMap exists in `kubernaut-workflows` with non-empty `.data`
- Registration fails with a clear error if any dependency is missing or empty

**2. WFE at execution time** (defense in depth):
- WFE queries DS for dependencies using `workflowRef.workflowId`
- WFE validates each Secret/ConfigMap exists in `kubernaut-workflows` with non-empty `.data`
- If validation fails, marks WFE as Failed with `FailureDetails.Reason: ConfigurationError` (no execution resource created)
- Catches post-registration changes: Secret deleted, emptied, or rotated incorrectly

### On-Demand DS Query (No CRD Propagation)

Dependencies are NOT propagated through the CRD chain (HAPI -> RO -> WFE). Rationale:

1. **Workflows are immutable** (DD-WORKFLOW-012): Once registered, the schema (including
   dependencies) cannot change. Only lifecycle state changes (enable/disable/deprecate).
2. **Single source of truth**: Dependencies live in the workflow catalog. Copying them into
   every WFE instance is redundant and adds CRD complexity.
3. **WFE has DS access**: The WFE controller already has a Data Storage client configured
   for audit events. Querying `GET /api/v1/workflows/{workflow_id}` is a lightweight call.

Data flow:
```
Registration:
  workflow-schema.yaml -> DS parser -> catalog DB (dependencies stored)

Execution:
  WFE spec (workflowRef.workflowId) -> DS GET /workflows/{id} -> dependencies
    -> validate in kubernaut-workflows -> mount in Job/PipelineRun
```

### Operational Ordering

Infrastructure (Secrets, ConfigMaps) MUST be provisioned in `kubernaut-workflows` BEFORE
workflow registration. Data Storage validates their existence at registration time. If an
operator attempts to register a workflow before creating its dependencies, registration fails
with a clear error message.

This is the intended behavior: deploy infrastructure first, then register workflows that
depend on it. It prevents "ticking time bomb" registrations where the workflow is in the
catalog but will fail at execution due to missing infrastructure.

### RBAC Requirements

**Data Storage ServiceAccount** (`data-storage-sa`):
- Needs a Kubernetes client (new capability -- DS currently only talks to PostgreSQL)
- Namespace-scoped Role in `kubernaut-workflows`: `get` on `secrets` and `configmaps`
- RoleBinding: `data-storage-sa` -> Role

**WFE Controller ServiceAccount** (`workflowexecution-controller`):
- Namespace-scoped Role in `kubernaut-workflows`: `get` on `secrets` and `configmaps`
- RoleBinding: `workflowexecution-controller` -> Role

Both roles are in the Helm chart templates for their respective services.

### Backward Compatibility

Workflows without `dependencies` in their schema continue to work unchanged. The
`Dependencies` field is optional (`omitempty`) in the schema types. When absent:
- Parser: no validation performed
- DS: nothing stored for dependencies
- WFE: no DS query for dependencies, no validation, no volumes/workspaces added
- Executors: existing behavior (parameters as env vars only)

---

## Affected Components

| Component | Team | Change |
|-----------|------|--------|
| Workflow schema types | DataStorage | Add `Dependencies` field to `WorkflowSchema` |
| Schema parser | DataStorage | Validate dependencies (structure + uniqueness) |
| DataStorage service | DataStorage | Add k8s client, validate dependencies at registration, store + expose |
| DataStorage Helm chart | DataStorage | Add Role + RoleBinding in `kubernaut-workflows` |
| OpenAPI schema | DataStorage | Add `dependencies` to `RemediationWorkflow` response |
| WFE reconciler | WorkflowExecution | Query DS for dependencies, validate, pass to executor |
| Job executor | WorkflowExecution | Add Volumes + VolumeMounts from dependencies |
| Tekton executor | WorkflowExecution | Add WorkspaceBindings from dependencies |
| Executor interface | WorkflowExecution | Add `CreateOptions` parameter to `Create()` |
| WFE Helm chart | WorkflowExecution | Add Role + RoleBinding in `kubernaut-workflows` |
| P17 workflow schema | Workflow Authors | Add `dependencies.secrets`, remove credential params |
| P17 remediate.sh | Workflow Authors | Read from `/run/kubernaut/secrets/gitea-repo-creds/` |
| P10 workflow schema | Workflow Authors | Add `dependencies.secrets` |
| P10 remediate.sh | Workflow Authors | Read from mounted secret instead of URL-embedded creds |
| Demo setup | Platform | Create `gitea-repo-creds` Secret in kubernaut-workflows |

---

## Consequences

### Positive

1. **Security**: Credentials never flow through the LLM or WFE parameters
2. **Explicit**: Dependencies are declared in the schema, visible during review
3. **Dual validation**: Missing/empty dependencies caught at registration AND execution
4. **Standard**: Volume mounts (Job) and workspace bindings (Tekton) -- native K8s patterns
5. **No CRD bloat**: Dependencies stay in the catalog, queried on demand
6. **General**: Volume mount works for credentials, TLS certs, config files, and binary data

### Negative

1. **DS needs k8s client**: New capability for a service that currently only talks to PostgreSQL
   - Mitigation: Scoped to `get` on `secrets`/`configmaps` in one namespace
2. **Operational ordering**: Secrets must exist before workflow registration
   - Mitigation: Clear error messages; documented requirement
3. **Tekton Pipeline coordination**: Pipeline authors must declare matching workspace names
   - Mitigation: Naming convention is predictable (`secret-<name>`, `configmap-<name>`)

### Neutral

1. Existing workflows: Backward compatible via optional `dependencies` field
2. Env var injection: Can be added later as an optional mode if demand emerges

---

## Related Documents

- [DD-WE-005: Workflow-Scoped RBAC](./DD-WE-005-workflow-scoped-rbac.md) -- Same philosophy: explicit schema declarations
- [DD-WE-002: Dedicated Execution Namespace](./DD-WE-002-dedicated-execution-namespace.md) -- Dependencies resolved in `kubernaut-workflows`
- [DD-WORKFLOW-003: Parameterized Remediation Actions](./DD-WORKFLOW-003-parameterized-actions.md) -- Parameters vs dependencies
- [DD-WORKFLOW-012: Workflow Immutability](./DD-WORKFLOW-012-workflow-immutability-constraints.md) -- Rationale for on-demand query
- [BR-WORKFLOW-004: Workflow Schema Format](../../requirements/BR-WORKFLOW-004-workflow-schema-format.md) -- Schema specification
- [BR-WE-014: Job Execution Backend](../../requirements/BR-WE-014-kubernetes-job-execution-backend.md) -- Acceptance criteria
- [Issue #233](https://github.com/jordigilh/kubernaut/issues/233) -- Implementation tracking

---

## Document Maintenance

| Date | Version | Changes |
|------|---------|---------|
| 2026-02-24 | 1.0 | Initial decision -- EnvFrom injection with CRD propagation |
| 2026-02-24 | 2.0 | Rewrite -- Volume mount, on-demand DS query, dual validation, no CRD propagation |
