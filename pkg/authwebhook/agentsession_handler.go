/*
Copyright 2026 Jordi Gil.

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

	agentsessionv1alpha1 "github.com/jordigilh/kubernaut/api/agentsession/v1alpha1"
	remediationv1 "github.com/jordigilh/kubernaut/api/remediation/v1alpha1"
	"github.com/jordigilh/kubernaut/pkg/audit"
	api "github.com/jordigilh/kubernaut/pkg/datastorage/ogen-client"
	admissionv1 "k8s.io/api/admission/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/webhook/admission"
)

// AgentSessionHandler handles admission requests for AgentSession CRD CREATE
// operations. Issue #2244, BR-AA-KA-065.13: a cross-resource existence gate
// mirroring #1661's validateActionTypeExists pattern (DD-WORKFLOW-018) --
// denies CREATE when Spec.RemediationRequestRef does not resolve to a real
// RemediationRequest in the same namespace (SI-10: input validation at the
// trust boundary, before KA's dispatcher ever observes the object).
//
// No UPDATE/DELETE gate is needed: AgentSessionSpec carries
// +kubebuilder:validation:XValidation:rule="self == oldSelf" (CEL-immutable),
// so RemediationRequestRef cannot change post-create.
//
// Unlike RemediationWorkflowHandler/ActionTypeHandler, this handler never
// writes .status -- AgentSession.Status is exclusively KA-owned
// (DD-AA-KA-001) -- so the audit-emit below is synchronous, before the
// admission response returns, with no async goroutine.
type AgentSessionHandler struct {
	auditStore audit.AuditStore
	k8sClient  client.Reader
}

// NewAgentSessionHandler creates a handler for AgentSession admission.
//
// k8sClient is the reader used to resolve RemediationRequestRef. Production
// MUST pass the manager's direct API reader (mgr.GetAPIReader()), not its
// cache-backed client (#2674): this is an admission-time existence gate, so a
// cache that has not started yet or is lagging turns a real RemediationRequest
// into a denial -- see validateRemediationRequestExists.
func NewAgentSessionHandler(auditStore audit.AuditStore, k8sClient client.Reader) *AgentSessionHandler {
	return &AgentSessionHandler{
		auditStore: auditStore,
		k8sClient:  k8sClient,
	}
}

// Handle processes admission requests for AgentSession CRD. Only CREATE is
// intercepted -- see the type doc comment for why UPDATE/DELETE need no gate.
func (h *AgentSessionHandler) Handle(ctx context.Context, req admission.Request) admission.Response {
	if req.Operation != admissionv1.Create {
		return admission.Allowed("operation not intercepted")
	}
	return h.handleCreate(ctx, req)
}

// handleCreate denies CREATE when Spec.RemediationRequestRef does not
// resolve to a real RemediationRequest in req.Namespace.
func (h *AgentSessionHandler) handleCreate(ctx context.Context, req admission.Request) admission.Response {
	logger := ctrl.Log.WithName("as-webhook").WithValues("operation", "CREATE", "name", req.Name, "namespace", req.Namespace)

	as := &agentsessionv1alpha1.AgentSession{}
	if err := json.Unmarshal(req.Object.Raw, as); err != nil {
		logger.Error(err, "Failed to unmarshal AgentSession")
		h.emitDeniedAudit(ctx, req, fmt.Sprintf("failed to unmarshal AgentSession: %v", err))
		return admission.Denied(fmt.Sprintf("failed to unmarshal AgentSession: %v", err))
	}

	if err := h.validateRemediationRequestExists(ctx, as.Spec.RemediationRequestRef.Name, req.Namespace); err != nil {
		logger.Error(err, "RemediationRequest existence check failed", "remediationRequestRef", as.Spec.RemediationRequestRef.Name)
		h.emitDeniedAudit(ctx, req, err.Error())
		return admission.Denied(err.Error())
	}

	logger.Info("AgentSession admitted", "remediationRequestRef", as.Spec.RemediationRequestRef.Name)
	h.emitAdmitAudit(ctx, req, as.Spec.RemediationRequestRef.Name)

	return admission.Allowed("agent session admitted")
}

// validateRemediationRequestExists checks that rrName resolves to a real
// RemediationRequest in namespace via a direct Get -- RemediationRequestRef
// is a plain ObjectRef{Name, Namespace}, so no field indexer is needed
// (unlike ActionType's list-by-spec.name lookup). For the same reason the
// reader needs no cache: an index-free direct read is exactly what this gate
// wants, so production wires mgr.GetAPIReader() (#2674).
//
// The read must NOT go through the manager's cache-backed client. Two cache
// windows break this gate (#2674): before the cache starts, Get returns
// &cache.ErrCacheNotStarted{} (controller-runtime starts webhook listeners
// before caches, and /readyz does not gate on cache sync), and after it starts
// a lagging informer reports a freshly created RemediationRequest as NotFound.
// Both deny a legitimate AgentSession, and both misattribute the cause in the
// denial audit event (BR-AA-KA-065.13, SI-10, AU-3).
//
// h.k8sClient is nil in unit tests that don't exercise this gate
// (production always wires the manager's direct API reader in
// cmd/authwebhook/main.go); the check is skipped rather than denied in that
// case, matching the existing best-effort precedent of
// RemediationWorkflowHandler.validateActionTypeExists.
func (h *AgentSessionHandler) validateRemediationRequestExists(ctx context.Context, rrName, namespace string) error {
	if h.k8sClient == nil {
		return nil
	}

	rr := &remediationv1.RemediationRequest{}
	key := types.NamespacedName{Name: rrName, Namespace: namespace}
	if err := h.k8sClient.Get(ctx, key, rr); err != nil {
		if apierrors.IsNotFound(err) {
			return fmt.Errorf("remediationRequestRef %q does not exist in namespace %q", rrName, namespace)
		}
		return fmt.Errorf("remediation request lookup failed for %q: %w", rrName, err)
	}
	return nil
}

// emitAdmitAudit emits a success audit event for an admitted AgentSession CREATE.
func (h *AgentSessionHandler) emitAdmitAudit(ctx context.Context, req admission.Request, rrName string) {
	if h.auditStore == nil {
		return
	}

	event := buildAuditEnvelope(req, WebhookAuditOpts{
		EventType:    EventTypeASAdmittedCreate,
		Category:     EventCategoryAgentSession,
		Action:       "admitted",
		Outcome:      api.AuditEventRequestEventOutcomeSuccess,
		ResourceKind: "AgentSession",
		ResourceID:   req.Name,
	})

	payload := api.AgentSessionWebhookAuditPayload{
		EventType:    api.AgentSessionWebhookAuditPayloadEventTypeAgentsessionAdmittedCreate,
		CrdName:      req.Name,
		CrdNamespace: req.Namespace,
		Action:       api.AgentSessionWebhookAuditPayloadActionCreate,
	}
	payload.RemediationRequestRef.SetTo(rrName)
	event.EventData = api.NewAuditEventRequestEventDataAgentsessionAdmittedCreateAuditEventRequestEventData(payload)

	storeAuditBestEffort(ctx, h.auditStore, event, "as-webhook", EventTypeASAdmittedCreate)
}

// emitDeniedAudit emits a denied audit event when an AgentSession CREATE is rejected.
func (h *AgentSessionHandler) emitDeniedAudit(ctx context.Context, req admission.Request, reason string) {
	if h.auditStore == nil {
		return
	}

	event := buildAuditEnvelope(req, WebhookAuditOpts{
		EventType:    EventTypeASDeniedCreate,
		Category:     EventCategoryAgentSession,
		Action:       "denied",
		Outcome:      api.AuditEventRequestEventOutcomeFailure,
		ResourceKind: "AgentSession",
		ResourceID:   req.Name,
	})

	payload := api.AgentSessionWebhookAuditPayload{
		EventType:    api.AgentSessionWebhookAuditPayloadEventTypeAgentsessionDeniedCreate,
		CrdName:      req.Name,
		CrdNamespace: req.Namespace,
		Action:       api.AgentSessionWebhookAuditPayloadActionDenied,
	}
	payload.DenialReason.SetTo(reason)
	event.EventData = api.NewAuditEventRequestEventDataAgentsessionDeniedCreateAuditEventRequestEventData(payload)

	storeAuditBestEffort(ctx, h.auditStore, event, "as-webhook", EventTypeASDeniedCreate)
}
