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
	"strings"

	workflowexecutionv1 "github.com/jordigilh/kubernaut/api/workflowexecution/v1alpha1"
	"github.com/jordigilh/kubernaut/pkg/audit"
	api "github.com/jordigilh/kubernaut/pkg/datastorage/ogen-client"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/webhook/admission"
)

// WorkflowExecutionAuthHandler handles authentication for WorkflowExecution block clearance
// BR-WE-013: Audit-Tracked Execution Block Clearing
// BR-AUTH-001: SOC2 CC8.1 Operator Attribution
// DD-WEBHOOK-003: Webhook-Complete Audit Pattern
//
// This mutating webhook intercepts WorkflowExecution status updates and:
// 1. Populates status.BlockClearance.ClearedBy (operator email/username)
// 2. Populates status.BlockClearance.ClearedAt (timestamp)
// 3. Writes complete audit event (WHO + WHAT + ACTION)
type WorkflowExecutionAuthHandler struct {
	authenticator *Authenticator
	decoder       admission.Decoder
	auditStore    audit.AuditStore
}

// NewWorkflowExecutionAuthHandler creates a new WorkflowExecution authentication handler
func NewWorkflowExecutionAuthHandler(auditStore audit.AuditStore) *WorkflowExecutionAuthHandler {
	return &WorkflowExecutionAuthHandler{
		authenticator: NewAuthenticator(),
		auditStore:    auditStore,
	}
}

// Handle processes the admission request for WorkflowExecution
// Implements admission.Handler interface from controller-runtime
func (h *WorkflowExecutionAuthHandler) Handle(ctx context.Context, req admission.Request) admission.Response {
	wfe := &workflowexecutionv1.WorkflowExecution{}

	// Decode the WorkflowExecution object from the request
	err := json.Unmarshal(req.Object.Raw, wfe)
	if err != nil {
		return admission.Errored(http.StatusBadRequest, fmt.Errorf("failed to decode WorkflowExecution: %w", err))
	}

	// Check if block clearance is being requested
	if wfe.Status.BlockClearance == nil {
		// No clearance requested - allow without modification
		return admission.Allowed("no block clearance requested")
	}

	// Extract authenticated user from admission request
	authCtx, err := h.authenticator.ExtractUser(ctx, &req.AdmissionRequest)
	if err != nil {
		return admission.Denied(fmt.Sprintf("authentication required: %v", err))
	}

	// Validate clearance reason (SOC2 CC7.4: Audit Completeness)
	if wfe.Status.BlockClearance.ClearReason == "" {
		return admission.Denied("invalid clearance reason: reason is required")
	}
	// Validate clearance reason length (must be at least 10 words for proper audit trail)
	words := strings.Fields(wfe.Status.BlockClearance.ClearReason)
	if len(words) < 10 {
		return admission.Denied(fmt.Sprintf("invalid clearance reason: reason must be at least 10 words for proper audit trail (SOC2 CC7.4), got %d words", len(words)))
	}

	// Check if clearedBy is already set (preserve existing attribution)
	if wfe.Status.BlockClearance.ClearedBy != "" {
		// Already cleared - don't overwrite
		return admission.Allowed("clearance already attributed")
	}

	// Populate authentication fields
	wfe.Status.BlockClearance.ClearedBy = authCtx.Username
	wfe.Status.BlockClearance.ClearedAt = metav1.Now()

	// Write complete audit event (DD-WEBHOOK-003: Webhook-Complete Audit Pattern)
	auditEvent := audit.NewAuditEventRequest()
	audit.SetEventType(auditEvent, "workflowexecution.block.cleared")
	audit.SetEventCategory(auditEvent, "webhook") // Per ADR-034 v1.4: event_category = emitter service
	audit.SetEventAction(auditEvent, "block_cleared")
	audit.SetEventOutcome(auditEvent, audit.OutcomeSuccess)
	audit.SetActor(auditEvent, "user", authCtx.Username)
	audit.SetResource(auditEvent, "WorkflowExecution", string(wfe.UID))
	audit.SetCorrelationID(auditEvent, wfe.Name) // Use WFE name for correlation
	audit.SetNamespace(auditEvent, wfe.Namespace)

	// Set event data payload
	// Per DD-WEBHOOK-003 lines 290-295: Business context ONLY (attribution in structured columns)
	// Use structured audit payload (eliminates map[string]interface{})
	// Per DD-AUDIT-004: Zero unstructured data in audit events
	payload := api.WorkflowExecutionWebhookAuditPayload{
		EventType:     api.WorkflowExecutionWebhookAuditPayloadEventTypeWorkflowexecutionBlockCleared,
		WorkflowName:  wfe.Name,
		ClearReason:   wfe.Status.BlockClearance.ClearReason,
		ClearedAt:     wfe.Status.BlockClearance.ClearedAt.Time,
		PreviousState: api.WorkflowExecutionWebhookAuditPayloadPreviousStateBlocked,
		NewState:      api.WorkflowExecutionWebhookAuditPayloadNewStateRunning,
	}
	// Note: Attribution fields (WHO, WHAT, WHERE, HOW) are in structured columns:
	// - actor_id: authCtx.Username (via audit.SetActor)
	// - resource_name: wfe.Name (via audit.SetResource)
	// - namespace: wfe.Namespace (via audit.SetNamespace)
	// - event_action: "block_cleared" (via audit.SetEventAction)
	auditEvent.EventData = api.NewWorkflowExecutionWebhookAuditPayloadAuditEventRequestEventData(payload)

	// Store audit event asynchronously (buffered write)
	// Explicitly ignore errors - audit should not block webhook operations
	// The audit store has retry + DLQ mechanisms for reliability
	_ = h.auditStore.StoreAudit(ctx, auditEvent)

	// Marshal the patched object
	marshaledWFE, err := json.Marshal(wfe)
	if err != nil {
		return admission.Errored(http.StatusInternalServerError, fmt.Errorf("failed to marshal patched WorkflowExecution: %w", err))
	}

	// Return patched response
	return admission.PatchResponseFromRaw(req.Object.Raw, marshaledWFE)
}

// InjectDecoder injects the decoder into the handler
// Required by controller-runtime admission webhook framework
func (h *WorkflowExecutionAuthHandler) InjectDecoder(d admission.Decoder) error {
	h.decoder = d
	return nil
}

