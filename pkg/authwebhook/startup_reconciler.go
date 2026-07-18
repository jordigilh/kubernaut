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
	"fmt"
	"time"

	"github.com/go-logr/logr"
	atv1alpha1 "github.com/jordigilh/kubernaut/api/actiontype/v1alpha1"
	rwv1alpha1 "github.com/jordigilh/kubernaut/api/remediationworkflow/v1alpha1"
	"github.com/jordigilh/kubernaut/pkg/audit"
	ogenclient "github.com/jordigilh/kubernaut/pkg/datastorage/ogen-client"
	"github.com/jordigilh/kubernaut/pkg/shared/contenthash"
	sharedtypes "github.com/jordigilh/kubernaut/pkg/shared/types"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/tools/record"
	"k8s.io/client-go/util/retry"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

const startupRegisteredBy = "system:authwebhook-startup"

// WorkflowDSClient is now an empty marker interface. #1661 Change 8c removed
// its only method (CreateWorkflowInline) -- syncWorkflowCRD computes/patches
// everything locally with zero DS round-trips. The DSWorkflow field is kept
// (rather than removed outright) to avoid an unrelated, high-blast-radius
// signature change across every test call site and cmd/authwebhook/main.go
// in this REFACTOR pass; full removal is deferred to Phase 55.
type WorkflowDSClient interface{}

// ActionTypeDSClient is now an empty marker interface. #1661 Change 8d
// removed its only method (CreateActionType) -- syncActionTypeCRD
// computes/patches everything locally with zero DS round-trips, mirroring
// WorkflowDSClient's Change 8c precedent above. The DSActionType field is
// kept (rather than removed outright) to avoid an unrelated, high-blast-radius
// signature change across every test call site and cmd/authwebhook/main.go
// in this GREEN pass; full removal is deferred to Phase 55.
type ActionTypeDSClient interface{}

// StartupReconciler lists all ActionType and RemediationWorkflow CRDs at
// startup and stamps both from a pure local computation with zero DS
// involvement (#1661 Change 8c for RemediationWorkflow, Change 8d for
// ActionType). It implements manager.Runnable so that the controller
// manager blocks readiness until the reconciliation completes.
//
// Issue #548: Ensures PVC-wipe resilience by re-registering CRDs on startup.
// Issue #1246: Graceful degradation — individual RW failures don't crash the pod.
type StartupReconciler struct {
	K8sClient      client.Client
	DSWorkflow     WorkflowDSClient
	DSActionType   ActionTypeDSClient
	Logger         logr.Logger
	Timeout        time.Duration
	InitialBackoff time.Duration
	EventRecorder  record.EventRecorder
	AuditStore     audit.AuditStore
}

// NeedLeaderElection returns false so the reconciler runs on every replica,
// ensuring every authwebhook instance has a consistent view.
func (r *StartupReconciler) NeedLeaderElection() bool {
	return false
}

// Start performs the full startup reconciliation: list CRDs, sync with DS,
// update CRD statuses. Returns error on failure (fail-closed).
func (r *StartupReconciler) Start(ctx context.Context) error {
	logger := r.Logger.WithName("startup-reconciler")

	if r.InitialBackoff == 0 {
		r.InitialBackoff = 500 * time.Millisecond
	}

	deadline := time.Now().Add(r.Timeout)

	// Phase 1: Sync ActionType CRDs (must complete before workflows)
	if err := r.syncActionTypes(ctx, logger, deadline); err != nil {
		return fmt.Errorf("startup reconciliation failed (ActionTypes): %w", err)
	}

	// Phase 2: Sync RemediationWorkflow CRDs
	if err := r.syncWorkflows(ctx, logger, deadline); err != nil {
		return fmt.Errorf("startup reconciliation failed (Workflows): %w", err)
	}

	logger.Info("Startup reconciliation completed successfully")
	return nil
}

func (r *StartupReconciler) syncActionTypes(ctx context.Context, logger logr.Logger, deadline time.Time) error {
	var atList atv1alpha1.ActionTypeList
	if err := r.K8sClient.List(ctx, &atList); err != nil {
		return fmt.Errorf("failed to list ActionType CRDs: %w", err)
	}

	if len(atList.Items) == 0 {
		logger.Info("No ActionType CRDs found, skipping")
		return nil
	}

	logger.Info("Syncing ActionType CRDs", "count", len(atList.Items))

	var succeeded, failed int
	for i := range atList.Items {
		at := &atList.Items[i]
		if err := r.syncActionTypeCRD(ctx, logger, at); err != nil {
			failed++
		} else {
			succeeded++
		}
	}

	if failed > 0 {
		logger.Error(fmt.Errorf("%d action type(s) failed registration", failed),
			"Startup reconciliation completed with degraded action types",
			"succeeded", succeeded, "failed", failed)
	}
	return nil
}

// syncActionTypeCRD stamps .status.registered/.status.catalogStatus from a
// pure local computation -- #1661 Change 8d removed the DS round-trip (and
// the deadline/backoff/retry machinery that only existed to survive DS
// being transiently unavailable) entirely, mirroring syncWorkflowCRD's
// Change 8c precedent above. The only remaining failure mode is a transient
// K8s API error on Get/Update, handled by RetryOnConflict.
func (r *StartupReconciler) syncActionTypeCRD(ctx context.Context, logger logr.Logger, at *atv1alpha1.ActionType) error {
	atLogger := logger.WithValues("actiontype", at.Spec.Name)

	key := client.ObjectKeyFromObject(at)
	err := retry.RetryOnConflict(retry.DefaultBackoff, func() error {
		fresh := &atv1alpha1.ActionType{}
		if getErr := r.K8sClient.Get(ctx, key, fresh); getErr != nil {
			return getErr
		}

		now := metav1.Now()
		fresh.Status.Registered = true
		fresh.Status.CatalogStatus = sharedtypes.CatalogStatusActive
		fresh.Status.RegisteredBy = startupRegisteredBy
		fresh.Status.RegisteredAt = &now
		fresh.Status.PreviouslyExisted = false

		return r.K8sClient.Status().Update(ctx, fresh)
	})
	if err != nil {
		atLogger.Error(err, "Failed to update ActionType CRD status after retries")
		return err
	}

	atLogger.Info("ActionType registered locally")
	return nil
}

func (r *StartupReconciler) syncWorkflows(ctx context.Context, logger logr.Logger, deadline time.Time) error {
	var rwList rwv1alpha1.RemediationWorkflowList
	if err := r.K8sClient.List(ctx, &rwList); err != nil {
		return fmt.Errorf("failed to list RemediationWorkflow CRDs: %w", err)
	}

	if len(rwList.Items) == 0 {
		logger.Info("No RemediationWorkflow CRDs found, skipping")
		return nil
	}

	logger.Info("Syncing RemediationWorkflow CRDs", "count", len(rwList.Items))

	var succeeded, failed int
	for i := range rwList.Items {
		rw := &rwList.Items[i]
		if err := r.syncWorkflowCRD(ctx, logger, rw); err != nil {
			failed++
		} else {
			succeeded++
		}
	}

	if failed > 0 {
		logger.Error(fmt.Errorf("%d workflow(s) failed registration", failed),
			"Startup reconciliation completed with degraded workflows",
			"succeeded", succeeded, "failed", failed)
	}
	return nil
}

// syncWorkflowCRD stamps .status.workflowId/.status.contentHash/
// .status.catalogStatus from a pure local computation -- #1661 Change 8c
// removed the DS round-trip (and the deadline/backoff/retry machinery that
// only existed to survive DS being transiently unavailable) entirely. The
// only remaining failure mode is a malformed CRD failing to marshal, which
// realistically cannot happen for an object that already passed
// admission-time validation when it was created/updated.
func (r *StartupReconciler) syncWorkflowCRD(ctx context.Context, logger logr.Logger, rw *rwv1alpha1.RemediationWorkflow) error {
	rwLogger := logger.WithValues("workflow", rw.Name)

	content, err := contenthash.MarshalCleanCRDContent(rw)
	if err != nil {
		r.markWorkflowFailed(ctx, rwLogger, rw, rwv1alpha1.ReasonValidationFailed,
			fmt.Sprintf("failed to marshal workflow: %v", err))
		return err
	}

	contentHash := contenthash.ComputeContentHash(string(content))
	workflowID := contenthash.DeterministicUUID(contentHash)

	rwLogger.Info("Workflow synced locally", "workflow_id", workflowID)
	r.markWorkflowSucceeded(ctx, rwLogger, rw, workflowID, contentHash)
	return nil
}

func (r *StartupReconciler) markWorkflowSucceeded(ctx context.Context, logger logr.Logger, rw *rwv1alpha1.RemediationWorkflow, workflowID, contentHash string) {
	err := retry.RetryOnConflict(retry.DefaultBackoff, func() error {
		fresh := &rwv1alpha1.RemediationWorkflow{}
		if getErr := r.K8sClient.Get(ctx, client.ObjectKeyFromObject(rw), fresh); getErr != nil {
			return getErr
		}

		now := metav1.Now()
		fresh.Status.WorkflowID = workflowID
		fresh.Status.ContentHash = contentHash
		fresh.Status.CatalogStatus = sharedtypes.CatalogStatusActive
		fresh.Status.RegisteredBy = startupRegisteredBy
		fresh.Status.RegisteredAt = &now
		fresh.Status.PreviouslyExisted = false
		setCondition(&fresh.Status.Conditions, metav1.Condition{
			Type:               rwv1alpha1.ConditionReady,
			Status:             metav1.ConditionTrue,
			Reason:             rwv1alpha1.ReasonRegistered,
			Message:            "Workflow registered successfully",
			LastTransitionTime: now,
		})
		return r.K8sClient.Status().Update(ctx, fresh)
	})
	if err != nil {
		logger.Error(err, "Failed to update RemediationWorkflow CRD status")
	}
}

func (r *StartupReconciler) markWorkflowFailed(ctx context.Context, logger logr.Logger, rw *rwv1alpha1.RemediationWorkflow, reason, message string) {
	err := retry.RetryOnConflict(retry.DefaultBackoff, func() error {
		fresh := &rwv1alpha1.RemediationWorkflow{}
		if getErr := r.K8sClient.Get(ctx, client.ObjectKeyFromObject(rw), fresh); getErr != nil {
			return getErr
		}

		now := metav1.Now()
		fresh.Status.CatalogStatus = sharedtypes.CatalogStatusDisabled
		setCondition(&fresh.Status.Conditions, metav1.Condition{
			Type:               rwv1alpha1.ConditionReady,
			Status:             metav1.ConditionFalse,
			Reason:             reason,
			Message:            message,
			LastTransitionTime: now,
		})
		return r.K8sClient.Status().Update(ctx, fresh)
	})
	if err != nil {
		logger.Error(err, "Failed to update RemediationWorkflow CRD status to Disabled")
	}

	if r.EventRecorder != nil {
		r.EventRecorder.Eventf(rw, "Warning", "RegistrationFailed",
			"Workflow registration failed: %s", message)
	}

	r.emitRegistrationFailedAudit(ctx, rw, reason, message)
}

// setCondition adds or updates a condition in the slice by type.
func setCondition(conditions *[]metav1.Condition, cond metav1.Condition) {
	for i, existing := range *conditions {
		if existing.Type == cond.Type {
			(*conditions)[i] = cond
			return
		}
	}
	*conditions = append(*conditions, cond)
}

// emitRegistrationFailedAudit emits an audit event when a workflow fails registration
// during startup reconciliation. Uses best-effort semantics (audit failure doesn't
// block startup). Category: "workflow", EventType: "authwebhook.workflow.registration_failed".
func (r *StartupReconciler) emitRegistrationFailedAudit(ctx context.Context, rw *rwv1alpha1.RemediationWorkflow, reason, message string) {
	if r.AuditStore == nil {
		return
	}

	event := audit.NewAuditEventRequest()
	audit.SetEventType(event, EventTypeRWRegistrationFailed)
	audit.SetEventCategory(event, EventCategoryWorkflow)
	audit.SetEventAction(event, "registration_failed")
	audit.SetEventOutcome(event, ogenclient.AuditEventRequestEventOutcomeFailure)
	audit.SetActor(event, "system", startupRegisteredBy)
	audit.SetResource(event, "RemediationWorkflow", rw.Name)
	audit.SetNamespace(event, rw.Namespace)
	audit.SetSeverity(event, "high")

	payload := ogenclient.AuthwebhookWorkflowRegistrationFailedPayload{
		EventType:    ogenclient.AuthwebhookWorkflowRegistrationFailedPayloadEventTypeAuthwebhookWorkflowRegistrationFailed,
		WorkflowName: rw.Name,
		Reason:       ogenclient.AuthwebhookWorkflowRegistrationFailedPayloadReason(reason),
		Message:      message,
	}
	payload.Namespace.SetTo(rw.Namespace)
	event.EventData = ogenclient.NewAuthwebhookWorkflowRegistrationFailedPayloadAuditEventRequestEventData(payload)

	storeAuditBestEffort(ctx, r.AuditStore, event, "startup-reconciler", EventTypeRWRegistrationFailed)
}
