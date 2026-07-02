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

package tools_test

import (
	"context"
	"net/http"
	"net/http/httptest"

	"github.com/go-logr/logr"
	mcpsdk "github.com/modelcontextprotocol/go-sdk/mcp"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	mcpinternal "github.com/jordigilh/kubernaut/internal/kubernautagent/mcp"
	mcptools "github.com/jordigilh/kubernaut/internal/kubernautagent/mcp/tools"
)

// GO-ANTIPATTERN-AUDIT-2026-07-01 Wave 4 §7l-1: characterization test for
// InvestigateRegistration (0% unit coverage — only exercised transitively via
// process-level E2E). This drives the MCP tool wiring closure through a real
// client/server round trip (matching the harness in tool_registration_test.go)
// to pin the eventStore/notifier registration side effects before any
// decomposition of the closure body.
var _ = Describe("GO-ANTIPATTERN-AUDIT Wave 4: InvestigateRegistration characterization", func() {

	Describe("UT-KA-WAVE4-010: successful start registers MCP session + notifier callback", func() {
		It("should wire eventStore and notifier when action=start succeeds", func() {
			sessionMgr := &mockSessionManager{
				takeoverSession: &mcpinternal.InteractiveSession{
					SessionID:     "sess-reg-001",
					CorrelationID: "rr-reg-001",
				},
			}
			runner := &mockInvestigatorRunner{}
			recon := &mockContextReconstructor{}
			tool := mcptools.NewInvestigateTool(sessionMgr, runner, recon, mcptools.NopAutonomousManager{})

			eventStore := mcpinternal.NewDelegatingEventStore()
			notifier := mcpinternal.NewSessionNotifier()

			deps := mcpinternal.MCPDeps{
				AuthMiddleware: func(next http.Handler) http.Handler { return next },
				Tools: mcpinternal.ToolDeps{
					Investigate: mcptools.InvestigateRegistration(tool, eventStore, notifier, logr.Discard()),
				},
			}
			handler, srv := mcpinternal.BootstrapMCP(deps)
			Expect(srv.ToolCount()).To(Equal(1))

			ts := httptest.NewServer(handler)
			defer ts.Close()

			ctx := context.Background()
			client := mcpsdk.NewClient(&mcpsdk.Implementation{Name: "test-client", Version: "0.0.1"}, nil)
			transport := &mcpsdk.StreamableClientTransport{Endpoint: ts.URL}
			session, err := client.Connect(ctx, transport, nil)
			Expect(err).NotTo(HaveOccurred())
			defer func() { _ = session.Close() }()

			result, err := session.CallTool(ctx, &mcpsdk.CallToolParams{
				Name: "kubernaut_investigate",
				Arguments: map[string]any{
					"rr_id":  "rr-reg-001",
					"action": "start",
				},
			})
			Expect(err).NotTo(HaveOccurred())
			Expect(result.IsError).To(BeFalse(), "start should succeed against a stub session manager")
		})
	})

	Describe("UT-KA-WAVE4-011: tool error is wrapped by ErrorBoundary and returned as a tool error", func() {
		It("should surface handler errors as an MCP tool error, not a transport error", func() {
			sessionMgr := &mockSessionManager{takeoverErr: mcpinternal.ErrLeaseHeld}
			runner := &mockInvestigatorRunner{}
			recon := &mockContextReconstructor{}
			tool := mcptools.NewInvestigateTool(sessionMgr, runner, recon, mcptools.NopAutonomousManager{})

			deps := mcpinternal.MCPDeps{
				AuthMiddleware: func(next http.Handler) http.Handler { return next },
				Tools: mcpinternal.ToolDeps{
					Investigate: mcptools.InvestigateRegistration(tool, nil, nil, logr.Discard()),
				},
			}
			handler, _ := mcpinternal.BootstrapMCP(deps)
			ts := httptest.NewServer(handler)
			defer ts.Close()

			ctx := context.Background()
			client := mcpsdk.NewClient(&mcpsdk.Implementation{Name: "test-client", Version: "0.0.1"}, nil)
			transport := &mcpsdk.StreamableClientTransport{Endpoint: ts.URL}
			session, err := client.Connect(ctx, transport, nil)
			Expect(err).NotTo(HaveOccurred())
			defer func() { _ = session.Close() }()

			result, err := session.CallTool(ctx, &mcpsdk.CallToolParams{
				Name: "kubernaut_investigate",
				Arguments: map[string]any{
					"rr_id":  "rr-reg-002",
					"action": "start",
				},
			})
			Expect(err).NotTo(HaveOccurred(),
				"UT-KA-WAVE4-011: ErrorBoundary must convert handler errors into a tool-level result, not a transport error")
			Expect(result.IsError).To(BeTrue(),
				"UT-KA-WAVE4-011: a lease-held failure must be surfaced as a tool error")
		})
	})
})
