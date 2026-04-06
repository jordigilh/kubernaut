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

func oomkilledPredictiveConfig() MockScenarioConfig {
	return MockScenarioConfig{
		ScenarioName: "oomkilled_predictive", SignalName: "OOMKilled", Severity: "critical",
		WorkflowName: "oomkill-increase-memory-v1", WorkflowID: uuid.DeterministicUUID("oomkill-increase-memory-v1"),
		WorkflowTitle: "OOMKill Recovery - Increase Memory Limits", Confidence: 0.88,
		Rationale:    "Predicted OOMKill trend warrants preemptive memory limit increase before the threshold is breached",
		RootCause:    "Predicted OOMKill based on memory utilization trend analysis (predict_linear). Current memory usage is 85% of limit and growing at 50MB/min. Preemptive action recommended to increase memory limits before the predicted OOMKill event occurs.",
		ResourceKind: "Deployment", ResourceNS: "production", ResourceName: "api-server",
		Parameters:   map[string]string{"MEMORY_LIMIT_NEW": "512Mi"}, ExecutionEngine: "job",
		Contributing: []string{"memory_trend_increasing", "predict_linear_threshold_breach", "no_HPA_configured"},
		InvestigationOutcome: "actionable",
		IsActionable:         BoolPtr(true),
	}
}

func predictiveNoActionConfig() MockScenarioConfig {
	return MockScenarioConfig{
		ScenarioName: "predictive_no_action", SignalName: "OOMKilled", Severity: "medium",
		Confidence:           0.82,
		RootCause:            "Predicted OOMKill based on trend analysis, but current assessment shows the trend is reversing. Memory usage has stabilized at 60% of limit after recent deployment rollout. No preemptive action needed — the prediction is unlikely to materialize.",
		ResourceKind:         "Pod", ResourceNS: "production", ResourceName: "api-server-def456",
		InvestigationOutcome: "predictive_no_action",
		IsActionable:         BoolPtr(false),
	}
}

func predictiveNoActionScenario() *configScenario {
	cfg := predictiveNoActionConfig()
	return &configScenario{
		config: cfg,
		matchFunc: func(ctx *DetectionContext) (bool, float64) {
			if !isProactive(ctx) {
				return false, 0
			}
			lower := strings.ToLower(ctx.Content + " " + ctx.AllText)
			if strings.Contains(lower, "predictive_no_action") || strings.Contains(lower, "mock_predictive_no_action") {
				return true, 0.98
			}
			return false, 0
		},
	}
}

func oomkilledPredictiveScenario() *configScenario {
	cfg := oomkilledPredictiveConfig()
	return &configScenario{
		config: cfg,
		matchFunc: func(ctx *DetectionContext) (bool, float64) {
			if !isProactive(ctx) {
				return false, 0
			}
			signal := extractSignal(ctx)
			if strings.Contains(signal, "oomkilled") || strings.Contains(signal, "oomkill") {
				return true, 0.96
			}
			return false, 0
		},
	}
}
