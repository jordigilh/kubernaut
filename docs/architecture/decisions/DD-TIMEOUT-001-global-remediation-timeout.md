# DD-TIMEOUT-001: Global Remediation Timeout Strategy

**Status**: ✅ Approved
**Version**: 1.0
**Date**: 2025-11-28
**Confidence**: 95%

---

## Context

A `RemediationRequest` orchestrates multiple child CRDs (SignalProcessing, AIAnalysis, RemediationApprovalRequest, WorkflowExecution) through the RemediationOrchestrator. Each phase can potentially hang indefinitely due to:

1. **AIAnalysis**: HolmesGPT API timeout, LLM hang
2. **Approval**: Operator never responds
3. **WorkflowExecution**: Tekton pipeline stuck
4. **External dependencies**: Data Storage API, Notification service

Without global timeout enforcement, stuck remediations consume resources and create operational blind spots.

---

## Problem

**Question**: How do we prevent RemediationRequests from becoming stuck indefinitely?

**Requirements**:
1. Global timeout for entire remediation lifecycle
2. Per-phase timeouts for granular control
3. Timeout detection without polling
4. Clear timeout notifications to operators

---

## Decision

**Implement a 2-level timeout strategy in RemediationOrchestrator:**

1. **Global timeout**: Maximum time for entire remediation (default: 1 hour)
2. **Phase timeouts**: Maximum time per phase (configurable per phase)

---

## Implementation

### Timeout Configuration

```yaml
# ConfigMap: remediation-config
timeouts:
  global: 1h              # Max time for entire remediation
  phases:
    signalProcessing: 5m  # Signal enrichment and classification
    aiAnalysis: 10m       # HolmesGPT investigation
    approval: 15m         # Operator approval (per ADR-040)
    workflowExecution: 30m # Tekton pipeline execution
```

### CRD Spec Extension

```go
// api/remediation/v1alpha1/remediationrequest_types.go
package v1alpha1

import (
    metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

type RemediationRequestSpec struct {
    // ... existing fields ...

    // Timeouts configures timeout behavior for this remediation
    // If not specified, defaults from ConfigMap are used
    Timeouts *TimeoutConfig `json:"timeouts,omitempty"`
}

type TimeoutConfig struct {
    // Global is the maximum duration for the entire remediation
    // Default: 1h
    Global *metav1.Duration `json:"global,omitempty"`

    // Phases configures per-phase timeouts
    Phases *PhaseTimeouts `json:"phases,omitempty"`
}

type PhaseTimeouts struct {
    // SignalProcessing timeout (default: 5m)
    SignalProcessing *metav1.Duration `json:"signalProcessing,omitempty"`

    // AIAnalysis timeout (default: 10m)
    AIAnalysis *metav1.Duration `json:"aiAnalysis,omitempty"`

    // Approval timeout (default: 15m, per ADR-040)
    Approval *metav1.Duration `json:"approval,omitempty"`

    // WorkflowExecution timeout (default: 30m)
    WorkflowExecution *metav1.Duration `json:"workflowExecution,omitempty"`
}
```

### Controller Implementation

```go
// internal/controller/remediationorchestrator/timeout.go
package remediationorchestrator

import (
    "context"
    "time"

    remediationv1alpha1 "github.com/jordigilh/kubernaut/api/remediation/v1alpha1"
    metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
    ctrl "sigs.k8s.io/controller-runtime"
)

// Default timeouts
const (
    DefaultGlobalTimeout            = 1 * time.Hour
    DefaultSignalProcessingTimeout  = 5 * time.Minute
    DefaultAIAnalysisTimeout        = 10 * time.Minute
    DefaultApprovalTimeout          = 15 * time.Minute
    DefaultWorkflowExecutionTimeout = 30 * time.Minute
)

// TimeoutChecker checks for timeout conditions
type TimeoutChecker struct {
    config *TimeoutConfig
}

func NewTimeoutChecker(remediation *remediationv1alpha1.RemediationRequest, defaults *TimeoutConfig) *TimeoutChecker {
    config := defaults
    if remediation.Spec.Timeouts != nil {
        config = mergeTimeouts(defaults, remediation.Spec.Timeouts)
    }
    return &TimeoutChecker{config: config}
}

// CheckGlobalTimeout returns true if global timeout exceeded
func (t *TimeoutChecker) CheckGlobalTimeout(remediation *remediationv1alpha1.RemediationRequest) bool {
    elapsed := time.Since(remediation.CreationTimestamp.Time)
    return elapsed > t.getGlobalTimeout()
}

// CheckPhaseTimeout returns true if current phase timeout exceeded
func (t *TimeoutChecker) CheckPhaseTimeout(remediation *remediationv1alpha1.RemediationRequest) bool {
    phaseStart := t.getPhaseStartTime(remediation)
    if phaseStart.IsZero() {
        return false
    }

    elapsed := time.Since(phaseStart)
    timeout := t.getPhaseTimeout(remediation.Status.Phase)

    return elapsed > timeout
}

func (t *TimeoutChecker) getPhaseStartTime(remediation *remediationv1alpha1.RemediationRequest) time.Time {
    if remediation.Status.PhaseTransitions == nil {
        return time.Time{}
    }
    if ts, ok := remediation.Status.PhaseTransitions[remediation.Status.Phase]; ok {
        return ts.Time
    }
    return time.Time{}
}

func (t *TimeoutChecker) getPhaseTimeout(phase string) time.Duration {
    switch phase {
    case "SignalProcessing":
        return t.config.Phases.SignalProcessing.Duration
    case "AIAnalysis":
        return t.config.Phases.AIAnalysis.Duration
    case "Approving":
        return t.config.Phases.Approval.Duration
    case "Executing":
        return t.config.Phases.WorkflowExecution.Duration
    default:
        return DefaultGlobalTimeout
    }
}

func (t *TimeoutChecker) getGlobalTimeout() time.Duration {
    if t.config.Global != nil {
        return t.config.Global.Duration
    }
    return DefaultGlobalTimeout
}
```

### Reconciler Integration

```go
// internal/controller/remediationorchestrator/remediationrequest_controller.go
package remediationorchestrator

func (r *RemediationRequestReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
    log := log.FromContext(ctx)

    var remediation remediationv1alpha1.RemediationRequest
    if err := r.Get(ctx, req.NamespacedName, &remediation); err != nil {
        return ctrl.Result{}, client.IgnoreNotFound(err)
    }

    // Skip if already in terminal state
    if isTerminalPhase(remediation.Status.Phase) {
        return ctrl.Result{}, nil
    }

    // Initialize timeout checker
    timeoutChecker := NewTimeoutChecker(&remediation, r.DefaultTimeouts)

    // Check global timeout first
    if timeoutChecker.CheckGlobalTimeout(&remediation) {
        log.Info("Global timeout exceeded", "elapsed", time.Since(remediation.CreationTimestamp.Time))
        return r.handleTimeout(ctx, &remediation, "GlobalTimeout", "Remediation exceeded maximum allowed time")
    }

    // Check phase-specific timeout
    if timeoutChecker.CheckPhaseTimeout(&remediation) {
        log.Info("Phase timeout exceeded", "phase", remediation.Status.Phase)
        return r.handleTimeout(ctx, &remediation, "PhaseTimeout", fmt.Sprintf("Phase %s exceeded timeout", remediation.Status.Phase))
    }

    // Continue with normal reconciliation...
    // Schedule requeue at next timeout boundary
    nextTimeout := timeoutChecker.NextTimeoutIn(&remediation)
    return ctrl.Result{RequeueAfter: nextTimeout}, nil
}

func (r *RemediationRequestReconciler) handleTimeout(
    ctx context.Context,
    remediation *remediationv1alpha1.RemediationRequest,
    reason string,
    message string,
) (ctrl.Result, error) {
    // Update status to Timeout
    remediation.Status.Phase = "Timeout"
    remediation.Status.Message = message
    remediation.Status.Conditions = append(remediation.Status.Conditions, metav1.Condition{
        Type:               "Timeout",
        Status:             metav1.ConditionTrue,
        Reason:             reason,
        Message:            message,
        LastTransitionTime: metav1.Now(),
    })

    if err := r.Status().Update(ctx, remediation); err != nil {
        return ctrl.Result{}, err
    }

    // Create notification for timeout
    if err := r.CreateNotificationForTimeout(ctx, remediation, reason); err != nil {
        // Log but don't fail - notification is best-effort
        log.FromContext(ctx).Error(err, "Failed to create timeout notification")
    }

    return ctrl.Result{}, nil
}

func isTerminalPhase(phase string) bool {
    return phase == "Completed" || phase == "Failed" || phase == "Timeout" || phase == "Rejected"
}
```

---

## Timeout Flow

```
┌─────────────────────────────────────────────────────────────────────────┐
│                         TIMEOUT ENFORCEMENT                              │
├─────────────────────────────────────────────────────────────────────────┤
│                                                                         │
│  RemediationRequest Created                                             │
│       │                                                                 │
│       │ Start global timeout timer (1h)                                 │
│       ▼                                                                 │
│  ┌─────────────────────────────────────────────────────────────────┐    │
│  │  SignalProcessing Phase                                         │    │
│  │  Timeout: 5m                                                    │    │
│  │  Check: elapsed > 5m → Phase Timeout                            │    │
│  └────────────────────────────┬────────────────────────────────────┘    │
│                               │                                         │
│                               ▼                                         │
│  ┌─────────────────────────────────────────────────────────────────┐    │
│  │  AIAnalysis Phase                                               │    │
│  │  Timeout: 10m                                                   │    │
│  │  Check: elapsed > 10m → Phase Timeout                           │    │
│  └────────────────────────────┬────────────────────────────────────┘    │
│                               │                                         │
│                               ▼                                         │
│  ┌─────────────────────────────────────────────────────────────────┐    │
│  │  Approving Phase (if needed)                                    │    │
│  │  Timeout: 15m (per ADR-040)                                     │    │
│  │  Check: elapsed > 15m → Phase Timeout                           │    │
│  └────────────────────────────┬────────────────────────────────────┘    │
│                               │                                         │
│                               ▼                                         │
│  ┌─────────────────────────────────────────────────────────────────┐    │
│  │  WorkflowExecution Phase                                        │    │
│  │  Timeout: 30m                                                   │    │
│  │  Check: elapsed > 30m → Phase Timeout                           │    │
│  └────────────────────────────┬────────────────────────────────────┘    │
│                               │                                         │
│                               ▼                                         │
│  ┌─────────────────────────────────────────────────────────────────┐    │
│  │  Global Check (every reconcile)                                 │    │
│  │  Total elapsed > 1h → Global Timeout                            │    │
│  └─────────────────────────────────────────────────────────────────┘    │
│                                                                         │
│  On ANY Timeout:                                                        │
│  1. Update RemediationRequest.status.phase = "Timeout"                  │
│  2. Create NotificationRequest for operator alert                       │
│  3. Stop reconciliation (terminal state)                                │
│                                                                         │
└─────────────────────────────────────────────────────────────────────────┘
```

---

## Related Documents

| Document | Relationship |
|----------|--------------|
| **ADR-040** | Approval timeout (15m default) |
| **ADR-017** | NotificationRequest creation by RO |
| **BR-ORCH-027** | Global timeout requirement |
| **BR-ORCH-028** | Phase timeout requirement |

---

## Consequences

### Positive

- ✅ **No stuck remediations**: All remediations eventually terminate
- ✅ **Operator visibility**: Timeout notifications alert operators
- ✅ **Resource cleanup**: Timeouts trigger cleanup of child CRDs
- ✅ **Configurable**: Per-remediation override possible

### Negative

- ⚠️ **False positives**: Legitimate long-running remediations may timeout
  - **Mitigation**: Configurable timeouts, generous defaults
- ⚠️ **Complexity**: Additional timeout tracking logic
  - **Mitigation**: Centralized TimeoutChecker

---

## Changelog

| Version | Date | Changes |
|---------|------|---------|
| 1.0 | 2025-11-28 | Initial DD: Global and phase timeout strategy |

