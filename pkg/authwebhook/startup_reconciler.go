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
	sharedtypes "github.com/jordigilh/kubernaut/pkg/shared/types"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/tools/record"
	"k8s.io/client-go/util/retry"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

const startupRegisteredBy = "system:authwebhook-startup"

// WorkflowDSClient abstracts the DS workflow operations needed by the startup reconciler.
type WorkflowDSClient interface {
	CreateWorkflowInline(ctx context.Context, content, source, registeredBy string) (*WorkflowRegistrationResult, error)
}

// ActionTypeDSClient abstracts the DS action type operations needed by the startup reconciler.
type ActionTypeDSClient interface {
	CreateActionType(ctx context.Context, name string, description ogenclient.ActionTypeDescription, registeredBy string) (*ActionTypeRegistrationResult, error)
}

// StartupReconciler lists all ActionType and RemediationWorkflow CRDs at startup
// and syncs them with DataStorage. It implements manager.Runnable so that the
// controller manager blocks readiness until the reconciliation completes.
//
// Issue #548: Ensures PVC-wipe resilience by re-registering CRDs on startup.
// Issue #1246: Graceful degradation — individual RW failures don't crash the pod.
// Ordering: ActionType CRDs are synced first, then RemediationWorkflow CRDs,
// because workflows reference action types.
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

	logger.Info("Syncing ActionType CRDs with DS", "count", len(atList.Items))

	for i := range atList.Items {
		at := &atList.Items[i]
		if err := r.syncActionTypeCRD(ctx, logger, at, deadline); err != nil {
			return err
		}
	}
	return nil
}

func (r *StartupReconciler) syncActionTypeCRD(ctx context.Context, logger logr.Logger, at *atv1alpha1.ActionType, deadline time.Time) error {
	atLogger := logger.WithValues("actiontype", at.Spec.Name)
	desc := ogenclient.ActionTypeDescription{
		What:      at.Spec.Description.What,
		WhenToUse: at.Spec.Description.WhenToUse,
	}
	desc.WhenNotToUse.SetTo(at.Spec.Description.WhenNotToUse)
	desc.Preconditions.SetTo(at.Spec.Description.Preconditions)

	backoff := r.InitialBackoff
	for {
		result, err := r.DSActionType.CreateActionType(ctx, at.Spec.Name, desc, startupRegisteredBy)
		if err == nil {
			atLogger.Info("ActionType synced with DS", "status", result.Status)

			at.Status.Registered = true
			at.Status.CatalogStatus = sharedtypes.CatalogStatusActive
			at.Status.RegisteredBy = startupRegisteredBy
			now := metav1.Now()
			at.Status.RegisteredAt = &now

			if statusErr := r.K8sClient.Status().Update(ctx, at); statusErr != nil {
				atLogger.Error(statusErr, "Failed to update ActionType CRD status")
			}
			return nil
		}

		if time.Now().Add(backoff).After(deadline) {
			return fmt.Errorf("DS unavailable for ActionType %q after retries: %w", at.Spec.Name, err)
		}

		atLogger.Info("DS unavailable, retrying", "backoff", backoff, "error", err)
		select {
		case <-ctx.Done():
			return fmt.Errorf("context cancelled while syncing ActionType %q: %w", at.Spec.Name, ctx.Err())
		case <-time.After(backoff):
			backoff *= 2
		}
	}
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

	logger.Info("Syncing RemediationWorkflow CRDs with DS", "count", len(rwList.Items))

	var succeeded, failed int
	for i := range rwList.Items {
		rw := &rwList.Items[i]
		if err := r.syncWorkflowCRD(ctx, logger, rw, deadline); err != nil {
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

func (r *StartupReconciler) syncWorkflowCRD(ctx context.Context, logger logr.Logger, rw *rwv1alpha1.RemediationWorkflow, deadline time.Time) error {
	rwLogger := logger.WithValues("workflow", rw.Name)

	content, err := marshalCleanCRDContent(rw)
	if err != nil {
		r.markWorkflowFailed(ctx, rwLogger, rw, rwv1alpha1.ReasonValidationFailed,
			fmt.Sprintf("failed to marshal workflow: %v", err))
		return err
	}

	backoff := r.InitialBackoff
	for {
		result, err := r.DSWorkflow.CreateWorkflowInline(ctx, string(content), rw.Name, startupRegisteredBy)
		if err == nil {
			rwLogger.Info("Workflow synced with DS",
				"workflow_id", result.WorkflowID,
				"status", result.Status,
				"previously_existed", result.PreviouslyExisted,
			)
			r.markWorkflowSucceeded(ctx, rwLogger, rw, result)
			return nil
		}

		if IsPermanentError(err) {
			rwLogger.Error(err, "Workflow registration permanently failed — marking as Disabled",
				"remediation", "fix the referenced dependency and re-apply the RW CR")
			r.markWorkflowFailed(ctx, rwLogger, rw, rwv1alpha1.ReasonDependencyMissing, err.Error())
			return err
		}

		if time.Now().Add(backoff).After(deadline) {
			rwLogger.Error(err, "DS unavailable for workflow after retries — marking as Disabled",
				"remediation", "ensure DataStorage is reachable and re-apply the RW CR")
			r.markWorkflowFailed(ctx, rwLogger, rw, rwv1alpha1.ReasonDataStorageError,
				fmt.Sprintf("DS unavailable after retries: %v", err))
			return err
		}

		rwLogger.Info("DS unavailable, retrying", "backoff", backoff, "error", err)
		select {
		case <-ctx.Done():
			r.markWorkflowFailed(ctx, rwLogger, rw, rwv1alpha1.ReasonDataStorageError, "context cancelled")
			return fmt.Errorf("context cancelled while syncing Workflow %q: %w", rw.Name, ctx.Err())
		case <-time.After(backoff):
			backoff *= 2
		}
	}
}

func (r *StartupReconciler) markWorkflowSucceeded(ctx context.Context, logger logr.Logger, rw *rwv1alpha1.RemediationWorkflow, result *WorkflowRegistrationResult) {
	err := retry.RetryOnConflict(retry.DefaultBackoff, func() error {
		fresh := &rwv1alpha1.RemediationWorkflow{}
		if getErr := r.K8sClient.Get(ctx, client.ObjectKeyFromObject(rw), fresh); getErr != nil {
			return getErr
		}

		now := metav1.Now()
		fresh.Status.WorkflowID = result.WorkflowID
		fresh.Status.CatalogStatus = sharedtypes.CatalogStatus(result.Status)
		fresh.Status.RegisteredBy = startupRegisteredBy
		fresh.Status.RegisteredAt = &now
		fresh.Status.PreviouslyExisted = result.PreviouslyExisted
		setCondition(&fresh.Status.Conditions, metav1.Condition{
			Type:               rwv1alpha1.ConditionReady,
			Status:             metav1.ConditionTrue,
			Reason:             rwv1alpha1.ReasonRegistered,
			Message:            "Workflow registered successfully in DataStorage catalog",
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
