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

package investigator_test

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/jordigilh/kubernaut/internal/kubernautagent/enrichment"
	"github.com/jordigilh/kubernaut/internal/kubernautagent/investigator"
	katypes "github.com/jordigilh/kubernaut/pkg/kubernautagent/types"
)

var _ = Describe("InjectRemediationTarget apiVersion propagation — Issue #1040", func() {

	Describe("UT-KA-1040-005: Preserves LLM apiVersion when kind matches root", func() {
		It("should keep apiVersion when LLM kind equals root owner kind", func() {
			signal := katypes.SignalContext{
				ResourceKind: "Route",
				ResourceName: "storefront",
				Namespace:    "demo-route",
			}
			enrichData := &enrichment.EnrichmentResult{
				ResourceKind:      "Route",
				ResourceName:      "storefront",
				ResourceNamespace: "demo-route",
				OwnerChain:        []enrichment.OwnerChainEntry{},
			}
			result := &katypes.InvestigationResult{
				RemediationTarget: katypes.RemediationTarget{
					Kind:       "Route",
					Name:       "storefront",
					Namespace:  "demo-route",
					APIVersion: "route.openshift.io/v1",
				},
			}

			investigator.InjectRemediationTarget(result, signal, enrichData)

			Expect(result.RemediationTarget.Kind).To(Equal("Route"))
			Expect(result.RemediationTarget.Name).To(Equal("storefront"))
			Expect(result.RemediationTarget.APIVersion).To(Equal("route.openshift.io/v1"),
				"UT-KA-1040-005: apiVersion must be preserved when LLM kind matches root")
		})
	})

	Describe("UT-KA-1040-006: Preserves LLM apiVersion for cross-type target", func() {
		It("should keep LLM apiVersion when target kind is not in owner chain", func() {
			signal := katypes.SignalContext{
				ResourceKind: "Pod",
				ResourceName: "worker-abc123",
				Namespace:    "production",
			}
			enrichData := &enrichment.EnrichmentResult{
				ResourceKind:      "Deployment",
				ResourceName:      "worker",
				ResourceNamespace: "production",
				OwnerChain: []enrichment.OwnerChainEntry{
					{Kind: "ReplicaSet", Name: "worker-abc", Namespace: "production"},
					{Kind: "Deployment", Name: "worker", Namespace: "production"},
				},
			}
			result := &katypes.InvestigationResult{
				RemediationTarget: katypes.RemediationTarget{
					Kind:       "Route",
					Name:       "storefront",
					Namespace:  "demo-route",
					APIVersion: "route.openshift.io/v1",
				},
			}

			investigator.InjectRemediationTarget(result, signal, enrichData)

			Expect(result.RemediationTarget.Kind).To(Equal("Route"),
				"UT-KA-1040-006: cross-type target kind must be preserved")
			Expect(result.RemediationTarget.APIVersion).To(Equal("route.openshift.io/v1"),
				"UT-KA-1040-006: cross-type target apiVersion must be preserved")
		})
	})

	Describe("UT-KA-1040-007: Empty apiVersion works (backwards compat)", func() {
		It("should produce valid result with empty apiVersion", func() {
			signal := katypes.SignalContext{
				ResourceKind: "Pod",
				ResourceName: "worker-abc123",
				Namespace:    "production",
			}
			enrichData := &enrichment.EnrichmentResult{
				ResourceKind:      "Deployment",
				ResourceName:      "worker",
				ResourceNamespace: "production",
				OwnerChain: []enrichment.OwnerChainEntry{
					{Kind: "ReplicaSet", Name: "worker-abc", Namespace: "production"},
					{Kind: "Deployment", Name: "worker", Namespace: "production"},
				},
			}
			result := &katypes.InvestigationResult{
				RemediationTarget: katypes.RemediationTarget{
					Kind:      "Deployment",
					Name:      "worker-wrong",
					Namespace: "production",
				},
			}

			investigator.InjectRemediationTarget(result, signal, enrichData)

			Expect(result.RemediationTarget.Kind).To(Equal("Deployment"))
			Expect(result.RemediationTarget.Name).To(Equal("worker"))
			Expect(result.RemediationTarget.APIVersion).To(BeEmpty(),
				"UT-KA-1040-007: empty apiVersion must remain empty for backwards compat")
		})
	})
})
