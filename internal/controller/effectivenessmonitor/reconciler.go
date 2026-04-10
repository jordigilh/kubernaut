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

// Package controller provides the Kubernetes controller for EffectivenessAssessment CRDs.
// The controller watches EA CRDs created by the Remediation Orchestrator and performs
// effectiveness assessment checks (health, alert, metrics, hash).
//
// Architecture: ADR-EM-001 (Effectiveness Monitor Service Integration)
// Controller Pattern: controller-runtime reconciler with dependency injection
//
// Business Requirements:
// - BR-EM-001 through BR-EM-004: Component assessment checks
// - BR-EM-005: Phase state transitions
// - BR-EM-006: Stabilization window
// - BR-EM-007: Validity window
// - BR-AUDIT-006: SOC 2 audit trail
package controller

import (
	"context"
	"fmt"
	"time"

	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/tools/record"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	ctrlcontroller "sigs.k8s.io/controller-runtime/pkg/controller"
	"sigs.k8s.io/controller-runtime/pkg/log"

	eav1 "github.com/jordigilh/kubernaut/api/effectivenessassessment/v1alpha1"
	"github.com/jordigilh/kubernaut/pkg/effectivenessmonitor/alert"
	emaudit "github.com/jordigilh/kubernaut/pkg/effectivenessmonitor/audit"
	emclient "github.com/jordigilh/kubernaut/pkg/effectivenessmonitor/client"
	emconfig "github.com/jordigilh/kubernaut/pkg/effectivenessmonitor/config"
	"github.com/jordigilh/kubernaut/pkg/effectivenessmonitor/hash"
	"github.com/jordigilh/kubernaut/pkg/effectivenessmonitor/health"
	emmetrics "github.com/jordigilh/kubernaut/pkg/effectivenessmonitor/metrics"
	"github.com/jordigilh/kubernaut/pkg/effectivenessmonitor/phase"
	"github.com/jordigilh/kubernaut/pkg/effectivenessmonitor/validity"
)

// Reconciler reconciles EffectivenessAssessment objects.
// It performs the 4 assessment checks and emits audit events.
type Reconciler struct {
	client.Client
	Scheme   *runtime.Scheme
	Recorder record.EventRecorder

	// Dependencies (injected via NewReconciler)
	Metrics            *emmetrics.Metrics
	PrometheusClient   emclient.PrometheusQuerier
	AlertManagerClient emclient.AlertManagerClient
	AuditManager       *emaudit.Manager
	DSQuerier          emclient.DataStorageQuerier

	// Internal components (created in constructor)
	healthScorer    health.Scorer
	alertScorer     alert.Scorer
	metricScorer    emmetrics.Scorer
	hashComputer    hash.Computer
	validityChecker validity.Checker

	// restMapper resolves Kind to GVR for unstructured resource fetches.
	restMapper meta.RESTMapper
	// apiReader bypasses the informer cache for direct API server reads (DD-EM-002, #396).
	apiReader client.Reader

	// Configuration
	Config ReconcilerConfig
}

// ReconcilerConfig holds runtime configuration for the reconciler.
type ReconcilerConfig struct {
	// PrometheusEnabled indicates whether metric comparison is active.
	PrometheusEnabled bool
	// AlertManagerEnabled indicates whether alert resolution checking is active.
	AlertManagerEnabled bool

	// ValidityWindow is the maximum duration for assessment completion.
	// The EM computes ValidityDeadline = EA.creationTimestamp + ValidityWindow
	// on first reconciliation and stores it in EA.Status.ValidityDeadline.
	// Default: 30m (from effectivenessmonitor.Config.Assessment.ValidityWindow).
	ValidityWindow time.Duration

	// PrometheusLookback is the duration before EA creation to query Prometheus.
	// Default: 10 minutes. Shorter values improve E2E test speed.
	PrometheusLookback time.Duration
	// RequeueGenericError is the delay before retrying on transient errors.
	// Default: 5s (from emconfig.RequeueGenericError).
	RequeueGenericError time.Duration
	// RequeueAssessmentInProgress is the delay before retrying while waiting
	// for external data (e.g., Prometheus scrape).
	// Default: 15s (from emconfig.RequeueAssessmentInProgress).
	RequeueAssessmentInProgress time.Duration
}

// DefaultReconcilerConfig returns a ReconcilerConfig with production defaults.
func DefaultReconcilerConfig() ReconcilerConfig {
	return ReconcilerConfig{
		ValidityWindow:              30 * time.Minute,
		PrometheusLookback:          30 * time.Minute,
		RequeueGenericError:         emconfig.RequeueGenericError,
		RequeueAssessmentInProgress: emconfig.RequeueAssessmentInProgress,
	}
}

// NewReconciler creates a new Reconciler with all dependencies injected.
// Per DD-METRICS-001: Metrics wired via dependency injection.
// Per DD-AUDIT-003: AuditManager wired via dependency injection (Pattern 2).
func NewReconciler(
	c client.Client,
	apiReader client.Reader,
	s *runtime.Scheme,
	recorder record.EventRecorder,
	m *emmetrics.Metrics,
	promClient emclient.PrometheusQuerier,
	amClient emclient.AlertManagerClient,
	auditMgr *emaudit.Manager,
	dsQuerier emclient.DataStorageQuerier,
	cfg ReconcilerConfig,
) *Reconciler {
	return &Reconciler{
		Client:             c,
		apiReader:          apiReader,
		Scheme:             s,
		Recorder:           recorder,
		Metrics:            m,
		PrometheusClient:   promClient,
		AlertManagerClient: amClient,
		AuditManager:       auditMgr,
		DSQuerier:          dsQuerier,
		healthScorer:       health.NewScorer(),
		alertScorer:        alert.NewScorer(),
		metricScorer:       emmetrics.NewScorer(),
		hashComputer:       hash.NewComputer(),
		validityChecker:    validity.NewChecker(),
		Config:             cfg,
	}
}

// Reconcile handles a single reconciliation of an EffectivenessAssessment.
// This is the main entry point called by controller-runtime. Steps 1-2 run here;
// the active reconciliation flow is delegated to reconcileActive (reconcile_orchestrate.go).
func (r *Reconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	logger := log.FromContext(ctx)

	// Step 1: Fetch EA
	ea := &eav1.EffectivenessAssessment{}
	if err := r.Get(ctx, req.NamespacedName, ea); err != nil {
		if apierrors.IsNotFound(err) {
			return ctrl.Result{}, nil
		}
		logger.Error(err, "Failed to fetch EffectivenessAssessment")
		return ctrl.Result{RequeueAfter: r.Config.RequeueGenericError}, err
	}

	logger = logger.WithValues(
		"ea", ea.Name,
		"namespace", ea.Namespace,
		"correlationID", ea.Spec.CorrelationID,
		"phase", ea.Status.Phase,
	)

	// Step 1b: Spec validation
	if reason := validateEASpec(ea); reason != "" {
		return r.failAssessment(ctx, ea, reason)
	}

	// Step 2: Check terminal state
	currentPhase := ea.Status.Phase
	if currentPhase == "" {
		currentPhase = eav1.PhasePending
	}
	if phase.IsTerminal(currentPhase) {
		logger.V(1).Info("EA in terminal state, skipping reconciliation")
		return ctrl.Result{}, nil
	}

	rctx := &reconcileContext{
		ea:           ea,
		currentPhase: currentPhase,
	}
	return r.reconcileActive(ctx, rctx)
}

// SetupWithManager registers the controller with the manager.
// Creates a field index on spec.correlationID for O(1) lookups and kubectl
// field-selector support (e.g., kubectl get ea --field-selector spec.correlationID=rr-xxx).
func (r *Reconciler) SetupWithManager(mgr ctrl.Manager, maxConcurrentReconciles ...int) error {
	// Field index on spec.correlationID for efficient lookups and kubectl UX
	if err := mgr.GetFieldIndexer().IndexField(
		context.Background(),
		&eav1.EffectivenessAssessment{},
		"spec.correlationID",
		func(obj client.Object) []string {
			ea := obj.(*eav1.EffectivenessAssessment)
			if ea.Spec.CorrelationID == "" {
				return nil
			}
			return []string{ea.Spec.CorrelationID}
		},
	); err != nil {
		return fmt.Errorf("failed to create correlationID field index: %w", err)
	}

	builder := ctrl.NewControllerManagedBy(mgr).
		For(&eav1.EffectivenessAssessment{})

	if len(maxConcurrentReconciles) > 0 && maxConcurrentReconciles[0] > 0 {
		builder = builder.WithOptions(ctrlcontroller.TypedOptions[ctrl.Request]{
			MaxConcurrentReconciles: maxConcurrentReconciles[0],
		})
	}

	return builder.Complete(r)
}

// SetRESTMapper sets the REST mapper used to resolve Kind -> GVR for unstructured fetches.
// Called after NewReconciler and before SetupWithManager so the controller has access
// to the manager's discovery-backed mapper.
func (r *Reconciler) SetRESTMapper(rm meta.RESTMapper) {
	r.restMapper = rm
}
