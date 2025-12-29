package notification

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	notificationv1alpha1 "github.com/jordigilh/kubernaut/api/notification/v1alpha1"
)

// ========================================
// BR-NOT-069: Routing Rule Visibility via Kubernetes Conditions
// 📋 Design Decision: DD-CRD-001 | ✅ Approved Design | Confidence: 90%
// See: docs/requirements/BR-NOT-069-routing-rule-visibility-conditions.md
// ========================================
//
// Kubernetes Conditions for Notification Service routing visibility.
// Enables operators to debug label-based routing without accessing controller logs.
//
// WHY BR-NOT-069?
// - ✅ Routing Rule Visibility: Operators can see which rule matched
// - ✅ Label-Based Debugging: Understand why certain channels were selected
// - ✅ Fallback Detection: Know when console fallback was used
// - ✅ kubectl UX: Routing diagnostics via `kubectl describe` without log access
//
// Reduces routing debug time from 15-30 min (logs) to <1 min (kubectl)
// ========================================

const (
	// ConditionTypeRoutingResolved indicates routing rule resolution completed
	// Status: True = routing resolved (rule matched or fallback)
	// Status: False = routing failed (error state)
	ConditionTypeRoutingResolved = "RoutingResolved"

	// Routing success reasons
	ReasonRoutingRuleMatched = "RoutingRuleMatched" // A routing rule matched successfully
	ReasonRoutingFallback    = "RoutingFallback"    // No rules matched, using console fallback
	ReasonRoutingFailed      = "RoutingFailed"      // Routing resolution failed (error state)
)

// SetRoutingResolved sets the RoutingResolved condition on the NotificationRequest.
//
// This function follows Kubernetes API conventions for condition management:
// - Updates LastTransitionTime only when Status changes
// - Preserves ObservedGeneration from NotificationRequest metadata
// - Appends if condition doesn't exist, updates if it does
//
// Parameters:
//   - notif: NotificationRequest CRD to update
//   - status: metav1.ConditionTrue (success) or metav1.ConditionFalse (failed)
//   - reason: One of ReasonRoutingRuleMatched, ReasonRoutingFallback, ReasonRoutingFailed
//   - message: Human-readable description including matched rule name and channels
//
// Example messages:
//   - "Matched rule 'production-critical' (severity=critical, env=production) → channels: slack, email, pagerduty"
//   - "No routing rules matched (labels: type=simple, severity=low), using console fallback"
//   - "Routing failed due to invalid configuration"
func SetRoutingResolved(notif *notificationv1alpha1.NotificationRequest, status metav1.ConditionStatus, reason, message string) {
	now := metav1.Now()
	newCondition := metav1.Condition{
		Type:               ConditionTypeRoutingResolved,
		Status:             status,
		Reason:             reason,
		Message:            message,
		LastTransitionTime: now,
		ObservedGeneration: notif.Generation,
	}

	// Find existing condition
	existingIndex := -1
	for i, condition := range notif.Status.Conditions {
		if condition.Type == ConditionTypeRoutingResolved {
			existingIndex = i
			break
		}
	}

	if existingIndex == -1 {
		// Condition doesn't exist, append it
		notif.Status.Conditions = append(notif.Status.Conditions, newCondition)
	} else {
		// Condition exists, update it
		existingCondition := notif.Status.Conditions[existingIndex]

		// Preserve LastTransitionTime if Status hasn't changed
		if existingCondition.Status == status {
			newCondition.LastTransitionTime = existingCondition.LastTransitionTime
		}

		// Replace the condition
		notif.Status.Conditions[existingIndex] = newCondition
	}
}

// GetRoutingResolved returns the RoutingResolved condition from the NotificationRequest.
//
// Returns:
//   - *metav1.Condition: The RoutingResolved condition if it exists
//   - nil: If the condition does not exist
func GetRoutingResolved(notif *notificationv1alpha1.NotificationRequest) *metav1.Condition {
	for i := range notif.Status.Conditions {
		if notif.Status.Conditions[i].Type == ConditionTypeRoutingResolved {
			return &notif.Status.Conditions[i]
		}
	}
	return nil
}

// IsRoutingResolved checks if routing was successfully resolved.
//
// Returns:
//   - true: If RoutingResolved condition exists and Status is True
//   - false: If condition doesn't exist or Status is False
//
// Note: Both RoutingRuleMatched and RoutingFallback are considered successful (Status=True).
// Only RoutingFailed results in Status=False.
func IsRoutingResolved(notif *notificationv1alpha1.NotificationRequest) bool {
	condition := GetRoutingResolved(notif)
	if condition == nil {
		return false
	}
	return condition.Status == metav1.ConditionTrue
}
