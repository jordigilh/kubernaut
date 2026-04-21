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

package investigator_test

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"k8s.io/apimachinery/pkg/api/meta"
	"k8s.io/apimachinery/pkg/runtime/schema"

	"github.com/jordigilh/kubernaut/internal/kubernautagent/investigator"
)

var _ = Describe("TP-763: ScopeResolver and namespace forcing", func() {

	Describe("UT-KA-763-001: ScopeResolver.IsClusterScoped returns true for Node", func() {
		It("should return true for a cluster-scoped kind", func() {
			mapper := meta.NewDefaultRESTMapper([]schema.GroupVersion{
				{Group: "", Version: "v1"},
			})
			mapper.Add(schema.GroupVersionKind{Group: "", Version: "v1", Kind: "Node"}, meta.RESTScopeRoot)

			resolver := investigator.NewMapperScopeResolver(mapper)
			isCluster, err := resolver.IsClusterScoped("Node")
			Expect(err).NotTo(HaveOccurred())
			Expect(isCluster).To(BeTrue(), "UT-KA-763-001: Node must be cluster-scoped")
		})
	})

	Describe("UT-KA-763-002: ScopeResolver.IsClusterScoped returns false for Deployment", func() {
		It("should return false for a namespaced kind", func() {
			mapper := meta.NewDefaultRESTMapper([]schema.GroupVersion{
				{Group: "apps", Version: "v1"},
			})
			mapper.Add(schema.GroupVersionKind{Group: "apps", Version: "v1", Kind: "Deployment"}, meta.RESTScopeNamespace)

			resolver := investigator.NewMapperScopeResolver(mapper)
			isCluster, err := resolver.IsClusterScoped("Deployment")
			Expect(err).NotTo(HaveOccurred())
			Expect(isCluster).To(BeFalse(), "UT-KA-763-002: Deployment must be namespaced")
		})
	})
})
