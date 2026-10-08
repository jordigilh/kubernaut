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
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"net/http/httptest"
	"os"
	"path/filepath"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	openai "github.com/jordigilh/kubernaut/pkg/shared/types/openai"
	"github.com/jordigilh/kubernaut/test/services/mock-llm/config"
	"github.com/jordigilh/kubernaut/test/services/mock-llm/handlers"
	"github.com/jordigilh/kubernaut/test/services/mock-llm/scenarios"
	"gopkg.in/yaml.v3"
)

var _ = Describe("APIFRONTEND structured decision selector", func() {
	Describe("UT-MOCK-AF-1395-001: structured decision selector survives reinvocation", func() {
		It("should keep the structured decision scenario selected after a synthetic continuation", func() {
			manifest, err := os.ReadFile("../../../deploy/apifrontend/overlays/e2e/mock-llm.yaml")
			Expect(err).NotTo(HaveOccurred())

			decoder := yaml.NewDecoder(bytes.NewReader(manifest))
			var scenarioConfig string
			for {
				var document struct {
					Kind string            `yaml:"kind"`
					Data map[string]string `yaml:"data"`
				}
				err := decoder.Decode(&document)
				if errors.Is(err, io.EOF) {
					break
				}
				Expect(err).NotTo(HaveOccurred())
				if document.Kind == "ConfigMap" {
					scenarioConfig = document.Data["config.yaml"]
					break
				}
			}
			Expect(scenarioConfig).NotTo(BeEmpty(), "E2E ConfigMap should contain the mock-LLM scenario config")

			configPath := filepath.Join(GinkgoT().TempDir(), "mock-llm-e2e.yaml")
			Expect(os.WriteFile(configPath, []byte(scenarioConfig), 0644)).To(Succeed())
			overrides, err := config.LoadYAMLOverrides(configPath)
			Expect(err).NotTo(HaveOccurred())

			registry := scenarios.DefaultRegistryWithOverrides(overrides)
			result := registry.Detect(&scenarios.DetectionContext{
				Content:         "present structured rca decision",
				AllText:         "user present structured rca decision assistant analysis user Continue the investigation.",
				LastUserContent: "Continue the investigation. If you need more information, use the available tools. If the investigation is complete, summarize your findings.",
			})
			Expect(result).NotTo(BeNil())
			Expect(result.Scenario.Name()).To(Equal("af_structured_decision"))

			scenarioWithConfig, ok := result.Scenario.(scenarios.ScenarioWithConfig)
			Expect(ok).To(BeTrue())
			Expect(scenarioWithConfig.Config().ToolCallName).To(Equal("kubernaut_present_decision"))
		})
	})
})

var _ = Describe("APIFRONTEND progressive no-RCA selector", func() {
	It("UT-MOCK-AF-1407-004: scopes the empty RCA response to KA extraction for #1407", func() {
		manifest, err := os.ReadFile("../../../deploy/apifrontend/overlays/e2e/mock-llm.yaml")
		Expect(err).NotTo(HaveOccurred())

		decoder := yaml.NewDecoder(bytes.NewReader(manifest))
		var scenarioConfig string
		for {
			var document struct {
				Kind string            `yaml:"kind"`
				Data map[string]string `yaml:"data"`
			}
			err := decoder.Decode(&document)
			if errors.Is(err, io.EOF) {
				break
			}
			Expect(err).NotTo(HaveOccurred())
			if document.Kind == "ConfigMap" {
				scenarioConfig = document.Data["config.yaml"]
				break
			}
		}
		Expect(scenarioConfig).NotTo(BeEmpty())

		configPath := filepath.Join(GinkgoT().TempDir(), "mock-llm-e2e.yaml")
		Expect(os.WriteFile(configPath, []byte(scenarioConfig), 0644)).To(Succeed())
		overrides, err := config.LoadYAMLOverrides(configPath)
		Expect(err).NotTo(HaveOccurred())

		var selector *config.ScenarioSelectorOverride
		for i := range overrides.ScenarioSelectors {
			if overrides.ScenarioSelectors[i].Name == "af_progressive_no_rca" {
				selector = &overrides.ScenarioSelectors[i]
				break
			}
		}
		Expect(selector).NotTo(BeNil(), "#1407 must have a dedicated no-RCA selector")
		Expect(selector.Caller).To(Equal("ka"))
		Expect(selector.Phase).To(Equal("rca"))
		Expect(selector.SignalPatterns).To(ContainElement("AFProgressiveGrounding"))
		Expect(selector.ToolCall.Name).To(Equal("submit_result"))

		registry := scenarios.DefaultRegistryWithOverrides(overrides)
		result := registry.Detect(&scenarios.DetectionContext{
			Content:        "Signal Name: AFProgressiveGrounding",
			AllText:        "Signal Name: AFProgressiveGrounding",
			Caller:         scenarios.CallerKA,
			Phase:          scenarios.PhaseRCA,
			AvailableTools: []string{"submit_result"},
		})
		Expect(result).NotTo(BeNil())
		Expect(result.Scenario.Name()).To(Equal("af_progressive_no_rca"))

		initialInvestigation := registry.Detect(&scenarios.DetectionContext{
			Content:        "Signal Name: AFProgressiveGrounding",
			AllText:        "Signal Name: AFProgressiveGrounding",
			Caller:         scenarios.CallerAF,
			Phase:          scenarios.PhaseInvestigation,
			AvailableTools: []string{"kubernaut_investigate"},
		})
		if initialInvestigation != nil {
			Expect(initialInvestigation.Scenario.Name()).NotTo(Equal("af_progressive_no_rca"),
				"the no-RCA override must not replace the initial AF investigation call")
		}

		server := httptest.NewServer(handlers.NewRouter(registry, false, config.ModeFull))
		defer server.Close()
		prompt := "Signal Name: AFProgressiveGrounding"
		response := postOpenAI(server.URL, openai.ChatCompletionRequest{
			Model:    "mock-model",
			Messages: []openai.Message{{Role: "user", Content: &prompt}},
			Tools: []openai.Tool{{
				Type:     "function",
				Function: openai.ToolDefinition{Name: openai.ToolSubmitResult},
			}},
		})
		Expect(response.Choices).To(HaveLen(1))
		Expect(response.Choices[0].Message.ToolCalls).To(HaveLen(1))
		Expect(response.Choices[0].Message.ToolCalls[0].Function.Name).To(Equal(openai.ToolSubmitResult))

		var arguments map[string]any
		Expect(json.Unmarshal([]byte(response.Choices[0].Message.ToolCalls[0].Function.Arguments), &arguments)).To(Succeed())
		rca, ok := arguments["root_cause_analysis"].(map[string]any)
		Expect(ok).To(BeTrue())
		Expect(rca["summary"]).To(Equal(""))
		Expect(rca["severity"]).To(Equal(""))
	})
})
