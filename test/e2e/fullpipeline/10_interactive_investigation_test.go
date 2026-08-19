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
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"

	aianalysisv1 "github.com/jordigilh/kubernaut/api/aianalysis/v1alpha1"
	isv1alpha1 "github.com/jordigilh/kubernaut/api/investigationsession/v1alpha1"
	"github.com/jordigilh/kubernaut/test/infrastructure"
)

// BR-INTERACTIVE-010: E2E tests for Interactive Investigation Architecture (#1293).
// These tests exercise the IS CRD as the universal interactive signal across the
// full pipeline (AA → KA → DS) in a Kind cluster with all services deployed.
var _ = Describe("E2E-1293: Interactive Investigation Architecture", Label("e2e", "fullpipeline", "interactive", "1293"), func() {

	// waitForAAInvestigating polls until an AIAnalysis for the given RR reaches
	// Investigating or a later phase (Analyzing, Completed). With mock-LLM, the
	// investigation can complete faster than the polling interval, so we accept
	// any phase that implies Investigating was reached.
	waitForAAInvestigating := func(rrName string) string {
		var aaName string
		Eventually(func() bool {
			aaList := &aianalysisv1.AIAnalysisList{}
			if err := apiReader.List(ctx, aaList, client.InNamespace(namespace)); err != nil {
				return false
			}
			for _, aa := range aaList.Items {
				if aa.Spec.RemediationRequestRef.Name == rrName {
					aaName = aa.Name
					phase := aa.Status.Phase
					return phase == aianalysisv1.PhaseInvestigating || phase == aianalysisv1.PhaseAnalyzing || phase == aianalysisv1.PhaseCompleted
				}
			}
			return false
		}, timeout, 1*time.Second).Should(BeTrue(), "AIAnalysis should reach Investigating for RR %s", rrName)
		return aaName
	}

	// createISForRR creates an Active InvestigationSession CRD referencing the given RR.
	createISForRR := func(testID, rrName string) *isv1alpha1.InvestigationSession {
		is := &isv1alpha1.InvestigationSession{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "is-" + testID,
				Namespace: namespace,
			},
			Spec: isv1alpha1.InvestigationSessionSpec{
				RemediationRequestRef: isv1alpha1.ObjectRef{
					Name:      rrName,
					Namespace: namespace,
				},
				A2ATaskID: "task-" + testID,
				UserIdentity: isv1alpha1.SessionUser{
					Username: "sre@kubernaut.ai",
					Groups:   []string{"sre"},
				},
				JoinMode: isv1alpha1.SessionJoinModeStart,
			},
		}
		Expect(k8sClient.Create(ctx, is)).To(Succeed())

		is.Status.Phase = isv1alpha1.SessionPhaseActive
		Expect(k8sClient.Status().Update(ctx, is)).To(Succeed())

		Eventually(func() isv1alpha1.SessionPhase {
			var updated isv1alpha1.InvestigationSession
			if err := apiReader.Get(ctx, client.ObjectKeyFromObject(is), &updated); err != nil {
				return ""
			}
			return updated.Status.Phase
		}, 30*time.Second, 1*time.Second).Should(Equal(isv1alpha1.SessionPhaseActive))

		return is
	}

	It("[E2E-1293-001] Interactive from start: IS created → KA session pending → MCP start → user_driving", func() {
		By("Setting up MCP session (pre-RR creation to minimize race window)")
		setup, err := infrastructure.SetupMCPSession(ctx, namespace, "e2e-1293-001-sa", kubeconfigPath, GinkgoWriter)
		Expect(err).NotTo(HaveOccurred())
		defer setup.Cleanup()

		By("Creating direct RR to trigger pipeline")
		rrName, err := infrastructure.CreateDirectRR(ctx, namespace, "e2e-1293-001")
		Expect(err).NotTo(HaveOccurred())

		By("Creating Active IS CRD immediately (interactive from start — IS exists before AA investigates)")
		is := createISForRR("1293-001", rrName)
		DeferCleanup(func() {
			_ = client.IgnoreNotFound(k8sClient.Delete(ctx, is))
		})

		By("Waiting for AIAnalysis to reach Investigating phase with interactive=true")
		aaName := waitForAAInvestigating(rrName)
		aa := &aianalysisv1.AIAnalysis{}
		Eventually(func(g Gomega) {
			g.Expect(apiReader.Get(ctx, client.ObjectKey{Name: aaName, Namespace: namespace}, aa)).To(Succeed())
			g.Expect(aa.Status.KASession).NotTo(BeNil())
			g.Expect(aa.Status.KASession.Interactive).To(BeTrue(),
				"BR-INTERACTIVE-010 SC-1: AA should set interactive=true when IS exists")
		}, 90*time.Second, 2*time.Second).Should(Succeed())

		By("Using MCP takeover to start the interactive investigation")
		callCtx, callCancel := context.WithTimeout(ctx, 30*time.Second)
		defer callCancel()
		result, err := infrastructure.CallInvestigate(callCtx, setup.Session, map[string]any{
			"rr_id":  rrName,
			"action": "takeover",
		})
		Expect(err).NotTo(HaveOccurred())
		Expect(result.IsError).To(BeFalse(), "takeover should succeed")

		By("Verifying InteractiveSession is populated on AA status")
		Eventually(func(g Gomega) {
			g.Expect(apiReader.Get(ctx, client.ObjectKey{Name: aaName, Namespace: namespace}, aa)).To(Succeed())
			g.Expect(aa.Status.InteractiveSession).NotTo(BeNil(),
				"BR-INTERACTIVE-010: InteractiveSession must appear after takeover")
			g.Expect(aa.Status.InteractiveSession.ActingUser).NotTo(BeEmpty())
		}, 90*time.Second, 2*time.Second).Should(Succeed())
	})

	It("[E2E-1293-002] Dynamic takeover: autonomous → interactive when IS appears mid-investigation", func() {
		By("Setting up MCP session pre-RR")
		setup, err := infrastructure.SetupMCPSession(ctx, namespace, "e2e-1293-002-sa", kubeconfigPath, GinkgoWriter)
		Expect(err).NotTo(HaveOccurred())
		defer setup.Cleanup()

		By("Creating direct RR — let AA start autonomous investigation")
		rrName, err := infrastructure.CreateDirectRR(ctx, namespace, "e2e-1293-002")
		Expect(err).NotTo(HaveOccurred())

		By("Creating Active IS CRD immediately (before KA completes — mock-LLM responds instantly)")
		is := createISForRR("1293-002", rrName)
		DeferCleanup(func() {
			_ = client.IgnoreNotFound(k8sClient.Delete(ctx, is))
		})

		By("Waiting for AA to reach Investigating with interactive=true")
		aaName := waitForAAInvestigating(rrName)
		aa := &aianalysisv1.AIAnalysis{}
		Eventually(func(g Gomega) {
			g.Expect(apiReader.Get(ctx, client.ObjectKey{Name: aaName, Namespace: namespace}, aa)).To(Succeed())
			g.Expect(aa.Status.KASession).NotTo(BeNil())
			g.Expect(aa.Status.KASession.Interactive).To(BeTrue(),
				"BR-INTERACTIVE-010 SC-1: re-submitted session should have interactive=true")
		}, 90*time.Second, 2*time.Second).Should(Succeed())

		By("Performing MCP takeover")
		callCtx, callCancel := context.WithTimeout(ctx, 30*time.Second)
		defer callCancel()
		result, err := infrastructure.CallInvestigate(callCtx, setup.Session, map[string]any{
			"rr_id":  rrName,
			"action": "takeover",
		})
		Expect(err).NotTo(HaveOccurred())
		Expect(result.IsError).To(BeFalse(), "takeover should succeed")
	})

	// [E2E-1293-003] "IS deletion cancels investigation" was retired
	// (DD-AA-KA-001 Amendment Gap 4, 2026-08-19): this decision deliberately
	// narrowed InvestigationSession to AF-side write-only bookkeeping with
	// no causal effect on KA/AA control flow (see "IS interactive
	// detection" note earlier in this decision), so deleting it can no
	// longer cancel anything by design -- the scenario this test proved no
	// longer exists as a real behavior. The actual user-facing cancel
	// journey (MCP kubernaut_investigate action=cancel on a taken-over
	// session) already has E2E coverage elsewhere, e.g.
	// test/e2e/kubernautagent/interactive_coverage_e2e_test.go, without
	// touching IS at all. Orphaned-investigation cleanup on
	// AgentSession/RemediationRequest deletion (the new capability this
	// same Gap 4 amendment adds) is proven at the dispatcher-wiring level
	// instead (internal/kubernautagent/agentsession/dispatcher_test.go's
	// UT-AA-2170-DELETE-001/TIMEOUT-001/002) -- RR-cascade deletion removes
	// AIAnalysis itself, leaving no AA Status left to assert on from an E2E
	// journey's perspective.

	It("[E2E-1293-006] Context reconstruction: takeover pre-loads prior session context", func() {
		By("Setting up MCP session pre-RR")
		setup, err := infrastructure.SetupMCPSession(ctx, namespace, "e2e-1293-006-sa", kubeconfigPath, GinkgoWriter)
		Expect(err).NotTo(HaveOccurred())
		defer setup.Cleanup()

		By("Creating direct RR to trigger autonomous investigation")
		rrName, err := infrastructure.CreateDirectRR(ctx, namespace, "e2e-1293-006")
		Expect(err).NotTo(HaveOccurred())

		By("Waiting for AA to reach Investigating with a KA session that has polled at least once")
		aaName := waitForAAInvestigating(rrName)
		aa := &aianalysisv1.AIAnalysis{}
		Eventually(func(g Gomega) {
			g.Expect(apiReader.Get(ctx, client.ObjectKey{Name: aaName, Namespace: namespace}, aa)).To(Succeed())
			g.Expect(aa.Status.KASession).NotTo(BeNil())
			g.Expect(aa.Status.KASession.ID).NotTo(BeEmpty())
			g.Expect(aa.Status.KASession.PollCount).To(BeNumerically(">=", int32(1)),
				"autonomous investigation must have completed at least one poll cycle to persist turns for reconstruction")
		}, 60*time.Second, 1*time.Second).Should(Succeed())

		By("Creating Active IS CRD to trigger dynamic takeover (autonomous → interactive)")
		is := createISForRR("1293-006", rrName)
		DeferCleanup(func() {
			_ = client.IgnoreNotFound(k8sClient.Delete(ctx, is))
		})

		By("Performing MCP takeover and verifying context reconstruction")
		callCtx, callCancel := context.WithTimeout(ctx, 30*time.Second)
		defer callCancel()
		result, err := infrastructure.CallInvestigate(callCtx, setup.Session, map[string]any{
			"rr_id":  rrName,
			"action": "takeover",
		})
		Expect(err).NotTo(HaveOccurred())
		Expect(result.IsError).To(BeFalse(), "takeover should succeed")

		takeoverText := infrastructure.ExtractToolResultText(result)
		GinkgoWriter.Printf("  Takeover response: %s\n", takeoverText)
		Expect(takeoverText).To(MatchRegexp(`[1-9]\d* prior turns reconstructed`),
			"BR-INTERACTIVE-010 SC-3: takeover should reconstruct at least 1 prior turn")

		By("Sending message to verify context is usable by the LLM")
		msgCtx, msgCancel := context.WithTimeout(ctx, 60*time.Second)
		defer msgCancel()
		result, err = infrastructure.CallInvestigate(msgCtx, setup.Session, map[string]any{
			"rr_id":   rrName,
			"action":  "message",
			"message": "Based on the prior investigation, what was the root cause?",
		})
		Expect(err).NotTo(HaveOccurred())
		Expect(result.IsError).To(BeFalse(), "message should succeed after context reconstruction")

		msgText := infrastructure.ExtractToolResultText(result)
		Expect(msgText).NotTo(BeEmpty(),
			"LLM should respond using reconstructed context from prior session")
		GinkgoWriter.Printf("  Message response (first 200 chars): %.200s\n", msgText)
	})
})
