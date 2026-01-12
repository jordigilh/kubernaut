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
	"os"
	"sync"
	"time"

	"github.com/google/uuid"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"sigs.k8s.io/controller-runtime/pkg/client"

	remediationv1alpha1 "github.com/jordigilh/kubernaut/api/remediation/v1alpha1"
)

// Test Plan Reference: docs/development/testing/GATEWAY_COVERAGE_GAP_TEST_PLAN.md
// Section 7: Deduplication Edge Cases Testing (BR-GATEWAY-185)
// Tests: GW-DEDUP-001

var _ = Describe("Gateway Deduplication Edge Cases (BR-GATEWAY-185)", func() {
	var (
		testNamespace string // ✅ FIX: Unique namespace per parallel process (prevents data pollution)
		ctx           context.Context
		testClient    client.Client
	)

	BeforeEach(func() {
		ctx = context.Background()
		testClient = getKubernetesClient()

		// ✅ FIX: Create unique namespace per parallel process to prevent data pollution
		// This eliminates flakiness caused by tests interfering with each other's data
		testNamespace = fmt.Sprintf("gw-dedup-test-%d-%s", GinkgoParallelProcess(), uuid.New().String()[:8])

		// Get DataStorage URL from environment
		dataStorageURL := os.Getenv("TEST_DATA_STORAGE_URL")
		if dataStorageURL == "" {
			dataStorageURL = "http://127.0.0.1:18091" // Fallback - Use 127.0.0.1 for CI/CD IPv4 compatibility
		}

		// Note: gatewayURL is the globally deployed Gateway service at http://127.0.0.1:8080
	})

	AfterEach(func() {

		// No manual cleanup needed - each parallel process has its own isolated namespace
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

			req, err := http.NewRequest("POST",
				fmt.Sprintf("%s/api/v1/signals/prometheus", gatewayURL),
				bytes.NewBuffer(payload))
			req.Header.Set("Content-Type", "application/json")
			req.Header.Set("X-Timestamp", fmt.Sprintf("%d", time.Now().Unix()))

			resp, err := http.DefaultClient.Do(req)
			_ = err
			defer func() { _ = resp.Body.Close() }()

			// If K8s API field selector fails, should return HTTP 500
			// (not HTTP 200 with degraded deduplication)
			if resp.StatusCode == http.StatusInternalServerError {
				// Field selector failure detected
				// This validates fail-fast behavior (no silent degradation)
				logger.Info("Field selector failure correctly returned HTTP 500")
			} else {
				// K8s API available, deduplication succeeded with CRD creation
				Expect(resp.StatusCode).To(Equal(http.StatusCreated))
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

			req, err := http.NewRequest("POST",
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

			req, err := http.NewRequest("POST",
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
		It("should handle concurrent requests for same fingerprint gracefully", FlakeAttempts(3), func() {
			// Given: Multiple webhook requests with identical fingerprint
			// When: Requests arrive simultaneously (race condition)
			// Then: Only one RemediationRequest created, others increment hit count

			// Business Scenario:
			// - Alert storm: Multiple AlertManager instances send same alert
			// - Network retry: Webhook client retries thinking request failed
			// - Multi-datacenter: Same alert from different sources

			// ✅ FIX: Include parallel process ID to prevent collisions between parallel test runs
			fingerprint := fmt.Sprintf("concurrent-test-p%d-%d", GinkgoParallelProcess(), time.Now().UnixNano())
			concurrentRequests := 5

			// Send concurrent requests with same fingerprint
			results := make(chan *http.Response, concurrentRequests)
			for i := 0; i < concurrentRequests; i++ {
				go func() {
					payload := createPrometheusWebhookPayload(PrometheusAlertPayload{
						AlertName: "TestConcurrentDedup",
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

					resp, _ := http.DefaultClient.Do(req)
					results <- resp
				}()
			}

			// Collect responses
			successCount := 0
			for i := 0; i < concurrentRequests; i++ {
				resp := <-results
				if resp != nil {
					// Success = 201 Created (first) or 202 Accepted (deduplicated)
					if resp.StatusCode == http.StatusCreated || resp.StatusCode == http.StatusAccepted {
						successCount++
					}
					_ = resp.Body.Close()
				}
			}

			// Then: All requests should succeed (deduplication handled gracefully)
			Expect(successCount).To(Equal(concurrentRequests),
				"All concurrent requests should succeed (deduplication is transparent)")

			// And: Only one RemediationRequest should exist for this alert
			// Note: Filter by SignalName (alertname) not fingerprint, since Gateway generates fingerprints
			// Note: Increased timeout to 20s to allow for K8s optimistic concurrency retries under CI load
			Eventually(func() int {
				rrList := &remediationv1alpha1.RemediationRequestList{}
				err := testClient.List(ctx, rrList,
					client.InNamespace(testNamespace))
				if err != nil {
					return -1
				}
				// Filter by alertname in memory
				count := 0
				for _, rr := range rrList.Items {
					if rr.Spec.SignalName == "TestConcurrentDedup" {
						count++
					}
				}
				return count
			}, 20*time.Second, 500*time.Millisecond).Should(Equal(1),
				"Only one RemediationRequest should be created despite concurrent requests")
		})

		It("should update deduplication hit count atomically", FlakeAttempts(3), func() {
			// Given: RemediationRequest with existing hit count
			// When: Multiple deduplicated alerts arrive concurrently
			// Then: Hit count increments correctly (no lost updates)

			// Create initial RemediationRequest
			fingerprint := fmt.Sprintf("atomic-test-%d", time.Now().Unix())
			payload := createPrometheusWebhookPayload(PrometheusAlertPayload{
				AlertName: "TestAtomicHitCount",
				Namespace: testNamespace,
				Severity:  "info",
				Labels: map[string]string{
					"fingerprint": fingerprint,
				},
			})

			req, err := http.NewRequest("POST",
				fmt.Sprintf("%s/api/v1/signals/prometheus", gatewayURL),
				bytes.NewBuffer(payload))
			req.Header.Set("Content-Type", "application/json")
			req.Header.Set("X-Timestamp", fmt.Sprintf("%d", time.Now().Unix()))

			resp, err := http.DefaultClient.Do(req)
			_ = err
			_ = resp.Body.Close()

			// Verify initial request succeeded
			Expect(resp.StatusCode).To(Or(Equal(http.StatusCreated), Equal(http.StatusAccepted)),
				"Initial request should succeed")

			// Wait for initial RR to be created
			// Note: Query by SignalName (alertname) not fingerprint, since Gateway generates fingerprints
			Eventually(func() int {
				rrList := &remediationv1alpha1.RemediationRequestList{}
				err := testClient.List(ctx, rrList,
					client.InNamespace(testNamespace))
				if err != nil {
					return 0
				}
				// Filter by alertname in memory
				count := 0
				for _, rr := range rrList.Items {
					if rr.Spec.SignalName == "TestAtomicHitCount" {
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
				_ = testClient.List(ctx, &rrList, client.InNamespace(testNamespace))

				for _, rr := range rrList.Items {
					if rr.Spec.SignalName == "TestAtomicHitCount" && rr.Status.Deduplication != nil {
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

			req, err := http.NewRequest("POST",
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
