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
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// EDIT THIS FILE!  THIS IS SCAFFOLDING FOR YOU TO OWN!
// NOTE: json tags are required.  Any new fields you add must have json tags for the fields to be serialized.

// +kubebuilder:validation:Enum=escalation;simple;status-update
type NotificationType string

const (
	NotificationTypeEscalation   NotificationType = "escalation"
	NotificationTypeSimple       NotificationType = "simple"
	NotificationTypeStatusUpdate NotificationType = "status-update"
)

// +kubebuilder:validation:Enum=critical;high;medium;low
type NotificationPriority string

const (
	NotificationPriorityCritical NotificationPriority = "critical"
	NotificationPriorityHigh     NotificationPriority = "high"
	NotificationPriorityMedium   NotificationPriority = "medium"
	NotificationPriorityLow      NotificationPriority = "low"
)

// +kubebuilder:validation:Enum=email;slack;teams;sms;webhook;console
type Channel string

const (
	ChannelEmail   Channel = "email"
	ChannelSlack   Channel = "slack"
	ChannelTeams   Channel = "teams"
	ChannelSMS     Channel = "sms"
	ChannelWebhook Channel = "webhook"
	ChannelConsole Channel = "console"
)

// +kubebuilder:validation:Enum=Pending;Sending;Sent;PartiallySent;Failed
type NotificationPhase string

const (
	NotificationPhasePending       NotificationPhase = "Pending"
	NotificationPhaseSending       NotificationPhase = "Sending"
	NotificationPhaseSent          NotificationPhase = "Sent"
	NotificationPhasePartiallySent NotificationPhase = "PartiallySent"
	NotificationPhaseFailed        NotificationPhase = "Failed"
)

// Recipient represents a notification recipient
type Recipient struct {
	// Email address (for email channel)
	// +optional
	Email string `json:"email,omitempty"`

	// Slack channel or user (for Slack channel)
	// Format: #channel-name or @username
	// +optional
	Slack string `json:"slack,omitempty"`

	// Teams channel or user (for Teams channel)
	// +optional
	Teams string `json:"teams,omitempty"`

	// Phone number (for SMS channel)
	// Format: E.164 (+1234567890)
	// +optional
	Phone string `json:"phone,omitempty"`

	// Webhook URL (for webhook channel)
	// +optional
	WebhookURL string `json:"webhookURL,omitempty"`
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
	Service string `json:"service"`

	// Action link URL
	URL string `json:"url"`

	// Human-readable label for the link
	Label string `json:"label"`
}

// NotificationRequestSpec defines the desired state of NotificationRequest
type NotificationRequestSpec struct {
	// Type of notification (escalation, simple, status-update)
	// +kubebuilder:validation:Required
	Type NotificationType `json:"type"`

	// Priority of notification (critical, high, medium, low)
	// +kubebuilder:validation:Required
	// +kubebuilder:default=medium
	Priority NotificationPriority `json:"priority"`

	// List of recipients for this notification
	// +kubebuilder:validation:Required
	// +kubebuilder:validation:MinItems=1
	Recipients []Recipient `json:"recipients"`

	// Subject line for notification
	// +kubebuilder:validation:Required
	// +kubebuilder:validation:MinLength=1
	// +kubebuilder:validation:MaxLength=500
	Subject string `json:"subject"`

	// Notification body content
	// +kubebuilder:validation:Required
	// +kubebuilder:validation:MinLength=1
	Body string `json:"body"`

	// Delivery channels to use
	// +kubebuilder:validation:Required
	// +kubebuilder:validation:MinItems=1
	Channels []Channel `json:"channels"`

	// Metadata for context (key-value pairs)
	// Examples: remediationRequestName, cluster, namespace, severity, alertName
	// +optional
	Metadata map[string]string `json:"metadata,omitempty"`

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
	Channel string `json:"channel"`

	// Attempt number (1-based)
	Attempt int `json:"attempt"`

	// Timestamp of this attempt
	Timestamp metav1.Time `json:"timestamp"`

	// Status of this attempt (success, failed, timeout, invalid)
	Status string `json:"status"`

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
	Reason string `json:"reason,omitempty"`

	// Human-readable message about current state
	// +optional
	Message string `json:"message,omitempty"`
}

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:resource:shortName=notif;notifs
// +kubebuilder:printcolumn:name="Type",type=string,JSONPath=`.spec.type`
// +kubebuilder:printcolumn:name="Priority",type=string,JSONPath=`.spec.priority`
// +kubebuilder:printcolumn:name="Phase",type=string,JSONPath=`.status.phase`
// +kubebuilder:printcolumn:name="Attempts",type=integer,JSONPath=`.status.totalAttempts`
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
