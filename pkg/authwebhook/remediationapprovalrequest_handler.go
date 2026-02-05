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
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	ctrl "sigs.k8s.io/controller-runtime"
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
	// Pattern: Kubernaut Logging Standard (LOGGING_STANDARD.md)
	// CRD controllers use ctrl.Log for structured logging
	logger := ctrl.Log.WithName("rar-webhook")

	// LOG: Webhook invocation
	logger.Info("Webhook invoked",
		"operation", req.Operation,
		"namespace", req.Namespace,
		"name", req.Name,
	)

	rar := &remediationv1.RemediationApprovalRequest{}

	// Decode the RemediationApprovalRequest object from the request
	err := json.Unmarshal(req.Object.Raw, rar)
	if err != nil {
		logger.Error(err, "Failed to decode RemediationApprovalRequest")
		return admission.Errored(http.StatusBadRequest, fmt.Errorf("failed to decode RemediationApprovalRequest: %w", err))
	}

	// SECURITY: Decode OLD object to determine if this is a truly NEW decision
	// Per AUTHWEBHOOK_INT_COMPLETE_RCA_FEB_03_2026.md: OLD object comparison prevents identity forgery
	// SOC 2 CC8.1 (User Attribution), CC6.8 (Non-Repudiation)
	var oldRAR *remediationv1.RemediationApprovalRequest
	if len(req.OldObject.Raw) > 0 {
		oldRAR = &remediationv1.RemediationApprovalRequest{}
		if err := json.Unmarshal(req.OldObject.Raw, oldRAR); err != nil {
			logger.Error(err, "Failed to decode old RemediationApprovalRequest")
			return admission.Errored(http.StatusBadRequest, fmt.Errorf("failed to decode old RemediationApprovalRequest: %w", err))
		}
	}

	// LOG: Decision check (verbose for debugging)
	oldDecision := ""
	oldDecidedBy := ""
	if oldRAR != nil {
		oldDecision = string(oldRAR.Status.Decision)
		oldDecidedBy = oldRAR.Status.DecidedBy
	}
	logger.Info("Checking decision status",
		"newDecision", rar.Status.Decision,
		"newDecidedBy", rar.Status.DecidedBy,
		"oldDecision", oldDecision,
		"oldDecidedBy", oldDecidedBy,
	)

	// Check if a decision has been made
	if rar.Status.Decision == "" {
		// No decision yet - allow without modification
		logger.Info("Skipping RAR (no decision made)")
		return admission.Allowed("no decision made")
	}

	// REFACTOR-AW-001: Validate decision using extracted helper
	if err := ValidateApprovalDecision(rar.Status.Decision); err != nil {
		logger.Info("Rejecting RAR (invalid decision)",
			"decision", rar.Status.Decision,
			"error", err,
		)
		return admission.Denied(err.Error())
	}

	// SECURITY: TRUE Idempotency Check - Compare OLD object with NEW object
	// Per AUTHWEBHOOK_INT_COMPLETE_RCA_FEB_03_2026.md Issue #4:
	// - OLD object has decision → true idempotency (preserve existing attribution)
	// - OLD object has NO decision → NEW decision (OVERWRITE any user-provided DecidedBy)
	isNewDecision := oldRAR == nil || oldRAR.Status.Decision == ""

	if !isNewDecision {
		// Decision already exists in OLD object - preserve existing attribution (true idempotency)
		// This prevents duplicate webhook processing on the same decision
		logger.Info("Skipping RAR (decision already exists in old object) - TRUE IDEMPOTENCY",
			"oldDecision", oldRAR.Status.Decision,
			"oldDecidedBy", oldRAR.Status.DecidedBy,
			"newDecision", rar.Status.Decision,
		)
		return admission.Allowed("decision already attributed")
	}

	// SECURITY: This is a NEW decision - Extract authenticated user
	// Even if user provided DecidedBy in their request, webhook MUST overwrite with authenticated identity
	authCtx, err := h.authenticator.ExtractUser(ctx, &req.AdmissionRequest)
	if err != nil {
		logger.Error(err, "Authentication failed")
		return admission.Denied(fmt.Sprintf("authentication required: %v", err))
	}

	logger.Info("User authenticated",
		"username", authCtx.Username,
		"uid", authCtx.UID,
	)

	// REFACTOR-AW-002: Detect and log identity forgery attempts using extracted helper
	DetectAndLogForgeryAttempt(logger, rar.Status.DecidedBy, authCtx.Username)

	// SECURITY: Populate DecidedBy with authenticated user (OVERWRITE any user-provided value)
	// Per BR-AUTH-001, SOC 2 CC8.1: User attribution is tamper-proof (webhook-enforced)
	logger.Info("Populating DecidedBy field (authenticated identity)",
		"authenticatedUser", authCtx.Username,
		"decision", rar.Status.Decision,
	)
	rar.Status.DecidedBy = authCtx.Username // ALWAYS use authenticated user, never trust user input
	now := metav1.Now()
	rar.Status.DecidedAt = &now

	// Write complete audit event (DD-WEBHOOK-003: Webhook-Complete Audit Pattern)
	// Per ADR-034 v1.7 Section 1.1.1: Two-Event Pattern for RAR approvals
	// - Event 1 (Webhook): webhook.remediationapprovalrequest.decided (WHO - authenticated user)
	// - Event 2 (Orchestration): orchestrator.approval.{approved|rejected} (WHAT/WHY - business context)
	auditEvent := audit.NewAuditEventRequest()
	audit.SetEventType(auditEvent, "webhook.remediationapprovalrequest.decided") // Per ADR-034 v1.7
	audit.SetEventCategory(auditEvent, "webhook")                                // Per ADR-034 v1.7: event_category = emitter service
	audit.SetEventAction(auditEvent, "approval_decided")
	audit.SetEventOutcome(auditEvent, audit.OutcomeSuccess)
	audit.SetActor(auditEvent, "user", authCtx.Username)
	audit.SetResource(auditEvent, "RemediationApprovalRequest", string(rar.UID))
	// CRITICAL: Use parent RR name for correlation (DD-AUDIT-CORRELATION-002)
	// This ensures all RAR audit events (webhook + orchestration) share the same correlation_id
	parentRRName := rar.Spec.RemediationRequestRef.Name
	logger.Info("Setting correlation_id for audit event",
		"parentRRName", parentRRName,
		"rarName", rar.Name,
		"remediationRequestRef", rar.Spec.RemediationRequestRef)
	audit.SetCorrelationID(auditEvent, parentRRName)
	audit.SetNamespace(auditEvent, rar.Namespace)

	// REFACTOR-AW-003: Build audit payload using extracted helper
	// Per DD-WEBHOOK-003: Business context ONLY (attribution in structured columns)
	// Per DD-AUDIT-004: Zero unstructured data in audit events
	payload := BuildRARApprovalAuditPayload(rar)
	auditEvent.EventData = WrapRARApprovalPayloadWithDiscriminator(payload)
	// Note: Attribution fields (WHO, WHAT, WHERE, HOW) are in structured columns:
	// - actor_id: authCtx.Username (via audit.SetActor)
	// - resource_name: rar.Name (via audit.SetResource)
	// - namespace: rar.Namespace (via audit.SetNamespace)
	// - event_action: "approval_decided" (via audit.SetEventAction)

	// Store audit event asynchronously (buffered write)
	// Explicitly ignore errors - audit should not block webhook operations
	// The audit store has retry + DLQ mechanisms for reliability
	_ = h.auditStore.StoreAudit(ctx, auditEvent)

	// BUGFIX: Log correct correlationID (parent RR name, not RAR name)
	logger.Info("Webhook audit event emitted",
		"correlationID", parentRRName,
		"rarName", rar.Name,
		"eventAction", "approval_decided",
	)

	// Marshal the patched object
	marshaledRAR, err := json.Marshal(rar)
	if err != nil {
		logger.Error(err, "Failed to marshal patched RAR")
		return admission.Errored(http.StatusInternalServerError, fmt.Errorf("failed to marshal patched RemediationApprovalRequest: %w", err))
	}

	// LOG: Success
	logger.Info("RAR mutation complete",
		"decidedBy", rar.Status.DecidedBy,
		"decidedAt", rar.Status.DecidedAt.Time,
		"decision", rar.Status.Decision,
	)

	// Return patched response
	return admission.PatchResponseFromRaw(req.Object.Raw, marshaledRAR)
}

// InjectDecoder injects the decoder into the handler
// Required by controller-runtime admission webhook framework
func (h *RemediationApprovalRequestAuthHandler) InjectDecoder(d admission.Decoder) error {
	h.decoder = d
	return nil
}
