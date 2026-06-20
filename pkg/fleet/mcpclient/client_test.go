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
	"testing"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/jordigilh/kubernaut/pkg/fleet/mcpclient"
	mockgw "github.com/jordigilh/kubernaut/test/services/mock-mcp-gateway/testutil"
)

func TestMCPClient(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Fleet MCPResourceClient Suite")
}

var _ = Describe("MCPResourceClient (BR-INTEGRATION-065)", func() {
	var (
		ctx context.Context
		gw  *mockgw.MockGateway
	)

	BeforeEach(func() {
		ctx = context.Background()
	})

	AfterEach(func() {
		if gw != nil {
			gw.Close()
		}
	})

	Describe("Connect", func() {
		It("UT-FLEET-MCP-001: connects to MCP Gateway and returns a usable client", func() {
			gw = mockgw.NewMockGateway(mockgw.WithMultiCluster("cluster-a"))

			client, err := mcpclient.New(ctx, gw.URL())
			Expect(err).ToNot(HaveOccurred())
			Expect(client).ToNot(BeNil())
			Expect(client.Close()).To(Succeed())
		})

		It("UT-FLEET-MCP-002: returns error for unreachable endpoint", func() {
			_, err := mcpclient.New(ctx, "http://localhost:1")
			Expect(err).To(HaveOccurred())
		})
	})

	Describe("Get", func() {
		It("UT-FLEET-MCP-003: calls {clusterID}__get_resource and returns content", func() {
			gw = mockgw.NewMockGateway(mockgw.WithMultiCluster("prod-east"))

			client, err := mcpclient.New(ctx, gw.URL())
			Expect(err).ToNot(HaveOccurred())
			defer client.Close()

			result, err := client.Get(ctx, "prod-east", "Pod", "nginx", "default")
			Expect(err).ToNot(HaveOccurred())
			Expect(result).To(ContainSubstring("prod-east"))
			Expect(result).To(ContainSubstring("Pod"))
			Expect(result).To(ContainSubstring("nginx"))

			calls := gw.CallLog()
			Expect(calls).To(HaveLen(1))
			Expect(calls[0].ToolName).To(Equal("prod-east__get_resource"))
		})

		It("UT-FLEET-MCP-004: returns error for unknown cluster tool", func() {
			gw = mockgw.NewMockGateway(mockgw.WithMultiCluster("prod-east"))

			client, err := mcpclient.New(ctx, gw.URL())
			Expect(err).ToNot(HaveOccurred())
			defer client.Close()

			_, err = client.Get(ctx, "unknown-cluster", "Pod", "nginx", "default")
			Expect(err).To(HaveOccurred())
		})
	})

	Describe("List", func() {
		It("UT-FLEET-MCP-005: calls {clusterID}__list_resources and returns content", func() {
			gw = mockgw.NewMockGateway(mockgw.WithMultiCluster("prod-west"))

			client, err := mcpclient.New(ctx, gw.URL())
			Expect(err).ToNot(HaveOccurred())
			defer client.Close()

			result, err := client.List(ctx, "prod-west", "Pod", "kube-system")
			Expect(err).ToNot(HaveOccurred())
			Expect(result).To(ContainSubstring("prod-west"))
			Expect(result).To(ContainSubstring("items"))

			calls := gw.CallLog()
			Expect(calls).To(HaveLen(1))
			Expect(calls[0].ToolName).To(Equal("prod-west__list_resources"))
		})
	})

	Describe("Close", func() {
		It("UT-FLEET-MCP-006: Close is idempotent", func() {
			gw = mockgw.NewMockGateway(mockgw.WithMultiCluster("cluster-x"))

			client, err := mcpclient.New(ctx, gw.URL())
			Expect(err).ToNot(HaveOccurred())

			Expect(client.Close()).To(Succeed())
			Expect(client.Close()).To(Succeed())
		})
	})

	Describe("Options", func() {
		It("UT-FLEET-MCP-007: accepts WithHTTPClient option", func() {
			gw = mockgw.NewMockGateway(mockgw.WithMultiCluster("cluster-opt"))

			client, err := mcpclient.New(ctx, gw.URL(), mcpclient.WithTimeout(5))
			Expect(err).ToNot(HaveOccurred())
			Expect(client).ToNot(BeNil())
			Expect(client.Close()).To(Succeed())
		})
	})
})
