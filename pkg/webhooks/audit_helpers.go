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

package webhooks

import (
	api "github.com/jordigilh/kubernaut/pkg/datastorage/ogen-client"
)

// Enum conversion helpers for webhook audit payloads

// toNotificationAuditPayloadType converts CRD notification type string to ogen enum
// Maps api/notification/v1alpha1/notificationrequest_types.go:31-40 NotificationType enum
func toNotificationAuditPayloadType(typeStr string) api.NotificationAuditPayloadType {
	switch typeStr {
	case "escalation":
		return api.NotificationAuditPayloadTypeEscalation
	case "simple":
		return api.NotificationAuditPayloadTypeSimple
	case "status-update":
		return api.NotificationAuditPayloadTypeStatusUpdate
	case "approval":
		return api.NotificationAuditPayloadTypeApproval
	case "manual-review":
		return api.NotificationAuditPayloadTypeManualReview
	default:
		return api.NotificationAuditPayloadTypeSimple // default fallback
	}
}

// toNotificationAuditPayloadNotificationType converts CRD notification type string to ogen enum (alias field)
// Maps api/notification/v1alpha1/notificationrequest_types.go:31-40 NotificationType enum
func toNotificationAuditPayloadNotificationType(typeStr string) api.NotificationAuditPayloadNotificationType {
	switch typeStr {
	case "escalation":
		return api.NotificationAuditPayloadNotificationTypeEscalation
	case "simple":
		return api.NotificationAuditPayloadNotificationTypeSimple
	case "status-update":
		return api.NotificationAuditPayloadNotificationTypeStatusUpdate
	case "approval":
		return api.NotificationAuditPayloadNotificationTypeApproval
	case "manual-review":
		return api.NotificationAuditPayloadNotificationTypeManualReview
	default:
		return api.NotificationAuditPayloadNotificationTypeSimple // default fallback
	}
}

// toNotificationAuditPayloadPriority converts CRD priority string to ogen enum
// Maps api/notification/v1alpha1/notificationrequest_types.go:47-50 NotificationPriority enum
func toNotificationAuditPayloadPriority(priority string) api.NotificationAuditPayloadPriority {
	switch priority {
	case "critical":
		return api.NotificationAuditPayloadPriorityCritical
	case "high":
		return api.NotificationAuditPayloadPriorityHigh
	case "medium":
		return api.NotificationAuditPayloadPriorityMedium
	case "low":
		return api.NotificationAuditPayloadPriorityLow
	default:
		return api.NotificationAuditPayloadPriorityMedium // default fallback
	}
}

// toNotificationAuditPayloadFinalStatus converts CRD phase string to ogen enum
func toNotificationAuditPayloadFinalStatus(phase string) api.NotificationAuditPayloadFinalStatus {
	switch phase {
	case "Pending":
		return api.NotificationAuditPayloadFinalStatusPending
	case "Sending":
		return api.NotificationAuditPayloadFinalStatusSending
	case "Sent":
		return api.NotificationAuditPayloadFinalStatusSent
	case "Failed":
		return api.NotificationAuditPayloadFinalStatusFailed
	case "Cancelled":
		return api.NotificationAuditPayloadFinalStatusCancelled
	default:
		return api.NotificationAuditPayloadFinalStatusPending // default fallback
	}
}

// toRemediationApprovalAuditPayloadDecision converts CRD decision string to ogen enum
func toRemediationApprovalAuditPayloadDecision(decision string) api.RemediationApprovalAuditPayloadDecision {
	switch decision {
	case "Approved":
		return api.RemediationApprovalAuditPayloadDecisionApproved
	case "Rejected":
		return api.RemediationApprovalAuditPayloadDecisionRejected
	default:
		return api.RemediationApprovalAuditPayloadDecisionApproved // default fallback
	}
}
