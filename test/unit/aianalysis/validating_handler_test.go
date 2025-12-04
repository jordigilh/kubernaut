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

package aianalysis

import (
	"context"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	logf "sigs.k8s.io/controller-runtime/pkg/log"

	aianalysisv1 "github.com/jordigilh/kubernaut/api/aianalysis/v1alpha1"
	"github.com/jordigilh/kubernaut/pkg/aianalysis/handlers"
	sharedtypes "github.com/jordigilh/kubernaut/pkg/shared/types"
)

// BR-AI-001, BR-AI-002: ValidatingHandler Unit Tests
var _ = Describe("ValidatingHandler", func() {
	var (
		ctx     context.Context
		handler *handlers.ValidatingHandler
	)

	BeforeEach(func() {
		ctx = context.Background()
		handler = handlers.NewValidatingHandler(
			handlers.WithLogger(logf.Log.WithName("test")),
		)
	})

	Describe("ValidateSpec", func() {
		// Table-driven tests for spec validation (BR-AI-001)
		// Reference: SERVICE_IMPLEMENTATION_PLAN_TEMPLATE.md lines 2586-2619
		DescribeTable("validates AIAnalysis spec fields",
			func(description string, modifySpec func(*aianalysisv1.AIAnalysisSpec), expectValid bool, expectedError string) {
				analysis := validAIAnalysisForTest()
				modifySpec(&analysis.Spec)

				err := handler.ValidateSpec(ctx, analysis)

				if expectValid {
					Expect(err).NotTo(HaveOccurred(), "Expected valid spec: %s", description)
				} else {
					Expect(err).To(HaveOccurred(), "Expected invalid spec: %s", description)
					Expect(err.Error()).To(ContainSubstring(expectedError))
				}
			},
			// Happy Path entries
			Entry("valid complete spec - BR-AI-001",
				"all required fields present",
				func(s *aianalysisv1.AIAnalysisSpec) {}, // No modification
				true, "",
			),
			Entry("valid spec with minimal enrichment - BR-AI-001",
				"minimal enrichment results",
				func(s *aianalysisv1.AIAnalysisSpec) {
					s.AnalysisRequest.SignalContext.EnrichmentResults = sharedtypes.EnrichmentResults{}
				},
				true, "",
			),

			// Edge Cases: Missing required fields
			Entry("missing remediationRequestRef.name - BR-AI-002",
				"remediationRequestRef.name is required",
				func(s *aianalysisv1.AIAnalysisSpec) {
					s.RemediationRequestRef.Name = ""
				},
				false, "remediationRequestRef.name",
			),
			Entry("missing remediationId - BR-AI-002",
				"remediationId is required",
				func(s *aianalysisv1.AIAnalysisSpec) {
					s.RemediationID = ""
				},
				false, "remediationId",
			),
			Entry("missing environment - BR-AI-002",
				"environment is required",
				func(s *aianalysisv1.AIAnalysisSpec) {
					s.AnalysisRequest.SignalContext.Environment = ""
				},
				false, "environment",
			),
			Entry("missing businessPriority - BR-AI-002",
				"businessPriority is required",
				func(s *aianalysisv1.AIAnalysisSpec) {
					s.AnalysisRequest.SignalContext.BusinessPriority = ""
				},
				false, "businessPriority",
			),
			Entry("missing fingerprint - BR-AI-002",
				"fingerprint is required",
				func(s *aianalysisv1.AIAnalysisSpec) {
					s.AnalysisRequest.SignalContext.Fingerprint = ""
				},
				false, "fingerprint",
			),
			Entry("missing signalType - BR-AI-002",
				"signalType is required",
				func(s *aianalysisv1.AIAnalysisSpec) {
					s.AnalysisRequest.SignalContext.SignalType = ""
				},
				false, "signalType",
			),
			Entry("missing targetResource.kind - BR-AI-002",
				"targetResource.kind is required",
				func(s *aianalysisv1.AIAnalysisSpec) {
					s.AnalysisRequest.SignalContext.TargetResource.Kind = ""
				},
				false, "targetResource.kind",
			),
			Entry("missing targetResource.name - BR-AI-002",
				"targetResource.name is required",
				func(s *aianalysisv1.AIAnalysisSpec) {
					s.AnalysisRequest.SignalContext.TargetResource.Name = ""
				},
				false, "targetResource.name",
			),
			Entry("missing analysisTypes - BR-AI-002",
				"at least one analysis type is required",
				func(s *aianalysisv1.AIAnalysisSpec) {
					s.AnalysisRequest.AnalysisTypes = []string{}
				},
				false, "analysisTypes",
			),

			// Edge Cases: Invalid values
			Entry("environment too long - BR-AI-003",
				"environment exceeds 63 characters",
				func(s *aianalysisv1.AIAnalysisSpec) {
					s.AnalysisRequest.SignalContext.Environment = "this-environment-name-is-way-too-long-and-should-fail-validation-check"
				},
				false, "maximum length",
			),
		)
	})

	// DD-WORKFLOW-001 v2.2: FailedDetections validation
	Describe("ValidateFailedDetections", func() {
		DescribeTable("validates FailedDetections field names",
			func(fields []string, expectValid bool, expectedError string) {
				analysis := validAIAnalysisForTest()
				analysis.Spec.AnalysisRequest.SignalContext.EnrichmentResults.DetectedLabels = &sharedtypes.DetectedLabels{
					FailedDetections: fields,
				}

				err := handler.ValidateFailedDetections(ctx, analysis)

				if expectValid {
					Expect(err).NotTo(HaveOccurred())
				} else {
					Expect(err).To(HaveOccurred())
					Expect(err.Error()).To(ContainSubstring(expectedError))
				}
			},
			Entry("valid fields - gitOpsManaged, pdbProtected",
				[]string{"gitOpsManaged", "pdbProtected"},
				true, "",
			),
			Entry("valid - empty slice",
				[]string{},
				true, "",
			),
			Entry("valid - all known fields",
				[]string{"gitOpsManaged", "pdbProtected", "hpaEnabled", "stateful", "helmManaged", "networkIsolated", "serviceMesh"},
				true, "",
			),
			Entry("invalid - unknown field",
				[]string{"unknownField"},
				false, "invalid field name: unknownField",
			),
			Entry("invalid - mixed valid/invalid",
				[]string{"gitOpsManaged", "badField"},
				false, "invalid field name: badField",
			),
			Entry("invalid - podSecurityLevel (removed in DD-WORKFLOW-001 v2.2)",
				[]string{"podSecurityLevel"},
				false, "invalid field name: podSecurityLevel",
			),
		)

		It("should pass when DetectedLabels is nil", func() {
			analysis := validAIAnalysisForTest()
			analysis.Spec.AnalysisRequest.SignalContext.EnrichmentResults.DetectedLabels = nil

			err := handler.ValidateFailedDetections(ctx, analysis)
			Expect(err).NotTo(HaveOccurred())
		})
	})
})

// ========================================
// TEST HELPERS
// ========================================

// validAIAnalysisForTest creates a fully valid AIAnalysis for testing.
func validAIAnalysisForTest() *aianalysisv1.AIAnalysis {
	return &aianalysisv1.AIAnalysis{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-analysis",
			Namespace: "default",
		},
		Spec: aianalysisv1.AIAnalysisSpec{
			RemediationRequestRef: corev1.ObjectReference{
				Name:      "test-rr",
				Namespace: "default",
			},
			RemediationID: "rem-12345",
			AnalysisRequest: aianalysisv1.AnalysisRequest{
				SignalContext: aianalysisv1.SignalContextInput{
					Fingerprint:      "abc123def456",
					Severity:         "warning",
					SignalType:       "OOMKilled",
					Environment:      "production",
					BusinessPriority: "P1",
					TargetResource: aianalysisv1.TargetResource{
						Kind:      "Pod",
						Name:      "test-pod",
						Namespace: "default",
					},
					EnrichmentResults: sharedtypes.EnrichmentResults{
						KubernetesContext: &sharedtypes.KubernetesContext{
							Namespace: "default",
						},
						DetectedLabels: &sharedtypes.DetectedLabels{
							GitOpsManaged: true,
							PDBProtected:  false,
						},
					},
				},
				AnalysisTypes: []string{"investigation", "root-cause", "workflow-selection"},
			},
		},
	}
}

