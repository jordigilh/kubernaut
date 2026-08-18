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

// Issue #2030 (main-tracking clone of #2029) Part A: reconcileInvestigating/
// reconcileAnalyzing must retry (with backoff, bounded) instead of
// fail-closing forever when the live cluster's installed AIAnalysis CRD
// schema rejects a Status().Update() call (apierrors.IsInvalid) -- e.g. CRD
// version skew during a rolling upgrade where the Go source already defines
// an enum value the installed CRD doesn't recognize yet. Previously this
// returned ctrl.Result{}, nil (no requeue, no error) on the FIRST rejection,
// permanently abandoning the AIAnalysis with no operator-visible recovery
// path.
//
// White-box (package aianalysis, not aianalysis_test): needs direct access
// to reconcileInvestigating/reconcileAnalyzing/handleSchemaRejectedStatusUpdate
// and the Phase* constants, none of which are exported. Runs in the same
// Ginkgo suite as predicates_test.go/suite_test.go (RunSpecs lives there;
// Describe() blocks register globally regardless of which file/package
// variant declares them).
package aianalysis

import (
	"context"
	"errors"
	"sync/atomic"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/go-logr/logr"
	"github.com/prometheus/client_golang/prometheus"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/util/validation/field"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	"k8s.io/client-go/tools/record"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/controller-runtime/pkg/client/interceptor"

	agentsessionv1 "github.com/jordigilh/kubernaut/api/agentsession/v1alpha1"
	aianalysisv1 "github.com/jordigilh/kubernaut/api/aianalysis/v1alpha1"
	aiaudit "github.com/jordigilh/kubernaut/pkg/aianalysis/audit"
	"github.com/jordigilh/kubernaut/pkg/aianalysis/handlers"
	"github.com/jordigilh/kubernaut/pkg/aianalysis/metrics"
	"github.com/jordigilh/kubernaut/pkg/aianalysis/rego"
	"github.com/jordigilh/kubernaut/pkg/aianalysis/status"
	"github.com/jordigilh/kubernaut/pkg/audit"
	ogenclient "github.com/jordigilh/kubernaut/pkg/datastorage/ogen-client"
	sharedtypes "github.com/jordigilh/kubernaut/pkg/shared/types"
)

// ----------------------------------------------------------------------------
// Test doubles
// ----------------------------------------------------------------------------

// fakeKAClientAlwaysErrors implements handlers.AgentSessionGetOrCreator.
// GetOrCreate always returns a generic (unclassified) error, which
// InvestigatingHandler's ErrorClassifier treats as a permanent error --
// naturally driving analysis.Status.Phase to Failed in memory without
// needing a real Kubernetes API server. The resulting Status().Update()
// attempt is what these tests' schema-rejection interceptors target.
type fakeKAClientAlwaysErrors struct{}

func (fakeKAClientAlwaysErrors) GetOrCreate(_ context.Context, _ *aianalysisv1.AIAnalysis) (*agentsessionv1.AgentSession, error) {
	return nil, errors.New("simulated KA outage (test double, #2030)")
}

// noopAuditClient implements handlers.AuditClientInterface. These tests
// assert on Status/annotation state, not audit emission.
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

// noopAnalyzingAuditClient implements handlers.AnalyzingAuditClientInterface.
type noopAnalyzingAuditClient struct{}

func (noopAnalyzingAuditClient) RecordRegoEvaluation(_ context.Context, _ *aianalysisv1.AIAnalysis, _ string, _ bool, _ int, _ string, _ string) {
}
func (noopAnalyzingAuditClient) RecordApprovalDecision(_ context.Context, _ *aianalysisv1.AIAnalysis, _, _ string) {
}
func (noopAnalyzingAuditClient) RecordAnalysisComplete(_ context.Context, _ *aianalysisv1.AIAnalysis) {
}
func (noopAnalyzingAuditClient) RecordAnalysisFailed(_ context.Context, _ *aianalysisv1.AIAnalysis, _ error) error {
	return nil
}

// noopAuditStore implements audit.AuditStore for r.AuditClient (a concrete
// *audit.AuditClient field on AIAnalysisReconciler, required by
// recordPhaseMetrics/RecordPhaseTransition whenever a phase change commits
// successfully -- distinct from the handlers.AuditClientInterface double
// injected into the handler constructors above).
type noopAuditStore struct{}

func (noopAuditStore) StoreAudit(_ context.Context, _ *ogenclient.AuditEventRequest) error {
	return nil
}
func (noopAuditStore) Flush(_ context.Context) error { return nil }
func (noopAuditStore) Close() error                  { return nil }

var _ audit.AuditStore = noopAuditStore{}

// neverCalledRegoEvaluator implements handlers.RegoEvaluatorInterface. Never
// invoked in these tests: AnalyzingHandler.Handle short-circuits to
// Phase=Failed/Reason=NoWorkflowSelected before evaluating Rego when
// Status.SelectedWorkflow is nil -- exactly the minimal fixture below.
type neverCalledRegoEvaluator struct{}

func (neverCalledRegoEvaluator) Evaluate(_ context.Context, _ *rego.PolicyInput) (*rego.PolicyResult, error) {
	return nil, errors.New("unexpected call (#2030 test double): fixture has no SelectedWorkflow, Handle must short-circuit first")
}

// ----------------------------------------------------------------------------
// Interceptor helpers
// ----------------------------------------------------------------------------

// failNStatusUpdates returns interceptor funcs whose SubResourceUpdate hook
// rejects the first n calls to Status().Update() with a synthetic
// apierrors.IsInvalid error (modeling a CRD schema rejection), then delegates
// to the real fake client for every call after that. A plain Update() (the
// annotation persistence path #2030 Part A relies on) is never intercepted,
// matching real Kubernetes CRD-with-status-subresource semantics: a plain
// Update() silently ignores any .status diff, so it is unaffected by
// Status().Update() being rejected.
func failNStatusUpdates(n int32) interceptor.Funcs {
	var calls int32
	return interceptor.Funcs{
		SubResourceUpdate: func(ctx context.Context, c client.Client, subResourceName string, obj client.Object, opts ...client.SubResourceUpdateOption) error {
			if subResourceName == "status" && atomic.AddInt32(&calls, 1) <= n {
				return apierrors.NewInvalid(
					schema.GroupKind{Group: "kubernaut.ai", Kind: "AIAnalysis"},
					obj.GetName(),
					field.ErrorList{field.NotSupported(field.NewPath("status", "subReason"),
						"SimulatedCRDSchemaLag2030", []string{"TransientError", "PermanentError"})},
				)
			}
			return c.SubResource(subResourceName).Update(ctx, obj, opts...)
		},
	}
}

// ----------------------------------------------------------------------------
// Fixture helpers
// ----------------------------------------------------------------------------

func newSchemaRejectionTestScheme() *runtime.Scheme {
	s := runtime.NewScheme()
	Expect(clientgoscheme.AddToScheme(s)).To(Succeed())
	Expect(aianalysisv1.AddToScheme(s)).To(Succeed())
	return s
}

func newSchemaRejectionTestAnalysis(name, phase string) *aianalysisv1.AIAnalysis {
	return &aianalysisv1.AIAnalysis{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: "default",
		},
		Spec: aianalysisv1.AIAnalysisSpec{
			RemediationRequestRef: corev1.ObjectReference{Name: "rr-" + name, Namespace: "default"},
			RemediationID:         "rr-" + name,
			AnalysisRequest: aianalysisv1.AnalysisRequest{
				SignalContext: aianalysisv1.SignalContextInput{
					Fingerprint:      "fp-" + name,
					Severity:         "critical",
					SignalName:       "TestSignal2030",
					Environment:      "staging",
					BusinessPriority: "P1",
					TargetResource: aianalysisv1.TargetResource{
						Kind:      "Deployment",
						Name:      "test-deploy",
						Namespace: "default",
					},
					EnrichmentResults: sharedtypes.EnrichmentResults{},
				},
				AnalysisTypes: []aianalysisv1.AnalysisType{aianalysisv1.AnalysisTypeInvestigation},
			},
		},
		Status: aianalysisv1.AIAnalysisStatus{
			Phase: phase,
		},
	}
}

// newSchemaRejectionTestReconciler builds a reconciler wired with a real
// InvestigatingHandler/AnalyzingHandler (per #2030 plan spike: both
// constructors are simple given just the small interfaces above, no deep
// dependency graph) backed by the given (possibly interceptor-wrapped)
// client, so reconcileInvestigating/reconcileAnalyzing run through their
// genuine production logic end to end.
func newSchemaRejectionTestReconciler(k8sClient client.Client) *AIAnalysisReconciler {
	log := logr.Discard()
	m := metrics.NewMetricsWithRegistry(prometheus.NewRegistry())

	r := &AIAnalysisReconciler{
		Client:        k8sClient,
		Scheme:        k8sClient.Scheme(),
		Recorder:      record.NewFakeRecorder(50),
		Log:           log,
		Metrics:       m,
		StatusManager: status.NewManager(k8sClient, k8sClient),
		AuditClient:   aiaudit.NewAuditClient(noopAuditStore{}, log),
		AnalyzingHandler: handlers.NewAnalyzingHandler(
			neverCalledRegoEvaluator{}, log, m, noopAnalyzingAuditClient{}),
	}
	r.InvestigatingHandler.Store(handlers.NewInvestigatingHandler(
		fakeKAClientAlwaysErrors{}, log, m, noopAuditClient{}))
	return r
}

// ----------------------------------------------------------------------------
// UT-AA-2030-001/002/003/005
// ----------------------------------------------------------------------------

var _ = Describe("Schema-rejection retry (#2030 Part A)", func() {
	var ctx context.Context

	BeforeEach(func() {
		ctx = context.Background()
	})

	It("UT-AA-2030-001: reconcileInvestigating retries (not freezes) on apierrors.IsInvalid", func() {
		scheme := newSchemaRejectionTestScheme()
		analysis := newSchemaRejectionTestAnalysis("ut-2030-001", PhaseInvestigating)

		k8sClient := fake.NewClientBuilder().
			WithScheme(scheme).
			WithStatusSubresource(&aianalysisv1.AIAnalysis{}).
			WithObjects(analysis).
			WithInterceptorFuncs(failNStatusUpdates(100)).
			Build()

		r := newSchemaRejectionTestReconciler(k8sClient)

		result, err := r.reconcileInvestigating(ctx, analysis)
		Expect(err).NotTo(HaveOccurred(),
			"schema rejection must not surface as a reconcile error -- that would just log-and-drop under controller-runtime's default rate limiter")
		Expect(result.RequeueAfter).To(BeNumerically(">", 0),
			"must requeue with backoff, not fail-close forever (ctrl.Result{}, nil) like the pre-#2030 behavior")

		var fresh aianalysisv1.AIAnalysis
		Expect(k8sClient.Get(ctx, client.ObjectKeyFromObject(analysis), &fresh)).To(Succeed())
		Expect(fresh.Annotations[handlers.SchemaRejectionRetryCountAnnotation]).To(Equal("1"),
			"retry count annotation must persist via plain Update() even though Status().Update() was rejected")
		Expect(fresh.Status.Phase).To(Equal(PhaseInvestigating),
			"phase must remain unchanged in the persisted object -- the rejected status write never actually landed")
	})

	It("UT-AA-2030-002: reconcileAnalyzing retries (not freezes) on apierrors.IsInvalid", func() {
		scheme := newSchemaRejectionTestScheme()
		analysis := newSchemaRejectionTestAnalysis("ut-2030-002", PhaseAnalyzing)

		k8sClient := fake.NewClientBuilder().
			WithScheme(scheme).
			WithStatusSubresource(&aianalysisv1.AIAnalysis{}).
			WithObjects(analysis).
			WithInterceptorFuncs(failNStatusUpdates(100)).
			Build()

		r := newSchemaRejectionTestReconciler(k8sClient)

		result, err := r.reconcileAnalyzing(ctx, analysis)
		Expect(err).NotTo(HaveOccurred())
		Expect(result.RequeueAfter).To(BeNumerically(">", 0),
			"must requeue with backoff, not fail-close forever like the pre-#2030 behavior")

		var fresh aianalysisv1.AIAnalysis
		Expect(k8sClient.Get(ctx, client.ObjectKeyFromObject(analysis), &fresh)).To(Succeed())
		Expect(fresh.Annotations[handlers.SchemaRejectionRetryCountAnnotation]).To(Equal("1"))
		Expect(fresh.Status.Phase).To(Equal(PhaseAnalyzing))
	})

	It("UT-AA-2030-003: retry count annotation persists via plain Update() across repeated rejections", func() {
		scheme := newSchemaRejectionTestScheme()
		analysis := newSchemaRejectionTestAnalysis("ut-2030-003", PhaseInvestigating)

		k8sClient := fake.NewClientBuilder().
			WithScheme(scheme).
			WithStatusSubresource(&aianalysisv1.AIAnalysis{}).
			WithObjects(analysis).
			WithInterceptorFuncs(failNStatusUpdates(100)).
			Build()

		r := newSchemaRejectionTestReconciler(k8sClient)

		_, err := r.reconcileInvestigating(ctx, analysis)
		Expect(err).NotTo(HaveOccurred())

		var afterFirst aianalysisv1.AIAnalysis
		Expect(k8sClient.Get(ctx, client.ObjectKeyFromObject(analysis), &afterFirst)).To(Succeed())
		Expect(afterFirst.Annotations[handlers.SchemaRejectionRetryCountAnnotation]).To(Equal("1"))

		// Second reconcile of the SAME still-rejected object: count must advance
		// to 2, proving the annotation (not an in-memory-only counter) is the
		// durable source of truth across independent reconciler invocations.
		_, err = r.reconcileInvestigating(ctx, &afterFirst)
		Expect(err).NotTo(HaveOccurred())

		var afterSecond aianalysisv1.AIAnalysis
		Expect(k8sClient.Get(ctx, client.ObjectKeyFromObject(analysis), &afterSecond)).To(Succeed())
		Expect(afterSecond.Annotations[handlers.SchemaRejectionRetryCountAnnotation]).To(Equal("2"))
	})

	It("UT-AA-2030-005: exceeding the retry cap escalates to a terminal Failed phase with valid enum values", func() {
		scheme := newSchemaRejectionTestScheme()
		analysis := newSchemaRejectionTestAnalysis("ut-2030-005", PhaseInvestigating)
		analysis.Annotations = map[string]string{
			handlers.SchemaRejectionRetryCountAnnotation: "5", // handlers.MaxSchemaRejectionRetries
		}

		k8sClient := fake.NewClientBuilder().
			WithScheme(scheme).
			WithStatusSubresource(&aianalysisv1.AIAnalysis{}).
			WithObjects(analysis).
			// Call #1 (the handler-driven write) is rejected, exactly like every
			// prior attempt; call #2 (this helper's escalation write, using
			// valid enum values) succeeds -- modeling that only the ORIGINAL
			// write is affected by the schema gap, not every write forever.
			WithInterceptorFuncs(failNStatusUpdates(1)).
			Build()

		r := newSchemaRejectionTestReconciler(k8sClient)

		result, err := r.reconcileInvestigating(ctx, analysis)
		Expect(err).NotTo(HaveOccurred())
		Expect(result.RequeueAfter).To(BeZero(),
			"a terminal Failed phase needs no further requeue")

		var fresh aianalysisv1.AIAnalysis
		Expect(k8sClient.Get(ctx, client.ObjectKeyFromObject(analysis), &fresh)).To(Succeed())
		Expect(fresh.Status.Phase).To(Equal(PhaseFailed),
			"must escalate to a visible terminal state instead of retrying forever")
		Expect(fresh.Status.Reason).To(Equal(aianalysisv1.ReasonAPIError))
		Expect(fresh.Status.SubReason).To(Equal("TransientError"))
	})

	It("UT-AA-2030-014: a subsequent successful status write clears the leftover retry-count annotation", func() {
		scheme := newSchemaRejectionTestScheme()
		analysis := newSchemaRejectionTestAnalysis("ut-2030-014", PhaseInvestigating)
		analysis.Annotations = map[string]string{
			handlers.SchemaRejectionRetryCountAnnotation: "3",
		}

		// No interceptor: the CRD schema lag has been resolved, so this
		// Status().Update() must succeed normally -- proving the leftover
		// count from an earlier, unrelated rejection episode doesn't linger
		// forever and prematurely trip handlers.MaxSchemaRejectionRetries
		// on some future, unrelated schema-rejection episode.
		k8sClient := fake.NewClientBuilder().
			WithScheme(scheme).
			WithStatusSubresource(&aianalysisv1.AIAnalysis{}).
			WithObjects(analysis).
			Build()

		r := newSchemaRejectionTestReconciler(k8sClient)

		_, err := r.reconcileInvestigating(ctx, analysis)
		Expect(err).NotTo(HaveOccurred())

		var fresh aianalysisv1.AIAnalysis
		Expect(k8sClient.Get(ctx, client.ObjectKeyFromObject(analysis), &fresh)).To(Succeed())
		_, stillPresent := fresh.Annotations[handlers.SchemaRejectionRetryCountAnnotation]
		Expect(stillPresent).To(BeFalse(),
			"the retry-count annotation must be cleared once a status write succeeds again")
	})
})
