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
	ogenclient "github.com/jordigilh/kubernaut/pkg/datastorage/ogen-client"
	sharedtypes "github.com/jordigilh/kubernaut/pkg/shared/types"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
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
// Ordering: ActionType CRDs are synced first, then RemediationWorkflow CRDs,
// because workflows reference action types.
type StartupReconciler struct {
	K8sClient      client.Client
	DSWorkflow     WorkflowDSClient
	DSActionType   ActionTypeDSClient
	Logger         logr.Logger
	Timeout        time.Duration
	InitialBackoff time.Duration
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

	for i := range rwList.Items {
		rw := &rwList.Items[i]
		if err := r.syncWorkflowCRD(ctx, logger, rw, deadline); err != nil {
			return err
		}
	}
	return nil
}

func (r *StartupReconciler) syncWorkflowCRD(ctx context.Context, logger logr.Logger, rw *rwv1alpha1.RemediationWorkflow, deadline time.Time) error {
	rwLogger := logger.WithValues("workflow", rw.Name)

	content, err := marshalCleanCRDContent(rw)
	if err != nil {
		return fmt.Errorf("failed to marshal workflow %q for DS: %w", rw.Name, err)
	}

	backoff := r.InitialBackoff
	for {
		result, err := r.DSWorkflow.CreateWorkflowInline(ctx, string(content), "crd-startup-reconcile", startupRegisteredBy)
		if err == nil {
			rwLogger.Info("Workflow synced with DS",
				"workflow_id", result.WorkflowID,
				"status", result.Status,
				"previously_existed", result.PreviouslyExisted,
			)

			rw.Status.WorkflowID = result.WorkflowID
			rw.Status.CatalogStatus = sharedtypes.CatalogStatus(result.Status)
			rw.Status.RegisteredBy = startupRegisteredBy
			now := metav1.Now()
			rw.Status.RegisteredAt = &now
			rw.Status.PreviouslyExisted = result.PreviouslyExisted

			if statusErr := r.K8sClient.Status().Update(ctx, rw); statusErr != nil {
				rwLogger.Error(statusErr, "Failed to update RemediationWorkflow CRD status")
			}
			return nil
		}

		if time.Now().Add(backoff).After(deadline) {
			return fmt.Errorf("DS unavailable for Workflow %q after retries: %w", rw.Name, err)
		}

		rwLogger.Info("DS unavailable, retrying", "backoff", backoff, "error", err)
		select {
		case <-ctx.Done():
			return fmt.Errorf("context cancelled while syncing Workflow %q: %w", rw.Name, ctx.Err())
		case <-time.After(backoff):
			backoff *= 2
		}
	}
}
