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

			managed, err := mgr.IsManaged(ctx, "production", "Pod", "api-pod")
			Expect(err).ToNot(HaveOccurred())
			Expect(managed).To(BeTrue(), "Resource without label should inherit managed from namespace")
		})

		// UT-SCOPE-001-002: Resource label explicit opt-in
		// Resource WITH managed=true is managed regardless of namespace
		It("UT-SCOPE-001-002: should be managed when resource has explicit opt-in label", func() {
			ns := makeNamespace("staging", nil) // namespace has NO managed label
			deploy := makeDeployment("staging", "payment-api", map[string]string{
				scope.ManagedLabelKey: scope.ManagedLabelValueTrue,
			})
			setup(ns, deploy)

			managed, err := mgr.IsManaged(ctx, "staging", "Deployment", "payment-api")
			Expect(err).ToNot(HaveOccurred())
			Expect(managed).To(BeTrue(), "Resource with explicit opt-in should be managed regardless of namespace")
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

			managed, err := mgr.IsManaged(ctx, "production", "Deployment", "legacy-app")
			Expect(err).ToNot(HaveOccurred())
			Expect(managed).To(BeFalse(), "Resource with explicit opt-out should override namespace")
		})

		// UT-SCOPE-001-004: No resource label — inherit from namespace
		// (Same scenario as 001 but different namespace setup to verify inheritance path)
		It("UT-SCOPE-001-004: should fall through to namespace check when resource has no label", func() {
			ns := makeNamespace("monitored", map[string]string{
				scope.ManagedLabelKey: scope.ManagedLabelValueTrue,
			})
			deploy := makeDeployment("monitored", "web-app", nil) // no managed label
			setup(ns, deploy)

			managed, err := mgr.IsManaged(ctx, "monitored", "Deployment", "web-app")
			Expect(err).ToNot(HaveOccurred())
			Expect(managed).To(BeTrue(), "Resource without label should fall through to namespace")
		})

		// UT-SCOPE-001-005: Namespace label managed
		It("UT-SCOPE-001-005: should be managed when namespace has managed label", func() {
			ns := makeNamespace("managed-ns", map[string]string{
				scope.ManagedLabelKey: scope.ManagedLabelValueTrue,
			})
			setup(ns)
			// Resource does not exist — fall through to namespace check

			managed, err := mgr.IsManaged(ctx, "managed-ns", "Deployment", "app")
			Expect(err).ToNot(HaveOccurred())
			Expect(managed).To(BeTrue(), "Namespace with managed label should make resources managed")
		})

		// UT-SCOPE-001-006: Namespace label unmanaged
		It("UT-SCOPE-001-006: should be unmanaged when namespace has unmanaged label", func() {
			ns := makeNamespace("unmanaged-ns", map[string]string{
				scope.ManagedLabelKey: scope.ManagedLabelValueFalse,
			})
			setup(ns)

			managed, err := mgr.IsManaged(ctx, "unmanaged-ns", "Deployment", "app")
			Expect(err).ToNot(HaveOccurred())
			Expect(managed).To(BeFalse(), "Namespace with unmanaged label should make resources unmanaged")
		})

		// UT-SCOPE-001-007: No labels anywhere — safe default unmanaged
		It("UT-SCOPE-001-007: should be unmanaged when no labels exist (safe default)", func() {
			ns := makeNamespace("bare-ns", nil) // no labels
			pod := makePod("bare-ns", "bare-pod", nil)
			setup(ns, pod)

			managed, err := mgr.IsManaged(ctx, "bare-ns", "Pod", "bare-pod")
			Expect(err).ToNot(HaveOccurred())
			Expect(managed).To(BeFalse(), "No labels anywhere should default to unmanaged (safe default)")
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

			managed, err := mgr.IsManaged(ctx, "managed-ns", "Deployment", "excluded-app")
			Expect(err).ToNot(HaveOccurred())
			Expect(managed).To(BeFalse(), "Resource opt-out (false) should override namespace opt-in (true)")
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

			managed, err := mgr.IsManaged(ctx, "unmanaged-ns", "Deployment", "special-app")
			Expect(err).ToNot(HaveOccurred())
			Expect(managed).To(BeTrue(), "Resource opt-in (true) should override namespace opt-out (false)")
		})
	})

	// ─────────────────────────────────────────────
	// Error Handling
	// ─────────────────────────────────────────────

	Describe("Error Handling", func() {

		// UT-SCOPE-001-010: Namespace not found
		It("UT-SCOPE-001-010: should return unmanaged when namespace does not exist", func() {
			setup() // empty cluster

			managed, err := mgr.IsManaged(ctx, "nonexistent-ns", "Deployment", "app")
			Expect(err).ToNot(HaveOccurred(), "Namespace not found should not be an error")
			Expect(managed).To(BeFalse(), "Nonexistent namespace should be unmanaged")
		})

		// UT-SCOPE-001-011: Resource not found — check namespace only
		It("UT-SCOPE-001-011: should fall through to namespace when resource is not found", func() {
			ns := makeNamespace("existing-ns", map[string]string{
				scope.ManagedLabelKey: scope.ManagedLabelValueTrue,
			})
			setup(ns) // namespace exists, resource does NOT

			managed, err := mgr.IsManaged(ctx, "existing-ns", "Deployment", "nonexistent-app")
			Expect(err).ToNot(HaveOccurred())
			Expect(managed).To(BeTrue(), "Resource not found should inherit from namespace")
		})

		// UT-SCOPE-001-012: Invalid label value
		It("UT-SCOPE-001-012: should treat invalid label value as unset (fall through)", func() {
			ns := makeNamespace("ns-with-valid", map[string]string{
				scope.ManagedLabelKey: scope.ManagedLabelValueTrue,
			})
			pod := makePod("ns-with-valid", "weird-pod", map[string]string{
				scope.ManagedLabelKey: "yes", // invalid value — not "true" or "false"
			})
			setup(ns, pod)

			managed, err := mgr.IsManaged(ctx, "ns-with-valid", "Pod", "weird-pod")
			Expect(err).ToNot(HaveOccurred())
			Expect(managed).To(BeTrue(), "Invalid resource label should be ignored, inheriting from namespace")
		})

		// UT-SCOPE-001-016: Resource lookup non-NotFound error — graceful fallthrough (ADR-053)
		// UPDATED: Non-NotFound resource errors now fall through to namespace check
		// (previously propagated as error). This aligns with ADR-053 Decision #5.
		It("UT-SCOPE-001-016: should fall through to namespace when resource lookup fails with non-NotFound error", func() {
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

			managed, err := mgr.IsManaged(ctx, "error-ns", "Pod", "error-pod")
			Expect(err).ToNot(HaveOccurred(), "Non-NotFound resource error should gracefully fall through (ADR-053)")
			Expect(managed).To(BeTrue(), "Should inherit from namespace when resource check fails")
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
			managed, err := mgr.IsManaged(ctx, "ns-error", "Pod", "nonexistent-pod")
			Expect(err).To(HaveOccurred(), "Non-NotFound namespace error must be propagated")
			Expect(err.Error()).To(ContainSubstring("connection refused"))
			Expect(managed).To(BeFalse())
		})

		// UT-SCOPE-001-018: Namespace with invalid label value (not "true" or "false")
		It("UT-SCOPE-001-018: should treat invalid namespace label value as unset (default unmanaged)", func() {
			ns := makeNamespace("ns-invalid-label", map[string]string{
				scope.ManagedLabelKey: "enabled", // invalid value
			})
			setup(ns)

			// Resource does not exist -> falls through to namespace -> invalid value -> default
			managed, err := mgr.IsManaged(ctx, "ns-invalid-label", "Pod", "some-pod")
			Expect(err).ToNot(HaveOccurred())
			Expect(managed).To(BeFalse(), "Invalid namespace label value should fall through to default unmanaged")
		})
	})

	// ─────────────────────────────────────────────
	// Unknown Kind & Graceful Fallthrough (ADR-053)
	// ─────────────────────────────────────────────

	Describe("Unknown Kind Resilience", func() {

		// UT-SCOPE-001-019: Unknown kind — skips resource check, falls through to namespace
		It("UT-SCOPE-001-019: should skip resource check for unknown kind and fall through to namespace", func() {
			ns := makeNamespace("production", map[string]string{
				scope.ManagedLabelKey: scope.ManagedLabelValueTrue,
			})
			setup(ns)

			// "Unknown" is not in kindToGroup — resource check should be skipped entirely
			managed, err := mgr.IsManaged(ctx, "production", "Unknown", "test-resource")
			Expect(err).ToNot(HaveOccurred())
			Expect(managed).To(BeTrue(), "Unknown kind should skip resource check and inherit from namespace")
		})

		// UT-SCOPE-001-020: Forbidden error on resource lookup — graceful fallthrough
		It("UT-SCOPE-001-020: should fall through to namespace when resource lookup returns Forbidden", func() {
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

			managed, err := mgr.IsManaged(ctx, "rbac-ns", "Pod", "restricted-pod")
			Expect(err).ToNot(HaveOccurred(), "Forbidden error should NOT propagate — graceful fallthrough")
			Expect(managed).To(BeTrue(), "Should inherit from namespace when resource check is Forbidden")
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
			managed, err := mgr.IsManaged(ctx, "kind-ns", "Pod", "custom-thing")
			Expect(err).ToNot(HaveOccurred(), "No-matches error should NOT propagate — graceful fallthrough")
			Expect(managed).To(BeTrue(), "Should inherit from namespace when resource returns no-matches error")
		})

		// UT-SCOPE-001-022: Unknown kind with managed namespace returns managed
		It("UT-SCOPE-001-022: unknown kind with managed namespace should return managed", func() {
			ns := makeNamespace("managed-ns", map[string]string{
				scope.ManagedLabelKey: scope.ManagedLabelValueTrue,
			})
			setup(ns)

			managed, err := mgr.IsManaged(ctx, "managed-ns", "CustomWidget", "my-widget")
			Expect(err).ToNot(HaveOccurred())
			Expect(managed).To(BeTrue(), "Unknown kind should inherit managed from namespace")
		})

		// UT-SCOPE-001-023: Unknown kind with unmanaged namespace returns unmanaged
		It("UT-SCOPE-001-023: unknown kind with unmanaged namespace should return unmanaged", func() {
			ns := makeNamespace("unmanaged-ns", map[string]string{
				scope.ManagedLabelKey: scope.ManagedLabelValueFalse,
			})
			setup(ns)

			managed, err := mgr.IsManaged(ctx, "unmanaged-ns", "CustomWidget", "my-widget")
			Expect(err).ToNot(HaveOccurred())
			Expect(managed).To(BeFalse(), "Unknown kind should inherit unmanaged from namespace")
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

			managed, err := mgr.IsManaged(ctx, "", "Node", "worker-01")
			Expect(err).ToNot(HaveOccurred())
			Expect(managed).To(BeTrue(), "Cluster-scoped resource with managed label should be managed")
		})

		// UT-SCOPE-001-014: Cluster-scoped resource without label — no namespace fallback
		It("UT-SCOPE-001-014: cluster-scoped resource without label should be unmanaged", func() {
			node := makeNode("worker-02", nil) // no labels
			setup(node)

			managed, err := mgr.IsManaged(ctx, "", "Node", "worker-02")
			Expect(err).ToNot(HaveOccurred())
			Expect(managed).To(BeFalse(), "Cluster-scoped resource without label should be unmanaged (no NS fallback)")
		})

		// UT-SCOPE-001-015: Cluster-scoped resource with opt-out label
		It("UT-SCOPE-001-015: cluster-scoped resource with opt-out label should be unmanaged", func() {
			node := makeNode("worker-03", map[string]string{
				scope.ManagedLabelKey: scope.ManagedLabelValueFalse,
			})
			setup(node)

			managed, err := mgr.IsManaged(ctx, "", "Node", "worker-03")
			Expect(err).ToNot(HaveOccurred())
			Expect(managed).To(BeFalse(), "Cluster-scoped resource with opt-out label should be unmanaged")
		})
	})
})
