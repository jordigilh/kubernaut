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
	"os"
	"path/filepath"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/jordigilh/kubernaut/test/services/mock-llm/config"
	"github.com/jordigilh/kubernaut/test/services/mock-llm/scenarios"
)

var _ = Describe("Keyword Scenarios YAML Override (issue #1160)", func() {

	Describe("UT-MOCK-KW-001: YAML parsing of keyword_scenarios", func() {
		It("UT-MOCK-KW-001-001: should parse keyword_scenarios from YAML", func() {
			yaml := `
keyword_scenarios:
  - name: "af_investigate"
    keywords: ["start investigation", "begin investigation"]
    tool_call:
      name: "kubernaut_investigate"
      arguments:
        namespace: "default"
        name: "nginx"
        kind: "Pod"
  - name: "kubectl_list_pods"
    keywords: ["get pods"]
    tool_call:
      name: "kubectl_list"
      arguments:
        kind: "Pod"
        namespace: "default"
`
			tmpFile := filepath.Join(GinkgoT().TempDir(), "overrides.yaml")
			Expect(os.WriteFile(tmpFile, []byte(yaml), 0644)).To(Succeed())

			overrides, err := config.LoadYAMLOverrides(tmpFile)
			Expect(err).NotTo(HaveOccurred())
			Expect(overrides.KeywordScenarios).To(HaveLen(2))
			Expect(overrides.KeywordScenarios[0].Name).To(Equal("af_investigate"))
			Expect(overrides.KeywordScenarios[0].Keywords).To(ConsistOf("start investigation", "begin investigation"))
			Expect(overrides.KeywordScenarios[0].ToolCall.Name).To(Equal("kubernaut_investigate"))
			Expect(overrides.KeywordScenarios[0].ToolCall.Arguments).To(HaveKeyWithValue("namespace", "default"))
			Expect(overrides.KeywordScenarios[0].ToolCall.Arguments).To(HaveKeyWithValue("name", "nginx"))
			Expect(overrides.KeywordScenarios[0].ToolCall.Arguments).To(HaveKeyWithValue("kind", "Pod"))
		})

		It("UT-MOCK-KW-001-002: should coexist with existing scenario overrides", func() {
			yaml := `
scenarios:
  oomkilled:
    workflow_id: "custom-uuid"
keyword_scenarios:
  - name: "kubectl_list_pods"
    keywords: ["get pods"]
    tool_call:
      name: "kubectl_list"
`
			tmpFile := filepath.Join(GinkgoT().TempDir(), "overrides.yaml")
			Expect(os.WriteFile(tmpFile, []byte(yaml), 0644)).To(Succeed())

			overrides, err := config.LoadYAMLOverrides(tmpFile)
			Expect(err).NotTo(HaveOccurred())
			Expect(overrides.Scenarios).To(HaveKey("oomkilled"))
			Expect(overrides.Scenarios["oomkilled"].WorkflowID).To(Equal("custom-uuid"))
			Expect(overrides.KeywordScenarios).To(HaveLen(1))
			Expect(overrides.KeywordScenarios[0].Name).To(Equal("kubectl_list_pods"))
		})

		It("UT-MOCK-KW-001-003: should handle missing keyword_scenarios gracefully", func() {
			yaml := `
scenarios:
  oomkilled:
    workflow_id: "custom-uuid"
`
			tmpFile := filepath.Join(GinkgoT().TempDir(), "overrides.yaml")
			Expect(os.WriteFile(tmpFile, []byte(yaml), 0644)).To(Succeed())

			overrides, err := config.LoadYAMLOverrides(tmpFile)
			Expect(err).NotTo(HaveOccurred())
			Expect(overrides.KeywordScenarios).To(BeEmpty())
		})
	})

	Describe("UT-MOCK-KW-002: Registry detection of keyword scenarios", func() {
		It("UT-MOCK-KW-002-001: should detect scenario by keyword match", func() {
			overrides := &config.Overrides{
				Scenarios: map[string]config.ScenarioOverride{},
				KeywordScenarios: []config.KeywordScenarioOverride{
					{
						Name:     "af_investigate",
						Keywords: []string{"start investigation"},
						ToolCall: config.ToolCallOverride{
							Name:      "kubernaut_investigate",
							Arguments: map[string]string{"namespace": "default"},
						},
					},
				},
			}
			registry := scenarios.DefaultRegistryWithOverrides(overrides)

			detCtx := &scenarios.DetectionContext{
				Content: "please start investigation on pod nginx",
				AllText: "please start investigation on pod nginx",
			}
			result := registry.Detect(detCtx)
			Expect(result.Confidence).To(Equal(1.0))
			Expect(result.Scenario.Name()).To(Equal("af_investigate"))

			scenarioWithCfg, ok := result.Scenario.(scenarios.ScenarioWithConfig)
			Expect(ok).To(BeTrue())
			cfg := scenarioWithCfg.Config()
			Expect(cfg.ToolCallName).To(Equal("kubernaut_investigate"))
			Expect(cfg.ToolCallArgs).To(HaveKeyWithValue("namespace", "default"))
		})

		It("UT-MOCK-KW-002-002: should not match when keyword is absent", func() {
			overrides := &config.Overrides{
				Scenarios: map[string]config.ScenarioOverride{},
				KeywordScenarios: []config.KeywordScenarioOverride{
					{
						Name:     "af_investigate",
						Keywords: []string{"start investigation"},
						ToolCall: config.ToolCallOverride{Name: "kubernaut_investigate"},
					},
				},
			}
			registry := scenarios.DefaultRegistryWithOverrides(overrides)

			detCtx := &scenarios.DetectionContext{
				Content: "something completely unrelated",
				AllText: "something completely unrelated",
			}
			result := registry.Detect(detCtx)
			// Should fall through to default fallback, not keyword scenario
			Expect(result.Scenario.Name()).ToNot(Equal("af_investigate"))
		})

		It("UT-MOCK-KW-002-003: should support multiple keywords for same scenario", func() {
			overrides := &config.Overrides{
				Scenarios: map[string]config.ScenarioOverride{},
				KeywordScenarios: []config.KeywordScenarioOverride{
					{
						Name:     "kubectl_list_pods",
						Keywords: []string{"get pods", "list pods", "show pods"},
						ToolCall: config.ToolCallOverride{Name: "kubectl_list"},
					},
				},
			}
			registry := scenarios.DefaultRegistryWithOverrides(overrides)

			for _, keyword := range []string{"get pods", "list pods", "show pods"} {
				detCtx := &scenarios.DetectionContext{
					Content: "I need to " + keyword + " in namespace default",
					AllText: "I need to " + keyword + " in namespace default",
				}
				result := registry.Detect(detCtx)
				Expect(result.Scenario.Name()).To(Equal("kubectl_list_pods"),
					"expected kubectl_list_pods for keyword %q", keyword)
			}
		})

		It("UT-MOCK-KW-002-004: keyword scenario should have ForceText=false for tool calls", func() {
			overrides := &config.Overrides{
				Scenarios: map[string]config.ScenarioOverride{},
				KeywordScenarios: []config.KeywordScenarioOverride{
					{
						Name:     "af_tool",
						Keywords: []string{"trigger tool"},
						ToolCall: config.ToolCallOverride{Name: "my_tool"},
					},
				},
			}
			registry := scenarios.DefaultRegistryWithOverrides(overrides)

			detCtx := &scenarios.DetectionContext{
				Content: "trigger tool now",
				AllText: "trigger tool now",
			}
			result := registry.Detect(detCtx)
			scenarioWithCfg := result.Scenario.(scenarios.ScenarioWithConfig)
			cfg := scenarioWithCfg.Config()
			Expect(cfg.ForceText).To(Equal(scenarios.BoolPtr(false)))
		})
	})
})
