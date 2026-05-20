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
package scenarios

import (
	"strings"
	"sync/atomic"

	"github.com/jordigilh/kubernaut/pkg/shared/uuid"
	"github.com/jordigilh/kubernaut/test/services/mock-llm/conversation"
)

// paramValidationSelfcorrectScenario is a stateful scenario that simulates
// LLM self-correction for parameter validation (BR-HAPI-191, #1170).
//
// Turn 1: Returns submit_result_with_workflow with invalid params (type
// mismatch: REPLICA_COUNT="three" instead of "3", plus undeclared param).
//
// Turn 2: After KA sends validation error feedback, returns the same workflow
// with corrected params (REPLICA_COUNT="3", no undeclared params).
type paramValidationSelfcorrectScenario struct {
	callCount       atomic.Int64
	overrideWfID    string // set by registry when YAML overrides provide the DS-generated UUID
}

const paramValScenarioName = "param_validation_selfcorrect"

func (s *paramValidationSelfcorrectScenario) Name() string { return paramValScenarioName }

func (s *paramValidationSelfcorrectScenario) Match(ctx *DetectionContext) (bool, float64) {
	signal := extractSignal(ctx)
	if strings.Contains(signal, "mock_param_validation_selfcorrect") {
		return true, 0.95
	}
	combined := strings.ToLower(ctx.Content + " " + ctx.AllText)
	if strings.Contains(combined, "mock_param_validation_selfcorrect") {
		return true, 0.95
	}
	return false, 0
}

func (s *paramValidationSelfcorrectScenario) Metadata() ScenarioMetadata {
	return ScenarioMetadata{
		Name:        paramValScenarioName,
		Description: "Multi-turn param validation self-correction (BR-HAPI-191)",
	}
}

func (s *paramValidationSelfcorrectScenario) DAG() *conversation.DAG { return nil }

func (s *paramValidationSelfcorrectScenario) Config() MockScenarioConfig {
	n := s.callCount.Add(1)
	if n <= 1 {
		return s.badParamsConfig()
	}
	return s.correctedParamsConfig()
}

func (s *paramValidationSelfcorrectScenario) effectiveWorkflowID() string {
	if s.overrideWfID != "" {
		return s.overrideWfID
	}
	return uuid.DeterministicUUID("param-validation-test-v1")
}

func (s *paramValidationSelfcorrectScenario) badParamsConfig() MockScenarioConfig {
	return MockScenarioConfig{
		ScenarioName:    paramValScenarioName,
		SignalName:      "MOCK_PARAM_VALIDATION_SELFCORRECT",
		Severity:        "high",
		WorkflowName:    "param-validation-test-v1",
		WorkflowID:      s.effectiveWorkflowID(),
		WorkflowTitle:   "Param Validation Test",
		Confidence:      0.85,
		RootCause:       "Pod api-server-xyz is experiencing high memory pressure. Scale replicas to handle load.",
		ResourceKind:    "Deployment",
		ResourceNS:      "production",
		ResourceName:    "api-server-xyz",
		APIVersion:      "apps/v1",
		ExecutionEngine: "job",
		Parameters: map[string]string{
			"REPLICA_COUNT":    "three",
			"NAMESPACE":        "production",
			"UNDECLARED_PARAM": "should_be_stripped",
		},
		InvestigationOutcome: "actionable",
		IsActionable:         BoolPtr(true),
	}
}

func (s *paramValidationSelfcorrectScenario) correctedParamsConfig() MockScenarioConfig {
	return MockScenarioConfig{
		ScenarioName:    paramValScenarioName,
		SignalName:      "MOCK_PARAM_VALIDATION_SELFCORRECT",
		Severity:        "high",
		WorkflowName:    "param-validation-test-v1",
		WorkflowID:      s.effectiveWorkflowID(),
		WorkflowTitle:   "Param Validation Test",
		Confidence:      0.90,
		RootCause:       "Pod api-server-xyz is experiencing high memory pressure. Scale replicas to handle load.",
		ResourceKind:    "Deployment",
		ResourceNS:      "production",
		ResourceName:    "api-server-xyz",
		APIVersion:      "apps/v1",
		ExecutionEngine: "job",
		RawParameters: map[string]interface{}{
			"REPLICA_COUNT": float64(3),
			"NAMESPACE":     "production",
		},
		InvestigationOutcome: "actionable",
		IsActionable:         BoolPtr(true),
	}
}

func paramValidationSelfcorrectScenarioNew() *paramValidationSelfcorrectScenario {
	return &paramValidationSelfcorrectScenario{}
}
