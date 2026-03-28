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

	"github.com/go-logr/logr"
	atv1alpha1 "github.com/jordigilh/kubernaut/api/actiontype/v1alpha1"
	rwv1alpha1 "github.com/jordigilh/kubernaut/api/remediationworkflow/v1alpha1"
	"github.com/jordigilh/kubernaut/pkg/audit"
	ogenclient "github.com/jordigilh/kubernaut/pkg/datastorage/ogen-client"
	"github.com/jordigilh/kubernaut/pkg/shared/types/ogenconv"
	admissionv1 "k8s.io/api/admission/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/webhook/admission"
)

// ActionTypeHandler handles admission requests for ActionType CRD
// CREATE, UPDATE, and DELETE operations, bridging CRD lifecycle with the DS catalog.
// BR-WORKFLOW-007, DD-ACTIONTYPE-001, ADR-059.
type ActionTypeHandler struct {
	dsClient      ActionTypeCatalogClient
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

	result, err := h.dsClient.CreateActionType(ctx, at.Spec.Name, crdDescriptionToOgen(at.Spec.Description), authCtx.Username)
	if err != nil {
		logger.Error(err, "DS CreateActionType failed")
		h.emitATDeniedAudit(ctx, req, fmt.Sprintf("DS registration failed: %v", err), "CREATE")
		return admission.Denied(fmt.Sprintf("data storage registration failed: %v", err))
	}

	logger.Info("ActionType registered in DS",
		"action_type", at.Spec.Name,
		"status", result.Status,
		"was_reenabled", result.WasReenabled,
	)

	h.emitATAdmitAudit(ctx, req, EventTypeATAdmittedCreate, at.Spec.Name, result.WasReenabled, "Active")

	go h.updateCRDStatusCreate(req.Namespace, req.Name, authCtx.Username, result)

	return admission.Allowed("action type registered in catalog")
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

	authCtx, err := h.authenticator.ExtractUser(ctx, &req.AdmissionRequest)
	if err != nil {
		logger.Error(err, "Authentication failed")
		return admission.Denied(fmt.Sprintf("authentication required: %v", err))
	}

	_, err = h.dsClient.UpdateActionType(ctx, newAT.Spec.Name, crdDescriptionToOgen(newAT.Spec.Description), authCtx.Username)
	if err != nil {
		logger.Error(err, "DS UpdateActionType failed")
		h.emitATDeniedAudit(ctx, req, fmt.Sprintf("DS update failed: %v", err), "UPDATE")
		return admission.Denied(fmt.Sprintf("data storage update failed: %v", err))
	}

	logger.Info("ActionType description updated in DS", "action_type", newAT.Spec.Name)
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

	username := req.UserInfo.Username
	if authCtx, err := h.authenticator.ExtractUser(ctx, &req.AdmissionRequest); err == nil {
		username = authCtx.Username
	}

	result, err := h.dsClient.DisableActionType(ctx, at.Spec.Name, username)
	if err != nil {
		logger.Error(err, "DS DisableActionType failed")
		h.emitATDeniedAudit(ctx, req, fmt.Sprintf("DS disable failed: %v", err), "DELETE")
		return admission.Denied(fmt.Sprintf("data storage disable failed: %v", err))
	}

	if !result.Disabled {
		msg := fmt.Sprintf("Cannot delete ActionType %q: %d active workflow(s) depend on it (%s)",
			at.Spec.Name, result.DependentWorkflowCount,
			strings.Join(result.DependentWorkflows, ", "))

		// Issue #512: Cross-check with K8s to detect orphaned DS entries.
		if recovered := h.attemptOrphanRecovery(ctx, logger, req, at, username, result); recovered {
			return admission.Allowed("action type disabled in catalog (orphan recovery)")
		}

		logger.Info("ActionType disable denied", "reason", msg)
		h.emitATDeniedAudit(ctx, req, msg, "DELETE")
		return admission.Denied(msg)
	}

	logger.Info("ActionType disabled in DS", "action_type", at.Spec.Name)
	h.emitATAdmitAudit(ctx, req, EventTypeATAdmittedDelete, at.Spec.Name, false, "Disabled")
	return admission.Allowed("action type disabled in catalog")
}

// attemptOrphanRecovery cross-checks DS-reported dependent workflows against
// live K8s RemediationWorkflow CRDs. If some dependents no longer exist in K8s,
// they are orphaned DS entries. The method calls ForceDisableActionType to clean
// them up. Returns true if the action type was successfully disabled.
// Issue #512: Prevents permanently undeletable ActionTypes.
func (h *ActionTypeHandler) attemptOrphanRecovery(
	ctx context.Context,
	logger logr.Logger,
	req admission.Request,
	at *atv1alpha1.ActionType,
	username string,
	result *ActionTypeDisableResult,
) bool {
	if h.k8sClient == nil {
		return false
	}

	rwList := &rwv1alpha1.RemediationWorkflowList{}
	if err := h.k8sClient.List(ctx, rwList, client.InNamespace(req.Namespace)); err != nil {
		logger.Error(err, "Failed to list RemediationWorkflows for orphan check")
		return false
	}

	liveNames := make(map[string]struct{})
	for i := range rwList.Items {
		if rwList.Items[i].Spec.ActionType == at.Spec.Name {
			liveNames[rwList.Items[i].Name] = struct{}{}
		}
	}

	var orphaned []string
	for _, depName := range result.DependentWorkflows {
		if _, live := liveNames[depName]; !live {
			orphaned = append(orphaned, depName)
		}
	}

	if len(orphaned) == 0 {
		return false
	}

	logger.Info("Orphaned DS workflows detected, attempting force-disable",
		"orphaned", orphaned,
		"live_count", len(liveNames),
		"action_type", at.Spec.Name,
	)

	forceResult, err := h.dsClient.ForceDisableActionType(ctx, at.Spec.Name, username, orphaned)
	if err != nil {
		logger.Error(err, "Force-disable failed during orphan recovery")
		return false
	}
	if !forceResult.Disabled {
		logger.Info("Force-disable denied — non-orphaned workflows remain",
			"remaining", forceResult.DependentWorkflows)
		return false
	}

	logger.Info("ActionType disabled via orphan recovery", "action_type", at.Spec.Name)
	h.emitATAdmitAudit(ctx, req, EventTypeATAdmittedDelete, at.Spec.Name, false, "disabled")
	return true
}

// updateCRDStatusCreate writes the DS registration result into the CRD's .status subresource.
func (h *ActionTypeHandler) updateCRDStatusCreate(namespace, name, registeredBy string, result *ActionTypeRegistrationResult) {
	logger := ctrl.Log.WithName("at-webhook").WithValues("operation", "status-update", "name", name, "namespace", namespace)

	if h.k8sClient == nil {
		logger.Info("k8sClient not configured — skipping status update")
		return
	}

	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	at := &atv1alpha1.ActionType{}
	if err := RetryGetCRD(ctx, h.k8sClient, types.NamespacedName{Namespace: namespace, Name: name}, at, 5); err != nil {
		logger.Error(err, "Failed to fetch CRD for status update after retries")
		return
	}

	now := metav1.Now()
	at.Status.Registered = true
	at.Status.CatalogStatus = sharedtypes.CatalogStatusActive
	at.Status.RegisteredBy = registeredBy
	at.Status.RegisteredAt = &now
	at.Status.PreviouslyExisted = result.WasReenabled

	if err := h.k8sClient.Status().Update(ctx, at); err != nil {
		logger.Error(err, "Failed to update CRD status", "action_type", at.Spec.Name)
		return
	}

	logger.Info("ActionType CRD status updated", "action_type", at.Spec.Name, "was_reenabled", result.WasReenabled)
}

// crdDescriptionToOgen converts a CRD ActionTypeDescription to the ogen-generated type.
func crdDescriptionToOgen(d atv1alpha1.ActionTypeDescription) ogenclient.ActionTypeDescription {
	return ogenconv.SharedDescriptionToOgen(atv1alpha1.DescriptionToShared(d))
}
