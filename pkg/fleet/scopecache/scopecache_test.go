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
