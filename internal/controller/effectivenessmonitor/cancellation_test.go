package controller

import (
	"context"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	eav1 "github.com/jordigilh/kubernaut/api/effectivenessassessment/v1alpha1"
	remediationv1 "github.com/jordigilh/kubernaut/api/remediation/v1alpha1"
	"github.com/jordigilh/kubernaut/pkg/shared/cancellation"
)

var _ = Describe("EffectivenessAssessment parent cancellation", func() {
	It("UT-EM-2588-001: stops assessment before acknowledging cancellation", func() {
		scheme := runtime.NewScheme()
		Expect(eav1.AddToScheme(scheme)).To(Succeed())
		Expect(remediationv1.AddToScheme(scheme)).To(Succeed())

		parent := &remediationv1.RemediationRequest{
			ObjectMeta: metav1.ObjectMeta{Name: "rr-ea-cancel", Namespace: "system", UID: types.UID("rr-ea-cancel-uid")},
			Status:     remediationv1.RemediationRequestStatus{OverallPhase: remediationv1.PhaseCancelling},
		}
		controller := true
		ea := &eav1.EffectivenessAssessment{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "ea-rr-ea-cancel",
				Namespace: "system",
				OwnerReferences: []metav1.OwnerReference{{
					Kind:       "RemediationRequest",
					Name:       parent.Name,
					UID:        parent.UID,
					Controller: &controller,
				}},
			},
			Spec:   eav1.EffectivenessAssessmentSpec{CorrelationID: parent.Name},
			Status: eav1.EffectivenessAssessmentStatus{Phase: eav1.PhasePending},
		}
		c := fake.NewClientBuilder().WithScheme(scheme).
			WithStatusSubresource(ea, parent).
			WithObjects(parent, ea).Build()
		reconciler := NewReconciler(ReconcilerDeps{Client: c, APIReader: c, Scheme: scheme}, DefaultReconcilerConfig())

		_, err := reconciler.Reconcile(context.Background(), ctrl.Request{NamespacedName: types.NamespacedName{Name: ea.Name, Namespace: ea.Namespace}})
		Expect(err).NotTo(HaveOccurred())
		var got eav1.EffectivenessAssessment
		Expect(c.Get(context.Background(), client.ObjectKeyFromObject(ea), &got)).To(Succeed())
		Expect(got.Status.Phase).To(Equal(eav1.PhaseCancelling))
		Expect(cancellation.IsAcknowledged(got.Status.Conditions)).To(BeFalse())

		_, err = reconciler.Reconcile(context.Background(), ctrl.Request{NamespacedName: types.NamespacedName{Name: ea.Name, Namespace: ea.Namespace}})
		Expect(err).NotTo(HaveOccurred())
		Expect(c.Get(context.Background(), client.ObjectKeyFromObject(ea), &got)).To(Succeed())
		Expect(got.Status.Phase).To(Equal(eav1.PhaseCancelled))
		Expect(cancellation.IsAcknowledged(got.Status.Conditions)).To(BeTrue())
	})

	It("UT-EM-2588-002: refuses cancellation for a same-name parent with a stale owner UID", func() {
		scheme := runtime.NewScheme()
		Expect(eav1.AddToScheme(scheme)).To(Succeed())
		Expect(remediationv1.AddToScheme(scheme)).To(Succeed())

		parent := &remediationv1.RemediationRequest{
			ObjectMeta: metav1.ObjectMeta{Name: "rr-ea-current", Namespace: "system", UID: types.UID("rr-current-uid")},
			Status:     remediationv1.RemediationRequestStatus{OverallPhase: remediationv1.PhaseCancelling},
		}
		controller := true
		ea := &eav1.EffectivenessAssessment{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "ea-rr-ea-current",
				Namespace: "system",
				OwnerReferences: []metav1.OwnerReference{{
					Kind:       "RemediationRequest",
					Name:       parent.Name,
					UID:        types.UID("rr-old-uid"),
					Controller: &controller,
				}},
			},
			Spec:   eav1.EffectivenessAssessmentSpec{CorrelationID: parent.Name},
			Status: eav1.EffectivenessAssessmentStatus{Phase: eav1.PhasePending},
		}
		c := fake.NewClientBuilder().WithScheme(scheme).
			WithStatusSubresource(ea, parent).
			WithObjects(parent, ea).Build()
		reconciler := NewReconciler(ReconcilerDeps{Client: c, APIReader: c, Scheme: scheme}, DefaultReconcilerConfig())

		result, err := reconciler.Reconcile(context.Background(), ctrl.Request{NamespacedName: types.NamespacedName{Name: ea.Name, Namespace: ea.Namespace}})
		Expect(err).To(MatchError(ContainSubstring("identity mismatch")))
		Expect(result).To(Equal(ctrl.Result{}))
		var got eav1.EffectivenessAssessment
		Expect(c.Get(context.Background(), client.ObjectKeyFromObject(ea), &got)).To(Succeed())
		Expect(got.Status.Phase).To(Equal(eav1.PhasePending))
		Expect(cancellation.IsAcknowledged(got.Status.Conditions)).To(BeFalse())
	})
})
