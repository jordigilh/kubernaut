package handler_test

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"time"

	"github.com/go-logr/logr"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/jordigilh/kubernaut/pkg/apifrontend/audit"
	"github.com/jordigilh/kubernaut/pkg/apifrontend/auth"
	"github.com/jordigilh/kubernaut/pkg/apifrontend/handler"
	"github.com/jordigilh/kubernaut/pkg/apifrontend/metrics"
)

// fakeConsoleAuthorizer implements auth.ConsoleAuthorizer for the GET
// /a2a/access IT tests, independent of the concrete SARChecker.
type fakeConsoleAuthorizer struct {
	allow bool
	err   error
}

func (f *fakeConsoleAuthorizer) CheckConsoleAccess(_ context.Context, _ string, _ []string) (bool, error) {
	return f.allow, f.err
}

// consoleTestUser is the identity injected by the fake AuthMiddleware used
// throughout this file's IT tests once a bearer token is present.
var consoleTestUser = &auth.UserIdentity{Username: "sre-user@example.com", Groups: []string{"sre"}}

// fakeIdentityAuthMiddleware mirrors the production AuthMiddleware's
// contract (401 without a bearer token, auth.UserIdentity injected into
// context otherwise) without exercising real JWT validation -- that is
// already covered by pkg/apifrontend/auth's own test suite. This lets these
// tests exercise the real handler.NewRouter + real route wiring +
// checkRBAC/console-access-handler logic.
func fakeIdentityAuthMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") == "" {
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}
		ctx := auth.WithUserIdentity(r.Context(), consoleTestUser)
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

var _ = Describe("Console Access Endpoint (#1919)", func() {
	var (
		metricsReg     *metrics.Registry
		consoleAuditor *fakeAuditor
		newRouterWith  func(checker auth.ConsoleAuthorizer) http.Handler
	)

	BeforeEach(func() {
		metricsReg = metrics.NewRegistry()
		consoleAuditor = &fakeAuditor{}
		newRouterWith = func(checker auth.ConsoleAuthorizer) http.Handler {
			router, err := handler.NewRouter(handler.RouterConfig{
				MetricsRegistry:      metricsReg,
				A2AHandler:           http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusOK) }),
				MCPHandler:           http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusOK) }),
				AgentCardHandler:     http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusOK) }),
				ConsoleAccessHandler: handler.NewConsoleAccessHandler(checker, consoleAuditor, logr.Discard()),
				AuthMiddleware:       fakeIdentityAuthMiddleware,
				ReadyChecker:         func() bool { return true },
			})
			Expect(err).NotTo(HaveOccurred())
			return router
		}
	})

	It("IT-AF-1919-001 [AC-3]: GET /a2a/access returns 200 when console access is allowed", func() {
		router := newRouterWith(&fakeConsoleAuthorizer{allow: true})
		req := httptest.NewRequest(http.MethodGet, "/a2a/access", http.NoBody)
		req.Header.Set("Authorization", "Bearer test-token")
		rec := httptest.NewRecorder()
		router.ServeHTTP(rec, req)
		Expect(rec.Code).To(Equal(http.StatusOK))
	})

	It("IT-AF-1919-002 [AC-3]: GET /a2a/access returns 403 problem+json when console access is denied", func() {
		router := newRouterWith(&fakeConsoleAuthorizer{allow: false})
		req := httptest.NewRequest(http.MethodGet, "/a2a/access", http.NoBody)
		req.Header.Set("Authorization", "Bearer test-token")
		rec := httptest.NewRecorder()
		router.ServeHTTP(rec, req)
		Expect(rec.Code).To(Equal(http.StatusForbidden))
		Expect(rec.Header().Get("Content-Type")).To(Equal("application/problem+json"))
	})

	It("IT-AF-1919-003 [AU-12]: denial emits an EventAuthAccessDenied audit event with endpoint=console", func() {
		router := newRouterWith(&fakeConsoleAuthorizer{allow: false})
		req := httptest.NewRequest(http.MethodGet, "/a2a/access", http.NoBody)
		req.Header.Set("Authorization", "Bearer test-token")
		rec := httptest.NewRecorder()
		router.ServeHTTP(rec, req)

		Eventually(consoleAuditor.Events).Should(HaveLen(1))
		ev := consoleAuditor.Events()[0]
		Expect(ev.Type).To(Equal(audit.EventAuthAccessDenied))
		Expect(ev.UserID).To(Equal(consoleTestUser.Username))
		Expect(ev.Detail["endpoint"]).To(Equal("console"))
	})

	It("IT-AF-1919-004 [IA-2]: GET /a2a/access returns 401 without a bearer token (route sits behind AuthMiddleware)", func() {
		router := newRouterWith(&fakeConsoleAuthorizer{allow: true})
		req := httptest.NewRequest(http.MethodGet, "/a2a/access", http.NoBody)
		rec := httptest.NewRecorder()
		router.ServeHTTP(rec, req)
		Expect(rec.Code).To(Equal(http.StatusUnauthorized))
	})

	It("IT-AF-1919-005 [AC-3]: bypass-closure -- real POST /mcp tool call is denied when console access is denied but tool access is allowed, without ever calling GET /a2a/access", func() {
		fakeK8s := newFakeDynamicClient()
		bridgeAuditor := &fakeAuditor{}
		mcpCfg := handler.MCPConfig{
			ServerName:    "kubernaut-apifrontend",
			ServerVersion: "v0.1.0-test",
			Enabled:       true,
			Bridge: &handler.MCPBridgeConfig{
				K8sClient:          fakeK8s,
				TypedClient:        newBridgeTypedClient(),
				Namespace:          "default",
				Authorizer:         &dualAuthorizer{toolAllowed: true, consoleAllowed: false},
				Auditor:            bridgeAuditor,
				Metrics:            newBridgeMetrics(),
				ToolTimeout:        2 * time.Second,
				MaxConcurrentTools: 5,
			},
		}
		mcpHandler, err := handler.NewMCPHandler(mcpCfg)
		Expect(err).NotTo(HaveOccurred())

		router, err := handler.NewRouter(handler.RouterConfig{
			MetricsRegistry:  metricsReg,
			A2AHandler:       http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusOK) }),
			MCPHandler:       mcpHandler,
			AgentCardHandler: http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusOK) }),
			AuthMiddleware:   fakeIdentityAuthMiddleware,
			ReadyChecker:     func() bool { return true },
		})
		Expect(err).NotTo(HaveOccurred())

		// No request to GET /a2a/access is ever made -- proves a client that
		// never calls the advisory endpoint cannot bypass the console gate
		// by going straight to POST /mcp through the real production router.
		sid := mcpInitializeThroughRouter(router)
		status, respBody := mcpCallToolThroughRouter(router, sid, kubernautListRemediations, map[string]any{})
		Expect(status).To(Equal(http.StatusOK), "MCP transport itself succeeds; denial is inside the tool result")
		Expect(isErrorResult(respBody)).To(BeTrue(),
			"AC-3: a client that never calls GET /a2a/access must still be denied via the real router + checkRBAC")
		Expect(extractTextContent(respBody)).To(ContainSubstring("console"))
	})
})

// mcpInitializeThroughRouter and mcpCallToolThroughRouter mirror
// mcpInitialize/mcpCallTool (mcp_bridge_test.go) but drive identity via the
// fakeIdentityAuthMiddleware bearer-token contract (Authorization header)
// instead of injecting it directly into the request context, since the
// handler under test here is the full production router, not the bare MCP
// handler.
func mcpInitializeThroughRouter(h http.Handler) string {
	initReq := map[string]any{
		"jsonrpc": "2.0",
		"id":      1,
		"method":  "initialize",
		"params": map[string]any{
			"protocolVersion": "2025-03-26",
			"capabilities":    map[string]any{},
			"clientInfo":      map[string]any{"name": "test-client", "version": "1.0"},
		},
	}
	rec := mcpPostThroughRouter(h, "", initReq)
	Expect(rec.Code).To(Equal(http.StatusOK))
	sessionID := rec.Header().Get("Mcp-Session-Id")
	Expect(sessionID).NotTo(BeEmpty())

	notif := map[string]any{
		"jsonrpc": "2.0",
		"method":  "notifications/initialized",
	}
	mcpPostThroughRouter(h, sessionID, notif)
	return sessionID
}

func mcpCallToolThroughRouter(h http.Handler, sessionID, toolName string, args map[string]any) (status int, body string) {
	callReq := map[string]any{
		"jsonrpc": "2.0",
		"id":      2,
		"method":  "tools/call",
		"params": map[string]any{
			"name":      toolName,
			"arguments": args,
		},
	}
	rec := mcpPostThroughRouter(h, sessionID, callReq)
	return rec.Code, rec.Body.String()
}

func mcpPostThroughRouter(h http.Handler, sessionID string, body any) *httptest.ResponseRecorder {
	data, _ := json.Marshal(body)
	req := httptest.NewRequest(http.MethodPost, "/mcp", bytes.NewReader(data))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json, text/event-stream")
	req.Header.Set("Authorization", "Bearer test-token")
	if sessionID != "" {
		req.Header.Set("Mcp-Session-Id", sessionID)
	}
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	return rec
}
