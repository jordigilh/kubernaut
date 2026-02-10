/*
Copyright 2025 Jordi Gil.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

// Package events defines the authoritative Kubernetes Event reason constants
// for all Kubernaut CRD controllers.
//
// DD-EVENT-001: Controller Kubernetes Event Registry
//
// All controllers MUST use constants from this package instead of inline strings.
// All events MUST use corev1.EventTypeNormal or corev1.EventTypeWarning (not raw strings).
//
// Naming convention:
//   - Constant: EventReason + PascalCase reason (e.g., EventReasonWorkflowCompleted)
//   - Reason string: PascalCase, verb-noun pattern (e.g., "WorkflowCompleted")
//   - Warning events: failures, errors, conditions requiring operator attention
//   - Normal events: phase transitions, successful completions, informational lifecycle
package events

// ============================================================
// AIAnalysis Controller Events
// ============================================================

const (
	// EventReasonAIAnalysisCreated is emitted when an AIAnalysis transitions
	// from Pending to Investigating (processing started).
	EventReasonAIAnalysisCreated = "AIAnalysisCreated"

	// EventReasonInvestigationComplete is emitted when the investigation phase
	// succeeds and the AIAnalysis transitions to Analyzing.
	EventReasonInvestigationComplete = "InvestigationComplete"

	// EventReasonAnalysisCompleted is emitted when analysis completes successfully
	// (terminal state).
	EventReasonAnalysisCompleted = "AnalysisCompleted"

	// EventReasonAnalysisFailed is emitted when analysis fails (terminal state).
	// Type: Warning
	EventReasonAnalysisFailed = "AnalysisFailed"

	// EventReasonSessionCreated is emitted when a HAPI investigation session
	// is submitted and accepted (issue #64: session-based pull design).
	EventReasonSessionCreated = "SessionCreated"

	// EventReasonSessionLost is emitted when a HAPI session is lost (404 on poll,
	// typically due to HAPI restart) and regeneration is attempted.
	// Type: Warning
	EventReasonSessionLost = "SessionLost"

	// EventReasonSessionRegenerationExceeded is emitted when the maximum number
	// of session regenerations (MaxSessionRegenerations) is exceeded, causing
	// the AIAnalysis to transition to Failed with escalation notification.
	// Type: Warning
	EventReasonSessionRegenerationExceeded = "SessionRegenerationExceeded"

	// EventReasonApprovalRequired is emitted when human approval is required
	// for workflow execution (low confidence or policy mandate).
	EventReasonApprovalRequired = "ApprovalRequired"

	// EventReasonHumanReviewRequired is emitted when HAPI flags the investigation
	// for human review (needs_human_review=true).
	// Type: Warning
	EventReasonHumanReviewRequired = "HumanReviewRequired"
)

// ============================================================
// WorkflowExecution Controller Events
// ============================================================

const (
	// EventReasonExecutionCreated is emitted when the execution resource
	// (Job or PipelineRun) is created in the target namespace.
	EventReasonExecutionCreated = "ExecutionCreated"

	// EventReasonLockReleased is emitted when the target resource lock
	// is released after the cooldown period.
	EventReasonLockReleased = "LockReleased"

	// EventReasonWorkflowExecutionDeleted is emitted when WorkflowExecution
	// cleanup completes (finalizer processing).
	EventReasonWorkflowExecutionDeleted = "WorkflowExecutionDeleted"

	// EventReasonPipelineRunCreated is emitted when a PipelineRun is created
	// or an existing one is adopted.
	EventReasonPipelineRunCreated = "PipelineRunCreated"

	// EventReasonWorkflowCompleted is emitted when the workflow execution
	// succeeds (Job/PipelineRun completed successfully).
	EventReasonWorkflowCompleted = "WorkflowCompleted"

	// EventReasonWorkflowFailed is emitted when the workflow execution fails.
	// Type: Warning
	EventReasonWorkflowFailed = "WorkflowFailed"

	// EventReasonWorkflowValidated is emitted when the workflow spec passes
	// pre-execution validation.
	EventReasonWorkflowValidated = "WorkflowValidated"

	// EventReasonWorkflowValidationFailed is emitted when the workflow spec
	// fails pre-execution validation.
	// Type: Warning
	EventReasonWorkflowValidationFailed = "WorkflowValidationFailed"
)

// ============================================================
// RemediationOrchestrator Controller Events
// ============================================================

const (
	// EventReasonRemediationCreated is emitted when a new RemediationRequest
	// is accepted by the orchestrator.
	EventReasonRemediationCreated = "RemediationCreated"

	// EventReasonRemediationCompleted is emitted when the remediation lifecycle
	// completes successfully (terminal state).
	EventReasonRemediationCompleted = "RemediationCompleted"

	// EventReasonRemediationFailed is emitted when the remediation lifecycle fails.
	// Type: Warning
	EventReasonRemediationFailed = "RemediationFailed"

	// EventReasonRemediationTimeout is emitted when the remediation exceeds its
	// configured timeout.
	// Type: Warning
	EventReasonRemediationTimeout = "RemediationTimeout"

	// EventReasonRecoveryInitiated is emitted when a recovery attempt is started
	// after a failed remediation.
	EventReasonRecoveryInitiated = "RecoveryInitiated"

	// EventReasonEscalatedToManualReview is emitted when an unrecoverable failure
	// triggers escalation notification to operators.
	// Type: Warning
	EventReasonEscalatedToManualReview = "EscalatedToManualReview"

	// EventReasonNotificationCreated is emitted when a NotificationRequest CRD
	// is created by the orchestrator.
	EventReasonNotificationCreated = "NotificationCreated"

	// EventReasonCooldownActive is emitted when a remediation is skipped because
	// the target resource is under active cooldown.
	EventReasonCooldownActive = "CooldownActive"
)

// ============================================================
// SignalProcessing Controller Events
// ============================================================

const (
	// EventReasonPolicyEvaluationFailed is emitted when a Rego policy fails
	// to map the external severity.
	// Type: Warning
	EventReasonPolicyEvaluationFailed = "PolicyEvaluationFailed"

	// EventReasonSignalProcessed is emitted when signal enrichment and
	// classification complete successfully.
	EventReasonSignalProcessed = "SignalProcessed"

	// EventReasonSignalEnriched is emitted when K8s context enrichment
	// completes for a signal.
	EventReasonSignalEnriched = "SignalEnriched"
)

// ============================================================
// Notification Controller Events
// ============================================================

const (
	// EventReasonReconcileStarted is emitted when notification reconciliation begins.
	EventReasonReconcileStarted = "ReconcileStarted"

	// EventReasonNotificationSent is emitted when a notification is delivered
	// successfully to the target channel.
	EventReasonNotificationSent = "NotificationSent"

	// EventReasonNotificationFailed is emitted when notification delivery fails.
	// Type: Warning
	EventReasonNotificationFailed = "NotificationFailed"
)

// ============================================================
// Shared Events (used by multiple controllers)
// ============================================================

const (
	// EventReasonPhaseTransition is emitted on any CRD phase transition.
	// Used by all controllers for generic phase change observability.
	EventReasonPhaseTransition = "PhaseTransition"
)
