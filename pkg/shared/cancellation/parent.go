package cancellation

import (
	remediationv1 "github.com/jordigilh/kubernaut/api/remediation/v1alpha1"
)

// RemediationRequestFinalizer retains a parent while cancellation shutdown is
// coordinated and child stop acknowledgements are verified.
const RemediationRequestFinalizer = "kubernaut.ai/remediationrequest-cancellation"

// ParentStopRequested reports whether a child must stop creating or executing
// work because its RemediationRequest is being cancelled or deleted. Ordinary
// failure, timeout, and skip outcomes are not cancellation requests; their
// normal terminal handling remains owned by the child controller.
func ParentStopRequested(rr *remediationv1.RemediationRequest) bool {
	if rr == nil {
		return false
	}
	if !rr.DeletionTimestamp.IsZero() {
		return true
	}
	switch rr.Status.OverallPhase {
	case remediationv1.PhaseCancelling,
		remediationv1.PhaseCancelled:
		return true
	default:
		return false
	}
}
