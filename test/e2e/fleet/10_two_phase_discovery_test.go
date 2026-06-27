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

package fleet

import (
	"context"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/jordigilh/kubernaut/pkg/fleet/mcpclient"
	"github.com/jordigilh/kubernaut/pkg/fleet/registry"
)

// E2E-FLEET-DISC: Two-phase discovery journey through Kuadrant MCP Gateway.
//
// This suite proves the Pyramid Invariant's E2E layer for the GatewayDiscoverer:
//   Alert → KA calls ListClusters (discover_tools) → LLM picks cluster →
//   KA calls ToolsForCluster (select_tools + ListTools) → tool call succeeds
//
// Authority: Issue #54, ADR-068 decision #11, BR-FLEET-054
// FedRAMP: CM-6 (Configuration Settings), AC-3 (Access Enforcement)
var _ = Describe("E2E-FLEET-DISC: Two-Phase Discovery Journey", Label("fleet"), func() {

	It("E2E-FLEET-DISC-001 [CM-6]: ListClusters discovers loopback-cluster via discover_tools meta-tool", func() {
		mcpCtx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
		defer cancel()

		By("Connecting to Kuadrant MCP gateway")
		c, err := mcpclient.New(mcpCtx, mcpGatewayURL)
		Expect(err).ToNot(HaveOccurred(), "should connect to MCP gateway")
		defer c.Close()

		By("Creating KuadrantDiscoverer via factory")
		discoverer, err := mcpclient.NewDiscoverer(registry.GatewayKuadrant, c.Session())
		Expect(err).ToNot(HaveOccurred(), "factory must create KuadrantDiscoverer")

		By("Calling ListClusters (wraps discover_tools)")
		var clusters []mcpclient.ClusterInfo
		Eventually(func(g Gomega) {
			clusters, err = discoverer.ListClusters(mcpCtx, "")
			g.Expect(err).ToNot(HaveOccurred(), "ListClusters must succeed")
			names := make([]string, 0, len(clusters))
			for _, cl := range clusters {
				names = append(names, cl.Name)
			}
			g.Expect(names).To(ContainElement("loopback-cluster"),
				"CM-6: loopback-cluster must be discoverable via discover_tools")
		}).WithTimeout(60 * time.Second).WithPolling(5 * time.Second).Should(Succeed())

	})

	It("E2E-FLEET-DISC-002 [AC-3]: ToolsForCluster returns scoped tools for loopback-cluster via select_tools", func() {
		mcpCtx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
		defer cancel()

		By("Connecting to Kuadrant MCP gateway")
		c, err := mcpclient.New(mcpCtx, mcpGatewayURL)
		Expect(err).ToNot(HaveOccurred())
		defer c.Close()

		By("Creating KuadrantDiscoverer")
		discoverer, err := mcpclient.NewDiscoverer(registry.GatewayKuadrant, c.Session())
		Expect(err).ToNot(HaveOccurred())

		By("Calling ToolsForCluster (wraps select_tools + ListTools)")
		var tools []mcpclient.ToolDefinition
		Eventually(func(g Gomega) {
			tools, err = discoverer.ToolsForCluster(mcpCtx, "loopback-cluster")
			g.Expect(err).ToNot(HaveOccurred(), "ToolsForCluster must succeed")
			names := make([]string, 0, len(tools))
			for _, t := range tools {
				names = append(names, t.Name)
			}
			g.Expect(names).To(ContainElement("loopback_cluster_namespaces_list"),
				"AC-3: scoped tools must include namespaces_list for loopback-cluster")
		}).WithTimeout(60 * time.Second).WithPolling(5 * time.Second).Should(Succeed())

		By("Verifying tool names use the loopback_cluster_ prefix")
		for _, tool := range tools {
			Expect(tool.Name).To(HavePrefix("loopback_cluster_"),
				"AC-3: all scoped tools must carry the cluster prefix")
		}

		By("Verifying namespaces_list tool is present (kube-mcp-server standard tool)")
		toolNames := make([]string, 0, len(tools))
		for _, tool := range tools {
			toolNames = append(toolNames, tool.Name)
		}
		Expect(toolNames).To(ContainElement("loopback_cluster_namespaces_list"),
			"kube-mcp-server must expose namespaces_list through the gateway")
	})

	It("E2E-FLEET-DISC-003 [AC-3]: Full journey: discover → scope → call tool succeeds", func() {
		mcpCtx, cancel := context.WithTimeout(context.Background(), 120*time.Second)
		defer cancel()

		By("Connecting to Kuadrant MCP gateway")
		c, err := mcpclient.New(mcpCtx, mcpGatewayURL)
		Expect(err).ToNot(HaveOccurred())
		defer c.Close()

		By("Phase 1: ListClusters — discover available clusters")
		discoverer, err := mcpclient.NewDiscoverer(registry.GatewayKuadrant, c.Session())
		Expect(err).ToNot(HaveOccurred())

		var clusters []mcpclient.ClusterInfo
		Eventually(func(g Gomega) {
			clusters, err = discoverer.ListClusters(mcpCtx, "")
			g.Expect(err).ToNot(HaveOccurred())
			names := make([]string, 0, len(clusters))
			for _, cl := range clusters {
				names = append(names, cl.Name)
			}
			g.Expect(names).To(ContainElement("loopback-cluster"),
				"discover_tools must return loopback-cluster")
		}).WithTimeout(60 * time.Second).WithPolling(5 * time.Second).Should(Succeed())

		By("Phase 2: ToolsForCluster — scope session to loopback-cluster")
		tools, err := discoverer.ToolsForCluster(mcpCtx, "loopback-cluster")
		Expect(err).ToNot(HaveOccurred())
		scopedNames := make([]string, 0, len(tools))
		for _, t := range tools {
			scopedNames = append(scopedNames, t.Name)
		}
		Expect(scopedNames).To(ContainElement("loopback_cluster_namespaces_list"),
			"scoped tools must include namespaces_list")

		By("Phase 3: Call a discovered tool — namespaces_list via the scoped session")
		result, err := c.Session().CallTool(mcpCtx, &mcp.CallToolParams{
			Name: "loopback_cluster_namespaces_list",
		})
		Expect(err).ToNot(HaveOccurred(),
			"AC-3: tool call through two-phase discovery must succeed end-to-end")
		Expect(len(result.Content)).To(BeNumerically(">=", 1),
			"tool call response must contain at least one content block with namespace data")
		Expect(result.IsError).To(BeFalse(),
			"tool call must not return an error result")
	})
})
