/*
Copyright 2025 Jordi Gil.

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

package scope_test

import (
	"context"
	"fmt"
	"testing"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/controller-runtime/pkg/client/interceptor"

	"github.com/jordigilh/kubernaut/pkg/shared/scope"
)

func TestScope(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Shared Scope Manager Suite")
}

// ========================================
// SHARED SCOPE MANAGER UNIT TESTS
// ========================================
//
// Business Requirements:
//   - BR-SCOPE-001: Resource Scope Management (2-level hierarchy)
//   - ADR-053: Resource Scope Management Architecture
//
// Test Plan: docs/testing/BR-SCOPE-001/TEST_PLAN.md
//
// Test IDs: UT-SCOPE-001-001 through UT-SCOPE-001-015
//
// Hierarchy:
//   1. Resource label → "true" = managed, "false" = unmanaged
//   2. Namespace label → "true" = managed, "false" = unmanaged
//   3. Default → unmanaged (safe default)
//
// Cluster-scoped resources (Node, PV): resource label only, no namespace fallback.

var _ = Describe("Scope Manager", func() {
	var (
		ctx       context.Context
		k8sClient client.Client
		mgr       *scope.Manager
		scheme    *runtime.Scheme
	)

	BeforeEach(func() {
		ctx = context.Background()
		scheme = runtime.NewScheme()
		Expect(corev1.AddToScheme(scheme)).To(Succeed())
		Expect(appsv1.AddToScheme(scheme)).To(Succeed())
	})

	// helper: build a fake client with the given objects and create a scope.Manager
	setup := func(objs ...client.Object) {
		k8sClient = fake.NewClientBuilder().
			WithScheme(scheme).
			WithObjects(objs...).
			Build()
		mgr = scope.NewManager(k8sClient)
	}

	// helper: build a fake client with interceptor for error injection
	setupWithInterceptor := func(funcs interceptor.Funcs, objs ...client.Object) {
		k8sClient = fake.NewClientBuilder().
			WithScheme(scheme).
			WithObjects(objs...).
			WithInterceptorFuncs(funcs).
			Build()
		mgr = scope.NewManager(k8sClient)
	}

	// helper: create a Namespace with optional labels
	makeNamespace := func(name string, labels map[string]string) *corev1.Namespace {
		return &corev1.Namespace{
			ObjectMeta: metav1.ObjectMeta{
				Name:   name,
				Labels: labels,
			},
		}
	}

	// helper: create a Pod with optional labels in a namespace
	makePod := func(namespace, name string, labels map[string]string) *corev1.Pod {
		return &corev1.Pod{
			ObjectMeta: metav1.ObjectMeta{
				Namespace: namespace,
				Name:      name,
				Labels:    labels,
			},
		}
	}

	// helper: create a Deployment with optional labels in a namespace
	makeDeployment := func(namespace, name string, labels map[string]string) *appsv1.Deployment {
		return &appsv1.Deployment{
			ObjectMeta: metav1.ObjectMeta{
				Namespace: namespace,
				Name:      name,
				Labels:    labels,
			},
		}
	}

	// helper: create a Node with optional labels (cluster-scoped)
	makeNode := func(name string, labels map[string]string) *corev1.Node {
		return &corev1.Node{
			ObjectMeta: metav1.ObjectMeta{
				Name:   name,
				Labels: labels,
			},
		}
	}

	// ─────────────────────────────────────────────
	// Namespaced Resources: 2-Level Hierarchy
	// ─────────────────────────────────────────────

	Describe("Namespaced Resources", func() {

		// UT-SCOPE-001-001: 2-level hierarchy resolution
		// Resource WITHOUT label inherits from namespace WITH label
		It("UT-SCOPE-001-001: should inherit managed from namespace when resource has no label", func() {
			ns := makeNamespace("production", map[string]string{
				scope.ManagedLabelKey: scope.ManagedLabelValueTrue,
			})
			pod := makePod("production", "api-pod", nil)
			setup(ns, pod)

			result, err := mgr.Check(ctx, scope.ResourceIdentity{Version: "v1", Namespace: "production", Kind: "Pod", Name: "api-pod"})
			Expect(err).ToNot(HaveOccurred())
			Expect(result.State).To(Equal(scope.ScopeVerifiedManaged), "Resource without label should inherit managed from namespace")
			Expect(result.Provenance).To(Equal(scope.ScopeSourceNamespaceLabel))
		})

		// UT-SCOPE-001-002: Resource label explicit opt-in
		// Resource WITH managed=true is managed regardless of namespace
		It("UT-SCOPE-001-002: should be managed when resource has explicit opt-in label", func() {
			ns := makeNamespace("staging", nil) // namespace has NO managed label
			deploy := makeDeployment("staging", "payment-api", map[string]string{
				scope.ManagedLabelKey: scope.ManagedLabelValueTrue,
			})
			setup(ns, deploy)

			result, err := mgr.Check(ctx, scope.ResourceIdentity{Group: "apps", Version: "v1", Namespace: "staging", Kind: "Deployment", Name: "payment-api"})
			Expect(err).ToNot(HaveOccurred())
			Expect(result.State).To(Equal(scope.ScopeVerifiedManaged), "Resource with explicit opt-in should be managed regardless of namespace")
			Expect(result.Provenance).To(Equal(scope.ScopeSourceResourceLabel))
		})

		// UT-SCOPE-001-003: Resource label explicit opt-out
		// Resource WITH managed=false is unmanaged even if namespace is managed
		It("UT-SCOPE-001-003: should be unmanaged when resource has explicit opt-out label", func() {
			ns := makeNamespace("production", map[string]string{
				scope.ManagedLabelKey: scope.ManagedLabelValueTrue,
			})
			deploy := makeDeployment("production", "legacy-app", map[string]string{
				scope.ManagedLabelKey: scope.ManagedLabelValueFalse,
			})
			setup(ns, deploy)

			result, err := mgr.Check(ctx, scope.ResourceIdentity{Group: "apps", Version: "v1", Namespace: "production", Kind: "Deployment", Name: "legacy-app"})
			Expect(err).ToNot(HaveOccurred())
			Expect(result.State).To(Equal(scope.ScopeVerifiedUnmanaged), "Resource with explicit opt-out should override namespace")
			Expect(result.Provenance).To(Equal(scope.ScopeSourceResourceLabel))
		})

		// UT-SCOPE-001-004: No resource label — inherit from namespace
		// (Same scenario as 001 but different namespace setup to verify inheritance path)
		It("UT-SCOPE-001-004: should fall through to namespace check when resource has no label", func() {
			ns := makeNamespace("monitored", map[string]string{
				scope.ManagedLabelKey: scope.ManagedLabelValueTrue,
			})
			deploy := makeDeployment("monitored", "web-app", nil) // no managed label
			setup(ns, deploy)

			result, err := mgr.Check(ctx, scope.ResourceIdentity{Group: "apps", Version: "v1", Namespace: "monitored", Kind: "Deployment", Name: "web-app"})
			Expect(err).ToNot(HaveOccurred())
			Expect(result.State).To(Equal(scope.ScopeVerifiedManaged), "Resource without label should fall through to namespace")
			Expect(result.Provenance).To(Equal(scope.ScopeSourceNamespaceLabel))
		})

		// UT-SCOPE-001-005: Namespace label managed
		It("UT-SCOPE-001-005: should be managed when namespace has managed label", func() {
			ns := makeNamespace("managed-ns", map[string]string{
				scope.ManagedLabelKey: scope.ManagedLabelValueTrue,
			})
			setup(ns)
			// Resource does not exist — fall through to namespace check

			resolution, err := mgr.Resolve(ctx, scope.ResourceIdentity{Group: "apps", Version: "v1", Namespace: "managed-ns", Kind: "Deployment", Name: "app"})
			Expect(err).ToNot(HaveOccurred())
			Expect(resolution.Included).To(BeTrue(), "Namespace with managed label should make resources managed")
			Expect(resolution.Source).To(Equal(scope.ScopeSourceNamespaceLabel))
		})

		// UT-SCOPE-001-006: Namespace label unmanaged
		It("UT-SCOPE-001-006: should be unmanaged when namespace has unmanaged label", func() {
			ns := makeNamespace("unmanaged-ns", map[string]string{
				scope.ManagedLabelKey: scope.ManagedLabelValueFalse,
			})
			setup(ns)

			resolution, err := mgr.Resolve(ctx, scope.ResourceIdentity{Group: "apps", Version: "v1", Namespace: "unmanaged-ns", Kind: "Deployment", Name: "app"})
			Expect(err).ToNot(HaveOccurred())
			Expect(resolution.Included).To(BeFalse(), "Namespace with unmanaged label should make resources unmanaged")
			Expect(resolution.Source).To(Equal(scope.ScopeSourceNamespaceLabel))
		})

		// UT-SCOPE-001-007: No labels anywhere — safe default unmanaged
		It("UT-SCOPE-001-007: should be unmanaged when no labels exist (safe default)", func() {
			ns := makeNamespace("bare-ns", nil) // no labels
			pod := makePod("bare-ns", "bare-pod", nil)
			setup(ns, pod)

			result, err := mgr.Check(ctx, scope.ResourceIdentity{Version: "v1", Namespace: "bare-ns", Kind: "Pod", Name: "bare-pod"})
			Expect(err).ToNot(HaveOccurred())
			Expect(result.State).To(Equal(scope.ScopeVerifiedUnmanaged), "No labels anywhere should default to unmanaged (safe default)")
			Expect(result.Provenance).To(Equal(scope.ScopeSourceNone))
		})

		// UT-SCOPE-001-008: Resource opt-out overrides namespace opt-in
		It("UT-SCOPE-001-008: resource opt-out should override namespace opt-in", func() {
			ns := makeNamespace("managed-ns", map[string]string{
				scope.ManagedLabelKey: scope.ManagedLabelValueTrue,
			})
			deploy := makeDeployment("managed-ns", "excluded-app", map[string]string{
				scope.ManagedLabelKey: scope.ManagedLabelValueFalse,
			})
			setup(ns, deploy)

			result, err := mgr.Check(ctx, scope.ResourceIdentity{Group: "apps", Version: "v1", Namespace: "managed-ns", Kind: "Deployment", Name: "excluded-app"})
			Expect(err).ToNot(HaveOccurred())
			Expect(result.State).To(Equal(scope.ScopeVerifiedUnmanaged), "Resource opt-out (false) should override namespace opt-in (true)")
			Expect(result.Provenance).To(Equal(scope.ScopeSourceResourceLabel))
		})

		// UT-SCOPE-001-009: Resource opt-in overrides namespace opt-out
		It("UT-SCOPE-001-009: resource opt-in should override namespace opt-out", func() {
			ns := makeNamespace("unmanaged-ns", map[string]string{
				scope.ManagedLabelKey: scope.ManagedLabelValueFalse,
			})
			deploy := makeDeployment("unmanaged-ns", "special-app", map[string]string{
				scope.ManagedLabelKey: scope.ManagedLabelValueTrue,
			})
			setup(ns, deploy)

			result, err := mgr.Check(ctx, scope.ResourceIdentity{Group: "apps", Version: "v1", Namespace: "unmanaged-ns", Kind: "Deployment", Name: "special-app"})
			Expect(err).ToNot(HaveOccurred())
			Expect(result.State).To(Equal(scope.ScopeVerifiedManaged), "Resource opt-in (true) should override namespace opt-out (false)")
			Expect(result.Provenance).To(Equal(scope.ScopeSourceResourceLabel))
		})
	})

	// ─────────────────────────────────────────────
	// Error Handling
	// ─────────────────────────────────────────────

	Describe("Error Handling", func() {

		// UT-SCOPE-001-010: Namespace not found
		It("UT-SCOPE-001-010: should return unmanaged when namespace does not exist", func() {
			setup() // empty cluster

			resolution, err := mgr.Resolve(ctx, scope.ResourceIdentity{Group: "apps", Version: "v1", Namespace: "nonexistent-ns", Kind: "Deployment", Name: "app"})
			Expect(err).ToNot(HaveOccurred(), "Namespace not found should not be an error")
			Expect(resolution.Included).To(BeFalse(), "Nonexistent namespace should be unmanaged")
			Expect(resolution.Source).To(Equal(scope.ScopeSourceNone))
		})

		// UT-SCOPE-001-011: Resource not found — check namespace only
		It("UT-SCOPE-001-011: should fall through to namespace when resource is not found", func() {
			ns := makeNamespace("existing-ns", map[string]string{
				scope.ManagedLabelKey: scope.ManagedLabelValueTrue,
			})
			setup(ns) // namespace exists, resource does NOT

			resolution, err := mgr.Resolve(ctx, scope.ResourceIdentity{Group: "apps", Version: "v1", Namespace: "existing-ns", Kind: "Deployment", Name: "nonexistent-app"})
			Expect(err).ToNot(HaveOccurred())
			Expect(resolution.Included).To(BeTrue(), "Resource not found should inherit from namespace")
			Expect(resolution.Source).To(Equal(scope.ScopeSourceNamespaceLabel))
		})

		// UT-SCOPE-001-012: Invalid label value
		It("UT-SCOPE-2592-LABEL-002 [BR-SCOPE-001]: invalid resource label must not inherit namespace scope", func() {
			ns := makeNamespace("ns-with-valid", map[string]string{
				scope.ManagedLabelKey: scope.ManagedLabelValueTrue,
			})
			pod := makePod("ns-with-valid", "weird-pod", map[string]string{
				scope.ManagedLabelKey: "yes", // invalid value — not "true" or "false"
			})
			setup(ns, pod)

			result, err := mgr.Check(ctx, scope.ResourceIdentity{Version: "v1", Namespace: "ns-with-valid", Kind: "Pod", Name: "weird-pod"})
			Expect(err).ToNot(HaveOccurred())
			Expect(result.State).To(Equal(scope.ScopeInvalid), "invalid resource label must fail closed instead of inheriting namespace scope")
			Expect(result.Provenance).To(Equal(scope.ScopeSourceUnavailable))
			Expect(result.Reason).To(Equal("invalid_resource_label"))
		})

		// UT-SCOPE-2592-LABEL-003: an empty-string label value is a present-but-
		// invalid label, not an absent one — it must fail closed rather than
		// fall through to namespace inheritance.
		It("UT-SCOPE-2592-LABEL-003 [BR-SCOPE-001]: empty-string label value is invalid, not absent", func() {
			ns := makeNamespace("ns-empty-label", map[string]string{
				scope.ManagedLabelKey: scope.ManagedLabelValueTrue,
			})
			pod := makePod("ns-empty-label", "empty-label-pod", map[string]string{
				scope.ManagedLabelKey: "", // present but empty — invalid, not absent
			})
			setup(ns, pod)

			result, err := mgr.Check(ctx, scope.ResourceIdentity{Version: "v1", Namespace: "ns-empty-label", Kind: "Pod", Name: "empty-label-pod"})
			Expect(err).ToNot(HaveOccurred())
			Expect(result.State).To(Equal(scope.ScopeInvalid),
				"an empty label value must be invalid (fail-closed), not absent (inheritable)")
		})

		// UT-SCOPE-001-016: Resource lookup non-NotFound error — fail closed.
		It("UT-SCOPE-001-016: should return an error when resource lookup fails with non-NotFound error", func() {
			ns := makeNamespace("error-ns", map[string]string{
				scope.ManagedLabelKey: scope.ManagedLabelValueTrue,
			})
			pod := makePod("error-ns", "error-pod", nil)

			setupWithInterceptor(interceptor.Funcs{
				Get: func(ctx context.Context, c client.WithWatch, key client.ObjectKey, obj client.Object, opts ...client.GetOption) error {
					// Fail on Pod Get (resource lookup), not on Namespace Get
					if key.Name == "error-pod" && key.Namespace == "error-ns" {
						return fmt.Errorf("API server unavailable")
					}
					return c.Get(ctx, key, obj, opts...)
				},
			}, ns, pod)

			result, err := mgr.Check(ctx, scope.ResourceIdentity{Version: "v1", Namespace: "error-ns", Kind: "Pod", Name: "error-pod"})
			Expect(err).To(HaveOccurred(), "Non-NotFound resource error must fail closed")
			Expect(result.State).To(Equal(scope.ScopeUnavailable))
			Expect(result.Retryable).To(BeTrue())
		})

		// UT-SCOPE-001-017: Namespace lookup non-NotFound error
		It("UT-SCOPE-001-017: should return error when namespace lookup fails with non-NotFound error", func() {
			ns := makeNamespace("ns-error", nil)

			setupWithInterceptor(interceptor.Funcs{
				Get: func(ctx context.Context, c client.WithWatch, key client.ObjectKey, obj client.Object, opts ...client.GetOption) error {
					// Fail only on Namespace Get (namespace lookup)
					if key.Name == "ns-error" && key.Namespace == "" {
						return fmt.Errorf("connection refused")
					}
					return c.Get(ctx, key, obj, opts...)
				},
			}, ns)

			// Resource "nonexistent-pod" does not exist -> falls through to namespace check -> error
			resolution, err := mgr.Resolve(ctx, scope.ResourceIdentity{Version: "v1", Namespace: "ns-error", Kind: "Pod", Name: "nonexistent-pod"})
			Expect(err).To(HaveOccurred(), "Non-NotFound namespace error must be propagated")
			Expect(err.Error()).To(ContainSubstring("connection refused"))
			Expect(resolution.Included).To(BeFalse())
		})

		// UT-SCOPE-001-018: Namespace with invalid label value (not "true" or "false")
		It("UT-SCOPE-001-018: should treat invalid namespace label value as unset (default unmanaged)", func() {
			ns := makeNamespace("ns-invalid-label", map[string]string{
				scope.ManagedLabelKey: "enabled", // invalid value
			})
			setup(ns)

			// Resource does not exist -> falls through to namespace -> invalid value -> default
			resolution, err := mgr.Resolve(ctx, scope.ResourceIdentity{Version: "v1", Namespace: "ns-invalid-label", Kind: "Pod", Name: "some-pod"})
			Expect(err).ToNot(HaveOccurred())
			Expect(resolution.Included).To(BeFalse(), "Invalid namespace label value should fall through to default unmanaged")
			Expect(resolution.Source).To(Equal(scope.ScopeSourceUnavailable))
			Expect(resolution.Reason).To(Equal("invalid_namespace_label"))
		})
	})

	// ─────────────────────────────────────────────
	// Unknown Kind & Graceful Fallthrough (ADR-053)
	// ─────────────────────────────────────────────

	Describe("Unknown Kind Resilience", func() {

		// DD-FLEET-015 (branch #2593): the typed Check lane is presence-first —
		// a missing target is an unavailable observation, never a silent
		// namespace-inheritance fallthrough (Resolve keeps the ADR-053
		// inheritance semantics asserted above).
		It("UT-SCOPE-001-011-CHECK [BR-SCOPE-001]: typed Check on a missing resource cannot inherit a managed namespace", func() {
			ns := makeNamespace("existing-ns", map[string]string{
				scope.ManagedLabelKey: scope.ManagedLabelValueTrue,
			})
			setup(ns) // namespace exists, resource does NOT

			result, err := mgr.Check(ctx, scope.ResourceIdentity{Namespace: "existing-ns", Group: "apps", Version: "v1", Kind: "Deployment", Name: "nonexistent-app"})
			Expect(err).ToNot(HaveOccurred())
			Expect(result.State).To(Equal(scope.ScopeUnavailable), "Missing target must not inherit")
		})

		It("UT-SCOPE-001-019 [BR-SCOPE-001, AC-4, AC-6, SI-10]: rejects unknown Kind without GVK", func() {
			ns := makeNamespace("production", map[string]string{
				scope.ManagedLabelKey: scope.ManagedLabelValueTrue,
			})
			setup(ns)

			result, err := mgr.Check(ctx, scope.ResourceIdentity{Namespace: "production", Kind: "Unknown", Name: "test-resource"})
			Expect(err).ToNot(HaveOccurred(), "an invalid identity is a completed domain result, not an execution failure")
			Expect(result.State).To(Equal(scope.ScopeInvalid))
			Expect(result.Reason).To(Equal("invalid_target_identity"))
		})

		// UT-SCOPE-001-020: Forbidden error on resource lookup — fail closed.
		It("UT-SCOPE-001-020: should return an error when resource lookup returns Forbidden", func() {
			ns := makeNamespace("rbac-ns", map[string]string{
				scope.ManagedLabelKey: scope.ManagedLabelValueTrue,
			})

			setupWithInterceptor(interceptor.Funcs{
				Get: func(ctx context.Context, c client.WithWatch, key client.ObjectKey, obj client.Object, opts ...client.GetOption) error {
					if key.Name == "restricted-pod" && key.Namespace == "rbac-ns" {
						return apierrors.NewForbidden(
							schema.GroupResource{Resource: "pods"},
							"restricted-pod",
							fmt.Errorf("RBAC denied"),
						)
					}
					return c.Get(ctx, key, obj, opts...)
				},
			}, ns)

			resolution, err := mgr.Resolve(ctx, scope.ResourceIdentity{Version: "v1", Namespace: "rbac-ns", Kind: "Pod", Name: "restricted-pod"})
			Expect(err).To(HaveOccurred(), "Forbidden error must fail closed")
			Expect(resolution.Included).To(BeFalse())
		})

		// UT-SCOPE-001-021: "No matches for kind" error — graceful fallthrough
		It("UT-SCOPE-001-021: should fall through to namespace when resource lookup returns no-matches error", func() {
			ns := makeNamespace("kind-ns", map[string]string{
				scope.ManagedLabelKey: scope.ManagedLabelValueTrue,
			})

			setupWithInterceptor(interceptor.Funcs{
				Get: func(ctx context.Context, c client.WithWatch, key client.ObjectKey, obj client.Object, opts ...client.GetOption) error {
					if key.Name == "custom-thing" && key.Namespace == "kind-ns" {
						return &apierrors.StatusError{
							ErrStatus: metav1.Status{
								Status:  metav1.StatusFailure,
								Code:    404,
								Reason:  metav1.StatusReasonNotFound,
								Message: "no matches for kind \"Pod\" in version \"v1\"",
							},
						}
					}
					return c.Get(ctx, key, obj, opts...)
				},
			}, ns)

			// Test a known kind (Pod) where the API server returns an unusual "no matches" error
			resolution, err := mgr.Resolve(ctx, scope.ResourceIdentity{Version: "v1", Namespace: "kind-ns", Kind: "Pod", Name: "custom-thing"})
			Expect(err).ToNot(HaveOccurred(), "No-matches error should NOT propagate — graceful fallthrough")
			Expect(resolution.Included).To(BeTrue(), "Should inherit from namespace when resource returns no-matches error")
			Expect(resolution.Source).To(Equal(scope.ScopeSourceNamespaceLabel))
		})

		It("UT-SCOPE-001-022: unknown kind without GVK cannot inherit managed namespace", func() {
			ns := makeNamespace("managed-ns", map[string]string{
				scope.ManagedLabelKey: scope.ManagedLabelValueTrue,
			})
			setup(ns)

			result, err := mgr.Check(ctx, scope.ResourceIdentity{Namespace: "managed-ns", Kind: "CustomWidget", Name: "my-widget"})
			Expect(err).ToNot(HaveOccurred(), "an invalid identity is a completed domain result, not an execution failure")
			Expect(result.State).To(Equal(scope.ScopeInvalid))
		})

		It("UT-SCOPE-001-023: unknown kind without GVK is rejected regardless of namespace", func() {
			ns := makeNamespace("unmanaged-ns", map[string]string{
				scope.ManagedLabelKey: scope.ManagedLabelValueFalse,
			})
			setup(ns)

			result, err := mgr.Check(ctx, scope.ResourceIdentity{Namespace: "unmanaged-ns", Kind: "CustomWidget", Name: "my-widget"})
			Expect(err).ToNot(HaveOccurred(), "an invalid identity is a completed domain result, not an execution failure")
			Expect(result.State).To(Equal(scope.ScopeInvalid))
		})
	})

	// ─────────────────────────────────────────────
	// Cluster-Scoped Resources
	// ─────────────────────────────────────────────

	Describe("Cluster-Scoped Resources", func() {

		// UT-SCOPE-001-013: Cluster-scoped resource with managed label
		It("UT-SCOPE-001-013: cluster-scoped resource with managed label should be managed", func() {
			node := makeNode("worker-01", map[string]string{
				scope.ManagedLabelKey: scope.ManagedLabelValueTrue,
			})
			setup(node)

			resolution, err := mgr.ResolveWithResourceLabels(ctx, scope.ResourceIdentity{Version: "v1", Kind: "Node", Name: "worker-01"},
				map[string]string{scope.ManagedLabelKey: scope.ManagedLabelValueTrue})
			Expect(err).ToNot(HaveOccurred())
			Expect(resolution.ClusterScoped).To(BeTrue())
			Expect(resolution.Included).To(BeTrue(), "Cluster-scoped resource with managed label should be managed")
			Expect(resolution.Source).To(Equal(scope.ScopeSourceResourceLabel))
		})

		// UT-SCOPE-001-014: Cluster-scoped resource without label — no namespace fallback
		It("UT-SCOPE-001-014: cluster-scoped resource without label should be unmanaged", func() {
			node := makeNode("worker-02", nil) // no labels
			setup(node)

			resolution, err := mgr.ResolveWithResourceLabels(ctx, scope.ResourceIdentity{Version: "v1", Kind: "Node", Name: "worker-02"}, nil)
			Expect(err).ToNot(HaveOccurred())
			Expect(resolution.ClusterScoped).To(BeTrue())
			Expect(resolution.Included).To(BeFalse(), "Cluster-scoped resource without label should be unmanaged (no NS fallback)")
			Expect(resolution.Source).To(Equal(scope.ScopeSourceNone))
		})

		// UT-SCOPE-001-015: Cluster-scoped resource with opt-out label
		It("UT-SCOPE-001-015: cluster-scoped resource with opt-out label should be unmanaged", func() {
			node := makeNode("worker-03", map[string]string{
				scope.ManagedLabelKey: scope.ManagedLabelValueFalse,
			})
			setup(node)

			resolution, err := mgr.ResolveWithResourceLabels(ctx, scope.ResourceIdentity{Version: "v1", Kind: "Node", Name: "worker-03"},
				map[string]string{scope.ManagedLabelKey: scope.ManagedLabelValueFalse})
			Expect(err).ToNot(HaveOccurred())
			Expect(resolution.ClusterScoped).To(BeTrue())
			Expect(resolution.Included).To(BeFalse(), "Cluster-scoped resource with opt-out label should be unmanaged")
			Expect(resolution.Source).To(Equal(scope.ScopeSourceResourceLabel))
		})
	})

	Describe("ScopeChecker with ResourceIdentity", func() {
		It("UT-SCOPE-RI-001 [SI-10]: Manager implements ScopeChecker interface", func() {
			setup()
			var checker scope.ScopeChecker = mgr
			Expect(checker).ToNot(BeNil())
		})

		It("UT-SCOPE-RI-002 [AC-4]: Resolve accepts ResourceIdentity and resolves managed resource", func() {
			deploy := &appsv1.Deployment{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "nginx",
					Namespace: "production",
					Labels:    map[string]string{scope.ManagedLabelKey: scope.ManagedLabelValueTrue},
				},
			}
			setup(deploy)

			resolution, err := mgr.Resolve(ctx, scope.ResourceIdentity{
				Namespace: "production",
				Kind:      "Deployment",
				Name:      "nginx",
				Group:     "apps",
				Version:   "v1",
			})
			Expect(err).ToNot(HaveOccurred())
			Expect(resolution.Included).To(BeTrue())
			Expect(resolution.Source).To(Equal(scope.ScopeSourceResourceLabel))
		})

		It("UT-SCOPE-RI-003 [BR-SCOPE-001, AC-4, AC-6, SI-10]: rejects empty Group/Version", func() {
			deploy := &appsv1.Deployment{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "redis",
					Namespace: "default",
					Labels:    map[string]string{scope.ManagedLabelKey: scope.ManagedLabelValueTrue},
				},
			}
			setup(deploy)

			_, err := mgr.Resolve(ctx, scope.ResourceIdentity{
				Namespace: "default",
				Kind:      "Deployment",
				Name:      "redis",
			})
			Expect(err).To(HaveOccurred())
		})

		It("UT-SCOPE-RI-004 [AC-4]: Resolve with ClusterID set on local Manager returns error", func() {
			_, err := mgr.Resolve(ctx, scope.ResourceIdentity{Version: "v1",
				ClusterID: "prod-east",
				Namespace: "default",
				Kind:      "Pod",
				Name:      "nginx",
			})
			Expect(err).To(HaveOccurred(), "Local Manager cannot resolve remote clusters")
		})

		It("UT-SCOPE-RI-005 [SI-10]: ResourceIdentity struct validates required fields", func() {
			ri := scope.ResourceIdentity{Kind: "Pod", Name: "nginx", Namespace: "default"}
			Expect(ri.Kind).To(Equal("Pod"))
			Expect(ri.Name).To(Equal("nginx"))
			Expect(ri.ClusterID).To(BeEmpty())
			Expect(ri.Group).To(BeEmpty())
			Expect(ri.Version).To(BeEmpty())
		})

		It("UT-SCOPE-RI-002-CHECK [AC-4]: Check accepts ResourceIdentity and resolves managed resource", func() {
			deploy := &appsv1.Deployment{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "nginx",
					Namespace: "production",
					Labels:    map[string]string{scope.ManagedLabelKey: scope.ManagedLabelValueTrue},
				},
			}
			setup(deploy)

			result, err := mgr.Check(ctx, scope.ResourceIdentity{
				Namespace: "production",
				Kind:      "Deployment",
				Name:      "nginx",
				Group:     "apps",
				Version:   "v1",
			})
			Expect(err).ToNot(HaveOccurred())
			Expect(result.State).To(Equal(scope.ScopeVerifiedManaged))
		})

		It("UT-SCOPE-RI-003-CHECK [BR-SCOPE-001, AC-4, AC-6, SI-10]: typed Check rejects a Kind-only identity", func() {
			result, err := mgr.Check(ctx, scope.ResourceIdentity{
				Namespace: "default",
				Kind:      "Deployment",
				Name:      "redis",
			})
			Expect(err).ToNot(HaveOccurred())
			Expect(result.State).To(Equal(scope.ScopeInvalid), "identity is never inferred from Kind")
		})

		It("UT-SCOPE-RI-004-CHECK [AC-4]: Check with ClusterID set on local Manager returns invalid", func() {
			result, err := mgr.Check(ctx, scope.ResourceIdentity{
				ClusterID: "prod-east",
				Namespace: "default",
				Kind:      "Pod",
				Name:      "nginx",
			})
			Expect(err).ToNot(HaveOccurred())
			Expect(result.State).To(Equal(scope.ScopeInvalid))
		})
	})
})
