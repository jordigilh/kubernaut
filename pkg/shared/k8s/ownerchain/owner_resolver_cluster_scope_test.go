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

package ownerchain_test

import (
	"context"

	"github.com/go-logr/logr"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/jordigilh/kubernaut/pkg/shared/k8s/ownerchain"
)

// clusterScopedOwnerReader serves a namespaced Pod controlled by a
// cluster-scoped owner, then the owner itself. It records the key of each fetch
// so the test can prove the traversal stopped passing the Pod's namespace down
// to the owner lookup (#2670).
type clusterScopedOwnerReader struct {
	ownerKind       string
	ownerAPIVersion string
	ownerName       string
	keys            []client.ObjectKey
}

func (r *clusterScopedOwnerReader) Get(_ context.Context, key client.ObjectKey, obj client.Object, _ ...client.GetOption) error {
	r.keys = append(r.keys, key)
	obj.SetNamespace(key.Namespace)
	obj.SetName(key.Name)
	if len(r.keys) == 1 && obj.GetObjectKind().GroupVersionKind().Kind == "Pod" {
		controller := true
		ownerKind := r.ownerKind
		if ownerKind == "" {
			ownerKind = "Node"
		}
		ownerName := r.ownerName
		if ownerName == "" {
			ownerName = "node-1"
		}
		ownerAPIVersion := r.ownerAPIVersion
		if ownerAPIVersion == "" {
			ownerAPIVersion = "v1"
		}
		obj.SetOwnerReferences([]metav1.OwnerReference{{
			APIVersion: ownerAPIVersion, Kind: ownerKind, Name: ownerName, Controller: &controller,
		}})
	}
	return nil
}

func (r *clusterScopedOwnerReader) List(_ context.Context, _ client.ObjectList, _ ...client.ListOption) error {
	return nil
}

// crdScopeResolver is a KindResolver that also answers the discovery-backed
// scope question, standing in for Gateway's adapters.APIResourceRegistry in the
// owner-chain package.
type crdScopeResolver struct {
	clusterScoped bool
}

func (crdScopeResolver) KindToGVR(string) (schema.GroupVersionResource, bool) {
	return schema.GroupVersionResource{}, false
}

func (crdScopeResolver) IsCoreBatchAppsKind(string) bool { return false }

func (r crdScopeResolver) IsClusterScopedTargetKind(string, string) bool { return r.clusterScoped }

var _ = Describe("Owner identity chain cluster-scoped namespace normalization (#2670)", func() {
	ctx := context.Background()

	It("UT-GW-2670-026: a mirror Pod resolving to its Node keeps no namespace", func() {
		reader := &clusterScopedOwnerReader{}
		resolver := ownerchain.NewK8sOwnerResolver(reader, logr.Discard())

		resolved, err := resolver.ResolveTopLevelOwnerIdentity(ctx, entry("kube-system", "Pod", "mirror-node-1", "v1"))

		Expect(err).NotTo(HaveOccurred())
		Expect(resolved).To(Equal(entry("", "Node", "node-1", "v1")),
			"a cluster-scoped owner carries no namespace; keeping kube-system would hash a different fingerprint than a direct Node event")
		Expect(reader.keys).To(HaveLen(2))
		Expect(reader.keys[1]).To(Equal(client.ObjectKey{Name: "node-1"}),
			"the Node fetch must be cluster-scoped, not issued in the Pod's namespace")
	})

	It("UT-GW-2670-027: a namespaced owner still inherits the child's namespace", func() {
		controller := true
		reader := &gvkCapturingReader{ownerRef: &metav1.OwnerReference{
			APIVersion: "apps/v1", Kind: "ReplicaSet", Name: "payment-api-abc", Controller: &controller,
		}}
		resolver := ownerchain.NewK8sOwnerResolver(reader, logr.Discard())

		resolved, err := resolver.ResolveTopLevelOwnerIdentity(ctx, entry("production", "Pod", "payment-api-xyz", "v1"))

		Expect(err).NotTo(HaveOccurred())
		Expect(resolved).To(Equal(entry("production", "ReplicaSet", "payment-api-abc", "apps/v1")),
			"a namespaced owner keeps the child's namespace")
		Expect(reader.gotGVKs[1]).To(Equal(schema.GroupVersionKind{Group: "apps", Version: "v1", Kind: "ReplicaSet"}))
	})

	It("UT-GW-2670-029: a discovered cluster-scoped CRD owner keeps no namespace", func() {
		// The static kind map does not know storage.example.com/v1 ClusterVolume,
		// so only the discovery-backed classifier can blank the namespace the
		// child Pod would otherwise pass down (#2670).
		reader := &clusterScopedOwnerReader{
			ownerKind: "ClusterVolume", ownerAPIVersion: "storage.example.com/v1", ownerName: "vol-1",
		}
		resolver := ownerchain.NewK8sOwnerResolver(reader, logr.Discard(),
			ownerchain.WithRegistry(crdScopeResolver{clusterScoped: true}))

		resolved, err := resolver.ResolveTopLevelOwnerIdentity(ctx, entry("production", "Pod", "pod-1", "v1"))

		Expect(err).NotTo(HaveOccurred())
		Expect(resolved).To(Equal(entry("", "ClusterVolume", "vol-1", "storage.example.com/v1")),
			"a discovered cluster-scoped owner must hash the same identity as a direct event for it")
		Expect(reader.keys).To(HaveLen(2))
		Expect(reader.keys[1]).To(Equal(client.ObjectKey{Name: "vol-1"}),
			"the owner verification fetch must be cluster-scoped")
	})
})
