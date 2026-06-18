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

// Issue #1449: Proves the full escalation wiring path through the reconciler.
// Unlike IT-AA-1449-001 (which only proves the CRD API server accepts the enum),
// this test proves the complete production dispatch: KA result with
// operator_escalation → ResponseProcessor → CRD status with all escalation fields.
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
	"github.com/jordigilh/kubernaut/pkg/aianalysis"
	aiaudit "github.com/jordigilh/kubernaut/pkg/aianalysis/audit"
	"github.com/jordigilh/kubernaut/pkg/aianalysis/handlers"
	sharedtypes "github.com/jordigilh/kubernaut/pkg/shared/types"
	"github.com/jordigilh/kubernaut/test/shared/helpers"
	"github.com/jordigilh/kubernaut/test/shared/mocks"
)

var _ = Describe("Escalation Wiring (#1449)", Label("integration", "escalation", "wiring"), Serial, func() {
	const (
		timeout  = 30 * time.Second
		interval = 200 * time.Millisecond
	)

	var (
		savedHandler *handlers.InvestigatingHandler
		mockClient   *mocks.MockAgentClient
	)

	BeforeEach(func() {
		savedHandler = reconciler.InvestigatingHandler.Load()
		mockClient = mocks.NewMockAgentClient()
		mockClient.WithSessionPollStatus("completed")
		mockClient.WithHumanReviewReasonEnum("operator_escalation", nil)

		auditClient := aiaudit.NewAuditClient(auditStore, ctrl.Log.WithName("escalation-wiring-test-audit"))
		isChecker := handlers.NewK8sInvestigationSessionChecker(k8sClient, testNamespace)
		reconciler.InvestigatingHandler.Store(handlers.NewInvestigatingHandler(
			mockClient,
			ctrl.Log.WithName("escalation-wiring-mock-handler"),
			testMetrics,
			auditClient,
			handlers.WithSessionMode(),
			handlers.WithSessionPollInterval(1*time.Second),
			handlers.WithInvestigationSessionChecker(isChecker),
			handlers.WithRecorder(k8sManager.GetEventRecorderFor("aianalysis-controller")),
		))
	})

	AfterEach(func() {
		if savedHandler != nil {
			reconciler.InvestigatingHandler.Store(savedHandler)
		}
	})

	It("IT-AA-1449-003: KA operator_escalation result flows through reconciler to CRD status (BR-HAPI-197)", func() {
		rrName := helpers.UniqueTestName("rr-1449-wiring")
		aaName := helpers.UniqueTestName("aa-1449-wiring")
		sessionID := "session-1449-escalation-wiring"

		By("creating Investigating AA with active KA session")
		analysis := &aianalysisv1.AIAnalysis{
			ObjectMeta: metav1.ObjectMeta{
				Name:      aaName,
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
						Fingerprint:      "fp-escalation-wiring",
						Severity:         "critical",
						SignalName:       "OperatorEscalationWiringTest",
						Environment:      "staging",
						BusinessPriority: "P1",
						TargetResource: aianalysisv1.TargetResource{
							Kind:      "Deployment",
							Name:      "test-deploy",
							Namespace: testNamespace,
						},
						EnrichmentResults: sharedtypes.EnrichmentResults{},
					},
					AnalysisTypes: []aianalysisv1.AnalysisType{aianalysisv1.AnalysisTypeInvestigation},
				},
			},
		}
		Expect(k8sClient.Create(ctx, analysis)).To(Succeed())

		Expect(k8sretry.RetryOnConflict(k8sretry.DefaultRetry, func() error {
			if err := k8sClient.Get(ctx, client.ObjectKeyFromObject(analysis), analysis); err != nil {
				return err
			}
			now := metav1.Now()
			analysis.Status.Phase = aianalysisv1.PhaseInvestigating
			analysis.Status.KASession = &aianalysisv1.KASession{
				ID:          sessionID,
				Interactive: false,
				CreatedAt:   &now,
			}
			return k8sClient.Status().Update(ctx, analysis)
		})).To(Succeed())

		By("waiting for reconciler to process KA result through ResponseProcessor")
		Eventually(func(g Gomega) {
			g.Expect(k8sClient.Get(ctx, client.ObjectKeyFromObject(analysis), analysis)).To(Succeed())
			g.Expect(analysis.Status.Phase).To(Equal(aianalysis.PhaseFailed),
				"Phase must transition to Failed for operator escalation")
			g.Expect(analysis.Status.NeedsHumanReview).To(BeTrue(),
				"NeedsHumanReview must be set by ResponseProcessor")
			g.Expect(analysis.Status.HumanReviewReason).To(Equal("operator_escalation"),
				"HumanReviewReason must flow from KA response through ResponseProcessor to CRD status")
			g.Expect(analysis.Status.SubReason).To(Equal("OperatorEscalation"),
				"SubReason must be mapped from operator_escalation via mapEnumToSubReason")
			g.Expect(analysis.Status.Reason).To(Equal(aianalysisv1.ReasonWorkflowResolutionFailed),
				"Reason must be WorkflowResolutionFailed for escalation path")
		}, timeout, interval).Should(Succeed(),
			"IT-AA-1449-003: Full escalation wiring path must work: KA result → reconciler → ResponseProcessor → CRD status")

		By("verifying mock KA client was called (proves wiring through handler)")
		Expect(mockClient.GetPollCallCount()).To(BeNumerically(">=", 1),
			"PollSession must have been called at least once by the reconciler")
	})
})
