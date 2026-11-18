package scope_test

import (
	"context"
	"encoding/json"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	"github.com/jordigilh/kubernaut/pkg/shared/scope"
)

var _ = Describe("Enum-only scope [BR-SCOPE-001]", func() {
	DescribeTable("S01-S04/S08 local observed label precedence and provenance",
		func(resourceLabel, namespaceLabel string, state scope.ScopeState, source scope.ScopeSource) {
			scheme := runtime.NewScheme()
			Expect(corev1.AddToScheme(scheme)).To(Succeed())
			labels := func(value string) map[string]string {
				if value == "" {
					return nil
				}
				return map[string]string{scope.ManagedLabelKey: value}
			}
			reader := fake.NewClientBuilder().WithScheme(scheme).WithObjects(
				&corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: "payments", Labels: labels(namespaceLabel)}},
				&corev1.Pod{ObjectMeta: metav1.ObjectMeta{Name: "api", Namespace: "payments", Labels: labels(resourceLabel)}},
			).Build()
			result, err := scope.NewManager(reader).Check(context.Background(), scope.ResourceIdentity{Version: "v1", Kind: "Pod", Namespace: "payments", Name: "api"})
			Expect(err).NotTo(HaveOccurred())
			Expect(result.Validate()).To(Succeed())
			Expect(result.State).To(Equal(state))
			Expect(result.Provenance).To(Equal(source))
		},
		Entry("S01 true", "true", "false", scope.ScopeVerifiedManaged, scope.ScopeSourceResourceLabel),
		Entry("S02 false", "false", "true", scope.ScopeVerifiedUnmanaged, scope.ScopeSourceResourceLabel),
		Entry("S03 namespace true", "", "true", scope.ScopeVerifiedManaged, scope.ScopeSourceNamespaceLabel),
		Entry("S03 namespace false", "", "false", scope.ScopeVerifiedUnmanaged, scope.ScopeSourceNamespaceLabel),
		Entry("S04 observed absent opt-in", "", "", scope.ScopeVerifiedUnmanaged, scope.ScopeSourceNone),
		Entry("S08 invalid resource", "invalid", "true", scope.ScopeInvalid, scope.ScopeSourceUnavailable),
		Entry("S08 invalid namespace", "", "invalid", scope.ScopeInvalid, scope.ScopeSourceUnavailable),
	)

	It("S07 cancellation remains an execution error and never authorizes", func() {
		ctx, cancel := context.WithCancel(context.Background())
		cancel()
		result, err := scope.NewManager(fake.NewClientBuilder().Build()).Check(ctx, scope.ResourceIdentity{Kind: "Pod", Name: "api", Namespace: "payments"})
		Expect(err).To(MatchError(context.Canceled))
		Expect(result.State).To(Equal(scope.ScopeUnavailable))
	})

	It("S05: a missing target cannot inherit a managed Namespace", func() {
		scheme := runtime.NewScheme()
		Expect(corev1.AddToScheme(scheme)).To(Succeed())
		reader := fake.NewClientBuilder().WithScheme(scheme).WithObjects(&corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: "payments", Labels: map[string]string{scope.ManagedLabelKey: "true"}}}).Build()
		result, err := scope.NewManager(reader).Check(context.Background(), scope.ResourceIdentity{Version: "v1", Kind: "Pod", Namespace: "payments", Name: "missing"})
		Expect(err).NotTo(HaveOccurred())
		Expect(result.State).To(Equal(scope.ScopeUnavailable))
		Expect(result.Provenance).To(Equal(scope.ScopeSourceUnavailable))
	})

	It("S06: present unlabeled target needs observed Namespace evidence", func() {
		scheme := runtime.NewScheme()
		Expect(corev1.AddToScheme(scheme)).To(Succeed())
		reader := fake.NewClientBuilder().WithScheme(scheme).WithObjects(&corev1.Pod{ObjectMeta: metav1.ObjectMeta{Name: "present", Namespace: "payments"}}).Build()
		result, err := scope.NewManager(reader).Check(context.Background(), scope.ResourceIdentity{Version: "v1", Kind: "Pod", Namespace: "payments", Name: "present"})
		Expect(err).NotTo(HaveOccurred())
		Expect(result.State).To(Equal(scope.ScopeUnavailable))
	})

	It("S13: serializes only state and provenance, never internal diagnostics or a managed boolean", func() {
		encoded, err := json.Marshal(scope.ScopeResult{State: scope.ScopeUnavailable, Provenance: scope.ScopeSourceUnavailable, Reason: "internal_diagnostic", Retryable: true})
		Expect(err).NotTo(HaveOccurred())
		Expect(string(encoded)).To(MatchJSON(`{"state":"unavailable","provenance":"unavailable"}`))
	})
})
