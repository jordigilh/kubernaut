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
			savedHandler = reconciler.InvestigatingHandler
			mockClient = mocks.NewMockAgentClient()
			mockClient.WithSessionPollStatus("investigating")

			auditClient := aiaudit.NewAuditClient(auditStore, ctrl.Log.WithName("interactive-test-audit"))
			isChecker := handlers.NewK8sInvestigationSessionChecker(k8sClient, testNamespace)
			reconciler.InvestigatingHandler = handlers.NewInvestigatingHandler(
				mockClient,
				ctrl.Log.WithName("interactive-mock-handler"),
				testMetrics,
				auditClient,
				handlers.WithSessionMode(),
				handlers.WithSessionPollInterval(100*time.Millisecond),
				handlers.WithInvestigationSessionChecker(isChecker),
				handlers.WithRecorder(k8sManager.GetEventRecorderFor("aianalysis-controller")),
			)
		}

		BeforeEach(func() {
			swapMockHandler()
		})

		AfterEach(func() {
			if savedHandler != nil {
				reconciler.InvestigatingHandler = savedHandler
			}
		})

		It("IT-AA-1293-002: should cancel autonomous session when Active IS is created", func() {
			rrName := helpers.UniqueTestName("rr-watch-create")
			sessionID := "session-autonomous-001"
			analysisName := helpers.UniqueTestName("aa-watch-create")

			analysis := createInvestigatingAA(analysisName, rrName, sessionID, false)

			By("waiting for controller to start polling (proves reconcile loop is active)")
			Eventually(func() int {
				return mockClient.PollCallCount
			}, timeout, interval).Should(BeNumerically(">=", 1))

			By("creating Active InvestigationSession for the same RR")
			createActiveIS(helpers.UniqueTestName("is-watch-create"), rrName)

			By("verifying IS watch triggered cancel and cleared session ID for re-submit")
			Eventually(func(g Gomega) {
				g.Expect(k8sClient.Get(ctx, client.ObjectKeyFromObject(analysis), analysis)).To(Succeed())
				g.Expect(analysis.Status.KASession).NotTo(BeNil())
				g.Expect(mockClient.CancelCallCount).To(BeNumerically(">=", 1),
					"CancelSession should be called when IS appears for autonomous session")
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
				if returnCancelled || mockClient.CancelCallCount > 0 {
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
				return mockClient.PollCallCount
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
		})

		It("IT-AA-1293-004: should cancel autonomous session and re-submit with interactive=true on takeover", func() {
			rrName := helpers.UniqueTestName("rr-takeover")
			oldSessionID := "session-autonomous-old"
			newSessionID := "session-interactive-new"
			analysisName := helpers.UniqueTestName("aa-takeover")

			submitCount := 0
			var submitMu sync.Mutex
			mockClient.SubmitInvestigationFunc = func(_ context.Context, req *agentclient.IncidentRequest) (string, error) {
				submitMu.Lock()
				defer submitMu.Unlock()
				submitCount++
				mockClient.LastRequest = req
				return newSessionID, nil
			}

			analysis := createInvestigatingAA(analysisName, rrName, oldSessionID, false)

			By("waiting for controller to start polling (proves reconcile loop is active)")
			Eventually(func() int {
				return mockClient.PollCallCount
			}, timeout, interval).Should(BeNumerically(">=", 1))

			By("creating Active InvestigationSession mid-investigation")
			createActiveIS(helpers.UniqueTestName("is-takeover"), rrName)

			By("verifying cancel + re-submit with interactive=true")
			Eventually(func(g Gomega) {
				g.Expect(k8sClient.Get(ctx, client.ObjectKeyFromObject(analysis), analysis)).To(Succeed())
				g.Expect(mockClient.CancelCallCount).To(BeNumerically(">=", 1))

				submitMu.Lock()
				sc := submitCount
				submitMu.Unlock()
				g.Expect(sc).To(BeNumerically(">=", 1))

				g.Expect(analysis.Status.KASession).NotTo(BeNil())
				g.Expect(analysis.Status.KASession.ID).To(Equal(newSessionID))
				g.Expect(analysis.Status.KASession.Interactive).To(BeTrue())
				g.Expect(mockClient.LastRequest).NotTo(BeNil())
				interactive, ok := mockClient.LastRequest.Interactive.Get()
				g.Expect(ok).To(BeTrue())
				g.Expect(interactive).To(BeTrue())
			}, timeout, interval).Should(Succeed())
		})
	})
})
