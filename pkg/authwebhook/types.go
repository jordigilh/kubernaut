package authwebhook

import (
	"fmt"

	authenticationv1 "k8s.io/api/authentication/v1"
)

// Webhook event type constants (L-3 SOC2 Fix: compile-time safety for event type strings)
const (
	EventTypeBlockCleared    = "workflowexecution.block.cleared"
	EventTypeTimeoutModified = "webhook.remediationrequest.timeout_modified"
	EventTypeRARDecided      = "webhook.remediationapprovalrequest.decided"
	EventTypeNotifCancelled  = "webhook.notification.cancelled"

	// ADR-058: RemediationWorkflow CRD admission event types
	EventTypeRWAdmittedCreate = "remediationworkflow.admitted.create"
	EventTypeRWAdmittedDelete = "remediationworkflow.admitted.delete"
	EventTypeRWAdmittedDenied = "remediationworkflow.admitted.denied"

	// ADR-059: ActionType CRD admission event types (BR-WORKFLOW-007)
	EventTypeATAdmittedCreate = "actiontype.admitted.create"
	EventTypeATAdmittedUpdate = "actiontype.admitted.update"
	EventTypeATAdmittedDelete = "actiontype.admitted.delete"
	EventTypeATDeniedCreate   = "actiontype.denied.create"
	EventTypeATDeniedUpdate   = "actiontype.denied.update"
	EventTypeATDeniedDelete   = "actiontype.denied.delete"
)

// Event category constants per ADR-034 v1.8: event_category = business domain
const (
	EventCategoryActionType       = "actiontype"
	EventCategoryWorkflow         = "workflow"
	EventCategoryWorkflowExec     = "workflowexecution"
	EventCategoryApproval         = "approval"
	EventCategoryNotification     = "notification"
)

// AuthContext holds authenticated user information extracted from admission requests.
// This struct is used for SOC2 CC8.1 operator attribution and audit trail persistence.
// Test Plan Reference: AUTH-001 to AUTH-012
type AuthContext struct {
	Username string
	UID      string
	Groups   []string
	Extra    map[string]authenticationv1.ExtraValue
}

// String returns formatted authentication string for audit trails
// Format: "username (UID: uid)"
func (a *AuthContext) String() string {
	return fmt.Sprintf("%s (UID: %s)", a.Username, a.UID)
}
