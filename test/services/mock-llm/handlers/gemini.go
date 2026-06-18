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
	"log"
	"net/http"
	"strings"
	"time"

	openai "github.com/jordigilh/kubernaut/pkg/shared/types/openai"
	"github.com/jordigilh/kubernaut/test/services/mock-llm/config"
	"github.com/jordigilh/kubernaut/test/services/mock-llm/response"
	"github.com/jordigilh/kubernaut/test/services/mock-llm/scenarios"
)

const geminiPathSuffix = ":generateContent"

// handleGemini handles Gemini generateContent requests.
// Path format: POST /v1beta/models/{model}:generateContent
func (h *handler) handleGemini(w http.ResponseWriter, r *http.Request) {
	start := time.Now()

	if r.Method != http.MethodPost {
		writeJSON(w, http.StatusMethodNotAllowed, map[string]string{"error": "method not allowed"})
		return
	}

	path := r.URL.Path
	if !strings.HasSuffix(path, geminiPathSuffix) {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "not found"})
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
		writeJSON(w, h.faultInjector.StatusCode(), response.BuildGeminiErrorResponse(h.faultInjector.Message()))
		return
	}

	var req response.GeminiRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid JSON"})
		return
	}

	lastUserText, allText := response.ExtractTextFromContents(req.Contents, req.SystemInstruction)
	hasFunctionResults := response.HasFunctionResponse(req.Contents)

	content := strings.ToLower(lastUserText)
	allTextLower := strings.ToLower(allText)

	isProactive := strings.Contains(content, "proactive mode") ||
		strings.Contains(content, "proactive signal") ||
		(strings.Contains(content, "predicted") && strings.Contains(content, "not yet occurred"))

	detCtx := &scenarios.DetectionContext{
		Content:         content,
		AllText:         allTextLower,
		IsProactive:     isProactive,
		LastUserContent: content,
	}

	if isPermanentError(detCtx) {
		writeJSON(w, http.StatusInternalServerError, response.BuildGeminiErrorResponse("Mock permanent LLM error for testing"))
		return
	}

	result := h.registry.Detect(detCtx)
	if result == nil {
		writeJSON(w, http.StatusOK, response.BuildGeminiTextResponse(scenarios.MockScenarioConfig{
			ScenarioName: "default", RootCause: "Unable to determine root cause", Severity: "medium",
			InvestigationOutcome: "actionable", IsActionable: scenarios.BoolPtr(true), Confidence: 0.5,
		}))
		h.recordRequestMetric(r.URL.Path, http.StatusOK, "default", time.Since(start).Seconds())
		return
	}

	if h.tracker != nil {
		h.tracker.RecordScenario(result.Scenario.Name())
	}

	cfg, ok := resolveScenarioConfig(result.Scenario, detCtx)
	if !ok {
		writeJSON(w, http.StatusOK, response.BuildGeminiTextResponse(scenarios.MockScenarioConfig{}))
		return
	}

	resolveGeminiTemplateArgs(req.Contents, &cfg)

	scenarioName := cfg.ScenarioName
	h.recordScenarioMetric(scenarioName, result.Method)

	hasTools := len(req.Tools) > 0
	hasSplit := geminiHasSubmitWithWorkflowTool(req.Tools)
	resolved := isResolvedOutcome(cfg)

	log.Printf("[mock-llm/gemini] scenario=%s mode=%s outcome=%s workflowID=%q tools=%d hasSplitTool=%v hasFuncResults=%v",
		scenarioName, h.mode, cfg.InvestigationOutcome, cfg.WorkflowID, len(req.Tools), hasSplit, hasFunctionResults)

	if cfg.SecondTurnDelay > 0 && hasFunctionResults {
		log.Printf("[mock-llm/gemini] scenario=%s applying %v second-turn delay", scenarioName, cfg.SecondTurnDelay)
		select {
		case <-r.Context().Done():
			return
		case <-time.After(cfg.SecondTurnDelay):
		}
	}

	switch h.mode {
	case config.ModeInteractive:
		writeJSON(w, http.StatusOK, response.BuildGeminiTextResponse(cfg))

	default: // config.ModeAutonomous, config.ModeFull, or unset
		effectiveForceText := h.forceText
		if h.mode == config.ModeAutonomous {
			effectiveForceText = true
		}
		if cfg.ForceText != nil {
			effectiveForceText = *cfg.ForceText
		}

		if effectiveForceText || !hasTools {
			if hasSplit && !resolved {
				h.respondGeminiWithSubmitToolCall(w, cfg)
			} else {
				writeJSON(w, http.StatusOK, response.BuildGeminiTextResponse(cfg))
			}
		} else {
			h.handleGeminiToolResponse(w, cfg, req.Tools, req.Contents, hasFunctionResults, hasSplit, resolved)
		}
	}

	h.recordRequestMetric(r.URL.Path, http.StatusOK, scenarioName, time.Since(start).Seconds())
}

// handleGeminiToolResponse handles the tool-calling path for Gemini requests.
func (h *handler) handleGeminiToolResponse(
	w http.ResponseWriter,
	cfg scenarios.MockScenarioConfig,
	tools []response.GeminiToolDecl,
	contents []response.GeminiContent,
	hasFunctionResults, hasSplit, resolved bool,
) {
	// Guard against infinite tool-call loops (#1189): if RepeatToolCall is
	// enabled but the last content in the conversation is already a
	// FunctionResponse (meaning we just executed the tool in this iteration),
	// fall through to text response instead of re-emitting the tool call.
	repeatAllowed := cfg.RepeatToolCall && !response.LastContentIsFunctionResponse(contents)

	// NextToolCall: when the initial tool has been called (FunctionResponse
	// present) and a chained next_tool_call is configured, emit it. This
	// simulates the LLM auto-proceeding to a second tool in the same session.
	if hasFunctionResults && cfg.NextToolCall != nil {
		h.trackToolCall(cfg.NextToolCall.Name)
		writeJSON(w, http.StatusOK, response.BuildGeminiToolCallResponse(cfg.NextToolCall.Name, scenarios.MockScenarioConfig{
			ToolCallName: cfg.NextToolCall.Name,
			ToolCallArgs: cfg.NextToolCall.Arguments,
		}))
		return
	}

	if len(cfg.MultiToolCalls) > 0 && (!hasFunctionResults || repeatAllowed) {
		for _, tc := range cfg.MultiToolCalls {
			h.trackToolCall(tc.Name)
		}
		writeJSON(w, http.StatusOK, response.BuildGeminiMultiToolCallResponse(cfg.MultiToolCalls))
		return
	}

	if cfg.ToolCallName != "" && (!hasFunctionResults || repeatAllowed) {
		h.trackToolCall(cfg.ToolCallName)
		writeJSON(w, http.StatusOK, response.BuildGeminiToolCallResponse(cfg.ToolCallName, cfg))
		return
	}

	// When no explicit tool call is configured but tools are declared and no
	// function results have come back yet, call the first declared tool.
	// This mirrors the DAG engine's initial step behavior for the OpenAI path.
	if !hasFunctionResults {
		if firstTool := firstDeclaredTool(tools); firstTool != "" {
			h.trackToolCall(firstTool)
			writeJSON(w, http.StatusOK, response.BuildGeminiToolCallResponse(firstTool, cfg))
			return
		}
	}

	if hasSplit && !resolved {
		h.respondGeminiWithSubmitToolCall(w, cfg)
	} else {
		writeJSON(w, http.StatusOK, response.BuildGeminiTextResponse(cfg))
	}
}

// respondGeminiWithSubmitToolCall writes the appropriate submit_result tool call in Gemini format.
func (h *handler) respondGeminiWithSubmitToolCall(w http.ResponseWriter, cfg scenarios.MockScenarioConfig) {
	toolName := openai.ToolSubmitResultWithWorkflow
	if cfg.WorkflowID == "" {
		toolName = openai.ToolSubmitResultNoWorkflow
	}
	h.trackToolCall(toolName)
	writeJSON(w, http.StatusOK, response.BuildGeminiToolCallResponse(toolName, cfg))
}

// geminiHasSubmitWithWorkflowTool checks if the Gemini tools contain the split submit tools.
func geminiHasSubmitWithWorkflowTool(tools []response.GeminiToolDecl) bool {
	for _, t := range tools {
		for _, fd := range t.FunctionDeclarations {
			if fd.Name == openai.ToolSubmitResultWithWorkflow || fd.Name == openai.ToolSubmitResultNoWorkflow {
				return true
			}
		}
	}
	return false
}

// firstDeclaredTool returns the name of the first function declaration across all tool groups.
func firstDeclaredTool(tools []response.GeminiToolDecl) string {
	for _, t := range tools {
		for _, fd := range t.FunctionDeclarations {
			if fd.Name != "" {
				return fd.Name
			}
		}
	}
	return ""
}

// resolveGeminiTemplateArgs scans cfg.ToolCallArgs for template placeholders
// of the form "$from_tool:<toolName>:<field>" and replaces them with values
// extracted from prior FunctionResponse parts in the conversation.
// The map is cloned before mutation to avoid data races and state leaks
// across concurrent requests sharing the same scenario singleton.
func resolveGeminiTemplateArgs(contents []response.GeminiContent, cfg *scenarios.MockScenarioConfig) {
	if len(cfg.ToolCallArgs) == 0 {
		return
	}
	cfg.ToolCallArgs = cloneAnyMap(cfg.ToolCallArgs)
	for k, v := range cfg.ToolCallArgs {
		sv, ok := v.(string)
		if !ok || !strings.HasPrefix(sv, templatePrefix) {
			continue
		}
		parts := strings.SplitN(sv[len(templatePrefix):], ":", 2)
		if len(parts) != 2 {
			continue
		}
		toolName, field := parts[0], parts[1]
		if resolved := response.ExtractFieldFromFunctionResponse(contents, toolName, field); resolved != "" {
			cfg.ToolCallArgs[k] = resolved
		}
	}
}

func cloneAnyMap(m map[string]interface{}) map[string]interface{} {
	c := make(map[string]interface{}, len(m))
	for k, v := range m {
		c[k] = v
	}
	return c
}

