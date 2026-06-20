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
	"testing"
	"time"

	"github.com/go-logr/logr"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/jordigilh/kubernaut/pkg/fleet/mcpclient"
	mockgw "github.com/jordigilh/kubernaut/test/services/mock-mcp-gateway/testutil"
)

func TestMCPClient(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Fleet MCPResourceClient Suite")
}

var _ = Describe("ResourceClient (BR-FLEET-002, Phase 0)", func() {
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
		It("UT-FLEET-P0-001: returns typed unstructured.Unstructured from get_resource", func() {
			gw = mockgw.NewMockGateway(mockgw.WithMultiCluster("prod-east"))

			client, err := mcpclient.New(ctx, gw.URL())
			Expect(err).ToNot(HaveOccurred())
			defer client.Close()

			obj, err := client.Get(ctx, "prod-east", "Pod", "default", "nginx")
			Expect(err).ToNot(HaveOccurred())
			Expect(obj.GetKind()).To(Equal("Pod"))
			Expect(obj.GetName()).To(Equal("nginx"))
			Expect(obj.GetNamespace()).To(Equal("default"))
			Expect(obj.GetLabels()).To(HaveKeyWithValue("kubernaut.ai/managed", "true"))

			calls := gw.CallLog()
			Expect(calls).To(HaveLen(1))
			Expect(calls[0].ToolName).To(Equal("prod-east__get_resource"))
		})

		It("UT-FLEET-MCP-004: returns error for unknown cluster tool", func() {
			gw = mockgw.NewMockGateway(mockgw.WithMultiCluster("prod-east"))

			client, err := mcpclient.New(ctx, gw.URL())
			Expect(err).ToNot(HaveOccurred())
			defer client.Close()

			_, err = client.Get(ctx, "unknown-cluster", "Pod", "default", "nginx")
			Expect(err).To(HaveOccurred())
		})
	})

	Describe("List", func() {
		It("UT-FLEET-P0-002: returns typed unstructured list from list_resources", func() {
			gw = mockgw.NewMockGateway(mockgw.WithMultiCluster("prod-west"))

			client, err := mcpclient.New(ctx, gw.URL())
			Expect(err).ToNot(HaveOccurred())
			defer client.Close()

			items, err := client.List(ctx, "prod-west", "Pod", "kube-system", nil)
			Expect(err).ToNot(HaveOccurred())
			Expect(items).To(HaveLen(2))
			Expect(items[0].GetKind()).To(Equal("Pod"))
			Expect(items[0].GetName()).To(Equal("item-1"))

			calls := gw.CallLog()
			Expect(calls).To(HaveLen(1))
			Expect(calls[0].ToolName).To(Equal("prod-west__list_resources"))
		})

		It("UT-FLEET-P0-003: passes labelSelector to list_resources", func() {
			gw = mockgw.NewMockGateway(mockgw.WithMultiCluster("prod-west"))

			client, err := mcpclient.New(ctx, gw.URL())
			Expect(err).ToNot(HaveOccurred())
			defer client.Close()

			_, err = client.List(ctx, "prod-west", "Pod", "kube-system", map[string]string{"app": "nginx"})
			Expect(err).ToNot(HaveOccurred())

			calls := gw.CallLog()
			Expect(calls).To(HaveLen(1))
			var args map[string]any
			Expect(json.Unmarshal(calls[0].Arguments, &args)).To(Succeed())
			Expect(args["labelSelector"]).To(Equal("app=nginx"))
		})
	})

	Describe("GetLabels", func() {
		It("UT-FLEET-P0-004: returns metadata.labels from remote resource", func() {
			gw = mockgw.NewMockGateway(mockgw.WithMultiCluster("prod-east"))

			client, err := mcpclient.New(ctx, gw.URL())
			Expect(err).ToNot(HaveOccurred())
			defer client.Close()

			labels, err := client.GetLabels(ctx, "prod-east", "Pod", "default", "nginx")
			Expect(err).ToNot(HaveOccurred())
			Expect(labels).To(HaveKeyWithValue("kubernaut.ai/managed", "true"))
			Expect(labels).To(HaveKey("app"))
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

	Describe("ResilientClient interface compliance", func() {
		It("UT-FLEET-P0-005: ResilientClient satisfies ResourceClient", func() {
			gw = mockgw.NewMockGateway(mockgw.WithMultiCluster("cluster-res"))

			cfg := mcpclient.DefaultResilienceConfig()
			cfg.MaxElapsedTime = 5 * time.Second
			rc, err := mcpclient.NewResilient(ctx, gw.URL(), cfg, logr.Discard())
			Expect(err).ToNot(HaveOccurred())
			defer rc.Close()

			var client mcpclient.ResourceClient = rc
			obj, err := client.Get(ctx, "cluster-res", "Pod", "default", "nginx")
			Expect(err).ToNot(HaveOccurred())
			Expect(obj.GetName()).To(Equal("nginx"))
		})
	})
})
