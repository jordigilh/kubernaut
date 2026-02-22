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

	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/tools/record"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"

	eav1 "github.com/jordigilh/kubernaut/api/effectivenessassessment/v1alpha1"
	"github.com/jordigilh/kubernaut/pkg/effectivenessmonitor/alert"
	emaudit "github.com/jordigilh/kubernaut/pkg/effectivenessmonitor/audit"
	emclient "github.com/jordigilh/kubernaut/pkg/effectivenessmonitor/client"
	"github.com/jordigilh/kubernaut/pkg/effectivenessmonitor/conditions"
	emconfig "github.com/jordigilh/kubernaut/pkg/effectivenessmonitor/config"
	"github.com/jordigilh/kubernaut/pkg/effectivenessmonitor/hash"
	"github.com/jordigilh/kubernaut/pkg/effectivenessmonitor/health"
	emmetrics "github.com/jordigilh/kubernaut/pkg/effectivenessmonitor/metrics"
	"github.com/jordigilh/kubernaut/pkg/effectivenessmonitor/phase"
	emtypes "github.com/jordigilh/kubernaut/pkg/effectivenessmonitor/types"
	"github.com/jordigilh/kubernaut/pkg/effectivenessmonitor/validity"
	"github.com/jordigilh/kubernaut/pkg/shared/events"
	canonicalhash "github.com/jordigilh/kubernaut/pkg/shared/hash"
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
		PrometheusLookback:          10 * time.Minute,
		RequeueGenericError:         emconfig.RequeueGenericError,
		RequeueAssessmentInProgress: emconfig.RequeueAssessmentInProgress,
	}
}

// NewReconciler creates a new Reconciler with all dependencies injected.
// Per DD-METRICS-001: Metrics wired via dependency injection.
// Per DD-AUDIT-003: AuditManager wired via dependency injection (Pattern 2).
func NewReconciler(
	c client.Client,
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
// This is the main entry point called by controller-runtime.
//
// Reconciliation flow:
//  1. Fetch EA from API server
//  2. Check if EA is in terminal state -> skip
//  3. Check validity window (expired takes priority)
//  4. If expired -> complete with partial data
//  5. If stabilizing -> requeue after stabilization
//  6. Transition Pending -> Assessing
//  7. Run component checks (skip already-completed components)
//  8. Update EA status with component results
//  9. If all components done -> complete assessment
//  10. Otherwise -> requeue for remaining components
func (r *Reconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	logger := log.FromContext(ctx)
	startTime := time.Now()

	// Step 1: Fetch EA
	ea := &eav1.EffectivenessAssessment{}
	if err := r.Get(ctx, req.NamespacedName, ea); err != nil {
		if apierrors.IsNotFound(err) {
			// EA was deleted, nothing to do
			return ctrl.Result{}, nil
		}
		logger.Error(err, "Failed to fetch EffectivenessAssessment")
		r.Metrics.RecordReconcile("error", time.Since(startTime).Seconds())
		return ctrl.Result{RequeueAfter: r.Config.RequeueGenericError}, err
	}

	logger = logger.WithValues(
		"ea", ea.Name,
		"namespace", ea.Namespace,
		"correlationID", ea.Spec.CorrelationID,
		"phase", ea.Status.Phase,
	)

	// Step 2: Check terminal state
	currentPhase := ea.Status.Phase
	if currentPhase == "" {
		currentPhase = eav1.PhasePending
	}
	if phase.IsTerminal(currentPhase) {
		logger.V(1).Info("EA in terminal state, skipping reconciliation")
		r.Metrics.RecordReconcile("skipped", time.Since(startTime).Seconds())
		return ctrl.Result{}, nil
	}

	// Step 3: Check validity window
	// ValidityDeadline is computed on first reconciliation (Pending -> Assessing).
	// If not yet set (first reconcile of a Pending EA), we skip expiration checks
	// and proceed to the phase transition which will compute and persist the deadline.
	var windowState validity.WindowState
	if ea.Status.ValidityDeadline != nil {
		windowState = r.validityChecker.Check(
			ea.CreationTimestamp,
			ea.Spec.Config.StabilizationWindow.Duration,
			*ea.Status.ValidityDeadline,
		)
	} else {
		// No deadline computed yet - check stabilization only
		stabilizationEnd := ea.CreationTimestamp.Add(ea.Spec.Config.StabilizationWindow.Duration)
		if time.Now().Before(stabilizationEnd) {
			windowState = validity.WindowStabilizing
		} else {
			windowState = validity.WindowActive
		}
	}

	// Step 4: If expired -> complete with partial data (ADR-EM-001: validity window enforcement)
	if windowState == validity.WindowExpired {
		logger.Info("Validity window expired, completing with available data")
		r.Recorder.Event(ea, corev1.EventTypeWarning, events.EventReasonAssessmentExpired,
			fmt.Sprintf("Validity window expired for correlation %s; completing with available data",
				ea.Spec.CorrelationID))
		r.Metrics.RecordK8sEvent("Warning", events.EventReasonAssessmentExpired)
		r.Metrics.RecordValidityExpiration()
		return r.completeAssessment(ctx, ea, startTime)
	}

	// Step 5: If stabilizing -> requeue
	if windowState == validity.WindowStabilizing {
		remaining := r.validityChecker.TimeUntilStabilized(
			ea.CreationTimestamp,
			ea.Spec.Config.StabilizationWindow.Duration,
		)
		logger.Info("Stabilization window active, requeueing", "remaining", remaining)
		r.Metrics.RecordStabilizationWait()
		r.Metrics.RecordReconcile("requeue", time.Since(startTime).Seconds())
		return ctrl.Result{RequeueAfter: remaining}, nil
	}

	// Step 6: Transition Pending -> Assessing + compute derived timing (BR-EM-009)
	//
	// We set phase and derived timing in-memory here but defer the status write to
	// Step 9 (single atomic update). This avoids an intermediate Status().Update()
	// that would change the resourceVersion and cause optimistic concurrency conflicts
	// when component checks later attempt their own status update.
	pendingTransition := false
	if currentPhase == eav1.PhasePending || currentPhase == "" {
		if !phase.CanTransition(currentPhase, eav1.PhaseAssessing) {
			logger.Error(nil, "Invalid phase transition", "from", currentPhase, "to", eav1.PhaseAssessing)
			r.Metrics.RecordReconcile("error", time.Since(startTime).Seconds())
			return ctrl.Result{RequeueAfter: r.Config.RequeueGenericError}, fmt.Errorf("invalid phase transition: %s -> %s", currentPhase, eav1.PhaseAssessing)
		}

		// Compute all derived timing fields on first reconciliation.
		// These are persisted in status to avoid recomputation and for operator observability.
		//
		// ValidityDeadline = EA.creationTimestamp + config.ValidityWindow (BR-EM-009.1)
		// PrometheusCheckAfter = EA.creationTimestamp + StabilizationWindow (BR-EM-009.2)
		// AlertManagerCheckAfter = EA.creationTimestamp + StabilizationWindow (BR-EM-009.3)
		//
		// The invariant StabilizationWindow < ValidityDeadline is guaranteed because
		// EM config validation enforces ValidityWindow > StabilizationWindow.
		deadline := metav1.NewTime(ea.CreationTimestamp.Add(r.Config.ValidityWindow))
		checkAfter := metav1.NewTime(ea.CreationTimestamp.Add(ea.Spec.Config.StabilizationWindow.Duration))

		ea.Status.Phase = eav1.PhaseAssessing
		ea.Status.ValidityDeadline = &deadline
		ea.Status.PrometheusCheckAfter = &checkAfter
		ea.Status.AlertManagerCheckAfter = &checkAfter

		logger.Info("Computed derived timing (BR-EM-009)",
			"creationTimestamp", ea.CreationTimestamp,
			"validityWindow", r.Config.ValidityWindow,
			"stabilizationWindow", ea.Spec.Config.StabilizationWindow.Duration,
			"validityDeadline", deadline,
			"prometheusCheckAfter", checkAfter,
			"alertManagerCheckAfter", checkAfter,
		)

		pendingTransition = true
	}

	// Step 6.5: Spec Drift Guard (DD-EM-002 v1.1)
	//
	// After the post-remediation hash is computed (HashComputed=true), re-hash
	// the target resource spec on every reconcile. If it differs, the resource
	// was modified (likely by another remediation) and the assessment is invalid.
	// Metrics and alerts would measure the wrong resource state.
	if ea.Status.Components.HashComputed && ea.Status.Components.PostRemediationSpecHash != "" {
		currentSpec := r.getTargetSpec(ctx, ea)
		currentHash, hashErr := canonicalhash.CanonicalSpecHash(currentSpec)
		if hashErr != nil {
			logger.Error(hashErr, "Failed to compute current spec hash for drift check")
		} else {
			ea.Status.Components.CurrentSpecHash = currentHash

			if currentHash != ea.Status.Components.PostRemediationSpecHash {
				logger.Info("Spec drift detected — resource modified since post-remediation hash (DD-EM-002 v1.1)",
					"postRemediationHash", ea.Status.Components.PostRemediationSpecHash,
					"currentHash", currentHash,
					"correlationID", ea.Spec.CorrelationID,
				)

				// Set SpecIntegrity condition to False (DD-CRD-002)
				conditions.SetCondition(ea, conditions.ConditionSpecIntegrity,
					metav1.ConditionFalse, conditions.ReasonSpecDrifted,
					fmt.Sprintf("Target spec hash changed: %s -> %s",
						ea.Status.Components.PostRemediationSpecHash, currentHash))

				// Set AssessmentComplete condition with SpecDrift reason
				conditions.SetCondition(ea, conditions.ConditionAssessmentComplete,
					metav1.ConditionTrue, conditions.ReasonSpecDrift,
					"Assessment invalidated: target resource spec was modified (spec drift detected)")

			r.Recorder.Event(ea, corev1.EventTypeWarning, events.EventReasonSpecDriftDetected,
				fmt.Sprintf("Target resource spec modified during assessment (correlation: %s)", ea.Spec.CorrelationID))
			r.Metrics.RecordK8sEvent("Warning", events.EventReasonSpecDriftDetected)

				// Complete with spec_drift reason — do NOT assess metrics/alerts
				return r.completeAssessmentWithReason(ctx, ea, startTime, eav1.AssessmentReasonSpecDrift)
			}

			// Spec unchanged — set positive condition
			conditions.SetCondition(ea, conditions.ConditionSpecIntegrity,
				metav1.ConditionTrue, conditions.ReasonSpecUnchanged,
				"Target resource spec unchanged since post-remediation hash")
		}
	}

	// Step 6b: no_execution guard (ADR-EM-001 Section 5)
	// If no workflowexecution.workflow.started event exists, the remediation failed
	// before execution began (e.g., AA failed, approval rejected). Skip all component
	// checks and complete with reason=no_execution.
	if r.DSQuerier != nil {
		started, err := r.DSQuerier.HasWorkflowStarted(ctx, ea.Spec.CorrelationID)
		if err != nil {
			logger.Error(err, "Failed to check workflow started status (non-fatal, continuing assessment)",
				"correlationID", ea.Spec.CorrelationID)
		} else if !started {
			logger.Info("No workflow execution started for this correlation ID — completing as no_execution",
				"correlationID", ea.Spec.CorrelationID)
			return r.completeAssessmentWithReason(ctx, ea, startTime, eav1.AssessmentReasonNoExecution)
		}
	}

	// Step 7: Run component checks (skip already-completed)
	componentsChanged := false

	// Hash check — Two-Phase Model (BR-EM-004, DD-EM-002 v2.1)
	//
	// MUST run FIRST after stabilization window expires. The post-remediation
	// hash establishes the baseline for spec drift detection. All subsequent
	// observability checks (health, alert, metrics) are only trustworthy if
	// the resource is still in the state the workflow left it.
	//
	// Phase 1 (here): Capture current spec hash as PostRemediationSpecHash.
	// Compare pre vs post to determine if the workflow changed the spec.
	// Set CurrentSpecHash = PostRemediationSpecHash as initial baseline.
	//
	// Phase 2 (Step 6.5 above): On subsequent reconciles, re-capture current
	// hash and compare against PostRemediationSpecHash. If different, spec
	// drift detected — abort observability collection.
	if !ea.Status.Components.HashComputed {
		result := r.assessHash(ctx, ea)
		ea.Status.Components.HashComputed = result.Component.Assessed
		ea.Status.Components.PostRemediationSpecHash = result.Hash
		// Set CurrentSpecHash to match PostRemediationSpecHash on first capture.
		// This ensures CurrentSpecHash is always populated when the EA completes
		// (even on single-pass completions where Step 6.5 never runs).
		ea.Status.Components.CurrentSpecHash = result.Hash
		componentsChanged = true
		r.Metrics.RecordComponentAssessment("hash", resultStatus(result.Component), time.Since(startTime).Seconds(), nil)
		r.emitHashEvent(ctx, ea, result)

		// Log pre vs post comparison (informational — Phase 1 of two-phase model)
		if result.Component.Assessed && ea.Spec.PreRemediationSpecHash != "" && result.Hash != "" {
			if ea.Spec.PreRemediationSpecHash != result.Hash {
				logger.Info("Workflow modified target spec (pre != post)",
					"preHash", ea.Spec.PreRemediationSpecHash,
					"postHash", result.Hash[:min(23, len(result.Hash))]+"...",
				)
			} else {
				logger.Info("Workflow did not modify target spec (pre == post, operational workflow)",
					"hash", result.Hash[:min(23, len(result.Hash))]+"...",
				)
			}
		}

		// Set initial SpecIntegrity=True baseline (DD-CRD-002).
		if result.Component.Assessed {
			conditions.SetCondition(ea, conditions.ConditionSpecIntegrity,
				metav1.ConditionTrue, conditions.ReasonSpecUnchanged,
				"Post-remediation spec hash computed; baseline established")
		}
	}

	// Health check (BR-EM-001)
	if !ea.Status.Components.HealthAssessed {
		healthResult := r.assessHealth(ctx, ea)
		ea.Status.Components.HealthAssessed = healthResult.Component.Assessed
		ea.Status.Components.HealthScore = healthResult.Component.Score
		componentsChanged = true
		r.Metrics.RecordComponentAssessment("health", resultStatus(healthResult.Component), time.Since(startTime).Seconds(), healthResult.Component.Score)
		// Skip health audit event when score is nil (N/A for non-pod resources).
		// The component is marked Assessed=true so allComponentsDone sees it as complete,
		// but there is no meaningful health data to emit in the audit trail.
		if healthResult.Component.Score != nil {
			r.emitHealthEvent(ctx, ea, healthResult)
		}
	}

	// Alert check (BR-EM-002) - skip if disabled or client unavailable
	if !ea.Status.Components.AlertAssessed {
		if r.Config.AlertManagerEnabled && r.AlertManagerClient != nil {
			alertResult := r.assessAlert(ctx, ea)
			ea.Status.Components.AlertAssessed = alertResult.Component.Assessed
			ea.Status.Components.AlertScore = alertResult.Component.Score
			r.Metrics.RecordComponentAssessment("alert", resultStatus(alertResult.Component), time.Since(startTime).Seconds(), alertResult.Component.Score)
			r.emitAlertEvent(ctx, ea, alertResult)
		} else {
			ea.Status.Components.AlertAssessed = true
			r.Metrics.RecordComponentAssessment("alert", "skipped", time.Since(startTime).Seconds(), nil)
		}
		componentsChanged = true
	}

	// Metrics check (BR-EM-003) - skip if disabled or client unavailable
	if !ea.Status.Components.MetricsAssessed {
		if r.Config.PrometheusEnabled && r.PrometheusClient != nil {
			metricsResult := r.assessMetrics(ctx, ea)
			ea.Status.Components.MetricsAssessed = metricsResult.Component.Assessed
			ea.Status.Components.MetricsScore = metricsResult.Component.Score
			r.Metrics.RecordComponentAssessment("metrics", resultStatus(metricsResult.Component), time.Since(startTime).Seconds(), metricsResult.Component.Score)
			// Only emit the audit event when the metrics assessment succeeds.
			// If Prometheus returns no data, Assessed=false and we silently retry
			// on the next reconcile. Emitting on every retry would create duplicate
			// audit events. The completion event captures metrics_timed_out if needed.
			if metricsResult.Component.Assessed {
				r.emitMetricsEvent(ctx, ea, metricsResult)
			}
		} else {
			ea.Status.Components.MetricsAssessed = true
			r.Metrics.RecordComponentAssessment("metrics", "skipped", time.Since(startTime).Seconds(), nil)
		}
		componentsChanged = true
	}

	// Step 8: Check if all components are done and prepare completion fields in-memory.
	completing := false
	if r.allComponentsDone(ea) {
		completing = true
		reason := r.determineAssessmentReason(ea)
		r.setCompletionFields(ea, reason)
		logger.Info("All components done, preparing completion",
			"reason", reason, "componentsChanged", componentsChanged,
			"pendingTransition", pendingTransition,
		)
	}

	// Step 9: Single atomic status update (phase transition + timing + components + completion).
	// All fields were updated in-memory above; this is the only Status().Update() per reconcile.
	if componentsChanged || pendingTransition || completing {
		if err := r.Status().Update(ctx, ea); err != nil {
			logger.Error(err, "Failed to update EA status",
				"phase", ea.Status.Phase, "completing", completing,
				"resourceVersion", ea.ResourceVersion,
			)
			r.Metrics.RecordReconcile("error", time.Since(startTime).Seconds())
			return ctrl.Result{RequeueAfter: r.Config.RequeueGenericError}, err
		}

		// Post-update event emission (after successful persist)
		if pendingTransition {
			r.emitPendingTransitionEvents(ctx, ea)
		}
		if completing {
			r.emitCompletionMetricsAndEvents(ctx, ea, ea.Status.AssessmentReason)
			logger.Info("Assessment completed",
				"reason", ea.Status.AssessmentReason,
				"correlationID", ea.Spec.CorrelationID,
			)
			r.Metrics.RecordReconcile("success", time.Since(startTime).Seconds())
			return ctrl.Result{}, nil
		}
	}

	// Step 10: Requeue for remaining components
	r.Metrics.RecordReconcile("requeue", time.Since(startTime).Seconds())
	return ctrl.Result{RequeueAfter: r.Config.RequeueAssessmentInProgress}, nil
}

// SetupWithManager registers the controller with the manager.
// Creates a field index on spec.correlationID for O(1) lookups and kubectl
// field-selector support (e.g., kubectl get ea --field-selector spec.correlationID=rr-xxx).
func (r *Reconciler) SetupWithManager(mgr ctrl.Manager) error {
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

	return ctrl.NewControllerManagedBy(mgr).
		For(&eav1.EffectivenessAssessment{}).
		Complete(r)
}

// SetRESTMapper sets the REST mapper used to resolve Kind -> GVR for unstructured fetches.
// Called after NewReconciler and before SetupWithManager so the controller has access
// to the manager's discovery-backed mapper.
func (r *Reconciler) SetRESTMapper(rm meta.RESTMapper) {
	r.restMapper = rm
}

// ============================================================================
// COMPONENT ASSESSMENT METHODS
// ============================================================================

// healthAssessResult contains both the component result and the structured TargetStatus
// for populating the health_checks typed sub-object in audit events (DD-017 v2.5).
type healthAssessResult struct {
	Component emtypes.ComponentResult
	Status    health.TargetStatus
}

// assessHealth evaluates the target resource's health via K8s API (BR-EM-001).
func (r *Reconciler) assessHealth(ctx context.Context, ea *eav1.EffectivenessAssessment) healthAssessResult {
	logger := log.FromContext(ctx)

	// Build target status from K8s API
	status := r.getTargetHealthStatus(ctx, ea)

	result := r.healthScorer.Score(ctx, status)
	logger.Info("Health assessment complete",
		"score", result.Score,
		"details", result.Details,
	)

	return healthAssessResult{Component: result, Status: status}
}

// assessHash computes the spec hash of the target resource and compares
// with the pre-remediation hash (BR-EM-004, DD-EM-002).
func (r *Reconciler) assessHash(ctx context.Context, ea *eav1.EffectivenessAssessment) hash.ComputeResult {
	logger := log.FromContext(ctx)

	// Step 1: Fetch target spec from K8s API
	spec := r.getTargetSpec(ctx, ea)

	// Step 2: Read pre-remediation hash from EA spec (set by RO via RR status).
	// Falls back to DataStorage query for backward compatibility with EAs created
	// before the RO started populating PreRemediationSpecHash.
	preHash := ea.Spec.PreRemediationSpecHash
	if preHash == "" {
		logger.V(1).Info("PreRemediationSpecHash not in EA spec, falling back to DataStorage query")
		preHash = r.queryPreRemediationHash(ctx, ea.Spec.CorrelationID)
	}

	// Step 3: Compute post-hash and compare with pre-hash
	result := r.hashComputer.Compute(hash.SpecHashInput{
		Spec:    spec,
		PreHash: preHash,
	})

	if result.Hash != "" {
		logger.Info("Hash computation complete",
			"hash", result.Hash[:23]+"...",
			"preHash", preHash,
			"match", result.Match,
		)
	}
	return result
}

// assessAlert checks if the original alert has resolved (BR-EM-002).
// alertAssessResult contains both the component result and the structured alert data
// for populating the alert_resolution typed sub-object in audit events (DD-017 v2.5).
type alertAssessResult struct {
	Component             emtypes.ComponentResult
	AlertResolved         bool
	ActiveCount           int32
	ResolutionTimeSeconds *float64 // Seconds from remediation to alert resolution (nil if not resolved)
}

func (r *Reconciler) assessAlert(ctx context.Context, ea *eav1.EffectivenessAssessment) alertAssessResult {
	logger := log.FromContext(ctx)

	// OBS-1: Use SignalName (the actual alert name) when available,
	// falling back to CorrelationID for backward compatibility.
	alertName := ea.Spec.SignalName
	if alertName == "" {
		alertName = ea.Spec.CorrelationID
	}
	alertCtx := alert.AlertContext{
		AlertName: alertName,
		Namespace: ea.Spec.TargetResource.Namespace,
	}

	result := r.alertScorer.Score(ctx, r.AlertManagerClient, alertCtx)
	logger.Info("Alert assessment complete",
		"score", result.Score,
		"details", result.Details,
	)

	// Derive structured alert data from the score (DD-017 v2.5):
	// 1.0 = resolved, 0.0 = still active
	alertResolved := result.Score != nil && *result.Score == 1.0

	// ActiveCount: the exact count is internal to the alert scorer. We derive a
	// minimum bound from the score: 0 if resolved, 1 if still active. The scorer
	// embeds the actual count in result.Details for debugging but does not expose
	// it as a structured field. Precise count is a V1.1 enhancement.
	var activeCount int32
	if !alertResolved && result.Score != nil {
		activeCount = 1
	}

	// ADR-EM-001 Section 9.2.3: resolution_time_seconds = time from remediation to alert resolution.
	// Only computed when the alert is resolved and RemediationCreatedAt is available.
	var resolutionTime *float64
	if alertResolved && ea.Spec.RemediationCreatedAt != nil {
		rt := time.Since(ea.Spec.RemediationCreatedAt.Time).Seconds()
		resolutionTime = &rt
	}

	return alertAssessResult{
		Component:             result,
		AlertResolved:         alertResolved,
		ActiveCount:           activeCount,
		ResolutionTimeSeconds: resolutionTime,
	}
}

// metricsAssessResult contains both the component result and the structured metric deltas
// for populating the metric_deltas typed sub-object in audit events (DD-017 v2.5).
type metricsAssessResult struct {
	Component            emtypes.ComponentResult
	CPUBefore            *float64
	CPUAfter             *float64
	MemoryBefore         *float64
	MemoryAfter          *float64
	LatencyP95BeforeMs   *float64
	LatencyP95AfterMs    *float64
	ErrorRateBefore      *float64
	ErrorRateAfter       *float64
	ThroughputBeforeRPS  *float64
	ThroughputAfterRPS   *float64
}

// metricQuerySpec defines a PromQL query for a single metric type.
type metricQuerySpec struct {
	// Name identifies the metric (used in MetricComparison and logging).
	Name string
	// Query is the PromQL expression to execute.
	Query string
	// LowerIsBetter indicates whether a decrease represents improvement.
	LowerIsBetter bool
}

// metricQueryResult contains the before/after values from a single metric query.
type metricQueryResult struct {
	Spec       metricQuerySpec
	PreValue   float64
	PostValue  float64
	Available  bool
	QueryError error
}

// assessMetrics compares pre/post remediation metrics (BR-EM-003, DD-017 v2.5 Phase B).
//
// Executes up to 5 independent PromQL queries (CPU, memory, latency p95, error rate, throughput).
// Each query is independent — individual query failures don't prevent overall assessment
// (graceful degradation). The score is the average of all available metric improvements.
//
// Scoring:
//   - 0.0: No improvement or degradation
//   - >0.0 to 1.0: Improvement detected
//   - nil: Not assessed (no data available for any metric)
func (r *Reconciler) assessMetrics(ctx context.Context, ea *eav1.EffectivenessAssessment) metricsAssessResult {
	logger := log.FromContext(ctx)
	ns := ea.Spec.TargetResource.Namespace

	// Range: from before EA creation to now, capturing pre- and post-remediation samples.
	start := ea.CreationTimestamp.Add(-r.Config.PrometheusLookback)
	end := time.Now()
	step := 1 * time.Second

	// Define all 4 metric queries (DD-017 v2.5 Phase B)
	// CPU and memory use sum() to aggregate across all containers/pods in the namespace
	// into a single time series. Without sum(), Prometheus returns multiple series
	// (one per container label combination) and Samples[0]/Samples[len-1] may come
	// from different series, causing non-deterministic pre/post comparisons.
	queries := []metricQuerySpec{
		{
			Name:          "container_cpu_usage_seconds_total",
			Query:         fmt.Sprintf(`sum(container_cpu_usage_seconds_total{namespace="%s"})`, ns),
			LowerIsBetter: true,
		},
		{
			Name:          "container_memory_working_set_bytes",
			Query:         fmt.Sprintf(`sum(container_memory_working_set_bytes{namespace="%s"})`, ns),
			LowerIsBetter: true,
		},
		{
			Name:          "http_request_duration_p95_ms",
			Query:         fmt.Sprintf(`histogram_quantile(0.95, rate(http_request_duration_seconds_bucket{namespace="%s"}[5m])) * 1000`, ns),
			LowerIsBetter: true,
		},
		{
			Name:          "http_error_rate",
			Query:         fmt.Sprintf(`sum(rate(http_requests_total{namespace="%s",code=~"5.."}[5m])) / sum(rate(http_requests_total{namespace="%s"}[5m]))`, ns, ns),
			LowerIsBetter: true,
		},
		{
			Name:          "http_throughput_rps",
			Query:         fmt.Sprintf(`sum(rate(http_requests_total{namespace="%s"}[5m]))`, ns),
			LowerIsBetter: false, // Higher throughput is better
		},
	}

	// Execute each query independently (graceful degradation per DD-017 v2.5)
	queryResults := make([]metricQueryResult, len(queries))
	for i, spec := range queries {
		queryResults[i] = r.executeMetricQuery(ctx, spec, start, end, step)
	}

	// Build comparisons from successful queries only
	var comparisons []emmetrics.MetricComparison
	for _, qr := range queryResults {
		if qr.Available {
			comparisons = append(comparisons, emmetrics.MetricComparison{
				Name:          qr.Spec.Name,
				PreValue:      qr.PreValue,
				PostValue:     qr.PostValue,
				LowerIsBetter: qr.Spec.LowerIsBetter,
			})
		}
	}

	// Score available comparisons
	result := emtypes.ComponentResult{
		Component: emtypes.ComponentMetrics,
	}
	if len(comparisons) == 0 {
		result.Assessed = false
		result.Details = "no metric data available for comparison"
		return metricsAssessResult{Component: result}
	}

	scored := r.metricScorer.Score(comparisons)
	result = scored.Component

	logger.Info("Metrics assessment complete",
		"score", result.Score,
		"queriesAvailable", len(comparisons),
		"queriesTotal", len(queries),
	)

	// Populate metricsAssessResult from query results
	mr := metricsAssessResult{Component: result}
	for _, qr := range queryResults {
		if !qr.Available {
			continue
		}
		switch qr.Spec.Name {
		case "container_cpu_usage_seconds_total":
			mr.CPUBefore = &qr.PreValue
			mr.CPUAfter = &qr.PostValue
		case "container_memory_working_set_bytes":
			mr.MemoryBefore = &qr.PreValue
			mr.MemoryAfter = &qr.PostValue
		case "http_request_duration_p95_ms":
			mr.LatencyP95BeforeMs = &qr.PreValue
			mr.LatencyP95AfterMs = &qr.PostValue
		case "http_error_rate":
			mr.ErrorRateBefore = &qr.PreValue
			mr.ErrorRateAfter = &qr.PostValue
		case "http_throughput_rps":
			mr.ThroughputBeforeRPS = &qr.PreValue
			mr.ThroughputAfterRPS = &qr.PostValue
		}
	}

	return mr
}

// executeMetricQuery runs a single PromQL range query and extracts before/after values.
// Returns Available=false if the query fails or returns insufficient data (graceful degradation).
func (r *Reconciler) executeMetricQuery(ctx context.Context, spec metricQuerySpec, start, end time.Time, step time.Duration) metricQueryResult {
	logger := log.FromContext(ctx)
	result := metricQueryResult{Spec: spec}

	queryResult, err := r.PrometheusClient.QueryRange(ctx, spec.Query, start, end, step)
	if err != nil {
		logger.V(1).Info("Prometheus query failed (graceful degradation)",
			"metric", spec.Name, "error", err)
		result.QueryError = err
		r.Metrics.RecordExternalCallError("prometheus", "query_range", "query_error")
		return result
	}

	if len(queryResult.Samples) < 2 {
		logger.V(1).Info("Insufficient samples for metric comparison",
			"metric", spec.Name, "samples", len(queryResult.Samples))
		return result
	}

	// Earliest = pre-remediation, latest = post-remediation
	result.PreValue = queryResult.Samples[0].Value
	result.PostValue = queryResult.Samples[len(queryResult.Samples)-1].Value
	result.Available = true

	return result
}

// ============================================================================
// HELPER METHODS
// ============================================================================

// getTargetHealthStatus queries the K8s API for the target resource health.
// Kind-aware: uses label-based listing for workload resources (Deployment,
// ReplicaSet, StatefulSet, DaemonSet) and direct pod lookup for Pod targets.
// Non-pod-owning resources (ConfigMap, Secret, Node, etc.) are checked for
// existence only — they have no pod health to assess.
func (r *Reconciler) getTargetHealthStatus(ctx context.Context, ea *eav1.EffectivenessAssessment) health.TargetStatus {
	logger := log.FromContext(ctx)

	targetKind := ea.Spec.TargetResource.Kind
	targetName := ea.Spec.TargetResource.Name
	targetNs := ea.Spec.TargetResource.Namespace

	var podList *corev1.PodList

	switch targetKind {
	case "Deployment", "ReplicaSet", "StatefulSet", "DaemonSet":
		// Workload resources: list pods by app label (standard convention)
		podList = &corev1.PodList{}
		err := r.List(ctx, podList,
			client.InNamespace(targetNs),
			client.MatchingLabels{"app": targetName},
		)
		if err != nil {
			logger.Error(err, "Failed to list pods for target resource",
				"kind", targetKind, "name", targetName)
			return health.TargetStatus{TargetExists: false}
		}

	case "Pod":
		// Direct pod target: fetch the specific pod by name
		pod := &corev1.Pod{}
		err := r.Get(ctx, client.ObjectKey{Name: targetName, Namespace: targetNs}, pod)
		if err != nil {
			logger.V(1).Info("Target pod not found", "name", targetName, "error", err)
			return health.TargetStatus{TargetExists: false}
		}
		podList = &corev1.PodList{Items: []corev1.Pod{*pod}}

	default:
		// Non-pod-owning resources (ConfigMap, Secret, Node, etc.)
		// Health scoring is not applicable — signal N/A to the scorer.
		logger.V(1).Info("Target resource kind has no pod health to assess",
			"kind", targetKind, "name", targetName)
		return health.TargetStatus{
			TargetExists:        true,
			HealthNotApplicable: true,
		}
	}

	if len(podList.Items) == 0 {
		return health.TargetStatus{TargetExists: false}
	}

	// Count ready pods, restarts, and detect CrashLoopBackOff/OOMKilled/Pending (DD-017 v2.5)
	totalReplicas := int32(len(podList.Items))
	readyReplicas := int32(0)
	totalRestarts := int32(0)
	crashLoops := false
	oomKilled := false
	pendingCount := int32(0)

	for i := range podList.Items {
		pod := &podList.Items[i]

		// Count pods still in Pending phase after stabilization (DD-017 v2.5)
		if pod.Status.Phase == corev1.PodPending {
			pendingCount++
		}

		for _, cs := range pod.Status.ContainerStatuses {
			totalRestarts += cs.RestartCount
			if cs.Ready {
				readyReplicas++
				break // Count pod as ready if any container is ready
			}
			// Detect CrashLoopBackOff from waiting state (DD-017 v2.5)
			if cs.State.Waiting != nil && cs.State.Waiting.Reason == "CrashLoopBackOff" {
				crashLoops = true
			}
			// Detect OOMKilled from last termination state (DD-017 v2.5)
			if cs.LastTerminationState.Terminated != nil && cs.LastTerminationState.Terminated.Reason == "OOMKilled" {
				oomKilled = true
			}
		}
	}

	return health.TargetStatus{
		TotalReplicas:           totalReplicas,
		ReadyReplicas:           readyReplicas,
		RestartsSinceRemediation: totalRestarts,
		TargetExists:           true,
		CrashLoops:             crashLoops,
		OOMKilled:              oomKilled,
		PendingCount:           pendingCount,
	}
}

// getTargetSpec retrieves the target resource's .spec as an unstructured map from the K8s API.
// Uses the REST mapper to resolve the Kind to a GVR, then fetches via unstructured client.
// Returns an empty map on error (graceful degradation — hash is still computed, just from empty spec).
func (r *Reconciler) getTargetSpec(ctx context.Context, ea *eav1.EffectivenessAssessment) map[string]interface{} {
	logger := log.FromContext(ctx)

	if r.restMapper == nil {
		logger.V(1).Info("RESTMapper not configured, falling back to EA metadata spec")
		return map[string]interface{}{
			"kind":      ea.Spec.TargetResource.Kind,
			"name":      ea.Spec.TargetResource.Name,
			"namespace": ea.Spec.TargetResource.Namespace,
		}
	}

	// Resolve Kind -> GVR
	gvk, err := resolveGVKForKind(r.restMapper, ea.Spec.TargetResource.Kind)
	if err != nil {
		logger.Error(err, "Failed to resolve GVK for target resource kind",
			"kind", ea.Spec.TargetResource.Kind)
		return map[string]interface{}{}
	}

	// Fetch target resource via unstructured client
	obj := &unstructured.Unstructured{}
	obj.SetGroupVersionKind(gvk)
	key := client.ObjectKey{
		Namespace: ea.Spec.TargetResource.Namespace,
		Name:      ea.Spec.TargetResource.Name,
	}
	if err := r.Get(ctx, key, obj); err != nil {
		if apierrors.IsNotFound(err) {
			logger.Info("Target resource not found, computing hash from empty spec",
				"kind", ea.Spec.TargetResource.Kind,
				"name", ea.Spec.TargetResource.Name)
		} else {
			logger.Error(err, "Failed to fetch target resource")
		}
		return map[string]interface{}{}
	}

	// Extract .spec field
	spec, found, err := unstructured.NestedMap(obj.Object, "spec")
	if err != nil || !found {
		logger.V(1).Info("Target resource has no .spec field",
			"kind", ea.Spec.TargetResource.Kind,
			"name", ea.Spec.TargetResource.Name)
		return map[string]interface{}{}
	}

	logger.V(2).Info("Target spec retrieved",
		"kind", ea.Spec.TargetResource.Kind,
		"name", ea.Spec.TargetResource.Name)
	return spec
}

// queryPreRemediationHash queries DataStorage for the pre-remediation spec hash
// from the RO's remediation.workflow_created audit event.
// Returns empty string if DS is unavailable or no pre-hash exists (graceful degradation).
func (r *Reconciler) queryPreRemediationHash(ctx context.Context, correlationID string) string {
	if r.DSQuerier == nil {
		log.FromContext(ctx).V(1).Info("DSQuerier not configured, skipping pre-remediation hash lookup")
		return ""
	}

	preHash, err := r.DSQuerier.QueryPreRemediationHash(ctx, correlationID)
	if err != nil {
		log.FromContext(ctx).Error(err, "Failed to query pre-remediation hash from DataStorage",
			"correlationID", correlationID)
		r.Metrics.RecordExternalCallError("datastorage", "query_pre_hash", "query_error")
		return ""
	}

	if preHash != "" {
		log.FromContext(ctx).V(1).Info("Pre-remediation hash retrieved from DataStorage",
			"correlationID", correlationID,
			"preHash", preHash[:min(23, len(preHash))]+"...")
	}
	return preHash
}

// resolveGVKForKind resolves a Kind string to a schema.GroupVersionKind using the REST mapper.
// This is needed because the EA spec only stores the Kind, not the full GVR.
func resolveGVKForKind(rm meta.RESTMapper, kind string) (schema.GroupVersionKind, error) {
	// First, try to find resources matching this kind
	// The REST mapper can resolve Kind -> GVR through the API server's discovery
	gvks, err := rm.KindsFor(schema.GroupVersionResource{Resource: kind})
	if err == nil && len(gvks) > 0 {
		return gvks[0], nil
	}

	// Fallback: try common core kinds
	coreKinds := map[string]schema.GroupVersionKind{
		"Deployment":  {Group: "apps", Version: "v1", Kind: "Deployment"},
		"StatefulSet": {Group: "apps", Version: "v1", Kind: "StatefulSet"},
		"DaemonSet":   {Group: "apps", Version: "v1", Kind: "DaemonSet"},
		"Pod":         {Group: "", Version: "v1", Kind: "Pod"},
		"Service":     {Group: "", Version: "v1", Kind: "Service"},
		"ConfigMap":   {Group: "", Version: "v1", Kind: "ConfigMap"},
		"Secret":      {Group: "", Version: "v1", Kind: "Secret"},
	}
	if gvk, ok := coreKinds[kind]; ok {
		return gvk, nil
	}

	return schema.GroupVersionKind{}, fmt.Errorf("cannot resolve GVK for kind %q: %w", kind, err)
}

// emitHashEvent emits K8s and audit events for the hash computation result.
// Uses the specialized RecordHashComputed to include pre/post hashes and match flag.
func (r *Reconciler) emitHashEvent(ctx context.Context, ea *eav1.EffectivenessAssessment, result hash.ComputeResult) {
	// K8s Event (DD-EVENT-001)
	if result.Component.Error != nil {
		r.Recorder.Event(ea, corev1.EventTypeWarning, events.EventReasonComponentAssessed,
			fmt.Sprintf("Component hash assessment failed: %v", result.Component.Error))
		r.Metrics.RecordK8sEvent("Warning", events.EventReasonComponentAssessed)
	} else {
		r.Recorder.Event(ea, corev1.EventTypeNormal, events.EventReasonComponentAssessed,
			fmt.Sprintf("Component hash computed (match: %v)", result.Match))
		r.Metrics.RecordK8sEvent("Normal", events.EventReasonComponentAssessed)
	}

	// Audit Event to DataStorage (DD-AUDIT-003, DD-EM-002)
	if r.AuditManager != nil {
		hashData := emaudit.HashComputedData{
			PostHash: result.Hash,
			PreHash:  result.PreHash,
			Match:    result.Match,
		}
		if err := r.AuditManager.RecordHashComputed(ctx, ea, result.Component, hashData); err != nil {
			log.FromContext(ctx).V(1).Info("Failed to store hash computed audit event",
				"error", err)
		}
		r.Metrics.RecordAuditEvent(string(emtypes.AuditHashComputed), resultStatus(result.Component))
	}
}

// completeAssessment finalizes the EA with Completed phase and assessment reason.
// It performs a single atomic status update and emits completion events.
// Used by both the normal completion path (Step 8-9) and the expired path (Step 4).
func (r *Reconciler) completeAssessment(ctx context.Context, ea *eav1.EffectivenessAssessment, startTime time.Time) (ctrl.Result, error) {
	logger := log.FromContext(ctx)

	reason := r.determineAssessmentReason(ea)
	r.setCompletionFields(ea, reason)

	if err := r.Status().Update(ctx, ea); err != nil {
		logger.Error(err, "Failed to update EA to Completed",
			"reason", reason, "resourceVersion", ea.ResourceVersion)
		r.Metrics.RecordReconcile("error", time.Since(startTime).Seconds())
		return ctrl.Result{RequeueAfter: r.Config.RequeueGenericError}, err
	}

	r.emitCompletionMetricsAndEvents(ctx, ea, reason)

	logger.Info("Assessment completed",
		"reason", reason,
		"correlationID", ea.Spec.CorrelationID,
	)

	r.Metrics.RecordReconcile("success", time.Since(startTime).Seconds())
	return ctrl.Result{}, nil
}

// completeAssessmentWithReason finalizes the EA with an explicit assessment reason.
// Unlike completeAssessment, which computes the reason from component state, this
// method uses the provided reason directly. Used by the spec drift guard (DD-EM-002 v1.1).
func (r *Reconciler) completeAssessmentWithReason(ctx context.Context, ea *eav1.EffectivenessAssessment, startTime time.Time, reason string) (ctrl.Result, error) {
	logger := log.FromContext(ctx)

	r.setCompletionFields(ea, reason)

	if err := r.Status().Update(ctx, ea); err != nil {
		logger.Error(err, "Failed to update EA to Completed",
			"reason", reason, "resourceVersion", ea.ResourceVersion)
		r.Metrics.RecordReconcile("error", time.Since(startTime).Seconds())
		return ctrl.Result{RequeueAfter: r.Config.RequeueGenericError}, err
	}

	r.emitCompletionMetricsAndEvents(ctx, ea, reason)

	logger.Info("Assessment completed",
		"reason", reason,
		"correlationID", ea.Spec.CorrelationID,
	)

	r.Metrics.RecordReconcile("success", time.Since(startTime).Seconds())
	return ctrl.Result{}, nil
}

// setCompletionFields sets the in-memory status fields for assessment completion.
// Extracted to share between the normal completion path and the expired path.
// Also sets the AssessmentComplete condition per DD-CRD-002.
func (r *Reconciler) setCompletionFields(ea *eav1.EffectivenessAssessment, reason string) {
	now := metav1.Now()
	ea.Status.Phase = eav1.PhaseCompleted
	ea.Status.CompletedAt = &now
	ea.Status.AssessmentReason = reason
	ea.Status.Message = fmt.Sprintf("Assessment completed: %s", reason)

	// Issue #79 Phase 7b: Set Ready condition on terminal transitions
	conditions.SetReady(ea, true, conditions.ReasonReady, "Assessment completed")

	// Set AssessmentComplete condition (DD-CRD-002) for all completion paths.
	// The condition reason maps from AssessmentReason to the DD-CRD-002 reason constant.
	condReason := mapAssessmentReasonToConditionReason(reason)
	conditions.SetCondition(ea, conditions.ConditionAssessmentComplete,
		metav1.ConditionTrue, condReason,
		fmt.Sprintf("Assessment completed: %s", reason))
}

// mapAssessmentReasonToConditionReason maps an AssessmentReason value to the
// corresponding DD-CRD-002 condition reason constant.
func mapAssessmentReasonToConditionReason(reason string) string {
	switch reason {
	case eav1.AssessmentReasonFull:
		return conditions.ReasonAssessmentFull
	case eav1.AssessmentReasonPartial:
		return conditions.ReasonAssessmentPartial
	case eav1.AssessmentReasonExpired:
		return conditions.ReasonAssessmentExpired
	case eav1.AssessmentReasonSpecDrift:
		return conditions.ReasonSpecDrift
	case eav1.AssessmentReasonMetricsTimedOut:
		return conditions.ReasonMetricsTimedOut
	case eav1.AssessmentReasonNoExecution:
		return conditions.ReasonNoExecution
	default:
		return reason // Fallback: use the reason string directly
	}
}

// emitCompletionMetricsAndEvents records metrics and emits K8s + audit events for completion.
// Extracted to share between the normal completion path and the expired path.
func (r *Reconciler) emitCompletionMetricsAndEvents(ctx context.Context, ea *eav1.EffectivenessAssessment, reason string) {
	r.Metrics.RecordPhaseTransition(eav1.PhaseAssessing, eav1.PhaseCompleted)
	r.Metrics.RecordAssessmentCompleted(reason)
	r.emitCompletionEvent(ea, reason)
	r.emitCompletedAuditEvent(ctx, ea, reason)
}

// determineAssessmentReason computes the reason based on component assessment state.
//
// Reason hierarchy (highest priority first):
//   - full: All enabled components assessed
//   - metrics_timed_out: Core checks done (health+hash), Prometheus enabled but metrics
//     not assessed, validity expired (distinct from generic partial)
//   - partial: Some components assessed, some not (generic)
//   - expired: Validity expired with no data collected
func (r *Reconciler) determineAssessmentReason(ea *eav1.EffectivenessAssessment) string {
	components := &ea.Status.Components

	allAssessed := components.HealthAssessed && components.HashComputed &&
		(components.AlertAssessed || !r.Config.AlertManagerEnabled) &&
		(components.MetricsAssessed || !r.Config.PrometheusEnabled)

	anyAssessed := components.HealthAssessed || components.HashComputed ||
		components.AlertAssessed || components.MetricsAssessed

	if allAssessed {
		return eav1.AssessmentReasonFull
	}

	// Check if validity expired (ValidityDeadline is always set by first reconciliation)
	validityExpired := ea.Status.ValidityDeadline != nil &&
		r.validityChecker.TimeUntilExpired(ea.Status.ValidityDeadline.Time) == 0

	if validityExpired {
		// ADR-EM-001, Batch 3: Distinguish metrics_timed_out from generic partial.
		// metrics_timed_out: ALL non-temporal checks completed (health + hash + alerts),
		// Prometheus is enabled but metrics were NOT assessed before validity expired.
		// This gives HAPI/DS a precise signal that the assessment was complete
		// except for metrics collection which requires temporal data.
		// If alerts are also missing, the correct reason is "partial" (multiple gaps).
		if r.Config.PrometheusEnabled && !components.MetricsAssessed &&
			components.HealthAssessed && components.HashComputed &&
			(components.AlertAssessed || !r.Config.AlertManagerEnabled) {
			return eav1.AssessmentReasonMetricsTimedOut
		}

		if anyAssessed {
			return eav1.AssessmentReasonPartial
		}
		return eav1.AssessmentReasonExpired
	}

	if anyAssessed {
		return eav1.AssessmentReasonPartial
	}

	return eav1.AssessmentReasonExpired
}

// allComponentsDone checks if all enabled components have been assessed.
func (r *Reconciler) allComponentsDone(ea *eav1.EffectivenessAssessment) bool {
	if !ea.Status.Components.HealthAssessed {
		return false
	}
	if !ea.Status.Components.HashComputed {
		return false
	}
	if r.Config.AlertManagerEnabled && !ea.Status.Components.AlertAssessed {
		return false
	}
	if r.Config.PrometheusEnabled && !ea.Status.Components.MetricsAssessed {
		return false
	}
	return true
}

// emitCompletionEvent emits a K8s event when the assessment completes.
// The EM emits raw component scores via audit events to DataStorage.
// The overall effectiveness determination is computed by DataStorage on demand,
// not by the EM (separation of concerns).
func (r *Reconciler) emitCompletionEvent(ea *eav1.EffectivenessAssessment, reason string) {
	r.Recorder.Event(ea, corev1.EventTypeNormal, events.EventReasonEffectivenessAssessed,
		fmt.Sprintf("Assessment completed: %s (correlation: %s)", reason, ea.Spec.CorrelationID))
	r.Metrics.RecordK8sEvent("Normal", events.EventReasonEffectivenessAssessed)
}

// emitK8sComponentEvent emits a K8s event for a component assessment result.
// K8s event uses DD-EVENT-001 simplified approach: single EventReasonComponentAssessed with component name in message.
func (r *Reconciler) emitK8sComponentEvent(ea *eav1.EffectivenessAssessment, component string, result emtypes.ComponentResult) {
	if result.Error != nil {
		r.Recorder.Event(ea, corev1.EventTypeWarning, events.EventReasonComponentAssessed,
			fmt.Sprintf("Component %s assessment failed: %v", component, result.Error))
		r.Metrics.RecordK8sEvent("Warning", events.EventReasonComponentAssessed)
	} else {
		msg := fmt.Sprintf("Component %s assessed", component)
		if result.Score != nil {
			msg = fmt.Sprintf("Component %s assessed (score: %.2f)", component, *result.Score)
		}
		r.Recorder.Event(ea, corev1.EventTypeNormal, events.EventReasonComponentAssessed, msg)
		r.Metrics.RecordK8sEvent("Normal", events.EventReasonComponentAssessed)
	}
}

// emitHealthEvent emits K8s event and health_checks typed audit event (DD-017 v2.5).
func (r *Reconciler) emitHealthEvent(ctx context.Context, ea *eav1.EffectivenessAssessment, hr healthAssessResult) {
	r.emitK8sComponentEvent(ea, "health", hr.Component)

	r.emitAuditEvent(ctx, emtypes.AuditHealthAssessed, func() error {
		return r.AuditManager.RecordHealthAssessed(ctx, ea, hr.Component, emaudit.HealthAssessedData{
			TotalReplicas:           hr.Status.TotalReplicas,
			ReadyReplicas:           hr.Status.ReadyReplicas,
			RestartsSinceRemediation: hr.Status.RestartsSinceRemediation,
			CrashLoops:             hr.Status.CrashLoops,
			OOMKilled:              hr.Status.OOMKilled,
			PendingCount:           hr.Status.PendingCount,
		})
	})
}

// emitAlertEvent emits K8s event and alert_resolution typed audit event (DD-017 v2.5).
func (r *Reconciler) emitAlertEvent(ctx context.Context, ea *eav1.EffectivenessAssessment, ar alertAssessResult) {
	r.emitK8sComponentEvent(ea, "alert", ar.Component)

	r.emitAuditEvent(ctx, emtypes.AuditAlertAssessed, func() error {
		return r.AuditManager.RecordAlertAssessed(ctx, ea, ar.Component, emaudit.AlertAssessedData{
			AlertResolved:         ar.AlertResolved,
			ActiveCount:           ar.ActiveCount,
			ResolutionTimeSeconds: ar.ResolutionTimeSeconds,
		})
	})
}

// emitMetricsEvent emits K8s event and metric_deltas typed audit event (DD-017 v2.5).
func (r *Reconciler) emitMetricsEvent(ctx context.Context, ea *eav1.EffectivenessAssessment, mr metricsAssessResult) {
	r.emitK8sComponentEvent(ea, "metrics", mr.Component)

	r.emitAuditEvent(ctx, emtypes.AuditMetricsAssessed, func() error {
		return r.AuditManager.RecordMetricsAssessed(ctx, ea, mr.Component, emaudit.MetricsAssessedData{
			CPUBefore:           mr.CPUBefore,
			CPUAfter:            mr.CPUAfter,
			MemoryBefore:        mr.MemoryBefore,
			MemoryAfter:         mr.MemoryAfter,
			LatencyP95BeforeMs:  mr.LatencyP95BeforeMs,
			LatencyP95AfterMs:   mr.LatencyP95AfterMs,
			ErrorRateBefore:     mr.ErrorRateBefore,
			ErrorRateAfter:      mr.ErrorRateAfter,
			ThroughputBeforeRPS: mr.ThroughputBeforeRPS,
			ThroughputAfterRPS:  mr.ThroughputAfterRPS,
		})
	})
}

// emitPendingTransitionEvents emits K8s events, audit events, and metrics for the
// Pending -> Assessing phase transition. Called after the status update succeeds.
func (r *Reconciler) emitPendingTransitionEvents(ctx context.Context, ea *eav1.EffectivenessAssessment) {
	r.Metrics.RecordPhaseTransition(eav1.PhasePending, eav1.PhaseAssessing)

	// Emit assessment_scheduled audit event (BR-EM-009.4)
	r.emitAuditEvent(ctx, emtypes.AuditAssessmentScheduled, func() error {
		return r.AuditManager.RecordAssessmentScheduled(ctx, ea, r.Config.ValidityWindow)
	})

	r.Recorder.Event(ea, corev1.EventTypeNormal, events.EventReasonAssessmentStarted,
		fmt.Sprintf("Assessment started for correlation %s", ea.Spec.CorrelationID))
	r.Metrics.RecordK8sEvent("Normal", events.EventReasonAssessmentStarted)
}

// emitCompletedAuditEvent emits the assessment.completed audit event to DataStorage.
func (r *Reconciler) emitCompletedAuditEvent(ctx context.Context, ea *eav1.EffectivenessAssessment, reason string) {
	r.emitAuditEvent(ctx, emtypes.AuditAssessmentCompleted, func() error {
		return r.AuditManager.RecordAssessmentCompleted(ctx, ea, reason)
	})
}

// emitAuditEvent is a helper that handles the nil-check, error logging, and metrics
// recording common to all audit event emissions. The recordFn performs the actual audit call.
func (r *Reconciler) emitAuditEvent(ctx context.Context, eventType emtypes.AuditEventType, recordFn func() error) {
	if r.AuditManager == nil {
		log.FromContext(ctx).V(1).Info("AuditManager not configured, skipping audit event",
			"eventType", string(eventType))
		return
	}

	if err := recordFn(); err != nil {
		log.FromContext(ctx).Error(err, "Failed to emit audit event", "eventType", string(eventType))
		r.Metrics.RecordAuditEvent(string(eventType), "error")
		return
	}
	r.Metrics.RecordAuditEvent(string(eventType), "success")
}

// resultStatus returns a metric label for a component result.
func resultStatus(result emtypes.ComponentResult) string {
	if result.Error != nil {
		return "error"
	}
	if result.Assessed {
		return "success"
	}
	return "skipped"
}
