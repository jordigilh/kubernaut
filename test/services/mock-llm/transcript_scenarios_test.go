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

var _ = Describe("Explicit A2A Transcript Scenarios", func() {
	It("UT-ML-2410-001: parses ordered transcript steps", func() {
		overrides := loadTranscriptOverrides(`
transcript_scenarios:
  - name: gitops-journey
    steps:
      - user: "investigate GitOps remediation"
        tool_call:
          name: kubernaut_investigate
      - user: "discover available workflows"
        tool_call:
          name: kubernaut_discover_workflows
`)

		Expect(overrides.TranscriptScenarios).To(HaveLen(1))
		Expect(overrides.TranscriptScenarios[0].Steps).To(HaveLen(2))
		Expect(overrides.TranscriptScenarios[0].Steps[1].ToolCall.Name).To(Equal("kubernaut_discover_workflows"))
	})

	It("UT-ML-2410-002: matches the complete current user turn only", func() {
		registry := scenarios.DefaultRegistryFull(&config.Overrides{
			TranscriptScenarios: []config.TranscriptScenarioOverride{{
				Name: "gitops-journey",
				Steps: []config.TranscriptStepOverride{{
					User:     "discover available workflows",
					ToolCall: config.ToolCallOverride{Name: "kubernaut_discover_workflows"},
				}},
			}},
		}, "")

		result := registry.Detect(&scenarios.DetectionContext{
			AllText:         "assistant mentioned discover available workflows",
			LastUserContent: "please discover available workflows now",
		})
		Expect(result).NotTo(BeNil())
		Expect(result.Scenario.Name()).NotTo(Equal("gitops-journey"))

		result = registry.Detect(&scenarios.DetectionContext{
			AllText:         "assistant mentioned discover available workflows",
			LastUserContent: "DISCOVER AVAILABLE WORKFLOWS",
		})
		Expect(result).NotTo(BeNil())
		Expect(result.Scenario.Name()).To(Equal("gitops-journey"))
		Expect(result.Confidence).To(Equal(1.05))
	})

	It("UT-ML-2410-003: selects step-specific arguments without mutating the transcript", func() {
		overrides := &config.Overrides{
			TranscriptScenarios: []config.TranscriptScenarioOverride{
				{
					Name: "gitops-journey",
					Steps: []config.TranscriptStepOverride{
						{User: "investigate", ToolCall: config.ToolCallOverride{
							Name:      "kubernaut_investigate",
							Arguments: map[string]interface{}{"namespace": "staging"},
						}},
						{User: "discover", ToolCall: config.ToolCallOverride{
							Name:      "kubernaut_discover_workflows",
							Arguments: map[string]interface{}{"rr_id": "$from_tool:kubernaut_investigate:rr_id"},
						}},
					},
				},
			},
		}
		registry := scenarios.DefaultRegistryFull(overrides, "")

		first := registry.Detect(&scenarios.DetectionContext{LastUserContent: "investigate"})
		second := registry.Detect(&scenarios.DetectionContext{LastUserContent: "discover"})
		Expect(first).NotTo(BeNil())
		Expect(second).NotTo(BeNil())
		firstConfig := first.Scenario.(scenarios.ScenarioWithContextConfig).ConfigForContext(&scenarios.DetectionContext{LastUserContent: "investigate"})
		secondConfig := second.Scenario.(scenarios.ScenarioWithContextConfig).ConfigForContext(&scenarios.DetectionContext{LastUserContent: "discover"})
		Expect(firstConfig.ToolCallArgs).To(HaveKeyWithValue("namespace", "staging"))
		Expect(secondConfig.ToolCallArgs).To(HaveKeyWithValue("rr_id", "$from_tool:kubernaut_investigate:rr_id"))
		firstConfig.ToolCallArgs["namespace"] = "mutated"
		freshFirstConfig := first.Scenario.(scenarios.ScenarioWithContextConfig).ConfigForContext(&scenarios.DetectionContext{LastUserContent: "investigate"})
		Expect(freshFirstConfig.ToolCallArgs).To(HaveKeyWithValue("namespace", "staging"))
	})

	It("UT-ML-2410-004: enables the existing repeat guard for each transcript step", func() {
		registry := scenarios.DefaultRegistryFull(&config.Overrides{TranscriptScenarios: []config.TranscriptScenarioOverride{{
			Name:  "journey",
			Steps: []config.TranscriptStepOverride{{User: "investigate", ToolCall: config.ToolCallOverride{Name: "kubernaut_investigate"}}},
		}}}, "")
		result := registry.Detect(&scenarios.DetectionContext{LastUserContent: "investigate"})
		cfg := result.Scenario.(scenarios.ScenarioWithContextConfig).ConfigForContext(&scenarios.DetectionContext{LastUserContent: "investigate"})
		Expect(cfg.RepeatToolCall).To(BeTrue())
		Expect(cfg.ForceText).To(Equal(scenarios.BoolPtr(false)))
	})

	It("UT-ML-2410-005: rejects incomplete transcript definitions", func() {
		path := filepath.Join(GinkgoT().TempDir(), "overrides.yaml")
		Expect(os.WriteFile(path, []byte("transcript_scenarios:\n  - name: broken\n    steps:\n      - user: investigate\n"), 0644)).To(Succeed())
		_, err := config.LoadYAMLOverrides(path)
		Expect(err).To(MatchError("transcript step tool call name is required"))
	})
})

func loadTranscriptOverrides(yamlContent string) *config.Overrides {
	path := filepath.Join(GinkgoT().TempDir(), "overrides.yaml")
	Expect(os.WriteFile(path, []byte(yamlContent), 0644)).To(Succeed())
	overrides, err := config.LoadYAMLOverrides(path)
	Expect(err).NotTo(HaveOccurred())
	return overrides
}
