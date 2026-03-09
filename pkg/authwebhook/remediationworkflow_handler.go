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
	"time"

	atv1alpha1 "github.com/jordigilh/kubernaut/api/actiontype/v1alpha1"
	rwv1alpha1 "github.com/jordigilh/kubernaut/api/remediationworkflow/v1alpha1"
	"github.com/jordigilh/kubernaut/pkg/audit"
	admissionv1 "k8s.io/api/admission/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/fields"
	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/webhook/admission"
)

// WorkflowCatalogClient defines the DS REST API operations required by the AW
// handler to register and manage workflows on behalf of CRD lifecycle events.
// BR-WORKFLOW-006: Kubernetes-native workflow registration via CRD + AW bridge.
type WorkflowCatalogClient interface {
	CreateWorkflowInline(ctx context.Context, content, source, registeredBy string) (*WorkflowRegistrationResult, error)
	DisableWorkflow(ctx context.Context, workflowID, reason, updatedBy string) error
}

// ActionTypeWorkflowCounter retrieves the authoritative active workflow count from DS.
// Used for best-effort cross-update of ActionType CRD status.activeWorkflowCount
// after RW CREATE/DELETE (Phase 3c, BR-WORKFLOW-007).
type ActionTypeWorkflowCounter interface {
	GetActiveWorkflowCount(ctx context.Context, actionType string) (int, error)
}

// WorkflowRegistrationResult holds the DS response after registering or re-enabling a workflow.
type WorkflowRegistrationResult struct {
	WorkflowID        string
	WorkflowName      string
	Version           string
	Status            string
	PreviouslyExisted bool
	Superseded        bool   // true when an active workflow was superseded by a new spec (different ContentHash)
	SupersededID      string // UUID of the workflow that was superseded (for audit trail)
}

// RemediationWorkflowHandler handles admission requests for RemediationWorkflow CRD
// CREATE and DELETE operations, bridging CRD lifecycle with the DS workflow catalog.
// BR-WORKFLOW-006, DD-WEBHOOK-003, ADR-058.
//
// CREATE: Extracts CRD spec, POSTs inline schema to DS, updates CRD .status
// asynchronously via k8sClient.Status().Update().
// DELETE: Extracts workflowId from status, PATCHes DS to disable.
type RemediationWorkflowHandler struct {
	dsClient      WorkflowCatalogClient
	auditStore    audit.AuditStore
	k8sClient     client.Client
	authenticator *Authenticator
	atCounter     ActionTypeWorkflowCounter // nullable; nil skips cross-update
}

// RWHandlerOption configures optional dependencies on RemediationWorkflowHandler.
type RWHandlerOption func(*RemediationWorkflowHandler)

// WithActionTypeWorkflowCounter enables best-effort cross-update of ActionType
// CRD status.activeWorkflowCount after RW CREATE/DELETE (Phase 3c, BR-WORKFLOW-007).
func WithActionTypeWorkflowCounter(counter ActionTypeWorkflowCounter) RWHandlerOption {
	return func(h *RemediationWorkflowHandler) {
		h.atCounter = counter
	}
}

// NewRemediationWorkflowHandler creates a handler for RemediationWorkflow admission.
func NewRemediationWorkflowHandler(
	dsClient WorkflowCatalogClient,
	auditStore audit.AuditStore,
	k8sClient client.Client,
	opts ...RWHandlerOption,
) *RemediationWorkflowHandler {
	h := &RemediationWorkflowHandler{
		dsClient:      dsClient,
		auditStore:    auditStore,
		k8sClient:     k8sClient,
		authenticator: NewAuthenticator(),
	}
	for _, opt := range opts {
		opt(h)
	}
	return h
}

// Handle processes admission requests for RemediationWorkflow CRD.
// Implements admission.Handler from controller-runtime.
//
// ADR-058: ValidatingWebhookConfiguration intercepts CREATE and DELETE.
// UPDATE is allowed without DS interaction (CRD spec is idempotent).
func (h *RemediationWorkflowHandler) Handle(ctx context.Context, req admission.Request) admission.Response {
	switch req.Operation {
	case admissionv1.Create:
		return h.handleCreate(ctx, req)
	case admissionv1.Delete:
		return h.handleDelete(ctx, req)
	default:
		return admission.Allowed("operation not intercepted")
	}
}

// handleCreate processes CREATE operations: registers the CRD with DS.
func (h *RemediationWorkflowHandler) handleCreate(ctx context.Context, req admission.Request) admission.Response {
	logger := ctrl.Log.WithName("rw-webhook").WithValues("operation", "CREATE", "name", req.Name, "namespace", req.Namespace)

	// Unmarshal the CRD from the admission request
	rw := &rwv1alpha1.RemediationWorkflow{}
	if err := json.Unmarshal(req.Object.Raw, rw); err != nil {
		logger.Error(err, "Failed to unmarshal RemediationWorkflow")
		h.emitDeniedAudit(ctx, req, "unmarshal failed")
		return admission.Denied(fmt.Sprintf("failed to unmarshal RemediationWorkflow: %v", err))
	}

	// SOC2 CC8.1: Extract authenticated user identity for attribution
	authCtx, err := h.authenticator.ExtractUser(ctx, &req.AdmissionRequest)
	if err != nil {
		logger.Error(err, "Authentication failed")
		h.emitDeniedAudit(ctx, req, fmt.Sprintf("authentication failed: %v", err))
		return admission.Denied(fmt.Sprintf("authentication required: %v", err))
	}

	// DD-WORKFLOW-017: Build clean content for DS that excludes Kubernetes runtime
	// metadata (UID, resourceVersion, creationTimestamp). Including these would make
	// the content hash non-deterministic across CRD delete+recreate cycles, breaking
	// BR-WORKFLOW-006 re-enable detection (disabled + same hash → re-enable).
	content, err := marshalCleanCRDContent(rw)
	if err != nil {
		logger.Error(err, "Failed to marshal CRD content for DS")
		h.emitDeniedAudit(ctx, req, "marshal failed")
		return admission.Denied(fmt.Sprintf("failed to marshal CRD content: %v", err))
	}

	// Call DS to register the workflow
	result, err := h.dsClient.CreateWorkflowInline(ctx, string(content), "crd", authCtx.Username)
	if err != nil {
		logger.Error(err, "DS CreateWorkflowInline failed")
		h.emitDeniedAudit(ctx, req, fmt.Sprintf("data storage registration failed: %v", err))
		return admission.Denied(fmt.Sprintf("data storage registration failed: %v", err))
	}

	logger.Info("Workflow registered in DS",
		"workflow_id", result.WorkflowID,
		"workflow_name", result.WorkflowName,
		"previously_existed", result.PreviouslyExisted,
	)

	// Emit successful CREATE audit event
	h.emitAdmitAudit(ctx, req, EventTypeRWAdmittedCreate, result.WorkflowID, rw.Name)

	// ADR-058: Update CRD .status asynchronously after admission to avoid blocking
	// the API server. The status subresource is used so this doesn't conflict with
	// the spec stored by the API server.
	go h.updateCRDStatus(req.Namespace, req.Name, authCtx.Username, result)

	// Phase 3c: best-effort cross-update of ActionType CRD status.activeWorkflowCount
	go h.refreshActionTypeWorkflowCount(rw.Spec.ActionType, req.Namespace)

	return admission.Allowed("workflow registered in catalog")
}

// handleDelete processes DELETE operations: disables the workflow in DS.
func (h *RemediationWorkflowHandler) handleDelete(ctx context.Context, req admission.Request) admission.Response {
	logger := ctrl.Log.WithName("rw-webhook").WithValues("operation", "DELETE", "name", req.Name, "namespace", req.Namespace)

	// Unmarshal the old object (for DELETE, the object is in OldObject)
	rw := &rwv1alpha1.RemediationWorkflow{}
	if err := json.Unmarshal(req.OldObject.Raw, rw); err != nil {
		logger.Error(err, "Failed to unmarshal RemediationWorkflow from OldObject")
		// Allow DELETE anyway to prevent GitOps drift
		return admission.Allowed("delete allowed (unmarshal failed, best-effort)")
	}

	// Extract workflowId from status
	workflowID := rw.Status.WorkflowID

	// SOC2 CC8.1: Extract authenticated user identity for attribution (best-effort for DELETE)
	username := req.UserInfo.Username
	if authCtx, err := h.authenticator.ExtractUser(ctx, &req.AdmissionRequest); err == nil {
		username = authCtx.Username
	}

	if workflowID == "" {
		logger.Info("No workflowId in CRD status — skipping DS disable (workflow may not have been registered)")
		h.emitAdmitAudit(ctx, req, EventTypeRWAdmittedDelete, "", rw.Name)
		return admission.Allowed("delete allowed (no workflowId in status)")
	}

	// Call DS to disable the workflow (best-effort — always allow DELETE)
	if err := h.dsClient.DisableWorkflow(ctx, workflowID, "CRD deleted", username); err != nil {
		logger.Error(err, "DS DisableWorkflow failed (best-effort — allowing DELETE)",
			"workflow_id", workflowID,
		)
	} else {
		logger.Info("Workflow disabled in DS",
			"workflow_id", workflowID,
		)
	}

	// Emit DELETE audit event
	h.emitAdmitAudit(ctx, req, EventTypeRWAdmittedDelete, workflowID, rw.Name)

	// Phase 3c: best-effort cross-update of ActionType CRD status.activeWorkflowCount
	go h.refreshActionTypeWorkflowCount(rw.Spec.ActionType, req.Namespace)

	return admission.Allowed("workflow disabled in catalog")
}

// updateCRDStatus writes the DS registration result into the CRD's .status subresource.
// Runs asynchronously after admission completes so it doesn't block the API server response.
// Uses a fresh context with a timeout since the admission context is cancelled after response.
//
// The CRD may not be committed by the API server yet when this goroutine starts
// (the admission response triggers the commit). A retry loop with exponential
// backoff (500ms, 1s, 2s) handles this race.
func (h *RemediationWorkflowHandler) updateCRDStatus(namespace, name, registeredBy string, result *WorkflowRegistrationResult) {
	logger := ctrl.Log.WithName("rw-webhook").WithValues("operation", "status-update", "name", name, "namespace", namespace)

	if h.k8sClient == nil {
		logger.Info("k8sClient not configured — skipping status update")
		return
	}

	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	rw := &rwv1alpha1.RemediationWorkflow{}

	backoff := 500 * time.Millisecond
	const maxRetries = 5
	var lastErr error
	for attempt := 0; attempt < maxRetries; attempt++ {
		if attempt > 0 {
			select {
			case <-ctx.Done():
				logger.Error(ctx.Err(), "Context expired waiting for CRD to appear", "attempts", attempt)
				return
			case <-time.After(backoff):
				backoff *= 2
			}
		}
		lastErr = h.k8sClient.Get(ctx, types.NamespacedName{Namespace: namespace, Name: name}, rw)
		if lastErr == nil {
			break
		}
	}
	if lastErr != nil {
		logger.Error(lastErr, "Failed to fetch CRD for status update after retries", "retries", maxRetries)
		return
	}

	now := metav1.Now()
	rw.Status.WorkflowID = result.WorkflowID
	rw.Status.CatalogStatus = result.Status
	rw.Status.RegisteredBy = registeredBy
	rw.Status.RegisteredAt = &now
	rw.Status.PreviouslyExisted = result.PreviouslyExisted

	if err := h.k8sClient.Status().Update(ctx, rw); err != nil {
		logger.Error(err, "Failed to update CRD status",
			"workflow_id", result.WorkflowID,
		)
		return
	}

	logger.Info("CRD status updated",
		"workflow_id", result.WorkflowID,
		"catalog_status", result.Status,
		"previously_existed", result.PreviouslyExisted,
	)
}

// refreshActionTypeWorkflowCount is a best-effort goroutine that queries DS for
// the current active workflow count for the given actionType, then patches the
// corresponding ActionType CRD's status.activeWorkflowCount. Errors are logged
// but never propagated — the RW admission result is already decided.
//
// Phase 3c (BR-WORKFLOW-007): keeps the kubectl get at WORKFLOWS column up-to-date.
func (h *RemediationWorkflowHandler) refreshActionTypeWorkflowCount(actionType, namespace string) {
	logger := ctrl.Log.WithName("rw-webhook").WithValues("operation", "at-cross-update", "actionType", actionType)

	if h.atCounter == nil || h.k8sClient == nil {
		logger.V(1).Info("Cross-update skipped: atCounter or k8sClient not configured")
		return
	}

	if actionType == "" {
		logger.V(1).Info("Cross-update skipped: empty actionType")
		return
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	count, err := h.atCounter.GetActiveWorkflowCount(ctx, actionType)
	if err != nil {
		logger.Error(err, "Failed to fetch active workflow count from DS")
		return
	}

	// Find the ActionType CRD using the selectable field .spec.name
	atList := &atv1alpha1.ActionTypeList{}
	if err := h.k8sClient.List(ctx, atList,
		client.InNamespace(namespace),
		client.MatchingFieldsSelector{Selector: fields.OneTermEqualSelector(".spec.name", actionType)},
	); err != nil {
		logger.Error(err, "Failed to list ActionType CRDs")
		return
	}

	if len(atList.Items) == 0 {
		logger.V(1).Info("No ActionType CRD found — cross-update skipped (may not be created yet)")
		return
	}

	for i := range atList.Items {
		at := &atList.Items[i]
		at.Status.ActiveWorkflowCount = count
		if err := h.k8sClient.Status().Update(ctx, at); err != nil {
			logger.Error(err, "Failed to patch ActionType CRD status.activeWorkflowCount",
				"crd", at.Name, "count", count)
		} else {
			logger.Info("ActionType CRD status.activeWorkflowCount updated",
				"crd", at.Name, "count", count)
		}
	}
}

// marshalCleanCRDContent produces a JSON representation of the CRD that only
// includes the fields relevant to the workflow definition: apiVersion, kind,
// metadata.name, and spec. Kubernetes runtime metadata (UID, resourceVersion,
// creationTimestamp, managedFields, etc.) is excluded so that the content hash
// computed by DS is deterministic across CRD delete+recreate cycles.
func marshalCleanCRDContent(rw *rwv1alpha1.RemediationWorkflow) ([]byte, error) {
	type cleanMetadata struct {
		Name string `json:"name"`
	}
	type cleanCRD struct {
		APIVersion string                              `json:"apiVersion"`
		Kind       string                              `json:"kind"`
		Metadata   cleanMetadata                       `json:"metadata"`
		Spec       rwv1alpha1.RemediationWorkflowSpec  `json:"spec"`
	}

	apiVersion := rw.APIVersion
	if apiVersion == "" {
		apiVersion = "kubernaut.ai/v1alpha1"
	}
	kind := rw.Kind
	if kind == "" {
		kind = "RemediationWorkflow"
	}

	clean := cleanCRD{
		APIVersion: apiVersion,
		Kind:       kind,
		Metadata:   cleanMetadata{Name: rw.Name},
		Spec:       rw.Spec,
	}
	return json.Marshal(clean)
}

