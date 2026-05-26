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
package controller

import (
	"context"
	"fmt"
	"strings"

	"github.com/go-logr/logr"
	coordinationv1 "k8s.io/api/coordination/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"

	v1alpha1 "github.com/jordigilh/kubernaut/api/investigationsession/v1alpha1"
)

// LeaseNamePrefix is the prefix KA uses for interactive session leases.
const LeaseNamePrefix = "kubernaut-interactive-"

// LeaseSyncReconciler watches InvestigationSession CRDs and syncs the
// corresponding KA Lease's holderIdentity and acquireTime into the IS
// status fields (LeaseHolder, LeaseAcquiredAt). This enables AF to
// expose session driver information without requiring direct Lease access
// from API consumers (BR-INTERACTIVE-007, AC-6).
type LeaseSyncReconciler struct {
	client    client.Client
	logger    logr.Logger
	namespace string
}

// NewLeaseSyncReconciler creates a reconciler that syncs KA Lease state
// to InvestigationSession CRD status.
func NewLeaseSyncReconciler(c client.Client, namespace string, logger logr.Logger) *LeaseSyncReconciler {
	if logger.GetSink() == nil {
		logger = logr.Discard()
	}
	return &LeaseSyncReconciler{
		client:    c,
		logger:    logger,
		namespace: namespace,
	}
}

// SetupWithManager registers the LeaseSyncReconciler.
func (r *LeaseSyncReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&v1alpha1.InvestigationSession{}).
		Named("lease-sync").
		Complete(r)
}

// Reconcile syncs KA Lease state into IS CRD status.
func (r *LeaseSyncReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	var sess v1alpha1.InvestigationSession
	if err := r.client.Get(ctx, req.NamespacedName, &sess); err != nil {
		if apierrors.IsNotFound(err) {
			return ctrl.Result{}, nil
		}
		return ctrl.Result{}, fmt.Errorf("get session: %w", err)
	}

	if sess.Status.Phase != v1alpha1.SessionPhaseActive {
		return ctrl.Result{}, nil
	}

	rrRef := sess.Spec.RemediationRequestRef
	leaseName := leaseNameFromRR(rrRef.Namespace, rrRef.Name)

	var lease coordinationv1.Lease
	leaseNN := types.NamespacedName{Name: leaseName, Namespace: r.namespace}
	if err := r.client.Get(ctx, leaseNN, &lease); err != nil {
		if apierrors.IsNotFound(err) {
			return ctrl.Result{}, nil
		}
		return ctrl.Result{}, fmt.Errorf("get lease %s: %w", leaseName, err)
	}

	holder := ""
	if lease.Spec.HolderIdentity != nil {
		holder = *lease.Spec.HolderIdentity
	}
	var acquiredAt *metav1.Time
	if lease.Spec.AcquireTime != nil {
		t := metav1.NewTime(lease.Spec.AcquireTime.Time)
		acquiredAt = &t
	}

	if sess.Status.LeaseHolder == holder {
		return ctrl.Result{}, nil
	}

	sess.Status.LeaseHolder = holder
	sess.Status.LeaseAcquiredAt = acquiredAt
	if err := r.client.Status().Update(ctx, &sess); err != nil {
		return ctrl.Result{}, fmt.Errorf("update session lease status: %w", err)
	}

	r.logger.Info("synced lease state to IS CRD",
		"session", sess.Name,
		"leaseHolder", holder,
	)
	return ctrl.Result{}, nil
}

// leaseNameFromRR builds the KA lease name from an RR reference.
// KA uses "kubernaut-interactive-{namespace}-{name}" to avoid "/" in lease names.
func leaseNameFromRR(namespace, name string) string {
	sanitized := strings.ReplaceAll(namespace+"-"+name, "/", "-")
	return LeaseNamePrefix + sanitized
}
