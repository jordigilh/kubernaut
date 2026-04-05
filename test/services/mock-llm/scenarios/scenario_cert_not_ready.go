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

func certNotReadyConfig() MockScenarioConfig {
	return MockScenarioConfig{
		ScenarioName: "cert_not_ready", SignalName: "CertManagerCertNotReady", Severity: "critical",
		WorkflowName: "fix-certificate-v1", WorkflowID: uuid.DeterministicUUID("fix-certificate-v1"),
		WorkflowTitle: "Fix Certificate - Recreate CA Secret", Confidence: 0.92,
		RootCause:        "cert-manager Certificate stuck in NotReady state due to missing or corrupted CA Secret backing the ClusterIssuer",
		ResourceKind:     "Certificate", ResourceNS: "default", ResourceName: "demo-app-cert",
		APIVersion:       "cert-manager.io/v1",
		OverrideResource: true,
		Parameters: map[string]string{
			"TARGET_NAMESPACE":   "default",
			"TARGET_CERTIFICATE": "demo-app-cert",
			"ISSUER_NAME":       "demo-selfsigned-ca",
			"CA_SECRET_NAME":    "demo-ca-key-pair",
		},
		ExecutionEngine:      "job",
		InvestigationOutcome: "actionable",
		IsActionable:         BoolPtr(true),
	}
}
