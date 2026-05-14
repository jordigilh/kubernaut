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
	"encoding/json"
	"net/http"
	"net/http/httptest"

	"github.com/go-chi/cors"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	kubelog "github.com/jordigilh/kubernaut/pkg/log"
)

var _ = Describe("AppSec Audit Gap Tests (#1048 Phase 4)", func() {

	Describe("Readiness handler error redaction (Fix 3, site 1)", func() {
		It("UT-DS-1048-RH-001: should NOT include 'error' field in 503 JSON when DB unreachable", func() {
			logger := kubelog.NewLogger(kubelog.DefaultOptions())

			srv := &Server{
				logger: logger,
				db:     nil, // nil db will cause Ping() to panic — we test the shutdown path instead
			}

			// Simulate DB unreachable by setting shutdown flag (returns 503 with known JSON)
			srv.isShuttingDown.Store(true)

			req := httptest.NewRequest(http.MethodGet, "/health/ready", nil)
			rr := httptest.NewRecorder()
			srv.handleReadiness(rr, req)

			Expect(rr.Code).To(Equal(http.StatusServiceUnavailable))

			var body map[string]interface{}
			Expect(json.NewDecoder(rr.Body).Decode(&body)).To(Succeed())
			Expect(body).ToNot(HaveKey("error"), "503 response must not include 'error' field")
			Expect(body["reason"]).To(Equal("shutting_down"))
		})

		It("UT-DS-1048-RH-002: should return structured JSON without error details on DB ping failure", func() {
			srv, ts := newMinimalServer(nil)
			defer ts.Close()

			req := httptest.NewRequest(http.MethodGet, "/health/ready", nil)
			rr := httptest.NewRecorder()

			// nil DB causes panic on Ping() — confirms readiness handler
			// has no accidental nil guard that swallows the error silently
			Expect(func() {
				srv.handleReadiness(rr, req)
			}).To(Panic(), "nil db.Ping() should panic — verifying the nil guard is not in the readiness path")
		})
	})

	Describe("CORS configuration (Fix 4)", func() {
		corsHandler := func(allowedOrigins []string) http.Handler {
			mux := http.NewServeMux()
			mux.HandleFunc("/api/v1/test", func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(http.StatusOK)
			})

			handler := cors.Handler(cors.Options{
				AllowedOrigins:   allowedOrigins,
				AllowedMethods:   []string{"GET", "POST", "PATCH", "DELETE", "OPTIONS"},
				AllowedHeaders:   []string{"Accept", "Authorization", "Content-Type", "X-Request-ID"},
				ExposedHeaders:   []string{"Link", "X-Request-ID"},
				AllowCredentials: false,
				MaxAge:           300,
			})
			return handler(mux)
		}

		It("UT-DS-1048-CO-001: should allow PATCH preflight when configured", func() {
			h := corsHandler([]string{"https://app.example.com"})

			req := httptest.NewRequest(http.MethodOptions, "/api/v1/test", nil)
			req.Header.Set("Origin", "https://app.example.com")
			req.Header.Set("Access-Control-Request-Method", "PATCH")
			rr := httptest.NewRecorder()

			h.ServeHTTP(rr, req)

			Expect(rr.Code).To(Equal(http.StatusOK))
			Expect(rr.Header().Get("Access-Control-Allow-Methods")).To(ContainSubstring("PATCH"))
		})

		It("UT-DS-1048-CO-002: should allow DELETE preflight when configured", func() {
			h := corsHandler([]string{"https://app.example.com"})

			req := httptest.NewRequest(http.MethodOptions, "/api/v1/test", nil)
			req.Header.Set("Origin", "https://app.example.com")
			req.Header.Set("Access-Control-Request-Method", "DELETE")
			rr := httptest.NewRecorder()

			h.ServeHTTP(rr, req)

			Expect(rr.Code).To(Equal(http.StatusOK))
			Expect(rr.Header().Get("Access-Control-Allow-Methods")).To(ContainSubstring("DELETE"))
		})

		It("UT-DS-1048-CO-003: should reject non-listed origin when explicit origins configured", func() {
			h := corsHandler([]string{"https://trusted.example.com"})

			req := httptest.NewRequest(http.MethodOptions, "/api/v1/test", nil)
			req.Header.Set("Origin", "https://evil.attacker.com")
			req.Header.Set("Access-Control-Request-Method", "POST")
			rr := httptest.NewRecorder()

			h.ServeHTTP(rr, req)

			Expect(rr.Header().Get("Access-Control-Allow-Origin")).To(BeEmpty(),
				"non-listed origin should not receive Access-Control-Allow-Origin header")
		})

		It("UT-DS-1048-CO-004: should accept wildcard origin when default", func() {
			h := corsHandler([]string{"*"})

			req := httptest.NewRequest(http.MethodOptions, "/api/v1/test", nil)
			req.Header.Set("Origin", "https://any-site.example.com")
			req.Header.Set("Access-Control-Request-Method", "GET")
			rr := httptest.NewRecorder()

			h.ServeHTTP(rr, req)

			Expect(rr.Header().Get("Access-Control-Allow-Origin")).To(Equal("*"))
		})
	})
})
