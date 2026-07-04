package tools

import (
	"fmt"

	eav1alpha1 "github.com/jordigilh/kubernaut/api/effectivenessassessment/v1alpha1"
)

// Step status constants per issue #1427 Console contract.
const (
	StepStatusCompleted  = "completed"
	StepStatusInProgress = "in_progress"
	StepStatusFailed     = "failed"
)

// VerificationStep represents a granular verification event emitted during
// the Verifying phase. Each step corresponds to a state change in the
// EffectivenessAssessment CRD's status fields (#1427).
//
// Data always contains step_status (completed|in_progress|failed) and detail
// (human-readable context). Additional keys are step-specific.
type VerificationStep struct {
	Step    string         `json:"step"`
	Message string         `json:"message"`
	Data    map[string]any `json:"data,omitempty"`
}

// DiffEASteps compares two EffectivenessAssessment snapshots (prev and curr)
// and returns the verification steps that transitioned between them.
// prev may be nil for the first observation (all non-zero fields are reported).
//
// Step names and Data keys follow the #1427 Console contract:
//   - stabilization_elapsed: emitted when Stabilizing/Pending -> Assessing
//   - spec_hash_computed:    emitted when HashComputed becomes true
//   - alert_check:           emitted as in_progress (decay retry) or completed
//   - health_check:          emitted when HealthAssessed becomes true
//   - metrics_check:         emitted when MetricsAssessed becomes true
//   - phase_transition:      emitted for other EA phase changes (e.g. -> Failed)
func DiffEASteps(prev, curr *eav1alpha1.EffectivenessAssessment) []VerificationStep {
	if curr == nil {
		return nil
	}

	prevC := eav1alpha1.EAComponents{}
	prevPhase := ""
	if prev != nil {
		prevC = prev.Status.Components
		prevPhase = prev.Status.Phase
	}
	currC := curr.Status.Components
	currPhase := curr.Status.Phase

	var steps []VerificationStep
	appendIfPresent := func(step *VerificationStep) {
		if step != nil {
			steps = append(steps, *step)
		}
	}

	appendIfPresent(diffPhaseTransitionStep(prevPhase, currPhase))
	appendIfPresent(diffSpecHashStep(prevC, currC))
	appendIfPresent(diffAlertCheckInProgressStep(prevC, currC, curr.Spec.SignalName))
	appendIfPresent(diffAlertCheckCompletedStep(prevC, currC))
	appendIfPresent(diffHealthCheckStep(prevC, currC))
	appendIfPresent(diffMetricsCheckStep(prevC, currC))

	return steps
}

// diffPhaseTransitionStep reports the EA phase transition between prevPhase
// and currPhase, if any. The Stabilizing/Pending -> Assessing transition gets
// its own dedicated "stabilization_elapsed" step name (Console contract);
// all other transitions are reported as a generic "phase_transition" step.
func diffPhaseTransitionStep(prevPhase, currPhase string) *VerificationStep {
	if prevPhase == currPhase || currPhase == "" {
		return nil
	}
	if currPhase == eav1alpha1.PhaseAssessing &&
		(prevPhase == eav1alpha1.PhaseStabilizing || prevPhase == "" || prevPhase == eav1alpha1.PhasePending) {
		return &VerificationStep{
			Step:    "stabilization_elapsed",
			Message: "Stabilization window elapsed",
			Data: map[string]any{
				"step_status": StepStatusCompleted,
				"detail":      "Stabilization window elapsed",
			},
		}
	}

	status := StepStatusCompleted
	if currPhase == eav1alpha1.PhaseFailed {
		status = StepStatusFailed
	}
	step := &VerificationStep{
		Step:    "phase_transition",
		Message: fmt.Sprintf("EA phase: %s", currPhase),
		Data: map[string]any{
			"phase":       currPhase,
			"step_status": status,
			"detail":      fmt.Sprintf("EA phase: %s", currPhase),
		},
	}
	if prevPhase != "" {
		step.Data["previous_phase"] = prevPhase
	}
	return step
}

// diffSpecHashStep reports when the post-remediation spec hash was just
// computed (HashComputed transitioned false -> true).
func diffSpecHashStep(prevC, currC eav1alpha1.EAComponents) *VerificationStep {
	if prevC.HashComputed || !currC.HashComputed {
		return nil
	}
	detail := "Spec hash comparison completed"
	if currC.PostRemediationSpecHash != "" {
		short := currC.PostRemediationSpecHash
		if len(short) > 12 {
			short = short[:12]
		}
		detail = fmt.Sprintf("Spec hash computed (%s)", short)
	}
	data := map[string]any{
		"step_status": StepStatusCompleted,
		"detail":      detail,
	}
	if currC.PostRemediationSpecHash != "" {
		data["post_remediation_spec_hash"] = currC.PostRemediationSpecHash
	}
	return &VerificationStep{Step: "spec_hash_computed", Message: detail, Data: data}
}

// diffAlertCheckInProgressStep reports an alert-decay retry as an in-progress
// alert_check step. Only emitted when not yet completed (!currC.AlertAssessed)
// so that in the rare case both this and diffAlertCheckCompletedStep would
// fire in the same diff, only the completed event is emitted.
func diffAlertCheckInProgressStep(prevC, currC eav1alpha1.EAComponents, signalName string) *VerificationStep {
	if prevC.AlertDecayRetries == currC.AlertDecayRetries || currC.AlertDecayRetries <= 0 || currC.AlertAssessed {
		return nil
	}
	detail := fmt.Sprintf("Waiting for alert to clear (retry %d)", currC.AlertDecayRetries)
	if signalName != "" {
		detail = fmt.Sprintf("Waiting for %s to clear (retry %d)", signalName, currC.AlertDecayRetries)
	}
	return &VerificationStep{
		Step:    "alert_check",
		Message: detail,
		Data: map[string]any{
			"step_status": StepStatusInProgress,
			"detail":      detail,
			"retry_count": currC.AlertDecayRetries,
		},
	}
}

// diffAlertCheckCompletedStep reports alert resolution check completion
// (AlertAssessed transitioned false -> true).
func diffAlertCheckCompletedStep(prevC, currC eav1alpha1.EAComponents) *VerificationStep {
	if prevC.AlertAssessed || !currC.AlertAssessed {
		return nil
	}
	detail := "Alert resolution check completed"
	data := map[string]any{"step_status": StepStatusCompleted, "detail": detail}
	if currC.AlertScore != nil {
		data["score"] = *currC.AlertScore
	}
	return &VerificationStep{Step: "alert_check", Message: detail, Data: data}
}

// diffHealthCheckStep reports health check completion (HealthAssessed
// transitioned false -> true).
func diffHealthCheckStep(prevC, currC eav1alpha1.EAComponents) *VerificationStep {
	if prevC.HealthAssessed || !currC.HealthAssessed {
		return nil
	}
	detail := "Health check completed"
	data := map[string]any{"step_status": StepStatusCompleted, "detail": detail}
	if currC.HealthScore != nil {
		data["score"] = *currC.HealthScore
	}
	return &VerificationStep{Step: "health_check", Message: detail, Data: data}
}

// diffMetricsCheckStep reports metrics comparison completion (MetricsAssessed
// transitioned false -> true).
func diffMetricsCheckStep(prevC, currC eav1alpha1.EAComponents) *VerificationStep {
	if prevC.MetricsAssessed || !currC.MetricsAssessed {
		return nil
	}
	detail := "Metrics comparison completed"
	data := map[string]any{"step_status": StepStatusCompleted, "detail": detail}
	if currC.MetricsScore != nil {
		data["score"] = *currC.MetricsScore
	}
	return &VerificationStep{Step: "metrics_check", Message: detail, Data: data}
}
