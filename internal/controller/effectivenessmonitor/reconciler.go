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
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/tools/record"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
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
	emtypes "github.com/jordigilh/kubernaut/pkg/effectivenessmonitor/types"
	"github.com/jordigilh/kubernaut/pkg/effectivenessmonitor/validity"
	"github.com/jordigilh/kubernaut/pkg/shared/events"
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

	// Internal components (created in constructor)
	healthScorer    health.Scorer
	alertScorer     alert.Scorer
	metricScorer    emmetrics.Scorer
	hashComputer    hash.Computer
	validityChecker validity.Checker

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
	// Default: 30m (from EMConfig.Assessment.ValidityWindow).
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
			ea.CreationTimestamp.Time,
			ea.Spec.Config.StabilizationWindow.Duration,
			ea.Status.ValidityDeadline.Time,
		)
	} else {
		// No deadline computed yet - check stabilization only
		stabilizationEnd := ea.CreationTimestamp.Time.Add(ea.Spec.Config.StabilizationWindow.Duration)
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
			ea.CreationTimestamp.Time,
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
		deadline := metav1.NewTime(ea.CreationTimestamp.Time.Add(r.Config.ValidityWindow))
		checkAfter := metav1.NewTime(ea.CreationTimestamp.Time.Add(ea.Spec.Config.StabilizationWindow.Duration))

		ea.Status.Phase = eav1.PhaseAssessing
		ea.Status.ValidityDeadline = &deadline
		ea.Status.PrometheusCheckAfter = &checkAfter
		ea.Status.AlertManagerCheckAfter = &checkAfter

		logger.Info("Computed derived timing (BR-EM-009)",
			"creationTimestamp", ea.CreationTimestamp.Time,
			"validityWindow", r.Config.ValidityWindow,
			"stabilizationWindow", ea.Spec.Config.StabilizationWindow.Duration,
			"validityDeadline", deadline.Time,
			"prometheusCheckAfter", checkAfter.Time,
			"alertManagerCheckAfter", checkAfter.Time,
		)

		pendingTransition = true
	}

	// Step 7: Run component checks (skip already-completed)
	componentsChanged := false

	// Health check (BR-EM-001)
	if !ea.Status.Components.HealthAssessed {
		result := r.assessHealth(ctx, ea)
		ea.Status.Components.HealthAssessed = result.Assessed
		ea.Status.Components.HealthScore = result.Score
		componentsChanged = true
		r.Metrics.RecordComponentAssessment("health", resultStatus(result), time.Since(startTime).Seconds(), result.Score)
		r.emitComponentEvent(ctx, ea, "health", result)
	}

	// Hash check (BR-EM-004)
	if !ea.Status.Components.HashComputed {
		result := r.assessHash(ctx, ea)
		ea.Status.Components.HashComputed = result.Component.Assessed
		ea.Status.Components.PostRemediationSpecHash = result.Hash
		componentsChanged = true
		r.Metrics.RecordComponentAssessment("hash", resultStatus(result.Component), time.Since(startTime).Seconds(), nil)
		r.emitComponentEvent(ctx, ea, "hash", result.Component)
	}

	// Alert check (BR-EM-002) - skip if disabled
	if !ea.Status.Components.AlertAssessed && ea.Spec.Config.AlertManagerEnabled {
		if r.AlertManagerClient != nil {
			result := r.assessAlert(ctx, ea)
			ea.Status.Components.AlertAssessed = result.Assessed
			ea.Status.Components.AlertScore = result.Score
			componentsChanged = true
			r.Metrics.RecordComponentAssessment("alert", resultStatus(result), time.Since(startTime).Seconds(), result.Score)
			r.emitComponentEvent(ctx, ea, "alert", result)
		} else {
			// Client not available, mark as assessed with nil score
			ea.Status.Components.AlertAssessed = true
			componentsChanged = true
			r.Metrics.RecordComponentAssessment("alert", "skipped", time.Since(startTime).Seconds(), nil)
		}
	} else if !ea.Status.Components.AlertAssessed && !ea.Spec.Config.AlertManagerEnabled {
		// AlertManager disabled in config, skip
		ea.Status.Components.AlertAssessed = true
		componentsChanged = true
		r.Metrics.RecordComponentAssessment("alert", "skipped", time.Since(startTime).Seconds(), nil)
	}

	// Metrics check (BR-EM-003) - skip if disabled
	if !ea.Status.Components.MetricsAssessed && ea.Spec.Config.PrometheusEnabled {
		if r.PrometheusClient != nil {
			result := r.assessMetrics(ctx, ea)
			ea.Status.Components.MetricsAssessed = result.Assessed
			ea.Status.Components.MetricsScore = result.Score
			componentsChanged = true
			r.Metrics.RecordComponentAssessment("metrics", resultStatus(result), time.Since(startTime).Seconds(), result.Score)
			r.emitComponentEvent(ctx, ea, "metrics", result)
		} else {
			ea.Status.Components.MetricsAssessed = true
			componentsChanged = true
			r.Metrics.RecordComponentAssessment("metrics", "skipped", time.Since(startTime).Seconds(), nil)
		}
	} else if !ea.Status.Components.MetricsAssessed && !ea.Spec.Config.PrometheusEnabled {
		ea.Status.Components.MetricsAssessed = true
		componentsChanged = true
		r.Metrics.RecordComponentAssessment("metrics", "skipped", time.Since(startTime).Seconds(), nil)
	}

	// Step 8: Check if all components are done and prepare completion fields
	//
	// If all components are assessed in this reconcile, we set the completion fields
	// (phase=Completed, CompletedAt, AssessmentReason) IN MEMORY alongside the component
	// results and phase transition. This ensures a SINGLE Status().Update() call per
	// reconcile, avoiding optimistic concurrency conflicts from multiple status writes.
	completing := false
	if r.allComponentsDone(ea) {
		completing = true
		now := metav1.Now()
		reason := r.determineAssessmentReason(ea)
		ea.Status.Phase = eav1.PhaseCompleted
		ea.Status.CompletedAt = &now
		ea.Status.AssessmentReason = reason
		ea.Status.Message = fmt.Sprintf("Assessment completed: %s", reason)
		logger.Info("All components done, preparing completion",
			"phase", ea.Status.Phase,
			"reason", reason,
			"componentsChanged", componentsChanged,
			"pendingTransition", pendingTransition,
		)
	}

	// Step 9: Single atomic status update (phase transition + timing + components + completion)
	if componentsChanged || pendingTransition || completing {
		logger.Info("Writing status update",
			"phase", ea.Status.Phase,
			"completing", completing,
			"pendingTransition", pendingTransition,
			"componentsChanged", componentsChanged,
			"resourceVersion", ea.ResourceVersion,
		)
		if err := r.Status().Update(ctx, ea); err != nil {
			logger.Error(err, "Failed to update EA status",
				"phase", ea.Status.Phase,
				"completing", completing,
				"resourceVersion", ea.ResourceVersion,
			)
			r.Metrics.RecordReconcile("error", time.Since(startTime).Seconds())
			return ctrl.Result{RequeueAfter: r.Config.RequeueGenericError}, err
		}
		logger.Info("Status update succeeded",
			"phase", ea.Status.Phase,
			"resourceVersion", ea.ResourceVersion,
			"completing", completing,
		)

		// Emit events and metrics for the Pending -> Assessing transition
		if pendingTransition {
			r.Metrics.RecordPhaseTransition(eav1.PhasePending, eav1.PhaseAssessing)

			// Emit assessment_scheduled audit event (BR-EM-009.4)
			r.emitAssessmentScheduledAuditEvent(ctx, ea)

			r.Recorder.Event(ea, corev1.EventTypeNormal, events.EventReasonAssessmentStarted,
				fmt.Sprintf("Assessment started for correlation %s", ea.Spec.CorrelationID))
			r.Metrics.RecordK8sEvent("Normal", events.EventReasonAssessmentStarted)
		}

		// Emit events and metrics for completion
		if completing {
			r.Metrics.RecordPhaseTransition(eav1.PhaseAssessing, eav1.PhaseCompleted)
			r.Metrics.RecordAssessmentCompleted(ea.Status.AssessmentReason)

			r.emitCompletionEvent(ea, ea.Status.AssessmentReason)
			r.emitCompletedAuditEvent(ctx, ea, ea.Status.AssessmentReason)

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
func (r *Reconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&eav1.EffectivenessAssessment{}).
		Complete(r)
}

// ============================================================================
// COMPONENT ASSESSMENT METHODS
// ============================================================================

// assessHealth evaluates the target resource's health via K8s API (BR-EM-001).
func (r *Reconciler) assessHealth(ctx context.Context, ea *eav1.EffectivenessAssessment) emtypes.ComponentResult {
	logger := log.FromContext(ctx)

	// Build target status from K8s API
	status := r.getTargetHealthStatus(ctx, ea)

	result := r.healthScorer.Score(ctx, status)
	logger.Info("Health assessment complete",
		"score", result.Score,
		"details", result.Details,
	)

	return result
}

// assessHash computes the spec hash of the target resource (BR-EM-004).
func (r *Reconciler) assessHash(ctx context.Context, ea *eav1.EffectivenessAssessment) hash.ComputeResult {
	logger := log.FromContext(ctx)

	specJSON := r.getTargetSpecJSON(ctx, ea)
	result := r.hashComputer.Compute(hash.SpecHashInput{SpecJSON: specJSON})

	logger.Info("Hash computation complete", "hash", result.Hash[:16]+"...")
	return result
}

// assessAlert checks if the original alert has resolved (BR-EM-002).
func (r *Reconciler) assessAlert(ctx context.Context, ea *eav1.EffectivenessAssessment) emtypes.ComponentResult {
	logger := log.FromContext(ctx)

	alertCtx := alert.AlertContext{
		AlertName: ea.Spec.CorrelationID, // Use correlationID as alert name for now
		Namespace: ea.Spec.TargetResource.Namespace,
	}

	result := r.alertScorer.Score(ctx, r.AlertManagerClient, alertCtx)
	logger.Info("Alert assessment complete",
		"score", result.Score,
		"details", result.Details,
	)

	return result
}

// assessMetrics compares pre/post remediation metrics (BR-EM-003).
//
// Uses QueryRange to fetch metric samples over a time window, then compares
// the earliest (pre-remediation) and latest (post-remediation) values using
// the metrics.Scorer to produce a normalized score (0.0-1.0).
//
// Scoring:
//   - 0.0: No improvement or degradation (same values or worse)
//   - >0.0 to 1.0: Improvement detected (lower CPU usage after remediation)
//   - nil: Not assessed (no data available)
func (r *Reconciler) assessMetrics(ctx context.Context, ea *eav1.EffectivenessAssessment) emtypes.ComponentResult {
	logger := log.FromContext(ctx)

	result := emtypes.ComponentResult{
		Component: emtypes.ComponentMetrics,
	}

	// Query Prometheus for metric data over the assessment window.
	// Use the raw gauge value (not rate) since EM compares instantaneous values.
	query := fmt.Sprintf(`container_cpu_usage_seconds_total{namespace="%s"}`,
		ea.Spec.TargetResource.Namespace)

	// Range: from before EA creation to now, capturing pre- and post-remediation samples.
	// The lookback duration is configurable to allow shorter values in E2E tests.
	start := ea.CreationTimestamp.Time.Add(-r.Config.PrometheusLookback)
	end := time.Now()
	// Use 1-second step to capture individual data points with high fidelity.
	// Prometheus gauge metrics store the latest value at each step; a small step
	// ensures that closely-spaced samples (e.g., before/after remediation) are preserved.
	step := 1 * time.Second

	queryResult, err := r.PrometheusClient.QueryRange(ctx, query, start, end, step)
	if err != nil {
		logger.Error(err, "Prometheus range query failed")
		result.Assessed = false
		result.Error = err
		result.Details = "Prometheus range query failed: " + err.Error()
		r.Metrics.RecordExternalCallError("prometheus", "query_range", "query_error")
		return result
	}

	if len(queryResult.Samples) < 2 {
		// Need at least 2 samples to compare before/after
		result.Assessed = false
		result.Details = fmt.Sprintf("insufficient metric data for comparison (%d samples)", len(queryResult.Samples))
		return result
	}

	// Use earliest sample as "before" (pre-remediation) and latest as "after" (post-remediation).
	// Samples from QueryRange are ordered by timestamp.
	earliest := queryResult.Samples[0]
	latest := queryResult.Samples[len(queryResult.Samples)-1]

	// Build comparison and score using the metrics.Scorer (BR-EM-003).
	// CPU usage is LowerIsBetter: a decrease indicates improvement.
	comparisons := []emmetrics.MetricComparison{
		{
			Name:          "container_cpu_usage_seconds_total",
			PreValue:      earliest.Value,
			PostValue:     latest.Value,
			LowerIsBetter: true,
		},
	}

	scored := r.metricScorer.Score(comparisons)
	result = scored.Component

	logger.Info("Metrics assessment complete",
		"score", result.Score,
		"preValue", earliest.Value,
		"postValue", latest.Value,
		"sampleCount", len(queryResult.Samples),
	)

	return result
}

// ============================================================================
// HELPER METHODS
// ============================================================================

// getTargetHealthStatus queries the K8s API for the target resource health.
func (r *Reconciler) getTargetHealthStatus(ctx context.Context, ea *eav1.EffectivenessAssessment) health.TargetStatus {
	logger := log.FromContext(ctx)

	// Look up pods matching the target resource in the target namespace
	podList := &corev1.PodList{}
	err := r.List(ctx, podList,
		client.InNamespace(ea.Spec.TargetResource.Namespace),
		client.MatchingLabels{"app": ea.Spec.TargetResource.Name},
	)
	if err != nil {
		logger.Error(err, "Failed to list pods for target resource")
		return health.TargetStatus{TargetExists: false}
	}

	if len(podList.Items) == 0 {
		return health.TargetStatus{TargetExists: false}
	}

	// Count ready pods and restarts
	totalReplicas := int32(len(podList.Items))
	readyReplicas := int32(0)
	totalRestarts := int32(0)

	for i := range podList.Items {
		pod := &podList.Items[i]
		for _, cs := range pod.Status.ContainerStatuses {
			totalRestarts += cs.RestartCount
			if cs.Ready {
				readyReplicas++
				break // Count pod as ready if any container is ready
			}
		}
	}

	return health.TargetStatus{
		TotalReplicas:           totalReplicas,
		ReadyReplicas:           readyReplicas,
		RestartsSinceRemediation: totalRestarts,
		TargetExists:           true,
	}
}

// getTargetSpecJSON retrieves the target resource's spec as JSON for hash computation.
func (r *Reconciler) getTargetSpecJSON(ctx context.Context, ea *eav1.EffectivenessAssessment) []byte {
	logger := log.FromContext(ctx)

	// Use unstructured client to get any resource type
	// For simplicity, we hash the EA spec itself as a proxy
	// In production, this would fetch the actual Deployment/StatefulSet spec
	specJSON := fmt.Sprintf(`{"kind":"%s","name":"%s","namespace":"%s"}`,
		ea.Spec.TargetResource.Kind,
		ea.Spec.TargetResource.Name,
		ea.Spec.TargetResource.Namespace,
	)

	logger.V(2).Info("Target spec JSON computed", "length", len(specJSON))
	return []byte(specJSON)
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

// setCompletionFields sets the in-memory status fields for assessment completion.
// Extracted to share between the normal completion path and the expired path.
func (r *Reconciler) setCompletionFields(ea *eav1.EffectivenessAssessment, reason string) {
	now := metav1.Now()
	ea.Status.Phase = eav1.PhaseCompleted
	ea.Status.CompletedAt = &now
	ea.Status.AssessmentReason = reason
	ea.Status.Message = fmt.Sprintf("Assessment completed: %s", reason)
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
func (r *Reconciler) determineAssessmentReason(ea *eav1.EffectivenessAssessment) string {
	components := &ea.Status.Components

	allAssessed := components.HealthAssessed && components.HashComputed &&
		(components.AlertAssessed || !ea.Spec.Config.AlertManagerEnabled) &&
		(components.MetricsAssessed || !ea.Spec.Config.PrometheusEnabled)

	anyAssessed := components.HealthAssessed || components.HashComputed ||
		components.AlertAssessed || components.MetricsAssessed

	if allAssessed {
		return eav1.AssessmentReasonFull
	}

	// Check if validity expired (ValidityDeadline is always set by first reconciliation)
	if ea.Status.ValidityDeadline != nil && r.validityChecker.TimeUntilExpired(ea.Status.ValidityDeadline.Time) == 0 {
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
	if ea.Spec.Config.AlertManagerEnabled && !ea.Status.Components.AlertAssessed {
		return false
	}
	if ea.Spec.Config.PrometheusEnabled && !ea.Status.Components.MetricsAssessed {
		return false
	}
	return true
}

// emitCompletionEvent emits a K8s event based on the assessment outcome.
func (r *Reconciler) emitCompletionEvent(ea *eav1.EffectivenessAssessment, reason string) {
	// Calculate average score for threshold check
	avgScore := r.calculateAverageScore(ea)

	if avgScore != nil && *avgScore < ea.Spec.Config.ScoringThreshold {
		r.Recorder.Event(ea, corev1.EventTypeWarning, events.EventReasonRemediationIneffective,
			fmt.Sprintf("Assessment score %.2f below threshold %.2f (reason: %s)",
				*avgScore, ea.Spec.Config.ScoringThreshold, reason))
		r.Metrics.RecordK8sEvent("Warning", events.EventReasonRemediationIneffective)
	} else {
		r.Recorder.Event(ea, corev1.EventTypeNormal, events.EventReasonEffectivenessAssessed,
			fmt.Sprintf("Assessment completed: %s (correlation: %s)", reason, ea.Spec.CorrelationID))
		r.Metrics.RecordK8sEvent("Normal", events.EventReasonEffectivenessAssessed)
	}
}

// emitComponentEvent emits a K8s event and an audit event for an individual component assessment.
// K8s event uses DD-EVENT-001 simplified approach: single EventReasonComponentAssessed with component name in message.
// Audit event uses DD-AUDIT-003: typed EffectivenessAssessmentAuditPayload to DataStorage.
// Error results emit a Warning; successful results emit Normal.
func (r *Reconciler) emitComponentEvent(ctx context.Context, ea *eav1.EffectivenessAssessment, component string, result emtypes.ComponentResult) {
	// K8s Event (DD-EVENT-001)
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

	// Audit Event to DataStorage (DD-AUDIT-003)
	if r.AuditManager != nil {
		if err := r.AuditManager.RecordComponentAssessed(ctx, ea, component, result); err != nil {
			log.FromContext(ctx).V(1).Info("Failed to store component audit event",
				"component", component, "error", err)
		}
		r.Metrics.RecordAuditEvent(string(emtypes.AuditEventTypeForComponent(component)), resultStatus(result))
	}
}

// calculateAverageScore computes the average of non-nil component scores.
func (r *Reconciler) calculateAverageScore(ea *eav1.EffectivenessAssessment) *float64 {
	var total float64
	count := 0

	scores := []*float64{
		ea.Status.Components.HealthScore,
		ea.Status.Components.AlertScore,
		ea.Status.Components.MetricsScore,
	}

	for _, s := range scores {
		if s != nil {
			total += *s
			count++
		}
	}

	if count == 0 {
		return nil
	}

	avg := total / float64(count)
	return &avg
}

// emitAssessmentScheduledAuditEvent emits the assessment.scheduled audit event to DataStorage.
// This event captures all derived timing computed on first reconciliation (BR-EM-009.4).
func (r *Reconciler) emitAssessmentScheduledAuditEvent(ctx context.Context, ea *eav1.EffectivenessAssessment) {
	if r.AuditManager == nil {
		log.FromContext(ctx).V(1).Info("AuditManager not configured, skipping scheduled audit event",
			"eventType", string(emtypes.AuditAssessmentScheduled))
		return
	}

	if err := r.AuditManager.RecordAssessmentScheduled(ctx, ea, r.Config.ValidityWindow); err != nil {
		log.FromContext(ctx).Error(err, "Failed to emit assessment scheduled audit event")
		r.Metrics.RecordAuditEvent(string(emtypes.AuditAssessmentScheduled), "error")
		return
	}
	r.Metrics.RecordAuditEvent(string(emtypes.AuditAssessmentScheduled), "success")
}

// emitCompletedAuditEvent emits the assessment.completed audit event to DataStorage.
// Uses the audit.Manager (Pattern 2) to build and store the typed event.
func (r *Reconciler) emitCompletedAuditEvent(ctx context.Context, ea *eav1.EffectivenessAssessment, reason string) {
	if r.AuditManager == nil {
		log.FromContext(ctx).V(1).Info("AuditManager not configured, skipping audit event",
			"eventType", string(emtypes.AuditAssessmentCompleted))
		return
	}

	if err := r.AuditManager.RecordAssessmentCompleted(ctx, ea, reason); err != nil {
		log.FromContext(ctx).Error(err, "Failed to emit completed audit event")
		r.Metrics.RecordAuditEvent(string(emtypes.AuditAssessmentCompleted), "error")
		return
	}
	r.Metrics.RecordAuditEvent(string(emtypes.AuditAssessmentCompleted), "success")
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
