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

package cascadecancel

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

// noopAuditStore implements pkg/audit.AuditStore, backing the top-level
// controller's *audit.AuditClient (r.AuditClient) with no real DataStorage
// dependency. The terminal-state dispatch path this suite exercises never
// calls RecordError (dispatchPhase always returns a nil error for
// PhaseFailed/PhaseCompleted), so this is present only to satisfy AA-BUG-005's
// non-nil AuditClient invariant, mirroring the sibling capacityretry suite.
type noopAuditStore struct{}

func (noopAuditStore) StoreAudit(_ context.Context, _ *ogenclient.AuditEventRequest) error {
	return nil
}
func (noopAuditStore) Flush(_ context.Context) error { return nil }
func (noopAuditStore) Close() error                  { return nil }

// newCascadeCancelledAnalysis builds an AIAnalysis fixture already
// externally terminated by RemediationOrchestrator's cascade-cancel (#1421):
// Phase=Failed, Reason=ParentCancelled -- the exact Status RO's
// cascadeToAIAnalysis patches directly (it does not delete the AIAnalysis,
// so no owner-cascade reaches the child AgentSession; #2214's
// cascadeCancelAgentSession is what closes that gap).
func newCascadeCancelledAnalysis(ns, name string) *aianalysisv1.AIAnalysis {
	rrName := "rr-" + name
	return &aianalysisv1.AIAnalysis{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: ns,
			// Pre-populate the finalizer as a real object mid-lifecycle
			// would have it: Reconcile()'s ensureFinalizer step adds it and
			// returns early on the very first pass, before ever reaching
			// the terminal-phase dispatch this test targets.
			Finalizers: []string{aianalysis.FinalizerName},
		},
		Spec: aianalysisv1.AIAnalysisSpec{
			RemediationRequestRef: corev1.ObjectReference{Name: rrName, Namespace: ns},
			RemediationID:         rrName,
			AnalysisRequest: aianalysisv1.AnalysisRequest{
				SignalContext: aianalysisv1.SignalContextInput{
					Fingerprint:      "fp-" + name,
					Severity:         "critical",
					SignalName:       "TestSignalCascadeCancelIT",
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
			Phase:  aianalysisv1.PhaseFailed,
			Reason: aianalysisv1.ReasonParentCancelled,
		},
	}
}

// seedAgentSession creates a real AgentSession CRD in envtest (via the same
// handlers.RequestBuilder the production creator.AgentSessionCreator uses,
// so the Spec is genuinely valid against the installed CRD schema), owned by
// analysis -- modeling the AgentSession created earlier in this AIAnalysis's
// (now-cancelled) investigation. analysis must already be persisted (real
// UID) so the owner reference is valid.
func seedAgentSession(ctx context.Context, k8sClient client.WithWatch, analysis *aianalysisv1.AIAnalysis) *agentsessionv1.AgentSession {
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
	return as
}

// newCascadeCancelReconciler wires a real AIAnalysisReconciler backed by the
// REAL production creator.AgentSessionCreator (not a fake) onto the given
// envtest-backed client -- this suite has no real KA subprocess to race with
// (unlike test/integration/aianalysis's heavy shared suite), proving the
// full cross-component wiring (IT-AA-2214-001) end to end.
func newCascadeCancelReconciler(k8sClient client.WithWatch) *aianalysis.AIAnalysisReconciler {
	log := ctrllog.Log.WithName("cascade-cancel-it")
	m := metrics.NewMetricsWithRegistry(prometheus.NewRegistry())
	auditClient := aiaudit.NewAuditClient(noopAuditStore{}, log)
	agentSessionCreator := creator.NewAgentSessionCreator(k8sClient, k8sClient.Scheme())

	return &aianalysis.AIAnalysisReconciler{
		Client:              k8sClient,
		Scheme:              k8sClient.Scheme(),
		Recorder:            record.NewFakeRecorder(50),
		Log:                 log,
		Metrics:             m,
		StatusManager:       status.NewManager(k8sClient, k8sClient),
		AuditClient:         auditClient,
		AgentSessionCreator: agentSessionCreator,
	}
}

// ----------------------------------------------------------------------------
// #2214 / DD-AA-KA-001 Amendment: AA cascade-cancel deletes AgentSession
// (rather than writing InvestigationSession directly)
// ----------------------------------------------------------------------------

var _ = Describe("AA cascade-cancel deletes the correlated AgentSession against a real API server (#2214) [IR-4(1)]", func() {
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

		ns = &corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: "cascade-cancel-it-" + randSuffix()}}
		Expect(realWatch.Create(ctx, ns)).To(Succeed())
	})

	AfterEach(func() {
		Expect(realWatch.Delete(ctx, ns)).To(Succeed())
	})

	It("IT-AA-2214-001: deletes the correlated AgentSession when AIAnalysis is Failed with ParentCancelled [IR-4(1)]", func() {
		analysis := newCascadeCancelledAnalysis(ns.Name, "cascade-cancel-happy-path")
		desiredStatus := analysis.Status
		Expect(realWatch.Create(ctx, analysis)).To(Succeed())
		analysis.Status = desiredStatus
		Expect(realWatch.Status().Update(ctx, analysis)).To(Succeed())

		as := seedAgentSession(ctx, realWatch, analysis)

		r := newCascadeCancelReconciler(realWatch)
		req := ctrl.Request{NamespacedName: client.ObjectKeyFromObject(analysis)}

		result, err := r.Reconcile(ctx, req)
		Expect(err).NotTo(HaveOccurred())
		Expect(result.RequeueAfter).To(BeNumerically("==", 0),
			"a terminal-state reconcile that has already cascaded must not requeue")

		var deleted agentsessionv1.AgentSession
		getErr := realWatch.Get(ctx, client.ObjectKeyFromObject(as), &deleted)
		Expect(apierrors.IsNotFound(getErr)).To(BeTrue(),
			"cascadeCancelAgentSession must delete the correlated AgentSession so AF's "+
				"AgentSessionTerminalCloseReconciler observes the delete and closes IS to "+
				"Cancelled, and KA's Dispatcher.cancelOnDelete stops the in-flight goroutine")
	})

	It("IT-AA-2214-001b: is idempotent when the AgentSession is already gone (duplicate reconcile delivery)", func() {
		analysis := newCascadeCancelledAnalysis(ns.Name, "cascade-cancel-already-gone")
		desiredStatus := analysis.Status
		Expect(realWatch.Create(ctx, analysis)).To(Succeed())
		analysis.Status = desiredStatus
		Expect(realWatch.Status().Update(ctx, analysis)).To(Succeed())

		// No AgentSession seeded at all -- models a redundant informer
		// delivery after a prior reconcile already deleted it.
		r := newCascadeCancelReconciler(realWatch)
		req := ctrl.Request{NamespacedName: client.ObjectKeyFromObject(analysis)}

		result, err := r.Reconcile(ctx, req)
		Expect(err).NotTo(HaveOccurred(),
			"DeleteForCascadeCancel's NotFound-is-not-an-error contract must surface as a clean, non-erroring reconcile")
		Expect(result.RequeueAfter).To(BeNumerically("==", 0))
	})
})

func randSuffix() string {
	return time.Now().Format("150405-000000")
}
