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

	openai "github.com/jordigilh/kubernaut/pkg/shared/types/openai"
	"github.com/jordigilh/kubernaut/test/services/mock-llm/scenarios"
)

var _ = Describe("Injection Scenario (BR-TESTING-657)", func() {

	Describe("UT-MOCK-657-009: Registry detects injection_configmap_read signal", func() {
		It("should detect the injection scenario from the signal name", func() {
			registry := scenarios.DefaultRegistry()
			ctx := &scenarios.DetectionContext{
				Content: "- Signal Name: injection_configmap_read\n- Namespace: default",
				AllText: "- Signal Name: injection_configmap_read\n- Namespace: default",
			}
			result := registry.Detect(ctx)
			Expect(result).NotTo(BeNil(), "expected injection scenario to be detected")
			Expect(result.Scenario.Name()).To(Equal("injection_configmap_read"))
		})
	})

	Describe("UT-MOCK-657-010: Injection scenario config has correct defaults", func() {
		It("should have ToolCallName=kubectl_get_yaml and ConfigMap resource target", func() {
			registry := scenarios.DefaultRegistry()
			ctx := &scenarios.DetectionContext{
				Content: "- Signal Name: injection_configmap_read\n- Namespace: default",
				AllText: "- Signal Name: injection_configmap_read\n- Namespace: default",
			}
			result := registry.Detect(ctx)
			Expect(result).NotTo(BeNil())

			cfgScenario, ok := result.Scenario.(scenarios.ScenarioWithConfig)
			Expect(ok).To(BeTrue(), "scenario should implement ScenarioWithConfig")

			cfg := cfgScenario.Config()
			Expect(cfg.ToolCallName).To(Equal(openai.ToolKubectlGetYAML))
			Expect(cfg.ResourceKind).To(Equal("ConfigMap"))
			Expect(cfg.ForceText).NotTo(BeNil(), "ForceText should be set")
			Expect(*cfg.ForceText).To(BeFalse(), "ForceText should be false to allow tool calls")
		})
	})
})
