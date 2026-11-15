// Package cancellation contains the shared status-condition contract used by
// child controllers during RemediationRequest cancellation (issue #2588).
package cancellation

import (
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

const (
	// ConditionAcknowledged is true only after the child has no remaining work
	// and any external backend stop has been verified.
	ConditionAcknowledged = "CancellationAcknowledged"

	ReasonRequested = "CancellationRequested"
	ReasonStopped   = "CancellationStopped"
	ReasonFailed    = "CancellationFailed"
)

// SetRequested records that the child observed parent cancellation but has not
// yet acknowledged that its work stopped.
func SetRequested(conditions *[]metav1.Condition, observedGeneration int64, message string) {
	setCondition(conditions, observedGeneration, metav1.ConditionFalse, ReasonRequested, message)
}

// SetAcknowledged records a verified cancellation acknowledgement.
func SetAcknowledged(conditions *[]metav1.Condition, observedGeneration int64, message string) {
	setCondition(conditions, observedGeneration, metav1.ConditionTrue, ReasonStopped, message)
}

// SetFailed records an observable cancellation failure without claiming that
// the child or its backend stopped.
func SetFailed(conditions *[]metav1.Condition, observedGeneration int64, message string) {
	setCondition(conditions, observedGeneration, metav1.ConditionFalse, ReasonFailed, message)
}

// IsAcknowledged reports whether the child has positively acknowledged
// cancellation. A false condition, including a failure, is never sufficient.
func IsAcknowledged(conditions []metav1.Condition) bool {
	condition := meta.FindStatusCondition(conditions, ConditionAcknowledged)
	return condition != nil && condition.Status == metav1.ConditionTrue && condition.Reason == ReasonStopped
}

// IsFailed reports whether the most recent cancellation observation failed.
func IsFailed(conditions []metav1.Condition) bool {
	condition := meta.FindStatusCondition(conditions, ConditionAcknowledged)
	return condition != nil && condition.Status == metav1.ConditionFalse && condition.Reason == ReasonFailed
}

func setCondition(
	conditions *[]metav1.Condition,
	observedGeneration int64,
	status metav1.ConditionStatus,
	reason, message string,
) {
	meta.SetStatusCondition(conditions, metav1.Condition{
		Type:               ConditionAcknowledged,
		Status:             status,
		ObservedGeneration: observedGeneration,
		Reason:             reason,
		Message:            message,
	})
}
