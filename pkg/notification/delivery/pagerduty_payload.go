package delivery

import (
	"fmt"

	notificationv1alpha1 "github.com/jordigilh/kubernaut/api/notification/v1alpha1"
)

// PagerDutyEvent represents a PagerDuty Events API v2 event envelope.
// See: https://developer.pagerduty.com/docs/events-api-v2/trigger-events/
type PagerDutyEvent struct {
	RoutingKey  string          `json:"routing_key"`
	EventAction string          `json:"event_action"`
	DedupKey    string          `json:"dedup_key"`
	Payload     PagerDutyDetail `json:"payload"`
}

// PagerDutyDetail represents the payload section of a PagerDuty event.
type PagerDutyDetail struct {
	Summary       string            `json:"summary"`
	Severity      string            `json:"severity"`
	Source        string            `json:"source"`
	Component     string            `json:"component,omitempty"`
	CustomDetails map[string]string `json:"custom_details,omitempty"`
}

// BuildPagerDutyPayload constructs a PagerDuty Events API v2 trigger event
// from a NotificationRequest. The routing key is supplied externally by the
// credential resolver (BR-NOT-104).
func BuildPagerDutyPayload(routingKey string, notification *notificationv1alpha1.NotificationRequest) PagerDutyEvent {
	return PagerDutyEvent{
		RoutingKey:  routingKey,
		EventAction: "trigger",
		DedupKey:    notification.Name,
		Payload: PagerDutyDetail{
			Summary:       notification.Spec.Subject,
			Severity:      mapPagerDutySeverity(notification.Spec.Priority),
			Source:        "kubernaut",
			Component:     notification.Namespace,
			CustomDetails: buildCustomDetails(notification),
		},
	}
}

// mapPagerDutySeverity maps NotificationPriority to PagerDuty severity values.
func mapPagerDutySeverity(priority notificationv1alpha1.NotificationPriority) string {
	switch priority {
	case notificationv1alpha1.NotificationPriorityCritical:
		return "critical"
	case notificationv1alpha1.NotificationPriorityHigh:
		return "error"
	case notificationv1alpha1.NotificationPriorityMedium:
		return "warning"
	case notificationv1alpha1.NotificationPriorityLow:
		return "info"
	default:
		return "warning"
	}
}

// buildCustomDetails extracts structured context fields from the notification
// into PagerDuty custom_details. Nil sub-structs are safely skipped.
func buildCustomDetails(notification *notificationv1alpha1.NotificationRequest) map[string]string {
	details := make(map[string]string)
	details["notification_type"] = string(notification.Spec.Type)
	details["correlation_id"] = notification.Name

	ctx := notification.Spec.Context
	if ctx == nil {
		return details
	}

	if ctx.Analysis != nil && ctx.Analysis.RootCause != "" {
		details["rca_summary"] = ctx.Analysis.RootCause
	}
	if ctx.Workflow != nil && ctx.Workflow.Confidence != "" {
		details["confidence"] = ctx.Workflow.Confidence
	}
	if ctx.Target != nil && ctx.Target.TargetResource != "" {
		details["affected_resource"] = ctx.Target.TargetResource
	}
	if ctx.Lineage != nil && ctx.Lineage.RemediationRequest != "" {
		details["kubectl_command"] = fmt.Sprintf(
			"kubectl kubernaut chat rar/%s -n %s",
			ctx.Lineage.RemediationRequest,
			notification.Namespace,
		)
	}

	return details
}
