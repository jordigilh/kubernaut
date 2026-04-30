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
	"net/http"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	mcpinternal "github.com/jordigilh/kubernaut/internal/kubernautagent/mcp"
	mcptools "github.com/jordigilh/kubernaut/internal/kubernautagent/mcp/tools"
)

var _ = Describe("MCP Tool Registration — PR5 Slice E", func() {

	Describe("UT-KA-PR5-E01: BootstrapMCP registers all three MCP tools", func() {
		It("should register investigate, enrich, and select_workflow tools", func() {
			deps := mcpinternal.MCPDeps{
				AuthMiddleware: func(next http.Handler) http.Handler { return next },
				Tools: mcpinternal.ToolDeps{
					Investigate:    &mcptools.InvestigateTool{},
					Enrich:         &mcptools.EnrichTool{},
					SelectWorkflow: &mcptools.SelectWorkflowTool{},
				},
			}

			handler, srv := mcpinternal.BootstrapMCP(deps)
			Expect(handler).NotTo(BeNil())
			Expect(srv.ToolCount()).To(Equal(3),
				"all three tools (investigate, enrich, select_workflow) must be registered")
		})
	})

	Describe("UT-KA-PR5-E02: BootstrapMCP with nil tools registers zero tools", func() {
		It("should register zero tools when no tool deps provided", func() {
			deps := mcpinternal.MCPDeps{
				AuthMiddleware: func(next http.Handler) http.Handler { return next },
			}

			_, srv := mcpinternal.BootstrapMCP(deps)
			Expect(srv.ToolCount()).To(Equal(0),
				"no tools should be registered when ToolDeps is zero-value")
		})
	})

	Describe("UT-KA-PR5-E03: BootstrapMCP with partial tools registers only provided tools", func() {
		It("should register only the investigate tool when others are nil", func() {
			deps := mcpinternal.MCPDeps{
				AuthMiddleware: func(next http.Handler) http.Handler { return next },
				Tools: mcpinternal.ToolDeps{
					Investigate: &mcptools.InvestigateTool{},
				},
			}

			_, srv := mcpinternal.BootstrapMCP(deps)
			Expect(srv.ToolCount()).To(Equal(1))
		})
	})
})
