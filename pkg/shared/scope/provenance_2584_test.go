package scope_test

import (
	"context"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	"github.com/jordigilh/kubernaut/pkg/shared/scope"
)

var _ = Describe("BR-SCOPE-001 / UT-AF-2584-003: structured scope provenance", func() {
	It("preserves resource-label precedence over namespace inheritance", func() {
		resolved := scope.ResolveLabels(
			map[string]string{scope.ManagedLabelKey: scope.ManagedLabelValueFalse},
			map[string]string{scope.ManagedLabelKey: scope.ManagedLabelValueTrue},
			false,
		)

		Expect(resolved.Included).To(BeFalse())
		Expect(resolved.Source).To(Equal(scope.ScopeSourceResourceLabel))
		Expect(resolved.ResourceLabel).To(Equal(scope.LabelStateFalse))
		Expect(resolved.NamespaceLabel).To(Equal(scope.LabelStateTrue))
	})

	It("UT-FMC-PRES-001 [BR-SCOPE-001]: treats an invalid resource label as unavailable and never inherits namespace scope", func() {
		resolved := scope.ResolveLabels(
			map[string]string{scope.ManagedLabelKey: "enabled"},
			map[string]string{scope.ManagedLabelKey: scope.ManagedLabelValueTrue},
			false,
		)

		Expect(resolved.Included).To(BeFalse())
		Expect(resolved.Source).To(Equal(scope.ScopeSourceUnavailable))
		Expect(resolved.ResourceLabel).To(Equal(scope.LabelStateInvalid))
		Expect(resolved.NamespaceLabel).To(Equal(scope.LabelStateTrue))
	})

	It("does not inherit namespace scope for cluster-scoped resources", func() {
		resolved := scope.ResolveLabels(
			nil,
			map[string]string{scope.ManagedLabelKey: scope.ManagedLabelValueTrue},
			true,
		)

		Expect(resolved.Included).To(BeFalse())
		Expect(resolved.Source).To(Equal(scope.ScopeSourceNone))
		Expect(resolved.ClusterScoped).To(BeTrue())
		Expect(resolved.NamespaceLabel).To(Equal(scope.LabelStateTrue))
	})

	It("preserves absent and invalid states through the Kubernetes-backed resolver", func() {
		scheme := runtime.NewScheme()
		Expect(corev1.AddToScheme(scheme)).To(Succeed())
		Expect(appsv1.AddToScheme(scheme)).To(Succeed())
		k8sClient := fake.NewClientBuilder().WithScheme(scheme).WithObjects(
			&corev1.Namespace{ObjectMeta: metav1.ObjectMeta{
				Name:   "payments",
				Labels: map[string]string{scope.ManagedLabelKey: scope.ManagedLabelValueTrue},
			}},
			&appsv1.Deployment{ObjectMeta: metav1.ObjectMeta{
				Name:      "checkout",
				Namespace: "payments",
				Labels:    map[string]string{scope.ManagedLabelKey: "enabled"},
			}},
		).Build()

		resolved, err := scope.NewManager(k8sClient).Resolve(context.Background(), scope.ResourceIdentity{
			Namespace: "payments",
			Kind:      "Deployment",
			Name:      "checkout",
		})
		Expect(err).NotTo(HaveOccurred())
		Expect(resolved.Included).To(BeFalse())
		Expect(resolved.ResourceLabel).To(Equal(scope.LabelStateInvalid))
		Expect(resolved.NamespaceLabel).To(Equal(scope.LabelStateTrue))
		Expect(resolved.Source).To(Equal(scope.ScopeSourceUnavailable))
	})
})
