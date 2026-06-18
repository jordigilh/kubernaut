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
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"sigs.k8s.io/controller-runtime/pkg/client"

	aianalysisv1 "github.com/jordigilh/kubernaut/api/aianalysis/v1alpha1"
	remediationv1 "github.com/jordigilh/kubernaut/api/remediation/v1alpha1"
	"github.com/jordigilh/kubernaut/test/infrastructure"
)

// #1456 / E2E-FP-1456-001: Full-pipeline E2E for the escalation journey.
//
// Pipeline (operator escalation):
//
//	CreateDirectRR → MCP start → kubernaut_complete_no_action(escalation_reason) →
//	KA marks session completed (operator_escalation) → IS→Completed →
//	ISEventPredicate wakes AA → AA Phase=Failed/OperatorEscalation →
//	RR Outcome=ManualReviewRequired → NO WE
//
// Proves the fix for #1449/#1456: CRD enum + IS watch predicate enable the
// full escalation lifecycle to complete without getting stuck in Investigating.
var _ = Describe("Escalation Lifecycle [#1456 / FedRAMP IR-5, SI-4]", Label("e2e", "fullpipeline", "interactive", "mcp", "escalation"), func() {

	It("E2E-FP-1456-001: operator escalation completes pipeline with ManualReviewRequired", func() {
		testCtx, testCancel := context.WithTimeout(ctx, 3*time.Minute)
		defer testCancel()

		By("Step 1: Creating direct RR with slow signal (keeps KA session alive for MCP escalation)")
		rrName, err := infrastructure.CreateDirectRRWithSignal(testCtx, namespace, "fp-e2e-1456", "slow-investigation-test")
		Expect(err).NotTo(HaveOccurred())
		GinkgoWriter.Printf("  RR created: %s\n", rrName)

		By("Step 2: Setting up MCP session")
		setup, err := infrastructure.SetupMCPSession(testCtx, namespace, "fp-e2e-1456-sa", kubeconfigPath, GinkgoWriter)
		Expect(err).NotTo(HaveOccurred())
		defer setup.Cleanup()

		By("Step 3: Starting interactive investigation")
		startCtx, startCancel := context.WithTimeout(testCtx, 30*time.Second)
		defer startCancel()
		result, err := infrastructure.CallInvestigate(startCtx, setup.Session, map[string]any{
			"rr_id":  rrName,
			"action": "start",
		})
		Expect(err).NotTo(HaveOccurred())
		Expect(result.IsError).To(BeFalse(), "start should succeed")
		GinkgoWriter.Printf("  Investigation started: %s\n", infrastructure.ExtractToolResultText(result))

		By("Step 4: Calling kubernaut_complete_no_action with escalation_reason")
		cnaCtx, cnaCancel := context.WithTimeout(testCtx, 30*time.Second)
		defer cnaCancel()
		result, err = infrastructure.CallCompleteNoAction(cnaCtx, setup.Session, map[string]any{
			"rr_id":             rrName,
			"reason":            "Critical infrastructure alert requires SRE team review",
			"escalation_reason": "Operator escalation: production database replication lag exceeds SLA",
		})
		Expect(err).NotTo(HaveOccurred())
		Expect(result).NotTo(BeNil())
		Expect(result.IsError).To(BeFalse(), "complete_no_action should succeed")
		Expect(infrastructure.ExtractToolResultText(result)).To(ContainSubstring("escalated"),
			"#1456: MCP tool must return escalated status")
		GinkgoWriter.Printf("  Escalation confirmed: %s\n", infrastructure.ExtractToolResultText(result))

		By("Step 5: Waiting for AIAnalysis to reach terminal state (Failed/OperatorEscalation)")
		Eventually(func(g Gomega) {
			aaList := &aianalysisv1.AIAnalysisList{}
			g.Expect(apiReader.List(testCtx, aaList, client.InNamespace(namespace))).To(Succeed())
			for _, aa := range aaList.Items {
				if aa.Spec.RemediationRequestRef.Name == rrName {
					GinkgoWriter.Printf("  AA %s: phase=%s reason=%s subReason=%s humanReview=%v\n",
						aa.Name, aa.Status.Phase, aa.Status.Reason, aa.Status.SubReason, aa.Status.NeedsHumanReview)
					g.Expect(aa.Status.Phase).To(Equal(aianalysisv1.PhaseFailed),
						"#1456: AA must reach Failed phase after escalation (not stuck in Investigating)")
					g.Expect(aa.Status.NeedsHumanReview).To(BeTrue(),
						"IR-5: NeedsHumanReview must be true for escalation routing")
					g.Expect(aa.Status.HumanReviewReason).To(Equal("operator_escalation"),
						"IR-5: HumanReviewReason must be operator_escalation")
					g.Expect(aa.Status.SubReason).To(Equal("OperatorEscalation"),
						"AU-12: SubReason must map to OperatorEscalation for structured audit")
					return
				}
			}
			g.Expect(false).To(BeTrue(), "AA for RR %s not found", rrName)
		}, 2*time.Minute, 2*time.Second).Should(Succeed())

		By("Step 6: Waiting for RR to reach Completed with ManualReviewRequired")
		Eventually(func(g Gomega) {
			var rr remediationv1.RemediationRequest
			g.Expect(apiReader.Get(testCtx, client.ObjectKey{Name: rrName, Namespace: namespace}, &rr)).To(Succeed())
			GinkgoWriter.Printf("  RR %s: phase=%s outcome=%s\n",
				rr.Name, rr.Status.OverallPhase, rr.Status.Outcome)
			g.Expect(rr.Status.OverallPhase).To(Equal(remediationv1.PhaseCompleted),
				"#1456: RR must reach terminal Completed phase")
			g.Expect(rr.Status.Outcome).To(Equal("ManualReviewRequired"),
				"#1456: RR outcome must be ManualReviewRequired for operator escalation")
		}, 2*time.Minute, 2*time.Second).Should(Succeed())

		By("Step 7: Verifying no WorkflowExecution was created")
		fpAssertNoWEForRR(rrName)
		GinkgoWriter.Println("  Confirmed: no WorkflowExecution created (escalation path)")
	})
})
