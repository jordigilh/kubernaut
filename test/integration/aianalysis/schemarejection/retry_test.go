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

package schemarejection

import (
	"context"
	"errors"
	"sync/atomic"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/go-logr/logr"
	"github.com/prometheus/client_golang/prometheus"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/util/validation/field"
	"k8s.io/client-go/kubernetes/scheme"
	"k8s.io/client-go/tools/record"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/interceptor"

	aianalysisv1 "github.com/jordigilh/kubernaut/api/aianalysis/v1alpha1"
	"github.com/jordigilh/kubernaut/internal/controller/aianalysis"
	"github.com/jordigilh/kubernaut/pkg/agentclient"
	"github.com/jordigilh/kubernaut/pkg/aianalysis/handlers"
	"github.com/jordigilh/kubernaut/pkg/aianalysis/metrics"
	"github.com/jordigilh/kubernaut/pkg/aianalysis/rego"
	"github.com/jordigilh/kubernaut/pkg/aianalysis/status"
	sharedtypes "github.com/jordigilh/kubernaut/pkg/shared/types"
)

// ----------------------------------------------------------------------------
// Test doubles (package-local; the ones in internal/controller/aianalysis's
// _test.go file are not importable from here — _test.go files never are).
// ----------------------------------------------------------------------------

type fakeKAClientAlwaysErrors struct{}

func (fakeKAClientAlwaysErrors) Investigate(_ context.Context, _ *agentclient.IncidentRequest) (*agentclient.IncidentResponse, error) {
	return nil, errors.New("simulated KA outage (test double, #2029 IT)")
}
func (fakeKAClientAlwaysErrors) SubmitInvestigation(_ context.Context, _ *agentclient.IncidentRequest) (string, error) {
	return "", errors.New("not implemented in test double (#2029 IT): session mode unused")
}
func (fakeKAClientAlwaysErrors) PollSession(_ context.Context, _ string) (*agentclient.SessionStatusResult, error) {
	return nil, errors.New("not implemented in test double (#2029 IT): session mode unused")
}
func (fakeKAClientAlwaysErrors) GetSessionResult(_ context.Context, _ string) (*agentclient.IncidentResponse, error) {
	return nil, errors.New("not implemented in test double (#2029 IT): session mode unused")
}
func (fakeKAClientAlwaysErrors) CancelSession(_ context.Context, _ string) error { return nil }

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
func (noopAuditClient) RecordAIAgentSessionLost(_ context.Context, _ *aianalysisv1.AIAnalysis, _ int32) {
}

type noopAnalyzingAuditClient struct{}

func (noopAnalyzingAuditClient) RecordRegoEvaluation(_ context.Context, _ *aianalysisv1.AIAnalysis, _ string, _ bool, _ int, _ string) {
}
func (noopAnalyzingAuditClient) RecordApprovalDecision(_ context.Context, _ *aianalysisv1.AIAnalysis, _, _ string) {
}
func (noopAnalyzingAuditClient) RecordAnalysisComplete(_ context.Context, _ *aianalysisv1.AIAnalysis) {
}
func (noopAnalyzingAuditClient) RecordAnalysisFailed(_ context.Context, _ *aianalysisv1.AIAnalysis, _ error) error {
	return nil
}

// neverCalledRegoEvaluator: fixture has no SelectedWorkflow, so
// AnalyzingHandler.Handle must short-circuit to Phase=Failed/
// Reason=NoWorkflowSelected before ever evaluating Rego.
type neverCalledRegoEvaluator struct{}

func (neverCalledRegoEvaluator) Evaluate(_ context.Context, _ *rego.PolicyInput) (*rego.PolicyResult, error) {
	return nil, errors.New("unexpected call (#2029 IT test double): fixture has no SelectedWorkflow")
}

// ----------------------------------------------------------------------------
// Interceptor: rejects the first n Status().Update() calls with a synthetic
// apierrors.IsInvalid, modeling a CRD-schema-lagging-behind-Go-source
// rejection, then delegates to the real envtest apiserver for every call
// after that (and always for plain, non-status Update() calls).
// ----------------------------------------------------------------------------

func failNStatusUpdates(n int32) interceptor.Funcs {
	var calls int32
	return interceptor.Funcs{
		SubResourceUpdate: func(ctx context.Context, c client.Client, subResourceName string, obj client.Object, opts ...client.SubResourceUpdateOption) error {
			if subResourceName == "status" && atomic.AddInt32(&calls, 1) <= n {
				return apierrors.NewInvalid(
					schema.GroupKind{Group: "kubernaut.ai", Kind: "AIAnalysis"},
					obj.GetName(),
					field.ErrorList{field.NotSupported(field.NewPath("status", "subReason"),
						"SimulatedCRDSchemaLag2029IT", []string{"TransientError", "PermanentError"})},
				)
			}
			return c.SubResource(subResourceName).Update(ctx, obj, opts...)
		},
	}
}

// ----------------------------------------------------------------------------
// Fixture + reconciler helpers
// ----------------------------------------------------------------------------

func newSchemaRejectionITAnalysis(ns, name, phase string) *aianalysisv1.AIAnalysis {
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
			RemediationRequestRef: corev1.ObjectReference{Name: "rr-" + name, Namespace: ns},
			RemediationID:         "rr-" + name,
			AnalysisRequest: aianalysisv1.AnalysisRequest{
				SignalContext: aianalysisv1.SignalContextInput{
					Fingerprint:      "fp-" + name,
					Severity:         "critical",
					SignalName:       "TestSignal2029IT",
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
			Phase: phase,
		},
	}
}

// newSchemaRejectionITReconciler wires a real InvestigatingHandler/
// AnalyzingHandler (backed by trivial test doubles for KA/Rego/audit) onto
// the given interceptor-wrapped, envtest-backed client, so
// reconcileInvestigating/reconcileAnalyzing run through their genuine
// production logic against a real Kubernetes API server.
func newSchemaRejectionITReconciler(k8sClient client.Client) *aianalysis.AIAnalysisReconciler {
	log := logr.Discard()
	m := metrics.NewMetricsWithRegistry(prometheus.NewRegistry())

	r := &aianalysis.AIAnalysisReconciler{
		Client:        k8sClient,
		Scheme:        k8sClient.Scheme(),
		Recorder:      record.NewFakeRecorder(50),
		Log:           log,
		Metrics:       m,
		StatusManager: status.NewManager(k8sClient, k8sClient),
		AnalyzingHandler: handlers.NewAnalyzingHandler(
			neverCalledRegoEvaluator{}, log, m, noopAnalyzingAuditClient{}),
	}
	r.InvestigatingHandler.Store(handlers.NewInvestigatingHandler(
		fakeKAClientAlwaysErrors{}, log, m, noopAuditClient{}))
	return r
}

// ----------------------------------------------------------------------------
// IT-AA-2029-006
// ----------------------------------------------------------------------------

var _ = Describe("Schema-rejection retry wiring against a real API server (#2029 Part A)", func() {
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

		ns = &corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: "schema-rejection-it-" + randSuffix()}}
		Expect(realWatch.Create(ctx, ns)).To(Succeed())
	})

	AfterEach(func() {
		Expect(realWatch.Delete(ctx, ns)).To(Succeed())
	})

	It("IT-AA-2029-006: retries with backoff against a real apiserver, then escalates once the cap is exceeded", func() {
		analysis := newSchemaRejectionITAnalysis(ns.Name, "it-2029-006", aianalysisv1.PhaseInvestigating)
		Expect(realWatch.Create(ctx, analysis)).To(Succeed())
		analysis.Status.Phase = aianalysisv1.PhaseInvestigating
		Expect(realWatch.Status().Update(ctx, analysis)).To(Succeed())

		req := ctrl.Request{NamespacedName: client.ObjectKeyFromObject(analysis)}

		By("first reconcile: schema rejection must produce a bounded backoff requeue, not a permanent freeze")
		wrapped := interceptor.NewClient(realWatch, failNStatusUpdates(100))
		r := newSchemaRejectionITReconciler(wrapped)

		result, err := r.Reconcile(ctx, req)
		Expect(err).NotTo(HaveOccurred())
		Expect(result.RequeueAfter).To(BeNumerically(">", 0),
			"must requeue with backoff, not fail-close forever (ctrl.Result{}, nil) like the pre-#2029 behavior")

		var fresh aianalysisv1.AIAnalysis
		Expect(realWatch.Get(ctx, req.NamespacedName, &fresh)).To(Succeed())
		Expect(fresh.Annotations[handlers.SchemaRejectionRetryCountAnnotation]).To(Equal("1"),
			"retry annotation must persist via a plain Update() against the REAL apiserver even while Status().Update() is rejected")
		Expect(fresh.Status.Phase).To(Equal(aianalysisv1.PhaseInvestigating),
			"phase must remain unchanged in the persisted object — the rejected status write never actually landed")

		By("second reconcile of the same still-rejected object: annotation must advance to 2 via the real apiserver")
		result2, err := r.Reconcile(ctx, req)
		Expect(err).NotTo(HaveOccurred())
		Expect(result2.RequeueAfter).To(BeNumerically(">", 0))

		Expect(realWatch.Get(ctx, req.NamespacedName, &fresh)).To(Succeed())
		Expect(fresh.Annotations[handlers.SchemaRejectionRetryCountAnnotation]).To(Equal("2"))

		By("simulating exhausted retries: escalation must land a valid, real-schema-accepted terminal status")
		fresh.Annotations[handlers.SchemaRejectionRetryCountAnnotation] = "5" // handlers.MaxSchemaRejectionRetries
		Expect(realWatch.Update(ctx, &fresh)).To(Succeed())

		wrappedOnceMore := interceptor.NewClient(realWatch, failNStatusUpdates(1))
		r2 := newSchemaRejectionITReconciler(wrappedOnceMore)

		result3, err := r2.Reconcile(ctx, req)
		Expect(err).NotTo(HaveOccurred())
		Expect(result3.RequeueAfter).To(BeZero(), "a terminal Failed phase needs no further requeue")

		var escalated aianalysisv1.AIAnalysis
		Expect(realWatch.Get(ctx, req.NamespacedName, &escalated)).To(Succeed())
		Expect(escalated.Status.Phase).To(Equal(aianalysisv1.PhaseFailed),
			"the real apiserver must accept the escalation write (valid enum values), proving handleSchemaRejectedStatusUpdate's terminal write is schema-safe")
		Expect(escalated.Status.Reason).To(Equal(aianalysisv1.ReasonAPIError))
		Expect(escalated.Status.SubReason).To(Equal("TransientError"))
	})
})

func randSuffix() string {
	return time.Now().Format("150405-000000")
}
