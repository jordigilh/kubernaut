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

package gateway_test

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	gateway "github.com/jordigilh/kubernaut/pkg/gateway"
	gwtypes "github.com/jordigilh/kubernaut/pkg/gateway/types"
)

// UT-GW-2658-501 / IT-GW-2658-501 (#2658 D4): the readable correlation ID is
// "<alertname>:<canonical identity>". The target is rendered by
// scope.BuildTargetIdentity, so group and version are ordinary parts of the
// coordinates and two siblings in different API groups never share an ID.
var _ = Describe("Gateway readable correlation identity (#2658) [BR-GATEWAY-058-A, AU-3, CC7.2]", func() {
	signal := func(apiVersion string) *gwtypes.NormalizedSignal {
		return &gwtypes.NormalizedSignal{
			SignalName: "HighLatency",
			Namespace:  "prod",
			Resource: gwtypes.ResourceIdentifier{
				Kind:       "Ingress",
				Name:       "web",
				Namespace:  "prod",
				APIVersion: apiVersion,
			},
		}
	}

	It("embeds the canonical identity including group and version", func() {
		Expect(gateway.ConstructReadableCorrelationIDForTest(signal("networking.k8s.io/v1"))).
			To(Equal("HighLatency:prod/networking.k8s.io/v1/Ingress/web"))
	})

	It("renders the core group explicitly", func() {
		Expect(gateway.ConstructReadableCorrelationIDForTest(signal("v1"))).
			To(Equal("HighLatency:prod/core/v1/Ingress/web"))
	})

	It("renders the unresolved form when the apiVersion could not be resolved", func() {
		Expect(gateway.ConstructReadableCorrelationIDForTest(signal(""))).
			To(Equal("HighLatency:prod/unresolved/unresolved/Ingress/web"))
	})

	It("keeps a cross-group pair on distinct correlation IDs", func() {
		first := gateway.ConstructReadableCorrelationIDForTest(signal("networking.k8s.io/v1"))
		second := gateway.ConstructReadableCorrelationIDForTest(signal("extensions/v1beta1"))
		Expect(first).NotTo(Equal(second),
			"two siblings must never be indistinguishable in the audit trail")
	})
})
