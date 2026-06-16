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

	var steps []VerificationStep
	prevC := eav1alpha1.EAComponents{}
	prevPhase := ""
	if prev != nil {
		prevC = prev.Status.Components
		prevPhase = prev.Status.Phase
	}
	currC := curr.Status.Components
	currPhase := curr.Status.Phase

	if prevPhase != currPhase && currPhase != "" {
		if currPhase == eav1alpha1.PhaseAssessing &&
			(prevPhase == eav1alpha1.PhaseStabilizing || prevPhase == "" || prevPhase == eav1alpha1.PhasePending) {
			steps = append(steps, VerificationStep{
				Step:    "stabilization_elapsed",
				Message: "Stabilization window elapsed",
				Data: map[string]any{
					"step_status": StepStatusCompleted,
					"detail":      "Stabilization window elapsed",
				},
			})
		} else {
			status := StepStatusCompleted
			if currPhase == eav1alpha1.PhaseFailed {
				status = StepStatusFailed
			}
			step := VerificationStep{
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
			steps = append(steps, step)
		}
	}

	if !prevC.HashComputed && currC.HashComputed {
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
		steps = append(steps, VerificationStep{
			Step:    "spec_hash_computed",
			Message: detail,
			Data:    data,
		})
	}

	// Alert decay retries → alert_check in_progress (only when not yet completed).
	// Placed before the completed check so that in the rare case both fire in the
	// same diff (AlertDecayRetries changed AND AlertAssessed became true), only
	// the completed event is emitted (the !currC.AlertAssessed guard filters it).
	if prevC.AlertDecayRetries != currC.AlertDecayRetries && currC.AlertDecayRetries > 0 && !currC.AlertAssessed {
		detail := fmt.Sprintf("Waiting for alert to clear (retry %d)", currC.AlertDecayRetries)
		if curr.Spec.SignalName != "" {
			detail = fmt.Sprintf("Waiting for %s to clear (retry %d)", curr.Spec.SignalName, currC.AlertDecayRetries)
		}
		steps = append(steps, VerificationStep{
			Step:    "alert_check",
			Message: detail,
			Data: map[string]any{
				"step_status": StepStatusInProgress,
				"detail":      detail,
				"retry_count": currC.AlertDecayRetries,
			},
		})
	}

	if !prevC.AlertAssessed && currC.AlertAssessed {
		detail := "Alert resolution check completed"
		data := map[string]any{
			"step_status": StepStatusCompleted,
			"detail":      detail,
		}
		if currC.AlertScore != nil {
			data["score"] = *currC.AlertScore
		}
		steps = append(steps, VerificationStep{
			Step:    "alert_check",
			Message: detail,
			Data:    data,
		})
	}

	if !prevC.HealthAssessed && currC.HealthAssessed {
		detail := "Health check completed"
		data := map[string]any{
			"step_status": StepStatusCompleted,
			"detail":      detail,
		}
		if currC.HealthScore != nil {
			data["score"] = *currC.HealthScore
		}
		steps = append(steps, VerificationStep{
			Step:    "health_check",
			Message: detail,
			Data:    data,
		})
	}

	if !prevC.MetricsAssessed && currC.MetricsAssessed {
		detail := "Metrics comparison completed"
		data := map[string]any{
			"step_status": StepStatusCompleted,
			"detail":      detail,
		}
		if currC.MetricsScore != nil {
			data["score"] = *currC.MetricsScore
		}
		steps = append(steps, VerificationStep{
			Step:    "metrics_check",
			Message: detail,
			Data:    data,
		})
	}

	return steps
}
