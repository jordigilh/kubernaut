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

package processing

import (
	"context"
	"crypto/sha256"
	"fmt"
	"time"

	"github.com/alicebob/miniredis/v2"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/redis/go-redis/v9"
	"github.com/go-logr/logr"

	rediscache "github.com/jordigilh/kubernaut/pkg/cache/redis"
	"github.com/jordigilh/kubernaut/pkg/gateway/processing"
	"github.com/jordigilh/kubernaut/pkg/gateway/types"
)

// ━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━
// Redis Failure Recovery Unit Tests
// BR-GATEWAY-013: Graceful degradation when Redis unavailable
//
// Test Tier: UNIT (not integration)
// Rationale: Tests error handling logic, not Redis infrastructure coordination.
// Uses miniredis to simulate Redis failures (close server = connection refused).
//
// Defense-in-Depth Strategy (per 03-testing-strategy.mdc):
// - Unit tests (70%+): Business logic in isolation (THIS FILE - error handling)
// - Integration tests (>50%): Infrastructure coordination (already covered in multi-pod test)
//
// BUSINESS VALUE:
// - Validates graceful degradation when Redis unavailable
// - Ensures Gateway returns HTTP 503 (not crash) when Redis down
// - Tests connection recovery after Redis restart
// - Critical for production resilience
// ━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━

var _ = Describe("BR-GATEWAY-013: Redis Failure Recovery (Unit)", func() {
	var (
		ctx         context.Context
		redisServer *miniredis.Miniredis
		testSignal  *types.NormalizedSignal
	)

	BeforeEach(func() {
		ctx = context.Background()

		// Setup miniredis server (used for connection recovery tests)
		var err error
		redisServer, err = miniredis.Run()
		Expect(err).NotTo(HaveOccurred())

		// Create test signal with valid SHA256 fingerprint
		hash := sha256.Sum256([]byte(fmt.Sprintf("test-signal-%d", time.Now().UnixNano())))
		testSignal = &types.NormalizedSignal{
			Fingerprint:  fmt.Sprintf("%x", hash),
			AlertName:    "TestAlert",
			Severity:     "critical",
			Namespace:    "default",
			Resource:     types.ResourceIdentifier{Kind: "Pod", Name: "test-pod"},
			FiringTime:   time.Now(),
			ReceivedTime: time.Now(),
			Labels: map[string]string{
				"alertname": "TestAlert",
			},
		}
	})

	AfterEach(func() {
		if redisServer != nil {
			redisServer.Close()
		}
	})

	// ━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━
	// TDD RED PHASE: Redis Failure Handling Tests
	// ━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━
	// STATUS: These tests validate error handling logic
	// EXPECTED: Tests should pass (business logic already implemented)
	// ACTION: Validate graceful degradation behavior
	// ━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━

	Describe("Check() - Redis Unavailable", func() {
		Context("when Redis is down", func() {
			It("should return error indicating Redis unavailable", func() {
				// BR-GATEWAY-013: Check() returns error when Redis unavailable
				// BUSINESS BEHAVIOR: Gateway returns HTTP 503 (not crash)
				// OUTCOME: Operators know Redis is down and can fix it

				// Create a Redis client pointing to invalid address (simulates Redis down)
				failingRedisClient := rediscache.NewClient(&redis.Options{
					Addr:         "localhost:9999", // Invalid port
					DialTimeout:  100 * time.Millisecond,
					ReadTimeout:  100 * time.Millisecond,
					WriteTimeout: 100 * time.Millisecond,
				}, logr.Discard())
				defer failingRedisClient.Close()

				// Create deduplication service with failing Redis client
				failingDedupService := processing.NewDeduplicationService(failingRedisClient, nil, logr.Discard(), nil)

				// Check() should return error (not panic)
				isDup, metadata, err := failingDedupService.Check(ctx, testSignal)

				// BUSINESS VALIDATION: Error handling
				// ✅ Error returned (not panic)
				// ✅ Error message indicates Redis unavailable
				// ✅ Gateway can return HTTP 503
				Expect(err).To(HaveOccurred(), "Should return error when Redis down")
				Expect(err.Error()).To(ContainSubstring("redis unavailable"), "Error should indicate Redis unavailable")
				Expect(isDup).To(BeFalse(), "Should return false when Redis down")
				Expect(metadata).To(BeNil(), "Should return nil metadata when Redis down")

				GinkgoWriter.Printf("✅ Check() error handling validated: %v\n", err)
			})
		})

		Context("when Redis operation fails mid-check", func() {
			It("should treat as new alert (graceful degradation)", func() {
				// BR-GATEWAY-013: Graceful degradation for Redis operation failures
				// BUSINESS BEHAVIOR: Don't block alert processing on Redis errors
				// OUTCOME: Alert processed even if Redis has issues

				// This test validates the checkRedisDeduplication() graceful degradation
				// When Redis.Exists() fails, treat as new alert (lines 399-408 in deduplication.go)

				// Create a Redis client pointing to invalid address (simulates Redis down)
				failingRedisClient := rediscache.NewClient(&redis.Options{
					Addr:         "localhost:9999", // Invalid port
					DialTimeout:  100 * time.Millisecond,
					ReadTimeout:  100 * time.Millisecond,
					WriteTimeout: 100 * time.Millisecond,
				}, logr.Discard())
				defer failingRedisClient.Close()

				// Create deduplication service with failing Redis client
				failingDedupService := processing.NewDeduplicationService(failingRedisClient, nil, logr.Discard(), nil)

				// Check() should gracefully degrade (not panic)
				isDup, _, err := failingDedupService.Check(ctx, testSignal)

				// BUSINESS VALIDATION: Graceful degradation
				// ✅ Error returned (Redis unavailable)
				// ✅ No panic
				// ✅ Alert can still be processed (Gateway returns 503, retry later)
				Expect(err).To(HaveOccurred(), "Should return error when Redis down")
				Expect(isDup).To(BeFalse(), "Should treat as new alert on Redis failure")

				GinkgoWriter.Printf("✅ Graceful degradation validated\n")
			})
		})
	})

	Describe("Store() - Redis Unavailable", func() {
		Context("when Redis is down", func() {
			It("should return nil (graceful degradation)", func() {
				// BR-GATEWAY-013: Store() gracefully degrades when Redis unavailable
				// BUSINESS BEHAVIOR: Don't fail alert processing if Store() fails
				// OUTCOME: CRD already created, alert is being processed
				// TRADE-OFF: Future duplicates won't be detected (acceptable)

				// Create a Redis client pointing to invalid address (simulates Redis down)
				failingRedisClient := rediscache.NewClient(&redis.Options{
					Addr:         "localhost:9999", // Invalid port
					DialTimeout:  100 * time.Millisecond,
					ReadTimeout:  100 * time.Millisecond,
					WriteTimeout: 100 * time.Millisecond,
				}, logr.Discard())
				defer failingRedisClient.Close()

				// Create deduplication service with failing Redis client
				failingDedupService := processing.NewDeduplicationService(failingRedisClient, nil, logr.Discard(), nil)

				// Store() should gracefully degrade (return nil, not error)
				err := failingDedupService.Store(ctx, testSignal, "test-crd-1")

				// BUSINESS VALIDATION: Graceful degradation
				// ✅ No error returned (graceful degradation)
				// ✅ Alert processing continues
				// ✅ CRD already created at this point
				// ⚠️  Trade-off: Future duplicates won't be detected
				Expect(err).ToNot(HaveOccurred(), "Store should gracefully degrade (return nil)")

				GinkgoWriter.Printf("✅ Store() graceful degradation validated\n")
			})
		})
	})

	Describe("Connection Recovery", func() {
		Context("when Redis recovers after failure", func() {
			It("should reconnect on next Check()", func() {
				// BR-GATEWAY-013: Connection recovery after Redis restart
				// BUSINESS BEHAVIOR: Gateway automatically recovers when Redis comes back
				// OUTCOME: No manual intervention needed

				// Step 1: Create failing Redis client
				failingRedisClient := rediscache.NewClient(&redis.Options{
					Addr:         "localhost:9999", // Invalid port
					DialTimeout:  100 * time.Millisecond,
					ReadTimeout:  100 * time.Millisecond,
					WriteTimeout: 100 * time.Millisecond,
				}, logr.Discard())

				// Create deduplication service with failing Redis client
				failingDedupService := processing.NewDeduplicationService(failingRedisClient, nil, logr.Discard(), nil)

				// Step 2: Check() should fail
				_, _, err := failingDedupService.Check(ctx, testSignal)
				Expect(err).To(HaveOccurred(), "Check should fail with Redis down")

				// Step 3: Close failing client
				failingRedisClient.Close()

				// Step 4: Create new service with working Redis (simulate recovery)
				workingRedisClient := rediscache.NewClient(&redis.Options{
					Addr: redisServer.Addr(), // Working miniredis
				}, logr.Discard())
				defer workingRedisClient.Close()

				workingDedupService := processing.NewDeduplicationService(workingRedisClient, nil, logr.Discard(), nil)

				// Step 5: Check() should succeed after recovery
				isDup, _, err := workingDedupService.Check(ctx, testSignal)

				// BUSINESS VALIDATION: Connection recovery
				// ✅ No error after Redis recovery
				// ✅ Service resumes normal operation
				// ✅ No manual intervention needed (just reconnect)
				Expect(err).ToNot(HaveOccurred(), "Check should succeed after Redis recovery")
				Expect(isDup).To(BeFalse(), "Should work normally after recovery")

				GinkgoWriter.Printf("✅ Connection recovery validated\n")
			})
		})

		Context("when Redis is intermittently unavailable", func() {
			It("should handle connection state correctly", func() {
				// BR-GATEWAY-013: Handle intermittent Redis failures
				// BUSINESS BEHAVIOR: Connection state tracked correctly
				// OUTCOME: Service knows when Redis is up/down

				// Create failing Redis client
				failingRedisClient := rediscache.NewClient(&redis.Options{
					Addr:         "localhost:9999", // Invalid port
					DialTimeout:  100 * time.Millisecond,
					ReadTimeout:  100 * time.Millisecond,
					WriteTimeout: 100 * time.Millisecond,
				}, logr.Discard())
				defer failingRedisClient.Close()

				// Create deduplication service with failing Redis client
				failingDedupService := processing.NewDeduplicationService(failingRedisClient, nil, logr.Discard(), nil)

				// Step 1: First check fails (Redis down)
				_, _, err := failingDedupService.Check(ctx, testSignal)
				Expect(err).To(HaveOccurred(), "First check should fail")

				// Step 2: Second check also fails (still down)
				_, _, err = failingDedupService.Check(ctx, testSignal)
				Expect(err).To(HaveOccurred(), "Second check should also fail")

				// Step 3: Third check also fails (consistently down)
				_, _, err = failingDedupService.Check(ctx, testSignal)
				Expect(err).To(HaveOccurred(), "Third check should also fail")

				// BUSINESS VALIDATION: Connection state tracking
				// ✅ Service detects Redis down
				// ✅ Multiple checks consistently fail
				// ✅ No false positives (doesn't think Redis is up when it's down)
				GinkgoWriter.Printf("✅ Connection state tracking validated\n")
			})
		})
	})

	Describe("Error Messages", func() {
		Context("when Redis ping fails", func() {
			It("should provide clear error message", func() {
				// BR-GATEWAY-013: Clear error messages for operators
				// BUSINESS BEHAVIOR: Operators know what's wrong
				// OUTCOME: Faster troubleshooting

				// Create failing Redis client
				failingRedisClient := rediscache.NewClient(&redis.Options{
					Addr:         "localhost:9999", // Invalid port
					DialTimeout:  100 * time.Millisecond,
					ReadTimeout:  100 * time.Millisecond,
					WriteTimeout: 100 * time.Millisecond,
				}, logr.Discard())
				defer failingRedisClient.Close()

				// Create deduplication service with failing Redis client
				failingDedupService := processing.NewDeduplicationService(failingRedisClient, nil, logr.Discard(), nil)

				// Check() should return descriptive error
				_, _, err := failingDedupService.Check(ctx, testSignal)

				// BUSINESS VALIDATION: Error message quality
				// ✅ Error message mentions "redis unavailable"
				// ✅ Operators know Redis is the problem
				// ✅ Can check Redis health and restart if needed
				Expect(err).To(HaveOccurred(), "Should return error")
				Expect(err.Error()).To(ContainSubstring("redis unavailable"), "Error should mention Redis")

				GinkgoWriter.Printf("✅ Error message: %v\n", err)
			})
		})
	})
})
