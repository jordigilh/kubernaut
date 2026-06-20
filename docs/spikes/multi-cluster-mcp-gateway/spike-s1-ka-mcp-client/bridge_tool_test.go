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
	"encoding/json"
	"errors"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	mcppkg "github.com/jordigilh/kubernaut/pkg/kubernautagent/tools/mcp"
	sdkmcp "github.com/modelcontextprotocol/go-sdk/mcp"
)

var _ = Describe("BridgeTool — Spike S1 Schema Validation", func() {

	Describe("UT-FLEET-S1-001: BridgeTool implements tools.Tool interface", func() {
		It("should expose Name, Description, and Parameters from the discovered tool", func() {
			tool := mcppkg.Tool{
				ToolName:        "k8s_pods_list",
				ToolDescription: "List pods in a namespace",
				ToolParameters:  json.RawMessage(`{"type":"object","properties":{"namespace":{"type":"string"}}}`),
			}
			bridge := mcppkg.NewBridgeTool(tool, "cluster-a", &mockSessionFactory{})

			Expect(bridge.Name()).To(Equal("k8s_pods_list"))
			Expect(bridge.Description()).To(Equal("List pods in a namespace"))
			Expect(bridge.Parameters()).To(MatchJSON(`{"type":"object","properties":{"namespace":{"type":"string"}}}`))
		})
	})

	Describe("UT-FLEET-S1-002: BridgeTool handles empty InputSchema", func() {
		It("should bridge a tool with no parameters (empty schema)", func() {
			tool := mcppkg.Tool{
				ToolName:        "k8s_cluster_info",
				ToolDescription: "Get cluster information",
				ToolParameters:  json.RawMessage(`{}`),
			}
			bridge := mcppkg.NewBridgeTool(tool, "cluster-a", &mockSessionFactory{})

			Expect(bridge.Parameters()).To(MatchJSON(`{}`))
		})
	})

	Describe("UT-FLEET-S1-003: BridgeTool executes via session-per-call", func() {
		It("should create a session, call the tool, and close the session", func() {
			factory := &mockSessionFactory{
				session: &mockSession{
					callResult: &sdkmcp.CallToolResult{
						Content: []sdkmcp.Content{
							&sdkmcp.TextContent{Text: "pod-1\npod-2\npod-3"},
						},
					},
				},
			}

			tool := mcppkg.Tool{
				ToolName:       "k8s_pods_list",
				ToolParameters: json.RawMessage(`{}`),
			}
			bridge := mcppkg.NewBridgeTool(tool, "cluster-a", factory)

			result, err := bridge.Execute(context.Background(), json.RawMessage(`{"namespace":"default"}`))
			Expect(err).NotTo(HaveOccurred())
			Expect(result).To(Equal("pod-1\npod-2\npod-3"))
			Expect(factory.session.closed).To(BeTrue(), "session should be closed after call")
		})
	})

	Describe("UT-FLEET-S1-004: BridgeTool propagates remote tool errors", func() {
		It("should return an error when the remote tool reports IsError=true", func() {
			factory := &mockSessionFactory{
				session: &mockSession{
					callResult: &sdkmcp.CallToolResult{
						IsError: true,
						Content: []sdkmcp.Content{
							&sdkmcp.TextContent{Text: "permission denied: cannot list pods"},
						},
					},
				},
			}

			tool := mcppkg.Tool{ToolName: "k8s_pods_list", ToolParameters: json.RawMessage(`{}`)}
			bridge := mcppkg.NewBridgeTool(tool, "cluster-a", factory)

			_, err := bridge.Execute(context.Background(), json.RawMessage(`{}`))
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("permission denied"))
			Expect(err.Error()).To(ContainSubstring("cluster-a"))
		})
	})

	Describe("UT-FLEET-S1-005: BridgeTool handles connection failures", func() {
		It("should propagate session creation errors with server context", func() {
			factory := &mockSessionFactory{
				err: errors.New("connection refused"),
			}

			tool := mcppkg.Tool{ToolName: "k8s_pods_list", ToolParameters: json.RawMessage(`{}`)}
			bridge := mcppkg.NewBridgeTool(tool, "cluster-a", factory)

			_, err := bridge.Execute(context.Background(), json.RawMessage(`{}`))
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("connection refused"))
			Expect(err.Error()).To(ContainSubstring("cluster-a"))
		})
	})

	Describe("UT-FLEET-S1-006: BridgeTool handles null/empty args", func() {
		It("should execute with nil arguments when args is null", func() {
			factory := &mockSessionFactory{
				session: &mockSession{
					callResult: &sdkmcp.CallToolResult{
						Content: []sdkmcp.Content{
							&sdkmcp.TextContent{Text: "ok"},
						},
					},
				},
			}

			tool := mcppkg.Tool{ToolName: "k8s_cluster_info", ToolParameters: json.RawMessage(`{}`)}
			bridge := mcppkg.NewBridgeTool(tool, "cluster-a", factory)

			result, err := bridge.Execute(context.Background(), json.RawMessage(`null`))
			Expect(err).NotTo(HaveOccurred())
			Expect(result).To(Equal("ok"))
		})
	})

	Describe("UT-FLEET-S1-007: Schema bridging with complex JSON Schema", func() {
		It("should preserve anyOf/oneOf schemas without transformation", func() {
			complexSchema := json.RawMessage(`{
				"type": "object",
				"properties": {
					"resource": {
						"oneOf": [
							{"type": "string"},
							{"type": "object", "properties": {"kind": {"type": "string"}}}
						]
					}
				},
				"required": ["resource"]
			}`)

			tool := mcppkg.Tool{
				ToolName:       "k8s_resources_get",
				ToolParameters: complexSchema,
			}
			bridge := mcppkg.NewBridgeTool(tool, "cluster-a", &mockSessionFactory{})

			Expect(bridge.Parameters()).To(MatchJSON(complexSchema))
		})
	})

	Describe("UT-FLEET-S1-008: DiscoverAndBridge deduplicates tools by name", func() {
		It("should skip duplicate tool names from multiple providers", func() {
			provider1 := &mockStreamableProvider{
				config: mcppkg.ServerConfig{Name: "cluster-a"},
				tools: []mcppkg.Tool{
					{ToolName: "k8s_pods_list", ToolDescription: "From A", ToolParameters: json.RawMessage(`{}`)},
				},
			}
			provider2 := &mockStreamableProvider{
				config: mcppkg.ServerConfig{Name: "cluster-b"},
				tools: []mcppkg.Tool{
					{ToolName: "k8s_pods_list", ToolDescription: "From B", ToolParameters: json.RawMessage(`{}`)},
					{ToolName: "k8s_nodes_list", ToolDescription: "From B", ToolParameters: json.RawMessage(`{}`)},
				},
			}

			bridged, err := mcppkg.DiscoverAndBridgeFromMock(
				context.Background(),
				[]mcppkg.MockProvider{provider1, provider2},
			)
			Expect(err).NotTo(HaveOccurred())
			Expect(bridged).To(HaveLen(2))
			Expect(bridged[0].Name()).To(Equal("k8s_pods_list"))
			Expect(bridged[0].Description()).To(Equal("From A"))
			Expect(bridged[1].Name()).To(Equal("k8s_nodes_list"))
		})
	})

	Describe("UT-FLEET-S1-009: marshalInputSchema handles edge cases", func() {
		It("should return empty object for nil schema", func() {
			result := mcppkg.MarshalInputSchemaPublic(nil)
			Expect(result).To(MatchJSON(`{}`))
		})

		It("should marshal schema with nested properties", func() {
			schema := map[string]any{
				"type": "object",
				"properties": map[string]any{
					"metadata": map[string]any{
						"type":        "object",
						"description": "Resource metadata",
					},
				},
			}
			result := mcppkg.MarshalInputSchemaPublic(schema)
			Expect(result).NotTo(BeEmpty())

			var parsed map[string]any
			Expect(json.Unmarshal(result, &parsed)).To(Succeed())
			Expect(parsed["type"]).To(Equal("object"))
			Expect(parsed["properties"]).NotTo(BeNil())
		})
	})
})

// --- Test Doubles ---

type mockSessionFactory struct {
	session *mockSession
	err     error
}

func (f *mockSessionFactory) NewSession(_ context.Context) (mcppkg.Session, error) {
	if f.err != nil {
		return nil, f.err
	}
	return f.session, nil
}

type mockSession struct {
	callResult *sdkmcp.CallToolResult
	callErr    error
	listResult *sdkmcp.ListToolsResult
	listErr    error
	closed     bool
}

func (s *mockSession) CallTool(_ context.Context, _ *sdkmcp.CallToolParams) (*sdkmcp.CallToolResult, error) {
	if s.callErr != nil {
		return nil, s.callErr
	}
	return s.callResult, nil
}

func (s *mockSession) ListTools(_ context.Context, _ *sdkmcp.ListToolsParams) (*sdkmcp.ListToolsResult, error) {
	if s.listErr != nil {
		return nil, s.listErr
	}
	return s.listResult, nil
}

func (s *mockSession) Close() error {
	s.closed = true
	return nil
}

type mockStreamableProvider struct {
	config mcppkg.ServerConfig
	tools  []mcppkg.Tool
}

func (p *mockStreamableProvider) GetConfig() mcppkg.ServerConfig { return p.config }
func (p *mockStreamableProvider) DiscoverTools(_ context.Context) ([]mcppkg.Tool, error) {
	return p.tools, nil
}
func (p *mockStreamableProvider) NewSession(_ context.Context) (mcppkg.Session, error) {
	return &mockSession{}, nil
}
