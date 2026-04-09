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

import "github.com/jordigilh/kubernaut/test/services/mock-llm/conversation"

// MockAlternativeWorkflow mirrors real Claude behavior where the LLM returns
// ranked alternatives alongside the primary workflow selection.
// Golden transcript ref: kubernaut-demo-scenarios#296
type MockAlternativeWorkflow struct {
	WorkflowName string
	WorkflowID   string
	Confidence   float64
	Rationale    string
}

// MockScenarioConfig holds the static configuration for a mock scenario.
type MockScenarioConfig struct {
	ScenarioName     string
	SignalName       string
	Severity         string
	WorkflowName     string
	WorkflowID       string
	WorkflowTitle    string
	Confidence       float64
	Rationale        string // primary workflow rationale (golden transcript fidelity)
	RootCause        string
	ResourceKind     string
	ResourceNS       string
	ResourceName     string
	APIVersion       string
	IncludeAffected  bool
	OverrideResource bool
	Parameters       map[string]string
	ExecutionEngine  string
	Contributing     []string
	NeedsHumanReview *bool
	Alternatives     []MockAlternativeWorkflow // ranked alternatives (golden transcript fidelity)

	// KA outcome routing fields — included as top-level JSON in text responses
	// so KA's parser can extract them for is_actionable / human_review routing.
	InvestigationOutcome string // "problem_resolved", "predictive_no_action", "actionable", "inconclusive"
	IsActionable         *bool
	HumanReviewReason    string

	// ExactAnalysisText, when non-empty, is returned verbatim as the LLM
	// response text instead of synthesizing from the other config fields.
	// Used by golden transcript replay to produce full-fidelity responses.
	ExactAnalysisText string
}

// BoolPtr is a helper for creating *bool literals in scenario configs.
func BoolPtr(v bool) *bool { return &v }

// configScenario is a Scenario backed by a static MockScenarioConfig and
// a MatchFunc that implements the detection logic.
type configScenario struct {
	config    MockScenarioConfig
	matchFunc func(ctx *DetectionContext) (bool, float64)
}

func (s *configScenario) Name() string { return s.config.ScenarioName }
func (s *configScenario) Match(ctx *DetectionContext) (bool, float64) {
	return s.matchFunc(ctx)
}
func (s *configScenario) Metadata() ScenarioMetadata {
	return ScenarioMetadata{
		Name:        s.config.ScenarioName,
		Description: s.config.RootCause,
	}
}
func (s *configScenario) DAG() *conversation.DAG { return nil }

// Config returns the underlying MockScenarioConfig for response building.
func (s *configScenario) Config() MockScenarioConfig { return s.config }

// ScenarioWithConfig is implemented by scenarios that expose their config.
type ScenarioWithConfig interface {
	Scenario
	Config() MockScenarioConfig
}
