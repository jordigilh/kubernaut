package controller

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"time"

	"github.com/go-logr/logr"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/tools/record"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"

	aianalysisv1 "github.com/jordigilh/kubernaut/api/aianalysis/v1alpha1"
	effectivenessv1 "github.com/jordigilh/kubernaut/api/effectivenessassessment/v1alpha1"
	notificationv1 "github.com/jordigilh/kubernaut/api/notification/v1alpha1"
	remediationv1 "github.com/jordigilh/kubernaut/api/remediation/v1alpha1"
	signalprocessingv1 "github.com/jordigilh/kubernaut/api/signalprocessing/v1alpha1"
	workflowexecutionv1 "github.com/jordigilh/kubernaut/api/workflowexecution/v1alpha1"
	ogenclient "github.com/jordigilh/kubernaut/pkg/datastorage/ogen-client"
	roaudit "github.com/jordigilh/kubernaut/pkg/remediationorchestrator/audit"
	rocreator "github.com/jordigilh/kubernaut/pkg/remediationorchestrator/creator"
	rometrics "github.com/jordigilh/kubernaut/pkg/remediationorchestrator/metrics"
	"github.com/jordigilh/kubernaut/pkg/shared/cancellation"
	"github.com/prometheus/client_golang/prometheus"
)

type cancellationAuditStore struct {
	events []*ogenclient.AuditEventRequest
}

func (s *cancellationAuditStore) StoreAudit(_ context.Context, event *ogenclient.AuditEventRequest) error {
	s.events = append(s.events, event)
	return nil
}

func (s *cancellationAuditStore) Flush(context.Context) error { return nil }

func (s *cancellationAuditStore) Close() error { return nil }

// terminalChildFixture bundles a terminal-phase child with its parent
// reference and the cancellation-state function under test.
type terminalChildFixture struct {
	child   client.Object
	ref     *corev1.ObjectReference
	stateFn func(r *Reconciler, ctx context.Context, rr *remediationv1.RemediationRequest) (bool, string, error)
}

// terminalWFEChildFixture builds a WorkflowExecution in the given terminal
// phase with a valid parent identity and no CancellationAcknowledged
// condition, mirroring a child that finished its business outcome normally.
func terminalWFEChildFixture(phase string) terminalChildFixture {
	wfe := &workflowexecutionv1.WorkflowExecution{
		ObjectMeta: metav1.ObjectMeta{Name: "wfe-terminal", Namespace: "system", UID: "wfe-terminal-uid"},
		Spec: workflowexecutionv1.WorkflowExecutionSpec{
			RemediationRequestRef: corev1.ObjectReference{
				Name: "rr-terminal-child", Namespace: "system", UID: "rr-terminal-child-uid",
			},
		},
		Status: workflowexecutionv1.WorkflowExecutionStatus{Phase: phase},
	}
	ref := &corev1.ObjectReference{Name: wfe.Name, Namespace: wfe.Namespace, UID: wfe.UID}
	return terminalChildFixture{child: wfe, ref: ref,
		stateFn: func(r *Reconciler, ctx context.Context, rr *remediationv1.RemediationRequest) (bool, string, error) {
			acknowledged, failure, backendVerified, err := r.workflowExecutionCancellationState(ctx, rr, ref)
			// BR-ORCH-047: a terminal child finished normally; no stop was
			// requested, so the acknowledgement must never claim backend
			// termination verification.
			ExpectWithOffset(1, backendVerified).To(BeFalse())
			return acknowledged, failure, err
		}}
}

func terminalAIAnalysisChildFixture(phase string) terminalChildFixture {
	analysis := &aianalysisv1.AIAnalysis{
		ObjectMeta: metav1.ObjectMeta{Name: "aa-terminal", Namespace: "system", UID: "aa-terminal-uid"},
		Spec: aianalysisv1.AIAnalysisSpec{
			RemediationRequestRef: corev1.ObjectReference{
				Name: "rr-terminal-child", Namespace: "system", UID: "rr-terminal-child-uid",
			},
		},
		Status: aianalysisv1.AIAnalysisStatus{Phase: phase},
	}
	ref := &corev1.ObjectReference{Name: analysis.Name, Namespace: analysis.Namespace, UID: analysis.UID}
	return terminalChildFixture{child: analysis, ref: ref,
		stateFn: func(r *Reconciler, ctx context.Context, rr *remediationv1.RemediationRequest) (bool, string, error) {
			return r.aiAnalysisCancellationState(ctx, rr, ref)
		}}
}

func terminalSPChildFixture(phase string) terminalChildFixture {
	sp := &signalprocessingv1.SignalProcessing{
		ObjectMeta: metav1.ObjectMeta{Name: "sp-terminal", Namespace: "system", UID: "sp-terminal-uid"},
		Spec: signalprocessingv1.SignalProcessingSpec{
			RemediationRequestRef: signalprocessingv1.ObjectReference{
				Name: "rr-terminal-child", Namespace: "system", UID: "rr-terminal-child-uid",
			},
		},
		Status: signalprocessingv1.SignalProcessingStatus{Phase: signalprocessingv1.SignalProcessingPhase(phase)},
	}
	ref := &corev1.ObjectReference{Name: sp.Name, Namespace: sp.Namespace, UID: sp.UID}
	return terminalChildFixture{child: sp, ref: ref,
		stateFn: func(r *Reconciler, ctx context.Context, rr *remediationv1.RemediationRequest) (bool, string, error) {
			return r.signalProcessingCancellationState(ctx, rr, ref)
		}}
}

func terminalEAChildFixture(phase string) terminalChildFixture {
	ea := &effectivenessv1.EffectivenessAssessment{
		ObjectMeta: metav1.ObjectMeta{Name: "ea-terminal", Namespace: "system", UID: "ea-terminal-uid"},
		Spec:       effectivenessv1.EffectivenessAssessmentSpec{CorrelationID: "rr-terminal-child"},
		Status:     effectivenessv1.EffectivenessAssessmentStatus{Phase: phase},
	}
	ref := &corev1.ObjectReference{Name: ea.Name, Namespace: ea.Namespace, UID: ea.UID}
	return terminalChildFixture{child: ea, ref: ref,
		stateFn: func(r *Reconciler, ctx context.Context, rr *remediationv1.RemediationRequest) (bool, string, error) {
			return r.effectivenessCancellationState(ctx, rr, ref)
		}}
}

var _ = Describe("RemediationRequest cancellation coordination", func() {
	DescribeTable("UT-RO-2588-014: treats a terminal-phase child as implicitly acknowledged (BR-ORCH-047)",
		func(fixture terminalChildFixture) {
			scheme := runtime.NewScheme()
			Expect(remediationv1.AddToScheme(scheme)).To(Succeed())
			Expect(aianalysisv1.AddToScheme(scheme)).To(Succeed())
			Expect(workflowexecutionv1.AddToScheme(scheme)).To(Succeed())
			Expect(signalprocessingv1.AddToScheme(scheme)).To(Succeed())
			Expect(effectivenessv1.AddToScheme(scheme)).To(Succeed())
			Expect(notificationv1.AddToScheme(scheme)).To(Succeed())
			now := metav1.Now()
			isController := true
			fixture.child.SetOwnerReferences([]metav1.OwnerReference{{
				APIVersion: remediationv1.GroupVersion.String(),
				Kind:       "RemediationRequest",
				Name:       "rr-terminal-child",
				UID:        "rr-terminal-child-uid",
				Controller: &isController,
			}})
			rr := &remediationv1.RemediationRequest{
				ObjectMeta: metav1.ObjectMeta{
					Name: "rr-terminal-child", Namespace: "system", UID: "rr-terminal-child-uid",
					DeletionTimestamp: &now,
					Finalizers:        []string{"kubernaut.ai/remediationrequest-cancellation"},
				},
				Status: remediationv1.RemediationRequestStatus{OverallPhase: remediationv1.PhaseExecuting},
			}
			c := fake.NewClientBuilder().WithScheme(scheme).WithObjects(rr, fixture.child).Build()
			reconciler := NewReconciler(ReconcilerDeps{
				Client: c, APIReader: c, Scheme: scheme, AuditStore: &cancellationAuditStore{},
				Recorder: record.NewFakeRecorder(10), Metrics: rometrics.NewMetricsWithRegistry(prometheus.NewRegistry()),
			})

			// A child that already reached a terminal business outcome has no
			// remaining work and never sets CancellationAcknowledged. CI run
			// 43971993157 proved the previous behavior hangs `kubectl delete rr`
			// forever behind the retained cancellation finalizer.
			acknowledged, failure, err := fixture.stateFn(reconciler, context.Background(), rr)
			Expect(err).NotTo(HaveOccurred())
			Expect(failure).To(BeEmpty())
			Expect(acknowledged).To(BeTrue(),
				"a terminal-phase child must count as implicitly acknowledged")
		},
		Entry("completed WorkflowExecution", terminalWFEChildFixture("Completed")),
		Entry("failed WorkflowExecution", terminalWFEChildFixture("Failed")),
		Entry("cancelled WorkflowExecution", terminalWFEChildFixture("Cancelled")),
		Entry("completed AIAnalysis", terminalAIAnalysisChildFixture("Completed")),
		Entry("failed AIAnalysis", terminalAIAnalysisChildFixture("Failed")),
		Entry("completed SignalProcessing", terminalSPChildFixture("Completed")),
		Entry("cancelled EffectivenessAssessment", terminalEAChildFixture("Cancelled")),
	)

	DescribeTable("UT-RO-2588-015: escalates a cancellation that waited silently beyond the wall-clock budget (BR-ORCH-048)",
		func(requestedAgo time.Duration) {
			now := metav1.Now()
			requestedAt := metav1.NewTime(now.Add(-requestedAgo))
			rr := &remediationv1.RemediationRequest{
				Status: remediationv1.RemediationRequestStatus{
					OverallPhase: remediationv1.PhaseCancelling,
					Cancellation: &remediationv1.CancellationStatus{
						State:       remediationv1.CancellationStateInProgress,
						RequestedAt: &requestedAt,
					},
				},
			}
			// CI runs 43971993157 and 44009101345 proved a silent wait
			// (children acknowledge neither, or notification delivery never
			// terminates) requeues forever and hangs `kubectl delete rr`
			// behind the retained finalizer. The wait is bounded by
			// wall-clock, not poll count: legitimate child stops are paced by
			// each child's own retries (UT-ORCH-2588-028).
			Expect(cancellationWaitExhausted(rr)).To(BeTrue(),
				"a request older than the wait budget must be considered exhausted")

			// A silent wait carries no observable failure detail; the
			// escalation must still record a meaningful audit reason.
			scheme := runtime.NewScheme()
			Expect(remediationv1.AddToScheme(scheme)).To(Succeed())
			Expect(notificationv1.AddToScheme(scheme)).To(Succeed())
			reconciler := NewReconciler(ReconcilerDeps{
				Client: fake.NewClientBuilder().WithScheme(scheme).Build(), Scheme: scheme,
				AuditStore: &cancellationAuditStore{}, Recorder: record.NewFakeRecorder(10),
				Metrics: rometrics.NewMetricsWithRegistry(prometheus.NewRegistry()),
			})
			reconciler.finalizeCancellationExhaustion(context.Background(), rr, "", logr.Discard(), true)
			Expect(rr.Status.Cancellation.State).To(Equal(remediationv1.CancellationStateFailed))
		},
		Entry("eleven minutes old", cancellationWaitTimeout+time.Minute),
		Entry("one hour old", time.Hour),
	)

	It("UT-RO-2588-018: a recent or untimestamped cancellation is not exhausted (BR-ORCH-048)", func() {
		now := metav1.Now()
		recent := metav1.NewTime(now.Add(-time.Minute))
		rr := &remediationv1.RemediationRequest{
			Status: remediationv1.RemediationRequestStatus{
				Cancellation: &remediationv1.CancellationStatus{RequestedAt: &recent},
			},
		}
		Expect(cancellationWaitExhausted(rr)).To(BeFalse(),
			"a recently requested cancellation must keep waiting")
		rr.Status.Cancellation.RequestedAt = nil
		Expect(cancellationWaitExhausted(rr)).To(BeFalse(),
			"a missing RequestedAt must never report exhaustion")
	})

	It("UT-RO-2588-017: releases the finalizer for a deleting RR after the exhausted status is persisted (BR-ORCH-048)", func() {
		scheme := runtime.NewScheme()
		Expect(remediationv1.AddToScheme(scheme)).To(Succeed())
		Expect(notificationv1.AddToScheme(scheme)).To(Succeed())
		now := metav1.Now()
		rr := &remediationv1.RemediationRequest{
			ObjectMeta: metav1.ObjectMeta{
				Name: "rr-exhausted-delete", Namespace: "system", UID: "rr-exhausted-delete-uid",
				DeletionTimestamp: &now,
				Finalizers:        []string{cancellation.RemediationRequestFinalizer},
			},
			Status: remediationv1.RemediationRequestStatus{
				OverallPhase: remediationv1.PhaseCancelling,
				Cancellation: &remediationv1.CancellationStatus{
					State:      remediationv1.CancellationStateFailed,
					RetryCount: remediationCancellationMaxRetries,
				},
			},
		}
		c := fake.NewClientBuilder().WithScheme(scheme).WithObjects(rr).WithStatusSubresource(rr).Build()
		reconciler := NewReconciler(ReconcilerDeps{
			Client: c, APIReader: c, Scheme: scheme, AuditStore: &cancellationAuditStore{},
			Recorder: record.NewFakeRecorder(10), Metrics: rometrics.NewMetricsWithRegistry(prometheus.NewRegistry()),
		})

		Expect(reconciler.releaseFinalizerIfDeleting(context.Background(), rr, logr.Discard())).To(Succeed())

		// The persisted object must lose the finalizer so the pending delete
		// completes; the exhausted status evidence must remain queryable.
		// With no finalizers left and a deletion timestamp, the fake client
		// completes the deletion, so the object is gone — exactly the
		// production outcome.
		persisted := &remediationv1.RemediationRequest{}
		err := c.Get(context.Background(), client.ObjectKeyFromObject(rr), persisted)
		if err == nil {
			Expect(persisted.Finalizers).NotTo(ContainElement(cancellation.RemediationRequestFinalizer))
			Expect(persisted.Status.Cancellation.State).To(Equal(remediationv1.CancellationStateFailed),
				"the exhausted status evidence must survive the finalizer release")
		} else {
			Expect(apierrors.IsNotFound(err)).To(BeTrue(),
				"the RR must be deleted once the finalizer is released")
		}
	})

	It("UT-RO-2588-016: retains the finalizer at exhaustion for a non-deleting RR (BR-ORCH-048)", func() {
		scheme := runtime.NewScheme()
		Expect(remediationv1.AddToScheme(scheme)).To(Succeed())
		Expect(notificationv1.AddToScheme(scheme)).To(Succeed())
		rr := &remediationv1.RemediationRequest{
			ObjectMeta: metav1.ObjectMeta{
				Name: "rr-silent-wait-api", Namespace: "system", UID: "rr-silent-wait-api-uid",
				Finalizers: []string{cancellation.RemediationRequestFinalizer},
			},
			Status: remediationv1.RemediationRequestStatus{
				OverallPhase: remediationv1.PhaseCancelling,
				Cancellation: &remediationv1.CancellationStatus{
					State:      remediationv1.CancellationStateInProgress,
					RetryCount: remediationCancellationMaxRetries - 1,
				},
			},
		}
		c := fake.NewClientBuilder().WithScheme(scheme).WithObjects(rr).WithStatusSubresource(rr).Build()
		reconciler := NewReconciler(ReconcilerDeps{
			Client: c, APIReader: c, Scheme: scheme, AuditStore: &cancellationAuditStore{},
			Recorder: record.NewFakeRecorder(10), Metrics: rometrics.NewMetricsWithRegistry(prometheus.NewRegistry()),
		})

		// The cancel-API path (no deletion) keeps the documented fail-closed
		// retention: watches and manual remediation remain the recovery path.
		Expect(reconciler.releaseFinalizerIfDeleting(context.Background(), rr, logr.Discard())).To(Succeed())

		Expect(rr.Finalizers).To(ContainElement(cancellation.RemediationRequestFinalizer),
			"only a deleting RR releases the finalizer at exhaustion")
	})

	It("UT-RO-2588-019: reconcile escalates a wall-clock-exhausted cancellation once and retains the finalizer for a non-deleting RR (BR-ORCH-048)", func() {
		scheme := runtime.NewScheme()
		Expect(remediationv1.AddToScheme(scheme)).To(Succeed())
		Expect(notificationv1.AddToScheme(scheme)).To(Succeed())
		Expect(aianalysisv1.AddToScheme(scheme)).To(Succeed())
		Expect(signalprocessingv1.AddToScheme(scheme)).To(Succeed())
		Expect(workflowexecutionv1.AddToScheme(scheme)).To(Succeed())
		Expect(effectivenessv1.AddToScheme(scheme)).To(Succeed())
		requestedAt := metav1.NewTime(time.Now().Add(-(cancellationWaitTimeout + time.Minute)))
		rr := &remediationv1.RemediationRequest{
			ObjectMeta: metav1.ObjectMeta{
				Name: "rr-wait-exhausted-api", Namespace: "system", UID: "rr-wait-exhausted-api-uid",
				Finalizers: []string{cancellation.RemediationRequestFinalizer},
			},
			Status: remediationv1.RemediationRequestStatus{
				OverallPhase: remediationv1.PhaseCancelling,
				Cancellation: &remediationv1.CancellationStatus{
					RequestID:   "system/system:cancellation",
					State:       remediationv1.CancellationStateInProgress,
					RequestedAt: &requestedAt,
				},
			},
		}
		c := fake.NewClientBuilder().WithScheme(scheme).WithObjects(rr).WithStatusSubresource(rr).Build()
		store := &cancellationAuditStore{}
		reconciler := NewReconciler(ReconcilerDeps{
			Client: c, APIReader: c, Scheme: scheme, AuditStore: store,
			Recorder: record.NewFakeRecorder(10), Metrics: rometrics.NewMetricsWithRegistry(prometheus.NewRegistry()),
		})

		// First wake: the aged cancellation escalates exactly once —
		// wait-exhausted audit, terminal state, manual-review message — and
		// the finalizer is retained on the cancel-API path.
		_, err := reconciler.reconcileCancellation(context.Background(), rr, logr.Discard())
		Expect(err).NotTo(HaveOccurred())
		Expect(rr.Status.Cancellation.State).To(Equal(remediationv1.CancellationStateFailed))
		Expect(rr.Status.Message).To(ContainSubstring("wait budget"))
		Expect(rr.Finalizers).To(ContainElement(cancellation.RemediationRequestFinalizer))
		waitEvents := 0
		for _, e := range store.events {
			raw, marshalErr := json.Marshal(e.EventData)
			Expect(marshalErr).NotTo(HaveOccurred())
			if strings.Contains(string(raw), "cancellation.wait.exhausted") {
				waitEvents++
			}
		}
		Expect(waitEvents).To(BeNumerically(">", 0), "the wait-exhausted audit must be emitted")

		// Second wake after exhaustion: the State=Failed guard prevents
		// re-firing the escalation — no additional wait-exhausted audit —
		// and the manual-review status message is preserved.
		waitEventsBefore := 0
		for _, e := range store.events {
			raw, marshalErr := json.Marshal(e.EventData)
			Expect(marshalErr).NotTo(HaveOccurred())
			if strings.Contains(string(raw), "cancellation.wait.exhausted") {
				waitEventsBefore++
			}
		}
		messageBefore := rr.Status.Message
		_, err = reconciler.reconcileCancellation(context.Background(), rr, logr.Discard())
		Expect(err).NotTo(HaveOccurred())
		waitEventsAfter := 0
		for _, e := range store.events {
			raw, marshalErr := json.Marshal(e.EventData)
			Expect(marshalErr).NotTo(HaveOccurred())
			if strings.Contains(string(raw), "cancellation.wait.exhausted") {
				waitEventsAfter++
			}
		}
		Expect(waitEventsAfter).To(Equal(waitEventsBefore),
			"an already-Failed cancellation must not re-emit wait-exhausted audits")
		Expect(rr.Status.Message).To(Equal(messageBefore),
			"the manual-review status message must not regress to a waiting message")
	})

	It("UT-RO-2588-020: reconcile releases the finalizer for a deleting RR after persisting the exhausted status (BR-ORCH-048)", func() {
		scheme := runtime.NewScheme()
		Expect(remediationv1.AddToScheme(scheme)).To(Succeed())
		Expect(notificationv1.AddToScheme(scheme)).To(Succeed())
		Expect(aianalysisv1.AddToScheme(scheme)).To(Succeed())
		Expect(signalprocessingv1.AddToScheme(scheme)).To(Succeed())
		Expect(workflowexecutionv1.AddToScheme(scheme)).To(Succeed())
		Expect(effectivenessv1.AddToScheme(scheme)).To(Succeed())
		now := metav1.Now()
		requestedAt := metav1.NewTime(now.Add(-(cancellationWaitTimeout + time.Minute)))
		rr := &remediationv1.RemediationRequest{
			ObjectMeta: metav1.ObjectMeta{
				Name: "rr-wait-exhausted-del", Namespace: "system", UID: "rr-wait-exhausted-del-uid",
				DeletionTimestamp: &now,
				Finalizers:        []string{cancellation.RemediationRequestFinalizer},
			},
			Status: remediationv1.RemediationRequestStatus{
				OverallPhase: remediationv1.PhaseCancelling,
				Cancellation: &remediationv1.CancellationStatus{
					RequestID:   "system/system:cancellation",
					State:       remediationv1.CancellationStateInProgress,
					RequestedAt: &requestedAt,
				},
			},
		}
		c := fake.NewClientBuilder().WithScheme(scheme).WithObjects(rr).WithStatusSubresource(rr).Build()
		reconciler := NewReconciler(ReconcilerDeps{
			Client: c, APIReader: c, Scheme: scheme, AuditStore: &cancellationAuditStore{},
			Recorder: record.NewFakeRecorder(10), Metrics: rometrics.NewMetricsWithRegistry(prometheus.NewRegistry()),
		})

		_, err := reconciler.reconcileCancellation(context.Background(), rr, logr.Discard())
		Expect(err).NotTo(HaveOccurred())

		// The exhausted status was persisted BEFORE the finalizer release, so
		// the manual-review evidence survives; with the finalizer gone and a
		// deletion timestamp present, the object completes deletion.
		persisted := &remediationv1.RemediationRequest{}
		getErr := c.Get(context.Background(), client.ObjectKeyFromObject(rr), persisted)
		if getErr == nil {
			Expect(persisted.Finalizers).NotTo(ContainElement(cancellation.RemediationRequestFinalizer))
			Expect(persisted.Status.Cancellation.State).To(Equal(remediationv1.CancellationStateFailed),
				"the exhausted status evidence must be persisted before the finalizer release")
		} else {
			Expect(apierrors.IsNotFound(getErr)).To(BeTrue(),
				"the deleting RR must complete deletion once the finalizer is released")
		}
	})

	It("UT-RO-2588-021: a terminal-Cancelled RR with a stuck notification escalates once by wall-clock and releases for deletion (BR-ORCH-048)", func() {
		scheme := runtime.NewScheme()
		Expect(remediationv1.AddToScheme(scheme)).To(Succeed())
		Expect(notificationv1.AddToScheme(scheme)).To(Succeed())
		now := metav1.Now()
		requestedAt := metav1.NewTime(now.Add(-(cancellationWaitTimeout + time.Minute)))
		rr := &remediationv1.RemediationRequest{
			ObjectMeta: metav1.ObjectMeta{
				Name: "rr-cancelled-stuck-nr", Namespace: "system", UID: "rr-cancelled-stuck-nr-uid",
				DeletionTimestamp: &now,
				Finalizers:        []string{cancellation.RemediationRequestFinalizer},
			},
			Status: remediationv1.RemediationRequestStatus{
				OverallPhase: remediationv1.PhaseCancelled,
				Cancellation: &remediationv1.CancellationStatus{
					RequestID:   "system/system:cancellation",
					State:       remediationv1.CancellationStateInProgress,
					RequestedAt: &requestedAt,
				},
			},
		}
		// The cancellation NotificationRequest exists but never reaches a
		// terminal delivery phase (this suite runs no NR controller).
		nr := &notificationv1.NotificationRequest{
			ObjectMeta: metav1.ObjectMeta{Name: "nr-cancellation-rr-cancelled-stuck-nr", Namespace: "system"},
		}
		c := fake.NewClientBuilder().WithScheme(scheme).WithObjects(rr, nr).WithStatusSubresource(rr, nr).Build()
		store := &cancellationAuditStore{}
		reconciler := NewReconciler(ReconcilerDeps{
			Client: c, APIReader: c, Scheme: scheme, AuditStore: store,
			Recorder: record.NewFakeRecorder(10), Metrics: rometrics.NewMetricsWithRegistry(prometheus.NewRegistry()),
		})

		// First wake: the aged terminal-Cancelled RR escalates by wall-clock.
		_, err := reconciler.reconcileCancellation(context.Background(), rr, logr.Discard())
		Expect(err).NotTo(HaveOccurred())
		Expect(rr.Status.Cancellation.State).To(Equal(remediationv1.CancellationStateFailed))
		Expect(rr.Status.Message).To(ContainSubstring("wait budget"))
		waitExhausted := func() int {
			count := 0
			for _, e := range store.events {
				raw, marshalErr := json.Marshal(e.EventData)
				Expect(marshalErr).NotTo(HaveOccurred())
				if strings.Contains(string(raw), "cancellation.wait.exhausted") {
					count++
				}
			}
			return count
		}
		firstCount := waitExhausted()
		Expect(firstCount).To(BeNumerically(">", 0), "the wait-exhausted audit must be emitted")

		// The exhausted status was persisted before the finalizer release.
		persisted := &remediationv1.RemediationRequest{}
		getErr := c.Get(context.Background(), client.ObjectKeyFromObject(rr), persisted)
		if getErr == nil {
			Expect(persisted.Finalizers).NotTo(ContainElement(cancellation.RemediationRequestFinalizer))
			Expect(persisted.Status.Cancellation.State).To(Equal(remediationv1.CancellationStateFailed))
		} else {
			Expect(apierrors.IsNotFound(getErr)).To(BeTrue(),
				"the deleting RR must complete deletion once the finalizer is released")
		}

		// Second wake: drive reconcileCancelledNotificationWait directly —
		// the RR-level entry would exit early once the finalizer is gone
		// (cancellationRequested requires PhaseCancelled+finalizer), which
		// would make a guard assertion vacuous. The State=Failed guard must
		// hold without re-escalating, and the release must be idempotent.
		_, err = reconciler.reconcileCancelledNotificationWait(context.Background(), rr, logr.Discard())
		Expect(err).NotTo(HaveOccurred())
		Expect(waitExhausted()).To(Equal(firstCount),
			"an already-Failed cancellation must not re-emit wait-exhausted audits")
	})

	It("UT-RO-2588-022: a failed finalizer release on the escalated notification-wait path is retried on the next wake (BR-ORCH-048)", func() {
		scheme := runtime.NewScheme()
		Expect(remediationv1.AddToScheme(scheme)).To(Succeed())
		Expect(notificationv1.AddToScheme(scheme)).To(Succeed())
		now := metav1.Now()
		rr := &remediationv1.RemediationRequest{
			ObjectMeta: metav1.ObjectMeta{
				Name: "rr-release-retry", Namespace: "system", UID: "rr-release-retry-uid",
				DeletionTimestamp: &now,
				Finalizers:        []string{cancellation.RemediationRequestFinalizer},
			},
			Status: remediationv1.RemediationRequestStatus{
				OverallPhase: remediationv1.PhaseCancelled,
				Cancellation: &remediationv1.CancellationStatus{
					State: remediationv1.CancellationStateFailed,
				},
			},
		}
		c := fake.NewClientBuilder().WithScheme(scheme).WithObjects(rr).WithStatusSubresource(rr).Build()
		reconciler := NewReconciler(ReconcilerDeps{
			Client: c, APIReader: c, Scheme: scheme, AuditStore: &cancellationAuditStore{},
			Recorder: record.NewFakeRecorder(10), Metrics: rometrics.NewMetricsWithRegistry(prometheus.NewRegistry()),
		})

		// An already-escalated deleting RR retries the release on every wake;
		// a successful release completes the pending deletion.
		_, err := reconciler.reconcileCancelledNotificationWait(context.Background(), rr, logr.Discard())
		Expect(err).NotTo(HaveOccurred())
		persisted := &remediationv1.RemediationRequest{}
		getErr := c.Get(context.Background(), client.ObjectKeyFromObject(rr), persisted)
		if getErr != nil {
			Expect(apierrors.IsNotFound(getErr)).To(BeTrue(),
				"the deleting RR must complete deletion once the finalizer release is retried")
		} else {
			Expect(persisted.Finalizers).NotTo(ContainElement(cancellation.RemediationRequestFinalizer))
		}
	})

	It("UT-RO-2588-024: a fresh terminal-Cancelled wait requeues with backoff (BR-ORCH-048)", func() {
		recent := metav1.NewTime(metav1.Now().Add(-time.Minute))
		rr := &remediationv1.RemediationRequest{
			ObjectMeta: metav1.ObjectMeta{Name: "rr-fresh-cancelled-wait", Namespace: "system", UID: "rr-fresh-cancelled-wait-uid"},
			Status: remediationv1.RemediationRequestStatus{
				OverallPhase: remediationv1.PhaseCancelled,
				Cancellation: &remediationv1.CancellationStatus{
					State:       remediationv1.CancellationStateInProgress,
					RequestedAt: &recent,
				},
			},
		}
		reconciler := &Reconciler{}

		result, err := reconciler.reconcileCancelledNotificationWait(context.Background(), rr, logr.Discard())
		Expect(err).NotTo(HaveOccurred())
		Expect(result.RequeueAfter).To(BeNumerically(">", 0),
			"a recently requested notification wait must keep polling with backoff")
		Expect(rr.Status.Cancellation.State).To(Equal(remediationv1.CancellationStateInProgress),
			"a fresh wait must not escalate")
	})

	It("UT-AUDIT-2588-012: preserves request observations after concurrent phase progress (BR-AUDIT-005)", func() {
		scheme := runtime.NewScheme()
		Expect(remediationv1.AddToScheme(scheme)).To(Succeed())
		c := fake.NewClientBuilder().WithScheme(scheme).Build()
		store := &cancellationAuditStore{}
		reconciler := NewReconciler(ReconcilerDeps{
			Client: c, APIReader: c, Scheme: scheme, AuditStore: store,
			Recorder: record.NewFakeRecorder(10), Metrics: rometrics.NewMetricsWithRegistry(prometheus.NewRegistry()),
		})
		now := metav1.Now()
		rr := &remediationv1.RemediationRequest{
			ObjectMeta: metav1.ObjectMeta{Name: "rr-cancel-race", Namespace: "system", UID: "rr-cancel-race-uid", DeletionTimestamp: &now},
			Status:     remediationv1.RemediationRequestStatus{OverallPhase: remediationv1.PhasePending},
		}
		// Cancellation status was not persisted when another reconcile advanced
		// the parent. Retry sees the same incarnation and intent, but a new phase.
		retry := rr.DeepCopy()
		retry.Status.OverallPhase = remediationv1.PhaseProcessing
		reconciler.initializeCancellationStatus(context.Background(), rr, rr.Status.OverallPhase)
		reconciler.initializeCancellationStatus(context.Background(), retry, retry.Status.OverallPhase)
		Expect(store.events).To(HaveLen(4))
		Expect(rr.Status.Cancellation.RequestID).To(Equal(retry.Status.Cancellation.RequestID))
		Expect(store.events[1].EventID.Value).NotTo(Equal(store.events[3].EventID.Value))
		first, ok := store.events[1].EventData.GetRemediationOrchestratorAuditPayload()
		Expect(ok).To(BeTrue())
		second, ok := store.events[3].EventData.GetRemediationOrchestratorAuditPayload()
		Expect(ok).To(BeTrue())
		Expect(first.FromPhase.Value).To(Equal("Pending"))
		Expect(second.FromPhase.Value).To(Equal("Processing"))
	})

	It("UT-AUDIT-2588-004: initializes cancellation request identity before the first cancellation audit", func() {
		scheme := runtime.NewScheme()
		Expect(remediationv1.AddToScheme(scheme)).To(Succeed())
		rr := &remediationv1.RemediationRequest{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "rr-cancel-audit-identity",
				Namespace: "system",
				UID:       "rr-cancel-audit-identity-uid",
			},
			Status: remediationv1.RemediationRequestStatus{OverallPhase: remediationv1.PhaseExecuting},
		}
		auditStore := &cancellationAuditStore{}
		client := fake.NewClientBuilder().WithScheme(scheme).Build()
		reconciler := NewReconciler(ReconcilerDeps{
			Client:     client,
			APIReader:  client,
			Scheme:     scheme,
			AuditStore: auditStore,
			Recorder:   record.NewFakeRecorder(10),
			Metrics:    rometrics.NewMetricsWithRegistry(prometheus.NewRegistry()),
		})

		reconciler.initializeCancellationStatus(context.Background(), rr, remediationv1.PhaseExecuting)

		Expect(rr.Status.Cancellation).NotTo(BeNil())
		Expect(rr.Status.Cancellation.RequestID).NotTo(BeEmpty())
		Expect(auditStore.events).To(HaveLen(2))
		expected, err := roaudit.NewManager(roaudit.ServiceName).BuildCancellationLifecycleEvent(roaudit.CancellationLifecycleEventParams{
			CorrelationID: rr.Name,
			Namespace:     rr.Namespace,
			RRName:        rr.Name,
			RRUID:         string(rr.UID),
			RequestID:     rr.Status.Cancellation.RequestID,
			Stage:         "request",
			FromPhase:     string(remediationv1.PhaseExecuting),
			ToPhase:       string(remediationv1.PhaseCancelling),
		})
		Expect(err).NotTo(HaveOccurred())
		Expect(auditStore.events[1].EventID.Set).To(BeTrue())
		Expect(auditStore.events[1].EventID.Value).To(Equal(expected.EventID.Value),
			"the first cancellation audit must use the initialized request identity")
	})

	It("UT-ORCH-2588-004: removes the cancellation finalizer from ordinary terminal RRs", func() {
		terminalPhases := []remediationv1.RemediationPhase{
			remediationv1.PhaseCompleted,
			remediationv1.PhaseFailed,
			remediationv1.PhaseTimedOut,
			remediationv1.PhaseSkipped,
		}
		for _, terminalPhase := range terminalPhases {
			scheme := runtime.NewScheme()
			Expect(remediationv1.AddToScheme(scheme)).To(Succeed())
			rr := &remediationv1.RemediationRequest{
				ObjectMeta: metav1.ObjectMeta{
					Name:       "rr-terminal-finalizer-" + strings.ToLower(string(terminalPhase)),
					Namespace:  "system",
					Finalizers: []string{remediationRequestCancellationFinalizer},
				},
				Status: remediationv1.RemediationRequestStatus{
					OverallPhase: terminalPhase,
					RetentionExpiryTime: func() *metav1.Time {
						future := metav1.NewTime(time.Now().Add(time.Hour))
						return &future
					}(),
				},
			}
			c := fake.NewClientBuilder().WithScheme(scheme).
				WithStatusSubresource(rr).WithObjects(rr).Build()
			reconciler := NewReconciler(ReconcilerDeps{
				Client:    c,
				APIReader: c,
				Scheme:    scheme,
				Recorder:  record.NewFakeRecorder(10),
				Metrics:   rometrics.NewMetricsWithRegistry(prometheus.NewRegistry()),
			})

			_, err := reconciler.Reconcile(context.Background(), ctrl.Request{
				NamespacedName: types.NamespacedName{Name: rr.Name, Namespace: rr.Namespace},
			})
			Expect(err).NotTo(HaveOccurred())

			var got remediationv1.RemediationRequest
			Expect(c.Get(context.Background(), client.ObjectKeyFromObject(rr), &got)).To(Succeed())
			Expect(got.Status.OverallPhase).To(Equal(terminalPhase))
			Expect(got.Finalizers).NotTo(ContainElement(remediationRequestCancellationFinalizer))
		}
	})

	It("UT-ORCH-2588-008: does not reinterpret ordinary terminal deletion as cancellation", func() {
		terminalPhases := []remediationv1.RemediationPhase{
			remediationv1.PhaseCompleted,
			remediationv1.PhaseFailed,
			remediationv1.PhaseTimedOut,
			remediationv1.PhaseSkipped,
		}
		now := metav1.Now()
		for _, terminalPhase := range terminalPhases {
			rr := &remediationv1.RemediationRequest{
				ObjectMeta: metav1.ObjectMeta{
					Name:              "rr-terminal-delete-" + strings.ToLower(string(terminalPhase)),
					Namespace:         "system",
					DeletionTimestamp: &now,
					Finalizers:        []string{remediationRequestCancellationFinalizer},
				},
				Status: remediationv1.RemediationRequestStatus{OverallPhase: terminalPhase},
			}

			Expect(cancellationRequested(rr)).To(BeFalse(), "terminal phase %s must not enter cancellation", terminalPhase)
		}
	})

	It("UT-ORCH-2588-012: refuses system RAR cancellation for a stale parent UID", func() {
		scheme := runtime.NewScheme()
		Expect(remediationv1.AddToScheme(scheme)).To(Succeed())
		rr := &remediationv1.RemediationRequest{
			ObjectMeta: metav1.ObjectMeta{Name: "rr-rar-stale", Namespace: "system", UID: types.UID("rr-current-uid")},
			Status:     remediationv1.RemediationRequestStatus{OverallPhase: remediationv1.PhaseCancelling},
		}
		rar := &remediationv1.RemediationApprovalRequest{
			ObjectMeta: metav1.ObjectMeta{Name: "rar-rr-rar-stale", Namespace: "system"},
			Spec: remediationv1.RemediationApprovalRequestSpec{
				RemediationRequestRef: corev1.ObjectReference{
					Name: "rr-rar-stale", Namespace: "system", UID: types.UID("rr-old-uid"),
				},
			},
			Status: remediationv1.RemediationApprovalRequestStatus{Decision: remediationv1.ApprovalDecisionPending},
		}
		c := fake.NewClientBuilder().WithScheme(scheme).
			WithStatusSubresource(rr, rar).
			WithObjects(rr, rar).Build()
		reconciler := &Reconciler{client: c}

		acknowledged, err := reconciler.cancelRARIfNeeded(context.Background(), rr)
		Expect(acknowledged).To(BeFalse())
		Expect(err).To(MatchError(ContainSubstring("identity mismatch")))

		var got remediationv1.RemediationApprovalRequest
		Expect(c.Get(context.Background(), client.ObjectKeyFromObject(rar), &got)).To(Succeed())
		Expect(got.Status.Decision).To(Equal(remediationv1.ApprovalDecisionPending))
	})

	It("IT-RO-2588-001: retains the parent until a child acknowledges verified stop without requiring an execution ref", func() {
		scheme := runtime.NewScheme()
		Expect(remediationv1.AddToScheme(scheme)).To(Succeed())
		Expect(aianalysisv1.AddToScheme(scheme)).To(Succeed())
		Expect(notificationv1.AddToScheme(scheme)).To(Succeed())
		Expect(workflowexecutionv1.AddToScheme(scheme)).To(Succeed())

		rr := &remediationv1.RemediationRequest{
			ObjectMeta: metav1.ObjectMeta{
				Name:       "rr-cancel-aggregate",
				Namespace:  "system",
				UID:        "rr-cancel-aggregate-uid",
				Finalizers: []string{remediationRequestCancellationFinalizer},
			},
			Status: remediationv1.RemediationRequestStatus{
				OverallPhase: remediationv1.PhaseCancelling,
				Cancellation: &remediationv1.CancellationStatus{
					RequestID: "system/rr-cancel-aggregate:cancellation",
					State:     remediationv1.CancellationStateRequested,
				},
				PhaseProgress: &remediationv1.PhaseProgress{
					WorkflowExecutionRef: &corev1.ObjectReference{Name: "wfe-cancel-aggregate", Namespace: "system"},
				},
			},
		}
		wfe := &workflowexecutionv1.WorkflowExecution{
			ObjectMeta: metav1.ObjectMeta{Name: "wfe-cancel-aggregate", Namespace: "system"},
			Spec: workflowexecutionv1.WorkflowExecutionSpec{
				RemediationRequestRef: corev1.ObjectReference{
					Name: rr.Name, Namespace: rr.Namespace, UID: rr.UID,
				},
			},
			Status: workflowexecutionv1.WorkflowExecutionStatus{
				Phase: workflowexecutionv1.PhaseCancelling,
				Conditions: func() []metav1.Condition {
					conditions := []metav1.Condition{}
					cancellation.SetRequested(&conditions, 1, "backend stop pending")
					return conditions
				}(),
			},
		}
		Expect(controllerutil.SetControllerReference(rr, wfe, scheme)).To(Succeed())
		rar := &remediationv1.RemediationApprovalRequest{
			ObjectMeta: metav1.ObjectMeta{Name: "rar-rr-cancel-aggregate", Namespace: "system"},
			Spec: remediationv1.RemediationApprovalRequestSpec{
				RemediationRequestRef: corev1.ObjectReference{Name: rr.Name, Namespace: rr.Namespace, UID: rr.UID},
			},
		}
		c := fake.NewClientBuilder().WithScheme(scheme).
			WithStatusSubresource(rr, wfe, rar, &notificationv1.NotificationRequest{}).
			WithObjects(rr, wfe, rar).Build()
		auditStore := &cancellationAuditStore{}
		reconciler := NewReconciler(ReconcilerDeps{
			Client:     c,
			APIReader:  c,
			Scheme:     scheme,
			AuditStore: auditStore,
			Recorder:   record.NewFakeRecorder(10),
			Metrics:    rometrics.NewMetricsWithRegistry(prometheus.NewRegistry()),
		})

		request := ctrl.Request{NamespacedName: types.NamespacedName{Name: rr.Name, Namespace: rr.Namespace}}
		_, err := reconciler.Reconcile(context.Background(), request)
		Expect(err).NotTo(HaveOccurred())

		var gotRR remediationv1.RemediationRequest
		Expect(c.Get(context.Background(), client.ObjectKeyFromObject(rr), &gotRR)).To(Succeed())
		Expect(gotRR.Status.OverallPhase).To(Equal(remediationv1.PhaseCancelling))
		Expect(gotRR.Status.Cancellation.State).To(Equal(remediationv1.CancellationStateInProgress))
		Expect(gotRR.Finalizers).To(ContainElement(remediationRequestCancellationFinalizer))

		var gotNotification notificationv1.NotificationRequest
		Expect(c.Get(context.Background(), types.NamespacedName{Name: "nr-cancellation-" + rr.Name, Namespace: rr.Namespace}, &gotNotification)).To(Succeed())
		gotNotification.Status.Phase = notificationv1.NotificationPhaseSent
		Expect(c.Status().Update(context.Background(), &gotNotification)).To(Succeed())

		var gotWFE workflowexecutionv1.WorkflowExecution
		Expect(c.Get(context.Background(), client.ObjectKeyFromObject(wfe), &gotWFE)).To(Succeed())
		cancellation.SetAcknowledged(&gotWFE.Status.Conditions, gotWFE.Generation, "backend termination verified")
		Expect(c.Status().Update(context.Background(), &gotWFE)).To(Succeed())

		_, err = reconciler.Reconcile(context.Background(), request)
		Expect(err).NotTo(HaveOccurred())
		Expect(c.Get(context.Background(), client.ObjectKeyFromObject(rr), &gotRR)).To(Succeed())
		Expect(gotRR.Status.OverallPhase).To(Equal(remediationv1.PhaseCancelled))
		Expect(gotRR.Status.Cancellation.State).To(Equal(remediationv1.CancellationStateCompleted))
		Expect(gotRR.Finalizers).NotTo(ContainElement(remediationRequestCancellationFinalizer))

		var gotRAR remediationv1.RemediationApprovalRequest
		Expect(c.Get(context.Background(), client.ObjectKeyFromObject(rar), &gotRAR)).To(Succeed())
		Expect(gotRAR.Status.Decision).To(Equal(remediationv1.ApprovalDecisionCancelled))
		Expect(gotRAR.Status.DecidedBy).To(Equal("system"))

		observedStages := map[string]bool{}
		for _, event := range auditStore.events {
			payload, ok := event.EventData.GetRemediationOrchestratorAuditPayload()
			if ok {
				observedStages[payload.ToPhase.Value] = true
				Expect(event.CorrelationID).To(Equal(rr.Name))
				if strings.HasPrefix(payload.ToPhase.Value, "cancellation.") {
					Expect(event.EventID.Set).To(BeTrue())
				}
			}
		}
		Expect(observedStages).To(HaveKey("cancellation.workflowexecution.acknowledged"))
		Expect(observedStages).To(HaveKey("cancellation.workflowexecution.termination_verified"))
		Expect(observedStages).To(HaveKey("cancellation.notification.created"))
	})

	It("UT-NT-2588-004: does not recreate a user-deleted cancellation notification", func() {
		scheme := runtime.NewScheme()
		Expect(remediationv1.AddToScheme(scheme)).To(Succeed())
		Expect(notificationv1.AddToScheme(scheme)).To(Succeed())

		notificationName := "nr-cancellation-rr-deleted-notification"
		rr := &remediationv1.RemediationRequest{
			ObjectMeta: metav1.ObjectMeta{
				Name:       "rr-deleted-notification",
				Namespace:  "system",
				UID:        types.UID("rr-deleted-notification-uid"),
				Finalizers: []string{remediationRequestCancellationFinalizer},
			},
			Status: remediationv1.RemediationRequestStatus{
				OverallPhase: remediationv1.PhaseCancelling,
				Cancellation: &remediationv1.CancellationStatus{
					RequestID: "system/rr-deleted-notification:cancellation",
					State:     remediationv1.CancellationStateInProgress,
				},
				CompletionStatus: &remediationv1.CompletionStatus{
					NotificationRequestRefs: []corev1.ObjectReference{{Name: notificationName, Namespace: "system"}},
				},
			},
		}
		notification := &notificationv1.NotificationRequest{
			ObjectMeta: metav1.ObjectMeta{Name: notificationName, Namespace: rr.Namespace},
		}
		c := fake.NewClientBuilder().WithScheme(scheme).
			WithStatusSubresource(rr, notification).
			WithObjects(rr, notification).Build()
		Expect(c.Delete(context.Background(), notification)).To(Succeed())

		reconciler := NewReconciler(ReconcilerDeps{
			Client:    c,
			APIReader: c,
			Scheme:    scheme,
			Recorder:  record.NewFakeRecorder(10),
			Metrics:   rometrics.NewMetricsWithRegistry(prometheus.NewRegistry()),
		})
		_, err := reconciler.Reconcile(context.Background(), ctrl.Request{NamespacedName: client.ObjectKeyFromObject(rr)})
		Expect(err).NotTo(HaveOccurred())

		var gotNotification notificationv1.NotificationRequest
		err = c.Get(context.Background(), client.ObjectKeyFromObject(notification), &gotNotification)
		Expect(apierrors.IsNotFound(err)).To(BeTrue())
		var gotRR remediationv1.RemediationRequest
		Expect(c.Get(context.Background(), client.ObjectKeyFromObject(rr), &gotRR)).To(Succeed())
		Expect(gotRR.Status.OverallPhase).To(Equal(remediationv1.PhaseCancelled))
		Expect(gotRR.Status.EnsureCompletionStatus().NotificationStatus).To(Equal("Cancelled"))
		Expect(gotRR.Finalizers).NotTo(ContainElement(remediationRequestCancellationFinalizer))
	})

	It("UT-ORCH-2588-019: resolves namespace-less legacy child references in the parent namespace", func() {
		scheme := runtime.NewScheme()
		Expect(remediationv1.AddToScheme(scheme)).To(Succeed())
		Expect(workflowexecutionv1.AddToScheme(scheme)).To(Succeed())
		rr := &remediationv1.RemediationRequest{
			ObjectMeta: metav1.ObjectMeta{Name: "rr-legacy-child-namespace", Namespace: "system", UID: types.UID("rr-legacy-child-namespace-uid")},
			Status: remediationv1.RemediationRequestStatus{
				OverallPhase: remediationv1.PhaseCancelling,
				PhaseProgress: &remediationv1.PhaseProgress{
					WorkflowExecutionRef: &corev1.ObjectReference{Name: "wfe-legacy-child-namespace"},
				},
			},
		}
		wfe := &workflowexecutionv1.WorkflowExecution{
			ObjectMeta: metav1.ObjectMeta{Name: "wfe-legacy-child-namespace", Namespace: rr.Namespace},
			Spec: workflowexecutionv1.WorkflowExecutionSpec{
				RemediationRequestRef: corev1.ObjectReference{Name: rr.Name, UID: rr.UID},
			},
			Status: workflowexecutionv1.WorkflowExecutionStatus{
				Phase: workflowexecutionv1.PhaseCancelling,
				Conditions: func() []metav1.Condition {
					conditions := []metav1.Condition{}
					cancellation.SetRequested(&conditions, 1, "backend stop pending")
					return conditions
				}(),
			},
		}
		c := fake.NewClientBuilder().WithScheme(scheme).
			WithStatusSubresource(rr, wfe).
			WithObjects(rr, wfe).Build()
		reconciler := &Reconciler{client: c}

		acknowledged, failure, err := reconciler.cancellationChildrenState(context.Background(), rr)
		Expect(err).NotTo(HaveOccurred())
		Expect(acknowledged).To(BeFalse())
		Expect(failure).To(BeEmpty())
	})

	It("UT-ORCH-2588-014: refuses to aggregate acknowledgement from a stale same-name child", func() {
		scheme := runtime.NewScheme()
		Expect(remediationv1.AddToScheme(scheme)).To(Succeed())
		Expect(workflowexecutionv1.AddToScheme(scheme)).To(Succeed())
		rr := &remediationv1.RemediationRequest{
			ObjectMeta: metav1.ObjectMeta{Name: "rr-aggregate-stale", Namespace: "system", UID: types.UID("rr-current-uid")},
			Status: remediationv1.RemediationRequestStatus{
				OverallPhase: remediationv1.PhaseCancelling,
				PhaseProgress: &remediationv1.PhaseProgress{
					WorkflowExecutionRef: &corev1.ObjectReference{Name: "wfe-aggregate-stale", Namespace: "system"},
				},
			},
		}
		wfe := &workflowexecutionv1.WorkflowExecution{
			ObjectMeta: metav1.ObjectMeta{Name: "wfe-aggregate-stale", Namespace: "system"},
			Spec: workflowexecutionv1.WorkflowExecutionSpec{
				RemediationRequestRef: corev1.ObjectReference{
					Name: rr.Name, Namespace: rr.Namespace, UID: types.UID("rr-old-uid"),
				},
			},
			Status: workflowexecutionv1.WorkflowExecutionStatus{Phase: workflowexecutionv1.PhaseCancelled, Conditions: func() []metav1.Condition {
				conditions := []metav1.Condition{}
				cancellation.SetAcknowledged(&conditions, 1, "backend termination verified")
				return conditions
			}()},
		}
		c := fake.NewClientBuilder().WithScheme(scheme).
			WithStatusSubresource(rr, wfe).
			WithObjects(rr, wfe).Build()
		reconciler := &Reconciler{client: c}

		acknowledged, _, err := reconciler.cancellationChildrenState(context.Background(), rr)
		Expect(acknowledged).To(BeFalse())
		Expect(err).To(MatchError(ContainSubstring("identity mismatch")))
	})

	It("UT-ORCH-2588-020: refuses acknowledgement from a fully UID-less legacy child", func() {
		scheme := runtime.NewScheme()
		Expect(remediationv1.AddToScheme(scheme)).To(Succeed())
		Expect(workflowexecutionv1.AddToScheme(scheme)).To(Succeed())
		rr := &remediationv1.RemediationRequest{
			ObjectMeta: metav1.ObjectMeta{Name: "rr-aggregate-legacy", Namespace: "system", UID: types.UID("rr-current-legacy-uid")},
			Status: remediationv1.RemediationRequestStatus{
				OverallPhase: remediationv1.PhaseCancelling,
				PhaseProgress: &remediationv1.PhaseProgress{
					WorkflowExecutionRef: &corev1.ObjectReference{Name: "wfe-aggregate-legacy", Namespace: "system"},
				},
			},
		}
		wfe := &workflowexecutionv1.WorkflowExecution{
			ObjectMeta: metav1.ObjectMeta{Name: "wfe-aggregate-legacy", Namespace: "system"},
			Spec: workflowexecutionv1.WorkflowExecutionSpec{
				RemediationRequestRef: corev1.ObjectReference{Name: rr.Name, Namespace: rr.Namespace},
			},
			Status: workflowexecutionv1.WorkflowExecutionStatus{Phase: workflowexecutionv1.PhaseCancelled, Conditions: func() []metav1.Condition {
				conditions := []metav1.Condition{}
				cancellation.SetAcknowledged(&conditions, 1, "backend termination verified")
				return conditions
			}()},
		}
		c := fake.NewClientBuilder().WithScheme(scheme).
			WithStatusSubresource(rr, wfe).
			WithObjects(rr, wfe).Build()
		reconciler := &Reconciler{client: c}

		acknowledged, _, err := reconciler.cancellationChildrenState(context.Background(), rr)
		Expect(acknowledged).To(BeFalse())
		Expect(err).To(MatchError(ContainSubstring("identity mismatch")))
	})

	It("UT-ORCH-2588-028: does not consume the aggregate retry budget while a child stop is pending", func() {
		conditions := []metav1.Condition{}
		cancellation.SetRequested(&conditions, 1, "backend stop is still pending")

		acknowledged, failure := childCancellationState(conditions)
		Expect(acknowledged).To(BeFalse())
		Expect(failure).To(BeEmpty())

		rr := &remediationv1.RemediationRequest{
			Status: remediationv1.RemediationRequestStatus{
				OverallPhase: remediationv1.PhaseCancelling,
				Cancellation: &remediationv1.CancellationStatus{
					State: remediationv1.CancellationStateInProgress,
				},
			},
		}
		reconciler := &Reconciler{}

		reconciler.recordCancellationProgress(
			context.Background(), rr, false, "", false, failure, logr.Discard(),
		)

		Expect(rr.Status.Cancellation.RetryCount).To(BeZero())
		Expect(rr.Status.Cancellation.State).To(Equal(remediationv1.CancellationStateInProgress))
		Expect(rr.Status.Message).To(Equal("Cancellation requested; waiting for child acknowledgements"))
	})

	It("UT-NT-2588-003: records PartiallySent as terminal but not fully delivered", func() {
		metrics := rometrics.NewMetricsWithRegistry(prometheus.NewRegistry())
		handler := NewNotificationHandler(nil, metrics)
		rr := &remediationv1.RemediationRequest{
			ObjectMeta: metav1.ObjectMeta{Name: "rr-partial-notification", Namespace: "system"},
			Status:     remediationv1.RemediationRequestStatus{OverallPhase: remediationv1.PhaseCancelling},
		}
		notif := &notificationv1.NotificationRequest{
			ObjectMeta: metav1.ObjectMeta{Name: "nr-cancellation-rr-partial-notification"},
			Status: notificationv1.NotificationRequestStatus{
				Phase:   notificationv1.NotificationPhasePartiallySent,
				Message: "slack channel exhausted retries",
			},
		}

		Expect(handler.UpdateNotificationStatus(context.Background(), rr, notif)).To(Succeed())
		Expect(rr.Status.EnsureCompletionStatus().NotificationStatus).To(Equal("PartiallySent"))
		condition := meta.FindStatusCondition(rr.Status.Conditions, "NotificationDelivered")
		Expect(condition).NotTo(BeNil())
		Expect(condition.Status).To(Equal(metav1.ConditionFalse))
		Expect(condition.Reason).To(Equal("DeliveryFailed"))
	})

	It("UT-AUDIT-2588-005: omits failure phase for cancellation notification creation failures", func() {
		scheme := runtime.NewScheme()
		Expect(remediationv1.AddToScheme(scheme)).To(Succeed())
		Expect(notificationv1.AddToScheme(scheme)).To(Succeed())
		rr := &remediationv1.RemediationRequest{
			ObjectMeta: metav1.ObjectMeta{Name: "rr-notification-audit-phase", Namespace: "system"},
			Status: remediationv1.RemediationRequestStatus{
				Cancellation: &remediationv1.CancellationStatus{RequestID: "system/rr-notification-audit-phase:cancellation"},
			},
		}
		c := fake.NewClientBuilder().WithScheme(scheme).Build()
		auditStore := &cancellationAuditStore{}
		reconciler := &Reconciler{
			client:              c,
			notificationCreator: rocreator.NewNotificationCreator(c, scheme, rometrics.NewMetricsWithRegistry(prometheus.NewRegistry())),
			auditStore:          auditStore,
			auditManager:        roaudit.NewManager(roaudit.ServiceName),
		}

		ready, failure, creationFailed, err := reconciler.prepareCancellationNotification(context.Background(), rr, logr.Discard())
		Expect(err).NotTo(HaveOccurred())
		Expect(ready).To(BeFalse())
		Expect(failure).NotTo(BeEmpty())
		Expect(creationFailed).To(BeTrue())
		Expect(auditStore.events).To(HaveLen(1))
		payload, ok := auditStore.events[0].EventData.GetRemediationOrchestratorAuditPayload()
		Expect(ok).To(BeTrue())
		Expect(payload.FailurePhase.IsSet()).To(BeFalse(), "notification failure is not a WorkflowExecution failure")
	})

	It("UT-ORCH-2588-003: retries cancellation finalizer removal after a transient update failure", func() {
		scheme := runtime.NewScheme()
		Expect(remediationv1.AddToScheme(scheme)).To(Succeed())
		Expect(notificationv1.AddToScheme(scheme)).To(Succeed())

		rr := &remediationv1.RemediationRequest{
			ObjectMeta: metav1.ObjectMeta{
				Name:       "rr-cancel-finalizer-retry",
				Namespace:  "system",
				UID:        "rr-cancel-finalizer-retry-uid",
				Finalizers: []string{remediationRequestCancellationFinalizer},
			},
			Status: remediationv1.RemediationRequestStatus{
				OverallPhase: remediationv1.PhaseCancelled,
				Cancellation: &remediationv1.CancellationStatus{
					RequestID: "system/rr-cancel-finalizer-retry:cancellation",
					State:     remediationv1.CancellationStateCompleted,
				},
			},
		}
		notification := &notificationv1.NotificationRequest{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "nr-cancellation-" + rr.Name,
				Namespace: rr.Namespace,
			},
			Spec: notificationv1.NotificationRequestSpec{
				RemediationRequestRef: &corev1.ObjectReference{
					APIVersion: remediationv1.GroupVersion.String(),
					Kind:       "RemediationRequest",
					Name:       rr.Name,
					Namespace:  rr.Namespace,
					UID:        rr.UID,
				},
				Type: notificationv1.NotificationTypeCancellation,
				Extensions: map[string]string{
					"cancellationRequestID": rr.Status.Cancellation.RequestID,
				},
			},
			Status: notificationv1.NotificationRequestStatus{
				Phase: notificationv1.NotificationPhaseSent,
			},
		}
		Expect(controllerutil.SetControllerReference(rr, notification, scheme)).To(Succeed())
		baseClient := fake.NewClientBuilder().WithScheme(scheme).
			WithStatusSubresource(rr, notification).
			WithObjects(rr, notification).Build()
		flakyClient := &failOnceOnCancellationFinalizerRemovalClient{
			Client: baseClient,
			fail:   true,
		}
		reconciler := NewReconciler(ReconcilerDeps{
			Client:    flakyClient,
			APIReader: flakyClient,
			Scheme:    scheme,
			Recorder:  record.NewFakeRecorder(10),
			Metrics:   rometrics.NewMetricsWithRegistry(prometheus.NewRegistry()),
		})
		request := ctrl.Request{NamespacedName: types.NamespacedName{
			Name: rr.Name, Namespace: rr.Namespace,
		}}
		Expect(cancellationRequested(rr)).To(BeTrue())

		_, err := reconciler.Reconcile(context.Background(), request)
		Expect(err).To(MatchError("remove completed cancellation finalizer: injected cancellation finalizer removal failure"))

		var afterFailure remediationv1.RemediationRequest
		Expect(baseClient.Get(context.Background(), client.ObjectKeyFromObject(rr), &afterFailure)).To(Succeed())
		Expect(afterFailure.Finalizers).To(ContainElement(remediationRequestCancellationFinalizer))

		_, err = reconciler.Reconcile(context.Background(), request)
		Expect(err).NotTo(HaveOccurred())
		var afterRetry remediationv1.RemediationRequest
		Expect(baseClient.Get(context.Background(), client.ObjectKeyFromObject(rr), &afterRetry)).To(Succeed())
		Expect(controllerutil.ContainsFinalizer(&afterRetry, remediationRequestCancellationFinalizer)).To(BeFalse())
	})

	It("UT-ORCH-2588-005: consumes retry budget when notification failure persists after child acknowledgement", func() {
		rr := &remediationv1.RemediationRequest{
			ObjectMeta: metav1.ObjectMeta{Name: "rr-cancel-notification-retry", Namespace: "system", UID: "rr-cancel-notification-retry-uid"},
			Status: remediationv1.RemediationRequestStatus{
				OverallPhase: remediationv1.PhaseCancelling,
				Cancellation: &remediationv1.CancellationStatus{
					State:      remediationv1.CancellationStateInProgress,
					RetryCount: remediationCancellationMaxRetries - 2,
				},
			},
		}
		scheme := runtime.NewScheme()
		Expect(remediationv1.AddToScheme(scheme)).To(Succeed())
		Expect(notificationv1.AddToScheme(scheme)).To(Succeed())
		c := fake.NewClientBuilder().WithScheme(scheme).WithObjects(rr).Build()
		auditStore := &cancellationAuditStore{}
		reconciler := &Reconciler{
			client:              c,
			notificationCreator: rocreator.NewNotificationCreator(c, scheme, rometrics.NewMetricsWithRegistry(prometheus.NewRegistry())),
			auditStore:          auditStore,
			auditManager:        roaudit.NewManager(roaudit.ServiceName),
		}

		reconciler.recordCancellationProgress(
			context.Background(), rr, true, "cancellation notification creation failed", true, "", logr.Discard(),
		)
		Expect(rr.Status.Cancellation.RetryCount).To(Equal(remediationCancellationMaxRetries - 1))
		Expect(rr.Status.Cancellation.State).To(Equal(remediationv1.CancellationStateInProgress))

		reconciler.recordCancellationProgress(
			context.Background(), rr, true, "cancellation notification creation failed", true, "", logr.Discard(),
		)
		Expect(rr.Status.Cancellation.RetryCount).To(Equal(remediationCancellationMaxRetries))
		Expect(rr.Status.Cancellation.State).To(Equal(remediationv1.CancellationStateFailed))

		observedStages := map[string]bool{}
		for _, event := range auditStore.events {
			payload, ok := event.EventData.GetRemediationOrchestratorAuditPayload()
			if ok && payload.ToPhase.IsSet() {
				observedStages[payload.ToPhase.Value] = true
			}
		}
		Expect(observedStages).To(HaveKey("cancellation.failed"))
		// BR-ORCH-048: the retry-budget path must attribute its exhaustion to
		// the retry stage, never to the wall-clock wait stage.
		for _, event := range auditStore.events {
			raw, marshalErr := json.Marshal(event.EventData)
			Expect(marshalErr).NotTo(HaveOccurred())
			Expect(string(raw)).NotTo(ContainSubstring("cancellation.wait.exhausted"),
				"the retry-budget path must not emit the wait-exhausted audit stage")
		}
		Expect(observedStages).To(HaveKey("cancellation.manual_review.required"))
	})

	It("UT-RO-2588-023: child-stop retry exhaustion attributes to the retry stage, never the wait stage (BR-ORCH-048)", func() {
		rr := &remediationv1.RemediationRequest{
			ObjectMeta: metav1.ObjectMeta{Name: "rr-child-stop-exhaust", Namespace: "system", UID: "rr-child-stop-exhaust-uid"},
			Status: remediationv1.RemediationRequestStatus{
				OverallPhase: remediationv1.PhaseCancelling,
				Cancellation: &remediationv1.CancellationStatus{
					State:      remediationv1.CancellationStateInProgress,
					RetryCount: remediationCancellationMaxRetries - 1,
				},
			},
		}
		scheme := runtime.NewScheme()
		Expect(remediationv1.AddToScheme(scheme)).To(Succeed())
		Expect(notificationv1.AddToScheme(scheme)).To(Succeed())
		c := fake.NewClientBuilder().WithScheme(scheme).WithObjects(rr).Build()
		auditStore := &cancellationAuditStore{}
		reconciler := &Reconciler{
			client:              c,
			notificationCreator: rocreator.NewNotificationCreator(c, scheme, rometrics.NewMetricsWithRegistry(prometheus.NewRegistry())),
			auditStore:          auditStore,
			auditManager:        roaudit.NewManager(roaudit.ServiceName),
		}

		// A persistent child-stop failure exhausts the retry budget through
		// the child-failure branch of recordCancellationProgress.
		reconciler.recordCancellationProgress(
			context.Background(), rr, false, "", false, "child did not stop", logr.Discard(),
		)
		Expect(rr.Status.Cancellation.RetryCount).To(Equal(remediationCancellationMaxRetries))
		Expect(rr.Status.Cancellation.State).To(Equal(remediationv1.CancellationStateFailed))
		Expect(rr.Status.Message).To(ContainSubstring("manual review required"))

		for _, event := range auditStore.events {
			raw, marshalErr := json.Marshal(event.EventData)
			Expect(marshalErr).NotTo(HaveOccurred())
			Expect(string(raw)).NotTo(ContainSubstring("cancellation.wait.exhausted"),
				"the child-stop retry-exhaustion path must not emit the wait-exhausted audit stage")
		}
	})
})

type failOnceOnCancellationFinalizerRemovalClient struct {
	client.Client
	fail bool
}

func (c *failOnceOnCancellationFinalizerRemovalClient) Update(ctx context.Context, obj client.Object, opts ...client.UpdateOption) error {
	rr, isRR := obj.(*remediationv1.RemediationRequest)
	if c.fail && isRR && rr.Status.OverallPhase == remediationv1.PhaseCancelled &&
		!controllerutil.ContainsFinalizer(rr, remediationRequestCancellationFinalizer) {
		c.fail = false
		return errors.New("injected cancellation finalizer removal failure")
	}
	return c.Client.Update(ctx, obj, opts...)
}
