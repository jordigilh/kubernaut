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
	"time"

	"github.com/jordigilh/kubernaut/test/services/mock-llm/conversation"
)

// afCreateRRConfig returns a config that instructs the mock LLM to call
// kubernaut_remediate when the AF ADK agent sends a Gemini request whose user
// message contains "create a remediation request".
//
// E2E-FP-1189-002 uses this for autonomous remediation (no IS creation).
func afCreateRRConfig() MockScenarioConfig {
	return MockScenarioConfig{
		ScenarioName:         "kubernaut_remediate",
		ToolCallName:         "kubernaut_remediate",
		ResourceKind:         "Deployment",
		ResourceNS:           "kubernaut-system",
		ResourceName:         "memory-eater",
		APIVersion:           "apps/v1",
		InvestigationOutcome: "actionable",
		IsActionable:         BoolPtr(true),
		ForceText:            BoolPtr(false),
	}
}

var deployNSRe = regexp.MustCompile(`deployment\s+(\S+)\s+in\s+(\S+)\s+namespace`)

// afCreateRRSlowConfig returns a kubernaut_remediate config with a 5-second
// second-turn delay. Used by TC-E2E-STREAM-03 to test client disconnect
// detection (BR-SESS-003, SI-4). The delay keeps the executor blocked on
// the mock-LLM after kubernaut_remediate completes, giving the test a window to
// close the SSE connection while the session CRD is Active.
func afCreateRRSlowConfig() MockScenarioConfig {
	cfg := afCreateRRConfig()
	cfg.ScenarioName = "kubernaut_remediate_slow"
	cfg.SecondTurnDelay = 5 * time.Second
	return cfg
}

// afCreateRRSlowScenario matches prompts containing "slow-disconnect-test"
// with priority 0.95 (above the general kubernaut_remediate scenario at 0.9).
func afCreateRRSlowScenario() *afCreateRRDynScenario {
	base := afCreateRRSlowConfig()
	return &afCreateRRDynScenario{
		baseConfig: base,
		matchOverride: func(ctx *DetectionContext) (bool, float64) {
			combined := strings.ToLower(ctx.Content + " " + ctx.AllText)
			if strings.Contains(combined, "slow-disconnect-test") {
				return true, 0.95
			}
			return false, 0
		},
	}
}

func afCreateRRScenario() *afCreateRRDynScenario {
	return &afCreateRRDynScenario{baseConfig: afCreateRRConfig()}
}

// afCreateRRCrossNSScenario matches prompts containing "cross-namespace remediation"
// with priority 0.95. Used by E2E-FP-1292-001 to test ADR-057: the workload namespace
// is extracted dynamically from the prompt via deployNSRe, while the RR CRD is placed
// in kubernaut-system (controllerNS injected at wiring time).
//
// Namespace extraction happens eagerly during Match() — not deferred to Config() —
// because the ADK Gemini adapter may restructure the content between the Match and
// Config phases (observed in CI run 26469769357).
func afCreateRRCrossNSScenario() *afCreateRRDynScenario {
	base := afCreateRRConfig()
	base.ScenarioName = "kubernaut_remediate_cross_ns"
	s := &afCreateRRDynScenario{baseConfig: base}
	s.matchOverride = func(ctx *DetectionContext) (bool, float64) {
		combined := strings.ToLower(ctx.Content + " " + ctx.AllText)
		if !strings.Contains(combined, "cross-namespace remediation") {
			return false, 0
		}
		return true, 0.95
	}
	return s
}

// afCreateRRDynScenario is a dynamic scenario that extracts the target resource
// from the user prompt to forward as kubernaut_remediate tool args.
// Post-#1282: namespace and severity are AF-resolved; only kind/name/description
// are sent by the LLM.
//
// Thread-safety (#1458): resource extraction is performed in ConfigForContext()
// from the DetectionContext passed by the handler, not from stored state.
// This eliminates the data race where concurrent Detect() callers overwrote
// lastCtx, causing kubernaut_remediate to create RRs with the wrong target name.
type afCreateRRDynScenario struct {
	baseConfig    MockScenarioConfig
	matchOverride func(ctx *DetectionContext) (bool, float64)
}

func (s *afCreateRRDynScenario) Name() string { return s.baseConfig.ScenarioName }

func (s *afCreateRRDynScenario) Metadata() ScenarioMetadata {
	return ScenarioMetadata{Name: s.baseConfig.ScenarioName, Description: "Dynamic kubernaut_remediate with resource extraction"}
}

func (s *afCreateRRDynScenario) DAG() *conversation.DAG { return nil }

func (s *afCreateRRDynScenario) Match(ctx *DetectionContext) (bool, float64) {
	if s.matchOverride != nil {
		return s.matchOverride(ctx)
	}
	combined := strings.ToLower(ctx.Content + " " + ctx.AllText)
	if strings.Contains(combined, "create a remediation request") {
		return true, 0.9
	}
	return false, 0
}

func (s *afCreateRRDynScenario) Config() MockScenarioConfig {
	return s.baseConfig
}

func (s *afCreateRRDynScenario) ConfigForContext(ctx *DetectionContext) MockScenarioConfig {
	cfg := s.baseConfig
	if ctx == nil {
		return cfg
	}
	for _, text := range []string{ctx.Content, ctx.AllText, ctx.LastUserContent} {
		if m := deployNSRe.FindStringSubmatch(text); len(m) == 3 {
			cfg.ResourceName = m[1]
			cfg.ResourceNS = m[2]
			return cfg
		}
	}
	return cfg
}
