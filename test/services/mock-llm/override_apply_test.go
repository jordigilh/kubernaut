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

var _ = Describe("Override Application (BR-TESTING-657)", func() {

	Describe("UT-MOCK-657-003: applyOverride applies ForceText to MockScenarioConfig", func() {
		It("should propagate ForceText=false from override to scenario config", func() {
			forceText := false
			overrides := &config.Overrides{
				Scenarios: map[string]config.ScenarioOverride{
					"oomkilled": {
						ForceText: &forceText,
					},
				},
			}

			registry := scenarios.DefaultRegistryWithOverrides(overrides)
			ctx := &scenarios.DetectionContext{
				Content:    "- Signal Name: OOMKilled\n- Namespace: default",
				AllText:    "- Signal Name: OOMKilled\n- Namespace: default",
				SignalName: "",
			}
			result := registry.Detect(ctx)
			Expect(result).NotTo(BeNil())
			Expect(result.Scenario.Name()).To(Equal("oomkilled"))

			cfgScenario, ok := result.Scenario.(scenarios.ScenarioWithConfig)
			Expect(ok).To(BeTrue(), "scenario should implement ScenarioWithConfig")

			cfg := cfgScenario.Config()
			Expect(cfg.ForceText).NotTo(BeNil(), "ForceText should be propagated from override")
			Expect(*cfg.ForceText).To(BeFalse())
		})
	})

	Describe("UT-MOCK-1169-005: alternative workflow UUIDs are replaced from overrides", func() {
		It("should replace the deterministic alternative UUID with the DS-assigned UUID", func() {
			overrides := &config.Overrides{
				Scenarios: map[string]config.ScenarioOverride{
					"generic-restart-v1:production": {
						WorkflowID: "real-ds-uuid-for-generic-restart",
					},
				},
			}

			registry := scenarios.DefaultRegistryWithOverrides(overrides)
			ctx := &scenarios.DetectionContext{
				Content:    "- Signal Name: OOMKilled\n- Namespace: default",
				AllText:    "- Signal Name: OOMKilled\n- Namespace: default",
				SignalName: "",
			}
			result := registry.Detect(ctx)
			Expect(result).NotTo(BeNil())
			Expect(result.Scenario.Name()).To(Equal("oomkilled"))

			cfgScenario, ok := result.Scenario.(scenarios.ScenarioWithConfig)
			Expect(ok).To(BeTrue())

			cfg := cfgScenario.Config()
			Expect(cfg.Alternatives).NotTo(BeEmpty(),
				"oomkilled scenario must have at least one alternative")
			Expect(cfg.Alternatives[0].WorkflowName).To(Equal("generic-restart-v1"))
			Expect(cfg.Alternatives[0].WorkflowID).To(Equal("real-ds-uuid-for-generic-restart"),
				"alternative UUID should be replaced by the override from DataStorage")
		})
	})

	Describe("UT-MOCK-657-004: applyOverride applies ToolCall fields to MockScenarioConfig", func() {
		It("should propagate ToolCallName and ToolCallArgs from override to scenario config", func() {
			forceText := false
			overrides := &config.Overrides{
				Scenarios: map[string]config.ScenarioOverride{
					"oomkilled": {
						ForceText: &forceText,
						ToolCall: &config.ToolCallOverride{
							Name: "kubectl_get_yaml",
							Arguments: map[string]string{
								"kind":      "ConfigMap",
								"name":      "poisoned-cm",
								"namespace": "default",
							},
						},
					},
				},
			}

			registry := scenarios.DefaultRegistryWithOverrides(overrides)
			ctx := &scenarios.DetectionContext{
				Content:    "- Signal Name: OOMKilled\n- Namespace: default",
				AllText:    "- Signal Name: OOMKilled\n- Namespace: default",
				SignalName: "",
			}
			result := registry.Detect(ctx)
			Expect(result).NotTo(BeNil())
			Expect(result.Scenario.Name()).To(Equal("oomkilled"))

			cfgScenario, ok := result.Scenario.(scenarios.ScenarioWithConfig)
			Expect(ok).To(BeTrue(), "scenario should implement ScenarioWithConfig")

			cfg := cfgScenario.Config()
			Expect(cfg.ToolCallName).To(Equal("kubectl_get_yaml"))
			Expect(cfg.ToolCallArgs).To(HaveKeyWithValue("kind", "ConfigMap"))
			Expect(cfg.ToolCallArgs).To(HaveKeyWithValue("name", "poisoned-cm"))
			Expect(cfg.ToolCallArgs).To(HaveKeyWithValue("namespace", "default"))
		})
	})
})
