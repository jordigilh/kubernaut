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
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	openai "github.com/jordigilh/kubernaut/pkg/shared/types/openai"
	"github.com/jordigilh/kubernaut/test/services/mock-llm/config"
	"github.com/jordigilh/kubernaut/test/services/mock-llm/scenarios"
)

var _ = Describe("E2E-AA-065 capacity-retry mock fixture", func() {
	It("UT-MOCK-AA-065-001: selects a phase-valid, parseable slow investigation scenario", func() {
		registry := scenarios.DefaultRegistryFull(&config.Overrides{}, "")
		result := registry.Detect(&scenarios.DetectionContext{
			Content:    "capacity-retry-controlled-investigation-test",
			SignalName: "capacity-retry-controlled-investigation-test",
		})

		Expect(result).NotTo(BeNil())
		configured, ok := result.Scenario.(scenarios.ScenarioWithConfig)
		Expect(ok).To(BeTrue())

		fixture := configured.Config()
		Expect(fixture.ToolCallName).To(Equal("kubectl_get_by_name"))
		Expect(fixture.ToolCallArgs).To(Equal(map[string]interface{}{
			"kind":      "Pod",
			"namespace": "staging",
			"name":      "capacity-retry-target",
		}))
		Expect(fixture.SecondTurnDelay).To(Equal(20 * time.Second))
		Expect(fixture.RootCause).NotTo(BeEmpty())
		Expect(fixture.InvestigationOutcome).To(Equal("inconclusive"))
		Expect(fixture.ForceText).NotTo(BeNil())
		Expect(*fixture.ForceText).To(BeFalse())

		contextConfigured, ok := result.Scenario.(scenarios.ScenarioWithContextConfig)
		Expect(ok).To(BeTrue())
		workflowFixture := contextConfigured.ConfigForContext(&scenarios.DetectionContext{
			Phase: scenarios.PhaseWorkflowDiscovery,
		})
		Expect(workflowFixture.ToolCallName).To(Equal("kubectl_get_by_name"))
		Expect(workflowFixture.ForceText).NotTo(BeNil())
		Expect(*workflowFixture.ForceText).To(BeTrue())

		advertisedFixture := contextConfigured.ConfigForContext(&scenarios.DetectionContext{
			AvailableTools: []string{openai.ToolListAvailableActions, openai.ToolSubmitResultWithWorkflow},
		})
		Expect(advertisedFixture.ForceText).NotTo(BeNil())
		Expect(*advertisedFixture.ForceText).To(BeTrue())

		rcaRetryFixture := contextConfigured.ConfigForContext(&scenarios.DetectionContext{
			Phase:          scenarios.PhaseRCA,
			AvailableTools: []string{openai.ToolSubmitResult},
		})
		Expect(rcaRetryFixture.ToolCallArgs).To(BeEmpty())
	})
})
