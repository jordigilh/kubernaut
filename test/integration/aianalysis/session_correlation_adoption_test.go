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
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"
	k8sretry "k8s.io/client-go/util/retry"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"

	aianalysisv1 "github.com/jordigilh/kubernaut/api/aianalysis/v1alpha1"
	isv1alpha1 "github.com/jordigilh/kubernaut/api/investigationsession/v1alpha1"
	aiaudit "github.com/jordigilh/kubernaut/pkg/aianalysis/audit"
	"github.com/jordigilh/kubernaut/pkg/aianalysis/handlers"
	"github.com/jordigilh/kubernaut/pkg/shared/events"
	"github.com/jordigilh/kubernaut/test/shared/helpers"
)

// Issue #2030 (main-tracking clone of #2029) Part B / FedRAMP SI-4: proves
// end-to-end that AA adopts a KA session AF has correlated onto this RR's
// InvestigationSession (e.g. after a reconnect/takeover) instead of continuing
// to poll — and potentially finalize as expired/terminal — its own now-stale
// session. Also proves the widened ISEventPredicate (see
// internal/controller/aianalysis/aianalysis_controller.go) wakes the
// reconciler promptly on a KACorrelationID-only change, rather than waiting
// for the next scheduled poll.
var _ = Describe("BR-INTERACTIVE-010: #2030 Part B session correlation adoption", Label("integration", "interactive"), func() {
	const (
		timeout  = 15 * time.Second
		interval = 200 * time.Millisecond
	)

	Context("AA adopts a live KA session correlated onto IS after takeover (SI-4)", Serial, func() {
		var savedHandler *handlers.InvestigatingHandler

		BeforeEach(func() {
			savedHandler = reconciler.InvestigatingHandler.Load()
			auditClient := aiaudit.NewAuditClient(auditStore, ctrl.Log.WithName("adoption-test-audit"))
			isChecker := handlers.NewK8sInvestigationSessionChecker(k8sClient, testNamespace)
			reconciler.InvestigatingHandler.Store(handlers.NewInvestigatingHandler(
				realAgentClient,
				ctrl.Log.WithName("adoption-real-handler"),
				testMetrics,
				auditClient,
				handlers.WithSessionMode(),
				// Deliberately long poll interval: adoption must be observed well
				// before this would ever fire, proving the widened predicate (not
				// the poll timer) is what woke the reconciler.
				handlers.WithSessionPollInterval(30*time.Second),
				handlers.WithInvestigationSessionChecker(isChecker),
				handlers.WithRecorder(k8sManager.GetEventRecorderFor("aianalysis-controller")),
			))
		})

		AfterEach(func() {
			if savedHandler != nil {
				reconciler.InvestigatingHandler.Store(savedHandler)
			}
		})

		It("IT-AA-2030-011: adopts a newly-correlated KA session instead of finalizing the stale one, waking promptly via the widened predicate", func() {
			rrName := helpers.UniqueTestName("rr-2030-adopt")
			isName := helpers.UniqueTestName("is-2030-adopt")
			aaName := helpers.UniqueTestName("aa-2030-adopt")

			By("creating Active IS for the RR")
			createActiveIS(isName, rrName)

			By("creating Investigating AA with a real interactive KA session (the 'old', now-stale session)")
			analysis := createInvestigatingAA(aaName, rrName, "slow-investigation-test", true)

			By("waiting for the old KA session to be established")
			var oldSessionID string
			Eventually(func(g Gomega) {
				g.Expect(k8sClient.Get(ctx, client.ObjectKeyFromObject(analysis), analysis)).To(Succeed())
				g.Expect(analysis.Status.KASession).NotTo(BeNil())
				g.Expect(analysis.Status.KASession.ID).NotTo(BeEmpty())
				oldSessionID = analysis.Status.KASession.ID
			}, timeout, interval).Should(Succeed())

			By("minting a second, real KA session directly (simulates the session AF creates on takeover)")
			builder := handlers.NewRequestBuilder(ctrl.Log.WithName("adoption-test-builder"))
			req := builder.BuildIncidentRequest(analysis)
			newSessionID, err := realAgentClient.SubmitInvestigation(ctx, req)
			Expect(err).NotTo(HaveOccurred())
			Expect(newSessionID).NotTo(BeEmpty())
			Expect(newSessionID).NotTo(Equal(oldSessionID))

			By("correlating the new session onto the IS CRD (simulating AF's UpdateISCorrelation after takeover)")
			Expect(k8sretry.RetryOnConflict(k8sretry.DefaultRetry, func() error {
				var is isv1alpha1.InvestigationSession
				if err := k8sClient.Get(ctx, client.ObjectKey{Name: isName, Namespace: testNamespace}, &is); err != nil {
					return err
				}
				is.Status.KACorrelationID = newSessionID
				return k8sClient.Status().Update(ctx, &is)
			})).To(Succeed())

			// #2030 Part B / SI-4 predicate-wake-up proof: the handler's poll
			// interval is 30s (see BeforeEach), so observing the session-ID
			// switch within this 5s window is only possible if the widened
			// ISEventPredicate woke the reconciler on the KACorrelationID
			// change — the poll timer alone could not have fired yet.
			By("verifying AA adopts the new session within 5s — well before the 30s poll interval — proving predicate-driven wake-up")
			Eventually(func(g Gomega) {
				g.Expect(k8sClient.Get(ctx, client.ObjectKeyFromObject(analysis), analysis)).To(Succeed())
				g.Expect(analysis.Status.KASession).NotTo(BeNil())
				g.Expect(analysis.Status.KASession.ID).To(Equal(newSessionID),
					"#2030 Part B / SI-4: AA must promptly adopt the correlated session instead of continuing to track the stale one")
			}, 5*time.Second, 200*time.Millisecond).Should(Succeed())

			By("verifying AA was never finalized as terminal for the abandoned old session")
			Expect(analysis.Status.Phase).NotTo(Equal(aianalysisv1.PhaseFailed),
				"the stale session must never be finalized (e.g. as expired) once adoption occurred")

			By("verifying the SessionAdopted K8s event was recorded (FedRAMP SI-4 observability)")
			Eventually(func(g Gomega) {
				var evtList corev1.EventList
				g.Expect(k8sClient.List(ctx, &evtList, client.InNamespace(testNamespace))).To(Succeed())
				found := false
				for i := range evtList.Items {
					e := &evtList.Items[i]
					if e.InvolvedObject.Name == aaName && e.Reason == events.EventReasonSessionAdopted {
						found = true
						break
					}
				}
				g.Expect(found).To(BeTrue(), "adoptCorrelatedSession must emit a SessionAdopted event")
			}, timeout, interval).Should(Succeed())
		})
	})
})
