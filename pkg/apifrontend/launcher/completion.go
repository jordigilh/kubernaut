package launcher

import (
	"fmt"

	"github.com/jordigilh/kubernaut/pkg/apifrontend/ka"
)

const (
	recoverySource         = "af_completion_recovery"
	recoveryReason         = "missing_present_decision"
	missingDataFailure     = "missing_authoritative_data"
	missingDataExplanation = "Authoritative investigation details were unavailable."
)

// DecisionRecoveryInput contains only server-observed data used to recover a
// missing presentation artifact. It deliberately excludes model-authored
// presentation fields so recovery cannot amplify untrusted narration.
type DecisionRecoveryInput struct {
	SessionID          string
	RRID               string
	RCA                map[string]any
	Summary            string
	SummaryProvisional bool
	Discovery          *ka.DiscoverWorkflowsResult
}

// DecisionRecoveryResult is the schema-shaped artifact payload and its
// human-readable fallback. Complete is false when the payload is a truthful
// failure outcome rather than a recoverable decision.
type DecisionRecoveryResult struct {
	Data         map[string]any
	TextFallback string
	Metadata     map[string]any
	Complete     bool
}

// BuildRecoveredDecisionArtifact builds a presentation-only artifact from
// authoritative RCA and workflow discovery data. It never selects a workflow
// or changes execution authorization.
func BuildRecoveredDecisionArtifact(input DecisionRecoveryInput) (DecisionRecoveryResult, error) {
	if input.SessionID == "" {
		return DecisionRecoveryResult{}, fmt.Errorf("session_id is required for decision recovery")
	}

	if input.Discovery == nil {
		return incompleteDecisionArtifact(input), nil
	}

	explanation := ""
	if input.RCA != nil {
		explanation, _ = input.RCA["explanation"].(string)
	}
	if explanation == "" && !input.SummaryProvisional {
		explanation = input.Summary
	}
	if explanation == "" {
		return incompleteDecisionArtifact(input), nil
	}

	options := make([]any, 0, len(input.Discovery.Workflows))
	for index, workflow := range input.Discovery.Workflows {
		if workflow.WorkflowID == "" || workflow.Name == "" {
			return DecisionRecoveryResult{}, fmt.Errorf("discovered workflow at index %d is missing workflow_id or name", index)
		}
		options = append(options, map[string]any{
			"workflow_id": workflow.WorkflowID,
			"name":        workflow.Name,
			"description": workflow.Description,
			"recommended": index == 0,
		})
	}

	rca := cloneMap(input.RCA)
	if len(rca) == 0 {
		rca = map[string]any{"explanation": explanation}
	}
	data := map[string]any{
		"session_id":              input.SessionID,
		"summary":                 explanation,
		"rca":                     rca,
		"options":                 options,
		"source":                  recoverySource,
		"recovery_reason":         recoveryReason,
		"requires_user_selection": true,
	}
	if input.RRID != "" {
		data["rr_id"] = input.RRID
	}

	return DecisionRecoveryResult{
		Data:         data,
		TextFallback: fmt.Sprintf("Decision: %s", explanation),
		Metadata: map[string]any{
			"type":            MetaTypeDecision,
			"schema":          "investigation_summary",
			"schema_version":  "1.0",
			"source":          recoverySource,
			"recovery_reason": recoveryReason,
		},
		Complete: true,
	}, nil
}

func incompleteDecisionArtifact(input DecisionRecoveryInput) DecisionRecoveryResult {
	data := map[string]any{
		"session_id":      input.SessionID,
		"summary":         "Structured workflow decision could not be completed.",
		"rca":             map[string]any{"explanation": missingDataExplanation},
		"options":         []any{},
		"status":          "failure",
		"failure_reason":  missingDataFailure,
		"source":          recoverySource,
		"recovery_reason": recoveryReason,
	}
	if input.RRID != "" {
		data["rr_id"] = input.RRID
	}
	return DecisionRecoveryResult{
		Data:         data,
		TextFallback: missingDataExplanation,
		Metadata: map[string]any{
			"type":            MetaTypeDecision,
			"schema":          "investigation_summary",
			"schema_version":  "1.0",
			"source":          recoverySource,
			"recovery_reason": recoveryReason,
			"failure_reason":  missingDataFailure,
		},
		Complete: false,
	}
}

func cloneMap(input map[string]any) map[string]any {
	result := make(map[string]any, len(input))
	for key, value := range input {
		result[key] = value
	}
	return result
}
