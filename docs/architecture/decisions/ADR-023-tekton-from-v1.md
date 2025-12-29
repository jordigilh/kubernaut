# ADR-023: Tekton Pipelines from V1 (Eliminate Custom Orchestration)

**Status**: ✅ Approved
**Date**: 2025-10-19
**Updated**: 2025-10-19 (Eliminate ActionExecution - see [ADR-024](ADR-024-eliminate-actionexecution-layer.md))
**Deciders**: Architecture Team
**Priority**: FOUNDATIONAL
**Supersedes**: [ADR-022: V1 Native Jobs with V2 Tekton Migration](ADR-022-v1-native-jobs-v2-tekton-migration.md)

---

## Context and Problem Statement

Kubernaut requires workflow orchestration for multi-step remediation workflows. The original plan was to build custom orchestration for V1 and migrate to Tekton in V2. However, this approach creates **500+ lines of throwaway code** and requires a complex migration.

**Key Questions**:
1. Should we build custom orchestration for V1 that will be deleted in V2?
2. Is Tekton universally available enough to use from V1?
3. What is the true cost of custom orchestration vs Tekton adoption?

**Strategic Context**:
- **Target Customer**: Red Hat (OpenShift Pipelines = Tekton)
- **Universal Availability**: OpenShift Pipelines (bundled) + Upstream Tekton (open source)
- **Industrial Acceptance**: CNCF Graduated Project
- **Development Timeline**: Q4 2025 production target

---

## Decision Drivers

### **1. Eliminate Throwaway Code** 🎯
- Custom orchestration: 500+ lines of code
- Timeline: 10 weeks development
- Lifecycle: **Deleted in V2** (9 months later)
- Result: **Architectural waste**

### **2. Red Hat Alignment** 🔴
- Red Hat ships **OpenShift Pipelines** (Tekton) as standard component
- OpenShift 4.x: Tekton pre-installed or one-click install
- Maximum industrial acceptance for target customer

### **3. Universal Availability** 🌍
- **OpenShift customers**: OpenShift Pipelines (bundled)
- **Non-OpenShift customers**: Upstream Tekton (open source, `kubectl apply`)
- **Installation effort**: 0 minutes (OpenShift) or 5 minutes (upstream)

### **4. Time-to-Market** ⏱️
- Custom orchestration: 10 weeks (write) + 6 weeks (V2 Tekton) = **16 weeks total**
- Tekton from V1: **8 weeks total** (no migration)
- Result: **50% faster to final architecture**

### **5. CNCF Graduated Project** ✅
- Same trust level as Kubernetes
- Active community (Google, IBM, Red Hat)
- Battle-tested at scale
- Long-term support guaranteed

---

## Considered Options

### **Option 1: Custom Orchestration (V1) → Tekton Migration (V2)** ❌

**Architecture**:
```
V1 (Q4 2025): Custom orchestration (600 lines)
    ├── DAG resolution (150 lines)
    ├── Parallel execution (120 lines)
    ├── PVC management (70 lines)
    ├── Retry logic (80 lines)
    └── Dependency monitoring (80 lines)

    ↓ (9 months later)

V2 (Q3 2026): Tekton Pipelines (100 lines)
    └── DELETE 500 lines of V1 code ❌
```

**Pros**:
- ✅ V1 has no external dependencies
- ✅ Full control over orchestration logic

**Cons**:
- ❌ 500+ lines of throwaway code
- ❌ 16 weeks total development time
- ❌ Complex V1 → V2 migration
- ❌ Lower Red Hat alignment (custom solution)

**Why Rejected**: **Architectural waste** (500 lines deleted after 9 months)

---

### **Option 2: Tekton from V1** ⭐ **CHOSEN**

**Architecture**:
```
V1 (Q4 2025): Tekton Pipelines (100 lines)
    └── PipelineRun translation

    ↓

V2-V10: Same architecture (no migration needed!) ✅
```

**Pros**:
- ✅ **Zero throwaway code** (100 lines permanent)
- ✅ **8 weeks development** (vs 16 weeks)
- ✅ **No V1 → V2 migration** (single architecture)
- ✅ **Maximum Red Hat alignment** (OpenShift Pipelines)
- ✅ **Universal availability** (bundled + upstream)
- ✅ **CNCF Graduated** (same trust as Kubernetes)
- ✅ **Battle-tested orchestration** (eliminates 500 lines of custom logic)

**Cons**:
- ⚠️ Learning curve for non-OpenShift teams (medium impact)
- ⚠️ Resource footprint: ~450MB (low impact - acceptable trade-off)
- ⚠️ External dependency (mitigated by CNCF Graduated status)

**Why Chosen**: **Eliminates architectural waste**, maximizes Red Hat alignment, faster to final architecture

---

### **Option 3: Argo Workflows** ❌

**Description**: Use Argo Workflows instead of Tekton

**Pros**:
- ✅ CNCF Graduated (same as Tekton)
- ✅ Rich UI and workflow visualization

**Cons**:
- ❌ **No Red Hat backing** (Tekton = OpenShift Pipelines)
- ❌ Less familiar to OpenShift teams
- ❌ Not bundled with OpenShift

**Why Rejected**: Lower Red Hat alignment, Tekton is standard in OpenShift

---

## Decision Outcome

**Chosen option**: **"Option 2: Tekton from V1"**

**Rationale**:
1. **Eliminates 500+ lines of throwaway code** (architectural efficiency)
2. **Maximum Red Hat alignment** (OpenShift Pipelines = Tekton)
3. **Universal availability** (bundled + upstream)
4. **Faster to final architecture** (8 weeks vs 16 weeks)
5. **No migration complexity** (V1 = final architecture)
6. **CNCF Graduated** (same trust level as Kubernetes)

---

## Architecture Overview

### **Core Components**

```
┌─────────────────────────────────────────────────────────┐
│ WorkflowExecution Controller (100 lines)                │
│ - Translates WorkflowExecution → Tekton PipelineRun     │
│ - Monitors PipelineRun status                           │
│ - Syncs status to WorkflowExecution CRD                 │
│ - Writes action records to Data Storage Service         │
└─────────────────────────────────────────────────────────┘
                          ↓
┌─────────────────────────────────────────────────────────┐
│ Tekton Pipelines                                         │
│ Source: OpenShift Pipelines (Red Hat)                   │
│         OR Upstream Tekton (vanilla K8s)                │
│                                                          │
│ - DAG orchestration                                      │
│ - Parallel execution (runAfter)                          │
│ - Workspace management                                   │
│ - Retry and timeout                                      │
└─────────────────────────────────────────────────────────┘
                          ↓
┌─────────────────────────────────────────────────────────┐
│ Tekton TaskRun (Generic Meta-Task)                      │
│ Task: kubernaut-action                                   │
│ - Executes ANY action container                          │
│ - Verifies Cosign signatures (via admission)            │
│ - Captures outputs                                       │
└─────────────────────────────────────────────────────────┘
                          ↓
┌─────────────────────────────────────────────────────────┐
│ Pod (Action Container)                                   │
│ Image: ghcr.io/kubernaut/actions/{k8s|gitops|aws}@sha256 │
│ Contract: /action-contract.yaml                          │
│ Security: Cosign-verified                                │
└─────────────────────────────────────────────────────────┘
                          ↓
┌─────────────────────────────────────────────────────────┐
│ Data Storage Service                                     │
│ - Stores action history (90+ days)                      │
│ - Stores effectiveness metrics                          │
│ - Queried by Pattern Monitoring                         │
│ - Queried by Effectiveness Monitor                      │
└─────────────────────────────────────────────────────────┘
```

**Key Design Decision**: **Direct Tekton integration** with no ActionExecution layer. Business data lives in RemediationRequest, WorkflowExecution, and Data Storage Service. See **[ADR-024: Eliminate ActionExecution Layer](ADR-024-eliminate-actionexecution-layer.md)** for detailed rationale.

---

## Generic Meta-Task Pattern

### **Single Tekton Task for All Actions**

```yaml
apiVersion: tekton.dev/v1
kind: Task
metadata:
  name: kubernaut-action
  namespace: kubernaut-system
spec:
  description: |
    Generic Tekton Task that executes any Kubernaut action container.
    Container contract defines action behavior (self-documenting).

  params:
    - name: actionType
      type: string
      description: "Action type (e.g., kubernetes/scale_deployment)"

    - name: actionImage
      type: string
      description: "Cosign-signed action container image with @sha256 digest"

    - name: inputs
      type: string
      description: "JSON-encoded action inputs"

  workspaces:
    - name: workspace
      description: "Shared workspace for multi-step workflows"
      optional: true

  steps:
    # Cosign verification happens at admission time via Sigstore Policy Controller

    - name: execute
      image: $(params.actionImage)
      env:
        - name: ACTION_TYPE
          value: $(params.actionType)
        - name: ACTION_INPUTS
          value: $(params.inputs)
        - name: WORKSPACE_PATH
          value: $(workspaces.workspace.path)
      script: |
        #!/bin/sh
        set -e

        # Action containers read inputs from env vars
        echo "$ACTION_INPUTS" | /action-entrypoint

        # Outputs written to stdout (captured by Tekton)

  results:
    - name: outputs
      description: "JSON outputs from action container"
```

**Benefits**:
- ✅ **1 Task definition** (not 29+)
- ✅ **Container contract** defines behavior (self-documenting)
- ✅ **Extensible** (add actions without new Tasks)
- ✅ **Action registry** in ConfigMap (easier updates)

---

## Example Workflow Execution

### **WorkflowExecution CRD** (User-Defined)

```yaml
apiVersion: kubernaut.ai/v1alpha1
kind: WorkflowExecution
metadata:
  name: remediate-payment-oom
  namespace: kubernaut-system
spec:
  workflowType: multi-step-remediation

  steps:
    # Step 1, 2: Parallel emergency actions
    - name: restart-pods
      actionType: kubernetes/restart_pod
      image: ghcr.io/kubernaut/actions/restart@sha256:def456
      inputs:
        namespace: production
        labelSelector: app=payment
      runAfter: []

    - name: scale-deployment
      actionType: kubernetes/scale_deployment
      image: ghcr.io/kubernaut/actions/scale@sha256:abc123
      inputs:
        deployment: payment-service
        namespace: production
        replicas: 10
      runAfter: []

    # Step 3: Sequential GitOps PR (depends on 1, 2)
    - name: create-gitops-pr
      actionType: git/create-pr
      image: ghcr.io/kubernaut/actions/git-pr@sha256:ghi789
      usesWorkspace: true
      inputs:
        repository: https://github.com/company/k8s-configs
        branch: kubernaut/payment-memory-fix
        title: "Fix payment-service OOMKilled"
      runAfter:
        - restart-pods
        - scale-deployment
```

---

### **Tekton PipelineRun** (Generated by WorkflowExecution Controller)

```yaml
apiVersion: tekton.dev/v1
kind: PipelineRun
metadata:
  name: remediate-payment-oom
  namespace: kubernaut-system
  labels:
    kubernaut.io/workflow: remediate-payment-oom
  ownerReferences:
    - apiVersion: kubernaut.ai/v1alpha1
      kind: WorkflowExecution
      name: remediate-payment-oom
spec:
  pipelineSpec:
    tasks:
      # Parallel tasks
      - name: restart-pods
        taskRef:
          name: kubernaut-action
        params:
          - name: actionType
            value: kubernetes/restart_pod
          - name: actionImage
            value: ghcr.io/kubernaut/actions/restart@sha256:def456
          - name: inputs
            value: '{"namespace":"production","labelSelector":"app=payment"}'

      - name: scale-deployment
        taskRef:
          name: kubernaut-action
        params:
          - name: actionType
            value: kubernetes/scale_deployment
          - name: actionImage
            value: ghcr.io/kubernaut/actions/scale@sha256:abc123
          - name: inputs
            value: '{"deployment":"payment-service","namespace":"production","replicas":10}'

      # Sequential task (runAfter)
      - name: create-gitops-pr
        taskRef:
          name: kubernaut-action
        params:
          - name: actionType
            value: git/create-pr
          - name: actionImage
            value: ghcr.io/kubernaut/actions/git-pr@sha256:ghi789
          - name: inputs
            value: '{"repository":"https://github.com/company/k8s-configs","branch":"kubernaut/payment-memory-fix","title":"Fix payment-service OOMKilled"}'
        runAfter:
          - restart-pods
          - scale-deployment
        workspaces:
          - name: workspace
            workspace: shared-workspace

    workspaces:
      - name: shared-workspace
        description: "Shared workspace for multi-step workflows"

  workspaces:
    - name: shared-workspace
      volumeClaimTemplate:
        spec:
          accessModes:
            - ReadWriteOnce
          resources:
            requests:
              storage: 1Gi
```

**Key Features**:
- ✅ Tekton handles DAG orchestration (runAfter dependencies)
- ✅ Tekton handles parallel execution (restart-pods + scale-deployment)
- ✅ Tekton handles workspace management (PVC creation/deletion)
- ✅ Generic meta-task (`kubernaut-action`) executes any container

---

## Deployment Prerequisites

### **For OpenShift Customers** (Primary Target)

**OpenShift Pipelines Operator** (Tekton):
```bash
# Check if already installed
oc get pods -n openshift-pipelines

# If not installed (one-time, 2 minutes)
oc apply -f - <<EOF
apiVersion: operators.coreos.com/v1alpha1
kind: Subscription
metadata:
  name: openshift-pipelines-operator
  namespace: openshift-operators
spec:
  channel: stable
  name: openshift-pipelines-operator-rh
  source: redhat-operators
EOF
```

**Effort**: ✅ Pre-installed or 2-minute install
**Support**: Red Hat enterprise support

---

### **For Non-OpenShift Customers**

**Upstream Tekton Pipelines**:
```bash
# Install Tekton Pipelines (one-time, 5 minutes)
kubectl apply -f https://storage.googleapis.com/tekton-releases/pipeline/latest/release.yaml

# Verify installation
kubectl get pods -n tekton-pipelines
NAME                                           READY   STATUS
tekton-pipelines-controller-68b8d87b6c-9xj2k   1/1     Running
tekton-pipelines-webhook-6c8d8c6f9d-7k4lm      1/1     Running

# Optional: Tekton Dashboard (for debugging)
kubectl apply -f https://storage.googleapis.com/tekton-releases/dashboard/latest/release.yaml
```

**Effort**: ✅ 5-minute one-time install
**Support**: CNCF community support

---

## Consequences

### **Positive Consequences**

#### **1. Zero Throwaway Code** 🎯
- **Before**: 600 lines (V1) → DELETE 500 lines → 100 lines (V2)
- **After**: 100 lines (V1) → Same architecture (V2-V10)
- **Savings**: 500 lines + 4 weeks development time

#### **2. No Migration Complexity** ✅
- **Before**: Feature flags, dual reconcilers, gradual rollout, rollback plans
- **After**: Single architecture from day 1
- **Result**: Zero migration risk

#### **3. Maximum Red Hat Alignment** 🔴
- OpenShift Pipelines = Tekton
- Pre-installed or one-click install
- Enterprise support from Red Hat
- Familiar to OpenShift teams

#### **4. Faster Time-to-Market** ⏱️
- **Custom orchestration**: 10 weeks (V1) + 6 weeks (V2) = 16 weeks
- **Tekton from V1**: 8 weeks total
- **Savings**: 50% reduction in time-to-final-architecture

#### **5. Battle-Tested Orchestration** 🏆
- CNCF Graduated Project
- Used by major enterprises (Google, IBM, Red Hat)
- Active community and ecosystem
- Long-term support guaranteed

#### **6. Universal Availability** 🌍
- OpenShift: Bundled (0 effort)
- Kubernetes: Upstream install (5 minutes)
- Result: No customer blocked

---

### **Negative Consequences**

#### **1. External Dependency** ⚠️

**Challenge**: Tekton is external dependency

**Mitigation**:
- CNCF Graduated (same trust as Kubernetes)
- Red Hat backing (OpenShift Pipelines)
- Active community (Google, IBM, Red Hat)
- **Risk Level**: Very Low

#### **2. Learning Curve** ⚠️

**Challenge**: Teams need Tekton knowledge

**Mitigation**:
- OpenShift teams already familiar (OpenShift Pipelines)
- Comprehensive documentation and runbooks
- Tekton CLI and Dashboard for debugging
- **Impact**: Medium (but worthwhile investment)

#### **3. Resource Footprint** ⚠️

**Challenge**: Tekton requires ~450MB

**Reality**:
- Acceptable trade-off for eliminating 500 lines of code
- Already installed on OpenShift (no additional footprint)
- **Impact**: Very Low

---

## Risks and Mitigations

### **Risk 1: Tekton Breaking Changes** 🚨

**Risk**: Tekton API changes break workflows

**Likelihood**: Low (CNCF Graduated = stable API)

**Mitigation**:
- Pin Tekton version in production
- Test upgrades in staging before production
- Red Hat enterprise support for OpenShift Pipelines
- **Residual Risk**: Very Low

---

### **Risk 2: Non-OpenShift Adoption Friction** 🚨

**Risk**: Vanilla Kubernetes customers resist Tekton install

**Likelihood**: Low (5-minute install is acceptable)

**Mitigation**:
- Comprehensive installation guide
- Automated installation scripts
- Clear value proposition (battle-tested orchestration)
- **Residual Risk**: Low

---

### **Risk 3: Tekton Complexity** 🚨

**Risk**: Tekton concepts harder to debug than custom code

**Likelihood**: Medium

**Mitigation**:
- Tekton CLI (`tkn`) for debugging
- Tekton Dashboard for visualization
- Comprehensive runbooks for common issues
- Red Hat support for OpenShift customers
- **Residual Risk**: Low (tooling mitigates)

---

## Related Decisions

- **[ADR-002: Native Kubernetes Jobs](ADR-002-native-kubernetes-jobs.md)** - Original execution foundation (now using Tekton TaskRuns)
- **[ADR-020: Workflow Parallel Execution Limits](ADR-020-workflow-parallel-execution-limits.md)** - Concurrency controls (now delegated to Tekton)
- **[ADR-021: Dependency Cycle Detection](ADR-021-workflow-dependency-cycle-detection.md)** - DAG validation (now delegated to Tekton)
- **[ADR-022: V1 Native Jobs with V2 Tekton Migration](ADR-022-v1-native-jobs-v2-tekton-migration.md)** - **SUPERSEDED** by this decision

---

## Links

### **Business Requirements**:
- **BR-REMEDIATION-001**: Multi-step workflow orchestration
  - Location: `docs/requirements/01_WORKFLOW_ORCHESTRATION.md`
  - Fulfilled: ✅ Via Tekton Pipelines

- **BR-REMEDIATION-002**: Parallel execution support
  - Location: `docs/requirements/01_WORKFLOW_ORCHESTRATION.md`
  - Fulfilled: ✅ Via Tekton `runAfter` dependencies

- **BR-PLATFORM-006**: OpenShift Pipelines integration (NEW)
  - Location: `docs/requirements/03_PLATFORM_KUBERNETES_OPERATIONS.md`
  - Fulfilled: ✅ Tekton = OpenShift Pipelines

### **Technical Documentation**:
- **Tekton Pipelines**: https://tekton.dev/docs/pipelines/
- **Tekton Tasks**: https://tekton.dev/docs/pipelines/tasks/
- **Tekton Workspaces**: https://tekton.dev/docs/pipelines/workspaces/
- **OpenShift Pipelines**: https://docs.openshift.com/pipelines/
- **Tekton CLI (tkn)**: https://tekton.dev/docs/cli/

---

## Success Metrics

### **Development Efficiency**
- ✅ **Code Reduction**: 600 lines → 100 lines (83% reduction)
- ✅ **Time Savings**: 16 weeks → 8 weeks (50% reduction)
- ✅ **Throwaway Code**: 500 lines → 0 lines (100% elimination)

### **Operational Metrics**
- ✅ **Installation Effort**: 0 min (OpenShift) or 5 min (upstream)
- ✅ **Resource Footprint**: ~450MB (acceptable)
- ✅ **Migration Risk**: Zero (no V1 → V2 migration)

### **Strategic Alignment**
- ✅ **Red Hat Alignment**: Maximum (OpenShift Pipelines = Tekton)
- ✅ **Industrial Acceptance**: Maximum (CNCF Graduated)
- ✅ **Universal Availability**: Bundled + upstream

---

## Decision Record

**Status**: ✅ Approved
**Decision Date**: 2025-10-19
**Approved By**: Architecture Team
**Implementation Target**: Q4 2025
**Confidence**: **95%** (Very High)

**Key Insight**: **Tekton from V1 eliminates 500+ lines of throwaway code** and ensures maximum Red Hat alignment with zero migration complexity.

**Supersedes**: [ADR-022: V1 Native Jobs with V2 Tekton Migration](ADR-022-v1-native-jobs-v2-tekton-migration.md)

