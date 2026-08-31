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

package scopecache_test

import (
	"context"
	"fmt"
	"testing"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/jordigilh/kubernaut/pkg/fleet/scopecache"
	"github.com/jordigilh/kubernaut/pkg/shared/scope"
)

func TestScopeCache(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Fleet ScopeCache Suite")
}

type mockCacheReader struct {
	store map[string]bool
}

func (m *mockCacheReader) Exists(_ context.Context, key string) (bool, error) {
	val, ok := m.store[key]
	if !ok {
		return false, fmt.Errorf("key not found: %s", key)
	}
	return val, nil
}

var _ = Describe("ScopeCache Client (BR-INTEGRATION-065)", func() {
	var (
		ctx    context.Context
		reader *mockCacheReader
		client *scopecache.Client
	)

	BeforeEach(func() {
		ctx = context.Background()
		reader = &mockCacheReader{store: make(map[string]bool)}
		client = scopecache.NewClient(reader)
	})

	Describe("IsManaged (Valkey cache lookup)", func() {
		It("UT-FLEET-SC-001: returns true for managed resource", func() {
			key, err := scopecache.BuildKey("prod-east", "apps", "v1", "Deployment", "default", "nginx")
			Expect(err).ToNot(HaveOccurred())
			reader.store[key] = true

			managed, err := client.IsManaged(ctx, "prod-east", "apps", "v1", "Deployment", "default", "nginx")
			Expect(err).ToNot(HaveOccurred())
			Expect(managed).To(BeTrue())
		})

		It("UT-FLEET-SC-002: returns error for cache miss", func() {
			_, err := client.IsManaged(ctx, "prod-west", "apps", "v1", "Deployment", "default", "redis")
			Expect(err).To(HaveOccurred())
		})
	})

	Describe("IsManagedResource (scope.ScopeChecker)", func() {
		It("UT-FLEET-SC-009: Client implements scope.ScopeChecker", func() {
			var checker scope.ScopeChecker = client
			Expect(checker).ToNot(BeNil())
		})

		It("UT-FLEET-SC-010: IsManagedResource delegates to IsManaged with ResourceIdentity fields", func() {
			key, err := scopecache.BuildKey("prod-east", "apps", "v1", "Deployment", "default", "nginx")
			Expect(err).ToNot(HaveOccurred())
			reader.store[key] = true

			managed, err := client.IsManagedResource(ctx, scope.ResourceIdentity{
				ClusterID: "prod-east",
				Group:     "apps",
				Version:   "v1",
				Kind:      "Deployment",
				Namespace: "default",
				Name:      "nginx",
			})
			Expect(err).ToNot(HaveOccurred())
			Expect(managed).To(BeTrue())
		})

		// Issue #54 SOC2 gap RCA (CI run 28495045499): every fleet E2E caller
		// (pkg/gateway/server.go validateScope) builds a scope.ResourceIdentity
		// with Group/Version left empty -- exactly as documented on
		// scope.ResourceIdentity ("optional -- inferred from Kind when empty").
		// pkg/fleet/fmc/syncer.go, however, writes cache keys using the
		// resource's REAL GVK obtained from the K8s API (e.g. "apps/v1" for
		// Deployment). Without inferring Group/Version here the same way
		// scope.Manager already does (pkg/shared/scope/manager.go InferGVK),
		// the read-side key ("kubernaut:managed:...://Deployment:...") never
		// matches the write-side key ("kubernaut:managed:...:apps/v1/Deployment:..."),
		// so every federated scope check permanently and deterministically
		// resolves to "not managed" regardless of any sync/cache timing.
		It("UT-FLEET-SC-011 [AC-4]: IsManagedResource with empty Group/Version infers from Kind like scope.Manager", func() {
			key, err := scopecache.BuildKey("loopback-cluster", "apps", "v1", "Deployment", "kubernaut-system", "memory-eater")
			Expect(err).ToNot(HaveOccurred())
			reader.store[key] = true

			managed, err := client.IsManagedResource(ctx, scope.ResourceIdentity{
				ClusterID: "loopback-cluster",
				Kind:      "Deployment",
				Namespace: "kubernaut-system",
				Name:      "memory-eater",
			})
			Expect(err).ToNot(HaveOccurred())
			Expect(managed).To(BeTrue(),
				"empty Group/Version must be inferred from Kind (apps/v1 for Deployment) to match syncer-written keys")
		})

		It("UT-FLEET-SC-012 [AC-4]: IsManagedResource infers core-group empty-string GVK for Pod", func() {
			key, err := scopecache.BuildKey("loopback-cluster", "", "v1", "Pod", "kubernaut-system", "web-0")
			Expect(err).ToNot(HaveOccurred())
			reader.store[key] = true

			managed, err := client.IsManagedResource(ctx, scope.ResourceIdentity{
				ClusterID: "loopback-cluster",
				Kind:      "Pod",
				Namespace: "kubernaut-system",
				Name:      "web-0",
			})
			Expect(err).ToNot(HaveOccurred())
			Expect(managed).To(BeTrue())
		})

		// Issue #2311: fleet (remote-cluster) scope checks must honor the same
		// 2-level hierarchy (resource label, then namespace label) that
		// pkg/shared/scope.Manager already implements for the local/hub
		// cluster (ADR-053). Before this fix, a namespace labeled
		// kubernaut.ai/managed=true with unlabeled resources inside it never
		// got those resources synced by pkg/fleet/fmc/syncer.go (a K8s label
		// selector can't match "resources whose namespace has this label"),
		// so every such resource was permanently rejected as unmanaged on
		// fleet clusters despite working correctly on the hub.
		It("UT-FLEET-SC-013 [Issue #2311]: IsManagedResource falls back to the namespace-level label when the resource-level key is not cached", func() {
			nsKey, err := scopecache.BuildKey("remote-cluster", "", "v1", "Namespace", "", "demo-checkout")
			Expect(err).ToNot(HaveOccurred())
			reader.store[nsKey] = true
			// Deliberately do NOT set the resource-level key -- the pod itself
			// carries no label, only its namespace does.

			managed, err := client.IsManagedResource(ctx, scope.ResourceIdentity{
				ClusterID: "remote-cluster",
				Kind:      "Pod",
				Namespace: "demo-checkout",
				Name:      "worker-5f74bb54f7-28jpv",
			})
			Expect(err).ToNot(HaveOccurred())
			Expect(managed).To(BeTrue(),
				"a namespace-level kubernaut.ai/managed=true label must be honored for fleet resources, same as local")
		})

		It("UT-FLEET-SC-014 [Issue #2311]: IsManagedResource prefers the resource-level label over the namespace fallback when both are cached", func() {
			resKey, err := scopecache.BuildKey("remote-cluster", "", "v1", "Pod", "demo-checkout", "worker-1")
			Expect(err).ToNot(HaveOccurred())
			reader.store[resKey] = true
			// No namespace-level key needed -- resource-level hit should short-circuit.

			managed, err := client.IsManagedResource(ctx, scope.ResourceIdentity{
				ClusterID: "remote-cluster",
				Kind:      "Pod",
				Namespace: "demo-checkout",
				Name:      "worker-1",
			})
			Expect(err).ToNot(HaveOccurred())
			Expect(managed).To(BeTrue())
		})

		It("UT-FLEET-SC-015 [Issue #2311]: IsManagedResource returns unmanaged when neither the resource nor its namespace is cached", func() {
			managed, _ := client.IsManagedResource(ctx, scope.ResourceIdentity{
				ClusterID: "remote-cluster",
				Kind:      "Pod",
				Namespace: "unmanaged-ns",
				Name:      "orphan",
			})
			Expect(managed).To(BeFalse())
		})

		It("UT-FLEET-SC-016 [Issue #2311]: IsManagedResource does not fall back to a namespace for cluster-scoped kinds, mirroring scope.Manager", func() {
			// A coincidentally-matching "namespace" key must never be
			// consulted for a cluster-scoped resource (e.g. Node) -- it has
			// no namespace to inherit from (ADR-053).
			nsKey, err := scopecache.BuildKey("remote-cluster", "", "v1", "Namespace", "", "worker-node-1")
			Expect(err).ToNot(HaveOccurred())
			reader.store[nsKey] = true

			managed, _ := client.IsManagedResource(ctx, scope.ResourceIdentity{
				ClusterID: "remote-cluster",
				Kind:      "Node",
				Namespace: "",
				Name:      "worker-node-1",
			})
			Expect(managed).To(BeFalse(),
				"Node is cluster-scoped and must only ever check its own resource-level label")
		})
	})

	Describe("BuildKey", func() {
		It("UT-FLEET-SC-003: produces expected key format", func() {
			key, err := scopecache.BuildKey("prod-east", "apps", "v1", "Deployment", "default", "nginx")
			Expect(err).ToNot(HaveOccurred())
			Expect(key).To(Equal("kubernaut:managed:prod-east:apps/v1/Deployment:default/nginx"))
		})

		It("UT-FLEET-SC-006: rejects empty clusterID", func() {
			_, err := scopecache.BuildKey("", "apps", "v1", "Deployment", "default", "nginx")
			Expect(err).To(MatchError(scopecache.ErrEmptyClusterID))
		})

		It("UT-FLEET-SC-007: rejects empty kind", func() {
			_, err := scopecache.BuildKey("prod-east", "apps", "v1", "", "default", "nginx")
			Expect(err).To(MatchError(scopecache.ErrEmptyKind))
		})

		It("UT-FLEET-SC-008: rejects empty name", func() {
			_, err := scopecache.BuildKey("prod-east", "apps", "v1", "Deployment", "default", "")
			Expect(err).To(MatchError(scopecache.ErrEmptyName))
		})
	})
})
