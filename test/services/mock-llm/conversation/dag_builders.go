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

// LegacyDAG returns the two-phase conversation DAG (search_workflow_catalog → final_analysis).
func LegacyDAG() *DAG {
	dag := NewDAG("dispatch")
	dag.AddNode("dispatch", &NoOpHandler{})
	dag.AddNode(openai.ToolSearchWorkflowCatalog, &ToolCallHandler{ToolName: openai.ToolSearchWorkflowCatalog})
	dag.AddNode("final_analysis", &FinalAnalysisHandler{})

	// toolResults >= 1 → final analysis (check first, higher priority)
	dag.AddTransition("dispatch", "final_analysis", &ToolResultCountGE{N: 1}, 0)
	// toolResults == 0 → search_workflow_catalog
	dag.AddTransition("dispatch", openai.ToolSearchWorkflowCatalog, &ToolResultCountEqual{N: 0}, 1)

	return dag
}

// ThreeStepDAG returns the three-step (or four-step with resource context) conversation DAG.
func ThreeStepDAG(hasResourceContext bool) *DAG {
	dag := NewDAG("dispatch")
	dag.AddNode("dispatch", &NoOpHandler{})

	var steps []string
	if hasResourceContext {
		steps = []string{
			openai.ToolGetResourceContext,
			openai.ToolListAvailableActions,
			openai.ToolListWorkflows,
			openai.ToolGetWorkflow,
		}
	} else {
		steps = []string{
			openai.ToolListAvailableActions,
			openai.ToolListWorkflows,
			openai.ToolGetWorkflow,
		}
	}

	for _, name := range steps {
		dag.AddNode(name, &ToolCallHandler{ToolName: name})
	}
	dag.AddNode("final_analysis", &FinalAnalysisHandler{})

	// Final analysis when all tool steps are done (highest priority = 0)
	dag.AddTransition("dispatch", "final_analysis", &ToolResultCountGE{N: len(steps)}, 0)

	// Each tool step keyed on exact tool result count (priority increases = lower precedence)
	for i, name := range steps {
		dag.AddTransition("dispatch", name, &ToolResultCountEqual{N: i}, i+1)
	}

	return dag
}

// SelectDAG determines the appropriate conversation DAG from the tools list.
// Replaces the legacy SelectMode function.
func SelectDAG(tools []openai.Tool) *DAG {
	if HasThreeStepTools(tools) {
		return ThreeStepDAG(HasResourceContextTool(tools))
	}
	return LegacyDAG()
}
