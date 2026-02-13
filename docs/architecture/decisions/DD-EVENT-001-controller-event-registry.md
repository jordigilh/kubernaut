# DD-EVENT-001: Controller Kubernetes Event Registry

**Status**: ✅ APPROVED  
**Decision Date**: 2026-02-09  
**Version**: 1.3  
**Authority Level**: FOUNDATIONAL  
**Applies To**: All CRD controllers (AA, WE, RO, SP, Notification, EM)

---

## Changelog

| Version | Date | Author | Changes |
|---------|------|--------|---------|
| 1.0 | 2026-02-09 | AI Assistant | Initial registry: 11 implemented events, migration pattern |
| 1.1 | 2026-02-05 | AI Assistant | Full coverage: P1-P4 gap analysis, 9 new constants, per-controller BRs (BR-*-095) |
| 1.3 | 2026-02-13 | AI Assistant | Removed EventReasonRemediationIneffective; EffectivenessAssessed always Normal (no threshold); DS computes score on demand |
| 1.2 | 2026-02-12 | AI Assistant | Added EM controller: 5 events (3 P1, 1 P2), inline string compliance, BR-EM-095 |

---

## Context & Problem

Kubernaut controllers emit Kubernetes Events via `record.EventRecorder` for operator observability. However:

1. **No centralized registry**: Event reasons are inline string literals scattered across controllers
2. **Inconsistent type references**: WE uses `"Normal"` / `"Warning"` strings; Notification and SP use `corev1.EventTypeNormal` / `corev1.EventTypeWarning`
3. **Incomplete implementation**: Design docs planned ~20 events; only 11 are implemented across all controllers
4. **No constants**: The testing guidelines (TESTING_GUIDELINES.md line 1728) proposed `pkg/shared/events/reasons.go` with `EventReasonXxx` constants, but this was never implemented
5. **RO has zero events**: Despite having `EventRecorder` wired, the RemediationOrchestrator emits no K8s events

### v1.1 Gap Analysis (2026-02-05)

A comprehensive triage of all 5 controllers revealed:

| Controller | Events Emitted | Total Lifecycle Points | Coverage |
|---|---|---|---|
| **WorkflowExecution** | 7 | ~15 | ~47% |
| **Notification** | 2 | ~16 | ~12% |
| **SignalProcessing** | 1 | ~14 | ~7% |
| **AIAnalysis** | 1 | ~18 | ~6% |
| **EffectivenessMonitor** | 5 | ~10 | ~50% |
| **RemediationOrchestrator** | 0 | ~25+ | 0% |

Gaps were classified into 4 priority tiers:

- **P1 (Terminal States)**: Success/failure completion invisible on `kubectl describe`
- **P2 (Decision Points)**: Branching decisions that change CRD trajectory (approval, escalation, blocking)
- **P3 (Intermediate Transitions)**: Phase-to-phase breadcrumb trail for debugging
- **P4 (Error Paths)**: Transient error conditions useful for diagnosing stuck CRDs

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

### 4. Priority Classification (v1.1)

| Priority | Description | Operator Impact |
|----------|-------------|-----------------|
| **P1** | Terminal state events (Completed, Failed, TimedOut) | `kubectl describe` shows outcome |
| **P2** | Decision point events (approval, escalation, blocking) | Explains *why* CRD is in current state |
| **P3** | Intermediate phase transitions | Breadcrumb trail for debugging |
| **P4** | Error path warnings (degraded, cleanup failure) | Diagnoses stuck CRDs |

### 5. Business Requirements (v1.1)

Each controller has a dedicated K8s Event Observability BR:

| BR ID | Controller | Description |
|-------|------------|-------------|
| BR-AA-095 | AIAnalysis | K8s Event Observability (DD-EVENT-001) |
| BR-WE-095 | WorkflowExecution | K8s Event Observability (DD-EVENT-001) |
| BR-SP-095 | SignalProcessing | K8s Event Observability (DD-EVENT-001) |
| BR-NT-095 | Notification | K8s Event Observability (DD-EVENT-001) |
| BR-ORCH-095 | RemediationOrchestrator | K8s Event Observability (DD-EVENT-001) |
| BR-EM-095 | EffectivenessMonitor | K8s Event Observability (DD-EVENT-001) |

Events tied to existing BRs (e.g., session events under BR-AA-HAPI-064) use the existing BR number for test IDs.

---

## Event Registry

### AIAnalysis Controller

| Reason Constant | Reason String | Type | Priority | When Emitted | Status |
|----------------|---------------|------|----------|-------------|--------|
| `EventReasonAIAnalysisCreated` | `AIAnalysisCreated` | Normal | P1 | Pending → Investigating transition | Implemented (v1.0) |
| `EventReasonInvestigationComplete` | `InvestigationComplete` | Normal | P1 | Investigation phase succeeded, transitioning to Analyzing | Planned (v1.1) |
| `EventReasonAnalysisCompleted` | `AnalysisCompleted` | Normal | P1 | Analysis completed successfully (terminal) | Planned (v1.1) |
| `EventReasonAnalysisFailed` | `AnalysisFailed` | Warning | P1 | Analysis failed (terminal) | Planned (v1.1) |
| `EventReasonApprovalRequired` | `ApprovalRequired` | Normal | P2 | Human approval required for workflow execution | Planned (v1.1) |
| `EventReasonHumanReviewRequired` | `HumanReviewRequired` | Warning | P2 | HAPI flagged investigation for human review | Planned (v1.1) |
| `EventReasonSessionCreated` | `SessionCreated` | Normal | P2 | HAPI investigation session submitted (issue #64) | Implemented (v1.1) |
| `EventReasonSessionLost` | `SessionLost` | Warning | P2 | HAPI session lost (404 on poll), regenerating | Implemented (v1.1) |
| `EventReasonSessionRegenerationExceeded` | `SessionRegenerationExceeded` | Warning | P2 | Max session regenerations (5) exceeded, transitioning to Failed | Implemented (v1.1) |
| `EventReasonPhaseTransition` | `PhaseTransition` | Normal | P3 | Any intermediate phase transition (shared constant) | Planned (v1.1) |

**Note**: Session events (SessionCreated, SessionLost, SessionRegenerationExceeded) were originally delegated to the Issue #64 team but have been implemented by the current team via `WithRecorder` functional option on `InvestigatingHandler`. See BR-AA-HAPI-064.6 and `docs/testing/BR-AA-HAPI-064/session_based_pull_test_plan_v1.0.md`.

### WorkflowExecution Controller

| Reason Constant | Reason String | Type | Priority | When Emitted | Status |
|----------------|---------------|------|----------|-------------|--------|
| `EventReasonExecutionCreated` | `ExecutionCreated` | Normal | P1 | Execution resource (Job/PipelineRun) created | Implemented (v1.0) |
| `EventReasonWorkflowCompleted` | `WorkflowCompleted` | Normal | P1 | Workflow execution succeeded | Implemented (v1.0) |
| `EventReasonWorkflowFailed` | `WorkflowFailed` | Warning | P1 | Workflow execution failed | Implemented (v1.0, 2 sites) |
| `EventReasonLockReleased` | `LockReleased` | Normal | P1 | Target resource lock released after cooldown | Implemented (v1.0) |
| `EventReasonWorkflowExecutionDeleted` | `WorkflowExecutionDeleted` | Normal | P1 | WFE cleanup completed (finalizer) | Implemented (v1.0) |
| `EventReasonPipelineRunCreated` | `PipelineRunCreated` | Normal | P1 | PipelineRun created or adopted | Implemented (v1.0) |
| `EventReasonWorkflowValidated` | `WorkflowValidated` | Normal | P2 | Workflow spec validated successfully | Planned (v1.1) |
| `EventReasonWorkflowValidationFailed` | `WorkflowValidationFailed` | Warning | P2 | Workflow spec validation failed | Planned (v1.1) |
| `EventReasonCooldownActive` | `CooldownActive` | Normal | P2 | Execution blocked by active cooldown on target resource | Planned (v1.1) |
| `EventReasonCleanupFailed` | `CleanupFailed` | Warning | P4 | Execution resource cleanup failed during terminal/delete | Planned (v1.1) |
| `EventReasonPhaseTransition` | `PhaseTransition` | Normal | P3 | Any intermediate phase transition (shared constant) | Planned (v1.1) |

### RemediationOrchestrator Controller

| Reason Constant | Reason String | Type | Priority | When Emitted | Status |
|----------------|---------------|------|----------|-------------|--------|
| `EventReasonRemediationCreated` | `RemediationCreated` | Normal | P1 | New RemediationRequest accepted | Planned (v1.1) |
| `EventReasonRemediationCompleted` | `RemediationCompleted` | Normal | P1 | Remediation lifecycle completed successfully | Planned (v1.1) |
| `EventReasonRemediationFailed` | `RemediationFailed` | Warning | P1 | Remediation lifecycle failed | Planned (v1.1) |
| `EventReasonRemediationTimeout` | `RemediationTimeout` | Warning | P1 | Remediation exceeded timeout | Planned (v1.1) |
| `EventReasonApprovalRequired` | `ApprovalRequired` | Normal | P2 | Human approval required, RAR created | Planned (v1.1) |
| `EventReasonApprovalGranted` | `ApprovalGranted` | Normal | P2 | RAR approved, transitioning to Executing | Planned (v1.1) |
| `EventReasonApprovalRejected` | `ApprovalRejected` | Warning | P2 | RAR rejected, transitioning to Failed | Planned (v1.1) |
| `EventReasonApprovalExpired` | `ApprovalExpired` | Warning | P2 | RAR expired past deadline | Planned (v1.1) |
| `EventReasonEscalatedToManualReview` | `EscalatedToManualReview` | Warning | P2 | Unrecoverable failure, escalation notification sent | Planned (v1.1) |
| ~~`EventReasonRecoveryInitiated`~~ | ~~`RecoveryInitiated`~~ | Normal | P2 | Recovery attempt started after failed remediation | **Deferred to DD-RECOVERY-002** (no recovery path implemented) |
| `EventReasonNotificationCreated` | `NotificationCreated` | Normal | P2 | NotificationRequest CRD created | Planned (v1.1) |
| `EventReasonCooldownActive` | `CooldownActive` | Normal | P2 | Remediation skipped due to active cooldown (shared constant) | Planned (v1.1) |
| `EventReasonConsecutiveFailureBlocked` | `ConsecutiveFailureBlocked` | Warning | P2 | Target resource blocked due to consecutive failures | Planned (v1.1) |
| `EventReasonPhaseTransition` | `PhaseTransition` | Normal | P3 | Any intermediate phase transition (shared constant) | Planned (v1.1) |

### SignalProcessing Controller

| Reason Constant | Reason String | Type | Priority | When Emitted | Status |
|----------------|---------------|------|----------|-------------|--------|
| `EventReasonPolicyEvaluationFailed` | `PolicyEvaluationFailed` | Warning | P2 | Rego policy failed to map severity | Implemented (v1.0) |
| `EventReasonSignalProcessed` | `SignalProcessed` | Normal | P1 | Signal enrichment and classification completed (terminal) | Planned (v1.1) |
| `EventReasonSignalEnriched` | `SignalEnriched` | Normal | P2 | K8s context enrichment completed | Planned (v1.1) |
| `EventReasonEnrichmentDegraded` | `EnrichmentDegraded` | Warning | P4 | K8s enrichment returned partial/degraded results | Planned (v1.1) |
| `EventReasonPhaseTransition` | `PhaseTransition` | Normal | P3 | Any intermediate phase transition (shared constant) | Planned (v1.1) |

### Notification Controller

| Reason Constant | Reason String | Type | Priority | When Emitted | Status |
|----------------|---------------|------|----------|-------------|--------|
| `EventReasonReconcileStarted` | `ReconcileStarted` | Normal | P3 | Notification reconciliation started | Implemented (v1.0) |
| `EventReasonPhaseTransition` | `PhaseTransition` | Normal | P3 | Phase transition (currently: to Sending) | Implemented (v1.0) |
| `EventReasonNotificationSent` | `NotificationSent` | Normal | P1 | Notification delivered successfully to all channels | Planned (v1.1) |
| `EventReasonNotificationFailed` | `NotificationFailed` | Warning | P1 | Notification delivery failed permanently | Planned (v1.1) |
| `EventReasonNotificationPartiallySent` | `NotificationPartiallySent` | Normal | P1 | Some channels succeeded, others failed terminally | Planned (v1.1) |
| `EventReasonNotificationRetrying` | `NotificationRetrying` | Normal | P3 | Retrying after transient delivery failure | Planned (v1.1) |
| `EventReasonCircuitBreakerOpen` | `CircuitBreakerOpen` | Warning | P2 | Slack circuit breaker tripped, channel temporarily disabled | Planned (v1.1) |

### EffectivenessMonitor Controller

| Reason Constant | Reason String | Type | Priority | When Emitted | Status |
|----------------|---------------|------|----------|-------------|--------|
| `EventReasonAssessmentStarted` | `AssessmentStarted` | Normal | P1 | EA transitions Pending → Assessing | Implemented (v1.2) |
| `EventReasonEffectivenessAssessed` | `EffectivenessAssessed` | Normal | P1 | Assessment completed; always Normal (no threshold comparison; DS computes score on demand) | Implemented (v1.2) |
| `EventReasonAssessmentExpired` | `AssessmentExpired` | Warning | P1 | Validity window expired (ADR-EM-001) | Implemented (v1.2) |
| `EventReasonComponentAssessed` | `ComponentAssessed` | Normal/Warning | P2 | Individual component (health/alert/metrics/hash) assessed; component name in message | Implemented (v1.2) |
| `EventReasonPhaseTransition` | `PhaseTransition` | Normal | P3 | Any intermediate phase transition (shared constant) | Planned |

### Shared Events (used by multiple controllers)

| Reason Constant | Reason String | Type | Priority | When Emitted | Status |
|----------------|---------------|------|----------|-------------|--------|
| `EventReasonPhaseTransition` | `PhaseTransition` | Normal | P3 | Any CRD phase transition (generic breadcrumb) | Implemented (v1.0, NT only); Planned for all |

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
    EventReasonCooldownActive                = "CooldownActive"
    EventReasonCleanupFailed                 = "CleanupFailed"
)

// ============================================================
// RemediationOrchestrator Controller Events
// ============================================================

const (
    EventReasonRemediationCreated            = "RemediationCreated"
    EventReasonRemediationCompleted          = "RemediationCompleted"
    EventReasonRemediationFailed             = "RemediationFailed"
    EventReasonRemediationTimeout            = "RemediationTimeout"
    // EventReasonRecoveryInitiated removed — deferred to DD-RECOVERY-002
    EventReasonEscalatedToManualReview       = "EscalatedToManualReview"
    EventReasonNotificationCreated           = "NotificationCreated"
    EventReasonApprovalGranted               = "ApprovalGranted"
    EventReasonApprovalRejected              = "ApprovalRejected"
    EventReasonApprovalExpired               = "ApprovalExpired"
    EventReasonConsecutiveFailureBlocked     = "ConsecutiveFailureBlocked"
)

// ============================================================
// SignalProcessing Controller Events
// ============================================================

const (
    EventReasonPolicyEvaluationFailed        = "PolicyEvaluationFailed"
    EventReasonSignalProcessed               = "SignalProcessed"
    EventReasonSignalEnriched                = "SignalEnriched"
    EventReasonEnrichmentDegraded            = "EnrichmentDegraded"
)

// ============================================================
// Notification Controller Events
// ============================================================

const (
    EventReasonReconcileStarted              = "ReconcileStarted"
    EventReasonNotificationSent              = "NotificationSent"
    EventReasonNotificationFailed            = "NotificationFailed"
    EventReasonNotificationPartiallySent     = "NotificationPartiallySent"
    EventReasonNotificationRetrying          = "NotificationRetrying"
    EventReasonCircuitBreakerOpen            = "CircuitBreakerOpen"
)

// ============================================================
// Effectiveness Monitor Controller Events
// ============================================================

const (
    EventReasonAssessmentStarted             = "AssessmentStarted"
    EventReasonEffectivenessAssessed         = "EffectivenessAssessed"
    EventReasonAssessmentExpired             = "AssessmentExpired"
    EventReasonComponentAssessed             = "ComponentAssessed"
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

// P3: Intermediate phase transition (shared constant, message distinguishes)
r.Recorder.Event(obj, corev1.EventTypeNormal, events.EventReasonPhaseTransition,
    fmt.Sprintf("Phase transition: %s → %s", oldPhase, newPhase))
```

---

## Migration Strategy

### v1.0 Migration (COMPLETED)

1. Created `pkg/shared/events/reasons.go` with initial constants
2. Each controller migrated inline strings to constants:
   - **AA**: Issue #69 -- replaced 1 inline event
   - **WE**: Issue #65 -- replaced 7 inline events + fixed raw type strings
   - **SP**: Issue #67 -- replaced 1 inline string
   - **Notification**: Issue #68 -- replaced 2 inline strings
3. All controllers now use `corev1.EventTypeNormal`/`corev1.EventTypeWarning`

### v1.1 Implementation (IN PROGRESS)

Per-controller issues with TDD methodology (RED-GREEN-REFACTOR):

| Controller | Issue | Events to Add | BR | Owner |
|---|---|---|---|---|
| AA | #72 | 6 (excl. 3 session events) | BR-AA-095 | Current team |
| AA (session) | #73 | 3 (SessionCreated, SessionLost, SessionRegenerationExceeded) | BR-AA-HAPI-064.6 | Current team (originally delegated to Issue #64 team) |
| WE | TBD | 5 | BR-WE-095 | Current team |
| SP | TBD | 5 | BR-SP-095 | Current team |
| NT | TBD | 6 | BR-NT-095 | Current team |
| RO | TBD | 14 (+ FakeRecorder infra) | BR-ORCH-095 | Current team |
| EM | N/A | 5 (all implemented v1.2) | BR-EM-095 | Current team |

### Test Strategy: Defense-in-Depth

Each event is tested at minimum 2 tiers:

| Tier | What | How | Assert |
|---|---|---|---|
| **Unit** | Individual event emission | `record.NewFakeRecorder(N)` injected into reconciler | `recorder.Events` channel: type + reason + message |
| **Integration** | Business flow event sequence | envtest with real EventRecorder | `corev1.EventList` by `involvedObject.name`: ordered trail |

Test IDs follow DD-TEST-006 convention: `{TestType}-{ServiceCode}-{BR#}-{Sequence}` (e.g., `UT-AA-095-01`, `IT-RO-095-01`).

---

## Consequences

### Positive

- Single source of truth for all K8s Event reasons
- Compile-time safety (typos in reason strings caught at build time)
- Consistent type references across all controllers
- Easy to audit which events each controller emits
- Grep-friendly: `EventReason` prefix makes all events discoverable
- Full terminal-state visibility via `kubectl describe` for every CRD (v1.1)
- Decision-point observability for troubleshooting (v1.1)

### Negative

- Initial migration effort to replace inline strings in 5 controllers
- Shared package creates a mild coupling between controllers (acceptable for constants)
- P3 events use shared `PhaseTransition` reason; message content required to distinguish transitions

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

### D. Per-Transition Reason Constants for P3 (v1.1)

Define unique constants for each intermediate transition (e.g., `EventReasonPendingToEnriching`).

**Rejected**: Creates constant explosion with minimal operator value. Shared `PhaseTransition` with descriptive messages is sufficient.

---

## Related Decisions

- **TESTING_GUIDELINES.md** (line 1728): Originally proposed this pattern (`pkg/shared/events/reasons.go`)
- **ADR-001**: CRD microservices architecture mandates K8s events for observability
- **DD-CONTROLLER-001**: Observed generation idempotency pattern (related controller design)
- **DD-TEST-006**: Test plan policy and test ID convention (`{TestType}-{ServiceCode}-{BR#}-{Sequence}`)
- **Issue #64**: AA-HAPI session-based pull design (owns session event implementation)
- **BR-AA-HAPI-064.6**: Session regeneration cap event (test plan: `docs/testing/BR-AA-HAPI-064/`)

## Test Plans

- `docs/testing/DD-EVENT-001/TP-EVENT-AA.md` -- AIAnalysis event test plan
- `docs/testing/DD-EVENT-001/TP-EVENT-WE.md` -- WorkflowExecution event test plan
- `docs/testing/DD-EVENT-001/TP-EVENT-SP.md` -- SignalProcessing event test plan
- `docs/testing/DD-EVENT-001/TP-EVENT-NT.md` -- Notification event test plan
- `docs/testing/DD-EVENT-001/TP-EVENT-RO.md` -- RemediationOrchestrator event test plan
- `docs/services/crd-controllers/07-effectivenessmonitor/EM_COMPREHENSIVE_TEST_PLAN.md` -- EffectivenessMonitor test plan (includes KE scenarios)
