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

	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/jordigilh/kubernaut/pkg/fleet/mcpclient"
)

// E2E-FLEET-005: WE dispatches remote Job via MCP gateway
// Authority: Issue #54, ADR-068
// FedRAMP: AC-3 (access enforcement -- remote execution boundary)
//
// This test validates that the MCP gateway exposes the tools needed for
// remote Job creation, and that a tool call routed through the gateway
// successfully creates a resource on the remote cluster.
var _ = Describe("E2E-FLEET-005 [AC-3]: WE dispatches remote Job via MCP gateway to remote cluster (BR-INTEGRATION-054)", Label("fleet"), func() {
	It("should discover job-creation tools via MCP gateway and verify tool availability", func() {
		mcpCtx := context.Background()
		authClient, err := fleetAuthenticatedHTTPClient()
		Expect(err).ToNot(HaveOccurred(), "should acquire Keycloak token for MCP gateway")
		mcpClient, err := mcpclient.New(mcpCtx, mcpGatewayURL, mcpclient.WithHTTPClient(authClient))
		Expect(err).ToNot(HaveOccurred(), "should connect to MCP gateway via NodePort")
		defer mcpClient.Close()

		// Issue #54 flakiness fix: the MCP gateway broker only exposes the
		// generic "discover_tools"/"select_tools" meta-tools until it finishes
		// syncing cluster-specific tools from kube-mcp-server (~60s observed in
		// spike S15 -- see suite_test.go's newFleetMCPClient for the identical
		// retry rationale). Poll instead of asserting on the first response.
		By("Verifying resource creation tools are available for remote execution (AC-3)")
		Eventually(func(g Gomega) {
			tools, listErr := mcpClient.Session().ListTools(mcpCtx, nil)
			g.Expect(listErr).ToNot(HaveOccurred())
			g.Expect(tools.Tools).ToNot(BeEmpty(),
				"MCP gateway should expose K8s MCP Server tools")

			toolNames := make(map[string]bool, len(tools.Tools))
			for _, tool := range tools.Tools {
				toolNames[tool.Name] = true
			}

			g.Expect(toolNames).To(HaveKey("remote_cluster_resources_create_or_update"),
				"AC-3: resources_create_or_update tool must be available for remote Job dispatch")
			g.Expect(toolNames).To(HaveKey("remote_cluster_resources_get"),
				"resources_get tool needed for WE status polling")
		}, 90*time.Second, 5*time.Second).Should(Succeed())
	})

	It("should execute a read operation on the remote cluster via MCP gateway", func() {
		mcpCtx := context.Background()
		mcpClient, err := newFleetMCPClient(mcpCtx)
		Expect(err).ToNot(HaveOccurred(), "should connect to MCP gateway via NodePort")
		defer mcpClient.Close()

		By("Reading a well-known resource (kube-system pods) via MCP gateway")
		podList := &unstructured.UnstructuredList{}
		podList.SetGroupVersionKind(schema.GroupVersionKind{Version: "v1", Kind: "PodList"})
		err = mcpClient.List(mcpCtx, podList, client.InNamespace("kube-system"))
		Expect(err).ToNot(HaveOccurred(),
			"AC-3: MCP gateway must support remote list operations")
		Expect(podList.Items).ToNot(BeEmpty(),
			"remote cluster must have pods in kube-system")
	})
})
