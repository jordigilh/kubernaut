/*
Copyright 2026 Jordi Gil.

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
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"k8s.io/apimachinery/pkg/api/meta"
	"k8s.io/apimachinery/pkg/runtime/schema"

	k8sutil "github.com/jordigilh/kubernaut/pkg/shared/k8s"
)

// apiVersionMapper implements meta.RESTMapper with RESTMapping support
// for testing ResolveGVKWithAPIVersion.
type apiVersionMapper struct {
	mappings map[schema.GroupKind]*meta.RESTMapping
}

func (m *apiVersionMapper) KindsFor(resource schema.GroupVersionResource) ([]schema.GroupVersionKind, error) {
	return nil, &meta.NoResourceMatchError{PartialResource: resource}
}

func (m *apiVersionMapper) KindFor(schema.GroupVersionResource) (schema.GroupVersionKind, error) {
	panic("not implemented")
}
func (m *apiVersionMapper) ResourceFor(schema.GroupVersionResource) (schema.GroupVersionResource, error) {
	panic("not implemented")
}
func (m *apiVersionMapper) ResourcesFor(schema.GroupVersionResource) ([]schema.GroupVersionResource, error) {
	panic("not implemented")
}
func (m *apiVersionMapper) RESTMapping(gk schema.GroupKind, versions ...string) (*meta.RESTMapping, error) {
	if mapping, ok := m.mappings[gk]; ok {
		return mapping, nil
	}
	return nil, &meta.NoResourceMatchError{PartialResource: schema.GroupVersionResource{
		Group: gk.Group, Resource: gk.Kind,
	}}
}
func (m *apiVersionMapper) RESTMappings(gk schema.GroupKind, versions ...string) ([]*meta.RESTMapping, error) {
	panic("not implemented")
}
func (m *apiVersionMapper) ResourceSingularizer(resource string) (string, error) {
	panic("not implemented")
}

var _ = Describe("ResolveGVKWithAPIVersion — Issue #1040", func() {
	var mapper *apiVersionMapper

	BeforeEach(func() {
		mapper = &apiVersionMapper{
			mappings: map[schema.GroupKind]*meta.RESTMapping{
				{Group: "route.openshift.io", Kind: "Route"}: {
					Resource: schema.GroupVersionResource{
						Group: "route.openshift.io", Version: "v1", Resource: "routes",
					},
					GroupVersionKind: schema.GroupVersionKind{
						Group: "route.openshift.io", Version: "v1", Kind: "Route",
					},
				},
				{Group: "apps", Kind: "Deployment"}: {
					Resource: schema.GroupVersionResource{
						Group: "apps", Version: "v1", Resource: "deployments",
					},
					GroupVersionKind: schema.GroupVersionKind{
						Group: "apps", Version: "v1", Kind: "Deployment",
					},
				},
			},
		}
	})

	Describe("UT-KA-1040-003: Uses apiVersion when present", func() {
		It("should resolve Route to route.openshift.io/v1 when apiVersion is provided", func() {
			gvk, err := k8sutil.ResolveGVKWithAPIVersion(mapper, "Route", "route.openshift.io/v1")
			Expect(err).NotTo(HaveOccurred())
			Expect(gvk.Group).To(Equal("route.openshift.io"),
				"UT-KA-1040-003: must resolve to route.openshift.io group")
			Expect(gvk.Version).To(Equal("v1"))
			Expect(gvk.Kind).To(Equal("Route"))
		})
	})

	Describe("UT-KA-1040-004: Falls back when apiVersion empty", func() {
		It("should fall back to ResolveGVKForKind for well-known kinds", func() {
			gvk, err := k8sutil.ResolveGVKWithAPIVersion(mapper, "Deployment", "")
			Expect(err).NotTo(HaveOccurred())
			Expect(gvk.Group).To(Equal("apps"),
				"UT-KA-1040-004: must fall back to static table for Deployment")
			Expect(gvk.Kind).To(Equal("Deployment"))
		})
	})

	Describe("UT-KA-1040-008: Rejects invalid apiVersion", func() {
		It("should return error for malformed apiVersion", func() {
			_, err := k8sutil.ResolveGVKWithAPIVersion(mapper, "Route", "///invalid")
			Expect(err).To(HaveOccurred(),
				"UT-KA-1040-008: malformed apiVersion must produce an error")
		})

		It("should return error when kind not found in given apiVersion", func() {
			_, err := k8sutil.ResolveGVKWithAPIVersion(mapper, "Widget", "custom.io/v1")
			Expect(err).To(HaveOccurred(),
				"UT-KA-1040-008: unknown kind+apiVersion combination must error")
		})
	})
})
