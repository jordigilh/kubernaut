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
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/jordigilh/kubernaut/test/services/mock-llm/config"
	"github.com/jordigilh/kubernaut/test/services/mock-llm/scenarios"
)

// UT-MOCK-KA-017: E2E-FLEET-017 CI RCA (run 30723525089, job 91432164701).
//
// KA's RCA-phase turn-0 user prompt is always
// `fmt.Sprintf("Investigate: %s %s in %s — %s", ...)`
// (internal/kubernautagent/investigator/investigator_rca.go). The Fleet E2E
// suite's shared AF keyword scenario "af_investigate"
// (test/infrastructure/shared_e2e.go afKeywordYAML) matches on the bare
// substring "investigate" using match_last_only, and per
// scenarios.DefaultRegistryFull's doc comment, YAML keyword scenarios are
// registered at confidence 1.0 -- strictly greater than the 0.95
// scenario_ka_fleet_investigation.go originally used. Since
// Registry.Detect picks the strictly-highest-confidence match, "af_investigate"
// silently hijacked every KA-internal RCA-phase call in the fleet suite,
// scripting a "kubernaut_investigate" tool call that doesn't exist in KA's
// RCA tool registry ("tool not found: kubernaut_investigate"), which
// cascaded into investigator_rca.go's parse-level submit-only retry path
// before "ka_tool_call_e2e" ever got a chance to run -- so the real
// kubectl_get_by_name/resources_get tool call this scenario exists to prove
// was never exercised. This regression test pins the fix: "ka_tool_call_e2e"
// must win detection even when the shared af_investigate keyword scenario is
// registered and also matches.
var _ = Describe("UT-MOCK-KA-017: ka_tool_call_e2e must not be hijacked by the shared af_investigate keyword scenario", func() {
	It("UT-MOCK-KA-017-001: wins detection over af_investigate on KA's real turn-0 RCA prompt", func() {
		overrides := &config.Overrides{
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
		registry := scenarios.DefaultRegistryFull(overrides, "")

		// Mirrors KA's literal turn-0 RCA prompt for the E2E-FLEET-017 fixture:
		// fmt.Sprintf("Investigate: %s %s in %s — %s", severity, name, namespace, message).
		prompt := "Investigate: high ka-tool-e2e-test in kubernaut-system — memory pressure detected"
		ctx := &scenarios.DetectionContext{
			Content:         prompt,
			AllText:         prompt,
			LastUserContent: prompt,
		}

		result := registry.Detect(ctx)
		Expect(result).NotTo(BeNil())
		Expect(result.Scenario.Name()).To(Equal("ka_tool_call_e2e"),
			"KA's RCA prompt always contains the substring \"investigate\", which the shared "+
				"af_investigate keyword scenario (confidence 1.0) also matches on -- ka_tool_call_e2e "+
				"must out-rank it so KA's real tool-calling loop is exercised instead of being hijacked "+
				"into calling the nonexistent kubernaut_investigate tool")
	})
})
