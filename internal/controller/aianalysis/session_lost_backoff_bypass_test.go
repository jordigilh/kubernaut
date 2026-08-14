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

// Issue #2080 recurrence: handleSessionLost's exponential backoff
// (RequeueAfter, added by the original #2080/#2079 fix) is silently
// defeated by this controller's own self-watch predicate
// (aiAnalysisUpdatePredicate) -- it wakes the reconciler IMMEDIATELY
// whenever Status.KASession.ID changes, which handleSessionLost's own
// regeneration writes do on every single attempt (clears ID on loss, sets a
// fresh UUID on resubmit). Evidence: CI runs 31454349115 (2026-08-11, the
// validation run for the original fix's own backport PR #2087) and
// 31695817033 (2026-08-13, unrelated #2117 merge) both show
// E2E-FP-1189-005 failing with "Session regeneration cap exceeded (5
// regenerations)" -- all 5 generations completing within ~1-2 wall-clock
// seconds instead of the documented ~1s/2s/4s/8s/16s (~31s) backoff
// sequence, because the self-watch re-triggers reconcileInvestigating long
// before RequeueAfter elapses.
//
// White-box (package aianalysis, not aianalysis_test): needs direct access
// to reconcileInvestigating and the Phase* constants, mirroring
// schema_rejection_retry_test.go's pattern. These tests call
// reconcileInvestigating directly, in a tight loop with NO sleep between
// calls -- this deterministically models the worst case of the self-watch
// bypass (wake-ups arriving far faster than any real backoff could allow)
// without depending on wall-clock timing or flaky sleeps.
//
// Business-level framing:
//   - FedRAMP SI-11 (Error Handling): a session-lifecycle race must not
//     surface as a permanent investigation failure when the underlying
//     investigation already succeeded -- the same continuity guarantee
//     #2080's original fix established, now made durable against ANY
//     wake-up source, not just the specific 404-cascade timing it was
//     tested against.
//   - FedRAMP SI-4/AU-2/AU-3: RecordAIAgentSessionLost must fire at most
//     once per genuine regeneration, not once per spurious early wake-up --
//     an inflated SessionLost audit trail would misrepresent how many times
//     the KA session was actually lost.
package aianalysis

import (
	"context"
	"errors"
	"fmt"
	"sync/atomic"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	aianalysisv1 "github.com/jordigilh/kubernaut/api/aianalysis/v1alpha1"
	"github.com/jordigilh/kubernaut/pkg/agentclient"
	"github.com/jordigilh/kubernaut/pkg/aianalysis/handlers"
)

// fakeKAClientSessionLost404 implements handlers.AgentClientInterface for
// session-mode tests: SubmitInvestigation always succeeds with a fresh
// session ID, PollSession always 404s. This is the exact shape of the
// #2080 recurrence -- a KA session that is never successfully polled (either
// because it was already evicted, or because the test simply never arms a
// success response) always drives handleSessionLost's regeneration path.
type fakeKAClientSessionLost404 struct {
	submitCount int32
}

func (f *fakeKAClientSessionLost404) Investigate(_ context.Context, _ *agentclient.IncidentRequest) (*agentclient.IncidentResponse, error) {
	return nil, errors.New("not implemented in test double (#2080 recurrence): batch mode unused")
}

func (f *fakeKAClientSessionLost404) SubmitInvestigation(_ context.Context, _ *agentclient.IncidentRequest) (string, error) {
	n := atomic.AddInt32(&f.submitCount, 1)
	return fmt.Sprintf("ka-session-gen-%d", n), nil
}

func (f *fakeKAClientSessionLost404) PollSession(_ context.Context, _ string) (*agentclient.SessionStatusResult, error) {
	return nil, &agentclient.APIError{StatusCode: 404, Message: "Session not found"}
}

func (f *fakeKAClientSessionLost404) GetSessionResult(_ context.Context, _ string) (*agentclient.IncidentResponse, error) {
	return nil, errors.New("not implemented in test double (#2080 recurrence): unused")
}

func (f *fakeKAClientSessionLost404) CancelSession(_ context.Context, _ string) error { return nil }

func newSessionLostBackoffTestAnalysis(name string, session *aianalysisv1.KASession) *aianalysisv1.AIAnalysis {
	analysis := newSchemaRejectionTestAnalysis(name, PhaseInvestigating)
	analysis.Status.KASession = session
	analysis.Status.InteractiveSession = nil
	return analysis
}

// newSessionLostBackoffTestReconciler mirrors newSchemaRejectionTestReconciler
// but wires the InvestigatingHandler in session mode against the given KA
// client double, matching production's async submit/poll configuration
// (BR-AA-HAPI-064) instead of the legacy synchronous Investigate() path.
func newSessionLostBackoffTestReconciler(k8sClient client.Client, kaClient handlers.AgentClientInterface) *AIAnalysisReconciler {
	r := newSchemaRejectionTestReconciler(k8sClient)
	r.InvestigatingHandler.Store(handlers.NewInvestigatingHandler(
		kaClient, r.Log, r.Metrics, noopAuditClient{},
		handlers.WithSessionMode(),
	))
	return r
}

var _ = Describe("Session-lost regeneration backoff durability (#2080 recurrence)", func() {
	var ctx context.Context

	BeforeEach(func() {
		ctx = context.Background()
	})

	It("UT-AA-2080-006: repeated immediate reconciles (simulating the self-watch bypass) must not exhaust the regeneration cap faster than the backoff allows", func() {
		scheme := newSchemaRejectionTestScheme()
		analysis := newSessionLostBackoffTestAnalysis("ut-2080-006", &aianalysisv1.KASession{
			ID:         "ka-session-initial",
			Generation: 0,
		})

		k8sClient := fake.NewClientBuilder().
			WithScheme(scheme).
			WithStatusSubresource(&aianalysisv1.AIAnalysis{}).
			WithObjects(analysis).
			Build()

		kaClient := &fakeKAClientSessionLost404{}
		r := newSessionLostBackoffTestReconciler(k8sClient, kaClient)

		// Simulate 12 immediate, back-to-back reconciles with NO sleep in
		// between -- exactly what the self-watch predicate (KASession.ID
		// changing on every regeneration write) produces in production,
		// bypassing RequeueAfter entirely. 12 is comfortably more than the
		// ~10 calls (submit, poll-lost) x 5 needed to exhaust the
		// 5-regeneration cap under the pre-fix code, since submit and the
		// subsequent poll-404 land on alternating reconciles. A
		// correctly-durable backoff must absorb ALL of these early wake-ups
		// after the very first one, instead of burning a regeneration on
		// each one.
		for i := 0; i < 12; i++ {
			var fresh aianalysisv1.AIAnalysis
			Expect(k8sClient.Get(ctx, client.ObjectKeyFromObject(analysis), &fresh)).To(Succeed())
			_, err := r.reconcileInvestigating(ctx, &fresh)
			Expect(err).NotTo(HaveOccurred(), "reconcile %d must not error", i)
		}

		var final aianalysisv1.AIAnalysis
		Expect(k8sClient.Get(ctx, client.ObjectKeyFromObject(analysis), &final)).To(Succeed())

		Expect(final.Status.Phase).NotTo(Equal(PhaseFailed),
			"5 immediate back-to-back wake-ups (no elapsed time) must not exhaust the regeneration cap -- "+
				"the backoff must be durable against early wake-ups, not just self-computed and hoped-for")
		Expect(final.Status.KASession.Generation).To(BeNumerically("<", 5),
			"generation must not reach the cap when every wake-up arrives well inside the intended backoff window")
	})

	It("UT-AA-2080-007: reconcileInvestigating skips the handler entirely while BackoffUntil is still in the future", func() {
		scheme := newSchemaRejectionTestScheme()
		future := metav1.NewTime(time.Now().Add(5 * time.Second))
		analysis := newSessionLostBackoffTestAnalysis("ut-2080-007", &aianalysisv1.KASession{
			ID:           "",
			Generation:   1,
			BackoffUntil: &future,
		})

		k8sClient := fake.NewClientBuilder().
			WithScheme(scheme).
			WithStatusSubresource(&aianalysisv1.AIAnalysis{}).
			WithObjects(analysis).
			Build()

		kaClient := &fakeKAClientSessionLost404{}
		r := newSessionLostBackoffTestReconciler(k8sClient, kaClient)

		result, err := r.reconcileInvestigating(ctx, analysis)
		Expect(err).NotTo(HaveOccurred())
		Expect(result.RequeueAfter).To(BeNumerically(">", 0),
			"must requeue for the remaining backoff, not fail-close or spin")
		Expect(result.RequeueAfter).To(BeNumerically("<=", 5*time.Second))

		Expect(atomic.LoadInt32(&kaClient.submitCount)).To(Equal(int32(0)),
			"the handler must not run at all while still inside the backoff window -- no new session may be submitted")

		var fresh aianalysisv1.AIAnalysis
		Expect(k8sClient.Get(ctx, client.ObjectKeyFromObject(analysis), &fresh)).To(Succeed())
		Expect(fresh.Status.KASession.Generation).To(Equal(int32(1)),
			"generation must not advance while the backoff guard is skipping the handler")
	})

	It("UT-AA-2080-008: reconcileInvestigating resumes normally once BackoffUntil has elapsed", func() {
		scheme := newSchemaRejectionTestScheme()
		past := metav1.NewTime(time.Now().Add(-1 * time.Second))
		analysis := newSessionLostBackoffTestAnalysis("ut-2080-008", &aianalysisv1.KASession{
			ID:           "",
			Generation:   1,
			BackoffUntil: &past,
		})

		k8sClient := fake.NewClientBuilder().
			WithScheme(scheme).
			WithStatusSubresource(&aianalysisv1.AIAnalysis{}).
			WithObjects(analysis).
			Build()

		kaClient := &fakeKAClientSessionLost404{}
		r := newSessionLostBackoffTestReconciler(k8sClient, kaClient)

		_, err := r.reconcileInvestigating(ctx, analysis)
		Expect(err).NotTo(HaveOccurred())

		Expect(atomic.LoadInt32(&kaClient.submitCount)).To(Equal(int32(1)),
			"once the backoff window has elapsed, the handler must resume normal processing (resubmit)")

		var fresh aianalysisv1.AIAnalysis
		Expect(k8sClient.Get(ctx, client.ObjectKeyFromObject(analysis), &fresh)).To(Succeed())
		Expect(fresh.Status.KASession.ID).To(Equal("ka-session-gen-1"))
	})
})
