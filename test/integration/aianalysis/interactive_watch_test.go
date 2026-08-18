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
	sharedtypes "github.com/jordigilh/kubernaut/pkg/shared/types"
	"github.com/jordigilh/kubernaut/test/shared/helpers"
	"github.com/jordigilh/kubernaut/test/shared/mocks"
)

// createActiveIS creates an InvestigationSession CRD for rrName and drives it
// to Phase=Active, waiting for that phase to be visible in the cache before
// returning. Package-level so it can be shared across integration test files
// in this package.
func createActiveIS(name, rrName string) {
	const (
		timeout  = 15 * time.Second
		interval = 200 * time.Millisecond
	)
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

	Eventually(func() isv1alpha1.SessionPhase {
		var updated isv1alpha1.InvestigationSession
		if err := k8sClient.Get(ctx, client.ObjectKeyFromObject(is), &updated); err != nil {
			return ""
		}
		return updated.Status.Phase
	}, timeout, interval).Should(Equal(isv1alpha1.SessionPhaseActive))
}

// createInvestigatingAA creates an AIAnalysis CRD already in PhaseInvestigating
// with a pre-set KASession, for tests exercising the InvestigatingHandler's
// write-only IS terminal-close path (K8sISPhaseUpdater.SetTerminalPhase,
// DD-AA-KA-001 Amendment Gap 1) through the real reconcile loop, rather than
// session establishment itself (which is now driven by AgentSession.GetOrCreate,
// not a direct KA HTTP submit -- see createActiveIS's caller for the field-index
// proof, and pkg/aianalysis/investigating_handler_is_phase_test.go's UT-AA-1376-*
// for the underlying business logic).
func createInvestigatingAA(name, rrName string) {
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

	// RetryOnConflict: the real per-process AIAnalysis controller reconciles
	// this object concurrently (it watches Create events too), so a plain
	// Get-then-Status().Update here can race its own status write and get
	// "the object has been modified" -- same documented pattern already used
	// for AIAnalysis/other CRDs in test/shared/helpers/crd_lifecycle.go
	// (SimulateAICompletedWithWorkflow et al.: "Uses RetryOnConflict to
	// handle races with the RO controller").
	now := metav1.Now()
	Expect(k8sretry.RetryOnConflict(k8sretry.DefaultRetry, func() error {
		if err := k8sClient.Get(ctx, client.ObjectKeyFromObject(analysis), analysis); err != nil {
			return err
		}
		analysis.Status.Phase = aianalysisv1.PhaseInvestigating
		analysis.Status.KASession = &aianalysisv1.KASession{
			Interactive: false,
			CreatedAt:   &now,
		}
		return k8sClient.Status().Update(ctx, analysis)
	})).To(Succeed())
}

// BR-INTERACTIVE-010: Integration tests for InvestigationSession field index
// wiring (#1293) and the write-only terminal-close path (#1376).
//
// DD-AA-KA-001 Amendment Gap 1 retires AA's own IS-watch-driven interactive
// detection/adoption entirely (is_checker.HasActiveSession,
// applyInteractiveDetection, ISEventPredicate, tryAdoptCorrelatedSession) --
// that decision now lives in KA's dispatch watcher, keyed off
// AgentSession.Status.Interactive/ActingUser, not AA reading IS. What
// survives on AA's side is a narrow, write-only concern: closing out AF's IS
// bookkeeping when the backing investigation reaches a terminal outcome
// (K8sISPhaseUpdater.SetTerminalPhase), proven below through the real
// reconcile loop against a real Kubernetes API server (CHECKPOINT W).
var _ = Describe("BR-INTERACTIVE-010: InvestigationSession field index + terminal-close wiring", Label("integration", "interactive"), func() {
	const (
		timeout  = 15 * time.Second
		interval = 200 * time.Millisecond
	)

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

	Context("#1376: IS terminal-close wiring through the real reconcile loop", Serial, func() {
		var (
			savedHandler *handlers.InvestigatingHandler
			mockClient   *mocks.MockAgentClient
		)

		BeforeEach(func() {
			savedHandler = reconciler.InvestigatingHandler.Load()
			mockClient = mocks.NewMockAgentClient()
			auditClient := aiaudit.NewAuditClient(auditStore, ctrl.Log.WithName("is-phase-wiring-test-audit"))
			isPhaseUpdater := handlers.NewK8sISPhaseUpdater(k8sClient, testNamespace)
			reconciler.InvestigatingHandler.Store(handlers.NewInvestigatingHandler(
				mockClient,
				ctrl.Log.WithName("is-phase-wiring-mock-handler"),
				testMetrics,
				auditClient,
				handlers.WithSessionPollInterval(500*time.Millisecond),
				handlers.WithISPhaseUpdater(isPhaseUpdater),
				handlers.WithRecorder(k8sManager.GetEventRecorderFor("aianalysis-controller")),
			))
		})

		AfterEach(func() {
			if savedHandler != nil {
				reconciler.InvestigatingHandler.Store(savedHandler)
			}
		})

		It("IT-AA-1376-001: IS transitions to Completed when the AgentSession-backed investigation completes", func() {
			rrName := helpers.UniqueTestName("rr-1376-complete")
			isName := helpers.UniqueTestName("is-1376-complete")
			aaName := helpers.UniqueTestName("aa-1376-complete")

			By("creating an Active IS for the RR (AF's own bookkeeping)")
			createActiveIS(isName, rrName)

			By("configuring the mock KA client to report a completed result")
			mockClient.WithSuccessResponse("mock analysis: issue resolved", 0.9, nil)

			By("creating an Investigating AA for the same RR")
			createInvestigatingAA(aaName, rrName)

			By("verifying IS CRD transitions to Completed (wiring proof, #1376)")
			Eventually(func(g Gomega) {
				var is isv1alpha1.InvestigationSession
				g.Expect(k8sClient.Get(ctx, client.ObjectKey{Name: isName, Namespace: testNamespace}, &is)).To(Succeed())
				g.Expect(is.Status.Phase).To(Equal(isv1alpha1.SessionPhaseCompleted),
					"#1376: IS must transition to Completed when the backing investigation completes")
			}, timeout, interval).Should(Succeed())
		})

		It("IT-AA-1376-002: IS transitions to Failed when the AgentSession-backed investigation fails", func() {
			rrName := helpers.UniqueTestName("rr-1376-failed")
			isName := helpers.UniqueTestName("is-1376-failed")
			aaName := helpers.UniqueTestName("aa-1376-failed")

			By("creating an Active IS for the RR")
			createActiveIS(isName, rrName)

			By("configuring the mock KA client to report a failed session")
			mockClient.WithFailed("simulated KA failure (#1376 IT)")

			By("creating an Investigating AA for the same RR")
			createInvestigatingAA(aaName, rrName)

			By("verifying IS CRD transitions to Failed (wiring proof, #1376)")
			Eventually(func(g Gomega) {
				var is isv1alpha1.InvestigationSession
				g.Expect(k8sClient.Get(ctx, client.ObjectKey{Name: isName, Namespace: testNamespace}, &is)).To(Succeed())
				g.Expect(is.Status.Phase).To(Equal(isv1alpha1.SessionPhaseFailed),
					"#1376: IS must transition to Failed when the backing investigation fails")
			}, timeout, interval).Should(Succeed())
		})
	})
})
