package notification

import (
	"k8s.io/apimachinery/pkg/api/meta"
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
	// ConditionReady indicates the NotificationRequest is ready (or not)
	ConditionReady = "Ready"

	// ConditionTypeRoutingResolved indicates routing rule resolution completed
	// Status: True = routing resolved (rule matched or fallback)
	// Status: False = routing failed (error state)
	ConditionTypeRoutingResolved = "RoutingResolved"

	// Routing success reasons
	ReasonRoutingRuleMatched = "RoutingRuleMatched" // A routing rule matched successfully
	ReasonRoutingFallback    = "RoutingFallback"   // No rules matched, using console fallback
	ReasonRoutingFailed      = "RoutingFailed"     // Routing resolution failed (error state)

	// Ready condition reasons
	ReasonReady    = "Ready"
	ReasonNotReady = "NotReady"
)

// SetCondition sets an arbitrary condition on the NotificationRequest using
// meta.SetStatusCondition, which handles LastTransitionTime correctly.
func SetCondition(notif *notificationv1alpha1.NotificationRequest, condType string, status metav1.ConditionStatus, reason, message string) {
	meta.SetStatusCondition(&notif.Status.Conditions, metav1.Condition{
		Type:               condType,
		Status:             status,
		Reason:             reason,
		Message:            message,
		ObservedGeneration: notif.Generation,
	})
}

// GetCondition returns the condition with the given type, or nil if not found.
func GetCondition(notif *notificationv1alpha1.NotificationRequest, condType string) *metav1.Condition {
	return meta.FindStatusCondition(notif.Status.Conditions, condType)
}

// SetReady sets the Ready condition on the NotificationRequest.
func SetReady(notif *notificationv1alpha1.NotificationRequest, ready bool, reason, message string) {
	status := metav1.ConditionTrue
	if !ready {
		status = metav1.ConditionFalse
	}
	SetCondition(notif, ConditionReady, status, reason, message)
}

// SetRoutingResolved sets the RoutingResolved condition on the NotificationRequest.
func SetRoutingResolved(notif *notificationv1alpha1.NotificationRequest, status metav1.ConditionStatus, reason, message string) {
	SetCondition(notif, ConditionTypeRoutingResolved, status, reason, message)
}

// GetRoutingResolved returns the RoutingResolved condition from the NotificationRequest.
func GetRoutingResolved(notif *notificationv1alpha1.NotificationRequest) *metav1.Condition {
	return GetCondition(notif, ConditionTypeRoutingResolved)
}

// IsRoutingResolved checks if routing was successfully resolved.
// Both RoutingRuleMatched and RoutingFallback are considered successful (Status=True).
func IsRoutingResolved(notif *notificationv1alpha1.NotificationRequest) bool {
	condition := GetRoutingResolved(notif)
	if condition == nil {
		return false
	}
	return condition.Status == metav1.ConditionTrue
}
