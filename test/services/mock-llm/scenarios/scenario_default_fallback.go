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

import "github.com/jordigilh/kubernaut/pkg/shared/uuid"

func defaultConfig() MockScenarioConfig {
	return MockScenarioConfig{
		ScenarioName: "default", SignalName: "Unknown", Severity: "medium",
		WorkflowName: "generic-restart-v1", WorkflowID: uuid.DeterministicUUID("generic-restart-v1"),
		WorkflowTitle: "Generic Pod Restart", Confidence: 0.75,
		RootCause:            "Unable to determine specific root cause",
		ResourceKind:         "Pod", ResourceNS: "default", ResourceName: "test-pod",
		Contributing:         []string{"traffic_spike", "resource_limits"},
		InvestigationOutcome: "actionable",
		IsActionable:         BoolPtr(true),
	}
}

func defaultFallbackScenario() *configScenario {
	cfg := defaultConfig()
	return &configScenario{
		config: cfg,
		matchFunc: func(_ *DetectionContext) (bool, float64) {
			return true, 0.01
		},
	}
}
