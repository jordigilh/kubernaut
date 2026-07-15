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

	"github.com/go-logr/logr"
	atv1alpha1 "github.com/jordigilh/kubernaut/api/actiontype/v1alpha1"
	rwv1alpha1 "github.com/jordigilh/kubernaut/api/remediationworkflow/v1alpha1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/client-go/util/retry"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
)

const (
	// RWFinalizerName guarantees the parent ActionType's activeWorkflowCount
	// refresh completes before the CRD is removed from etcd. Issue #418:
	// fire-and-forget goroutines in the webhook cannot guarantee this on
	// their own. #1661 Change 8c: no longer also guards a DS-disable step --
	// DELETE is a true etcd removal with no DS-side "disabled" state.
	RWFinalizerName = "remediationworkflow.kubernaut.ai/catalog-cleanup"
)

// RemediationWorkflowReconciler guarantees the parent ActionType's
// activeWorkflowCount is refreshed before an RW CRD is removed from etcd. It
// adds a finalizer on creation and, during deletion, performs that refresh
// before allowing the finalizer to be removed.
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
//  1. Refresh the parent ActionType's activeWorkflowCount from DS
//  2. Remove the finalizer so K8s completes the deletion
//
// #1661 Change 8c: the DS-disable step is removed entirely -- DELETE is now a
// true etcd removal with no DS-side "disabled" state to notify. This
// finalizer's only remaining job is the ActionType count refresh guarantee.
func (r *RemediationWorkflowReconciler) reconcileDelete(ctx context.Context, logger logr.Logger, rw *rwv1alpha1.RemediationWorkflow) (ctrl.Result, error) {
	if !controllerutil.ContainsFinalizer(rw, RWFinalizerName) {
		return ctrl.Result{}, nil
	}

	logger.Info("Reconciling deletion")

	r.refreshActionTypeWorkflowCount(ctx, logger, rw.Spec.ActionType, rw.Namespace, rw.Name)

	controllerutil.RemoveFinalizer(rw, RWFinalizerName)
	if err := r.Update(ctx, rw); err != nil {
		logger.Error(err, "Failed to remove finalizer")
		return ctrl.Result{}, err
	}

	logger.Info("Finalizer removed, deletion complete")
	return ctrl.Result{}, nil
}

// refreshActionTypeWorkflowCount counts live RemediationWorkflow CRDs
// referencing actionType and patches the matching ActionType CRD's status.
// Uses RetryOnConflict to handle races with the webhook's concurrent
// status update goroutine (fixes E2E-AT-300-003 flake).
// Best-effort: errors are logged but do not block finalizer removal.
//
// #1661 Change 8d: the count is now a direct K8s-native list against the
// cache-backed client -- DS's Postgres catalog stopped learning about
// RemediationWorkflow CRDs the moment Change 8c removed AW's
// CreateWorkflowInline call, making a DS-backed count permanently stale
// (DD-WORKFLOW-018). excludeRWName excludes the RW currently being deleted
// (still present in etcd at this point -- finalizers block physical removal
// until this reconcile completes) so the count reflects what remains
// afterward, not what remains including it (see listDependentWorkflowNames).
func (r *RemediationWorkflowReconciler) refreshActionTypeWorkflowCount(ctx context.Context, logger logr.Logger, actionType, namespace, excludeRWName string) {
	if actionType == "" {
		return
	}

	dependents, err := listDependentWorkflowNames(ctx, r.Client, actionType, excludeRWName)
	if err != nil {
		logger.Error(err, "Failed to list dependent RemediationWorkflows", "actionType", actionType)
		return
	}
	count := len(dependents)

	atKey, err := r.findActionTypeKey(ctx, actionType, namespace)
	if err != nil {
		logger.Error(err, "Failed to find ActionType CRD", "actionType", actionType)
		return
	}
	if atKey == nil {
		return
	}

	if err := retry.RetryOnConflict(retry.DefaultRetry, func() error {
		at := &atv1alpha1.ActionType{}
		if err := r.Get(ctx, *atKey, at); err != nil {
			return fmt.Errorf("get ActionType: %w", err)
		}
		at.Status.ActiveWorkflowCount = count
		return r.Status().Update(ctx, at)
	}); err != nil {
		logger.Error(err, "Failed to update AT activeWorkflowCount", "actionType", actionType, "count", count)
	} else {
		logger.Info("ActionType activeWorkflowCount updated", "actionType", actionType, "count", count)
	}
}

// findActionTypeKey locates the ActionType CRD whose spec.name matches actionType.
func (r *RemediationWorkflowReconciler) findActionTypeKey(ctx context.Context, actionType, namespace string) (*client.ObjectKey, error) {
	atList := &atv1alpha1.ActionTypeList{}
	if err := r.List(ctx, atList, client.InNamespace(namespace)); err != nil {
		return nil, err
	}
	for i := range atList.Items {
		if atList.Items[i].Spec.Name == actionType {
			key := client.ObjectKeyFromObject(&atList.Items[i])
			return &key, nil
		}
	}
	return nil, nil
}

func (r *RemediationWorkflowReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&rwv1alpha1.RemediationWorkflow{}).
		Complete(r)
}
