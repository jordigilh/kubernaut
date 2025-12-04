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

// Package signalprocessing implements the SignalProcessing CRD controller.
// Design Decision: DD-006 Controller Scaffolding Strategy
// Business Requirements: BR-SP-001 to BR-SP-104
package signalprocessing

import (
	"context"

	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"

	signalprocessingv1alpha1 "github.com/jordigilh/kubernaut/api/signalprocessing/v1alpha1"
)

// Phase constants for SignalProcessing status
const (
	PhasePending   = "pending"
	PhaseEnriching = "enriching"
	PhaseCompleted = "completed"
	PhaseFailed    = "failed"
)

// SignalProcessingReconciler reconciles a SignalProcessing object.
// Implements the reconciliation loop per Appendix B: CRD Controller Patterns.
type SignalProcessingReconciler struct {
	client.Client
	Scheme *runtime.Scheme
	// TODO: Add dependencies in Day 2-6
	// Enricher   *enricher.Enricher
	// Classifier *classifier.Classifier
	// Detector   *detector.LabelDetector
	// Metrics    *metrics.Metrics
}

// +kubebuilder:rbac:groups=signalprocessing.kubernaut.ai,resources=signalprocessings,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=signalprocessing.kubernaut.ai,resources=signalprocessings/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=signalprocessing.kubernaut.ai,resources=signalprocessings/finalizers,verbs=update
// +kubebuilder:rbac:groups="",resources=pods;deployments;replicasets;nodes;services;configmaps,verbs=get;list;watch
// +kubebuilder:rbac:groups=apps,resources=deployments;replicasets;statefulsets,verbs=get;list;watch
// +kubebuilder:rbac:groups=autoscaling,resources=horizontalpodautoscalers,verbs=get;list;watch
// +kubebuilder:rbac:groups=policy,resources=poddisruptionbudgets,verbs=get;list;watch
// +kubebuilder:rbac:groups=networking.k8s.io,resources=networkpolicies,verbs=get;list;watch

// Reconcile implements the reconciliation loop for SignalProcessing CRDs.
// Pattern: Fetch → Check Terminal → Initialize → Process → Update Status
func (r *SignalProcessingReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	logger := log.FromContext(ctx)

	// 1. FETCH RESOURCE
	sp := &signalprocessingv1alpha1.SignalProcessing{}
	if err := r.Get(ctx, req.NamespacedName, sp); err != nil {
		if apierrors.IsNotFound(err) {
			// Resource deleted, nothing to do
			logger.Info("SignalProcessing resource not found, ignoring")
			return ctrl.Result{}, nil
		}
		logger.Error(err, "Failed to get SignalProcessing")
		return ctrl.Result{}, err
	}

	// 2. CHECK TERMINAL STATES
	if sp.Status.Phase == PhaseCompleted || sp.Status.Phase == PhaseFailed {
		logger.Info("SignalProcessing already in terminal state", "phase", sp.Status.Phase)
		return ctrl.Result{}, nil
	}

	// 3. INITIALIZE STATUS (if new resource)
	if sp.Status.Phase == "" || sp.Status.Phase == PhasePending {
		logger.Info("Initializing SignalProcessing status", "name", sp.Name)
		sp.Status.Phase = PhaseEnriching
		now := metav1.Now()
		sp.Status.StartTime = &now

		if err := r.Status().Update(ctx, sp); err != nil {
			logger.Error(err, "Failed to update status to enriching")
			return ctrl.Result{}, err
		}

		logger.Info("Status initialized", "phase", sp.Status.Phase)
	}

	// 4. BUSINESS LOGIC (TODO: Implement in Days 2-6)
	// - Enrich K8s context (Day 3)
	// - Classify environment/priority (Day 4-5)
	// - Detect labels (Day 7-9)
	// - Audit trail (Day 8)

	// 5. For now, just return without requeue (skeleton)
	// Full implementation will continue processing phases
	return ctrl.Result{}, nil
}

// SetupWithManager sets up the controller with the Manager.
func (r *SignalProcessingReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&signalprocessingv1alpha1.SignalProcessing{}).
		Complete(r)
}

