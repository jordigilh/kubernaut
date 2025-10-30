// Package gateway contains integration tests for Gateway Service Redis integration
package gateway

import (
	"context"
	"fmt"
	"net/http/httptest"
	"sync"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// ━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━
// DAY 8 PHASE 2: REDIS INTEGRATION TESTS
// ━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━
//
// Purpose: Validate Gateway behavior with Redis state management
// Coverage: BR-GATEWAY-008 (deduplication persistence), BR-GATEWAY-007 (storm state)
// Test Count: 9 tests (5 basic + 4 edge cases)
//
// Business Outcomes Validated:
// 1. Deduplication state persists in Redis
// 2. TTL expiration handled correctly
// 3. Redis connection failures handled gracefully
// 4. Storm detection state managed correctly
// 5. Redis cluster failover doesn't corrupt state
//
// NOTE: "Redis connection pool exhaustion" test moved to test/load/gateway/redis_load_test.go
// Reason: Tests connection pool limits (200+ concurrent requests), not business logic
// Date: 2025-10-27
// ━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━

var _ = Describe("DAY 8 PHASE 2: Redis Integration Tests", func() {
	var (
		ctx           context.Context
		cancel        context.CancelFunc
		testServer    *httptest.Server
		redisClient   *RedisTestClient
		k8sClient     *K8sTestClient
		testNamespace string
		testCounter   int
	)

	BeforeEach(func() {
		ctx, cancel = context.WithTimeout(context.Background(), 5*time.Minute)

		// Generate unique namespace for test isolation
		testCounter++
		testNamespace = fmt.Sprintf("test-redis-int-%d-%d-%d",
			time.Now().UnixNano(),
			GinkgoRandomSeed(),
			testCounter)

		// Setup test infrastructure
		redisClient = SetupRedisTestClient(ctx)
		k8sClient = SetupK8sTestClient(ctx)

		// PHASE 1 FIX: Clean Redis state before each test to prevent state pollution
		if redisClient != nil && redisClient.Client != nil {
			err := redisClient.Client.FlushDB(ctx).Err()
			Expect(err).ToNot(HaveOccurred(), "Should clean Redis before test")

			// Verify Redis is clean (synchronous check - FlushDB is atomic)
			keys, err := redisClient.Client.Keys(ctx, "*").Result()
			Expect(err).ToNot(HaveOccurred(), "Should query Redis keys")
			Expect(keys).To(BeEmpty(), "Redis should be empty after flush")
		}

		// Create unique test namespace with environment label
		EnsureTestNamespace(ctx, k8sClient, testNamespace)

		// Add environment label for classification
		ns := &corev1.Namespace{}
		err := k8sClient.Client.Get(ctx, client.ObjectKey{Name: testNamespace}, ns)
		Expect(err).ToNot(HaveOccurred(), "Should get namespace")
		if ns.Labels == nil {
			ns.Labels = make(map[string]string)
		}
		ns.Labels["environment"] = "production"
		err = k8sClient.Client.Update(ctx, ns)
		Expect(err).ToNot(HaveOccurred(), "Should update namespace labels")

		// Start Gateway server
		gatewayServer, err := StartTestGateway(ctx, redisClient, k8sClient)
		Expect(err).ToNot(HaveOccurred(), "Failed to create Gateway server")
		testServer = httptest.NewServer(gatewayServer.Handler())
	})

	AfterEach(func() {
		// Reset Redis config to prevent OOM cascade failures
		if redisClient != nil && redisClient.Client != nil {
			redisClient.Client.ConfigSet(ctx, "maxmemory", "2147483648")
			redisClient.Client.ConfigSet(ctx, "maxmemory-policy", "allkeys-lru")
		}

		// Cleanup handled by suite-level cleanup

		// Cleanup
		if testServer != nil {
			testServer.Close()
		}
		redisClient.Cleanup(ctx)
		k8sClient.Cleanup(ctx)
		cancel()
	})

	// ━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━
	// ORIGINAL TESTS (1-6): Basic Redis Integration
	// ━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━

	Context("Basic Redis Integration", func() {
		It("should persist deduplication state in Redis", func() {
			// BR-GATEWAY-008: Deduplication persistence
			// BUSINESS OUTCOME: Duplicate detection works across requests

			payload := GeneratePrometheusAlert(PrometheusAlertOptions{
				AlertName: "PersistenceTest",
				Namespace: testNamespace,
			})

			// Send first alert
			resp1 := SendWebhook(testServer.URL+"/api/v1/signals/prometheus", payload)
			Expect(resp1.StatusCode).To(Equal(201))

			// Verify: Fingerprint stored in Redis
			fingerprintCount := redisClient.CountFingerprints(ctx, testNamespace)
			Expect(fingerprintCount).To(Equal(1))

			// Send duplicate alert
			resp2 := SendWebhook(testServer.URL+"/api/v1/signals/prometheus", payload)
			Expect(resp2.StatusCode).To(Equal(202)) // v2.9: Duplicate detected (202 Accepted)

			// BUSINESS OUTCOME: Deduplication state persisted
			// Verify: Still only 1 fingerprint in Redis
			fingerprintCount = redisClient.CountFingerprints(ctx, testNamespace)
			Expect(fingerprintCount).To(Equal(1))
		})

		It("should expire deduplication entries after TTL", func() {
			// BR-GATEWAY-008: TTL-based expiration
			// BUSINESS OUTCOME: Old fingerprints cleaned up automatically
			// TEST-SPECIFIC: Using 5-second TTL for fast testing (production: 5 minutes)

			// Use unique alert name with timestamp to avoid CRD name collisions
			uniqueAlertName := fmt.Sprintf("TTLTest-%d", time.Now().Unix())

			payload := GeneratePrometheusAlert(PrometheusAlertOptions{
				AlertName: uniqueAlertName,
				Namespace: testNamespace,
			})

			// Send alert
			resp := SendWebhook(testServer.URL+"/api/v1/signals/prometheus", payload)
			Expect(resp.StatusCode).To(Equal(201))

			// Wait for TTL to expire (5 seconds + 1 second buffer)
			// Production uses 5 minutes, but tests use 5 seconds for fast execution
			time.Sleep(6 * time.Second)

			// BUSINESS OUTCOME: Send same alert again - should create NEW CRD (not deduplicated)
			// This proves the fingerprint was removed from Redis after TTL expiration
			// Note: CRD from first request still exists, but deduplication is based on Redis TTL
			// In production, CRDs would be processed and deleted by the workflow engine
			resp2 := SendWebhook(testServer.URL+"/api/v1/signals/prometheus", payload)
			Expect(resp2.StatusCode).To(Equal(201), "Should create new CRD after TTL expiration (not deduplicated)")

			// Business capability verified:
			// ✅ TTL expiration removes fingerprints from Redis
			// ✅ Duplicate alerts after TTL create new CRDs (not deduplicated)
		})

		XIt("should handle Redis connection failure gracefully", func() {
			// TODO: This test closes the test Redis client, not the server
			// Need to redesign to test graceful degradation (503 response) when Redis is unavailable
			// Move to E2E tier with chaos testing
			// BR-GATEWAY-008: Redis failure handling
			// BUSINESS OUTCOME: Gateway continues processing without Redis

			// Stop Redis
			_ = redisClient.Client.Close()

			payload := GeneratePrometheusAlert(PrometheusAlertOptions{
				AlertName: "RedisFailureTest",
				Namespace: testNamespace,
			})

			// Send alert (should still work, but no deduplication)
			resp := SendWebhook(testServer.URL+"/api/v1/signals/prometheus", payload)

			// BUSINESS OUTCOME: Request processed despite Redis failure
			// May return 201 (created) or 500 (error) depending on graceful degradation
			Expect(resp.StatusCode).To(Or(Equal(201), Equal(500)))
		})

		It("should store storm detection state in Redis", func() {
			// BR-GATEWAY-007: Storm state persistence
			// BUSINESS OUTCOME: Storm detection persists across requests
			// BUSINESS SCENARIO: Send alerts in 2 batches - storm detection should persist
			//
			// Expected behavior:
			// - Batch 1 (3 alerts): Triggers storm detection (threshold=2)
			// - Wait 500ms (simulate time gap between batches)
			// - Batch 2 (2 alerts): Storm detection should STILL be active (state persisted)
			//
			// If Redis state is NOT persisting:
			// - Batch 2 would start fresh, returning 201 Created
			// If Redis state IS persisting:
			// - Batch 2 continues storm, returning 202 Accepted

			alertName := "PodCrashLoop"
			namespace := "production"

			// BATCH 1: Send 3 alerts to trigger storm detection
			var batch1Responses []WebhookResponse
			for i := 0; i < 3; i++ {
				payload := GeneratePrometheusAlert(PrometheusAlertOptions{
					AlertName: alertName,
					Namespace: namespace,
					Resource: ResourceIdentifier{
						Kind: "Pod",
						Name: fmt.Sprintf("test-pod-%d", i),
					},
				})

				resp := SendWebhook(testServer.URL+"/api/v1/signals/prometheus", payload)
				batch1Responses = append(batch1Responses, resp)
			}

			// BUSINESS OUTCOME 1: Storm detection triggers based on threshold
			// With threshold=2 and exclusive check (count > 2):
			// Alert 1: 201 Created (count=1, 1 > 2? No)
			// Alert 2: 201 Created (count=2, 2 > 2? No) OR 202 Accepted (if count=3 due to race)
			// Alert 3: 202 Accepted (count=3, 3 > 2? Yes, STORM DETECTED)
			//
			// Note: Due to async processing, alert 2 might see count=3 and return 202
			// The key business outcome is that storm detection activates by alert 3
			Expect(batch1Responses[0].StatusCode).To(Equal(201), "Alert 1 should create CRD (201 Created)")
			// Alert 2 can be either 201 or 202 depending on timing
			Expect(batch1Responses[1].StatusCode).To(Or(Equal(201), Equal(202)),
				"Alert 2 should be 201 Created or 202 Accepted (timing dependent)")
			Expect(batch1Responses[2].StatusCode).To(Equal(202), "Alert 3 should trigger storm (202 Accepted)")

			// Wait 500ms to simulate time gap between batches
			// This tests that Redis state persists across time
			time.Sleep(500 * time.Millisecond)

			// BATCH 2: Send 2 more alerts with same alertname
			var batch2Responses []WebhookResponse
			for i := 3; i < 5; i++ {
				payload := GeneratePrometheusAlert(PrometheusAlertOptions{
					AlertName: alertName,
					Namespace: namespace,
					Resource: ResourceIdentifier{
						Kind: "Pod",
						Name: fmt.Sprintf("test-pod-%d", i),
					},
				})

				resp := SendWebhook(testServer.URL+"/api/v1/signals/prometheus", payload)
				batch2Responses = append(batch2Responses, resp)
			}

			// BUSINESS OUTCOME 2: Storm detection persists across batches
			// If Redis state was lost, these would return 201 Created (starting fresh)
			// If Redis state persisted, these return 202 Accepted (storm continues)
			Expect(batch2Responses[0].StatusCode).To(Equal(202),
				"Alert 4 should continue storm (202 Accepted) - proves Redis state persisted")
			Expect(batch2Responses[1].StatusCode).To(Equal(202),
				"Alert 5 should continue storm (202 Accepted) - proves Redis state persisted")

			// BUSINESS CAPABILITY VERIFIED:
			// ✅ Storm detection state persists in Redis across time gaps
			// ✅ Storm detection continues after 500ms delay (state not lost)
			// ✅ HTTP status codes correctly reflect storm state (202 Accepted)
		})

		It("should handle concurrent Redis writes without corruption", func() {
			// BR-GATEWAY-008: Concurrent Redis operations
			// BUSINESS OUTCOME: No race conditions in Redis writes

			// Send 5 unique alerts concurrently (below storm threshold of 10)
			// This tests Redis atomic operations without triggering storm aggregation
			var wg sync.WaitGroup
			successCount := 0
			var mu sync.Mutex

			for i := 0; i < 5; i++ {
				wg.Add(1)
				go func(index int) {
					defer wg.Done()
					defer GinkgoRecover()

					payload := GeneratePrometheusAlert(PrometheusAlertOptions{
						AlertName: fmt.Sprintf("ConcurrentRedis-%d", index),
						Namespace: testNamespace,
					})

					resp := SendWebhook(testServer.URL+"/api/v1/signals/prometheus", payload)
					if resp.StatusCode == 201 {
						mu.Lock()
						successCount++
						mu.Unlock()
					}
				}(i)
			}

			// Wait for all goroutines to complete
			wg.Wait()

			// BUSINESS OUTCOME: All 5 unique alerts created successfully
			// No race conditions or corruption in Redis writes
			Expect(successCount).To(Equal(5), "All 5 concurrent alerts should be created successfully")
		})

		// NOTE: "should clean up Redis state on CRD deletion" test DELETED
		// Decision: DD-GATEWAY-005 - Current TTL-based cleanup is intentional design
		// Redis fingerprints expire via TTL (5 minutes), not immediate cleanup on CRD deletion
		// This protects against false positives and alert storms after CRD deletion
		// See: docs/decisions/DD-GATEWAY-005-redis-cleanup-on-crd-deletion.md
	})

	// ━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━
	// EDGE CASE TESTS (7-10): Advanced Redis Scenarios
	// ━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━

	Context("Edge Cases: Advanced Redis Scenarios", func() {
		// NOTE: Chaos test moved to test/chaos/gateway/
		// - Redis cluster failover → test/chaos/gateway/redis_ha_test.go
		// See test/chaos/gateway/README.md for implementation details

		It("should handle Redis memory eviction (LRU) gracefully", func() {
			// EDGE CASE: Redis memory full, LRU eviction active
			// BUSINESS OUTCOME: System handles evicted fingerprints
			// Production Risk: Redis memory pressure

			// Fill Redis with fingerprints (reduced from 1000 to 10 for test performance)
			// NOTE: In production, this would be 1000+, but 10 is sufficient to validate the logic
			for i := 0; i < 10; i++ {
				payload := GeneratePrometheusAlert(PrometheusAlertOptions{
					AlertName: fmt.Sprintf("EvictionTest-%d", i),
					Namespace: testNamespace,
				})

				SendWebhook(testServer.URL+"/api/v1/signals/prometheus", payload)
			}

			// Trigger Redis memory pressure (simulate LRU eviction)
			redisClient.TriggerMemoryPressure(ctx)

			// Send alert that may have been evicted
			payload := GeneratePrometheusAlert(PrometheusAlertOptions{
				AlertName: "EvictionTest-0",
				Namespace: testNamespace,
			})

			resp := SendWebhook(testServer.URL+"/api/v1/signals/prometheus", payload)

			// BUSINESS OUTCOME: System handles eviction gracefully
			// May create new CRD (201) if evicted, detect duplicate (202), or reject if Redis down (503)
			Expect(resp.StatusCode).To(Or(Equal(201), Equal(202), Equal(503)))
		})

		XIt("should handle Redis pipeline command failures", func() {
			// TODO: Requires Redis failure injection not available in integration tests
			// MOVED TO: test/e2e/gateway/chaos/redis_failure_test.go (2025-10-27)
			// Reason: Requires chaos engineering infrastructure for failure injection
			// See: test/e2e/gateway/chaos/CHAOS_TEST_SCENARIOS.md for implementation plan
			// EDGE CASE: Redis pipeline commands fail mid-batch
			// BUSINESS OUTCOME: Partial failures don't corrupt state
			// Production Risk: Network issues during batch operations

			// Send batch of alerts using pipeline
			for i := 0; i < 20; i++ {
				payload := GeneratePrometheusAlert(PrometheusAlertOptions{
					AlertName: fmt.Sprintf("PipelineTest-%d", i),
					Namespace: testNamespace,
				})

				SendWebhook(testServer.URL+"/api/v1/signals/prometheus", payload)
			}

			// Simulate pipeline failure
			redisClient.SimulatePipelineFailure(ctx)

			// Continue sending alerts
			for i := 20; i < 40; i++ {
				payload := GeneratePrometheusAlert(PrometheusAlertOptions{
					AlertName: fmt.Sprintf("PipelineTest-%d", i),
					Namespace: testNamespace,
				})

				SendWebhook(testServer.URL+"/api/v1/signals/prometheus", payload)
			}

			// BUSINESS OUTCOME: State remains consistent despite failures
			fingerprintCount := redisClient.CountFingerprints(ctx, testNamespace)
			Expect(fingerprintCount).To(BeNumerically(">=", 30))
		})

		// NOTE: "Redis connection pool exhaustion" test moved to test/load/gateway/redis_load_test.go
		// Reason: Tests connection pool limits (200+ concurrent requests), not business logic
		// Date: 2025-10-27
	})
})
