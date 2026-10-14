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
	"context"
	"io"
	"os"
	"path/filepath"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"gopkg.in/yaml.v3"

	"github.com/jordigilh/kubernaut/test/infrastructure"
	"github.com/jordigilh/kubernaut/test/services/mock-llm/config"
	"github.com/jordigilh/kubernaut/test/services/mock-llm/scenarios"
)

var _ = Describe("Keyword Scenarios YAML Override (issue #1160)", func() {

	Describe("UT-MOCK-SEL-001: YAML parsing of scenario_selectors", func() {
		It("should parse the canonical selector schema", func() {
			yaml := `
scenario_selectors:
  - name: "af_investigate"
    caller: "af"
    phase: "investigation"
    keywords: ["start investigation"]
    match_last_only: true
    tool_call:
      name: "kubernaut_investigate"
`
			tmpFile := filepath.Join(GinkgoT().TempDir(), "overrides.yaml")
			Expect(os.WriteFile(tmpFile, []byte(yaml), 0644)).To(Succeed())

			overrides, err := config.LoadYAMLOverrides(tmpFile)
			Expect(err).NotTo(HaveOccurred())
			Expect(overrides.ScenarioSelectors).To(HaveLen(1))
			Expect(overrides.ScenarioSelectors[0].Name).To(Equal("af_investigate"))
			Expect(overrides.ScenarioSelectors[0].Caller).To(Equal("af"))
			Expect(overrides.ScenarioSelectors[0].Phase).To(Equal("investigation"))
		})

		It("should retain support for the deprecated keyword_scenarios schema", func() {
			yaml := `
keyword_scenarios:
  - name: "legacy"
    keywords: ["legacy trigger"]
    tool_call:
      name: "legacy_tool"
`
			tmpFile := filepath.Join(GinkgoT().TempDir(), "overrides.yaml")
			Expect(os.WriteFile(tmpFile, []byte(yaml), 0644)).To(Succeed())

			overrides, err := config.LoadYAMLOverrides(tmpFile)
			Expect(err).NotTo(HaveOccurred())
			Expect(overrides.KeywordScenarios).To(HaveLen(1))
			Expect(overrides.KeywordScenarios[0].Name).To(Equal("legacy"))
		})

		It("should register canonical selectors through the normal detection path", func() {
			registry := scenarios.DefaultRegistryFull(&config.Overrides{
				ScenarioSelectors: []config.ScenarioSelectorOverride{{
					Name:     "canonical_selector",
					Caller:   "af",
					Phase:    "investigation",
					Keywords: []string{"canonical trigger"},
					ToolCall: config.ToolCallOverride{Name: "canonical_tool"},
				}},
			}, "")

			result := registry.Detect(&scenarios.DetectionContext{
				Content: "canonical trigger",
				Caller:  scenarios.CallerAF,
				Phase:   scenarios.PhaseInvestigation,
			})
			Expect(result).NotTo(BeNil())
			Expect(result.Scenario.Name()).To(Equal("canonical_selector"))
		})

		It("should reject malformed canonical selectors before registration", func() {
			yaml := `
scenario_selectors:
  - name: "broken"
    caller: "frontend"
    keywords: []
    tool_call:
      name: ""
`
			tmpFile := filepath.Join(GinkgoT().TempDir(), "overrides.yaml")
			Expect(os.WriteFile(tmpFile, []byte(yaml), 0644)).To(Succeed())

			_, err := config.LoadYAMLOverrides(tmpFile)
			Expect(err).To(MatchError("scenario selector caller must be one of: af, ka"))
		})
	})

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
		It("UT-MOCK-KW-002-005: should carry workflow discovery overrides into scoped selectors", func() {
			registry := scenarios.DefaultRegistryWithOverrides(&config.Overrides{
				ScenarioSelectors: []config.ScenarioSelectorOverride{{
					Name:       "ka_consent_discovery",
					Caller:     "ka",
					Phase:      "workflow_discovery",
					Keywords:   []string{"FullPipelineA2ASeverityGrounding"},
					WorkflowID: "consent-workflow-uuid",
					ActionType: "RestartPod",
					ToolCall:   config.ToolCallOverride{Name: "list_available_actions"},
				}},
			})

			result := registry.Detect(&scenarios.DetectionContext{
				Content:        "FullPipelineA2ASeverityGrounding workflow discovery",
				Caller:         scenarios.CallerKA,
				Phase:          scenarios.PhaseWorkflowDiscovery,
				AvailableTools: []string{"list_available_actions", "list_workflows", "get_workflow", "submit_result_with_workflow"},
			})
			Expect(result).NotTo(BeNil())
			Expect(result.Scenario.Name()).To(Equal("ka_consent_discovery"))

			scenarioWithCfg, ok := result.Scenario.(scenarios.ScenarioWithConfig)
			Expect(ok).To(BeTrue())
			cfg := scenarioWithCfg.Config()
			Expect(cfg.WorkflowID).To(Equal("consent-workflow-uuid"))
			Expect(cfg.ActionType).To(Equal("RestartPod"))
		})

		It("UT-MOCK-KW-002-001: should detect scenario by keyword match", func() {
			overrides := &config.Overrides{
				Scenarios: map[string]config.ScenarioOverride{},
				KeywordScenarios: []config.KeywordScenarioOverride{
					{
						Name:     "af_investigate",
						Keywords: []string{"start investigation"},
						ToolCall: config.ToolCallOverride{
							Name:      "kubernaut_investigate",
							Arguments: map[string]interface{}{"namespace": "default"},
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

	Describe("UT-MOCK-2390-002: GitOps workflow discovery signal selector", func() {
		currentScore, knownZero := .95, 0.
		DescribeTable("UT-MOCK-2496-013 (BR-KA-016): declarative workflow scores preserve explicit zero and never use an unrelated scenario score",
			func(score *float64, expected float64) {
				unrelated := .99
				registry := scenarios.DefaultRegistryFull(&config.Overrides{
					Scenarios: map[string]config.ScenarioOverride{
						"current-workflow-score-2496": {Confidence: score}, "unrelated-rca-2496": {Confidence: &unrelated},
					},
					ScenarioSelectors: []config.ScenarioSelectorOverride{{
						Name: "current-workflow-score-2496", Caller: "ka", Phase: "workflow_discovery",
						SignalPatterns: []string{"current-workflow-score-2496"}, WorkflowID: "selected-workflow-2496",
						ToolCall: config.ToolCallOverride{Name: "list_available_actions"},
					}},
				}, "")
				result := registry.Detect(&scenarios.DetectionContext{
					Content: `{"signal_name":"current-workflow-score-2496"}`, Caller: scenarios.CallerKA, Phase: scenarios.PhaseWorkflowDiscovery,
				})
				Expect(result).NotTo(BeNil())
				Expect(result.Scenario.Name()).To(Equal("current-workflow-score-2496"))
				withConfig, ok := result.Scenario.(scenarios.ScenarioWithConfig)
				Expect(ok).To(BeTrue())
				Expect(withConfig.Config().WorkflowID).To(Equal("selected-workflow-2496"))
				Expect(withConfig.Config().Confidence).To(Equal(expected))
			},
			Entry("explicit current score", &currentScore, .95),
			Entry("known zero remains zero", &knownZero, 0.),
			Entry("missing score does not inherit another scenario", nil, 0.),
		)

		It("UT-MOCK-2496-012 (BR-KA-016): the deployed GitOps selector supplies its own workflow confidence, not RCA confidence", func() {
			// Capture the real deployment's first external kubectl request and
			// intentionally reject it, before a Kubernetes client is constructed.
			// Registry and response configuration remain real; only kubectl is doubled.
			dir := GinkgoT().TempDir()
			configMapPath := filepath.Join(dir, "configmap.yaml")
			Expect(os.WriteFile(filepath.Join(dir, "kubectl"), []byte("#!/bin/sh\ncat > \"$MOCK_SELECTOR_CONFIGMAP\"\nexit 1\n"), 0755)).To(Succeed())
			GinkgoT().Setenv("PATH", dir+string(os.PathListSeparator)+os.Getenv("PATH"))
			GinkgoT().Setenv("MOCK_SELECTOR_CONFIGMAP", configMapPath)
			err := infrastructure.DeployMockLLMInNamespace(context.Background(), "selector-fixture", "unused-kubeconfig", "mock-image",
				map[string]string{"gitops-drift-2390-v1:staging": "gitops-staging-uuid", "gitops-drift-2390-v1:production": "gitops-production-uuid"},
				map[string]string{"interactive": "fp-int-2496"}, io.Discard)
			Expect(err).To(MatchError(ContainSubstring("failed to create Mock LLM ConfigMap")))
			manifest, err := os.ReadFile(configMapPath)
			Expect(err).NotTo(HaveOccurred())
			var configMap struct {
				Data map[string]string `yaml:"data"`
			}
			Expect(yaml.Unmarshal(manifest, &configMap)).To(Succeed())
			selectorPath := filepath.Join(dir, "selectors.yaml")
			Expect(os.WriteFile(selectorPath, []byte(configMap.Data["scenarios.yaml"]), 0644)).To(Succeed())
			overrides, err := config.LoadYAMLOverrides(selectorPath)
			Expect(err).NotTo(HaveOccurred())
			registry := scenarios.DefaultRegistryFull(overrides, "")
			result := registry.Detect(&scenarios.DetectionContext{
				Content: `{"signal_name":"FullPipelineA2ASeverityGrounding_fp_int_2496"}`,
				Caller:  scenarios.CallerKA, Phase: scenarios.PhaseWorkflowDiscovery,
			})
			Expect(result).NotTo(BeNil())
			Expect(result.Scenario.Name()).To(Equal("ka_gitops_workflow_discovery_2390"))
			withConfig, ok := result.Scenario.(scenarios.ScenarioWithConfig)
			Expect(ok).To(BeTrue())
			cfg := withConfig.Config()
			Expect(cfg.WorkflowID).To(Equal("gitops-staging-uuid"))
			Expect(cfg.Confidence).To(Equal(.95), "the selected workflow needs an explicit current score; zero correctly requires review")
		})

		It("UT-MOCK-2390-002-001: should route a suffixed FullPipeline signal to the configured GitOps workflow", func() {
			yaml := `
scenario_selectors:
  - name: "ka_gitops_workflow_discovery_2390"
    caller: "ka"
    phase: "workflow_discovery"
    signal_patterns: ["FullPipelineA2ASeverityGrounding_fp_int_"]
    workflow_id: "gitops-staging-uuid"
    action_type: "IncreaseMemoryLimits"
    tool_call:
      name: "list_available_actions"
`
			tmpFile := filepath.Join(GinkgoT().TempDir(), "overrides.yaml")
			Expect(os.WriteFile(tmpFile, []byte(yaml), 0644)).To(Succeed())

			overrides, err := config.LoadYAMLOverrides(tmpFile)
			Expect(err).NotTo(HaveOccurred())

			registry := scenarios.DefaultRegistryFull(overrides, "")
			result := registry.Detect(&scenarios.DetectionContext{
				Content: `{"signal_name":"FullPipelineA2ASeverityGrounding_fp_int_3e56c0e0"}`,
				Caller:  scenarios.CallerKA,
				Phase:   scenarios.PhaseWorkflowDiscovery,
			})
			Expect(result).NotTo(BeNil())
			Expect(result.Scenario.Name()).To(Equal("ka_gitops_workflow_discovery_2390"))

			scenarioWithCfg, ok := result.Scenario.(scenarios.ScenarioWithConfig)
			Expect(ok).To(BeTrue())
			cfg := scenarioWithCfg.Config()
			Expect(cfg.WorkflowID).To(Equal("gitops-staging-uuid"))
			Expect(cfg.ActionType).To(Equal("IncreaseMemoryLimits"))

			unrelatedResult := registry.Detect(&scenarios.DetectionContext{
				Content: `{"signal_name":"FullPipelineA2ASeverityGrounding_consent_3e56c0e0"}`,
				Caller:  scenarios.CallerKA,
				Phase:   scenarios.PhaseWorkflowDiscovery,
			})
			Expect(unrelatedResult).NotTo(BeNil())
			Expect(unrelatedResult.Scenario.Name()).NotTo(Equal("ka_gitops_workflow_discovery_2390"))
		})

		It("UT-MOCK-2390-002-002: should reject a selector with an empty signal pattern", func() {
			yaml := `
scenario_selectors:
  - name: "invalid_signal_pattern"
    signal_patterns: ["   "]
    tool_call:
      name: "list_available_actions"
`
			tmpFile := filepath.Join(GinkgoT().TempDir(), "overrides.yaml")
			Expect(os.WriteFile(tmpFile, []byte(yaml), 0644)).To(Succeed())

			_, err := config.LoadYAMLOverrides(tmpFile)
			Expect(err).To(MatchError("scenario selector signal pattern must not be empty"))
		})

		It("UT-MOCK-2390-002-003: should require keywords or signal patterns", func() {
			yaml := `
scenario_selectors:
  - name: "missing_matcher"
    tool_call:
      name: "list_available_actions"
`
			tmpFile := filepath.Join(GinkgoT().TempDir(), "overrides.yaml")
			Expect(os.WriteFile(tmpFile, []byte(yaml), 0644)).To(Succeed())

			_, err := config.LoadYAMLOverrides(tmpFile)
			Expect(err).To(MatchError("scenario selector must define at least one keyword or signal pattern"))
		})
	})
})
