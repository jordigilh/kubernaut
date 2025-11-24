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

	goredis "github.com/go-redis/redis/v8"

	"github.com/jordigilh/kubernaut/pkg/gateway/processing"
	"github.com/jordigilh/kubernaut/pkg/gateway/types"
)

// ━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━
// Storm Window Lifecycle Integration Tests - Redis TTL + Window Expiration
// BR-GATEWAY-008: Maximum window duration safety limit (5 minutes)
//
// Test Tier: INTEGRATION (not unit)
// Rationale: Tests real Redis TTL behavior and window expiration logic.
// Window lifecycle requires Redis TTL coordination with business logic,
// which cannot be reliably tested with miniredis (TTL timing differences).
//
// Defense-in-Depth Strategy (per 03-testing-strategy.mdc):
// - Unit tests (70%+): Business logic in isolation (window creation, resource tracking)
// - Integration tests (>50%): Infrastructure interaction (THIS FILE - Redis TTL + expiration)
// - E2E tests (10-15%): Complete workflow (alert storm → window lifecycle → CRD creation)
//
// BUSINESS VALUE:
// - Prevents infinite storm windows that could exhaust Redis memory
// - Ensures timely CRD creation when windows expire
// - Validates multi-tenant isolation (namespace-specific windows)
// ━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━

var _ = Describe("BR-GATEWAY-008: Storm Window Lifecycle (Integration)", func() {
	var (
		ctx         context.Context
		redisClient *goredis.Client
		aggregator  *processing.StormAggregator
	)

	BeforeEach(func() {
		ctx = context.Background()

		// Use real Redis from suite setup (envtest environment)
		// Window lifecycle requires real TTL behavior for expiration testing
		redisTestClient := SetupRedisTestClient(ctx)
		Expect(redisTestClient).ToNot(BeNil(), "Redis test client required for window lifecycle tests")
		Expect(redisTestClient.Client).ToNot(BeNil(), "Redis client required for window lifecycle tests")
		redisClient = redisTestClient.Client

		// Clean Redis state before each test
		err := redisClient.FlushDB(ctx).Err()
		Expect(err).ToNot(HaveOccurred(), "Failed to flush Redis before test")

		// Create aggregator with SHORT maxWindowDuration for testing (10 seconds instead of 5 minutes)
		// This allows tests to complete quickly while validating expiration logic
		aggregator = processing.NewStormAggregatorWithConfig(
			redisClient,
			1,             // bufferThreshold: 1 alert triggers window creation
			60*time.Second, // inactivityTimeout: 1 minute (not tested here)
			10*time.Second, // maxWindowDuration: 10 seconds (SHORT for testing)
			1000,          // defaultMaxSize: 1000 alerts per namespace
			5000,          // globalMaxSize: 5000 alerts total
			nil,           // perNamespaceLimits: none
			0.95,          // samplingThreshold: 95% utilization
			0.5,           // samplingRate: 50% when sampling enabled
		)
	})

	AfterEach(func() {
		// Clean up Redis state after test
		if redisClient != nil {
			_ = redisClient.FlushDB(ctx)
		}
	})

	// ━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━
	// TDD RED PHASE: Window Expiration Tests
	// ━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━
	// STATUS: These tests will FAIL until window expiration logic is implemented
	// EXPECTED: Tests should fail because AddResource doesn't check window age
	// ACTION: Implement window expiration validation in StormAggregator.AddResource
	// ━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━

	Describe("Window Expiration - Maximum Duration Safety", func() {
		Context("when window exceeds maximum duration", func() {
			It("should reject new alert and force window closure", func() {
				// TDD RED: This test will FAIL because expiration logic not implemented
				// BR-GATEWAY-008: Maximum window duration safety limit
				// BUSINESS BEHAVIOR: Old windows must be closed to prevent memory exhaustion
				// OUTCOME: System creates new window instead of extending expired window

				processID := GinkgoParallelProcess()
				namespace := fmt.Sprintf("test-window-expiry-p%d-%d", processID, time.Now().UnixNano())
				alertName := fmt.Sprintf("HighCPU-p%d", processID)

				signal := &types.NormalizedSignal{
					Namespace:   namespace,
					AlertName:   alertName,
					Severity:    "critical",
					Fingerprint: fmt.Sprintf("cpu-high-%s-pod1", namespace),
					Resource: types.ResourceIdentifier{
						Kind: "Pod",
						Name: "api-server-1",
					},
					Labels: map[string]string{
						"alertname": alertName,
						"namespace": namespace,
					},
				}

				// Step 1: Buffer first alert (threshold=1, so should trigger aggregation)
				bufferSize, shouldAggregate, err := aggregator.BufferFirstAlert(ctx, signal)
				Expect(err).ToNot(HaveOccurred(), "Should buffer first alert")
				Expect(shouldAggregate).To(BeTrue(), "Should trigger aggregation (threshold=1)")
				Expect(bufferSize).To(BeNumerically(">=", 1), "Should have at least 1 alert in buffer")

				// Step 2: Start aggregation to get windowID
				stormMetadata := &processing.StormMetadata{
					StormType:  "rate",
					AlertCount: 1,
					Window:     "1m",
				}
				windowID, err := aggregator.StartAggregation(ctx, signal, stormMetadata)
				Expect(err).ToNot(HaveOccurred(), "Should start aggregation")
				Expect(windowID).ToNot(BeEmpty(), "Should return windowID")

				// Step 4: Verify window exists in Redis
				windowKey := fmt.Sprintf("alert:storm:aggregate:%s", alertName)
				windowData, err := redisClient.Get(ctx, windowKey).Result()
				Expect(err).ToNot(HaveOccurred(), "Window should exist in Redis")
				Expect(windowData).To(Equal(windowID), "Window data should match windowID")

				// Step 6: Wait for window to exceed maxWindowDuration (10 seconds)
				GinkgoWriter.Printf("⏳ Waiting 11 seconds for window to expire (maxWindowDuration: 10s)...\n")
				time.Sleep(11 * time.Second)

				// Step 7: Try to add resource to expired window
				signal.Resource.Name = "api-server-2" // Different resource
				err = aggregator.AddResource(ctx, windowID, signal)

				// BUSINESS VALIDATION: System rejects expired window
				// ✅ Error returned (window expired)
				// ✅ Error message indicates expiration/duration issue
				// ✅ Forces caller to create new window
				Expect(err).To(HaveOccurred(), "Should reject alert for expired window")
				Expect(err.Error()).To(Or(
					ContainSubstring("expired"),
					ContainSubstring("duration"),
					ContainSubstring("max"),
					ContainSubstring("window"),
				), "Error should indicate window expiration")

				GinkgoWriter.Printf("✅ Window expiration validated: %v\n", err)
			})

			It("should allow new window creation after old window expires", func() {
				// TDD RED: This test will FAIL because new window creation after expiry not tested
				// BR-GATEWAY-008: Window lifecycle management
				// BUSINESS BEHAVIOR: System should create fresh window after expiration
				// OUTCOME: New window with new windowID, old window data cleaned up

				processID := GinkgoParallelProcess()
				namespace := fmt.Sprintf("test-new-window-p%d-%d", processID, time.Now().UnixNano())
				alertName := fmt.Sprintf("HighMemory-p%d", processID)

				signal := &types.NormalizedSignal{
					Namespace:   namespace,
					AlertName:   alertName,
					Severity:    "critical",
					Fingerprint: fmt.Sprintf("mem-high-%s-pod1", namespace),
					Resource: types.ResourceIdentifier{
						Kind: "Pod",
						Name: "db-server-1",
					},
					Labels: map[string]string{
						"alertname": alertName,
						"namespace": namespace,
					},
				}

				// Step 1: Buffer and start aggregation for initial window
				_, shouldAggregate1, err := aggregator.BufferFirstAlert(ctx, signal)
				Expect(err).ToNot(HaveOccurred(), "Should buffer first alert")
				Expect(shouldAggregate1).To(BeTrue(), "Should trigger aggregation")

				stormMetadata1 := &processing.StormMetadata{
					StormType:  "rate",
					AlertCount: 1,
					Window:     "1m",
				}
				windowID1, err := aggregator.StartAggregation(ctx, signal, stormMetadata1)
				Expect(err).ToNot(HaveOccurred(), "Should start aggregation")
				Expect(windowID1).ToNot(BeEmpty(), "Should return windowID")

				GinkgoWriter.Printf("📦 Created initial window: %s\n", windowID1)

				// Step 2: Wait for window to expire
				GinkgoWriter.Printf("⏳ Waiting 11 seconds for window to expire...\n")
				time.Sleep(11 * time.Second)

				// Step 3: Create new window after expiration
				signal.Resource.Name = "db-server-2" // Different resource
				_, shouldAggregate2, err := aggregator.BufferFirstAlert(ctx, signal)
				Expect(err).ToNot(HaveOccurred(), "Should buffer alert after expiration")
				Expect(shouldAggregate2).To(BeTrue(), "Should trigger aggregation")

				stormMetadata2 := &processing.StormMetadata{
					StormType:  "rate",
					AlertCount: 1,
					Window:     "1m",
				}
				windowID2, err := aggregator.StartAggregation(ctx, signal, stormMetadata2)

				// BUSINESS VALIDATION: System creates new window after expiration
				// ✅ No error (window creation succeeds)
				// ✅ New window created (not reusing expired window)
				// ✅ Different windowID (proves new window)
				Expect(err).ToNot(HaveOccurred(), "Should create new window after expiration")
				Expect(windowID2).ToNot(BeEmpty(), "Should return new windowID")
				Expect(windowID2).ToNot(Equal(windowID1), "Should create different window (not reuse expired)")

				GinkgoWriter.Printf("✅ New window created after expiration: %s (old: %s)\n", windowID2, windowID1)
			})
		})

		Context("when window is within maximum duration", func() {
			It("should accept new alerts and extend window", func() {
				// TDD RED: This test validates happy path (should PASS even without expiration logic)
				// BR-GATEWAY-008: Normal window operation within duration limit
				// BUSINESS BEHAVIOR: Active windows should accept new resources
				// OUTCOME: Window grows with additional resources

				processID := GinkgoParallelProcess()
				namespace := fmt.Sprintf("test-active-window-p%d-%d", processID, time.Now().UnixNano())
				alertName := fmt.Sprintf("DiskFull-p%d", processID)

				signal := &types.NormalizedSignal{
					Namespace:   namespace,
					AlertName:   alertName,
					Severity:    "critical",
					Fingerprint: fmt.Sprintf("disk-full-%s-pod1", namespace),
					Resource: types.ResourceIdentifier{
						Kind: "Pod",
						Name: "storage-1",
					},
					Labels: map[string]string{
						"alertname": alertName,
						"namespace": namespace,
					},
				}

				// Step 1: Buffer and start aggregation
				_, shouldAggregate, err := aggregator.BufferFirstAlert(ctx, signal)
				Expect(err).ToNot(HaveOccurred(), "Should buffer first alert")
				Expect(shouldAggregate).To(BeTrue(), "Should trigger aggregation")

				stormMetadata := &processing.StormMetadata{
					StormType:  "rate",
					AlertCount: 1,
					Window:     "1m",
				}
				windowID, err := aggregator.StartAggregation(ctx, signal, stormMetadata)
				Expect(err).ToNot(HaveOccurred(), "Should start aggregation")
				Expect(windowID).ToNot(BeEmpty(), "Should return windowID")

				// Step 2: Add resource within duration limit (immediately)
				signal.Resource.Name = "storage-2" // Different resource
				err = aggregator.AddResource(ctx, windowID, signal)

				// BUSINESS VALIDATION: Active window accepts new resources
				// ✅ No error (resource added successfully)
				// ✅ Window still active (within duration limit)
				Expect(err).ToNot(HaveOccurred(), "Should accept resource for active window")

				// Step 3: Verify window has 2 resources
				resources, err := aggregator.GetAggregatedResources(ctx, windowID)
				Expect(err).ToNot(HaveOccurred(), "Should retrieve resources")
				Expect(resources).To(HaveLen(2), "Window should have 2 resources")

				GinkgoWriter.Printf("✅ Active window accepted new resource: %d resources total\n", len(resources))
			})
		})
	})

	// ━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━
	// Window TTL and Redis Expiration
	// ━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━

	Describe("Window TTL Management", func() {
		Context("when window is created", func() {
			It("should set Redis TTL matching inactivityTimeout (windowDuration)", func() {
				// BR-GATEWAY-008: Window TTL management (sliding window with inactivity timeout)
				// BUSINESS BEHAVIOR: Redis keys should expire after inactivityTimeout
				// OUTCOME: Windows close after inactivity, maxWindowDuration enforced via IsWindowExpired

				processID := GinkgoParallelProcess()
				namespace := fmt.Sprintf("test-ttl-p%d-%d", processID, time.Now().UnixNano())
				alertName := fmt.Sprintf("NetworkError-p%d", processID)

				signal := &types.NormalizedSignal{
					Namespace:   namespace,
					AlertName:   alertName,
					Severity:    "critical",
					Fingerprint: fmt.Sprintf("net-err-%s-pod1", namespace),
					Resource: types.ResourceIdentifier{
						Kind: "Pod",
						Name: "frontend-1",
					},
					Labels: map[string]string{
						"alertname": alertName,
						"namespace": namespace,
					},
				}

				// Step 1: Buffer and start aggregation
				_, shouldAggregate, err := aggregator.BufferFirstAlert(ctx, signal)
				Expect(err).ToNot(HaveOccurred(), "Should buffer first alert")
				Expect(shouldAggregate).To(BeTrue(), "Should trigger aggregation")

				stormMetadata := &processing.StormMetadata{
					StormType:  "rate",
					AlertCount: 1,
					Window:     "1m",
				}
				windowID, err := aggregator.StartAggregation(ctx, signal, stormMetadata)
				Expect(err).ToNot(HaveOccurred(), "Should start aggregation")

				// Step 2: Check Redis TTL on window key
				windowKey := fmt.Sprintf("alert:storm:aggregate:%s", alertName)
				ttl, err := redisClient.TTL(ctx, windowKey).Result()
				Expect(err).ToNot(HaveOccurred(), "Should get TTL")

				// BUSINESS VALIDATION: TTL matches inactivityTimeout/windowDuration (60 seconds in test)
				// ✅ TTL is set (not -1 which means no expiration)
				// ✅ TTL is approximately windowDuration (within 5 seconds tolerance)
				// Note: maxWindowDuration (10s) is enforced via IsWindowExpired check in AddResource
				Expect(ttl).To(BeNumerically(">", 0), "TTL should be set")
				Expect(ttl).To(BeNumerically("<=", 60*time.Second), "TTL should not exceed windowDuration")
				Expect(ttl).To(BeNumerically(">=", 55*time.Second), "TTL should be close to windowDuration")

				GinkgoWriter.Printf("✅ Window TTL validated: %v (windowID: %s)\n", ttl, windowID)
			})
		})
	})
})

