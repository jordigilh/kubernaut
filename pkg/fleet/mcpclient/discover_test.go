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
