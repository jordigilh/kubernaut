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
	"time"

	openai "github.com/jordigilh/kubernaut/pkg/shared/types/openai"
	"github.com/jordigilh/kubernaut/test/services/mock-llm/conversation"
	"github.com/jordigilh/kubernaut/test/services/mock-llm/fault"
	mockmetrics "github.com/jordigilh/kubernaut/test/services/mock-llm/metrics"
	"github.com/jordigilh/kubernaut/test/services/mock-llm/response"
	"github.com/jordigilh/kubernaut/test/services/mock-llm/scenarios"
	"github.com/jordigilh/kubernaut/test/services/mock-llm/tracker"
)

func (h *handler) handleOllama(w http.ResponseWriter, r *http.Request) {
	start := time.Now()

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
	scenarioName := "default"
	cfg := scenarios.MockScenarioConfig{
		ScenarioName: scenarioName, RootCause: "Unable to determine root cause", Severity: "medium",
	}
	if result != nil {
		if s, ok := result.Scenario.(scenarios.ScenarioWithConfig); ok {
			cfg = s.Config()
			scenarioName = cfg.ScenarioName
		}
		h.recordScenarioMetric(scenarioName, result.Method)
	}

	writeJSON(w, http.StatusOK, response.BuildOllamaResponse(model, cfg))
	h.recordRequestMetric(r.URL.Path, http.StatusOK, scenarioName, time.Since(start).Seconds())
}

// handler holds dependencies for all HTTP handlers.
type handler struct {
	registry       *scenarios.Registry
	forceText      bool
	tracker        *tracker.Tracker
	headerRecorder *tracker.HeaderRecorder
	faultInjector  *fault.Injector
	metrics        *mockmetrics.Metrics
}

func (h *handler) recordRequestMetric(endpoint string, statusCode int, scenario string, duration float64) {
	if h.metrics != nil {
		h.metrics.RecordRequest(endpoint, statusCode, scenario, duration)
	}
}

func (h *handler) recordScenarioMetric(scenario, method string) {
	if h.metrics != nil {
		h.metrics.RecordScenarioDetection(scenario, method)
	}
}

func (h *handler) recordDAGTransitions(path []string) {
	if h.metrics == nil || len(path) < 2 {
		return
	}
	for i := 0; i < len(path)-1; i++ {
		h.metrics.RecordDAGTransition(path[i], path[i+1])
	}
}
