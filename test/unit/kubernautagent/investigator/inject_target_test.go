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

var _ = Describe("TP-693: injectRemediationTarget — remediation target resolution", func() {

	Describe("UT-KA-693-001: Empty owner chain uses enrichment source as root", func() {
		It("should use enrichment source identity, not signal, when owner chain is empty", func() {
			signal := katypes.SignalContext{
				ResourceKind: "Pod",
				ResourceName: "worker-77784c6cf7-l27g4",
				Namespace:    "demo-crashloop",
			}
			enrichData := &enrichment.EnrichmentResult{
				ResourceKind:      "Deployment",
				ResourceName:      "worker",
				ResourceNamespace: "demo-crashloop",
				OwnerChain:        []enrichment.OwnerChainEntry{}, // empty after re-enrichment
			}
			result := &katypes.InvestigationResult{
				RemediationTarget: katypes.RemediationTarget{
					Kind:      "Deployment",
					Name:      "worker-77784c6cf7", // LLM hallucinated RS name
					Namespace: "demo-crashloop",
				},
			}

			investigator.InjectRemediationTarget(result, signal, enrichData)

			Expect(result.RemediationTarget.Kind).To(Equal("Deployment"),
				"UT-KA-693-001: must use enrichment source kind")
			Expect(result.RemediationTarget.Name).To(Equal("worker"),
				"UT-KA-693-001: must use enrichment source name, not LLM hallucination")
			Expect(result.RemediationTarget.Namespace).To(Equal("demo-crashloop"))
		})
	})

	Describe("UT-KA-693-002: Non-empty owner chain uses last entry as root", func() {
		It("should use last owner chain entry when chain is populated", func() {
			signal := katypes.SignalContext{
				ResourceKind: "Pod",
				ResourceName: "worker-77784c6cf7-l27g4",
				Namespace:    "demo-crashloop",
			}
			enrichData := &enrichment.EnrichmentResult{
				ResourceKind:      "Pod",
				ResourceName:      "worker-77784c6cf7-l27g4",
				ResourceNamespace: "demo-crashloop",
				OwnerChain: []enrichment.OwnerChainEntry{
					{Kind: "ReplicaSet", Name: "worker-77784c6cf7", Namespace: "demo-crashloop"},
					{Kind: "Deployment", Name: "worker", Namespace: "demo-crashloop"},
				},
			}
			result := &katypes.InvestigationResult{
				RemediationTarget: katypes.RemediationTarget{
					Kind:      "Deployment",
					Name:      "worker-77784c6cf7", // LLM hallucinated RS name
					Namespace: "demo-crashloop",
				},
			}

			investigator.InjectRemediationTarget(result, signal, enrichData)

			Expect(result.RemediationTarget.Kind).To(Equal("Deployment"))
			Expect(result.RemediationTarget.Name).To(Equal("worker"),
				"UT-KA-693-002: must use last chain entry name")
			Expect(result.RemediationTarget.Namespace).To(Equal("demo-crashloop"))
		})
	})

	Describe("UT-KA-693-003: Nil enrichData falls back to signal", func() {
		It("should use signal identity when enrichData is nil", func() {
			signal := katypes.SignalContext{
				ResourceKind: "Pod",
				ResourceName: "worker-77784c6cf7-l27g4",
				Namespace:    "demo-crashloop",
			}
			result := &katypes.InvestigationResult{
				RemediationTarget: katypes.RemediationTarget{},
			}

			investigator.InjectRemediationTarget(result, signal, nil)

			Expect(result.RemediationTarget.Kind).To(Equal("Pod"),
				"UT-KA-693-003: nil enrichData must fall back to signal kind")
			Expect(result.RemediationTarget.Name).To(Equal("worker-77784c6cf7-l27g4"),
				"UT-KA-693-003: nil enrichData must fall back to signal name")
			Expect(result.RemediationTarget.Namespace).To(Equal("demo-crashloop"))
		})
	})
})
