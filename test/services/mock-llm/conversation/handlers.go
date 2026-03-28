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

// ToolCallHandler produces a HandlerResult indicating a tool call response.
type ToolCallHandler struct {
	ToolName string
}

func (h *ToolCallHandler) Handle(_ *Context) (*HandlerResult, error) {
	return &HandlerResult{
		NodeName:     h.ToolName,
		ResponseType: StepToolCall,
		ToolName:     h.ToolName,
	}, nil
}

// FinalAnalysisHandler produces a HandlerResult indicating a text response.
type FinalAnalysisHandler struct{}

func (h *FinalAnalysisHandler) Handle(_ *Context) (*HandlerResult, error) {
	return &HandlerResult{
		NodeName:     "final_analysis",
		ResponseType: StepFinalAnalysis,
	}, nil
}

// NoOpHandler is a handler that produces no meaningful result.
// Used for dispatch/routing nodes that only serve as transition hubs.
type NoOpHandler struct{}

func (h *NoOpHandler) Handle(_ *Context) (*HandlerResult, error) {
	return &HandlerResult{NodeName: "dispatch"}, nil
}
