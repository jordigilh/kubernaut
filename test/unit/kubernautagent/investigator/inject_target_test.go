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

	Describe("UT-KA-693-004: LLM names child in owner chain — resolves to root", func() {
		It("should resolve to root owner when LLM names a descendant kind in the chain", func() {
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
					Kind:      "Pod",
					Name:      "worker-77784c6cf7-l27g4",
					Namespace: "demo-crashloop",
				},
			}

			investigator.InjectRemediationTarget(result, signal, enrichData)

			Expect(result.RemediationTarget.Kind).To(Equal("Deployment"),
				"UT-KA-693-004: Pod is in chain; must resolve to root Deployment per BR-496 v2")
			Expect(result.RemediationTarget.Name).To(Equal("worker"),
				"UT-KA-693-004: must use root owner name")
			Expect(result.RemediationTarget.Namespace).To(Equal("demo-crashloop"))
		})
	})

	Describe("UT-KA-693-005: LLM names genuinely cross-type kind — preserves it", func() {
		It("should preserve LLM target when kind is not in the owner chain", func() {
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
					Kind:      "Node",
					Name:      "ip-10-0-1-42",
					Namespace: "",
				},
			}

			investigator.InjectRemediationTarget(result, signal, enrichData)

			Expect(result.RemediationTarget.Kind).To(Equal("Node"),
				"UT-KA-693-005: Node is not in chain; LLM cross-type target must be preserved")
			Expect(result.RemediationTarget.Name).To(Equal("ip-10-0-1-42"),
				"UT-KA-693-005: LLM cross-type name must be preserved")
		})
	})

	Describe("UT-KA-693-006: LLM names mid-chain kind — resolves to root", func() {
		It("should resolve to root when LLM names ReplicaSet that appears in the chain", func() {
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
					Kind:      "ReplicaSet",
					Name:      "worker-77784c6cf7",
					Namespace: "demo-crashloop",
				},
			}

			investigator.InjectRemediationTarget(result, signal, enrichData)

			Expect(result.RemediationTarget.Kind).To(Equal("Deployment"),
				"UT-KA-693-006: ReplicaSet is in chain; must resolve to root Deployment per BR-HAPI-261 AC#4")
			Expect(result.RemediationTarget.Name).To(Equal("worker"),
				"UT-KA-693-006: must use root owner name")
			Expect(result.RemediationTarget.Namespace).To(Equal("demo-crashloop"))
		})
	})

	Describe("UT-KA-693-007: Non-nil enrichData with empty chain and different LLM kind — preserves it", func() {
		It("should preserve cross-type LLM target when enrichData exists but chain is empty", func() {
			signal := katypes.SignalContext{
				ResourceKind: "Pod",
				ResourceName: "worker-77784c6cf7-l27g4",
				Namespace:    "demo-crashloop",
			}
			enrichData := &enrichment.EnrichmentResult{
				ResourceKind:      "Deployment",
				ResourceName:      "worker",
				ResourceNamespace: "demo-crashloop",
				OwnerChain:        []enrichment.OwnerChainEntry{},
			}
			result := &katypes.InvestigationResult{
				RemediationTarget: katypes.RemediationTarget{
					Kind:      "ConfigMap",
					Name:      "worker-config",
					Namespace: "demo-crashloop",
				},
			}

			investigator.InjectRemediationTarget(result, signal, enrichData)

			Expect(result.RemediationTarget.Kind).To(Equal("ConfigMap"),
				"UT-KA-693-007: ConfigMap not in empty chain, not signal kind; must preserve cross-type target")
			Expect(result.RemediationTarget.Name).To(Equal("worker-config"),
				"UT-KA-693-007: must preserve cross-type name")
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
