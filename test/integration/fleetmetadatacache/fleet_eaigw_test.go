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

package fleetmetadatacache_test

import (
	"context"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/jordigilh/kubernaut/pkg/fleet/mcpclient"
	"github.com/jordigilh/kubernaut/test/infrastructure"
	mockgw "github.com/jordigilh/kubernaut/test/services/mock-mcp-gateway/testutil"
)

// IT-FLEET-EAIGW-001 proves EAIGW's raw tools/list + tool-name-prefixing
// behavior against a real standalone EAIGW container -- a fast, always-on
// complement to the heavier CRD-based E2E coverage in
// test/e2e/fleetmetadatacache/eaigw/ (which drives EAIGW through its actual
// Helm-installed controller, GatewayClass/Gateway, and Backend/MCPRoute
// CRDs, Spike S18). This container-only test previously lived in
// test/integration/kubernautagent/fleet/ gated behind FLEET_IT_CONTAINERS
// (never set in CI); relocated here (the package that already owns EAIGW's
// Backend-CRD registry wiring, pkg/fleet/registry/eaigw_registry.go) and
// un-gated -- it is a single lightweight container, consistent with this
// package's existing --network=host kube-mcp-server containers
// (fmc_e2e_test.go).
var _ = Describe("IT-FLEET-EAIGW-001 [SC-7]: All remote cluster tool calls are routed through the gateway chokepoint with per-cluster namespace isolation (BR-INTEGRATION-054)", Label("container"), func() {
	var (
		ctx         context.Context
		mockBackend *mockgw.MockGateway
		eaigw       *infrastructure.ContainerInstance
	)

	BeforeEach(func() {
		ctx = context.Background()
	})

	AfterEach(func() {
		if eaigw != nil {
			_ = infrastructure.StopEAIGWContainer(eaigw, GinkgoWriter)
		}
		if mockBackend != nil {
			mockBackend.Close()
		}
	})

	It("routes tool calls through real EAIGW container to mock backend", func() {
		mockBackend = mockgw.NewMockGateway(mockgw.WithMultiCluster("cluster-a"))

		backendURL := mockBackend.URL()
		GinkgoWriter.Printf("Mock backend started at: %s\n", backendURL)

		servers := []infrastructure.EAIGWMCPServerEntry{
			{Name: "cluster-a", Host: backendURL},
		}

		var err error
		eaigw, err = infrastructure.StartEAIGWContainer(servers, GinkgoWriter)
		Expect(err).ToNot(HaveOccurred(), "EAIGW container should start successfully")

		eaigwURL := "http://127.0.0.1:19750/mcp"
		client, err := mcpclient.New(ctx, eaigwURL)
		Expect(err).ToNot(HaveOccurred(), "should connect to EAIGW MCP endpoint")
		defer client.Close()

		tools, err := client.Session().ListTools(ctx, nil)
		Expect(err).ToNot(HaveOccurred())
		Expect(tools.Tools).ToNot(BeEmpty(),
			"EAIGW should expose tools from backend")

		var foundGet bool
		for _, tool := range tools.Tools {
			if tool.Name == "cluster-a__resources_get" {
				foundGet = true
				break
			}
		}
		Expect(foundGet).To(BeTrue(),
			"EAIGW must namespace tools with {backendName}__ prefix (SC-7 boundary protection)")
	})
})
