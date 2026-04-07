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

	"github.com/jordigilh/kubernaut/test/services/mock-llm/config"
)

// DefaultRegistryWithOverrides returns a fully populated registry with optional
// per-scenario overrides applied. If overrides is nil, behaves identically to
// DefaultRegistry.
//
// Override keys in the ConfigMap use "<workflow_name>:<environment>" format
// (populated by test infrastructure from DataStorage UUIDs). The lookup checks:
//  1. Exact match by ScenarioName (backward compatibility)
//  2. Fallback match by WorkflowName prefix (strips ":environment" suffix),
//     preferring ":production" when multiple environments exist
func DefaultRegistryWithOverrides(overrides *config.Overrides) *Registry {
	return DefaultRegistryFull(overrides, "")
}

func applyOverride(cs *configScenario, ov config.ScenarioOverride) {
	if ov.WorkflowID != "" {
		cs.config.WorkflowID = ov.WorkflowID
	}
	if ov.Confidence != nil {
		cs.config.Confidence = *ov.Confidence
	}
}

// findOverrideByWorkflowName searches override keys for entries matching the
// given workflow name. Keys have format "workflow_name:environment". When
// multiple environments match, ":production" is preferred since the E2E
// tests assert against production workflows.
func findOverrideByWorkflowName(overrides map[string]config.ScenarioOverride, workflowName string) (config.ScenarioOverride, bool) {
	var best config.ScenarioOverride
	found := false
	for key, ov := range overrides {
		name := key
		if idx := strings.Index(key, ":"); idx != -1 {
			name = key[:idx]
		}
		if name == workflowName {
			best = ov
			found = true
			if strings.HasSuffix(key, ":production") {
				return ov, true
			}
		}
	}
	return best, found
}

// DefaultRegistryFull returns a registry with optional overrides and golden
// transcript replay. goldenDir may be empty to skip replay loading.
func DefaultRegistryFull(overrides *config.Overrides, goldenDir string) *Registry {
	r := defaultRegistryWithGoldenDir(goldenDir)
	if overrides != nil {
		for _, s := range r.scenarios {
			cs, ok := s.(*configScenario)
			if !ok {
				continue
			}
			if ov, found := overrides.Scenarios[cs.config.ScenarioName]; found {
				applyOverride(cs, ov)
				continue
			}
			if cs.config.WorkflowName == "" {
				continue
			}
			if ov, found := findOverrideByWorkflowName(overrides.Scenarios, cs.config.WorkflowName); found {
				applyOverride(cs, ov)
			}
		}
	}
	return r
}

// DefaultRegistry returns a fully populated registry with all 15 scenarios
// and a default fallback, matching the Python MOCK_SCENARIOS catalog.
func DefaultRegistry() *Registry {
	return defaultRegistryInternal()
}

func defaultRegistryInternal() *Registry {
	return defaultRegistryWithGoldenDir("")
}

func defaultRegistryWithGoldenDir(goldenDir string) *Registry {
	r := NewRegistry()

	// Golden transcript replay scenarios (highest priority = 1.1)
	if goldenDir != "" {
		replays, _ := LoadReplayScenarios(goldenDir)
		for _, rs := range replays {
			r.Register(rs)
		}
	}

	// Mock keyword scenarios (highest priority = 1.0)
	r.Register(mockKeywordScenario("no_workflow_found", "mock_no_workflow_found", noWorkflowFoundConfig()))
	r.Register(mockKeywordScenario("low_confidence", "mock_low_confidence", lowConfidenceConfig()))
	r.Register(mockKeywordScenario("problem_resolved_contradiction", "mock_problem_resolved_contradiction", problemResolvedContradictionConfig()))
	r.Register(mockKeywordScenario("problem_resolved", "mock_problem_resolved", problemResolvedConfig()))
	r.Register(mockKeywordScenarioMulti("problem_resolved", []string{"mock_not_reproducible", "mock not reproducible"}, problemResolvedConfig()))
	r.Register(mockKeywordScenario("rca_incomplete", "mock_rca_incomplete", rcaIncompleteConfig()))
	r.Register(mockKeywordScenario("max_retries_exhausted", "mock_max_retries_exhausted", maxRetriesExhaustedConfig()))
	r.Register(mockKeywordScenario("not_actionable", "mock_not_actionable", notActionableConfig()))

	// Test signal scenario
	r.Register(testSignalScenario())

	// Proactive scenarios (checked before signal-name)
	r.Register(predictiveNoActionScenario())
	r.Register(oomkilledPredictiveScenario())

	// Signal name scenarios
	r.Register(signalScenario("cert_not_ready", []string{"certmanagercertnotready", "cert_not_ready"}, certNotReadyConfig()))
	r.Register(signalScenario("node_not_ready", []string{"nodenotready"}, nodeNotReadyConfig()))
	r.Register(signalScenario("oomkilled", []string{"memoryexceedslimit", "memoryexceeds", "oomkilled", "oomkill"}, oomkilledConfig()))
	r.Register(signalScenario("crashloop", []string{"crashloop", "backoff"}, crashloopConfig()))

	// Default fallback (lowest priority = 0.01)
	r.Register(defaultFallbackScenario())

	return r
}
