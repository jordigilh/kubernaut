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

func noWorkflowFoundConfig() MockScenarioConfig {
	return MockScenarioConfig{
		ScenarioName: "no_workflow_found", SignalName: "MOCK_NO_WORKFLOW_FOUND", Severity: "critical",
		Confidence: 0.0,
		RootCause:            "No suitable workflow found in catalog for this signal type",
		ResourceKind:         "Pod", ResourceNS: "production", ResourceName: "failing-pod",
		NeedsHumanReview:     BoolPtr(true),
		HumanReviewReason:    "no_matching_workflows",
		InvestigationOutcome: "inconclusive",
	}
}

func lowConfidenceConfig() MockScenarioConfig {
	return MockScenarioConfig{
		ScenarioName: "low_confidence", SignalName: "MOCK_LOW_CONFIDENCE", Severity: "critical",
		WorkflowName: "generic-restart-v1", WorkflowID: uuid.DeterministicUUID("generic-restart-v1"),
		WorkflowTitle: "Generic Pod Restart", Confidence: 0.35,
		RootCause:            "Multiple possible root causes identified, requires human judgment",
		ResourceKind:         "Pod", ResourceNS: "production", ResourceName: "ambiguous-pod",
		Parameters:           map[string]string{"NAMESPACE": "production", "POD_NAME": "ambiguous-pod"},
		InvestigationOutcome: "actionable",
		IsActionable:         BoolPtr(true),
	}
}

func problemResolvedConfig() MockScenarioConfig {
	return MockScenarioConfig{
		ScenarioName: "problem_resolved", SignalName: "MOCK_PROBLEM_RESOLVED", Severity: "low",
		Confidence: 0.85,
		RootCause:            "Problem self-resolved through auto-scaling or transient condition cleared",
		ResourceKind:         "Pod", ResourceNS: "production", ResourceName: "recovered-pod",
		Contributing:         []string{"Transient condition", "Auto-recovery"},
		InvestigationOutcome: "problem_resolved",
		IsActionable:         BoolPtr(false),
	}
}

func problemResolvedContradictionConfig() MockScenarioConfig {
	return MockScenarioConfig{
		ScenarioName: "problem_resolved_contradiction", SignalName: "MOCK_PROBLEM_RESOLVED_CONTRADICTION", Severity: "low",
		Confidence: 0.85,
		RootCause:            "Problem self-resolved. Transient OOM cleared after pod restart",
		ResourceKind:         "Pod", ResourceNS: "production", ResourceName: "recovered-pod",
		Contributing:         []string{"Transient condition", "Auto-recovery"},
		NeedsHumanReview:     BoolPtr(true),
		HumanReviewReason:    "contradictory_signals",
		InvestigationOutcome: "problem_resolved",
		IsActionable:         BoolPtr(false),
	}
}

func maxRetriesExhaustedConfig() MockScenarioConfig {
	return MockScenarioConfig{
		ScenarioName: "max_retries_exhausted", SignalName: "MOCK_MAX_RETRIES_EXHAUSTED", Severity: "high",
		Confidence: 0.0,
		RootCause:            "LLM analysis completed but failed validation after maximum retry attempts. Response format was unparseable or contained invalid data.",
		ResourceKind:         "Pod", ResourceNS: "production", ResourceName: "failed-analysis-pod",
		NeedsHumanReview:     BoolPtr(true),
		HumanReviewReason:    "llm_parsing_error",
		InvestigationOutcome: "inconclusive",
	}
}

func rcaIncompleteConfig() MockScenarioConfig {
	return MockScenarioConfig{
		ScenarioName: "rca_incomplete", SignalName: "MOCK_RCA_INCOMPLETE", Severity: "critical",
		WorkflowName: "generic-restart-v1", WorkflowID: uuid.DeterministicUUID("generic-restart-v1"),
		WorkflowTitle: "Generic Pod Restart", Confidence: 0.88,
		RootCause:            "Root cause identified but affected resource could not be determined from signal context",
		ResourceKind:         "Pod", ResourceNS: "production", ResourceName: "ambiguous-pod",
		APIVersion:           "v1",
		Parameters:           map[string]string{"NAMESPACE": "production", "POD_NAME": "ambiguous-pod"},
		NeedsHumanReview:     BoolPtr(true),
		HumanReviewReason:    "investigation_inconclusive",
		InvestigationOutcome: "actionable",
		IsActionable:         BoolPtr(true),
	}
}
