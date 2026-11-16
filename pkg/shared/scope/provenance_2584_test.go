package scope_test

import (
	"context"
	"fmt"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
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

	It("UT-SCOPE-2592-LABEL-001 [BR-SCOPE-001]: treats an invalid resource label as unavailable and never inherits namespace scope", func() {
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

		resolved, err := scope.NewManager(k8sClient).Resolve(context.Background(), scope.ResourceIdentity{Group: "apps", Version: "v1",
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

	It("UT-SCOPE-2592-TYPED-001 [BR-SCOPE-001]: returns verified unmanaged with a boolean decision", func() {
		scheme := runtime.NewScheme()
		Expect(corev1.AddToScheme(scheme)).To(Succeed())
		Expect(appsv1.AddToScheme(scheme)).To(Succeed())
		k8sClient := fake.NewClientBuilder().WithScheme(scheme).WithObjects(
			&corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: "payments"}},
			&corev1.ConfigMap{ObjectMeta: metav1.ObjectMeta{
				Name:      "checkout",
				Namespace: "payments",
				Labels:    map[string]string{scope.ManagedLabelKey: scope.ManagedLabelValueFalse},
			}},
		).Build()

		result, err := scope.NewManager(k8sClient).Check(context.Background(), scope.ResourceIdentity{
			Group: "", Version: "v1", Kind: "ConfigMap", Namespace: "payments", Name: "checkout",
		})

		Expect(err).NotTo(HaveOccurred())
		Expect(result.State).To(Equal(scope.ScopeVerifiedUnmanaged))
		Expect(result.Managed).NotTo(BeNil())
		Expect(*result.Managed).To(BeFalse())
		Expect(result.Provenance).To(Equal(scope.ScopeSourceResourceLabel))
	})

	It("UT-SCOPE-2592-TYPED-002 [BR-SCOPE-001, SI-10]: returns invalid without an authorization boolean for an invalid label", func() {
		scheme := runtime.NewScheme()
		Expect(corev1.AddToScheme(scheme)).To(Succeed())
		k8sClient := fake.NewClientBuilder().WithScheme(scheme).WithObjects(
			&corev1.Namespace{ObjectMeta: metav1.ObjectMeta{
				Name:   "payments",
				Labels: map[string]string{scope.ManagedLabelKey: "enabled"},
			}},
		).Build()

		result, err := scope.NewManager(k8sClient).Check(context.Background(), scope.ResourceIdentity{
			Version: "v1", Kind: "ConfigMap", Namespace: "payments", Name: "checkout",
		})

		Expect(err).NotTo(HaveOccurred())
		Expect(result.State).To(Equal(scope.ScopeInvalid))
		Expect(result.Managed).To(BeNil())
		Expect(result.Provenance).To(Equal(scope.ScopeSourceUnavailable))
		Expect(result.Reason).To(Equal("invalid_namespace_label"))
	})

	It("UT-SCOPE-2592-TYPED-003 [BR-SCOPE-001, SI-10]: returns unavailable when the local metadata lookup fails", func() {
		scheme := runtime.NewScheme()
		Expect(corev1.AddToScheme(scheme)).To(Succeed())

		result, err := scope.NewManager(failingScopeReader{}).Check(
			context.Background(), scope.ResourceIdentity{
				Group: "apps", Version: "v1", Kind: "Deployment", Namespace: "payments", Name: "checkout",
			},
		)

		Expect(err).To(HaveOccurred())
		Expect(result.State).To(Equal(scope.ScopeUnavailable))
		Expect(result.Managed).To(BeNil())
		Expect(result.Retryable).To(BeTrue())
	})
})

type failingScopeReader struct{}

func (failingScopeReader) Get(context.Context, client.ObjectKey, client.Object, ...client.GetOption) error {
	return fmt.Errorf("scope reader unavailable")
}

func (failingScopeReader) List(context.Context, client.ObjectList, ...client.ListOption) error {
	return fmt.Errorf("scope reader unavailable")
}
