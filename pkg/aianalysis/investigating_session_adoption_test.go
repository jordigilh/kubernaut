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

// Issue #2029 Part B: AA must adopt a live KA session that AF correlated onto
// this RR's InvestigationSession (status.kaCorrelationID) after AA's own
// tracked session went stale (timeout, reconnect, takeover) -- instead of
// silently discarding a real, possibly-completed investigation.
//
// Business-level framing (not just implementation mechanics):
//   - FedRAMP SI-4 (Information System Monitoring): a correlation change is a
//     security/operationally-relevant state change and must be detected and
//     acted on, not silently dropped (extends #1449's SI-4 rationale from
//     terminal-phase-only to correlation changes).
//   - FedRAMP AU-2/AU-3 (Audit Events / Content of Audit Records): the
//     adoption itself is durably recorded (RecordAIAgentCall("session_adopted"))
//     so there is a traceable record that a real investigation was rescued
//     rather than silently discarded as "decision_expired".
//   - FedRAMP IR-4 (Incident Handling): this is fundamentally an incident
//     analysis continuity guarantee -- a human's completed investigation must
//     not be lost to a race between session expiry and reconnection.
package aianalysis_test

import (
	"context"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/tools/record"
	ctrl "sigs.k8s.io/controller-runtime"

	aianalysisv1 "github.com/jordigilh/kubernaut/api/aianalysis/v1alpha1"
	"github.com/jordigilh/kubernaut/internal/controller/aianalysis"
	"github.com/jordigilh/kubernaut/pkg/agentclient"
	"github.com/jordigilh/kubernaut/pkg/aianalysis/handlers"
	"github.com/jordigilh/kubernaut/pkg/aianalysis/metrics"
	"github.com/jordigilh/kubernaut/test/shared/mocks"
)

// adoptionTestAnalysis builds a minimal, valid Investigating AIAnalysis with
// an existing KA session, for #2029 Part B adoption tests.
func adoptionTestAnalysis(rrName, sessionID string, interactive bool) *aianalysisv1.AIAnalysis {
	now := metav1.Now()
	return &aianalysisv1.AIAnalysis{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-adoption-analysis",
			Namespace: "default",
		},
		Spec: aianalysisv1.AIAnalysisSpec{
			RemediationRequestRef: corev1.ObjectReference{
				Kind:      "RemediationRequest",
				Name:      rrName,
				Namespace: "default",
			},
			RemediationID: "test-remediation-2029",
			AnalysisRequest: aianalysisv1.AnalysisRequest{
				SignalContext: aianalysisv1.SignalContextInput{
					Fingerprint:      "fp-2029",
					Severity:         "critical",
					SignalName:       "OOMKilled",
					Environment:      "production",
					BusinessPriority: "P0",
					TargetResource: aianalysisv1.TargetResource{
						Kind:      "Deployment",
						Name:      "api-server",
						Namespace: "default",
					},
				},
				AnalysisTypes: []aianalysisv1.AnalysisType{aianalysisv1.AnalysisTypeInvestigation},
			},
		},
		Status: aianalysisv1.AIAnalysisStatus{
			Phase: aianalysis.PhaseInvestigating,
			KASession: &aianalysisv1.KASession{
				ID:          sessionID,
				Interactive: interactive,
				PollCount:   3,
				CreatedAt:   &now,
			},
		},
	}
}

var _ = Describe("InvestigatingHandler Session Correlation Adoption (#2029 Part B)", func() {
	var (
		ctx        context.Context
		mockClient *mocks.MockAgentClient
		auditSpy   *sessionAuditSpy
		recorder   *record.FakeRecorder
	)

	BeforeEach(func() {
		ctx = context.Background()
		mockClient = mocks.NewMockAgentClient()
		auditSpy = &sessionAuditSpy{}
		recorder = record.NewFakeRecorder(20)
	})

	// This is the incident's actual steady-state gap: IS is active and AA is
	// already Interactive=true, so neither of checkISMismatchAndCancel's
	// existing cases (upgrade / cancel) fires. Without this new case the
	// correlation change is silently ignored until the CorrelatedSessionID
	// check is added.
	Context("UT-AA-2029-009: general mismatch adoption (hasIS && Interactive, correlated ID differs)", func() {
		It("adopts the correlated session instead of continuing to poll the stale one", func() {
			isChecker := &mockISChecker{
				hasSession:       true,
				correlatedID:     "ka-session-new-009",
				correlatedActive: true,
			}
			phaseUpdater := &mockISPhaseUpdater{}
			mockClient.WithSessionPollStatus("investigating") // must never be reached -- PollCallCount asserted below

			handler := handlers.NewInvestigatingHandler(
				mockClient, ctrl.Log.WithName("test-2029-009"), metrics.NewMetrics(), auditSpy,
				handlers.WithSessionMode(),
				handlers.WithRecorder(recorder),
				handlers.WithInvestigationSessionChecker(isChecker),
				handlers.WithISPhaseUpdater(phaseUpdater),
			)

			analysis := adoptionTestAnalysis("rr-2029-009", "ka-session-old-009", true)
			oldCreatedAt := analysis.Status.KASession.CreatedAt

			result, err := handler.Handle(ctx, analysis)
			Expect(err).NotTo(HaveOccurred())
			Expect(result.Requeue).To(BeTrue(), "must requeue immediately to poll the newly adopted session")

			By("verifying the business outcome: AA now tracks the session AF/KA actually has live")
			Expect(analysis.Status.KASession.ID).To(Equal("ka-session-new-009"))
			Expect(analysis.Status.KASession.Interactive).To(BeTrue())
			Expect(analysis.Status.KASession.PollCount).To(Equal(int32(0)), "poll tracking must reset for the newly adopted session")
			Expect(analysis.Status.KASession.LastPolled).To(BeNil())
			Expect(analysis.Status.KASession.CreatedAt).NotTo(Equal(oldCreatedAt), "CreatedAt must refresh to the adoption time")
			Expect(analysis.Status.KASession.Generation).To(Equal(int32(0)),
				"adoption is not a regeneration -- AA is catching up to existing KA work, so Generation must stay unchanged")
			Expect(mockClient.PollCallCount).To(Equal(0), "must adopt before ever polling the stale session")

			By("verifying FedRAMP AU-2/AU-3: the adoption is durably audited, not just logged")
			Expect(auditSpy.agentCallEvents).To(HaveLen(1))
			Expect(auditSpy.agentCallEvents[0].endpoint).To(Equal("session_adopted"))
			Expect(auditSpy.agentCallEvents[0].statusCode).To(Equal(200))

			By("verifying FedRAMP SI-4: the correlation change is an observable event, not a silent state swap")
			var evt string
			Eventually(recorder.Events).Should(Receive(&evt))
			Expect(evt).To(ContainSubstring("SessionAdopted"))
			Expect(evt).To(ContainSubstring("ka-session-new-009"))
		})

		It("is a no-op when CorrelatedSessionID matches the currently tracked session (no regression)", func() {
			isChecker := &mockISChecker{
				hasSession:       true,
				correlatedID:     "ka-session-current",
				correlatedActive: true,
			}
			mockClient.WithSessionPollStatus("investigating")

			handler := handlers.NewInvestigatingHandler(
				mockClient, ctrl.Log.WithName("test-2029-009-noop"), metrics.NewMetrics(), auditSpy,
				handlers.WithSessionMode(),
				handlers.WithRecorder(recorder),
				handlers.WithInvestigationSessionChecker(isChecker),
			)

			analysis := adoptionTestAnalysis("rr-2029-009-noop", "ka-session-current", true)

			result, err := handler.Handle(ctx, analysis)
			Expect(err).NotTo(HaveOccurred())
			Expect(result.RequeueAfter).To(BeNumerically(">", 0), "should proceed to normal poll, not adopt")
			Expect(mockClient.PollCallCount).To(Equal(1))
			Expect(auditSpy.agentCallEvents).To(BeEmpty(), "no adoption audit event when nothing was adopted")
		})
	})

	// Models the incident's exact race: by the time checkISMismatchAndCancel
	// ran, AF's correlation write had not landed yet (correlatedSequence's
	// first entry). Moments later, PollSession returns "completed" for the
	// now-stale session -- but AF's correlation write has landed in the
	// meantime (correlatedSequence's second entry), so the re-check inside
	// handleSessionPollCompleted must catch it.
	Context("UT-AA-2029-010: race-closing adoption in handleSessionPollCompleted", func() {
		It("adopts the newer session instead of finalizing the stale one as Completed", func() {
			isChecker := &mockISChecker{
				hasSession: true,
				correlatedSequence: []correlatedSessionStub{
					{id: "", active: false},                          // general check: correlation not landed yet
					{id: "ka-session-new-010", active: true},         // race-closing check: it has landed now
				},
			}
			phaseUpdater := &mockISPhaseUpdater{}
			mockClient.WithSessionPollStatus("completed")
			mockClient.Response = &agentclient.IncidentResponse{
				IncidentID:        "mock-2029-010",
				Analysis:          "should never be persisted for the stale session",
				RootCauseAnalysis: mocks.BuildMockRCA("should never be persisted", "high", nil),
				Confidence:        0.5,
				Timestamp:         "2026-08-09T00:53:11Z",
			}

			handler := handlers.NewInvestigatingHandler(
				mockClient, ctrl.Log.WithName("test-2029-010"), metrics.NewMetrics(), auditSpy,
				handlers.WithSessionMode(),
				handlers.WithRecorder(recorder),
				handlers.WithInvestigationSessionChecker(isChecker),
				handlers.WithISPhaseUpdater(phaseUpdater),
			)

			analysis := adoptionTestAnalysis("rr-2029-010", "ka-session-old-010", true)

			result, err := handler.Handle(ctx, analysis)
			Expect(err).NotTo(HaveOccurred())
			Expect(result.Requeue).To(BeTrue())

			By("verifying the business protection: a real investigation is never discarded as decision_expired")
			Expect(analysis.Status.Phase).To(Equal(aianalysis.PhaseInvestigating),
				"must NOT transition to a terminal phase for the stale session (this was the #2029 incident's exact bug)")
			Expect(mockClient.GetResultCallCount).To(Equal(0), "must not fetch/persist a result for the stale session")
			Expect(phaseUpdater.getTerminalPhaseCalls()).To(BeEmpty(),
				"the live, newly-adopted IS must never be marked terminal because of the stale session's poll")
			Expect(analysis.Status.KASession.ID).To(Equal("ka-session-new-010"))

			By("verifying FedRAMP AU-2/AU-3: the rescue is durably audited")
			Expect(auditSpy.agentCallEvents).To(HaveLen(1))
			Expect(auditSpy.agentCallEvents[0].endpoint).To(Equal("session_adopted"))
		})
	})

	// Symmetric to UT-AA-2029-010: closes the same race for the failed-poll path.
	Context("UT-AA-2029-012: race-closing adoption in handleSessionPollFailed (symmetric to -010)", func() {
		It("adopts the newer session instead of finalizing the stale one as Failed", func() {
			isChecker := &mockISChecker{
				hasSession: true,
				correlatedSequence: []correlatedSessionStub{
					{id: "", active: false},
					{id: "ka-session-new-012", active: true},
				},
			}
			phaseUpdater := &mockISPhaseUpdater{}
			mockClient.WithSessionPollStatus("failed")
			mockClient.DefaultSessionStatus = &agentclient.SessionStatusResult{
				Status: "failed",
				Error:  "LLM timeout (stale session)",
			}

			handler := handlers.NewInvestigatingHandler(
				mockClient, ctrl.Log.WithName("test-2029-012"), metrics.NewMetrics(), auditSpy,
				handlers.WithSessionMode(),
				handlers.WithRecorder(recorder),
				handlers.WithInvestigationSessionChecker(isChecker),
				handlers.WithISPhaseUpdater(phaseUpdater),
			)

			analysis := adoptionTestAnalysis("rr-2029-012", "ka-session-old-012", true)

			result, err := handler.Handle(ctx, analysis)
			Expect(err).NotTo(HaveOccurred())
			Expect(result.Requeue).To(BeTrue())

			By("verifying the business protection: the stale session's failure must not be reported as the outcome")
			Expect(analysis.Status.Phase).To(Equal(aianalysis.PhaseInvestigating),
				"must NOT transition to Failed for a session that has actually been superseded by a newer, live one")
			Expect(phaseUpdater.getTerminalPhaseCalls()).To(BeEmpty())
			Expect(analysis.Status.KASession.ID).To(Equal("ka-session-new-012"))

			By("verifying FedRAMP AU-3: the outcome recorded is the true one (adopted), not a misleading failure")
			Expect(auditSpy.failedEvents).To(BeEmpty(),
				"RecordAnalysisFailed must NOT fire for a session that was adopted, not truly failed")
			Expect(auditSpy.agentCallEvents).To(HaveLen(1))
			Expect(auditSpy.agentCallEvents[0].endpoint).To(Equal("session_adopted"))
		})
	})
})
