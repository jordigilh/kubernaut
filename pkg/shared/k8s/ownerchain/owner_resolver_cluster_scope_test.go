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
// cluster-scoped Node, then a standalone Node. It records the key of each fetch
// so the test can prove the traversal stopped passing the Pod's namespace down
// to the Node lookup (#2670).
type clusterScopedOwnerReader struct {
	keys []client.ObjectKey
}

func (r *clusterScopedOwnerReader) Get(_ context.Context, key client.ObjectKey, obj client.Object, _ ...client.GetOption) error {
	r.keys = append(r.keys, key)
	obj.SetNamespace(key.Namespace)
	obj.SetName(key.Name)
	if len(r.keys) == 1 && obj.GetObjectKind().GroupVersionKind().Kind == "Pod" {
		controller := true
		obj.SetOwnerReferences([]metav1.OwnerReference{{
			APIVersion: "v1", Kind: "Node", Name: "node-1", Controller: &controller,
		}})
	}
	return nil
}

func (r *clusterScopedOwnerReader) List(_ context.Context, _ client.ObjectList, _ ...client.ListOption) error {
	return nil
}

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
})
