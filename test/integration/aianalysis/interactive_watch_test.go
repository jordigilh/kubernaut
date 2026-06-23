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
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	k8sretry "k8s.io/client-go/util/retry"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"

	aianalysisv1 "github.com/jordigilh/kubernaut/api/aianalysis/v1alpha1"
	isv1alpha1 "github.com/jordigilh/kubernaut/api/investigationsession/v1alpha1"
	aiaudit "github.com/jordigilh/kubernaut/pkg/aianalysis/audit"
	"github.com/jordigilh/kubernaut/pkg/aianalysis/handlers"
	ogenclient "github.com/jordigilh/kubernaut/pkg/datastorage/ogen-client"
	sharedtypes "github.com/jordigilh/kubernaut/pkg/shared/types"
	"github.com/jordigilh/kubernaut/test/shared/helpers"
)

// BR-INTERACTIVE-010: Integration tests for InvestigationSession watch wiring (#1293).
var _ = Describe("BR-INTERACTIVE-010: InvestigationSession Watch Integration", Label("integration", "interactive"), func() {
	const (
		timeout  = 15 * time.Second
		interval = 200 * time.Millisecond
	)

	createActiveIS := func(name, rrName string) *isv1alpha1.InvestigationSession {
		is := &isv1alpha1.InvestigationSession{
			ObjectMeta: metav1.ObjectMeta{
				Name:      name,
				Namespace: testNamespace,
			},
			Spec: isv1alpha1.InvestigationSessionSpec{
				RemediationRequestRef: isv1alpha1.ObjectRef{
					Name:      rrName,
					Namespace: testNamespace,
				},
				A2ATaskID: "task-" + name,
				UserIdentity: isv1alpha1.SessionUser{
					Username: "integration-test-user",
				},
				JoinMode: isv1alpha1.SessionJoinModeStart,
			},
		}
		Expect(k8sClient.Create(ctx, is)).To(Succeed())
		is.Status.Phase = isv1alpha1.SessionPhaseActive
		Expect(k8sClient.Status().Update(ctx, is)).To(Succeed())

		// Wait for Active phase to be visible in the cache — prevents race where
		// IS watch fires on Create but HasActiveSession sees non-Active phase.
		Eventually(func() isv1alpha1.SessionPhase {
			var updated isv1alpha1.InvestigationSession
			if err := k8sClient.Get(ctx, client.ObjectKeyFromObject(is), &updated); err != nil {
				return ""
			}
			return updated.Status.Phase
		}, timeout, interval).Should(Equal(isv1alpha1.SessionPhaseActive))

		return is
	}

	createInvestigatingAA := func(name, rrName, sessionID, signalName string, interactive bool) *aianalysisv1.AIAnalysis {
		if signalName == "" {
			signalName = "CrashLoopBackOff"
		}
		analysis := &aianalysisv1.AIAnalysis{
			ObjectMeta: metav1.ObjectMeta{
				Name:      name,
				Namespace: testNamespace,
			},
			Spec: aianalysisv1.AIAnalysisSpec{
				RemediationRequestRef: corev1.ObjectReference{
					Name:      rrName,
					Namespace: testNamespace,
				},
				RemediationID: rrName,
				AnalysisRequest: aianalysisv1.AnalysisRequest{
					SignalContext: aianalysisv1.SignalContextInput{
						Fingerprint:      "fp-interactive",
						Severity:         "warning",
						SignalName:       signalName,
						Environment:      "staging",
						BusinessPriority: "P2",
						TargetResource: aianalysisv1.TargetResource{
							Kind:      "Pod",
							Name:      "test-pod",
							Namespace: testNamespace,
						},
						EnrichmentResults: sharedtypes.EnrichmentResults{},
					},
					AnalysisTypes: []aianalysisv1.AnalysisType{aianalysisv1.AnalysisTypeInvestigation},
				},
			},
		}
		Expect(k8sClient.Create(ctx, analysis)).To(Succeed())

		// RetryOnConflict: the controller races to set Phase=Pending on new AAs,
		// which bumps the resource version. Same pattern as crd_lifecycle.go helpers.
		Expect(k8sretry.RetryOnConflict(k8sretry.DefaultRetry, func() error {
			if err := k8sClient.Get(ctx, client.ObjectKeyFromObject(analysis), analysis); err != nil {
				return err
			}
			now := metav1.Now()
			analysis.Status.Phase = aianalysisv1.PhaseInvestigating
			analysis.Status.KASession = &aianalysisv1.KASession{
				ID:          sessionID,
				Interactive: interactive,
				CreatedAt:   &now,
			}
			return k8sClient.Status().Update(ctx, analysis)
		})).To(Succeed())
		return analysis
	}

	Context("IT-AA-1293-001: Field index returns IS by RR name", func() {
		It("should list InvestigationSession using spec.remediationRequestRef.name field index", func() {
			rrName := "rr-test-001"
			isName := helpers.UniqueTestName("is-field-index")
			createActiveIS(isName, rrName)

			var list isv1alpha1.InvestigationSessionList
			Expect(k8sClient.List(ctx, &list,
				client.InNamespace(testNamespace),
				client.MatchingFields{handlers.ISFieldIndexRRName: rrName},
			)).To(Succeed())

			Expect(list.Items).To(HaveLen(1))
			Expect(list.Items[0].Name).To(Equal(isName))
			Expect(list.Items[0].Spec.RemediationRequestRef.Name).To(Equal(rrName))
		})
	})

	// IT-002..004 use the real KA agent client + slow-investigation-test scenario
	// to verify IS watch triggers and upgrade behavior through CRD state assertions.
	Context("IS watch-driven reconciliation", Serial, func() {
		var savedHandler *handlers.InvestigatingHandler

		BeforeEach(func() {
			savedHandler = reconciler.InvestigatingHandler.Load()
			auditClient := aiaudit.NewAuditClient(auditStore, ctrl.Log.WithName("interactive-test-audit"))
			isChecker := handlers.NewK8sInvestigationSessionChecker(k8sClient, testNamespace)
			reconciler.InvestigatingHandler.Store(handlers.NewInvestigatingHandler(
				realAgentClient,
				ctrl.Log.WithName("interactive-real-handler"),
				testMetrics,
				auditClient,
				handlers.WithSessionMode(),
				handlers.WithSessionPollInterval(500*time.Millisecond),
				handlers.WithInvestigationSessionChecker(isChecker),
				handlers.WithRecorder(k8sManager.GetEventRecorderFor("aianalysis-controller")),
			))
		})

		AfterEach(func() {
			if savedHandler != nil {
				reconciler.InvestigatingHandler.Store(savedHandler)
			}
		})

		It("IT-AA-1293-002: should upgrade autonomous session in-place when Active IS is created (#1390)", func() {
			rrName := helpers.UniqueTestName("rr-watch-create")
			analysisName := helpers.UniqueTestName("aa-watch-create")

			analysis := createInvestigatingAA(analysisName, rrName, "", "slow-investigation-test", false)

			By("waiting for real KA session to be established (KASession.ID set)")
			Eventually(func(g Gomega) {
				g.Expect(k8sClient.Get(ctx, client.ObjectKeyFromObject(analysis), analysis)).To(Succeed())
				g.Expect(analysis.Status.KASession).NotTo(BeNil())
				g.Expect(analysis.Status.KASession.ID).NotTo(BeEmpty(),
					"KASession.ID must be set after successful SubmitInvestigation to real KA")
			}, timeout, interval).Should(Succeed())

			sessionIDBefore := analysis.Status.KASession.ID

			By("creating Active InvestigationSession for the same RR")
			createActiveIS(helpers.UniqueTestName("is-watch-create"), rrName)

			By("verifying IS watch triggered upgrade: Interactive=true, session ID preserved")
			Eventually(func(g Gomega) {
				g.Expect(k8sClient.Get(ctx, client.ObjectKeyFromObject(analysis), analysis)).To(Succeed())
				g.Expect(analysis.Status.KASession).NotTo(BeNil())
				g.Expect(analysis.Status.KASession.Interactive).To(BeTrue(),
					"Interactive flag must be set by upgrade path")
				g.Expect(analysis.Status.KASession.ID).To(Equal(sessionIDBefore),
					"session ID must be preserved — upgrade in-place, no cancel")
			}, timeout, interval).Should(Succeed())
		})

		It("IT-AA-1293-003: should fail with ReasonInteractiveCancelled when IS is deleted", func() {
			rrName := helpers.UniqueTestName("rr-watch-delete")
			analysisName := helpers.UniqueTestName("aa-watch-delete")

			By("creating Active IS first so controller sees it on first AA reconcile")
			isName := helpers.UniqueTestName("is-watch-delete")
			createActiveIS(isName, rrName)

			analysis := createInvestigatingAA(analysisName, rrName, "", "slow-investigation-test", true)

			By("waiting for real KA session to be established")
			Eventually(func(g Gomega) {
				g.Expect(k8sClient.Get(ctx, client.ObjectKeyFromObject(analysis), analysis)).To(Succeed())
				g.Expect(analysis.Status.KASession).NotTo(BeNil())
				g.Expect(analysis.Status.KASession.ID).NotTo(BeEmpty())
			}, timeout, interval).Should(Succeed())

			By("deleting the InvestigationSession (triggers cancel)")
			is := &isv1alpha1.InvestigationSession{
				ObjectMeta: metav1.ObjectMeta{Name: isName, Namespace: testNamespace},
			}
			Expect(k8sClient.Delete(ctx, is)).To(Succeed())

			By("verifying AA transitions to PhaseFailed with ReasonInteractiveCancelled")
			Eventually(func(g Gomega) {
				g.Expect(k8sClient.Get(ctx, client.ObjectKeyFromObject(analysis), analysis)).To(Succeed())
				g.Expect(analysis.Status.Phase).To(Equal(aianalysisv1.PhaseFailed))
				g.Expect(analysis.Status.Reason).To(Equal(aianalysisv1.ReasonInteractiveCancelled))
			}, timeout, interval).Should(Succeed())

			By("verifying analysis.failed audit event was emitted (FedRAMP AU-2)")
			if dsClients != nil && dsClients.OpenAPIClient != nil {
				Eventually(func() bool {
					_ = auditStore.Flush(ctx)
					resp, err := dsClients.OpenAPIClient.QueryAuditEvents(ctx, ogenclient.QueryAuditEventsParams{
						CorrelationID: ogenclient.NewOptString(rrName),
						EventType:     ogenclient.NewOptString(aiaudit.EventTypeAnalysisFailed),
					})
					if err != nil {
						return false
					}
					return len(resp.Data) >= 1
				}, timeout, interval).Should(BeTrue(),
					"FedRAMP AU-2: analysis.failed audit event must be emitted when IS deletion causes PhaseFailed")
			}
		})

		It("IT-AA-1293-004: should upgrade autonomous session in-place with Interactive=true and SetActivePhase (#1390) [BR-INTERACTIVE-010]", func() {
			rrName := helpers.UniqueTestName("rr-takeover")
			analysisName := helpers.UniqueTestName("aa-takeover")

			analysis := createInvestigatingAA(analysisName, rrName, "", "slow-investigation-test", false)

			By("waiting for real KA session to be established")
			Eventually(func(g Gomega) {
				g.Expect(k8sClient.Get(ctx, client.ObjectKeyFromObject(analysis), analysis)).To(Succeed())
				g.Expect(analysis.Status.KASession).NotTo(BeNil())
				g.Expect(analysis.Status.KASession.ID).NotTo(BeEmpty())
			}, timeout, interval).Should(Succeed())

			sessionIDBefore := analysis.Status.KASession.ID

			By("creating Active InvestigationSession mid-investigation")
			createActiveIS(helpers.UniqueTestName("is-takeover"), rrName)

			By("verifying upgrade in-place: Interactive=true, session ID preserved")
			Eventually(func(g Gomega) {
				g.Expect(k8sClient.Get(ctx, client.ObjectKeyFromObject(analysis), analysis)).To(Succeed())
				g.Expect(analysis.Status.KASession).NotTo(BeNil())
				g.Expect(analysis.Status.KASession.Interactive).To(BeTrue(),
					"Interactive flag must be set by upgrade path")
				g.Expect(analysis.Status.KASession.ID).To(Equal(sessionIDBefore),
					"session ID must be preserved — no cancel/resubmit in #1390")
			}, timeout, interval).Should(Succeed())
		})
	})

	// IT-AA-1390-W04: AA upgrade path (no cancel) wiring through envtest reconcile loop.
	// Proves that when IS appears for an autonomous session, the handler sets Interactive=true
	// and calls SetActivePhase instead of cancelling and resubmitting.
	Context("#1390: AA upgrade-in-place wiring through reconcile loop", Serial, func() {
		var savedHandler *handlers.InvestigatingHandler

		BeforeEach(func() {
			savedHandler = reconciler.InvestigatingHandler.Load()
			auditClient := aiaudit.NewAuditClient(auditStore, ctrl.Log.WithName("upgrade-test-audit"))
			isChecker := handlers.NewK8sInvestigationSessionChecker(k8sClient, testNamespace)
			isPhaseUpdater := handlers.NewK8sISPhaseUpdater(k8sClient, testNamespace)
			reconciler.InvestigatingHandler.Store(handlers.NewInvestigatingHandler(
				realAgentClient,
				ctrl.Log.WithName("upgrade-real-handler"),
				testMetrics,
				auditClient,
				handlers.WithSessionMode(),
				handlers.WithSessionPollInterval(500*time.Millisecond),
				handlers.WithInvestigationSessionChecker(isChecker),
				handlers.WithISPhaseUpdater(isPhaseUpdater),
				handlers.WithRecorder(k8sManager.GetEventRecorderFor("aianalysis-controller")),
			))
		})

		AfterEach(func() {
			if savedHandler != nil {
				reconciler.InvestigatingHandler.Store(savedHandler)
			}
		})

		It("IT-AA-1390-W04: should set Interactive=true and SetActivePhase without cancel when IS appears for autonomous session [AC-12]", func() {
			rrName := helpers.UniqueTestName("rr-1390-upgrade")
			analysisName := helpers.UniqueTestName("aa-1390-upgrade")

			analysis := createInvestigatingAA(analysisName, rrName, "", "slow-investigation-test", false)

			By("waiting for real KA session to be established")
			Eventually(func(g Gomega) {
				g.Expect(k8sClient.Get(ctx, client.ObjectKeyFromObject(analysis), analysis)).To(Succeed())
				g.Expect(analysis.Status.KASession).NotTo(BeNil())
				g.Expect(analysis.Status.KASession.ID).NotTo(BeEmpty())
			}, timeout, interval).Should(Succeed())

			sessionIDBefore := analysis.Status.KASession.ID

			By("creating Active InvestigationSession for the same RR")
			isName := helpers.UniqueTestName("is-1390-upgrade")
			createActiveIS(isName, rrName)

			By("verifying upgrade path: Interactive=true, session ID preserved")
			Eventually(func(g Gomega) {
				g.Expect(k8sClient.Get(ctx, client.ObjectKeyFromObject(analysis), analysis)).To(Succeed())
				g.Expect(analysis.Status.KASession).NotTo(BeNil())
				g.Expect(analysis.Status.KASession.Interactive).To(BeTrue(),
					"Interactive flag must be set by upgrade path")
				g.Expect(analysis.Status.KASession.ID).To(Equal(sessionIDBefore),
					"session ID must be preserved — no cancel/resubmit")
			}, timeout, interval).Should(Succeed())

			By("verifying IS CRD Phase=Active was set via K8sISPhaseUpdater (best-effort)")
			var is isv1alpha1.InvestigationSession
			Expect(k8sClient.Get(ctx, client.ObjectKey{Name: isName, Namespace: testNamespace}, &is)).To(Succeed())
			Expect(is.Status.Phase).To(Equal(isv1alpha1.SessionPhaseActive),
				"IS phase must be Active after SetActivePhase call")
		})
	})

	// IT-AA-1376: IS terminal phase transitions through the reconcile loop.
	// These tests prove that K8sISPhaseUpdater.SetTerminalPhase is called through
	// the production dispatch path (envtest reconcile → handler → real KA → IS CRD).
	Context("#1376: IS terminal phase wiring through reconcile loop", Serial, func() {
		var savedHandler *handlers.InvestigatingHandler

		swapRealHandlerWithPhaseUpdater := func(opts ...handlers.InvestigatingHandlerOption) {
			savedHandler = reconciler.InvestigatingHandler.Load()
			auditClient := aiaudit.NewAuditClient(auditStore, ctrl.Log.WithName("is-phase-test-audit"))
			isChecker := handlers.NewK8sInvestigationSessionChecker(k8sClient, testNamespace)
			isPhaseUpdater := handlers.NewK8sISPhaseUpdater(k8sClient, testNamespace)

			baseOpts := []handlers.InvestigatingHandlerOption{
				handlers.WithSessionMode(),
				handlers.WithSessionPollInterval(500 * time.Millisecond),
				handlers.WithInvestigationSessionChecker(isChecker),
				handlers.WithISPhaseUpdater(isPhaseUpdater),
				handlers.WithRecorder(k8sManager.GetEventRecorderFor("aianalysis-controller")),
			}
			reconciler.InvestigatingHandler.Store(handlers.NewInvestigatingHandler(
				realAgentClient,
				ctrl.Log.WithName("is-phase-real-handler"),
				testMetrics,
				auditClient,
				append(baseOpts, opts...)...,
			))
		}

		AfterEach(func() {
			if savedHandler != nil {
				reconciler.InvestigatingHandler.Store(savedHandler)
			}
		})

		It("IT-AA-1376-001: IS transitions to Completed when KA session completes [BR-INTERACTIVE-010, #1376]", func() {
			swapRealHandlerWithPhaseUpdater()
			rrName := helpers.UniqueTestName("rr-1376-complete")
			isName := helpers.UniqueTestName("is-1376-complete")
			aaName := helpers.UniqueTestName("aa-1376-complete")

			// brief-investigation-test uses 3s SecondTurnDelay applied across
			// multiple investigator phases (RCA, workflow_discovery, etc.),
			// so the full investigation takes ~9-12s.
			completionTimeout := 25 * time.Second

			By("creating Investigating AA first (autonomous session — no IS yet)")
			analysis := createInvestigatingAA(aaName, rrName, "", "brief-investigation-test", false)

			By("waiting for real KA session to be established")
			Eventually(func(g Gomega) {
				g.Expect(k8sClient.Get(ctx, client.ObjectKeyFromObject(analysis), analysis)).To(Succeed())
				g.Expect(analysis.Status.KASession).NotTo(BeNil())
				g.Expect(analysis.Status.KASession.ID).NotTo(BeEmpty())
			}, timeout, interval).Should(Succeed())

			By("creating Active IS mid-investigation (triggers autonomous→interactive upgrade)")
			createActiveIS(isName, rrName)

			By("verifying IS CRD transitions to Completed (wiring proof)")
			Eventually(func(g Gomega) {
				var is isv1alpha1.InvestigationSession
				g.Expect(k8sClient.Get(ctx, client.ObjectKey{Name: isName, Namespace: testNamespace}, &is)).To(Succeed())
				g.Expect(is.Status.Phase).To(Equal(isv1alpha1.SessionPhaseCompleted),
					"#1376: IS must transition to Completed when KA session completes")
			}, completionTimeout, interval).Should(Succeed())

			By("verifying AA progresses past Investigating (sanity)")
			Eventually(func(g Gomega) {
				g.Expect(k8sClient.Get(ctx, client.ObjectKeyFromObject(analysis), analysis)).To(Succeed())
				g.Expect(string(analysis.Status.Phase)).NotTo(Equal(string(aianalysisv1.PhaseInvestigating)),
					"AA should have left Investigating phase after completed poll")
			}, completionTimeout, interval).Should(Succeed())
		})

		It("IT-AA-1376-002: IS transitions to Failed when KA session is cancelled [BR-INTERACTIVE-010, #1376]", func() {
			swapRealHandlerWithPhaseUpdater()
			rrName := helpers.UniqueTestName("rr-1376-failed")
			isName := helpers.UniqueTestName("is-1376-failed")
			aaName := helpers.UniqueTestName("aa-1376-failed")

			By("creating Active IS for the RR")
			createActiveIS(isName, rrName)

			By("creating Investigating AA with slow scenario")
			analysis := createInvestigatingAA(aaName, rrName, "", "slow-investigation-test", true)

			By("waiting for session to be established")
			Eventually(func(g Gomega) {
				g.Expect(k8sClient.Get(ctx, client.ObjectKeyFromObject(analysis), analysis)).To(Succeed())
				g.Expect(analysis.Status.KASession).NotTo(BeNil())
				g.Expect(analysis.Status.KASession.ID).NotTo(BeEmpty())
			}, timeout, interval).Should(Succeed())

			By("deleting IS to trigger session cancellation")
			is := &isv1alpha1.InvestigationSession{
				ObjectMeta: metav1.ObjectMeta{Name: isName, Namespace: testNamespace},
			}
			Expect(k8sClient.Delete(ctx, is)).To(Succeed())

			By("verifying AA transitions to PhaseFailed")
			Eventually(func(g Gomega) {
				g.Expect(k8sClient.Get(ctx, client.ObjectKeyFromObject(analysis), analysis)).To(Succeed())
				g.Expect(analysis.Status.Phase).To(Equal(aianalysisv1.PhaseFailed))
			}, timeout, interval).Should(Succeed())
		})

		It("IT-AA-1376-003: IS transitions to Failed on autonomous investigation timeout [BR-INTERACTIVE-010, AA-CRIT-1, #1376]", func() {
			swapRealHandlerWithPhaseUpdater(handlers.WithMaxInvestigationDuration(500 * time.Millisecond))
			rrName := helpers.UniqueTestName("rr-1376-auto-tout")
			isName := helpers.UniqueTestName("is-1376-auto-tout")
			aaName := helpers.UniqueTestName("aa-1376-auto-tout")

			By("creating Active IS for the RR")
			createActiveIS(isName, rrName)

			By("creating Investigating AA with slow scenario")
			analysis := createInvestigatingAA(aaName, rrName, "", "slow-investigation-test", false)

			By("waiting for session to be established then backdating")
			Eventually(func(g Gomega) {
				g.Expect(k8sClient.Get(ctx, client.ObjectKeyFromObject(analysis), analysis)).To(Succeed())
				g.Expect(analysis.Status.KASession).NotTo(BeNil())
				g.Expect(analysis.Status.KASession.ID).NotTo(BeEmpty())
			}, timeout, interval).Should(Succeed())

			Expect(k8sretry.RetryOnConflict(k8sretry.DefaultRetry, func() error {
				if err := k8sClient.Get(ctx, client.ObjectKeyFromObject(analysis), analysis); err != nil {
					return err
				}
				pastTime := metav1.NewTime(time.Now().Add(-2 * time.Hour))
				analysis.Status.KASession.CreatedAt = &pastTime
				return k8sClient.Status().Update(ctx, analysis)
			})).To(Succeed())

			By("verifying IS CRD transitions to Failed on timeout (wiring proof)")
			Eventually(func(g Gomega) {
				var is isv1alpha1.InvestigationSession
				g.Expect(k8sClient.Get(ctx, client.ObjectKey{Name: isName, Namespace: testNamespace}, &is)).To(Succeed())
				g.Expect(is.Status.Phase).To(Equal(isv1alpha1.SessionPhaseFailed),
					"#1376: IS must transition to Failed when autonomous investigation times out")
			}, timeout, interval).Should(Succeed())

			By("verifying AA transitions to PhaseFailed")
			Eventually(func(g Gomega) {
				g.Expect(k8sClient.Get(ctx, client.ObjectKeyFromObject(analysis), analysis)).To(Succeed())
				g.Expect(analysis.Status.Phase).To(Equal(aianalysisv1.PhaseFailed))
			}, timeout, interval).Should(Succeed())
		})

		It("IT-AA-1376-004: IS transitions to Failed on interactive investigation timeout [BR-INTERACTIVE-010, AA-CRIT-1, #1376]", func() {
			swapRealHandlerWithPhaseUpdater(handlers.WithMaxInvestigationDuration(500 * time.Millisecond))
			rrName := helpers.UniqueTestName("rr-1376-ud-tout")
			isName := helpers.UniqueTestName("is-1376-ud-tout")
			aaName := helpers.UniqueTestName("aa-1376-ud-tout")

			By("creating Active IS for the RR")
			createActiveIS(isName, rrName)

			By("creating Investigating AA with slow scenario (interactive)")
			analysis := createInvestigatingAA(aaName, rrName, "", "slow-investigation-test", true)

			By("waiting for session to be established then backdating")
			Eventually(func(g Gomega) {
				g.Expect(k8sClient.Get(ctx, client.ObjectKeyFromObject(analysis), analysis)).To(Succeed())
				g.Expect(analysis.Status.KASession).NotTo(BeNil())
				g.Expect(analysis.Status.KASession.ID).NotTo(BeEmpty())
			}, timeout, interval).Should(Succeed())

			Expect(k8sretry.RetryOnConflict(k8sretry.DefaultRetry, func() error {
				if err := k8sClient.Get(ctx, client.ObjectKeyFromObject(analysis), analysis); err != nil {
					return err
				}
				pastTime := metav1.NewTime(time.Now().Add(-2 * time.Hour))
				analysis.Status.KASession.CreatedAt = &pastTime
				return k8sClient.Status().Update(ctx, analysis)
			})).To(Succeed())

			By("verifying IS CRD transitions to Failed on timeout (wiring proof)")
			Eventually(func(g Gomega) {
				var is isv1alpha1.InvestigationSession
				g.Expect(k8sClient.Get(ctx, client.ObjectKey{Name: isName, Namespace: testNamespace}, &is)).To(Succeed())
				g.Expect(is.Status.Phase).To(Equal(isv1alpha1.SessionPhaseFailed),
					"#1376: IS must transition to Failed when interactive session times out")
			}, timeout, interval).Should(Succeed())

			By("verifying AA transitions to PhaseFailed")
			Eventually(func(g Gomega) {
				g.Expect(k8sClient.Get(ctx, client.ObjectKeyFromObject(analysis), analysis)).To(Succeed())
				g.Expect(analysis.Status.Phase).To(Equal(aianalysisv1.PhaseFailed))
			}, timeout, interval).Should(Succeed())
		})
	})

	// ═══════════════════════════════════════════════════════════════════════
	// Issue #1449 / FedRAMP SI-4: IS terminal transition wakes AA immediately
	// ═══════════════════════════════════════════════════════════════════════
	Context("#1449: IS terminal transition triggers immediate AA reconciliation (SI-4)", Serial, func() {
		var savedHandler *handlers.InvestigatingHandler

		BeforeEach(func() {
			savedHandler = reconciler.InvestigatingHandler.Load()
			auditClient := aiaudit.NewAuditClient(auditStore, ctrl.Log.WithName("terminal-watch-test-audit"))
			isChecker := handlers.NewK8sInvestigationSessionChecker(k8sClient, testNamespace)
			reconciler.InvestigatingHandler.Store(handlers.NewInvestigatingHandler(
				realAgentClient,
				ctrl.Log.WithName("terminal-watch-real-handler"),
				testMetrics,
				auditClient,
				handlers.WithSessionMode(),
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

		It("IT-AA-1449-002: AA reconciler fires within 5s when IS transitions to Completed (not waiting for 30s poll)", func() {
			rrName := helpers.UniqueTestName("rr-1449-watch")
			isName := helpers.UniqueTestName("is-1449-watch")
			aaName := helpers.UniqueTestName("aa-1449-watch")

			By("creating Active IS for the RR")
			createActiveIS(isName, rrName)

			By("creating Investigating AA with slow-investigation scenario")
			analysis := createInvestigatingAA(aaName, rrName, "", "slow-investigation-test", true)

			By("waiting for real KA session to be established")
			Eventually(func(g Gomega) {
				g.Expect(k8sClient.Get(ctx, client.ObjectKeyFromObject(analysis), analysis)).To(Succeed())
				g.Expect(analysis.Status.KASession).NotTo(BeNil())
				g.Expect(analysis.Status.KASession.ID).NotTo(BeEmpty())
			}, timeout, interval).Should(Succeed())

			By("recording ObservedGeneration before IS transition")
			var genBefore int64
			Expect(k8sClient.Get(ctx, client.ObjectKeyFromObject(analysis), analysis)).To(Succeed())
			genBefore = analysis.Status.ObservedGeneration

			By("patching IS → Completed (simulating API Frontend completing the session)")
			Expect(k8sretry.RetryOnConflict(k8sretry.DefaultRetry, func() error {
				var is isv1alpha1.InvestigationSession
				if err := k8sClient.Get(ctx, client.ObjectKey{Name: isName, Namespace: testNamespace}, &is); err != nil {
					return err
				}
				is.Status.Phase = isv1alpha1.SessionPhaseCompleted
				return k8sClient.Status().Update(ctx, &is)
			})).To(Succeed())

			By("verifying AA status changes within 5s (not waiting for 30s poll interval)")
			Eventually(func(g Gomega) {
				g.Expect(k8sClient.Get(ctx, client.ObjectKeyFromObject(analysis), analysis)).To(Succeed())
				g.Expect(analysis.Status.ObservedGeneration).To(BeNumerically(">", genBefore),
					"SI-4: IS→Completed must trigger immediate AA reconciliation via watch predicate")
			}, 5*time.Second, 200*time.Millisecond).Should(Succeed())
		})
	})
})
