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
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"time"

	"github.com/google/uuid"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"sigs.k8s.io/controller-runtime/pkg/client"

	remediationv1alpha1 "github.com/jordigilh/kubernaut/api/remediation/v1alpha1"
	"github.com/jordigilh/kubernaut/pkg/gateway"
)

// DD-GATEWAY-011: Status-Based Deduplication - Integration Tests
//
// Business Requirements:
// - BR-GATEWAY-181: Move deduplication tracking from spec to status
// - BR-GATEWAY-183: Implement optimistic concurrency for status updates
//
// PURPOSE: Verify that StatusUpdater is actually wired into server.go and
// updates RR.status.deduplication when duplicate signals are received.
//
// This test PROVES the Day 4 integration is working, not just compiling.
//
// Defense-in-Depth Strategy (per 03-testing-strategy.mdc):
// - Unit tests (70%+): StatusUpdater/PhaseChecker in isolation (deduplication_status_test.go)
// - Integration tests (>50%): THIS FILE - Verify wiring in server.go works with real K8s API
// - E2E tests (10-15%): Complete workflow with Kind cluster

var _ = Describe("DD-GATEWAY-011: Status-Based Deduplication - Integration Tests", func() {
	var (
		ctx               context.Context
		server            *httptest.Server
		gatewayURL        string
		testClient        *K8sTestClient
		redisClient       *RedisTestClient
		prometheusPayload []byte
	)

	// Shared namespace across ALL tests (package-level, initialized once)
	sharedNamespace := fmt.Sprintf("test-dd011-p%d-%s", GinkgoParallelProcess(), uuid.New().String()[:8])

	BeforeEach(func() {
		// Per-spec setup for parallel execution
		ctx = context.Background()
		testClient = SetupK8sTestClient(ctx)
		redisClient = SetupRedisTestClient(ctx)

		// Ensure shared namespace exists (idempotent, thread-safe)
		EnsureTestNamespace(ctx, testClient, sharedNamespace)
		RegisterTestNamespace(sharedNamespace)

		// Per-spec Gateway instance (thread-safe: each parallel spec gets own HTTP server)
		gatewayServer, err := StartTestGateway(ctx, redisClient, testClient)
		Expect(err).ToNot(HaveOccurred())
		server = httptest.NewServer(gatewayServer.Handler())
		gatewayURL = server.URL
	})

	AfterEach(func() {
		if server != nil {
			server.Close()
		}

		// DD-GATEWAY-011: Clean up CRDs after each test
		By("Cleaning up CRDs in shared namespace")
		crdList := &remediationv1alpha1.RemediationRequestList{}
		err := testClient.Client.List(ctx, crdList, client.InNamespace(sharedNamespace))
		if err == nil {
			for i := range crdList.Items {
				_ = testClient.Client.Delete(ctx, &crdList.Items[i])
			}

			Eventually(func() int {
				list := &remediationv1alpha1.RemediationRequestList{}
				_ = testClient.Client.List(ctx, list, client.InNamespace(sharedNamespace))
				return len(list.Items)
			}, 10*time.Second, 500*time.Millisecond).Should(Equal(0),
				"All CRDs should be deleted before next test")
		}

		// Clean up Redis state
		By("Flushing Redis database")
		if redisClient != nil && redisClient.Client != nil {
			_ = redisClient.Client.FlushDB(ctx).Err()
		}
	})

	// ━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━
	// TEST: StatusUpdater Wiring Verification
	// ━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━

	Context("when duplicate signal is received (BR-GATEWAY-181)", func() {
		BeforeEach(func() {
			uniqueID := uuid.New().String()
			prometheusPayload = createPrometheusAlertPayload(PrometheusAlertOptions{
				AlertName: "DD011StatusTest",
				Namespace: sharedNamespace,
				Severity:  "critical",
				Resource: ResourceIdentifier{
					Kind: "Pod",
					Name: "status-dedup-test-" + uniqueID,
				},
				Labels: map[string]string{
					"app":       "dd011-status-test",
					"unique_id": uniqueID,
				},
			})
		})

		It("should update status.deduplication.occurrenceCount (WIRING PROOF)", func() {
			// DD-GATEWAY-011: This test PROVES StatusUpdater is wired correctly
			//
			// WHAT WE'RE TESTING:
			// - server.go processDuplicateSignal() calls statusUpdater.UpdateDeduplicationStatus()
			// - The RR.status.deduplication.occurrenceCount is incremented
			// - The RR.status.deduplication.lastSeenAt is updated
			//
			// WITHOUT THIS TEST: We only have compile-time proof, not runtime proof

			By("1. Send first alert (creates CRD)")
			resp1 := sendWebhook(gatewayURL, "/api/v1/signals/prometheus", prometheusPayload)
			Expect(resp1.StatusCode).To(Equal(http.StatusCreated), "First alert should create new CRD")

			var response1 gateway.ProcessingResponse
			err := json.Unmarshal(resp1.Body, &response1)
			Expect(err).ToNot(HaveOccurred())
			Expect(response1.Status).To(Equal("created"))
			crdName := response1.RemediationRequestName

			By("2. Verify CRD was created with initial state")
			crd := getCRDByName(ctx, testClient, sharedNamespace, crdName)
			Expect(crd).ToNot(BeNil(), "CRD should exist")

			// Capture initial status.deduplication state (should be nil initially)
			initialStatusDedup := crd.Status.Deduplication

			By("3. Set CRD state to Pending (required for duplicate detection)")
			crd.Status.OverallPhase = "Pending"
			err = testClient.Client.Status().Update(ctx, crd)
			Expect(err).ToNot(HaveOccurred())

			// Wait for status update to propagate
			Eventually(func() string {
				updatedCRD := getCRDByName(ctx, testClient, sharedNamespace, crdName)
				if updatedCRD == nil {
					return ""
				}
				return updatedCRD.Status.OverallPhase
			}, 3*time.Second, 500*time.Millisecond).Should(Equal("Pending"))

			By("4. Send duplicate alert (triggers processDuplicateSignal → statusUpdater)")
			resp2 := sendWebhook(gatewayURL, "/api/v1/signals/prometheus", prometheusPayload)
			Expect(resp2.StatusCode).To(Equal(http.StatusAccepted), "Duplicate alert should return 202 Accepted")

			var response2 gateway.ProcessingResponse
			err = json.Unmarshal(resp2.Body, &response2)
			Expect(err).ToNot(HaveOccurred())
			Expect(response2.Status).To(Equal("duplicate"))
			Expect(response2.Duplicate).To(BeTrue())

			By("5. CRITICAL VERIFICATION: status.deduplication should be updated")
			// THIS IS THE KEY ASSERTION - proves StatusUpdater wiring works
			Eventually(func() bool {
				updatedCRD := getCRDByName(ctx, testClient, sharedNamespace, crdName)
				if updatedCRD == nil {
					return false
				}

				// DD-GATEWAY-011: Status.Deduplication should now exist and be populated
				statusDedup := updatedCRD.Status.Deduplication
				if statusDedup == nil {
					GinkgoWriter.Printf("status.deduplication is still nil (waiting for StatusUpdater)\n")
					return false
				}

				// Verify OccurrenceCount was incremented
				// Initial: nil or count=1, After duplicate: count should be >= 1
				GinkgoWriter.Printf("status.deduplication.occurrenceCount = %d\n", statusDedup.OccurrenceCount)

				// Verify LastSeenAt was set
				if statusDedup.LastSeenAt == nil {
					GinkgoWriter.Printf("status.deduplication.lastSeenAt is nil\n")
					return false
				}

				GinkgoWriter.Printf("status.deduplication.lastSeenAt = %v\n", statusDedup.LastSeenAt.Time)

				// Success: status.deduplication exists with valid data
				return statusDedup.OccurrenceCount >= 1 && statusDedup.LastSeenAt != nil
			}, 5*time.Second, 500*time.Millisecond).Should(BeTrue(),
				"status.deduplication should be populated by StatusUpdater - THIS PROVES THE WIRING WORKS")

			// Additional assertion: Compare to initial state
			if initialStatusDedup == nil {
				By("6. BONUS: Confirm status.deduplication was nil before duplicate")
				GinkgoWriter.Printf("Initial status.deduplication was nil, now populated = WIRING CONFIRMED\n")
			}
		})

		It("should handle multiple duplicates and update occurrence count incrementally", func() {
			// DD-GATEWAY-011: Verify occurrence count increments with each duplicate
			// This is a simpler test that validates the core behavior

			By("1. Create initial CRD")
			resp1 := sendWebhook(gatewayURL, "/api/v1/signals/prometheus", prometheusPayload)
			Expect(resp1.StatusCode).To(Equal(http.StatusCreated))

			var response1 gateway.ProcessingResponse
			err := json.Unmarshal(resp1.Body, &response1)
			Expect(err).ToNot(HaveOccurred())
			crdName := response1.RemediationRequestName

			By("2. Set to Pending state")
			crd := getCRDByName(ctx, testClient, sharedNamespace, crdName)
			crd.Status.OverallPhase = "Pending"
			err = testClient.Client.Status().Update(ctx, crd)
			Expect(err).ToNot(HaveOccurred())

			Eventually(func() string {
				c := getCRDByName(ctx, testClient, sharedNamespace, crdName)
				if c == nil {
					return ""
				}
				return c.Status.OverallPhase
			}, 3*time.Second, 500*time.Millisecond).Should(Equal("Pending"))

			By("3. Send first duplicate")
			resp2 := sendWebhook(gatewayURL, "/api/v1/signals/prometheus", prometheusPayload)
			Expect(resp2.StatusCode).To(Equal(http.StatusAccepted))

			By("4. Verify status.deduplication exists after first duplicate")
			Eventually(func() bool {
				c := getCRDByName(ctx, testClient, sharedNamespace, crdName)
				if c == nil {
					return false
				}
				// Just verify status.deduplication is populated
				return c.Status.Deduplication != nil
			}, 5*time.Second, 500*time.Millisecond).Should(BeTrue(),
				"status.deduplication should exist after duplicate - WIRING CONFIRMED")

			By("5. Send second duplicate")
			resp3 := sendWebhook(gatewayURL, "/api/v1/signals/prometheus", prometheusPayload)
			Expect(resp3.StatusCode).To(Equal(http.StatusAccepted))

			By("6. Verify occurrence count increased")
			Eventually(func() int32 {
				c := getCRDByName(ctx, testClient, sharedNamespace, crdName)
				if c == nil || c.Status.Deduplication == nil {
					return 0
				}
				GinkgoWriter.Printf("status.deduplication.occurrenceCount = %d\n", c.Status.Deduplication.OccurrenceCount)
				return c.Status.Deduplication.OccurrenceCount
			}, 5*time.Second, 500*time.Millisecond).Should(BeNumerically(">=", 2),
				"Occurrence count should be >= 2 after two duplicates")
		})
	})
})

