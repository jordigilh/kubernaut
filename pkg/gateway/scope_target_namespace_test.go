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

// Package-internal spec: it pins the namespace the Gateway's scope identity
// carries, including the discovery-extended classification. Ginkgo registers it
// in the same binary as the gateway_test specs, which own RunSpecs.
package gateway

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	fakediscovery "k8s.io/client-go/discovery/fake"
	fakeclientset "k8s.io/client-go/kubernetes/fake"

	"github.com/jordigilh/kubernaut/pkg/gateway/adapters"
)

func scopeRegistryWithClusterScopedCRD() *adapters.APIResourceRegistry {
	cs := fakeclientset.NewSimpleClientset()
	fd, ok := cs.Discovery().(*fakediscovery.FakeDiscovery)
	if !ok {
		panic("fake clientset Discovery() is not *fakediscovery.FakeDiscovery")
	}
	fd.Resources = []*metav1.APIResourceList{
		{
			GroupVersion: "v1",
			APIResources: []metav1.APIResource{
				{Name: "pods", SingularName: "pod", Kind: "Pod", Namespaced: true},
				{Name: "nodes", SingularName: "node", Kind: "Node", Namespaced: false},
			},
		},
		{
			GroupVersion: "storage.example.com/v1",
			APIResources: []metav1.APIResource{
				{Name: "clustervolumes", SingularName: "clustervolume", Kind: "ClusterVolume", Namespaced: false},
			},
		},
	}
	registry, err := adapters.NewAPIResourceRegistry(fd)
	Expect(err).ToNot(HaveOccurred())
	return registry
}

var _ = Describe("Scope target namespace selection (#2670)", func() {

	It("UT-GW-2670-007: a known cluster-scoped kind carries no namespace", func() {
		Expect(scopeTargetNamespace("Node", "openshift-monitoring", "openshift-monitoring", nil)).To(BeEmpty())
		Expect(scopeTargetNamespace("PersistentVolume", "monitoring", "", nil)).To(BeEmpty())
	})

	It("UT-GW-2670-008: discovery extends the rule to a cluster-scoped CRD", func() {
		registry := scopeRegistryWithClusterScopedCRD()
		// The adapter already blanked Resource.Namespace; without the discovery
		// clause the alert namespace would be written back onto the RR target.
		Expect(scopeTargetNamespace("ClusterVolume", "monitoring", "", registry)).To(BeEmpty())
	})

	It("UT-GW-2670-009: namespaced and unknown kinds keep the target namespace", func() {
		registry := scopeRegistryWithClusterScopedCRD()
		Expect(scopeTargetNamespace("Pod", "production", "", registry)).To(Equal("production"))
		Expect(scopeTargetNamespace("Pod", "production", "payments", registry)).To(Equal("payments"))
		Expect(scopeTargetNamespace("NotDiscovered", "production", "", registry)).To(Equal("production"))
		Expect(scopeTargetNamespace("NotDiscovered", "production", "", nil)).To(Equal("production"))
	})
})
