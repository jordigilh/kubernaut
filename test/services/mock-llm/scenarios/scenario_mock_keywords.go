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
		Confidence:   0.0,
		RootCause:    "No suitable workflow found in catalog for this signal type",
		ResourceKind: "Pod", ResourceNS: "production", ResourceName: "failing-pod",
		APIVersion:           "v1",
		InvestigationOutcome: "inconclusive",
	}
}

func lowConfidenceConfig() MockScenarioConfig {
	return MockScenarioConfig{
		ScenarioName: "low_confidence", SignalName: "MOCK_LOW_CONFIDENCE", Severity: "critical",
		WorkflowName: "generic-restart-v1", WorkflowID: uuid.DeterministicUUID("generic-restart-v1"),
		WorkflowTitle: "Generic Pod Restart", Confidence: 0.35,
		Rationale:    "Multiple possible root causes identified; generic restart is safest but requires human judgment to confirm",
		RootCause:    "Multiple possible root causes identified, requires human judgment",
		ResourceKind: "Pod", ResourceNS: "production", ResourceName: "ambiguous-pod",
		APIVersion:   "v1",
		Parameters:   map[string]string{"NAMESPACE": "production", "POD_NAME": "ambiguous-pod"},
		Contributing: []string{"ambiguous_root_cause", "multiple_correlated_signals"},
		Alternatives: []MockAlternativeWorkflow{
			{WorkflowName: "oomkill-increase-memory-v1", WorkflowID: uuid.DeterministicUUID("oomkill-increase-memory-v1"), Confidence: 0.30, Rationale: "Alternative approach for ambiguous root cause"},
			{WorkflowName: "node-drain-reboot-v1", WorkflowID: uuid.DeterministicUUID("node-drain-reboot-v1"), Confidence: 0.20, Rationale: "Requires human expertise to determine correct remediation"},
		},
		InvestigationOutcome: "actionable",
		IsActionable:         BoolPtr(true),
	}
}

func problemResolvedConfig() MockScenarioConfig {
	return MockScenarioConfig{
		ScenarioName: "problem_resolved", SignalName: "MOCK_PROBLEM_RESOLVED", Severity: "info",
		Confidence:   0.85,
		RootCause:    "Problem self-resolved through auto-scaling or transient condition cleared",
		ResourceKind: "Pod", ResourceNS: "production", ResourceName: "recovered-pod",
		APIVersion:           "v1",
		Contributing:         []string{"Transient condition", "Auto-recovery"},
		InvestigationOutcome: "problem_resolved",
		IsActionable:         BoolPtr(false),
	}
}

// reasoningCaptureConfig simulates a DeepSeek/vLLM-style OpenAI-compatible
// reasoning model for KA's openaicompat reasoning-capture E2E test
// (E2E-KA-AUDIT-001, BR-AI-086 AC6, #1578): the response carries a
// reasoning_content field alongside the submit_result_with_workflow tool
// call, proving the full pipeline (capture -> InvestigationResult ->
// audit trail -> DataStorage, correlation_id-reconstructable per SOC2
// CC8.1) without requiring a real reasoning-capable provider.
func reasoningCaptureConfig() MockScenarioConfig {
	return MockScenarioConfig{
		ScenarioName: "mock_reasoning_capture", SignalName: "MOCK_REASONING_CAPTURE", Severity: "critical",
		WorkflowName: "oomkill-increase-memory-v1", WorkflowID: uuid.DeterministicUUID("oomkill-increase-memory-v1"),
		WorkflowTitle: "OOMKill Recovery - Increase Memory Limits", Confidence: 0.92,
		Rationale:    "Sustained memory climb over 6h rules out a transient spike; increasing limits addresses the sustained leak",
		RootCause:    "Container exceeded memory limits due to a sustained memory leak",
		ResourceKind: "Deployment", ResourceNS: "production", ResourceName: "api-server",
		APIVersion: "apps/v1",
		Parameters: map[string]string{"MEMORY_LIMIT_NEW": "512Mi"}, ExecutionEngine: "job",
		Contributing:         []string{"memory_leak", "insufficient_memory_limits"},
		InvestigationOutcome: "actionable",
		IsActionable:         BoolPtr(true),
		ReasoningText:        "Weighed a transient traffic spike against a sustained leak: memory climbed steadily over 6h with no correlated traffic increase, which rules out a spike and points to a leak. Increasing the memory limit is the safe immediate mitigation while the leak itself would need a code-level fix.",
	}
}

func problemResolvedContradictionConfig() MockScenarioConfig {
	return MockScenarioConfig{
		ScenarioName: "problem_resolved_contradiction", SignalName: "MOCK_PROBLEM_RESOLVED_CONTRADICTION", Severity: "info",
		Confidence:   0.85,
		RootCause:    "Problem self-resolved. Transient OOM cleared after pod restart",
		ResourceKind: "Pod", ResourceNS: "production", ResourceName: "recovered-pod",
		APIVersion:           "v1",
		Contributing:         []string{"Transient condition", "Auto-recovery"},
		InvestigationOutcome: "problem_resolved",
		IsActionable:         BoolPtr(false),
	}
}

func maxRetriesExhaustedConfig() MockScenarioConfig {
	return MockScenarioConfig{
		ScenarioName: "max_retries_exhausted", SignalName: "MOCK_MAX_RETRIES_EXHAUSTED", Severity: "high",
		WorkflowName: "nonexistent-invalid-workflow-xyz", WorkflowID: uuid.DeterministicUUID("nonexistent-invalid-workflow-xyz"),
		WorkflowTitle: "Invalid Workflow", Confidence: 0.6,
		RootCause:    "LLM analysis completed but selected an invalid workflow not present in the catalog.",
		ResourceKind: "Pod", ResourceNS: "production", ResourceName: "failed-analysis-pod",
		APIVersion:           "v1",
		InvestigationOutcome: "actionable",
		IsActionable:         BoolPtr(true),
	}
}

func notActionableConfig() MockScenarioConfig {
	return MockScenarioConfig{
		ScenarioName: "not_actionable", SignalName: "MOCK_NOT_ACTIONABLE", Severity: "info",
		Confidence:   0.0,
		Rationale:    "Orphaned PVC from completed batch job; no active workload references it",
		RootCause:    "Orphaned PVC from completed batch job — no active workload references this volume",
		ResourceKind: "PersistentVolumeClaim", ResourceNS: "production", ResourceName: "batch-job-pvc-expired",
		APIVersion:           "v1",
		Contributing:         []string{"batch_job_completed", "pvc_retention_policy", "no_active_consumers"},
		InvestigationOutcome: "predictive_no_action",
		IsActionable:         BoolPtr(false),
	}
}

func parallelToolsConfig() MockScenarioConfig {
	actionable := true
	return MockScenarioConfig{
		ScenarioName: "parallel_tools", SignalName: "MOCK_PARALLEL_TOOLS", Severity: "high",
		WorkflowName: "oom-increase-memory-v1", WorkflowID: uuid.DeterministicUUID("oom-increase-memory-v1"),
		WorkflowTitle: "Increase Memory Limits", Confidence: 0.9,
		RootCause:    "Container OOMKilled due to memory limits below steady-state usage",
		ResourceKind: "Pod", ResourceNS: "production", ResourceName: "api-server-abc",
		APIVersion:           "v1",
		Parameters:           map[string]string{"NAMESPACE": "production", "POD_NAME": "api-server-abc"},
		InvestigationOutcome: "actionable",
		IsActionable:         &actionable,
		ForceText:            BoolPtr(false),
		MultiToolCalls: []MultiToolCallEntry{
			{Name: "kubectl_describe", Arguments: map[string]interface{}{"kind": "Pod", "name": "api-server-abc", "namespace": "production"}},
			{Name: "kubectl_events", Arguments: map[string]interface{}{"kind": "Pod", "name": "api-server-abc", "namespace": "production"}},
			{Name: "kubectl_logs", Arguments: map[string]interface{}{"kind": "Pod", "name": "api-server-abc", "namespace": "production"}},
		},
	}
}

func alertmanagerNodeToolsConfig() MockScenarioConfig {
	actionable := true
	return MockScenarioConfig{
		ScenarioName: "alertmanager_node_tools", SignalName: "MOCK_ALERTMANAGER_NODE_TOOLS", Severity: "high",
		WorkflowName: "oom-increase-memory-v1", WorkflowID: uuid.DeterministicUUID("oom-increase-memory-v1"),
		WorkflowTitle: "Increase Memory Limits", Confidence: 0.88,
		RootCause:    "Node resource exhaustion correlated with active alerts",
		ResourceKind: "Pod", ResourceNS: "production", ResourceName: "api-server-abc",
		APIVersion:           "v1",
		Parameters:           map[string]string{"NAMESPACE": "production", "POD_NAME": "api-server-abc"},
		InvestigationOutcome: "actionable",
		IsActionable:         &actionable,
		ForceText:            BoolPtr(false),
		MultiToolCalls: []MultiToolCallEntry{
			{Name: "get_alerts", Arguments: map[string]interface{}{}},
			{Name: "nodes_stats_summary", Arguments: map[string]interface{}{"node": "kubernaut-agent-e2e-control-plane"}},
		},
	}
}

func rcaIncompleteConfig() MockScenarioConfig {
	return MockScenarioConfig{
		ScenarioName: "rca_incomplete", SignalName: "MOCK_RCA_INCOMPLETE", Severity: "critical",
		WorkflowName: "generic-restart-v1", WorkflowID: uuid.DeterministicUUID("generic-restart-v1"),
		WorkflowTitle: "Generic Pod Restart", Confidence: 0.88,
		RootCause:    "Root cause identified but affected resource could not be determined from signal context",
		ResourceKind: "Pod", ResourceNS: "production", ResourceName: "unreachable-pod",
		APIVersion:           "v1",
		OverrideResource:     true,
		Parameters:           map[string]string{"NAMESPACE": "production", "POD_NAME": "unreachable-pod"},
		InvestigationOutcome: "actionable",
		IsActionable:         BoolPtr(true),
	}
}
