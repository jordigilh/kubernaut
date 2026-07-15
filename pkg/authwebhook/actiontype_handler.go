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
	"strings"
	"time"

	atv1alpha1 "github.com/jordigilh/kubernaut/api/actiontype/v1alpha1"
	"github.com/jordigilh/kubernaut/pkg/audit"
	ogenclient "github.com/jordigilh/kubernaut/pkg/datastorage/ogen-client"
	sharedtypes "github.com/jordigilh/kubernaut/pkg/shared/types"
	"github.com/jordigilh/kubernaut/pkg/shared/types/ogenconv"
	admissionv1 "k8s.io/api/admission/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/util/retry"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/webhook/admission"
)

// ActionTypeHandler handles admission requests for ActionType CRD
// CREATE, UPDATE, and DELETE operations. #1661 Change 8d: CRD lifecycle is no
// longer bridged to a DS action-type catalog -- registered/catalogStatus/
// dependents-on-delete are all computed locally against the live etcd state
// (via the cache-backed k8sClient), zero DS round-trips (DD-WORKFLOW-018).
// BR-WORKFLOW-007.
type ActionTypeHandler struct {
	dsClient      ActionTypeCatalogClient // vestigial; see ActionTypeCatalogClient doc
	auditStore    audit.AuditStore
	k8sClient     client.Client
	authenticator *Authenticator
}

// NewActionTypeHandler creates a handler for ActionType admission.
func NewActionTypeHandler(
	dsClient ActionTypeCatalogClient,
	auditStore audit.AuditStore,
	k8sClient client.Client,
) *ActionTypeHandler {
	return &ActionTypeHandler{
		dsClient:      dsClient,
		auditStore:    auditStore,
		k8sClient:     k8sClient,
		authenticator: NewAuthenticator(),
	}
}

// Handle processes admission requests for ActionType CRD.
// Intercepts CREATE, UPDATE, and DELETE per BR-WORKFLOW-007.
func (h *ActionTypeHandler) Handle(ctx context.Context, req admission.Request) admission.Response {
	switch req.Operation {
	case admissionv1.Create:
		return h.handleCreate(ctx, req)
	case admissionv1.Update:
		return h.handleUpdate(ctx, req)
	case admissionv1.Delete:
		return h.handleDelete(ctx, req)
	default:
		return admission.Allowed("operation not intercepted")
	}
}

func (h *ActionTypeHandler) handleCreate(ctx context.Context, req admission.Request) admission.Response {
	logger := ctrl.Log.WithName("at-webhook").WithValues("operation", "CREATE", "name", req.Name, "namespace", req.Namespace)

	at := &atv1alpha1.ActionType{}
	if err := json.Unmarshal(req.Object.Raw, at); err != nil {
		logger.Error(err, "Failed to unmarshal ActionType")
		return admission.Denied(fmt.Sprintf("failed to unmarshal ActionType: %v", err))
	}

	authCtx, err := h.authenticator.ExtractUser(ctx, &req.AdmissionRequest)
	if err != nil {
		logger.Error(err, "Authentication failed")
		return admission.Denied(fmt.Sprintf("authentication required: %v", err))
	}

	// #1661 Change 8d: registration is a pure local computation -- no DS
	// round-trip. There is no external catalog state to defer to (see
	// DD-WORKFLOW-018), so PreviouslyExisted is always false: once DELETE
	// performs a true etcd removal with no "disabled" intermediate state,
	// there is no local way (or need) to distinguish "brand new" from
	// "recreated after deletion".
	logger.Info("ActionType registered locally", "action_type", at.Spec.Name)
	h.emitATAdmitAudit(ctx, req, EventTypeATAdmittedCreate, at.Spec.Name, false, "Active")

	go h.updateCRDStatusCreate(req.Namespace, req.Name, authCtx.Username)

	return admission.Allowed("action type registered")
}

func (h *ActionTypeHandler) handleUpdate(ctx context.Context, req admission.Request) admission.Response {
	logger := ctrl.Log.WithName("at-webhook").WithValues("operation", "UPDATE", "name", req.Name, "namespace", req.Namespace)

	oldAT := &atv1alpha1.ActionType{}
	if err := json.Unmarshal(req.OldObject.Raw, oldAT); err != nil {
		logger.Error(err, "Failed to unmarshal old ActionType")
		return admission.Denied(fmt.Sprintf("failed to unmarshal old ActionType: %v", err))
	}

	newAT := &atv1alpha1.ActionType{}
	if err := json.Unmarshal(req.Object.Raw, newAT); err != nil {
		logger.Error(err, "Failed to unmarshal new ActionType")
		return admission.Denied(fmt.Sprintf("failed to unmarshal new ActionType: %v", err))
	}

	// BR-WORKFLOW-007.2: spec.name is immutable
	if oldAT.Spec.Name != newAT.Spec.Name {
		h.emitATDeniedAudit(ctx, req, "spec.name is immutable", "UPDATE")
		return admission.Denied(fmt.Sprintf("spec.name is immutable: cannot change from %q to %q", oldAT.Spec.Name, newAT.Spec.Name))
	}

	// Check if description actually changed
	if oldAT.Spec.Description == newAT.Spec.Description {
		return admission.Allowed("no description change")
	}

	if _, err := h.authenticator.ExtractUser(ctx, &req.AdmissionRequest); err != nil {
		logger.Error(err, "Authentication failed")
		return admission.Denied(fmt.Sprintf("authentication required: %v", err))
	}

	// #1661 Change 8d: description updates are local-only -- there is no DS
	// catalog left to notify (DD-WORKFLOW-018).
	logger.Info("ActionType description updated locally", "action_type", newAT.Spec.Name)
	h.emitATAdmitAudit(ctx, req, EventTypeATAdmittedUpdate, newAT.Spec.Name, false, "Active")

	return admission.Allowed("action type description updated")
}

func (h *ActionTypeHandler) handleDelete(ctx context.Context, req admission.Request) admission.Response {
	logger := ctrl.Log.WithName("at-webhook").WithValues("operation", "DELETE", "name", req.Name, "namespace", req.Namespace)

	at := &atv1alpha1.ActionType{}
	if err := json.Unmarshal(req.OldObject.Raw, at); err != nil {
		logger.Error(err, "Failed to unmarshal ActionType from OldObject")
		return admission.Allowed("delete allowed (unmarshal failed, best-effort)")
	}

	// #1661 Change 8d: the live etcd list (via the cache-backed k8sClient) is
	// now the SOLE dependents gate -- DS's Postgres catalog stopped learning
	// about RemediationWorkflow CRDs the moment Change 8c removed AW's
	// CreateWorkflowInline call, making any DS-backed check permanently
	// blind to workflows created afterward (DD-WORKFLOW-018,
	// IT-AW-1111-009). There is no DS-side "orphan" state left to
	// reconcile, so there is no recovery path to attempt: what K8s reports
	// IS the dependents set.
	dependents, err := listDependentWorkflowNames(ctx, h.k8sClient, at.Spec.Name, "")
	if err != nil {
		logger.Error(err, "Failed to list dependent RemediationWorkflows")
		msg := fmt.Sprintf("failed to check dependent workflows: %v", err)
		h.emitATDeniedAudit(ctx, req, msg, "DELETE")
		return admission.Denied(msg)
	}

	if len(dependents) > 0 {
		msg := fmt.Sprintf("Cannot delete ActionType %q: %d active workflow(s) depend on it (%s)",
			at.Spec.Name, len(dependents), strings.Join(dependents, ", "))
		logger.Info("ActionType delete denied", "reason", msg)
		h.emitATDeniedAudit(ctx, req, msg, "DELETE")
		return admission.Denied(msg)
	}

	logger.Info("ActionType has no dependent workflows, deletion allowed", "action_type", at.Spec.Name)
	h.emitATAdmitAudit(ctx, req, EventTypeATAdmittedDelete, at.Spec.Name, false, "Disabled")
	return admission.Allowed("action type deleted")
}

// updateCRDStatusCreate writes the locally-computed registration outcome
// into the CRD's .status subresource. #1661 Change 8d: catalogStatus is
// always Active and previouslyExisted is always false -- there is no DS
// response left to carry (see handleCreate for why). Uses RetryOnConflict
// to handle the race between this goroutine and the API server committing
// the newly-created object.
func (h *ActionTypeHandler) updateCRDStatusCreate(namespace, name, registeredBy string) {
	logger := ctrl.Log.WithName("at-webhook").WithValues("operation", "status-update", "name", name, "namespace", namespace)

	if h.k8sClient == nil {
		logger.Info("k8sClient not configured — skipping status update")
		return
	}

	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	key := types.NamespacedName{Namespace: namespace, Name: name}

	err := retry.RetryOnConflict(retry.DefaultBackoff, func() error {
		at := &atv1alpha1.ActionType{}
		if err := h.k8sClient.Get(ctx, key, at); err != nil {
			return err
		}

		now := metav1.Now()
		at.Status.Registered = true
		at.Status.CatalogStatus = sharedtypes.CatalogStatusActive
		at.Status.RegisteredBy = registeredBy
		at.Status.RegisteredAt = &now
		at.Status.PreviouslyExisted = false

		return h.k8sClient.Status().Update(ctx, at)
	})
	if err != nil {
		logger.Error(err, "Failed to update CRD status after retries", "action_type", name)
		return
	}

	logger.Info("ActionType CRD status updated", "action_type", name)
}

// crdDescriptionToOgen converts a CRD ActionTypeDescription to the ogen-generated type.
func crdDescriptionToOgen(d atv1alpha1.ActionTypeDescription) ogenclient.ActionTypeDescription {
	return ogenconv.SharedDescriptionToOgen(atv1alpha1.DescriptionToShared(d))
}
