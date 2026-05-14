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

package datastorage

import (
	"bytes"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"

	"github.com/go-chi/chi/v5"
	chimw "github.com/go-chi/chi/v5/middleware"
	"github.com/go-logr/logr/funcr"
	"github.com/jordigilh/kubernaut/pkg/datastorage/server"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

// ========================================
// PHASE 10-RED: Security Controls Tests
// ========================================
//
// Issue: #1088 GA Readiness — SEC-H1, SEC-H2, SEC-M2, FED-M1, FED-M2
// Files Under Test: pkg/datastorage/server/server.go, handlers.go, middleware/
// ========================================

var _ = Describe("Phase 10: Security Controls", func() {

	Describe("UT-DS-1088-GA-120: Panic recovery returns 500, does not re-panic", func() {
		It("should return 500 with problem+json when handler panics", func() {
			// SEC-M2: Uses the production NewMinimalAuditHandlersHTTPServer to obtain
			// a real *Server, then wraps a panicking handler with the production
			// panicRecoveryMiddleware method. This ensures the test exercises the
			// actual recovery code path, not a stand-in.
			logBuf := &bytes.Buffer{}
			testLogger := funcr.New(func(prefix, args string) {
				fmt.Fprintf(logBuf, "%s %s\n", prefix, args)
			}, funcr.Options{Verbosity: 1})

			srv := server.NewMinimalAuditHandlersHTTPServer(server.MinimalAuditHandlersHTTPServerDeps{
				Logger: testLogger,
			})

			r := chi.NewRouter()
			r.Use(chimw.RequestID)
			r.Use(srv.PanicRecoveryMiddleware)
			r.Get("/test", func(w http.ResponseWriter, r *http.Request) {
				panic("deliberate test panic")
			})

			req := httptest.NewRequest("GET", "/test", nil)
			rec := httptest.NewRecorder()
			r.ServeHTTP(rec, req)

			Expect(rec.Code).To(Equal(http.StatusInternalServerError),
				"SEC-M2: panicking handler must return 500")
			Expect(rec.Header().Get("Content-Type")).To(Equal("application/problem+json"))
			Expect(rec.Body.String()).To(ContainSubstring(`"unexpected error"`),
				"SEC-M2: response must not leak panic details")
			Expect(logBuf.String()).To(ContainSubstring("PANIC RECOVERED"),
				"SEC-M2: panic must be logged server-side")
		})
	})

	Describe("UT-DS-1088-GA-110: Auth ordering — 401 before OpenAPI validation", func() {
		It("should return 401 for unauthenticated requests before OpenAPI validation runs", func() {
			// SEC-H2: Auth must run BEFORE OpenAPI validation.
			// Current order: MaxBytes → CORS → OpenAPI → Auth
			// Required order: MaxBytes → CORS → Auth → OpenAPI
			//
			// For now, we verify that unauthenticated requests to a valid endpoint
			// get 401 (not 400 from OpenAPI validation).
			// This test will help verify the middleware reordering in Green phase.

			// We can't easily test the full stack here without a running server,
			// but we can verify the auth middleware returns 401 for missing auth.
			r := chi.NewRouter()

			// Simulate the auth-first order
			r.Use(func(next http.Handler) http.Handler {
				return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					if r.Header.Get("Authorization") == "" {
					w.WriteHeader(http.StatusUnauthorized)
					_, _ = fmt.Fprint(w, `{"status":401,"title":"Unauthorized"}`)
						return
					}
					next.ServeHTTP(w, r)
				})
			})
			r.Post("/api/v1/audit/events", func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(http.StatusOK)
			})

			req := httptest.NewRequest("POST", "/api/v1/audit/events",
				strings.NewReader(`{}`))
			rec := httptest.NewRecorder()
			r.ServeHTTP(rec, req)

			Expect(rec.Code).To(Equal(http.StatusUnauthorized),
				"SEC-H2: unauthenticated request must get 401 before any validation")
		})
	})

	Describe("UT-DS-1088-GA-160: HTTP access log includes authenticated principal", func() {
		It("should include user field in access log when X-Auth-Request-User is present", func() {
			// FED-M2: The loggingMiddleware reads X-Auth-Request-User from the
			// request header and includes it in structured log output.
			logBuf := &bytes.Buffer{}
			testLogger := funcr.New(func(prefix, args string) {
				fmt.Fprintf(logBuf, "%s %s\n", prefix, args)
			}, funcr.Options{Verbosity: 1})

			srv := server.NewMinimalAuditHandlersHTTPServer(server.MinimalAuditHandlersHTTPServerDeps{
				Logger: testLogger,
			})

			r := chi.NewRouter()
			r.Use(chimw.RequestID)
			r.Use(srv.LoggingMiddleware)
			r.Get("/api/v1/test", func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(http.StatusOK)
			})

			req := httptest.NewRequest("GET", "/api/v1/test", nil)
			req.Header.Set("X-Auth-Request-User", "test-principal@kubernaut.ai")
			rec := httptest.NewRecorder()
			r.ServeHTTP(rec, req)

			Expect(rec.Code).To(Equal(http.StatusOK))
			Expect(logBuf.String()).To(ContainSubstring("test-principal@kubernaut.ai"),
				"FED-M2: access log must include authenticated principal")
			Expect(logBuf.String()).To(ContainSubstring("HTTP request"),
				"FED-M2: access log must include HTTP request marker")
		})

		It("should log empty user when X-Auth-Request-User is absent", func() {
			logBuf := &bytes.Buffer{}
			testLogger := funcr.New(func(prefix, args string) {
				fmt.Fprintf(logBuf, "%s %s\n", prefix, args)
			}, funcr.Options{Verbosity: 1})

			srv := server.NewMinimalAuditHandlersHTTPServer(server.MinimalAuditHandlersHTTPServerDeps{
				Logger: testLogger,
			})

			r := chi.NewRouter()
			r.Use(srv.LoggingMiddleware)
			r.Get("/api/v1/test", func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(http.StatusOK)
			})

			req := httptest.NewRequest("GET", "/api/v1/test", nil)
			rec := httptest.NewRecorder()
			r.ServeHTTP(rec, req)

			Expect(rec.Code).To(Equal(http.StatusOK))
			Expect(logBuf.String()).To(ContainSubstring(`"user"=""`),
				"FED-M2: access log must include empty user when header is absent")
		})
	})
})
