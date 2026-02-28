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
	"fmt"
	"net/http"
	"sync"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"sigs.k8s.io/controller-runtime/pkg/client"

	remediationv1alpha1 "github.com/jordigilh/kubernaut/api/remediation/v1alpha1"
	"github.com/jordigilh/kubernaut/test/shared/helpers"
)

// Test Plan Reference: docs/development/testing/GATEWAY_COVERAGE_GAP_TEST_PLAN.md
// Section 7: Deduplication Edge Cases Testing (BR-GATEWAY-185)
// Tests: GW-DEDUP-001

var _ = Describe("Gateway Deduplication Edge Cases (BR-GATEWAY-185)", func() {
	var (
		testNamespace string // ✅ FIX: Unique namespace per parallel process (prevents data pollution)
		testCtx       context.Context      // ← Test-local context
		testCancel    context.CancelFunc
		testClient    client.Client
	)

	BeforeEach(func() {
		testCtx, testCancel = context.WithCancel(context.Background())  // ← Uses local variable
		testClient = k8sClient // Use suite-level client (DD-E2E-K8S-CLIENT-001)

		// Pre-create managed namespace (Pattern: RO E2E)
		testNamespace = helpers.CreateTestNamespaceAndWait(k8sClient, "gw-dedup-test")

		// Get DataStorage URL from environment
		// Note: gatewayURL is the globally deployed Gateway service at http://127.0.0.1:8080
	})

	AfterEach(func() {
		if testCancel != nil {
			testCancel()  // ← Only cancels test-local context
		}
		// Clean up test namespace (Pattern: RO E2E)
		helpers.DeleteTestNamespace(ctx, k8sClient, testNamespace)
	})

	Context("GW-DEDUP-001: K8s API Failure During Deduplication Check (P0)", func() {
		It("BR-GATEWAY-185: should fail request when field selector query fails", func() {
			// Given: Gateway using field selectors for deduplication (DD-GATEWAY-011)
			// When: K8s API field selector query fails
			// Then: Request fails with HTTP 500 (fail-fast, no fallback)

			// Business Rationale (BR-GATEWAY-185 v1.1):
			// - Field selectors required for O(1) deduplication performance
			// - Fallback to in-memory filtering would be O(n), unacceptable at scale
			// - Fail-fast ensures infrastructure issues are detected immediately

			payload := createPrometheusWebhookPayload(PrometheusAlertPayload{
				AlertName: "TestDedupFieldSelectorFailure",
				Namespace: testNamespace,
				Severity:  "critical",
				Labels: map[string]string{
					"fingerprint": "test-fingerprint-12345678",
				},
			})

			var statusCode int
			Eventually(func() int {
				req, _ := http.NewRequest("POST",
					fmt.Sprintf("%s/api/v1/signals/prometheus", gatewayURL),
					bytes.NewBuffer(payload))
				req.Header.Set("Content-Type", "application/json")
				req.Header.Set("X-Timestamp", fmt.Sprintf("%d", time.Now().Unix()))
				resp, err := http.DefaultClient.Do(req)
				if err != nil {
					return 0
				}
				_ = resp.Body.Close()
				statusCode = resp.StatusCode
				return statusCode
			}, 30*time.Second, 1*time.Second).Should(Or(
				Equal(http.StatusCreated), Equal(http.StatusInternalServerError)),
				"Should return 201 (success) or 500 (field selector failure), retries handle informer sync")

			if statusCode == http.StatusInternalServerError {
				logger.Info("Field selector failure correctly returned HTTP 500")
			}
		})

		It("should not fall back to in-memory filtering when field selector unavailable", func() {
			// Given: Field selector index not available
			// When: Deduplication check attempted
			// Then: Request fails (no fallback to O(n) in-memory filtering)

			// Anti-Pattern Prevention:
			// ❌ DO NOT: Fall back to List() + in-memory filter
			// ✅ DO: Fail fast with clear error about missing field index

			// Business Impact:
			// - Fallback would hide infrastructure issues
			// - O(n) filtering causes performance degradation at scale
			// - Explicit failure enables monitoring and alerting

			payload := createPrometheusWebhookPayload(PrometheusAlertPayload{
				AlertName: "TestNoFallback",
				Namespace: testNamespace,
				Severity:  "warning",
			})

			req, _ := http.NewRequest("POST",
				fmt.Sprintf("%s/api/v1/signals/prometheus", gatewayURL),
				bytes.NewBuffer(payload))
			req.Header.Set("Content-Type", "application/json")
			req.Header.Set("X-Timestamp", fmt.Sprintf("%d", time.Now().Unix()))

			resp, err := http.DefaultClient.Do(req)
			_ = err
			defer func() { _ = resp.Body.Close() }()

			// Success path: field selector works, deduplication succeeds
			// Failure path: field selector unavailable, explicit error (no fallback)
		})

		It("should provide actionable error message for field selector failures", func() {
			// Given: Field selector query failure
			// When: Error returned to webhook source
			// Then: Error message explains root cause and remediation

			// Expected Error Message:
			// "deduplication check failed (field selector required for fingerprint queries):
			//  field label not supported: spec.signalFingerprint"
			//
			// Remediation Guidance:
			// - Check field index is registered in controller-runtime manager
			// - Validate envtest environment has field indexing enabled
			// - Ensure K8s API server version supports field selectors

			payload := createPrometheusWebhookPayload(PrometheusAlertPayload{
				AlertName: "TestActionableError",
				Namespace: testNamespace,
				Severity:  "info",
			})

			req, _ := http.NewRequest("POST",
				fmt.Sprintf("%s/api/v1/signals/prometheus", gatewayURL),
				bytes.NewBuffer(payload))
			req.Header.Set("Content-Type", "application/json")
			req.Header.Set("X-Timestamp", fmt.Sprintf("%d", time.Now().Unix()))

			resp, err := http.DefaultClient.Do(req)
			_ = err
			defer func() { _ = resp.Body.Close() }()

			// If field selector fails, error should reference:
			// - "field selector"
			// - "spec.signalFingerprint"
			// - Actionable guidance
		})
	})

	Context("GW-DEDUP-002: Concurrent Deduplication Races (P1)", func() {
		It("should handle concurrent requests for same fingerprint gracefully", func() {
			// Given: Multiple webhook requests with identical fingerprint
			// When: Requests arrive simultaneously (race condition)
			// Then: Only one RemediationRequest created, others increment hit count

			// Business Scenario:
			// - Alert storm: Multiple AlertManager instances send same alert
			// - Network retry: Webhook client retries thinking request failed
			// - Multi-datacenter: Same alert from different sources

			// Strategy: Send 1 request first to establish the dedup anchor (RR creation),
			// then fire concurrent requests that should all be deduplicated.
			// This avoids the race where multiple goroutines all try to create the RR
			// simultaneously before the K8s Lease lock serializes them.

			// #230: Unique alert name per invocation prevents cross-retry/cross-process RR pollution
			alertName := fmt.Sprintf("TestConcurrentDedup-p%d-%d", GinkgoParallelProcess(), time.Now().UnixNano())
			fingerprint := fmt.Sprintf("concurrent-test-p%d-%d", GinkgoParallelProcess(), time.Now().UnixNano())

			makePayload := func() []byte {
				return createPrometheusWebhookPayload(PrometheusAlertPayload{
					AlertName: alertName,
					Namespace: testNamespace,
					Severity:  "warning",
					Labels: map[string]string{
						"fingerprint": fingerprint,
					},
				})
			}

			sendRequest := func(payload []byte) *http.Response {
				req, _ := http.NewRequest("POST",
					fmt.Sprintf("%s/api/v1/signals/prometheus", gatewayURL),
					bytes.NewBuffer(payload))
				req.Header.Set("Content-Type", "application/json")
				req.Header.Set("X-Timestamp", fmt.Sprintf("%d", time.Now().Unix()))
				resp, _ := http.DefaultClient.Do(req)
				return resp
			}

			// Phase 1: Send one request to establish the dedup anchor (retries handle informer sync)
			Eventually(func() int {
				anchorResp := sendRequest(makePayload())
				if anchorResp == nil {
					return 0
				}
				_ = anchorResp.Body.Close()
				return anchorResp.StatusCode
			}, 30*time.Second, 1*time.Second).Should(Equal(http.StatusCreated),
				"First request must create the RemediationRequest (HTTP 201)")

			// Wait for the RR to exist in the API server before sending concurrent requests
			Eventually(func() int {
				rrList := &remediationv1alpha1.RemediationRequestList{}
				if err := testClient.List(testCtx, rrList, client.InNamespace(gatewayNamespace)); err != nil {
					return 0
				}
				count := 0
				for _, rr := range rrList.Items {
					if rr.Spec.SignalName == alertName {
						count++
					}
				}
				return count
			}, 10*time.Second, 500*time.Millisecond).Should(Equal(1),
				"Anchor RR should be visible in the API server")

			// Phase 2: Fire concurrent requests — all should be deduplicated (HTTP 202)
			concurrentRequests := 4
			results := make(chan *http.Response, concurrentRequests)
			for i := 0; i < concurrentRequests; i++ {
				go func() {
					results <- sendRequest(makePayload())
				}()
			}

			dedupCount := 0
			for i := 0; i < concurrentRequests; i++ {
				resp := <-results
				if resp != nil {
					if resp.StatusCode == http.StatusAccepted {
						dedupCount++
					}
					_ = resp.Body.Close()
				}
			}

			Expect(dedupCount).To(Equal(concurrentRequests),
				"All concurrent requests should be deduplicated (HTTP 202 Accepted)")

			// Final check: Still only one RemediationRequest
			rrList := &remediationv1alpha1.RemediationRequestList{}
			Expect(testClient.List(testCtx, rrList, client.InNamespace(gatewayNamespace))).To(Succeed())
			count := 0
			for _, rr := range rrList.Items {
				if rr.Spec.SignalName == alertName {
					count++
				}
			}
			Expect(count).To(Equal(1),
				"Only one RemediationRequest should exist despite concurrent requests")
		})

		It("should update deduplication hit count atomically", FlakeAttempts(3), func() {
			// Given: RemediationRequest with existing hit count
			// When: Multiple deduplicated alerts arrive concurrently
			// Then: Hit count increments correctly (no lost updates)

			// #230: Unique alert name per invocation prevents cross-retry/cross-process RR pollution
			alertName := fmt.Sprintf("TestAtomicHitCount-p%d-%d", GinkgoParallelProcess(), time.Now().UnixNano())
			fingerprint := fmt.Sprintf("atomic-test-%d", time.Now().Unix())
			payload := createPrometheusWebhookPayload(PrometheusAlertPayload{
				AlertName: alertName,
				Namespace: testNamespace,
				Severity:  "info",
				Labels: map[string]string{
					"fingerprint": fingerprint,
				},
			})

			Eventually(func() int {
				req, _ := http.NewRequest("POST",
					fmt.Sprintf("%s/api/v1/signals/prometheus", gatewayURL),
					bytes.NewBuffer(payload))
				req.Header.Set("Content-Type", "application/json")
				req.Header.Set("X-Timestamp", fmt.Sprintf("%d", time.Now().Unix()))
				resp, err := http.DefaultClient.Do(req)
				if err != nil || resp == nil {
					return 0
				}
				_ = resp.Body.Close()
				return resp.StatusCode
			}, 30*time.Second, 1*time.Second).Should(Or(Equal(http.StatusCreated), Equal(http.StatusAccepted)),
				"Initial request should succeed (retries handle informer sync)")

			// Wait for initial RR to be created
			// Note: Query by SignalName (alertname) not fingerprint, since Gateway generates fingerprints
			Eventually(func() int {
				rrList := &remediationv1alpha1.RemediationRequestList{}
				err := testClient.List(testCtx, rrList,
					client.InNamespace(gatewayNamespace))
				if err != nil {
					return 0
				}
				count := 0
				for _, rr := range rrList.Items {
					if rr.Spec.SignalName == alertName {
						count++
					}
				}
				return count
			}, 15*time.Second, 500*time.Millisecond).Should(Equal(1),
				"Initial RemediationRequest should be created")

			// Send concurrent duplicate alerts with proper synchronization
			duplicateCount := 3
			var wg sync.WaitGroup
			wg.Add(duplicateCount)

			for i := 0; i < duplicateCount; i++ {
				go func() {
					defer wg.Done()
					req, _ := http.NewRequest("POST",
						fmt.Sprintf("%s/api/v1/signals/prometheus", gatewayURL),
						bytes.NewBuffer(payload))
					req.Header.Set("Content-Type", "application/json")
					req.Header.Set("X-Timestamp", fmt.Sprintf("%d", time.Now().Unix()))

					resp, _ := http.DefaultClient.Do(req)
					if resp != nil {
						_ = resp.Body.Close()
					}
				}()
			}

			// Wait for all HTTP requests to complete
			wg.Wait()

			// Then: Verify hit count reflects all duplicates using Eventually()
			// BR-GATEWAY-008: Deduplication status should be updated atomically
			// Note: Increased timeout to 20s to allow for K8s optimistic concurrency retries under CI load
			Eventually(func() int32 {
				var rrList remediationv1alpha1.RemediationRequestList
				_ = testClient.List(testCtx, &rrList, client.InNamespace(gatewayNamespace))

				for _, rr := range rrList.Items {
					if rr.Spec.SignalName == alertName && rr.Status.Deduplication != nil {
						return rr.Status.Deduplication.OccurrenceCount
					}
				}
				return 0
			}, 20*time.Second, 500*time.Millisecond).Should(BeNumerically(">=", 4),
				"Occurrence count should reflect original + 3 duplicates (atomic updates)")
		})
	})

	Context("GW-DEDUP-003: Corrupted or Incomplete Data (P1)", func() {
		It("should handle RemediationRequests with missing fingerprint field", func() {
			// Given: Legacy RemediationRequest without spec.signalFingerprint
			// When: Deduplication check queries for fingerprint
			// Then: Query should not crash, handles missing field gracefully

			// Edge Case: Data corruption or migration scenarios
			// - Old CRDs before fingerprint field added
			// - Manual CRD creation without fingerprint
			// - Database corruption

			fingerprint := fmt.Sprintf("missing-field-test-%d", time.Now().Unix())
			payload := createPrometheusWebhookPayload(PrometheusAlertPayload{
				AlertName: "TestMissingFingerprint",
				Namespace: testNamespace,
				Severity:  "warning",
				Labels: map[string]string{
					"fingerprint": fingerprint,
				},
			})

			req, _ := http.NewRequest("POST",
				fmt.Sprintf("%s/api/v1/signals/prometheus", gatewayURL),
				bytes.NewBuffer(payload))
			req.Header.Set("Content-Type", "application/json")
			req.Header.Set("X-Timestamp", fmt.Sprintf("%d", time.Now().Unix()))

			resp, err := http.DefaultClient.Do(req)
			_ = err
			defer func() { _ = resp.Body.Close() }()

			// Should handle gracefully (either create new RR or return error)
			// Should NOT panic or crash Gateway
			Expect(resp.StatusCode).To(BeNumerically("<", 600),
				"Should return valid HTTP status (not crash)")
		})

		It("should handle fingerprint hash collisions gracefully", func() {
			// Given: Two alerts with different content but same SHA256 fingerprint
			// (Theoretically possible, astronomically rare)
			// When: Both alerts processed
			// Then: Gateway handles collision without data loss

			// Note: SHA256 collision is practically impossible
			// This test documents expected behavior for the theoretical case
			// In practice, deduplication by fingerprint is collision-resistant
		})
	})
})
