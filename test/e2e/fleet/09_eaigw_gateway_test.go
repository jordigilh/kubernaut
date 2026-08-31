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

// E2E-FLEET-EAIGW: Envoy AI Gateway (EAIGW) E2E tests.
//
// These tests validate the EAIGW integration this suite runs on
// (kubernaut#2309: Kuadrant's static broker credential -- used for its
// self-initiated tool-discovery connection to kube-mcp-server -- proved a
// structural SPOF with no hot-reload and manual-only rotation. EAIGW instead
// forwards the CALLER's own Authorization header straight through to
// kube-mcp-server on every tools/call, so there is no cached credential to
// go stale). This covers Backend/MCPRoute-driven tool routing and FMC
// readiness against the EAIGW gateway type. Kuadrant itself stays covered by
// its own standalone FMC E2E lane (test/e2e/fleetmetadatacache).
//
// Authority: BR-INTEGRATION-065, ADR-068 (MCP Gateway Adapter Pattern),
// kubernaut#2309
// FedRAMP: CM-6 (Configuration Settings), AC-3 (Access Enforcement)
var _ = Describe("E2E-FLEET-EAIGW: Envoy AI Gateway Pipeline", Label("fleet"), func() {
	It("E2E-FLEET-EAIGW-001 [CM-6]: EAIGW responds to MCP initialize and exposes tools with correct prefix", func() {
		mcpCtx := context.Background()
		authClient, err := fleetAuthenticatedHTTPClient()
		Expect(err).ToNot(HaveOccurred(), "should acquire Keycloak token for MCP gateway")
		mcpClient, err := mcpclient.New(mcpCtx, mcpGatewayURL, mcpclient.WithHTTPClient(authClient))
		Expect(err).ToNot(HaveOccurred(), "should connect to EAIGW's generated Service via NodePort")
		defer mcpClient.Close()

		// EAIGW's MCPRoute aggregates tools from every backend it fronts as
		// soon as the route/backend reconcile, but the underlying
		// kube-mcp-server connection is still negotiated lazily per session
		// (same class of delay newFleetMCPClient's 90s retry documents) --
		// poll instead of asserting on the first response.
		By("Listing tools via EAIGW (polling for post-reconcile tool set)")
		var toolNames []string
		Eventually(func(g Gomega) {
			tools, listErr := mcpClient.Session().ListTools(mcpCtx, nil)
			g.Expect(listErr).ToNot(HaveOccurred(), "tools/list must succeed through EAIGW's MCPRoute")
			g.Expect(tools.Tools).ToNot(BeEmpty(), "EAIGW must expose kube-mcp-server tools")

			toolNames = make([]string, 0, len(tools.Tools))
			for _, tool := range tools.Tools {
				toolNames = append(toolNames, tool.Name)
			}
			g.Expect(toolNames).To(ContainElement(HavePrefix("remote-cluster__")),
				"CM-6: tool names must use EAIGW's \"{backendRefs[].name}__\" auto-derived prefix")
		}, 90*time.Second, 5*time.Second).Should(Succeed())
	})

	It("E2E-FLEET-EAIGW-002 [AC-3]: tool call routes through EAIGW (forwarding caller's own token) to kube-mcp-server backend", func() {
		mcpCtx := context.Background()
		mcpClient, err := newFleetMCPClient(mcpCtx)
		Expect(err).ToNot(HaveOccurred())
		defer mcpClient.Close()

		By("Executing namespaces_list tool call through EAIGW's MCPRoute")
		result, err := mcpClient.Session().CallTool(mcpCtx, &mcp.CallToolParams{
			Name: "remote-cluster__namespaces_list",
		})
		Expect(err).ToNot(HaveOccurred(),
			"AC-3: tool call must route through EAIGW to kube-mcp-server using the caller's forwarded Authorization header")
		Expect(result.Content).ToNot(BeEmpty(),
			"tool call response must contain namespace data from remote cluster")
	})

	It("E2E-FLEET-EAIGW-003 [CM-6]: FMC is running and healthy with gatewayType=eaigw", func() {
		By("Verifying FMC deployment has ready replicas")
		cmd := exec.CommandContext(context.Background(),
			"kubectl", "get", "deployment", "fleetmetadatacache",
			"-n", namespace, "--kubeconfig", kubeconfigPath,
			"-o", "jsonpath={.status.readyReplicas}")
		out, err := cmd.Output()
		Expect(err).ToNot(HaveOccurred(), "kubectl get fleetmetadatacache deployment must succeed")
		Expect(strings.TrimSpace(string(out))).To(Equal("1"),
			"CM-6: FMC deployment must have 1 ready replica")

		By("Verifying EAIGW Backend 'remote-cluster' exists (Kuadrant's MCPServerRegistration equivalent)")
		regCmd := exec.CommandContext(context.Background(),
			"kubectl", "get", "backend.gateway.envoyproxy.io", "remote-cluster",
			"-n", namespace, "--kubeconfig", kubeconfigPath,
			"-o", "name")
		regOut, regErr := regCmd.Output()
		Expect(regErr).ToNot(HaveOccurred(), "EAIGW Backend must exist")
		Expect(strings.TrimSpace(string(regOut))).To(ContainSubstring("remote-cluster"),
			"CM-6: Backend 'remote-cluster' must be present")

		By("Verifying the shared MCPRoute exists")
		routeCmd := exec.CommandContext(context.Background(),
			"kubectl", "get", "mcproute.aigateway.envoyproxy.io",
			"-n", namespace, "--kubeconfig", kubeconfigPath,
			"-o", "name")
		routeOut, routeErr := routeCmd.Output()
		Expect(routeErr).ToNot(HaveOccurred(), "kubectl get mcproute must succeed")
		Expect(strings.TrimSpace(string(routeOut))).ToNot(BeEmpty(),
			"CM-6: at least one MCPRoute must be present")

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
