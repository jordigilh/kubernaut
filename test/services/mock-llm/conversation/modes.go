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
package conversation

import openai "github.com/jordigilh/kubernaut/pkg/shared/types/openai"

// StepType identifies the kind of response a conversation step produces.
type StepType int

const (
	StepToolCall      StepType = iota // Step produces a tool call request
	StepFinalAnalysis                 // Step produces the final analysis text
)

// ConversationMode defines the conversation flow for a particular tool configuration.
type ConversationMode struct {
	name  string
	steps []step
}

type step struct {
	stype    StepType
	toolName string
}

// Name returns the mode identifier.
func (m *ConversationMode) Name() string { return m.name }

// TotalSteps returns the number of steps including the final analysis.
func (m *ConversationMode) TotalSteps() int { return len(m.steps) }

// StepType returns the type for the given step index.
func (m *ConversationMode) StepType(index int) StepType {
	if index < 0 || index >= len(m.steps) {
		return StepFinalAnalysis
	}
	return m.steps[index].stype
}

// ToolNameForStep returns the tool name for a tool_call step.
func (m *ConversationMode) ToolNameForStep(index int) string {
	if index < 0 || index >= len(m.steps) {
		return ""
	}
	return m.steps[index].toolName
}

// LegacyMode returns the two-phase conversation mode (search_workflow_catalog).
func LegacyMode() *ConversationMode {
	return &ConversationMode{
		name: "legacy",
		steps: []step{
			{stype: StepToolCall, toolName: openai.ToolSearchWorkflowCatalog},
			{stype: StepFinalAnalysis},
		},
	}
}

// ThreeStepMode returns the three-step (or four-step with resource context) mode.
func ThreeStepMode(hasResourceContext bool) *ConversationMode {
	var steps []step
	name := "three_step"

	if hasResourceContext {
		name = "three_step_rc"
		steps = append(steps, step{stype: StepToolCall, toolName: openai.ToolGetResourceContext})
	}

	steps = append(steps,
		step{stype: StepToolCall, toolName: openai.ToolListAvailableActions},
		step{stype: StepToolCall, toolName: openai.ToolListWorkflows},
		step{stype: StepToolCall, toolName: openai.ToolGetWorkflow},
		step{stype: StepFinalAnalysis},
	)

	return &ConversationMode{name: name, steps: steps}
}

// SelectMode determines the appropriate conversation mode from the tools list.
func SelectMode(tools []openai.Tool) *ConversationMode {
	if HasThreeStepTools(tools) {
		return ThreeStepMode(HasResourceContextTool(tools))
	}
	return LegacyMode()
}
