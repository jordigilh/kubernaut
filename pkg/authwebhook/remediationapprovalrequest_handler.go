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

	"github.com/go-logr/logr"
	remediationv1 "github.com/jordigilh/kubernaut/api/remediation/v1alpha1"
	remediationworkflowv1 "github.com/jordigilh/kubernaut/api/remediationworkflow/v1alpha1"
	"github.com/jordigilh/kubernaut/pkg/audit"
	sharedtypes "github.com/jordigilh/kubernaut/pkg/shared/types"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/webhook/admission"
)

// afServiceAccountName is the compile-time constant for the AF ServiceAccount name.
// This is the sole trusted intermediary for approval delegation (DD-AUTH-MCP-001 v3.0).
const afServiceAccountName = "apifrontend"

// BuildTrustedAFSA constructs the fully-qualified AF SA identity from a namespace.
// The SA name is a compile-time constant; only the namespace varies per deployment.
func BuildTrustedAFSA(namespace string) string {
	return fmt.Sprintf("system:serviceaccount:%s:%s", namespace, afServiceAccountName)
}

// RemediationApprovalRequestAuthHandler handles authentication for RemediationApprovalRequest decisions
// BR-AUTH-001: SOC2 CC8.1 Operator Attribution
// ADR-040: RemediationApprovalRequest CRD Architecture
// DD-WEBHOOK-003: Webhook-Complete Audit Pattern
//
// This mutating webhook intercepts RemediationApprovalRequest status updates and:
// 1. Populates status.DecidedBy (operator email/username)
// 2. Populates status.DecidedAt (timestamp)
// 3. Populates status.DecidedVia (trusted intermediary identity, if delegated)
// 4. Writes complete audit event (WHO + WHAT + ACTION)
type RemediationApprovalRequestAuthHandler struct {
	authenticator *Authenticator
	decoder       admission.Decoder
	auditStore    audit.AuditStore
	reader        client.Reader
	trustedAFSA   string
}

// NewRemediationApprovalRequestAuthHandler creates a new RemediationApprovalRequest authentication handler.
// The reader is used for override validation (F2: RW lookup, I1: use mgr.GetClient() in production).
// podNamespace is the namespace where AW + AF are colocated (used to derive the trusted AF SA identity).
func NewRemediationApprovalRequestAuthHandler(auditStore audit.AuditStore, reader client.Reader, podNamespace string) *RemediationApprovalRequestAuthHandler {
	return &RemediationApprovalRequestAuthHandler{
		authenticator: NewAuthenticator(),
		auditStore:    auditStore,
		reader:        reader,
		trustedAFSA:   BuildTrustedAFSA(podNamespace),
	}
}

// isAFServiceAccount returns true if the authenticated K8s caller is the trusted AF SA.
// This is the sole trusted intermediary for approval delegation (AC-6: Least Privilege).
func (h *RemediationApprovalRequestAuthHandler) isAFServiceAccount(username string) bool {
	return username == h.trustedAFSA
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

	rar, oldRAR, resp, done := h.decodeRARRequest(logger, req)
	if done {
		return resp
	}

	if earlyResp, earlyDone := earlyDecisionResponse(logger, rar, oldRAR); earlyDone {
		return earlyResp
	}

	authCtx, isDelegation, resp, done := h.authenticateAndAttribute(ctx, logger, rar, req)
	if done {
		return resp
	}

	if rar.Status.WorkflowOverride != nil {
		if resp, done := h.checkWorkflowOverride(ctx, logger, rar, authCtx); done {
			return resp
		}
	}

	now := metav1.Now()
	rar.Status.DecidedAt = &now
	logger.Info("Populating decision attribution",
		"decidedBy", rar.Status.DecidedBy,
		"decidedVia", rar.Status.DecidedVia,
		"decision", rar.Status.Decision,
		"isDelegation", isDelegation,
	)

	h.emitApprovalAuditEvent(ctx, logger, rar, authCtx, isDelegation)

	return finalizeRARResponse(logger, req, rar)
}

// decodeRARRequest decodes the new (and, if present, old) RemediationApprovalRequest
// objects from the admission request. done is true when an error response has
// already been returned and the caller must stop processing.
func (h *RemediationApprovalRequestAuthHandler) decodeRARRequest(logger logr.Logger, req admission.Request) (rar, oldRAR *remediationv1.RemediationApprovalRequest, resp admission.Response, done bool) {
	rar = &remediationv1.RemediationApprovalRequest{}
	if err := json.Unmarshal(req.Object.Raw, rar); err != nil {
		logger.Error(err, "Failed to decode RemediationApprovalRequest")
		return nil, nil, admission.Errored(http.StatusBadRequest, fmt.Errorf("failed to decode RemediationApprovalRequest: %w", err)), true
	}

	// SECURITY: Decode OLD object to determine if this is a truly NEW decision
	// Per AUTHWEBHOOK_INT_COMPLETE_RCA_FEB_03_2026.md: OLD object comparison prevents identity forgery
	// SOC 2 CC8.1 (User Attribution), CC6.8 (Non-Repudiation)
	if len(req.OldObject.Raw) > 0 {
		oldRAR = &remediationv1.RemediationApprovalRequest{}
		if err := json.Unmarshal(req.OldObject.Raw, oldRAR); err != nil {
			logger.Error(err, "Failed to decode old RemediationApprovalRequest")
			return nil, nil, admission.Errored(http.StatusBadRequest, fmt.Errorf("failed to decode old RemediationApprovalRequest: %w", err)), true
		}
	}

	oldDecision, oldDecidedBy := "", ""
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

	return rar, oldRAR, admission.Response{}, false
}

// earlyDecisionResponse handles the request states that terminate processing
// before authentication is needed: no decision yet, an invalid decision,
// true idempotency (decision already attributed), and system-initiated expiry.
func earlyDecisionResponse(logger logr.Logger, rar, oldRAR *remediationv1.RemediationApprovalRequest) (admission.Response, bool) {
	if rar.Status.Decision == "" {
		logger.Info("Skipping RAR (no decision made)")
		return admission.Allowed("no decision made"), true
	}

	// REFACTOR-AW-001: Validate decision using extracted helper
	if err := ValidateApprovalDecision(rar.Status.Decision); err != nil {
		logger.Info("Rejecting RAR (invalid decision)",
			"decision", rar.Status.Decision,
			"error", err,
		)
		return admission.Denied(err.Error()), true
	}

	// SECURITY: TRUE Idempotency Check - Compare OLD object with NEW object
	// Per AUTHWEBHOOK_INT_COMPLETE_RCA_FEB_03_2026.md Issue #4:
	// - OLD object has decision → true idempotency (preserve existing attribution)
	// - OLD object has NO decision → NEW decision (OVERWRITE any user-provided DecidedBy)
	isNewDecision := oldRAR == nil || oldRAR.Status.Decision == ""
	if !isNewDecision {
		logger.Info("Skipping RAR (decision already exists in old object) - TRUE IDEMPOTENCY",
			"oldDecision", oldRAR.Status.Decision,
			"oldDecidedBy", oldRAR.Status.DecidedBy,
			"newDecision", rar.Status.Decision,
		)
		return admission.Allowed("decision already attributed"), true
	}

	// CRD spec (remediationapprovalrequest_types.go:197): DecidedBy = "system" for timeout.
	// When the controller expires an RAR, it sets Decision=Expired and DecidedBy="system".
	// The webhook must preserve this value rather than overwriting with the controller's SA.
	if rar.Status.Decision == remediationv1.ApprovalDecisionExpired && rar.Status.DecidedBy == "system" {
		logger.Info("System-initiated expiry detected, preserving DecidedBy=system per CRD spec")
		return admission.Allowed("system-initiated expiry, DecidedBy preserved per CRD spec"), true
	}

	return admission.Response{}, false
}

// authenticateAndAttribute extracts the authenticated caller and applies
// attribution to rar.Status (delegation via the trusted AF SA, or standard
// forgery-prevention overwrite). done is true when authentication failed and
// the caller must return resp immediately.
func (h *RemediationApprovalRequestAuthHandler) authenticateAndAttribute(ctx context.Context, logger logr.Logger, rar *remediationv1.RemediationApprovalRequest, req admission.Request) (authCtx *AuthContext, isDelegation bool, resp admission.Response, done bool) {
	// SECURITY: This is a NEW decision - Extract authenticated user
	// Even if user provided DecidedBy in their request, webhook MUST overwrite with authenticated identity
	authCtx, err := h.authenticator.ExtractUser(ctx, &req.AdmissionRequest)
	if err != nil {
		logger.Error(err, "Authentication failed")
		return nil, false, admission.Denied(fmt.Sprintf("authentication required: %v", err)), true
	}

	logger.Info("User authenticated",
		"username", authCtx.Username,
		"uid", authCtx.UID,
	)

	// Trusted intermediary delegation (DD-AUTH-MCP-001 v3.0, AC-6: Least Privilege)
	// AF SA is the sole trusted intermediary — hardcoded, derived from POD_NAMESPACE.
	// When AF delegates, it has already authenticated the human via JWT+SAR.
	isDelegation = h.isAFServiceAccount(authCtx.Username) && rar.Status.DecidedBy != ""

	if isDelegation {
		// AF SA delegation: preserve human decidedBy, set decidedVia for operational visibility
		logger.Info("Trusted intermediary delegation accepted (AC-6)",
			"intermediary", authCtx.Username,
			"delegatedUser", rar.Status.DecidedBy,
		)
		rar.Status.DecidedVia = authCtx.Username
		// DecidedBy preserved from patch body (the human AF authenticated via JWT)
	} else {
		// Non-AF caller OR AF with empty decidedBy: standard forgery prevention
		DetectAndLogForgeryAttempt(logger, rar.Status.DecidedBy, authCtx.Username)
		rar.Status.DecidedBy = authCtx.Username
		rar.Status.DecidedVia = "" // AC-3: Clear any user-submitted value (prevent spoofing)
	}

	return authCtx, isDelegation, admission.Response{}, false
}

// checkWorkflowOverride validates a WorkflowOverride, if present, AFTER
// authentication (SOC2 CC8.1 — audit trail captures WHO attempted invalid
// overrides). done is true when validation failed and resp must be returned.
func (h *RemediationApprovalRequestAuthHandler) checkWorkflowOverride(ctx context.Context, logger logr.Logger, rar *remediationv1.RemediationApprovalRequest, authCtx *AuthContext) (admission.Response, bool) {
	overrideLog := logger.WithValues(
		"override.workflowName", rar.Status.WorkflowOverride.WorkflowName,
		"override.hasParams", rar.Status.WorkflowOverride.Parameters != nil,
		"override.rationale", rar.Status.WorkflowOverride.Rationale,
		"authenticatedUser", authCtx.Username,
	)
	if err := h.validateWorkflowOverride(ctx, rar); err != nil {
		overrideLog.Info("Override validation failed", "error", err.Error(), "decision", rar.Status.Decision)
		return admission.Denied(err.Error()), true
	}
	overrideLog.Info("Override validation passed")
	return admission.Response{}, false
}

// emitApprovalAuditEvent writes the webhook-complete audit event for the
// decision (DD-WEBHOOK-003, ADR-034 v1.7 two-event pattern, Event 1).
// Storage failures are non-blocking but logged for observability.
func (h *RemediationApprovalRequestAuthHandler) emitApprovalAuditEvent(ctx context.Context, logger logr.Logger, rar *remediationv1.RemediationApprovalRequest, authCtx *AuthContext, isDelegation bool) {
	auditEvent := audit.NewAuditEventRequest()
	audit.SetEventType(auditEvent, EventTypeRARDecided) // Per ADR-034 v1.7
	audit.SetEventCategory(auditEvent, EventCategoryApproval)
	audit.SetEventAction(auditEvent, "approval_decided")
	audit.SetEventOutcome(auditEvent, audit.OutcomeSuccess)
	// AU-3: For delegation, actor is the intermediary service (factual K8s caller).
	// For direct approvals, actor is the human user.
	if isDelegation {
		audit.SetActor(auditEvent, "service", authCtx.Username)
	} else {
		audit.SetActor(auditEvent, "user", authCtx.Username)
	}
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

	if err := h.auditStore.StoreAudit(ctx, auditEvent); err != nil {
		logger.Error(err, "Audit event storage failed (non-blocking)",
			"eventType", auditEvent.EventType,
			"eventAction", auditEvent.EventAction)
	}

	// BUGFIX: Log correct correlationID (parent RR name, not RAR name)
	logger.Info("Webhook audit event emitted",
		"correlationID", parentRRName,
		"rarName", rar.Name,
		"eventAction", "approval_decided",
	)
}

// finalizeRARResponse marshals the patched RAR and builds the admission
// patch response, or an error response if marshaling fails.
func finalizeRARResponse(logger logr.Logger, req admission.Request, rar *remediationv1.RemediationApprovalRequest) admission.Response {
	marshaledRAR, err := json.Marshal(rar)
	if err != nil {
		logger.Error(err, "Failed to marshal patched RAR")
		return admission.Errored(http.StatusInternalServerError, fmt.Errorf("failed to marshal patched RemediationApprovalRequest: %w", err))
	}

	logger.Info("RAR mutation complete",
		"decidedBy", rar.Status.DecidedBy,
		"decidedAt", rar.Status.DecidedAt.Time,
		"decision", rar.Status.Decision,
	)

	return admission.PatchResponseFromRaw(req.Object.Raw, marshaledRAR)
}

// validateWorkflowOverride validates the WorkflowOverride on an RAR (#594).
// - Override is only valid when decision is Approved
// - If workflowName is set, the referenced RW must exist and be Active
func (h *RemediationApprovalRequestAuthHandler) validateWorkflowOverride(ctx context.Context, rar *remediationv1.RemediationApprovalRequest) error {
	override := rar.Status.WorkflowOverride
	if override == nil {
		return nil
	}

	if rar.Status.Decision != remediationv1.ApprovalDecisionApproved {
		return fmt.Errorf("override rejected: workflowOverride is only valid when decision is Approved (got %q)", rar.Status.Decision)
	}

	if override.WorkflowName == "" {
		return nil
	}

	rw := &remediationworkflowv1.RemediationWorkflow{}
	err := h.reader.Get(ctx, client.ObjectKey{Name: override.WorkflowName, Namespace: rar.Namespace}, rw)
	if err != nil {
		if apierrors.IsNotFound(err) {
			return fmt.Errorf("override rejected: RemediationWorkflow %q not found in namespace %q", override.WorkflowName, rar.Namespace)
		}
		return fmt.Errorf("override rejected: failed to lookup RemediationWorkflow %q: %w", override.WorkflowName, err)
	}

	if rw.Status.CatalogStatus != sharedtypes.CatalogStatusActive {
		return fmt.Errorf("override rejected: RemediationWorkflow %q has catalogStatus %q (must be Active)", override.WorkflowName, rw.Status.CatalogStatus)
	}

	return nil
}

// InjectDecoder injects the decoder into the handler
// Required by controller-runtime admission webhook framework
func (h *RemediationApprovalRequestAuthHandler) InjectDecoder(d admission.Decoder) error {
	h.decoder = d
	return nil
}
