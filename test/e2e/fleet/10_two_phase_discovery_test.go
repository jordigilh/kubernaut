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

// E2E-FLEET-DISC: Two-phase discovery journey through Envoy AI Gateway (EAIGW).
//
// This suite proves the Pyramid Invariant's E2E layer for the
// GatewayDiscoverer's own two-phase protocol (ListTools scan → filter by
// "{clusterID}__" prefix), in isolation from any caller:
//
//	ListClusters (tools/list scan) → ToolsForCluster (prefix filter)
//	→ tool call succeeds
//
// This is a client-protocol test, not a KA production-flow test. KA itself
// no longer drives this sequence at investigation time — per DD-FLEET-005
// (issue #1732), KA pre-scopes server-side via ToolsForCluster(signal.ClusterID)
// alone, and never calls ListClusters or lets the LLM pick a cluster.
// ListClusters remains exercised here as part of the GatewayDiscoverer
// contract other programmatic callers (SP, WE, FMC, EM) may rely on.
//
// This suite moved from Kuadrant to EAIGW (kubernaut#2309: Kuadrant's static
// broker credential proved a structural SPOF). Kuadrant's own
// discover_tools/select_tools meta-tool protocol stays covered by
// KuadrantDiscoverer's unit tests plus the standalone FMC E2E lane.
//
// Authority: Issue #54, ADR-068 decision #11 (as amended by DD-FLEET-005), BR-FLEET-054
// FedRAMP: CM-6 (Configuration Settings), AC-3 (Access Enforcement)
var _ = Describe("E2E-FLEET-DISC: Two-Phase Discovery Journey", Label("fleet"), func() {

	It("E2E-FLEET-DISC-001 [CM-6]: ListClusters discovers remote-cluster via the EAIGW tools/list prefix scan", func() {
		mcpCtx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
		defer cancel()

		By("Connecting to EAIGW")
		authClient, err := fleetAuthenticatedHTTPClient()
		Expect(err).ToNot(HaveOccurred(), "should acquire Keycloak token for MCP gateway")
		c, err := mcpclient.New(mcpCtx, mcpGatewayURL, mcpclient.WithHTTPClient(authClient))
		Expect(err).ToNot(HaveOccurred(), "should connect to MCP gateway")
		defer c.Close()

		By("Creating EAIGWDiscoverer via factory")
		discoverer, err := mcpclient.NewDiscoverer(registry.GatewayEAIGW, c.Session())
		Expect(err).ToNot(HaveOccurred(), "factory must create EAIGWDiscoverer")

		By("Calling ListClusters (scans tools/list for the \"{clusterID}__\" prefix)")
		var clusters []mcpclient.ClusterInfo
		Eventually(func(g Gomega) {
			clusters, err = discoverer.ListClusters(mcpCtx, "")
			g.Expect(err).ToNot(HaveOccurred(), "ListClusters must succeed")
			names := make([]string, 0, len(clusters))
			for _, cl := range clusters {
				names = append(names, cl.Name)
			}
			g.Expect(names).To(ContainElement("remote-cluster"),
				"CM-6: remote-cluster must be discoverable via the EAIGW-generated tool prefix")
		}).WithTimeout(60 * time.Second).WithPolling(5 * time.Second).Should(Succeed())

	})

	It("E2E-FLEET-DISC-002 [AC-3]: ToolsForCluster returns scoped tools for remote-cluster via prefix filtering", func() {
		mcpCtx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
		defer cancel()

		By("Connecting to EAIGW")
		authClient, err := fleetAuthenticatedHTTPClient()
		Expect(err).ToNot(HaveOccurred(), "should acquire Keycloak token for MCP gateway")
		c, err := mcpclient.New(mcpCtx, mcpGatewayURL, mcpclient.WithHTTPClient(authClient))
		Expect(err).ToNot(HaveOccurred())
		defer c.Close()

		By("Creating EAIGWDiscoverer")
		discoverer, err := mcpclient.NewDiscoverer(registry.GatewayEAIGW, c.Session())
		Expect(err).ToNot(HaveOccurred())

		By("Calling ToolsForCluster (filters tools/list by the \"remote-cluster__\" prefix)")
		var tools []mcpclient.ToolDefinition
		Eventually(func(g Gomega) {
			tools, err = discoverer.ToolsForCluster(mcpCtx, "remote-cluster")
			g.Expect(err).ToNot(HaveOccurred(), "ToolsForCluster must succeed")
			names := make([]string, 0, len(tools))
			for _, t := range tools {
				names = append(names, t.Name)
			}
			g.Expect(names).To(ContainElement("remote-cluster__namespaces_list"),
				"AC-3: scoped tools must include namespaces_list for remote-cluster")
		}).WithTimeout(60 * time.Second).WithPolling(5 * time.Second).Should(Succeed())

		By("Verifying tool names use the remote-cluster__ prefix")
		for _, tool := range tools {
			Expect(tool.Name).To(HavePrefix("remote-cluster__"),
				"AC-3: all scoped tools must carry the cluster prefix")
		}

		By("Verifying namespaces_list tool is present (kube-mcp-server standard tool)")
		toolNames := make([]string, 0, len(tools))
		for _, tool := range tools {
			toolNames = append(toolNames, tool.Name)
		}
		Expect(toolNames).To(ContainElement("remote-cluster__namespaces_list"),
			"kube-mcp-server must expose namespaces_list through the gateway")
	})

	It("E2E-FLEET-DISC-003 [AC-3]: Full journey: discover → scope → call tool succeeds", func() {
		mcpCtx, cancel := context.WithTimeout(context.Background(), 120*time.Second)
		defer cancel()

		By("Connecting to EAIGW")
		authClient, err := fleetAuthenticatedHTTPClient()
		Expect(err).ToNot(HaveOccurred(), "should acquire Keycloak token for MCP gateway")
		c, err := mcpclient.New(mcpCtx, mcpGatewayURL, mcpclient.WithHTTPClient(authClient))
		Expect(err).ToNot(HaveOccurred())
		defer c.Close()

		By("Phase 1: ListClusters — discover available clusters")
		discoverer, err := mcpclient.NewDiscoverer(registry.GatewayEAIGW, c.Session())
		Expect(err).ToNot(HaveOccurred())

		var clusters []mcpclient.ClusterInfo
		Eventually(func(g Gomega) {
			clusters, err = discoverer.ListClusters(mcpCtx, "")
			g.Expect(err).ToNot(HaveOccurred())
			names := make([]string, 0, len(clusters))
			for _, cl := range clusters {
				names = append(names, cl.Name)
			}
			g.Expect(names).To(ContainElement("remote-cluster"),
				"the tools/list prefix scan must return remote-cluster")
		}).WithTimeout(60 * time.Second).WithPolling(5 * time.Second).Should(Succeed())

		By("Phase 2: ToolsForCluster — scope session to remote-cluster")
		tools, err := discoverer.ToolsForCluster(mcpCtx, "remote-cluster")
		Expect(err).ToNot(HaveOccurred())
		scopedNames := make([]string, 0, len(tools))
		for _, t := range tools {
			scopedNames = append(scopedNames, t.Name)
		}
		Expect(scopedNames).To(ContainElement("remote-cluster__namespaces_list"),
			"scoped tools must include namespaces_list")

		By("Phase 3: Call a discovered tool — namespaces_list via the scoped session")
		// Unlike Phase 1/2 above, this was previously a single unretried call
		// -- kube-mcp-server negotiates its session with the remote cluster
		// lazily on first use, so this exact call is where a CI run observed
		// "authorization required" during the thundering-herd race at suite
		// start (12 parallel processes all opening fresh sessions within the
		// same ~150ms window). Eventually here mirrors Phase 1/2's existing
		// retry budget instead of leaving this call as the one unprotected
		// step in an otherwise-retried journey.
		var result *mcp.CallToolResult
		Eventually(func(g Gomega) {
			result, err = c.Session().CallTool(mcpCtx, &mcp.CallToolParams{
				Name: "remote-cluster__namespaces_list",
			})
			g.Expect(err).ToNot(HaveOccurred(),
				"AC-3: tool call through two-phase discovery must succeed end-to-end")
		}).WithTimeout(60 * time.Second).WithPolling(5 * time.Second).Should(Succeed())
		Expect(result.Content).ToNot(BeEmpty(), "tool call response must contain at least one content block with namespace data")
		Expect(result.IsError).To(BeFalse(),
			"tool call must not return an error result")
	})
})
