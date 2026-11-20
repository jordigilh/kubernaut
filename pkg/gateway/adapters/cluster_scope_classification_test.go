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
		Expect(targetClusterScoped(nil, "Node", "v1")).To(BeTrue())
		Expect(targetClusterScoped(nil, "PersistentVolume", "v1")).To(BeTrue())
		Expect(targetClusterScoped(nil, "Namespace", "v1")).To(BeTrue())
	})

	It("UT-GW-2670-005: discovery extends the classification to cluster-scoped CRDs", func() {
		registry, err := NewAPIResourceRegistry(discoveryWithClusterScopedCRD())
		Expect(err).ToNot(HaveOccurred())

		Expect(targetClusterScoped(registry, "ClusterVolume", "storage.example.com/v1")).To(BeTrue(),
			"a discovered cluster-scoped CRD must lose its alert namespace, or its dedup key and RR target disagree")
		Expect(targetClusterScoped(registry, "Node", "v1")).To(BeTrue(), "static and discovery agree for Node")
	})

	It("UT-GW-2670-006: an unknown or namespaced kind keeps its namespace", func() {
		registry, err := NewAPIResourceRegistry(discoveryWithClusterScopedCRD())
		Expect(err).ToNot(HaveOccurred())

		Expect(targetClusterScoped(registry, "Pod", "v1")).To(BeFalse())
		Expect(targetClusterScoped(registry, "NotDiscovered", "example.com/v1")).To(BeFalse(),
			"an unknown kind stays namespaced: dropping a namespace it might need is worse than keeping one it does not")
		Expect(targetClusterScoped(nil, "NotDiscovered", "example.com/v1")).To(BeFalse())
	})

	It("UT-GW-2670-022: a Kind served with different scopes by two groups is classified per group", func() {
		// Same Kind, two groups, opposite scopes. The per-Kind map can only hold
		// one answer (first group discovered wins), so the group named by the
		// target's apiVersion must decide (#2670).
		clusterGroup := &metav1.APIResourceList{GroupVersion: "cluster.example.com/v1", APIResources: []metav1.APIResource{
			{Name: "widgets", SingularName: "widget", Kind: "Widget", Namespaced: false},
		}}
		tenantGroup := &metav1.APIResourceList{GroupVersion: "tenant.example.com/v1", APIResources: []metav1.APIResource{
			{Name: "widgets", SingularName: "widget", Kind: "Widget", Namespaced: true},
		}}
		for _, discovery := range [][]*metav1.APIResourceList{
			{clusterGroup, tenantGroup},
			{tenantGroup, clusterGroup},
		} {
			cs := fakeclientset.NewSimpleClientset()
			fd, ok := cs.Discovery().(*fakediscovery.FakeDiscovery)
			Expect(ok).To(BeTrue())
			fd.Resources = discovery
			registry, err := NewAPIResourceRegistry(fd)
			Expect(err).ToNot(HaveOccurred())

			Expect(targetClusterScoped(registry, "Widget", "cluster.example.com/v1")).To(BeTrue(),
				"the cluster-scoped group must blank the namespace in either discovery order")
			Expect(targetClusterScoped(registry, "Widget", "tenant.example.com/v1")).To(BeFalse(),
				"the namespaced group must keep its namespace in either discovery order")
		}
	})

	It("UT-GW-2670-028: an explicit apiVersion group outranks the static map", func() {
		// A CRD may reuse a core Kind with the opposite scope. When the identity
		// names the group, discovery for that group is authoritative; the static
		// map is only the fallback (#2670).
		cs := fakeclientset.NewSimpleClientset()
		fd, ok := cs.Discovery().(*fakediscovery.FakeDiscovery)
		Expect(ok).To(BeTrue())
		fd.Resources = []*metav1.APIResourceList{
			{GroupVersion: "v1", APIResources: []metav1.APIResource{
				{Name: "nodes", SingularName: "node", Kind: "Node", Namespaced: false},
			}},
			{GroupVersion: "tenant.example.com/v1", APIResources: []metav1.APIResource{
				{Name: "nodes", SingularName: "node", Kind: "Node", Namespaced: true},
			}},
		}
		registry, err := NewAPIResourceRegistry(fd)
		Expect(err).ToNot(HaveOccurred())

		Expect(targetClusterScoped(registry, "Node", "tenant.example.com/v1")).To(BeFalse(),
			"a namespaced CRD that reuses a core Kind must keep its namespace")
		Expect(targetClusterScoped(registry, "Node", "v1")).To(BeTrue(),
			"the core Node stays cluster-scoped")
		Expect(targetClusterScoped(nil, "Node", "tenant.example.com/v1")).To(BeTrue(),
			"without discovery the static map is the only answer")
	})
})
