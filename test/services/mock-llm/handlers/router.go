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

	"github.com/jordigilh/kubernaut/test/services/mock-llm/fault"
	"github.com/jordigilh/kubernaut/test/services/mock-llm/scenarios"
	"github.com/jordigilh/kubernaut/test/services/mock-llm/tracker"
)

// NewRouter creates the HTTP mux with all Mock LLM endpoints registered.
func NewRouter(registry *scenarios.Registry, forceText bool) http.Handler {
	return NewFullRouter(registry, forceText, "", nil)
}

// NewFullRouter creates the HTTP mux with all Mock LLM endpoints,
// including verification, fault injection, and header recording APIs.
func NewFullRouter(
	registry *scenarios.Registry,
	forceText bool,
	recordHeaders string,
	faultInjector *fault.Injector,
) http.Handler {
	mux := http.NewServeMux()

	t := tracker.New()

	var hr *tracker.HeaderRecorder
	if recordHeaders != "" {
		hr = tracker.NewHeaderRecorder(recordHeaders)
	}

	if faultInjector == nil {
		faultInjector = fault.NewInjector()
	}

	h := &handler{
		registry:       registry,
		forceText:      forceText,
		tracker:        t,
		headerRecorder: hr,
		faultInjector:  faultInjector,
	}

	// Health
	mux.HandleFunc("/health", h.handleHealth)

	// Models
	mux.HandleFunc("/v1/models", h.handleModels)
	mux.HandleFunc("/api/tags", h.handleModels)

	// OpenAI chat completions
	mux.HandleFunc("/v1/chat/completions", h.handleOpenAI)

	// Ollama endpoints
	mux.HandleFunc("/api/chat", h.handleOllama)
	mux.HandleFunc("/api/generate", h.handleOllama)

	// Verification API
	vh := &verificationHandler{tracker: t, headerRecorder: hr}
	mux.HandleFunc("/api/test/tool-calls", vh.handleGetToolCalls)
	mux.HandleFunc("/api/test/scenario", vh.handleGetScenario)
	mux.HandleFunc("/api/test/dag-path", vh.handleGetDAGPath)
	mux.HandleFunc("/api/test/request-count", vh.handleGetRequestCount)
	mux.HandleFunc("/api/test/reset", vh.handleReset)
	mux.HandleFunc("/api/test/headers", vh.handleGetHeaders)

	// Fault injection API
	fh := &faultHandler{injector: faultInjector}
	mux.HandleFunc("/api/test/fault", fh.handleConfigureFault)
	mux.HandleFunc("/api/test/fault/status", fh.handleGetFault)
	mux.HandleFunc("/api/test/fault/reset", fh.handleResetFault)

	return strictRouter(mux)
}

func strictRouter(next *http.ServeMux) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		next.ServeHTTP(w, r)
	})
}
