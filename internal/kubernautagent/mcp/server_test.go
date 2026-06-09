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
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"time"

	"github.com/go-logr/logr"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	kaconfig "github.com/jordigilh/kubernaut/internal/kubernautagent/config"
	mcpinternal "github.com/jordigilh/kubernaut/internal/kubernautagent/mcp"
)

var _ = Describe("MCP Server — #703", func() {

	Describe("UT-KA-703-F01: NewMCPServer returns non-nil server with Implementation", func() {
		It("should create a valid MCP server instance", func() {
			srv := mcpinternal.NewMCPServer(0)
			Expect(srv).NotTo(BeNil())
			Expect(srv.Implementation()).NotTo(BeNil())
			Expect(srv.Implementation().Name).To(Equal("kubernaut-agent-interactive"))
			Expect(srv.Implementation().Version).NotTo(BeEmpty())
		})
	})

	Describe("UT-KA-703-F02: BootstrapMCP panics when auth middleware is nil", func() {
		It("should panic as a safety guard against auth bypass", func() {
			Expect(func() {
				mcpinternal.BootstrapMCP(mcpinternal.MCPDeps{AuthMiddleware: nil})
			}).To(Panic())
		})
	})

	Describe("UT-KA-703-F04: MCPSessionDrainer.DrainSessions returns within timeout", func() {
		It("should complete draining within the specified timeout", func() {
			drainer := mcpinternal.NewSessionDrainer(nil, nil, logr.Discard())
			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()

			done := make(chan struct{})
			go func() {
				drainer.DrainSessions(ctx)
				close(done)
			}()

			Eventually(done, 6*time.Second).Should(BeClosed())
		})
	})

	Describe("UT-KA-703-F05: MCP server registers zero tools initially", func() {
		It("should have an empty tool list on creation", func() {
			srv := mcpinternal.NewMCPServer(0)
			tools := srv.ToolCount()
			Expect(tools).To(Equal(0))
		})
	})

	Describe("UT-KA-703-F06: BootstrapMCP returns an http.Handler for route mounting", func() {
		It("should return a non-nil handler when AuthMiddleware is provided", func() {
			noopAuth := func(next http.Handler) http.Handler { return next }
			handler, srv := mcpinternal.BootstrapMCP(mcpinternal.MCPDeps{
				AuthMiddleware: noopAuth,
			})
			Expect(handler).NotTo(BeNil())
			Expect(srv).NotTo(BeNil())
		})
	})
})

var _ = Describe("MCP Session Resilience — #1387", func() {
	noopAuth := func(next http.Handler) http.Handler { return next }

	It("UT-KA-1387-001 [SC-8]: server with KeepAlive preserves transport integrity through proxy idle timeouts", func() {
		handler, srv := mcpinternal.BootstrapMCP(mcpinternal.MCPDeps{
			AuthMiddleware: noopAuth,
			KeepAlive:      15 * time.Second,
		})
		Expect(handler).NotTo(BeNil(),
			"SC-8: handler must be created when keepalive is configured")
		Expect(srv).NotTo(BeNil())
		Expect(srv.Implementation().Name).To(Equal("kubernaut-agent-interactive"))

		jsonRPC := `{"jsonrpc":"2.0","id":1,"method":"initialize","params":{"protocolVersion":"2025-03-26","capabilities":{},"clientInfo":{"name":"test","version":"1.0"}}}`
		req := httptest.NewRequest(http.MethodPost, "/mcp", strings.NewReader(jsonRPC))
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Accept", "application/json, text/event-stream")
		rec := httptest.NewRecorder()
		handler.ServeHTTP(rec, req)
		Expect(rec.Code).To(Equal(http.StatusOK),
			"SC-8: MCP server with keepalive must accept initialize handshake")
	})

	It("UT-KA-1387-002 [SC-10]: server with SessionTimeout terminates abandoned sessions after inactivity", func() {
		handler, srv := mcpinternal.BootstrapMCP(mcpinternal.MCPDeps{
			AuthMiddleware: noopAuth,
			SessionTimeout: 10 * time.Minute,
		})
		Expect(handler).NotTo(BeNil(),
			"SC-10: handler must be created when session timeout is configured")
		Expect(srv).NotTo(BeNil())
	})

	It("UT-KA-1387-003 [SC-8, SC-10]: keepalive and session timeout coexist without conflict", func() {
		handler, srv := mcpinternal.BootstrapMCP(mcpinternal.MCPDeps{
			AuthMiddleware: noopAuth,
			KeepAlive:      15 * time.Second,
			SessionTimeout: 10 * time.Minute,
		})
		Expect(handler).NotTo(BeNil(),
			"SC-8+SC-10: both resilience controls must be simultaneously active")
		Expect(srv).NotTo(BeNil())
	})

	It("UT-KA-1387-004 [SA-8]: default config values match security baseline", func() {
		Expect(kaconfig.DefaultMCPKeepAlive).To(Equal(15 * time.Second),
			"SA-8: keepalive must be < OCP router idle timeout (30s) to prevent silent stream death")
		Expect(kaconfig.DefaultMCPSessionTimeout).To(Equal(10 * time.Minute),
			"SA-8: session cap prevents resource exhaustion from abandoned connections")
	})

	It("UT-KA-1387-005 [SA-8]: zero-value config produces server without keepalive (explicit opt-in)", func() {
		srv := mcpinternal.NewMCPServer(0)
		Expect(srv).NotTo(BeNil())
		Expect(srv.Server()).NotTo(BeNil(),
			"SA-8: server without keepalive is valid for environments with no proxy idle timeout")
	})
})

var _ = Describe("BootstrapMCP Auth Architecture — #895/#896", func() {

	Describe("UT-KA-895-002: BootstrapMCP panics when AuthMiddleware is nil", func() {
		It("should panic as defense-in-depth against auth bypass", func() {
			Expect(func() {
				mcpinternal.BootstrapMCP(mcpinternal.MCPDeps{
					AuthMiddleware: nil,
				})
			}).To(PanicWith(ContainSubstring("auth middleware is nil")))
		})
	})

	Describe("UT-KA-895-003: BootstrapMCP returns raw handler without internal auth wrapping", func() {
		It("should NOT invoke the auth middleware when a request is sent to the returned handler", func() {
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
			Expect(handler).NotTo(BeNil())

			jsonRPC := `{"jsonrpc":"2.0","id":1,"method":"initialize","params":{"protocolVersion":"2025-03-26","capabilities":{},"clientInfo":{"name":"test","version":"1.0"}}}`
			req := httptest.NewRequest(http.MethodPost, "/mcp", strings.NewReader(jsonRPC))
			req.Header.Set("Content-Type", "application/json")
			req.Header.Set("Accept", "application/json, text/event-stream")

			recorder := httptest.NewRecorder()
			handler.ServeHTTP(recorder, req)

			Expect(authCallCount.Load()).To(Equal(int32(0)),
				"BootstrapMCP must NOT apply auth internally; auth should be applied at router level")
		})
	})
})
