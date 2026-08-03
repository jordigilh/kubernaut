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
package mockllm_test

import (
	"strings"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/jordigilh/kubernaut/test/services/mock-llm/config"
	"github.com/jordigilh/kubernaut/test/services/mock-llm/scenarios"
)

// UT-MOCK-KA-018: E2E-FLEET-018 CI RCA (runs 30853023883 and 30858855479,
// job 91823709232 and successor).
//
// Turn 1 (kubernaut_remediate) triggers a real, AIAnalysis-controller-driven
// autonomous investigation before Turn 2/3 of the E2E test's own A2A flow
// run. That autonomous call's turn-0 RCA prompt is always
// `fmt.Sprintf("Investigate: %s %s in %s — %s", ...)`
// (internal/kubernautagent/investigator/investigator_rca.go), which never
// contains kaInteractiveFleetKeyword ("ka-interactive-fleet-e2e-test" only
// exists in Turn 3's kubernaut_message text argument). Before the fix, this
// meant "ka_interactive_fleet_bridge_e2e" never even entered detection for
// that call, leaving the shared "af_investigate" keyword scenario (bare
// "investigate" substring, confidence 1.0) as the only match -- mirroring
// UT-MOCK-KA-017's precedent for E2E-FLEET-017 (ka_tool_call_e2e_scenario_test.go)
// exactly, one keyword scenario collision at a time.
var _ = Describe("UT-MOCK-KA-018: ka_interactive_fleet_bridge_e2e must not be hijacked by the shared af_investigate keyword scenario", func() {
	var overrides *config.Overrides

	BeforeEach(func() {
		overrides = &config.Overrides{
			Scenarios: map[string]config.ScenarioOverride{},
			KeywordScenarios: []config.KeywordScenarioOverride{
				{
					Name:          "af_investigate",
					Keywords:      []string{"start investigation", "investigate", "begin investigation"},
					MatchLastOnly: true,
					ToolCall: config.ToolCallOverride{
						Name: "kubernaut_investigate",
						Arguments: map[string]interface{}{
							"rr_id": "$from_tool:kubernaut_remediate:rr_id",
						},
					},
				},
			},
		}
	})

	It("UT-MOCK-KA-018-001: wins detection over af_investigate on KA's real turn-0 autonomous RCA prompt", func() {
		registry := scenarios.DefaultRegistryFull(overrides, "")

		// Mirrors KA's literal turn-0 RCA prompt for the E2E-FLEET-018
		// fixture: fmt.Sprintf("Investigate: %s %s in %s — %s", severity,
		// signal.Name, namespace, signal.Message), plus the
		// "- Resource: <ns>/<kind>/<name>" line RenderInvestigation's
		// incident_investigation.tmpl renders into the system prompt from
		// prompt.SignalData.ResourceName.
		prompt := "investigate: high kainteractivefleetbridgegrounding in kubernaut-system — memory pressure detected\n" +
			"- resource: kubernaut-system/deployment/ka-interactive-fleet-target"
		ctx := &scenarios.DetectionContext{
			Content:         prompt,
			AllText:         prompt,
			LastUserContent: prompt,
			AvailableTools:  []string{"kubectl_get_by_name", "submit_result"},
		}

		result := registry.Detect(ctx)
		Expect(result).NotTo(BeNil())
		Expect(result.Scenario.Name()).To(Equal("ka_interactive_fleet_bridge_e2e"),
			"KA's autonomous RCA prompt always contains the bare substring \"investigate\" plus the "+
				"ka-interactive-fleet-target resource name; ka_interactive_fleet_bridge_e2e must out-rank "+
				"the shared af_investigate keyword scenario (confidence 1.0) so KA's real tool-calling loop "+
				"is exercised instead of being hijacked into calling the nonexistent kubernaut_investigate tool")
	})

	It("UT-MOCK-KA-018-002: autonomous-phase turn (no interactive keyword) never returns ForceText, so RCA can conclude via submit_result", func() {
		registry := scenarios.DefaultRegistryFull(overrides, "")

		prompt := "investigate: high kainteractivefleetbridgegrounding in kubernaut-system — memory pressure detected\n" +
			"- resource: kubernaut-system/deployment/ka-interactive-fleet-target"
		ctx := &scenarios.DetectionContext{
			Content:         prompt,
			AllText:         prompt,
			LastUserContent: prompt,
			AvailableTools:  []string{"kubectl_get_by_name", "submit_result"},
		}

		result := registry.Detect(ctx)
		Expect(result).NotTo(BeNil())
		Expect(result.Scenario.Name()).To(Equal("ka_interactive_fleet_bridge_e2e"))

		cfgScenario, ok := result.Scenario.(scenarios.ScenarioWithContextConfig)
		Expect(ok).To(BeTrue(), "scenario should implement ScenarioWithContextConfig")
		cfg := cfgScenario.ConfigForContext(ctx)

		Expect(cfg.ForceText == nil || !*cfg.ForceText).To(BeTrue(),
			"the autonomous RCA-phase call must never force a plain-text response -- RCA phase requires a "+
				"submit_result-compatible completion to conclude, and a ForceText answer here would starve "+
				"that requirement, exhausting maxTurns instead of finishing (this was the actual failure "+
				"mode once af_investigate's hijack was fixed by keyword alone without this branch)")
		Expect(cfg.RootCause).NotTo(BeEmpty(),
			"the autonomous-phase branch must produce a RootCause completion so the loop concludes normally")
	})

	It("UT-MOCK-KA-018-003: interactive turn (kubernaut_message keyword present) still returns the evidence-bearing ForceText response", func() {
		registry := scenarios.DefaultRegistryFull(overrides, "")

		// Mirrors Turn 3's kubernaut_message text plus the resources_get
		// tool result already appended to the conversation (turn 2 of
		// KA's own internal RunInteractiveTurn loop). ctx.AllText is
		// lowercased by buildDetectionContext in the real handler
		// (test/services/mock-llm/handlers/openai.go), so this test
		// lowercases it too -- an uppercase-preserving AllText here would
		// mask the exact case-sensitivity bug this scenario was already
		// fixed for once (see the evidence-found branch's doc comment).
		content := "ka-interactive-fleet-e2e-test: what is the current memory limit configured on the target deployment?"
		ctx := &scenarios.DetectionContext{
			Content:         content,
			AllText:         strings.ToLower(content + " 247Mi"),
			LastUserContent: content,
			AvailableTools:  []string{"resources_get", "submit_result"},
		}

		result := registry.Detect(ctx)
		Expect(result).NotTo(BeNil())
		Expect(result.Scenario.Name()).To(Equal("ka_interactive_fleet_bridge_e2e"))

		cfgScenario, ok := result.Scenario.(scenarios.ScenarioWithContextConfig)
		Expect(ok).To(BeTrue(), "scenario should implement ScenarioWithContextConfig")
		cfg := cfgScenario.ConfigForContext(ctx)

		Expect(cfg.ForceText).NotTo(BeNil())
		Expect(*cfg.ForceText).To(BeTrue(),
			"the interactive kubernaut_message turn must force a plain-text response so "+
				"investigator_loop.go's EventTypeReasoningDelta can stream the evidence into the A2A artifact")
		Expect(cfg.ExactAnalysisText).To(ContainSubstring("247Mi"),
			"the evidence-found branch must echo the genuine remote-cluster evidence value")
	})
})
