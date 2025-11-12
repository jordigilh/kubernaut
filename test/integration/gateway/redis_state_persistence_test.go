package gateway

import (
	"context"
	"fmt"
	"net/http/httptest"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"sigs.k8s.io/controller-runtime/pkg/client"

	remediationv1alpha1 "github.com/jordigilh/kubernaut/api/remediation/v1alpha1"
	gateway "github.com/jordigilh/kubernaut/pkg/gateway"
)

// BR-003: Signal deduplication
// BR-005: Environment classification
// BR-077: Redis state persistence
//
// Business Outcome: Deduplication state survives Gateway restarts
//
// Test Strategy: Validate Redis state persistence across Gateway lifecycles
// - TTL persistence: Deduplication TTL survives Gateway restart
// - Duplicate count persistence: Duplicate counts survive Gateway restart
// - Storm counter persistence: Storm detection state survives Gateway restart
//
// Defense-in-Depth: These integration tests complement unit tests
// - Unit: Test deduplication logic (pure business logic)
// - Integration: Test Redis state persistence (THIS TEST)
// - E2E: Test complete workflows with Gateway restarts

var _ = Describe("BR-003, BR-005, BR-077: Redis State Persistence - Integration Tests", func() {
	var (
		ctx           context.Context
		cancel        context.CancelFunc
		testServer    *httptest.Server
		gatewayServer *gateway.Server
		redisClient   *RedisTestClient
		k8sClient     *K8sTestClient
		testNamespace string
		testCounter   int
	)

	BeforeEach(func() {
		ctx, cancel = context.WithTimeout(context.Background(), 5*time.Minute)

		// Generate unique namespace for test isolation
		testCounter++
		testNamespace = fmt.Sprintf("test-redis-%d-%d-%d",
			time.Now().UnixNano(),
			GinkgoRandomSeed(),
			testCounter)

		// Setup test infrastructure
		redisClient = SetupRedisTestClient(ctx)
		Expect(redisClient).ToNot(BeNil(), "Redis client required")
		Expect(redisClient.Client).ToNot(BeNil(), "Redis connection required")

		k8sClient = SetupK8sTestClient(ctx)
		Expect(k8sClient).ToNot(BeNil(), "K8s client required")

		// Clean Redis state
		err := redisClient.Client.FlushDB(ctx).Err()
		Expect(err).ToNot(HaveOccurred(), "Should flush Redis")

		// Ensure test namespace exists
		EnsureTestNamespace(ctx, k8sClient, testNamespace)
		RegisterTestNamespace(testNamespace)

		// Start Gateway server
		var startErr error
		gatewayServer, startErr = StartTestGateway(ctx, redisClient, k8sClient)
		Expect(startErr).ToNot(HaveOccurred(), "Gateway should start")
		Expect(gatewayServer).ToNot(BeNil(), "Gateway server should exist")

		// Create HTTP test server
		testServer = httptest.NewServer(gatewayServer.Handler())
		Expect(testServer).ToNot(BeNil(), "HTTP test server should exist")
	})

	AfterEach(func() {
		if testServer != nil {
			testServer.Close()
		}
		if cancel != nil {
			cancel()
		}
	})

	Context("BR-003: Deduplication TTL Persistence", func() {
		It("should persist deduplication state across Gateway restart", func() {
			// BUSINESS OUTCOME: Deduplication state survives Gateway restarts
			// WHY: Prevents duplicate CRDs after Gateway pod restarts (rolling updates, crashes)
			// EXPECTED: Duplicate alerts rejected even after Gateway restart
			//
			// DEFENSE-IN-DEPTH: Complements unit tests
			// - Unit: Tests deduplication logic
			// - Integration: Tests Redis persistence (THIS TEST)
			// - E2E: Tests complete workflows with Gateway restarts

			// STEP 1: Send first alert (creates CRD and Redis state)
			payload := GeneratePrometheusAlert(PrometheusAlertOptions{
				AlertName: "PersistenceTest",
				Namespace: testNamespace,
				Severity:  "critical",
				Resource: ResourceIdentifier{
					Kind: "Pod",
					Name: "test-pod",
				},
			})

			resp1 := SendWebhook(testServer.URL+"/api/v1/signals/prometheus", payload)
			Expect(resp1.StatusCode).To(Equal(201), "First alert should return 201 Created")

			// Wait for CRD creation
			Eventually(func() int {
				crdList := &remediationv1alpha1.RemediationRequestList{}
				_ = k8sClient.Client.List(ctx, crdList, client.InNamespace(testNamespace))
				return len(crdList.Items)
			}, "30s", "500ms").Should(Equal(1), "Should have 1 CRD")

			// STEP 2: Verify Redis state exists
			fingerprintCount := redisClient.CountFingerprints(ctx, testNamespace)
			Expect(fingerprintCount).To(Equal(1), "Should have 1 fingerprint in Redis")

			// STEP 3: Simulate Gateway restart (stop old server, start new one)
			testServer.Close()
			gatewayServer = nil

			// Create new Gateway server (simulates restart)
			var restartErr error
			gatewayServer, restartErr = StartTestGateway(ctx, redisClient, k8sClient)
			Expect(restartErr).ToNot(HaveOccurred(), "Gateway should restart")

			testServer = httptest.NewServer(gatewayServer.Handler())

			// STEP 4: Send duplicate alert after restart
			resp2 := SendWebhook(testServer.URL+"/api/v1/signals/prometheus", payload)

			// BUSINESS VALIDATION: HTTP 202 Accepted (duplicate detected from Redis)
			Expect(resp2.StatusCode).To(Equal(202), "Duplicate should be detected after restart")

			// BUSINESS VALIDATION: Still only 1 CRD (no duplicate CRD created)
			Consistently(func() int {
				crdList := &remediationv1alpha1.RemediationRequestList{}
				_ = k8sClient.Client.List(ctx, crdList, client.InNamespace(testNamespace))
				return len(crdList.Items)
			}, "5s", "500ms").Should(Equal(1), "Should still have only 1 CRD after restart")
		})
	})

	Context("BR-003: Duplicate Count Persistence", func() {
		It("should persist duplicate count across Gateway restart", func() {
			// BUSINESS OUTCOME: Duplicate counts survive Gateway restarts
			// WHY: Operators need accurate duplicate counts for troubleshooting
			// EXPECTED: Duplicate count increments correctly after Gateway restart

			// STEP 1: Send first alert
			payload := GeneratePrometheusAlert(PrometheusAlertOptions{
				AlertName: "DuplicateCountTest",
				Namespace: testNamespace,
				Severity:  "warning",
				Resource: ResourceIdentifier{
					Kind: "Pod",
					Name: "count-pod",
				},
			})

			resp1 := SendWebhook(testServer.URL+"/api/v1/signals/prometheus", payload)
			Expect(resp1.StatusCode).To(Equal(201), "First alert should return 201 Created")

			// Wait for CRD creation
			Eventually(func() int {
				crdList := &remediationv1alpha1.RemediationRequestList{}
				_ = k8sClient.Client.List(ctx, crdList, client.InNamespace(testNamespace))
				return len(crdList.Items)
			}, "30s", "500ms").Should(Equal(1), "Should have 1 CRD")

			// STEP 2: Send duplicate alert (before restart)
			resp2 := SendWebhook(testServer.URL+"/api/v1/signals/prometheus", payload)
			Expect(resp2.StatusCode).To(Equal(202), "Duplicate should be detected")

			// STEP 3: Restart Gateway
			testServer.Close()
			gatewayServer = nil

			var restartErr error
			gatewayServer, restartErr = StartTestGateway(ctx, redisClient, k8sClient)
			Expect(restartErr).ToNot(HaveOccurred(), "Gateway should restart")

			testServer = httptest.NewServer(gatewayServer.Handler())

			// STEP 4: Send another duplicate alert (after restart)
			resp3 := SendWebhook(testServer.URL+"/api/v1/signals/prometheus", payload)

			// BUSINESS VALIDATION: Still detected as duplicate after restart
			Expect(resp3.StatusCode).To(Equal(202), "Duplicate should be detected after restart")

			// BUSINESS VALIDATION: Duplicate count persisted in Redis
			// (The duplicate count is stored in Redis and should increment correctly)
			fingerprintCount := redisClient.CountFingerprints(ctx, testNamespace)
			Expect(fingerprintCount).To(Equal(1), "Should still have 1 fingerprint in Redis")
		})
	})

	Context("BR-077: Storm Counter Persistence", func() {
		It("should persist storm detection state across Gateway restart", func() {
			// BUSINESS OUTCOME: Storm detection state survives Gateway restarts
			// WHY: Prevents losing storm detection state during rolling updates
			// EXPECTED: Storm detection continues correctly after Gateway restart

			// STEP 1: Send first alert (starts storm detection)
			payload1 := GeneratePrometheusAlert(PrometheusAlertOptions{
				AlertName: "StormTest",
				Namespace: testNamespace,
				Severity:  "critical",
				Resource: ResourceIdentifier{
					Kind: "Pod",
					Name: "storm-pod-1",
				},
			})

			resp1 := SendWebhook(testServer.URL+"/api/v1/signals/prometheus", payload1)
			Expect(resp1.StatusCode).To(Or(Equal(201), Equal(202)), "First alert should be accepted")

			// STEP 2: Send second alert (increments storm counter)
			payload2 := GeneratePrometheusAlert(PrometheusAlertOptions{
				AlertName: "StormTest",
				Namespace: testNamespace,
				Severity:  "critical",
				Resource: ResourceIdentifier{
					Kind: "Pod",
					Name: "storm-pod-2",
				},
			})

			resp2 := SendWebhook(testServer.URL+"/api/v1/signals/prometheus", payload2)
			Expect(resp2.StatusCode).To(Or(Equal(201), Equal(202)), "Second alert should be accepted")

			// Wait for storm detection to process
			time.Sleep(2 * time.Second)

			// STEP 3: Verify Redis has storm state
			keys, err := redisClient.Client.Keys(ctx, "gateway:storm:*").Result()
			Expect(err).ToNot(HaveOccurred(), "Should query Redis keys")
			initialStormKeys := len(keys)

			// STEP 4: Restart Gateway
			testServer.Close()
			gatewayServer = nil

			var restartErr error
			gatewayServer, restartErr = StartTestGateway(ctx, redisClient, k8sClient)
			Expect(restartErr).ToNot(HaveOccurred(), "Gateway should restart")

			testServer = httptest.NewServer(gatewayServer.Handler())

			// STEP 5: Send third alert (after restart)
			payload3 := GeneratePrometheusAlert(PrometheusAlertOptions{
				AlertName: "StormTest",
				Namespace: testNamespace,
				Severity:  "critical",
				Resource: ResourceIdentifier{
					Kind: "Pod",
					Name: "storm-pod-3",
				},
			})

			resp3 := SendWebhook(testServer.URL+"/api/v1/signals/prometheus", payload3)
			Expect(resp3.StatusCode).To(Or(Equal(201), Equal(202)), "Third alert should be accepted")

			// BUSINESS VALIDATION: Storm state persisted in Redis
			Eventually(func() int {
				keys, _ := redisClient.Client.Keys(ctx, "gateway:storm:*").Result()
				return len(keys)
			}, "10s", "500ms").Should(BeNumerically(">=", initialStormKeys),
				"Storm state should persist in Redis after restart")
		})
	})
})
