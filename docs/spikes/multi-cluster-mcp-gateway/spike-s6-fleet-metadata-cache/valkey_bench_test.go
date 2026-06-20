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

// Package spike_s6 contains validation code for the Fleet Metadata Cache concept.
// This code is NOT production code — it lives under docs/spikes/ per project convention.
package spike_s6

import (
	"context"
	"fmt"
	"testing"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

func TestSpikeS6(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Spike S6 — Fleet Metadata Cache")
}

// ScopeCacheClient is the interface that GW/RO will use to check if a resource
// is managed by kubernaut on a remote cluster.
type ScopeCacheClient interface {
	// IsManaged checks if a resource is labeled kubernaut.ai/managed=true
	// on the given cluster. Returns true/false and an error if the check fails.
	IsManaged(ctx context.Context, clusterID, group, version, kind, namespace, name string) (bool, error)
}

// mockValkeyClient simulates the Valkey-backed scope cache for spike validation.
type mockValkeyClient struct {
	store map[string]bool
	delay time.Duration
}

func newMockValkeyClient(delay time.Duration) *mockValkeyClient {
	return &mockValkeyClient{
		store: make(map[string]bool),
		delay: delay,
	}
}

func (m *mockValkeyClient) Set(clusterID, gvr, nsName string, managed bool) {
	key := fmt.Sprintf("kubernaut:managed:%s:%s:%s", clusterID, gvr, nsName)
	m.store[key] = managed
}

func (m *mockValkeyClient) IsManaged(_ context.Context, clusterID, group, version, kind, namespace, name string) (bool, error) {
	time.Sleep(m.delay)
	gvr := fmt.Sprintf("%s/%s/%s", group, version, kind)
	nsName := fmt.Sprintf("%s/%s", namespace, name)
	key := fmt.Sprintf("kubernaut:managed:%s:%s:%s", clusterID, gvr, nsName)
	managed, exists := m.store[key]
	if !exists {
		return false, fmt.Errorf("cache miss for key: %s", key)
	}
	return managed, nil
}

var _ ScopeCacheClient = (*mockValkeyClient)(nil)

var _ = Describe("Spike S6 — Fleet Metadata Cache", func() {
	var (
		ctx    context.Context
		client *mockValkeyClient
	)

	BeforeEach(func() {
		ctx = context.Background()
		client = newMockValkeyClient(100 * time.Microsecond)
	})

	Describe("Scope check latency", func() {
		It("S6-001: single scope check completes under 5ms", func() {
			client.Set("prod-east", "apps/v1/Deployment", "default/nginx", true)

			start := time.Now()
			managed, err := client.IsManaged(ctx, "prod-east", "apps", "v1", "Deployment", "default", "nginx")
			elapsed := time.Since(start)

			Expect(err).ToNot(HaveOccurred())
			Expect(managed).To(BeTrue())
			Expect(elapsed).To(BeNumerically("<", 5*time.Millisecond))
		})

		It("S6-002: cache miss returns error (triggers fallback)", func() {
			_, err := client.IsManaged(ctx, "prod-west", "apps", "v1", "Deployment", "default", "redis")
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("cache miss"))
		})

		It("S6-003: unmanaged resource returns false", func() {
			client.Set("prod-east", "/v1/Pod", "kube-system/coredns", false)

			managed, err := client.IsManaged(ctx, "prod-east", "", "v1", "Pod", "kube-system", "coredns")
			Expect(err).ToNot(HaveOccurred())
			Expect(managed).To(BeFalse())
		})

		It("S6-004: batch scope checks complete under 5ms total", func() {
			for i := 0; i < 10; i++ {
				client.Set("cluster-1", "apps/v1/Deployment", fmt.Sprintf("ns-%d/app-%d", i, i), true)
			}

			start := time.Now()
			for i := 0; i < 10; i++ {
				managed, err := client.IsManaged(ctx, "cluster-1", "apps", "v1", "Deployment",
					fmt.Sprintf("ns-%d", i), fmt.Sprintf("app-%d", i))
				Expect(err).ToNot(HaveOccurred())
				Expect(managed).To(BeTrue())
			}
			elapsed := time.Since(start)
			Expect(elapsed).To(BeNumerically("<", 5*time.Millisecond))
		})
	})

	Describe("Key schema", func() {
		It("S6-005: key format matches specification", func() {
			client.Set("prod-east", "apps/v1/Deployment", "default/nginx", true)
			key := "kubernaut:managed:prod-east:apps/v1/Deployment:default/nginx"
			_, exists := client.store[key]
			Expect(exists).To(BeTrue())
		})
	})
})
