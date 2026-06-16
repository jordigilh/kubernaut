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

package fullpipeline

import (
	"context"
	"fmt"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"

	aianalysisv1 "github.com/jordigilh/kubernaut/api/aianalysis/v1alpha1"
	remediationv1 "github.com/jordigilh/kubernaut/api/remediation/v1alpha1"
)

// #1432 / BR-HAPI-200 / E2E-FP-1430-001: Full-pipeline E2E for the no-action journey.
//
// Pipeline (problem_resolved):
//
//	Gateway POST → RR → SP → AA → KA(MockLLM: problem_resolved) → WorkflowNotNeeded → RR NoActionRequired → NO WE
//
// This test validates that when the RCA concludes no action is required
// (problem_resolved outcome), the entire pipeline completes without creating
// a WorkflowExecution. It is the first FP test to use direct Gateway HTTP POST
// signal injection rather than the event-exporter.
var _ = Describe("Problem Resolved No WorkflowExecution [#1432 / BR-HAPI-200]", func() {

	var (
		testNamespace string
		testCtx       context.Context
		testCancel    context.CancelFunc
	)

	BeforeEach(func() {
		testCtx, testCancel = context.WithTimeout(ctx, 5*time.Minute)

		By("Creating managed test namespace")
		testNamespace = fmt.Sprintf("fp-e2e-noa-%d", time.Now().UnixNano())
		ns := &corev1.Namespace{
			ObjectMeta: metav1.ObjectMeta{
				Name: testNamespace,
				Labels: map[string]string{
					"kubernaut.ai/managed": "true",
				},
			},
		}
		Expect(k8sClient.Create(testCtx, ns)).To(Succeed())
		GinkgoWriter.Printf("  Namespace created: %s\n", testNamespace)

		By("Deploying dummy pod for owner resolution")
		pod := &corev1.Pod{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "noa-target-pod",
				Namespace: testNamespace,
				Labels:    map[string]string{"app": "noa-target-pod"},
			},
			Spec: corev1.PodSpec{
				Containers: []corev1.Container{
					{
						Name:    "pause",
						Image:   "registry.k8s.io/pause:3.9",
						Command: []string{"/pause"},
					},
				},
			},
		}
		Expect(k8sClient.Create(testCtx, pod)).To(Succeed())

		Eventually(func() corev1.PodPhase {
			var p corev1.Pod
			if err := apiReader.Get(testCtx, client.ObjectKeyFromObject(pod), &p); err != nil {
				return ""
			}
			return p.Status.Phase
		}, 60*time.Second, 2*time.Second).Should(Equal(corev1.PodRunning),
			"dummy pod should reach Running phase before signal injection")
		GinkgoWriter.Println("  Pod running: noa-target-pod")
	})

	AfterEach(func() {
		if testNamespace != "" {
			By("Cleaning up test namespace")
			ns := &corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: testNamespace}}
			_ = k8sClient.Delete(ctx, ns)
		}
		testCancel()
	})

	// E2E-FP-1430-001: Full pipeline completes with NoActionRequired when RCA is problem_resolved.
	// FedRAMP AU-2: audit trail records the complete lifecycle.
	// FedRAMP SI-4: skip-discovery decision is observable end-to-end.
	It("should complete the pipeline with NoActionRequired and no WorkflowExecution [E2E-FP-1430-001]", func() {
		// Step 1: Inject signal via direct Gateway POST
		By("Step 1: Injecting mock_problem_resolved signal via Gateway")
		rrName := fpPostSignalToGateway("mock_problem_resolved", "noa-target-pod", testNamespace)
		GinkgoWriter.Printf("  RR created: %s\n", rrName)

		// Step 2: Wait for AA to reach Completed with WorkflowNotNeeded
		By("Step 2: Waiting for AIAnalysis to complete with WorkflowNotNeeded")
		Eventually(func() string {
			aaList := &aianalysisv1.AIAnalysisList{}
			if err := apiReader.List(testCtx, aaList, client.InNamespace(namespace)); err != nil {
				return ""
			}
			for _, aa := range aaList.Items {
				if aa.Spec.RemediationRequestRef.Name == rrName {
					GinkgoWriter.Printf("  AA %s: phase=%s reason=%s subReason=%s\n",
						aa.Name, aa.Status.Phase, aa.Status.Reason, aa.Status.SubReason)
					if aa.Status.Phase == aianalysisv1.PhaseCompleted {
						return string(aa.Status.Reason)
					}
				}
			}
			return ""
		}, 3*time.Minute, 2*time.Second).Should(Equal(string(aianalysisv1.ReasonWorkflowNotNeeded)),
			"#1432: AA must complete with WorkflowNotNeeded for problem_resolved signal")

		// Step 3: Verify AA status fields
		By("Step 3: Verifying AA status details")
		aaList := &aianalysisv1.AIAnalysisList{}
		Expect(apiReader.List(testCtx, aaList, client.InNamespace(namespace))).To(Succeed())
		var matchedAA *aianalysisv1.AIAnalysis
		for i := range aaList.Items {
			if aaList.Items[i].Spec.RemediationRequestRef.Name == rrName {
				matchedAA = &aaList.Items[i]
				break
			}
		}
		Expect(matchedAA).NotTo(BeNil(), "AA for RR %s must exist", rrName)
		Expect(matchedAA.Status.SubReason).To(Equal("ProblemResolved"),
			"#1432: AA SubReason must be ProblemResolved")
		Expect(matchedAA.Status.SelectedWorkflow).To(BeNil(),
			"#1432: AA must have no SelectedWorkflow for problem_resolved")
		Expect(matchedAA.Status.NeedsHumanReview).To(BeFalse(),
			"#1432: AA must not need human review for problem_resolved")

		// Step 4: Wait for RR to reach Completed with NoActionRequired
		By("Step 4: Waiting for RR to complete with NoActionRequired")
		Eventually(func() string {
			var rr remediationv1.RemediationRequest
			if err := apiReader.Get(testCtx, client.ObjectKey{Name: rrName, Namespace: namespace}, &rr); err != nil {
				return ""
			}
			GinkgoWriter.Printf("  RR %s: phase=%s outcome=%s\n",
				rr.Name, rr.Status.OverallPhase, rr.Status.Outcome)
			if rr.Status.OverallPhase == remediationv1.PhaseCompleted {
				return rr.Status.Outcome
			}
			return ""
		}, 2*time.Minute, 2*time.Second).Should(Equal("NoActionRequired"),
			"#1432: RR must reach Completed with Outcome=NoActionRequired")

		// Step 5: Assert no WorkflowExecution was created
		By("Step 5: Verifying no WorkflowExecution exists for this RR")
		fpAssertNoWEForRR(rrName)
		GinkgoWriter.Println("  Confirmed: no WorkflowExecution created (no-action path)")
	})
})
