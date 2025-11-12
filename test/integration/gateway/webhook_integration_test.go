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
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"go.uber.org/zap"
	"sigs.k8s.io/controller-runtime/pkg/client"

	remediationv1alpha1 "github.com/jordigilh/kubernaut/api/remediation/v1alpha1"
	gateway "github.com/jordigilh/kubernaut/pkg/gateway"
)

// Business Outcome Testing: Test WHAT complete webhook processing enables
//
// ❌ WRONG: "HTTP response contains status field" (tests implementation)
// ✅ RIGHT: "Prometheus alerts create RemediationRequest CRDs for AI analysis" (tests business outcome)
//
// These tests verify the COMPLETE end-to-end business flow:
// 1. Webhook arrives (Prometheus or K8s Event)
// 2. CRD created in Kubernetes with correct business metadata
// 3. Fingerprint stored in Redis for deduplication
// 4. Duplicate alerts return 202 and NO new CRD created
// 5. Storm detection aggregates multiple alerts into single CRD
//
// This REPLACES the old tests that only verified HTTP response body structure.

var _ = Describe("BR-GATEWAY-001-015: End-to-End Webhook Processing - Integration Tests", func() {
	var (
		ctx           context.Context
		gatewayServer *gateway.Server
		testServer    *httptest.Server
		redisClient   *RedisTestClient
		k8sClient     *K8sTestClient
		logger        *zap.Logger
		testNamespace string // Unique namespace per test
		testCounter   int    // Counter to ensure unique namespaces
	)

	BeforeEach(func() {
		ctx = context.Background()
		logger = zap.NewNop()

		// Setup test infrastructure using helpers
		redisClient = SetupRedisTestClient(ctx)
		Expect(redisClient).ToNot(BeNil(), "Redis client required for integration tests")
		Expect(redisClient.Client).ToNot(BeNil(), "Redis connection required")

		k8sClient = SetupK8sTestClient(ctx)
		Expect(k8sClient).ToNot(BeNil(), "K8s client required for integration tests")

		// Clean Redis before each test
		err := redisClient.Client.FlushDB(ctx).Err()
		Expect(err).ToNot(HaveOccurred(), "Should clean Redis before test")

		// Verify Redis is actually empty (prevent state leakage between tests)
		keys, err := redisClient.Client.Keys(ctx, "*").Result()
		Expect(err).ToNot(HaveOccurred(), "Should query Redis keys")
		Expect(keys).To(BeEmpty(), "Redis should be completely empty after FlushDB")

		// Create unique production namespace for this test (prevents collisions)
		// Use counter to ensure uniqueness even when tests run in same second
		testCounter++
		testNamespace = fmt.Sprintf("test-prod-%d-%d-%d", time.Now().UnixNano(), GinkgoRandomSeed(), testCounter)
		EnsureTestNamespace(ctx, k8sClient, testNamespace)

		// Register namespace for suite-level cleanup
		RegisterTestNamespace(testNamespace)

		// Create Gateway server using helper
		gatewayServer, err = StartTestGateway(ctx, redisClient, k8sClient)
		Expect(err).ToNot(HaveOccurred(), "Failed to create Gateway server")
		Expect(gatewayServer).ToNot(BeNil(), "Gateway server should be created")

		// Create HTTP test server
		testServer = httptest.NewServer(gatewayServer.Handler())
		Expect(testServer).ToNot(BeNil(), "Test server should be created")

		logger.Info("Test setup complete",
			zap.String("test_server_url", testServer.URL),
			zap.String("test_namespace", testNamespace),
			zap.String("redis_addr", redisClient.Client.Options().Addr),
		)
	})

	AfterEach(func() {
		// Reset Redis config to prevent OOM cascade failures
		if redisClient != nil && redisClient.Client != nil {
			redisClient.Client.ConfigSet(ctx, "maxmemory", "2147483648")
			redisClient.Client.ConfigSet(ctx, "maxmemory-policy", "allkeys-lru")
		}

		// NOTE: Namespace cleanup handled by suite-level AfterSuite (batch cleanup)
		// This prevents "namespace is being terminated" errors from parallel test execution

		// Cleanup
		if testServer != nil {
			testServer.Close()
		}
		if redisClient != nil && redisClient.Client != nil {
			_ = redisClient.Client.FlushDB(ctx).Err()
		}
	})

	// ━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━
	// BR-GATEWAY-001: Prometheus Alert → CRD Creation
	// ━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━

	Context("BR-GATEWAY-001: Prometheus Alert → CRD Creation", func() {
		It("creates RemediationRequest CRD from Prometheus AlertManager webhook", func() {
			// BR-GATEWAY-001, BR-GATEWAY-015: Complete webhook-to-CRD flow
			// BUSINESS SCENARIO: Production pod memory alert → AI analysis triggered
			// Expected: CRD created in K8s with correct priority and environment

			payload := []byte(fmt.Sprintf(`{
				"alerts": [{
					"status": "firing",
					"labels": {
						"alertname": "HighMemoryUsage",
						"severity": "critical",
						"namespace": "%s",
						"pod": "payment-api-123"
					},
					"annotations": {
						"summary": "Pod payment-api-123 using 95%% memory",
						"description": "Memory threshold exceeded, may cause OOM"
					},
					"startsAt": "2025-10-22T10:00:00Z"
				}]
			}`, testNamespace))

			// Send webhook to Gateway
			url := fmt.Sprintf("%s/api/v1/signals/prometheus", testServer.URL)
			resp, err := http.Post(url, "application/json", bytes.NewReader(payload))
			Expect(err).ToNot(HaveOccurred(), "HTTP request should succeed")
			defer resp.Body.Close()

			// BUSINESS OUTCOME 1: HTTP 201 Created
			Expect(resp.StatusCode).To(Equal(http.StatusCreated),
				"First occurrence must create CRD (201 Created)")

			// Parse response to get fingerprint (for Redis check before TTL expires)
			var response map[string]interface{}
			err = json.NewDecoder(resp.Body).Decode(&response)
			Expect(err).NotTo(HaveOccurred(), "Should parse JSON response")
			fingerprint, ok := response["fingerprint"].(string)
			Expect(ok).To(BeTrue(), "Response should contain fingerprint")
			Expect(fingerprint).NotTo(BeEmpty(), "Fingerprint should not be empty")

			// BUSINESS OUTCOME 2: Fingerprint stored in Redis for deduplication
			// Check Redis IMMEDIATELY after HTTP response (before 5-second TTL expires)
			exists, err := redisClient.Client.Exists(ctx, "gateway:dedup:fingerprint:"+fingerprint).Result()
			Expect(err).NotTo(HaveOccurred(), "Redis query should succeed")
			Expect(exists).To(Equal(int64(1)),
				"Fingerprint must be stored in Redis to enable deduplication")

			// BUSINESS OUTCOME 3: CRD created in Kubernetes
			var crdList remediationv1alpha1.RemediationRequestList
			err = k8sClient.Client.List(ctx, &crdList, client.InNamespace(testNamespace))
			Expect(err).NotTo(HaveOccurred(), "Should list CRDs in test namespace")
			Expect(crdList.Items).To(HaveLen(1), "One CRD should be created")

			crd := crdList.Items[0]
			Expect(crd.Spec.Priority).To(Equal("P0"),
				"critical + production = P0 (revenue-impacting)")
			Expect(crd.Spec.Environment).To(Equal("production"),
				"Environment should be classified from namespace label")
			Expect(crd.Spec.SignalName).To(Equal("HighMemoryUsage"),
				"Alert name enables AI to understand failure type")

			// Verify fingerprint label matches response fingerprint (truncated to K8s 63-char limit)
			fingerprintLabel := crd.Labels["kubernaut.io/signal-fingerprint"]
			expectedLabel := fingerprint
			if len(expectedLabel) > 63 {
				expectedLabel = expectedLabel[:63] // K8s label value max length
			}
			Expect(fingerprintLabel).To(Equal(expectedLabel),
				"CRD fingerprint label must match response fingerprint (truncated to 63 chars for K8s)")

			// BUSINESS CAPABILITY VERIFIED:
			// ✅ Prometheus alert → Gateway → CRD created
			// ✅ Priority assigned based on severity + environment
			// ✅ Environment classified from namespace
			// ✅ Fingerprint generated for deduplication
		})
	})

	// ━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━
	// BR-GATEWAY-003-005: Deduplication
	// ━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━

	Context("BR-GATEWAY-003-005: Deduplication", func() {
		It("returns 202 Accepted for duplicate alerts within TTL window", func() {
			// BR-GATEWAY-003-005: Duplicate detection prevents CRD spam
			// BUSINESS SCENARIO: Same alert fires 3 times in 5 seconds
			// Expected: First = 201 Created, subsequent = 202 Accepted, NO duplicate CRDs

			payload := []byte(fmt.Sprintf(`{
				"alerts": [{
					"status": "firing",
					"labels": {
						"alertname": "CPUThrottling",
						"severity": "warning",
						"namespace": "%s",
						"pod": "api-gateway-7"
					},
					"annotations": {
						"summary": "CPU throttling detected"
					},
					"startsAt": "2025-10-22T12:00:00Z"
				}]
			}`, testNamespace))

			url := fmt.Sprintf("%s/api/v1/signals/prometheus", testServer.URL)

			// First alert: Creates CRD
			resp1, err := http.Post(url, "application/json", bytes.NewReader(payload))
			Expect(err).ToNot(HaveOccurred())
			defer resp1.Body.Close()
			Expect(resp1.StatusCode).To(Equal(http.StatusCreated),
				"First alert must create CRD")

			// BUSINESS OUTCOME 1: First CRD created
			// Use Eventually to handle Kubernetes API caching/propagation delays
			var crdList1 remediationv1alpha1.RemediationRequestList
			var firstCRDName string
			Eventually(func() int {
				err = k8sClient.Client.List(ctx, &crdList1, client.InNamespace(testNamespace))
				Expect(err).NotTo(HaveOccurred())
				return len(crdList1.Items)
			}, "5s", "100ms").Should(Equal(1), "First alert creates CRD")

			firstCRDName = crdList1.Items[0].Name

			// Second alert: Duplicate (within TTL)
			resp2, err := http.Post(url, "application/json", bytes.NewReader(payload))
			Expect(err).ToNot(HaveOccurred())
			defer resp2.Body.Close()
			Expect(resp2.StatusCode).To(Equal(http.StatusAccepted),
				"Duplicate alert must return 202 Accepted")

			// BUSINESS OUTCOME 2: NO new CRD created
			var crdList2 remediationv1alpha1.RemediationRequestList
			err = k8sClient.Client.List(ctx, &crdList2, client.InNamespace(testNamespace))
			Expect(err).NotTo(HaveOccurred())
			Expect(crdList2.Items).To(HaveLen(1),
				"Duplicate alert must NOT create new CRD")
			Expect(crdList2.Items[0].Name).To(Equal(firstCRDName),
				"Same CRD name confirms deduplication")

			// Third alert: Still duplicate
			resp3, err := http.Post(url, "application/json", bytes.NewReader(payload))
			Expect(err).ToNot(HaveOccurred())
			defer resp3.Body.Close()
			Expect(resp3.StatusCode).To(Equal(http.StatusAccepted),
				"Third duplicate must also return 202 Accepted")

			// BUSINESS OUTCOME 3: Still only 1 CRD
			var crdList3 remediationv1alpha1.RemediationRequestList
			err = k8sClient.Client.List(ctx, &crdList3, client.InNamespace(testNamespace))
			Expect(err).NotTo(HaveOccurred())
			Expect(crdList3.Items).To(HaveLen(1),
				"Third duplicate must NOT create new CRD (still only 1 CRD)")

			// BUSINESS CAPABILITY VERIFIED:
			// ✅ Deduplication prevents CRD spam (1 CRD, not 3)
			// ✅ Duplicate alerts tracked but don't create new CRDs
		})

		It("tracks duplicate count and timestamps in Redis metadata", func() {
			// BR-GATEWAY-005: Duplicate metadata for operational visibility
			// BUSINESS SCENARIO: Alert fires 5 times → Ops sees escalation pattern
			// Expected: Redis metadata includes count, first seen, last seen

			payload := []byte(fmt.Sprintf(`{
				"alerts": [{
					"status": "firing",
					"labels": {
						"alertname": "NetworkLatency",
						"severity": "critical",
						"namespace": "%s"
					},
					"annotations": {
						"summary": "Network latency > 500ms"
					},
					"startsAt": "2025-10-22T13:00:00Z"
				}]
			}`, testNamespace))

		url := fmt.Sprintf("%s/api/v1/signals/prometheus", testServer.URL)

		// First alert
		resp1, err := http.Post(url, "application/json", bytes.NewReader(payload))
		Expect(err).NotTo(HaveOccurred(), "Should send first alert")
		defer resp1.Body.Close()

		// Parse response to get full fingerprint (before K8s label truncation)
		var response map[string]interface{}
		err = json.NewDecoder(resp1.Body).Decode(&response)
			Expect(err).NotTo(HaveOccurred(), "Should parse JSON response")
			fingerprint, ok := response["fingerprint"].(string)
			Expect(ok).To(BeTrue(), "Response should contain fingerprint")
			Expect(fingerprint).NotTo(BeEmpty(), "Fingerprint should not be empty")

		// Send 4 more duplicates
		for i := 0; i < 4; i++ {
			resp, err := http.Post(url, "application/json", bytes.NewReader(payload))
			Expect(err).NotTo(HaveOccurred(), "Should send duplicate alert")
			resp.Body.Close()
		}

			// BUSINESS OUTCOME: Redis metadata tracks duplicate count
			// Use Eventually because Redis writes are async
			Eventually(func() int {
				count, err := redisClient.Client.HGet(ctx, "gateway:dedup:fingerprint:"+fingerprint, "count").Int()
				if err != nil {
					return 0
				}
				return count
			}, "2s", "100ms").Should(BeNumerically(">=", 5),
				"Count shows alert fired 5 times (1 original + 4 duplicates)")

			firstSeen, err := redisClient.Client.HGet(ctx, "gateway:dedup:fingerprint:"+fingerprint, "firstSeen").Result()
			Expect(err).NotTo(HaveOccurred())
			Expect(firstSeen).NotTo(BeEmpty(),
				"First seen timestamp shows when issue started")

			lastSeen, err := redisClient.Client.HGet(ctx, "gateway:dedup:fingerprint:"+fingerprint, "lastSeen").Result()
			Expect(err).NotTo(HaveOccurred())
			Expect(lastSeen).NotTo(BeEmpty(),
				"Last seen timestamp shows issue is ongoing")

			// BUSINESS CAPABILITY VERIFIED:
			// ✅ Duplicate tracking provides operational visibility
			// ✅ Metadata helps ops understand alert escalation patterns
		})
	})

	// ━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━
	// BR-GATEWAY-013: Storm Detection
	// ━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━

	Context("BR-GATEWAY-013: Storm Detection", func() {
		It("aggregates multiple related alerts into single storm CRD", func() {
			// BR-GATEWAY-013: Storm detection prevents CRD flood
			// BUSINESS SCENARIO: Node failure → 15 pod alerts in 10 seconds
			// Expected: 3 CRDs (2 before storm + 1 aggregated), not 15 individual CRDs
			//
			// Storm detection flow:
			// - Alerts 1-2: Create individual CRDs (rate threshold not yet exceeded)
			// - Alert 3: Storm detected (rate > 2), start aggregation window
			// - Alerts 4-15: Added to aggregation window (no new CRDs)
			// - After 5 seconds: 1 aggregated storm CRD created with 13 alerts
			//
			// Business outcome: 87% reduction in K8s API load (3 CRDs vs 15)

			url := fmt.Sprintf("%s/api/v1/signals/prometheus", testServer.URL)

			// Simulate node failure: 15 pods on same node report issues
			for i := 1; i <= 15; i++ {
				payload := []byte(fmt.Sprintf(`{
					"alerts": [{
						"status": "firing",
						"labels": {
							"alertname": "PodNotReady",
							"severity": "critical",
							"namespace": "%s",
							"pod": "app-pod-%d",
							"node": "worker-node-03"
						},
						"annotations": {
							"summary": "Pod not ready after node failure"
						},
						"startsAt": "2025-10-22T14:00:00Z"
					}]
				}`, testNamespace, i))

				resp, err := http.Post(url, "application/json", bytes.NewReader(payload))
				Expect(err).ToNot(HaveOccurred())
				resp.Body.Close()
			}

			// Wait for storm aggregation window to complete (5 seconds + 1 second buffer)
			time.Sleep(6 * time.Second)

			// BUSINESS OUTCOME: Storm aggregation prevents CRD flood
			// Expected: 3 CRDs total (2 before storm threshold + 1 aggregated storm CRD)
			// - Alerts 1-2: Individual CRDs (before rate threshold of 2 is exceeded)
			// - Alerts 3-15: Aggregated into 1 storm CRD (after storm detection kicks in)
			var crdList remediationv1alpha1.RemediationRequestList
			err := k8sClient.Client.List(ctx, &crdList, client.InNamespace(testNamespace))
			Expect(err).NotTo(HaveOccurred())

			// Should have 3 CRDs (2 individual + 1 storm), not 15 individual CRDs
			Expect(crdList.Items).To(HaveLen(3),
				"Storm detection should create 3 CRDs (2 before storm + 1 aggregated), not 15")

			// Find the storm CRD (has kubernaut.io/storm label)
			var stormCRD *remediationv1alpha1.RemediationRequest
			for i := range crdList.Items {
				if crdList.Items[i].Labels["kubernaut.io/storm"] == "true" {
					stormCRD = &crdList.Items[i]
					break
				}
			}
			Expect(stormCRD).ToNot(BeNil(), "Should have 1 storm CRD with storm label")

			// Verify storm CRD aggregated 13 alerts (alerts 3-15)
			Expect(stormCRD.Spec.StormAlertCount).To(BeNumerically(">=", 13),
				"Storm CRD should aggregate alerts 3-15 (13 total)")
			Expect(stormCRD.Labels["kubernaut.io/storm"]).To(Equal("true"),
				"Storm label indicates aggregated CRD")

			// BUSINESS CAPABILITY VERIFIED:
			// ✅ Storm detection prevents K8s API overload (1 CRD, not 15)
			// ✅ Related alerts aggregated for efficient AI analysis
		})
	})

	// ━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━
	// BR-GATEWAY-002: Kubernetes Event Webhooks
	// ━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━

	Context("BR-GATEWAY-002: Kubernetes Event Webhooks", func() {
		It("creates CRD from Kubernetes Warning events", func() {
			// BR-GATEWAY-002: K8s events trigger remediation workflow
			// BUSINESS SCENARIO: Pod OOMKilled event → AI analyzes memory issue
			// Expected: CRD created with event details

			payload := []byte(fmt.Sprintf(`{
				"type": "Warning",
				"reason": "OOMKilled",
				"message": "Container killed due to out of memory",
				"involvedObject": {
					"kind": "Pod",
					"namespace": "%s",
					"name": "payment-processor-42"
				},
				"metadata": {
					"namespace": "%s"
				}
			}`, testNamespace, testNamespace))

			url := fmt.Sprintf("%s/api/v1/signals/kubernetes-event", testServer.URL)
			resp, err := http.Post(url, "application/json", bytes.NewReader(payload))
			Expect(err).ToNot(HaveOccurred())
			defer resp.Body.Close()

			// BUSINESS OUTCOME 1: HTTP 201 Created
			Expect(resp.StatusCode).To(Equal(http.StatusCreated),
				"Warning event must create CRD for AI analysis")

			// BUSINESS OUTCOME 2: CRD created in Kubernetes
			var crdList remediationv1alpha1.RemediationRequestList
			err = k8sClient.Client.List(ctx, &crdList, client.InNamespace(testNamespace))
			Expect(err).NotTo(HaveOccurred())
			Expect(crdList.Items).To(HaveLen(1), "K8s event should create CRD")

			crd := crdList.Items[0]
			Expect(crd.Spec.SignalName).To(Equal("OOMKilled"),
				"Event reason helps AI identify root cause")
			Expect(crd.Spec.SignalType).To(Equal("kubernetes-event"),
				"Signal type distinguishes K8s events from Prometheus alerts")

			// BUSINESS CAPABILITY VERIFIED:
			// ✅ K8s events trigger remediation workflow
			// ✅ Event details provide AI with root cause context
		})
	})
})
