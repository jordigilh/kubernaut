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

package middleware

import (
	"context"
	"net/http"
	"net/http/httptest"

	"github.com/go-logr/logr"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/jordigilh/kubernaut/pkg/gateway/middleware"
)

// ============================================================================
// BUSINESS OUTCOME TESTS: Request ID Middleware
// ============================================================================
//
// BR-109: Request ID tracing for debugging
//
// BUSINESS VALUE:
// - Operators can trace requests across Gateway components
// - Every log entry has unique request_id for correlation
// - Request ID returned in response header for client debugging
// ============================================================================

var _ = Describe("BR-109: Request ID Middleware", func() {
	var (
		nextHandler http.Handler
		logger      logr.Logger
		capturedCtx context.Context
	)

	BeforeEach(func() {
		logger = logr.Discard()
		capturedCtx = nil

		// Handler that captures context for verification
		nextHandler = http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			capturedCtx = r.Context()
			w.WriteHeader(http.StatusOK)
		})
	})

	Context("request ID generation", func() {
		It("adds unique request ID to every request", func() {
			// BUSINESS OUTCOME: Every request can be traced in logs
			request := httptest.NewRequest(http.MethodPost, "/api/v1/signals/prometheus", nil)

			handler := middleware.RequestIDMiddleware(logger)(nextHandler)
			recorder := httptest.NewRecorder()
			handler.ServeHTTP(recorder, request)

			requestID := recorder.Header().Get("X-Request-ID")
			Expect(requestID).NotTo(BeEmpty(),
				"Request ID header should be set for client tracing")
			Expect(len(requestID)).To(BeNumerically(">", 20),
				"Request ID should be a UUID")
		})

		It("generates different IDs for different requests", func() {
			// BUSINESS OUTCOME: Each request has unique identifier
			handler := middleware.RequestIDMiddleware(logger)(nextHandler)

			req1 := httptest.NewRequest(http.MethodPost, "/api/v1/signals/prometheus", nil)
			rec1 := httptest.NewRecorder()
			handler.ServeHTTP(rec1, req1)

			req2 := httptest.NewRequest(http.MethodPost, "/api/v1/signals/prometheus", nil)
			rec2 := httptest.NewRecorder()
			handler.ServeHTTP(rec2, req2)

			Expect(rec1.Header().Get("X-Request-ID")).NotTo(Equal(rec2.Header().Get("X-Request-ID")),
				"Each request should have a unique ID")
		})
	})

	Context("context propagation", func() {
		It("makes request ID available in handler context", func() {
			// BUSINESS OUTCOME: Handlers can access request ID for logging
			request := httptest.NewRequest(http.MethodPost, "/api/v1/signals/kubernetes", nil)

			handler := middleware.RequestIDMiddleware(logger)(nextHandler)
			recorder := httptest.NewRecorder()
			handler.ServeHTTP(recorder, request)

			Expect(capturedCtx).NotTo(BeNil())
			requestID := middleware.GetRequestID(capturedCtx)
			Expect(requestID).NotTo(BeEmpty())
			Expect(requestID).NotTo(Equal("unknown"),
				"Request ID should be available in context")
		})

		It("makes logger available in handler context", func() {
			// BUSINESS OUTCOME: Handlers use request-scoped logger
			request := httptest.NewRequest(http.MethodPost, "/api/v1/signals/prometheus", nil)

			handler := middleware.RequestIDMiddleware(logger)(nextHandler)
			recorder := httptest.NewRecorder()
			handler.ServeHTTP(recorder, request)

			Expect(capturedCtx).NotTo(BeNil())
			ctxLogger := middleware.GetLogger(capturedCtx)
			Expect(ctxLogger).NotTo(BeNil(),
				"Logger should be available in context")
		})
	})

	Context("GetRequestID fallback", func() {
		It("returns unknown for empty context", func() {
			// BUSINESS OUTCOME: Safe default when middleware not applied
			ctx := context.Background()
			requestID := middleware.GetRequestID(ctx)
			Expect(requestID).To(Equal("unknown"),
				"Fallback to 'unknown' for empty context")
		})
	})

	Context("GetLogger fallback", func() {
		It("returns discard logger for empty context", func() {
			// BUSINESS OUTCOME: Safe default prevents nil panics
			ctx := context.Background()
			logger := middleware.GetLogger(ctx)
			Expect(logger).NotTo(BeNil(),
				"Should return valid logger even for empty context")
		})
	})
})
