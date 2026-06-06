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

// istioAuthzConfig returns a mock LLM scenario for cross-resource RCA where
// the alert targets a Deployment but the root cause is an AuthorizationPolicy.
// Used by E2E-KA-DISC-005 (#1374).
func istioAuthzConfig() MockScenarioConfig {
	return MockScenarioConfig{
		ScenarioName:  "istio_authz",
		SignalName:    "IstioHighDenyRate",
		Severity:      "critical",
		WorkflowName:  "istio-authz-fix-v1",
		WorkflowID:    uuid.DeterministicUUID("istio-authz-fix-v1"),
		WorkflowTitle: "Istio AuthorizationPolicy Fix",
		Confidence:    0.92,
		Rationale:     "High deny rate caused by overly restrictive AuthorizationPolicy; fix policy rules",
		RootCause:     "AuthorizationPolicy deny-all-traffic is blocking legitimate traffic to the API server deployment",
		ResourceKind:  "AuthorizationPolicy",
		ResourceNS:    "production",
		ResourceName:  "deny-all-traffic",
		APIVersion:    "security.istio.io/v1",
		Parameters:    map[string]string{"POLICY_ACTION": "ALLOW"},
		ExecutionEngine: "job",
		Contributing:  []string{"overly_restrictive_authz_policy", "missing_allow_rule", "istio_sidecar_injection_enabled"},
		InvestigationOutcome: "actionable",
		IsActionable:         BoolPtr(true),
	}
}
