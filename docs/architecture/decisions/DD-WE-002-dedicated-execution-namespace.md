# DD-WE-002: Dedicated Execution Namespace for PipelineRuns

**Version**: 1.0
**Date**: 2025-12-03
**Status**: ✅ APPROVED
**Author**: WorkflowExecution Team
**Reviewers**: Platform Team

---

## Context

WorkflowExecution creates Tekton PipelineRuns to execute remediation workflows. We needed to decide where these PipelineRuns should run:

1. **Target namespace** - Where the resource being remediated lives
2. **Dedicated namespace** - A single namespace for all workflow executions
3. **Hybrid** - Namespaced targets in target namespace, cluster-scoped in dedicated namespace

### Challenges

- **Cluster-scoped resources** (Nodes, PersistentVolumes) have no target namespace
- **Per-namespace ServiceAccounts** require setup in every namespace before remediation
- **Audit visibility** is fragmented across many namespaces
- **Cleanup** of completed PipelineRuns becomes complex

---

## Decision

**All PipelineRuns execute in a dedicated `kubernaut-workflows` namespace**, regardless of target resource scope.

### Execution Namespace Resolution

```
ANY targetResource (namespaced or cluster-scoped)
  → Run PipelineRun in "kubernaut-workflows" namespace
```

### Architecture

```
┌─────────────────────────────────────────────────────────┐
│  kubernaut-workflows namespace                          │
│  ┌───────────────────────────────────────────────────┐  │
│  │ ServiceAccount: kubernaut-workflow-runner         │  │
│  │ (ClusterRoleBinding → cross-namespace permissions)│  │
│  └───────────────────────────────────────────────────┘  │
│                                                         │
│  ┌─────────────────────────────────┐                    │
│  │ PipelineRun: node-disk-cleanup  │────────────────────┼──► Node: worker-1 (cluster-scoped)
│  └─────────────────────────────────┘                    │
│  ┌─────────────────────────────────┐                    │
│  │ PipelineRun: restart-deployment │────────────────────┼──► Deployment: payment-api (production ns)
│  └─────────────────────────────────┘                    │
│  ┌─────────────────────────────────┐                    │
│  │ PipelineRun: scale-statefulset  │────────────────────┼──► StatefulSet: redis (cache ns)
│  └─────────────────────────────────┘                    │
└─────────────────────────────────────────────────────────┘
```

---

## Alternatives Considered

### Option A: Target Namespace Execution

**Description**: Run PipelineRun in the same namespace as the target resource.

```go
// Extract namespace from targetResource
parts := strings.Split(wfe.Spec.TargetResource, "/")
if len(parts) == 3 {
    namespace = parts[0]  // e.g., "production"
}
```

**Pros**:
- Natural isolation per application
- Namespace-scoped RBAC possible

**Cons**:
- ❌ Cluster-scoped resources have no namespace
- ❌ Requires ServiceAccount in EVERY namespace
- ❌ Audit trail fragmented across namespaces
- ❌ Complex cleanup (find PipelineRuns in all namespaces)

**Decision**: Rejected

### Option B: Hybrid Approach

**Description**: Namespaced targets run in target namespace, cluster-scoped targets run in dedicated namespace.

**Pros**:
- More granular isolation for namespaced resources

**Cons**:
- ❌ Complex logic to determine execution namespace
- ❌ Still requires per-namespace ServiceAccounts for namespaced targets
- ❌ Inconsistent behavior
- ❌ Workflow authors must understand the difference

**Decision**: Rejected

### Option C: Dedicated Namespace (SELECTED)

**Description**: ALL PipelineRuns run in `kubernaut-workflows` namespace.

**Pros**:
- ✅ Simple, consistent behavior
- ✅ Single ServiceAccount with ClusterRoleBinding
- ✅ All remediation activity in one place (audit clarity)
- ✅ Easy PipelineRun cleanup (single namespace)
- ✅ Resource quotas applied centrally
- ✅ No pollution of application namespaces
- ✅ Matches industry patterns (Crossplane, AWX, Argo)

**Cons**:
- Requires ClusterRole (mitigated: audited, pre-defined permissions)
- Single point of monitoring (mitigated: dedicated namespace is easier to monitor)

**Decision**: Selected

---

## Industry Research

| Tool | Pattern | Execution Location |
|------|---------|-------------------|
| **Crossplane** | Provider pods | `crossplane-system` for all |
| **Ansible AWX/Tower** | Jobs | Single `awx` namespace |
| **Argo Workflows** | Cluster-scoped | Dedicated namespace |
| **Kubeflow Pipelines** | Multi-user | User-specific namespaces |
| **Tekton (typical)** | Namespace-scoped | Dedicated ops namespace |

**Consensus**: Run workflows in a dedicated operations namespace, not scattered across application namespaces.

---

## Implementation

### Required Kubernetes Resources

```yaml
# 1. Dedicated namespace
apiVersion: v1
kind: Namespace
metadata:
  name: kubernaut-workflows

---
# 2. ServiceAccount
apiVersion: v1
kind: ServiceAccount
metadata:
  name: kubernaut-workflow-runner
  namespace: kubernaut-workflows

---
# 3. ClusterRole with cross-namespace permissions
apiVersion: rbac.authorization.k8s.io/v1
kind: ClusterRole
metadata:
  name: kubernaut-workflow-runner
rules:
  # Workload remediation (all namespaces)
  - apiGroups: ["apps"]
    resources: ["deployments", "statefulsets", "daemonsets"]
    verbs: ["get", "list", "patch", "update"]
  - apiGroups: [""]
    resources: ["pods"]
    verbs: ["get", "list", "delete"]
  # Node operations (cluster-scoped)
  - apiGroups: [""]
    resources: ["nodes"]
    verbs: ["get", "list", "patch"]
  # ConfigMaps/Secrets for workflow data
  - apiGroups: [""]
    resources: ["configmaps", "secrets"]
    verbs: ["get", "list"]

---
# 4. ClusterRoleBinding
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

### Controller Configuration

```yaml
# workflowexecution-config ConfigMap
execution:
  namespace: "kubernaut-workflows"  # All PipelineRuns run here
  serviceAccount: "kubernaut-workflow-runner"
```

### Controller Code

```go
func (r *WorkflowExecutionReconciler) buildPipelineRun(
    wfe *workflowexecutionv1.WorkflowExecution,
) *tektonv1.PipelineRun {
    return &tektonv1.PipelineRun{
        ObjectMeta: metav1.ObjectMeta{
            Name:      wfe.Name,
            Namespace: r.ExecutionNamespace,  // Always "kubernaut-workflows"
            Labels: map[string]string{
                "kubernaut.ai/workflow-execution": wfe.Name,
                "kubernaut.ai/target-resource":    wfe.Spec.TargetResource,
            },
            // Note: OwnerReference crosses namespaces - use finalizer cleanup instead
        },
        Spec: tektonv1.PipelineRunSpec{
            PipelineRef: &tektonv1.PipelineRef{
                ResolverRef: tektonv1.ResolverRef{
                    Resolver: "bundles",
                    Params: []tektonv1.Param{
                        {Name: "bundle", Value: tektonv1.ParamValue{StringVal: wfe.Spec.WorkflowRef.ContainerImage}},
                        {Name: "name", Value: tektonv1.ParamValue{StringVal: "workflow"}},
                    },
                },
            },
            Params:             r.convertParameters(wfe.Spec.Parameters),
            ServiceAccountName: r.ServiceAccountName,  // "kubernaut-workflow-runner"
        },
    }
}
```

### Cross-Namespace Owner Reference Note

Since WorkflowExecution CRD may be in `kubernaut-system` but PipelineRun is in `kubernaut-workflows`, Kubernetes owner references don't work cross-namespace. Instead:

1. **Label linking**: PipelineRun has label `kubernaut.ai/workflow-execution: <wfe-name>` (Issue #91: still valid for external K8s resources like PipelineRun/Job)
2. **Finalizer cleanup**: WorkflowExecution finalizer deletes associated PipelineRun
3. **Watch by label**: Controller watches PipelineRuns by label selector

---

## Consequences

### Positive

1. **Simplified setup**: Single namespace + ServiceAccount for all workflows
2. **Audit clarity**: All remediation activity in one place
3. **Easy cleanup**: TTL controller or manual cleanup in single namespace
4. **Resource management**: Quotas and limits in one namespace
5. **Security review**: Single ClusterRole to audit

### Negative

1. **Cross-namespace permissions**: ServiceAccount has cluster-wide access
   - **Mitigation**: Pre-defined, audited ClusterRole with least-privilege
2. **No OwnerReference**: Can't use native Kubernetes garbage collection
   - **Mitigation**: Finalizer-based cleanup in controller

### Neutral

1. **Monitoring**: All PipelineRuns in one namespace (easier, not harder)
2. **Scaling**: No impact - Tekton handles scheduling across nodes

---

## Related Documents

- [DD-WE-001: Resource Locking Safety](./DD-WE-001-resource-locking-safety.md)
- [ADR-044: Workflow Execution Engine Delegation](./ADR-044-workflow-execution-engine-delegation.md)
- [CONFIG_STANDARDS.md](../../configuration/CONFIG_STANDARDS.md)
- [WE README Prerequisites](../../services/crd-controllers/03-workflowexecution/README.md#-prerequisites)

---

## Document Maintenance

| Date | Version | Changes |
|------|---------|---------|
| 2025-12-03 | 1.0 | Initial decision - dedicated namespace pattern |

