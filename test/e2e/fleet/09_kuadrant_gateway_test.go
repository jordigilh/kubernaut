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
	"os/exec"
	"strings"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/jordigilh/kubernaut/pkg/fleet/mcpclient"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// E2E-FLEET-KUA: Kuadrant MCP Gateway E2E tests.
//
// These tests validate the Kuadrant MCP Gateway integration including
// MCPServerRegistration discovery, tool routing via the gateway broker,
// and FMC readiness. Kuadrant is the default fleet MCP gateway.
//
// Authority: BR-INTEGRATION-065, ADR-068 (MCP Gateway Adapter Pattern)
// FedRAMP: CM-6 (Configuration Settings), AC-3 (Access Enforcement)
var _ = Describe("E2E-FLEET-KUA: Kuadrant MCP Gateway Pipeline", Label("fleet"), func() {
	It("E2E-FLEET-KUA-001 [CM-6]: Kuadrant broker responds to MCP initialize and exposes tools with correct prefix", func() {
		mcpCtx := context.Background()
		authClient, err := fleetAuthenticatedHTTPClient()
		Expect(err).ToNot(HaveOccurred(), "should acquire Keycloak token for MCP gateway")
		mcpClient, err := mcpclient.New(mcpCtx, mcpGatewayURL, mcpclient.WithHTTPClient(authClient))
		Expect(err).ToNot(HaveOccurred(), "should connect to Kuadrant MCP gateway via NodePort")
		defer mcpClient.Close()

		// Issue #54 flakiness fix: the Kuadrant broker only exposes the generic
		// "discover_tools"/"select_tools" meta-tools until it finishes syncing
		// cluster-specific tools from kube-mcp-server (~60s observed in spike
		// S15 -- see newFleetMCPClient's identical retry rationale above). List
		// immediately after connecting can race that sync, so poll instead of
		// asserting on the first response.
		By("Listing tools via Kuadrant MCP gateway (polling for post-sync tool set)")
		var toolNames []string
		Eventually(func(g Gomega) {
			tools, listErr := mcpClient.Session().ListTools(mcpCtx, nil)
			g.Expect(listErr).ToNot(HaveOccurred(), "tools/list must succeed through Kuadrant broker")
			g.Expect(tools.Tools).ToNot(BeEmpty(), "Kuadrant broker must expose kube-mcp-server tools")

			toolNames = make([]string, 0, len(tools.Tools))
			for _, tool := range tools.Tools {
				toolNames = append(toolNames, tool.Name)
			}
			g.Expect(toolNames).To(ContainElement(HavePrefix("remote_cluster_")),
				"CM-6: tool names must use prefix from MCPServerRegistration.spec.prefix")
		}, 90*time.Second, 5*time.Second).Should(Succeed())
	})

	It("E2E-FLEET-KUA-002 [AC-3]: tool call routes through Kuadrant broker to kube-mcp-server backend", func() {
		mcpCtx := context.Background()
		mcpClient, err := newFleetMCPClient(mcpCtx, "remote-cluster")
		Expect(err).ToNot(HaveOccurred())
		defer mcpClient.Close()

		By("Executing namespaces_list tool call through Kuadrant gateway")
		result, err := mcpClient.Session().CallTool(mcpCtx, &mcp.CallToolParams{
			Name: "remote_cluster_namespaces_list",
		})
		Expect(err).ToNot(HaveOccurred(),
			"AC-3: tool call must route through Kuadrant broker to kube-mcp-server")
		Expect(result.Content).ToNot(BeEmpty(),
			"tool call response must contain namespace data from remote cluster")
	})

	It("E2E-FLEET-KUA-003 [CM-6]: FMC is running and healthy with gatewayType=kuadrant", func() {
		By("Verifying FMC deployment has ready replicas")
		cmd := exec.CommandContext(context.Background(),
			"kubectl", "get", "deployment", "fleetmetadatacache",
			"-n", namespace, "--kubeconfig", kubeconfigPath,
			"-o", "jsonpath={.status.readyReplicas}")
		out, err := cmd.Output()
		Expect(err).ToNot(HaveOccurred(), "kubectl get fleetmetadatacache deployment must succeed")
		Expect(strings.TrimSpace(string(out))).To(Equal("1"),
			"CM-6: FMC deployment must have 1 ready replica")

		By("Verifying MCPServerRegistration 'remote-cluster' exists")
		regCmd := exec.CommandContext(context.Background(),
			"kubectl", "get", "mcpserverregistration", "remote-cluster",
			"-n", namespace, "--kubeconfig", kubeconfigPath,
			"-o", "name")
		regOut, regErr := regCmd.Output()
		Expect(regErr).ToNot(HaveOccurred(), "MCPServerRegistration must exist")
		Expect(strings.TrimSpace(string(regOut))).To(ContainSubstring("remote-cluster"),
			"CM-6: MCPServerRegistration 'remote-cluster' must be present")

		By("Verifying Valkey deployment is ready")
		valkeyCmd := exec.CommandContext(context.Background(),
			"kubectl", "get", "deployment", "valkey",
			"-n", namespace, "--kubeconfig", kubeconfigPath,
			"-o", "jsonpath={.status.readyReplicas}")
		valkeyOut, valkeyErr := valkeyCmd.Output()
		Expect(valkeyErr).ToNot(HaveOccurred(), "kubectl get valkey deployment must succeed")
		Expect(strings.TrimSpace(string(valkeyOut))).To(Equal("1"),
			"Valkey deployment must have 1 ready replica")
	})
})
