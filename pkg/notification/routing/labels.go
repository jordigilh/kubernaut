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

