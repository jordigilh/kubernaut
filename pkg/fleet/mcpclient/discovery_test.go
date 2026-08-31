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

package mcpclient_test

import (
	"context"
	"encoding/json"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/jordigilh/kubernaut/pkg/fleet/mcpclient"
	"github.com/jordigilh/kubernaut/pkg/fleet/registry"
	mockgw "github.com/jordigilh/kubernaut/test/services/mock-mcp-gateway/testutil"
)

var _ = Describe("GatewayDiscoverer (BR-FLEET-054, ADR-068 #11)", func() {
	var ctx context.Context

	BeforeEach(func() {
		ctx = context.Background()
	})

	// =========================================================================
	// 6.1 Factory tests (CM-6)
	// =========================================================================
	Describe("Factory — NewDiscoverer (CM-6)", func() {
		It("UT-DISC-001: NewDiscoverer with GatewayKuadrant returns KuadrantDiscoverer", func() {
			gw := mockgw.NewMockGateway(mockgw.WithDiscoverableTools(
				mockgw.DiscoverableClusterOption{Name: "c1", Prefix: "c1_", Categories: []string{"k8s"}},
			))
			defer gw.Close()

			c, err := mcpclient.New(ctx, gw.URL())
			Expect(err).ToNot(HaveOccurred())
			defer c.Close()

			disc, err := mcpclient.NewDiscoverer(registry.GatewayKuadrant, c.Session())
			Expect(err).ToNot(HaveOccurred())
			Expect(disc).ToNot(BeNil())
			_, ok := disc.(*mcpclient.KuadrantDiscoverer)
			Expect(ok).To(BeTrue(), "must return *KuadrantDiscoverer")
		})

		It("UT-DISC-002: NewDiscoverer with GatewayEAIGW returns EAIGWDiscoverer", func() {
			gw := mockgw.NewMockGateway(mockgw.WithMultiCluster("c1"))
			defer gw.Close()

			c, err := mcpclient.New(ctx, gw.URL())
			Expect(err).ToNot(HaveOccurred())
			defer c.Close()

			disc, err := mcpclient.NewDiscoverer(registry.GatewayEAIGW, c.Session())
			Expect(err).ToNot(HaveOccurred())
			Expect(disc).ToNot(BeNil())
			_, ok := disc.(*mcpclient.EAIGWDiscoverer)
			Expect(ok).To(BeTrue(), "must return *EAIGWDiscoverer")
		})

		It("UT-DISC-003: NewDiscoverer with empty/invalid type returns error", func() {
			gw := mockgw.NewMockGateway(mockgw.WithMultiCluster("c1"))
			defer gw.Close()
			c, err := mcpclient.New(ctx, gw.URL())
			Expect(err).ToNot(HaveOccurred())
			defer c.Close()

			_, err = mcpclient.NewDiscoverer("", c.Session())
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("unsupported gateway type"))

			_, err = mcpclient.NewDiscoverer("nonexistent", c.Session())
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("unsupported gateway type"))

			_, err = mcpclient.NewDiscoverer(registry.GatewayEAIGW, nil)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("session must not be nil"))
		})
	})

	// =========================================================================
	// 6.2 KuadrantDiscoverer tests (AC-3)
	// =========================================================================
	Describe("KuadrantDiscoverer (AC-3)", func() {
		var (
			gw   *mockgw.MockGateway
			disc mcpclient.GatewayDiscoverer
			c    *mcpclient.Client
		)

		BeforeEach(func() {
			gw = mockgw.NewMockGateway(mockgw.WithDiscoverableTools(
				mockgw.DiscoverableClusterOption{
					Name:       "prod-east",
					Prefix:     "prod_east_",
					Categories: []string{"k8s", "monitoring"},
					Hint:       "Production cluster in us-east-1",
				},
				mockgw.DiscoverableClusterOption{
					Name:       "staging-west",
					Prefix:     "staging_west_",
					Categories: []string{"k8s"},
					Hint:       "Staging cluster in us-west-2",
				},
			))

			var err error
			c, err = mcpclient.New(ctx, gw.URL())
			Expect(err).ToNot(HaveOccurred())

			disc, err = mcpclient.NewDiscoverer(registry.GatewayKuadrant, c.Session())
			Expect(err).ToNot(HaveOccurred())
		})

		AfterEach(func() {
			if c != nil {
				c.Close()
			}
			if gw != nil {
				gw.Close()
			}
		})

		It("UT-DISC-KUA-001: ListClusters returns cluster metadata without tool names", func() {
			clusters, err := disc.ListClusters(ctx, "")
			Expect(err).ToNot(HaveOccurred())
			Expect(clusters).To(HaveLen(2))

			names := make([]string, len(clusters))
			for i, ci := range clusters {
				names[i] = ci.Name
			}
			Expect(names).To(ContainElement("prod-east"))
			Expect(names).To(ContainElement("staging-west"))

			for _, ci := range clusters {
				Expect(ci.Categories).ToNot(BeEmpty())
				Expect(ci.Hint).ToNot(BeEmpty())
			}

			calls := gw.CallLog()
			var discoverCalls int
			for _, call := range calls {
				if call.ToolName == mcpclient.MetaToolDiscoverTools {
					discoverCalls++
				}
			}
			Expect(discoverCalls).To(Equal(1), "must call discover_tools exactly once")
		})

		It("UT-DISC-KUA-002: ListClusters with category filter returns filtered results", func() {
			clusters, err := disc.ListClusters(ctx, "monitoring")
			Expect(err).ToNot(HaveOccurred())
			Expect(clusters).To(HaveLen(1))
			Expect(clusters[0].Name).To(Equal("prod-east"))
		})

		It("UT-DISC-KUA-003: ToolsForCluster calls select_tools then ListTools, returns scoped tool schemas", func() {
			tools, err := disc.ToolsForCluster(ctx, "prod-east")
			Expect(err).ToNot(HaveOccurred())
			Expect(tools).ToNot(BeEmpty())

			for _, t := range tools {
				Expect(t.Name).To(HavePrefix("prod_east_"),
					"returned tools must match cluster prefix")
				Expect(t.InputSchema).ToNot(BeEmpty(),
					"full schemas must be present")
			}

			calls := gw.CallLog()
			var selectCalls int
			for _, call := range calls {
				if call.ToolName == mcpclient.MetaToolSelectTools {
					selectCalls++
				}
			}
			Expect(selectCalls).To(Equal(1), "must call select_tools exactly once")
		})

		It("UT-DISC-KUA-004: ToolsForCluster for unknown cluster returns error", func() {
			_, err := disc.ToolsForCluster(ctx, "nonexistent-cluster")
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("nonexistent-cluster"))
		})

		It("UT-DISC-KUA-005: ListClusters with non-matching category returns empty slice", func() {
			clusters, err := disc.ListClusters(ctx, "nonexistent-category")
			Expect(err).ToNot(HaveOccurred())
			Expect(clusters).To(BeEmpty())
		})
	})

	// =========================================================================
	// 6.3 EAIGWDiscoverer tests (AC-3)
	// =========================================================================
	Describe("EAIGWDiscoverer (AC-3)", func() {
		var (
			gw   *mockgw.MockGateway
			disc mcpclient.GatewayDiscoverer
			c    *mcpclient.Client
		)

		BeforeEach(func() {
			gw = mockgw.NewMockGateway(mockgw.WithMultiCluster("prod-east", "prod-west", "staging"))

			var err error
			c, err = mcpclient.New(ctx, gw.URL())
			Expect(err).ToNot(HaveOccurred())

			disc, err = mcpclient.NewDiscoverer(registry.GatewayEAIGW, c.Session())
			Expect(err).ToNot(HaveOccurred())
		})

		AfterEach(func() {
			if c != nil {
				c.Close()
			}
			if gw != nil {
				gw.Close()
			}
		})

		It("UT-DISC-EAIGW-001: ListClusters extracts unique cluster IDs from __ prefixed tools", func() {
			clusters, err := disc.ListClusters(ctx, "")
			Expect(err).ToNot(HaveOccurred())
			Expect(clusters).ToNot(BeEmpty())

			for _, ci := range clusters {
				Expect(ci.Prefix).To(HaveSuffix("__"),
					"EAIGW prefix must use '__' separator")
			}
		})

		It("UT-DISC-EAIGW-002: ListClusters with multiple clusters returns all", func() {
			clusters, err := disc.ListClusters(ctx, "")
			Expect(err).ToNot(HaveOccurred())
			Expect(clusters).To(HaveLen(3))

			names := make([]string, len(clusters))
			for i, ci := range clusters {
				names[i] = ci.Name
			}
			Expect(names).To(ContainElement("prod-east"))
			Expect(names).To(ContainElement("prod-west"))
			Expect(names).To(ContainElement("staging"))
		})

		It("UT-DISC-EAIGW-003: ToolsForCluster filters tools by cluster prefix", func() {
			tools, err := disc.ToolsForCluster(ctx, "prod-east")
			Expect(err).ToNot(HaveOccurred())
			Expect(tools).ToNot(BeEmpty())

			for _, t := range tools {
				Expect(t.Name).To(HavePrefix("prod-east__"),
					"only tools with 'prod-east__' prefix must be returned")
				Expect(t.InputSchema).ToNot(BeEmpty(),
					"full schemas must be present")
			}
		})

		It("UT-DISC-EAIGW-004: ToolsForCluster for unknown cluster returns error", func() {
			_, err := disc.ToolsForCluster(ctx, "nonexistent-cluster")
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("nonexistent-cluster"))
		})

		It("UT-DISC-EAIGW-005: ListClusters ignores non-prefixed tools (meta-tools)", func() {
			standaloneSchema := json.RawMessage(`{"type":"object","properties":{"input":{"type":"string"}}}`)
			gwWithMeta := mockgw.NewMockGateway(
				mockgw.WithMultiCluster("cluster-a"),
				mockgw.WithTool("standalone_tool", "a tool without prefix", standaloneSchema, func(_ context.Context, _ *mcp.CallToolRequest) (*mcp.CallToolResult, error) {
					return &mcp.CallToolResult{}, nil
				}),
			)
			defer gwWithMeta.Close()

			cMeta, err := mcpclient.New(ctx, gwWithMeta.URL())
			Expect(err).ToNot(HaveOccurred())
			defer cMeta.Close()

			discMeta, err := mcpclient.NewDiscoverer(registry.GatewayEAIGW, cMeta.Session())
			Expect(err).ToNot(HaveOccurred())

			clusters, err := discMeta.ListClusters(ctx, "")
			Expect(err).ToNot(HaveOccurred())

			for _, ci := range clusters {
				Expect(ci.Name).ToNot(Equal("standalone_tool"),
					"tools without '__' separator must not be treated as clusters")
			}
			Expect(clusters).To(HaveLen(1))
			Expect(clusters[0].Name).To(Equal("cluster-a"))
		})
	})

	// =========================================================================
	// 6.4 ResolveSession (issue #2315 self-healing fix)
	// =========================================================================
	Describe("ResolveSession (issue #2315)", func() {
		It("UT-DISC-RS-001: prefers a live provider session over a static one", func() {
			gw := mockgw.NewMockGateway(mockgw.WithMultiCluster("c1"))
			defer gw.Close()
			c, err := mcpclient.New(ctx, gw.URL())
			Expect(err).ToNot(HaveOccurred())
			defer c.Close()

			session, err := mcpclient.ResolveSession(nil, c.Session)
			Expect(err).ToNot(HaveOccurred())
			Expect(session).To(Equal(c.Session()))
		})

		It("UT-DISC-RS-002: falls back to static when no provider is set", func() {
			gw := mockgw.NewMockGateway(mockgw.WithMultiCluster("c1"))
			defer gw.Close()
			c, err := mcpclient.New(ctx, gw.URL())
			Expect(err).ToNot(HaveOccurred())
			defer c.Close()

			session, err := mcpclient.ResolveSession(c.Session(), nil)
			Expect(err).ToNot(HaveOccurred())
			Expect(session).To(Equal(c.Session()))
		})

		It("UT-DISC-RS-003: returns a clear error when the provider currently reports disconnected", func() {
			_, err := mcpclient.ResolveSession(nil, func() *mcp.ClientSession { return nil })
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("MCP session not available"))
		})

		It("UT-DISC-RS-004: returns a clear error when both static and provider are nil", func() {
			_, err := mcpclient.ResolveSession(nil, nil)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("MCP session not available"))
		})
	})

	// =========================================================================
	// 6.5 NewDiscovererWithProvider (issue #2315 self-healing fix)
	// =========================================================================
	Describe("NewDiscovererWithProvider (issue #2315)", func() {
		It("UT-DISC-PROV-001: resolves the live session on each call and discovers tools normally", func() {
			gw := mockgw.NewMockGateway(mockgw.WithMultiCluster("prod-east"))
			defer gw.Close()
			c, err := mcpclient.New(ctx, gw.URL())
			Expect(err).ToNot(HaveOccurred())
			defer c.Close()

			disc := mcpclient.NewDiscovererWithProvider(registry.GatewayEAIGW, c.Session)

			clusters, err := disc.ListClusters(ctx, "")
			Expect(err).ToNot(HaveOccurred())
			Expect(clusters).To(HaveLen(1))
			Expect(clusters[0].Name).To(Equal("prod-east"))

			tools, err := disc.ToolsForCluster(ctx, "prod-east")
			Expect(err).ToNot(HaveOccurred())
			Expect(tools).ToNot(BeEmpty())
		})

		It("UT-DISC-PROV-002: ListClusters returns a clear transient error when the provider reports disconnected", func() {
			disc := mcpclient.NewDiscovererWithProvider(registry.GatewayEAIGW, func() *mcp.ClientSession { return nil })

			_, err := disc.ListClusters(ctx, "")
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("MCP session not available"))
		})

		It("UT-DISC-PROV-003: ToolsForCluster returns a clear transient error when the provider reports disconnected", func() {
			disc := mcpclient.NewDiscovererWithProvider(registry.GatewayKuadrant, func() *mcp.ClientSession { return nil })

			_, err := disc.ToolsForCluster(ctx, "prod-east")
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("MCP session not available"))
		})

		It("UT-DISC-PROV-004: recovers automatically once the provider starts returning a live session (self-healing)", func() {
			gw := mockgw.NewMockGateway(mockgw.WithMultiCluster("prod-east"))
			defer gw.Close()
			c, err := mcpclient.New(ctx, gw.URL())
			Expect(err).ToNot(HaveOccurred())
			defer c.Close()

			var connected bool
			provider := func() *mcp.ClientSession {
				if !connected {
					return nil
				}
				return c.Session()
			}
			disc := mcpclient.NewDiscovererWithProvider(registry.GatewayEAIGW, provider)

			_, err = disc.ListClusters(ctx, "")
			Expect(err).To(HaveOccurred(), "must fail while disconnected")

			connected = true
			clusters, err := disc.ListClusters(ctx, "")
			Expect(err).ToNot(HaveOccurred(), "must succeed once the provider resolves a live session, with no reconstruction needed")
			Expect(clusters).To(HaveLen(1))
		})
	})
})
