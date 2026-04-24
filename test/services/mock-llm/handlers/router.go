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

	"github.com/prometheus/client_golang/prometheus/promhttp"

	"github.com/jordigilh/kubernaut/test/services/mock-llm/config"
	"github.com/jordigilh/kubernaut/test/services/mock-llm/fault"
	mockmetrics "github.com/jordigilh/kubernaut/test/services/mock-llm/metrics"
	"github.com/jordigilh/kubernaut/test/services/mock-llm/scenarios"
	"github.com/jordigilh/kubernaut/test/services/mock-llm/tracker"
)

// NewRouter creates the HTTP mux with all Mock LLM endpoints registered.
func NewRouter(registry *scenarios.Registry, forceText bool) http.Handler {
	return NewFullRouter(registry, forceText, "", nil)
}

// NewMetricsRouter creates the HTTP mux with all Mock LLM endpoints plus
// a /metrics endpoint serving Prometheus-format metrics.
func NewMetricsRouter(registry *scenarios.Registry, forceText bool) http.Handler {
	return NewFullRouterWithMetrics(registry, forceText, "", nil, mockmetrics.NewMetrics())
}

// NewFullRouter creates the HTTP mux with all Mock LLM endpoints,
// including verification, fault injection, and header recording APIs.
func NewFullRouter(
	registry *scenarios.Registry,
	forceText bool,
	recordHeaders string,
	faultInjector *fault.Injector,
) http.Handler {
	return NewFullRouterWithMetrics(registry, forceText, recordHeaders, faultInjector, nil)
}

// NewFullRouterWithMetrics creates the HTTP mux with all Mock LLM endpoints,
// including verification, fault injection, header recording, and Prometheus metrics.
// When overrides is non-nil and overrides.Mode == "shadow", a minimal router
// is returned that only serves health and shadow alignment evaluation endpoints.
func NewFullRouterWithMetrics(
	registry *scenarios.Registry,
	forceText bool,
	recordHeaders string,
	faultInjector *fault.Injector,
	m *mockmetrics.Metrics,
	overrides ...*config.Overrides,
) http.Handler {
	if len(overrides) > 0 && overrides[0] != nil && overrides[0].Mode == "shadow" {
		return newShadowRouter(m)
	}

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
		metrics:        m,
	}

	// Health
	mux.HandleFunc("/health", h.handleHealth)

	// Models
	mux.HandleFunc("/v1/models", h.handleModels)
	mux.HandleFunc("/api/tags", h.handleModels)

	// OpenAI chat completions (BR-MOCK-001: both /v1/ prefixed and unprefixed)
	mux.HandleFunc("/v1/chat/completions", h.handleOpenAI)
	mux.HandleFunc("/chat/completions", h.handleOpenAI)

	// Ollama endpoints
	mux.HandleFunc("/api/chat", h.handleOllama)
	mux.HandleFunc("/api/generate", h.handleOllama)

	// Verification API
	vh := &verificationHandler{tracker: t, headerRecorder: hr, metrics: m}
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

	// Prometheus metrics endpoint (BR-MOCK-080)
	if m != nil {
		mux.Handle("/metrics", promhttp.HandlerFor(m.Registry(), promhttp.HandlerOpts{}))
	}

	return strictRouter(mux)
}

// newShadowRouter creates a minimal router for shadow alignment evaluation mode.
// Only health and OpenAI chat completion endpoints are registered; the chat
// endpoint uses the shadow handler that returns JSON alignment verdicts.
func newShadowRouter(m *mockmetrics.Metrics) http.Handler {
	mux := http.NewServeMux()

	mux.HandleFunc("/health", func(w http.ResponseWriter, _ *http.Request) {
		writeJSON(w, http.StatusOK, map[string]string{"status": "ok", "mode": "shadow"})
	})

	mux.HandleFunc("/v1/chat/completions", handleShadowOpenAI)
	mux.HandleFunc("/chat/completions", handleShadowOpenAI)

	if m != nil {
		mux.Handle("/metrics", promhttp.HandlerFor(m.Registry(), promhttp.HandlerOpts{}))
	}

	return strictRouter(mux)
}

func strictRouter(next *http.ServeMux) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		next.ServeHTTP(w, r)
	})
}
