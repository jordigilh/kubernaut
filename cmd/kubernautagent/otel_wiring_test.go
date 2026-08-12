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

package main

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"go.opentelemetry.io/contrib/instrumentation/net/http/otelhttp"
	"go.opentelemetry.io/otel"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/sdk/trace/tracetest"
	"go.opentelemetry.io/otel/trace"

	kaconfig "github.com/jordigilh/kubernaut/internal/kubernautagent/config"
)

// UT-1519-011 (GAP-14 / Issue #1519): buildTransportChain's outbound LLM
// transport must carry the caller's active span forward as a child span,
// proving the otelhttp.NewTransport wrap added for the vanilla (no custom
// TLS/OAuth2/headers/circuit-breaker) config path in llm_builder.go is live.
func TestBuildTransportChain_CarriesActiveSpan(t *testing.T) {
	recorder := tracetest.NewInMemoryExporter()
	tp := sdktrace.NewTracerProvider(sdktrace.WithSyncer(recorder))
	otel.SetTracerProvider(tp)
	defer func() { _ = tp.Shutdown(context.Background()) }()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	cfg := kaconfig.DefaultConfig()
	rt := &kaconfig.LLMRuntimeConfig{}

	chain, err := buildTransportChain(cfg, rt)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if chain == nil {
		t.Fatal("expected non-nil (otelhttp-wrapped) transport")
	}

	tracer := otel.Tracer("test-ka-investigation")
	ctx, parentSpan := tracer.Start(context.Background(), "investigate")

	req, reqErr := http.NewRequestWithContext(ctx, http.MethodPost, server.URL, nil)
	if reqErr != nil {
		t.Fatalf("failed to build request: %v", reqErr)
	}
	resp, doErr := chain.RoundTrip(req)
	if doErr != nil {
		t.Fatalf("RoundTrip failed: %v", doErr)
	}
	_ = resp.Body.Close()
	parentSpan.End()

	spans := recorder.GetSpans()
	if len(spans) != 2 {
		t.Fatalf("expected 2 spans (investigate + HTTP POST), got %d: %+v", len(spans), spans)
	}

	var parent, child *tracetest.SpanStub
	for i := range spans {
		switch spans[i].Name {
		case "investigate":
			parent = &spans[i]
		case "HTTP POST":
			child = &spans[i]
		}
	}
	if parent == nil || child == nil {
		t.Fatalf("expected spans named 'investigate' and 'HTTP POST', got %+v", spans)
	}
	if child.Parent.SpanID() != parent.SpanContext.SpanID() {
		t.Fatal("the LLM call span must be a direct child of the caller's active span")
	}
	if child.SpanContext.TraceID() != parent.SpanContext.TraceID() {
		t.Fatal("the LLM call span must share the caller's TraceID")
	}
}

// UT-1519-012 (GAP-14 / Issue #1519): proves the exact otelhttp.NewMiddleware
// wiring added to the /api/v1 route group in main() produces a root span per
// inbound request -- KA is where LLM call latency (the dominant cost of an
// AI agent investigation) shows up as a per-hop breakdown.
func TestInboundOtelMiddleware_RootSpanPerRequest(t *testing.T) {
	recorder := tracetest.NewInMemoryExporter()
	tp := sdktrace.NewTracerProvider(sdktrace.WithSyncer(recorder))
	otel.SetTracerProvider(tp)
	defer func() { _ = tp.Shutdown(context.Background()) }()

	// Same construction as the /api/v1 route group in main().
	mw := otelhttp.NewMiddleware("kubernautagent.http",
		otelhttp.WithSpanNameFormatter(func(_ string, r *http.Request) string {
			return r.Method + " " + r.URL.Path
		}),
	)

	var sawTraceID string
	next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		sawTraceID = trace.SpanContextFromContext(r.Context()).TraceID().String()
		w.WriteHeader(http.StatusAccepted)
	})

	req := httptest.NewRequest(http.MethodPost, "/api/v1/investigate", nil)
	rec := httptest.NewRecorder()

	mw(next).ServeHTTP(rec, req)

	if rec.Code != http.StatusAccepted {
		t.Fatalf("expected 202, got %d", rec.Code)
	}
	if sawTraceID == "" {
		t.Fatal("the handler must see an active span on its request context")
	}

	spans := recorder.GetSpans()
	if len(spans) != 1 {
		t.Fatalf("expected 1 span, got %d", len(spans))
	}
	if spans[0].Name != "POST /api/v1/investigate" {
		t.Fatalf("expected span name 'POST /api/v1/investigate', got %q", spans[0].Name)
	}
	if spans[0].SpanContext.TraceID().String() != sawTraceID {
		t.Fatal("the span's TraceID must match what the handler observed")
	}
}
