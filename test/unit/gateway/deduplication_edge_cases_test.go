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
	"sync"
	"time"

	"github.com/alicebob/miniredis/v2"
	goredis "github.com/go-redis/redis/v8"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"go.uber.org/zap"

	"github.com/jordigilh/kubernaut/pkg/gateway/metrics"
	"github.com/jordigilh/kubernaut/pkg/gateway/processing"
	"github.com/jordigilh/kubernaut/pkg/gateway/types"
	"github.com/prometheus/client_golang/prometheus"
)

// Edge Case Testing: Test boundary conditions and error scenarios
// These tests complement the main deduplication tests with edge cases

var _ = Describe("BR-GATEWAY-003: Deduplication Edge Cases", func() {
	var (
		ctx          context.Context
		dedupService *processing.DeduplicationService
		redisServer  *miniredis.Miniredis
		redisClient  *goredis.Client
		logger       *zap.Logger
	)

	BeforeEach(func() {
		ctx = context.Background()
		logger = zap.NewNop()

		var err error
		redisServer, err = miniredis.Run()
		Expect(err).NotTo(HaveOccurred())

		redisClient = goredis.NewClient(&goredis.Options{
			Addr: redisServer.Addr(),
		})

		// Create metrics instance for testing (required for Store operations)
		registry := prometheus.NewRegistry()
		metricsInstance := metrics.NewMetricsWithRegistry(registry)

		// k8sClient=nil (unit tests don't need K8s client)
		dedupService = processing.NewDeduplicationService(redisClient, nil, logger, metricsInstance)
	})

	AfterEach(func() {
		_ = redisClient.Close()
		redisServer.Close()
	})

	Describe("Edge Case 1: Fingerprint Collision Handling", func() {
		It("should handle different alerts with same fingerprint correctly", func() {
			// BR-GATEWAY-003: Fingerprint collision edge case
			// BUSINESS SCENARIO: Two different alerts generate same fingerprint (hash collision)
			// Expected: Both treated as duplicates (by design - fingerprint is the identity)

			fingerprint := "collision_fingerprint_123"

			signal1 := &types.NormalizedSignal{
				AlertName:   "HighCPU",
				Namespace:   "production",
				Resource:    types.ResourceIdentifier{Kind: "Pod", Name: "api-1"},
				Severity:    "critical",
				Fingerprint: fingerprint,
			}

			signal2 := &types.NormalizedSignal{
				AlertName:   "HighMemory", // Different alert
				Namespace:   "production",
				Resource:    types.ResourceIdentifier{Kind: "Pod", Name: "api-2"}, // Different pod
				Severity:    "warning",                                            // Different severity
				Fingerprint: fingerprint,                                          // Same fingerprint
			}

			// First signal is not duplicate
			isDup1, _, err := dedupService.Check(ctx, signal1)
			Expect(err).NotTo(HaveOccurred())
			Expect(isDup1).To(BeFalse(), "First signal should not be duplicate")

			// Store first signal
			err = dedupService.Store(ctx, signal1, "crd-1")
			Expect(err).NotTo(HaveOccurred())

			// Second signal with same fingerprint IS duplicate (by design)
			isDup2, metadata, err := dedupService.Check(ctx, signal2)
			Expect(err).NotTo(HaveOccurred())
			Expect(isDup2).To(BeTrue(), "Same fingerprint means duplicate, regardless of content")
			Expect(metadata).NotTo(BeNil(), "Metadata should exist for duplicates")
			Expect(metadata.Count).To(Equal(2), "Duplicate count should increment")
			Expect(metadata.RemediationRequestRef).To(Equal("crd-1"), "Should reference same CRD")
		})
	})

	Describe("Edge Case 2: TTL Expiration Race Condition", func() {
		It("should handle TTL expiration during processing", func() {
			// BR-GATEWAY-003: TTL expiration race condition
			// BUSINESS SCENARIO: Fingerprint expires between Check() and Store() calls
			// Expected: Graceful handling, no data corruption

			signal := &types.NormalizedSignal{
				AlertName:   "HighCPU",
				Namespace:   "production",
				Resource:    types.ResourceIdentifier{Kind: "Pod", Name: "api-1"},
				Severity:    "critical",
				Fingerprint: "ttl_race_fingerprint",
			}

			// Create service with very short TTL (1 second)
			registry := prometheus.NewRegistry()
			metricsInstance := metrics.NewMetricsWithRegistry(registry)
			dedupServiceShortTTL := processing.NewDeduplicationServiceWithTTL(
				redisClient,
				nil,           // k8sClient=nil (unit tests don't need K8s)
				1*time.Second, // 1 second TTL
				logger,
				metricsInstance,
			)

			// Check (not duplicate)
			isDup, _, err := dedupServiceShortTTL.Check(ctx, signal)
			Expect(err).NotTo(HaveOccurred())
			Expect(isDup).To(BeFalse())

			// Store
			err = dedupServiceShortTTL.Store(ctx, signal, "crd-1")
			Expect(err).NotTo(HaveOccurred())

			// Fast-forward time in miniredis (no real sleep needed)
			redisServer.FastForward(1100 * time.Millisecond)

			// Check again (should not be duplicate after TTL expiration)
			isDup2, _, err := dedupServiceShortTTL.Check(ctx, signal)
			Expect(err).NotTo(HaveOccurred())
			Expect(isDup2).To(BeFalse(), "After TTL expiration, should not be duplicate")

			// Store again (should succeed)
			err = dedupServiceShortTTL.Store(ctx, signal, "crd-2")
			Expect(err).NotTo(HaveOccurred())
		})
	})

	Describe("Edge Case 3: Redis Connection Loss Mid-Deduplication", func() {
		It("should gracefully degrade during Check operation when Redis unavailable", func() {
			// BR-GATEWAY-003: Request rejection when Redis unavailable (DD-GATEWAY-003)
			// BUSINESS SCENARIO: Redis becomes unavailable during deduplication check
			// Expected: Return error (Gateway returns HTTP 503 to client)

			signal := &types.NormalizedSignal{
				AlertName:   "HighCPU",
				Namespace:   "production",
				Resource:    types.ResourceIdentifier{Kind: "Pod", Name: "api-1"},
				Severity:    "critical",
				Fingerprint: "redis_disconnect_fingerprint",
			}

			// Close Redis server to simulate connection loss
			redisServer.Close()

			// Check should return error (Gateway returns HTTP 503)
			// BUSINESS LOGIC: Reject request when Redis unavailable
			isDup, metadata, err := dedupService.Check(ctx, signal)
			Expect(err).To(HaveOccurred(), "Redis unavailable should return error (HTTP 503)")
			Expect(isDup).To(BeFalse(), "Should return false when error occurs")
			Expect(metadata).To(BeNil(), "Should return nil metadata when error occurs")
		})

		It("should gracefully degrade during Store operation when Redis unavailable", func() {
			// BR-GATEWAY-003 + BR-GATEWAY-013: Graceful degradation during store
			// BUSINESS SCENARIO: Redis becomes unavailable during metadata storage
			// Expected: Graceful degradation - log warning, allow processing to continue

			signal := &types.NormalizedSignal{
				AlertName:   "HighCPU",
				Namespace:   "production",
				Resource:    types.ResourceIdentifier{Kind: "Pod", Name: "api-1"},
				Severity:    "critical",
				Fingerprint: "redis_disconnect_store_fingerprint",
			}

			// Close Redis server
			redisServer.Close()

			// Store should gracefully degrade (not panic)
			// BUSINESS LOGIC: Log warning but don't fail (CRD already created)
			err := dedupService.Store(ctx, signal, "crd-1")
			Expect(err).NotTo(HaveOccurred(), "Graceful degradation: should not error")
			// Note: Future duplicates won't be detected, but that's acceptable trade-off
		})
	})

	Describe("Edge Case 4: Concurrent Deduplication of Same Fingerprint", func() {
		It("should handle concurrent Check calls for same fingerprint", func() {
			// BR-GATEWAY-003: Concurrent deduplication edge case
			// BUSINESS SCENARIO: Multiple goroutines check same fingerprint simultaneously
			// Expected: All get consistent results, no race conditions

			fingerprint := "concurrent_fingerprint"
			signal := &types.NormalizedSignal{
				AlertName:   "HighCPU",
				Namespace:   "production",
				Resource:    types.ResourceIdentifier{Kind: "Pod", Name: "api-1"},
				Severity:    "critical",
				Fingerprint: fingerprint,
			}

			// Store initial fingerprint
			err := dedupService.Store(ctx, signal, "crd-1")
			Expect(err).NotTo(HaveOccurred())

			// Concurrent checks
			const numGoroutines = 10
			var wg sync.WaitGroup
			results := make([]bool, numGoroutines)
			errors := make([]error, numGoroutines)

			for i := 0; i < numGoroutines; i++ {
				wg.Add(1)
				go func(index int) {
					defer wg.Done()
					isDup, _, err := dedupService.Check(ctx, signal)
					results[index] = isDup
					errors[index] = err
				}(i)
			}

			wg.Wait()

			// All checks should succeed
			for i, err := range errors {
				Expect(err).NotTo(HaveOccurred(), "Check %d should not error", i)
			}

			// All checks should return true (duplicate)
			for i, isDup := range results {
				Expect(isDup).To(BeTrue(), "Check %d should detect duplicate", i)
			}
		})

		It("should handle concurrent Store calls for same fingerprint", func() {
			// BR-GATEWAY-003: Concurrent store edge case
			// BUSINESS SCENARIO: Multiple goroutines try to store same fingerprint
			// Expected: All succeed (last write wins), no data corruption

			fingerprint := "concurrent_store_fingerprint"
			signal := &types.NormalizedSignal{
				AlertName:   "HighCPU",
				Namespace:   "production",
				Resource:    types.ResourceIdentifier{Kind: "Pod", Name: "api-1"},
				Severity:    "critical",
				Fingerprint: fingerprint,
			}

			// Concurrent stores
			const numGoroutines = 10
			var wg sync.WaitGroup
			errors := make([]error, numGoroutines)

			for i := 0; i < numGoroutines; i++ {
				wg.Add(1)
				go func(index int) {
					defer wg.Done()
					crdName := "crd-" + string(rune('0'+index))
					errors[index] = dedupService.Store(ctx, signal, crdName)
				}(i)
			}

			wg.Wait()

			// All stores should succeed (last write wins in Redis)
			for i, err := range errors {
				Expect(err).NotTo(HaveOccurred(), "Store %d should not error", i)
			}

			// Verify data is consistent (not corrupted)
			isDup, metadata, err := dedupService.Check(ctx, signal)
			Expect(err).NotTo(HaveOccurred())
			Expect(isDup).To(BeTrue(), "Should be duplicate after stores")
			Expect(metadata).NotTo(BeNil(), "Metadata should exist for duplicates")
			Expect(metadata.RemediationRequestRef).NotTo(BeEmpty(), "Should have CRD reference")
		})
	})
})
