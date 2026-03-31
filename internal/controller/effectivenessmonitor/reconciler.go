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
	"k8s.io/client-go/tools/record"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	ctrlcontroller "sigs.k8s.io/controller-runtime/pkg/controller"
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
	emtiming "github.com/jordigilh/kubernaut/pkg/effectivenessmonitor/timing"
	emtypes "github.com/jordigilh/kubernaut/pkg/effectivenessmonitor/types"
	"github.com/jordigilh/kubernaut/pkg/effectivenessmonitor/validity"
	"github.com/jordigilh/kubernaut/pkg/shared/events"
	canonicalhash "github.com/jordigilh/kubernaut/pkg/shared/hash"
	k8sutil "github.com/jordigilh/kubernaut/pkg/shared/k8s"
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

// assessmentScope classifies how deeply the reconciler should assess an EA,
// based on the workflow execution lifecycle (ADR-EM-001 §5, #573 G4).
type assessmentScope int

const (
	// scopeFull runs all configured component checks (health, hash, alert, metrics).
	scopeFull assessmentScope = iota
	// scopePartial runs only health + hash. Used when the WFE started but never
	// completed — metrics and alerts are meaningless without a completed workflow.
	scopePartial
	// scopeNoExecution skips all component checks. Used when no WFE started event
	// exists (remediation failed before execution, e.g., AA rejected).
	scopeNoExecution
)

// determineAssessmentScope queries DataStorage to classify the assessment depth.
// Returns scopeFull when DSQuerier is nil or on transient errors (graceful degradation).
func (r *Reconciler) determineAssessmentScope(ctx context.Context, correlationID string) assessmentScope {
	if r.DSQuerier == nil {
		return scopeFull
	}

	logger := log.FromContext(ctx)

	started, err := r.DSQuerier.HasWorkflowStarted(ctx, correlationID)
	if err != nil {
		logger.Error(err, "Failed to check workflow started status (non-fatal, assuming full assessment)",
			"correlationID", correlationID)
		return scopeFull
	}
	if !started {
		return scopeNoExecution
	}

	completed, err := r.DSQuerier.HasWorkflowCompleted(ctx, correlationID)
	if err != nil {
		logger.Error(err, "Failed to check workflow completed status (non-fatal, assuming full assessment)",
			"correlationID", correlationID)
		return scopeFull
	}
	if !completed {
		return scopePartial
	}

	return scopeFull
}

// validateEASpec checks for unrecoverable spec errors that should immediately fail the EA.
// Returns the validation failure reason, or empty string if the spec is valid.
func validateEASpec(ea *eav1.EffectivenessAssessment) string {
	if ea.Spec.CorrelationID == "" {
		return "correlationID is required"
	}
	return ""
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

	// Step 1: Fetch EA
	ea := &eav1.EffectivenessAssessment{}
	if err := r.Get(ctx, req.NamespacedName, ea); err != nil {
		if apierrors.IsNotFound(err) {
			// EA was deleted, nothing to do
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

	// Step 1b: Spec validation — fail-fast for unrecoverable spec errors (#573, ADR-EM-001 §11)
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

	// Step 3: Check validity window
	// Issue #253, #277: For async targets, the stabilization anchor is
	// creation + HashComputeDelay (not creation alone).
	stabilizationAnchor := ea.CreationTimestamp
	isAsync := ea.Spec.Config.HashComputeDelay != nil && ea.Spec.Config.HashComputeDelay.Duration > 0
	if isAsync {
		stabilizationAnchor = metav1.NewTime(ea.CreationTimestamp.Add(ea.Spec.Config.HashComputeDelay.Duration))
	}

	var windowState validity.WindowState
	if ea.Status.ValidityDeadline != nil {
		windowState = r.validityChecker.Check(
			stabilizationAnchor,
			ea.Spec.Config.StabilizationWindow.Duration,
			*ea.Status.ValidityDeadline,
		)
	} else {
		stabilizationEnd := stabilizationAnchor.Add(ea.Spec.Config.StabilizationWindow.Duration)
		if time.Now().Before(stabilizationEnd) {
			windowState = validity.WindowStabilizing
		} else {
			windowState = validity.WindowActive
		}
	}

	// Step 3b: WaitingForPropagation phase (Issue #253, #277, BR-EM-010.3)
	// For async targets where the hash deferral deadline is still in the future,
	// enter/stay in WaitingForPropagation. Requeue until deadline elapses.
	hashDeadline := stabilizationAnchor // for async, this is creation + HashComputeDelay
	if isAsync && time.Now().Before(hashDeadline.Time) {
		if currentPhase == eav1.PhasePending || currentPhase == "" {
			dt := emtiming.ComputeDerivedTiming(ea.CreationTimestamp, ea.Spec.Config.StabilizationWindow.Duration, r.Config.ValidityWindow, ea.Spec.Config.HashComputeDelay, ea.Spec.Config.AlertCheckDelay)
			deadline := dt.ValidityDeadline
			checkAfter := dt.CheckAfter
			alertCheckAfter := dt.AlertCheckAfter

			ea.Status.Phase = eav1.PhaseWaitingForPropagation
			ea.Status.ValidityDeadline = &deadline
			ea.Status.PrometheusCheckAfter = &checkAfter
			ea.Status.AlertManagerCheckAfter = &alertCheckAfter

			logger.Info("Async target: entered WaitingForPropagation (BR-EM-010.3)",
				"hashComputeDelay", ea.Spec.Config.HashComputeDelay.Duration,
				"validityDeadline", deadline,
				"checkAfter", checkAfter,
				"alertCheckAfter", alertCheckAfter,
			)

			if err := r.Status().Update(ctx, ea); err != nil {
				logger.Error(err, "Failed to persist WaitingForPropagation phase")
				return ctrl.Result{RequeueAfter: r.Config.RequeueGenericError}, err
			}

			r.emitScheduledEventIfFirst(ctx, ea)
		}

		remaining := time.Until(hashDeadline.Time)
		logger.Info("Waiting for async propagation to complete", "remaining", remaining)
		return ctrl.Result{RequeueAfter: remaining}, nil
	}

	// Step 4: If expired -> complete with partial data (ADR-EM-001: validity window enforcement)
	if windowState == validity.WindowExpired {
		logger.Info("Validity window expired, completing with available data")
		r.Recorder.Event(ea, corev1.EventTypeWarning, events.EventReasonAssessmentExpired,
			fmt.Sprintf("Validity window expired for correlation %s; completing with available data",
				ea.Spec.CorrelationID))
		r.Metrics.RecordValidityExpiration()
		return r.completeAssessment(ctx, ea)
	}

	// Step 5: If stabilizing -> persist derived timing if not yet set, then requeue
	// BR-EM-009: ValidityDeadline is computed on first reconciliation. When StabilizationWindow
	// is long (e.g., 35m), we must persist it during stabilization so operators see the
	// deadline and integration tests can assert it within a reasonable timeout.
	if windowState == validity.WindowStabilizing {
		remaining := r.validityChecker.TimeUntilStabilized(
			stabilizationAnchor,
			ea.Spec.Config.StabilizationWindow.Duration,
		)

		// Issue #253: WaitingForPropagation → Stabilizing transition.
		// When HCA has elapsed and we're in WaitingForPropagation, transition to Stabilizing.
		// ValidityDeadline was already persisted in step 3b, so only update the phase.
		if currentPhase == eav1.PhaseWaitingForPropagation {
			ea.Status.Phase = eav1.PhaseStabilizing
			logger.Info("Async target: WaitingForPropagation → Stabilizing (HCA elapsed)",
				"remaining", remaining)
			if err := r.Status().Update(ctx, ea); err != nil {
				logger.Error(err, "Failed to persist WaitingForPropagation → Stabilizing transition")
				return ctrl.Result{RequeueAfter: r.Config.RequeueGenericError}, err
			}
		}

		// BR-EM-009: Pre-compute and persist ValidityDeadline during stabilization so
		// operators can observe the deadline immediately. Transition to Stabilizing phase.
		if ea.Status.ValidityDeadline == nil && (currentPhase == eav1.PhasePending || currentPhase == "") {
			dt := emtiming.ComputeDerivedTiming(ea.CreationTimestamp, ea.Spec.Config.StabilizationWindow.Duration, r.Config.ValidityWindow, ea.Spec.Config.HashComputeDelay, ea.Spec.Config.AlertCheckDelay)
			if dt.Extended {
				logger.Info("Runtime guard: extended ValidityDeadline (StabilizationWindow >= ValidityWindow)",
					"originalValidity", r.Config.ValidityWindow,
					"effectiveValidity", dt.EffectiveValidity,
					"stabilizationWindow", ea.Spec.Config.StabilizationWindow.Duration,
				)
				r.Recorder.Eventf(ea, corev1.EventTypeWarning, "ValidityWindowExtended",
					"StabilizationWindow (%v) >= ValidityWindow (%v); extended deadline to %v",
					ea.Spec.Config.StabilizationWindow.Duration, r.Config.ValidityWindow, dt.EffectiveValidity)
			}
			deadline := dt.ValidityDeadline
			checkAfter := dt.CheckAfter
			alertCheckAfter := dt.AlertCheckAfter

			ea.Status.Phase = eav1.PhaseStabilizing
			ea.Status.ValidityDeadline = &deadline
			ea.Status.PrometheusCheckAfter = &checkAfter
			ea.Status.AlertManagerCheckAfter = &alertCheckAfter

			logger.Info("Transitioned to Stabilizing, persisted derived timing (BR-EM-009)",
				"creationTimestamp", ea.CreationTimestamp,
				"effectiveValidity", dt.EffectiveValidity,
				"stabilizationWindow", ea.Spec.Config.StabilizationWindow.Duration,
				"validityDeadline", deadline,
			)

			if err := r.Status().Update(ctx, ea); err != nil {
				logger.Error(err, "Failed to persist Stabilizing phase and derived timing")
				return ctrl.Result{RequeueAfter: r.Config.RequeueGenericError}, err
			}

			r.emitScheduledEventIfFirst(ctx, ea)
		}

		logger.Info("Stabilization window active, requeueing", "remaining", remaining)
		return ctrl.Result{RequeueAfter: remaining}, nil
	}

	// Step 6: Transition Pending/Stabilizing -> Assessing + compute derived timing (BR-EM-009)
	//
	// We set phase and derived timing in-memory here but defer the status write to
	// Step 9 (single atomic update). This avoids an intermediate Status().Update()
	// that would change the resourceVersion and cause optimistic concurrency conflicts
	// when component checks later attempt their own status update.
	pendingTransition := false
	if currentPhase == eav1.PhasePending || currentPhase == eav1.PhaseWaitingForPropagation || currentPhase == eav1.PhaseStabilizing || currentPhase == "" {
		if !phase.CanTransition(currentPhase, eav1.PhaseAssessing) {
			logger.Error(nil, "Invalid phase transition", "from", currentPhase, "to", eav1.PhaseAssessing)
			return ctrl.Result{RequeueAfter: r.Config.RequeueGenericError}, fmt.Errorf("invalid phase transition: %s -> %s", currentPhase, eav1.PhaseAssessing)
		}

		// Compute all derived timing fields on first reconciliation.
		// These are persisted in status to avoid recomputation and for operator observability.
		// See timing.ComputeDerivedTiming for the formula and runtime guard (Issue #188).
		dt := emtiming.ComputeDerivedTiming(ea.CreationTimestamp, ea.Spec.Config.StabilizationWindow.Duration, r.Config.ValidityWindow, ea.Spec.Config.HashComputeDelay, ea.Spec.Config.AlertCheckDelay)
		if dt.Extended {
			logger.Info("Runtime guard: extended ValidityDeadline (StabilizationWindow >= ValidityWindow)",
				"originalValidity", r.Config.ValidityWindow,
				"effectiveValidity", dt.EffectiveValidity,
				"stabilizationWindow", ea.Spec.Config.StabilizationWindow.Duration,
			)
			r.Recorder.Eventf(ea, corev1.EventTypeWarning, "ValidityWindowExtended",
				"StabilizationWindow (%v) >= ValidityWindow (%v); extended deadline to %v",
				ea.Spec.Config.StabilizationWindow.Duration, r.Config.ValidityWindow, dt.EffectiveValidity)
		}
		deadline := dt.ValidityDeadline
		checkAfter := dt.CheckAfter
		alertCheckAfter := dt.AlertCheckAfter

		ea.Status.Phase = eav1.PhaseAssessing
		ea.Status.ValidityDeadline = &deadline
		ea.Status.PrometheusCheckAfter = &checkAfter
		ea.Status.AlertManagerCheckAfter = &alertCheckAfter

		logger.Info("Computed derived timing (BR-EM-009)",
			"creationTimestamp", ea.CreationTimestamp,
			"configuredValidityWindow", r.Config.ValidityWindow,
			"effectiveValidity", dt.EffectiveValidity,
			"stabilizationWindow", ea.Spec.Config.StabilizationWindow.Duration,
			"validityDeadline", deadline,
			"prometheusCheckAfter", checkAfter,
			"alertManagerCheckAfter", alertCheckAfter,
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
		currentSpec, _ := r.getTargetSpec(ctx, ea.Spec.RemediationTarget)
		specHash, specHashErr := canonicalhash.CanonicalSpecHash(currentSpec)
		if specHashErr != nil {
			logger.Error(specHashErr, "Failed to compute current spec hash for drift check")
		} else {
			driftConfigMapHashes := r.resolveConfigMapHashes(ctx, currentSpec, ea.Spec.RemediationTarget)
			if len(driftConfigMapHashes) > 0 {
				logger.V(2).Info("Drift guard resolved ConfigMap hashes",
					"configMapCount", len(driftConfigMapHashes),
					"correlationID", ea.Spec.CorrelationID)
			}
			currentHash, compositeErr := canonicalhash.CompositeSpecHash(specHash, driftConfigMapHashes)
			if compositeErr != nil {
				logger.Error(compositeErr, "Failed to compute composite hash for drift check")
			} else {
				ea.Status.Components.CurrentSpecHash = currentHash

				if currentHash != ea.Status.Components.PostRemediationSpecHash {
					logger.Info("Spec drift detected — resource modified since post-remediation hash (DD-EM-002 v1.1)",
						"postRemediationHash", ea.Status.Components.PostRemediationSpecHash,
						"currentHash", currentHash,
						"correlationID", ea.Spec.CorrelationID,
					)

					conditions.SetCondition(ea, conditions.ConditionSpecIntegrity,
						metav1.ConditionFalse, conditions.ReasonSpecDrifted,
						fmt.Sprintf("Target spec hash changed: %s -> %s",
							ea.Status.Components.PostRemediationSpecHash, currentHash))

					conditions.SetCondition(ea, conditions.ConditionAssessmentComplete,
						metav1.ConditionTrue, conditions.ReasonSpecDrift,
						"Assessment invalidated: target resource spec was modified (spec drift detected)")

					r.Recorder.Event(ea, corev1.EventTypeWarning, events.EventReasonSpecDriftDetected,
						fmt.Sprintf("Target resource spec modified during assessment (correlation: %s)", ea.Spec.CorrelationID))

					return r.completeAssessmentWithReason(ctx, ea, eav1.AssessmentReasonSpecDrift)
				}

				conditions.SetCondition(ea, conditions.ConditionSpecIntegrity,
					metav1.ConditionTrue, conditions.ReasonSpecUnchanged,
					"Target resource spec unchanged since post-remediation hash")
			}
		}
	}

	// Step 6b: Assessment scope determination (ADR-EM-001 §5, #573 G4)
	// Query DataStorage to classify the assessment depth based on WFE lifecycle:
	//   scopeNoExecution → WFE never started → complete immediately
	//   scopePartial     → WFE started but not completed → health+hash only
	//   scopeFull        → WFE completed (or DSQuerier unavailable) → all components
	scope := r.determineAssessmentScope(ctx, ea.Spec.CorrelationID)
	if scope == scopeNoExecution {
		logger.Info("No workflow execution started for this correlation ID — completing as no_execution",
			"correlationID", ea.Spec.CorrelationID)
		return r.completeAssessmentWithReason(ctx, ea, eav1.AssessmentReasonNoExecution)
	}
	if scope == scopePartial {
		logger.Info("Workflow started but not completed — narrowing to health+hash assessment (ADR-EM-001 §5)",
			"correlationID", ea.Spec.CorrelationID)
	}

	// Step 7: Run component checks (skip already-completed)
	componentsChanged := false
	var alertDeferred alert.AlertDeferralResult

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
		// DD-EM-004, #277: Defer hash for async-managed targets (GitOps, operator CRDs).
		// The RO sets HashComputeDelay when the RemediationTarget is managed by an
		// external controller whose reconciliation happens after the WE completes.
		deferral := hash.CheckHashDeferral(ea)
		if deferral.ShouldDefer {
			logger.V(1).Info("Hash computation deferred for async-managed target",
				"hashComputeDelay", ea.Spec.Config.HashComputeDelay.Duration,
				"remaining", deferral.RequeueAfter)
			return ctrl.Result{RequeueAfter: deferral.RequeueAfter}, nil
		}
		result := r.assessHash(ctx, ea)
		ea.Status.Components.HashComputed = result.Component.Assessed
		ea.Status.Components.PostRemediationSpecHash = result.Hash
		// Set CurrentSpecHash to match PostRemediationSpecHash on first capture.
		// This ensures CurrentSpecHash is always populated when the EA completes
		// (even on single-pass completions where Step 6.5 never runs).
		ea.Status.Components.CurrentSpecHash = result.Hash
		componentsChanged = true
		r.Metrics.RecordComponentAssessment("hash", resultStatus(result.Component), nil)
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
		r.Metrics.RecordComponentAssessment("health", resultStatus(healthResult.Component), healthResult.Component.Score)
		// Skip health audit event when score is nil (N/A for non-pod resources).
		// The component is marked Assessed=true so allComponentsDone sees it as complete,
		// but there is no meaningful health data to emit in the audit trail.
		// DD-CONTROLLER-001 Pattern C: emit health audit only on first assessment.
		// Alert decay (#369) resets HealthAssessed for re-probe; without this guard
		// the audit event would be emitted again on subsequent reconciles.
		if healthResult.Component.Score != nil && ea.Status.Components.AlertDecayRetries == 0 {
			r.emitHealthEvent(ctx, ea, healthResult)
		}
	}

	// Step 7a: Partial-execution early completion (ADR-EM-001 §5, #573 G4)
	// When WFE started but completed event isn't in DS yet, health+hash are done but
	// alert/metrics should only be skipped if we're confident the workflow genuinely
	// didn't complete. The workflow.completed event may be delayed by DS's async batch
	// writer (typically <5s). Requeue within a grace period to re-evaluate scope;
	// after the grace period, accept partial as the final state.
	if scope == scopePartial && ea.Status.Components.HealthAssessed && ea.Status.Components.HashComputed {
		const partialGracePeriod = 30 * time.Second
		age := time.Since(ea.CreationTimestamp.Time)
		if age < partialGracePeriod {
			logger.Info("Partial scope within grace period — requeueing to re-evaluate after workflow.completed may arrive",
				"correlationID", ea.Spec.CorrelationID,
				"eaAge", age.Round(time.Second),
				"gracePeriod", partialGracePeriod)
			return ctrl.Result{RequeueAfter: 5 * time.Second}, nil
		}
		return r.completeAssessmentWithReason(ctx, ea, eav1.AssessmentReasonPartial)
	}

	// Alert check (BR-EM-002) - skip if disabled or client unavailable.
	// #277: Defer alert check when AlertCheckDelay is set (proactive signals).
	// #369: Detect alert decay — keep EA open when resource is healthy but alert firing.
	// Health and metrics proceed independently; only alert resolution is gated.
	if !ea.Status.Components.AlertAssessed {
		if r.Config.AlertManagerEnabled && r.AlertManagerClient != nil {
			alertDeferred = alert.CheckAlertDeferral(ea)
			if alertDeferred.ShouldDefer {
				logger.V(1).Info("Alert check deferred (proactive signal, #277)",
					"alertManagerCheckAfter", ea.Status.AlertManagerCheckAfter,
					"remaining", alertDeferred.RequeueAfter)
				r.Metrics.RecordComponentAssessment("alert", "deferred", nil)
			} else {
				alertResult := r.assessAlert(ctx, ea)

				if r.isAlertDecay(ea, alertResult) {
					if ea.Status.Components.AlertDecayRetries == 0 {
						r.emitAlertDecayEvent(ctx, ea, alertResult)
					}
					ea.Status.Components.AlertDecayRetries++
					alertResult.Component.Assessed = false
					// Health re-probe (#369 Option D): reset HealthAssessed so the next
					// reconcile re-probes live from K8s API. Prevents stale health data
					// from masking a genuine failure that develops after the initial check.
					ea.Status.Components.HealthAssessed = false

					// BR-EM-012, DD-CRD-002: Signal active decay monitoring via condition.
					conditions.SetCondition(ea, conditions.ConditionAlertDecayDetected,
						metav1.ConditionTrue, conditions.ReasonDecayActive,
						fmt.Sprintf("Alert decay suspected: health=%.1f, alert still firing, retries=%d",
							*ea.Status.Components.HealthScore, ea.Status.Components.AlertDecayRetries))

					logger.Info("Alert decay suspected: deferring alert assessment, scheduling health re-probe",
						"healthScore", ea.Status.Components.HealthScore,
						"alertScore", alertResult.Component.Score,
						"retries", ea.Status.Components.AlertDecayRetries)
				} else {
					// BR-EM-012: If we were previously tracking decay and the hypothesis
					// is now killed (alert resolved, health degraded, or metrics gate),
					// mark the condition as resolved.
					if ea.Status.Components.AlertDecayRetries > 0 {
						conditions.SetCondition(ea, conditions.ConditionAlertDecayDetected,
							metav1.ConditionFalse, conditions.ReasonDecayResolved,
							"Alert decay monitoring resolved: alert is no longer considered decaying")
					}
					r.emitAlertEvent(ctx, ea, alertResult)
				}

				ea.Status.Components.AlertAssessed = alertResult.Component.Assessed
				ea.Status.Components.AlertScore = alertResult.Component.Score
				r.Metrics.RecordComponentAssessment("alert", resultStatus(alertResult.Component), alertResult.Component.Score)
				componentsChanged = true
			}
		} else {
			ea.Status.Components.AlertAssessed = true
			r.Metrics.RecordComponentAssessment("alert", "skipped", nil)
			componentsChanged = true
		}
	}

	// Metrics check (BR-EM-003) - skip if disabled or client unavailable
	if !ea.Status.Components.MetricsAssessed {
		if r.Config.PrometheusEnabled && r.PrometheusClient != nil {
			metricsResult := r.assessMetrics(ctx, ea)
			ea.Status.Components.MetricsAssessed = metricsResult.Component.Assessed
			ea.Status.Components.MetricsScore = metricsResult.Component.Score
			r.Metrics.RecordComponentAssessment("metrics", resultStatus(metricsResult.Component), metricsResult.Component.Score)
			// Only emit the audit event when the metrics assessment succeeds.
			// If Prometheus returns no data, Assessed=false and we silently retry
			// on the next reconcile. Emitting on every retry would create duplicate
			// audit events. The completion event captures metrics_timed_out if needed.
			if metricsResult.Component.Assessed {
				r.emitMetricsEvent(ctx, ea, metricsResult)
			}
		} else {
			ea.Status.Components.MetricsAssessed = true
			r.Metrics.RecordComponentAssessment("metrics", "skipped", nil)
		}
		componentsChanged = true
	}

	// Step 7b: Precise requeue when only alert is deferred (#277).
	// If health, hash, and metrics are done but alert is deferred for a proactive signal,
	// sleep until AlertManagerCheckAfter instead of polling every 15s.
	if alertDeferred.ShouldDefer &&
		ea.Status.Components.HealthAssessed && ea.Status.Components.HashComputed &&
		(ea.Status.Components.MetricsAssessed || !r.Config.PrometheusEnabled) {
		if componentsChanged || pendingTransition {
			if err := r.Status().Update(ctx, ea); err != nil {
				logger.Error(err, "Failed to persist status before alert deferral requeue")
				return ctrl.Result{RequeueAfter: r.Config.RequeueGenericError}, err
			}
		}
		logger.Info("All components except alert done; precise requeue for alert deferral (#277)",
			"requeueAfter", alertDeferred.RequeueAfter)
		return ctrl.Result{RequeueAfter: alertDeferred.RequeueAfter}, nil
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
			return ctrl.Result{RequeueAfter: r.Config.RequeueGenericError}, err
		}

		// Post-update event emission (after successful persist)
		if pendingTransition {
			r.emitAssessingTransitionEvents(ctx, ea)
		}
		if completing {
			r.emitCompletionMetricsAndEvents(ctx, ea, ea.Status.AssessmentReason)
			logger.Info("Assessment completed",
				"reason", ea.Status.AssessmentReason,
				"correlationID", ea.Spec.CorrelationID,
			)
			return ctrl.Result{}, nil
		}
	}

	// Step 10: Requeue for remaining components.
	// Cap at remaining validity time so the expiry check (Step 4) fires on time
	// instead of up to one full interval late (BR-EM-007, Issue #591).
	requeue := r.Config.RequeueAssessmentInProgress
	if ea.Status.ValidityDeadline != nil {
		remaining := r.validityChecker.TimeUntilExpired(ea.Status.ValidityDeadline.Time)
		if remaining > 0 && remaining < requeue {
			requeue = remaining
		}
	}
	return ctrl.Result{RequeueAfter: requeue}, nil
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

	// Build target status from K8s API (DD-EM-003: health uses RemediationTarget).
	// The remediation target (e.g. Deployment) survives rolling restarts, whereas the
	// signal target (e.g. the original Pod) may be deleted and replaced (#275).
	// Pass RemediationCreatedAt so restarts can be counted relative to remediation time (#246).
	status := r.getTargetHealthStatus(ctx, ea.Spec.RemediationTarget, ea.Spec.RemediationCreatedAt)

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

	// Step 1: Fetch target spec from K8s API (DD-EM-003: hash uses RemediationTarget)
	spec, postHashDegradedReason := r.getTargetSpec(ctx, ea.Spec.RemediationTarget)

	// Issue #546: Set EA condition based on spec fetch result
	if postHashDegradedReason != "" {
		conditions.SetCondition(ea, conditions.ConditionPostHashCaptured,
			metav1.ConditionFalse, conditions.ReasonPostHashCaptureFailed, postHashDegradedReason)
		logger.Info("Post-remediation spec fetch degraded, hash comparison will be unreliable",
			"degradedReason", postHashDegradedReason)
	} else {
		conditions.SetCondition(ea, conditions.ConditionPostHashCaptured,
			metav1.ConditionTrue, conditions.ReasonPostHashCaptured, "Post-remediation spec hash captured")
	}

	// Step 2: Read pre-remediation hash from EA spec (set by RO via RR status).
	// Falls back to DataStorage query for backward compatibility with EAs created
	// before the RO started populating PreRemediationSpecHash.
	preHash := ea.Spec.PreRemediationSpecHash
	if preHash == "" {
		logger.V(1).Info("PreRemediationSpecHash not in EA spec, falling back to DataStorage query")
		preHash = r.queryPreRemediationHash(ctx, ea.Spec.CorrelationID)
	}

	// Step 3: Resolve ConfigMap content hashes (#396, BR-EM-004)
	configMapHashes := r.resolveConfigMapHashes(ctx, spec, ea.Spec.RemediationTarget)

	// Step 4: Compute post-hash (composite when ConfigMaps present) and compare with pre-hash
	result := r.hashComputer.Compute(hash.SpecHashInput{
		Spec:            spec,
		PreHash:         preHash,
		ConfigMapHashes: configMapHashes,
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

// resolveConfigMapHashes extracts ConfigMap references from the resource spec,
// fetches each ConfigMap via the uncached apiReader (bypassing the informer cache),
// and returns a map of name -> content hash.
// Missing/forbidden ConfigMaps produce a deterministic sentinel hash.
func (r *Reconciler) resolveConfigMapHashes(
	ctx context.Context,
	spec map[string]interface{},
	target eav1.TargetResource,
) map[string]string {
	refs := canonicalhash.ExtractConfigMapRefs(spec, target.Kind)
	if len(refs) == 0 {
		return nil
	}

	logger := log.FromContext(ctx)
	configMapHashes := make(map[string]string, len(refs))

	for _, cmName := range refs {
		cm := &corev1.ConfigMap{}
		key := client.ObjectKey{Name: cmName, Namespace: target.Namespace}
		if err := r.apiReader.Get(ctx, key, cm); err != nil {
			// All fetch errors (404, 403, transient) use sentinel to ensure deterministic
			// hash computation: the same set of ConfigMap names always contributes to the
			// composite hash, preventing false-positive drift from intermittent failures.
			sentinelData := map[string]string{"__sentinel__": fmt.Sprintf("__absent:%s__", cmName)}
			sentinelHash, hashErr := canonicalhash.ConfigMapDataHash(sentinelData, nil)
			if hashErr != nil {
				logger.Error(hashErr, "Failed to compute sentinel hash for ConfigMap", "configMap", cmName)
				continue
			}
			configMapHashes[cmName] = sentinelHash
			if apierrors.IsNotFound(err) || apierrors.IsForbidden(err) {
				logger.V(1).Info("ConfigMap not accessible, using sentinel hash",
					"configMap", cmName, "namespace", target.Namespace, "reason", err.Error())
			} else {
				logger.Error(err, "Transient ConfigMap fetch error, using sentinel hash",
					"configMap", cmName, "namespace", target.Namespace)
			}
			continue
		}

		cmHash, err := canonicalhash.ConfigMapDataHash(cm.Data, cm.BinaryData)
		if err != nil {
			logger.Error(err, "Failed to compute hash for ConfigMap, skipping", "configMap", cmName)
			continue
		}
		configMapHashes[cmName] = cmHash
	}

	return configMapHashes
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
		Namespace: ea.Spec.SignalTarget.Namespace,
	}

	// #269: Resolve active pod names from SignalTarget so the scorer can filter
	// out stale alerts for pods deleted during rolling restarts.
	if podNames := r.listActivePodNames(ctx, ea.Spec.SignalTarget); podNames != nil {
		alertCtx.ActivePodNames = podNames
		logger.V(1).Info("Alert pod correlation enabled",
			"signalTarget", ea.Spec.SignalTarget.Name, "activePods", len(podNames))
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

// isAlertDecay detects Prometheus alert decay: the resource is healthy and spec is stable,
// but the alert is still firing due to Prometheus lookback window lag.
// Returns true only when all conditions are met (Issue #369, BR-EM-012):
//   - Health has been assessed with a positive score (resource is healthy)
//   - Hash has been computed (spec is stable, no drift since remediation)
//   - Metrics (if assessed) are not negative — proactive signal gate (#369 Option D)
//   - The alert was just assessed as still firing (score == 0.0)
func (r *Reconciler) isAlertDecay(ea *eav1.EffectivenessAssessment, ar alertAssessResult) bool {
	if !ea.Status.Components.HealthAssessed || ea.Status.Components.HealthScore == nil || *ea.Status.Components.HealthScore <= 0 {
		return false
	}
	if !ea.Status.Components.HashComputed {
		return false
	}
	// Metrics gate (#369 Option D): if metrics have been assessed and show no
	// improvement, the alert is genuine — the proactive/predictive signal proves
	// the remediation failed. Nil MetricsScore (no data) is neutral and does not
	// prevent decay detection.
	if ea.Status.Components.MetricsAssessed && ea.Status.Components.MetricsScore != nil && *ea.Status.Components.MetricsScore <= 0.0 {
		return false
	}
	if !ar.Component.Assessed || ar.Component.Score == nil || *ar.Component.Score != 0.0 {
		return false
	}
	return true
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
	ns := ea.Spec.SignalTarget.Namespace

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

// getTargetHealthStatus queries the K8s API for a target resource's health.
// Kind-aware: uses label-based listing for workload resources (Deployment,
// ReplicaSet, StatefulSet, DaemonSet) and direct pod lookup for Pod targets.
// Non-pod-owning resources (ConfigMap, Secret, Node, etc.) are checked for
// existence only -- they have no pod health to assess.
// DD-EM-003: Health uses RemediationTarget (#275), hash uses RemediationTarget.
func (r *Reconciler) getTargetHealthStatus(ctx context.Context, target eav1.TargetResource, remediationStartedAt *metav1.Time) health.TargetStatus {
	logger := log.FromContext(ctx)

	targetKind := target.Kind
	targetName := target.Name
	targetNs := target.Namespace

	var podList *corev1.PodList

	switch targetKind {
	case "Deployment", "ReplicaSet", "StatefulSet", "DaemonSet":
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
		pod := &corev1.Pod{}
		err := r.Get(ctx, client.ObjectKey{Name: targetName, Namespace: targetNs}, pod)
		if err != nil {
			logger.V(1).Info("Target pod not found", "name", targetName, "error", err)
			return health.TargetStatus{TargetExists: false}
		}
		podList = &corev1.PodList{Items: []corev1.Pod{*pod}}

	default:
		logger.V(1).Info("Target resource kind has no pod health to assess",
			"kind", targetKind, "name", targetName)
		return health.TargetStatus{
			TargetExists:        true,
			HealthNotApplicable: true,
		}
	}

	activePods := FilterActivePods(podList.Items)
	if len(activePods) == 0 {
		return health.TargetStatus{TargetExists: false}
	}

	return ComputePodHealthStats(activePods, remediationStartedAt)
}

// FilterActivePods returns only pods that are active workload members:
// pods that are not terminating (DeletionTimestamp == nil) and not in a
// terminal phase (Succeeded/Failed). Exported for unit testing (#246).
func FilterActivePods(pods []corev1.Pod) []*corev1.Pod {
	active := make([]*corev1.Pod, 0, len(pods))
	for i := range pods {
		pod := &pods[i]
		if pod.DeletionTimestamp != nil {
			continue
		}
		if pod.Status.Phase == corev1.PodSucceeded || pod.Status.Phase == corev1.PodFailed {
			continue
		}
		active = append(active, pod)
	}
	return active
}

// listActivePodNames returns the names of currently running pods for a workload
// target (Deployment, ReplicaSet, StatefulSet, DaemonSet). Returns nil for non-
// workload kinds or when the listing fails (#269: stale alert pod correlation).
func (r *Reconciler) listActivePodNames(ctx context.Context, target eav1.TargetResource) []string {
	switch target.Kind {
	case "Deployment", "ReplicaSet", "StatefulSet", "DaemonSet":
	default:
		return nil
	}

	podList := &corev1.PodList{}
	if err := r.List(ctx, podList,
		client.InNamespace(target.Namespace),
		client.MatchingLabels{"app": target.Name},
	); err != nil {
		log.FromContext(ctx).V(1).Info("Failed to list pods for alert correlation, skipping filter",
			"kind", target.Kind, "name", target.Name, "error", err)
		return nil
	}

	active := FilterActivePods(podList.Items)
	if len(active) == 0 {
		return nil
	}

	names := make([]string, 0, len(active))
	for _, pod := range active {
		names = append(names, pod.Name)
	}
	return names
}

// ComputePodHealthStats aggregates health indicators from a set of active pods.
// remediationStartedAt controls restart counting: pods created before that time
// have their cumulative RestartCount excluded (they predate the remediation).
// Exported for unit testing (#246).
func ComputePodHealthStats(pods []*corev1.Pod, remediationStartedAt *metav1.Time) health.TargetStatus {
	totalReplicas := int32(len(pods))
	readyReplicas := int32(0)
	totalRestarts := int32(0)
	crashLoops := false
	oomKilled := false
	pendingCount := int32(0)

	for _, pod := range pods {
		if pod.Status.Phase == corev1.PodPending {
			pendingCount++
		}

		preRemediationPod := remediationStartedAt != nil && !pod.CreationTimestamp.Time.After(remediationStartedAt.Time)

		for _, cs := range pod.Status.ContainerStatuses {
			if !preRemediationPod {
				totalRestarts += cs.RestartCount
			}
			if cs.Ready {
				readyReplicas++
				break
			}
			if cs.State.Waiting != nil && cs.State.Waiting.Reason == "CrashLoopBackOff" {
				crashLoops = true
			}
			if cs.LastTerminationState.Terminated != nil && cs.LastTerminationState.Terminated.Reason == "OOMKilled" {
				oomKilled = true
			}
		}
	}

	return health.TargetStatus{
		TotalReplicas:            totalReplicas,
		ReadyReplicas:            readyReplicas,
		RestartsSinceRemediation: totalRestarts,
		TargetExists:            true,
		CrashLoops:              crashLoops,
		OOMKilled:               oomKilled,
		PendingCount:            pendingCount,
	}
}

// getTargetSpec retrieves a target resource's .spec as an unstructured map from the K8s API.
// Uses the REST mapper to resolve the Kind to a GVR, then fetches via unstructured client.
//
// Returns (spec, degradedReason) where:
//   - (specMap, "") on success
//   - (emptyMap, "") when not applicable: NotFound, no .spec, unknown GVK, nil RESTMapper
//   - (emptyMap, "reason") when degraded: Forbidden, transient API errors (Issue #546)
//
// DD-EM-003: Caller decides which target to pass (RemediationTarget for hash, etc.).
func (r *Reconciler) getTargetSpec(ctx context.Context, target eav1.TargetResource) (map[string]interface{}, string) {
	logger := log.FromContext(ctx)

	if r.restMapper == nil {
		logger.V(1).Info("RESTMapper not configured, falling back to metadata spec")
		return map[string]interface{}{
			"kind":      target.Kind,
			"name":      target.Name,
			"namespace": target.Namespace,
		}, ""
	}

	gvk, err := k8sutil.ResolveGVKForKind(r.restMapper, target.Kind)
	if err != nil {
		logger.Error(err, "Failed to resolve GVK for target resource kind",
			"kind", target.Kind)
		return map[string]interface{}{}, ""
	}

	obj := &unstructured.Unstructured{}
	obj.SetGroupVersionKind(gvk)
	key := client.ObjectKey{
		Namespace: target.Namespace,
		Name:      target.Name,
	}
	if err := r.Get(ctx, key, obj); err != nil {
		if apierrors.IsNotFound(err) {
			logger.Info("Target resource not found, computing hash from empty spec",
				"kind", target.Kind,
				"name", target.Name)
			return map[string]interface{}{}, ""
		}
		logger.Error(err, "Failed to fetch target resource")
		return map[string]interface{}{}, fmt.Sprintf("failed to fetch target resource %s/%s: %v", target.Kind, target.Name, err)
	}

	spec, found, err := unstructured.NestedMap(obj.Object, "spec")
	if err != nil || !found {
		logger.V(1).Info("Target resource has no .spec field",
			"kind", target.Kind,
			"name", target.Name)
		return map[string]interface{}{}, ""
	}

	logger.V(2).Info("Target spec retrieved",
		"kind", target.Kind,
		"name", target.Name)
	return spec, ""
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


// emitHashEvent emits K8s and audit events for the hash computation result.
// Uses the specialized RecordHashComputed to include pre/post hashes and match flag.
func (r *Reconciler) emitHashEvent(ctx context.Context, ea *eav1.EffectivenessAssessment, result hash.ComputeResult) {
	// K8s Event (DD-EVENT-001)
	if result.Component.Error != nil {
		r.Recorder.Event(ea, corev1.EventTypeWarning, events.EventReasonComponentAssessed,
			fmt.Sprintf("Component hash assessment failed: %v", result.Component.Error))
	} else {
		r.Recorder.Event(ea, corev1.EventTypeNormal, events.EventReasonComponentAssessed,
			fmt.Sprintf("Component hash computed (match: %v)", result.Match))
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
	}
}

// completeAssessment finalizes the EA with Completed phase and assessment reason.
// It performs a single atomic status update and emits completion events.
// Used by both the normal completion path (Step 8-9) and the expired path (Step 4).
func (r *Reconciler) completeAssessment(ctx context.Context, ea *eav1.EffectivenessAssessment) (ctrl.Result, error) {
	logger := log.FromContext(ctx)

	reason := r.determineAssessmentReason(ea)
	r.setCompletionFields(ea, reason)

	if err := r.Status().Update(ctx, ea); err != nil {
		logger.Error(err, "Failed to update EA to Completed",
			"reason", reason, "resourceVersion", ea.ResourceVersion)
		return ctrl.Result{RequeueAfter: r.Config.RequeueGenericError}, err
	}

	r.emitCompletionMetricsAndEvents(ctx, ea, reason)

	logger.Info("Assessment completed",
		"reason", reason,
		"correlationID", ea.Spec.CorrelationID,
	)

	return ctrl.Result{}, nil
}

// failAssessment transitions the EA to PhaseFailed for unrecoverable conditions (#573).
// Unlike completeAssessment (which uses PhaseCompleted), this sets PhaseFailed and
// reason "unrecoverable". The RO handles PhaseFailed in trackEffectivenessStatus.
func (r *Reconciler) failAssessment(ctx context.Context, ea *eav1.EffectivenessAssessment, reason string) (ctrl.Result, error) {
	logger := log.FromContext(ctx)

	now := metav1.Now()
	ea.Status.Phase = eav1.PhaseFailed
	ea.Status.CompletedAt = &now
	ea.Status.AssessmentReason = "unrecoverable"
	ea.Status.Message = fmt.Sprintf("Assessment failed: %s", reason)

	conditions.SetCondition(ea, conditions.ConditionAssessmentComplete, metav1.ConditionFalse, "ValidationFailed", ea.Status.Message)

	if err := r.Status().Update(ctx, ea); err != nil {
		logger.Error(err, "Failed to update EA to Failed",
			"reason", reason, "resourceVersion", ea.ResourceVersion)
		return ctrl.Result{RequeueAfter: r.Config.RequeueGenericError}, err
	}

	r.Recorder.Event(ea, corev1.EventTypeWarning, "AssessmentFailed", ea.Status.Message)
	logger.Info("Assessment failed due to unrecoverable condition",
		"reason", reason,
		"correlationID", ea.Spec.CorrelationID,
	)

	return ctrl.Result{}, nil
}

// completeAssessmentWithReason finalizes the EA with an explicit assessment reason.
// Unlike completeAssessment, which computes the reason from component state, this
// method uses the provided reason directly. Used by the spec drift guard (DD-EM-002 v1.1).
func (r *Reconciler) completeAssessmentWithReason(ctx context.Context, ea *eav1.EffectivenessAssessment, reason string) (ctrl.Result, error) {
	logger := log.FromContext(ctx)

	r.setCompletionFields(ea, reason)

	if err := r.Status().Update(ctx, ea); err != nil {
		logger.Error(err, "Failed to update EA to Completed",
			"reason", reason, "resourceVersion", ea.ResourceVersion)
		return ctrl.Result{RequeueAfter: r.Config.RequeueGenericError}, err
	}

	r.emitCompletionMetricsAndEvents(ctx, ea, reason)

	logger.Info("Assessment completed",
		"reason", reason,
		"correlationID", ea.Spec.CorrelationID,
	)

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

	// BR-EM-012: If the EA was in active decay monitoring (AlertDecayRetries > 0),
	// resolve the AlertDecayDetected condition on any terminal transition.
	// This covers early-termination paths (spec_drift, no_execution) that bypass
	// the alert check where Point B would normally resolve the condition.
	if ea.Status.Components.AlertDecayRetries > 0 {
		decayReason := conditions.ReasonDecayResolved
		decayMsg := "Alert decay monitoring ended: assessment completed"
		if reason == eav1.AssessmentReasonAlertDecayTimeout {
			decayReason = conditions.ReasonDecayTimeout
			decayMsg = "Alert decay monitoring ended: validity window expired before alert resolved"
		}
		conditions.SetCondition(ea, conditions.ConditionAlertDecayDetected,
			metav1.ConditionFalse, decayReason, decayMsg)
	}
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
	case eav1.AssessmentReasonAlertDecayTimeout:
		return conditions.ReasonAlertDecayTimeout
	default:
		return reason // Fallback: use the reason string directly
	}
}

// emitCompletionMetricsAndEvents records metrics and emits K8s + audit events for completion.
// Extracted to share between the normal completion path and the expired path.
func (r *Reconciler) emitCompletionMetricsAndEvents(ctx context.Context, ea *eav1.EffectivenessAssessment, reason string) {
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
		// Issue #369, BR-EM-012: Distinguish alert_decay_timeout from generic partial.
		// alert_decay_timeout: The EM was actively monitoring alert decay (retries > 0)
		// but the alert never resolved before validity expired. This is distinct from
		// "partial" (alert never checked) — the EM confirmed health+hash and re-checked
		// the alert multiple times.
		if components.AlertDecayRetries > 0 && !components.AlertAssessed {
			return eav1.AssessmentReasonAlertDecayTimeout
		}

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
}

// emitK8sComponentEvent emits a K8s event for a component assessment result.
// K8s event uses DD-EVENT-001 simplified approach: single EventReasonComponentAssessed with component name in message.
func (r *Reconciler) emitK8sComponentEvent(ea *eav1.EffectivenessAssessment, component string, result emtypes.ComponentResult) {
	if result.Error != nil {
		r.Recorder.Event(ea, corev1.EventTypeWarning, events.EventReasonComponentAssessed,
			fmt.Sprintf("Component %s assessment failed: %v", component, result.Error))
	} else {
		msg := fmt.Sprintf("Component %s assessed", component)
		if result.Score != nil {
			msg = fmt.Sprintf("Component %s assessed (score: %.2f)", component, *result.Score)
		}
		r.Recorder.Event(ea, corev1.EventTypeNormal, events.EventReasonComponentAssessed, msg)
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

// emitAlertDecayEvent emits a one-time audit event when alert decay is first detected.
// Called only when AlertDecayRetries == 0 (before increment). Follows the metrics
// component precedent: silent retries after the first detection (Issue #369, BR-EM-012).
func (r *Reconciler) emitAlertDecayEvent(ctx context.Context, ea *eav1.EffectivenessAssessment, ar alertAssessResult) {
	r.emitK8sComponentEvent(ea, "alert_decay", emtypes.ComponentResult{
		Component: emtypes.ComponentAlertDecay,
		Assessed:  false,
		Score:     ar.Component.Score,
		Details:   "Alert decay detected: resource healthy but alert still firing",
	})

	var healthScore float64
	if ea.Status.Components.HealthScore != nil {
		healthScore = *ea.Status.Components.HealthScore
	}
	var alertScore float64
	if ar.Component.Score != nil {
		alertScore = *ar.Component.Score
	}

	r.emitAuditEvent(ctx, emtypes.AuditAlertDecayDetected, func() error {
		return r.AuditManager.RecordAlertDecayDetected(ctx, ea, emaudit.AlertDecayDetectedData{
			HealthScore: healthScore,
			AlertScore:  alertScore,
			RetryCount:  ea.Status.Components.AlertDecayRetries + 1,
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

// emitScheduledEventIfFirst emits the assessment.scheduled audit event and K8s event
// when ValidityDeadline is first set (WFP or Stabilizing transition).
// Called exactly once per EA lifecycle at the point where ValidityDeadline is persisted
// (#573, ADR-EM-001 §9.2.0). NOT called from the Assessing transition to avoid duplicates.
func (r *Reconciler) emitScheduledEventIfFirst(ctx context.Context, ea *eav1.EffectivenessAssessment) {
	r.emitAuditEvent(ctx, emtypes.AuditAssessmentScheduled, func() error {
		return r.AuditManager.RecordAssessmentScheduled(ctx, ea, r.Config.ValidityWindow)
	})

	r.Recorder.Event(ea, corev1.EventTypeNormal, "AssessmentScheduled",
		fmt.Sprintf("Assessment scheduled for correlation %s (deadline: %s)",
			ea.Spec.CorrelationID, ea.Status.ValidityDeadline.Format("15:04:05")))
}

// emitAssessingTransitionEvents emits the AssessmentStarted K8s event for the
// transition to Assessing phase. Called after the status update succeeds.
// The scheduled event is NOT emitted here — it was already emitted at the WFP or
// Stabilizing entry point where ValidityDeadline was first persisted.
func (r *Reconciler) emitAssessingTransitionEvents(ctx context.Context, ea *eav1.EffectivenessAssessment) {
	r.Recorder.Event(ea, corev1.EventTypeNormal, events.EventReasonAssessmentStarted,
		fmt.Sprintf("Assessment started for correlation %s", ea.Spec.CorrelationID))
}

// emitCompletedAuditEvent emits the assessment.completed audit event to DataStorage.
func (r *Reconciler) emitCompletedAuditEvent(ctx context.Context, ea *eav1.EffectivenessAssessment, reason string) {
	r.emitAuditEvent(ctx, emtypes.AuditAssessmentCompleted, func() error {
		return r.AuditManager.RecordAssessmentCompleted(ctx, ea, reason)
	})
}

// emitAuditEvent is a helper that handles the nil-check and error logging common to
// all audit event emissions. The recordFn performs the actual audit call.
func (r *Reconciler) emitAuditEvent(ctx context.Context, eventType emtypes.AuditEventType, recordFn func() error) {
	if r.AuditManager == nil {
		log.FromContext(ctx).V(1).Info("AuditManager not configured, skipping audit event",
			"eventType", string(eventType))
		return
	}

	if err := recordFn(); err != nil {
		log.FromContext(ctx).Error(err, "Failed to emit audit event", "eventType", string(eventType))
		return
	}
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
