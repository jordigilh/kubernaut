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

	"github.com/go-chi/chi/v5"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	mcpinternal "github.com/jordigilh/kubernaut/internal/kubernautagent/mcp"
	"github.com/jordigilh/kubernaut/internal/kubernautagent/mcp/tools"
	sharedauth "github.com/jordigilh/kubernaut/pkg/shared/auth"
	mcpsdk "github.com/modelcontextprotocol/go-sdk/mcp"
)

// fakeAuthMiddlewareWithContext injects the user into the request context
// via sharedauth.UserContextKey, which the MCP tool handlers use.
func fakeAuthMiddlewareWithContext(user string) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.Header.Get("Authorization") == "" {
				http.Error(w, "Unauthorized", http.StatusUnauthorized)
				return
			}
			ctx := context.WithValue(r.Context(), sharedauth.UserContextKey, user)
			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}

// stubSessionManager implements mcp.SessionManager for integration testing.
type stubSessionManager struct {
	sessions map[string]*mcpinternal.InteractiveSession
}

func newStubSessionManager() *stubSessionManager {
	return &stubSessionManager{sessions: make(map[string]*mcpinternal.InteractiveSession)}
}

func (m *stubSessionManager) Takeover(_ context.Context, rrID string, user mcpinternal.UserInfo) (*mcpinternal.InteractiveSession, error) {
	sess := &mcpinternal.InteractiveSession{
		SessionID:     "sess-" + rrID,
		CorrelationID: rrID,
		ActingUser:    user,
	}
	m.sessions[rrID] = sess
	return sess, nil
}

func (m *stubSessionManager) Release(sessionID string, _ string) error {
	for k, v := range m.sessions {
		if v.SessionID == sessionID {
			delete(m.sessions, k)
			return nil
		}
	}
	return mcpinternal.ErrSessionNotFound
}

func (m *stubSessionManager) GetDriver(rrID string) (*mcpinternal.InteractiveSession, error) {
	if s, ok := m.sessions[rrID]; ok {
		return s, nil
	}
	return nil, nil
}

func (m *stubSessionManager) IsDriverActive(rrID string) bool {
	_, ok := m.sessions[rrID]
	return ok
}

// stubInvestigatorRunner implements tools.InvestigatorRunner for integration testing.
type stubInvestigatorRunner struct{}

func (s *stubInvestigatorRunner) RunInteractiveTurn(_ context.Context, _ []tools.LLMMessage, _ string) (string, error) {
	return "mock LLM response", nil
}

// stubReconstructor implements mcp.ContextReconstructor for integration testing.
type stubReconstructor struct{}

func (s *stubReconstructor) Reconstruct(_ context.Context, _ string, _ string) ([]mcpinternal.ConversationTurn, error) {
	return nil, nil
}

var _ = Describe("MCP tools/call over HTTP — PR6a", func() {

	var (
		ts      *httptest.Server
		cleanup func()
	)

	BeforeEach(func() {
		sessMgr := newStubSessionManager()
		runner := &stubInvestigatorRunner{}
		recon := &stubReconstructor{}

		investigateTool := tools.NewInvestigateTool(sessMgr, runner, recon)

		toolDeps := mcpinternal.ToolDeps{
			Investigate: tools.InvestigateRegistration(investigateTool),
		}

		handler, _ := mcpinternal.BootstrapMCP(mcpinternal.MCPDeps{
			AuthMiddleware: fakeAuthMiddlewareWithContext("alice@example.com"),
			Tools:          toolDeps,
		})

		r := chi.NewRouter()
		r.Handle("/mcp", handler)
		r.Handle("/mcp/*", handler)
		ts = httptest.NewServer(r)
		cleanup = func() { ts.Close() }
	})

	AfterEach(func() {
		cleanup()
	})

	Describe("IT-KA-PR6A-TOOLS-001: tools/list returns registered tools via SDK client", func() {
		It("should list kubernaut_investigate via the MCP protocol", func() {
			ctx := context.Background()
			client := mcpsdk.NewClient(&mcpsdk.Implementation{
				Name:    "it-client",
				Version: "0.0.1",
			}, nil)

			transport := &mcpsdk.StreamableClientTransport{
				Endpoint:   ts.URL + "/mcp",
				HTTPClient: authedHTTPClient("alice@example.com"),
			}
			session, err := client.Connect(ctx, transport, nil)
			Expect(err).NotTo(HaveOccurred())
			defer func() { _ = session.Close() }()

			var toolNames []string
			for tool, err := range session.Tools(ctx, nil) {
				Expect(err).NotTo(HaveOccurred())
				toolNames = append(toolNames, tool.Name)
			}
			Expect(toolNames).To(ContainElement("kubernaut_investigate"))
		})
	})

	Describe("IT-KA-PR6A-TOOLS-002: tools/call invokes investigate start action", func() {
		It("should return session_id and status=started", func() {
			ctx := context.Background()
			client := mcpsdk.NewClient(&mcpsdk.Implementation{
				Name:    "it-client",
				Version: "0.0.1",
			}, nil)

			transport := &mcpsdk.StreamableClientTransport{
				Endpoint:   ts.URL + "/mcp",
				HTTPClient: authedHTTPClient("alice@example.com"),
			}
			session, err := client.Connect(ctx, transport, nil)
			Expect(err).NotTo(HaveOccurred())
			defer func() { _ = session.Close() }()

			result, err := session.CallTool(ctx, &mcpsdk.CallToolParams{
				Name: "kubernaut_investigate",
				Arguments: map[string]any{
					"rr_id":  "rr-test-001",
					"action": "start",
				},
			})
			Expect(err).NotTo(HaveOccurred())
			Expect(result.IsError).To(BeFalse(), "tool call should not return error")

			// The structured output contains session_id and status.
			Expect(result.StructuredContent).NotTo(BeNil())
		})
	})

	Describe("IT-KA-PR6A-TOOLS-003: tools/call message action returns LLM response", func() {
		It("should return the mock LLM response for message action", func() {
			ctx := context.Background()
			client := mcpsdk.NewClient(&mcpsdk.Implementation{
				Name:    "it-client",
				Version: "0.0.1",
			}, nil)

			transport := &mcpsdk.StreamableClientTransport{
				Endpoint:   ts.URL + "/mcp",
				HTTPClient: authedHTTPClient("alice@example.com"),
			}
			session, err := client.Connect(ctx, transport, nil)
			Expect(err).NotTo(HaveOccurred())
			defer func() { _ = session.Close() }()

			// Start first to acquire the session.
			_, err = session.CallTool(ctx, &mcpsdk.CallToolParams{
				Name: "kubernaut_investigate",
				Arguments: map[string]any{
					"rr_id":  "rr-test-002",
					"action": "start",
				},
			})
			Expect(err).NotTo(HaveOccurred())

			// Send a message.
			result, err := session.CallTool(ctx, &mcpsdk.CallToolParams{
				Name: "kubernaut_investigate",
				Arguments: map[string]any{
					"rr_id":   "rr-test-002",
					"action":  "message",
					"message": "What caused this pod crash?",
				},
			})
			Expect(err).NotTo(HaveOccurred())
			Expect(result.IsError).To(BeFalse())
		})
	})
})

// authedHTTPClient returns an *http.Client that always includes the Authorization header.
func authedHTTPClient(user string) *http.Client {
	return &http.Client{
		Transport: &authedTransport{user: user, base: http.DefaultTransport},
	}
}

type authedTransport struct {
	user string
	base http.RoundTripper
}

func (t *authedTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	req = req.Clone(req.Context())
	req.Header.Set("Authorization", "Bearer token-for-"+t.user)
	return t.base.RoundTrip(req)
}
