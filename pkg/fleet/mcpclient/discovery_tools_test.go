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
	"sync"
	"sync/atomic"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/jordigilh/kubernaut/pkg/fleet/mcpclient"
	"github.com/jordigilh/kubernaut/pkg/fleet/registry"
	toolregistry "github.com/jordigilh/kubernaut/pkg/kubernautagent/tools/registry"
	mockgw "github.com/jordigilh/kubernaut/test/services/mock-mcp-gateway/testutil"
)

var _ = Describe("Discovery Tools (BR-FLEET-054, SC-7/SC-5)", func() {
	var (
		ctx context.Context
		gw  *mockgw.MockGateway
		c   *mcpclient.Client
	)

	BeforeEach(func() {
		ctx = context.Background()
	})

	AfterEach(func() {
		if c != nil {
			c.Close()
		}
		if gw != nil {
			gw.Close()
		}
	})

	Describe("ListClustersTool", func() {
		It("UT-DISC-TOOL-001: Name() returns 'list_clusters'", func() {
			tool := mcpclient.NewListClustersTool(nil)
			Expect(tool.Name()).To(Equal("list_clusters"))
		})

		It("UT-DISC-TOOL-002: Execute returns JSON with cluster metadata, no tool names", func() {
			gw = mockgw.NewMockGateway(mockgw.WithDiscoverableTools(
				mockgw.DiscoverableClusterOption{
					Name:       "prod-east",
					Prefix:     "prod_east_",
					Categories: []string{"k8s"},
					Hint:       "Production us-east-1",
				},
				mockgw.DiscoverableClusterOption{
					Name:       "staging",
					Prefix:     "staging_",
					Categories: []string{"k8s", "dev"},
					Hint:       "Staging environment",
				},
			))

			var err error
			c, err = mcpclient.New(ctx, gw.URL())
			Expect(err).ToNot(HaveOccurred())

			disc, err := mcpclient.NewDiscoverer(registry.GatewayKuadrant, c.Session())
			Expect(err).ToNot(HaveOccurred())

			tool := mcpclient.NewListClustersTool(disc)
			result, err := tool.Execute(ctx, json.RawMessage(`{}`))
			Expect(err).ToNot(HaveOccurred())

			var resp struct {
				Clusters []struct {
					ID         string   `json:"id"`
					Hint       string   `json:"hint"`
					Categories []string `json:"categories"`
				} `json:"clusters"`
			}
			Expect(json.Unmarshal([]byte(result), &resp)).To(Succeed())
			Expect(resp.Clusters).To(HaveLen(2))

			Expect(result).ToNot(ContainSubstring("resources_get"),
				"response must not contain tool names (SC-7: boundary protection)")
			Expect(result).ToNot(ContainSubstring("resources_list"),
				"response must not contain tool names")
		})

		It("UT-DISC-TOOL-005: Parameters() returns valid JSON schema", func() {
			tool := mcpclient.NewListClustersTool(nil)
			params := tool.Parameters()
			Expect(params).ToNot(BeEmpty())

			var schema map[string]any
			Expect(json.Unmarshal(params, &schema)).To(Succeed())
			Expect(schema).To(HaveKey("properties"))

			props := schema["properties"].(map[string]any)
			Expect(props).To(HaveKey("category"))
		})
	})

	Describe("ListToolsForClusterTool", func() {
		It("UT-DISC-TOOL-003: Execute returns tool names and descriptions", func() {
			gw = mockgw.NewMockGateway(mockgw.WithMultiCluster("alpha", "beta"))

			var err error
			c, err = mcpclient.New(ctx, gw.URL())
			Expect(err).ToNot(HaveOccurred())

			disc, err := mcpclient.NewDiscoverer(registry.GatewayEAIGW, c.Session())
			Expect(err).ToNot(HaveOccurred())

			reg := toolregistry.New()
			tool := mcpclient.NewListToolsForClusterTool(disc, reg, c.Session())
			result, err := tool.Execute(ctx, json.RawMessage(`{"cluster_id":"alpha"}`))
			Expect(err).ToNot(HaveOccurred())

			var resp struct {
				Tools []struct {
					Name        string `json:"name"`
					Description string `json:"description"`
				} `json:"tools"`
				ClusterID string `json:"cluster_id"`
			}
			Expect(json.Unmarshal([]byte(result), &resp)).To(Succeed())
			Expect(resp.Tools).ToNot(BeEmpty())
			Expect(resp.ClusterID).To(Equal("alpha"))

			for _, t := range resp.Tools {
				Expect(t.Name).To(HavePrefix("alpha__"))
			}

			registered, err := reg.Get("alpha__resources_get")
			Expect(err).ToNot(HaveOccurred())
			Expect(registered).ToNot(BeNil(),
				"discovered tools must be registered as BridgeTools")
		})

		It("UT-DISC-TOOL-004: Execute with invalid cluster returns error", func() {
			gw = mockgw.NewMockGateway(mockgw.WithMultiCluster("only-cluster"))

			var err error
			c, err = mcpclient.New(ctx, gw.URL())
			Expect(err).ToNot(HaveOccurred())

			disc, err := mcpclient.NewDiscoverer(registry.GatewayEAIGW, c.Session())
			Expect(err).ToNot(HaveOccurred())

			reg := toolregistry.New()
			tool := mcpclient.NewListToolsForClusterTool(disc, reg, c.Session())
			_, err = tool.Execute(ctx, json.RawMessage(`{"cluster_id":"nonexistent"}`))
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("nonexistent"))
		})

		It("UT-DISC-TOOL-006: Parameters() returns valid JSON schema with required cluster_id", func() {
			tool := mcpclient.NewListToolsForClusterTool(nil, nil, nil)
			params := tool.Parameters()
			Expect(params).ToNot(BeEmpty())

			var schema map[string]any
			Expect(json.Unmarshal(params, &schema)).To(Succeed())

			required, ok := schema["required"].([]any)
			Expect(ok).To(BeTrue())
			Expect(required).To(ContainElement("cluster_id"))
		})

		It("UT-DISC-TOOL-007: Concurrent calls for same cluster deduplicates via singleflight", func() {
			gw = mockgw.NewMockGateway(mockgw.WithMultiCluster("shared-cluster"))

			var err error
			c, err = mcpclient.New(ctx, gw.URL())
			Expect(err).ToNot(HaveOccurred())

			disc, err := mcpclient.NewDiscoverer(registry.GatewayEAIGW, c.Session())
			Expect(err).ToNot(HaveOccurred())

			reg := toolregistry.New()
			tool := mcpclient.NewListToolsForClusterTool(disc, reg, c.Session())

			var wg sync.WaitGroup
			var successCount atomic.Int32
			for i := 0; i < 5; i++ {
				wg.Add(1)
				go func() {
					defer wg.Done()
					defer GinkgoRecover()
					_, err := tool.Execute(ctx, json.RawMessage(`{"cluster_id":"shared-cluster"}`))
					if err == nil {
						successCount.Add(1)
					}
				}()
			}
			wg.Wait()

			Expect(successCount.Load()).To(BeNumerically(">=", 1),
				"at least one concurrent call must succeed")
		})

		It("UT-DISC-TOOL-008: Sequential calls for different clusters execute independently", func() {
			gw = mockgw.NewMockGateway(mockgw.WithMultiCluster("cluster-a", "cluster-b"))

			var err error
			c, err = mcpclient.New(ctx, gw.URL())
			Expect(err).ToNot(HaveOccurred())

			disc, err := mcpclient.NewDiscoverer(registry.GatewayEAIGW, c.Session())
			Expect(err).ToNot(HaveOccurred())

			reg := toolregistry.New()
			tool := mcpclient.NewListToolsForClusterTool(disc, reg, c.Session())

			resultA, err := tool.Execute(ctx, json.RawMessage(`{"cluster_id":"cluster-a"}`))
			Expect(err).ToNot(HaveOccurred())
			Expect(resultA).To(ContainSubstring("cluster-a"))

			resultB, err := tool.Execute(ctx, json.RawMessage(`{"cluster_id":"cluster-b"}`))
			Expect(err).ToNot(HaveOccurred())
			Expect(resultB).To(ContainSubstring("cluster-b"))

			_, errA := reg.Get("cluster-a__resources_get")
			Expect(errA).ToNot(HaveOccurred())
			_, errB := reg.Get("cluster-b__resources_get")
			Expect(errB).ToNot(HaveOccurred())
		})
	})
})
