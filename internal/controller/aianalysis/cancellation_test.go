package aianalysis_test

import (
	"context"
	"errors"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/controller-runtime/pkg/client/interceptor"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"

	agentsessionv1 "github.com/jordigilh/kubernaut/api/agentsession/v1alpha1"
	aianalysisv1 "github.com/jordigilh/kubernaut/api/aianalysis/v1alpha1"
	remediationv1 "github.com/jordigilh/kubernaut/api/remediation/v1alpha1"
	controller "github.com/jordigilh/kubernaut/internal/controller/aianalysis"
	"github.com/jordigilh/kubernaut/pkg/aianalysis/creator"
	"github.com/jordigilh/kubernaut/pkg/shared/cancellation"
)

var _ = Describe("AIAnalysis parent cancellation", func() {
	It("UT-AI-2588-002: keeps cancellation nonterminal until an existing AgentSession is observed stopping", func() {
		scheme := runtime.NewScheme()
		Expect(remediationv1.AddToScheme(scheme)).To(Succeed())
		Expect(aianalysisv1.AddToScheme(scheme)).To(Succeed())
		Expect(agentsessionv1.AddToScheme(scheme)).To(Succeed())

		parent := &remediationv1.RemediationRequest{
			ObjectMeta: metav1.ObjectMeta{Name: "rr-stale-session", Namespace: "system", UID: types.UID("rr-stale-session-uid")},
			Status:     remediationv1.RemediationRequestStatus{OverallPhase: remediationv1.PhaseCancelling},
		}
		analysis := &aianalysisv1.AIAnalysis{
			ObjectMeta: metav1.ObjectMeta{Name: "aa-stale-session", Namespace: "system", UID: types.UID("aa-stale-session-uid")},
			Spec: aianalysisv1.AIAnalysisSpec{
				RemediationRequestRef: corev1.ObjectReference{Name: parent.Name, Namespace: parent.Namespace, UID: parent.UID},
			},
			Status: aianalysisv1.AIAnalysisStatus{Phase: aianalysisv1.PhaseInvestigating},
		}
		session := &agentsessionv1.AgentSession{
			ObjectMeta: metav1.ObjectMeta{
				Name:       "as-" + parent.Name,
				Namespace:  parent.Namespace,
				Finalizers: []string{agentsessionv1.DispatchCleanupFinalizer},
			},
			Status: agentsessionv1.AgentSessionStatus{Phase: agentsessionv1.AgentSessionPhaseInvestigating},
		}
		Expect(controllerutil.SetControllerReference(analysis, session, scheme)).To(Succeed())
		c := fake.NewClientBuilder().WithScheme(scheme).
			WithStatusSubresource(parent, analysis, session).
			WithObjects(parent, analysis, session).Build()
		reconciler := &controller.AIAnalysisReconciler{
			Client:              c,
			AgentSessionCreator: creator.NewAgentSessionCreator(c, scheme),
		}

		_, err := reconciler.Reconcile(context.Background(), ctrl.Request{NamespacedName: types.NamespacedName{Name: analysis.Name, Namespace: analysis.Namespace}})
		Expect(err).NotTo(HaveOccurred())
		var got aianalysisv1.AIAnalysis
		Expect(c.Get(context.Background(), client.ObjectKeyFromObject(analysis), &got)).To(Succeed())
		Expect(got.Status.Phase).To(Equal(aianalysisv1.PhaseCancelling))
		Expect(cancellation.IsAcknowledged(got.Status.Conditions)).To(BeFalse())
	})

	It("IT-AI-2588-001: stops the AgentSession before acknowledging cancellation", func() {
		scheme := runtime.NewScheme()
		Expect(remediationv1.AddToScheme(scheme)).To(Succeed())
		Expect(aianalysisv1.AddToScheme(scheme)).To(Succeed())
		Expect(agentsessionv1.AddToScheme(scheme)).To(Succeed())

		parent := &remediationv1.RemediationRequest{
			ObjectMeta: metav1.ObjectMeta{Name: "rr-1", Namespace: "system", UID: types.UID("rr-1-uid")},
			Status: remediationv1.RemediationRequestStatus{
				OverallPhase: remediationv1.PhaseCancelling,
				PhaseProgress: &remediationv1.PhaseProgress{
					AIAnalysisRef: &corev1.ObjectReference{Name: "aa-rr-1", Namespace: "system"},
				},
			},
		}
		analysis := &aianalysisv1.AIAnalysis{
			ObjectMeta: metav1.ObjectMeta{Name: "aa-rr-1", Namespace: "system", UID: types.UID("aa-rr-1-uid")},
			Spec: aianalysisv1.AIAnalysisSpec{
				RemediationRequestRef: corev1.ObjectReference{Name: "rr-1", Namespace: "system", UID: parent.UID},
			},
			Status: aianalysisv1.AIAnalysisStatus{Phase: aianalysisv1.PhaseInvestigating},
		}
		session := &agentsessionv1.AgentSession{
			ObjectMeta: metav1.ObjectMeta{
				Name:       "as-rr-1",
				Namespace:  "system",
				Finalizers: []string{agentsessionv1.DispatchCleanupFinalizer},
			},
			Status: agentsessionv1.AgentSessionStatus{Phase: agentsessionv1.AgentSessionPhaseInvestigating},
		}
		Expect(controllerutil.SetControllerReference(analysis, session, scheme)).To(Succeed())
		c := fake.NewClientBuilder().WithScheme(scheme).
			WithStatusSubresource(parent, analysis, session).
			WithObjects(parent, analysis, session).Build()

		reconciler := &controller.AIAnalysisReconciler{
			Client:              c,
			AgentSessionCreator: creator.NewAgentSessionCreator(c, scheme),
		}
		_, err := reconciler.Reconcile(context.Background(), ctrl.Request{NamespacedName: types.NamespacedName{Name: analysis.Name, Namespace: analysis.Namespace}})
		Expect(err).NotTo(HaveOccurred())

		var gotAnalysis aianalysisv1.AIAnalysis
		Expect(c.Get(context.Background(), client.ObjectKeyFromObject(analysis), &gotAnalysis)).To(Succeed())
		Expect(gotAnalysis.Status.Phase).To(Equal(aianalysisv1.PhaseCancelling))
		Expect(cancellation.IsAcknowledged(gotAnalysis.Status.Conditions)).To(BeFalse())

		var gotSession agentsessionv1.AgentSession
		Expect(c.Get(context.Background(), client.ObjectKeyFromObject(session), &gotSession)).To(Succeed())
		Expect(gotSession.DeletionTimestamp.IsZero()).To(BeFalse())
		controllerutil.RemoveFinalizer(&gotSession, agentsessionv1.DispatchCleanupFinalizer)
		Expect(c.Update(context.Background(), &gotSession)).To(Succeed())

		_, err = reconciler.Reconcile(context.Background(), ctrl.Request{NamespacedName: types.NamespacedName{Name: analysis.Name, Namespace: analysis.Namespace}})
		Expect(err).NotTo(HaveOccurred())
		Expect(c.Get(context.Background(), client.ObjectKeyFromObject(analysis), &gotAnalysis)).To(Succeed())
		Expect(gotAnalysis.Status.Phase).To(Equal(aianalysisv1.PhaseCancelled))
		Expect(gotAnalysis.Status.Reason).To(Equal(aianalysisv1.ReasonParentCancelled))
		Expect(cancellation.IsAcknowledged(gotAnalysis.Status.Conditions)).To(BeTrue())
	})

	It("UT-AI-2588-005: bounds AgentSession cancellation retries and records manual-review evidence", func() {
		scheme := runtime.NewScheme()
		Expect(remediationv1.AddToScheme(scheme)).To(Succeed())
		Expect(aianalysisv1.AddToScheme(scheme)).To(Succeed())
		Expect(agentsessionv1.AddToScheme(scheme)).To(Succeed())

		parent := &remediationv1.RemediationRequest{
			ObjectMeta: metav1.ObjectMeta{Name: "rr-agent-session-budget", Namespace: "system", UID: types.UID("rr-agent-session-budget-uid")},
			Status:     remediationv1.RemediationRequestStatus{OverallPhase: remediationv1.PhaseCancelling},
		}
		analysis := &aianalysisv1.AIAnalysis{
			ObjectMeta: metav1.ObjectMeta{Name: "aa-agent-session-budget", Namespace: "system", UID: types.UID("aa-agent-session-budget-uid")},
			Spec: aianalysisv1.AIAnalysisSpec{
				RemediationRequestRef: corev1.ObjectReference{Name: parent.Name, Namespace: parent.Namespace, UID: parent.UID},
			},
			Status: aianalysisv1.AIAnalysisStatus{Phase: aianalysisv1.PhaseInvestigating},
		}
		session := &agentsessionv1.AgentSession{
			ObjectMeta: metav1.ObjectMeta{
				Name:       "as-" + parent.Name,
				Namespace:  parent.Namespace,
				Finalizers: []string{agentsessionv1.DispatchCleanupFinalizer},
			},
		}
		Expect(controllerutil.SetControllerReference(analysis, session, scheme)).To(Succeed())
		c := fake.NewClientBuilder().WithScheme(scheme).
			WithStatusSubresource(parent, analysis, session).
			WithObjects(parent, analysis, session).Build()
		reconciler := &controller.AIAnalysisReconciler{
			Client:              c,
			AgentSessionCreator: creator.NewAgentSessionCreator(c, scheme),
		}

		request := ctrl.Request{NamespacedName: types.NamespacedName{Name: analysis.Name, Namespace: analysis.Namespace}}
		for attempt := int32(1); attempt <= cancellation.MaxCancellationRetries; attempt++ {
			result, err := reconciler.Reconcile(context.Background(), request)
			Expect(err).NotTo(HaveOccurred())
			if attempt < cancellation.MaxCancellationRetries {
				Expect(result.RequeueAfter).To(BeNumerically(">", 0))
			} else {
				Expect(result).To(Equal(ctrl.Result{}))
			}
		}

		var gotAnalysis aianalysisv1.AIAnalysis
		Expect(c.Get(context.Background(), client.ObjectKeyFromObject(analysis), &gotAnalysis)).To(Succeed())
		Expect(gotAnalysis.Status.Phase).To(Equal(aianalysisv1.PhaseCancelling))
		Expect(gotAnalysis.Status.CancellationRetryCount).To(Equal(cancellation.MaxCancellationRetries))
		Expect(cancellation.IsFailed(gotAnalysis.Status.Conditions)).To(BeTrue())
		Expect(cancellation.IsAcknowledged(gotAnalysis.Status.Conditions)).To(BeFalse())
		Expect(gotAnalysis.Status.GetReview().NeedsHumanReview).To(BeTrue())
		Expect(gotAnalysis.Status.GetReview().HumanReviewReason).To(Equal("operator_escalation"))
		Expect(gotAnalysis.Status.SubReason).To(Equal(aianalysisv1.SubReasonMaxRetriesExceeded))
	})

	It("UT-AI-2588-007: schedules persisted AgentSession cancellation failures without returning an error", func() {
		scheme := runtime.NewScheme()
		Expect(remediationv1.AddToScheme(scheme)).To(Succeed())
		Expect(aianalysisv1.AddToScheme(scheme)).To(Succeed())
		Expect(agentsessionv1.AddToScheme(scheme)).To(Succeed())

		parent := &remediationv1.RemediationRequest{
			ObjectMeta: metav1.ObjectMeta{Name: "rr-agent-session-error", Namespace: "system", UID: types.UID("rr-agent-session-error-uid")},
			Status:     remediationv1.RemediationRequestStatus{OverallPhase: remediationv1.PhaseCancelling},
		}
		analysis := &aianalysisv1.AIAnalysis{
			ObjectMeta: metav1.ObjectMeta{Name: "aa-agent-session-error", Namespace: "system", UID: types.UID("aa-agent-session-error-uid")},
			Spec: aianalysisv1.AIAnalysisSpec{
				RemediationRequestRef: corev1.ObjectReference{Name: parent.Name, Namespace: parent.Namespace, UID: parent.UID},
			},
			Status: aianalysisv1.AIAnalysisStatus{Phase: aianalysisv1.PhaseInvestigating},
		}
		Expect(controllerutil.SetControllerReference(parent, analysis, scheme)).To(Succeed())
		c := fake.NewClientBuilder().WithScheme(scheme).
			WithStatusSubresource(parent, analysis).WithObjects(parent, analysis).
			WithInterceptorFuncs(interceptor.Funcs{
				Get: func(ctx context.Context, c client.WithWatch, key client.ObjectKey, obj client.Object, opts ...client.GetOption) error {
					if _, ok := obj.(*agentsessionv1.AgentSession); ok {
						return errors.New("agentsession lookup failed")
					}
					return c.Get(ctx, key, obj, opts...)
				},
			}).Build()
		reconciler := &controller.AIAnalysisReconciler{
			Client:              c,
			AgentSessionCreator: creator.NewAgentSessionCreator(c, scheme),
		}

		result, err := reconciler.Reconcile(context.Background(), ctrl.Request{NamespacedName: types.NamespacedName{Name: analysis.Name, Namespace: analysis.Namespace}})
		Expect(err).NotTo(HaveOccurred())
		Expect(result.RequeueAfter).To(BeNumerically(">", 0))

		var got aianalysisv1.AIAnalysis
		Expect(c.Get(context.Background(), client.ObjectKeyFromObject(analysis), &got)).To(Succeed())
		Expect(got.Status.CancellationRetryCount).To(Equal(int32(1)))
		Expect(cancellation.IsFailed(got.Status.Conditions)).To(BeTrue())
	})

	It("UT-AI-2588-003: refuses cancellation for a same-name parent with a stale UID", func() {
		scheme := runtime.NewScheme()
		Expect(remediationv1.AddToScheme(scheme)).To(Succeed())
		Expect(aianalysisv1.AddToScheme(scheme)).To(Succeed())

		parent := &remediationv1.RemediationRequest{
			ObjectMeta: metav1.ObjectMeta{Name: "rr-current", Namespace: "system", UID: types.UID("rr-current-uid")},
			Status:     remediationv1.RemediationRequestStatus{OverallPhase: remediationv1.PhaseCancelling},
		}
		analysis := &aianalysisv1.AIAnalysis{
			ObjectMeta: metav1.ObjectMeta{Name: "aa-rr-current", Namespace: "system"},
			Spec: aianalysisv1.AIAnalysisSpec{
				RemediationRequestRef: corev1.ObjectReference{
					Name: "rr-current", Namespace: "system", UID: types.UID("rr-old-uid"),
				},
			},
			Status: aianalysisv1.AIAnalysisStatus{Phase: aianalysisv1.PhaseInvestigating},
		}
		c := fake.NewClientBuilder().WithScheme(scheme).
			WithStatusSubresource(parent, analysis).
			WithObjects(parent, analysis).Build()
		reconciler := &controller.AIAnalysisReconciler{Client: c}
		var fetchedParent remediationv1.RemediationRequest
		Expect(c.Get(context.Background(), client.ObjectKeyFromObject(parent), &fetchedParent)).To(Succeed())
		Expect(cancellation.ParentStopRequested(&fetchedParent)).To(BeTrue())
		var fetchedAnalysis aianalysisv1.AIAnalysis
		Expect(c.Get(context.Background(), client.ObjectKeyFromObject(analysis), &fetchedAnalysis)).To(Succeed())
		Expect(fetchedAnalysis.Spec.RemediationRequestRef.UID).To(Equal(types.UID("rr-old-uid")))
		Expect(cancellation.MatchesParentIdentity(
			cancellation.ParentIdentity{Name: fetchedParent.Name, Namespace: fetchedParent.Namespace, UID: fetchedParent.UID},
			cancellation.ReferenceIdentity{Name: analysis.Spec.RemediationRequestRef.Name, Namespace: analysis.Namespace, UID: analysis.Spec.RemediationRequestRef.UID},
			nil,
			"RemediationRequest",
		)).To(BeFalse())

		_, err := reconciler.Reconcile(context.Background(), ctrl.Request{NamespacedName: types.NamespacedName{Name: analysis.Name, Namespace: analysis.Namespace}})
		Expect(err).To(MatchError(ContainSubstring("identity mismatch")))

		var got aianalysisv1.AIAnalysis
		Expect(c.Get(context.Background(), client.ObjectKeyFromObject(analysis), &got)).To(Succeed())
		Expect(got.Status.Phase).To(Equal(aianalysisv1.PhaseInvestigating))
		Expect(cancellation.IsAcknowledged(got.Status.Conditions)).To(BeFalse())
	})

	It("UT-AI-2588-004: removes the AIAnalysis finalizer after acknowledged cancellation deletion", func() {
		scheme := runtime.NewScheme()
		Expect(remediationv1.AddToScheme(scheme)).To(Succeed())
		Expect(aianalysisv1.AddToScheme(scheme)).To(Succeed())

		parent := &remediationv1.RemediationRequest{
			ObjectMeta: metav1.ObjectMeta{Name: "rr-terminal-delete", Namespace: "system", UID: types.UID("rr-terminal-delete-uid")},
			Status:     remediationv1.RemediationRequestStatus{OverallPhase: remediationv1.PhaseCancelling},
		}
		analysis := &aianalysisv1.AIAnalysis{
			ObjectMeta: metav1.ObjectMeta{
				Name:       "aa-terminal-delete",
				Namespace:  "system",
				Finalizers: []string{controller.FinalizerName},
			},
			Spec: aianalysisv1.AIAnalysisSpec{
				RemediationRequestRef: corev1.ObjectReference{Name: parent.Name, Namespace: parent.Namespace, UID: parent.UID},
			},
			Status: aianalysisv1.AIAnalysisStatus{
				Phase: aianalysisv1.PhaseCancelled,
				Conditions: func() []metav1.Condition {
					conditions := []metav1.Condition{}
					cancellation.SetAcknowledged(&conditions, 1, "parent cancellation acknowledged")
					return conditions
				}(),
			},
		}
		c := fake.NewClientBuilder().WithScheme(scheme).
			WithStatusSubresource(parent, analysis).
			WithObjects(parent, analysis).Build()
		reconciler := &controller.AIAnalysisReconciler{Client: c}

		Expect(c.Delete(context.Background(), analysis)).To(Succeed())
		_, err := reconciler.Reconcile(context.Background(), ctrl.Request{NamespacedName: types.NamespacedName{Name: analysis.Name, Namespace: analysis.Namespace}})
		Expect(err).NotTo(HaveOccurred())

		var got aianalysisv1.AIAnalysis
		err = c.Get(context.Background(), client.ObjectKeyFromObject(analysis), &got)
		Expect(apierrors.IsNotFound(err)).To(BeTrue())
	})
})
