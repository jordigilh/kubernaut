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

// Issue #2080: session-regeneration races with AF-correlated-session
// adoption, exhausting the regeneration cap and failing investigations that
// already succeeded.
//
// handleSessionLost (reached from a 404 on PollSession/GetSessionResult) is
// the one remaining call site in the #2029 Part B adoption family that never
// re-checked whether AF had already correlated a different, currently-active
// KA session before treating the 404 as loss. A rapid, legitimate sequence
// of session hand-offs (autonomous->interactive upgrade, one or more
// AF-correlated takeovers) can each independently 404 on an
// already-superseded session ID, incrementing Generation and exhausting the
// 5-regeneration cap before adoption ever gets another chance to run --
// permanently failing an AIAnalysis whose real investigation had already
// completed.
//
// Business-level framing:
//   - FedRAMP IR-4 (Incident Handling): a completed investigation must not be
//     discarded because of a session-lifecycle race -- this is the same
//     continuity guarantee #2029 Part B established for the poll-completed/
//     poll-failed paths, now closed for the session-lost path too.
//   - FedRAMP SI-4/AU-2/AU-3: the adoption remains an observable, durably
//     audited event (unchanged: adoptCorrelatedSession's existing K8s Event +
//     RecordAIAgentCall("session_adopted")) -- these tests verify that path
//     fires instead of the SessionLost path, not a new audit mechanism.
//   - The added backoff (RequeueAfter, not immediate Requeue) gives a
//     legitimate multi-hop hand-off room to settle before the next
//     regeneration attempt is even considered, directly addressing the
//     incident's sub-2-second cascade window.
package aianalysis_test

import (
	"context"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"k8s.io/client-go/tools/record"
	ctrl "sigs.k8s.io/controller-runtime"

	"github.com/jordigilh/kubernaut/internal/controller/aianalysis"
	"github.com/jordigilh/kubernaut/pkg/agentclient"
	"github.com/jordigilh/kubernaut/pkg/aianalysis/handlers"
	"github.com/jordigilh/kubernaut/pkg/aianalysis/metrics"
	"github.com/jordigilh/kubernaut/test/shared/mocks"
)

var _ = Describe("InvestigatingHandler Session-Lost Correlation Adoption (#2080)", func() {
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

	// Models the incident's exact race: the general mismatch check at the top
	// of handleSessionBased runs before AF's correlation write lands
	// (correlatedSequence's first entry), so polling proceeds against the
	// now-stale session. By the time PollSession 404s, AF's correlation has
	// landed (second entry) -- handleSessionLost's new re-check must adopt
	// instead of treating this as loss.
	Context("UT-AA-2080-001: race-closing adoption in handleSessionPollError (404)", func() {
		It("adopts the newer correlated session instead of incrementing Generation", func() {
			isChecker := &mockISChecker{
				hasSession: true,
				correlatedSequence: []correlatedSessionStub{
					{id: "", active: false},                  // general mismatch check: not landed yet
					{id: "ka-session-new-001", active: true}, // session-lost re-check: landed now
				},
			}
			mockClient.WithSessionPollError(&agentclient.APIError{StatusCode: 404, Message: "Session not found"})

			handler := handlers.NewInvestigatingHandler(
				mockClient, ctrl.Log.WithName("test-2080-001"), metrics.NewMetrics(), auditSpy,
				handlers.WithSessionMode(),
				handlers.WithRecorder(recorder),
				handlers.WithInvestigationSessionChecker(isChecker),
			)

			analysis := adoptionTestAnalysis("rr-2080-001", "ka-session-old-001", true)

			result, err := handler.Handle(ctx, analysis)
			Expect(err).NotTo(HaveOccurred())
			Expect(result.Requeue).To(BeTrue(), "must requeue immediately to poll the newly adopted session")

			By("verifying the business protection: a real/still-running session is never counted as a regeneration")
			Expect(analysis.Status.KASession.ID).To(Equal("ka-session-new-001"))
			Expect(analysis.Status.KASession.Generation).To(Equal(int32(0)),
				"adoption on a 404 is not a regeneration -- the session was never actually lost")
			Expect(analysis.Status.Phase).NotTo(Equal(aianalysis.PhaseFailed))

			By("verifying no SessionLost audit/event fired for what turned out to be an adoption")
			Expect(auditSpy.sessionLostEvents).To(BeEmpty())
			Expect(auditSpy.agentCallEvents).To(HaveLen(1))
			Expect(auditSpy.agentCallEvents[0].endpoint).To(Equal("session_adopted"))
		})
	})

	// Symmetric to UT-AA-2080-001: closes the same race for the
	// GetSessionResult 404 path (handleSessionGetResultError). Requires 3
	// correlatedSequence entries because handleSessionPollCompleted's own
	// #2029 Part B re-check (checkCorrelatedSessionBeforeFinalizing) sits
	// between the general mismatch check and the GetSessionResult call.
	Context("UT-AA-2080-002: race-closing adoption in handleSessionGetResultError (404)", func() {
		It("adopts the newer correlated session instead of incrementing Generation", func() {
			isChecker := &mockISChecker{
				hasSession: true,
				correlatedSequence: []correlatedSessionStub{
					{id: "", active: false}, // general mismatch check: not landed yet
					{id: "", active: false}, // finalize-recheck in handleSessionPollCompleted: still not landed
					{id: "ka-session-new-002", active: true}, // session-lost re-check: landed now
				},
			}
			mockClient.WithSessionPollStatus("completed")
			mockClient.WithSessionResultError(&agentclient.APIError{StatusCode: 404, Message: "Session not found"})

			handler := handlers.NewInvestigatingHandler(
				mockClient, ctrl.Log.WithName("test-2080-002"), metrics.NewMetrics(), auditSpy,
				handlers.WithSessionMode(),
				handlers.WithRecorder(recorder),
				handlers.WithInvestigationSessionChecker(isChecker),
			)

			analysis := adoptionTestAnalysis("rr-2080-002", "ka-session-old-002", true)

			result, err := handler.Handle(ctx, analysis)
			Expect(err).NotTo(HaveOccurred())
			Expect(result.Requeue).To(BeTrue())

			By("verifying the business protection: a real/still-running session is never counted as a regeneration")
			Expect(analysis.Status.KASession.ID).To(Equal("ka-session-new-002"))
			Expect(analysis.Status.KASession.Generation).To(Equal(int32(0)))
			Expect(analysis.Status.Phase).NotTo(Equal(aianalysis.PhaseFailed))
			Expect(mockClient.GetResultCallCount).To(Equal(1), "GetSessionResult must have been attempted (and 404'd) before adoption fires")

			Expect(auditSpy.sessionLostEvents).To(BeEmpty())
			Expect(auditSpy.agentCallEvents).To(HaveLen(1))
			Expect(auditSpy.agentCallEvents[0].endpoint).To(Equal("session_adopted"))
		})
	})

	// Regression guard: an isChecker IS configured (interactive mode), but
	// genuinely reports no active correlation for this 404 -- the new
	// re-check must fall through to the existing regeneration behavior
	// unchanged (this, plus UT-AA-064-008/009/010 which run with no isChecker
	// configured at all, together prove the #2080 change is fully additive).
	Context("UT-AA-2080-003: regression -- no correlation available, session genuinely lost", func() {
		It("still increments Generation and regenerates when nothing can be adopted", func() {
			isChecker := &mockISChecker{
				hasSession:       true,
				correlatedActive: false, // no correlation recorded for this RR
			}
			mockClient.WithSessionPollError(&agentclient.APIError{StatusCode: 404, Message: "Session not found"})

			handler := handlers.NewInvestigatingHandler(
				mockClient, ctrl.Log.WithName("test-2080-003"), metrics.NewMetrics(), auditSpy,
				handlers.WithSessionMode(),
				handlers.WithRecorder(recorder),
				handlers.WithInvestigationSessionChecker(isChecker),
			)

			analysis := adoptionTestAnalysis("rr-2080-003", "ka-session-003", true)

			result, err := handler.Handle(ctx, analysis)
			Expect(err).NotTo(HaveOccurred())
			Expect(analysis.Status.KASession.Generation).To(Equal(int32(1)), "must still regenerate when adoption genuinely finds nothing")
			Expect(analysis.Status.KASession.ID).To(BeEmpty())
			Expect(result.RequeueAfter).To(BeNumerically(">", 0), "#2080: regeneration retry now backs off instead of an immediate tight-loop requeue")
			Expect(auditSpy.sessionLostEvents).To(HaveLen(1))
		})
	})

	// #2080 Option 2: the regeneration retry must back off (RequeueAfter)
	// rather than requeue immediately (Requeue: true) -- gives a legitimate
	// hand-off cascade room to settle before the next regeneration attempt,
	// shrinking the exact race window UT-AA-2080-001/002 close structurally.
	// Reuses the shared, DD-SHARED-001-compliant ErrorClassifier.GetRetryDelay
	// already used by handleError (BR-AI-009), keyed on session.Generation
	// instead of ConsecutiveFailures.
	Context("UT-AA-2080-004: session-lost regeneration backs off instead of tight-looping", func() {
		It("requeues with a positive, generation-scaled delay, not immediately", func() {
			analysis := adoptionTestAnalysis("rr-2080-004", "ka-session-004", false)
			analysis.Status.KASession.Generation = 0

			mockClient.WithSessionPollError(&agentclient.APIError{StatusCode: 404, Message: "Session not found"})

			handler := handlers.NewInvestigatingHandler(
				mockClient, ctrl.Log.WithName("test-2080-004"), metrics.NewMetrics(), auditSpy,
				handlers.WithSessionMode(),
				handlers.WithRecorder(recorder),
			)

			result, err := handler.Handle(ctx, analysis)
			Expect(err).NotTo(HaveOccurred())
			Expect(analysis.Status.KASession.Generation).To(Equal(int32(1)))
			Expect(result.Requeue).To(BeFalse(), "must use RequeueAfter backoff, not an immediate Requeue")
			Expect(result.RequeueAfter).To(BeNumerically(">", 500*time.Millisecond),
				"backoff for the first regeneration must be meaningfully non-zero")
			Expect(result.RequeueAfter).To(BeNumerically("<", 10*time.Second),
				"backoff must stay well under the investigation's overall time budget")
		})
	})

	// Incident reproduction at unit level: a cascade of 4 rapid, legitimate
	// hand-off-correlated 404s (one below the 5-regeneration cap) must each
	// adopt rather than regenerate, so Generation never moves and the
	// AIAnalysis never fails -- directly modeling the incident's "6 sessions
	// in 1.1s, cap exhausted 440ms after the real session had already
	// completed" timeline.
	Context("UT-AA-2080-005: repeated hand-off cascade never exhausts the regeneration cap", func() {
		It("adopts on every 404 in the cascade instead of accumulating Generation", func() {
			handOffs := []string{"ka-session-hop-1", "ka-session-hop-2", "ka-session-hop-3", "ka-session-hop-4"}
			var sequence []correlatedSessionStub
			for _, id := range handOffs {
				sequence = append(sequence,
					correlatedSessionStub{id: "", active: false}, // general mismatch check: not landed yet
					correlatedSessionStub{id: id, active: true},  // session-lost re-check: landed
				)
			}
			isChecker := &mockISChecker{hasSession: true, correlatedSequence: sequence}
			mockClient.WithSessionPollError(&agentclient.APIError{StatusCode: 404, Message: "Session not found"})

			handler := handlers.NewInvestigatingHandler(
				mockClient, ctrl.Log.WithName("test-2080-005"), metrics.NewMetrics(), auditSpy,
				handlers.WithSessionMode(),
				handlers.WithRecorder(recorder),
				handlers.WithInvestigationSessionChecker(isChecker),
			)

			analysis := adoptionTestAnalysis("rr-2080-005", "ka-session-old-005", true)

			for i, id := range handOffs {
				result, err := handler.Handle(ctx, analysis)
				Expect(err).NotTo(HaveOccurred())
				Expect(result.Requeue).To(BeTrue(), "hop %d must requeue immediately to poll the newly adopted session", i)
				Expect(analysis.Status.KASession.ID).To(Equal(id), "hop %d must adopt the correlated session", i)
			}

			Expect(analysis.Status.KASession.Generation).To(Equal(int32(0)),
				"none of the 4 hand-offs should have counted as a regeneration")
			Expect(analysis.Status.Phase).NotTo(Equal(aianalysis.PhaseFailed),
				"the cap (5) must never be threatened by a cascade of legitimate adoptions")
			Expect(auditSpy.sessionLostEvents).To(BeEmpty())
			Expect(auditSpy.agentCallEvents).To(HaveLen(len(handOffs)))
		})
	})
})
