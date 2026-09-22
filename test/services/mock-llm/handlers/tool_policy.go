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
package handlers

import (
	openai "github.com/jordigilh/kubernaut/pkg/shared/types/openai"
	"github.com/jordigilh/kubernaut/test/services/mock-llm/config"
	"github.com/jordigilh/kubernaut/test/services/mock-llm/scenarios"
)

// shouldUseToolProtocol applies one response-mode policy to all provider
// handlers. An explicitly configured scenario wins; advertised DD-KA-017 tools
// are allowed to run discovery even when the global default is force-text.
func shouldUseToolProtocol(mode string, globalForceText bool, scenario scenarios.MockScenarioConfig, hasDiscoveryTools bool, toolCount int) bool {
	if toolCount == 0 {
		return false
	}
	if scenario.ForceText != nil {
		if mode == config.ModeInteractive && !hasDiscoveryTools {
			return false
		}
		return !*scenario.ForceText
	}
	if hasDiscoveryTools {
		return true
	}
	switch mode {
	case config.ModeInteractive:
		return false
	case config.ModeAutonomous:
		return false
	default:
		return !globalForceText
	}
}

func scenarioForcesText(scenario scenarios.MockScenarioConfig) bool {
	return scenario.ForceText != nil && *scenario.ForceText
}

func withoutWorkflowSelection(scenario scenarios.MockScenarioConfig) scenarios.MockScenarioConfig {
	scenario.WorkflowID = ""
	return scenario
}

func hasDiscoveryOverride(scenario scenarios.MockScenarioConfig) bool {
	if isDiscoveryToolName(scenario.ToolCallName) {
		return true
	}
	for _, toolCall := range scenario.MultiToolCalls {
		if isDiscoveryToolName(toolCall.Name) {
			return true
		}
	}
	for toolCall := scenario.NextToolCall; toolCall != nil; toolCall = toolCall.NextToolCall {
		if isDiscoveryToolName(toolCall.Name) {
			return true
		}
	}
	return false
}

func isDiscoveryToolName(name string) bool {
	switch name {
	case openai.ToolListAvailableActions, openai.ToolGetResourceContext, openai.ToolListWorkflows, openai.ToolGetWorkflow, openai.ToolSubmitResultWithWorkflow:
		return true
	default:
		return false
	}
}
