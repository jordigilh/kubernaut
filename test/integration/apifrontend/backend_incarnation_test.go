package apifrontend_test

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"

	"github.com/go-logr/logr"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/jordigilh/kubernaut/pkg/apifrontend/auth"
	"github.com/jordigilh/kubernaut/pkg/apifrontend/handler"
	"github.com/jordigilh/kubernaut/pkg/apifrontend/metrics"
)

const integrationBackendIncarnation = "01234567-89ab-4cde-8123-456789abcdef"

var _ = Describe("API Frontend backend incarnation contract (#2614)", func() {
	It("IT-AF-2614-001/002: serves the exact public no-store contract without auth", func() {
		resp, err := http.Get(routerServer.URL + "/.well-known/kubernaut-console-contract.json")
		Expect(err).NotTo(HaveOccurred())
		defer resp.Body.Close()

		Expect(resp.StatusCode).To(Equal(http.StatusOK))
		Expect(resp.Header.Get("Content-Type")).To(Equal("application/json"))
		Expect(resp.Header.Get("Cache-Control")).To(Equal("no-store"))
		Expect(resp.Header.Get("X-Kubernaut-Backend-Incarnation")).To(BeEmpty())

		var payload map[string]string
		Expect(json.NewDecoder(resp.Body).Decode(&payload)).To(Succeed())
		Expect(payload).To(HaveLen(1))
		Expect(payload).To(HaveKeyWithValue("backend_incarnation", integrationBackendIncarnation))
	})

	It("IT-AF-2614-003/004/006: carries one matching value on authenticated A2A, root, MCP, and status responses", func() {
		token := signValidToken("it-user-2614")
		paths := []string{"/a2a/invoke", "/", "/mcp", "/a2a/status"}
		bodies := []string{
			`{"jsonrpc":"2.0","method":"message/send","id":"a2a"}`,
			`{"jsonrpc":"2.0","method":"message/send","id":"root"}`,
			`{"jsonrpc":"2.0","method":"initialize","id":"mcp"}`,
			`{"jsonrpc":"2.0","method":"status/subscribe","id":"status","params":{"rr_id":"missing-2614"}}`,
		}

		for i, path := range paths {
			req, err := http.NewRequest(http.MethodPost, routerServer.URL+path, strings.NewReader(bodies[i]))
			Expect(err).NotTo(HaveOccurred())
			req.Header.Set("Authorization", "Bearer "+token)
			req.Header.Set("Content-Type", "application/json")

			resp, err := http.DefaultClient.Do(req)
			Expect(err).NotTo(HaveOccurred())
			body, readErr := io.ReadAll(resp.Body)
			Expect(readErr).NotTo(HaveOccurred())
			Expect(resp.Body.Close()).To(Succeed())
			Expect(resp.Header.Get("X-Kubernaut-Backend-Incarnation")).To(Equal(integrationBackendIncarnation), path)
			Expect(string(body)).NotTo(ContainSubstring(token), path)
		}
	})

	It("IT-AF-2614-008: keeps the lifecycle header outside the auth boundary", func() {
		req, err := http.NewRequest(http.MethodPost, routerServer.URL+"/a2a/invoke", strings.NewReader(`{}`))
		Expect(err).NotTo(HaveOccurred())
		resp, err := http.DefaultClient.Do(req)
		Expect(err).NotTo(HaveOccurred())
		defer resp.Body.Close()

		Expect(resp.StatusCode).To(Equal(http.StatusUnauthorized))
		Expect(resp.Header.Get("X-Kubernaut-Backend-Incarnation")).To(BeEmpty())
	})

	It("IT-AF-2614-007: preserves deterministic MCP SSE behavior around a blocking stub callback", func() {
		started := make(chan struct{})
		release := make(chan struct{})
		var startedOnce sync.Once
		var releaseOnce sync.Once
		mcpHandler, err := handler.NewMCPHandler(handler.MCPConfig{
			ServerName:    "kubernaut-af-2614",
			ServerVersion: "test",
			Enabled:       true,
			Bridge:        nil,
			Tools: []handler.MCPToolDef{{
				Name:        "backend_incarnation_blocking_tool",
				Description: "test tool",
				InputSchema: map[string]any{"type": "object"},
			}},
			ToolCallback: func(ctx context.Context, _ string) {
				startedOnce.Do(func() { close(started) })
				select {
				case <-release:
				case <-ctx.Done():
				}
			},
		})
		Expect(err).NotTo(HaveOccurred())

		localMetrics := metrics.NewRegistry()
		authMiddleware := auth.MiddlewareWithConfig(auth.MiddlewareConfig{
			Validator:    jwtValidator,
			Logger:       logr.Discard(),
			AuthDuration: localMetrics.AuthDuration,
		})
		router, err := handler.NewRouter(handler.RouterConfig{
			MetricsRegistry: localMetrics,
			Logger:          logr.Discard(),
			A2AHandler:      http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusOK) }),
			MCPHandler:      mcpHandler,
			PublicRoutes: handler.PublicRouteConfig{
				AgentCardHandler: http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusOK) }),
				BackendContract:  handler.BackendContractConfig{Incarnation: integrationBackendIncarnation},
			},
			AuthMiddleware: authMiddleware,
			ReadyChecker:   func() bool { return true },
		})
		Expect(err).NotTo(HaveOccurred())
		server := httptest.NewServer(router)
		DeferCleanup(server.Close)
		DeferCleanup(func() { releaseOnce.Do(func() { close(release) }) })

		token := signValidToken("it-user-2614-mcp")
		postMCP := func(ctx context.Context, sessionID, body string) *http.Response {
			req, requestErr := http.NewRequestWithContext(ctx, http.MethodPost, server.URL+"/mcp", strings.NewReader(body))
			Expect(requestErr).NotTo(HaveOccurred())
			req.Header.Set("Authorization", "Bearer "+token)
			req.Header.Set("Content-Type", "application/json")
			req.Header.Set("Accept", "application/json, text/event-stream")
			if sessionID != "" {
				req.Header.Set("Mcp-Session-Id", sessionID)
			}
			response, requestErr := http.DefaultClient.Do(req)
			Expect(requestErr).NotTo(HaveOccurred())
			return response
		}

		initialize := postMCP(context.Background(), "", `{"jsonrpc":"2.0","id":1,"method":"initialize","params":{"protocolVersion":"2025-03-26","capabilities":{},"clientInfo":{"name":"test","version":"1"}}}`)
		Expect(initialize.StatusCode).To(Equal(http.StatusOK))
		sessionID := initialize.Header.Get("Mcp-Session-Id")
		Expect(sessionID).NotTo(BeEmpty())
		Expect(initialize.Body.Close()).To(Succeed())

		notification := postMCP(context.Background(), sessionID, `{"jsonrpc":"2.0","method":"notifications/initialized"}`)
		Expect(notification.StatusCode).To(Equal(http.StatusAccepted))
		Expect(notification.Body.Close()).To(Succeed())

		responseCh := make(chan *http.Response, 1)
		requestCtx, cancel := context.WithCancel(context.Background())
		DeferCleanup(cancel)
		go func() {
			responseCh <- postMCP(requestCtx, sessionID, `{"jsonrpc":"2.0","id":2,"method":"tools/call","params":{"name":"backend_incarnation_blocking_tool","arguments":{}}}`)
		}()
		Eventually(started, "2s", "10ms").Should(BeClosed())
		// The SDK sets SSE headers before dispatch but does not flush the first
		// response frame until the callback returns. Release before awaiting Do.
		releaseOnce.Do(func() { close(release) })

		var response *http.Response
		Eventually(responseCh, "5s", "10ms").Should(Receive(&response))
		Expect(response.StatusCode).To(Equal(http.StatusOK))
		Expect(response.Header.Get("Content-Type")).To(ContainSubstring("text/event-stream"))
		Expect(response.Header.Get("X-Kubernaut-Backend-Incarnation")).To(Equal(integrationBackendIncarnation))
		body, err := io.ReadAll(response.Body)
		Expect(err).NotTo(HaveOccurred())
		Expect(response.Body.Close()).To(Succeed())
		Expect(string(body)).To(ContainSubstring("backend_incarnation_blocking_tool"))
	})
})
