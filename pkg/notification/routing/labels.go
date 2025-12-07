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

package routing

// Standard Kubernetes label keys for notification routing.
// BR-NOT-065: Channel Routing Based on Labels
//
// IMPORTANT: All labels use the `kubernaut.ai/` domain, NOT `kubernaut.io/`.
// This is consistent with:
//   - API groups: signalprocessing.kubernaut.ai/v1alpha1
//   - Existing labels: kubernaut.ai/workflow-execution
//   - Finalizers: workflowexecution.kubernaut.ai/finalizer
//
// See: docs/handoff/NOTICE_LABEL_DOMAIN_AND_NOTIFICATION_ROUTING.md
const (
	// LabelNotificationType is the label key for notification type routing.
	// Values: approval_required, completed, failed, escalation, status_update
	LabelNotificationType = "kubernaut.ai/notification-type"

	// LabelSeverity is the label key for severity-based routing.
	// Values: critical, high, medium, low
	LabelSeverity = "kubernaut.ai/severity"

	// LabelEnvironment is the label key for environment-based routing.
	// Values: production, staging, development, test
	LabelEnvironment = "kubernaut.ai/environment"

	// LabelPriority is the label key for priority-based routing.
	// Values: P0, P1, P2, P3 (or critical, high, medium, low)
	LabelPriority = "kubernaut.ai/priority"

	// LabelComponent is the label key identifying the source component.
	// Values: remediation-orchestrator, workflow-execution, signal-processing, etc.
	LabelComponent = "kubernaut.ai/component"

	// LabelRemediationRequest is the label key for linking to the parent remediation.
	// Value: name of the RemediationRequest CRD
	LabelRemediationRequest = "kubernaut.ai/remediation-request"

	// LabelNamespace is the label key for namespace-based routing.
	// Value: Kubernetes namespace name
	LabelNamespace = "kubernaut.ai/namespace"

	// LabelSkipReason is the label key for WFE skip reason-based routing.
	// Enables fine-grained routing based on why a WorkflowExecution was skipped.
	// Values: PreviousExecutionFailed, ExhaustedRetries, ResourceBusy, RecentlyRemediated
	// See: docs/handoff/NOTICE_WE_EXPONENTIAL_BACKOFF_DD_WE_004.md
	// Added per cross-team agreement: WE→NOT Q7 (2025-12-06)
	LabelSkipReason = "kubernaut.ai/skip-reason"

	// LabelInvestigationOutcome is the label key for HolmesGPT-API investigation outcome routing.
	// Enables routing based on how an investigation concluded before workflow selection.
	// Values: resolved, inconclusive, workflow_selected
	// See: docs/handoff/NOTICE_INVESTIGATION_INCONCLUSIVE_BR_HAPI_200.md
	// Added per BR-HAPI-200: Investigation Outcome Reporting (2025-12-07)
	LabelInvestigationOutcome = "kubernaut.ai/investigation-outcome"
)

// NotificationTypeValues are the standard notification type label values.
const (
	NotificationTypeApprovalRequired = "approval_required"
	NotificationTypeCompleted        = "completed"
	NotificationTypeFailed           = "failed"
	NotificationTypeEscalation       = "escalation"
	NotificationTypeStatusUpdate     = "status_update"
)

// SeverityValues are the standard severity label values.
const (
	SeverityCritical = "critical"
	SeverityHigh     = "high"
	SeverityMedium   = "medium"
	SeverityLow      = "low"
)

// EnvironmentValues are the standard environment label values.
const (
	EnvironmentProduction  = "production"
	EnvironmentStaging     = "staging"
	EnvironmentDevelopment = "development"
	EnvironmentTest        = "test"
)

// SkipReasonValues are the standard WorkflowExecution skip reason label values.
// These map to WorkflowExecution.Status.SkipDetails.Reason values.
// See: DD-WE-004 v1.1 (Exponential Backoff Cooldown)
const (
	// SkipReasonPreviousExecutionFailed indicates a workflow ran and failed.
	// Cluster state is unknown/partially modified - manual intervention required.
	// Severity: CRITICAL - route to PagerDuty or high-priority channels.
	SkipReasonPreviousExecutionFailed = "PreviousExecutionFailed"

	// SkipReasonExhaustedRetries indicates 5+ pre-execution failures.
	// Infrastructure issues persisting - manual intervention required.
	// Severity: HIGH - route to Slack or email for team awareness.
	SkipReasonExhaustedRetries = "ExhaustedRetries"

	// SkipReasonResourceBusy indicates another WFE is running on target.
	// Temporary condition - will auto-resolve.
	// Severity: LOW - typically bulk notifications (BR-ORCH-034).
	SkipReasonResourceBusy = "ResourceBusy"

	// SkipReasonRecentlyRemediated indicates cooldown/backoff is active.
	// Temporary condition - will auto-resolve after cooldown expires.
	// Severity: LOW - typically bulk notifications (BR-ORCH-034).
	SkipReasonRecentlyRemediated = "RecentlyRemediated"
)

// InvestigationOutcomeValues are the HolmesGPT-API investigation outcome label values.
// These map to the investigation conclusion before workflow selection.
// See: BR-HAPI-200 (Investigation Outcome Reporting)
const (
	// InvestigationOutcomeResolved indicates the alert resolved during investigation.
	// The problem self-corrected before any remediation was needed.
	// Action: Skip notification - no human action required (alert fatigue prevention).
	InvestigationOutcomeResolved = "resolved"

	// InvestigationOutcomeInconclusive indicates LLM could not determine root cause.
	// Analysis completed but no confident diagnosis was reached.
	// Action: Route to ops channel (Slack #ops) for human review.
	// Severity: MEDIUM - requires human attention but not critical.
	InvestigationOutcomeInconclusive = "inconclusive"

	// InvestigationOutcomeWorkflowSelected indicates normal workflow selection.
	// Investigation successfully identified root cause and selected a workflow.
	// Action: Standard routing based on other labels (severity, environment, etc.)
	InvestigationOutcomeWorkflowSelected = "workflow_selected"
)

