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
	"strings"

	"github.com/jordigilh/kubernaut/pkg/shared/uuid"
)

func oomkilledConfig() MockScenarioConfig {
	return MockScenarioConfig{
		ScenarioName: "oomkilled", SignalName: "OOMKilled", Severity: "critical",
		WorkflowName: "oomkill-increase-memory-v1", WorkflowID: uuid.DeterministicUUID("oomkill-increase-memory-v1"),
		WorkflowTitle: "OOMKill Recovery - Increase Memory Limits", Confidence: 0.95,
		Rationale:    "Container exceeded memory limits under traffic spike; increasing limits is the safest remediation with medium risk tolerance",
		RootCause:    "Container exceeded memory limits due to traffic spike",
		ResourceKind: "Deployment", ResourceNS: "production", ResourceName: "api-server",
		APIVersion: "apps/v1",
		Parameters: map[string]string{"MEMORY_LIMIT_NEW": "512Mi"}, ExecutionEngine: "job",
		Contributing: []string{"traffic_spike", "insufficient_memory_limits", "no_HPA_configured"},
		Alternatives: []MockAlternativeWorkflow{
			{WorkflowName: "generic-restart-v1", WorkflowID: uuid.DeterministicUUID("generic-restart-v1"), Confidence: 0.60, Rationale: "Restart would temporarily resolve the OOM but doesn't address the underlying memory limit issue", Parameters: map[string]string{"REPLICA_COUNT": "3"}},
		},
		InvestigationOutcome: "actionable",
		IsActionable:         BoolPtr(true),
	}
}

// oomkilledScenario matches explicit OOM signal names at high confidence and
// falls back to the generic Kubernetes "BackOff" crash-loop reason at lower
// confidence. "BackOff" alone is ambiguous -- it fires for ANY crash-looping
// container regardless of root cause (OOM vs. config error) -- so the
// fallback must not outrank a genuine crashloop-app match (see
// crashloopScenario, which requires positive "crashloop" evidence for the
// same "backoff" signal). Issue #1542 follow-up.
func oomkilledScenario() *configScenario {
	cfg := oomkilledConfig()
	highConfidencePatterns := []string{"memoryexceedslimit", "memoryexceeds", "oomkilled", "oomkill"}
	return &configScenario{
		config: cfg,
		matchFunc: func(ctx *DetectionContext) (bool, float64) {
			signal := extractSignal(ctx)
			if signal == "" {
				return false, 0
			}
			for _, p := range highConfidencePatterns {
				if strings.Contains(signal, p) {
					return true, 0.8
				}
			}
			if strings.Contains(signal, "backoff") {
				return true, 0.5
			}
			return false, 0
		},
	}
}
