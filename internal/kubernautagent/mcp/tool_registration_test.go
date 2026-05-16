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

	mcpinternal "github.com/jordigilh/kubernaut/internal/kubernautagent/mcp"
	mcptools "github.com/jordigilh/kubernaut/internal/kubernautagent/mcp/tools"
	mcpsdk "github.com/modelcontextprotocol/go-sdk/mcp"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

// stubInvestigate returns a minimal InvestigateTool for registration tests.
func stubInvestigate() *mcptools.InvestigateTool {
	return mcptools.NewInvestigateTool(nil, nil, nil, mcptools.NopAutonomousManager{})
}

// stubSelectWorkflow returns a minimal SelectWorkflowTool for registration tests.
func stubSelectWorkflow() *mcptools.SelectWorkflowTool {
	return mcptools.NewSelectWorkflowTool(nil, nil)
}

// stubCompleteNoAction returns a minimal CompleteNoActionTool for registration tests.
func stubCompleteNoAction() *mcptools.CompleteNoActionTool {
	return mcptools.NewCompleteNoActionTool(nil)
}

var _ = Describe("MCP Tool Registration — PR6a", func() {

	Describe("UT-KA-PR6A-001: BootstrapMCP registers all tools with the MCP SDK (#1012: 3-tool surface)", func() {
		It("should expose 3 tools via the SDK tools/list protocol", func() {
			deps := mcpinternal.MCPDeps{
				AuthMiddleware: func(next http.Handler) http.Handler { return next },
				Tools: mcpinternal.ToolDeps{
					Investigate:      mcptools.InvestigateRegistration(stubInvestigate(), nil, nil),
					SelectWorkflow:   mcptools.SelectWorkflowRegistration(stubSelectWorkflow()),
					CompleteNoAction: mcptools.CompleteNoActionRegistration(stubCompleteNoAction()),
				},
			}

			handler, srv := mcpinternal.BootstrapMCP(deps)
			Expect(handler).NotTo(BeNil())
			Expect(srv.ToolCount()).To(Equal(3))

			ts := httptest.NewServer(handler)
			defer ts.Close()

			ctx := context.Background()
			client := mcpsdk.NewClient(&mcpsdk.Implementation{
				Name:    "test-client",
				Version: "0.0.1",
			}, nil)

			transport := &mcpsdk.StreamableClientTransport{Endpoint: ts.URL}
			session, err := client.Connect(ctx, transport, nil)
			Expect(err).NotTo(HaveOccurred())
			defer func() { _ = session.Close() }()

			var toolNames []string
			for tool, err := range session.Tools(ctx, nil) {
				Expect(err).NotTo(HaveOccurred())
				toolNames = append(toolNames, tool.Name)
			}

			Expect(toolNames).To(ConsistOf(
				"kubernaut_investigate",
				"kubernaut_select_workflow",
				"kubernaut_complete_no_action",
			))
		})
	})

	Describe("UT-KA-PR6A-002: BootstrapMCP with nil tools registers zero", func() {
		It("should register zero tools when no tool deps provided", func() {
			deps := mcpinternal.MCPDeps{
				AuthMiddleware: func(next http.Handler) http.Handler { return next },
			}

			_, srv := mcpinternal.BootstrapMCP(deps)
			Expect(srv.ToolCount()).To(Equal(0))
		})
	})

	Describe("UT-KA-PR6A-003: BootstrapMCP with partial tools registers only provided", func() {
		It("should register only investigate when others are nil", func() {
			deps := mcpinternal.MCPDeps{
				AuthMiddleware: func(next http.Handler) http.Handler { return next },
				Tools: mcpinternal.ToolDeps{
					Investigate: mcptools.InvestigateRegistration(stubInvestigate(), nil, nil),
				},
			}

			_, srv := mcpinternal.BootstrapMCP(deps)
			Expect(srv.ToolCount()).To(Equal(1))
		})
	})

	Describe("UT-KA-PR6A-004: BootstrapMCP panics without auth middleware", func() {
		It("should panic when AuthMiddleware is nil", func() {
			Expect(func() {
				mcpinternal.BootstrapMCP(mcpinternal.MCPDeps{})
			}).To(Panic())
		})
	})
})
