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

func crashloopConfig() MockScenarioConfig {
	return MockScenarioConfig{
		ScenarioName: "crashloop", SignalName: "CrashLoopBackOff", Severity: "high",
		WorkflowName: "crashloop-config-fix-v1", WorkflowID: uuid.DeterministicUUID("crashloop-config-fix-v1"),
		WorkflowTitle: "CrashLoopBackOff - Configuration Fix", Confidence: 0.95,
		Rationale:    "Configuration regression introduced in recent deployment revision; rollback to last known good revision is the safest approach with medium risk tolerance",
		RootCause:    "Container failing due to invalid configuration directive introduced in recent deployment update",
		ResourceKind: "Deployment", ResourceNS: "staging", ResourceName: "worker",
		APIVersion: "apps/v1",
		Parameters: map[string]string{
			"NAMESPACE":       "staging",
			"DEPLOYMENT_NAME": "worker",
			"CONFIGMAP_NAME":  "crashloop-app-config",
			"CONFIGMAP_KEY":   "APP_MODE",
			"CONFIGMAP_VALUE": "healthy",
		},
		Contributing: []string{"invalid_configuration_directive", "recent_deployment_update", "application_fails_validation_on_startup"},
		Alternatives: []MockAlternativeWorkflow{
			{WorkflowName: "generic-restart-v1", WorkflowID: uuid.DeterministicUUID("generic-restart-v1"), Confidence: 0.60, Rationale: "Restart-only approach would be faster but doesn't address the root cause (bad configuration)"},
		},
		InvestigationOutcome: "actionable",
		IsActionable:         BoolPtr(true),
		ExecutionEngine:      "job",
	}
}

// crashloopScenario matches the explicit "crashloop" signal name at high
// confidence. The generic Kubernetes "BackOff" reason is ambiguous -- it is
// shared by any crash-looping container, including OOM-induced restarts --
// so it only matches "backoff" when broader investigation content confirms a
// config-driven crashloop (the resource name contains "crashloop", as with
// the crashloop-app fixture). This prevents the mandatory
// CONFIGMAP_NAME/KEY/VALUE parameters required by crashloop-config-fix-v1's
// real remediation script (BR-WE-014, Issue #1542) from being applied to
// unrelated scenarios (e.g. memory-eater OOM tests) where no such ConfigMap
// exists, which previously caused the Job to fail fast ("configmap not
// found"). See root cause: fullpipeline CI runs 28717432528/28718786969.
func crashloopScenario() *configScenario {
	cfg := crashloopConfig()
	return &configScenario{
		config: cfg,
		matchFunc: func(ctx *DetectionContext) (bool, float64) {
			signal := extractSignal(ctx)
			if signal == "" {
				return false, 0
			}
			if strings.Contains(signal, "crashloop") {
				return true, 0.8
			}
			if strings.Contains(signal, "backoff") && strings.Contains(ctx.AllText, "crashloop") {
				return true, 0.8
			}
			return false, 0
		},
	}
}
