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

	"github.com/jordigilh/kubernaut/test/services/mock-llm/scenarios"
)

const interactiveRCARetrySignal2586 = "InteractiveRCAExtractionRetry2586"

func interactiveRCARetryContext2586(lastUserContent string, availableTools []string) *scenarios.DetectionContext {
	return &scenarios.DetectionContext{
		Content:         `authoritative signal context signal_name: "` + interactiveRCARetrySignal2586 + `"`,
		AllText:         `authoritative signal context signal_name: "` + interactiveRCARetrySignal2586 + `" ` + lastUserContent,
		LastUserContent: lastUserContent,
		Caller:          scenarios.CallerKA,
		Phase:           scenarios.PhaseRCA,
		AvailableTools:  availableTools,
	}
}

var _ = Describe("Interactive RCA correction scenario #2586", func() {
	It("UT-MOCK-2586-041: emits malformed RCA only for the initial submit-only extraction turn", func() {
		result := scenarios.DefaultRegistry().Detect(interactiveRCARetryContext2586(
			"Based on your investigation, submit the root cause analysis",
			[]string{"submit_result"},
		))
		Expect(result).NotTo(BeNil())
		Expect(result.Scenario.Name()).To(Equal("interactive_rca_retry_2586"))

		cfg, ok := result.Scenario.(scenarios.ScenarioWithContextConfig)
		Expect(ok).To(BeTrue())
		resolved := cfg.ConfigForContext(interactiveRCARetryContext2586(
			"Based on your investigation, submit the root cause analysis",
			[]string{"submit_result"},
		))
		Expect(resolved.ToolCallArgs).To(HaveKeyWithValue("root_cause_analysis", "malformed RCA object"))
		Expect(resolved.ReasoningText).To(Equal("rejected RCA turn"))
	})

	It("UT-MOCK-2586-042: emits valid RCA and distinctive reasoning after the correction prompt", func() {
		result := scenarios.DefaultRegistry().Detect(interactiveRCARetryContext2586(
			"Your response could not be parsed. You MUST call submit_result with a JSON object",
			[]string{"submit_result"},
		))
		Expect(result).NotTo(BeNil())
		Expect(result.Scenario.Name()).To(Equal("interactive_rca_retry_2586"))

		cfg, ok := result.Scenario.(scenarios.ScenarioWithContextConfig)
		Expect(ok).To(BeTrue())
		resolved := cfg.ConfigForContext(interactiveRCARetryContext2586(
			"Your response could not be parsed. You MUST call submit_result with a JSON object",
			[]string{"submit_result"},
		))
		Expect(resolved.ToolCallArgs).To(HaveKeyWithValue("confidence", BeNumerically("~", 0.65, 0.001)))
		Expect(resolved.ReasoningText).To(Equal("accepted correction evidence"))
	})

	It("UT-MOCK-2586-043: does not hijack autonomous RCA or workflow-discovery requests", func() {
		result := scenarios.DefaultRegistry().Detect(interactiveRCARetryContext2586(
			"Investigate the signal and submit your findings",
			[]string{"submit_result", "kubectl_get_pods"},
		))
		Expect(result).NotTo(BeNil())
		Expect(result.Scenario.Name()).NotTo(Equal("interactive_rca_retry_2586"))
	})
})
