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
	"reflect"

	"github.com/go-logr/logr"
	remediationv1 "github.com/jordigilh/kubernaut/api/remediation/v1alpha1"
	remediationworkflowv1 "github.com/jordigilh/kubernaut/api/remediationworkflow/v1alpha1"
	"github.com/jordigilh/kubernaut/pkg/audit"
	sharedclock "github.com/jordigilh/kubernaut/pkg/clock"
	rarconditions "github.com/jordigilh/kubernaut/pkg/remediationapprovalrequest"
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

const roServiceAccountName = "remediationorchestrator-controller"

const systemDecisionActor = "system"

// BuildTrustedAFSA constructs the fully-qualified AF SA identity from a namespace.
// The SA name is a compile-time constant; only the namespace varies per deployment.
func BuildTrustedAFSA(namespace string) string {
	return fmt.Sprintf("system:serviceaccount:%s:%s", namespace, afServiceAccountName)
}

// BuildTrustedROSA constructs the fully-qualified RO service-account identity
// used for system-only expiry and cancellation decisions.
func BuildTrustedROSA(namespace string) string {
	return fmt.Sprintf("system:serviceaccount:%s:%s", namespace, roServiceAccountName)
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
	trustedROSA   string
	clock         sharedclock.Clock
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
		trustedROSA:   BuildTrustedROSA(podNamespace),
		clock:         sharedclock.RealClock{},
	}
}

// SetClock replaces the admission time source for deterministic deadline
// tests. Production uses the real UTC clock configured by the constructor.
func (h *RemediationApprovalRequestAuthHandler) SetClock(value sharedclock.Clock) {
	if value != nil {
		h.clock = value
	}
}

// isAFServiceAccount returns true if the authenticated K8s caller is the trusted AF SA.
// This is the sole trusted intermediary for approval delegation (AC-6: Least Privilege).
func (h *RemediationApprovalRequestAuthHandler) isAFServiceAccount(username string) bool {
	return username == h.trustedAFSA
}

func (h *RemediationApprovalRequestAuthHandler) isROServiceAccount(username string) bool {
	return username == h.trustedROSA
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

	if oldRAR != nil && oldRAR.Status.Decision != remediationv1.ApprovalDecisionPending {
		return h.handleTerminalRARUpdate(ctx, logger, rar, oldRAR, req)
	}

	// A status write with no business decision is not an approval attempt.
	// Preserve the existing no-op behavior for controller bookkeeping.
	if rar.Status.Decision == remediationv1.ApprovalDecisionPending {
		logger.Info("Skipping RAR (no decision made)")
		return admission.Allowed("no decision made")
	}

	authCtx, err := h.authenticateCaller(ctx, req)
	if err != nil {
		logger.Error(err, "Authentication failed")
		return admission.Denied(fmt.Sprintf("authentication required: %v", err))
	}
	return h.handleNewDecision(ctx, logger, req, rar, authCtx)
}

func (h *RemediationApprovalRequestAuthHandler) handleTerminalRARUpdate(ctx context.Context, logger logr.Logger, rar, oldRAR *remediationv1.RemediationApprovalRequest, req admission.Request) admission.Response {
	authCtx, err := h.authenticateCaller(ctx, req)
	if err != nil {
		logger.Error(err, "Authentication failed")
		return admission.Denied(fmt.Sprintf("authentication required: %v", err))
	}
	if isSystemOnlyDecision(oldRAR.Status.Decision) &&
		(!h.isROServiceAccount(authCtx.Username) || rar.Status.DecidedBy != systemDecisionActor) {
		return h.denyDecision(ctx, logger, rar, authCtx, "system-only decision requires the trusted remediation orchestrator")
	}
	if terminalBusinessTupleEqual(rar, oldRAR) && h.terminalBookkeepingUnchanged(oldRAR, rar, authCtx.Username) {
		logger.Info("Skipping RAR (decision already attributed) - exact terminal tuple")
		return admission.Allowed("decision already attributed")
	}
	return h.denyDecision(ctx, logger, rar, authCtx, "terminal decision is immutable")
}

func (h *RemediationApprovalRequestAuthHandler) handleNewDecision(ctx context.Context, logger logr.Logger, req admission.Request, rar *remediationv1.RemediationApprovalRequest, authCtx *AuthContext) admission.Response {
	if rar.DeletionTimestamp != nil {
		return h.denyDecision(ctx, logger, rar, authCtx, "cannot decide on a terminating RemediationApprovalRequest")
	}
	if err := ValidateApprovalDecision(rar.Status.Decision); err != nil {
		return h.denyDecision(ctx, logger, rar, authCtx, err.Error())
	}
	if isSystemOnlyDecision(rar.Status.Decision) {
		return h.handleSystemDecision(ctx, logger, req, rar, authCtx)
	}
	return h.handleHumanDecision(ctx, logger, req, rar, authCtx)
}

func (h *RemediationApprovalRequestAuthHandler) handleSystemDecision(ctx context.Context, logger logr.Logger, req admission.Request, rar *remediationv1.RemediationApprovalRequest, authCtx *AuthContext) admission.Response {
	if !h.isROServiceAccount(authCtx.Username) || rar.Status.DecidedBy != systemDecisionActor {
		return h.denyDecision(ctx, logger, rar, authCtx, "Expired and Cancelled decisions are system-only")
	}
	// The trusted RO identity, not a client field, establishes provenance.
	rar.Status.DecidedVia = ""
	now := metav1.NewTime(h.clock.Now())
	rar.Status.DecidedAt = &now
	h.emitApprovalAuditEvent(ctx, logger, rar, authCtx, false)
	return finalizeRARResponse(logger, req, rar)
}

func (h *RemediationApprovalRequestAuthHandler) handleHumanDecision(ctx context.Context, logger logr.Logger, req admission.Request, rar *remediationv1.RemediationApprovalRequest, authCtx *AuthContext) admission.Response {
	now := h.clock.Now()
	if rar.Spec.RequiredBy.IsZero() || !now.Before(rar.Spec.RequiredBy.Time) {
		return h.denyDecision(ctx, logger, rar, authCtx, "approval decision deadline has passed")
	}
	isDelegation := h.attributeDecision(logger, rar, authCtx)
	decidedAt := metav1.NewTime(now)
	rar.Status.DecidedAt = &decidedAt

	if rar.Status.WorkflowOverride != nil {
		if resp, done := h.checkWorkflowOverride(ctx, logger, rar, authCtx); done {
			return resp
		}
	}

	logger.Info("Populating decision attribution",
		"decidedBy", rar.Status.DecidedBy,
		"decidedVia", rar.Status.DecidedVia,
		"decision", rar.Status.Decision,
		"isDelegation", isDelegation,
	)
	setHumanDecisionConditions(rar)

	h.emitApprovalAuditEvent(ctx, logger, rar, authCtx, isDelegation)
	return finalizeRARResponse(logger, req, rar)
}

// setHumanDecisionConditions commits the terminal approval conditions in the
// same admission mutation as the business decision. The RO callback remains
// terminal-condition aware and does not rewrite this decision-bearing tuple.
func setHumanDecisionConditions(rar *remediationv1.RemediationApprovalRequest) {
	switch rar.Status.Decision {
	case remediationv1.ApprovalDecisionApproved:
		rarconditions.SetApprovalDecided(rar, true,
			rarconditions.ReasonApproved,
			fmt.Sprintf("Approved by %s", rar.Status.DecidedBy), nil)
	case remediationv1.ApprovalDecisionRejected:
		rarconditions.SetApprovalDecided(rar, true,
			rarconditions.ReasonRejected,
			fmt.Sprintf("Rejected by %s: %s", rar.Status.DecidedBy, rar.Status.DecisionMessage), nil)
	case remediationv1.ApprovalDecisionPending,
		remediationv1.ApprovalDecisionExpired,
		remediationv1.ApprovalDecisionCancelled:
		return
	}
	rarconditions.SetApprovalPending(rar, false, "Decision received", nil)
	rarconditions.SetReady(rar, true, rarconditions.ReasonReady, "Approval decided", nil)
}

func (h *RemediationApprovalRequestAuthHandler) authenticateCaller(ctx context.Context, req admission.Request) (*AuthContext, error) {
	return h.authenticator.ExtractUser(ctx, &req.AdmissionRequest)
}

func isSystemOnlyDecision(decision remediationv1.ApprovalDecision) bool {
	return decision == remediationv1.ApprovalDecisionExpired || decision == remediationv1.ApprovalDecisionCancelled
}

type terminalBusinessTuple struct {
	Decision         remediationv1.ApprovalDecision
	DecidedBy        string
	DecidedVia       string
	DecidedAt        *metav1.Time
	DecisionMessage  string
	WorkflowOverride *remediationv1.WorkflowOverride
	Expired          bool
	CreatedAt        *metav1.Time
	Reason           string
	Message          string
	Spec             remediationv1.RemediationApprovalRequestSpec
}

func terminalBusinessTupleOf(rar *remediationv1.RemediationApprovalRequest) terminalBusinessTuple {
	return terminalBusinessTuple{
		Decision:         rar.Status.Decision,
		DecidedBy:        rar.Status.DecidedBy,
		DecidedVia:       rar.Status.DecidedVia,
		DecidedAt:        rar.Status.DecidedAt,
		DecisionMessage:  rar.Status.DecisionMessage,
		WorkflowOverride: rar.Status.WorkflowOverride,
		Expired:          rar.Status.Expired,
		CreatedAt:        rar.Status.CreatedAt,
		Reason:           rar.Status.Reason,
		Message:          rar.Status.Message,
		Spec:             rar.Spec,
	}
}

func terminalBusinessTupleEqual(left, right *remediationv1.RemediationApprovalRequest) bool {
	if left == nil || right == nil {
		return false
	}
	return reflect.DeepEqual(terminalBusinessTupleOf(left), terminalBusinessTupleOf(right))
}

func (h *RemediationApprovalRequestAuthHandler) terminalBookkeepingUnchanged(oldRAR, rar *remediationv1.RemediationApprovalRequest, username string) bool {
	if oldRAR.Status.TimeRemaining != rar.Status.TimeRemaining {
		return false
	}
	if !h.isROServiceAccount(username) {
		return oldRAR.Status.ObservedGeneration == rar.Status.ObservedGeneration &&
			reflect.DeepEqual(oldRAR.Status.Conditions, rar.Status.Conditions)
	}
	if rar.Status.ObservedGeneration < oldRAR.Status.ObservedGeneration {
		return false
	}
	return terminalConditionsAllowAuditBookkeeping(oldRAR, rar)
}

func (h *RemediationApprovalRequestAuthHandler) attributeDecision(logger logr.Logger, rar *remediationv1.RemediationApprovalRequest, authCtx *AuthContext) bool {
	isDelegation := h.isAFServiceAccount(authCtx.Username) && rar.Status.DecidedBy != ""
	if isDelegation {
		logger.Info("Trusted intermediary delegation accepted (AC-6)",
			"intermediary", authCtx.Username,
			"delegatedUser", rar.Status.DecidedBy,
		)
		rar.Status.DecidedVia = authCtx.Username
		return true
	}
	DetectAndLogForgeryAttempt(logger, rar.Status.DecidedBy, authCtx.Username)
	rar.Status.DecidedBy = authCtx.Username
	rar.Status.DecidedVia = ""
	return false
}

func (h *RemediationApprovalRequestAuthHandler) denyDecision(ctx context.Context, logger logr.Logger, rar *remediationv1.RemediationApprovalRequest, authCtx *AuthContext, reason string) admission.Response {
	logger.Info("RAR decision denied", "reason", reason, "decision", rar.Status.Decision)
	isDelegation := authCtx != nil && h.isAFServiceAccount(authCtx.Username) && rar.Status.DecidedBy != "" && rar.Status.DecidedBy != systemDecisionActor
	h.emitDeniedApprovalAuditEvent(ctx, logger, rar, authCtx, isDelegation, reason)
	return admission.Denied(reason)
}

func terminalConditionsAllowAuditBookkeeping(oldRAR, rar *remediationv1.RemediationApprovalRequest) bool {
	oldConditions, oldOK := conditionMap(oldRAR.Status.Conditions)
	newConditions, newOK := conditionMap(rar.Status.Conditions)
	if !oldOK || !newOK {
		return false
	}

	oldAudit, hadOldAudit := oldConditions[rarconditions.ConditionAuditRecorded]
	newAudit, hasNewAudit := newConditions[rarconditions.ConditionAuditRecorded]
	if hasNewAudit && !validAuditRecordedCondition(newAudit, rar.Generation) {
		return false
	}
	if hadOldAudit {
		if !hasNewAudit {
			return false
		}
		if oldAudit.Status == metav1.ConditionTrue && !reflect.DeepEqual(oldAudit, newAudit) {
			return false
		}
	}

	delete(oldConditions, rarconditions.ConditionAuditRecorded)
	delete(newConditions, rarconditions.ConditionAuditRecorded)
	return reflect.DeepEqual(oldConditions, newConditions)
}

func conditionMap(conditions []metav1.Condition) (map[string]metav1.Condition, bool) {
	result := make(map[string]metav1.Condition, len(conditions))
	for _, condition := range conditions {
		if condition.Type == "" {
			return nil, false
		}
		if _, exists := result[condition.Type]; exists {
			return nil, false
		}
		result[condition.Type] = condition
	}
	return result, true
}

func validAuditRecordedCondition(condition metav1.Condition, generation int64) bool {
	if condition.Type != rarconditions.ConditionAuditRecorded ||
		condition.ObservedGeneration != generation ||
		condition.LastTransitionTime.IsZero() ||
		condition.Message == "" {
		return false
	}
	switch condition.Status {
	case metav1.ConditionTrue:
		return condition.Reason == rarconditions.ReasonAuditSucceeded
	case metav1.ConditionFalse:
		return condition.Reason == rarconditions.ReasonAuditFailed
	default:
		return false
	}
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
	if h.auditStore == nil {
		logger.Info("Approval audit event accepted without configured audit store", "rar", rar.Name)
		return
	}
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
	audit.SetClusterID(auditEvent, rar.Spec.ClusterID)

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

// emitDeniedApprovalAuditEvent records an authenticated denial attempt. The
// event is best effort, matching the existing asynchronous audit boundary; it
// never turns an admission denial into a fabricated actor or a blocking flush.
func (h *RemediationApprovalRequestAuthHandler) emitDeniedApprovalAuditEvent(ctx context.Context, logger logr.Logger, rar *remediationv1.RemediationApprovalRequest, authCtx *AuthContext, isDelegation bool, reason string) {
	if h.auditStore == nil || authCtx == nil {
		return
	}
	event := audit.NewAuditEventRequest()
	audit.SetEventType(event, EventTypeRARDecided)
	audit.SetEventCategory(event, EventCategoryApproval)
	audit.SetEventAction(event, "approval_decision_denied")
	audit.SetEventOutcome(event, audit.OutcomeFailure)
	if isDelegation {
		audit.SetActor(event, "service", authCtx.Username)
	} else {
		audit.SetActor(event, "user", authCtx.Username)
	}
	audit.SetResource(event, "RemediationApprovalRequest", string(rar.UID))
	audit.SetCorrelationID(event, rar.Spec.RemediationRequestRef.Name)
	audit.SetNamespace(event, rar.Namespace)
	audit.SetClusterID(event, rar.Spec.ClusterID)
	// The normal typed payload remains useful for forensic context. Denied
	// attempts may not carry a client timestamp, so use a local observation
	// timestamp on a copy rather than dereferencing a nil status field.
	payloadRAR := rar.DeepCopy()
	payloadRAR.Status.DecisionMessage = reason
	if payloadRAR.Status.DecidedAt == nil {
		now := metav1.NewTime(h.clock.Now())
		payloadRAR.Status.DecidedAt = &now
	}
	event.EventData = WrapRARApprovalPayloadWithDiscriminator(BuildRARApprovalAuditPayload(payloadRAR))
	if err := h.auditStore.StoreAudit(ctx, event); err != nil {
		logger.Error(err, "Denied approval audit enqueue failed", "reason", reason)
	}
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
