# DD-EVENT-001: Controller Kubernetes Event Registry

**Status**: ✅ APPROVED  
**Decision Date**: 2026-02-09  
**Version**: 1.0  
**Authority Level**: FOUNDATIONAL  
**Applies To**: All CRD controllers (AA, WE, RO, SP, Notification)

---

## Context & Problem

Kubernaut controllers emit Kubernetes Events via `record.EventRecorder` for operator observability. However:

1. **No centralized registry**: Event reasons are inline string literals scattered across controllers
2. **Inconsistent type references**: WE uses `"Normal"` / `"Warning"` strings; Notification and SP use `corev1.EventTypeNormal` / `corev1.EventTypeWarning`
3. **Incomplete implementation**: Design docs planned ~20 events; only 11 are implemented across all controllers
4. **No constants**: The testing guidelines (TESTING_GUIDELINES.md line 1728) proposed `pkg/shared/events/reasons.go` with `EventReasonXxx` constants, but this was never implemented
5. **RO has zero events**: Despite having `EventRecorder` wired, the RemediationOrchestrator emits no K8s events

This DD establishes the authoritative event registry and implementation pattern.

---

## Decision

### 1. Constants Location

All K8s Event reason constants MUST be defined in a centralized package:

```
pkg/shared/events/reasons.go
```

Per-controller event reasons are grouped by controller in this single file. Controllers import and reference these constants instead of using inline strings.

### 2. Naming Convention

- **Constant name**: `EventReason` prefix + PascalCase reason (e.g., `EventReasonWorkflowCompleted`)
- **Reason string**: PascalCase, verb-noun pattern (e.g., `"WorkflowCompleted"`, `"PhaseTransition"`)
- **Event type**: Always use `corev1.EventTypeNormal` or `corev1.EventTypeWarning` -- never raw strings
- **Warning events**: Reserved for failures, errors, and conditions requiring operator attention
- **Normal events**: Phase transitions, successful completions, informational lifecycle events

### 3. Message Format

- Include the resource name or identifier for correlation
- Use `fmt.Sprintf` for dynamic values
- Keep messages concise (under 256 characters)
- Example: `fmt.Sprintf("Workflow %s completed successfully in %s", workflowID, duration)`

---

## Event Registry

### AIAnalysis Controller

| Reason Constant | Reason String | Type | When Emitted | Status |
|----------------|---------------|------|-------------|--------|
| `EventReasonAIAnalysisCreated` | `AIAnalysisCreated` | Normal | Pending → Investigating transition | Implemented |
| `EventReasonInvestigationComplete` | `InvestigationComplete` | Normal | Investigation phase succeeded, transitioning to Analyzing | Planned |
| `EventReasonAnalysisCompleted` | `AnalysisCompleted` | Normal | Analysis completed successfully (terminal) | Planned |
| `EventReasonAnalysisFailed` | `AnalysisFailed` | Warning | Analysis failed (terminal) | Planned |
| `EventReasonPhaseTransition` | `PhaseTransition` | Normal | Any phase transition (generic) | Planned |
| `EventReasonSessionCreated` | `SessionCreated` | Normal | HAPI investigation session submitted (issue #64) | Planned |
| `EventReasonSessionLost` | `SessionLost` | Warning | HAPI session lost (404 on poll), regenerating | Planned |
| `EventReasonSessionRegenerationExceeded` | `SessionRegenerationExceeded` | Warning | Max session regenerations (5) exceeded, transitioning to Failed | Planned |
| `EventReasonApprovalRequired` | `ApprovalRequired` | Normal | Human approval required for workflow execution | Planned |
| `EventReasonHumanReviewRequired` | `HumanReviewRequired` | Warning | HAPI flagged investigation for human review | Planned |

### WorkflowExecution Controller

| Reason Constant | Reason String | Type | When Emitted | Status |
|----------------|---------------|------|-------------|--------|
| `EventReasonExecutionCreated` | `ExecutionCreated` | Normal | Execution resource (Job/PipelineRun) created | Implemented |
| `EventReasonLockReleased` | `LockReleased` | Normal | Target resource lock released after cooldown | Implemented |
| `EventReasonWorkflowExecutionDeleted` | `WorkflowExecutionDeleted` | Normal | WFE cleanup completed (finalizer) | Implemented |
| `EventReasonPipelineRunCreated` | `PipelineRunCreated` | Normal | PipelineRun created or adopted | Implemented |
| `EventReasonWorkflowCompleted` | `WorkflowCompleted` | Normal | Workflow execution succeeded | Implemented |
| `EventReasonWorkflowFailed` | `WorkflowFailed` | Warning | Workflow execution failed | Implemented (2 sites) |
| `EventReasonPhaseTransition` | `PhaseTransition` | Normal | Any phase transition | Planned |
| `EventReasonWorkflowValidated` | `WorkflowValidated` | Normal | Workflow spec validated successfully | Planned |
| `EventReasonWorkflowValidationFailed` | `WorkflowValidationFailed` | Warning | Workflow spec validation failed | Planned |

### RemediationOrchestrator Controller

| Reason Constant | Reason String | Type | When Emitted | Status |
|----------------|---------------|------|-------------|--------|
| `EventReasonRemediationCreated` | `RemediationCreated` | Normal | New RemediationRequest accepted | Planned |
| `EventReasonRemediationCompleted` | `RemediationCompleted` | Normal | Remediation lifecycle completed successfully | Planned |
| `EventReasonRemediationFailed` | `RemediationFailed` | Warning | Remediation lifecycle failed | Planned |
| `EventReasonRemediationTimeout` | `RemediationTimeout` | Warning | Remediation exceeded timeout | Planned |
| `EventReasonRecoveryInitiated` | `RecoveryInitiated` | Normal | Recovery attempt started after failed remediation | Planned |
| `EventReasonEscalatedToManualReview` | `EscalatedToManualReview` | Warning | Unrecoverable failure, escalation notification sent | Planned |
| `EventReasonNotificationCreated` | `NotificationCreated` | Normal | NotificationRequest CRD created | Planned |
| `EventReasonPhaseTransition` | `PhaseTransition` | Normal | Any phase transition | Planned |
| `EventReasonCooldownActive` | `CooldownActive` | Normal | Remediation skipped due to active cooldown | Planned |

### SignalProcessing Controller

| Reason Constant | Reason String | Type | When Emitted | Status |
|----------------|---------------|------|-------------|--------|
| `EventReasonPolicyEvaluationFailed` | `PolicyEvaluationFailed` | Warning | Rego policy failed to map severity | Implemented |
| `EventReasonSignalProcessed` | `SignalProcessed` | Normal | Signal enrichment and classification completed | Planned |
| `EventReasonSignalEnriched` | `SignalEnriched` | Normal | K8s context enrichment completed | Planned |
| `EventReasonPhaseTransition` | `PhaseTransition` | Normal | Any phase transition | Planned |

### Notification Controller

| Reason Constant | Reason String | Type | When Emitted | Status |
|----------------|---------------|------|-------------|--------|
| `EventReasonReconcileStarted` | `ReconcileStarted` | Normal | Notification reconciliation started | Implemented |
| `EventReasonPhaseTransition` | `PhaseTransition` | Normal | Phase transition (currently: to Sending) | Implemented |
| `EventReasonNotificationSent` | `NotificationSent` | Normal | Notification delivered successfully | Planned |
| `EventReasonNotificationFailed` | `NotificationFailed` | Warning | Notification delivery failed | Planned |

---

## Implementation Pattern

### Constants File

```go
// pkg/shared/events/reasons.go
package events

// ============================================================
// AIAnalysis Controller Events
// ============================================================

const (
    EventReasonAIAnalysisCreated            = "AIAnalysisCreated"
    EventReasonInvestigationComplete         = "InvestigationComplete"
    EventReasonAnalysisCompleted             = "AnalysisCompleted"
    EventReasonAnalysisFailed                = "AnalysisFailed"
    EventReasonSessionCreated                = "SessionCreated"
    EventReasonSessionLost                   = "SessionLost"
    EventReasonSessionRegenerationExceeded   = "SessionRegenerationExceeded"
    EventReasonApprovalRequired              = "ApprovalRequired"
    EventReasonHumanReviewRequired           = "HumanReviewRequired"
)

// ============================================================
// WorkflowExecution Controller Events
// ============================================================

const (
    EventReasonExecutionCreated              = "ExecutionCreated"
    EventReasonLockReleased                  = "LockReleased"
    EventReasonWorkflowExecutionDeleted      = "WorkflowExecutionDeleted"
    EventReasonPipelineRunCreated            = "PipelineRunCreated"
    EventReasonWorkflowCompleted             = "WorkflowCompleted"
    EventReasonWorkflowFailed                = "WorkflowFailed"
    EventReasonWorkflowValidated             = "WorkflowValidated"
    EventReasonWorkflowValidationFailed      = "WorkflowValidationFailed"
)

// ============================================================
// RemediationOrchestrator Controller Events
// ============================================================

const (
    EventReasonRemediationCreated            = "RemediationCreated"
    EventReasonRemediationCompleted          = "RemediationCompleted"
    EventReasonRemediationFailed             = "RemediationFailed"
    EventReasonRemediationTimeout            = "RemediationTimeout"
    EventReasonRecoveryInitiated             = "RecoveryInitiated"
    EventReasonEscalatedToManualReview       = "EscalatedToManualReview"
    EventReasonNotificationCreated           = "NotificationCreated"
    EventReasonCooldownActive                = "CooldownActive"
)

// ============================================================
// SignalProcessing Controller Events
// ============================================================

const (
    EventReasonPolicyEvaluationFailed        = "PolicyEvaluationFailed"
    EventReasonSignalProcessed               = "SignalProcessed"
    EventReasonSignalEnriched                = "SignalEnriched"
)

// ============================================================
// Notification Controller Events
// ============================================================

const (
    EventReasonReconcileStarted              = "ReconcileStarted"
    EventReasonNotificationSent              = "NotificationSent"
    EventReasonNotificationFailed            = "NotificationFailed"
)

// ============================================================
// Shared Events (used by multiple controllers)
// ============================================================

const (
    EventReasonPhaseTransition               = "PhaseTransition"
)
```

### Usage Pattern

```go
import (
    corev1 "k8s.io/api/core/v1"
    "github.com/jordigilh/kubernaut/pkg/shared/events"
)

// Normal event
r.Recorder.Event(obj, corev1.EventTypeNormal, events.EventReasonWorkflowCompleted,
    fmt.Sprintf("Workflow %s completed successfully in %s", workflowID, duration))

// Warning event
r.Recorder.Event(obj, corev1.EventTypeWarning, events.EventReasonSessionRegenerationExceeded,
    fmt.Sprintf("Max session regenerations (%d) exceeded for investigation", maxRegenerations))
```

---

## Migration Strategy

1. Create `pkg/shared/events/reasons.go` with all constants
2. Each controller migrates to constants in its own issue/PR:
   - **AA**: Issue #64 (session-based pull design) -- implements all AA events
   - **WE**: Separate issue -- replaces 7 inline strings with constants, adds planned events
   - **RO**: Separate issue -- implements all 9 planned events
   - **SP**: Separate issue -- replaces 1 inline string, adds planned events
   - **Notification**: Separate issue -- replaces 2 inline strings, adds planned events
3. Each migration PR also fixes inconsistent type references (`"Normal"` → `corev1.EventTypeNormal`)

---

## Consequences

### Positive

- Single source of truth for all K8s Event reasons
- Compile-time safety (typos in reason strings caught at build time)
- Consistent type references across all controllers
- Easy to audit which events each controller emits
- Grep-friendly: `EventReason` prefix makes all events discoverable

### Negative

- Initial migration effort to replace inline strings in 5 controllers
- Shared package creates a mild coupling between controllers (acceptable for constants)

---

## Alternatives Considered

### A. Per-Controller Constants

Each controller defines its own constants (e.g., `internal/controller/aianalysis/events.go`).

**Rejected**: Duplicates shared events like `PhaseTransition`, harder to audit globally.

### B. No Constants (Status Quo)

Continue using inline strings.

**Rejected**: No compile-time safety, inconsistent naming, no discoverability.

### C. Event Registry CRD

Define events in a custom resource.

**Rejected**: Over-engineering for string constants.

---

## Related Decisions

- **TESTING_GUIDELINES.md** (line 1728): Originally proposed this pattern (`pkg/shared/events/reasons.go`)
- **ADR-001**: CRD microservices architecture mandates K8s events for observability
- **DD-CONTROLLER-001**: Observed generation idempotency pattern (related controller design)
- **Issue #64**: AA-HAPI session-based pull design (first consumer of new session events)
