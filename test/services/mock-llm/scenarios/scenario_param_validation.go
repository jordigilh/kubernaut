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
	"sync"

	"github.com/jordigilh/kubernaut/pkg/shared/uuid"
	"github.com/jordigilh/kubernaut/test/services/mock-llm/conversation"
)

// paramValidationSelfCorrectScenario is a stateful scenario that returns
// invalid parameters on the first tool-call request and corrected parameters
// when KA sends validation error feedback. This supports testing the full
// parameter validation self-correction loop (#1170).
//
// Detection: matches keyword "mock_param_validation_selfcorrect" in message text.
// State transition: presence of "expected parameters" (from FormatSchemaHint) in
// the conversation indicates KA already sent validation feedback, so we return
// the corrected config.
type paramValidationSelfCorrectScenario struct {
	mu            sync.Mutex
	lastCtx       *DetectionContext
	badConfig     MockScenarioConfig
	correctedConfig MockScenarioConfig
}

func paramValidationSelfCorrectConfigs() (bad, corrected MockScenarioConfig) {
	bad = MockScenarioConfig{
		ScenarioName: "param_validation_selfcorrect",
		SignalName:   "MOCK_PARAM_VALIDATION_SELFCORRECT",
		Severity:     "high",
		WorkflowName: "param-validation-test-v1",
		WorkflowID:   uuid.DeterministicUUID("param-validation-test-v1"),
		WorkflowTitle: "Param Validation Test",
		Confidence:   0.85,
		Rationale:    "Scaling deployment to handle increased load",
		RootCause:    "Deployment under-scaled for current traffic",
		ResourceKind: "Pod",
		ResourceNS:   "production",
		ResourceName: "api-server-xyz",
		APIVersion:   "v1",
		Parameters: map[string]string{
			"REPLICA_COUNT": "not-a-number",
			"EXTRA_HALLUCINATED": "should-be-stripped",
		},
		InvestigationOutcome: "actionable",
		IsActionable:         BoolPtr(true),
		ForceText:            BoolPtr(false),
	}

	corrected = MockScenarioConfig{
		ScenarioName: "param_validation_selfcorrect",
		SignalName:   "MOCK_PARAM_VALIDATION_SELFCORRECT",
		Severity:     "high",
		WorkflowName: "param-validation-test-v1",
		WorkflowID:   uuid.DeterministicUUID("param-validation-test-v1"),
		WorkflowTitle: "Param Validation Test",
		Confidence:   0.85,
		Rationale:    "Scaling deployment to handle increased load",
		RootCause:    "Deployment under-scaled for current traffic",
		ResourceKind: "Pod",
		ResourceNS:   "production",
		ResourceName: "api-server-xyz",
		APIVersion:   "v1",
		RawParameters: map[string]interface{}{
			"REPLICA_COUNT": float64(5),
			"NAMESPACE":     "production",
		},
		InvestigationOutcome: "actionable",
		IsActionable:         BoolPtr(true),
		ForceText:            BoolPtr(false),
	}
	return
}

func newParamValidationSelfCorrectScenario() *paramValidationSelfCorrectScenario {
	bad, corrected := paramValidationSelfCorrectConfigs()
	return &paramValidationSelfCorrectScenario{
		badConfig:       bad,
		correctedConfig: corrected,
	}
}

func (s *paramValidationSelfCorrectScenario) Name() string {
	return "param_validation_selfcorrect"
}

func (s *paramValidationSelfCorrectScenario) Match(ctx *DetectionContext) (bool, float64) {
	combined := strings.ToLower(ctx.Content + " " + ctx.AllText)
	if strings.Contains(combined, "mock_param_validation_selfcorrect") ||
		strings.Contains(combined, "mock param validation selfcorrect") {
		s.mu.Lock()
		s.lastCtx = ctx
		s.mu.Unlock()
		return true, 1.0
	}
	return false, 0
}

func (s *paramValidationSelfCorrectScenario) Metadata() ScenarioMetadata {
	return ScenarioMetadata{
		Name:        "param_validation_selfcorrect",
		Description: "Returns invalid params first, then corrected params after KA sends validation feedback (#1170)",
	}
}

func (s *paramValidationSelfCorrectScenario) DAG() *conversation.DAG { return nil }

// Config returns the corrected config if KA already sent validation feedback
// (detected by "expected parameters" in the conversation text from
// FormatSchemaHint), otherwise returns the bad config.
func (s *paramValidationSelfCorrectScenario) Config() MockScenarioConfig {
	s.mu.Lock()
	ctx := s.lastCtx
	s.mu.Unlock()

	if ctx != nil {
		combined := strings.ToLower(ctx.Content + " " + ctx.AllText)
		if strings.Contains(combined, "expected parameters") ||
			strings.Contains(combined, "type mismatch") ||
			strings.Contains(combined, "parameter validation") {
			return s.correctedConfig
		}
	}
	return s.badConfig
}
