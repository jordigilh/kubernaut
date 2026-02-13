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
	emclient "github.com/jordigilh/kubernaut/pkg/effectivenessmonitor/client"
	emconfig "github.com/jordigilh/kubernaut/pkg/effectivenessmonitor/config"
	"github.com/jordigilh/kubernaut/pkg/effectivenessmonitor/hash"
	"github.com/jordigilh/kubernaut/pkg/effectivenessmonitor/health"
	emmetrics "github.com/jordigilh/kubernaut/pkg/effectivenessmonitor/metrics"
	"github.com/jordigilh/kubernaut/pkg/effectivenessmonitor/phase"
	emtypes "github.com/jordigilh/kubernaut/pkg/effectivenessmonitor/types"
	"github.com/jordigilh/kubernaut/pkg/effectivenessmonitor/validity"
)

// AuditEmitter defines the interface for emitting audit events from the reconciler.
// This abstraction allows the reconciler to emit audit events without depending
// on the specific OpenAPI types, which will be wired when EM event types are
// added to the DataStorage OpenAPI spec.
type AuditEmitter interface {
	// EmitAssessmentCompleted emits an audit event when the assessment finishes.
	EmitAssessmentCompleted(ctx context.Context, ea *eav1.EffectivenessAssessment, reason string) error
}

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
	AuditEmitter       AuditEmitter

	// Internal components (created in constructor)
	healthScorer    health.Scorer
	alertScorer     alert.Scorer
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
}

// NewReconciler creates a new Reconciler with all dependencies injected.
// Per DD-METRICS-001: Metrics wired via dependency injection.
func NewReconciler(
	c client.Client,
	s *runtime.Scheme,
	recorder record.EventRecorder,
	m *emmetrics.Metrics,
	promClient emclient.PrometheusQuerier,
	amClient emclient.AlertManagerClient,
	cfg ReconcilerConfig,
) *Reconciler {
	return &Reconciler{
		Client:             c,
		Scheme:             s,
		Recorder:           recorder,
		Metrics:            m,
		PrometheusClient:   promClient,
		AlertManagerClient: amClient,
		healthScorer:       health.NewScorer(),
		alertScorer:        alert.NewScorer(),
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
		return ctrl.Result{RequeueAfter: emconfig.RequeueGenericError}, err
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
	windowState := r.validityChecker.Check(
		ea.CreationTimestamp.Time,
		ea.Spec.Config.StabilizationWindow.Duration,
		ea.Spec.Config.ValidityDeadline.Time,
	)

	// Step 4: If expired -> complete with partial data
	if windowState == validity.WindowExpired {
		logger.Info("Validity window expired, completing with available data")
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

	// Step 6: Transition Pending -> Assessing
	if currentPhase == eav1.PhasePending || currentPhase == "" {
		if err := r.transitionPhase(ctx, ea, eav1.PhaseAssessing); err != nil {
			logger.Error(err, "Failed to transition to Assessing")
			r.Metrics.RecordReconcile("error", time.Since(startTime).Seconds())
			return ctrl.Result{RequeueAfter: emconfig.RequeueGenericError}, err
		}
		r.Recorder.Event(ea, corev1.EventTypeNormal, "AssessmentStarted",
			fmt.Sprintf("Assessment started for correlation %s", ea.Spec.CorrelationID))
		r.Metrics.RecordK8sEvent("Normal", "AssessmentStarted")
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
	}

	// Hash check (BR-EM-004)
	if !ea.Status.Components.HashComputed {
		result := r.assessHash(ctx, ea)
		ea.Status.Components.HashComputed = result.Component.Assessed
		ea.Status.Components.PostRemediationSpecHash = result.Hash
		componentsChanged = true
		r.Metrics.RecordComponentAssessment("hash", resultStatus(result.Component), time.Since(startTime).Seconds(), nil)
	}

	// Alert check (BR-EM-002) - skip if disabled
	if !ea.Status.Components.AlertAssessed && ea.Spec.Config.AlertManagerEnabled {
		if r.AlertManagerClient != nil {
			result := r.assessAlert(ctx, ea)
			ea.Status.Components.AlertAssessed = result.Assessed
			ea.Status.Components.AlertScore = result.Score
			componentsChanged = true
			r.Metrics.RecordComponentAssessment("alert", resultStatus(result), time.Since(startTime).Seconds(), result.Score)
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

	// Step 8: Update status if changed
	if componentsChanged {
		if err := r.Status().Update(ctx, ea); err != nil {
			logger.Error(err, "Failed to update EA status with component results")
			r.Metrics.RecordReconcile("error", time.Since(startTime).Seconds())
			return ctrl.Result{RequeueAfter: emconfig.RequeueGenericError}, err
		}
	}

	// Step 9: Check if all components are done
	if r.allComponentsDone(ea) {
		return r.completeAssessment(ctx, ea, startTime)
	}

	// Step 10: Requeue for remaining components
	r.Metrics.RecordReconcile("requeue", time.Since(startTime).Seconds())
	return ctrl.Result{RequeueAfter: emconfig.RequeueAssessmentInProgress}, nil
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
func (r *Reconciler) assessMetrics(ctx context.Context, ea *eav1.EffectivenessAssessment) emtypes.ComponentResult {
	logger := log.FromContext(ctx)

	// Query Prometheus for metric data
	// For now, attempt a basic query and score based on the result
	result := emtypes.ComponentResult{
		Component: emtypes.ComponentMetrics,
	}

	query := fmt.Sprintf(`rate(container_cpu_usage_seconds_total{namespace="%s"}[5m])`,
		ea.Spec.TargetResource.Namespace)

	queryResult, err := r.PrometheusClient.Query(ctx, query, time.Now())
	if err != nil {
		logger.Error(err, "Prometheus query failed")
		result.Assessed = false
		result.Error = err
		result.Details = "Prometheus query failed: " + err.Error()
		r.Metrics.RecordExternalCallError("prometheus", "query", "query_error")
		return result
	}

	result.Assessed = true
	if len(queryResult.Samples) == 0 {
		// No data available yet
		result.Assessed = false
		result.Details = "no metric data available"
		return result
	}

	// Simple scoring: if metrics are available, score based on value
	// In production, this would compare pre/post values
	score := 0.5 // Default: metrics available but no comparison baseline
	result.Score = &score
	result.Details = fmt.Sprintf("%d metric samples available", len(queryResult.Samples))

	logger.Info("Metrics assessment complete",
		"score", result.Score,
		"samples", len(queryResult.Samples),
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

// transitionPhase updates the EA status phase with validation.
func (r *Reconciler) transitionPhase(ctx context.Context, ea *eav1.EffectivenessAssessment, target string) error {
	current := ea.Status.Phase
	if current == "" {
		current = eav1.PhasePending
	}

	if !phase.CanTransition(current, target) {
		return fmt.Errorf("invalid phase transition: %s -> %s", current, target)
	}

	ea.Status.Phase = target
	if err := r.Status().Update(ctx, ea); err != nil {
		return fmt.Errorf("updating phase to %s: %w", target, err)
	}

	r.Metrics.RecordPhaseTransition(current, target)
	return nil
}

// completeAssessment finalizes the EA with Completed phase and assessment reason.
func (r *Reconciler) completeAssessment(ctx context.Context, ea *eav1.EffectivenessAssessment, startTime time.Time) (ctrl.Result, error) {
	logger := log.FromContext(ctx)

	// Determine assessment reason
	reason := r.determineAssessmentReason(ea)
	now := metav1.Now()

	// Update status to Completed
	ea.Status.Phase = eav1.PhaseCompleted
	ea.Status.CompletedAt = &now
	ea.Status.AssessmentReason = reason
	ea.Status.Message = fmt.Sprintf("Assessment completed: %s", reason)

	if err := r.Status().Update(ctx, ea); err != nil {
		logger.Error(err, "Failed to update EA to Completed")
		r.Metrics.RecordReconcile("error", time.Since(startTime).Seconds())
		return ctrl.Result{RequeueAfter: emconfig.RequeueGenericError}, err
	}

	r.Metrics.RecordPhaseTransition(eav1.PhaseAssessing, eav1.PhaseCompleted)
	r.Metrics.RecordAssessmentCompleted(reason)

	// Emit K8s event based on score
	r.emitCompletionEvent(ea, reason)

	// Emit audit event
	r.emitCompletedAuditEvent(ctx, ea, reason)

	logger.Info("Assessment completed",
		"reason", reason,
		"correlationID", ea.Spec.CorrelationID,
	)

	r.Metrics.RecordReconcile("success", time.Since(startTime).Seconds())
	return ctrl.Result{}, nil
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

	// Check if validity expired
	if r.validityChecker.TimeUntilExpired(ea.Spec.Config.ValidityDeadline.Time) == 0 {
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
		r.Recorder.Event(ea, corev1.EventTypeWarning, "RemediationIneffective",
			fmt.Sprintf("Assessment score %.2f below threshold %.2f (reason: %s)",
				*avgScore, ea.Spec.Config.ScoringThreshold, reason))
		r.Metrics.RecordK8sEvent("Warning", "RemediationIneffective")
	} else {
		r.Recorder.Event(ea, corev1.EventTypeNormal, "EffectivenessAssessed",
			fmt.Sprintf("Assessment completed: %s (correlation: %s)", reason, ea.Spec.CorrelationID))
		r.Metrics.RecordK8sEvent("Normal", "EffectivenessAssessed")
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

// emitCompletedAuditEvent emits the assessment.completed audit event to DataStorage.
// Uses the AuditEmitter abstraction, which will be wired when EM event types
// are added to the DataStorage OpenAPI spec (separate task).
func (r *Reconciler) emitCompletedAuditEvent(ctx context.Context, ea *eav1.EffectivenessAssessment, reason string) {
	if r.AuditEmitter == nil {
		// Audit emitter not wired yet - log but don't fail
		log.FromContext(ctx).V(1).Info("AuditEmitter not configured, skipping audit event",
			"eventType", string(emtypes.AuditAssessmentCompleted))
		return
	}

	if err := r.AuditEmitter.EmitAssessmentCompleted(ctx, ea, reason); err != nil {
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
