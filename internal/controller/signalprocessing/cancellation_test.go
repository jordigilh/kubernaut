package signalprocessing

import (
	"context"
	"testing"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"

	aianalysisv1 "github.com/jordigilh/kubernaut/api/aianalysis/v1alpha1"
	remediationv1 "github.com/jordigilh/kubernaut/api/remediation/v1alpha1"
	signalprocessingv1 "github.com/jordigilh/kubernaut/api/signalprocessing/v1alpha1"
	"github.com/jordigilh/kubernaut/pkg/shared/cancellation"
)

func TestSignalProcessingCancellation(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "SignalProcessing Cancellation Suite")
}

var _ = Describe("SignalProcessing parent cancellation", func() {
	It("IT-SP-2588-001: acknowledges parent cancellation without running processing", func() {
		scheme := runtime.NewScheme()
		Expect(remediationv1.AddToScheme(scheme)).To(Succeed())
		Expect(signalprocessingv1.AddToScheme(scheme)).To(Succeed())
		Expect(aianalysisv1.AddToScheme(scheme)).To(Succeed())

		parent := &remediationv1.RemediationRequest{
			ObjectMeta: metav1.ObjectMeta{Name: "rr-1", Namespace: "system", UID: types.UID("rr-1-uid")},
			Status: remediationv1.RemediationRequestStatus{
				OverallPhase: remediationv1.PhaseCancelling,
				PhaseProgress: &remediationv1.PhaseProgress{
					SignalProcessingRef: &corev1.ObjectReference{Name: "sp-rr-1", Namespace: "system"},
				},
			},
		}
		sp := &signalprocessingv1.SignalProcessing{
			ObjectMeta: metav1.ObjectMeta{Name: "sp-rr-1", Namespace: "system"},
			Spec: signalprocessingv1.SignalProcessingSpec{
				RemediationRequestRef: signalprocessingv1.ObjectReference{Name: "rr-1", Namespace: "system", UID: string(parent.UID)},
			},
			Status: signalprocessingv1.SignalProcessingStatus{Phase: signalprocessingv1.PhaseEnriching},
		}
		Expect(controllerutil.SetControllerReference(parent, sp, scheme)).To(Succeed())
		c := fake.NewClientBuilder().WithScheme(scheme).
			WithStatusSubresource(parent, sp).
			WithObjects(parent, sp).Build()

		reconciler := &SignalProcessingReconciler{Client: c}
		_, err := reconciler.Reconcile(context.Background(), ctrl.Request{NamespacedName: types.NamespacedName{Name: sp.Name, Namespace: sp.Namespace}})
		Expect(err).NotTo(HaveOccurred())

		var got signalprocessingv1.SignalProcessing
		Expect(c.Get(context.Background(), client.ObjectKeyFromObject(sp), &got)).To(Succeed())
		Expect(got.Status.Phase).To(Equal(signalprocessingv1.PhaseCancelled))
		Expect(cancellation.IsAcknowledged(got.Status.Conditions)).To(BeTrue())
	})

	It("IT-SP-2588-002: treats a missing parent as a stop request", func() {
		scheme := runtime.NewScheme()
		Expect(remediationv1.AddToScheme(scheme)).To(Succeed())
		Expect(signalprocessingv1.AddToScheme(scheme)).To(Succeed())
		sp := &signalprocessingv1.SignalProcessing{
			ObjectMeta: metav1.ObjectMeta{Name: "sp-orphan", Namespace: "system"},
			Spec: signalprocessingv1.SignalProcessingSpec{
				RemediationRequestRef: signalprocessingv1.ObjectReference{Name: "missing", Namespace: "system"},
			},
			Status: signalprocessingv1.SignalProcessingStatus{Phase: signalprocessingv1.PhasePending},
		}
		controller := true
		sp.OwnerReferences = []metav1.OwnerReference{{
			APIVersion: remediationv1.GroupVersion.String(),
			Kind:       "RemediationRequest",
			Name:       "missing",
			UID:        types.UID("missing-uid"),
			Controller: &controller,
		}}
		c := fake.NewClientBuilder().WithScheme(scheme).
			WithStatusSubresource(sp).
			WithObjects(sp).Build()

		reconciler := &SignalProcessingReconciler{Client: c}
		_, err := reconciler.Reconcile(context.Background(), ctrl.Request{NamespacedName: types.NamespacedName{Name: sp.Name, Namespace: sp.Namespace}})
		Expect(err).NotTo(HaveOccurred())

		var got signalprocessingv1.SignalProcessing
		Expect(c.Get(context.Background(), client.ObjectKeyFromObject(sp), &got)).To(Succeed())
		Expect(got.Status.Phase).To(Equal(signalprocessingv1.PhaseCancelled))
		Expect(cancellation.IsAcknowledged(got.Status.Conditions)).To(BeTrue())
	})

	It("UT-SP-2588-003: refuses cancellation for a same-name parent with a stale UID", func() {
		scheme := runtime.NewScheme()
		Expect(remediationv1.AddToScheme(scheme)).To(Succeed())
		Expect(signalprocessingv1.AddToScheme(scheme)).To(Succeed())

		parent := &remediationv1.RemediationRequest{
			ObjectMeta: metav1.ObjectMeta{Name: "rr-current", Namespace: "system", UID: types.UID("rr-current-uid")},
			Status:     remediationv1.RemediationRequestStatus{OverallPhase: remediationv1.PhaseCancelling},
		}
		sp := &signalprocessingv1.SignalProcessing{
			ObjectMeta: metav1.ObjectMeta{Name: "sp-rr-current", Namespace: "system"},
			Spec: signalprocessingv1.SignalProcessingSpec{
				RemediationRequestRef: signalprocessingv1.ObjectReference{
					Name: "rr-current", Namespace: "system", UID: "rr-old-uid",
				},
			},
			Status: signalprocessingv1.SignalProcessingStatus{Phase: signalprocessingv1.PhaseEnriching},
		}
		Expect(controllerutil.SetControllerReference(parent, sp, scheme)).To(Succeed())
		c := fake.NewClientBuilder().WithScheme(scheme).
			WithStatusSubresource(parent, sp).
			WithObjects(parent, sp).Build()

		reconciler := &SignalProcessingReconciler{Client: c}
		_, err := reconciler.Reconcile(context.Background(), ctrl.Request{NamespacedName: types.NamespacedName{Name: sp.Name, Namespace: sp.Namespace}})
		Expect(err).To(MatchError(ContainSubstring("identity mismatch")))

		var got signalprocessingv1.SignalProcessing
		Expect(c.Get(context.Background(), client.ObjectKeyFromObject(sp), &got)).To(Succeed())
		Expect(got.Status.Phase).To(Equal(signalprocessingv1.PhaseEnriching))
		Expect(cancellation.IsAcknowledged(got.Status.Conditions)).To(BeFalse())
	})
})
