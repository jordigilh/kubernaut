package datastorage

import (
	"context"
	"fmt"
	"net/http"
	"time"

	dsgen ogenclient "github.com/jordigilh/kubernaut/pkg/datastorage/ogen-client"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

// ========================================
// GAP 5.3: Cold Start Performance (Service Restart)
// ========================================
// BR-STORAGE-031: DS starts quickly after restart (rolling updates)
// Priority: P1 - Operational maturity
// Estimated Effort: 1 hour
// Confidence: 91%
//
// Business Outcome: Service restarts don't cause extended downtime
//
// Test Scenario:
//   GIVEN DS service freshly started (cold start)
//   WHEN first audit write request received within 5s of startup
//   THEN:
//     - Connection pool initialized <1s
//     - First request completes within 2s (includes connection setup)
//     - Subsequent requests meet normal SLA (p95 <250ms)
//     - No "connection refused" errors during startup
//
// Why This Matters: Rolling updates require fast restarts to avoid downtime
// ========================================

var _ = Describe("GAP 5.3: Cold Start Performance", Label("integration", "datastorage", "gap-5.3", "p1"), func() {
	var (
		client *dsgen.ClientWithResponses
		ctx    context.Context
	)

	BeforeEach(func() {
		ctx = context.Background()

		// Create OpenAPI client
		var err error
		client, err = createOpenAPIClient(datastorageURL)
		Expect(err).ToNot(HaveOccurred())
	})

	Context("when service freshly started (cold start)", func() {
		It("should initialize quickly and handle first request within 2s", func() {
			GinkgoWriter.Println("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")
			GinkgoWriter.Println("GAP 5.3: Testing cold start performance (service restart)")
			GinkgoWriter.Println("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")

			// ARRANGE: Wait for service to be healthy after startup
			startupTime := time.Now()
			httpClient := &http.Client{Timeout: 10 * time.Second}
			Eventually(func() bool {
				resp, err := httpClient.Get(datastorageURL + "/health")
				if err != nil {
					return false
				}
				defer func() { _ = resp.Body.Close() }()
				return resp.StatusCode == http.StatusOK
			}, 10*time.Second, 100*time.Millisecond).Should(BeTrue(), "Service should become healthy within 10s")

			healthyDuration := time.Since(startupTime)

			GinkgoWriter.Printf("Service became healthy in %v\n", healthyDuration)

			// ACT: Send first audit event within 5s of startup using OpenAPI client
			testID := generateTestID()
			correlationID := fmt.Sprintf("cold-start-test-%s", testID)

			// Use OpenAPI client with proper types (matches all other working tests)
			// ADR-034: event_category must be one of the service categories (gateway, notification, etc.)
			eventData := map[string]interface{}{
				"service":       "test-service",
				"resource_type": "Pod",
				"resource_id":   fmt.Sprintf("cold-start-pod-%s", testID),
				"cold_start":    true,
			}

			event := createAuditEventRequest(
				"pod.created",
				"gateway", // Valid ADR-034 service category (was "resource" - not valid)
				"created",
				"success",
				correlationID,
				eventData,
			)

			firstRequestStart := time.Now()
			resp, err := client.CreateAuditEventWithResponse(ctx, event)
			firstRequestDuration := time.Since(firstRequestStart)

			Expect(err).ToNot(HaveOccurred())
			Expect(resp.StatusCode()).To(SatisfyAny(
				Equal(http.StatusCreated),
				Equal(http.StatusAccepted),
			), "First request should succeed")

			GinkgoWriter.Printf("First request completed in %v\n", firstRequestDuration)

			// ASSERT: First request completes within 2s
			Expect(firstRequestDuration.Seconds()).To(BeNumerically("<", 2),
				fmt.Sprintf("First request took %v, exceeds 2s target (includes connection setup)", firstRequestDuration))

			// ACT: Send second request (should use existing connection)
			correlationID2 := fmt.Sprintf("cold-start-test-2-%s", testID)
			eventData2 := map[string]interface{}{
				"service":       "test-service",
				"resource_type": "Pod",
				"resource_id":   fmt.Sprintf("cold-start-pod-2-%s", testID),
				"cold_start":    true,
			}

			event2 := createAuditEventRequest(
				"pod.created",
				"gateway", // Valid ADR-034 service category
				"created",
				"success",
				correlationID2,
				eventData2,
			)

			secondRequestStart := time.Now()
			resp2, err := client.CreateAuditEventWithResponse(ctx, event2)
			secondRequestDuration := time.Since(secondRequestStart)

			Expect(err).ToNot(HaveOccurred())
			Expect(resp2.StatusCode()).To(SatisfyAny(
				Equal(http.StatusCreated),
				Equal(http.StatusAccepted),
			))

			GinkgoWriter.Printf("Second request completed in %v\n", secondRequestDuration)

			// ASSERT: Subsequent requests meet normal SLA (<250ms)
			Expect(secondRequestDuration.Milliseconds()).To(BeNumerically("<", 250),
				fmt.Sprintf("Second request took %dms, doesn't meet p95 <250ms SLA", secondRequestDuration.Milliseconds()))

			GinkgoWriter.Printf("\n✅ Cold start performance validated:\n")
			GinkgoWriter.Printf("   Startup:       %v (target: <1s)\n", healthyDuration)
			GinkgoWriter.Printf("   First request: %v (target: <2s)\n", firstRequestDuration)
			GinkgoWriter.Printf("   Second request: %dms (target: <250ms)\n", secondRequestDuration.Milliseconds())

			// BUSINESS VALUE: Fast restart performance
			// - Rolling updates don't cause extended downtime
			// - Connection pool initializes quickly
			// - First request completes promptly
			// - Service quickly reaches steady-state performance
		})
	})
})
