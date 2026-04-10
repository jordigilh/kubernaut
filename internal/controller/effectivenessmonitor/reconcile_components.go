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
	"time"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/log"

	eav1 "github.com/jordigilh/kubernaut/api/effectivenessassessment/v1alpha1"
	"github.com/jordigilh/kubernaut/pkg/effectivenessmonitor/alert"
	"github.com/jordigilh/kubernaut/pkg/effectivenessmonitor/conditions"
	"github.com/jordigilh/kubernaut/pkg/effectivenessmonitor/hash"
)

// runComponentPipeline executes Steps 6b-7b: scope determination, hash, health,
// partial-execution early completion, alert, metrics checks, and alert deferral requeue.
// Returns (result, true, err) when the pipeline terminates early.
func (r *Reconciler) runComponentPipeline(ctx context.Context, rctx *reconcileContext) (ctrl.Result, bool, error) {
	logger := log.FromContext(ctx)
	ea := rctx.ea

	// Step 6b: Assessment scope determination (ADR-EM-001 §5, #573 G4)
	rctx.scope = r.determineAssessmentScope(ctx, ea.Spec.CorrelationID)
	if rctx.scope == scopeNoExecution {
		logger.Info("No workflow execution started for this correlation ID — completing as no_execution",
			"correlationID", ea.Spec.CorrelationID)
		result, err := r.completeAssessmentWithReason(ctx, ea, eav1.AssessmentReasonNoExecution)
		return result, true, err
	}
	if rctx.scope == scopePartial {
		logger.Info("Workflow started but not completed — narrowing to health+hash assessment (ADR-EM-001 §5)",
			"correlationID", ea.Spec.CorrelationID)
	}

	// Step 7: Hash check — Two-Phase Model (BR-EM-004, DD-EM-002 v2.1)
	if result, done, err := r.runHashCheck(ctx, rctx); done {
		return result, true, err
	}

	// Health check (BR-EM-001)
	r.runHealthCheck(ctx, rctx)

	// Step 7a: Partial-execution early completion (ADR-EM-001 §5, #573 G4)
	if result, done, err := r.handlePartialScope(ctx, rctx); done {
		return result, true, err
	}

	// Alert check (BR-EM-002)
	r.runAlertCheck(ctx, rctx)

	// Metrics check (BR-EM-003)
	r.runMetricsCheck(ctx, rctx)

	// Step 7b: Precise requeue when only alert is deferred (#277)
	if result, done, err := r.handleAlertDeferralRequeue(ctx, rctx); done {
		return result, true, err
	}

	return ctrl.Result{}, false, nil
}

// runHashCheck executes the hash component check (Phase 1 of two-phase model).
func (r *Reconciler) runHashCheck(ctx context.Context, rctx *reconcileContext) (ctrl.Result, bool, error) {
	ea := rctx.ea
	if ea.Status.Components.HashComputed {
		return ctrl.Result{}, false, nil
	}

	logger := log.FromContext(ctx)

	// DD-EM-004, #277: Defer hash for async-managed targets.
	deferral := hash.CheckHashDeferral(ea)
	if deferral.ShouldDefer {
		logger.V(1).Info("Hash computation deferred for async-managed target",
			"hashComputeDelay", ea.Spec.Config.HashComputeDelay.Duration,
			"remaining", deferral.RequeueAfter)
		return ctrl.Result{RequeueAfter: r.capRequeueAtDeadline(ea, deferral.RequeueAfter)}, true, nil
	}

	result := r.assessHash(ctx, ea)
	ea.Status.Components.HashComputed = result.Component.Assessed
	ea.Status.Components.PostRemediationSpecHash = result.Hash
	ea.Status.Components.CurrentSpecHash = result.Hash
	rctx.componentsChanged = true
	r.Metrics.RecordComponentAssessment("hash", resultStatus(result.Component), nil)
	r.emitHashEvent(ctx, ea, result)

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

	if result.Component.Assessed {
		conditions.SetCondition(ea, conditions.ConditionSpecIntegrity,
			metav1.ConditionTrue, conditions.ReasonSpecUnchanged,
			"Post-remediation spec hash computed; baseline established")
	}

	return ctrl.Result{}, false, nil
}

// runHealthCheck executes the health component check.
func (r *Reconciler) runHealthCheck(ctx context.Context, rctx *reconcileContext) {
	ea := rctx.ea
	if ea.Status.Components.HealthAssessed {
		return
	}

	healthResult := r.assessHealth(ctx, ea)
	ea.Status.Components.HealthAssessed = healthResult.Component.Assessed
	ea.Status.Components.HealthScore = healthResult.Component.Score
	rctx.componentsChanged = true
	r.Metrics.RecordComponentAssessment("health", resultStatus(healthResult.Component), healthResult.Component.Score)

	if healthResult.Component.Score != nil && ea.Status.Components.AlertDecayRetries == 0 {
		r.emitHealthEvent(ctx, ea, healthResult)
	}
}

// handlePartialScope handles Step 7a: partial-execution early completion.
func (r *Reconciler) handlePartialScope(ctx context.Context, rctx *reconcileContext) (ctrl.Result, bool, error) {
	ea := rctx.ea
	if rctx.scope != scopePartial || !ea.Status.Components.HealthAssessed || !ea.Status.Components.HashComputed {
		return ctrl.Result{}, false, nil
	}

	logger := log.FromContext(ctx)
	const partialGracePeriod = 30 * time.Second
	age := time.Since(ea.CreationTimestamp.Time)
	if age < partialGracePeriod {
		logger.Info("Partial scope within grace period — requeueing to re-evaluate after workflow.completed may arrive",
			"correlationID", ea.Spec.CorrelationID,
			"eaAge", age.Round(time.Second),
			"gracePeriod", partialGracePeriod)
		return ctrl.Result{RequeueAfter: 5 * time.Second}, true, nil
	}

	result, err := r.completeAssessmentWithReason(ctx, ea, eav1.AssessmentReasonPartial)
	return result, true, err
}

// runAlertCheck executes the alert component check.
func (r *Reconciler) runAlertCheck(ctx context.Context, rctx *reconcileContext) {
	ea := rctx.ea
	if ea.Status.Components.AlertAssessed {
		return
	}

	logger := log.FromContext(ctx)

	if r.Config.AlertManagerEnabled && r.AlertManagerClient != nil {
		rctx.alertDeferred = alert.CheckAlertDeferral(ea)
		if rctx.alertDeferred.ShouldDefer {
			logger.V(1).Info("Alert check deferred (proactive signal, #277)",
				"alertManagerCheckAfter", ea.Status.AlertManagerCheckAfter,
				"remaining", rctx.alertDeferred.RequeueAfter)
			r.Metrics.RecordComponentAssessment("alert", "deferred", nil)
		} else {
			alertResult := r.assessAlert(ctx, ea)

			if r.isAlertDecay(ea, alertResult) {
				if ea.Status.Components.AlertDecayRetries == 0 {
					r.emitAlertDecayEvent(ctx, ea, alertResult)
				}
				ea.Status.Components.AlertDecayRetries++
				alertResult.Component.Assessed = false
				ea.Status.Components.HealthAssessed = false

				conditions.SetCondition(ea, conditions.ConditionAlertDecayDetected,
					metav1.ConditionTrue, conditions.ReasonDecayActive,
					fmt.Sprintf("Alert decay suspected: health=%.1f, alert still firing, retries=%d",
						*ea.Status.Components.HealthScore, ea.Status.Components.AlertDecayRetries))

				logger.Info("Alert decay suspected: deferring alert assessment, scheduling health re-probe",
					"healthScore", ea.Status.Components.HealthScore,
					"alertScore", alertResult.Component.Score,
					"retries", ea.Status.Components.AlertDecayRetries)
			} else {
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
			rctx.componentsChanged = true
		}
	} else {
		ea.Status.Components.AlertAssessed = true
		r.Metrics.RecordComponentAssessment("alert", "skipped", nil)
		rctx.componentsChanged = true
	}
}

// runMetricsCheck executes the metrics component check.
func (r *Reconciler) runMetricsCheck(ctx context.Context, rctx *reconcileContext) {
	ea := rctx.ea
	if ea.Status.Components.MetricsAssessed {
		return
	}

	if r.Config.PrometheusEnabled && r.PrometheusClient != nil {
		metricsResult := r.assessMetrics(ctx, ea)
		ea.Status.Components.MetricsAssessed = metricsResult.Component.Assessed
		ea.Status.Components.MetricsScore = metricsResult.Component.Score
		r.Metrics.RecordComponentAssessment("metrics", resultStatus(metricsResult.Component), metricsResult.Component.Score)
		if metricsResult.Component.Assessed {
			r.emitMetricsEvent(ctx, ea, metricsResult)
		}
	} else {
		ea.Status.Components.MetricsAssessed = true
		r.Metrics.RecordComponentAssessment("metrics", "skipped", nil)
	}
	rctx.componentsChanged = true
}

// handleAlertDeferralRequeue handles Step 7b: precise requeue when only alert is deferred.
func (r *Reconciler) handleAlertDeferralRequeue(ctx context.Context, rctx *reconcileContext) (ctrl.Result, bool, error) {
	ea := rctx.ea

	if !rctx.alertDeferred.ShouldDefer {
		return ctrl.Result{}, false, nil
	}
	if !ea.Status.Components.HealthAssessed || !ea.Status.Components.HashComputed {
		return ctrl.Result{}, false, nil
	}
	if !ea.Status.Components.MetricsAssessed && r.Config.PrometheusEnabled {
		return ctrl.Result{}, false, nil
	}

	logger := log.FromContext(ctx)

	if rctx.componentsChanged || rctx.pendingTransition {
		if err := r.Status().Update(ctx, ea); err != nil {
			logger.Error(err, "Failed to persist status before alert deferral requeue")
			return ctrl.Result{RequeueAfter: r.Config.RequeueGenericError}, true, err
		}
	}
	capped := r.capRequeueAtDeadline(ea, rctx.alertDeferred.RequeueAfter)
	logger.Info("All components except alert done; precise requeue for alert deferral (#277)",
		"requeueAfter", capped)
	return ctrl.Result{RequeueAfter: capped}, true, nil
}
