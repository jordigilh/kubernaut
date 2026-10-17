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
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"

	agentsessionv1 "github.com/jordigilh/kubernaut/api/agentsession/v1alpha1"
	aianalysisv1 "github.com/jordigilh/kubernaut/api/aianalysis/v1alpha1"
	"github.com/jordigilh/kubernaut/pkg/aianalysis"
	aiaudit "github.com/jordigilh/kubernaut/pkg/aianalysis/audit"
	"github.com/jordigilh/kubernaut/pkg/aianalysis/handlers"
	sharedtypes "github.com/jordigilh/kubernaut/pkg/shared/types"
	"github.com/jordigilh/kubernaut/test/shared/helpers"
	"github.com/jordigilh/kubernaut/test/shared/mocks"
)

// IT-AA-2521-001 exercises the real AIAnalysis reconciler and
// InvestigatingHandler production dispatch with the typed AgentSession result
// boundary. The handler's external KA dependency is replaced only at that
// boundary; Kubernetes status persistence and ResponseProcessor routing are
// real.
var _ = Describe("Typed investigation outcome production wiring — #2521", Label("integration", "typed-outcome", "wiring"), Serial, func() {
	const (
		timeout  = 30 * time.Second
		interval = 200 * time.Millisecond
	)

	installTypedOutcomeHandler := func(result *agentsessionv1.AgentSessionResult) {
		savedHandler := reconciler.InvestigatingHandler.Load()
		mockClient := mocks.NewMockAgentClient().WithResult(result)
		auditClient := aiaudit.NewAuditClient(auditStore, ctrl.Log.WithName("typed-outcome-wiring-audit"))
		reconciler.InvestigatingHandler.Store(handlers.NewInvestigatingHandler(
			mockClient,
			ctrl.Log.WithName("typed-outcome-wiring-mock-handler"),
			testMetrics,
			auditClient,
			handlers.WithRecorder(k8sManager.GetEventRecorderFor("aianalysis-controller")),
		))
		DeferCleanup(func() {
			reconciler.InvestigatingHandler.Store(savedHandler)
		})
	}

	newAnalysis := func(prefix string) *aianalysisv1.AIAnalysis {
		rrName := helpers.UniqueTestName("rr-" + prefix)
		analysis := &aianalysisv1.AIAnalysis{
			ObjectMeta: metav1.ObjectMeta{
				Name:      helpers.UniqueTestName("aa-" + prefix),
				Namespace: testNamespace,
			},
			Spec: aianalysisv1.AIAnalysisSpec{
				RemediationRequestRef: corev1.ObjectReference{Name: rrName, Namespace: testNamespace},
				RemediationID:         rrName,
				AnalysisRequest: aianalysisv1.AnalysisRequest{
					SignalContext: aianalysisv1.SignalContextInput{
						Fingerprint:      helpers.UniqueTestName("fp-" + prefix),
						Severity:         "high",
						SignalName:       "TypedOutcomeWiringTest",
						SignalMode:       "reactive",
						Environment:      "staging",
						BusinessPriority: "P1",
						TargetResource: aianalysisv1.TargetResource{
							Kind: "Deployment", Name: "typed-outcome-target", Namespace: testNamespace,
						},
						EnrichmentResults: sharedtypes.EnrichmentResults{},
					},
					AnalysisTypes: []aianalysisv1.AnalysisType{aianalysisv1.AnalysisTypeInvestigation},
				},
			},
		}
		return analysis
	}

	processTypedResult := func(prefix string, result *agentsessionv1.AgentSessionResult) *aianalysisv1.AIAnalysis {
		installTypedOutcomeHandler(result)
		analysis := newAnalysis(prefix)
		Expect(k8sClient.Create(ctx, analysis)).To(Succeed())
		Eventually(func(g Gomega) {
			g.Expect(k8sClient.Get(ctx, client.ObjectKeyFromObject(analysis), analysis)).To(Succeed())
			g.Expect(analysis.Status.GetRCAResult().TerminalOutcome).To(Equal(result.TerminalOutcome))
			g.Expect(analysis.Status.GetRCAResult().WorkflowDiscoveryStatus).To(Equal(result.WorkflowDiscoveryStatus))
			g.Expect(analysis.Status.GetRCAResult().HistoryStatus).To(Equal(result.HistoryStatus))
		}, timeout, interval).Should(Succeed())
		return analysis
	}

	It("IT-AA-2521-001a: routes a selected workflow through the normal analyzing path (BR-2521)", func() {
		result := &agentsessionv1.AgentSessionResult{
			IncidentID:              "incident-2521-selected",
			Analysis:                "A remediation workflow was selected",
			Confidence:              0.95,
			Timestamp:               time.Now().UTC().Format(time.RFC3339),
			SelectedWorkflow:        mocks.BuildMockSelectedWorkflow("workflow-2521-selected", "example/image:latest", 0.95, "typed outcome wiring"),
			TerminalOutcome:         sharedtypes.TerminalOutcomeWorkflowSelected,
			WorkflowDiscoveryStatus: sharedtypes.WorkflowDiscoveryStatusCompletedSelected,
			HistoryStatus:           sharedtypes.HistoryStatusAvailable,
		}
		analysis := processTypedResult("2521-selected", result)
		Expect(analysis.Status.Phase).To(BeElementOf(aianalysis.PhaseAnalyzing, aianalysis.PhaseCompleted))
		Expect(analysis.Status.GetRCAResult().SelectedWorkflow).NotTo(BeNil())
	})

	It("IT-AA-2521-001b: completes explicit no-match only with the completed-no-match gate (BR-KA-197)", func() {
		result := &agentsessionv1.AgentSessionResult{
			IncidentID:              "incident-2521-no-match",
			Analysis:                "No acceptable remediation workflow matched",
			NeedsHumanReview:        true,
			HumanReviewReason:       aianalysisv1.HumanReviewReasonNoMatchingWorkflows,
			TerminalOutcome:         sharedtypes.TerminalOutcomeNoMatchingWorkflows,
			WorkflowDiscoveryStatus: sharedtypes.WorkflowDiscoveryStatusCompletedNoMatch,
			HistoryStatus:           sharedtypes.HistoryStatusEmpty,
			Timestamp:               time.Now().UTC().Format(time.RFC3339),
		}
		analysis := processTypedResult("2521-no-match", result)
		Expect(analysis.Status.Phase).To(Equal(aianalysis.PhaseCompleted))
		Expect(analysis.Status.SubReason).To(Equal(aianalysisv1.SubReasonNoMatchingWorkflows))
		Expect(analysis.Status.GetReview().HumanReviewReason).To(Equal(aianalysisv1.HumanReviewReasonNoMatchingWorkflows))
	})

	It("IT-AA-2521-001c: keeps discovery failure distinct from no-match (BR-2521)", func() {
		result := &agentsessionv1.AgentSessionResult{
			IncidentID:              "incident-2521-discovery-failed",
			Analysis:                "Workflow discovery could not complete",
			NeedsHumanReview:        true,
			HumanReviewReason:       "investigation_inconclusive",
			TerminalOutcome:         sharedtypes.TerminalOutcomeWorkflowDiscoveryFailed,
			WorkflowDiscoveryStatus: sharedtypes.WorkflowDiscoveryStatusFailed,
			HistoryStatus:           sharedtypes.HistoryStatusAvailable,
			Timestamp:               time.Now().UTC().Format(time.RFC3339),
		}
		analysis := processTypedResult("2521-discovery-failed", result)
		Expect(analysis.Status.Phase).To(Equal(aianalysis.PhaseFailed))
		Expect(analysis.Status.SubReason).To(Equal("InvestigationInconclusive"))
		Expect(analysis.Status.SubReason).NotTo(Equal(aianalysisv1.SubReasonNoMatchingWorkflows))
		Expect(analysis.Status.GetReview().HumanReviewReason).To(Equal("investigation_inconclusive"))
	})

	It("IT-AA-2521-001d: preserves the RCA compatibility projection for evidence transport exhaustion (BR-AUDIT-005)", func() {
		result := &agentsessionv1.AgentSessionResult{
			IncidentID:              "incident-2521-transport",
			Analysis:                "Actionable RCA retained while history transport is unavailable",
			NeedsHumanReview:        true,
			HumanReviewReason:       aianalysisv1.HumanReviewReasonRCAIncomplete,
			TerminalOutcome:         sharedtypes.TerminalOutcomeEvidenceTransportUnavailable,
			WorkflowDiscoveryStatus: sharedtypes.WorkflowDiscoveryStatusSkippedDueToFailure,
			HistoryStatus:           sharedtypes.HistoryStatusTransportUnavailable,
			Timestamp:               time.Now().UTC().Format(time.RFC3339),
		}
		analysis := processTypedResult("2521-transport", result)
		Expect(analysis.Status.Phase).To(Equal(aianalysis.PhaseFailed))
		Expect(analysis.Status.SubReason).To(Equal("RcaIncomplete"))
		Expect(analysis.Status.GetReview().HumanReviewReason).To(Equal(aianalysisv1.HumanReviewReasonRCAIncomplete))
		Expect(analysis.Status.GetRCAResult().TerminalOutcome).To(Equal(sharedtypes.TerminalOutcomeEvidenceTransportUnavailable))
	})
})
