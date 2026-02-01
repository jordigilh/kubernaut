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

package authwebhook

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"

	remediationv1 "github.com/jordigilh/kubernaut/api/remediation/v1alpha1"
	"github.com/jordigilh/kubernaut/pkg/audit"
	api "github.com/jordigilh/kubernaut/pkg/datastorage/ogen-client"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/webhook/admission"
)

// RemediationApprovalRequestAuthHandler handles authentication for RemediationApprovalRequest decisions
// BR-AUTH-001: SOC2 CC8.1 Operator Attribution
// ADR-040: RemediationApprovalRequest CRD Architecture
// DD-WEBHOOK-003: Webhook-Complete Audit Pattern
//
// This mutating webhook intercepts RemediationApprovalRequest status updates and:
// 1. Populates status.DecidedBy (operator email/username)
// 2. Populates status.DecidedAt (timestamp)
// 3. Writes complete audit event (WHO + WHAT + ACTION)
type RemediationApprovalRequestAuthHandler struct {
	authenticator *Authenticator
	decoder       admission.Decoder
	auditStore    audit.AuditStore
}

// NewRemediationApprovalRequestAuthHandler creates a new RemediationApprovalRequest authentication handler
func NewRemediationApprovalRequestAuthHandler(auditStore audit.AuditStore) *RemediationApprovalRequestAuthHandler {
	return &RemediationApprovalRequestAuthHandler{
		authenticator: NewAuthenticator(),
		auditStore:    auditStore,
	}
}

// Handle processes the admission request for RemediationApprovalRequest
// Implements admission.Handler interface from controller-runtime
func (h *RemediationApprovalRequestAuthHandler) Handle(ctx context.Context, req admission.Request) admission.Response {
	rar := &remediationv1.RemediationApprovalRequest{}

	// Decode the RemediationApprovalRequest object from the request
	err := json.Unmarshal(req.Object.Raw, rar)
	if err != nil {
		return admission.Errored(http.StatusBadRequest, fmt.Errorf("failed to decode RemediationApprovalRequest: %w", err))
	}

	// Check if a decision has been made
	if rar.Status.Decision == "" {
		// No decision yet - allow without modification
		return admission.Allowed("no decision made")
	}

	// Validate decision is one of the allowed enum values
	validDecisions := map[remediationv1.ApprovalDecision]bool{
		remediationv1.ApprovalDecisionApproved: true,
		remediationv1.ApprovalDecisionRejected: true,
		remediationv1.ApprovalDecisionExpired:  true,
	}
	if !validDecisions[rar.Status.Decision] {
		return admission.Denied(fmt.Sprintf("invalid decision: %s (must be Approved, Rejected, or Expired)", rar.Status.Decision))
	}

	// Extract authenticated user from admission request
	authCtx, err := h.authenticator.ExtractUser(ctx, &req.AdmissionRequest)
	if err != nil {
		return admission.Denied(fmt.Sprintf("authentication required: %v", err))
	}

	// Check if decidedBy is already set (preserve existing attribution)
	if rar.Status.DecidedBy != "" {
		// Already decided - don't overwrite
		return admission.Allowed("decision already attributed")
	}

	// Populate authentication fields
	rar.Status.DecidedBy = authCtx.Username
	now := metav1.Now()
	rar.Status.DecidedAt = &now

	// Write complete audit event (DD-WEBHOOK-003: Webhook-Complete Audit Pattern)
	auditEvent := audit.NewAuditEventRequest()
	audit.SetEventType(auditEvent, fmt.Sprintf("remediation.approval.%s", string(rar.Status.Decision)))
	audit.SetEventCategory(auditEvent, "webhook") // Per ADR-034 v1.4: event_category = emitter service
	audit.SetEventAction(auditEvent, "approval_decided")
	audit.SetEventOutcome(auditEvent, audit.OutcomeSuccess)
	audit.SetActor(auditEvent, "user", authCtx.Username)
	audit.SetResource(auditEvent, "RemediationApprovalRequest", string(rar.UID))
	audit.SetCorrelationID(auditEvent, rar.Name) // Use RAR name for correlation
	audit.SetNamespace(auditEvent, rar.Namespace)

	// Set event data payload
	// Per DD-WEBHOOK-003 lines 314-318: Business context ONLY (attribution in structured columns)
	// Use structured audit payload (eliminates map[string]interface{})
	// Per DD-AUDIT-004: Zero unstructured data in audit events
	payload := api.RemediationApprovalAuditPayload{
		EventType:       "webhook.approval.decided",
		RequestName:     rar.Name,
		Decision:        toRemediationApprovalAuditPayloadDecision(string(rar.Status.Decision)),
		DecidedAt:       rar.Status.DecidedAt.Time,
		DecisionMessage: rar.Status.DecisionMessage, // Per DD-WEBHOOK-003 line 316
		AiAnalysisRef:   rar.Spec.AIAnalysisRef.Name, // Per DD-WEBHOOK-003 line 317 (note: lowercase 'i' in ogen)
	}
	// Note: Attribution fields (WHO, WHAT, WHERE, HOW) are in structured columns:
	// - actor_id: authCtx.Username (via audit.SetActor)
	// - resource_name: rar.Name (via audit.SetResource)
	// - namespace: rar.Namespace (via audit.SetNamespace)
	// - event_action: "approval_decided" (via audit.SetEventAction)
	auditEvent.EventData = api.NewRemediationApprovalAuditPayloadAuditEventRequestEventData(payload)

	// Store audit event asynchronously (buffered write)
	// Explicitly ignore errors - audit should not block webhook operations
	// The audit store has retry + DLQ mechanisms for reliability
	_ = h.auditStore.StoreAudit(ctx, auditEvent)

	// Marshal the patched object
	marshaledRAR, err := json.Marshal(rar)
	if err != nil {
		return admission.Errored(http.StatusInternalServerError, fmt.Errorf("failed to marshal patched RemediationApprovalRequest: %w", err))
	}

	// Return patched response
	return admission.PatchResponseFromRaw(req.Object.Raw, marshaledRAR)
}

// InjectDecoder injects the decoder into the handler
// Required by controller-runtime admission webhook framework
func (h *RemediationApprovalRequestAuthHandler) InjectDecoder(d admission.Decoder) error {
	h.decoder = d
	return nil
}

