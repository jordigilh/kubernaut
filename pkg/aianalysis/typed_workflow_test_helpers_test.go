package aianalysis_test

import (
	"encoding/json"
	"fmt"

	corev1 "k8s.io/api/core/v1"
	apiextensionsv1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"

	agentsessionv1 "github.com/jordigilh/kubernaut/api/agentsession/v1alpha1"
	sharedtypes "github.com/jordigilh/kubernaut/pkg/shared/types"
)

const jsonNull = "null"

// typedAgentSessionWorkflow adapts the historical snake_case test fixture
// shape to the current typed AgentSession handoff. It intentionally ignores
// parameter_schema and declared_parameter_names: #2575 removes both persisted
// schema copies from the cross-service contract.
func typedAgentSessionWorkflow(value interface{}) *agentsessionv1.AgentSessionSelectedWorkflow {
	raw, err := json.Marshal(value)
	if err != nil {
		panic(err)
	}
	var fields map[string]json.RawMessage
	if err := json.Unmarshal(raw, &fields); err != nil {
		panic(err)
	}

	workflow := &agentsessionv1.AgentSessionSelectedWorkflow{
		WorkflowID:            stringField(fields, "workflowId", "workflow_id"),
		WorkflowName:          stringField(fields, "workflowName", "workflow_name"),
		ActionType:            stringField(fields, "actionType", "action_type"),
		Version:               stringField(fields, "version", "workflow_version"),
		ExecutionBundle:       stringField(fields, "executionBundle", "execution_bundle"),
		ExecutionBundleDigest: stringField(fields, "executionBundleDigest", "execution_bundle_digest"),
		ExecutionEngine:       stringField(fields, "executionEngine", "execution_engine"),
		ServiceAccountName:    stringField(fields, "serviceAccountName", "service_account_name"),
		ExecutionClusterID:    stringField(fields, "executionClusterId", "execution_cluster_id"),
		Rationale:             stringField(fields, "rationale", "workflow_rationale"),
		RepeatValidation:      stringField(fields, "repeatValidationOutcome", "repeat_validation_outcome"),
		HandoffDigest:         stringField(fields, "handoffDigest", "handoff_digest"),
	}
	if rawConfidence := field(fields, "confidence"); rawConfidence != nil {
		if err := json.Unmarshal(rawConfidence, &workflow.Confidence); err != nil {
			panic(err)
		}
	}
	if rawConfidence := field(fields, "workflowSelectionConfidence", "workflow_selection_confidence"); rawConfidence != nil {
		var confidence float64
		if err := json.Unmarshal(rawConfidence, &confidence); err != nil {
			panic(err)
		}
		workflow.SelectionConfidence = &confidence
	}
	if rawRequired := field(fields, "repeatJustificationRequired", "repeat_justification_required"); rawRequired != nil {
		var required bool
		if err := json.Unmarshal(rawRequired, &required); err != nil {
			panic(err)
		}
		workflow.RepeatRequired = &required
	}
	if rawMatch := field(fields, "priorWorkflowMatch", "prior_workflow_match"); rawMatch != nil {
		var match bool
		if err := json.Unmarshal(rawMatch, &match); err != nil {
			panic(err)
		}
		workflow.PriorWorkflowMatch = &match
	}
	if rawPrior := field(fields, "priorRemediationUIDs", "prior_remediation_uids"); rawPrior != nil {
		if err := json.Unmarshal(rawPrior, &workflow.PriorRemediationUIDs); err != nil {
			panic(err)
		}
	}
	if rawJustification := field(fields, "repeatJustification", "repeat_justification"); rawJustification != nil {
		var justification sharedtypes.RepeatJustification
		if err := json.Unmarshal(rawJustification, &justification); err != nil {
			panic(err)
		}
		workflow.RepeatJustification = &justification
	}
	if rawEngineConfig := field(fields, "engineConfig", "engine_config"); rawEngineConfig != nil && string(rawEngineConfig) != jsonNull {
		workflow.EngineConfig = &apiextensionsv1.JSON{Raw: append([]byte(nil), rawEngineConfig...)}
	}
	if rawDependencies := field(fields, "dependencies"); rawDependencies != nil && string(rawDependencies) != jsonNull {
		var dependencies sharedtypes.WorkflowDependencies
		if err := json.Unmarshal(rawDependencies, &dependencies); err != nil {
			panic(err)
		}
		workflow.Dependencies = &dependencies
	}
	if rawResources := field(fields, "resources"); rawResources != nil && string(rawResources) != jsonNull {
		var resources corev1.ResourceRequirements
		if err := json.Unmarshal(rawResources, &resources); err != nil {
			panic(err)
		}
		workflow.Resources = &resources
	}
	if rawParameters := field(fields, "parameters"); rawParameters != nil && string(rawParameters) != jsonNull {
		parameters, evidence, err := sharedtypes.DecodeTypedParameterMap(rawParameters)
		if err != nil {
			panic(fmt.Errorf("typed test workflow parameters: %w", err))
		}
		workflow.Parameters = parameters
		workflow.ParameterEvidence = evidence
	}
	return workflow
}

func field(fields map[string]json.RawMessage, names ...string) json.RawMessage {
	for _, name := range names {
		if value, ok := fields[name]; ok {
			return value
		}
	}
	return nil
}

func stringField(fields map[string]json.RawMessage, names ...string) string {
	value := field(fields, names...)
	if value == nil {
		return ""
	}
	var result string
	if err := json.Unmarshal(value, &result); err != nil {
		panic(err)
	}
	return result
}
