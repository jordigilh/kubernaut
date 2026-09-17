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
	"fmt"
	"time"

	"github.com/go-logr/logr"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/jordigilh/kubernaut/pkg/fleet/registry"
)

// IT-REG-GW-005 proves BR-INTEGRATION-065's runtime health contract against
// the real envtest API: the registry must reconcile its authoritative state
// after startup, not remain permanently healthy based only on its first sync.
// FedRAMP SI-4 is satisfied by detecting API-backed registry health changes.
var _ = Describe("IT-REG-GW-005 [SI-4]: authoritative registry health refresh", Label("fmc", "integration"), func() {
	It("refreshes EAIGW registry membership after Kubernetes API changes", func() {
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		name := fmt.Sprintf("it-reg-gw-005-eaigw-%d", GinkgoParallelProcess())

		reg := registry.NewEAIGWRegistry(dynClient, registry.EAIGWRegistryConfig{}, nil, logr.Discard())
		defer reg.Stop()
		Expect(reg.Start(ctx)).To(Succeed())
		_, found := reg.Get(name)
		Expect(found).To(BeFalse())

		By("creating a managed Backend after the initial informer sync")
		createBackend(ctx, name)
		Expect(reg.Probe(ctx)).To(Succeed())
		_, found = reg.Get(name)
		Expect(found).To(BeTrue(), "SI-4: an authoritative refresh must discover new managed clusters")

		By("deleting the Backend and refreshing again")
		deleteBackend(ctx, name)
		Expect(reg.Probe(ctx)).To(Succeed())
		_, found = reg.Get(name)
		Expect(found).To(BeFalse(), "SI-4: an authoritative refresh must remove deleted managed clusters")
	})

	It("refreshes Kuadrant registry membership after Kubernetes API changes", func() {
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		name := fmt.Sprintf("it-reg-gw-005-kuadrant-%d", GinkgoParallelProcess())

		reg := registry.NewKuadrantRegistry(dynClient, registry.EAIGWRegistryConfig{}, nil, logr.Discard())
		defer reg.Stop()
		Expect(reg.Start(ctx)).To(Succeed())
		_, found := reg.Get(name)
		Expect(found).To(BeFalse())

		By("creating a managed MCPServerRegistration after the initial informer sync")
		createMCPServerRegistration(ctx, name)
		Expect(reg.Probe(ctx)).To(Succeed())
		_, found = reg.Get(name)
		Expect(found).To(BeTrue(), "SI-4: an authoritative refresh must discover new managed clusters")

		By("deleting the MCPServerRegistration and refreshing again")
		deleteMCPServerRegistration(ctx, name)
		Expect(reg.Probe(ctx)).To(Succeed())
		_, found = reg.Get(name)
		Expect(found).To(BeFalse(), "SI-4: an authoritative refresh must remove deleted managed clusters")
	})
})
