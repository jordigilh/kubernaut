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

package remediation

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	remediationv1alpha1 "github.com/jordigilh/kubernaut/api/remediation/v1alpha1"
	remediationctrl "github.com/jordigilh/kubernaut/internal/controller/remediation"
)

// Business Requirement: BR-ORCHESTRATION-004 (Failure Handling)
// Unit tests for failure detection and handling
var _ = Describe("RemediationRequest Controller - Failure Handling", func() {
	var (
		scheme     *runtime.Scheme
		reconciler *remediationctrl.RemediationRequestReconciler
	)

	BeforeEach(func() {
		scheme = runtime.NewScheme()
		Expect(remediationv1alpha1.AddToScheme(scheme)).To(Succeed())

		k8sClient := fake.NewClientBuilder().WithScheme(scheme).Build()
		reconciler = &remediationctrl.RemediationRequestReconciler{
			Client: k8sClient,
			Scheme: scheme,
		}
	})

	Describe("IsPhaseInFailedState", func() {
		DescribeTable("child CRD failure detection",
			func(childPhase string, expectedFailed bool) {
				Expect(reconciler.IsPhaseInFailedState(childPhase)).To(Equal(expectedFailed))
			},
			// RemediationProcessing failures
			Entry("RemediationProcessing: failed", "failed", true),
			Entry("RemediationProcessing: completed (success)", "completed", false),
			Entry("RemediationProcessing: enriching (in-progress)", "enriching", false),

			// AIAnalysis failures
			Entry("AIAnalysis: Failed (capitalized)", "Failed", true),
			Entry("AIAnalysis: Completed (success)", "Completed", false),
			Entry("AIAnalysis: Investigating (in-progress)", "Investigating", false),

			// WorkflowExecution failures
			Entry("WorkflowExecution: failed", "failed", true),
			Entry("WorkflowExecution: completed (success)", "completed", false),
			Entry("WorkflowExecution: executing (in-progress)", "executing", false),

			// Edge cases
			Entry("Unknown phase: unknown-state", "unknown-state", false),
			Entry("Empty phase", "", false),
		)
	})

	Describe("BuildFailureReason", func() {
		It("should build descriptive failure reason for RemediationProcessing", func() {
			reason := reconciler.BuildFailureReason("processing", "RemediationProcessing", "Enrichment timeout")
			Expect(reason).To(ContainSubstring("processing"))
			Expect(reason).To(ContainSubstring("RemediationProcessing"))
		})

		It("should build descriptive failure reason for AIAnalysis", func() {
			reason := reconciler.BuildFailureReason("analyzing", "AIAnalysis", "LLM API unavailable")
			Expect(reason).To(ContainSubstring("analyzing"))
			Expect(reason).To(ContainSubstring("AIAnalysis"))
		})

		It("should build descriptive failure reason for WorkflowExecution", func() {
			reason := reconciler.BuildFailureReason("executing", "WorkflowExecution", "Step 3 failed")
			Expect(reason).To(ContainSubstring("executing"))
			Expect(reason).To(ContainSubstring("WorkflowExecution"))
		})

		It("should handle empty error message gracefully", func() {
			reason := reconciler.BuildFailureReason("processing", "RemediationProcessing", "")
			Expect(reason).NotTo(BeEmpty())
			Expect(reason).To(ContainSubstring("processing"))
		})
	})

	Describe("ShouldTransitionToFailed", func() {
		DescribeTable("failure state transition decision",
			func(overallPhase string, childFailed bool, expectedTransition bool) {
				remediation := &remediationv1alpha1.RemediationRequest{
					Status: remediationv1alpha1.RemediationRequestStatus{
						OverallPhase: overallPhase,
					},
				}

				Expect(reconciler.ShouldTransitionToFailed(remediation, childFailed)).To(Equal(expectedTransition))
			},
			// Normal cases - should transition
			Entry("processing phase + child failed → transition", "processing", true, true),
			Entry("analyzing phase + child failed → transition", "analyzing", true, true),
			Entry("executing phase + child failed → transition", "executing", true, true),

			// Normal cases - should NOT transition
			Entry("processing phase + child succeeded → no transition", "processing", false, false),
			Entry("analyzing phase + child succeeded → no transition", "analyzing", false, false),

			// Terminal states - should NOT transition
			Entry("completed phase + child failed → no transition (already terminal)", "completed", true, false),
			Entry("failed phase + child failed → no transition (already terminal)", "failed", true, false),

			// Edge cases
			Entry("pending phase + child failed → no transition (no child yet)", "pending", true, false),
			Entry("empty phase + child failed → no transition", "", true, false),
		)
	})
})
