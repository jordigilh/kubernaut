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
	"net/http"

	mockmetrics "github.com/jordigilh/kubernaut/test/services/mock-llm/metrics"
	"github.com/jordigilh/kubernaut/test/services/mock-llm/tracker"
)

// verificationHandler exposes the /api/test/* endpoints for test assertions.
type verificationHandler struct {
	tracker        *tracker.Tracker
	headerRecorder *tracker.HeaderRecorder
	metrics        *mockmetrics.Metrics
}

func (vh *verificationHandler) handleGetToolCalls(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, map[string]interface{}{
		"tool_calls": vh.tracker.GetToolCalls(),
	})
}

func (vh *verificationHandler) handleGetScenario(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, map[string]interface{}{
		"scenario": vh.tracker.GetLastScenario(),
	})
}

func (vh *verificationHandler) handleGetDAGPath(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, map[string]interface{}{
		"path": vh.tracker.GetDAGPath(),
	})
}

func (vh *verificationHandler) handleGetRequestCount(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, map[string]interface{}{
		"count": vh.tracker.GetRequestCount(),
	})
}

func (vh *verificationHandler) handleReset(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeJSON(w, http.StatusMethodNotAllowed, map[string]string{"error": "method not allowed"})
		return
	}
	vh.tracker.Reset()
	if vh.headerRecorder != nil {
		vh.headerRecorder.Reset()
	}
	if vh.metrics != nil {
		vh.metrics.Reset()
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "reset"})
}

func (vh *verificationHandler) handleGetHeaders(w http.ResponseWriter, _ *http.Request) {
	if vh.headerRecorder == nil {
		writeJSON(w, http.StatusOK, map[string]interface{}{"headers": map[string]string{}})
		return
	}
	writeJSON(w, http.StatusOK, map[string]interface{}{
		"headers": vh.headerRecorder.GetRecordedHeaders(),
	})
}
