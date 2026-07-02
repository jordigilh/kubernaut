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
package testutil_test

import (
	"context"
	"encoding/json"
	"testing"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	mockgw "github.com/jordigilh/kubernaut/test/services/mock-mcp-gateway/testutil"
)

func TestMockMCPGateway(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Mock MCP Gateway Suite")
}

var _ = Describe("MockGateway (BR-INTEGRATION-065)", func() {
	var (
		gw  *mockgw.MockGateway
		ctx context.Context
	)

	BeforeEach(func() {
		ctx = context.Background()
	})

	AfterEach(func() {
		if gw != nil {
			gw.Close()
		}
	})

	Describe("NewMockGateway", func() {
		It("UT-MOCK-GW-001: starts a server and returns a valid URL", func() {
			gw = mockgw.NewMockGateway()
			Expect(gw.URL()).ToNot(BeEmpty())
			Expect(gw.URL()).To(HavePrefix("http://"))
		})

		It("UT-MOCK-GW-002: serves MCP initialize handshake", func() {
			gw = mockgw.NewMockGateway()
			session := mustConnect(ctx, gw)
			defer session.Close()
			Expect(session.ID()).ToNot(BeEmpty())
		})
	})

	Describe("WithTools", func() {
		It("UT-MOCK-GW-003: registers static tools discoverable via tools/list", func() {
			gw = mockgw.NewMockGateway(
				mockgw.WithTool("get_pods", "List pods in a namespace", json.RawMessage(`{"type":"object","properties":{"namespace":{"type":"string"}}}`),
					func(_ context.Context, req *mcp.CallToolRequest) (*mcp.CallToolResult, error) {
						return &mcp.CallToolResult{
							Content: []mcp.Content{&mcp.TextContent{Text: "pod-1\npod-2"}},
						}, nil
					},
				),
			)

			session := mustConnect(ctx, gw)
			defer session.Close()

			tools, err := session.ListTools(ctx, nil)
			Expect(err).ToNot(HaveOccurred())
			Expect(tools.Tools).To(HaveLen(1))
			Expect(tools.Tools[0].Name).To(Equal("get_pods"))
		})

		It("UT-MOCK-GW-004: calls a registered tool and returns content", func() {
			gw = mockgw.NewMockGateway(
				mockgw.WithTool("get_pods", "List pods in a namespace", json.RawMessage(`{"type":"object","properties":{"namespace":{"type":"string"}}}`),
					func(_ context.Context, req *mcp.CallToolRequest) (*mcp.CallToolResult, error) {
						var args struct {
							Namespace string `json:"namespace"`
						}
						_ = json.Unmarshal(req.Params.Arguments, &args)
						return &mcp.CallToolResult{
							Content: []mcp.Content{&mcp.TextContent{Text: "pod in " + args.Namespace}},
						}, nil
					},
				),
			)

			session := mustConnect(ctx, gw)
			defer session.Close()

			result, err := session.CallTool(ctx, &mcp.CallToolParams{
				Name:      "get_pods",
				Arguments: map[string]any{"namespace": "kube-system"},
			})
			Expect(err).ToNot(HaveOccurred())
			Expect(result.Content).To(HaveLen(1))
			tc, ok := result.Content[0].(*mcp.TextContent)
			Expect(ok).To(BeTrue())
			Expect(tc.Text).To(Equal("pod in kube-system"))
		})
	})

	Describe("WithMultiCluster", func() {
		It("UT-MOCK-GW-005: registers tools for multiple clusters with cluster prefix routing", func() {
			gw = mockgw.NewMockGateway(
				mockgw.WithMultiCluster("cluster-a", "cluster-b"),
			)

			session := mustConnect(ctx, gw)
			defer session.Close()

			tools, err := session.ListTools(ctx, nil)
			Expect(err).ToNot(HaveOccurred())
			Expect(len(tools.Tools)).To(BeNumerically(">=", 2))

			var toolNames []string
			for _, t := range tools.Tools {
				toolNames = append(toolNames, t.Name)
			}
			Expect(toolNames).To(ContainElement("cluster-a__resources_get"))
			Expect(toolNames).To(ContainElement("cluster-b__resources_get"))
		})

		It("UT-MOCK-GW-006: calls cluster-scoped tool and returns cluster-specific content", func() {
			gw = mockgw.NewMockGateway(
				mockgw.WithMultiCluster("prod-east", "prod-west"),
			)

			session := mustConnect(ctx, gw)
			defer session.Close()

			result, err := session.CallTool(ctx, &mcp.CallToolParams{
				Name:      "prod-east__resources_get",
				Arguments: map[string]any{"kind": "Pod", "namespace": "default", "name": "nginx"},
			})
			Expect(err).ToNot(HaveOccurred())
			Expect(result.Content).To(HaveLen(1))
			tc, ok := result.Content[0].(*mcp.TextContent)
			Expect(ok).To(BeTrue())
			Expect(tc.Text).To(ContainSubstring("nginx"))
			Expect(tc.Text).To(ContainSubstring("Pod"))
		})
	})

	Describe("CallLog", func() {
		It("UT-MOCK-GW-007: records tool invocations for test assertions", func() {
			gw = mockgw.NewMockGateway(
				mockgw.WithTool("check_health", "Check cluster health", json.RawMessage(`{"type":"object"}`),
					func(_ context.Context, req *mcp.CallToolRequest) (*mcp.CallToolResult, error) {
						return &mcp.CallToolResult{
							Content: []mcp.Content{&mcp.TextContent{Text: "healthy"}},
						}, nil
					},
				),
			)

			session := mustConnect(ctx, gw)
			defer session.Close()

			_, err := session.CallTool(ctx, &mcp.CallToolParams{
				Name:      "check_health",
				Arguments: map[string]any{},
			})
			Expect(err).ToNot(HaveOccurred())

			calls := gw.CallLog()
			Expect(calls).To(HaveLen(1))
			Expect(calls[0].ToolName).To(Equal("check_health"))
		})
	})
})

func mustConnect(ctx context.Context, gw *mockgw.MockGateway) *mcp.ClientSession {
	client := mcp.NewClient(&mcp.Implementation{Name: "test-client", Version: "v0.1.0"}, nil)
	transport := &mcp.StreamableClientTransport{Endpoint: gw.URL()}
	session, err := client.Connect(ctx, transport, nil)
	Expect(err).ToNot(HaveOccurred())
	return session
}
