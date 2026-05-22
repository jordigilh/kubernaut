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
	"regexp"
	"strings"

	"github.com/jordigilh/kubernaut/test/services/mock-llm/conversation"
)

// afCreateRRConfig returns a config that instructs the mock LLM to call
// af_create_rr when the AF ADK agent sends a Gemini request whose user
// message contains "create a remediation request".
//
// Issue #1189: E2E-FP-1189-002 and E2E-FP-1189-003 need the mock LLM to
// tell the AF agent to call af_create_rr so that an RR is actually created
// and the full downstream pipeline (RO → SP → AA → KA → WE) triggers.
func afCreateRRConfig() MockScenarioConfig {
	return MockScenarioConfig{
		ScenarioName: "af_create_rr",
		ToolCallName: "af_create_rr",
		ResourceKind: "Deployment",
		ResourceNS:   "kubernaut-system",
		ResourceName: "memory-eater",
		InvestigationOutcome: "actionable",
		IsActionable:         BoolPtr(true),
	}
}

var severityRe = regexp.MustCompile(`with severity (\w+)`)
var deployNSRe = regexp.MustCompile(`deployment (\S+) in (\S+) namespace`)

func afCreateRRScenario() *afCreateRRDynScenario {
	return &afCreateRRDynScenario{baseConfig: afCreateRRConfig()}
}

// afCreateRRDynScenario is a dynamic scenario that extracts target resource
// and severity from the user prompt to forward as af_create_rr tool args.
type afCreateRRDynScenario struct {
	baseConfig MockScenarioConfig
	lastCtx    *DetectionContext
}

func (s *afCreateRRDynScenario) Name() string { return s.baseConfig.ScenarioName }

func (s *afCreateRRDynScenario) Metadata() ScenarioMetadata {
	return ScenarioMetadata{Name: s.baseConfig.ScenarioName, Description: "Dynamic af_create_rr with severity extraction"}
}

func (s *afCreateRRDynScenario) DAG() *conversation.DAG { return nil }

func (s *afCreateRRDynScenario) Match(ctx *DetectionContext) (bool, float64) {
	combined := strings.ToLower(ctx.Content + " " + ctx.AllText)
	if strings.Contains(combined, "create a remediation request") {
		s.lastCtx = ctx
		return true, 0.9
	}
	return false, 0
}

func (s *afCreateRRDynScenario) Config() MockScenarioConfig {
	cfg := s.baseConfig
	if s.lastCtx == nil {
		return cfg
	}
	text := s.lastCtx.Content
	if m := deployNSRe.FindStringSubmatch(text); len(m) == 3 {
		cfg.ResourceName = m[1]
	}
	if m := severityRe.FindStringSubmatch(text); len(m) == 2 {
		cfg.Severity = m[1]
	}
	return cfg
}
