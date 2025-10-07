# E2E Test Scenarios - GitOps-Aware Strategy with Rego Auto-Approval

**Date**: 2025-10-02
**Status**: ✅ **APPROVED APPROACH**
**Strategy**: Imperative operations with Rego auto-approval + GitOps escalation for declarative changes

---

## 🎯 **UNIFIED GITOPS-AWARE REMEDIATION STRATEGY**

### Core Principle

**Hybrid Remediation**: Fast imperative mitigation + permanent GitOps fixes

```
Alert → AI Analysis → GitOps Detection
    ↓
Classify Actions:
  1. Imperative (restart, delete pod, node ops) → Rego policy evaluation → Auto-approve if safe
  2. Declarative (modify Deployment, ConfigMap, HPA) → Generate Git PR → Escalate
    ↓
Execute both in parallel:
  - Immediate: Imperative operation (auto-approved by Rego)
  - Permanent: Git PR escalation (human approval)
```

---

## 🔐 **REGO POLICY FOR IMPERATIVE AUTO-APPROVAL**

### Policy: `imperative-operations-auto-approval.rego`

```rego
package kubernaut.approval.imperative

import future.keywords.if
import data.kubernaut.approval.common

# Default: Require approval for all operations
default auto_approve := false
default require_approval := true

# Auto-approve LOW-RISK imperative operations
auto_approve if {
    input.operation_type == "imperative"
    is_low_risk_imperative_operation
}

# Low-risk imperative operations (safe, reversible, non-destructive)
is_low_risk_imperative_operation if {
    input.action in [
        "restart-deployment",      # kubectl rollout restart
        "delete-pod",              # kubectl delete pod (ReplicaSet recreates)
        "cordon-node",             # Prevents new scheduling, non-destructive
        "uncordon-node",           # Reverses cordon
    ]
}

# MEDIUM-RISK imperative operations (require approval in production)
require_approval if {
    input.operation_type == "imperative"
    is_medium_risk_imperative_operation
    input.environment == "production"
}

is_medium_risk_imperative_operation if {
    input.action in [
        "drain-node",              # Evicts pods, disruptive
        "backup-database",         # Resource intensive
        "restore-database",        # Potential data loss
    ]
}

# Auto-approve medium-risk in non-production
auto_approve if {
    input.operation_type == "imperative"
    is_medium_risk_imperative_operation
    input.environment in ["development", "staging"]
}

# DECLARATIVE operations always require Git PR (never auto-approve direct modification)
require_approval if {
    input.operation_type == "declarative"
    input.git_ops_managed == true
}

decision := {
    "auto_approve": auto_approve,
    "require_approval": require_approval,
    "approval_type": approval_type,
    "reason": reason,
}

approval_type := "rego-auto-approved" if {
    auto_approve
}

approval_type := "git-pr-required" if {
    input.operation_type == "declarative"
    input.git_ops_managed == true
}

approval_type := "manual-approval-required" if {
    require_approval
    not auto_approve
}

reason := sprintf("Auto-approved: %s is low-risk imperative operation in %s", [
    input.action,
    input.environment,
]) if {
    auto_approve
    is_low_risk_imperative_operation
}

reason := sprintf("Git PR required: %s is GitOps-managed resource", [
    input.resource_type,
]) if {
    input.operation_type == "declarative"
    input.git_ops_managed == true
}

reason := sprintf("Manual approval required: %s is medium-risk in %s", [
    input.action,
    input.environment,
]) if {
    require_approval
    not auto_approve
}
```

---

## 📋 **SCENARIO REMEDIATION PATTERNS**

### Pattern A: Imperative-Only (Auto-Approved by Rego)

**When**: Issue can be temporarily fixed by imperative operation

**Example**: Scenario 2 (Deployment Not Ready)

```yaml
Alert: Deployment 0/3 ready
AIAnalysis:
  rootCause: Stale pods with old config
  recommendation: Restart deployment
  operationType: "imperative"

WorkflowExecution:
  Step 1: Evaluate Rego policy
    Input:
      operation_type: "imperative"
      action: "restart-deployment"
      environment: "production"
      resource_type: "Deployment"

    Rego Decision:
      auto_approve: true
      approval_type: "rego-auto-approved"
      reason: "restart-deployment is low-risk imperative operation"

  Step 2: Execute immediately (no human approval needed)
    KubernetesExecution: restart-deployment

Result: ✅ Deployment restarted in 3-5 minutes (auto-approved)
```

---

### Pattern B: Imperative + GitOps Escalation (Hybrid)

**When**: Imperative operation provides temporary relief, but permanent fix requires Git PR

**Example**: Scenario 1 (Memory Limit Increase)

```yaml
Alert: Pod OOMKilled
AIAnalysis:
  rootCause: Memory limit 512Mi insufficient
  immediateMitigation: Restart pod (clears memory leaks temporarily)
  permanentFix: Increase memory to 1Gi (requires Git PR)

WorkflowExecution:
  # Parallel execution

  Track 1 (Immediate - Imperative):
    Step 1a: Evaluate Rego policy
      Input:
        operation_type: "imperative"
        action: "restart-deployment"
        environment: "production"

      Rego Decision:
        auto_approve: true
        reason: "restart-deployment is low-risk"

    Step 2a: Execute immediately
      KubernetesExecution: restart-deployment
      Status: ✅ Pod restarted (T+2min)

  Track 2 (Permanent - Declarative):
    Step 1b: Detect GitOps annotation on Deployment
      Annotation: "argocd.argoproj.io/tracking-id"
      Decision: Cannot modify Deployment directly

    Step 2b: Generate Git PR proposal
      gitOpsMetadata:
        repository: "github.com/company/k8s-manifests"
        path: "production/webapp/deployment.yaml"

      proposedChange: |
        resources:
          limits:
            memory: 1Gi  # Changed from 512Mi

    Step 3b: Escalate to human
      AIAnalysis.status.phase = "awaiting_git_pr"
      Notification sent with PR template
      Status: ⏳ Awaiting human Git PR (T+2min)

Result:
  - ✅ Immediate: Pod restarted, app recovers temporarily (T+3min)
  - ⏳ Permanent: Git PR created, awaiting merge (T+15-30min)
```

---

### Pattern C: GitOps-Only Escalation (No Immediate Action)

**When**: No imperative operation can help, must wait for Git PR

**Example**: Scenario 5 (Image Pull Error)

```yaml
Alert: ImagePullBackOff (bad image tag v2.0.1)
AIAnalysis:
  rootCause: Image tag not found in registry
  immediateMitigation: ❌ None available
  permanentFix: Revert image tag to v2.0.0 via Git

WorkflowExecution:
  Step 1: Detect GitOps-managed Deployment
    Annotation: "argocd.argoproj.io/tracking-id"
    Decision: Cannot modify Deployment.spec.image

  Step 2: Generate Git revert PR
    proposedChange: |
      image: webapp:v2.0.0  # Revert from v2.0.1

  Step 3: Escalate to human (URGENT)
    Priority: HIGH (app completely broken)
    AIAnalysis.status.phase = "awaiting_git_pr"

Result:
  - ❌ No immediate action available
  - ⏳ Must wait for Git PR merge + ArgoCD sync
  - Duration: T+15-30min (human-dependent)
```

---

### Pattern D: Medium-Risk Imperative (Manual Approval in Production)

**When**: Imperative operation is disruptive, requires human approval in production

**Example**: Scenario 11 (Node Drain)

```yaml
Alert: Node network connectivity issues
AIAnalysis:
  rootCause: Node hardware failure
  recommendation: Drain node + migrate pods
  operationType: "imperative"

WorkflowExecution:
  Step 1: Evaluate Rego policy
    Input:
      operation_type: "imperative"
      action: "drain-node"
      environment: "production"
      affected_pods: 15

    Rego Decision:
      auto_approve: false
      require_approval: true
      approval_type: "manual-approval-required"
      reason: "drain-node is medium-risk in production (affects 15 pods)"
      min_approvers: 1
      approver_groups: ["system:kubernaut:production-approvers"]

  Step 2: Create AIApprovalRequest CRD
    spec:
      recommendation: "drain-node node-5"
      impact: "15 pods will be evicted and rescheduled"
      estimated_duration: "8-12 minutes"
    Status: ⏳ Awaiting 1 approver (T+0)

  Step 3: User approves (kubectl patch aiapprovalrequest)
    Status: ✅ Approved by alice@company.com (T+5min)

  Step 4: Execute drain operation
    KubernetesExecution: drain-node
    Status: ✅ Node drained, pods migrated (T+12min)

Result:
  - ⏳ Manual approval required (5 min)
  - ✅ Node drained successfully (12 min total)
```

---

## 📊 **REVISED SCENARIO CLASSIFICATION**

### Auto-Approved Imperative (Rego Policy)

| # | Scenario | Imperative Action | Auto-Approve? | Duration |
|---|---|---|---|---|
| 2 | Restart Deployment | `kubectl rollout restart` | ✅ Yes (low-risk) | 3-5 min |
| 3 | Cordon Node | `kubectl cordon` | ✅ Yes (non-destructive) | 2-3 min |
| 4 | Delete Pod | `kubectl delete pod` | ✅ Yes (ReplicaSet recreates) | 2-3 min |
| 8 | Restart Service | `kubectl rollout restart` | ✅ Yes (low-risk) | 3-5 min |

### Hybrid (Auto-Approved Imperative + Git Escalation)

| # | Scenario | Imperative (Auto) | Declarative (Git PR) | Duration |
|---|---|---|---|---|
| 1 | Memory Increase | Restart pod ✅ | Increase memory ⏳ | 3 min + 15-30 min |
| 9 | Full Recovery | Restart deployment ✅ | Fix ConfigMap ⏳ | 5 min + 15-30 min |
| 10 | Backup + Restore | Backup/restore ✅ | Update deployment ⏳ | 15 min + 30 min |
| 14 | CPU Optimization | Restart pod ✅ | Increase CPU ⏳ | 3 min + 15-30 min |

### Manual Approval Imperative (Rego Policy - Production)

| # | Scenario | Imperative Action | Approval | Duration |
|---|---|---|---|---|
| 11 | Node Migration | `kubectl drain` | Manual (1 approver) | 5 min + 8-12 min |

### GitOps-Only Escalation (No Immediate Action)

| # | Scenario | Why No Immediate Action? | Git PR Required | Duration |
|---|---|---|---|---|
| 5 | Image Rollback | Bad image cannot be fixed imperatively | Yes | 15-30 min |
| 6 | ConfigMap Fix | GitOps-managed config | Yes | 15-30 min |
| 7 | HPA Adjustment | GitOps-managed HPA | Yes | 15-30 min |
| 12 | API Rollback | Multi-resource GitOps change | Yes | 15-30 min |
| 13 | Scale + Optimize | Multi-resource GitOps change | Yes | 15-30 min |
| 15 | Iterative Learning | Multiple GitOps changes | Yes (3 PRs) | 30-60 min |

### AI Feedback Loop (Mixed Patterns)

| # | Scenario | Pattern | Duration |
|---|---|---|---|
| 14 | Memory → CPU Fix | Hybrid (restart + 2 Git PRs) | 30-45 min |
| 15 | Iterative Learning | GitOps-only (3 Git PRs) | 45-60 min |
| 16 | Manual Escalation | Mixed (backup + escalation) | 15-20 min |

---

## 🔐 **REGO POLICY INTEGRATION IN WORKFLOW**

### AIAnalysis Phase: Operation Classification

```go
// In AIAnalysisReconciler.recommendingPhase()
func (r *AIAnalysisReconciler) classifyRecommendation(
    recommendation *Recommendation,
    targetResource *unstructured.Unstructured,
) (*OperationClassification, error) {

    classification := &OperationClassification{
        OperationType: determineOperationType(recommendation.Action),
        GitOpsManaged: hasGitOpsAnnotation(targetResource),
    }

    // Check for GitOps annotations
    if hasArgoAnnotation(targetResource) {
        classification.GitOpsManaged = true
        classification.GitOpsMetadata = extractArgoMetadata(targetResource)
    }

    return classification, nil
}

func determineOperationType(action string) string {
    imperativeActions := []string{
        "restart-deployment",
        "restart-pod",
        "delete-pod",
        "cordon-node",
        "drain-node",
        "uncordon-node",
        "backup-database",
        "restore-database",
    }

    for _, imperative := range imperativeActions {
        if action == imperative {
            return "imperative"
        }
    }

    return "declarative"  // Modifies resource spec
}
```

### WorkflowExecution Phase: Rego Policy Evaluation

```go
// In WorkflowExecutionReconciler.validatingPhase()
func (r *WorkflowExecutionReconciler) evaluateApprovalPolicy(
    ctx context.Context,
    workflow *workflowv1.WorkflowExecution,
) (*PolicyDecision, error) {

    // Build Rego input
    policyInput := map[string]interface{}{
        "operation_type": workflow.Spec.OperationClassification.OperationType,
        "action": workflow.Spec.RecommendedAction.Action,
        "environment": workflow.Spec.Environment,
        "git_ops_managed": workflow.Spec.OperationClassification.GitOpsManaged,
        "resource_type": workflow.Spec.RecommendedAction.TargetResource.Kind,
    }

    // Evaluate Rego policy
    results, err := r.PolicyEngine.Eval(ctx, rego.EvalInput(policyInput))
    if err != nil {
        return nil, err
    }

    decision := results[0].Expressions[0].Value.(map[string]interface{})

    return &PolicyDecision{
        AutoApprove: decision["auto_approve"].(bool),
        RequireApproval: decision["require_approval"].(bool),
        ApprovalType: decision["approval_type"].(string),
        Reason: decision["reason"].(string),
    }, nil
}
```

### Dual-Track Execution

```go
// In WorkflowExecutionReconciler.executingPhase()
func (r *WorkflowExecutionReconciler) executeHybridWorkflow(
    ctx context.Context,
    workflow *workflowv1.WorkflowExecution,
) error {

    // Track 1: Immediate imperative action (if auto-approved)
    if policyDecision.AutoApprove {
        go r.executeImperativeTrack(ctx, workflow)
    }

    // Track 2: GitOps escalation (if declarative change needed)
    if workflow.Spec.OperationClassification.GitOpsManaged {
        go r.escalateGitOpsTrack(ctx, workflow)
    }

    // Monitor both tracks
    return r.monitorDualTrackExecution(ctx, workflow)
}
```

---

## ✅ **BENEFITS OF THIS APPROACH**

| Benefit | Description |
|---|---|
| **Fast Mitigation** | Imperative operations execute immediately (auto-approved) |
| **GitOps Compliance** | Declarative changes always go through Git |
| **Policy-Driven** | Rego policies define what's auto-approved |
| **Audit Trail** | All operations logged (Rego decision + Git PR) |
| **Safety** | Medium-risk operations require manual approval in production |
| **Flexibility** | Policies can be updated via ConfigMap without code changes |

---

## 🚀 **NEXT STEPS**

1. ✅ **Rewrite E2E scenarios** with this unified approach
2. ✅ **Add Rego policy examples** for all operation types
3. ✅ **Update AIAnalysis design** with operation classification
4. ✅ **Update WorkflowExecution design** with dual-track execution
5. ✅ **Add BR-GITOPS-001** business requirement

---

**Status**: ✅ **APPROVED STRATEGY** - Ready to implement in E2E scenarios

