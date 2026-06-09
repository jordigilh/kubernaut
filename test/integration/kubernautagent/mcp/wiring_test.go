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

package mcp_test

import (
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"time"

	"github.com/go-chi/chi/v5"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	mcpinternal "github.com/jordigilh/kubernaut/internal/kubernautagent/mcp"
	kaserver "github.com/jordigilh/kubernaut/internal/kubernautagent/server"
)

var httpTestClient = &http.Client{Timeout: 10 * time.Second}

func fakeAuthMiddleware(user string) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.Header.Get("Authorization") == "" {
				http.Error(w, "Unauthorized", http.StatusUnauthorized)
				return
			}
			next.ServeHTTP(w, r)
		})
	}
}

func newMCPTestRouter() (*httptest.Server, *mcpinternal.MCPServer) {
	authMw := fakeAuthMiddleware("test-user")
	handler, srv := mcpinternal.BootstrapMCP(mcpinternal.MCPDeps{
		AuthMiddleware: authMw,
	})

	r := chi.NewRouter()
	r.Route("/api/v1", func(r chi.Router) {
		r.Use(authMw)
		r.Handle("/mcp", kaserver.SSEHeadersMiddleware(handler))
		r.Handle("/mcp/*", kaserver.SSEHeadersMiddleware(handler))
	})

	ts := httptest.NewServer(r)
	return ts, srv
}

var _ = Describe("MCP Route Wiring — #703 BR-INTERACTIVE-001", func() {

	Describe("IT-KA-703-F01: MCP endpoint responds to authenticated POST (JSON-RPC initialize)", func() {
		It("should return a valid JSON-RPC response to initialize", func() {
			ts, _ := newMCPTestRouter()
			defer ts.Close()

			jsonRPC := `{"jsonrpc":"2.0","id":1,"method":"initialize","params":{"protocolVersion":"2025-03-26","capabilities":{},"clientInfo":{"name":"test","version":"1.0"}}}`

			req, err := http.NewRequest("POST", ts.URL+"/api/v1/mcp", strings.NewReader(jsonRPC))
			Expect(err).NotTo(HaveOccurred())
			req.Header.Set("Content-Type", "application/json")
			req.Header.Set("Accept", "application/json, text/event-stream")
			req.Header.Set("Authorization", "Bearer test-token")

			resp, err := httpTestClient.Do(req)
			Expect(err).NotTo(HaveOccurred())
			defer resp.Body.Close()

			body, err := io.ReadAll(resp.Body)
			Expect(err).NotTo(HaveOccurred())

			Expect(resp.StatusCode).To(Equal(http.StatusOK), "body: %s", string(body))
			Expect(string(body)).To(ContainSubstring("kubernaut-agent-interactive"))
		})
	})

	Describe("IT-KA-703-F02: MCP endpoint rejects unauthenticated request (401)", func() {
		It("should return 401 when no Authorization header", func() {
			ts, _ := newMCPTestRouter()
			defer ts.Close()

			jsonRPC := `{"jsonrpc":"2.0","id":1,"method":"initialize","params":{"protocolVersion":"2025-03-26","capabilities":{},"clientInfo":{"name":"test","version":"1.0"}}}`

			req, err := http.NewRequest("POST", ts.URL+"/api/v1/mcp", strings.NewReader(jsonRPC))
			Expect(err).NotTo(HaveOccurred())
			req.Header.Set("Content-Type", "application/json")

			resp, err := httpTestClient.Do(req)
			Expect(err).NotTo(HaveOccurred())
			defer resp.Body.Close()

			Expect(resp.StatusCode).To(Equal(http.StatusUnauthorized))
		})
	})

	Describe("IT-KA-703-F03: MCP SSE headers applied to response", func() {
		It("should include SSE headers on MCP endpoint responses", func() {
			ts, _ := newMCPTestRouter()
			defer ts.Close()

			jsonRPC := `{"jsonrpc":"2.0","id":1,"method":"initialize","params":{"protocolVersion":"2025-03-26","capabilities":{},"clientInfo":{"name":"test","version":"1.0"}}}`

			req, err := http.NewRequest("POST", ts.URL+"/api/v1/mcp", strings.NewReader(jsonRPC))
			Expect(err).NotTo(HaveOccurred())
			req.Header.Set("Content-Type", "application/json")
			req.Header.Set("Accept", "application/json, text/event-stream")
			req.Header.Set("Authorization", "Bearer test-token")

			resp, err := httpTestClient.Do(req)
			Expect(err).NotTo(HaveOccurred())
			defer resp.Body.Close()

			Expect(resp.Header.Get("Cache-Control")).To(ContainSubstring("no-cache"))
			Expect(resp.Header.Get("X-Accel-Buffering")).To(Equal("no"))
		})
	})
})

var _ = Describe("MCP Auth Architecture — #895/#896 BR-SECURITY-896", func() {

	Describe("IT-KA-895-001: Unauthenticated POST to /api/v1/mcp returns 401", func() {
		It("should reject requests without Authorization header at the router level", func() {
			var authCallCount atomic.Int32
			countingAuth := func(next http.Handler) http.Handler {
				return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					authCallCount.Add(1)
					if r.Header.Get("Authorization") == "" {
						http.Error(w, "Unauthorized", http.StatusUnauthorized)
						return
					}
					next.ServeHTTP(w, r)
				})
			}

			handler, _ := mcpinternal.BootstrapMCP(mcpinternal.MCPDeps{
				AuthMiddleware: countingAuth,
			})

			r := chi.NewRouter()
			r.Route("/api/v1", func(r chi.Router) {
				r.Use(countingAuth)
				r.Handle("/mcp", kaserver.SSEHeadersMiddleware(handler))
				r.Handle("/mcp/*", kaserver.SSEHeadersMiddleware(handler))
			})
			ts := httptest.NewServer(r)
			defer ts.Close()

			jsonRPC := `{"jsonrpc":"2.0","id":1,"method":"initialize","params":{"protocolVersion":"2025-03-26","capabilities":{},"clientInfo":{"name":"test","version":"1.0"}}}`
			req, err := http.NewRequest("POST", ts.URL+"/api/v1/mcp", strings.NewReader(jsonRPC))
			Expect(err).NotTo(HaveOccurred())
			req.Header.Set("Content-Type", "application/json")

			resp, err := httpTestClient.Do(req)
			Expect(err).NotTo(HaveOccurred())
			defer resp.Body.Close()

			Expect(resp.StatusCode).To(Equal(http.StatusUnauthorized))
		})
	})

	Describe("IT-KA-895-002: Auth middleware invoked exactly once per MCP request", func() {
		It("should call auth middleware only at the router level, not inside BootstrapMCP", func() {
			var authCallCount atomic.Int32
			countingAuth := func(next http.Handler) http.Handler {
				return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					authCallCount.Add(1)
					next.ServeHTTP(w, r)
				})
			}

			handler, _ := mcpinternal.BootstrapMCP(mcpinternal.MCPDeps{
				AuthMiddleware: countingAuth,
			})

			r := chi.NewRouter()
			r.Route("/api/v1", func(r chi.Router) {
				r.Use(countingAuth)
				r.Handle("/mcp", kaserver.SSEHeadersMiddleware(handler))
				r.Handle("/mcp/*", kaserver.SSEHeadersMiddleware(handler))
			})
			ts := httptest.NewServer(r)
			defer ts.Close()

			jsonRPC := `{"jsonrpc":"2.0","id":1,"method":"initialize","params":{"protocolVersion":"2025-03-26","capabilities":{},"clientInfo":{"name":"test","version":"1.0"}}}`
			req, err := http.NewRequest("POST", ts.URL+"/api/v1/mcp", strings.NewReader(jsonRPC))
			Expect(err).NotTo(HaveOccurred())
			req.Header.Set("Content-Type", "application/json")
			req.Header.Set("Accept", "application/json, text/event-stream")
			req.Header.Set("Authorization", "Bearer test-token")

			resp, err := httpTestClient.Do(req)
			Expect(err).NotTo(HaveOccurred())
			defer resp.Body.Close()

			Expect(resp.StatusCode).To(Equal(http.StatusOK))
			Expect(authCallCount.Load()).To(Equal(int32(1)),
				"auth middleware must be invoked exactly once (router level only), not 2x (double-auth bug)")
		})
	})

	Describe("IT-KA-895-003: Authenticated request reaches MCP SDK and returns valid response", func() {
		It("should process the request through the MCP SDK when auth passes at router level", func() {
			passAuth := func(next http.Handler) http.Handler {
				return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					if r.Header.Get("Authorization") == "" {
						http.Error(w, "Unauthorized", http.StatusUnauthorized)
						return
					}
					next.ServeHTTP(w, r)
				})
			}

			handler, _ := mcpinternal.BootstrapMCP(mcpinternal.MCPDeps{
				AuthMiddleware: passAuth,
			})

			r := chi.NewRouter()
			r.Route("/api/v1", func(r chi.Router) {
				r.Use(passAuth)
				r.Handle("/mcp", kaserver.SSEHeadersMiddleware(handler))
				r.Handle("/mcp/*", kaserver.SSEHeadersMiddleware(handler))
			})
			ts := httptest.NewServer(r)
			defer ts.Close()

			jsonRPC := `{"jsonrpc":"2.0","id":1,"method":"initialize","params":{"protocolVersion":"2025-03-26","capabilities":{},"clientInfo":{"name":"test","version":"1.0"}}}`
			req, err := http.NewRequest("POST", ts.URL+"/api/v1/mcp", strings.NewReader(jsonRPC))
			Expect(err).NotTo(HaveOccurred())
			req.Header.Set("Content-Type", "application/json")
			req.Header.Set("Accept", "application/json, text/event-stream")
			req.Header.Set("Authorization", "Bearer test-token")

			resp, err := httpTestClient.Do(req)
			Expect(err).NotTo(HaveOccurred())
			defer resp.Body.Close()

			body, err := io.ReadAll(resp.Body)
			Expect(err).NotTo(HaveOccurred())

			Expect(resp.StatusCode).To(Equal(http.StatusOK), "body: %s", string(body))
			Expect(string(body)).To(ContainSubstring("kubernaut-agent-interactive"),
				"MCP SDK must process the request and return server implementation info")
		})
	})
})

// =============================================================================
// IT-KA-1387: MCP Session Resilience — KeepAlive/SessionTimeout Wiring (#1387)
//
// Pyramid Invariant:
//   UT (server_test.go) proves BootstrapMCP accepts KeepAlive/SessionTimeout.
//   IT (this block) proves config flows through to a functional MCP endpoint.
// =============================================================================

var _ = Describe("MCP Session Resilience — KeepAlive/SessionTimeout Wiring (#1387)", Label("integration", "mcp-resilience"), func() {

	It("IT-KA-1387-W01 [SC-8, SC-10]: BootstrapMCP with KeepAlive+SessionTimeout produces functional MCP endpoint", func() {
		authMw := fakeAuthMiddleware("test-user")
		handler, srv := mcpinternal.BootstrapMCP(mcpinternal.MCPDeps{
			AuthMiddleware: authMw,
			KeepAlive:      15 * time.Second,
			SessionTimeout: 10 * time.Minute,
		})
		Expect(handler).NotTo(BeNil())
		Expect(srv).NotTo(BeNil())

		r := chi.NewRouter()
		r.Route("/api/v1", func(r chi.Router) {
			r.Handle("/mcp", kaserver.SSEHeadersMiddleware(handler))
			r.Handle("/mcp/*", kaserver.SSEHeadersMiddleware(handler))
		})
		ts := httptest.NewServer(r)
		defer ts.Close()

		jsonRPC := `{"jsonrpc":"2.0","id":1,"method":"initialize","params":{"protocolVersion":"2025-03-26","capabilities":{},"clientInfo":{"name":"test","version":"1.0"}}}`
		req, err := http.NewRequest("POST", ts.URL+"/api/v1/mcp", strings.NewReader(jsonRPC))
		Expect(err).NotTo(HaveOccurred())
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Accept", "application/json, text/event-stream")
		req.Header.Set("Authorization", "Bearer test-token")

		resp, err := httpTestClient.Do(req)
		Expect(err).NotTo(HaveOccurred())
		defer resp.Body.Close()

		body, err := io.ReadAll(resp.Body)
		Expect(err).NotTo(HaveOccurred())

		Expect(resp.StatusCode).To(Equal(http.StatusOK), "body: %s", string(body))
		Expect(string(body)).To(ContainSubstring("kubernaut-agent-interactive"),
			"SC-8+SC-10: MCP endpoint with resilience config must accept initialize handshake and return valid server info")
	})
})
