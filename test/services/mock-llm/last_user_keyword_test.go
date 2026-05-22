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

var _ = Describe("Multi-Turn Keyword Matching Fix (issue #1189)", func() {

	Describe("UT-ML-1189-001: Single user message populates LastUserContent", func() {
		It("should set LastUserContent to the user message text", func() {
			ctx := &scenarios.DetectionContext{
				Content:         "hello world",
				AllText:         "user hello world",
				LastUserContent: "hello world",
			}
			Expect(ctx.LastUserContent).To(Equal("hello world"))
		})
	})

	Describe("UT-ML-1189-002: Multi-turn LastUserContent is last user message only", func() {
		It("should contain only the third user message", func() {
			ctx := &scenarios.DetectionContext{
				Content:         "first message second message third message",
				AllText:         "user first message assistant reply user second message assistant reply user third message",
				LastUserContent: "third message",
			}
			Expect(ctx.LastUserContent).To(Equal("third message"))
		})
	})

	Describe("UT-ML-1189-003: No user messages yields empty LastUserContent", func() {
		It("should have empty LastUserContent for system-only messages", func() {
			ctx := &scenarios.DetectionContext{
				Content:         "system prompt only",
				AllText:         "system system prompt only",
				LastUserContent: "",
			}
			Expect(ctx.LastUserContent).To(BeEmpty())
		})
	})

	Describe("UT-ML-1189-004: match_last_only=true matches only last user message", func() {
		It("should match keyword in last user message and ignore prior turns", func() {
			overrides := &config.Overrides{
				Scenarios: map[string]config.ScenarioOverride{},
				KeywordScenarios: []config.KeywordScenarioOverride{
					{
						Name:          "af_start_investigation",
						Keywords:      []string{"start investigation"},
						ToolCall:      config.ToolCallOverride{Name: "kubernaut_start_investigation"},
						MatchLastOnly: true,
					},
					{
						Name:          "af_select_workflow",
						Keywords:      []string{"select workflow"},
						ToolCall:      config.ToolCallOverride{Name: "kubernaut_select_workflow"},
						MatchLastOnly: true,
					},
				},
			}
			registry := scenarios.DefaultRegistryWithOverrides(overrides)

			detCtx := &scenarios.DetectionContext{
				Content:         "start investigation for pod nginx select workflow oomkill",
				AllText:         "user start investigation for pod nginx assistant found issues user select workflow oomkill",
				LastUserContent: "select workflow oomkill",
			}
			result := registry.Detect(detCtx)
			Expect(result).NotTo(BeNil())
			Expect(result.Scenario.Name()).To(Equal("af_select_workflow"))
		})
	})

	Describe("UT-ML-1189-005: match_last_only=false matches full conversation (backward compat)", func() {
		It("should match keyword present in any prior message", func() {
			overrides := &config.Overrides{
				Scenarios: map[string]config.ScenarioOverride{},
				KeywordScenarios: []config.KeywordScenarioOverride{
					{
						Name:          "af_start_investigation",
						Keywords:      []string{"start investigation"},
						ToolCall:      config.ToolCallOverride{Name: "kubernaut_start_investigation"},
						MatchLastOnly: false,
					},
				},
			}
			registry := scenarios.DefaultRegistryWithOverrides(overrides)

			detCtx := &scenarios.DetectionContext{
				Content:         "start investigation for pod nginx now do something else",
				AllText:         "user start investigation for pod nginx assistant done user now do something else",
				LastUserContent: "now do something else",
			}
			result := registry.Detect(detCtx)
			Expect(result).NotTo(BeNil())
			Expect(result.Scenario.Name()).To(Equal("af_start_investigation"))
		})
	})

	Describe("UT-ML-1189-006: Multi-turn 4-phase determinism", func() {
		It("should match the correct scenario for each of 4 sequential turns", func() {
			overrides := &config.Overrides{
				Scenarios: map[string]config.ScenarioOverride{},
				KeywordScenarios: []config.KeywordScenarioOverride{
					{
						Name:          "af_start_investigation",
						Keywords:      []string{"start investigation"},
						ToolCall:      config.ToolCallOverride{Name: "kubernaut_stream_investigation"},
						MatchLastOnly: true,
					},
					{
						Name:          "af_discover_workflows",
						Keywords:      []string{"discover available workflows"},
						ToolCall:      config.ToolCallOverride{Name: "kubernaut_discover_workflows"},
						MatchLastOnly: true,
					},
					{
						Name:          "af_select_workflow",
						Keywords:      []string{"select workflow"},
						ToolCall:      config.ToolCallOverride{Name: "kubernaut_select_workflow"},
						MatchLastOnly: true,
					},
					{
						Name:          "af_create_rr",
						Keywords:      []string{"create a remediation request"},
						ToolCall:      config.ToolCallOverride{Name: "af_create_rr"},
						MatchLastOnly: true,
					},
				},
			}
			registry := scenarios.DefaultRegistryWithOverrides(overrides)

			type turnCase struct {
				accumulatedContent string
				accumulatedAllText string
				lastUserContent    string
				expectedScenario   string
			}

			turns := []turnCase{
				{
					accumulatedContent: "start investigation for deployment memory-eater",
					accumulatedAllText: "system prompt user start investigation for deployment memory-eater",
					lastUserContent:    "start investigation for deployment memory-eater",
					expectedScenario:   "af_start_investigation",
				},
				{
					accumulatedContent: "start investigation for deployment memory-eater investigation results discover available workflows",
					accumulatedAllText: "system prompt user start investigation for deployment memory-eater assistant investigation results user discover available workflows",
					lastUserContent:    "discover available workflows",
					expectedScenario:   "af_discover_workflows",
				},
				{
					accumulatedContent: "start investigation for deployment memory-eater investigation results discover available workflows workflow list select workflow oomkill",
					accumulatedAllText: "system prompt user start investigation for deployment memory-eater assistant results user discover available workflows assistant workflow list user select workflow oomkill",
					lastUserContent:    "select workflow oomkill",
					expectedScenario:   "af_select_workflow",
				},
				{
					accumulatedContent: "start investigation for deployment memory-eater investigation results discover available workflows workflow list select workflow oomkill selected create a remediation request",
					accumulatedAllText: "system prompt user start investigation assistant results user discover available workflows assistant list user select workflow oomkill assistant selected user create a remediation request",
					lastUserContent:    "create a remediation request",
					expectedScenario:   "af_create_rr",
				},
			}

			for i, tc := range turns {
				detCtx := &scenarios.DetectionContext{
					Content:         tc.accumulatedContent,
					AllText:         tc.accumulatedAllText,
					LastUserContent: tc.lastUserContent,
				}
				result := registry.Detect(detCtx)
				Expect(result).NotTo(BeNil(), "Turn %d: expected scenario match", i+1)
				Expect(result.Scenario.Name()).To(Equal(tc.expectedScenario),
					"Turn %d: expected %s but got %s", i+1, tc.expectedScenario, result.Scenario.Name())
			}
		})
	})

	Describe("UT-ML-1189-007: match_last_only with empty LastUserContent", func() {
		It("should not match when LastUserContent is empty", func() {
			overrides := &config.Overrides{
				Scenarios: map[string]config.ScenarioOverride{},
				KeywordScenarios: []config.KeywordScenarioOverride{
					{
						Name:          "af_start_investigation",
						Keywords:      []string{"start investigation"},
						ToolCall:      config.ToolCallOverride{Name: "kubernaut_start_investigation"},
						MatchLastOnly: true,
					},
				},
			}
			registry := scenarios.DefaultRegistryWithOverrides(overrides)

			detCtx := &scenarios.DetectionContext{
				Content:         "start investigation for pod nginx",
				AllText:         "system start investigation for pod nginx",
				LastUserContent: "",
			}
			result := registry.Detect(detCtx)
			Expect(result).NotTo(BeNil())
			Expect(result.Scenario.Name()).NotTo(Equal("af_start_investigation"),
				"match_last_only scenario should not match when LastUserContent is empty")
		})
	})

	Describe("UT-ML-1189-008: YAML MatchLastOnly parsing", func() {
		It("should parse match_last_only from YAML config", func() {
			yamlContent := `
keyword_scenarios:
  - name: "af_start_investigation"
    keywords: ["start investigation"]
    match_last_only: true
    tool_call:
      name: "kubernaut_start_investigation"
      arguments:
        namespace: "default"
  - name: "kubectl_list_pods"
    keywords: ["get pods"]
    tool_call:
      name: "kubectl_list"
`
			tmpFile := filepath.Join(GinkgoT().TempDir(), "overrides.yaml")
			Expect(os.WriteFile(tmpFile, []byte(yamlContent), 0644)).To(Succeed())

			overrides, err := config.LoadYAMLOverrides(tmpFile)
			Expect(err).NotTo(HaveOccurred())
			Expect(overrides.KeywordScenarios).To(HaveLen(2))
			Expect(overrides.KeywordScenarios[0].MatchLastOnly).To(BeTrue())
			Expect(overrides.KeywordScenarios[1].MatchLastOnly).To(BeFalse())
		})
	})

	Describe("UT-ML-1189-009: MatchLastOnly scenario registered with lastUser matcher", func() {
		It("should use lastUser matching in registry Detect", func() {
			overrides := &config.Overrides{
				Scenarios: map[string]config.ScenarioOverride{},
				KeywordScenarios: []config.KeywordScenarioOverride{
					{
						Name:          "af_tool",
						Keywords:      []string{"trigger tool"},
						ToolCall:      config.ToolCallOverride{Name: "my_tool"},
						MatchLastOnly: true,
					},
				},
			}
			registry := scenarios.DefaultRegistryWithOverrides(overrides)

			detCtx := &scenarios.DetectionContext{
				Content:         "trigger tool now please",
				AllText:         "user trigger tool now please",
				LastUserContent: "trigger tool now please",
			}
			result := registry.Detect(detCtx)
			Expect(result).NotTo(BeNil())
			Expect(result.Scenario.Name()).To(Equal("af_tool"))

			scenarioWithCfg, ok := result.Scenario.(scenarios.ScenarioWithConfig)
			Expect(ok).To(BeTrue())
			cfg := scenarioWithCfg.Config()
			Expect(cfg.ToolCallName).To(Equal("my_tool"))
		})
	})

	Describe("UT-ML-1189-010: Mixed registry with both match modes", func() {
		It("should correctly apply each scenario's individual match mode", func() {
			overrides := &config.Overrides{
				Scenarios: map[string]config.ScenarioOverride{},
				KeywordScenarios: []config.KeywordScenarioOverride{
					{
						Name:          "legacy_full_match",
						Keywords:      []string{"legacy keyword"},
						ToolCall:      config.ToolCallOverride{Name: "legacy_tool"},
						MatchLastOnly: false,
					},
					{
						Name:          "modern_last_only",
						Keywords:      []string{"modern keyword"},
						ToolCall:      config.ToolCallOverride{Name: "modern_tool"},
						MatchLastOnly: true,
					},
				},
			}
			registry := scenarios.DefaultRegistryWithOverrides(overrides)

			// legacy_full_match matches from prior turn content (MatchLastOnly=false)
			detCtx1 := &scenarios.DetectionContext{
				Content:         "legacy keyword was said earlier now something new",
				AllText:         "user legacy keyword was said earlier assistant ok user now something new",
				LastUserContent: "now something new",
			}
			result1 := registry.Detect(detCtx1)
			Expect(result1).NotTo(BeNil())
			Expect(result1.Scenario.Name()).To(Equal("legacy_full_match"))

			// modern_last_only does NOT match from prior turn content
			detCtx2 := &scenarios.DetectionContext{
				Content:         "modern keyword was said earlier now something else",
				AllText:         "user modern keyword was said earlier assistant ok user now something else",
				LastUserContent: "now something else",
			}
			result2 := registry.Detect(detCtx2)
			Expect(result2).NotTo(BeNil())
			Expect(result2.Scenario.Name()).NotTo(Equal("modern_last_only"),
				"modern_last_only should NOT match because keyword is not in LastUserContent")
		})
	})
})
