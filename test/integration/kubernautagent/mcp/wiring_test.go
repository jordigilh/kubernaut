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

	"github.com/go-chi/chi/v5"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	mcpinternal "github.com/jordigilh/kubernaut/internal/kubernautagent/mcp"
	kaserver "github.com/jordigilh/kubernaut/internal/kubernautagent/server"
)

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
	handler, srv := mcpinternal.BootstrapMCP(mcpinternal.MCPDeps{
		AuthMiddleware: fakeAuthMiddleware("test-user"),
	})

	r := chi.NewRouter()
	r.Route("/api/v1", func(r chi.Router) {
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

			resp, err := http.DefaultClient.Do(req)
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

			resp, err := http.DefaultClient.Do(req)
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

			resp, err := http.DefaultClient.Do(req)
			Expect(err).NotTo(HaveOccurred())
			defer resp.Body.Close()

			Expect(resp.Header.Get("Cache-Control")).To(ContainSubstring("no-cache"))
			Expect(resp.Header.Get("X-Accel-Buffering")).To(Equal("no"))
		})
	})
})
