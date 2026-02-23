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
	"github.com/go-logr/logr"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	aianalysisv1 "github.com/jordigilh/kubernaut/api/aianalysis/v1alpha1"
	"github.com/jordigilh/kubernaut/pkg/aianalysis/handlers"
	"github.com/jordigilh/kubernaut/pkg/holmesgpt/client"
	sharedtypes "github.com/jordigilh/kubernaut/pkg/shared/types"
	"github.com/jordigilh/kubernaut/test/shared/helpers"
)

// Unit Tests: RequestBuilder - Signal Mode Pass-through
// BR-AI-084: Predictive Signal Mode Prompt Strategy
// ADR-054: Predictive Signal Mode Classification and Prompt Strategy
var _ = Describe("RequestBuilder", func() {
	var (
		builder *handlers.RequestBuilder
	)

	BeforeEach(func() {
		builder = handlers.NewRequestBuilder(logr.Discard())
	})

	Describe("BuildIncidentRequest", func() {
		Context("BR-AI-084: Signal mode pass-through to HAPI", func() {
			It("UT-AA-084-001: should pass signalMode = reactive to HAPI", func() {
				// Arrange: AA with reactive signal mode
				analysis := helpers.NewAIAnalysis("ai-test", "default")
				analysis.Spec.AnalysisRequest.SignalContext.SignalMode = "reactive"
				analysis.Spec.AnalysisRequest.SignalContext.SignalName = "OOMKilled"

				// Act
				req := builder.BuildIncidentRequest(analysis)

				// Assert
				Expect(req).ToNot(BeNil())
				Expect(req.SignalMode.Set).To(BeTrue())
				Expect(req.SignalMode.Value).To(Equal(client.SignalMode("reactive")))
				Expect(req.SignalType).To(Equal("OOMKilled"))
			})

			It("UT-AA-084-002: should pass signalMode = predictive to HAPI", func() {
				// Arrange: AA with predictive signal mode
				analysis := helpers.NewAIAnalysis("ai-test", "default")
				analysis.Spec.AnalysisRequest.SignalContext.SignalMode = "predictive"
				analysis.Spec.AnalysisRequest.SignalContext.SignalName = "OOMKilled" // normalized by SP

				// Act
				req := builder.BuildIncidentRequest(analysis)

				// Assert
				Expect(req).ToNot(BeNil())
				Expect(req.SignalMode.Set).To(BeTrue())
				Expect(req.SignalMode.Value).To(Equal(client.SignalMode("predictive")))
				// SignalType should be the normalized type from SP (not PredictedOOMKill)
				Expect(req.SignalType).To(Equal("OOMKilled"))
			})

			It("should not set signalMode when empty (backwards compatible)", func() {
				// Arrange: AA without signal mode (pre-BR-SP-106 CRDs)
				analysis := helpers.NewAIAnalysis("ai-test", "default")
				analysis.Spec.AnalysisRequest.SignalContext.SignalMode = "" // empty

				// Act
				req := builder.BuildIncidentRequest(analysis)

				// Assert
				Expect(req).ToNot(BeNil())
				Expect(req.SignalMode.Set).To(BeFalse(), "SignalMode should not be set when empty")
			})
		})
	})

	Describe("BuildIncidentRequest - existing fields", func() {
		It("should set all required HAPI fields", func() {
			analysis := helpers.NewAIAnalysis("ai-test", "default")
			analysis.Spec.AnalysisRequest.SignalContext.Severity = "critical"
			analysis.Spec.AnalysisRequest.SignalContext.SignalName = "OOMKilled"
			analysis.Spec.AnalysisRequest.SignalContext.Environment = "production"
			analysis.Spec.AnalysisRequest.SignalContext.BusinessPriority = "P0"
			analysis.Spec.AnalysisRequest.SignalContext.TargetResource = aianalysisv1.TargetResource{
				Kind:      "Pod",
				Name:      "test-pod",
				Namespace: "default",
			}

			req := builder.BuildIncidentRequest(analysis)

			Expect(req.SignalType).To(Equal("OOMKilled"))
			Expect(req.Severity).To(Equal(client.Severity("critical")))
			Expect(req.Environment).To(Equal("production"))
			Expect(req.Priority).To(Equal("P0"))
			Expect(req.ResourceKind).To(Equal("Pod"))
			Expect(req.ResourceName).To(Equal("test-pod"))
			Expect(req.ResourceNamespace).To(Equal("default"))
		})
	})

	Describe("BuildIncidentRequest - BusinessClassification mapping (BR-SP-002)", func() {
		It("should map all BusinessClassification fields to HAPI client types", func() {
			analysis := helpers.NewAIAnalysis("ai-bizclass", "default")
			analysis.Spec.AnalysisRequest.SignalContext.EnrichmentResults.BusinessClassification = &sharedtypes.BusinessClassification{
				BusinessUnit:   "payments",
				ServiceOwner:   "team-checkout",
				Criticality:    "critical",
				SLARequirement: "platinum",
			}

			req := builder.BuildIncidentRequest(analysis)

			bc, ok := req.EnrichmentResults.Value.BusinessClassification.Get()
			Expect(ok).To(BeTrue(), "BusinessClassification should be set")
			Expect(bc.BusinessUnit.Or("")).To(Equal("payments"))
			Expect(bc.ServiceOwner.Or("")).To(Equal("team-checkout"))
			Expect(bc.Criticality.Or("")).To(Equal("critical"))
			Expect(bc.SlaRequirement.Or("")).To(Equal("platinum"))
		})

		It("should not set BusinessClassification when nil in enrichment", func() {
			analysis := helpers.NewAIAnalysis("ai-no-bizclass", "default")

			req := builder.BuildIncidentRequest(analysis)

			Expect(req.EnrichmentResults.Value.BusinessClassification.IsSet()).To(BeFalse())
		})

		It("should map partial BusinessClassification fields", func() {
			analysis := helpers.NewAIAnalysis("ai-partial-bizclass", "default")
			analysis.Spec.AnalysisRequest.SignalContext.EnrichmentResults.BusinessClassification = &sharedtypes.BusinessClassification{
				Criticality: "high",
			}

			req := builder.BuildIncidentRequest(analysis)

			bc, ok := req.EnrichmentResults.Value.BusinessClassification.Get()
			Expect(ok).To(BeTrue())
			Expect(bc.Criticality.Or("")).To(Equal("high"))
			Expect(bc.BusinessUnit.IsSet()).To(BeFalse())
			Expect(bc.ServiceOwner.IsSet()).To(BeFalse())
			Expect(bc.SlaRequirement.IsSet()).To(BeFalse())
		})
	})

	Describe("BuildRecoveryRequest - BusinessClassification mapping (BR-SP-002)", func() {
		It("should map BusinessClassification to recovery request enrichment", func() {
			analysis := helpers.NewAIAnalysis("ai-recovery-bizclass", "default")
			analysis.Spec.AnalysisRequest.SignalContext.EnrichmentResults.BusinessClassification = &sharedtypes.BusinessClassification{
				BusinessUnit:   "platform",
				Criticality:    "medium",
				SLARequirement: "silver",
			}

			req := builder.BuildRecoveryRequest(analysis)

			bc, ok := req.EnrichmentResults.Value.BusinessClassification.Get()
			Expect(ok).To(BeTrue())
			Expect(bc.BusinessUnit.Or("")).To(Equal("platform"))
			Expect(bc.Criticality.Or("")).To(Equal("medium"))
			Expect(bc.SlaRequirement.Or("")).To(Equal("silver"))
		})
	})
})
