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

package capacityretry

import (
	"context"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/prometheus/client_golang/prometheus"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes/scheme"
	"k8s.io/client-go/tools/record"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	ctrllog "sigs.k8s.io/controller-runtime/pkg/log"

	agentsessionv1 "github.com/jordigilh/kubernaut/api/agentsession/v1alpha1"
	aianalysisv1 "github.com/jordigilh/kubernaut/api/aianalysis/v1alpha1"
	"github.com/jordigilh/kubernaut/internal/controller/aianalysis"
	aiaudit "github.com/jordigilh/kubernaut/pkg/aianalysis/audit"
	"github.com/jordigilh/kubernaut/pkg/aianalysis/creator"
	"github.com/jordigilh/kubernaut/pkg/aianalysis/handlers"
	"github.com/jordigilh/kubernaut/pkg/aianalysis/metrics"
	"github.com/jordigilh/kubernaut/pkg/aianalysis/status"
	ogenclient "github.com/jordigilh/kubernaut/pkg/datastorage/ogen-client"
	sharedtypes "github.com/jordigilh/kubernaut/pkg/shared/types"
)

// ----------------------------------------------------------------------------
// Test doubles (package-local; mirrors the equivalent doubles in the sibling
// schemarejection suite -- those live in a _test.go file and are therefore
// not importable from here).
// ----------------------------------------------------------------------------

// noopAuditClient implements handlers.AuditClientInterface. The
// retry-within-budget path (retryCapacityExceeded) never calls it; the
// budget-exhausted path falls through to handleSessionFailed's permanent-fail
// branch, which does call RecordAnalysisFailed.
type noopAuditClient struct{}

func (noopAuditClient) RecordAIAgentCall(_ context.Context, _ *aianalysisv1.AIAnalysis, _ string, _ int, _ int) {
}
func (noopAuditClient) RecordPhaseTransition(_ context.Context, _ *aianalysisv1.AIAnalysis, _, _ string) {
}
func (noopAuditClient) RecordAnalysisFailed(_ context.Context, _ *aianalysisv1.AIAnalysis, _ error) error {
	return nil
}
func (noopAuditClient) RecordAnalysisComplete(_ context.Context, _ *aianalysisv1.AIAnalysis) {}
func (noopAuditClient) RecordAIAgentSubmit(_ context.Context, _ *aianalysisv1.AIAnalysis, _ string) {
}
func (noopAuditClient) RecordAIAgentResult(_ context.Context, _ *aianalysisv1.AIAnalysis, _ int64) {}

// noopAuditStore implements pkg/audit.AuditStore, backing the top-level
// controller's *audit.AuditClient (r.AuditClient) with no real DataStorage
// dependency. Only exercised when a phase actually transitions
// (finalizeInvestigatingTransition -> RecordPhaseTransition), which the
// budget-exhausted scenario below does trigger.
type noopAuditStore struct{}

func (noopAuditStore) StoreAudit(_ context.Context, _ *ogenclient.AuditEventRequest) error {
	return nil
}
func (noopAuditStore) Flush(_ context.Context) error { return nil }
func (noopAuditStore) Close() error                  { return nil }

// ----------------------------------------------------------------------------
// Fixture + reconciler helpers
// ----------------------------------------------------------------------------

// newCapacityRetryAnalysis builds an AIAnalysis fixture already mid-lifecycle
// (Investigating, with a KASession referencing the deterministic AgentSession
// name a real creator.AgentSessionCreator would use) at the given retry
// attempt count (KASession.Generation, backoff-pacing counter only) and
// session age. #2189: the capacity-exceeded retry budget is bounded by the
// investigation's own deadline (session.CreatedAt+maxInvestigationDuration,
// absent an RO-set Spec.TimesOutAt), not by attempt count -- sessionAge
// lets callers simulate either "deadline far away" (retry) or "deadline
// passed" (exhausted) independently of kaSessionGeneration.
func newCapacityRetryAnalysis(ns, name string, kaSessionGeneration int32, sessionAge time.Duration) *aianalysisv1.AIAnalysis {
	rrName := "rr-" + name
	sessionCreated := metav1.NewTime(time.Now().Add(-sessionAge))
	return &aianalysisv1.AIAnalysis{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: ns,
			// Pre-populate the finalizer as a real object mid-lifecycle would
			// have it: Reconcile()'s step 3 adds it and returns early on the
			// very first pass, before ever reaching the phase dispatch this
			// test targets.
			Finalizers: []string{aianalysis.FinalizerName},
		},
		Spec: aianalysisv1.AIAnalysisSpec{
			RemediationRequestRef: corev1.ObjectReference{Name: rrName, Namespace: ns},
			RemediationID:         rrName,
			AnalysisRequest: aianalysisv1.AnalysisRequest{
				SignalContext: aianalysisv1.SignalContextInput{
					Fingerprint:      "fp-" + name,
					Severity:         "critical",
					SignalName:       "TestSignalCapacityRetryIT",
					Environment:      "staging",
					BusinessPriority: "P1",
					TargetResource: aianalysisv1.TargetResource{
						Kind:      "Deployment",
						Name:      "test-deploy",
						Namespace: ns,
					},
					EnrichmentResults: sharedtypes.EnrichmentResults{},
				},
				AnalysisTypes: []aianalysisv1.AnalysisType{aianalysisv1.AnalysisTypeInvestigation},
			},
		},
		Status: aianalysisv1.AIAnalysisStatus{
			Phase: aianalysisv1.PhaseInvestigating,
			KASession: &aianalysisv1.KASession{
				ID:         "as-" + rrName,
				Generation: kaSessionGeneration,
				CreatedAt:  &sessionCreated,
			},
		},
	}
}

// seedFailedCapacityExceededAgentSession creates a real AgentSession CRD in
// envtest (via the same handlers.RequestBuilder the production
// creator.AgentSessionCreator uses, so the Spec is genuinely valid against
// the installed CRD schema), then writes its Status straight to
// Failed/CapacityExceeded -- modeling KA's dispatcher having already rejected
// this AIAnalysis's investigation for capacity reasons (BR-AI-009,
// DD-AA-KA-001 amendment). analysis must already be persisted (real UID) so
// the owner reference is valid.
func seedFailedCapacityExceededAgentSession(ctx context.Context, k8sClient client.WithWatch, analysis *aianalysisv1.AIAnalysis, curatedMsg string) *agentsessionv1.AgentSession {
	builder := handlers.NewRequestBuilder(ctrllog.Log)
	as := &agentsessionv1.AgentSession{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "as-" + analysis.Spec.RemediationRequestRef.Name,
			Namespace: analysis.Namespace,
		},
		Spec: builder.BuildAgentSessionSpec(analysis),
	}
	ExpectWithOffset(1, controllerutil.SetControllerReference(analysis, as, k8sClient.Scheme())).To(Succeed())
	ExpectWithOffset(1, k8sClient.Create(ctx, as)).To(Succeed())

	as.Status.Phase = agentsessionv1.AgentSessionPhaseFailed
	as.Status.Reason = agentsessionv1.AgentSessionReasonCapacityExceeded
	as.Status.Error = curatedMsg
	now := metav1.Now()
	as.Status.CompletedAt = &now
	ExpectWithOffset(1, k8sClient.Status().Update(ctx, as)).To(Succeed())
	return as
}

// newCapacityRetryReconciler wires a real AIAnalysisReconciler backed by the
// REAL production creator.AgentSessionCreator (not a fake) onto the given
// envtest-backed client -- this suite has no real KA subprocess to race with
// (unlike test/integration/aianalysis's heavy shared suite), so GetOrCreate/
// DeleteForRetry can safely touch genuine AgentSession CRDs, proving the full
// cross-component wiring end to end.
func newCapacityRetryReconciler(k8sClient client.WithWatch) *aianalysis.AIAnalysisReconciler {
	log := ctrllog.Log.WithName("capacity-retry-it")
	m := metrics.NewMetricsWithRegistry(prometheus.NewRegistry())
	auditClient := aiaudit.NewAuditClient(noopAuditStore{}, log)
	agentSessionCreator := creator.NewAgentSessionCreator(k8sClient, k8sClient.Scheme())

	r := &aianalysis.AIAnalysisReconciler{
		Client:        k8sClient,
		Scheme:        k8sClient.Scheme(),
		Recorder:      record.NewFakeRecorder(50),
		Log:           log,
		Metrics:       m,
		StatusManager: status.NewManager(k8sClient, k8sClient),
		AuditClient:   auditClient,
	}
	r.InvestigatingHandler.Store(handlers.NewInvestigatingHandler(
		agentSessionCreator, log, m, noopAuditClient{}))
	return r
}

// ----------------------------------------------------------------------------
// BR-AI-009 / DD-AA-KA-001 amendment: AgentSession capacity-exceeded retry
// ----------------------------------------------------------------------------

var _ = Describe("AgentSession capacity-exceeded retry against a real API server (BR-AI-009)", func() {
	var (
		ctx       context.Context
		ns        *corev1.Namespace
		realWatch client.WithWatch
	)

	BeforeEach(func() {
		ctx = context.Background()

		var err error
		realWatch, err = client.NewWithWatch(cfg, client.Options{Scheme: scheme.Scheme})
		Expect(err).NotTo(HaveOccurred())

		ns = &corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: "capacity-retry-it-" + randSuffix()}}
		Expect(realWatch.Create(ctx, ns)).To(Succeed())
	})

	AfterEach(func() {
		Expect(realWatch.Delete(ctx, ns)).To(Succeed())
	})

	It("IT-AA-KA-065-210: retries with backoff and deletes the stale AgentSession while the retry budget remains", func() {
		analysis := newCapacityRetryAnalysis(ns.Name, "cap-retry-within-budget", 0, 5*time.Second)
		desiredStatus := analysis.Status
		Expect(realWatch.Create(ctx, analysis)).To(Succeed())
		analysis.Status = desiredStatus
		Expect(realWatch.Status().Update(ctx, analysis)).To(Succeed())

		as := seedFailedCapacityExceededAgentSession(ctx, realWatch, analysis, "KA dispatch capacity exceeded (test, within budget)")

		r := newCapacityRetryReconciler(realWatch)
		req := ctrl.Request{NamespacedName: client.ObjectKeyFromObject(analysis)}

		By("first reconcile: a capacity-exceeded Failed AgentSession within retry budget must requeue with backoff, not permanently fail")
		result, err := r.Reconcile(ctx, req)
		Expect(err).NotTo(HaveOccurred())
		Expect(result.RequeueAfter).To(BeNumerically(">", 0),
			"must requeue with backoff instead of permanently failing while retry budget remains")

		var deleted agentsessionv1.AgentSession
		getErr := realWatch.Get(ctx, client.ObjectKeyFromObject(as), &deleted)
		Expect(apierrors.IsNotFound(getErr)).To(BeTrue(),
			"DeleteForRetry must remove the stale Failed+CapacityExceeded AgentSession so the next GetOrCreate falls through to Create")

		var fresh aianalysisv1.AIAnalysis
		Expect(realWatch.Get(ctx, req.NamespacedName, &fresh)).To(Succeed())
		Expect(fresh.Status.Phase).To(Equal(aianalysisv1.PhaseInvestigating),
			"the AIAnalysis must remain non-terminal -- a capacity rejection is not a genuine investigation failure")
		Expect(fresh.Status.KASession).NotTo(BeNil())
		Expect(fresh.Status.KASession.Generation).To(Equal(int32(1)),
			"the retry-attempt counter (KASession.Generation) must advance by exactly one")

		By("second reconcile: a brand-new AgentSession must be created under the same deterministic name")
		result2, err := r.Reconcile(ctx, req)
		Expect(err).NotTo(HaveOccurred())
		Expect(result2.RequeueAfter).To(BeNumerically(">", 0))

		var recreated agentsessionv1.AgentSession
		Expect(realWatch.Get(ctx, client.ObjectKeyFromObject(as), &recreated)).To(Succeed())
		Expect(recreated.UID).NotTo(Equal(as.UID),
			"the recreated AgentSession must be a genuinely new object, not the deleted one coming back")
		Expect(recreated.Status.Phase).To(BeEmpty(),
			"the fresh AgentSession must start with no KA-written status yet")
	})

	It("IT-AA-KA-065-211: escalates to permanent Failed once the capacity-exceeded retry budget is exhausted", func() {
		// #2189: exhaustion is now deadline-bound, not attempt-count-bound
		// -- a low Generation (1) with a session older than
		// DefaultMaxInvestigationDuration proves the investigation's own
		// deadline (not a fixed retry count) is what triggers permanent
		// failure.
		analysis := newCapacityRetryAnalysis(ns.Name, "cap-retry-exhausted", 1, handlers.DefaultMaxInvestigationDuration+time.Minute)
		desiredStatus := analysis.Status
		Expect(realWatch.Create(ctx, analysis)).To(Succeed())
		analysis.Status = desiredStatus
		Expect(realWatch.Status().Update(ctx, analysis)).To(Succeed())

		curatedMsg := "KA dispatch capacity exceeded (test, exhausted budget)"
		as := seedFailedCapacityExceededAgentSession(ctx, realWatch, analysis, curatedMsg)

		r := newCapacityRetryReconciler(realWatch)
		req := ctrl.Request{NamespacedName: client.ObjectKeyFromObject(analysis)}

		result, err := r.Reconcile(ctx, req)
		Expect(err).NotTo(HaveOccurred())
		// finalizeInvestigatingTransition's generic 100ms "settle" requeue on
		// ANY Investigating phase change (terminal or not) is pre-existing
		// controller plumbing, not part of this BR's contract -- what matters
		// here is that it is NOT the multi-second retry backoff from the
		// within-budget scenario above, confirming no retry was taken.
		Expect(result.RequeueAfter).To(BeNumerically("<", time.Second),
			"a permanent-fail transition must not be treated as a capacity-exceeded retry (no backoff-scale requeue)")

		var stillThere agentsessionv1.AgentSession
		Expect(realWatch.Get(ctx, client.ObjectKeyFromObject(as), &stillThere)).To(Succeed())
		Expect(stillThere.Status.Phase).To(Equal(agentsessionv1.AgentSessionPhaseFailed),
			"DeleteForRetry must NOT be called once the retry budget is exhausted")

		var fresh aianalysisv1.AIAnalysis
		Expect(realWatch.Get(ctx, req.NamespacedName, &fresh)).To(Succeed())
		Expect(fresh.Status.Phase).To(Equal(aianalysisv1.PhaseFailed),
			"a capacity-exceeded failure that has exhausted its retries is reported identically to any other AgentSession failure")
		Expect(fresh.Status.Message).To(Equal(curatedMsg))
	})
})

func randSuffix() string {
	return time.Now().Format("150405-000000")
}
