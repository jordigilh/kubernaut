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
	katypes "github.com/jordigilh/kubernaut/internal/kubernautagent/types"
)

var _ = Describe("TP-762: InjectRemediationTarget cluster-scoped fix", func() {

	Describe("UT-KA-762-010: InjectRemediationTarget sets namespace=\"\" for cluster-scoped enrichment", func() {
		It("should override signal namespace with empty string for cluster-scoped resource", func() {
			signal := katypes.SignalContext{
				ResourceKind: "Node",
				ResourceName: "worker-1",
				Namespace:    "kube-system",
			}
			enrichData := &enrichment.EnrichmentResult{
				ResourceKind:      "Node",
				ResourceName:      "worker-1",
				ResourceNamespace: "",
				OwnerChain:        []enrichment.OwnerChainEntry{},
			}
			result := &katypes.InvestigationResult{
				RemediationTarget: katypes.RemediationTarget{},
			}

			investigator.InjectRemediationTarget(result, signal, enrichData)

			Expect(result.RemediationTarget.Kind).To(Equal("Node"),
				"UT-KA-762-010: kind should be Node")
			Expect(result.RemediationTarget.Name).To(Equal("worker-1"),
				"UT-KA-762-010: name should be worker-1")
			Expect(result.RemediationTarget.Namespace).To(BeEmpty(),
				"UT-KA-762-010: namespace must be empty for cluster-scoped resource, not signal's kube-system")
		})
	})

	Describe("UT-KA-762-011: InjectRemediationTarget preserves namespace for namespaced enrichment", func() {
		It("should keep the enrichment namespace for namespaced resources", func() {
			signal := katypes.SignalContext{
				ResourceKind: "Pod",
				ResourceName: "web-abc",
				Namespace:    "production",
			}
			enrichData := &enrichment.EnrichmentResult{
				ResourceKind:      "Deployment",
				ResourceName:      "web",
				ResourceNamespace: "production",
				OwnerChain:        []enrichment.OwnerChainEntry{},
			}
			result := &katypes.InvestigationResult{
				RemediationTarget: katypes.RemediationTarget{},
			}

			investigator.InjectRemediationTarget(result, signal, enrichData)

			Expect(result.RemediationTarget.Kind).To(Equal("Deployment"))
			Expect(result.RemediationTarget.Name).To(Equal("web"))
			Expect(result.RemediationTarget.Namespace).To(Equal("production"),
				"UT-KA-762-011: namespace must be preserved for namespaced resources")
		})
	})
})
