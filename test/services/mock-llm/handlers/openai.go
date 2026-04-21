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
	"encoding/json"
	"net/http"
	"strings"
	"time"

	openai "github.com/jordigilh/kubernaut/pkg/shared/types/openai"
	"github.com/jordigilh/kubernaut/test/services/mock-llm/conversation"
	"github.com/jordigilh/kubernaut/test/services/mock-llm/response"
	"github.com/jordigilh/kubernaut/test/services/mock-llm/scenarios"
)

func (h *handler) handleOpenAI(w http.ResponseWriter, r *http.Request) {
	start := time.Now()

	if r.Method != http.MethodPost {
		writeJSON(w, http.StatusMethodNotAllowed, map[string]string{"error": "method not allowed"})
		return
	}

	if h.headerRecorder != nil {
		h.headerRecorder.RecordFrom(r)
	}

	if h.tracker != nil {
		h.tracker.IncrementRequestCount()
	}

	if h.faultInjector != nil && h.faultInjector.IsActive() {
		applyFaultDelay(h.faultInjector)
		writeJSON(w, h.faultInjector.StatusCode(),
			response.BuildErrorResponse(h.faultInjector.Message()))
		return
	}

	var req openai.ChatCompletionRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid JSON"})
		return
	}

	model := req.Model
	if model == "" {
		model = openai.DefaultModel
	}

	ctx := conversation.NewContext(req.Messages)
	detCtx := buildDetectionContext(ctx)

	if isPermanentError(detCtx) {
		writeJSON(w, http.StatusInternalServerError,
			response.BuildErrorResponse("Mock permanent LLM error for testing"))
		return
	}

	result := h.registry.Detect(detCtx)
	if result == nil {
		actionable := true
		writeJSON(w, http.StatusOK, response.BuildTextResponse(model, scenarios.MockScenarioConfig{
			ScenarioName: "default", RootCause: "Unable to determine root cause", Severity: "medium",
			InvestigationOutcome: "actionable", IsActionable: &actionable, Confidence: 0.5,
		}))
		h.recordRequestMetric(r.URL.Path, http.StatusOK, "default", time.Since(start).Seconds())
		return
	}

	if h.tracker != nil {
		h.tracker.RecordScenario(result.Scenario.Name())
	}

	scenarioWithCfg, ok := result.Scenario.(scenarios.ScenarioWithConfig)
	if !ok {
		writeJSON(w, http.StatusOK, response.BuildTextResponse(model, scenarios.MockScenarioConfig{}))
		return
	}
	cfg := scenarioWithCfg.Config()

	if !cfg.OverrideResource {
		if res := ctx.ExtractResource(); res.Kind != "" && res.Name != "" {
			cfg.ResourceKind = res.Kind
			cfg.ResourceName = res.Name
			cfg.ResourceNS = res.Namespace
		}
	}

	scenarioName := cfg.ScenarioName
	h.recordScenarioMetric(scenarioName, result.Method)

	if h.forceText || len(req.Tools) == 0 {
		if conversation.HasSubmitWithWorkflowTool(req.Tools) && !isResolvedOutcome(cfg) {
			toolName := openai.ToolSubmitResultWithWorkflow
			if cfg.WorkflowID == "" {
				toolName = openai.ToolSubmitResultNoWorkflow
			}
			h.trackToolCall(toolName)
			writeJSON(w, http.StatusOK, response.BuildToolCallResponse(model, toolName, cfg))
		} else {
			writeJSON(w, http.StatusOK, response.BuildForceTextResponse(model, cfg, req.Tools))
		}
		h.recordRequestMetric(r.URL.Path, http.StatusOK, scenarioName, time.Since(start).Seconds())
		return
	}

	dag := conversation.SelectDAG(req.Tools)
	execResult, err := dag.Execute(ctx)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError,
			response.BuildErrorResponse("DAG execution error: "+err.Error()))
		return
	}

	if h.tracker != nil {
		h.tracker.RecordDAGPath(execResult.Path)
	}
	h.recordDAGTransitions(execResult.Path)

	hr := execResult.Result
	switch hr.ResponseType {
	case conversation.StepToolCall:
		h.trackToolCall(hr.ToolName)
		writeJSON(w, http.StatusOK, response.BuildToolCallResponse(model, hr.ToolName, cfg))
	default:
		if conversation.HasSubmitWithWorkflowTool(req.Tools) && !isResolvedOutcome(cfg) {
			toolName := openai.ToolSubmitResultWithWorkflow
			if cfg.WorkflowID == "" {
				toolName = openai.ToolSubmitResultNoWorkflow
			}
			h.trackToolCall(toolName)
			writeJSON(w, http.StatusOK, response.BuildToolCallResponse(model, toolName, cfg))
		} else {
			writeJSON(w, http.StatusOK, response.BuildTextResponse(model, cfg))
		}
	}
	h.recordRequestMetric(r.URL.Path, http.StatusOK, scenarioName, time.Since(start).Seconds())
}

func (h *handler) trackToolCall(name string) {
	if h.tracker != nil {
		h.tracker.RecordToolCall(name, "")
	}
}

func buildDetectionContext(ctx *conversation.Context) *scenarios.DetectionContext {
	var contentParts, allParts []string
	for _, m := range ctx.Messages {
		if m.Content != nil {
			contentParts = append(contentParts, *m.Content)
		}
		allParts = append(allParts, msgString(m))
	}
	content := strings.ToLower(strings.Join(contentParts, " "))
	allText := strings.ToLower(strings.Join(allParts, " "))

	isProactive := strings.Contains(content, "proactive mode") ||
		strings.Contains(content, "proactive signal") ||
		(strings.Contains(content, "predicted") && strings.Contains(content, "not yet occurred"))

	return &scenarios.DetectionContext{
		Content:     content,
		AllText:     allText,
		IsProactive: isProactive,
	}
}

// isResolvedOutcome returns true when the scenario represents a resolved or
// non-actionable investigation that should bypass the split submit tools and
// use a text response so the KA parser handles investigation_outcome routing.
func isResolvedOutcome(cfg scenarios.MockScenarioConfig) bool {
	switch cfg.InvestigationOutcome {
	case "problem_resolved", "predictive_no_action":
		return true
	default:
		return cfg.IsActionable != nil && !*cfg.IsActionable && cfg.WorkflowID == ""
	}
}

func isPermanentError(ctx *scenarios.DetectionContext) bool {
	return strings.Contains(ctx.Content, "mock_rca_permanent_error") ||
		strings.Contains(ctx.Content, "mock rca permanent error")
}

func msgString(m openai.Message) string {
	parts := []string{m.Role}
	if m.Content != nil {
		parts = append(parts, *m.Content)
	}
	for _, tc := range m.ToolCalls {
		parts = append(parts, tc.Function.Name, tc.Function.Arguments)
	}
	return strings.Join(parts, " ")
}
