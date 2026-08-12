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

	"github.com/jordigilh/kubernaut/pkg/audit"
	ogenclient "github.com/jordigilh/kubernaut/pkg/datastorage/ogen-client"
	"github.com/jordigilh/kubernaut/pkg/shared/auth"
	sharedtls "github.com/jordigilh/kubernaut/pkg/shared/tls"
)

// UT-1519-009: proves the exact wiring added to NewServerWithMetrics's
// Data Storage audit-client construction (otelhttp.NewTransport wrapping
// the same default base+auth transport chain pkg/audit builds internally)
// produces a span that is a child of the caller's active span (GAP-14 /
// Issue #1519) -- Gateway's HTTP request span correlates with its outbound
// audit write, without modifying the shared pkg/audit adapter used by 9
// other services.
var _ = Describe("GAP-14 / Issue #1519: Gateway outbound DS audit span (otelhttp.NewTransport wiring)", func() {
	It("emits a span that is a child of the caller's active span", func() {
		recorder := tracetest.NewInMemoryExporter()
		tp := sdktrace.NewTracerProvider(sdktrace.WithSyncer(recorder))
		otel.SetTracerProvider(tp)
		defer func() { _ = tp.Shutdown(context.Background()) }()

		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusCreated)
			_, _ = w.Write([]byte(`{}`))
		}))
		defer server.Close()

		// Same construction as NewServerWithMetrics when cfg.DataStorage.Transport is nil.
		baseTransport, err := sharedtls.DefaultBaseTransportWithRetry()
		Expect(err).NotTo(HaveOccurred())
		tracedTransport := otelhttp.NewTransport(auth.NewAuthTransport(auth.NewDefaultTokenSource(), baseTransport))

		dsClient, err := audit.NewOpenAPIClientAdapterWithTransport(server.URL, 0, tracedTransport)
		Expect(err).NotTo(HaveOccurred())

		// Simulate the "Gateway HTTP request span" that setupRoutes()'s
		// otelhttp.NewMiddleware would already have started by the time the
		// handler reaches the audit write.
		tracer := otel.Tracer("test-gateway-http")
		ctx, parentSpan := tracer.Start(context.Background(), "POST /api/v1/signals/prometheus")

		event := audit.NewAuditEventRequest()
		_ = dsClient.StoreBatch(ctx, []*ogenclient.AuditEventRequest{event})
		parentSpan.End()

		spans := recorder.GetSpans()
		// FINDING (same as AuthWebhook's ds_client_tracing_test.go): the
		// ogen-generated Data Storage client already carries its own OTel
		// instrumentation, producing a free extra span
		// ("CreateAuditEventsBatch") between the admission span and the
		// otelhttp-wrapped transport's "HTTP POST" span.
		Expect(spans).To(HaveLen(3), "expected admission + ogen client + otelhttp transport spans")

		byName := map[string]*tracetest.SpanStub{}
		for i := range spans {
			byName[spans[i].Name] = &spans[i]
		}
		admission := byName["POST /api/v1/signals/prometheus"]
		ogenSpan := byName["CreateAuditEventsBatch"]
		httpSpan := byName["HTTP POST"]
		Expect(admission).NotTo(BeNil())
		Expect(ogenSpan).NotTo(BeNil())
		Expect(httpSpan).NotTo(BeNil())

		traceID := admission.SpanContext.TraceID()
		Expect(ogenSpan.SpanContext.TraceID()).To(Equal(traceID), "ogen client span must share the admission span's TraceID")
		Expect(httpSpan.SpanContext.TraceID()).To(Equal(traceID), "otelhttp transport span must share the admission span's TraceID")

		Expect(ogenSpan.Parent.SpanID()).To(Equal(admission.SpanContext.SpanID()), "ogen client span must be a direct child of the admission span")
		Expect(httpSpan.Parent.SpanID()).To(Equal(ogenSpan.SpanContext.SpanID()), "otelhttp transport span must be a direct child of the ogen client span")
	})
})
