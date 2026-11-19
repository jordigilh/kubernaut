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

// Package-internal spec: it pins the unexported classifier the adapters use
// before fingerprinting. Ginkgo registers it in the same binary as the
// adapters_test specs, which own RunSpecs.
package adapters

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	fakediscovery "k8s.io/client-go/discovery/fake"
	fakeclientset "k8s.io/client-go/kubernetes/fake"
)

// discoveryWithClusterScopedCRD returns a discovery client that knows a
// cluster-scoped custom resource absent from the static scope map.
func discoveryWithClusterScopedCRD() *fakediscovery.FakeDiscovery {
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
	return fd
}

var _ = Describe("Cluster-scoped target detection for fingerprint normalization (BR-GATEWAY-004, #2670)", func() {

	It("UT-GW-2670-004: the static map classifies known cluster-scoped kinds without discovery", func() {
		Expect(targetClusterScoped(nil, "Node")).To(BeTrue())
		Expect(targetClusterScoped(nil, "PersistentVolume")).To(BeTrue())
		Expect(targetClusterScoped(nil, "Namespace")).To(BeTrue())
	})

	It("UT-GW-2670-005: discovery extends the classification to cluster-scoped CRDs", func() {
		registry, err := NewAPIResourceRegistry(discoveryWithClusterScopedCRD())
		Expect(err).ToNot(HaveOccurred())

		Expect(targetClusterScoped(registry, "ClusterVolume")).To(BeTrue(),
			"a discovered cluster-scoped CRD must lose its alert namespace, or its dedup key and RR target disagree")
		Expect(targetClusterScoped(registry, "Node")).To(BeTrue(), "static and discovery agree for Node")
	})

	It("UT-GW-2670-006: an unknown or namespaced kind keeps its namespace", func() {
		registry, err := NewAPIResourceRegistry(discoveryWithClusterScopedCRD())
		Expect(err).ToNot(HaveOccurred())

		Expect(targetClusterScoped(registry, "Pod")).To(BeFalse())
		Expect(targetClusterScoped(registry, "NotDiscovered")).To(BeFalse(),
			"an unknown kind stays namespaced: dropping a namespace it might need is worse than keeping one it does not")
		Expect(targetClusterScoped(nil, "NotDiscovered")).To(BeFalse())
	})
})
