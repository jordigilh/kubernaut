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

// ambiguousKindConfig returns a scenario where the remediation target uses a
// Kind that exists in multiple API groups (TestWidget in alpha and beta groups).
// APIVersion is intentionally empty to trigger the apiVersionValidationGate.
// Issue #1044.
func ambiguousKindConfig() MockScenarioConfig {
	return MockScenarioConfig{
		ScenarioName:         "ambiguous_kind",
		SignalName:           "AmbiguousKindTestSignal",
		Severity:             "high",
		WorkflowName:         "ambiguous-kind-fix-v1",
		WorkflowID:           uuid.DeterministicUUID("ambiguous-kind-fix-v1"),
		Confidence:           0.85,
		Rationale:            "TestWidget misconfiguration requires reconfiguration of the resource spec",
		RootCause:            "TestWidget misconfigured in default namespace",
		ResourceKind:         "TestWidget",
		ResourceNS:           "default",
		ResourceName:         "test-widget-instance",
		InvestigationOutcome: "actionable",
		IsActionable:         BoolPtr(true),
		ExecutionEngine:      "job",
		Contributing:         []string{"misconfigured_spec", "missing_api_version"},
	}
}
