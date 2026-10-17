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

package scenarios

import (
	"time"

	openai "github.com/jordigilh/kubernaut/pkg/shared/types/openai"
)

type capacityRetrySlowInvestigationScenario struct {
	*configScenario
}

func newCapacityRetrySlowInvestigationScenario() *capacityRetrySlowInvestigationScenario {
	return &capacityRetrySlowInvestigationScenario{
		configScenario: newKeywordScenario(
			"capacity_retry_slow_investigation",
			"capacity-retry-controlled-investigation-test",
			capacityRetrySlowInvestigationConfig(),
		),
	}
}

// ConfigForContext keeps the resource-context tool in the RCA phase only.
// Workflow discovery submits the explicit no-workflow sentinel so the typed
// outcome contract records completed_no_match rather than inferring failure
// from an absent selected workflow.
func (s *capacityRetrySlowInvestigationScenario) ConfigForContext(ctx *DetectionContext) MockScenarioConfig {
	cfg := s.config
	cfg.ForceText = BoolPtr(false)
	workflowDiscovery := false
	rcaSubmitOnly := false
	if ctx != nil {
		workflowDiscovery = ctx.Phase == PhaseWorkflowDiscovery || ctx.Phase == PhaseWorkflowSelection
		rcaSubmitOnly = len(ctx.AvailableTools) == 1 && ctx.AvailableTools[0] == openai.ToolSubmitResult
		for _, tool := range ctx.AvailableTools {
			if tool == openai.ToolSubmitResultWithWorkflow || tool == openai.ToolSubmitResultNoWorkflow {
				workflowDiscovery = true
				break
			}
		}
	}
	if workflowDiscovery {
		cfg.ToolCallName = openai.ToolSubmitResultNoWorkflow
		cfg.ToolCallArgs = nil
	}
	if rcaSubmitOnly {
		// The response builder owns submit_result's structured RCA payload. A
		// custom tool-argument map would otherwise replace that payload with
		// kubectl_get_by_name arguments during same-kind gate retries.
		cfg.ToolCallArgs = nil
	}
	return cfg
}

// capacityRetrySlowInvestigationConfig keeps the first capacity wave active
// without relying on a tool that the trusted RCA phase rejects. The dedicated
// fixture is intentionally separate from slowInvestigationConfig, which is
// used by older tests with different resource and tool assumptions.
func capacityRetrySlowInvestigationConfig() MockScenarioConfig {
	return MockScenarioConfig{
		ScenarioName: "capacity_retry_slow_investigation",
		SignalName:   "capacity-retry-controlled-investigation-test",
		Severity:     "warning",
		RootCause:    "Deterministic capacity-retry investigation fixture",
		ResourceKind: "Pod",
		ResourceNS:   "staging",
		ResourceName: "capacity-retry-target",
		APIVersion:   "v1",
		Contributing: []string{"capacity-retry-fixture", "controlled-investigation-duration"},
		// No workflow is intentional: E2E-AA-065 validates KA dispatch-capacity
		// retry/convergence, not workflow catalog selection.
		InvestigationOutcome: "inconclusive",
		ToolCallName:         "kubectl_get_by_name",
		ToolCallArgs: map[string]interface{}{
			"kind":      "Pod",
			"namespace": "staging",
			"name":      "capacity-retry-target",
		},
		ForceText:       BoolPtr(false),
		SecondTurnDelay: 20 * time.Second,
	}
}
