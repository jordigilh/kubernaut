# 📢 API Contract Change Notification: WorkflowExecution v3.0 / v3.1

**From**: Workflow Engine (WE) Service Team
**To**: RemediationOrchestrator (RO) Team, AIAnalysis Service Team
**Date**: December 1, 2025
**Priority**: 🔴 High (v3.1 requires RO changes)
**Effective Version**: WorkflowExecution CRD Schema v3.0 + v3.1

---

## 📋 Executive Summary

The WorkflowExecution service is introducing two significant changes:

### v3.0: Enhanced Failure Details (Non-Breaking)
- **New field**: `status.failureDetails` with structured failure information
- **Purpose**: Richer failure data for notifications, LLM recovery, deterministic decisions

### v3.1: Resource Locking Safety (🔴 RO Action Required)
- **New spec field**: `spec.targetResource` (REQUIRED)
- **New status phase**: `Skipped` with `skipDetails`
- **Purpose**: Prevent parallel/redundant workflow executions on same target
- **RO Impact**: Must populate `targetResource` and handle `Skipped` phase

---

## 🔄 What's Changing

### New Status Field: `failureDetails`

When a workflow execution fails (`status.phase = "Failed"`), WE will now populate:

```yaml
status:
  phase: Failed
  # DEPRECATED (still populated for backward compatibility)
  failureReason: "RBAC denied: cannot patch deployments.apps"

  # NEW in v3.0
  failureDetails:
    failedTaskIndex: 1                    # 0-indexed position in pipeline
    failedTaskName: "apply-memory-increase"
    failedStepName: "kubectl-patch"       # Optional: step within task
    reason: "Forbidden"                   # K8s-style enum (see below)
    message: "RBAC denied: cannot patch deployments.apps in namespace payment"
    exitCode: 1                           # Optional: container exit code
    failedAt: "2025-12-01T10:15:45Z"
    executionTimeBeforeFailure: "45s"
    naturalLanguageSummary: |
      Task 'apply-memory-increase' (step 2 of 3) failed after 45s with Forbidden error.
      The PipelineRun service account (e.g. 'my-workflow-sa' per DD-WE-005 v2.0) lacks required RBAC permissions.
      Recommendation: Grant patch permission to that ServiceAccount or use an alternative workflow.
```

### Failure Reason Code Enumeration

| Reason Code | Description | Typical Cause |
|-------------|-------------|---------------|
| `OOMKilled` | Container killed due to memory limits | Task needs more memory |
| `DeadlineExceeded` | Timeout reached | Slow operation or short timeout |
| `Forbidden` | RBAC/permission failure | Missing ServiceAccount permissions |
| `ResourceExhausted` | Cluster resource limits | Quota exceeded |
| `ConfigurationError` | Invalid parameters | Should be caught by validation |
| `ImagePullBackOff` | Cannot pull container image | Image doesn't exist or no creds |
| `Unknown` | Unclassified failure | Manual investigation needed |

---

## 🔐 v3.1: Resource Locking (RO Action Required)

### Problem Solved

When a node experiences DiskPressure, multiple pods may be evicted, generating 10+ signals. Each signal resolves to the same root cause (`node-disk-cleanup` workflow). Without resource locking, 10+ parallel WorkflowExecutions would target the same node.

### New Spec Field: `targetResource` (REQUIRED)

```yaml
spec:
  # NEW in v3.1 - RO must populate this
  targetResource: "payment/deployment/payment-api"  # namespace/kind/name
  # For cluster-scoped: "node/worker-node-1"       # kind/name
```

**Format**:
- Namespaced: `namespace/kind/name` (e.g., `payment/deployment/payment-api`)
- Cluster-scoped: `kind/name` (e.g., `node/worker-node-1`)

### New Phase: `Skipped`

```yaml
status:
  phase: Skipped  # NEW valid phase

  # No PipelineRun created
  pipelineRunRef: null

  skipDetails:
    reason: "ResourceBusy"  # or "RecentlyRemediated"
    message: "Another workflow is currently remediating this resource"
    skippedAt: "2025-12-01T10:16:00Z"
    conflictingWorkflow:       # When reason=ResourceBusy
      name: "workflow-payment-oom-001"
      workflowId: "oomkill-increase-memory"
      startedAt: "2025-12-01T10:15:00Z"
      targetResource: "payment/deployment/payment-api"
    recentRemediation:         # When reason=RecentlyRemediated
      name: "workflow-node-disk-001"
      workflowId: "node-disk-cleanup"
      completedAt: "2025-12-01T10:18:00Z"
      outcome: "Completed"
      targetResource: "node/worker-node-1"
      cooldownRemaining: "4m30s"
```

### Lock Decision Rules

| Scenario | Decision |
|----------|----------|
| Another workflow **Running** on same target | **Skip (ResourceBusy)** |
| Same workflow+target completed **<5 min ago** | **Skip (RecentlyRemediated)** |
| Different workflow on same target, completed recently | **Allow** |
| Same workflow+target completed **>5 min ago** | **Allow** |
| Different target | **Allow** |

### 🔴 RO Required Changes

#### 1. Populate `targetResource` When Creating WE

```go
// pkg/remediationorchestrator/reconciler.go
func (r *Reconciler) createWorkflowExecution(
    ctx context.Context,
    aiAnalysis *v1alpha1.AIAnalysis,
    rr *v1alpha1.RemediationRequest,
) error {
    wfe := &v1alpha1.WorkflowExecution{
        Spec: v1alpha1.WorkflowExecutionSpec{
            // ... existing fields ...

            // NEW v3.1: Required for resource locking
            TargetResource: buildTargetResource(rr, aiAnalysis),
        },
    }
    return r.Create(ctx, wfe)
}

// Extract target from signal context
func buildTargetResource(rr *v1alpha1.RemediationRequest, aia *v1alpha1.AIAnalysis) string {
    // For pods/deployments: namespace/kind/name
    if rr.Spec.SignalContext.TargetNamespace != "" {
        return fmt.Sprintf("%s/%s/%s",
            rr.Spec.SignalContext.TargetNamespace,
            rr.Spec.SignalContext.TargetKind,
            rr.Spec.SignalContext.TargetName)
    }
    // For cluster-scoped (nodes): kind/name
    return fmt.Sprintf("%s/%s",
        rr.Spec.SignalContext.TargetKind,
        rr.Spec.SignalContext.TargetName)
}
```

#### 2. Handle `Skipped` Phase in Status Watching

```go
// pkg/remediationorchestrator/reconciler.go
func (r *Reconciler) handleWorkflowExecutionStatus(
    ctx context.Context,
    rr *v1alpha1.RemediationRequest,
    we *v1alpha1.WorkflowExecution,
) error {
    switch we.Status.Phase {
    case "Completed":
        return r.handleWorkflowExecutionCompleted(ctx, rr, we)
    case "Failed":
        return r.handleWorkflowExecutionFailed(ctx, rr, we)

    // NEW v3.1: Handle Skipped phase
    case "Skipped":
        return r.handleWorkflowExecutionSkipped(ctx, rr, we)

    case "Running", "Pending":
        return nil // Still in progress
    }
    return nil
}

func (r *Reconciler) handleWorkflowExecutionSkipped(
    ctx context.Context,
    rr *v1alpha1.RemediationRequest,
    we *v1alpha1.WorkflowExecution,
) error {
    // Log skip reason
    log.Info("WorkflowExecution skipped",
        "reason", we.Status.SkipDetails.Reason,
        "target", we.Spec.TargetResource)

    // Update RemediationRequest status
    rr.Status.Phase = "Skipped"
    rr.Status.Message = we.Status.SkipDetails.Message

    // Optionally create notification
    if we.Status.SkipDetails.Reason == "ResourceBusy" {
        // Notify that another remediation is in progress
        return r.createSkippedNotification(ctx, rr, we)
    }

    return r.Status().Update(ctx, rr)
}
```

---

## 👥 Impact by Team

### 🎯 RemediationOrchestrator (RO) Team

**Action Required**: Update recovery flow to use `failureDetails`

#### Current Flow (No Change Required)
```go
// RO watches WorkflowExecution status
if we.Status.Phase == "Failed" {
    // Previously: we.Status.FailureReason (still works)
}
```

#### Enhanced Flow (Recommended)
```go
// RO watches WorkflowExecution status
if we.Status.Phase == "Failed" && we.Status.FailureDetails != nil {
    // Use structured data for recovery AIAnalysis
    prevExec := v1alpha1.PreviousExecution{
        WorkflowExecutionRef: we.Name,
        Failure: v1alpha1.ExecutionFailure{
            FailedStepIndex: we.Status.FailureDetails.FailedTaskIndex,
            FailedStepName:  we.Status.FailureDetails.FailedTaskName,
            Reason:          we.Status.FailureDetails.Reason,    // K8s enum
            Message:         we.Status.FailureDetails.Message,
            ExitCode:        we.Status.FailureDetails.ExitCode,
            FailedAt:        we.Status.FailureDetails.FailedAt,
            ExecutionTime:   we.Status.FailureDetails.ExecutionTimeBeforeFailure,
        },
        // Include natural language for LLM context
        NaturalLanguageSummary: we.Status.FailureDetails.NaturalLanguageSummary,
    }

    // Pass to recovery AIAnalysis
    recoveryAIA.Spec.PreviousExecutions = append(..., prevExec)
}
```

#### Data Mapping Table

| WE Status Field | → | AIAnalysis Spec Field |
|-----------------|---|----------------------|
| `failureDetails.failedTaskIndex` | → | `previousExecutions[].failure.failedStepIndex` |
| `failureDetails.failedTaskName` | → | `previousExecutions[].failure.failedStepName` |
| `failureDetails.reason` | → | `previousExecutions[].failure.reason` |
| `failureDetails.message` | → | `previousExecutions[].failure.message` |
| `failureDetails.exitCode` | → | `previousExecutions[].failure.exitCode` |
| `failureDetails.failedAt` | → | `previousExecutions[].failure.failedAt` |
| `failureDetails.executionTimeBeforeFailure` | → | `previousExecutions[].failure.executionTime` |
| `failureDetails.naturalLanguageSummary` | → | `previousExecutions[].naturalLanguageSummary` |

---

### 🤖 AIAnalysis Service Team

**Action Required**: None (if using existing `PreviousExecution` struct)

The `AIAnalysisSpec.PreviousExecutions[].Failure` struct already supports:
- `FailedStepIndex int`
- `FailedStepName string`
- `Reason string`
- `Message string`
- `ExitCode *int32`
- `FailedAt metav1.Time`
- `ExecutionTime string`

**New Field Recommendation**: Consider adding `NaturalLanguageSummary string` to `PreviousExecution` if not already present. This allows the LLM to receive human-readable failure context for better recovery analysis.

#### LLM Prompt Integration

The `naturalLanguageSummary` is designed for direct inclusion in recovery prompts:

```markdown
## Previous Execution Attempt

The previous remediation attempt failed:

{previousExecutions[0].naturalLanguageSummary}

Please analyze this failure and select an alternative workflow that avoids this issue.
```

---

## 📊 Backward Compatibility

| Aspect | Status |
|--------|--------|
| `status.failureReason` | ✅ Still populated (deprecated) |
| `status.failureDetails` | ✅ New (optional, may be nil on success) |
| Existing RO code | ✅ Works unchanged |
| Enhanced RO code | ✅ Uses new structured data |

**Migration Path**:
1. **Phase 1** (Now): WE populates both `failureReason` and `failureDetails`
2. **Phase 2** (v4.0): RO migrates to `failureDetails` exclusively
3. **Phase 3** (v5.0): `failureReason` removed

---

## 📚 Reference Documents

| Document | Version | Purpose |
|----------|---------|---------|
| [WE CRD Schema](./crd-schema.md) | v3.0 | Complete type definitions |
| [DD-CONTRACT-001](../../../architecture/decisions/DD-CONTRACT-001-aianalysis-workflowexecution-alignment.md) | v1.3 | Contract alignment, recovery flow |
| [DD-RECOVERY-002](../../../architecture/decisions/DD-RECOVERY-002-direct-aianalysis-recovery-flow.md) | - | Recovery flow design |
| [DD-RECOVERY-003](../../../architecture/decisions/DD-RECOVERY-003-recovery-prompt-design.md) | - | Recovery prompt with K8s reason codes |

---

## ❓ Questions & Support

**WE Service Team Contact**: [TBD - Add team contact]

**Architecture Review**: This change was discussed in the AIAnalysis ↔ WE brainstorming session on 2025-12-01.

**Key Decisions Made**:
1. ✅ Rich structured failure data for user notification and LLM recovery
2. ✅ K8s-style reason codes for deterministic recovery decisions
3. ✅ Natural language summary generated by WE (not RO)
4. ✅ RO is the sole coordinator - no direct AIAnalysis ↔ WE relationship
5. ✅ Alternative workflows NOT passed to WE (recovery creates new AIAnalysis)

---

## ✅ Action Items Checklist

### RO Team
- [ ] Review `FailureDetails` struct in WE CRD schema v3.0
- [ ] Plan integration of structured failure data into recovery flow
- [ ] Update `handleWorkflowExecutionFailed()` to use `failureDetails`
- [ ] Add `NaturalLanguageSummary` to `PreviousExecution` struct (if needed)
- [ ] Test recovery flow with new failure data

### AIAnalysis Team
- [ ] Review `PreviousExecution.Failure` struct alignment
- [ ] Ensure `NaturalLanguageSummary` field exists (or add it)
- [ ] Update recovery prompt template to include natural language summary
- [ ] Test LLM recovery with enhanced failure context

---

**Document Version**: 1.1
**Last Updated**: March 21, 2026
**Status**: 📬 Pending Team Acknowledgment

**Note (DD-WE-005 v2.0)**: Failure summaries should refer to the **per-workflow** PipelineRun ServiceAccount (for example `my-workflow-sa` from `spec.executionConfig.serviceAccountName`), not a platform-managed shared runner SA.

