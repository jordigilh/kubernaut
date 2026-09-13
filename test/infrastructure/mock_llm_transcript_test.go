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
package infrastructure

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"gopkg.in/yaml.v3"

	"github.com/jordigilh/kubernaut/test/services/mock-llm/config"
)

var _ = Describe("Mock LLM transcript wiring", func() {
	It("IT-ML-2410-006: emits valid Issue 2390 transcript YAML", func() {
		raw := gitOpsInteractiveInvestigationScenarioYAML("fp-a2a-interactive", "gitops-workflow-uuid")
		var overrides config.Overrides
		Expect(yaml.Unmarshal([]byte(raw), &overrides)).To(Succeed())
		Expect(overrides.TranscriptScenarios).To(HaveLen(1))
		Expect(overrides.TranscriptScenarios[0].Name).To(Equal("af_gitops_interactive_2390"))
		Expect(overrides.TranscriptScenarios[0].Steps).To(HaveLen(4))
		Expect(overrides.TranscriptScenarios[0].Steps[0].ToolCall.Name).To(Equal("kubernaut_investigate"))
		Expect(overrides.TranscriptScenarios[0].Steps[2].ToolCall.Arguments).To(HaveKeyWithValue("workflow_id", "gitops-workflow-uuid"))
		Expect(overrides.TranscriptScenarios[0].Steps[3].ToolCall.Arguments).To(HaveKeyWithValue("name", "$from_tool:kubernaut_investigate:rr_id"))
	})
})
