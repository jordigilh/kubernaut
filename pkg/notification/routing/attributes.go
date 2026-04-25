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

// Routing attribute keys for notification routing.
// Issue #91: Migrated from mutable Kubernetes labels to immutable spec fields.
// BR-NOT-065: Channel Routing Based on Spec Fields
//
// These keys correspond to spec field names (type, severity, phase, reviewSource,
// priority) and spec.context / spec.extensions keys (skip-reason, investigation-outcome, environment).
// Used in routing config match entries and RoutingAttributesFromSpec().
const (
	// AttrType is the routing attribute key for notification type.
	// Maps to spec.type. Values: approval, completion, escalation, simple, manual-review, status-update
	AttrType = "type"

	// AttrSeverity is the routing attribute key for severity.
	// Maps to spec.severity. Values: critical, high, medium, low
	AttrSeverity = "severity"

	// AttrEnvironment is the routing attribute key for environment.
	// Maps to spec.extensions["environment"]. Values: production, staging, development, test
	AttrEnvironment = "environment"

	// AttrPhase is the routing attribute key for remediation phase.
	// Maps to spec.phase. Values: signal-processing, ai-analysis, workflow-execution, etc.
	AttrPhase = "phase"

	// AttrReviewSource is the routing attribute key for manual review source.
	// Maps to spec.reviewSource. Values: WorkflowResolutionFailed, ExhaustedRetries, etc.
	AttrReviewSource = "review-source"

	// AttrPriority is the routing attribute key for priority.
	// Maps to spec.priority. Values: P0, P1, P2, P3 (or critical, high, medium, low)
	AttrPriority = "priority"

	// AttrNamespace is the routing attribute key for namespace.
	// Maps to spec.extensions["namespace"]. Value: Kubernetes namespace name
	AttrNamespace = "namespace"

	// AttrSkipReason is the routing attribute key for WFE skip reason.
	// Maps to spec.extensions["skip-reason"].
	// Values: PreviousExecutionFailed, ExhaustedRetries, ResourceBusy, RecentlyRemediated
	AttrSkipReason = "skip-reason"

	// AttrInvestigationOutcome is the routing attribute key for investigation outcome.
	// Maps to spec.extensions["investigation-outcome"].
	// Values: resolved, inconclusive, workflow_selected
	AttrInvestigationOutcome = "investigation-outcome"

	// AttrNotificationTarget is the routing attribute key for notification target scope.
	// Maps to spec.extensions["notification-target"].
	// Values: signal, rca, both
	// Issue #416: Label-based notification routing
	AttrNotificationTarget = "notification-target"

	// AttrTeam is the routing attribute key for owning team.
	// Maps to spec.extensions["team"], populated from kubernaut.ai/team label.
	// Issue #416: Label-based notification routing
	AttrTeam = "team"

	// AttrOwner is the routing attribute key for resource owner.
	// Maps to spec.extensions["owner"], populated from kubernaut.ai/owner label.
	// Issue #416: Label-based notification routing
	AttrOwner = "owner"

	// AttrTargetKind is the routing attribute key for target resource kind.
	// Maps to spec.extensions["target-kind"].
	// Issue #416: Label-based notification routing
	AttrTargetKind = "target-kind"
)

// NotificationTypeValues are the standard notification type routing values.
const (
	NotificationTypeApprovalRequired = "approval_required"
	NotificationTypeCompleted        = "completed"
	NotificationTypeFailed           = "failed"
	NotificationTypeEscalation       = "Escalation"
	NotificationTypeStatusUpdate     = "status_update"
	// NotificationTypeManualReview indicates manual review is required (BR-ORCH-036).
	NotificationTypeManualReview = "ManualReview"
	// NotificationTypeBulkDuplicate indicates a bulk duplicate notification (BR-ORCH-034).
	NotificationTypeBulkDuplicate = "bulk-duplicate"
)

// SeverityValues are the standard severity routing values.
const (
	SeverityCritical = "critical"
	SeverityHigh     = "high"
	SeverityMedium   = "medium"
	SeverityLow      = "low"
)

// EnvironmentValues are the standard environment routing values.
const (
	EnvironmentProduction  = "production"
	EnvironmentStaging     = "staging"
	EnvironmentDevelopment = "development"
	EnvironmentTest        = "test"
)

// SkipReasonValues are the standard WorkflowExecution skip reason routing values.
// These map to WorkflowExecution.Status.SkipDetails.Reason values.
// See: DD-WE-004 v1.1 (Exponential Backoff Cooldown)
const (
	SkipReasonPreviousExecutionFailed = "PreviousExecutionFailed"
	SkipReasonExhaustedRetries        = "ExhaustedRetries"
	SkipReasonResourceBusy            = "ResourceBusy"
	SkipReasonRecentlyRemediated      = "RecentlyRemediated"
)

// InvestigationOutcomeValues are the KA investigation outcome routing values.
// See: BR-HAPI-200 (Investigation Outcome Reporting)
const (
	InvestigationOutcomeResolved         = "resolved"
	InvestigationOutcomeInconclusive     = "inconclusive"
	InvestigationOutcomeWorkflowSelected = "workflow_selected"
)
