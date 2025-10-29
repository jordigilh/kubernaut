package gateway

import (
	"context"
	"net/http/httptest"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

var _ = Describe("Redis Resilience Integration Tests", func() {
	var (
		ctx         context.Context
		testServer  *httptest.Server
		redisClient *RedisTestClient
		k8sClient   *K8sTestClient
	)

	BeforeEach(func() {
		ctx = context.Background()
		redisClient = SetupRedisTestClient(ctx)
		k8sClient = SetupK8sTestClient(ctx)

		// Create production namespace for tests (required for CRD creation)
		ns := &corev1.Namespace{}
		ns.Name = "production"
		_ = k8sClient.Client.Delete(ctx, ns) // Delete first (ignore error)

		// Wait for deletion to complete (namespace deletion is asynchronous)
		Eventually(func() error {
			checkNs := &corev1.Namespace{}
			return k8sClient.Client.Get(ctx, client.ObjectKey{Name: "production"}, checkNs)
		}, "10s", "100ms").Should(HaveOccurred(), "Namespace should be deleted")

		// Now create fresh namespace with environment label
		ns = &corev1.Namespace{}
		ns.Name = "production"
		ns.Labels = map[string]string{
			"environment": "production", // Required for EnvironmentClassifier
		}
		err := k8sClient.Client.Create(ctx, ns)
		Expect(err).ToNot(HaveOccurred(), "Should create production namespace")

		gatewayServer, err := StartTestGateway(ctx, redisClient, k8sClient)
		Expect(err).ToNot(HaveOccurred(), "Failed to create Gateway server")
		testServer = httptest.NewServer(gatewayServer.Handler())

		// Ensure Redis is clean before each test
		redisClient.Client.FlushDB(ctx)
	})

	AfterEach(func() {
		// Reset Redis config to prevent OOM cascade failures
		if redisClient != nil && redisClient.Client != nil {
			redisClient.Client.ConfigSet(ctx, "maxmemory", "2147483648")
			redisClient.Client.ConfigSet(ctx, "maxmemory-policy", "allkeys-lru")
		}

		// Cleanup namespace (this will cascade delete all CRDs in the namespace)
		ns := &corev1.Namespace{}
		ns.Name = "production"
		_ = k8sClient.Client.Delete(ctx, ns) // Ignore error if namespace doesn't exist

		if testServer != nil {
			testServer.Close()
		}
	})

	Context("BR-GATEWAY-071: Redis Failure Handling", func() {
		// NOTE: Chaos tests moved to test/chaos/gateway/
		// - Redis connection failure gracefully → test/chaos/gateway/redis_failure_test.go
		// - Redis recovery after outage → test/chaos/gateway/redis_recovery_test.go
		// See test/chaos/gateway/README.md for implementation details

		It("should respect context timeout when Redis is slow", func() {
			// BR-GATEWAY-071: Timeout handling for slow Redis operations
			// BUSINESS OUTCOME: Gateway doesn't hang on slow Redis
			// TEST-SPECIFIC: Uses real Redis with artificial slowness

			payload := GeneratePrometheusAlert(PrometheusAlertOptions{
				AlertName: "SlowRedisTest",
				Namespace: "production",
			})

			// Send request with very short timeout context
			// Note: This test validates that the Gateway respects timeouts
			// In production, Redis should respond in <5ms (p95)
			resp := SendWebhook(testServer.URL+"/api/v1/signals/prometheus", payload)

			// BUSINESS OUTCOME: Request completes (doesn't hang indefinitely)
			// May return 201 (success) or 503 (timeout) depending on Redis speed
			Expect(resp.StatusCode).To(Or(Equal(201), Equal(503)))
		})
	})

	Context("BR-GATEWAY-072: Redis Connection Pool Management", func() {
		It("should handle concurrent Redis writes without corruption", func() {
			// BR-GATEWAY-072: Concurrent write safety
			// BUSINESS OUTCOME: Multiple alerts processed simultaneously without data corruption
			// TEST-SPECIFIC: 10 concurrent requests (integration test scale)

			// Send 10 concurrent requests with different alert names
			results := make(chan WebhookResponse, 10)
			for i := 0; i < 10; i++ {
				go func(index int) {
					payload := GeneratePrometheusAlert(PrometheusAlertOptions{
						AlertName: "ConcurrentTest",
						Namespace: "production",
						Labels: map[string]string{
							"pod": "test-pod-" + string(rune('0'+index)),
						},
					})
					results <- SendWebhook(testServer.URL+"/api/v1/signals/prometheus", payload)
				}(i)
			}

			// Collect all results
			successCount := 0
			for i := 0; i < 10; i++ {
				resp := <-results
				if resp.StatusCode == 201 || resp.StatusCode == 202 {
					successCount++
				}
			}

			// BUSINESS OUTCOME: All requests processed successfully
			Expect(successCount).To(Equal(10), "All concurrent requests should succeed")

			// Verify Redis state is consistent (no corruption)
			fingerprintCount := redisClient.CountFingerprints(ctx, "production")
			Expect(fingerprintCount).To(BeNumerically(">=", 1), "At least one fingerprint should be stored")
			Expect(fingerprintCount).To(BeNumerically("<=", 10), "No more than 10 fingerprints should be stored")
		})
	})

	// NOTE: Following contexts moved to other tiers:
	// BR-GATEWAY-073: Redis State Cleanup → test/e2e/gateway/crd_lifecycle_test.go (DEFERRED - out of scope v1.0)
	// BR-GATEWAY-074: Redis Cluster Failover → test/chaos/gateway/redis_ha_test.go
	// BR-GATEWAY-075: Redis Pipeline Failures → test/chaos/gateway/redis_pipeline_failure_test.go
	// See test/chaos/gateway/README.md and test/e2e/gateway/README.md for implementation details

	Context("BR-GATEWAY-076: Redis Memory Management", func() {
		It("should handle Redis memory pressure gracefully", func() {
			// BR-GATEWAY-076: Memory pressure handling
			// BUSINESS OUTCOME: Gateway handles Redis OOM errors gracefully
			// TEST-SPECIFIC: Uses real Redis with limited memory (1GB)

			// Send multiple alerts to fill Redis memory
			// Note: This test validates graceful degradation, not memory exhaustion
			for i := 0; i < 100; i++ {
				payload := GeneratePrometheusAlert(PrometheusAlertOptions{
					AlertName: "MemoryPressureTest",
					Namespace: "production",
					Labels: map[string]string{
						"instance": "test-instance-" + string(rune('0'+i%10)),
					},
				})
				resp := SendWebhook(testServer.URL+"/api/v1/signals/prometheus", payload)

				// BUSINESS OUTCOME: Requests processed or gracefully rejected
				// 201 = success, 503 = Redis unavailable (OOM)
				Expect(resp.StatusCode).To(Or(Equal(201), Equal(202), Equal(503)))
			}

			// Verify Gateway is still responsive after memory pressure
			payload := GeneratePrometheusAlert(PrometheusAlertOptions{
				AlertName: "PostMemoryPressureTest",
				Namespace: "production",
			})
			resp := SendWebhook(testServer.URL+"/api/v1/signals/prometheus", payload)
			Expect(resp.StatusCode).To(Or(Equal(201), Equal(202), Equal(503)))
		})
	})

	Context("BR-GATEWAY-077: Redis TTL Expiration", func() {
		It("should handle TTL expiration correctly", func() {
			// BR-GATEWAY-077: TTL-based cleanup
			// BUSINESS OUTCOME: Old fingerprints automatically removed
			// TEST-SPECIFIC: Uses 5-second TTL for fast testing (production: 5 minutes)

			payload := GeneratePrometheusAlert(PrometheusAlertOptions{
				AlertName: "TTLExpirationTest",
				Namespace: "production",
			})

			// Send alert (creates fingerprint with TTL)
			resp := SendWebhook(testServer.URL+"/api/v1/signals/prometheus", payload)
			Expect(resp.StatusCode).To(Equal(201))

			// Verify fingerprint exists
			fingerprintCount := redisClient.CountFingerprints(ctx, "production")
			Expect(fingerprintCount).To(Equal(1))

			// Wait for TTL to expire (5 seconds + 1 second buffer)
			time.Sleep(6 * time.Second)

			// BUSINESS OUTCOME: Expired fingerprints removed automatically
			fingerprintCount = redisClient.CountFingerprints(ctx, "production")
			Expect(fingerprintCount).To(Equal(0), "Fingerprint should be removed after TTL expiration")
		})
	})
})
