/*
Copyright 2025 Jordi Gil.

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

// Package aianalysis contains unit tests for AnalyzingHandler's PostRCAContext
// integration with Rego policy evaluation.
//
// ADR-056: AnalyzingHandler reads PostRCAContext.DetectedLabels (from HAPI,
// computed at runtime) when building Rego policy input. This ensures Rego
// policies evaluate against the actual cluster state at RCA time.
//
// Business Requirements:
//   - BR-AI-056: DetectedLabels from PostRCAContext for Rego approval gating
//   - BR-AI-013: Production + stateful workload requires approval
package aianalysis

import (
	"context"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	ctrl "sigs.k8s.io/controller-runtime"

	aianalysisv1 "github.com/jordigilh/kubernaut/api/aianalysis/v1alpha1"
	"github.com/jordigilh/kubernaut/pkg/aianalysis"
	"github.com/jordigilh/kubernaut/pkg/aianalysis/handlers"
	"github.com/jordigilh/kubernaut/pkg/aianalysis/metrics"
	sharedtypes "github.com/jordigilh/kubernaut/pkg/shared/types"
	"github.com/jordigilh/kubernaut/test/shared/mocks"
)

var _ = Describe("AnalyzingHandler PostRCAContext Rego Integration (ADR-056)", func() {
	var (
		handler       *handlers.AnalyzingHandler
		mockEvaluator *mocks.MockRegoEvaluator
		ctx           context.Context
	)

	BeforeEach(func() {
		ctx = context.Background()
		mockEvaluator = mocks.NewMockRegoEvaluator()
		mockAuditClient := &noopAnalyzingAuditClient{}
		testMetrics := metrics.NewMetrics()
		handler = handlers.NewAnalyzingHandler(mockEvaluator, ctrl.Log.WithName("test"), testMetrics, mockAuditClient)
	})

	createAnalysisWithPostRCA := func() *aianalysisv1.AIAnalysis {
		now := metav1.Now()
		return &aianalysisv1.AIAnalysis{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "test-analysis-prc",
				Namespace: "default",
			},
			Spec: aianalysisv1.AIAnalysisSpec{
				RemediationRequestRef: corev1.ObjectReference{
					Kind:      "RemediationRequest",
					Name:      "test-rr-prc",
					Namespace: "default",
				},
				RemediationID: "test-remediation-prc-001",
				AnalysisRequest: aianalysisv1.AnalysisRequest{
					SignalContext: aianalysisv1.SignalContextInput{
						Fingerprint:      "test-fingerprint-prc",
						Severity:         "warning",
						SignalName:       "OOMKilled",
						Environment:      "production",
						BusinessPriority: "P0",
						TargetResource: aianalysisv1.TargetResource{
							Kind:      "Pod",
							Name:      "test-pod-prc",
							Namespace: "default",
						},
					},
					AnalysisTypes: []string{"investigation", "analysis"},
				},
			},
			Status: aianalysisv1.AIAnalysisStatus{
				Phase:     aianalysis.PhaseAnalyzing,
				RootCause: "OOM caused by memory leak",
				SelectedWorkflow: &aianalysisv1.SelectedWorkflow{
					WorkflowID:      "wf-restart-pod",
					ExecutionBundle: "kubernaut.io/workflows/restart:v1.0.0",
					Confidence:      0.92,
					Rationale:       "Selected for OOM recovery",
				},
				PostRCAContext: &aianalysisv1.PostRCAContext{
					DetectedLabels: &sharedtypes.DetectedLabels{
						GitOpsManaged:   true,
						GitOpsTool:      "argocd",
						PDBProtected:    true,
						HPAEnabled:      false,
						Stateful:        true,
						HelmManaged:     false,
						NetworkIsolated: true,
						ServiceMesh:     "istio",
					},
					SetAt: &now,
				},
			},
		}
	}

	// ═══════════════════════════════════════════════════════════════════════
	// UT-AA-056-009: DetectedLabels from PostRCAContext populate Rego input
	// ADR-056: HAPI's runtime-computed labels take precedence for Rego
	// ═══════════════════════════════════════════════════════════════════════

	It("UT-AA-056-009: should read DetectedLabels from PostRCAContext for Rego input", func() {
		analysis := createAnalysisWithPostRCA()

		_, err := handler.Handle(ctx, analysis)

		Expect(err).NotTo(HaveOccurred())
		Expect(mockEvaluator.LastInput).NotTo(BeNil())
		Expect(mockEvaluator.LastInput.DetectedLabels).NotTo(BeNil())

		dl := mockEvaluator.LastInput.DetectedLabels
		Expect(dl["git_ops_managed"]).To(BeTrue(), "gitOpsManaged from PostRCAContext")
		Expect(dl["git_ops_tool"]).To(Equal("argocd"), "gitOpsTool from PostRCAContext")
		Expect(dl["pdb_protected"]).To(BeTrue(), "pdbProtected from PostRCAContext")
		Expect(dl["hpa_enabled"]).To(BeFalse(), "hpaEnabled from PostRCAContext")
		Expect(dl["stateful"]).To(BeTrue(), "stateful from PostRCAContext")
		Expect(dl["helm_managed"]).To(BeFalse(), "helmManaged from PostRCAContext")
		Expect(dl["network_isolated"]).To(BeTrue(), "networkIsolated from PostRCAContext")
		Expect(dl["service_mesh"]).To(Equal("istio"), "serviceMesh from PostRCAContext")
	})

	// ═══════════════════════════════════════════════════════════════════════
	// UT-AA-056-010: FailedDetections from PostRCAContext populate Rego input
	// DD-WORKFLOW-001 v2.2: Detection failure tracking
	// ═══════════════════════════════════════════════════════════════════════

	It("UT-AA-056-010: should read FailedDetections from PostRCAContext for Rego input", func() {
		analysis := createAnalysisWithPostRCA()
		analysis.Status.PostRCAContext.DetectedLabels.FailedDetections = []string{"pdbProtected", "hpaEnabled"}

		_, err := handler.Handle(ctx, analysis)

		Expect(err).NotTo(HaveOccurred())
		Expect(mockEvaluator.LastInput).NotTo(BeNil())
		Expect(mockEvaluator.LastInput.FailedDetections).To(HaveLen(2))
		Expect(mockEvaluator.LastInput.FailedDetections).To(ContainElement("pdbProtected"))
		Expect(mockEvaluator.LastInput.FailedDetections).To(ContainElement("hpaEnabled"))
	})

	// ═══════════════════════════════════════════════════════════════════════
	// UT-AA-056-011: DetectedLabels read exclusively from PostRCAContext
	// ADR-056: EnrichmentResults.DetectedLabels removed; PostRCAContext is sole source
	// ═══════════════════════════════════════════════════════════════════════

	It("UT-AA-056-011: should read DetectedLabels exclusively from PostRCAContext", func() {
		analysis := createAnalysisWithPostRCA()
		Expect(analysis.Status.PostRCAContext.DetectedLabels.Stateful).To(BeTrue(),
			"Precondition: PostRCAContext has stateful=true")

		_, err := handler.Handle(ctx, analysis)

		Expect(err).NotTo(HaveOccurred())
		Expect(mockEvaluator.LastInput).NotTo(BeNil())
		dl := mockEvaluator.LastInput.DetectedLabels
		Expect(dl["stateful"]).To(BeTrue(),
			"ADR-056: DetectedLabels from PostRCAContext")
		Expect(dl["git_ops_managed"]).To(BeTrue(),
			"ADR-056: DetectedLabels from PostRCAContext")
	})

	// ═══════════════════════════════════════════════════════════════════════
	// UT-AA-056-012: Nil DetectedLabels produces empty Rego map
	// ADR-056: detectedLabelsToMap must return empty (non-nil) map when PostRCAContext is nil
	// ═══════════════════════════════════════════════════════════════════════

	It("UT-AA-056-012: should produce empty non-nil Rego DetectedLabels map when PostRCAContext is nil", func() {
		analysis := createAnalysisWithPostRCA()
		analysis.Status.PostRCAContext = nil // Explicitly nil

		_, err := handler.Handle(ctx, analysis)

		Expect(err).NotTo(HaveOccurred())
		Expect(mockEvaluator.LastInput).NotTo(BeNil())
		Expect(mockEvaluator.LastInput.DetectedLabels).NotTo(BeNil(),
			"DetectedLabels must be non-nil empty map, not nil")
		Expect(mockEvaluator.LastInput.DetectedLabels).To(BeEmpty(),
			"DetectedLabels must be empty when PostRCAContext is nil")
	})

})
