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

package v1alpha1

import (
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// EDIT THIS FILE!  THIS IS SCAFFOLDING FOR YOU TO OWN!
// NOTE: json tags are required.  Any new fields you add must have json tags for the fields to be serialized.

// +kubebuilder:validation:Enum=Escalation;Simple;StatusUpdate;Approval;ManualReview;Completion
type NotificationType string

const (
	NotificationTypeEscalation   NotificationType = "Escalation"
	NotificationTypeSimple       NotificationType = "Simple"
	NotificationTypeStatusUpdate NotificationType = "StatusUpdate"
	// NotificationTypeApproval is used for approval request notifications (BR-ORCH-001)
	// Added Dec 2025 per RO team request for explicit approval workflow support
	NotificationTypeApproval NotificationType = "Approval"
	// NotificationTypeManualReview is used for manual intervention required notifications (BR-ORCH-036)
	// Added Dec 2025 for ExhaustedRetries/PreviousExecutionFailed scenarios requiring operator action
	// Distinct from 'escalation' to enable spec-field-based routing rules (BR-NOT-065)
	NotificationTypeManualReview NotificationType = "ManualReview"
	// NotificationTypeCompletion is used for successful remediation completion notifications (BR-ORCH-045)
	// Created when WorkflowExecution completes successfully and RR transitions to Completed phase
	// Enables operators to track successful autonomous remediations
	NotificationTypeCompletion NotificationType = "Completion"
)

// +kubebuilder:validation:Enum=Critical;High;Medium;Low
type NotificationPriority string

const (
	NotificationPriorityCritical NotificationPriority = "Critical"
	NotificationPriorityHigh     NotificationPriority = "High"
	NotificationPriorityMedium   NotificationPriority = "Medium"
	NotificationPriorityLow      NotificationPriority = "Low"
)

// +kubebuilder:validation:Enum=email;slack;teams;sms;webhook;console;file;log
type Channel string

const (
	ChannelEmail   Channel = "email"
	ChannelSlack   Channel = "slack"
	ChannelTeams   Channel = "teams"
	ChannelSMS     Channel = "sms"
	ChannelWebhook Channel = "webhook"
	ChannelConsole Channel = "console"
	ChannelFile    Channel = "file" // File-based delivery for audit trails and compliance
	ChannelLog     Channel = "log"  // Structured JSON logs to stdout for observability
)

// +kubebuilder:validation:Enum=Pending;Sending;Retrying;Sent;PartiallySent;Failed
type NotificationPhase string

const (
	NotificationPhasePending       NotificationPhase = "Pending"
	NotificationPhaseSending       NotificationPhase = "Sending"
	NotificationPhaseRetrying      NotificationPhase = "Retrying" // Partial failure with retries remaining (non-terminal)
	NotificationPhaseSent          NotificationPhase = "Sent"
	NotificationPhasePartiallySent NotificationPhase = "PartiallySent"
	NotificationPhaseFailed        NotificationPhase = "Failed"
)

// +kubebuilder:validation:Enum=AIAnalysis;WorkflowExecution;RoutingEngine
type ReviewSourceType string

const (
	ReviewSourceAIAnalysis        ReviewSourceType = "AIAnalysis"
	ReviewSourceWorkflowExecution ReviewSourceType = "WorkflowExecution"
	ReviewSourceRoutingEngine     ReviewSourceType = "RoutingEngine"
)

// +kubebuilder:validation:Enum=success;failed;timeout;invalid
type DeliveryAttemptStatus string

const (
	DeliveryAttemptStatusSuccess DeliveryAttemptStatus = "success"
	DeliveryAttemptStatusFailed  DeliveryAttemptStatus = "failed"
	DeliveryAttemptStatusTimeout DeliveryAttemptStatus = "timeout"
	DeliveryAttemptStatusInvalid DeliveryAttemptStatus = "invalid"
)

type NotificationStatusReason string

const (
	StatusReasonAllDeliveriesSucceeded NotificationStatusReason = "AllDeliveriesSucceeded"
	StatusReasonPartialDeliverySuccess NotificationStatusReason = "PartialDeliverySuccess"
	StatusReasonAllDeliveriesFailed    NotificationStatusReason = "AllDeliveriesFailed"
	StatusReasonNoChannelsResolved     NotificationStatusReason = "NoChannelsResolved"
	StatusReasonPartialFailureRetrying NotificationStatusReason = "PartialFailureRetrying"
	StatusReasonMaxRetriesExhausted    NotificationStatusReason = "MaxRetriesExhausted"
)

type DeliveryChannelName string

type ActionLinkServiceType string

const (
	ActionLinkServiceGrafana    ActionLinkServiceType = "grafana"
	ActionLinkServicePrometheus ActionLinkServiceType = "prometheus"
)

// NotificationContext provides structured context for a notification,
// replacing the former unstructured Metadata map[string]string.
type NotificationContext struct {
	// Lineage tracks parent resource references for audit correlation.
	// +optional
	Lineage *LineageContext `json:"lineage,omitempty"`

	// Workflow captures selected workflow details (approval/completion notifications).
	// +optional
	Workflow *WorkflowContext `json:"workflow,omitempty"`

	// Analysis captures AI analysis results (approval/completion notifications).
	// +optional
	Analysis *AnalysisContext `json:"analysis,omitempty"`

	// Review captures manual review context (manual-review notifications).
	// +optional
	Review *ReviewContext `json:"review,omitempty"`

	// Execution captures execution and retry context (manual-review WE source, timeout notifications).
	// +optional
	Execution *ExecutionContext `json:"execution,omitempty"`

	// Dedup captures deduplication context (bulk duplicate notifications).
	// +optional
	Dedup *DedupContext `json:"dedup,omitempty"`

	// Target captures target resource context (timeout notifications).
	// +optional
	Target *TargetContext `json:"target,omitempty"`

	// Verification captures EA verification results (completion notifications, #318).
	// Enables routing rules to match on verification outcome (e.g., inconclusive -> escalation).
	// +optional
	Verification *VerificationContext `json:"verification,omitempty"`
}

// FlattenToMap reconstructs a flat map[string]string from the typed sub-structs.
// Used by routing resolver and audit manager for backward compatibility.
// Only non-empty fields are included. Nil sub-structs are safely skipped.
func (c *NotificationContext) FlattenToMap() map[string]string {
	if c == nil {
		return map[string]string{}
	}
	m := make(map[string]string)
	if c.Lineage != nil {
		setIfNonEmpty(m, "remediationRequest", c.Lineage.RemediationRequest)
		setIfNonEmpty(m, "aiAnalysis", c.Lineage.AIAnalysis)
	}
	if c.Workflow != nil {
		setIfNonEmpty(m, "selectedWorkflow", c.Workflow.SelectedWorkflow)
		setIfNonEmpty(m, "confidence", c.Workflow.Confidence)
		setIfNonEmpty(m, "workflowId", c.Workflow.WorkflowID)
		setIfNonEmpty(m, "executionEngine", c.Workflow.ExecutionEngine)
	}
	if c.Analysis != nil {
		setIfNonEmpty(m, "approvalReason", c.Analysis.ApprovalReason)
		setIfNonEmpty(m, "rootCause", c.Analysis.RootCause)
		setIfNonEmpty(m, "outcome", c.Analysis.Outcome)
	}
	if c.Review != nil {
		setIfNonEmpty(m, "reason", c.Review.Reason)
		setIfNonEmpty(m, "subReason", c.Review.SubReason)
		setIfNonEmpty(m, "humanReviewReason", c.Review.HumanReviewReason)
		setIfNonEmpty(m, "rootCauseAnalysis", c.Review.RootCauseAnalysis)
	}
	if c.Execution != nil {
		setIfNonEmpty(m, "retryCount", c.Execution.RetryCount)
		setIfNonEmpty(m, "maxRetries", c.Execution.MaxRetries)
		setIfNonEmpty(m, "lastExitCode", c.Execution.LastExitCode)
		setIfNonEmpty(m, "previousExecution", c.Execution.PreviousExecution)
		setIfNonEmpty(m, "timeoutPhase", c.Execution.TimeoutPhase)
		setIfNonEmpty(m, "phaseTimeout", c.Execution.PhaseTimeout)
	}
	if c.Dedup != nil {
		setIfNonEmpty(m, "duplicateCount", c.Dedup.DuplicateCount)
	}
	if c.Target != nil {
		setIfNonEmpty(m, "targetResource", c.Target.TargetResource)
	}
	if c.Verification != nil {
		if c.Verification.Assessed {
			m["verificationAssessed"] = "true"
		} else {
			m["verificationAssessed"] = "false"
		}
		setIfNonEmpty(m, "verificationOutcome", c.Verification.Outcome)
		setIfNonEmpty(m, "verificationReason", c.Verification.Reason)
		if c.Verification.Degraded {
			m["verificationDegraded"] = "true"
		} else {
			m["verificationDegraded"] = "false"
		}
		setIfNonEmpty(m, "verificationDegradedReason", c.Verification.DegradedReason)
	}
	return m
}

func setIfNonEmpty(m map[string]string, key, value string) {
	if value != "" {
		m[key] = value
	}
}

// LineageContext tracks parent resource references for audit correlation (BR-NOT-064).
type LineageContext struct {
	// RemediationRequest is the name of the parent RemediationRequest.
	// +optional
	RemediationRequest string `json:"remediationRequest,omitempty"`
	// AIAnalysis is the name of the parent AIAnalysis.
	// +optional
	AIAnalysis string `json:"aiAnalysis,omitempty"`
}

// WorkflowContext captures selected workflow details.
type WorkflowContext struct {
	// SelectedWorkflow is the ID of the workflow selected by AI.
	// +optional
	SelectedWorkflow string `json:"selectedWorkflow,omitempty"`
	// Confidence is the AI confidence score (as string, e.g. "0.95").
	// +optional
	Confidence string `json:"confidence,omitempty"`
	// WorkflowID is the ID of the executed workflow.
	// +optional
	WorkflowID string `json:"workflowId,omitempty"`
	// ExecutionEngine is the engine used to execute the workflow.
	// +optional
	ExecutionEngine string `json:"executionEngine,omitempty"`
}

// AnalysisContext captures AI analysis results.
type AnalysisContext struct {
	// ApprovalReason explains why approval was required.
	// +optional
	ApprovalReason string `json:"approvalReason,omitempty"`
	// RootCause is the AI-determined root cause summary.
	// +optional
	RootCause string `json:"rootCause,omitempty"`
	// Outcome is the remediation outcome (e.g., "Success", "Failed").
	// +optional
	Outcome string `json:"outcome,omitempty"`
}

// ReviewContext captures manual review details (BR-ORCH-036).
type ReviewContext struct {
	// Reason is the high-level failure reason (e.g., "WorkflowResolutionFailed").
	// +optional
	Reason string `json:"reason,omitempty"`
	// SubReason provides granular detail (e.g., "WorkflowNotFound").
	// +optional
	SubReason string `json:"subReason,omitempty"`
	// HumanReviewReason from HAPI when needs_human_review=true (BR-HAPI-197).
	// +optional
	HumanReviewReason string `json:"humanReviewReason,omitempty"`
	// RootCauseAnalysis from AIAnalysis if available.
	// +optional
	RootCauseAnalysis string `json:"rootCauseAnalysis,omitempty"`
}

// ExecutionContext captures execution and retry data.
type ExecutionContext struct {
	// RetryCount is the number of retries attempted.
	// +optional
	RetryCount string `json:"retryCount,omitempty"`
	// MaxRetries is the maximum number of retries allowed.
	// +optional
	MaxRetries string `json:"maxRetries,omitempty"`
	// LastExitCode is the last exit code from the workflow execution.
	// +optional
	LastExitCode string `json:"lastExitCode,omitempty"`
	// PreviousExecution is the name of the previous WorkflowExecution.
	// +optional
	PreviousExecution string `json:"previousExecution,omitempty"`
	// TimeoutPhase is the phase that timed out.
	// +optional
	TimeoutPhase string `json:"timeoutPhase,omitempty"`
	// PhaseTimeout is the duration string for the phase timeout.
	// +optional
	PhaseTimeout string `json:"phaseTimeout,omitempty"`
}

// DedupContext captures deduplication context (BR-ORCH-034).
type DedupContext struct {
	// DuplicateCount is the number of duplicate signals.
	// +optional
	DuplicateCount string `json:"duplicateCount,omitempty"`
}

// TargetContext captures target resource context.
type TargetContext struct {
	// TargetResource in "Kind/Name" format.
	// +optional
	TargetResource string `json:"targetResource,omitempty"`
}

// VerificationContext captures EA verification results for completion notifications (#318).
// Enables programmatic routing (e.g., inconclusive outcomes -> escalation channel).
type VerificationContext struct {
	// Assessed indicates whether verification was performed at all.
	Assessed bool `json:"assessed"`
	// Outcome is the high-level result: "passed", "completed", "partial", "inconclusive", "unavailable".
	// "completed" indicates all components were assessed but some scores < 1.0 (Issue #596).
	// +optional
	Outcome string `json:"outcome,omitempty"`
	// Reason maps to EffectivenessAssessment.Status.AssessmentReason.
	// +optional
	Reason string `json:"reason,omitempty"`
	// Summary is the operator-facing human-readable message.
	// +optional
	Summary string `json:"summary,omitempty"`
	// Degraded indicates that the EA was unable to reliably compare pre- and
	// post-remediation state because hash capture failed (Issue #546).
	// Routing rules can match on this to escalate degraded notifications.
	// +optional
	Degraded bool `json:"degraded,omitempty"`
	// DegradedReason describes why the EA is degraded (e.g., RBAC Forbidden for
	// the target CRD). Empty when Degraded is false.
	// +optional
	DegradedReason string `json:"degradedReason,omitempty"`
}

// RetryPolicy defines retry behavior for notification delivery
type RetryPolicy struct {
	// Maximum number of delivery attempts
	// +kubebuilder:default=5
	// +kubebuilder:validation:Minimum=1
	// +kubebuilder:validation:Maximum=10
	MaxAttempts int `json:"maxAttempts,omitempty"`

	// Initial backoff duration in seconds
	// +kubebuilder:default=30
	// +kubebuilder:validation:Minimum=1
	// +kubebuilder:validation:Maximum=300
	InitialBackoffSeconds int `json:"initialBackoffSeconds,omitempty"`

	// Backoff multiplier (exponential backoff)
	// +kubebuilder:default=2
	// +kubebuilder:validation:Minimum=1
	// +kubebuilder:validation:Maximum=10
	BackoffMultiplier int `json:"backoffMultiplier,omitempty"`

	// Maximum backoff duration in seconds
	// +kubebuilder:default=480
	// +kubebuilder:validation:Minimum=60
	// +kubebuilder:validation:Maximum=3600
	MaxBackoffSeconds int `json:"maxBackoffSeconds,omitempty"`
}

// ActionLink represents an external service action link
type ActionLink struct {
	// Service name (github, grafana, prometheus, kubernetes-dashboard, etc.)
	Service ActionLinkServiceType `json:"service"`

	// Action link URL
	URL string `json:"url"`

	// Human-readable label for the link
	Label string `json:"label"`
}

// NotificationRequestSpec defines the desired state of NotificationRequest
//
// DD-NOT-005: Spec Immutability
// ALL spec fields are immutable after CRD creation. Users cannot update
// notification content once created. To change a notification, delete
// and recreate the CRD.
//
// Rationale: Notifications are immutable events, not mutable resources.
// This prevents race conditions, simplifies controller logic, and provides
// perfect audit trail.
//
// Cancellation: Delete the NotificationRequest CRD to cancel delivery.
//
// +kubebuilder:validation:XValidation:rule="self == oldSelf",message="spec is immutable after creation (DD-NOT-005)"
type NotificationRequestSpec struct {
	// Reference to parent RemediationRequest (if applicable)
	// Used for audit correlation and lineage tracking (BR-NOT-064)
	// Optional: NotificationRequest can be standalone (e.g., system-generated alerts)
	// +optional
	RemediationRequestRef *corev1.ObjectReference `json:"remediationRequestRef,omitempty"`

	// Type of notification (escalation, simple, status-update)
	// +kubebuilder:validation:Required
	Type NotificationType `json:"type"`

	// Priority of notification (critical, high, medium, low)
	// +kubebuilder:validation:Required
	// +kubebuilder:default=Medium
	Priority NotificationPriority `json:"priority"`

	// Subject line for notification
	// +kubebuilder:validation:Required
	// +kubebuilder:validation:MinLength=1
	// +kubebuilder:validation:MaxLength=500
	Subject string `json:"subject"`

	// Notification body content
	// +kubebuilder:validation:Required
	// +kubebuilder:validation:MinLength=1
	Body string `json:"body"`

	// Severity from the originating signal (used for routing)
	// Issue #91: promoted from mutable label to immutable spec field
	// +optional
	Severity string `json:"severity,omitempty"`

	// Phase that triggered this notification (for phase-timeout notifications)
	// Issue #91: promoted from mutable label to immutable spec field
	// +optional
	Phase string `json:"phase,omitempty"`

	// ReviewSource indicates what triggered manual review (for manual-review notifications)
	// Issue #91: promoted from mutable label to immutable spec field
	// +optional
	ReviewSource ReviewSourceType `json:"reviewSource,omitempty"`

	// Context provides typed, structured notification context replacing the
	// former unstructured Metadata map. Each sub-struct is optional (nil means
	// not applicable for this notification type).
	// +optional
	Context *NotificationContext `json:"context,omitempty"`

	// Extensions holds arbitrary key-value pairs for routing and custom data
	// that don't fit the typed Context schema (e.g., test routing overrides,
	// vendor-specific tags). Routing rules can match on these keys.
	// +optional
	Extensions map[string]string `json:"extensions,omitempty"`

	// Action links to external services
	// +optional
	ActionLinks []ActionLink `json:"actionLinks,omitempty"`

	// Retry policy for delivery
	// +optional
	RetryPolicy *RetryPolicy `json:"retryPolicy,omitempty"`

	// Retention period in days after completion
	// +kubebuilder:default=7
	// +kubebuilder:validation:Minimum=1
	// +kubebuilder:validation:Maximum=90
	// +optional
	RetentionDays int `json:"retentionDays,omitempty"`
}

// DeliveryAttempt records a single delivery attempt to a channel
type DeliveryAttempt struct {
	// Channel name
	Channel DeliveryChannelName `json:"channel"`

	// Attempt number (1-based)
	Attempt int `json:"attempt"`

	// Timestamp of this attempt
	Timestamp metav1.Time `json:"timestamp"`

	// Status of this attempt (success, failed, timeout, invalid)
	Status DeliveryAttemptStatus `json:"status"`

	// Error message if failed
	// +optional
	Error string `json:"error,omitempty"`

	// Duration of delivery attempt in seconds
	// +optional
	DurationSeconds float64 `json:"durationSeconds,omitempty"`
}

// NotificationRequestStatus defines the observed state of NotificationRequest
type NotificationRequestStatus struct {
	// Phase of notification lifecycle (Pending, Sending, Sent, PartiallySent, Failed)
	// +optional
	Phase NotificationPhase `json:"phase,omitempty"`

	// Conditions represent the latest available observations of the notification's state
	// +optional
	Conditions []metav1.Condition `json:"conditions,omitempty"`

	// List of all delivery attempts across all channels
	// +optional
	DeliveryAttempts []DeliveryAttempt `json:"deliveryAttempts,omitempty"`

	// Total number of delivery attempts across all channels
	// +optional
	TotalAttempts int `json:"totalAttempts,omitempty"`

	// Number of successful deliveries
	// +optional
	SuccessfulDeliveries int `json:"successfulDeliveries,omitempty"`

	// Number of failed deliveries
	// +optional
	FailedDeliveries int `json:"failedDeliveries,omitempty"`

	// Time when notification was queued for processing
	// +optional
	QueuedAt *metav1.Time `json:"queuedAt,omitempty"`

	// Time when processing started
	// +optional
	ProcessingStartedAt *metav1.Time `json:"processingStartedAt,omitempty"`

	// Time when all deliveries completed (success or failure)
	// +optional
	CompletionTime *metav1.Time `json:"completionTime,omitempty"`

	// Observed generation from spec
	// +optional
	ObservedGeneration int64 `json:"observedGeneration,omitempty"`

	// Reason for current phase
	// +optional
	Reason NotificationStatusReason `json:"reason,omitempty"`

	// Human-readable message about current state
	// +optional
	Message string `json:"message,omitempty"`
}

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:selectablefield:JSONPath=.spec.remediationRequestRef.name
// +kubebuilder:selectablefield:JSONPath=.spec.type
// +kubebuilder:selectablefield:JSONPath=.spec.severity
// +kubebuilder:resource:shortName=notif;notifs
// +kubebuilder:printcolumn:name="Type",type=string,JSONPath=`.spec.type`
// +kubebuilder:printcolumn:name="Priority",type=string,JSONPath=`.spec.priority`
// +kubebuilder:printcolumn:name="Phase",type=string,JSONPath=`.status.phase`
// +kubebuilder:printcolumn:name="Attempts",type=integer,JSONPath=`.status.totalAttempts`
// +kubebuilder:printcolumn:name="Reason",type=string,JSONPath=`.status.conditions[?(@.type=="Ready")].reason`,priority=1
// +kubebuilder:printcolumn:name="Age",type=date,JSONPath=`.metadata.creationTimestamp`

// NotificationRequest is the Schema for the notificationrequests API
type NotificationRequest struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   NotificationRequestSpec   `json:"spec,omitempty"`
	Status NotificationRequestStatus `json:"status,omitempty"`
}

// +kubebuilder:object:root=true

// NotificationRequestList contains a list of NotificationRequest
type NotificationRequestList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []NotificationRequest `json:"items"`
}

func init() {
	SchemeBuilder.Register(&NotificationRequest{}, &NotificationRequestList{})
}
