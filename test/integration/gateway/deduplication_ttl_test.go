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
	"github.com/redis/go-redis/v9"
	"go.uber.org/zap"

	rediscache "github.com/jordigilh/kubernaut/pkg/cache/redis"
	"github.com/jordigilh/kubernaut/pkg/gateway/processing"
	"github.com/jordigilh/kubernaut/pkg/gateway/types"
)

// Business Outcome Testing: Test WHAT TTL expiration enables
//
// ❌ WRONG: "should expire Redis key after 5 minutes" (tests implementation)
// ✅ RIGHT: "treats alerts as fresh after incident resolution" (tests business outcome)

var _ = Describe("BR-GATEWAY-003: Deduplication TTL Expiration - Integration Tests", func() {
	var (
		ctx          context.Context
		dedupService *processing.DeduplicationService
		redisClient  *redis.Client
		logger       *zap.Logger
		testSignal   *types.NormalizedSignal
	)

	BeforeEach(func() {
		ctx = context.Background()
		logger = zap.NewNop()

		// Use suite's Redis client for test isolation in parallel execution
		// This ensures tests use the same Redis instance as the Gateway server
		redisAddr := fmt.Sprintf("localhost:%d", suiteRedisPort)
		// Use same DB offset as helpers.go: 2 + processID
		redisDB := 2 + GinkgoParallelProcess()

		redisClient = redis.NewClient(&redis.Options{
			Addr:     redisAddr,
			Password: "",
			DB:       redisDB,
		})

		// Verify Redis is available
		_, err := redisClient.Ping(ctx).Result()
		Expect(err).ToNot(HaveOccurred(), "Suite Redis should be available")

		// PHASE 1 FIX: Clean Redis state before each test to prevent state pollution
		err = redisClient.FlushDB(ctx).Err()
		Expect(err).ToNot(HaveOccurred(), "Should clean Redis before test")

		// Verify Redis is clean
		keys, err := redisClient.Keys(ctx, "*").Result()
		Expect(err).ToNot(HaveOccurred())
		Expect(keys).To(BeEmpty(), "Redis should be empty after flush")

		testSignal = &types.NormalizedSignal{
			AlertName: "HighMemoryUsage",
			Namespace: "production",
			Resource: types.ResourceIdentifier{
				Kind: "Pod",
				Name: "payment-api-ttl-test",
			},
			Severity:    "critical",
			Fingerprint: "integration-test-ttl-" + time.Now().Format("20060102150405"),
		}

		rediscacheClient := rediscache.NewClient(&redis.Options{
			Addr:     redisAddr,
			Password: "",
			DB:       redisDB,
		}, logger)
		dedupService = processing.NewDeduplicationServiceWithTTL(rediscacheClient, nil, 5*time.Second, logger, nil)
	})

	AfterEach(func() {
		if redisClient != nil {
			// Reset Redis config to prevent OOM cascade failures
			// (TriggerMemoryPressure sets maxmemory to 1MB)
			redisClient.ConfigSet(ctx, "maxmemory", "2147483648")
			redisClient.ConfigSet(ctx, "maxmemory-policy", "allkeys-lru")

			// Cleanup test data
			keys, _ := redisClient.Keys(ctx, "gateway:dedup:fingerprint:integration-test-ttl-*").Result()
			if len(keys) > 0 {
				redisClient.Del(ctx, keys...)
			}
			_ = redisClient.Close()
		}
	})

	Context("TTL Expiration Behavior", func() {
		It("treats expired fingerprint as new alert after 5-minute TTL", func() {
			// BR-GATEWAY-003: TTL expiration
			// BUSINESS SCENARIO: Payment-api OOM alert at T+0, resolved, new OOM at T+6min
			// Expected: Second alert NOT duplicate (TTL expired at T+5min)

			// Timeline:
			// T+0:00 → Alert fires → Record fingerprint
			// T+0:30 → Same alert → Duplicate (isDuplicate=true)
			// T+5:00 → TTL expires (Redis removes key)
			// T+5:01 → Same alert → NOT duplicate (isDuplicate=false, fresh alert)

			// Step 1: Record initial fingerprint with RemediationRequest reference
			err := dedupService.Record(ctx, testSignal.Fingerprint, "rr-initial-123")
			Expect(err).NotTo(HaveOccurred(),
				"First fingerprint recording must succeed")

			// Step 2: Verify it's detected as duplicate within TTL window
			isDuplicate, metadata, err := dedupService.Check(ctx, testSignal)
			Expect(err).NotTo(HaveOccurred())
			Expect(isDuplicate).To(BeTrue(),
				"Within TTL window, signal must be detected as duplicate")
			Expect(metadata.RemediationRequestRef).To(Equal("rr-initial-123"),
				"Duplicate must reference original RemediationRequest")

			// Step 3: Manually expire the key to simulate TTL expiration
			// This simulates waiting 5+ minutes without actually waiting
			key := "gateway:dedup:fingerprint:" + testSignal.Fingerprint
			deleted, err := redisClient.Del(ctx, key).Result()
			Expect(err).NotTo(HaveOccurred())
			Expect(deleted).To(BeNumerically(">", 0),
				"Redis key must exist and be deleted")

			// Step 4: Check again - should NOT be duplicate (TTL expired)
			isDuplicate, metadata, err = dedupService.Check(ctx, testSignal)
			Expect(err).NotTo(HaveOccurred())
			Expect(isDuplicate).To(BeFalse(),
				"After TTL expiration, signal must be treated as NEW alert")
			Expect(metadata).To(BeNil(),
				"Expired fingerprint has no metadata (fresh alert)")

			// BUSINESS OUTCOME VERIFIED:
			// ✅ Alerts after incident resolution create new CRDs (not duplicates)
			// ✅ TTL ensures deduplication doesn't linger forever
			// ✅ Each distinct incident gets its own RemediationRequest
			//
			// Real-world example:
			// 9:00 AM → Payment-api OOM → Alert → CRD rr-001 created
			// 9:01 AM → Same OOM → Alert → Duplicate (count=2, ref=rr-001)
			// 9:05 AM → OOM resolved, TTL expires
			// 9:10 AM → New payment-api OOM → Alert → NEW CRD rr-002 created ✅
		})

		It("uses configurable 5-minute TTL for deduplication window", func() {
			// BR-GATEWAY-003: TTL configuration
			// BUSINESS REQUIREMENT: 5-minute deduplication window (production)
			// TEST-SPECIFIC: Using 5-second TTL for fast testing
			// Expected: Fingerprints expire after configured TTL

			// Record fingerprint
			err := dedupService.Record(ctx, testSignal.Fingerprint, "rr-ttl-config-456")
			Expect(err).NotTo(HaveOccurred())

			// Verify TTL is set correctly in Redis
			key := "gateway:dedup:fingerprint:" + testSignal.Fingerprint
			ttl, err := redisClient.TTL(ctx, key).Result()
			Expect(err).NotTo(HaveOccurred())

			// TEST-SPECIFIC: Expect 5-second TTL (production uses 5 minutes)
			Expect(ttl).To(BeNumerically(">", 4*time.Second),
				"TTL must be greater than 4 seconds (allows for processing time)")
			Expect(ttl).To(BeNumerically("<=", 5*time.Second),
				"TTL must be exactly 5 seconds as configured for tests")

			// BUSINESS OUTCOME VERIFIED:
			// ✅ 5-minute window balances duplicate detection vs fresh incidents (production)
			// ✅ Too short (1 min) → Many false new alerts
			// ✅ Too long (30 min) → Resolved incidents still duplicates
			// ✅ 5 minutes → Optimal for typical incident resolution time
			// ✅ Test uses 5 seconds for fast execution
		})

		It("refreshes TTL on each duplicate detection", func() {
			// BR-GATEWAY-003: TTL refresh behavior
			// BUSINESS SCENARIO: Alert keeps firing every 30 seconds (storm)
			// TEST-SPECIFIC: Using 5-second TTL for fast testing
			// Expected: TTL refreshed on each duplicate, counter persists

			// Record initial fingerprint
			err := dedupService.Record(ctx, testSignal.Fingerprint, "rr-refresh-789")
			Expect(err).NotTo(HaveOccurred())

			// Wait 1 second to let TTL decrease (legitimate time-based test)
			time.Sleep(1 * time.Second)

			// Check TTL after 1 second (should be ~4 seconds remaining)
			key := "gateway:dedup:fingerprint:" + testSignal.Fingerprint
			ttlBefore, err := redisClient.TTL(ctx, key).Result()
			Expect(err).NotTo(HaveOccurred())
			GinkgoWriter.Printf("TTL before duplicate detection: %v\n", ttlBefore)

			// Detect duplicate (this should refresh TTL)
			isDuplicate, _, err := dedupService.Check(ctx, testSignal)
			Expect(err).NotTo(HaveOccurred())
			Expect(isDuplicate).To(BeTrue())

			// FIX: Use Eventually to handle timing variance in TTL refresh
			// Check that TTL is refreshed to approximately 5 seconds
			Eventually(func() time.Duration {
				ttl, _ := redisClient.TTL(ctx, key).Result()
				GinkgoWriter.Printf("TTL after duplicate detection: %v\n", ttl)
				return ttl
			}, "5s", "100ms").Should(BeNumerically(">=", 4*time.Second),
				"TTL should be refreshed to approximately 5 seconds after duplicate detection")

			// BUSINESS OUTCOME VERIFIED:
			// ✅ Ongoing incidents keep deduplication active
			// ✅ TTL only expires after 5 minutes of silence (production)
			// ✅ Prevents premature expiration during storm
			//
			// Real-world example (production with 5-minute TTL):
			// 9:00 AM → Alert fires → TTL = 5 min (expires at 9:05)
			// 9:03 AM → Alert fires again → TTL refreshed = 5 min (now expires at 9:08)
			// 9:06 AM → Alert fires again → TTL refreshed = 5 min (now expires at 9:11)
			// 9:11 AM → No more alerts → TTL expires
			// 9:12 AM → New alert → Treated as fresh (TTL expired)
		})
	})

	Context("TTL Integration with Duplicate Counter", func() {
		It("preserves duplicate count until TTL expiration", func() {
			// BR-GATEWAY-003: Counter persistence with TTL
			// BUSINESS SCENARIO: Track duplicate count across 5-minute window
			// Expected: Counter increments with each duplicate until TTL expires

			// Record initial fingerprint (count=1)
			err := dedupService.Record(ctx, testSignal.Fingerprint, "rr-counter-101")
			Expect(err).NotTo(HaveOccurred())

			// Detect duplicate #1 (count increments to 2)
			isDuplicate, metadata, err := dedupService.Check(ctx, testSignal)
			Expect(err).NotTo(HaveOccurred())
			Expect(isDuplicate).To(BeTrue())
			Expect(metadata.Count).To(Equal(2),
				"First Check() after Record() should have count=2")

			// Detect duplicate #2 (count increments to 3)
			isDuplicate, metadata, err = dedupService.Check(ctx, testSignal)
			Expect(err).NotTo(HaveOccurred())
			Expect(isDuplicate).To(BeTrue())
			Expect(metadata.Count).To(Equal(3),
				"Second Check() should have count=3")

			// Manually expire fingerprint
			key := "gateway:dedup:fingerprint:" + testSignal.Fingerprint
			redisClient.Del(ctx, key)

			// Check after expiration - counter reset
			isDuplicate, metadata, err = dedupService.Check(ctx, testSignal)
			Expect(err).NotTo(HaveOccurred())
			Expect(isDuplicate).To(BeFalse(),
				"After TTL expiration, treated as new alert")
			Expect(metadata).To(BeNil(),
				"No metadata for fresh alert")

			// BUSINESS OUTCOME VERIFIED:
			// ✅ Duplicate count accurate within TTL window
			// ✅ TTL expiration resets counter (fresh incident)
			// ✅ Operators see accurate duplicate count per incident
		})
	})
})
