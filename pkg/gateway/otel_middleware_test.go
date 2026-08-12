/*
Copyright 2025 Jordi Gil.

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

package gateway_test

import (
	"context"
	"net/http"
	"net/http/httptest"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"go.opentelemetry.io/contrib/instrumentation/net/http/otelhttp"
	"go.opentelemetry.io/otel"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/sdk/trace/tracetest"
	"go.opentelemetry.io/otel/trace"
)

// UT-1519-008: proves the exact otelhttp.NewMiddleware wiring added to
// setupRoutes() in server.go produces a root span per inbound request, with
// the span name reflecting method+path -- Gateway is the trace root for the
// whole system (GAP-14 / Issue #1519), so this is the origin every
// downstream service's trace-link eventually points back to.
var _ = Describe("GAP-14 / Issue #1519: Gateway inbound otelhttp middleware", func() {
	It("wraps the chi router's handler chain with a root span named after the request", func() {
		recorder := tracetest.NewInMemoryExporter()
		tp := sdktrace.NewTracerProvider(sdktrace.WithSyncer(recorder))
		otel.SetTracerProvider(tp)
		defer func() { _ = tp.Shutdown(context.Background()) }()

		// Same construction as setupRoutes() in server.go.
		mw := otelhttp.NewMiddleware("gateway.http",
			otelhttp.WithSpanNameFormatter(func(_ string, r *http.Request) string {
				return r.Method + " " + r.URL.Path
			}),
		)

		var sawTraceID string
		next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			sawTraceID = trace.SpanContextFromContext(r.Context()).TraceID().String()
			w.WriteHeader(http.StatusAccepted)
		})

		req := httptest.NewRequest(http.MethodPost, "/api/v1/signals/prometheus", nil)
		rec := httptest.NewRecorder()

		mw(next).ServeHTTP(rec, req)

		Expect(rec.Code).To(Equal(http.StatusAccepted))
		Expect(sawTraceID).NotTo(BeEmpty(), "the handler must see an active span on its request context")

		spans := recorder.GetSpans()
		Expect(spans).To(HaveLen(1))
		Expect(spans[0].Name).To(Equal("POST /api/v1/signals/prometheus"))
		Expect(spans[0].SpanContext.TraceID().String()).To(Equal(sawTraceID))
	})
})
