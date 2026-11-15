package scope_test

import (
	"context"
	"fmt"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/controller-runtime/pkg/client/interceptor"

	"github.com/jordigilh/kubernaut/pkg/shared/scope"
)

var _ = Describe("BR-SCOPE-001: metadata-backed scope precedence", func() {
	It("UT-AF-2584-SCOPE-EXPLICIT-001: resolves explicit resource labels without reading Namespace metadata", func() {
		scheme := runtime.NewScheme()
		Expect(corev1.AddToScheme(scheme)).To(Succeed())
		Expect(appsv1.AddToScheme(scheme)).To(Succeed())

		var namespaceGets int
		k8sClient := fake.NewClientBuilder().
			WithScheme(scheme).
			WithInterceptorFuncs(interceptor.Funcs{
				Get: func(ctx context.Context, c client.WithWatch, key client.ObjectKey, obj client.Object, opts ...client.GetOption) error {
					if key.Namespace == "" && key.Name == "payments" {
						namespaceGets++
						return fmt.Errorf("Namespace metadata unavailable")
					}
					return c.Get(ctx, key, obj, opts...)
				},
			}).
			Build()
		mgr := scope.NewManager(k8sClient)

		resolved, err := mgr.ResolveWithResourceLabels(context.Background(), scope.ResourceIdentity{
			Namespace: "payments",
			Kind:      "Deployment",
			Name:      "checkout",
		}, map[string]string{scope.ManagedLabelKey: scope.ManagedLabelValueTrue})

		Expect(err).NotTo(HaveOccurred())
		Expect(resolved.Included).To(BeTrue())
		Expect(resolved.Source).To(Equal(scope.ScopeSourceResourceLabel))
		Expect(namespaceGets).To(BeZero())
	})

	It("UT-AF-2584-SCOPE-EXPLICIT-002: preserves explicit resource opt-out without Namespace metadata", func() {
		scheme := runtime.NewScheme()
		Expect(corev1.AddToScheme(scheme)).To(Succeed())
		Expect(appsv1.AddToScheme(scheme)).To(Succeed())
		k8sClient := fake.NewClientBuilder().WithScheme(scheme).Build()
		mgr := scope.NewManager(k8sClient)

		resolved, err := mgr.ResolveWithResourceLabels(context.Background(), scope.ResourceIdentity{
			Namespace: "payments",
			Kind:      "Deployment",
			Name:      "checkout",
		}, map[string]string{scope.ManagedLabelKey: scope.ManagedLabelValueFalse})

		Expect(err).NotTo(HaveOccurred())
		Expect(resolved.Included).To(BeFalse())
		Expect(resolved.Source).To(Equal(scope.ScopeSourceResourceLabel))
	})

	It("UT-AF-2584-SCOPE-FAIL-CLOSED-001: returns an error when resource metadata is inaccessible", func() {
		scheme := runtime.NewScheme()
		Expect(corev1.AddToScheme(scheme)).To(Succeed())
		Expect(appsv1.AddToScheme(scheme)).To(Succeed())
		k8sClient := fake.NewClientBuilder().
			WithScheme(scheme).
			WithInterceptorFuncs(interceptor.Funcs{
				Get: func(_ context.Context, _ client.WithWatch, key client.ObjectKey, _ client.Object, _ ...client.GetOption) error {
					return fmt.Errorf("resource metadata unavailable for %s", key.Name)
				},
			}).
			Build()

		_, err := scope.NewManager(k8sClient).Resolve(context.Background(), scope.ResourceIdentity{
			Namespace: "payments",
			Kind:      "Deployment",
			Name:      "checkout",
		})

		Expect(err).To(HaveOccurred())
	})
})
