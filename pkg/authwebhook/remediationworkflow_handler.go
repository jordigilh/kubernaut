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
	"github.com/jordigilh/kubernaut/pkg/shared/contenthash"
	sharedtypes "github.com/jordigilh/kubernaut/pkg/shared/types"
	admissionv1 "k8s.io/api/admission/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/util/retry"
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
// ADR-058: ValidatingWebhookConfiguration intercepts CREATE, UPDATE, and DELETE.
// Issue #371: UPDATE now forwards CRD spec changes to DS so that version
// upgrades supersede the old active catalog entry.
func (h *RemediationWorkflowHandler) Handle(ctx context.Context, req admission.Request) admission.Response {
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

// handleCreate processes CREATE operations: registers the CRD with DS.
func (h *RemediationWorkflowHandler) handleCreate(ctx context.Context, req admission.Request) admission.Response {
	return h.registerWorkflow(ctx, req, "CREATE", EventTypeRWAdmittedCreate)
}

// handleUpdate processes UPDATE operations: re-registers the CRD with DS.
// Issue #371: When a CRD's spec changes (e.g., version bump, added dependencies),
// DS content integrity logic determines the correct action: idempotent return
// (same hash), reject (same version + different hash), supersede (cross-version), or create new.
// Issue #773: Hardened to CREATE-level strictness — all error paths return Denied
// with denied audit event (SOC2 CC8.1 compliance).
func (h *RemediationWorkflowHandler) handleUpdate(ctx context.Context, req admission.Request) admission.Response {
	// During CRD deletion, K8s sends UPDATE admission requests for metadata
	// changes (e.g., finalizer removal by the catalog-cleanup controller).
	// Registering here would undo the DisableWorkflow performed by handleDelete
	// (DS sees "disabled + same hash → re-enable"). Skip to preserve the disable.
	rw := &rwv1alpha1.RemediationWorkflow{}
	if err := json.Unmarshal(req.Object.Raw, rw); err == nil && rw.DeletionTimestamp != nil {
		return admission.Allowed("update during deletion — skipped (DELETE handles DS lifecycle)")
	}
	return h.registerWorkflow(ctx, req, "UPDATE", EventTypeRWAdmittedUpdate)
}

// registerWorkflow is the shared implementation for CREATE and UPDATE operations.
// Both operations follow the same flow: unmarshal → marshal clean content (also
// used as the audit content_hash input) → authenticate → register with DS →
// emit audit → async status update.
// All error paths return admission.Denied and emit denied audit events (SOC2 CC8.1).
func (h *RemediationWorkflowHandler) registerWorkflow(ctx context.Context, req admission.Request, operation, auditEventType string) admission.Response {
	logger := ctrl.Log.WithName("rw-webhook").WithValues("operation", operation, "name", req.Name, "namespace", req.Namespace)

	rw := &rwv1alpha1.RemediationWorkflow{}
	if err := json.Unmarshal(req.Object.Raw, rw); err != nil {
		logger.Error(err, "Failed to unmarshal RemediationWorkflow")
		h.emitDeniedAudit(ctx, req, "unmarshal failed", nil, "")
		return admission.Denied(fmt.Sprintf("failed to unmarshal RemediationWorkflow: %v", err))
	}

	// DD-WORKFLOW-017: Build clean content for DS that excludes Kubernetes runtime
	// metadata (UID, resourceVersion, creationTimestamp). Including these would make
	// the content hash non-deterministic across CRD delete+recreate cycles, breaking
	// BR-WORKFLOW-006 re-enable detection (disabled + same hash → re-enable).
	//
	// #1661 Change 2: computed early (right after unmarshal succeeds, before auth/
	// ActionType checks) — this is a pure, local, side-effect-free marshal with no
	// authorization implications, so every denial path below can attach the
	// attempted workflow_content + content_hash for forensics (BR-AUDIT-005 v2.0
	// #7, SOC2 CC7.2), not just the DS-registration-failure path.
	content, contentHash, err := computeRWContentHash(rw)
	if err != nil {
		logger.Error(err, "Failed to marshal CRD content for DS")
		h.emitDeniedAudit(ctx, req, "marshal failed", &rw.Spec, "")
		return admission.Denied(fmt.Sprintf("failed to marshal CRD content: %v", err))
	}

	// #1661 Change 8a: workflow_id is computed locally from the same
	// deterministic algorithm DS has always used (DeterministicUUID applied
	// to the content hash), rather than trusted from a DS response. This is
	// a pure relocation of an existing computation, so every pre-existing
	// workflow_id remains stable across the migration (DD-WORKFLOW-018).
	workflowID := contenthash.DeterministicUUID(contentHash)

	// SOC2 CC8.1: Extract authenticated user identity for attribution
	authCtx, err := h.authenticator.ExtractUser(ctx, &req.AdmissionRequest)
	if err != nil {
		logger.Error(err, "Authentication failed")
		h.emitDeniedAudit(ctx, req, fmt.Sprintf("authentication failed: %v", err), &rw.Spec, contentHash)
		return admission.Denied(fmt.Sprintf("authentication required: %v", err))
	}

	// #1661: AW is the sole control gate for the RW-to-ActionType relationship.
	// Validate directly against etcd (via the .spec.name field indexer already
	// registered in cmd/authwebhook/main.go) instead of delegating to DS's
	// Postgres-backed taxonomy check (superseded, DD-WORKFLOW-016 GAP-4).
	if err := h.validateActionTypeExists(ctx, rw.Spec.ActionType, req.Namespace); err != nil {
		logger.Error(err, "ActionType existence check failed", "actionType", rw.Spec.ActionType)
		h.emitDeniedAudit(ctx, req, err.Error(), &rw.Spec, contentHash)
		return admission.Denied(err.Error())
	}

	// #1661 Change 8b: today's "same version + different content" 409 (DS's
	// contentIntegrityError, pkg/datastorage/server/workflow_create_handlers.go)
	// moves to a local, zero-dependency check against the UPDATE admission
	// request's own OldObject -- no DS round-trip and no etcd List() needed.
	// RemediationWorkflowSpec has no identity field of its own (DS's schema
	// parser sets spec.WorkflowName = crd.Metadata.Name), so this scenario can
	// only ever happen on the one live object being updated. CREATE has no
	// OldObject, so the check is a no-op there.
	if err := validateContentIntegrity(operation, req.OldObject.Raw, rw.Spec.Version, contentHash); err != nil {
		logger.Error(err, "Content-integrity check failed")
		h.emitDeniedAudit(ctx, req, err.Error(), &rw.Spec, contentHash)
		return admission.Denied(err.Error())
	}

	result, err := h.dsClient.CreateWorkflowInline(ctx, string(content), "crd", authCtx.Username)
	if err != nil {
		logger.Error(err, "DS CreateWorkflowInline failed")
		h.emitDeniedAudit(ctx, req, fmt.Sprintf("data storage registration failed: %v", err), &rw.Spec, contentHash)
		return admission.Denied(fmt.Sprintf("data storage registration failed: %v", err))
	}

	// #1661 Change 8a: workflow_id is authoritative from AW's own local
	// computation (above), not DS's response -- overridden here so every
	// downstream consumer of result (the audit event below, and
	// updateCRDStatus's .status.workflowId write) sees the locally-computed,
	// stable value instead of whatever DS's (still Postgres-backed, until
	// Change 8c removes this call entirely) response happened to return.
	result.WorkflowID = workflowID

	logger.Info("Workflow registered in DS",
		"workflow_id", result.WorkflowID,
		"workflow_name", result.WorkflowName,
		"previously_existed", result.PreviouslyExisted,
	)

	h.emitAdmitAudit(ctx, req, auditEventType, result.WorkflowID, rw.Name, &rw.Spec, contentHash)

	// ADR-058: Update CRD .status asynchronously after admission to avoid blocking
	// the API server. The status subresource is used so this doesn't conflict with
	// the spec stored by the API server.
	go h.updateCRDStatus(req.Namespace, req.Name, authCtx.Username, result)

	// Phase 3c: best-effort cross-update of ActionType CRD status.activeWorkflowCount
	go h.refreshActionTypeWorkflowCount(rw.Spec.ActionType, req.Namespace)

	return admission.Allowed("workflow registered in catalog")
}

// validateActionTypeExists checks whether an Active ActionType CRD exists for
// actionType in namespace, using the ".spec.name" field indexer already
// registered on the manager (cmd/authwebhook/main.go). AW is now the sole
// control gate for the RW-to-ActionType relationship, replacing DS's
// Postgres-backed action_type_taxonomy lookup (#1661).
//
// h.k8sClient is nil in unit tests that don't exercise this gate (production
// always wires a real cache-backed client in cmd/authwebhook/main.go); the
// check is skipped rather than denied in that case, matching the existing
// best-effort precedent of refreshActionTypeWorkflowCount.
func (h *RemediationWorkflowHandler) validateActionTypeExists(ctx context.Context, actionType, namespace string) error {
	if h.k8sClient == nil {
		return nil
	}

	atList := &atv1alpha1.ActionTypeList{}
	if err := h.k8sClient.List(ctx, atList,
		client.InNamespace(namespace),
		client.MatchingFields{".spec.name": actionType},
	); err != nil {
		return fmt.Errorf("action type lookup failed for %q: %w", actionType, err)
	}

	for i := range atList.Items {
		if atList.Items[i].Status.CatalogStatus == sharedtypes.CatalogStatusActive {
			return nil
		}
	}

	// Wording matches DS's now-superseded validateActionType detail message
	// (pkg/datastorage/server/workflow_create_handlers.go) so operators see
	// the same denial text regardless of which gate rejected the request.
	return fmt.Errorf("action_type '%s' is not in the action type taxonomy (DD-WORKFLOW-016)", actionType)
}

// validateContentIntegrity rejects an UPDATE that keeps spec.version unchanged
// while the content hash changes -- the same "content changed without a
// version bump" rule DS's Postgres-backed contentIntegrityError enforced
// (pkg/datastorage/server/workflow_create_handlers.go), now evaluated locally
// from the admission request's own OldObject (#1661 Change 8b). oldObjectRaw
// is empty for CREATE (no prior state to compare against), so the check is a
// no-op there.
func validateContentIntegrity(operation string, oldObjectRaw []byte, newVersion, newContentHash string) error {
	if operation != "UPDATE" || len(oldObjectRaw) == 0 {
		return nil
	}

	oldRW := &rwv1alpha1.RemediationWorkflow{}
	if err := json.Unmarshal(oldObjectRaw, oldRW); err != nil {
		// Best-effort: an unparsable OldObject shouldn't block the update --
		// the ActionType/DS checks around this one already guard correctness.
		return nil
	}
	if oldRW.Spec.Version != newVersion {
		return nil
	}

	_, oldHash, err := computeRWContentHash(oldRW)
	if err != nil {
		return nil
	}
	if oldHash == newContentHash {
		return nil
	}

	// Wording matches DS's now-superseded contentIntegrityError.Error()
	// (pkg/datastorage/server/workflow_create_handlers.go) verbatim -- including
	// the "active" qualifier, which stays accurate here too, since there is
	// exactly one live CRD per name and it is always the "active" one -- so
	// operators see no behavioral difference regardless of which gate denies.
	return fmt.Errorf(
		"active workflow %q version %q already has different content (hash %s→%s); bump the version to register new content",
		oldRW.Name, newVersion, shortHash(oldHash), shortHash(newContentHash),
	)
}

// shortHash truncates a content hash to its first 12 characters for
// human-readable denial messages, matching DS's contentIntegrityError.Error()
// formatting precedent.
func shortHash(hash string) string {
	if len(hash) > 12 {
		return hash[:12]
	}
	return hash
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
		// #1661 Change 2: delete stays unchanged — it only disables an
		// already-audited workflow_id, so there is no new content to capture.
		h.emitAdmitAudit(ctx, req, EventTypeRWAdmittedDelete, "", rw.Name, nil, "")
		return admission.Allowed("delete allowed (no workflowId in status)")
	}

	// Call DS to disable the workflow (best-effort — always allow DELETE)
	disableOK := false
	if err := h.dsClient.DisableWorkflow(ctx, workflowID, "CRD deleted", username); err != nil {
		logger.Error(err, "DS DisableWorkflow failed (best-effort — allowing DELETE)",
			"workflow_id", workflowID,
		)
	} else {
		disableOK = true
		logger.Info("Workflow disabled in DS",
			"workflow_id", workflowID,
		)
	}

	// Emit DELETE audit event. #1661 Change 2: no workflow_content/content_hash —
	// delete stays unchanged (the CREATE/UPDATE audit trail already has the last
	// known content for this workflow_id).
	h.emitAdmitAudit(ctx, req, EventTypeRWAdmittedDelete, workflowID, rw.Name, nil, "")

	// Issue #418 Fix A: Only refresh the count when the DS disable succeeded.
	// Writing the count after a failed disable would actively reinforce stale data.
	// The finalizer reconciler (RWFinalizerName) handles the guaranteed path.
	if disableOK {
		go h.refreshActionTypeWorkflowCount(rw.Spec.ActionType, req.Namespace)
	}

	return admission.Allowed("workflow disabled in catalog")
}

// updateCRDStatus writes the DS registration result into the CRD's .status subresource.
// Runs asynchronously after admission completes so it doesn't block the API server response.
// Uses a fresh context with a timeout since the admission context is cancelled after response.
//
// Uses RetryOnConflict to handle the race between this goroutine and the API server
// committing the spec change. On the UPDATE path, a plain GET succeeds immediately
// (CRD already exists) but returns a stale resourceVersion; the subsequent Status().Update
// gets a 409 Conflict once the API server commits the new resourceVersion. The retry
// loop re-GETs the CRD (fresh resourceVersion) and retries the status write.
func (h *RemediationWorkflowHandler) updateCRDStatus(namespace, name, registeredBy string, result *WorkflowRegistrationResult) {
	logger := ctrl.Log.WithName("rw-webhook").WithValues("operation", "status-update", "name", name, "namespace", namespace)

	if h.k8sClient == nil {
		logger.Info("k8sClient not configured — skipping status update")
		return
	}

	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	key := types.NamespacedName{Namespace: namespace, Name: name}

	err := retry.RetryOnConflict(retry.DefaultBackoff, func() error {
		rw := &rwv1alpha1.RemediationWorkflow{}
		if err := h.k8sClient.Get(ctx, key, rw); err != nil {
			return err
		}

		now := metav1.Now()
		rw.Status.WorkflowID = result.WorkflowID
		rw.Status.CatalogStatus = sharedtypes.CatalogStatus(result.Status)
		rw.Status.RegisteredBy = registeredBy
		rw.Status.RegisteredAt = &now
		rw.Status.PreviouslyExisted = result.PreviouslyExisted
		setCondition(&rw.Status.Conditions, metav1.Condition{
			Type:               rwv1alpha1.ConditionReady,
			Status:             metav1.ConditionTrue,
			Reason:             rwv1alpha1.ReasonRegistered,
			Message:            "Workflow registered successfully in DataStorage catalog",
			LastTransitionTime: now,
		})

		return h.k8sClient.Status().Update(ctx, rw)
	})
	if err != nil {
		logger.Error(err, "Failed to update CRD status after retries",
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

	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	count, err := h.atCounter.GetActiveWorkflowCount(ctx, actionType)
	if err != nil {
		logger.Error(err, "Failed to fetch active workflow count from DS")
		return
	}

	atList := &atv1alpha1.ActionTypeList{}
	if err := h.k8sClient.List(ctx, atList, client.InNamespace(namespace)); err != nil {
		logger.Error(err, "Failed to list ActionType CRDs")
		return
	}

	for i := range atList.Items {
		at := &atList.Items[i]
		if at.Spec.Name != actionType {
			continue
		}
		if err := h.updateATWorkflowCountWithRetry(ctx, at.Name, at.Namespace, count); err != nil {
			logger.Error(err, "Failed to update ActionType activeWorkflowCount after retries",
				"crd", at.Name, "count", count)
		} else {
			logger.Info("ActionType CRD status.activeWorkflowCount updated",
				"crd", at.Name, "count", count)
		}
		return
	}

	logger.V(1).Info("No ActionType CRD found — cross-update skipped (may not be created yet)")
}

const maxStatusUpdateRetries = 5

// updateATWorkflowCountWithRetry performs a fresh Get + Status().Update in a
// retry loop so that resource-version conflicts don't leave the count stale.
// Fixes #367: stale activeWorkflowCount after conflict in fire-and-forget goroutine.
func (h *RemediationWorkflowHandler) updateATWorkflowCountWithRetry(ctx context.Context, name, namespace string, count int) error {
	key := client.ObjectKey{Namespace: namespace, Name: name}
	for attempt := 0; attempt < maxStatusUpdateRetries; attempt++ {
		at := &atv1alpha1.ActionType{}
		if err := h.k8sClient.Get(ctx, key, at); err != nil {
			return fmt.Errorf("get ActionType: %w", err)
		}
		at.Status.ActiveWorkflowCount = count
		err := h.k8sClient.Status().Update(ctx, at)
		if err == nil {
			return nil
		}
		if !apierrors.IsConflict(err) {
			return fmt.Errorf("status update: %w", err)
		}
	}
	return fmt.Errorf("conflict after %d retries", maxStatusUpdateRetries)
}

// computeRWContentHash marshals rw's clean CRD content and hashes it in one
// step. Shared by registerWorkflow (new object) and validateContentIntegrity
// (old object, #1661 Change 8b REFACTOR) so both compute the hash the exact
// same way with no duplicated marshal+hash call sites.
func computeRWContentHash(rw *rwv1alpha1.RemediationWorkflow) ([]byte, string, error) {
	content, err := marshalCleanCRDContent(rw)
	if err != nil {
		return nil, "", err
	}
	return content, contenthash.ComputeContentHash(string(content)), nil
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
