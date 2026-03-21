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
	"errors"
	"net"
	"strings"
	"time"

	"github.com/go-logr/logr"
	atv1alpha1 "github.com/jordigilh/kubernaut/api/actiontype/v1alpha1"
	rwv1alpha1 "github.com/jordigilh/kubernaut/api/remediationworkflow/v1alpha1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
)

const (
	// RWFinalizerName ensures DS catalog consistency before RW deletion.
	// Issue #418: fire-and-forget goroutines in the webhook cannot guarantee
	// that the DS disable + AT count refresh complete before the CRD is removed.
	RWFinalizerName = "remediationworkflow.kubernaut.ai/catalog-cleanup"
)

// RemediationWorkflowReconciler ensures DS catalog consistency for RW CRD
// lifecycle events. It adds a finalizer on creation and, during deletion,
// guarantees the workflow is disabled in DS and the parent ActionType's
// activeWorkflowCount is refreshed before the CRD is removed from etcd.
//
// Issue #418, BR-WORKFLOW-006, BR-WORKFLOW-007.
type RemediationWorkflowReconciler struct {
	client.Client
	Log       logr.Logger
	DSClient  WorkflowCatalogClient
	ATCounter ActionTypeWorkflowCounter
}

//+kubebuilder:rbac:groups=kubernaut.ai,resources=remediationworkflows,verbs=get;list;watch;update;patch
//+kubebuilder:rbac:groups=kubernaut.ai,resources=remediationworkflows/status,verbs=get;update;patch
//+kubebuilder:rbac:groups=kubernaut.ai,resources=remediationworkflows/finalizers,verbs=update
//+kubebuilder:rbac:groups=kubernaut.ai,resources=actiontypes,verbs=get;list
//+kubebuilder:rbac:groups=kubernaut.ai,resources=actiontypes/status,verbs=get;update;patch

func (r *RemediationWorkflowReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	logger := r.Log.WithValues("name", req.Name, "namespace", req.Namespace)

	rw := &rwv1alpha1.RemediationWorkflow{}
	if err := r.Get(ctx, req.NamespacedName, rw); err != nil {
		if apierrors.IsNotFound(err) {
			return ctrl.Result{}, nil
		}
		return ctrl.Result{}, err
	}

	if !rw.DeletionTimestamp.IsZero() {
		return r.reconcileDelete(ctx, logger, rw)
	}

	if !controllerutil.ContainsFinalizer(rw, RWFinalizerName) {
		logger.Info("Adding finalizer")
		controllerutil.AddFinalizer(rw, RWFinalizerName)
		if err := r.Update(ctx, rw); err != nil {
			return ctrl.Result{}, err
		}
		return ctrl.Result{Requeue: true}, nil
	}

	return ctrl.Result{}, nil
}

// reconcileDelete handles the deletion flow:
//  1. Disable the workflow in DS (retry until success)
//  2. Refresh the parent ActionType's activeWorkflowCount from DS
//  3. Remove the finalizer so K8s completes the deletion
func (r *RemediationWorkflowReconciler) reconcileDelete(ctx context.Context, logger logr.Logger, rw *rwv1alpha1.RemediationWorkflow) (ctrl.Result, error) {
	if !controllerutil.ContainsFinalizer(rw, RWFinalizerName) {
		return ctrl.Result{}, nil
	}

	logger.Info("Reconciling deletion")

	workflowID := rw.Status.WorkflowID
	if workflowID != "" {
		if err := r.DSClient.DisableWorkflow(ctx, workflowID, "CRD deleted (finalizer)", ""); err != nil {
			if isConnectionError(err) {
				// Issue #469: During helm uninstall + reinstall, DS may be
				// unreachable. Proceed with finalizer removal so CRDs don't
				// get stuck in Terminating. The stale catalog entry (if any)
				// will be overwritten by the seed job on next install.
				logger.Error(err, "DS unreachable during deletion, proceeding with finalizer removal", "workflowID", workflowID)
			} else {
				logger.Error(err, "DS DisableWorkflow failed, will retry", "workflowID", workflowID)
				return ctrl.Result{RequeueAfter: 5 * time.Second}, nil
			}
		} else {
			logger.Info("Workflow disabled in DS", "workflowID", workflowID)
		}
	} else {
		logger.Info("No WorkflowID in status — skipping DS disable")
	}

	r.refreshActionTypeWorkflowCount(ctx, logger, rw.Spec.ActionType, rw.Namespace)

	controllerutil.RemoveFinalizer(rw, RWFinalizerName)
	if err := r.Update(ctx, rw); err != nil {
		logger.Error(err, "Failed to remove finalizer")
		return ctrl.Result{}, err
	}

	logger.Info("Finalizer removed, deletion complete")
	return ctrl.Result{}, nil
}

// refreshActionTypeWorkflowCount queries DS for the authoritative active
// workflow count and patches the matching ActionType CRD's status.
// Best-effort: errors are logged but do not block finalizer removal.
func (r *RemediationWorkflowReconciler) refreshActionTypeWorkflowCount(ctx context.Context, logger logr.Logger, actionType, namespace string) {
	if r.ATCounter == nil || actionType == "" {
		return
	}

	count, err := r.ATCounter.GetActiveWorkflowCount(ctx, actionType)
	if err != nil {
		logger.Error(err, "Failed to fetch active workflow count from DS", "actionType", actionType)
		return
	}

	atList := &atv1alpha1.ActionTypeList{}
	if err := r.List(ctx, atList, client.InNamespace(namespace)); err != nil {
		logger.Error(err, "Failed to list ActionType CRDs")
		return
	}

	for i := range atList.Items {
		at := &atList.Items[i]
		if at.Spec.Name != actionType {
			continue
		}
		at.Status.ActiveWorkflowCount = count
		if err := r.Status().Update(ctx, at); err != nil {
			if apierrors.IsConflict(err) {
				logger.V(1).Info("AT status update conflict (will be corrected on next reconcile)", "crd", at.Name)
			} else {
				logger.Error(err, "Failed to update AT activeWorkflowCount", "crd", at.Name, "count", count)
			}
		} else {
			logger.Info("ActionType activeWorkflowCount updated", "crd", at.Name, "count", count)
		}
		return
	}
}

func (r *RemediationWorkflowReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&rwv1alpha1.RemediationWorkflow{}).
		Complete(r)
}

// isConnectionError returns true when err indicates that DataStorage is
// unreachable (connection refused, DNS failure, timeout). These are expected
// during helm uninstall when DS pods are being deleted concurrently.
func isConnectionError(err error) bool {
	if err == nil {
		return false
	}
	var netErr *net.OpError
	if errors.As(err, &netErr) {
		return true
	}
	var dnsErr *net.DNSError
	if errors.As(err, &dnsErr) {
		return true
	}
	msg := err.Error()
	return strings.Contains(msg, "connection refused") ||
		strings.Contains(msg, "no such host") ||
		strings.Contains(msg, "i/o timeout")
}
