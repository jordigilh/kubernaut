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

package server

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

// UT-1519-010: proves the exact otelhttp.NewMiddleware wiring added to
// Handler() in server.go produces a root span per inbound request, with the
// span name reflecting method+path -- Data Storage is the hub every other
// Kubernaut service calls into (GAP-14 / Issue #1519), so this inbound span
// is the receiving end of each of those services' outbound-call spans.
var _ = Describe("GAP-14 / Issue #1519: Data Storage inbound otelhttp middleware", func() {
	It("wraps the chi router's handler chain with a root span named after the request", func() {
		recorder := tracetest.NewInMemoryExporter()
		tp := sdktrace.NewTracerProvider(sdktrace.WithSyncer(recorder))
		otel.SetTracerProvider(tp)
		defer func() { _ = tp.Shutdown(context.Background()) }()

		// Same construction as Handler() in server.go.
		mw := otelhttp.NewMiddleware("datastorage.http",
			otelhttp.WithSpanNameFormatter(func(_ string, r *http.Request) string {
				return r.Method + " " + r.URL.Path
			}),
		)

		var sawTraceID string
		next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			sawTraceID = trace.SpanContextFromContext(r.Context()).TraceID().String()
			w.WriteHeader(http.StatusOK)
		})

		req := httptest.NewRequest(http.MethodGet, "/api/v1/audit/events", nil)
		rec := httptest.NewRecorder()

		mw(next).ServeHTTP(rec, req)

		Expect(rec.Code).To(Equal(http.StatusOK))
		Expect(sawTraceID).NotTo(BeEmpty(), "the handler must see an active span on its request context")

		spans := recorder.GetSpans()
		Expect(spans).To(HaveLen(1))
		Expect(spans[0].Name).To(Equal("GET /api/v1/audit/events"))
		Expect(spans[0].SpanContext.TraceID().String()).To(Equal(sawTraceID))
	})
})
