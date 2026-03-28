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

func testSignalConfig() MockScenarioConfig {
	return MockScenarioConfig{
		ScenarioName: "test_signal", SignalName: "TestSignal", Severity: "critical",
		WorkflowName: "test-signal-handler-v1", WorkflowID: uuid.DeterministicUUID("test-signal-handler-v1"),
		WorkflowTitle: "Test Signal Handler", Confidence: 0.90,
		RootCause: "Test signal for graceful shutdown validation",
		ResourceKind: "Pod", ResourceNS: "test", ResourceName: "test-pod",
		Parameters: map[string]string{"NAMESPACE": "test", "POD_NAME": "test-pod"},
	}
}

func testSignalScenario() *configScenario {
	cfg := testSignalConfig()
	return &configScenario{
		config: cfg,
		matchFunc: func(ctx *DetectionContext) (bool, float64) {
			lower := strings.ToLower(ctx.Content)
			if strings.Contains(lower, "testsignal") || strings.Contains(lower, "test signal") {
				return true, 0.95
			}
			return false, 0
		},
	}
}
