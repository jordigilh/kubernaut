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
)

// E2E-FLEET-007: AA investigation with fleet context uses MCP-backed K8s reader
// Authority: Issue #54, ADR-068
// FedRAMP: SI-4 (information system monitoring -- fleet investigation)
var _ = Describe("E2E-FLEET-007 [SI-4]: AA investigation with fleet context uses MCP-backed K8s reader (BR-INTEGRATION-054)", Label("fleet"), func() {
	It("should read deployment status from remote cluster via MCP gateway for investigation context", func() {
		mcpCtx := context.Background()
		mcpClient, err := newFleetMCPClient(mcpCtx, "loopback-cluster")
		Expect(err).ToNot(HaveOccurred(), "should connect to MCP gateway for fleet investigation")
		defer mcpClient.Close()

		By("Reading deployments in kube-system via MCP gateway (SI-4: system monitoring)")
		deployList := &unstructured.UnstructuredList{}
		deployList.SetGroupVersionKind(schema.GroupVersionKind{
			Group:   "apps",
			Version: "v1",
			Kind:    "DeploymentList",
		})
		err = mcpClient.List(mcpCtx, deployList, client.InNamespace("kube-system"))
		Expect(err).ToNot(HaveOccurred(),
			"SI-4: AA must be able to read remote deployment status via MCP")
		Expect(deployList.Items).ToNot(BeEmpty(),
			"kube-system must have deployments accessible for investigation")

		By("Verifying coredns deployment is discoverable (fleet investigation context)")
		var foundCoreDNS bool
		for _, deploy := range deployList.Items {
			if deploy.GetName() == "coredns" {
				foundCoreDNS = true
				break
			}
		}
		Expect(foundCoreDNS).To(BeTrue(),
			"SI-4: fleet investigation must discover coredns deployment on remote cluster")
	})

	It("should read pod events from remote cluster for root cause analysis", func() {
		mcpCtx := context.Background()
		mcpClient, err := newFleetMCPClient(mcpCtx, "loopback-cluster")
		Expect(err).ToNot(HaveOccurred())
		defer mcpClient.Close()

		By("Reading events in kube-system via MCP gateway (fleet RCA)")
		eventList := &unstructured.UnstructuredList{}
		eventList.SetGroupVersionKind(schema.GroupVersionKind{
			Version: "v1",
			Kind:    "EventList",
		})
		err = mcpClient.List(mcpCtx, eventList, client.InNamespace("kube-system"))
		Expect(err).ToNot(HaveOccurred(),
			"SI-4: AA fleet investigation must be able to read events via MCP gateway")
	})
})
