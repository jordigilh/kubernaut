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
	"time"

	"github.com/go-logr/logr"
	"github.com/prometheus/client_golang/prometheus"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"

	signalprocessingv1alpha1 "github.com/jordigilh/kubernaut/api/signalprocessing/v1alpha1"
	"github.com/jordigilh/kubernaut/pkg/signalprocessing/classifier"
	"github.com/jordigilh/kubernaut/pkg/signalprocessing/detection"
	"github.com/jordigilh/kubernaut/pkg/signalprocessing/enricher"
	"github.com/jordigilh/kubernaut/pkg/signalprocessing/metrics"
	"github.com/jordigilh/kubernaut/pkg/signalprocessing/ownerchain"
	sharedtypes "github.com/jordigilh/kubernaut/pkg/shared/types"
)

// DefaultEnrichmentTimeout is the default timeout for K8s API calls during enrichment.
const DefaultEnrichmentTimeout = 10 * time.Second

// SignalProcessingReconciler reconciles a SignalProcessing object.
// Implements the reconciliation loop per Appendix B: CRD Controller Patterns.
//
// BR-SP-051: Phase State Machine
// BR-SP-052: K8s Enrichment
// BR-SP-053: Environment Classification
// BR-SP-054: Priority Classification
// BR-SP-100: OwnerChain Traversal
// BR-SP-101: DetectedLabels Auto-Detection
type SignalProcessingReconciler struct {
	client.Client
	Scheme *runtime.Scheme
	Logger logr.Logger

	// Core enrichment components
	K8sEnricher   *enricher.K8sEnricher
	OwnerBuilder  *ownerchain.Builder
	LabelDetector *detection.LabelDetector

	// Classification components
	EnvClassifier      *classifier.EnvironmentClassifier
	PriorityClassifier *classifier.PriorityClassifier
	BusinessClassifier *classifier.BusinessClassifier

	// Metrics
	Metrics *metrics.Metrics
}

// NewReconcilerWithDependencies creates a new SignalProcessingReconciler with all dependencies.
// This constructor is used for testing with dependency injection.
// For tests, it creates a new Prometheus registry per instance to avoid duplicate registration panics.
func NewReconcilerWithDependencies(c client.Client, scheme *runtime.Scheme, logger logr.Logger) *SignalProcessingReconciler {
	// Use a separate registry for each reconciler to avoid duplicate metric registration in tests
	reg := prometheus.NewRegistry()
	m := metrics.NewMetricsWithRegistry(reg)
	return &SignalProcessingReconciler{
		Client:  c,
		Scheme:  scheme,
		Logger:  logger,
		Metrics: m,
		// Initialize all components with the shared client
		// Pass nil metrics for enricher since we're using component-level metrics
		K8sEnricher:        enricher.NewK8sEnricher(c, logger, nil, DefaultEnrichmentTimeout),
		OwnerBuilder:       ownerchain.NewBuilder(c, logger),
		LabelDetector:      detection.NewLabelDetector(c, logger),
		EnvClassifier:      classifier.NewEnvironmentClassifier(logger),
		PriorityClassifier: classifier.NewPriorityClassifier(logger),
		BusinessClassifier: classifier.NewBusinessClassifier(logger),
	}
}

// +kubebuilder:rbac:groups=signalprocessing.kubernaut.ai,resources=signalprocessings,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=signalprocessing.kubernaut.ai,resources=signalprocessings/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=signalprocessing.kubernaut.ai,resources=signalprocessings/finalizers,verbs=update
// +kubebuilder:rbac:groups="",resources=pods;deployments;replicasets;nodes;services;configmaps;namespaces,verbs=get;list;watch
// +kubebuilder:rbac:groups=apps,resources=deployments;replicasets;statefulsets;daemonsets,verbs=get;list;watch
// +kubebuilder:rbac:groups=autoscaling,resources=horizontalpodautoscalers,verbs=get;list;watch
// +kubebuilder:rbac:groups=policy,resources=poddisruptionbudgets,verbs=get;list;watch
// +kubebuilder:rbac:groups=networking.k8s.io,resources=networkpolicies,verbs=get;list;watch

// Reconcile implements the reconciliation loop for SignalProcessing CRDs.
// Pattern: Fetch → Check Terminal → Phase State Machine → Update Status
//
// BR-SP-051: Phase State Machine
// Phases: pending → enriching → classifying → completed (or failed)
func (r *SignalProcessingReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	logger := log.FromContext(ctx)
	if r.Logger.GetSink() != nil {
		logger = r.Logger
	}

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
	if sp.Status.Phase == signalprocessingv1alpha1.PhaseCompleted ||
		sp.Status.Phase == signalprocessingv1alpha1.PhaseFailed {
		logger.Info("SignalProcessing already in terminal state", "phase", sp.Status.Phase)
		return ctrl.Result{}, nil
	}

	// 3. PHASE STATE MACHINE
	switch sp.Status.Phase {
	case "", signalprocessingv1alpha1.PhasePending:
		return r.handlePendingPhase(ctx, sp)
	case signalprocessingv1alpha1.PhaseEnriching:
		return r.handleEnrichingPhase(ctx, sp)
	case signalprocessingv1alpha1.PhaseClassifying:
		return r.handleClassifyingPhase(ctx, sp)
	default:
		logger.Error(nil, "Unknown phase", "phase", string(sp.Status.Phase))
		return ctrl.Result{}, nil
	}
}

// handlePendingPhase initializes the SignalProcessing and transitions to enriching.
// BR-SP-051: Phase State Machine - pending → enriching
func (r *SignalProcessingReconciler) handlePendingPhase(ctx context.Context, sp *signalprocessingv1alpha1.SignalProcessing) (ctrl.Result, error) {
	logger := log.FromContext(ctx)
	if r.Logger.GetSink() != nil {
		logger = r.Logger
	}

	logger.Info("Initializing SignalProcessing status", "name", sp.Name)
	sp.Status.Phase = signalprocessingv1alpha1.PhaseEnriching
	now := metav1.Now()
	sp.Status.StartTime = &now

	if err := r.Status().Update(ctx, sp); err != nil {
		logger.Error(err, "Failed to update status to enriching")
		return ctrl.Result{}, err
	}

	logger.Info("Status initialized", "phase", sp.Status.Phase)
	return ctrl.Result{Requeue: true}, nil
}

// handleEnrichingPhase performs K8s context enrichment.
// BR-SP-052: K8s Enrichment
// BR-SP-100: OwnerChain Traversal
// BR-SP-101: DetectedLabels Auto-Detection
func (r *SignalProcessingReconciler) handleEnrichingPhase(ctx context.Context, sp *signalprocessingv1alpha1.SignalProcessing) (ctrl.Result, error) {
	logger := log.FromContext(ctx)
	if r.Logger.GetSink() != nil {
		logger = r.Logger
	}

	logger.Info("Processing enriching phase", "name", sp.Name)

	// Initialize enrichment results if nil
	if sp.Status.EnrichmentResults == nil {
		sp.Status.EnrichmentResults = &sharedtypes.EnrichmentResults{}
	}

	// BR-SP-052: K8s Context Enrichment
	if r.K8sEnricher != nil {
		// Determine the enrichment method based on target resource kind
		var enrichResult *enricher.EnrichmentResult
		var err error

		switch sp.Spec.TargetResource.Kind {
		case "Pod":
			enrichResult, err = r.K8sEnricher.EnrichPodSignal(ctx,
				sp.Spec.TargetResource.Namespace,
				sp.Spec.TargetResource.Name)
		default:
			enrichResult, err = r.K8sEnricher.EnrichNamespaceOnly(ctx,
				sp.Spec.TargetResource.Namespace)
		}

		if err != nil {
			logger.Error(err, "K8s enrichment failed (non-fatal)")
			// Continue with partial enrichment
		} else if enrichResult != nil {
			// Map EnrichmentResult to KubernetesContext
			sp.Status.EnrichmentResults.KubernetesContext = &sharedtypes.KubernetesContext{
				Namespace:         sp.Spec.TargetResource.Namespace,
				NamespaceLabels:   enrichResult.NamespaceLabels,
				PodDetails:        enrichResult.Pod,
				NodeDetails:       enrichResult.Node,
				DeploymentDetails: enrichResult.Deployment,
			}
		}
	}

	// BR-SP-100: OwnerChain Traversal
	if r.OwnerBuilder != nil && sp.Status.EnrichmentResults.KubernetesContext != nil {
		ownerChain, err := r.OwnerBuilder.Build(ctx,
			sp.Spec.TargetResource.Namespace,
			sp.Spec.TargetResource.Kind,
			sp.Spec.TargetResource.Name)
		if err != nil {
			logger.Error(err, "OwnerChain build failed (non-fatal)")
		} else {
			sp.Status.EnrichmentResults.OwnerChain = ownerChain
		}
	}

	// BR-SP-101: DetectedLabels Auto-Detection
	if r.LabelDetector != nil && sp.Status.EnrichmentResults.KubernetesContext != nil {
		detectedLabels := r.LabelDetector.DetectLabels(ctx, sp.Status.EnrichmentResults.KubernetesContext)
		sp.Status.EnrichmentResults.DetectedLabels = detectedLabels
	}

	// Transition to classifying phase
	sp.Status.Phase = signalprocessingv1alpha1.PhaseClassifying

	if err := r.Status().Update(ctx, sp); err != nil {
		logger.Error(err, "Failed to update status to classifying")
		return ctrl.Result{}, err
	}

	logger.Info("Enrichment complete, transitioning to classifying", "name", sp.Name)
	return ctrl.Result{Requeue: true}, nil
}

// handleClassifyingPhase performs environment and priority classification.
// BR-SP-053: Environment Classification
// BR-SP-054: Priority Classification
// BR-SP-080, BR-SP-081: Business Classification
func (r *SignalProcessingReconciler) handleClassifyingPhase(ctx context.Context, sp *signalprocessingv1alpha1.SignalProcessing) (ctrl.Result, error) {
	logger := log.FromContext(ctx)
	if r.Logger.GetSink() != nil {
		logger = r.Logger
	}

	logger.Info("Processing classifying phase", "name", sp.Name)

	// Initialize classification results
	sp.Status.ClassificationResults = &signalprocessingv1alpha1.ClassificationResults{}

	// Get namespace labels for classification
	var namespaceLabels map[string]string
	if sp.Status.EnrichmentResults != nil && sp.Status.EnrichmentResults.KubernetesContext != nil {
		namespaceLabels = sp.Status.EnrichmentResults.KubernetesContext.NamespaceLabels
	}

	// BR-SP-053: Environment Classification
	if r.EnvClassifier != nil {
		envResult := r.EnvClassifier.Classify(ctx, namespaceLabels, sp.Spec.SignalLabels)
		sp.Status.ClassificationResults.Environment = envResult.Environment
		sp.Status.ClassificationResults.EnvironmentConfidence = envResult.Confidence
	} else {
		// Fallback: use spec environment or derive from namespace labels
		if sp.Spec.Environment != "" {
			sp.Status.ClassificationResults.Environment = sp.Spec.Environment
		} else if env, ok := namespaceLabels["environment"]; ok {
			sp.Status.ClassificationResults.Environment = env
		} else {
			sp.Status.ClassificationResults.Environment = "development"
		}
		sp.Status.ClassificationResults.EnvironmentConfidence = 0.8
	}

	// BR-SP-054: Priority Classification
	if r.PriorityClassifier != nil {
		priorityResult := r.PriorityClassifier.Classify(ctx, sp.Spec.Severity,
			sp.Status.ClassificationResults.Environment)
		sp.Status.ClassificationResults.Priority = priorityResult.Priority
		sp.Status.ClassificationResults.PriorityConfidence = priorityResult.Confidence
	} else {
		// Fallback: use spec priority or derive from severity
		if sp.Spec.Priority != "" {
			sp.Status.ClassificationResults.Priority = sp.Spec.Priority
		} else {
			// Map severity to priority
			switch sp.Spec.Severity {
			case "critical":
				sp.Status.ClassificationResults.Priority = "P1"
			case "warning":
				sp.Status.ClassificationResults.Priority = "P2"
			default:
				sp.Status.ClassificationResults.Priority = "P3"
			}
		}
		sp.Status.ClassificationResults.PriorityConfidence = 0.8
	}

	// BR-SP-080, BR-SP-081: Business Classification
	if r.BusinessClassifier != nil {
		businessResult := r.BusinessClassifier.Classify(ctx, namespaceLabels, nil)
		sp.Status.ClassificationResults.BusinessUnit = businessResult.BusinessUnit
		sp.Status.ClassificationResults.ServiceOwner = businessResult.ServiceOwner
		sp.Status.ClassificationResults.Criticality = businessResult.Criticality
		sp.Status.ClassificationResults.SLARequirement = businessResult.SLARequirement
		sp.Status.ClassificationResults.OverallConfidence = businessResult.OverallConfidence
	}

	// Transition to completed phase
	sp.Status.Phase = signalprocessingv1alpha1.PhaseCompleted
	now := metav1.Now()
	sp.Status.CompletedAt = &now

	if err := r.Status().Update(ctx, sp); err != nil {
		logger.Error(err, "Failed to update status to completed")
		return ctrl.Result{}, err
	}

	logger.Info("Classification complete, SignalProcessing completed", "name", sp.Name,
		"environment", sp.Status.ClassificationResults.Environment,
		"priority", sp.Status.ClassificationResults.Priority)
	return ctrl.Result{}, nil
}

// SetupWithManager sets up the controller with the Manager.
func (r *SignalProcessingReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&signalprocessingv1alpha1.SignalProcessing{}).
		Complete(r)
}
