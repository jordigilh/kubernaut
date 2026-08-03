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

package k8s_test

import (
	"testing"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"k8s.io/apimachinery/pkg/api/meta"
	"k8s.io/apimachinery/pkg/runtime/schema"

	k8sutil "github.com/jordigilh/kubernaut/pkg/shared/k8s"
)

func TestSharedK8s(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Shared K8s Utilities Suite")
}

// mockRESTMapper implements meta.RESTMapper for unit testing.
// Only KindsFor is used by ResolveGVKForKind; other methods panic if called.
type mockRESTMapper struct {
	kindsForResults map[string][]schema.GroupVersionKind
}

func (m *mockRESTMapper) KindsFor(resource schema.GroupVersionResource) ([]schema.GroupVersionKind, error) {
	if gvks, ok := m.kindsForResults[resource.Resource]; ok {
		return gvks, nil
	}
	return nil, &meta.NoResourceMatchError{PartialResource: resource}
}

func (m *mockRESTMapper) KindFor(schema.GroupVersionResource) (schema.GroupVersionKind, error) {
	panic("not implemented")
}
func (m *mockRESTMapper) ResourceFor(schema.GroupVersionResource) (schema.GroupVersionResource, error) {
	panic("not implemented")
}
func (m *mockRESTMapper) ResourcesFor(schema.GroupVersionResource) ([]schema.GroupVersionResource, error) {
	panic("not implemented")
}
func (m *mockRESTMapper) RESTMapping(gk schema.GroupKind, versions ...string) (*meta.RESTMapping, error) {
	panic("not implemented")
}
func (m *mockRESTMapper) RESTMappings(gk schema.GroupKind, versions ...string) ([]*meta.RESTMapping, error) {
	panic("not implemented")
}
func (m *mockRESTMapper) ResourceSingularizer(resource string) (string, error) {
	panic("not implemented")
}

// resettableMockMapper simulates a restmapper.DeferredDiscoveryRESTMapper whose first
// KindsFor call misses a CRD group (as if it were absent from the pod's first discovery
// round — #1888) but resolves correctly once Reset() is called and the lookup retried.
type resettableMockMapper struct {
	*mockRESTMapper
	kindsForCalls    int
	resetCalls       int
	postResetResults map[string][]schema.GroupVersionKind
}

func (m *resettableMockMapper) KindsFor(resource schema.GroupVersionResource) ([]schema.GroupVersionKind, error) {
	m.kindsForCalls++
	if m.resetCalls > 0 {
		if gvks, ok := m.postResetResults[resource.Resource]; ok {
			return gvks, nil
		}
		return nil, &meta.NoResourceMatchError{PartialResource: resource}
	}
	return m.mockRESTMapper.KindsFor(resource)
}

func (m *resettableMockMapper) Reset() {
	m.resetCalls++
}

// alwaysFailingResettableMapper simulates a mapper that still cannot resolve a kind even
// after Reset() — e.g., a genuinely invalid/typo'd Kind, or discovery still down.
type alwaysFailingResettableMapper struct {
	*mockRESTMapper
	resetCalls int
}

func (m *alwaysFailingResettableMapper) Reset() {
	m.resetCalls++
}

var _ = Describe("ResolveGVKForKind (#310)", func() {

	// Mock REST mapper that returns metrics.k8s.io/v1beta1/Node first —
	// simulating a cluster with metrics-server installed.
	var metricsMapper *mockRESTMapper

	BeforeEach(func() {
		metricsMapper = &mockRESTMapper{
			kindsForResults: map[string][]schema.GroupVersionKind{
				"nodes": {
					{Group: "metrics.k8s.io", Version: "v1beta1", Kind: "Node"},
					{Group: "", Version: "v1", Kind: "Node"},
				},
				"customwidgets": {
					{Group: "example.com", Version: "v1", Kind: "CustomWidget"},
				},
			},
		}
	})

	// #310: Node must resolve to core/v1, not metrics.k8s.io
	Context("well-known kinds", func() {
		DescribeTable("should resolve to the correct GVK without using the REST mapper",
			func(kind string, expectedGroup, expectedVersion string) {
				gvk, err := k8sutil.ResolveGVKForKind(nil, kind)
				Expect(err).NotTo(HaveOccurred())
				Expect(gvk.Group).To(Equal(expectedGroup))
				Expect(gvk.Version).To(Equal(expectedVersion))
				Expect(gvk.Kind).To(Equal(kind))
			},
			Entry("Node → core/v1 (#310)", "Node", "", "v1"),
			Entry("ReplicaSet → apps/v1 (#303)", "ReplicaSet", "apps", "v1"),
			Entry("Deployment → apps/v1", "Deployment", "apps", "v1"),
			Entry("StatefulSet → apps/v1", "StatefulSet", "apps", "v1"),
			Entry("DaemonSet → apps/v1", "DaemonSet", "apps", "v1"),
			Entry("Pod → core/v1", "Pod", "", "v1"),
			Entry("Service → core/v1", "Service", "", "v1"),
			Entry("ConfigMap → core/v1", "ConfigMap", "", "v1"),
			Entry("HorizontalPodAutoscaler → autoscaling/v2", "HorizontalPodAutoscaler", "autoscaling", "v2"),
			Entry("PodDisruptionBudget → policy/v1", "PodDisruptionBudget", "policy", "v1"),
			Entry("Certificate → cert-manager.io/v1", "Certificate", "cert-manager.io", "v1"),

		// Issue #1275: kubernaut.ai CRD kinds (CM-6)
		Entry("UT-K8S-1275-001: RemediationRequest → kubernaut.ai/v1alpha1", "RemediationRequest", "kubernaut.ai", "v1alpha1"),
		Entry("UT-K8S-1275-002: RemediationWorkflow → kubernaut.ai/v1alpha1", "RemediationWorkflow", "kubernaut.ai", "v1alpha1"),
		Entry("UT-K8S-1275-003: InvestigationSession → kubernaut.ai/v1alpha1", "InvestigationSession", "kubernaut.ai", "v1alpha1"),
		Entry("UT-K8S-1275-004: AIAnalysis → kubernaut.ai/v1alpha1", "AIAnalysis", "kubernaut.ai", "v1alpha1"),
		Entry("UT-K8S-1275-005: SignalProcessing → kubernaut.ai/v1alpha1", "SignalProcessing", "kubernaut.ai", "v1alpha1"),
		Entry("UT-K8S-1275-006: EffectivenessAssessment → kubernaut.ai/v1alpha1", "EffectivenessAssessment", "kubernaut.ai", "v1alpha1"),
		Entry("UT-K8S-1275-007: WorkflowExecution → kubernaut.ai/v1alpha1", "WorkflowExecution", "kubernaut.ai", "v1alpha1"),
		Entry("UT-K8S-1275-008: ActionType → kubernaut.ai/v1alpha1", "ActionType", "kubernaut.ai", "v1alpha1"),
		Entry("UT-K8S-1275-009: NotificationRequest → kubernaut.ai/v1alpha1", "NotificationRequest", "kubernaut.ai", "v1alpha1"),
		)

		It("should resolve Node to core/v1 even when metrics-server registers metrics.k8s.io/v1beta1/Node", func() {
			gvk, err := k8sutil.ResolveGVKForKind(metricsMapper, "Node")
			Expect(err).NotTo(HaveOccurred())
			Expect(gvk).To(Equal(schema.GroupVersionKind{Group: "", Version: "v1", Kind: "Node"}),
				"#310: Node must resolve to core/v1, not metrics.k8s.io")
		})
	})

	Context("REST mapper fallback for unknown kinds", func() {
		It("should fall back to mapper for CRDs", func() {
			gvk, err := k8sutil.ResolveGVKForKind(metricsMapper, "CustomWidget")
			Expect(err).NotTo(HaveOccurred())
			Expect(gvk).To(Equal(schema.GroupVersionKind{
				Group: "example.com", Version: "v1", Kind: "CustomWidget",
			}))
		})

		It("should return error when kind is unknown and mapper has no match", func() {
			_, err := k8sutil.ResolveGVKForKind(metricsMapper, "NonExistentKind")
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("NonExistentKind"))
		})

		It("should return error when kind is unknown and mapper is nil", func() {
			_, err := k8sutil.ResolveGVKForKind(nil, "NonExistentKind")
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("NonExistentKind"))
		})
	})

	Context("REST mapper self-heal on lookup failure (#1888)", func() {
		It("UT-K8S-1888-001: resolves a kind that missed the first discovery round once Reset() retries", func() {
			mapper := &resettableMockMapper{
				mockRESTMapper: &mockRESTMapper{},
				// meta.UnsafeGuessKindToResource("Search") guesses the plural "searchs"
				// (the naive heuristic doesn't know the real CRD plural is "searches" —
				// irrelevant here since this mock controls resolution directly).
				postResetResults: map[string][]schema.GroupVersionKind{
					"searchs": {{Group: "search.open-cluster-management.io", Version: "v1alpha1", Kind: "Search"}},
				},
			}

			gvk, err := k8sutil.ResolveGVKForKind(mapper, "Search")

			Expect(err).NotTo(HaveOccurred(),
				"a kind missing from the first discovery round must self-heal on retry, not stay permanently unresolvable")
			Expect(gvk).To(Equal(schema.GroupVersionKind{
				Group: "search.open-cluster-management.io", Version: "v1alpha1", Kind: "Search",
			}))
		})

		It("UT-K8S-1888-002: retries exactly once — Reset() and KindsFor are not called in an unbounded loop", func() {
			mapper := &resettableMockMapper{
				mockRESTMapper: &mockRESTMapper{},
				postResetResults: map[string][]schema.GroupVersionKind{
					"searchs": {{Group: "search.open-cluster-management.io", Version: "v1alpha1", Kind: "Search"}},
				},
			}

			_, err := k8sutil.ResolveGVKForKind(mapper, "Search")

			Expect(err).NotTo(HaveOccurred())
			Expect(mapper.resetCalls).To(Equal(1), "Reset() must be called exactly once on failure, not looped")
			Expect(mapper.kindsForCalls).To(Equal(2), "KindsFor must be called exactly twice: once before Reset(), once after")
		})

		It("UT-K8S-1888-003: propagates the original error when the kind is still unresolvable after Reset()", func() {
			mapper := &alwaysFailingResettableMapper{mockRESTMapper: &mockRESTMapper{}}

			_, err := k8sutil.ResolveGVKForKind(mapper, "TrulyUnknownKind")

			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("TrulyUnknownKind"))
			Expect(mapper.resetCalls).To(Equal(1), "Reset() should still be attempted once even though it doesn't help")
		})

		It("UT-K8S-1888-004: a mapper without Reset() behaves exactly as before (no retry, no panic)", func() {
			// metricsMapper (plain mockRESTMapper) does not implement meta.ResettableRESTMapper.
			_, err := k8sutil.ResolveGVKForKind(metricsMapper, "NonExistentKind")

			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("NonExistentKind"))
		})
	})
})
