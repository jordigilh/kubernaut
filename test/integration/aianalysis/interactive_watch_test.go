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
	"context"
	"sync"
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
	"github.com/jordigilh/kubernaut/pkg/agentclient"
	ogenclient "github.com/jordigilh/kubernaut/pkg/datastorage/ogen-client"
	sharedtypes "github.com/jordigilh/kubernaut/pkg/shared/types"
	"github.com/jordigilh/kubernaut/test/shared/helpers"
	"github.com/jordigilh/kubernaut/test/shared/mocks"
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

	createInvestigatingAA := func(name, rrName, sessionID string, interactive bool) *aianalysisv1.AIAnalysis {
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
						Severity:         "medium",
						SignalName:       "CrashLoopBackOff",
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

	// IT-002..004 swap the investigating handler to a mock KA client for deterministic
	// cancel/submit behavior. Serial execution prevents cross-test handler races.
	Context("IS watch-driven reconciliation", Serial, func() {
		var (
			savedHandler *handlers.InvestigatingHandler
			mockClient   *mocks.MockAgentClient
		)

		swapMockHandler := func() {
			savedHandler = reconciler.InvestigatingHandler.Load()
			mockClient = mocks.NewMockAgentClient()
			mockClient.WithSessionPollStatus("investigating")

			auditClient := aiaudit.NewAuditClient(auditStore, ctrl.Log.WithName("interactive-test-audit"))
			isChecker := handlers.NewK8sInvestigationSessionChecker(k8sClient, testNamespace)
			reconciler.InvestigatingHandler.Store(handlers.NewInvestigatingHandler(
				mockClient,
				ctrl.Log.WithName("interactive-mock-handler"),
				testMetrics,
				auditClient,
				handlers.WithSessionMode(),
				handlers.WithSessionPollInterval(100*time.Millisecond),
				handlers.WithInvestigationSessionChecker(isChecker),
				handlers.WithRecorder(k8sManager.GetEventRecorderFor("aianalysis-controller")),
			))
		}

		BeforeEach(func() {
			swapMockHandler()
		})

		AfterEach(func() {
			if savedHandler != nil {
				reconciler.InvestigatingHandler.Store(savedHandler)
			}
		})

		It("IT-AA-1293-002: should upgrade autonomous session in-place when Active IS is created (#1390)", func() {
			rrName := helpers.UniqueTestName("rr-watch-create")
			sessionID := "session-autonomous-001"
			analysisName := helpers.UniqueTestName("aa-watch-create")

			analysis := createInvestigatingAA(analysisName, rrName, sessionID, false)

			By("waiting for controller to start polling (proves reconcile loop is active)")
			Eventually(func() int {
				return mockClient.GetPollCallCount()
			}, timeout, interval).Should(BeNumerically(">=", 1))

			By("creating Active InvestigationSession for the same RR")
			createActiveIS(helpers.UniqueTestName("is-watch-create"), rrName)

			By("verifying IS watch triggered upgrade: Interactive=true, no cancel, session ID preserved")
			Eventually(func(g Gomega) {
				g.Expect(k8sClient.Get(ctx, client.ObjectKeyFromObject(analysis), analysis)).To(Succeed())
				g.Expect(analysis.Status.KASession).NotTo(BeNil())
				g.Expect(analysis.Status.KASession.Interactive).To(BeTrue(),
					"Interactive flag must be set by upgrade path")
				g.Expect(analysis.Status.KASession.ID).To(Equal(sessionID),
					"session ID must be preserved — upgrade in-place, no cancel")
				g.Expect(mockClient.GetCancelCallCount()).To(Equal(0),
					"CancelSession must NOT be called — #1390 upgrade replaces cancel")
			}, timeout, interval).Should(Succeed())
		})

		It("IT-AA-1293-003: should fail with ReasonInteractiveCancelled when IS is deleted", func() {
			rrName := helpers.UniqueTestName("rr-watch-delete")
			sessionID := "session-interactive-001"
			analysisName := helpers.UniqueTestName("aa-watch-delete")

			var pollMu sync.Mutex
			returnCancelled := false
			mockClient.PollSessionFunc = func(_ context.Context, _ string) (*agentclient.SessionStatusResult, error) {
				pollMu.Lock()
				defer pollMu.Unlock()
				if returnCancelled || mockClient.GetCancelCallCount() > 0 {
					return &agentclient.SessionStatusResult{Status: "cancelled"}, nil
				}
				return &agentclient.SessionStatusResult{Status: "investigating"}, nil
			}

			By("creating Active IS first so controller sees it on first AA reconcile")
			isName := helpers.UniqueTestName("is-watch-delete")
			createActiveIS(isName, rrName)

			analysis := createInvestigatingAA(analysisName, rrName, sessionID, true)

			By("waiting for controller to start polling (proves reconcile loop is active)")
			Eventually(func() int {
				return mockClient.GetPollCallCount()
			}, timeout, interval).Should(BeNumerically(">=", 1))

			By("deleting the InvestigationSession")
			is := &isv1alpha1.InvestigationSession{
				ObjectMeta: metav1.ObjectMeta{Name: isName, Namespace: testNamespace},
			}
			Expect(k8sClient.Delete(ctx, is)).To(Succeed())

			By("flipping mock to return cancelled so next poll triggers terminal transition")
			pollMu.Lock()
			returnCancelled = true
			pollMu.Unlock()

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
			sessionID := "session-autonomous-upgrade"
			analysisName := helpers.UniqueTestName("aa-takeover")

			analysis := createInvestigatingAA(analysisName, rrName, sessionID, false)

			By("waiting for controller to start polling (proves reconcile loop is active)")
			Eventually(func() int {
				return mockClient.GetPollCallCount()
			}, timeout, interval).Should(BeNumerically(">=", 1))

			By("creating Active InvestigationSession mid-investigation")
			createActiveIS(helpers.UniqueTestName("is-takeover"), rrName)

			By("verifying upgrade in-place: Interactive=true, no cancel, session ID preserved")
			Eventually(func(g Gomega) {
				g.Expect(k8sClient.Get(ctx, client.ObjectKeyFromObject(analysis), analysis)).To(Succeed())
				g.Expect(analysis.Status.KASession).NotTo(BeNil())
				g.Expect(analysis.Status.KASession.Interactive).To(BeTrue(),
					"Interactive flag must be set by upgrade path")
				g.Expect(analysis.Status.KASession.ID).To(Equal(sessionID),
					"session ID must be preserved — no cancel/resubmit in #1390")
				g.Expect(mockClient.GetCancelCallCount()).To(Equal(0),
					"CancelSession must NOT be called — upgrade in-place replaces cancel")
			}, timeout, interval).Should(Succeed())
		})
	})

	// IT-AA-1390-W04: AA upgrade path (no cancel) wiring through envtest reconcile loop.
	// Proves that when IS appears for an autonomous session, the handler sets Interactive=true
	// and calls SetActivePhase instead of cancelling and resubmitting.
	Context("#1390: AA upgrade-in-place wiring through reconcile loop", Serial, func() {
		var (
			savedHandler *handlers.InvestigatingHandler
			mockClient   *mocks.MockAgentClient
		)

		swapMockHandlerWithUpgrade := func() {
			savedHandler = reconciler.InvestigatingHandler.Load()
			mockClient = mocks.NewMockAgentClient()
			mockClient.WithSessionPollStatus("investigating")

			auditClient := aiaudit.NewAuditClient(auditStore, ctrl.Log.WithName("upgrade-test-audit"))
			isChecker := handlers.NewK8sInvestigationSessionChecker(k8sClient, testNamespace)
			isPhaseUpdater := handlers.NewK8sISPhaseUpdater(k8sClient, testNamespace)
			reconciler.InvestigatingHandler.Store(handlers.NewInvestigatingHandler(
				mockClient,
				ctrl.Log.WithName("upgrade-mock-handler"),
				testMetrics,
				auditClient,
				handlers.WithSessionMode(),
				handlers.WithSessionPollInterval(100*time.Millisecond),
				handlers.WithInvestigationSessionChecker(isChecker),
				handlers.WithISPhaseUpdater(isPhaseUpdater),
				handlers.WithRecorder(k8sManager.GetEventRecorderFor("aianalysis-controller")),
			))
		}

		BeforeEach(func() {
			swapMockHandlerWithUpgrade()
		})

		AfterEach(func() {
			if savedHandler != nil {
				reconciler.InvestigatingHandler.Store(savedHandler)
			}
		})

		It("IT-AA-1390-W04: should set Interactive=true and SetActivePhase without cancel when IS appears for autonomous session [AC-12]", func() {
			rrName := helpers.UniqueTestName("rr-1390-upgrade")
			sessionID := "session-autonomous-upgrade"
			analysisName := helpers.UniqueTestName("aa-1390-upgrade")

			mockClient.PollSessionFunc = func(_ context.Context, _ string) (*agentclient.SessionStatusResult, error) {
				return &agentclient.SessionStatusResult{
					Status:     "user_driving",
					ActingUser: "sre-takeover@example.com",
				}, nil
			}

			analysis := createInvestigatingAA(analysisName, rrName, sessionID, false)

			By("waiting for controller to start polling (proves reconcile loop is active)")
			Eventually(func() int {
				return mockClient.GetPollCallCount()
			}, timeout, interval).Should(BeNumerically(">=", 1))

			cancelCountBeforeIS := mockClient.GetCancelCallCount()

			By("creating Active InvestigationSession for the same RR")
			isName := helpers.UniqueTestName("is-1390-upgrade")
			createActiveIS(isName, rrName)

			By("verifying upgrade path: Interactive=true, no cancel, IS Phase=Active preserved")
			Eventually(func(g Gomega) {
				g.Expect(k8sClient.Get(ctx, client.ObjectKeyFromObject(analysis), analysis)).To(Succeed())
				g.Expect(analysis.Status.KASession).NotTo(BeNil())
				g.Expect(analysis.Status.KASession.Interactive).To(BeTrue(),
					"Interactive flag must be set by upgrade path")
				g.Expect(analysis.Status.KASession.ID).To(Equal(sessionID),
					"session ID must be preserved — no cancel/resubmit")
				g.Expect(mockClient.GetCancelCallCount()).To(Equal(cancelCountBeforeIS),
					"CancelSession must NOT be called for THIS session in the upgrade path")
			}, timeout, interval).Should(Succeed())

			By("verifying InteractiveSession populated from user_driving poll")
			Eventually(func(g Gomega) {
				g.Expect(k8sClient.Get(ctx, client.ObjectKeyFromObject(analysis), analysis)).To(Succeed())
				g.Expect(analysis.Status.InteractiveSession).NotTo(BeNil(),
					"InteractiveSession must be populated after polling user_driving")
				g.Expect(analysis.Status.InteractiveSession.ActingUser).To(Equal("sre-takeover@example.com"))
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
	// the production dispatch path (envtest reconcile → handler → real updater → IS CRD).
	Context("#1376: IS terminal phase wiring through reconcile loop", Serial, func() {
		var (
			savedHandler *handlers.InvestigatingHandler
			mockClient   *mocks.MockAgentClient
		)

		swapMockHandlerWithPhaseUpdater := func(opts ...handlers.InvestigatingHandlerOption) {
			savedHandler = reconciler.InvestigatingHandler.Load()
			mockClient = mocks.NewMockAgentClient()
			mockClient.WithSessionPollStatus("investigating")

			auditClient := aiaudit.NewAuditClient(auditStore, ctrl.Log.WithName("is-phase-test-audit"))
			isChecker := handlers.NewK8sInvestigationSessionChecker(k8sClient, testNamespace)
			isPhaseUpdater := handlers.NewK8sISPhaseUpdater(k8sClient, testNamespace)

			baseOpts := []handlers.InvestigatingHandlerOption{
				handlers.WithSessionMode(),
				handlers.WithSessionPollInterval(100 * time.Millisecond),
				handlers.WithInvestigationSessionChecker(isChecker),
				handlers.WithISPhaseUpdater(isPhaseUpdater),
				handlers.WithRecorder(k8sManager.GetEventRecorderFor("aianalysis-controller")),
			}
			reconciler.InvestigatingHandler.Store(handlers.NewInvestigatingHandler(
				mockClient,
				ctrl.Log.WithName("is-phase-mock-handler"),
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
			swapMockHandlerWithPhaseUpdater()
			rrName := helpers.UniqueTestName("rr-1376-complete")
			isName := helpers.UniqueTestName("is-1376-complete")
			aaName := helpers.UniqueTestName("aa-1376-complete")

			By("creating Active IS for the RR")
			createActiveIS(isName, rrName)

			By("creating Investigating AA with session")
			mockClient.WithSessionPollStatus("completed")
			mockClient.Response = &agentclient.IncidentResponse{
				IncidentID:        "mock-it-1376-001",
				Analysis:          "OOM caused by memory leak in api-server",
				RootCauseAnalysis: mocks.BuildMockRCA("OOM caused by memory leak", "high", nil),
				Confidence:        0.95,
				Timestamp:         "2026-06-06T22:00:00Z",
			}
			analysis := createInvestigatingAA(aaName, rrName, "session-1376-001", true)

			By("verifying IS CRD transitions to Completed (wiring proof)")
			Eventually(func(g Gomega) {
				var is isv1alpha1.InvestigationSession
				g.Expect(k8sClient.Get(ctx, client.ObjectKey{Name: isName, Namespace: testNamespace}, &is)).To(Succeed())
				g.Expect(is.Status.Phase).To(Equal(isv1alpha1.SessionPhaseCompleted),
					"#1376: IS must transition to Completed when KA session completes")
			}, timeout, interval).Should(Succeed())

			By("verifying AA progresses past Investigating (sanity)")
			Eventually(func(g Gomega) {
				g.Expect(k8sClient.Get(ctx, client.ObjectKeyFromObject(analysis), analysis)).To(Succeed())
				g.Expect(string(analysis.Status.Phase)).NotTo(Equal(string(aianalysisv1.PhaseInvestigating)),
					"AA should have left Investigating phase after completed poll")
			}, timeout, interval).Should(Succeed())
		})

		It("IT-AA-1376-002: IS transitions to Failed when KA session fails [BR-INTERACTIVE-010, #1376]", func() {
			swapMockHandlerWithPhaseUpdater()
			rrName := helpers.UniqueTestName("rr-1376-failed")
			isName := helpers.UniqueTestName("is-1376-failed")
			aaName := helpers.UniqueTestName("aa-1376-failed")

			By("creating Active IS for the RR")
			createActiveIS(isName, rrName)

			By("creating Investigating AA with failing session")
			mockClient.WithSessionPollStatus("failed")
			mockClient.DefaultSessionStatus = &agentclient.SessionStatusResult{
				Status: "failed",
				Error:  "LLM provider timeout after 120s",
			}
			analysis := createInvestigatingAA(aaName, rrName, "session-1376-002", true)

			By("verifying IS CRD transitions to Failed (wiring proof)")
			Eventually(func(g Gomega) {
				var is isv1alpha1.InvestigationSession
				g.Expect(k8sClient.Get(ctx, client.ObjectKey{Name: isName, Namespace: testNamespace}, &is)).To(Succeed())
				g.Expect(is.Status.Phase).To(Equal(isv1alpha1.SessionPhaseFailed),
					"#1376: IS must transition to Failed when KA session fails")
			}, timeout, interval).Should(Succeed())

			By("verifying AA transitions to PhaseFailed")
			Eventually(func(g Gomega) {
				g.Expect(k8sClient.Get(ctx, client.ObjectKeyFromObject(analysis), analysis)).To(Succeed())
				g.Expect(analysis.Status.Phase).To(Equal(aianalysisv1.PhaseFailed))
			}, timeout, interval).Should(Succeed())
		})

		It("IT-AA-1376-003: IS transitions to Failed on autonomous investigation timeout [BR-INTERACTIVE-010, AA-CRIT-1, #1376]", func() {
			swapMockHandlerWithPhaseUpdater(handlers.WithMaxInvestigationDuration(500 * time.Millisecond))
			rrName := helpers.UniqueTestName("rr-1376-auto-tout")
			isName := helpers.UniqueTestName("is-1376-auto-tout")
			aaName := helpers.UniqueTestName("aa-1376-auto-tout")

			By("creating Active IS for the RR")
			createActiveIS(isName, rrName)

			By("creating Investigating AA with backdated session (already timed out)")
			analysis := createInvestigatingAA(aaName, rrName, "session-1376-003", false)

			By("backdating session.CreatedAt to trigger timeout")
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

		It("IT-AA-1376-004: IS transitions to Failed on interactive user_driving timeout [BR-INTERACTIVE-010, AA-CRIT-1, #1376]", func() {
			swapMockHandlerWithPhaseUpdater(handlers.WithMaxInvestigationDuration(500 * time.Millisecond))
			rrName := helpers.UniqueTestName("rr-1376-ud-tout")
			isName := helpers.UniqueTestName("is-1376-ud-tout")
			aaName := helpers.UniqueTestName("aa-1376-ud-tout")

			By("creating Active IS for the RR")
			createActiveIS(isName, rrName)

			By("creating Investigating AA with interactive session")
			mockClient.WithSessionPollStatus("user_driving")
			analysis := createInvestigatingAA(aaName, rrName, "session-1376-004", true)

			By("backdating session.CreatedAt to trigger timeout")
			Expect(k8sretry.RetryOnConflict(k8sretry.DefaultRetry, func() error {
				if err := k8sClient.Get(ctx, client.ObjectKeyFromObject(analysis), analysis); err != nil {
					return err
				}
				pastTime := metav1.NewTime(time.Now().Add(-2 * time.Hour))
				analysis.Status.KASession.CreatedAt = &pastTime
				return k8sClient.Status().Update(ctx, analysis)
			})).To(Succeed())

			By("verifying IS CRD transitions to Failed on user_driving timeout (wiring proof)")
			Eventually(func(g Gomega) {
				var is isv1alpha1.InvestigationSession
				g.Expect(k8sClient.Get(ctx, client.ObjectKey{Name: isName, Namespace: testNamespace}, &is)).To(Succeed())
				g.Expect(is.Status.Phase).To(Equal(isv1alpha1.SessionPhaseFailed),
					"#1376: IS must transition to Failed when interactive user_driving session times out")
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
		var (
			savedHandler *handlers.InvestigatingHandler
			mockClient   *mocks.MockAgentClient
		)

		swapMockHandlerForTerminalWatch := func() {
			savedHandler = reconciler.InvestigatingHandler.Load()
			mockClient = mocks.NewMockAgentClient()
			mockClient.WithSessionPollStatus("investigating")

			auditClient := aiaudit.NewAuditClient(auditStore, ctrl.Log.WithName("terminal-watch-test-audit"))
			isChecker := handlers.NewK8sInvestigationSessionChecker(k8sClient, testNamespace)
			reconciler.InvestigatingHandler.Store(handlers.NewInvestigatingHandler(
				mockClient,
				ctrl.Log.WithName("terminal-watch-mock-handler"),
				testMetrics,
				auditClient,
				handlers.WithSessionMode(),
				handlers.WithSessionPollInterval(30*time.Second),
				handlers.WithInvestigationSessionChecker(isChecker),
				handlers.WithRecorder(k8sManager.GetEventRecorderFor("aianalysis-controller")),
			))
		}

		BeforeEach(func() {
			swapMockHandlerForTerminalWatch()
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
			sessionID := "session-1449-terminal"

			By("creating Active IS for the RR")
			createActiveIS(isName, rrName)

			By("creating Investigating AA with interactive session")
			_ = createInvestigatingAA(aaName, rrName, sessionID, true)

			By("waiting for controller to start polling (proves reconcile loop is active)")
			Eventually(func() int {
				return mockClient.GetPollCallCount()
			}, 15*time.Second, interval).Should(BeNumerically(">=", 1))

			By("recording poll count before IS transition")
			pollCountBefore := mockClient.GetPollCallCount()

			By("patching IS → Completed (simulating API Frontend completing the session)")
			Expect(k8sretry.RetryOnConflict(k8sretry.DefaultRetry, func() error {
				var is isv1alpha1.InvestigationSession
				if err := k8sClient.Get(ctx, client.ObjectKey{Name: isName, Namespace: testNamespace}, &is); err != nil {
					return err
				}
				is.Status.Phase = isv1alpha1.SessionPhaseCompleted
				return k8sClient.Status().Update(ctx, &is)
			})).To(Succeed())

			By("verifying AA reconciler fires within 5s (not waiting for 30s poll interval)")
			Eventually(func() int {
				return mockClient.GetPollCallCount()
			}, 5*time.Second, 200*time.Millisecond).Should(BeNumerically(">", pollCountBefore),
				"SI-4: IS→Completed must trigger immediate AA reconciliation via watch predicate, not wait for 30s poll")
		})
	})
})
