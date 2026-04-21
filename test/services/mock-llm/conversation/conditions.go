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

// HasThreeStepTools checks whether the tools list includes list_available_actions,
// indicating the three-step discovery protocol (DD-HAPI-017).
func HasThreeStepTools(tools []openai.Tool) bool {
	for _, t := range tools {
		if t.Function.Name == openai.ToolListAvailableActions {
			return true
		}
	}
	return false
}

// HasResourceContextTool checks whether get_resource_context is in the tools list
// (ADR-056 v1.4 four-step flow).
func HasResourceContextTool(tools []openai.Tool) bool {
	for _, t := range tools {
		if t.Function.Name == openai.ToolGetResourceContext {
			return true
		}
	}
	return false
}

// ToolResultCountEqual evaluates to true when the context has exactly N tool results.
type ToolResultCountEqual struct {
	N int
}

func (c *ToolResultCountEqual) Evaluate(ctx *Context) bool {
	return ctx.CountToolResults() == c.N
}

// ToolResultCountGE evaluates to true when the context has >= N tool results.
type ToolResultCountGE struct {
	N int
}

func (c *ToolResultCountGE) Evaluate(ctx *Context) bool {
	return ctx.CountToolResults() >= c.N
}

// HasSubmitWithWorkflowTool checks whether the tools list includes
// submit_result_with_workflow, indicating the split-submit protocol (#760).
func HasSubmitWithWorkflowTool(tools []openai.Tool) bool {
	for _, t := range tools {
		if t.Function.Name == openai.ToolSubmitResultWithWorkflow {
			return true
		}
	}
	return false
}

// AlwaysTrue is a fallback condition that always evaluates to true.
type AlwaysTrue struct{}

func (c *AlwaysTrue) Evaluate(_ *Context) bool { return true }
