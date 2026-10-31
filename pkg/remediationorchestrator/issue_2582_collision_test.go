package remediationorchestrator_test

import (
	"context"
	"errors"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	remediationv1 "github.com/jordigilh/kubernaut/api/remediation/v1alpha1"
	signalprocessingv1 "github.com/jordigilh/kubernaut/api/signalprocessing/v1alpha1"
	"github.com/jordigilh/kubernaut/pkg/remediationorchestrator/creator"
)

var _ = Describe("RO CreateOnly child provenance", func() {
	It("UT-RO-2582-001: refuses to adopt a pre-existing SignalProcessing [BR-ORCH-025, AC-4]", func() {
		ctx := context.Background()
		scheme := runtime.NewScheme()
		Expect(remediationv1.AddToScheme(scheme)).To(Succeed())
		Expect(signalprocessingv1.AddToScheme(scheme)).To(Succeed())
		rr := &remediationv1.RemediationRequest{
			ObjectMeta: metav1.ObjectMeta{Name: "rr-collision", Namespace: "default", UID: "rr-uid"},
		}
		existing := &signalprocessingv1.SignalProcessing{
			ObjectMeta: metav1.ObjectMeta{Name: "sp-rr-collision", Namespace: "default"},
		}
		k8sClient := fake.NewClientBuilder().WithScheme(scheme).WithObjects(existing).Build()

		name, err := creator.NewSignalProcessingCreator(k8sClient, scheme, nil).Create(ctx, rr)

		Expect(name).To(BeEmpty())
		Expect(err).To(HaveOccurred())
		var collision *creator.ChildCollisionError
		Expect(errors.As(err, &collision)).To(BeTrue())
		Expect(collision.ChildKind).To(Equal("SignalProcessing"))
	})
})
