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

package registry_test

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"sigs.k8s.io/controller-runtime/pkg/log/zap"

	"github.com/jordigilh/kubernaut/pkg/fleet/registry"
)

var _ = Describe("NewClusterRegistry factory (MCP Gateway Adapter)", func() {
	logger := zap.New(zap.UseDevMode(true))
	cfg := registry.RegistryConfig{}

	It("UT-REG-FAC-001 [CM-6]: Factory returns EAIGWRegistry for GatewayEAIGW", func() {
		reg, err := registry.NewClusterRegistry(registry.GatewayEAIGW, nil, cfg, nil, logger)
		Expect(err).ToNot(HaveOccurred())
		Expect(reg).ToNot(BeNil())
		_, ok := reg.(*registry.EAIGWRegistry)
		Expect(ok).To(BeTrue(), "factory must return *EAIGWRegistry for eaigw")
	})

	It("UT-REG-FAC-002 [CM-6]: Factory returns KuadrantRegistry for GatewayKuadrant", func() {
		reg, err := registry.NewClusterRegistry(registry.GatewayKuadrant, nil, cfg, nil, logger)
		Expect(err).ToNot(HaveOccurred())
		Expect(reg).ToNot(BeNil())
		_, ok := reg.(*registry.KuadrantRegistry)
		Expect(ok).To(BeTrue(), "factory must return *KuadrantRegistry for kuadrant")
	})

	It("UT-REG-FAC-003 [SI-10]: Factory rejects invalid MCPGatewayType with error (input validation)", func() {
		reg, err := registry.NewClusterRegistry(registry.MCPGatewayType("invalid"), nil, cfg, nil, logger)
		Expect(err).To(HaveOccurred())
		Expect(err.Error()).To(ContainSubstring("unsupported MCP gateway type"))
		Expect(reg).To(BeNil())
	})
})
