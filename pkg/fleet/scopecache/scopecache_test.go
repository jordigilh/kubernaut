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
	// labels carries presence-aware observations for cases a boolean cannot
	// express (absent, invalid). It takes precedence over store; a key in
	// neither map is a cache miss.
	labels map[string]scope.LabelState
}

func (m *mockCacheReader) Exists(_ context.Context, key string) (bool, error) {
	val, ok := m.store[key]
	if !ok {
		return false, fmt.Errorf("key not found: %s", key)
	}
	return val, nil
}

func (m *mockCacheReader) Read(_ context.Context, key string) (scopecache.CachedObservation, error) {
	if state, ok := m.labels[key]; ok {
		return scopecache.CachedObservation{State: scopecache.RecordVerifiedPresent, LabelState: state}, nil
	}
	value, exists := m.store[key]
	if !exists {
		return scopecache.CachedObservation{State: scopecache.RecordMissing}, nil
	}
	label := scope.LabelStateFalse
	if value {
		label = scope.LabelStateTrue
	}
	return scopecache.CachedObservation{State: scopecache.RecordVerifiedPresent, LabelState: label}, nil
}

var _ = Describe("ScopeCache Client (BR-INTEGRATION-065)", func() {
	var (
		ctx    context.Context
		reader *mockCacheReader
		client *scopecache.Client
	)

	BeforeEach(func() {
		ctx = context.Background()
		reader = &mockCacheReader{store: make(map[string]bool), labels: make(map[string]scope.LabelState)}
		client = scopecache.NewClient(reader)
	})

	Describe("Check (observed Valkey metadata)", func() {
		It("UT-FLEET-SC-001: returns true for managed resource", func() {
			key, err := scopecache.BuildKey("prod-east", "apps", "v1", "Deployment", "default", "nginx")
			Expect(err).ToNot(HaveOccurred())
			reader.store[key] = true

			managed, err := client.Check(ctx, scope.ResourceIdentity{ClusterID: "prod-east", Group: "apps", Version: "v1", Kind: "Deployment", Namespace: "default", Name: "nginx"})
			Expect(err).ToNot(HaveOccurred())
			Expect(managed.State).To(Equal(scope.ScopeVerifiedManaged))
		})

		It("UT-FLEET-SC-002: returns unavailable for cache miss", func() {
			result, err := client.Check(ctx, scope.ResourceIdentity{ClusterID: "prod-west", Group: "apps", Version: "v1", Kind: "Deployment", Namespace: "default", Name: "redis"})
			Expect(err).NotTo(HaveOccurred())
			Expect(result.State).To(Equal(scope.ScopeUnavailable))
		})
	})

	Describe("Check (scope.ScopeChecker)", func() {
		It("UT-FLEET-SC-2592 [BR-SCOPE-001, AC-4, AC-6, SI-10]: rejects a missing GVK even when the namespace is managed", func() {
			key, err := scopecache.BuildKey("remote", "", "v1", "Namespace", "", "default")
			Expect(err).NotTo(HaveOccurred())
			reader.store[key] = true
			result, err := client.Check(ctx, scope.ResourceIdentity{ClusterID: "remote", Kind: "Pod", Namespace: "default", Name: "p"})
			Expect(err).NotTo(HaveOccurred())
			Expect(result.State).To(Equal(scope.ScopeInvalid), "identity is never inferred from Kind")
		})
		It("UT-FLEET-SC-009: Client implements scope.ScopeChecker", func() {
			var checker scope.ScopeChecker = client
			Expect(checker).ToNot(BeNil())
		})

		It("UT-FLEET-SC-010: Check delegates to IsManaged with ResourceIdentity fields", func() {
			key, err := scopecache.BuildKey("prod-east", "apps", "v1", "Deployment", "default", "nginx")
			Expect(err).ToNot(HaveOccurred())
			reader.store[key] = true

			managed, err := client.Check(ctx, scope.ResourceIdentity{
				ClusterID: "prod-east",
				Group:     "apps",
				Version:   "v1",
				Kind:      "Deployment",
				Namespace: "default",
				Name:      "nginx",
			})
			Expect(err).ToNot(HaveOccurred())
			Expect(managed.State).To(Equal(scope.ScopeVerifiedManaged))
		})

		It("UT-FMC-PRES-008 [SI-10]: remote scope checks require explicit GVK", func() {
			key, err := scopecache.BuildKey("loopback-cluster", "apps", "v1", "Deployment", "kubernaut-system", "memory-eater")
			Expect(err).ToNot(HaveOccurred())
			reader.store[key] = true

			managed, err := client.Check(ctx, scope.ResourceIdentity{
				ClusterID: "loopback-cluster",
				Group:     "apps",
				Version:   "v1",
				Kind:      "Deployment",
				Namespace: "kubernaut-system",
				Name:      "memory-eater",
			})
			Expect(err).ToNot(HaveOccurred())
			Expect(managed.State).To(Equal(scope.ScopeVerifiedManaged),
				"the exact explicit GVK must match the syncer-written key")
		})

		It("UT-FMC-PRES-008 [SI-10]: missing remote GVK cannot use a core-v1 fallback", func() {
			key, err := scopecache.BuildKey("loopback-cluster", "", "v1", "Pod", "kubernaut-system", "web-0")
			Expect(err).NotTo(HaveOccurred())
			reader.store[key] = true

			managed, err := client.Check(ctx, scope.ResourceIdentity{
				ClusterID: "loopback-cluster",
				Kind:      "Pod",
				Namespace: "kubernaut-system",
				Name:      "web-0",
			})
			Expect(err).NotTo(HaveOccurred())
			Expect(managed.State).To(Equal(scope.ScopeInvalid))
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
		It("UT-FMC-CACHE-001 [BR-SCOPE-001]: cache miss does not fall back to a managed namespace", func() {
			nsKey, err := scopecache.BuildKey("remote-cluster", "", "v1", "Namespace", "", "demo-checkout")
			Expect(err).ToNot(HaveOccurred())
			reader.store[nsKey] = true
			// Deliberately do NOT set the resource-level key -- the pod itself
			// carries no label, only its namespace does.

			managed, err := client.Check(ctx, scope.ResourceIdentity{
				ClusterID: "remote-cluster",
				Version:   "v1",
				Kind:      "Pod",
				Namespace: "demo-checkout",
				Name:      "worker-5f74bb54f7-28jpv",
			})
			Expect(err).NotTo(HaveOccurred(), "a completed missing-observation outcome is not an execution error")
			Expect(managed.State).To(Equal(scope.ScopeUnavailable),
				"a managed namespace must not authorize a resource whose presence is unverified")
		})

		It("UT-FLEET-SC-014 [Issue #2311]: Check prefers the resource-level label over the namespace fallback when both are cached", func() {
			resKey, err := scopecache.BuildKey("remote-cluster", "", "v1", "Pod", "demo-checkout", "worker-1")
			Expect(err).ToNot(HaveOccurred())
			reader.store[resKey] = true
			// No namespace-level key needed -- resource-level hit should short-circuit.

			managed, err := client.Check(ctx, scope.ResourceIdentity{
				ClusterID: "remote-cluster",
				Version:   "v1",
				Kind:      "Pod",
				Namespace: "demo-checkout",
				Name:      "worker-1",
			})
			Expect(err).ToNot(HaveOccurred())
			Expect(managed.State).To(Equal(scope.ScopeVerifiedManaged))
		})

		It("UT-FMC-CACHE-002 [BR-SCOPE-001]: cache miss is observable instead of verified unmanaged", func() {
			managed, err := client.Check(ctx, scope.ResourceIdentity{
				ClusterID: "remote-cluster",
				Version:   "v1",
				Kind:      "Pod",
				Namespace: "unmanaged-ns",
				Name:      "orphan",
			})
			Expect(err).NotTo(HaveOccurred())
			Expect(managed.State).To(Equal(scope.ScopeUnavailable))
		})

		It("UT-FLEET-SC-016 [Issue #2311, #2670]: a cluster-scoped target never falls back to a namespace", func() {
			// A coincidentally-matching "namespace" key must never be
			// consulted for a cluster-scoped resource (e.g. Node) -- it has
			// no namespace to inherit from (ADR-053, AC-6). The Node is
			// present with an absent label, which is decisive unmanaged.
			nsKey, err := scopecache.BuildKey("remote-cluster", "", "v1", "Namespace", "", "worker-node-1")
			Expect(err).ToNot(HaveOccurred())
			reader.store[nsKey] = true
			nodeKey, err := scopecache.BuildKey("remote-cluster", "", "v1", "Node", "", "worker-node-1")
			Expect(err).ToNot(HaveOccurred())
			reader.labels[nodeKey] = scope.LabelStateAbsent

			managed, _ := client.Check(ctx, scope.ResourceIdentity{
				ClusterID: "remote-cluster",
				Version:   "v1",
				Kind:      "Node",
				Namespace: "",
				Name:      "worker-node-1",
			})
			Expect(managed.State).To(Equal(scope.ScopeVerifiedUnmanaged),
				"Node resolves at level 1 only; the namespace key must not authorize it")
			Expect(managed.Provenance).To(Equal(scope.ScopeSourceNone))
		})
	})

	Describe("BuildKey", func() {
		It("UT-FLEET-SC-2592-KEY [BR-SCOPE-001, AC-4, AC-6, SI-10]: rejects missing version", func() {
			_, err := scopecache.BuildKey("prod-east", "apps", "", "Deployment", "default", "nginx")
			Expect(err).To(HaveOccurred())
		})
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
