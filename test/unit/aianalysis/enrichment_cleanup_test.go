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

package aianalysis

import (
	"encoding/json"

	"github.com/go-logr/logr"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/jordigilh/kubernaut/pkg/aianalysis/handlers"
	sharedtypes "github.com/jordigilh/kubernaut/pkg/shared/types"
	"github.com/jordigilh/kubernaut/test/shared/helpers"
)

// ═══════════════════════════════════════════════════════════════════════
// ADR-056 Phase 4: EnrichmentResults Cleanup
//
// Cycle 4.1: Verify DetectedLabels + OwnerChain removed from shared type
// Cycle 4.2: Verify old propagation paths removed from request builder
// ═══════════════════════════════════════════════════════════════════════

var _ = Describe("EnrichmentResults Cleanup (ADR-056 Phase 4)", func() {

	// ═══════════════════════════════════════════════════════════════════
	// Cycle 4.1: Clean type after field removal
	// ═══════════════════════════════════════════════════════════════════

	Context("Cycle 4.1: EnrichmentResults without DetectedLabels/OwnerChain", func() {

		It("UT-AA-056-013: should construct EnrichmentResults with only KubernetesContext and CustomLabels", func() {
			enrichment := sharedtypes.EnrichmentResults{
				KubernetesContext: &sharedtypes.KubernetesContext{
					Namespace: &sharedtypes.NamespaceContext{Name: "production"},
					Workload: &sharedtypes.WorkloadDetails{
						Kind: "Pod",
						Name: "api-pod-abc123",
					},
					CustomLabels: map[string][]string{
						"constraint": {"cost-constrained", "stateful-safe"},
						"team":       {"name=payments"},
					},
				},
			}

			Expect(enrichment.KubernetesContext.Workload.Kind).To(Equal("Pod"))
			Expect(enrichment.KubernetesContext.Namespace.Name).To(Equal("production"))
			Expect(enrichment.KubernetesContext.CustomLabels).To(HaveLen(2))
			Expect(enrichment.KubernetesContext.CustomLabels["constraint"]).To(ContainElement("cost-constrained"))
		})

		It("UT-AA-056-014: should serialize EnrichmentResults JSON without detectedLabels or ownerChain", func() {
			enrichment := sharedtypes.EnrichmentResults{
				KubernetesContext: &sharedtypes.KubernetesContext{
					Namespace: &sharedtypes.NamespaceContext{Name: "default"},
					CustomLabels: map[string][]string{
						"team": {"platform"},
					},
				},
			}

			data, err := json.Marshal(enrichment)
			Expect(err).ToNot(HaveOccurred())

			var raw map[string]interface{}
			err = json.Unmarshal(data, &raw)
			Expect(err).ToNot(HaveOccurred())

			Expect(raw).ToNot(HaveKey("detectedLabels"),
				"detectedLabels must not appear in serialized EnrichmentResults")
			Expect(raw).ToNot(HaveKey("ownerChain"),
				"ownerChain must not appear in serialized EnrichmentResults")
			Expect(raw).To(HaveKey("kubernetesContext"))
			kc := raw["kubernetesContext"].(map[string]interface{})
			Expect(kc).To(HaveKey("customLabels"))
		})

		It("UT-AA-056-015: RequestBuilder should produce valid HAPI request without DetectedLabels", func() {
			builder := handlers.NewRequestBuilder(logr.Discard())

			analysis := helpers.NewAIAnalysis("ai-cleanup-test", "default")
			analysis.Spec.AnalysisRequest.SignalContext.EnrichmentResults = sharedtypes.EnrichmentResults{
				KubernetesContext: &sharedtypes.KubernetesContext{
					CustomLabels: map[string][]string{
						"constraint": {"cost-constrained"},
					},
				},
			}

			req := builder.BuildIncidentRequest(analysis)

			Expect(req.EnrichmentResults.Set).To(BeTrue(),
				"EnrichmentResults should be set in HAPI request")
			Expect(req.EnrichmentResults.Value.CustomLabels.Set).To(BeTrue(),
				"CustomLabels should be forwarded to HAPI")
		})

		It("UT-AA-056-016: incident request builds without removed fields", func() {
			builder := handlers.NewRequestBuilder(logr.Discard())

			analysis := helpers.NewAIAnalysis("ai-validate-test", "default")
			analysis.Spec.AnalysisRequest.SignalContext.EnrichmentResults = sharedtypes.EnrichmentResults{
				KubernetesContext: &sharedtypes.KubernetesContext{
					CustomLabels: map[string][]string{
						"constraint": {"cost-constrained"},
						"team":       {"name=payments"},
					},
				},
			}

			incidentReq := builder.BuildIncidentRequest(analysis)
			Expect(incidentReq.EnrichmentResults.Set).To(BeTrue(),
				"EnrichmentResults should be set in incident request")
		})
	})

	// ═══════════════════════════════════════════════════════════════════
	// Cycle 4.2: Old propagation paths removed
	// ═══════════════════════════════════════════════════════════════════

	Context("Cycle 4.2: Old propagation paths removed", func() {

		It("UT-AA-056-017: HAPI request should not contain DetectedLabels from EnrichmentResults", func() {
			builder := handlers.NewRequestBuilder(logr.Discard())

			analysis := helpers.NewAIAnalysis("ai-no-old-labels", "default")
			analysis.Spec.AnalysisRequest.SignalContext.EnrichmentResults = sharedtypes.EnrichmentResults{
				KubernetesContext: &sharedtypes.KubernetesContext{
					CustomLabels: map[string][]string{
						"team": {"platform"},
					},
				},
			}

			req := builder.BuildIncidentRequest(analysis)

			Expect(req.EnrichmentResults.Set).To(BeTrue(),
				"EnrichmentResults should still be set even without removed fields")
		})

		It("UT-AA-056-019: EnrichmentResults JSON schema contains only authorized fields", func() {
			enrichment := sharedtypes.EnrichmentResults{
				KubernetesContext: &sharedtypes.KubernetesContext{
					Namespace:    &sharedtypes.NamespaceContext{Name: "test"},
					CustomLabels: map[string][]string{"k": {"v"}},
				},
			}

			data, err := json.Marshal(enrichment)
			Expect(err).ToNot(HaveOccurred())

			var raw map[string]interface{}
			err = json.Unmarshal(data, &raw)
			Expect(err).ToNot(HaveOccurred())

			allowedKeys := map[string]bool{
				"kubernetesContext":   true,
				"businessClassification": true,
			}
			for key := range raw {
				Expect(allowedKeys).To(HaveKey(key),
					"unexpected field '%s' found in EnrichmentResults JSON", key)
			}
		})
	})
})
