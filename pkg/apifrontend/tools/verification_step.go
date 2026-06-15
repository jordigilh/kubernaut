package tools

import (
	"fmt"

	eav1alpha1 "github.com/jordigilh/kubernaut/api/effectivenessassessment/v1alpha1"
)

// VerificationStep represents a granular verification event emitted during
// the Verifying phase. Each step corresponds to a state change in the
// EffectivenessAssessment CRD's status fields (#1427).
type VerificationStep struct {
	Step    string         `json:"step"`
	Message string         `json:"message"`
	Data    map[string]any `json:"data,omitempty"`
}

// DiffEASteps compares two EffectivenessAssessment snapshots (prev and curr)
// and returns the verification steps that transitioned between them.
// prev may be nil for the first observation (all non-zero fields are reported).
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
		step := VerificationStep{
			Step:    "phase_transition",
			Message: fmt.Sprintf("EA phase: %s", currPhase),
			Data:    map[string]any{"phase": currPhase},
		}
		if prevPhase != "" {
			step.Data["previous_phase"] = prevPhase
		}
		steps = append(steps, step)
	}

	if !prevC.HashComputed && currC.HashComputed {
		data := map[string]any{"completed": true}
		if currC.PostRemediationSpecHash != "" {
			data["post_remediation_spec_hash"] = currC.PostRemediationSpecHash
		}
		steps = append(steps, VerificationStep{
			Step:    "spec_hash_computed",
			Message: "Spec hash comparison completed",
			Data:    data,
		})
	}

	if !prevC.AlertAssessed && currC.AlertAssessed {
		data := map[string]any{"completed": true}
		if currC.AlertScore != nil {
			data["score"] = *currC.AlertScore
		}
		steps = append(steps, VerificationStep{
			Step:    "alert_check",
			Message: "Alert resolution check completed",
			Data:    data,
		})
	}

	if prevC.AlertDecayRetries != currC.AlertDecayRetries && currC.AlertDecayRetries > 0 {
		steps = append(steps, VerificationStep{
			Step:    "alert_decay_retry",
			Message: fmt.Sprintf("Alert decay retry %d", currC.AlertDecayRetries),
			Data:    map[string]any{"retry_count": currC.AlertDecayRetries},
		})
	}

	if !prevC.HealthAssessed && currC.HealthAssessed {
		data := map[string]any{"completed": true}
		if currC.HealthScore != nil {
			data["score"] = *currC.HealthScore
		}
		steps = append(steps, VerificationStep{
			Step:    "health_check",
			Message: "Health check completed",
			Data:    data,
		})
	}

	if !prevC.MetricsAssessed && currC.MetricsAssessed {
		data := map[string]any{"completed": true}
		if currC.MetricsScore != nil {
			data["score"] = *currC.MetricsScore
		}
		steps = append(steps, VerificationStep{
			Step:    "metrics_check",
			Message: "Metrics comparison completed",
			Data:    data,
		})
	}

	return steps
}
