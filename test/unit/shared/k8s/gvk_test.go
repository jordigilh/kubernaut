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
})
