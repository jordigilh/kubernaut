/*
Copyright 2025 Jordi Gil.

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

package gateway

import (
	"context"
	"fmt"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/jordigilh/kubernaut/pkg/gateway/adapters"
	"github.com/jordigilh/kubernaut/pkg/gateway/types"
)

// mockOwnerResolver implements adapters.OwnerResolver for testing
type mockOwnerResolver struct {
	// resolveFunc allows per-test customization of resolve behavior
	resolveFunc func(ctx context.Context, namespace, kind, name string) (string, string, error)
}

func (m *mockOwnerResolver) ResolveTopLevelOwner(ctx context.Context, namespace, kind, name string) (string, string, error) {
	if m.resolveFunc != nil {
		return m.resolveFunc(ctx, namespace, kind, name)
	}
	return "", "", fmt.Errorf("no resolve function configured")
}

// newK8sEventJSON creates a K8s event JSON payload for testing
func newK8sEventJSON(reason, eventType, podName, namespace string) []byte {
	now := time.Now().Format(time.RFC3339)
	return []byte(fmt.Sprintf(`{
		"type": "%s",
		"reason": "%s",
		"message": "test event",
		"involvedObject": {
			"kind": "Pod",
			"name": "%s",
			"namespace": "%s",
			"uid": "pod-uid-123",
			"apiVersion": "v1"
		},
		"firstTimestamp": "%s",
		"lastTimestamp": "%s",
		"source": {
			"component": "kubelet",
			"host": "node-1"
		}
	}`, eventType, reason, podName, namespace, now, now))
}

var _ = Describe("K8s Event Deduplication with Owner Chain Resolution", func() {
	var ctx context.Context

	BeforeEach(func() {
		ctx = context.Background()
	})

	// Test 1: Same pod, different reasons → same fingerprint (owner-based)
	Describe("Same pod with different event reasons", func() {
		It("should produce the same fingerprint when OwnerResolver is configured", func() {
			// Mock: Pod "payment-api-789" is owned by Deployment "payment-api"
			resolver := &mockOwnerResolver{
				resolveFunc: func(ctx context.Context, namespace, kind, name string) (string, string, error) {
					return "Deployment", "payment-api", nil
				},
			}
			adapter := adapters.NewKubernetesEventAdapter(resolver)

			// Event 1: BackOff reason
			event1 := newK8sEventJSON("BackOff", "Warning", "payment-api-789", "prod")
			signal1, err := adapter.Parse(ctx, event1)
			Expect(err).ToNot(HaveOccurred())

			// Event 2: OOMKilling reason (same pod, different reason)
			event2 := newK8sEventJSON("OOMKilling", "Warning", "payment-api-789", "prod")
			signal2, err := adapter.Parse(ctx, event2)
			Expect(err).ToNot(HaveOccurred())

			// Both should produce the same fingerprint (owner-based, reason excluded)
			Expect(signal1.Fingerprint).To(Equal(signal2.Fingerprint),
				"Events from the same pod with different reasons should have the same fingerprint (owner-based dedup)")

			// Verify the fingerprint matches expected owner-based calculation
			expectedFingerprint := types.CalculateOwnerFingerprint(types.ResourceIdentifier{
				Namespace: "prod",
				Kind:      "Deployment",
				Name:      "payment-api",
			})
			Expect(signal1.Fingerprint).To(Equal(expectedFingerprint),
				"Fingerprint should be based on the owner (Deployment), not the pod")
		})
	})

	// Test 2: Different pods owned by same Deployment → same fingerprint
	Describe("Different pods from the same Deployment", func() {
		It("should produce the same fingerprint for pods owned by the same Deployment", func() {
			// Mock: Both pods are owned by Deployment "payment-api"
			resolver := &mockOwnerResolver{
				resolveFunc: func(ctx context.Context, namespace, kind, name string) (string, string, error) {
					return "Deployment", "payment-api", nil
				},
			}
			adapter := adapters.NewKubernetesEventAdapter(resolver)

			// Event from first pod
			event1 := newK8sEventJSON("BackOff", "Warning", "payment-api-789abc", "prod")
			signal1, err := adapter.Parse(ctx, event1)
			Expect(err).ToNot(HaveOccurred())

			// Event from second pod (recreated by ReplicaSet)
			event2 := newK8sEventJSON("BackOff", "Warning", "payment-api-def456", "prod")
			signal2, err := adapter.Parse(ctx, event2)
			Expect(err).ToNot(HaveOccurred())

			// Both should produce the same fingerprint (same owner)
			Expect(signal1.Fingerprint).To(Equal(signal2.Fingerprint),
				"Events from different pods of the same Deployment should have the same fingerprint")
		})
	})

	// Test 3: Fallback to involvedObject when owner resolution fails
	Describe("Fallback behavior on owner resolution failure", func() {
		It("should fall back to involvedObject-based fingerprint when resolution fails", func() {
			// Mock: Owner resolution fails (RBAC error)
			resolver := &mockOwnerResolver{
				resolveFunc: func(ctx context.Context, namespace, kind, name string) (string, string, error) {
					return "", "", fmt.Errorf("RBAC: forbidden")
				},
			}
			adapter := adapters.NewKubernetesEventAdapter(resolver)

			event := newK8sEventJSON("BackOff", "Warning", "payment-api-789", "prod")
			signal, err := adapter.Parse(ctx, event)
			Expect(err).ToNot(HaveOccurred())

			// Should fall back to involvedObject-based fingerprint (without reason)
			// Even on fallback, reason is excluded from fingerprint
			expectedFingerprint := types.CalculateOwnerFingerprint(types.ResourceIdentifier{
				Namespace: "prod",
				Kind:      "Pod",
				Name:      "payment-api-789",
			})
			Expect(signal.Fingerprint).To(Equal(expectedFingerprint),
				"On owner resolution failure, should fall back to involvedObject-based fingerprint (reason excluded)")
		})
	})

	// Test 4: No OwnerResolver (nil) → reason excluded from fingerprint (Issue #227)
	Describe("Default behavior without OwnerResolver", func() {
		It("should exclude reason from fingerprint when no OwnerResolver is configured", func() {
			adapter := adapters.NewKubernetesEventAdapter()

			event := newK8sEventJSON("BackOff", "Warning", "payment-api-789", "prod")
			signal, err := adapter.Parse(ctx, event)
			Expect(err).ToNot(HaveOccurred())

			// Issue #227: Reason must be excluded from fingerprint even without OwnerResolver,
			// matching Prometheus adapter behavior for cross-adapter deduplication consistency
			expectedFingerprint := types.CalculateOwnerFingerprint(types.ResourceIdentifier{
				Namespace: "prod",
				Kind:      "Pod",
				Name:      "payment-api-789",
			})
			Expect(signal.Fingerprint).To(Equal(expectedFingerprint),
				"Without OwnerResolver, should use resource-level fingerprint (reason excluded, matching Prometheus adapter)")
		})
	})
})
