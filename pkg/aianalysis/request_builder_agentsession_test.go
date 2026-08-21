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

package aianalysis_test

import (
	"encoding/json"
	"time"

	"github.com/go-logr/logr"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	aianalysisv1 "github.com/jordigilh/kubernaut/api/aianalysis/v1alpha1"
	"github.com/jordigilh/kubernaut/pkg/aianalysis/handlers"
	sharedtypes "github.com/jordigilh/kubernaut/pkg/shared/types"
	"github.com/jordigilh/kubernaut/test/shared/helpers"
)

// goconst dedup: test-fixture literals deduplicated below.
const (
	oomkilled     = "OOMKilled"
	criticalSev   = "critical"
	productionEnv = "production"
)

// Unit Tests: RequestBuilder.BuildAgentSessionSpec
// DD-AA-KA-001, BR-AA-KA-065.2: 1:1, lossless translation of the retired
// agentclient.IncidentRequest into AgentSessionSpec, sourced from the exact
// same AIAnalysis.Spec.AnalysisRequest fields BuildIncidentRequest read.
var _ = Describe("RequestBuilder.BuildAgentSessionSpec", func() {
	var builder *handlers.RequestBuilder

	BeforeEach(func() {
		builder = handlers.NewRequestBuilder(logr.Discard())
	})

	Describe("required fields", func() {
		It("UT-AA-KA-065-101: should map all required KA fields", func() {
			analysis := helpers.NewAIAnalysis("ai-test", "default")
			analysis.Spec.RemediationRequestRef = corev1.ObjectReference{Name: "rr-test"}
			analysis.Spec.AnalysisRequest.SignalContext.Severity = criticalSev
			analysis.Spec.AnalysisRequest.SignalContext.SignalName = oomkilled
			analysis.Spec.AnalysisRequest.SignalContext.Environment = productionEnv
			analysis.Spec.AnalysisRequest.SignalContext.BusinessPriority = "P0"
			analysis.Spec.AnalysisRequest.SignalContext.TargetResource = aianalysisv1.TargetResource{
				Kind:       "Pod",
				Name:       "test-pod",
				Namespace:  "default",
				APIVersion: "v1",
			}

			spec := builder.BuildAgentSessionSpec(analysis)

			Expect(spec.RemediationRequestRef.Name).To(Equal("rr-test"))
			Expect(spec.RemediationRequestRef.Namespace).To(Equal("default"),
				"AgentSession MUST be created in the same namespace as the RR (analysis.Namespace)")
			Expect(spec.IncidentID).To(Equal("ai-test"), "Q1: use CR name")
			Expect(spec.SignalName).To(Equal(oomkilled))
			Expect(spec.Severity).To(Equal(criticalSev))
			Expect(spec.SignalSource).To(Equal("kubernaut"))
			Expect(spec.Environment).To(Equal(productionEnv))
			Expect(spec.Priority).To(Equal("P0"))
			Expect(spec.ResourceKind).To(Equal("Pod"))
			Expect(spec.ResourceName).To(Equal("test-pod"))
			Expect(spec.ResourceNamespace).To(Equal("default"))
			Expect(spec.ResourceAPIVersion).To(Equal("v1"))
		})

		It("UT-AA-KA-065-102: should send an empty string, not omit, ResourceAPIVersion when unknown (#2064 parity)", func() {
			analysis := helpers.NewAIAnalysis("ai-apiversion-unknown", "default")
			analysis.Spec.AnalysisRequest.SignalContext.TargetResource = aianalysisv1.TargetResource{
				Kind: "Widget", Name: "some-widget", Namespace: "default",
			}

			spec := builder.BuildAgentSessionSpec(analysis)

			Expect(spec.ResourceAPIVersion).To(Equal(""))
		})
	})

	Describe("RemediationID correlation (DD-AUDIT-CORRELATION-001)", func() {
		It("UT-AA-KA-065-103: should prefer RemediationRequestRef.Name over Spec.RemediationID", func() {
			analysis := helpers.NewAIAnalysis("ai-test", "default")
			analysis.Spec.RemediationID = "fallback-id"
			analysis.Spec.RemediationRequestRef = corev1.ObjectReference{Name: "preferred-rr-name"}

			spec := builder.BuildAgentSessionSpec(analysis)

			Expect(spec.RemediationID).To(Equal("preferred-rr-name"))
		})

		It("UT-AA-KA-065-104: should fall back to Spec.RemediationID when RemediationRequestRef.Name is empty", func() {
			analysis := helpers.NewAIAnalysis("ai-test", "default")
			analysis.Spec.RemediationID = "fallback-id"
			analysis.Spec.RemediationRequestRef = corev1.ObjectReference{}

			spec := builder.BuildAgentSessionSpec(analysis)

			Expect(spec.RemediationID).To(Equal("fallback-id"))
		})
	})

	Describe("BR-FLEET-054: ClusterName resolution parity with BuildIncidentRequest", func() {
		It("UT-AA-KA-065-105: should source ClusterName from Spec.ClusterID when set (fleet target)", func() {
			analysis := helpers.NewAIAnalysis("ai-fleet-target", "default")
			analysis.Spec.ClusterID = "remote-cluster"

			spec := builder.BuildAgentSessionSpec(analysis)

			Expect(spec.ClusterName).To(Equal("remote-cluster"))
		})

		It("UT-AA-KA-065-106: should fall back to the cluster_name custom label default when ClusterID is empty", func() {
			analysis := helpers.NewAIAnalysis("ai-hub-local", "default")

			spec := builder.BuildAgentSessionSpec(analysis)

			Expect(spec.ClusterName).To(Equal("default"))
		})
	})

	Describe("BR-FLEET-003: Cluster business classification (omit-when-empty)", func() {
		It("UT-AA-KA-065-107: should forward Cluster when set", func() {
			analysis := helpers.NewAIAnalysis("ai-cluster-classified", "default")
			analysis.Spec.AnalysisRequest.SignalContext.Cluster = "production-fleet"

			spec := builder.BuildAgentSessionSpec(analysis)

			Expect(spec.Cluster).To(Equal("production-fleet"))
		})

		It("UT-AA-KA-065-108: should leave Cluster empty when unset (non-fleet)", func() {
			analysis := helpers.NewAIAnalysis("ai-non-fleet", "default")

			spec := builder.BuildAgentSessionSpec(analysis)

			Expect(spec.Cluster).To(Equal(""))
		})
	})

	Describe("BR-AI-084: SignalMode pass-through", func() {
		It("UT-AA-KA-065-109: should pass signalMode=reactive through", func() {
			analysis := helpers.NewAIAnalysis("ai-test", "default")
			analysis.Spec.AnalysisRequest.SignalContext.SignalMode = "reactive"

			spec := builder.BuildAgentSessionSpec(analysis)

			Expect(spec.SignalMode).To(Equal("reactive"))
		})

		It("UT-AA-KA-065-110: should leave SignalMode empty when unset (backwards compatible)", func() {
			analysis := helpers.NewAIAnalysis("ai-test", "default")

			spec := builder.BuildAgentSessionSpec(analysis)

			Expect(spec.SignalMode).To(Equal(""))
		})
	})

	Describe("#462: SignalAnnotations forwarding", func() {
		It("UT-AA-KA-065-111: should populate SignalAnnotations when present", func() {
			analysis := helpers.NewAIAnalysis("ai-annot-test", "default")
			analysis.Spec.AnalysisRequest.SignalContext.SignalAnnotations = map[string]string{
				"description": "Pod OOMKilled in production",
			}

			spec := builder.BuildAgentSessionSpec(analysis)

			Expect(spec.SignalAnnotations).To(Equal(map[string]string{
				"description": "Pod OOMKilled in production",
			}))
		})

		It("UT-AA-KA-065-112: should leave SignalAnnotations nil when absent", func() {
			analysis := helpers.NewAIAnalysis("ai-no-annot", "default")

			spec := builder.BuildAgentSessionSpec(analysis)

			Expect(spec.SignalAnnotations).To(BeEmpty())
		})
	})

	Describe("EnrichmentResults: lossless raw-JSON carry (improves on the retired lossy ogen mapping)", func() {
		It("UT-AA-KA-065-113: should marshal the full EnrichmentResults, including KubernetesContext content the old ogen path dropped", func() {
			analysis := helpers.NewAIAnalysis("ai-enrich-test", "default")
			analysis.Spec.AnalysisRequest.SignalContext.EnrichmentResults = sharedtypes.EnrichmentResults{
				KubernetesContext: &sharedtypes.KubernetesContext{
					CustomLabels: map[string][]string{"team": {"checkout"}},
				},
				BusinessClassification: &sharedtypes.BusinessClassification{
					BusinessUnit: "payments",
				},
			}

			spec := builder.BuildAgentSessionSpec(analysis)

			Expect(spec.EnrichmentResults).ToNot(BeNil())
			var decoded map[string]interface{}
			Expect(json.Unmarshal(spec.EnrichmentResults.Raw, &decoded)).To(Succeed())
			kc, ok := decoded["kubernetesContext"].(map[string]interface{})
			Expect(ok).To(BeTrue(), "KubernetesContext content must survive the translation, unlike the retired ogen path's SetToNull() placeholder")
			Expect(kc["customLabels"]).ToNot(BeNil())
		})

		It("UT-AA-KA-065-114: should leave EnrichmentResults nil when enrichment is entirely empty", func() {
			analysis := helpers.NewAIAnalysis("ai-no-enrich", "default")

			spec := builder.BuildAgentSessionSpec(analysis)

			Expect(spec.EnrichmentResults).To(BeNil())
		})
	})

	Describe("RiskTolerance / BusinessCategory defaults (custom-label derived, parity with BuildIncidentRequest)", func() {
		It("UT-AA-KA-065-115: should default RiskTolerance and BusinessCategory when no custom labels present", func() {
			analysis := helpers.NewAIAnalysis("ai-defaults", "default")

			spec := builder.BuildAgentSessionSpec(analysis)

			Expect(spec.RiskTolerance).To(Equal("medium"))
			Expect(spec.BusinessCategory).To(Equal("standard"))
		})

		It("UT-AA-KA-065-116: should source RiskTolerance/BusinessCategory from custom labels when present", func() {
			analysis := helpers.NewAIAnalysis("ai-custom-labels", "default")
			analysis.Spec.AnalysisRequest.SignalContext.EnrichmentResults = sharedtypes.EnrichmentResults{
				KubernetesContext: &sharedtypes.KubernetesContext{
					CustomLabels: map[string][]string{
						"risk_tolerance":    {"aggressive"},
						"business_category": {"revenue-critical"},
					},
				},
			}

			spec := builder.BuildAgentSessionSpec(analysis)

			Expect(spec.RiskTolerance).To(Equal("aggressive"))
			Expect(spec.BusinessCategory).To(Equal("revenue-critical"))
		})
	})

	Describe("#2170 (DD-AA-KA-001 Amendment N): TimesOutAt propagation for KA self-enforced timeout", func() {
		It("should propagate AIAnalysis.Spec.TimesOutAt into AgentSessionSpec.TimesOutAt verbatim", func() {
			analysis := helpers.NewAIAnalysis("ai-timeout-test", "default")
			deadline := metav1.NewTime(time.Date(2026, 8, 19, 12, 0, 0, 0, time.UTC))
			analysis.Spec.TimesOutAt = &deadline

			spec := builder.BuildAgentSessionSpec(analysis)

			Expect(spec.TimesOutAt).NotTo(BeNil())
			Expect(spec.TimesOutAt.Time).To(Equal(deadline.Time),
				"KA's dispatcher can only self-enforce the same deadline AA already enforces if it is propagated verbatim, not recomputed")
		})

		It("should leave TimesOutAt nil when AIAnalysis.Spec.TimesOutAt is unset (RO has no authoritative deadline)", func() {
			analysis := helpers.NewAIAnalysis("ai-no-timeout", "default")

			spec := builder.BuildAgentSessionSpec(analysis)

			Expect(spec.TimesOutAt).To(BeNil(),
				"KA must apply no self-enforced deadline when AA has none to propagate (back-compat/defensive)")
		})
	})
})
