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

import openai "github.com/jordigilh/kubernaut/pkg/shared/types/openai"

func injectionConfigmapReadConfig() MockScenarioConfig {
	forceText := false
	return MockScenarioConfig{
		ScenarioName:         "injection_configmap_read",
		SignalName:           "injection_configmap_read",
		Severity:             "critical",
		RootCause:            "Prompt injection detected in ConfigMap content",
		Confidence:           0.50,
		ResourceKind:         "ConfigMap",
		ResourceNS:           "default",
		ResourceName:         "poisoned-cm",
		InvestigationOutcome: "inconclusive",
		NeedsHumanReview:     BoolPtr(true),
		HumanReviewReason:    "prompt_injection_detected",
		ForceText:            &forceText,
		ToolCallName:         openai.ToolKubectlGetYAML,
	}
}
