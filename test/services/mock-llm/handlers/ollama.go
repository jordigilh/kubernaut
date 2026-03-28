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

	openai "github.com/jordigilh/kubernaut/pkg/shared/types/openai"
	"github.com/jordigilh/kubernaut/test/services/mock-llm/conversation"
	"github.com/jordigilh/kubernaut/test/services/mock-llm/fault"
	"github.com/jordigilh/kubernaut/test/services/mock-llm/response"
	"github.com/jordigilh/kubernaut/test/services/mock-llm/scenarios"
	"github.com/jordigilh/kubernaut/test/services/mock-llm/tracker"
)

func (h *handler) handleOllama(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeJSON(w, http.StatusMethodNotAllowed, map[string]string{"error": "method not allowed"})
		return
	}

	// Check fault injection
	if h.faultInjector != nil && h.faultInjector.IsActive() {
		applyFaultDelay(h.faultInjector)
		writeJSON(w, h.faultInjector.StatusCode(), map[string]string{
			"error": h.faultInjector.Message(),
		})
		return
	}

	var reqData struct {
		Model    string           `json:"model"`
		Messages []openai.Message `json:"messages"`
		Prompt   string           `json:"prompt"`
	}
	if err := json.NewDecoder(r.Body).Decode(&reqData); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid JSON"})
		return
	}

	model := reqData.Model
	if model == "" {
		model = openai.DefaultModel
	}

	// Handle /api/generate which uses "prompt" instead of "messages"
	messages := reqData.Messages
	if len(messages) == 0 && reqData.Prompt != "" {
		messages = []openai.Message{{Role: "user", Content: &reqData.Prompt}}
	}

	ctx := conversation.NewContext(messages)
	detCtx := buildDetectionContext(ctx)

	if isPermanentError(detCtx) {
		writeJSON(w, http.StatusInternalServerError, map[string]string{
			"error": "Mock permanent LLM error for testing",
		})
		return
	}

	result := h.registry.Detect(detCtx)
	cfg := scenarios.MockScenarioConfig{
		ScenarioName: "default", RootCause: "Unable to determine root cause", Severity: "medium",
	}
	if result != nil {
		if s, ok := result.Scenario.(scenarios.ScenarioWithConfig); ok {
			cfg = s.Config()
		}
	}

	writeJSON(w, http.StatusOK, response.BuildOllamaResponse(model, cfg))
}

// handler holds dependencies for all HTTP handlers.
type handler struct {
	registry       *scenarios.Registry
	forceText      bool
	tracker        *tracker.Tracker
	headerRecorder *tracker.HeaderRecorder
	faultInjector  *fault.Injector
}
