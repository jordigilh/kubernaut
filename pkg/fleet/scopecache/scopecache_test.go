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

	"github.com/go-logr/logr"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"k8s.io/apimachinery/pkg/runtime/schema"
	"sigs.k8s.io/controller-runtime/pkg/client"

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

type mockLocalChecker struct {
	managed map[string]bool
}

func (m *mockLocalChecker) IsManaged(_ context.Context, namespace, kind, name string) (bool, error) {
	key := namespace + "/" + kind + "/" + name
	return m.managed[key], nil
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

	Describe("IsManaged", func() {
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

var _ = Describe("FederatedScopeChecker shared interface compliance (Phase B)", func() {
	It("UT-FLEET-FC-010 [CC6.1]: FederatedScopeChecker satisfies scope.FederatedScopeChecker interface for logical access security", func() {
		local := &mockLocalChecker{managed: map[string]bool{}}
		reader := &mockCacheReader{store: make(map[string]bool)}
		client := scopecache.NewClient(reader)
		fc := scopecache.NewFederatedScopeChecker(local, client, logr.Discard())

		var iface scope.FederatedScopeChecker = fc
		Expect(iface).ToNot(BeNil(),
			"FederatedScopeChecker must implement the shared scope.FederatedScopeChecker interface (CC6.1: consistent logical access control)")
	})
})

var _ = Describe("FederatedScopeChecker (BR-INTEGRATION-065)", func() {
	var (
		ctx    context.Context
		local  *mockLocalChecker
		reader *mockCacheReader
		fc     *scopecache.FederatedScopeChecker
	)

	BeforeEach(func() {
		ctx = context.Background()
		local = &mockLocalChecker{managed: map[string]bool{
			"default/Deployment/nginx": true,
		}}
		reader = &mockCacheReader{store: make(map[string]bool)}
		client := scopecache.NewClient(reader)
		fc = scopecache.NewFederatedScopeChecker(local, client, logr.Discard())
	})

	Describe("IsManaged (standard interface)", func() {
		It("UT-FLEET-FC-001: delegates to local checker", func() {
			managed, err := fc.IsManaged(ctx, "default", "Deployment", "nginx")
			Expect(err).ToNot(HaveOccurred())
			Expect(managed).To(BeTrue())
		})

		It("UT-FLEET-FC-002: returns false for unmanaged local resource", func() {
			managed, err := fc.IsManaged(ctx, "default", "Deployment", "redis")
			Expect(err).ToNot(HaveOccurred())
			Expect(managed).To(BeFalse())
		})
	})

	Describe("IsManagedOnCluster", func() {
		It("UT-FLEET-FC-003: empty clusterID routes to local", func() {
			managed, err := fc.IsManagedOnCluster(ctx, "", "default", "Deployment", "nginx")
			Expect(err).ToNot(HaveOccurred())
			Expect(managed).To(BeTrue())
		})

		It("UT-FLEET-FC-004: non-empty clusterID routes to remote cache", func() {
			key, keyErr := scopecache.BuildKey("prod-east", "", "", "Deployment", "default", "nginx")
			Expect(keyErr).ToNot(HaveOccurred())
			reader.store[key] = true

			managed, err := fc.IsManagedOnCluster(ctx, "prod-east", "default", "Deployment", "nginx")
			Expect(err).ToNot(HaveOccurred())
			Expect(managed).To(BeTrue())
		})

		It("UT-FLEET-FC-005: cache miss on remote returns false (fail-safe)", func() {
			managed, err := fc.IsManagedOnCluster(ctx, "prod-west", "default", "Deployment", "unknown")
			Expect(err).ToNot(HaveOccurred())
			Expect(managed).To(BeFalse())
		})
	})
})

var _ = Describe("RemoteScopeResolver typed params (Phase C)", func() {
	It("UT-FLEET-SC-010 [SI-10]: IsManaged accepts GVK + ObjectKey for type-safe input validation", func() {
		reader := &mockCacheReader{store: make(map[string]bool)}
		key, err := scopecache.BuildKey("prod-east", "apps", "v1", "Deployment", "default", "nginx")
		Expect(err).ToNot(HaveOccurred())
		reader.store[key] = true

		resolver := scopecache.NewClient(reader)

		gvk := schema.GroupVersionKind{Group: "apps", Version: "v1", Kind: "Deployment"}
		objKey := client.ObjectKey{Namespace: "default", Name: "nginx"}

		managed, err := resolver.IsManagedTyped(context.Background(), "prod-east", gvk, objKey)
		Expect(err).ToNot(HaveOccurred())
		Expect(managed).To(BeTrue(),
			"typed params must produce the same result as raw string params (SI-10: input validation)")
	})
})
