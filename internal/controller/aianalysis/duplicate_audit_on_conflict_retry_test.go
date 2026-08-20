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

// #2204 follow-up (2026-08-20, helios08 E2E RCA): 05_audit_trail_test.go's
// "should have EXACTLY 1 AI agent call in happy path" occasionally observes 2.
// RCA: status.Manager.AtomicStatusUpdate wraps its updateFunc closure in
// k8sretry.RetryOnConflict (pkg/aianalysis/status/manager.go). That closure
// is runInvestigatingHandler (phase_handlers.go), which calls
// InvestigatingHandler.Handle -> handleSessionCompleted -> the non-idempotent,
// side-effecting audit.AuditClient.RecordAIAgentResult (investigating.go) --
// a real write to Data Storage, executed *inside* the retried closure. If the
// closure's own Status().Update() call at the end hits a resourceVersion
// Conflict (plausible under concurrent load: MaxConcurrentReconciles>1,
// contended CI/helios08), RetryOnConflict re-runs the ENTIRE closure --
// re-invoking the handler and re-emitting the audit call -- even though the
// eventual K8s status write only commits once. This double-emits exactly the
// "aianalysis.aiagent.call" event the E2E flake observed. This is the same
// class of bug already fixed elsewhere in this file's sibling function,
// reconcilePending (see its "DD-AUDIT-003: Record phase transition AFTER
// status update" comment) -- audit side effects must run only after the
// atomic write actually commits, never inside the retried closure.
package aianalysis

import (
	"context"
	"sync"
	"sync/atomic"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/go-logr/logr"
	"github.com/prometheus/client_golang/prometheus"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/tools/record"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/controller-runtime/pkg/client/interceptor"

	agentsessionv1 "github.com/jordigilh/kubernaut/api/agentsession/v1alpha1"
	aianalysisv1 "github.com/jordigilh/kubernaut/api/aianalysis/v1alpha1"
	aiaudit "github.com/jordigilh/kubernaut/pkg/aianalysis/audit"
	"github.com/jordigilh/kubernaut/pkg/aianalysis/handlers"
	"github.com/jordigilh/kubernaut/pkg/aianalysis/metrics"
	"github.com/jordigilh/kubernaut/pkg/aianalysis/status"
	sharedaudit "github.com/jordigilh/kubernaut/pkg/audit"
	ogenclient "github.com/jordigilh/kubernaut/pkg/datastorage/ogen-client"
	sharedtypes "github.com/jordigilh/kubernaut/pkg/shared/types"
)

// ----------------------------------------------------------------------------
// Test doubles
// ----------------------------------------------------------------------------

// fakeKACompletedSession implements handlers.AgentSessionGetOrCreator.
// GetOrCreate always returns the same already-Completed AgentSession with a
// minimal, valid Result -- modeling the reconcile that observes KA's
// terminal write and must process it exactly once.
type fakeKACompletedSession struct {
	name string
}

func (f fakeKACompletedSession) GetOrCreate(_ context.Context, analysis *aianalysisv1.AIAnalysis) (*agentsessionv1.AgentSession, error) {
	return &agentsessionv1.AgentSession{
		ObjectMeta: metav1.ObjectMeta{
			Name:              f.name,
			Namespace:         analysis.Namespace,
			CreationTimestamp: metav1.Now(),
		},
		Status: agentsessionv1.AgentSessionStatus{
			Phase: agentsessionv1.AgentSessionPhaseCompleted,
			Result: &agentsessionv1.AgentSessionResult{
				Analysis:         "test analysis",
				Confidence:       0.9,
				NeedsHumanReview: false,
			},
		},
	}, nil
}

func (fakeKACompletedSession) DeleteForRetry(_ context.Context, _ *agentsessionv1.AgentSession) error {
	return nil
}

// auditEventTypeCounter implements audit.AuditStore, counting stored events
// by EventType -- the assertion target for this bug. #2204 fix: the
// RecordAIAgentResult call moved from inside InvestigatingHandler (which
// runs inside AtomicStatusUpdate's retried closure) to
// finalizeInvestigatingTransition (which runs once, after the closure
// commits) -- see phase_handlers.go and DD-WE-009. This spy is wired into
// the controller-level r.AuditClient (a real *audit.AuditClient backed by
// this store), not the handler-level handlers.AuditClientInterface, to
// observe the actual emitted events end to end, matching DD-WE-009's own
// validation technique ("audit-event count stayed at exactly 1").
type auditEventTypeCounter struct {
	mu     sync.Mutex
	counts map[string]int32
}

func newAuditEventTypeCounter() *auditEventTypeCounter {
	return &auditEventTypeCounter{counts: make(map[string]int32)}
}

func (c *auditEventTypeCounter) StoreAudit(_ context.Context, event *ogenclient.AuditEventRequest) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.counts[event.EventType]++
	return nil
}
func (c *auditEventTypeCounter) Flush(_ context.Context) error { return nil }
func (c *auditEventTypeCounter) Close() error                  { return nil }

func (c *auditEventTypeCounter) countOf(eventType string) int32 {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.counts[eventType]
}

var _ sharedaudit.AuditStore = &auditEventTypeCounter{}

// conflictOnceOnStatusUpdate returns interceptor funcs whose
// SubResourceUpdate hook rejects exactly the first call to Status().Update()
// with a synthetic apierrors.IsConflict error (modeling a concurrent writer
// racing the same resourceVersion), then delegates to the real fake client
// for every call after that -- forcing k8sretry.RetryOnConflict to retry the
// whole AtomicStatusUpdate closure exactly once.
func conflictOnceOnStatusUpdate() interceptor.Funcs {
	var calls int32
	return interceptor.Funcs{
		SubResourceUpdate: func(ctx context.Context, c client.Client, subResourceName string, obj client.Object, opts ...client.SubResourceUpdateOption) error {
			if subResourceName == "status" && atomic.AddInt32(&calls, 1) == 1 {
				return apierrors.NewConflict(
					schema.GroupResource{Group: "kubernaut.ai", Resource: "aianalyses"},
					obj.GetName(),
					errors2204ConflictSentinel,
				)
			}
			return c.SubResource(subResourceName).Update(ctx, obj, opts...)
		},
	}
}

// errors2204ConflictSentinel is a fixed error value for the synthetic
// Conflict above; apierrors.NewConflict requires a non-nil err argument to
// build its message, its identity is not otherwise significant.
var errors2204ConflictSentinel = apierrors.NewBadRequest("simulated resourceVersion conflict (test double, #2204)")

// ----------------------------------------------------------------------------
// UT-AA-2204-101
// ----------------------------------------------------------------------------

var _ = Describe("Duplicate audit emission on AtomicStatusUpdate conflict-retry (#2204)", func() {
	It("UT-AA-2204-101: RecordAIAgentResult must fire exactly once even when the status write is retried after a Conflict", func() {
		ctx := context.Background()
		scheme := newSchemaRejectionTestScheme()

		analysis := &aianalysisv1.AIAnalysis{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "ut-2204-101",
				Namespace: "default",
			},
			Spec: aianalysisv1.AIAnalysisSpec{
				RemediationRequestRef: corev1.ObjectReference{Name: "rr-ut-2204-101", Namespace: "default"},
				RemediationID:         "rr-ut-2204-101",
				AnalysisRequest: aianalysisv1.AnalysisRequest{
					SignalContext: aianalysisv1.SignalContextInput{
						Fingerprint:      "fp-ut-2204-101",
						Severity:         "warning",
						SignalName:       "TestSignal2204",
						Environment:      "staging",
						BusinessPriority: "P2",
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
				Phase: PhaseInvestigating,
				// Non-nil KASession -> firstObservation=false, so Handle()
				// skips finalizeSessionSubmit/RecordAIAgentSubmit and goes
				// straight to branching on the AgentSession's phase, exactly
				// like a real reconcile that already submitted the session
				// on a prior pass and is now observing KA's Completed write.
				KASession: &aianalysisv1.KASession{
					ID:        "as-ut-2204-101",
					CreatedAt: &metav1.Time{Time: metav1.Now().Time},
				},
			},
		}

		k8sClient := fake.NewClientBuilder().
			WithScheme(scheme).
			WithStatusSubresource(&aianalysisv1.AIAnalysis{}).
			WithObjects(analysis).
			WithInterceptorFuncs(conflictOnceOnStatusUpdate()).
			Build()

		log := logr.Discard()
		m := metrics.NewMetricsWithRegistry(prometheus.NewRegistry())
		auditStoreSpy := newAuditEventTypeCounter()

		r := &AIAnalysisReconciler{
			Client:        k8sClient,
			Scheme:        k8sClient.Scheme(),
			Recorder:      record.NewFakeRecorder(50),
			Log:           log,
			Metrics:       m,
			StatusManager: status.NewManager(k8sClient, k8sClient),
			AuditClient:   aiaudit.NewAuditClient(auditStoreSpy, log),
		}
		r.InvestigatingHandler.Store(handlers.NewInvestigatingHandler(
			fakeKACompletedSession{name: "as-ut-2204-101"}, log, m, noopAuditClient{}))

		_, err := r.reconcileInvestigating(ctx, analysis)
		Expect(err).NotTo(HaveOccurred(), "the Conflict must be absorbed by RetryOnConflict, not surfaced as a reconcile error")

		Expect(auditStoreSpy.countOf(aiaudit.EventTypeAIAgentCall)).To(Equal(int32(1)),
			"aianalysis.aiagent.call must be recorded exactly once "+
				"even though AtomicStatusUpdate's Status().Update() hit a Conflict and retried the whole closure once (#2204)")
		Expect(auditStoreSpy.countOf(aiaudit.EventTypeAIAgentResult)).To(Equal(int32(1)),
			"aianalysis.aiagent.result must be recorded exactly once for the same reason")
	})
})
