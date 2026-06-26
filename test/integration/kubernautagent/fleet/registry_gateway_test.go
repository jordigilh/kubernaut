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

package fleet_test

import (
	"context"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log/zap"

	"github.com/jordigilh/kubernaut/pkg/fleet/mcpclient"
	"github.com/jordigilh/kubernaut/pkg/fleet/registry"
	mockgw "github.com/jordigilh/kubernaut/test/services/mock-mcp-gateway/testutil"
)

// IT-REG-GW: Integration tests for the MCP Gateway Adapter Pattern.
// These tests verify that the registry factory, ToolPrefixResolver, and
// MCP client work together end-to-end through the production dispatch path.
//
// Authority: BR-INTEGRATION-065 (Multi-Cluster Fleet Registry)
// FedRAMP: CM-6 (Configuration Settings), AC-3 (Access Enforcement)
var _ = Describe("Registry Gateway Adapter Integration (BR-INTEGRATION-065)", func() {
	var (
		ctx    context.Context
		gw     *mockgw.MockGateway
		logger = zap.New(zap.UseDevMode(true))
	)

	BeforeEach(func() {
		ctx = context.Background()
	})

	AfterEach(func() {
		if gw != nil {
			gw.Close()
		}
	})

	Describe("IT-REG-GW-001 [CM-6]: Factory-created EAIGW registry responds to cluster events", func() {
		It("creates an EAIGWRegistry via factory and verifies cluster add propagates", func() {
			cfg := registry.RegistryConfig{}
			reg, err := registry.NewClusterRegistry(registry.GatewayEAIGW, nil, cfg, nil, logger)
			Expect(err).ToNot(HaveOccurred())
			Expect(reg).ToNot(BeNil())

			eaigw, ok := reg.(*registry.EAIGWRegistry)
			Expect(ok).To(BeTrue(), "factory must return *EAIGWRegistry for eaigw")

			Expect(eaigw.List()).To(BeEmpty(), "fresh registry must start empty")
			Expect(eaigw.Ready()).To(BeFalse(), "fresh registry must not be ready")
		})
	})

	Describe("IT-REG-GW-002 [CM-6]: Factory-created Kuadrant registry responds to cluster events", func() {
		It("creates a KuadrantRegistry via factory and verifies initial state", func() {
			cfg := registry.RegistryConfig{}
			reg, err := registry.NewClusterRegistry(registry.GatewayKuadrant, nil, cfg, nil, logger)
			Expect(err).ToNot(HaveOccurred())
			Expect(reg).ToNot(BeNil())

			kuadrant, ok := reg.(*registry.KuadrantRegistry)
			Expect(ok).To(BeTrue(), "factory must return *KuadrantRegistry for kuadrant")

			Expect(kuadrant.List()).To(BeEmpty(), "fresh registry must start empty")
			Expect(kuadrant.Ready()).To(BeFalse(), "fresh registry must not be ready")
		})
	})

	Describe("IT-REG-GW-003 [AC-3]: ToolPrefixResolver wires prefix from registry into MCP client calls", func() {
		It("MCP client uses ToolPrefix from ToolPrefixAdapter when resolving tool names", func() {
			gw = mockgw.NewMockGateway(mockgw.WithMultiCluster("spoke-a"))

			parentClient, err := mcpclient.New(ctx, gw.URL())
			Expect(err).ToNot(HaveOccurred())
			defer parentClient.Close()

			session := parentClient.Session()
			Expect(session).ToNot(BeNil())

			toolPrefix := "spoke-a__"
			child := mcpclient.NewFromSession(session, "spoke-a",
				mcpclient.WithToolPrefix(toolPrefix))

			var reader client.Reader = child
			list := &unstructured.UnstructuredList{}
			list.SetGroupVersionKind(schema.GroupVersionKind{Version: "v1", Kind: "PodList"})

			err = reader.List(ctx, list)
			Expect(err).ToNot(HaveOccurred())

			calls := gw.CallLog()
			Expect(calls).ToNot(BeEmpty())
			Expect(calls[0].ToolName).To(Equal("spoke-a__resources_list"),
				"MCP client must use ToolPrefix for tool name resolution")
		})
	})
})
