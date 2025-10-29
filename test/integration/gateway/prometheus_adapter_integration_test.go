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

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"go.uber.org/zap"
	corev1 "k8s.io/api/core/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"

	remediationv1alpha1 "github.com/jordigilh/kubernaut/api/remediation/v1alpha1"
	gateway "github.com/jordigilh/kubernaut/pkg/gateway"
)

// Business Outcome Testing: Test WHAT Prometheus alert processing enables
//
// ❌ WRONG: "Parse() returns struct with AlertName field" (tests implementation)
// ✅ RIGHT: "Prometheus alerts enable deduplication" (tests business outcome)
//
// These tests verify the COMPLETE business flow:
// 1. Prometheus alert arrives via webhook
// 2. Gateway creates CRD in Kubernetes
// 3. Fingerprint stored in Redis for deduplication
// 4. Environment classified from namespace
// 5. Priority assigned based on severity + environment
//
// This replaces the old unit tests in test/unit/gateway/adapters/prometheus_adapter_test.go
// which only tested struct field extraction (implementation logic).

var _ = Describe("BR-GATEWAY-001-003: Prometheus Alert Processing - Integration Tests", func() {
	var (
		ctx           context.Context
		gatewayServer *gateway.Server
		testServer    *httptest.Server
		redisClient   *RedisTestClient
		k8sClient     *K8sTestClient
		logger        *zap.Logger
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

		// Create Gateway server using helper
		gatewayServer, err = StartTestGateway(ctx, redisClient, k8sClient)
		Expect(err).ToNot(HaveOccurred(), "Failed to create Gateway server")
		Expect(gatewayServer).ToNot(BeNil(), "Gateway server should be created")

		// Create HTTP test server
		testServer = httptest.NewServer(gatewayServer.Handler())
		Expect(testServer).ToNot(BeNil(), "Test server should be created")

		// Create test namespace with environment label for classification
		// This is required for environment-based priority assignment
		// Delete first to ensure clean state (ignore error if doesn't exist)
		ns := &corev1.Namespace{}
		ns.Name = "production"
		_ = k8sClient.Client.Delete(ctx, ns)

		// Wait for deletion to complete (namespace deletion is asynchronous)
		Eventually(func() error {
			checkNs := &corev1.Namespace{}
			return k8sClient.Client.Get(ctx, client.ObjectKey{Name: "production"}, checkNs)
		}, "10s", "100ms").Should(HaveOccurred(), "Namespace should be deleted")

		// Recreate with correct label
		ns = &corev1.Namespace{}
		ns.Name = "production"
		ns.Labels = map[string]string{
			"environment": "production", // Required for EnvironmentClassifier
		}
		err = k8sClient.Client.Create(ctx, ns)
		Expect(err).ToNot(HaveOccurred(), "Should create production namespace with environment label")

		logger.Info("Test setup complete",
			zap.String("test_server_url", testServer.URL),
			zap.String("redis_addr", redisClient.Client.Options().Addr),
		)
	})

	AfterEach(func() {
		// Cleanup namespace
		ns := &corev1.Namespace{}
		ns.Name = "production"
		_ = k8sClient.Client.Delete(ctx, ns) // Ignore error if namespace doesn't exist

		// Cleanup test server and Redis
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

	Context("BR-GATEWAY-001: Prometheus Alert → CRD Creation with Business Metadata", func() {
		It("creates RemediationRequest CRD with correct business metadata for AI analysis", func() {
			// BR-GATEWAY-001, BR-GATEWAY-015: Complete webhook-to-CRD flow
			// BUSINESS SCENARIO: Production pod memory alert → AI analysis triggered
			// Expected: CRD created with priority, environment, severity for AI decision-making

			payload := []byte(`{
				"alerts": [{
					"status": "firing",
					"labels": {
						"alertname": "HighMemoryUsage",
						"severity": "critical",
						"namespace": "production",
						"pod": "payment-api-123"
					},
					"annotations": {
						"summary": "Pod payment-api-123 using 95% memory",
						"description": "Memory threshold exceeded, may cause OOM"
					},
					"startsAt": "2025-10-22T10:00:00Z"
				}]
			}`)

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

			// BUSINESS OUTCOME 3: CRD created in Kubernetes with correct business metadata
			var crdList remediationv1alpha1.RemediationRequestList
			err = k8sClient.Client.List(ctx, &crdList, client.InNamespace("production"))
			Expect(err).NotTo(HaveOccurred(), "Should list CRDs in production namespace")
			Expect(crdList.Items).To(HaveLen(1), "Exactly one CRD should be created")

			crd := crdList.Items[0]

			// Verify business metadata for AI analysis
			Expect(crd.Spec.SignalName).To(Equal("HighMemoryUsage"),
				"AI needs alert name to understand failure type")
			Expect(crd.Spec.Priority).To(Equal("P0"),
				"critical + production = P0 (revenue-impacting, immediate AI analysis)")
			Expect(crd.Spec.Environment).To(Equal("production"),
				"Environment classification drives priority assignment")
			Expect(crd.Spec.Severity).To(Equal("critical"),
				"Severity helps AI choose remediation strategy")
			Expect(crd.Namespace).To(Equal("production"),
				"Namespace enables kubectl targeting: 'kubectl -n production'")

			// Verify fingerprint label matches response fingerprint (truncated to K8s 63-char limit)
			fingerprintLabel := crd.Labels["kubernaut.io/signal-fingerprint"]
			expectedLabel := fingerprint
			if len(expectedLabel) > 63 {
				expectedLabel = expectedLabel[:63] // K8s label value max length
			}
			Expect(fingerprintLabel).To(Equal(expectedLabel),
				"CRD fingerprint label must match response fingerprint (truncated to 63 chars for K8s)")

			// BUSINESS CAPABILITY VERIFIED:
			// ✅ Prometheus alert → Gateway → CRD created with complete business metadata
			// ✅ AI receives all context needed for intelligent analysis (alert name, severity, priority, environment)
			// ✅ Fingerprint generation enables deduplication (stored in Redis)
			// ✅ Environment classification from namespace works (production → P0 priority)
		})

		It("extracts resource information for AI targeting and remediation", func() {
			// BR-GATEWAY-001: Resource info extraction for AI targeting
			// BUSINESS SCENARIO: Alert includes pod/node info → AI can target specific resources
			// Expected: CRD includes resource details for kubectl commands

			payload := []byte(`{
				"alerts": [{
					"status": "firing",
					"labels": {
						"alertname": "DiskSpaceWarning",
						"severity": "warning",
						"namespace": "staging",
						"pod": "database-replica-2",
						"node": "worker-node-05"
					},
					"annotations": {
						"summary": "Disk usage at 85%",
						"runbook_url": "https://runbooks.example.com/disk-space"
					},
					"startsAt": "2025-10-22T11:30:00Z"
				}]
			}`)

			url := fmt.Sprintf("%s/api/v1/signals/prometheus", testServer.URL)
			resp, err := http.Post(url, "application/json", bytes.NewReader(payload))
			Expect(err).ToNot(HaveOccurred())
			defer resp.Body.Close()

			Expect(resp.StatusCode).To(Equal(http.StatusCreated))

			// BUSINESS OUTCOME: CRD contains resource information for AI targeting
			var crdList remediationv1alpha1.RemediationRequestList
			err = k8sClient.Client.List(ctx, &crdList, client.InNamespace("staging"))
			Expect(err).NotTo(HaveOccurred())
			Expect(crdList.Items).To(HaveLen(1))

			crd := crdList.Items[0]

			// Verify resource information enables AI to target specific resources
			Expect(crd.Spec.SignalLabels["pod"]).To(Equal("database-replica-2"),
				"Pod name enables AI to run: kubectl delete pod database-replica-2 -n staging")
			Expect(crd.Spec.SignalLabels["node"]).To(Equal("worker-node-05"),
				"Node name helps AI correlate infrastructure issues across pods")

			// BUSINESS CAPABILITY VERIFIED:
			// ✅ Resource information extracted from alert labels
			// ✅ AI receives pod/node context for targeted remediation
			// ✅ kubectl commands can be generated from CRD resource info
		})
	})

	// ━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━
	// BR-GATEWAY-005: Deduplication Using Prometheus Alert Fingerprint
	// ━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━

	Context("BR-GATEWAY-005: Deduplication Prevents Duplicate CRDs", func() {
		It("prevents duplicate CRDs for identical Prometheus alerts using fingerprint", func() {
			// BR-GATEWAY-005, BR-GATEWAY-006: Fingerprint-based deduplication
			// BUSINESS SCENARIO: Same alert fires twice in 5 seconds → Only 1 CRD created
			// Expected: First alert creates CRD, second alert returns 202 Accepted, NO new CRD

			payload := []byte(`{
				"alerts": [{
					"status": "firing",
					"labels": {
						"alertname": "CPUThrottling",
						"severity": "warning",
						"namespace": "production",
						"pod": "api-gateway-7"
					},
					"annotations": {
						"summary": "CPU throttling detected"
					},
					"startsAt": "2025-10-22T12:00:00Z"
				}]
			}`)

			url := fmt.Sprintf("%s/api/v1/signals/prometheus", testServer.URL)

			// First alert: Creates CRD
			resp1, err := http.Post(url, "application/json", bytes.NewReader(payload))
			Expect(err).ToNot(HaveOccurred())
			defer resp1.Body.Close()
			Expect(resp1.StatusCode).To(Equal(http.StatusCreated),
				"First alert must create CRD (201 Created)")

			// BUSINESS OUTCOME 1: First CRD created in K8s
			var crdList1 remediationv1alpha1.RemediationRequestList
			err = k8sClient.Client.List(ctx, &crdList1, client.InNamespace("production"))
			Expect(err).NotTo(HaveOccurred())
			Expect(crdList1.Items).To(HaveLen(1), "First alert creates exactly one CRD")

			firstCRDName := crdList1.Items[0].Name
			fingerprint := crdList1.Items[0].Labels["kubernaut.io/fingerprint"]

			// Verify fingerprint stored in Redis
			exists, _ := redisClient.Client.Exists(ctx, "alert:fingerprint:"+fingerprint).Result()
			Expect(exists).To(Equal(int64(1)), "Fingerprint must be in Redis after first alert")

			// Second alert: Duplicate (within TTL)
			resp2, err := http.Post(url, "application/json", bytes.NewReader(payload))
			Expect(err).ToNot(HaveOccurred())
			defer resp2.Body.Close()
			Expect(resp2.StatusCode).To(Equal(http.StatusAccepted),
				"Duplicate alert must return 202 Accepted (not 201 Created)")

			// BUSINESS OUTCOME 2: NO new CRD created (deduplication works)
			var crdList2 remediationv1alpha1.RemediationRequestList
			err = k8sClient.Client.List(ctx, &crdList2, client.InNamespace("production"))
			Expect(err).NotTo(HaveOccurred())
			Expect(crdList2.Items).To(HaveLen(1),
				"Duplicate alert must NOT create new CRD (still only 1 CRD)")
			Expect(crdList2.Items[0].Name).To(Equal(firstCRDName),
				"Same CRD name confirms no duplicate CRD created")

			// BUSINESS OUTCOME 3: Redis metadata updated with duplicate count
			count, err := redisClient.Client.HGet(ctx, "alert:fingerprint:"+fingerprint, "count").Int()
			Expect(err).NotTo(HaveOccurred())
			Expect(count).To(BeNumerically(">=", 2),
				"Duplicate count must be tracked in Redis (at least 2)")

			// BUSINESS CAPABILITY VERIFIED:
			// ✅ Fingerprint generation enables deduplication
			// ✅ Duplicate alerts don't create duplicate CRDs (prevents K8s API spam)
			// ✅ Redis tracks duplicate count for operational visibility
			// ✅ HTTP status codes differentiate new (201) vs duplicate (202) alerts
		})
	})

	// ━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━
	// BR-GATEWAY-011: Environment Classification from Namespace
	// ━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━

	Context("BR-GATEWAY-011: Environment Classification Drives Priority Assignment", func() {
		It("classifies environment from namespace and assigns correct priority", func() {
			// BR-GATEWAY-011, BR-GATEWAY-020-021: Environment classification → Priority assignment
			// BUSINESS SCENARIO: Namespace determines environment → Affects priority → Affects AI resource allocation
			// Expected: production critical = P0, staging critical = P1, dev critical = P2

			testCases := []struct {
				namespace   string
				severity    string
				expectedEnv string
				expectedPri string
				rationale   string
			}{
				{
					namespace:   "production",
					severity:    "critical",
					expectedEnv: "production",
					expectedPri: "P0",
					rationale:   "Revenue-impacting, immediate AI analysis required",
				},
				{
					namespace:   "staging",
					severity:    "critical",
					expectedEnv: "staging",
					expectedPri: "P1",
					rationale:   "Pre-production issue, high priority to prevent prod impact",
				},
				{
					namespace:   "development",
					severity:    "critical",
					expectedEnv: "development",
					expectedPri: "P2",
					rationale:   "Development work, medium priority (no revenue impact)",
				},
			}

			for _, tc := range testCases {
				// Clean K8s namespace before each test case
				k8sClient.Client.DeleteAllOf(ctx, &remediationv1alpha1.RemediationRequest{},
					client.InNamespace(tc.namespace))

				payload := []byte(fmt.Sprintf(`{
					"alerts": [{
						"status": "firing",
						"labels": {
							"alertname": "TestAlert",
							"severity": "%s",
							"namespace": "%s",
							"pod": "test-pod"
						},
						"startsAt": "2025-10-22T14:00:00Z"
					}]
				}`, tc.severity, tc.namespace))

				url := fmt.Sprintf("%s/api/v1/signals/prometheus", testServer.URL)
				resp, err := http.Post(url, "application/json", bytes.NewReader(payload))
				Expect(err).ToNot(HaveOccurred())
				resp.Body.Close()

				// BUSINESS OUTCOME: CRD has correct environment and priority based on namespace
				var crdList remediationv1alpha1.RemediationRequestList
				err = k8sClient.Client.List(ctx, &crdList, client.InNamespace(tc.namespace))
				Expect(err).NotTo(HaveOccurred())
				Expect(crdList.Items).To(HaveLen(1),
					"Alert in %s namespace should create CRD", tc.namespace)

				crd := crdList.Items[0]
				Expect(crd.Spec.Environment).To(Equal(tc.expectedEnv),
					"Namespace '%s' → Environment '%s'", tc.namespace, tc.expectedEnv)
				Expect(crd.Spec.Priority).To(Equal(tc.expectedPri),
					"%s + %s → Priority %s (%s)", tc.severity, tc.expectedEnv, tc.expectedPri, tc.rationale)
			}

			// BUSINESS CAPABILITY VERIFIED:
			// ✅ Environment classification from namespace works correctly
			// ✅ Priority assignment uses environment (production critical = P0, staging critical = P1)
			// ✅ Business rules for resource allocation are enforced (P0 = immediate, P1 = high, P2 = medium)
		})
	})
})
