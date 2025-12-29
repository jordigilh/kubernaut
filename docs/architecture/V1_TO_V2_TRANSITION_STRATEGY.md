# Kubernaut V1 → V2 Transition Strategy

**Version**: 1.0
**Date**: 2025-10-19
**Status**: ✅ Approved

---

## Executive Summary

Kubernaut's execution architecture follows a **strategic two-phase approach**:
- **V1 (Q4 2025)**: Production-ready with **native Kubernetes Jobs** (zero dependencies)
- **V2 (Q3 2026)**: Enterprise-optimized with **Tekton Pipelines** (industrial standard)

**Key Insight**: **Container portability** enables seamless migration. Same Cosign-signed action containers work in both V1 and V2! 🎯

---

## Strategic Drivers

### **Why V1 with Native Jobs?**

1. **Production Readiness (Q4 2025)**
   - ✅ Zero external dependencies
   - ✅ Native Kubernetes primitives only
   - ✅ Fast time-to-market
   - ✅ Minimal operational complexity

2. **Customer Validation**
   - ✅ Prove remediation value before adding complexity
   - ✅ Real-world workflow patterns emerge
   - ✅ Performance baselines established

### **Why V2 with Tekton?**

1. **Industrial Acceptance**
   - ✅ CNCF Graduated Project
   - ✅ upstream community Tekton Pipelines (enterprise support)
   - ✅ Familiar to CI/CD teams
   - ✅ Reduces deployment friction

2. **Operational Excellence**
   - ✅ Battle-tested DAG orchestration
   - ✅ Built-in retry and timeout
   - ✅ Rich debugging tools
   - ✅ Active community ecosystem

3. **Code Simplification**
   - ✅ Eliminates 500+ lines of custom orchestration
   - ✅ Delegates complexity to Tekton
   - ✅ Focus on business logic

---

## V1 Architecture: Native Kubernetes Jobs

### **Core Components**

```
┌─────────────────────────────────────────────────────────┐
│ WorkflowExecution Controller (V1)                       │
│ - Parses workflow steps                                  │
│ - Builds dependency graph (DAG)                          │
│ - Creates ActionExecution CRDs sequentially              │
│ - Manages shared PVC workspace (multi-step workflows)    │
└─────────────────────────────────────────────────────────┘
                          ↓
┌─────────────────────────────────────────────────────────┐
│ ActionExecution Controller                               │
│ - Verifies Cosign image signatures                       │
│ - Loads container contract from image                    │
│ - Creates Kubernetes Job                                 │
│ - Monitors Job status                                    │
└─────────────────────────────────────────────────────────┘
                          ↓
┌─────────────────────────────────────────────────────────┐
│ Kubernetes Job                                           │
│ Image: ghcr.io/kubernaut/actions/scale@sha256:abc123    │
│ Contract: /action-contract.yaml                          │
│ Security: Cosign-verified, least privilege RBAC          │
└─────────────────────────────────────────────────────────┘
```

### **V1 Workflow Types**

#### **Type 1: Single Action (90% of workflows)**
```yaml
apiVersion: kubernaut.ai/v1alpha1
kind: WorkflowExecution
spec:
  steps:
    - name: scale-deployment
      actionType: kubernetes/scale_deployment
      image: ghcr.io/kubernaut/actions/scale@sha256:abc123
      inputs:
        deployment: payment-service
        replicas: 10
```

**Execution**:
1. WorkflowExecution creates 1 ActionExecution CRD
2. ActionExecution verifies Cosign signature
3. ActionExecution creates Kubernetes Job
4. Job scales deployment
5. Output captured in ActionExecution status

**Duration**: 5-10 seconds

---

#### **Type 2: Multi-Step with Dependencies (10% of workflows)**
```yaml
apiVersion: kubernaut.ai/v1alpha1
kind: WorkflowExecution
spec:
  steps:
    # Step 1, 2: Parallel emergency actions
    - name: restart-pods
      actionType: kubernetes/restart_pod
      image: ghcr.io/kubernaut/actions/restart@sha256:def456
      runAfter: []

    - name: scale-deployment
      actionType: kubernetes/scale_deployment
      image: ghcr.io/kubernaut/actions/scale@sha256:abc123
      runAfter: []

    # Step 3: Sequential GitOps PR (depends on 1, 2)
    - name: create-gitops-pr
      actionType: git/create-pr
      image: ghcr.io/kubernaut/actions/git-pr@sha256:ghi789
      usesWorkspace: true
      runAfter:
        - restart-pods
        - scale-deployment
```

**Execution**:
1. WorkflowExecution creates shared PVC workspace
2. WorkflowExecution creates ActionExecution for `restart-pods` and `scale-deployment` (parallel)
3. WorkflowExecution waits for both to complete
4. WorkflowExecution creates ActionExecution for `create-gitops-pr`
5. GitOps PR action mounts PVC workspace
6. WorkflowExecution deletes PVC after completion

**Duration**: 20-30 seconds

---

### **V1 GitOps Workflow: 4-Step PR Creation**

**Use Case**: Create GitHub PR with evidence-based remediation

```yaml
spec:
  steps:
    - name: git-clone
      actionType: git/clone
      image: ghcr.io/kubernaut/actions/git-clone@sha256:...
      usesWorkspace: true
      inputs:
        repository: https://github.com/company/k8s-configs
        destination: /workspace
      runAfter: []

    - name: modify-deployment
      actionType: git/modify-file
      image: ghcr.io/kubernaut/actions/yaml-patch@sha256:...
      usesWorkspace: true
      inputs:
        file: /workspace/production/payment-service.yaml
        patch: |
          spec.resources.limits.memory: 2Gi
      runAfter:
        - git-clone

    - name: git-commit
      actionType: git/commit
      image: ghcr.io/kubernaut/actions/git-commit@sha256:...
      usesWorkspace: true
      inputs:
        message: "fix(payment): Increase memory to 2Gi (AI-recommended)"
      runAfter:
        - modify-deployment

    - name: git-push-pr
      actionType: git/create-pr
      image: ghcr.io/kubernaut/actions/git-push@sha256:...
      usesWorkspace: true
      inputs:
        branch: kubernaut/payment-memory-fix
        title: "Kubernaut: Fix OOMKilled (87% confidence)"
      runAfter:
        - git-commit
```

**V1 Execution**:
1. WorkflowExecution creates PVC `workflow-xyz-workspace` (1Gi)
2. ActionExecution for `git-clone` → Job mounts PVC, clones repo
3. ActionExecution for `modify-deployment` → Job mounts PVC, patches YAML
4. ActionExecution for `git-commit` → Job mounts PVC, commits changes
5. ActionExecution for `git-push-pr` → Job mounts PVC, pushes and creates PR
6. WorkflowExecution deletes PVC

**Duration**: 30-45 seconds

---

## V2 Architecture: Tekton Pipelines

### **Core Components**

```
┌─────────────────────────────────────────────────────────┐
│ WorkflowExecution Controller (V2)                       │
│ - Translates WorkflowExecution to PipelineRun           │
│ - Creates Tekton PipelineRun                            │
│ - Monitors PipelineRun status                           │
│ - Syncs status back to WorkflowExecution                │
└─────────────────────────────────────────────────────────┘
                          ↓
┌─────────────────────────────────────────────────────────┐
│ Tekton PipelineRun                                       │
│ - Built-in DAG orchestration                             │
│ - Parallel execution (runAfter)                          │
│ - Workspace management                                   │
│ - Retry and timeout                                      │
└─────────────────────────────────────────────────────────┘
                          ↓
┌─────────────────────────────────────────────────────────┐
│ Tekton TaskRun (Generic Meta-Task)                      │
│ - Executes Kubernaut action container                   │
│ - Verifies Cosign signature (via admission)             │
│ - Loads contract from container                          │
│ - Captures outputs                                       │
└─────────────────────────────────────────────────────────┘
                          ↓
┌─────────────────────────────────────────────────────────┐
│ Pod (Created by TaskRun)                                 │
│ Image: ghcr.io/kubernaut/actions/scale@sha256:abc123    │
│ Contract: /action-contract.yaml                          │
│ Security: Cosign-verified, least privilege RBAC          │
└─────────────────────────────────────────────────────────┘
```

### **V2 Generic Meta-Task**

**Single Tekton Task definition executes ANY action container**:

```yaml
apiVersion: tekton.dev/v1
kind: Task
metadata:
  name: kubernaut-action
spec:
  params:
    - name: actionType
      type: string
    - name: actionImage
      type: string
    - name: inputs
      type: string

  workspaces:
    - name: workspace
      optional: true

  steps:
    - name: execute
      image: $(params.actionImage)  # Same container as V1!
      env:
        - name: ACTION_INPUTS
          value: $(params.inputs)
      script: |
        echo "$ACTION_INPUTS" | /action-entrypoint

  results:
    - name: outputs
      description: "JSON outputs"
```

**Key**: **Same action containers** work in V1 (Jobs) and V2 (Tekton)! 🎯

---

### **V2 GitOps Workflow: Same Steps, Tekton Orchestration**

```yaml
# WorkflowExecution (same as V1!)
apiVersion: kubernaut.ai/v1alpha1
kind: WorkflowExecution
spec:
  steps:
    - name: git-clone
      actionType: git/clone
      image: ghcr.io/kubernaut/actions/git-clone@sha256:...
      usesWorkspace: true
      inputs:
        repository: https://github.com/company/k8s-configs
      runAfter: []

    - name: modify-deployment
      actionType: git/modify-file
      image: ghcr.io/kubernaut/actions/yaml-patch@sha256:...
      usesWorkspace: true
      runAfter:
        - git-clone

    # ... same steps as V1 ...
```

**V2 Execution** (Tekton handles orchestration):
1. WorkflowExecution controller creates PipelineRun
2. Tekton creates workspace PVC automatically
3. Tekton creates TaskRun for each step
4. Tekton manages dependencies (runAfter)
5. TaskRuns execute same action containers as V1
6. Tekton handles retry and timeout
7. Tekton deletes PVC automatically (ttlAfterFinished)

**Duration**: 25-35 seconds (Tekton optimization)

---

## Container Contract: Portability Layer

### **Why Same Containers Work in V1 and V2**

**Action Container Structure**:
```
ghcr.io/kubernaut/actions/git-clone@sha256:def456
│
├── /action-contract.yaml      ← Contract definition
├── /action-entrypoint          ← Executable
├── /usr/bin/git                ← Action tooling
└── ... (other files)

Cosign Signature: ✅ Verified
```

**Contract Example**:
```yaml
# /action-contract.yaml
apiVersion: kubernaut.ai/v1alpha1
kind: ActionContract
metadata:
  name: git-clone
  version: v1.0.0

spec:
  inputs:
    - name: repository
      type: string
      required: true
    - name: branch
      type: string
      default: "main"

  outputs:
    - name: commitSHA
      type: string

  resources:
    requests:
      cpu: "100m"
      memory: "128Mi"
```

**Entrypoint (Dual Input Mode)**:
```bash
#!/bin/sh
# /action-entrypoint

# V1: Inputs from environment variable (Kubernetes Job)
if [ -n "$ACTION_INPUTS" ]; then
    INPUTS="$ACTION_INPUTS"
# V2: Inputs from stdin (Tekton Task)
elif [ ! -t 0 ]; then
    INPUTS=$(cat)
else
    echo "ERROR: No inputs" >&2
    exit 1
fi

# Parse and execute
REPOSITORY=$(echo "$INPUTS" | jq -r '.repository')
git clone "$REPOSITORY" /workspace

# Output JSON results
echo '{"commitSHA":"abc123"}'
```

**Key Features**:
- ✅ **Dual input mode**: Env vars (V1) or stdin (V2)
- ✅ **JSON outputs**: Captured by both Job logs and Tekton results
- ✅ **Same Cosign signature**: Verified identically in V1 and V2
- ✅ **No code changes**: Container runs identically

---

## Migration Timeline

### **Phase 1: V1 Production (Q4 2025)**

**Goals**:
- ✅ Deploy Kubernaut with native Jobs
- ✅ Build 29+ action containers with Cosign signing
- ✅ Validate workflows in production
- ✅ Establish performance baselines

**Deliverables**:
- WorkflowExecution controller (native Jobs)
- ActionExecution controller (Cosign verification)
- Action container registry (29+ actions)
- Production runbooks and monitoring

**Success Metrics**:
- 93% average workflow success rate
- 5 min average MTTR (2-8 min by scenario)
- 100% Cosign signature verification

---

### **Phase 2: Tekton Preparation (Q1 2026)**

**Goals**:
- ✅ Install Tekton Pipelines in test clusters
- ✅ Validate action containers work with Tekton meta-task
- ✅ Build dual reconciler (Jobs + Tekton)
- ✅ Performance comparison (V1 vs V2)

**Deliverables**:
- Tekton meta-task definition
- V2 WorkflowExecution reconciler (Tekton translation)
- Feature flag for V1/V2 routing
- A/B testing framework

**Validation**:
- Same action containers run in Tekton
- Cosign verification works via admission controller
- Performance parity or improvement

---

### **Phase 3: Gradual Rollout (Q2 2026)**

**Week 1-2**: GitOps workflows only (2% of traffic)
```go
func (r *WorkflowExecutionReconciler) shouldUseTekton(workflow *WorkflowExecution) bool {
    // Enable Tekton for GitOps workflows
    if workflow.Spec.WorkflowType == "gitops-pr-creation" {
        return true
    }
    return false  // All others use V1 Jobs
}
```

**Week 3-4**: Multi-step workflows (10% of traffic)
```go
func (r *WorkflowExecutionReconciler) shouldUseTekton(workflow *WorkflowExecution) bool {
    // Enable Tekton for multi-step workflows
    if len(workflow.Spec.Steps) > 3 || r.hasDependencies(workflow) {
        return true
    }
    return false
}
```

**Week 5-6**: Monitor and compare
- Prometheus metrics: V1 vs V2 success rate
- MTTR comparison
- Error rate analysis
- Customer feedback

**Week 7-8**: Full rollout (100% of traffic)
```go
func (r *WorkflowExecutionReconciler) shouldUseTekton(workflow *WorkflowExecution) bool {
    // All workflows use Tekton
    return true
}
```

**Rollback Strategy**:
- Feature flag revert: 30 seconds
- V1 reconciler kept for emergency rollback
- Prometheus alerts for anomalies

---

### **Phase 4: V2 Stabilization (Q3 2026)**

**Goals**:
- ✅ Remove V1 reconciler (keep for reference)
- ✅ Document Tekton best practices
- ✅ Train SRE teams on Tekton debugging
- ✅ Community contributions (Tekton Hub)

**Deliverables**:
- V2 production runbooks
- Tekton debugging guide
- SRE training materials
- Tekton Hub submissions (action catalog)

**Success Metrics**:
- 95%+ workflow success rate
- 4 min average MTTR (Tekton optimization)
- 90%+ SRE team Tekton familiarity
- 50% reduction in controller code complexity

---

## V1 vs V2 Comparison

| Aspect | V1 (Native Jobs) | V2 (Tekton Pipelines) |
|--------|------------------|----------------------|
| **Dependencies** | Zero (Kubernetes only) | Tekton Pipelines (~450MB) |
| **Orchestration** | Custom (500+ lines) | Tekton (100 lines) |
| **DAG Resolution** | Custom topological sort | Tekton built-in |
| **Retry Logic** | Custom exponential backoff | Tekton built-in |
| **Workspace Management** | Manual PVC creation | Tekton workspaces |
| **Debugging** | kubectl logs + events | Tekton CLI + dashboard |
| **Learning Curve** | Low (Kubernetes primitives) | Medium (Tekton concepts) |
| **Industrial Acceptance** | Custom solution | CNCF Graduated Project |
| **Performance** | 5-10s (single action) | 6-11s (+1s overhead) |
| **Performance** | 20-30s (multi-step) | 15-25s (Tekton optimization) |
| **Action Containers** | ✅ Same containers | ✅ Same containers |
| **Cosign Verification** | ✅ Admission controller | ✅ Admission controller |
| **Production Readiness** | Q4 2025 | Q3 2026 |

---

## Key Insights

### **1. Container Portability is the Migration Secret** 🎯

**Same action containers work in V1 and V2** because:
- ✅ **Dual input mode**: Entrypoint reads env vars (V1) or stdin (V2)
- ✅ **JSON outputs**: Captured identically by Job logs and Tekton results
- ✅ **Cosign verification**: Admission controller validates in both
- ✅ **Contract-based**: `/action-contract.yaml` defines behavior

**Result**: Zero container rewrites during migration! 🚀

---

### **2. V1 Proves Value, V2 Scales Adoption**

**V1 Strategy**: Ship fast, prove remediation value
- Zero dependencies (Kubernetes only)
- Fast time-to-market (Q4 2025)
- Real-world validation

**V2 Strategy**: Leverage industrial standards
- Tekton is familiar to CI/CD teams
- Reduces deployment friction
- Enterprise support (upstream community Tekton Pipelines)

**Result**: Customer choice based on maturity! 🎯

---

### **3. Feature Flag Enables Safe Migration**

**Gradual Rollout**:
```go
// Week 1-2: GitOps workflows only (2%)
if workflow.Spec.WorkflowType == "gitops-pr-creation" {
    return r.TektonReconciler.Reconcile(ctx, req)
}

// Week 3-4: Multi-step workflows (10%)
if len(workflow.Spec.Steps) > 3 {
    return r.TektonReconciler.Reconcile(ctx, req)
}

// Week 7-8: All workflows (100%)
return r.TektonReconciler.Reconcile(ctx, req)
```

**Rollback**: 30 seconds (feature flag revert)

---

## FAQ

### **Q: Why not use Tekton from V1?**
**A**: External dependency adds risk before product validation. V1 proves value first.

### **Q: Will V1 be deprecated after V2?**
**A**: No. V1 remains supported for customers preferring zero dependencies.

### **Q: Do action containers need rewrites for V2?**
**A**: No! Same containers work in V1 and V2 (dual input mode).

### **Q: What if Tekton has breaking changes?**
**A**: Pin Tekton version, test upgrades in staging, rollback to V1 if needed.

### **Q: Can customers mix V1 and V2 workflows?**
**A**: Yes! Feature flag routes workflows to V1 (Jobs) or V2 (Tekton) based on type.

### **Q: How long does migration take?**
**A**: 8 weeks (gradual rollout), zero downtime.

---

## Related Documentation

- **[ADR-002: Native Kubernetes Jobs](decisions/ADR-002-native-kubernetes-jobs.md)** - V1 execution foundation
- **[ADR-022: V1 Native Jobs with V2 Tekton Migration](decisions/ADR-022-v1-native-jobs-v2-tekton-migration.md)** - Complete migration plan
- **[Container Action Registry](../services/action-execution/ACTION_REGISTRY.md)** - Action container catalog
- **[Cosign Verification Guide](../security/COSIGN_VERIFICATION.md)** - Image signing and verification

---

## Conclusion

Kubernaut's **V1 → V2 transition strategy** balances production readiness (V1) with industrial acceptance (V2):

- **V1 (Q4 2025)**: Ship fast with native Jobs, prove value
- **V2 (Q3 2026)**: Scale adoption with Tekton, reduce friction
- **Migration**: Zero downtime, container portability, gradual rollout

**Key Insight**: **Same Cosign-signed action containers work in V1 and V2**, enabling seamless migration! 🎯

---

**Status**: ✅ Approved
**Next Review**: After V1 production deployment (Q1 2026)
**Owner**: Architecture Team

