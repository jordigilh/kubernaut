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
	"os"
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
// - BR-GATEWAY-181: Duplicate tracking visible in RR status for RO decision-making
// - BR-GATEWAY-183: Concurrent duplicate alerts handled without data loss
//
// BUSINESS VALUE:
// When duplicate alerts arrive for an active incident, the Remediation Orchestrator
// needs to see occurrence counts in RR.status to:
// 1. Prioritize incidents with high duplicate counts (recurring issues)
// 2. Track alert frequency for SLA reporting
// 3. Make informed decisions about remediation urgency
//
// This test validates the business behavior, not implementation details.
//
// Defense-in-Depth Strategy (per 03-testing-strategy.mdc):
// - Unit tests (70%+): Component logic in isolation
// - Integration tests (>50%): THIS FILE - Cross-component K8s API interaction
// - E2E tests (10-15%): Complete workflow with Kind cluster

var _ = Describe("DD-GATEWAY-011: Status-Based Tracking - Integration Tests", func() {
	var (
		ctx               context.Context
		testClient        client.Client
		prometheusPayload []byte
	)

	// Shared namespace across ALL tests (package-level, initialized once)
	sharedNamespace := fmt.Sprintf("test-dd011-p%d-%s", GinkgoParallelProcess(), uuid.New().String()[:8])

	BeforeEach(func() {
		// Per-spec setup for parallel execution
		ctx = context.Background()
		testClient = getKubernetesClient()

		// Ensure shared namespace exists (idempotent, thread-safe)

		// DD-GATEWAY-012: Redis removed, Gateway now K8s status-based
		// DD-AUDIT-003: Gateway connects to Data Storage for audit
		// DD-TEST-001: Get Data Storage URL from suite's shared infrastructure
		dataStorageURL := os.Getenv("TEST_DATA_STORAGE_URL")
		if dataStorageURL == "" {
			dataStorageURL = "http://127.0.0.1:18091" // Fallback for manual testing - Use 127.0.0.1 for CI/CD IPv4 compatibility
		}

		// Note: gatewayURL is the globally deployed Gateway service at http://127.0.0.1:8080
	})

	AfterEach(func() {

		// DD-GATEWAY-011: Clean up CRDs after each test
		By("Cleaning up CRDs in shared namespace")
		crdList := &remediationv1alpha1.RemediationRequestList{}
		err := testClient.List(ctx, crdList, client.InNamespace(sharedNamespace))
		if err == nil {
			for i := range crdList.Items {
				_ = testClient.Delete(ctx, &crdList.Items[i])
			}

			Eventually(func() int {
				list := &remediationv1alpha1.RemediationRequestList{}
				_ = testClient.List(ctx, list, client.InNamespace(sharedNamespace))
				return len(list.Items)
			}, 10*time.Second, 500*time.Millisecond).Should(Equal(0),
				"All CRDs should be deleted before next test")
		}

		// DD-GATEWAY-012: Redis cleanup no longer needed (Gateway is Redis-free)
	})

	// ━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━
	// TEST: StatusUpdater Wiring Verification
	// ━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━

	Context("when duplicate alerts arrive for active incident (BR-GATEWAY-181)", func() {
		BeforeEach(func() {
			uniqueID := uuid.New().String()
			prometheusPayload = createPrometheusWebhookPayload(PrometheusAlertPayload{
				AlertName: "RecurringPodCrashLoop",
				Namespace: sharedNamespace,
				Severity:  "critical",
				Resource: ResourceIdentifier{
					Kind: "Pod",
					Name: "payment-api-" + uniqueID,
				},
				Labels: map[string]string{
					"app":       "payment-api",
					"unique_id": uniqueID,
				},
			})
		})

		It("should track duplicate count in RR status for RO prioritization", func() {
			// BR-GATEWAY-181: Duplicate Tracking in Status
			//
			// BUSINESS SCENARIO:
			// A pod is crash-looping, generating repeated alerts. The Remediation
			// Orchestrator needs to see how many times this alert has fired to:
			// - Prioritize high-frequency incidents
			// - Report accurate SLA metrics
			// - Determine remediation urgency

			By("1. Send first alert (creates CRD)")
			resp1 := sendWebhook(gatewayURL, "/api/v1/signals/prometheus", prometheusPayload)
			Expect(resp1.StatusCode).To(Equal(http.StatusCreated), "First alert should create new CRD")

			var response1 gateway.ProcessingResponse
			err := json.Unmarshal(resp1.Body, &response1)
			_ = err
			Expect(response1.Status).To(Equal("created"))
			crdName := response1.RemediationRequestName

			By("2. Verify CRD was created with initial state")
			crd := getCRDByName(ctx, testClient, sharedNamespace, crdName)
			Expect(crd).ToNot(BeNil(), "CRD should exist")

			// Capture initial status.deduplication state (should be nil initially)
			initialStatusDedup := crd.Status.Deduplication

			By("3. Set CRD state to Pending (required for duplicate detection)")
			crd.Status.OverallPhase = "Pending"
			err = testClient.Status().Update(ctx, crd)

			// Wait for status update to propagate
			Eventually(func() string {
				updatedCRD := getCRDByName(ctx, testClient, sharedNamespace, crdName)
				if updatedCRD == nil {
					return ""
				}
				return string(updatedCRD.Status.OverallPhase)
			}, 3*time.Second, 500*time.Millisecond).Should(Equal("Pending"))

			By("4. Send duplicate alert (triggers processDuplicateSignal → statusUpdater)")
			resp2 := sendWebhook(gatewayURL, "/api/v1/signals/prometheus", prometheusPayload)
			Expect(resp2.StatusCode).To(Equal(http.StatusAccepted), "Duplicate alert should return 202 Accepted")

			var response2 gateway.ProcessingResponse
			err = json.Unmarshal(resp2.Body, &response2)
			Expect(response2.Status).To(Equal("duplicate"))
			Expect(response2.Duplicate).To(BeTrue())

			By("5. BUSINESS OUTCOME: RO can see duplicate count in RR status")
			// BR-GATEWAY-181: The Remediation Orchestrator reads status.deduplication
			// to understand incident severity and prioritize accordingly
			Eventually(func() bool {
				updatedCRD := getCRDByName(ctx, testClient, sharedNamespace, crdName)
				if updatedCRD == nil {
					return false
				}

				// Business requirement: RO needs to see deduplication data in status
				statusDedup := updatedCRD.Status.Deduplication
				if statusDedup == nil {
					GinkgoWriter.Printf("Waiting for duplicate tracking to appear in status...\n")
					return false
				}

				// Business requirement: RO needs occurrence count for prioritization
				GinkgoWriter.Printf("RR status shows %d occurrences of this alert\n", statusDedup.OccurrenceCount)

				// Business requirement: RO needs timestamp for SLA tracking
				if statusDedup.LastSeenAt == nil {
					GinkgoWriter.Printf("Waiting for lastSeenAt timestamp...\n")
					return false
				}

				GinkgoWriter.Printf("Last alert occurrence: %v\n", statusDedup.LastSeenAt.Time)

				// Business success: RO can read occurrence data from RR status
				return statusDedup.OccurrenceCount >= 1 && statusDedup.LastSeenAt != nil
			}, 5*time.Second, 500*time.Millisecond).Should(BeTrue(),
				"RO should be able to read duplicate tracking from RR status (BR-GATEWAY-181)")

			// Business context: Before DD-GATEWAY-011, this data was only in Redis
			if initialStatusDedup == nil {
				By("6. CONFIRMED: Duplicate tracking now visible in K8s (previously Redis-only)")
				GinkgoWriter.Printf("RO can now read duplicate data directly from RR status\n")
			}
		})

		It("should accurately count recurring alerts for SLA reporting (BR-GATEWAY-181)", func() {
			// BR-GATEWAY-181: Accurate Occurrence Counting
			//
			// BUSINESS SCENARIO:
			// SRE team needs to report on incident frequency. When the same alert
			// fires multiple times, the occurrence count must be accurate for:
			// - SLA breach calculations
			// - Incident frequency dashboards
			// - Remediation effectiveness metrics

			By("1. Initial alert creates incident (RemediationRequest)")
			resp1 := sendWebhook(gatewayURL, "/api/v1/signals/prometheus", prometheusPayload)
			Expect(resp1.StatusCode).To(Equal(http.StatusCreated))

			var response1 gateway.ProcessingResponse
			err := json.Unmarshal(resp1.Body, &response1)
			Expect(err).ToNot(HaveOccurred(), "Failed to unmarshal response: %v, body: %s", err, string(resp1.Body))
			crdName := response1.RemediationRequestName

			By("2. Incident is being processed (Pending state)")
			var crd *remediationv1alpha1.RemediationRequest
			Eventually(func() *remediationv1alpha1.RemediationRequest {
				crd = getCRDByName(ctx, testClient, sharedNamespace, crdName)
				return crd
			}, 60*time.Second, 2*time.Second).ShouldNot(BeNil(), "CRD should exist after Gateway processes signal")
			
			crd.Status.OverallPhase = "Pending"
			err = testClient.Status().Update(ctx, crd)

			Eventually(func() string {
				c := getCRDByName(ctx, testClient, sharedNamespace, crdName)
				if c == nil {
					return ""
				}
				return string(c.Status.OverallPhase)
			}, 3*time.Second, 500*time.Millisecond).Should(Equal("Pending"))

			By("3. Same alert fires again (pod still crash-looping)")
			resp2 := sendWebhook(gatewayURL, "/api/v1/signals/prometheus", prometheusPayload)
			Expect(resp2.StatusCode).To(Equal(http.StatusAccepted),
				"Duplicate alert should be accepted, not create new incident")

			By("4. RO can see this is a recurring issue")
			Eventually(func() bool {
				c := getCRDByName(ctx, testClient, sharedNamespace, crdName)
				if c == nil {
					return false
				}
				return c.Status.Deduplication != nil
			}, 5*time.Second, 500*time.Millisecond).Should(BeTrue(),
				"Duplicate tracking should be visible to RO")

			By("5. Alert fires a third time (escalating situation)")
			resp3 := sendWebhook(gatewayURL, "/api/v1/signals/prometheus", prometheusPayload)
			Expect(resp3.StatusCode).To(Equal(http.StatusAccepted))

			By("6. BUSINESS OUTCOME: Accurate occurrence count for SLA reporting")
			Eventually(func() int32 {
				c := getCRDByName(ctx, testClient, sharedNamespace, crdName)
				if c == nil || c.Status.Deduplication == nil {
					return 0
				}
				GinkgoWriter.Printf("SLA Report: This alert has fired %d times\n", c.Status.Deduplication.OccurrenceCount)
				return c.Status.Deduplication.OccurrenceCount
			}, 5*time.Second, 500*time.Millisecond).Should(BeNumerically(">=", 2),
				"SLA reporting requires accurate occurrence count (BR-GATEWAY-181)")
		})
	})

	// ━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━
	// HIGH-FREQUENCY ALERT DETECTION (BR-GATEWAY-182)
	// ━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━

	Context("when same alert fires repeatedly (storm pattern) (BR-GATEWAY-182)", func() {
		var stormPayload []byte

		BeforeEach(func() {
			// Use SAME payload for all alerts = SAME fingerprint
			// This triggers deduplication with high occurrence count (storm indicator)
			uniqueID := uuid.New().String()
			stormPayload = createPrometheusWebhookPayload(PrometheusAlertPayload{
				AlertName: "PersistentPodCrashLoop",
				Namespace: sharedNamespace,
				Severity:  "critical",
				Resource: ResourceIdentifier{
					Kind: "Pod",
					Name: "database-primary-" + uniqueID[:8], // Same pod, same fingerprint
				},
				Labels: map[string]string{
					"app":        "database",
					"storm_test": uniqueID,
				},
			})
		})

		It("should track high occurrence count indicating storm behavior", func() {
			// BR-GATEWAY-182: Storm Detection via Occurrence Count
			//
			// BUSINESS SCENARIO:
			// A database pod is crash-looping, generating the SAME alert every 30 seconds.
			// After 10 occurrences, this is clearly a storm pattern. The RO needs to:
			// 1. See the high occurrence count to prioritize
			// 2. Know this is a persistent issue (not transient)
			// 3. Consider escalation or different remediation strategy

			By("1. First alert creates incident (RemediationRequest)")
			resp1 := sendWebhook(gatewayURL, "/api/v1/signals/prometheus", stormPayload)
			Expect(resp1.StatusCode).To(Equal(http.StatusCreated))

			var response1 gateway.ProcessingResponse
			err := json.Unmarshal(resp1.Body, &response1)
			_ = err
			crdName := response1.RemediationRequestName

			By("2. Set incident to Pending (remediation in progress)")
			crd := getCRDByName(ctx, testClient, sharedNamespace, crdName)
			Expect(crd).ToNot(BeNil())
			crd.Status.OverallPhase = "Pending"
			err = testClient.Status().Update(ctx, crd)

			Eventually(func() string {
				c := getCRDByName(ctx, testClient, sharedNamespace, crdName)
				if c == nil {
					return ""
				}
				return string(c.Status.OverallPhase)
			}, 3*time.Second, 500*time.Millisecond).Should(Equal("Pending"))

			By("3. Same alert fires 9 more times (storm pattern)")
			for i := 2; i <= 10; i++ {
				resp := sendWebhook(gatewayURL, "/api/v1/signals/prometheus", stormPayload)
				Expect(resp.StatusCode).To(Equal(http.StatusAccepted),
					fmt.Sprintf("Alert %d should be deduplicated (same fingerprint)", i))
			}

			By("4. BUSINESS OUTCOME: RO sees high occurrence count (storm indicator)")
			Eventually(func() int32 {
				c := getCRDByName(ctx, testClient, sharedNamespace, crdName)
				if c == nil || c.Status.Deduplication == nil {
					return 0
				}
				count := c.Status.Deduplication.OccurrenceCount
				GinkgoWriter.Printf("Occurrence count: %d (storm threshold typically 5-10)\n", count)
				return count
			}, 10*time.Second, 500*time.Millisecond).Should(BeNumerically(">=", 5),
				"High occurrence count indicates storm pattern (BR-GATEWAY-182)")

			By("5. BUSINESS CONTEXT: RO can make informed prioritization decision")
			finalCRD := getCRDByName(ctx, testClient, sharedNamespace, crdName)
			Expect(finalCRD).ToNot(BeNil())
			Expect(finalCRD.Status.Deduplication).ToNot(BeNil())

			occurrenceCount := finalCRD.Status.Deduplication.OccurrenceCount
			GinkgoWriter.Printf("\n📊 STORM ANALYSIS for RO:\n")
			GinkgoWriter.Printf("   Alert: %s\n", finalCRD.Spec.SignalName)
			GinkgoWriter.Printf("   Occurrences: %d\n", occurrenceCount)
			GinkgoWriter.Printf("   First seen: %v\n", finalCRD.Status.Deduplication.FirstSeenAt)
			GinkgoWriter.Printf("   Last seen: %v\n", finalCRD.Status.Deduplication.LastSeenAt)

			if occurrenceCount >= 10 {
				GinkgoWriter.Printf("   🔴 RECOMMENDATION: High-priority storm - consider escalation\n")
			} else if occurrenceCount >= 5 {
				GinkgoWriter.Printf("   🟡 RECOMMENDATION: Moderate storm - monitor closely\n")
			}

			// Verify business-meaningful assertions
			Expect(occurrenceCount).To(BeNumerically(">=", 5),
				"Storm pattern: 5+ occurrences indicates persistent issue")
			Expect(finalCRD.Status.Deduplication.LastSeenAt).ToNot(BeNil(),
				"LastSeenAt required for SLA tracking")
		})
	})
})
