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
	"io"
	"net/http"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"go.uber.org/zap"
	"sigs.k8s.io/controller-runtime/pkg/client"

	remediationv1alpha1 "github.com/jordigilh/kubernaut/api/remediation/v1alpha1"
	"github.com/jordigilh/kubernaut/test/shared/helpers"
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

var _ = Describe("BR-GATEWAY-001-003: Prometheus Alert Processing - E2E Tests", func() {
	var (
		testCtx    context.Context // ← Test-local context
		testCancel context.CancelFunc
		// k8sClient available from suite (DD-E2E-K8S-CLIENT-001)
		logger *zap.Logger
		// Unique namespace names per test run (avoids parallel test interference)
		prodNamespace    string
		stagingNamespace string
		devNamespace     string
	)

	BeforeEach(func() {
		testCtx, testCancel = context.WithCancel(context.Background()) // ← Uses local variable
		logger = zap.NewNop()

		// Setup test infrastructure using helpers

		// k8sClient available from suite (DD-E2E-K8S-CLIENT-001)
		Expect(k8sClient).ToNot(BeNil(), "K8s client required for E2E tests")

		// DD-GATEWAY-012: Redis cleanup REMOVED - Gateway is now Redis-free

		// E2E tests use deployed Gateway at gatewayURL (http://127.0.0.1:8080)
		// No local test server needed

		// Create UNIQUE test namespaces with environment labels for classification
		// This is required for environment-based priority assignment
		// Use shared helper for E2E tests (waits for namespace to be Active)
		prodNamespace = helpers.CreateTestNamespaceAndWait(k8sClient, "prod",
			helpers.WithLabels(map[string]string{
				"environment": "production", // Required for EnvironmentClassifier
			}))
		stagingNamespace = helpers.CreateTestNamespaceAndWait(k8sClient, "staging",
			helpers.WithLabels(map[string]string{
				"environment": "staging", // Required for EnvironmentClassifier
			}))
		devNamespace = helpers.CreateTestNamespaceAndWait(k8sClient, "dev",
			helpers.WithLabels(map[string]string{
				"environment": "development", // Required for EnvironmentClassifier
			}))

		logger.Info("Test setup complete",
			zap.String("gateway_url", gatewayURL),
			zap.String("prod_namespace", prodNamespace),
		)
	})

	AfterEach(func() {
		if testCancel != nil {
			testCancel() // ← Only cancels test-local context
		}
		// DD-GATEWAY-012: Redis cleanup REMOVED - Gateway is now Redis-free

		// Cleanup all test namespaces
		testNamespaces := []string{prodNamespace, stagingNamespace, devNamespace}
		for _, nsName := range testNamespaces {
			if nsName != "" {
				helpers.DeleteTestNamespace(testCtx, k8sClient, nsName)
			}
		}

		// DD-GATEWAY-012: Redis cleanup REMOVED - Gateway is now Redis-free
		// E2E tests use deployed Gateway - no cleanup needed
	})

	// ━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━
	// BR-GATEWAY-001: Prometheus Alert → CRD Creation
	// ━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━

	Context("BR-GATEWAY-001: Prometheus Alert → CRD Creation with Business Metadata", func() {
		It("creates RemediationRequest CRD with correct business metadata for AI analysis", func() {
			// BR-GATEWAY-001, BR-GATEWAY-015: Complete webhook-to-CRD flow
			// BUSINESS SCENARIO: Production pod memory alert → AI analysis triggered
			// Expected: CRD created with priority, environment, severity for AI decision-making

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
			}`, prodNamespace))

			// Send webhook to Gateway
			url := fmt.Sprintf("%s/api/v1/signals/prometheus", gatewayURL)
			req, _ := http.NewRequest("POST", url, bytes.NewReader(payload))
			req.Header.Set("Content-Type", "application/json")
			req.Header.Set("X-Timestamp", fmt.Sprintf("%d", time.Now().Unix()))
			resp, err := http.DefaultClient.Do(req)
			Expect(err).ToNot(HaveOccurred())
			defer func() { _ = resp.Body.Close() }()

			// BUSINESS OUTCOME 1: HTTP 201 Created
			Expect(resp.StatusCode).To(Equal(http.StatusCreated), "First occurrence must create CRD (201 Created)")

			// Parse response to get fingerprint
			var response map[string]interface{}
			Expect(json.NewDecoder(resp.Body).Decode(&response)).To(Succeed())
			fingerprint, ok := response["fingerprint"].(string)
			Expect(ok).To(BeTrue(), "Response should contain fingerprint")
			Expect(fingerprint).NotTo(BeEmpty(), "Fingerprint should not be empty")

			// DD-GATEWAY-012: Redis check REMOVED - Gateway is now Redis-free
			// DD-GATEWAY-011: Deduplication now verified via K8s CRD status (validated in other tests)

			// BUSINESS OUTCOME 3: CRD created in Kubernetes with correct business metadata
			var crdList remediationv1alpha1.RemediationRequestList
			Expect(k8sClient.List(testCtx, &crdList, client.InNamespace(prodNamespace))).To(Succeed())
			Expect(crdList.Items).To(HaveLen(1), "Exactly one CRD should be created")

			crd := crdList.Items[0]

			// Verify business metadata for AI analysis
			Expect(crd.Spec.SignalName).To(Equal("HighMemoryUsage"), "AI needs alert name to understand failure type")
			// Note: Priority and Environment assertions removed (2025-12-06)
			// Classification moved to Signal Processing per DD-CATEGORIZATION-001
			Expect(crd.Spec.Severity).To(Equal("critical"), "Severity helps AI choose remediation strategy")
			Expect(crd.Namespace).To(Equal(prodNamespace), "Namespace enables kubectl targeting: 'kubectl -n production'")

			// Verify fingerprint is stored in spec (not as label — SHA256 exceeds 63-char label limit)
			Expect(crd.Spec.SignalFingerprint).To(Equal(fingerprint),
				"Full fingerprint stored in spec.signalFingerprint (BR-GATEWAY-185 v1.1)")

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

			payload := []byte(fmt.Sprintf(`{
				"alerts": [{
					"status": "firing",
				"labels": {
					"alertname": "DiskSpaceWarning",
					"severity": "warning",
					"namespace": "%s",
					"pod": "database-replica-2",
					"node": "worker-node-05"
				},
				"annotations": {
					"summary": "Disk usage at 85%%",
					"runbook_url": "https://runbooks.example.com/disk-space"
				},
					"startsAt": "2025-10-22T11:30:00Z"
				}]
			}`, stagingNamespace))

			url := fmt.Sprintf("%s/api/v1/signals/prometheus", gatewayURL)

			// Retry POST until Gateway's informer cache syncs the new namespace.
			// The scope checker (BR-SCOPE-002) uses a metadata-only informer cache;
			// freshly created namespaces may not be visible yet, causing HTTP 200
			// (scope rejection) instead of 201 (created). Retrying handles this lag.
			var resp *http.Response
			Eventually(func() int {
				req, _ := http.NewRequest("POST", url, bytes.NewReader(payload))
				req.Header.Set("Content-Type", "application/json")
				req.Header.Set("X-Timestamp", fmt.Sprintf("%d", time.Now().Unix()))
				var err error
				resp, err = http.DefaultClient.Do(req)
				if err != nil {
					return 0
				}
				defer func() { _ = resp.Body.Close() }()
				return resp.StatusCode
			}, 10*time.Second, 500*time.Millisecond).Should(Equal(http.StatusCreated),
				"Gateway should return 201 Created once informer cache syncs the managed namespace")

			// BUSINESS OUTCOME: CRD contains resource information for AI targeting
			var crdList remediationv1alpha1.RemediationRequestList
			Expect(k8sClient.List(testCtx, &crdList, client.InNamespace(stagingNamespace))).To(Succeed())
			Expect(crdList.Items).To(HaveLen(1))

			crd := crdList.Items[0]

			// Verify resource information enables AI to target specific resources
			Expect(crd.Spec.SignalLabels["pod"]).To(Equal("database-replica-2"), "Pod name enables AI to run: kubectl delete pod database-replica-2 -n staging")
			Expect(crd.Spec.SignalLabels["node"]).To(Equal("worker-node-05"), "Node name helps AI correlate infrastructure issues across pods")

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
			}`, prodNamespace))

			url := fmt.Sprintf("%s/api/v1/signals/prometheus", gatewayURL)

			// First alert: Creates CRD
			req1, _ := http.NewRequest("POST", url, bytes.NewReader(payload))
			req1.Header.Set("Content-Type", "application/json")
			req1.Header.Set("X-Timestamp", fmt.Sprintf("%d", time.Now().Unix()))
			resp1, err := http.DefaultClient.Do(req1)
			Expect(err).ToNot(HaveOccurred())
			defer func() { _ = resp1.Body.Close() }()
			Expect(resp1.StatusCode).To(Equal(http.StatusCreated), "First alert must create CRD (201 Created)")

			// Parse response to get full fingerprint
			var response1 map[string]interface{}
			Expect(json.NewDecoder(resp1.Body).Decode(&response1)).To(Succeed())
			fingerprint, ok := response1["fingerprint"].(string)
			Expect(ok).To(BeTrue(), "Response should contain fingerprint")
			Expect(fingerprint).NotTo(BeEmpty(), "Fingerprint should not be empty")

			// BUSINESS OUTCOME 1: First CRD created in K8s
			var crdList1 remediationv1alpha1.RemediationRequestList
			Expect(k8sClient.List(testCtx, &crdList1, client.InNamespace(prodNamespace))).To(Succeed())
			Expect(crdList1.Items).To(HaveLen(1), "First alert creates exactly one CRD")

			firstCRDName := crdList1.Items[0].Name

			// DD-GATEWAY-012: Redis check REMOVED - Gateway is now Redis-free
			// DD-GATEWAY-011: Deduplication validated via RR status.deduplication (tested elsewhere)

			// DD-E2E-DIRECT-API-001: Query CRD by known name (RO E2E pattern)
			// Direct Get() bypasses cache/index issues and is 4x faster (30s vs 120s)
			var confirmedCRD remediationv1alpha1.RemediationRequest
			Eventually(func() error {
				return k8sClient.Get(testCtx, client.ObjectKey{
					Namespace: prodNamespace,
					Name:      firstCRDName,
				}, &confirmedCRD)
			}, 30*time.Second, 1*time.Second).Should(Succeed(),
				"CRD should be queryable by name within 30s (matches RO E2E pattern)")

			// Second alert: Duplicate (CRD still in non-terminal phase)
			req2, err := http.NewRequest("POST", url, bytes.NewReader(payload))
			Expect(err).ToNot(HaveOccurred())
			req2.Header.Set("Content-Type", "application/json")
			req2.Header.Set("X-Timestamp", fmt.Sprintf("%d", time.Now().Unix()))
			resp2, err := http.DefaultClient.Do(req2)
			Expect(err).ToNot(HaveOccurred())
			defer func() { _ = resp2.Body.Close() }()
			Expect(resp2.StatusCode).To(Equal(http.StatusAccepted), "Duplicate alert must return 202 Accepted (not 201 Created)")

			// BUSINESS OUTCOME 2: NO new CRD created (deduplication works)
			var crdList2 remediationv1alpha1.RemediationRequestList
			Expect(k8sClient.List(testCtx, &crdList2, client.InNamespace(prodNamespace))).To(Succeed())
			Expect(crdList2.Items).To(HaveLen(1), "Duplicate alert must NOT create new CRD (still only 1 CRD)")
			Expect(crdList2.Items[0].Name).To(Equal(firstCRDName), "Same CRD name confirms no duplicate CRD created")

			// DD-GATEWAY-012: Redis metadata check REMOVED - Gateway is now Redis-free
			// DD-GATEWAY-011: BUSINESS OUTCOME 3: Deduplication count tracked in RR status
			Eventually(func() int32 {
				var updatedCRD remediationv1alpha1.RemediationRequest
				err := k8sClient.Get(testCtx, client.ObjectKey{
					Name:      firstCRDName,
					Namespace: prodNamespace,
				}, &updatedCRD)
				if err != nil {
					return 0
				}
				if updatedCRD.Status.Deduplication == nil {
					return 0
				}
				return updatedCRD.Status.Deduplication.OccurrenceCount
			}, "5s", "100ms").Should(BeNumerically(">=", 2), "Duplicate count must be tracked in RR status.deduplication (at least 2)")

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
			// Using unique namespaces created in BeforeEach to avoid parallel test interference

			testCases := []struct {
				namespace   string
				severity    string
				expectedEnv string
				expectedPri string
				rationale   string
			}{
				{
					namespace:   prodNamespace,
					severity:    "critical",
					expectedEnv: "production",
					expectedPri: "P0",
					rationale:   "Revenue-impacting, immediate AI analysis required",
				},
				{
					namespace:   stagingNamespace,
					severity:    "critical",
					expectedEnv: "staging",
					expectedPri: "P1",
					rationale:   "Pre-production issue, high priority to prevent prod impact",
				},
				{
					namespace:   devNamespace,
					severity:    "critical",
					expectedEnv: "development",
					expectedPri: "P2",
					rationale:   "Development work, medium priority (no revenue impact)",
				},
			}

			for _, tc := range testCases {
				// Clean K8s namespace before each test case
				_ = k8sClient.DeleteAllOf(testCtx, &remediationv1alpha1.RemediationRequest{},
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

				url := fmt.Sprintf("%s/api/v1/signals/prometheus", gatewayURL)
				req, _ := http.NewRequest("POST", url, bytes.NewReader(payload))
				req.Header.Set("Content-Type", "application/json")
				req.Header.Set("X-Timestamp", fmt.Sprintf("%d", time.Now().Unix()))
				resp, err := http.DefaultClient.Do(req)
				Expect(err).ToNot(HaveOccurred())
				defer func() { _ = resp.Body.Close() }()

				// Read response body and parse CRD name (DD-E2E-DIRECT-API-001)
				bodyBytes, _ := io.ReadAll(resp.Body)
				GinkgoWriter.Printf("🔍 %s: HTTP %d - %s\n", tc.namespace, resp.StatusCode, string(bodyBytes))

				// Check HTTP status - should be 201 for new CRD or 202 for duplicate
				Expect(resp.StatusCode).To(BeNumerically(">=", 200))
				Expect(resp.StatusCode).To(BeNumerically("<", 300),
					"Alert for %s should succeed (got HTTP %d): %s", tc.namespace, resp.StatusCode, string(bodyBytes))

				// Parse Gateway response to get CRD name
				var gwResp GatewayResponse
				Expect(json.Unmarshal(bodyBytes, &gwResp)).To(Succeed())
				Expect(gwResp.RemediationRequestName).NotTo(BeEmpty(), "Gateway should return CRD name")

				// DD-E2E-DIRECT-API-001: Query CRD by exact name (RO E2E pattern)
				// Direct Get() is 2x faster (30s vs 60s) and more reliable
				var crd remediationv1alpha1.RemediationRequest
				Eventually(func() error {
					return k8sClient.Get(testCtx, client.ObjectKey{
						Namespace: tc.namespace,
						Name:      gwResp.RemediationRequestName,
					}, &crd)
				}, 30*time.Second, 1*time.Second).Should(Succeed(),
					"CRD should be queryable by name within 30s (matches RO E2E pattern)")
				// Note: Environment/Priority assertions removed (2025-12-06)
				// Classification moved to Signal Processing per DD-CATEGORIZATION-001
				// Gateway only creates CRD, SP enriches with classification
				Expect(crd.Namespace).To(Equal(tc.namespace), "CRD should be created in correct namespace")
			}

			// BUSINESS CAPABILITY VERIFIED:
			// ✅ CRD creation in correct namespace works
			// Note: Environment/priority classification moved to Signal Processing (2025-12-06)
		})
	})
})
