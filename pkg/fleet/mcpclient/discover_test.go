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

	"github.com/modelcontextprotocol/go-sdk/mcp"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/jordigilh/kubernaut/pkg/fleet/mcpclient"
	"github.com/jordigilh/kubernaut/pkg/fleet/registry"
	mockgw "github.com/jordigilh/kubernaut/test/services/mock-mcp-gateway/testutil"
)

var _ = Describe("DiscoverToolPrefix (BR-INTEGRATION-054, ADR-068 #10)", func() {
	var (
		gw  *mockgw.MockGateway
		ctx = context.Background()
	)

	AfterEach(func() {
		if gw != nil {
			gw.Close()
		}
	})

	DescribeTable("discovers prefix for cluster from tools/list",
		func(clusterID string, setupGW func() *mockgw.MockGateway, expectedPrefix string) {
			gw = setupGW()

			mcpClient := mcp.NewClient(
				&mcp.Implementation{Name: "test-discover", Version: "v0.1.0"},
				nil,
			)
			session, err := mcpClient.Connect(ctx, &mcp.StreamableClientTransport{Endpoint: gw.URL()}, nil)
			Expect(err).ToNot(HaveOccurred())
			defer session.Close()

			prefix, err := mcpclient.DiscoverToolPrefix(ctx, session, clusterID)
			Expect(err).ToNot(HaveOccurred())
			Expect(prefix).To(Equal(expectedPrefix))
		},

		Entry("UT-DISC-001 [CM-6]: EAIGW convention (cluster-a → cluster-a__)",
			"cluster-a",
			func() *mockgw.MockGateway { return mockgw.NewMockGateway(mockgw.WithMultiCluster("cluster-a")) },
			"cluster-a__",
		),

		Entry("UT-DISC-002 [CM-6]: EAIGW multi-cluster selects correct prefix",
			"cluster-b",
			func() *mockgw.MockGateway {
				return mockgw.NewMockGateway(mockgw.WithMultiCluster("cluster-a", "cluster-b"))
			},
			"cluster-b__",
		),

		Entry("UT-DISC-003 [CM-6]: Kuadrant convention (cluster-a → cluster_a_)",
			"cluster-a",
			func() *mockgw.MockGateway {
				return mockgw.NewMockGateway(mockgw.WithKuadrantCluster("cluster-a", "cluster_a_"))
			},
			"cluster_a_",
		),

		Entry("UT-DISC-004 [CM-6]: Kuadrant multi-segment prefix (prod-east → prod_east_)",
			"prod-east",
			func() *mockgw.MockGateway {
				return mockgw.NewMockGateway(mockgw.WithKuadrantCluster("prod-east", "prod_east_"))
			},
			"prod_east_",
		),

		Entry("UT-DISC-005 [CM-6]: Kuadrant with hyphenated prefix preserving hyphens",
			"my-cluster",
			func() *mockgw.MockGateway {
				return mockgw.NewMockGateway(mockgw.WithKuadrantCluster("my-cluster", "my_cluster_"))
			},
			"my_cluster_",
		),

		Entry("UT-DISC-006 [CM-6]: EAIGW cluster ID without hyphens",
			"loopback",
			func() *mockgw.MockGateway {
				return mockgw.NewMockGateway(mockgw.WithMultiCluster("loopback"))
			},
			"loopback__",
		),
	)

	It("UT-DISC-007 [CM-6]: returns error when cluster not found in tools/list", func() {
		gw = mockgw.NewMockGateway(mockgw.WithMultiCluster("cluster-a"))

		mcpClient := mcp.NewClient(
			&mcp.Implementation{Name: "test-discover", Version: "v0.1.0"},
			nil,
		)
		session, err := mcpClient.Connect(ctx, &mcp.StreamableClientTransport{Endpoint: gw.URL()}, nil)
		Expect(err).ToNot(HaveOccurred())
		defer session.Close()

		_, err = mcpclient.DiscoverToolPrefix(ctx, session, "nonexistent")
		Expect(err).To(HaveOccurred())
		Expect(err.Error()).To(ContainSubstring("no tools found for cluster"))
	})

	It("UT-DISC-008 [CM-6]: returns error when gateway has no tools", func() {
		gw = mockgw.NewMockGateway()

		mcpClient := mcp.NewClient(
			&mcp.Implementation{Name: "test-discover", Version: "v0.1.0"},
			nil,
		)
		session, err := mcpClient.Connect(ctx, &mcp.StreamableClientTransport{Endpoint: gw.URL()}, nil)
		Expect(err).ToNot(HaveOccurred())
		defer session.Close()

		_, err = mcpclient.DiscoverToolPrefix(ctx, session, "cluster-a")
		Expect(err).To(HaveOccurred())
		Expect(err.Error()).To(ContainSubstring("no tools found for cluster"))
	})
})

// PrefixFromToolNames is DiscoverToolPrefix's pure extraction logic, operating
// on an already-discovered slice of tool names rather than a live MCP
// session. Extracted for issue #1756: gatewayOverlayResolver.Overlay()
// (cmd/kubernautagent) needs the exact same gateway-agnostic matching against
// names it already has from ToolsForCluster(), without an extra ListTools()
// round trip and without duplicating (and silently drifting from) this logic.
//
// Authority: ADR-068 decision #10 (gateway-agnostic business logic),
// DD-FLEET-005, BR-INTEGRATION-054, BR-INTEGRATION-1489, Issue #1756.
var _ = Describe("PrefixFromToolNames (BR-INTEGRATION-054, BR-INTEGRATION-1489, ADR-068 #10) [AC-4]", func() {
	DescribeTable("derives the gateway-specific prefix from an already-discovered tool name list",
		func(clusterID string, names []string, expectedPrefix string) {
			prefix, err := mcpclient.PrefixFromToolNames(clusterID, names)
			Expect(err).ToNot(HaveOccurred())
			Expect(prefix).To(Equal(expectedPrefix))
		},

		Entry("UT-MCP-TN-101 [AC-4]: EAIGW convention (cluster-a -> cluster-a__)",
			"cluster-a",
			[]string{"cluster-a__resources_get", "cluster-a__resources_list"},
			"cluster-a__",
		),
		Entry("UT-MCP-TN-102 [AC-4]: Kuadrant convention (cluster-a -> cluster_a_)",
			"cluster-a",
			[]string{"cluster_a_resources_get", "cluster_a_resources_list"},
			"cluster_a_",
		),
		Entry("UT-MCP-TN-103 [AC-4]: Kuadrant multi-segment prefix (prod-east -> prod_east_)",
			"prod-east",
			[]string{"prod_east_resources_get"},
			"prod_east_",
		),
		Entry("UT-MCP-TN-104 [AC-4]: mixed tool list -- only the target cluster's own tools match (AC-4 cross-cluster isolation)",
			"prod-east",
			[]string{"prod_west_resources_get", "prod_east_resources_list"},
			"prod_east_",
		),
		Entry("UT-MCP-TN-107 [AC-4]: API discovery suffix wins when it appears before resources_list",
			"cluster-a",
			[]string{"cluster-a__api_resources_list", "cluster-a__resources_list"},
			"cluster-a__",
		),
		Entry("UT-MCP-TN-108 [AC-4]: neighboring cluster names do not satisfy the target boundary",
			"cluster-a",
			[]string{"cluster-a2__api_resources_list", "cluster-a__resources_list"},
			"cluster-a__",
		),
	)

	It("UT-MCP-TN-105 [AC-4]: returns an error (never a wrong-cluster guess) when no name matches the cluster ID", func() {
		_, err := mcpclient.PrefixFromToolNames("nonexistent", []string{"cluster-a__resources_get"})
		Expect(err).To(HaveOccurred())
		Expect(err.Error()).To(ContainSubstring("no tools found for cluster"))
	})

	It("UT-MCP-TN-106 [AC-4]: returns an error for an empty name list", func() {
		_, err := mcpclient.PrefixFromToolNames("cluster-a", nil)
		Expect(err).To(HaveOccurred())
	})
})

var _ = Describe("PrefixFromToolNamesForGateway (BR-INTEGRATION-054, ADR-068 #10) [AC-4]", func() {
	DescribeTable("uses the configured gateway's prefix contract",
		func(gatewayType registry.MCPGatewayType, clusterID string, names []string, expectedPrefix string) {
			prefix, err := mcpclient.PrefixFromToolNamesForGateway(gatewayType, clusterID, names)
			Expect(err).ToNot(HaveOccurred())
			Expect(prefix).To(Equal(expectedPrefix))
		},

		Entry("UT-MCP-TN-109 [AC-4]: EAIGW requires the exact cluster boundary before suffix selection",
			registry.GatewayEAIGW,
			"cluster-a",
			[]string{"cluster-a2__api_resources_list", "cluster-a__resources_list"},
			"cluster-a__",
		),
		Entry("UT-MCP-TN-110 [AC-4]: Kuadrant accepts an admin prefix unrelated to the cluster ID",
			registry.GatewayKuadrant,
			"cluster-a",
			[]string{"admin-prod_resources_get"},
			"admin-prod_",
		),
	)

	It("UT-MCP-TN-111 [AC-4]: rejects an unsupported gateway type", func() {
		_, err := mcpclient.PrefixFromToolNamesForGateway("unsupported", "cluster-a", []string{"cluster-a__resources_get"})
		Expect(err).To(MatchError(ContainSubstring("unsupported MCP gateway type")))
	})
})
