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

package signalprocessing

import (
	"context"
	"os"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/jordigilh/kubernaut/pkg/fleet/mcpclient"
	"github.com/jordigilh/kubernaut/test/infrastructure"
)

var _ = Describe("E2E-FLEET-SP-001 [SC-7]: Fleet infrastructure deploys with the gateway as the sole entry point for remote cluster access (BR-INTEGRATION-054)", Label("fleet"), func() {
	BeforeEach(func() {
		if os.Getenv("FLEET_E2E") != "true" {
			Skip("FLEET_E2E=true required for fleet E2E tests")
		}
	})

	It("deploys EAIGW + K8s MCP Server and discovers loopback-cluster tools", func() {
		ctx := context.Background()

		kubeconfigPath := os.Getenv("KUBECONFIG")
		if kubeconfigPath == "" {
			kubeconfigPath = os.ExpandEnv("$HOME/.kube/signalprocessing-e2e-config")
		}
		namespace := "kubernaut-system"

		err := infrastructure.DeployFleetInfra(ctx, namespace, kubeconfigPath, GinkgoWriter)
		Expect(err).ToNot(HaveOccurred(), "Fleet infrastructure should deploy successfully")

		err = infrastructure.WaitForFleetReady(GinkgoWriter)
		Expect(err).ToNot(HaveOccurred(), "EAIGW health endpoint should be reachable")

		eaigwURL := "http://localhost:31975/mcp"
		client, err := mcpclient.New(ctx, eaigwURL)
		Expect(err).ToNot(HaveOccurred(), "should connect to EAIGW via NodePort")
		defer client.Close()

		tools, err := client.Session().ListTools(ctx, nil)
		Expect(err).ToNot(HaveOccurred())
		Expect(tools.Tools).ToNot(BeEmpty(),
			"EAIGW should expose K8s MCP Server tools")

		var foundGet bool
		for _, tool := range tools.Tools {
			if tool.Name == "loopback-cluster__resources_get" {
				foundGet = true
				break
			}
		}
		Expect(foundGet).To(BeTrue(),
			"EAIGW must expose loopback-cluster__resources_get tool (SC-7: sole entry point)")
	})
})
