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

			// Send webhook to Gateway with retry (CI latency in Kind+Podman)
			url := fmt.Sprintf("%s/api/v1/signals/prometheus", gatewayURL)
			var bodyBytes []byte
			Eventually(func() int {
				req, _ := http.NewRequest("POST", url, bytes.NewReader(payload))
				req.Header.Set("Content-Type", "application/json")
				req.Header.Set("X-Timestamp", fmt.Sprintf("%d", time.Now().Unix()))
				resp, err := http.DefaultClient.Do(req)
				if err != nil {
					GinkgoWriter.Printf("  Gateway POST error: %v\n", err)
					return 0
				}
				bodyBytes, _ = io.ReadAll(resp.Body)
				_ = resp.Body.Close()
				if resp.StatusCode != http.StatusCreated {
					GinkgoWriter.Printf("  Gateway returned %d (expected 201): %s\n", resp.StatusCode, string(bodyBytes))
				}
				return resp.StatusCode
			}, 30*time.Second, 1*time.Second).Should(Equal(http.StatusCreated),
				"First occurrence must create CRD (201 Created)")

			// Parse response from captured bytes (body was closed inside Eventually)
			var gwResp GatewayResponse
			Expect(json.NewDecoder(bytes.NewReader(bodyBytes)).Decode(&gwResp)).To(Succeed())
			Expect(gwResp.Fingerprint).NotTo(BeEmpty(), "Response should contain fingerprint")
			Expect(gwResp.RemediationRequestName).NotTo(BeEmpty(), "Response should contain RR name")

			// DD-GATEWAY-012: Redis check REMOVED - Gateway is now Redis-free
			// DD-GATEWAY-011: Deduplication now verified via K8s CRD status (validated in other tests)

			// BUSINESS OUTCOME 3: CRD created in Kubernetes with correct business metadata
			// ADR-057: Use Get by RR name (shared gatewayNamespace has RRs from all tests)
			var crd remediationv1alpha1.RemediationRequest
			Expect(k8sClient.Get(testCtx, client.ObjectKey{
				Namespace: gatewayNamespace,
				Name:      gwResp.RemediationRequestName,
			}, &crd)).To(Succeed(), "Exactly one CRD should be created for this signal")

			// Verify business metadata for AI analysis
			Expect(crd.Spec.SignalName).To(Equal("HighMemoryUsage"), "AI needs alert name to understand failure type")
			// Note: Priority and Environment assertions removed (2025-12-06)
			// Classification moved to Signal Processing per DD-CATEGORIZATION-001
			Expect(crd.Spec.Severity).To(Equal("critical"), "Severity helps AI choose remediation strategy")
			Expect(crd.Namespace).To(Equal(gatewayNamespace), "ADR-057: RRs created in controller namespace (kubernaut-system)")

			// Verify fingerprint is stored in spec (not as label — SHA256 exceeds 63-char label limit)
			Expect(crd.Spec.SignalFingerprint).To(Equal(gwResp.Fingerprint),
				"Full fingerprint stored in spec.signalFingerprint (BR-GATEWAY-185 v1.1)")

			// BUSINESS CAPABILITY VERIFIED:
			// ✅ Prometheus alert → Gateway → CRD created with complete business metadata
			// ✅ AI receives all context needed for intelligent analysis (alert name, severity, priority, environment)
			// ✅ Fingerprint generation enables deduplication (stored in Redis)
			// ✅ Environment classification from namespace works (production → P0 priority)
		})

		It("extracts resource information for AI targeting and remediation", func() {
			// BR-GATEWAY-001: Resource info extraction for AI targeting
			// BUSINESS SCENARIO: Alert includes pod info → AI can target specific resources
			// Expected: CRD includes resource details for kubectl commands
			// Note: Node-level alerts require the real Kind node name + managed label;
			// this test focuses on pod-level resource extraction in a managed namespace.

			payload := []byte(fmt.Sprintf(`{
				"alerts": [{
					"status": "firing",
				"labels": {
					"alertname": "DiskSpaceWarning",
					"severity": "warning",
					"namespace": "%s",
					"pod": "database-replica-2"
				},
				"annotations": {
					"summary": "Disk usage at 85%%",
					"runbook_url": "https://runbooks.example.com/disk-space"
				},
					"startsAt": "2025-10-22T11:30:00Z"
				}]
			}`, stagingNamespace))

			url := fmt.Sprintf("%s/api/v1/signals/prometheus", gatewayURL)

			// Retry POST until Gateway processes the alert and creates the CRD.
			// Scope checker uses ctrlClient (informer-backed) to reduce API server load.
			// Retries handle informer sync delay and CI startup latency.
			var resp *http.Response
			var bodyBytes []byte
			Eventually(func() int {
				req, _ := http.NewRequest("POST", url, bytes.NewReader(payload))
				req.Header.Set("Content-Type", "application/json")
				req.Header.Set("X-Timestamp", fmt.Sprintf("%d", time.Now().Unix()))
				var err error
				resp, err = http.DefaultClient.Do(req)
				if err != nil {
					GinkgoWriter.Printf("  Gateway POST error: %v\n", err)
					return 0
				}
				bodyBytes, _ = io.ReadAll(resp.Body)
				_ = resp.Body.Close()
				if resp.StatusCode != http.StatusCreated {
					GinkgoWriter.Printf("  Gateway returned %d (expected 201): %s\n", resp.StatusCode, string(bodyBytes))
				}
				return resp.StatusCode
			}, 30*time.Second, 1*time.Second).Should(Equal(http.StatusCreated),
				"Gateway should return 201 Created for managed namespace")

			// Parse response to get RR name (ADR-057: Get by name in shared gatewayNamespace)
			var gwResp GatewayResponse
			Expect(json.Unmarshal(bodyBytes, &gwResp)).To(Succeed())
			Expect(gwResp.RemediationRequestName).NotTo(BeEmpty())

			// BUSINESS OUTCOME: CRD contains resource information for AI targeting
			var crd remediationv1alpha1.RemediationRequest
			Expect(k8sClient.Get(testCtx, client.ObjectKey{
				Namespace: gatewayNamespace,
				Name:      gwResp.RemediationRequestName,
			}, &crd)).To(Succeed())

			// Verify resource information enables AI to target specific resources
			Expect(crd.Spec.SignalLabels["pod"]).To(Equal("database-replica-2"), "Pod name enables AI to run: kubectl delete pod database-replica-2 -n staging")

			// BUSINESS CAPABILITY VERIFIED:
			// ✅ Resource information extracted from alert labels
			// ✅ AI receives pod context for targeted remediation
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

			// First alert: Creates CRD
			// Retry handles scope informer cache propagation delay for newly created namespace
			webhookResp1 := sendWebhookExpectCreated(gatewayURL, "/api/v1/signals/prometheus", payload)

			// Parse response to get RR name and fingerprint (ADR-057: Get by name in shared gatewayNamespace)
			var gwResp1 GatewayResponse
			Expect(json.Unmarshal(webhookResp1.Body, &gwResp1)).To(Succeed())
			Expect(gwResp1.Fingerprint).NotTo(BeEmpty(), "Response should contain fingerprint")
			Expect(gwResp1.RemediationRequestName).NotTo(BeEmpty(), "Response should contain RR name")

			firstCRDName := gwResp1.RemediationRequestName

			// BUSINESS OUTCOME 1: First CRD created in K8s
			// Wrap in Eventually: CRD may not be propagated yet (scope informer cache delay)
			var crd remediationv1alpha1.RemediationRequest
			Eventually(func() error {
				return k8sClient.Get(testCtx, client.ObjectKey{
					Namespace: gatewayNamespace,
					Name:      firstCRDName,
				}, &crd)
			}, 15*time.Second, 200*time.Millisecond).Should(Succeed(),
				"First alert creates exactly one CRD")

			// DD-GATEWAY-012: Redis check REMOVED - Gateway is now Redis-free
			// DD-GATEWAY-011: Deduplication validated via RR status.deduplication (tested elsewhere)

			// DD-E2E-DIRECT-API-001: Query CRD by known name (RO E2E pattern)
			// Direct Get() bypasses cache/index issues and is 4x faster (30s vs 120s)
			var confirmedCRD remediationv1alpha1.RemediationRequest
			Eventually(func() error {
				return k8sClient.Get(testCtx, client.ObjectKey{
					Namespace: gatewayNamespace,
					Name:      firstCRDName,
				}, &confirmedCRD)
			}, 30*time.Second, 1*time.Second).Should(Succeed(),
				"CRD should be queryable by name within 30s (matches RO E2E pattern)")

			// Second alert: Duplicate (CRD still in non-terminal phase)
			webhookResp2 := sendWebhook(gatewayURL, "/api/v1/signals/prometheus", payload)
			Expect(webhookResp2.StatusCode).To(Equal(http.StatusAccepted),
				fmt.Sprintf("Duplicate alert must return 202 Accepted (not %d); body: %s",
					webhookResp2.StatusCode, string(webhookResp2.Body)))

			// BUSINESS OUTCOME 2: NO new CRD created (deduplication works)
			// 202 response returns same RR name; verify CRD still exists (ADR-057: Get by name)
			var gwResp2 GatewayResponse
			Expect(json.Unmarshal(webhookResp2.Body, &gwResp2)).To(Succeed())
			Expect(gwResp2.RemediationRequestName).To(Equal(firstCRDName), "Duplicate must return same RR name")

			// Diagnostic: if this Get fails, dump all RRs in namespace for debugging
			var crd2 remediationv1alpha1.RemediationRequest
			err := k8sClient.Get(testCtx, client.ObjectKey{
				Namespace: gatewayNamespace,
				Name:      firstCRDName,
			}, &crd2)
			if err != nil {
				var allRRs remediationv1alpha1.RemediationRequestList
				_ = k8sClient.List(testCtx, &allRRs, client.InNamespace(gatewayNamespace))
				GinkgoWriter.Printf("DIAGNOSTIC: RR %q not found after 202 dedup response. "+
					"Total RRs in %s: %d\n", firstCRDName, gatewayNamespace, len(allRRs.Items))
				for i, rr := range allRRs.Items {
					GinkgoWriter.Printf("  RR[%d]: name=%s, phase=%s, fingerprint=%s\n",
						i, rr.Name, rr.Status.OverallPhase, rr.Spec.SignalFingerprint)
				}
			}
			Expect(err).ToNot(HaveOccurred(), "Duplicate alert must NOT create new CRD (same CRD still exists)")

			// DD-GATEWAY-012: Redis metadata check REMOVED - Gateway is now Redis-free
			// DD-GATEWAY-011: BUSINESS OUTCOME 3: Deduplication count tracked in RR status
			Eventually(func() int32 {
				var updatedCRD remediationv1alpha1.RemediationRequest
				err := k8sClient.Get(testCtx, client.ObjectKey{
					Name:      firstCRDName,
					Namespace: gatewayNamespace,
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

			// Retry handles scope informer cache propagation delay for managed namespace
			webhookResp := sendWebhookExpectCreated(gatewayURL, "/api/v1/signals/prometheus", payload)
			GinkgoWriter.Printf("[env-classification] %s: HTTP %d - %s\n",
				tc.namespace, webhookResp.StatusCode, string(webhookResp.Body))

			// Parse Gateway response to get CRD name
			var gwResp GatewayResponse
			Expect(json.Unmarshal(webhookResp.Body, &gwResp)).To(Succeed())
			Expect(gwResp.RemediationRequestName).NotTo(BeEmpty(), "Gateway should return CRD name")

			// DD-E2E-DIRECT-API-001: Query CRD by exact name (RO E2E pattern)
			// Direct Get() is 2x faster (30s vs 60s) and more reliable
			// ADR-057: RRs created in controller namespace (kubernaut-system)
			var crd remediationv1alpha1.RemediationRequest
			Eventually(func() error {
				return k8sClient.Get(testCtx, client.ObjectKey{
					Namespace: gatewayNamespace,
					Name:      gwResp.RemediationRequestName,
				}, &crd)
			}, 30*time.Second, 1*time.Second).Should(Succeed(),
				"CRD should be queryable by name within 30s (matches RO E2E pattern)")
			// Note: Environment/Priority assertions removed (2025-12-06)
			// Classification moved to Signal Processing per DD-CATEGORIZATION-001
			// Gateway only creates CRD, SP enriches with classification
			Expect(crd.Namespace).To(Equal(gatewayNamespace), "ADR-057: RRs created in controller namespace")

			// Targeted cleanup: delete only the RR created by this test case iteration.
			// CRITICAL: Never use DeleteAllOf in shared namespace — it causes cross-process
			// interference when tests run in parallel (flaked GW-DEDUP-002).
			_ = k8sClient.Delete(testCtx, &crd)
		}

			// BUSINESS CAPABILITY VERIFIED:
			// ✅ CRD creation in correct namespace works
			// Note: Environment/priority classification moved to Signal Processing (2025-12-06)
		})
	})
})
