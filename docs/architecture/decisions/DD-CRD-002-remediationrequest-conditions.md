# DD-CRD-002-RemediationRequest: Kubernetes Conditions for RemediationRequest CRD

**Status**: ✅ APPROVED
**Version**: 1.0
**Date**: December 16, 2025
**CRD**: RemediationRequest
**Service**: RemediationOrchestrator
**Parent Standard**: DD-CRD-002

---

## 📋 Overview

This document specifies the Kubernetes Conditions for the **RemediationRequest** CRD per DD-CRD-002 standard.

**Business Requirement**: [BR-ORCH-043](mdc:docs/requirements/BR-ORCH-043-kubernetes-conditions-orchestration-visibility.md)

---

## 🎯 Condition Types (9)

| Condition Type | Purpose | Set By |
|----------------|---------|--------|
| `Ready` | Aggregate: True on Completed/Skipped, False on Failed/TimedOut/Cancelled | Reconciler |
| `SignalProcessingReady` | SP CRD created successfully | Creator |
| `SignalProcessingComplete` | SP completed/failed | Phase handler |
| `AIAnalysisReady` | AI CRD created successfully | Creator |
| `AIAnalysisComplete` | AI completed/failed | Phase handler |
| `WorkflowExecutionReady` | WE CRD created successfully | Creator |
| `WorkflowExecutionComplete` | WE completed/failed | Phase handler |
| `RecoveryComplete` | Terminal phase reached | Reconciler |
| `NotificationDelivered` | Notification delivery outcome | Reconciler |

---

## 📊 Condition Specifications

### SignalProcessingReady

| Status | Reason | Message Pattern |
|--------|--------|-----------------|
| `True` | `SignalProcessingCreated` | "SignalProcessing CRD {name} created successfully" |
| `False` | `SignalProcessingCreationFailed` | "Failed to create SignalProcessing: {error}" |

### SignalProcessingComplete

| Status | Reason | Message Pattern |
|--------|--------|-----------------|
| `True` | `SignalProcessingSucceeded` | "SignalProcessing completed (env: {env}, priority: {priority})" |
| `False` | `SignalProcessingFailed` | "SignalProcessing failed: {error}" |
| `False` | `SignalProcessingTimeout` | "SignalProcessing timed out after {duration}" |

### AIAnalysisReady

| Status | Reason | Message Pattern |
|--------|--------|-----------------|
| `True` | `AIAnalysisCreated` | "AIAnalysis CRD {name} created successfully" |
| `False` | `AIAnalysisCreationFailed` | "Failed to create AIAnalysis: {error}" |

### AIAnalysisComplete

| Status | Reason | Message Pattern |
|--------|--------|-----------------|
| `True` | `AIAnalysisSucceeded` | "AIAnalysis completed (workflow: {workflowID})" |
| `False` | `AIAnalysisFailed` | "AIAnalysis failed: {error}" |
| `False` | `AIAnalysisTimeout` | "AIAnalysis timed out after {duration}" |
| `False` | `NoWorkflowSelected` | "AIAnalysis completed but no workflow selected" |

### WorkflowExecutionReady

| Status | Reason | Message Pattern |
|--------|--------|-----------------|
| `True` | `WorkflowExecutionCreated` | "WorkflowExecution CRD {name} created successfully" |
| `False` | `WorkflowExecutionCreationFailed` | "Failed to create WorkflowExecution: {error}" |
| `False` | `ApprovalPending` | "Waiting for approval before WorkflowExecution" |

### WorkflowExecutionComplete

| Status | Reason | Message Pattern |
|--------|--------|-----------------|
| `True` | `WorkflowSucceeded` | "Workflow executed successfully" |
| `False` | `WorkflowFailed` | "Workflow failed: {error}" |
| `False` | `WorkflowTimeout` | "Workflow timed out after {duration}" |

### RecoveryComplete

| Status | Reason | Message Pattern |
|--------|--------|-----------------|
| `True` | `RecoverySucceeded` | "Remediation completed successfully" |
| `False` | `RecoveryFailed` | "Remediation failed: {error}" |
| `False` | `MaxAttemptsReached` | "Maximum recovery attempts ({max}) reached" |
| `False` | `BlockedByConsecutiveFailures` | "Blocked after {count} consecutive failures" |
| `False` | `InProgress` | "Remediation in progress (phase: {phase})" |

**Note**: RecoveryComplete is now set on timeout and blocked-terminal paths (previously a gap).

### Ready

| Status | Reason | Message Pattern |
|--------|--------|-----------------|
| `True` | `Ready` | "Remediation completed or skipped" |
| `False` | `NotReady` | "Remediation failed, timed out, or cancelled" |

**When Set**: True when phase is Completed or Skipped; False when phase is Failed, TimedOut, or Cancelled.

### NotificationDelivered

| Status | Reason | Message Pattern |
|--------|--------|-----------------|
| `True` | `ReasonDeliverySucceeded` | "Notification delivered successfully" |
| `False` | `ReasonDeliveryFailed` | "Notification delivery failed: {error}" |
| `False` | `ReasonUserCancelled` | "NotificationRequest deleted by user" |

**Constants**: Use `pkg/remediationrequest/conditions.go` constants: `ReasonDeliverySucceeded`, `ReasonDeliveryFailed`, `ReasonUserCancelled`.

---

## 🔧 Implementation

**Helper File**: `pkg/remediationrequest/conditions.go`

**MANDATORY**: Use canonical Kubernetes functions per DD-CRD-002 v1.2:
- `meta.SetStatusCondition()` for setting conditions
- `meta.FindStatusCondition()` for reading conditions

**Note**: Per DD-CRD-002 v1.1, each CRD has its own dedicated conditions file regardless of which controller manages it.

### Integration Points

| Integration Point | Conditions Set |
|-------------------|----------------|
| `creator/signalprocessing.go` | SignalProcessingReady |
| `controller/reconciler.go:handleProcessingPhase` | SignalProcessingComplete |
| `creator/aianalysis.go` | AIAnalysisReady |
| `controller/reconciler.go:handleAnalyzingPhase` | AIAnalysisComplete |
| `creator/workflowexecution.go` | WorkflowExecutionReady |
| `controller/reconciler.go:handleExecutingPhase` | WorkflowExecutionComplete |
| `controller/reconciler.go:transitionToCompleted` | RecoveryComplete (success) |
| `controller/reconciler.go:transitionToFailed` | RecoveryComplete (failure) |
| `controller/reconciler.go` (timeout/blocked-terminal paths) | RecoveryComplete (timeout, blocked) |

---

## ✅ Validation

```bash
kubectl explain remediationrequest.status.conditions
kubectl describe remediationrequest rr-test-123 | grep -A20 "Conditions:"
kubectl wait --for=condition=RecoveryComplete rr/rr-test-123 --timeout=10m
```

---

## 🏗️ Status

| Component | Status |
|-----------|--------|
| CRD Schema | ✅ Exists (line 635) |
| Helper functions | ⏳ Pending (BR-ORCH-043) |
| Controller integration | ⏳ Pending |
| Unit tests | ⏳ Pending |

---

## 🔗 References

- [DD-CRD-002](mdc:docs/architecture/decisions/DD-CRD-002-kubernetes-conditions-standard.md) (Parent)
- [BR-ORCH-043](mdc:docs/requirements/BR-ORCH-043-kubernetes-conditions-orchestration-visibility.md)
- [DD-CRD-002-RemediationApprovalRequest](mdc:docs/architecture/decisions/DD-CRD-002-remediationapprovalrequest-conditions.md)

