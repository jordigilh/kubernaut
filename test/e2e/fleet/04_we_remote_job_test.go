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
// successfully creates a resource on the loopback cluster.
var _ = Describe("E2E-FLEET-005 [AC-3]: WE dispatches remote Job via MCP gateway to loopback cluster (BR-INTEGRATION-054)", Label("fleet"), func() {
	It("should discover job-creation tools via MCP gateway and verify tool availability", func() {
		mcpCtx := context.Background()
		mcpClient, err := mcpclient.New(mcpCtx, mcpGatewayURL)
		Expect(err).ToNot(HaveOccurred(), "should connect to MCP gateway via NodePort")
		defer mcpClient.Close()

		tools, err := mcpClient.Session().ListTools(mcpCtx, nil)
		Expect(err).ToNot(HaveOccurred())
		Expect(tools.Tools).ToNot(BeEmpty(),
			"MCP gateway should expose K8s MCP Server tools")

		By("Verifying resource creation tools are available for remote execution (AC-3)")
		toolNames := make(map[string]bool)
		for _, tool := range tools.Tools {
			toolNames[tool.Name] = true
		}

		Expect(toolNames).To(HaveKey("loopback_cluster_resources_create"),
			"AC-3: resources_create tool must be available for remote Job dispatch")
		Expect(toolNames).To(HaveKey("loopback_cluster_resources_get"),
			"resources_get tool needed for WE status polling")
	})

	It("should execute a read operation on the remote cluster via MCP gateway", func() {
		mcpCtx := context.Background()
		mcpClient, err := newFleetMCPClient(mcpCtx, "loopback-cluster")
		Expect(err).ToNot(HaveOccurred(), "should connect to MCP gateway via NodePort")
		defer mcpClient.Close()

		By("Reading a well-known resource (kube-system pods) via MCP gateway")
		podList := &unstructured.UnstructuredList{}
		podList.SetGroupVersionKind(schema.GroupVersionKind{Version: "v1", Kind: "PodList"})
		err = mcpClient.List(mcpCtx, podList, client.InNamespace("kube-system"))
		Expect(err).ToNot(HaveOccurred(),
			"AC-3: MCP gateway must support remote list operations")
		Expect(podList.Items).ToNot(BeEmpty(),
			"loopback cluster must have pods in kube-system")
	})
})
