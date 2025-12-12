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
	"os"
	"time"

	"github.com/google/uuid"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/jordigilh/kubernaut/pkg/gateway"
)

// =============================================================================
// DD-AUDIT-003: Gateway → Data Storage Audit Integration Tests (TDD RED PHASE)
// =============================================================================
//
// Business Requirements:
// - BR-AUDIT-001: All service operations MUST generate audit events
// - BR-AUDIT-002: Audit events MUST be persisted to Data Storage
// - BR-GATEWAY-190: Signal ingestion MUST create audit trail
// - BR-GATEWAY-191: Deduplication decisions MUST be audited
// - BR-GATEWAY-192: Storm detection MUST be audited
//
// Test Strategy:
// - Per TESTING_GUIDELINES.md: Integration tests use REAL infrastructure
// - Gateway connects to REAL Data Storage (via podman-compose)
// - Tests verify audit events appear in Data Storage database
// - LLM is mocked (cost constraint), but Data Storage is REAL
//
// Current State: TDD RED PHASE
// - Tests are written to FAIL
// - Gateway does NOT currently emit audit events to Data Storage
// - Tests will pass when Gateway audit integration is implemented
//
// To run these tests:
//   1. Start infrastructure: podman-compose -f test/infrastructure/podman-compose.test.yml up -d
//   2. Run tests: go test ./test/integration/gateway/... --ginkgo.focus="Audit Integration"
//
// =============================================================================

var _ = Describe("DD-AUDIT-003: Gateway → Data Storage Audit Integration", func() {
	var (
		ctx               context.Context
		server            *httptest.Server
		gatewayURL        string
		testClient        *K8sTestClient
		dataStorageURL    string
		prometheusPayload []byte
	)

	// Shared test namespace
	sharedNamespace := fmt.Sprintf("test-audit-%d-%s", GinkgoParallelProcess(), uuid.New().String()[:8])

	BeforeEach(func() {
		ctx = context.Background()
		testClient = SetupK8sTestClient(ctx)

		// DD-TEST-001: Get Data Storage URL from suite's shared infrastructure
		// Per DD-TEST-001: All parallel processes share same Data Storage instance
		dataStorageURL = os.Getenv("TEST_DATA_STORAGE_URL")
		if dataStorageURL == "" {
			dataStorageURL = "http://localhost:18090" // Fallback for manual testing
		}

		// MANDATORY: Verify Data Storage is running
		// Per TESTING_GUIDELINES.md: Tests MUST FAIL if infrastructure unavailable
		healthResp, err := http.Get(dataStorageURL + "/health")
		if err != nil {
			Fail(fmt.Sprintf(
				"REQUIRED: Data Storage not available at %s\n"+
					"  Per DD-AUDIT-003: Gateway MUST have audit capability\n"+
					"  Per TESTING_GUIDELINES.md: Integration tests MUST use real services\n\n"+
					"  Start with: podman-compose -f test/infrastructure/podman-compose.test.yml up -d\n\n"+
					"  Error: %v", dataStorageURL, err))
		}
		defer healthResp.Body.Close()
		if healthResp.StatusCode != http.StatusOK {
			Fail(fmt.Sprintf(
				"REQUIRED: Data Storage health check failed at %s\n"+
					"  Status: %d\n"+
					"  Expected: 200 OK\n\n"+
					"  Check Data Storage logs: podman-compose logs datastorage",
				dataStorageURL, healthResp.StatusCode))
		}

		// Setup test namespace
		EnsureTestNamespace(ctx, testClient, sharedNamespace)
		RegisterTestNamespace(sharedNamespace)

		// Start test Gateway
		// DD-GATEWAY-012: Redis removed, Gateway now connects to Data Storage for audit
		gatewayServer, err := StartTestGateway(ctx, testClient, dataStorageURL)
		Expect(err).ToNot(HaveOccurred())
		server = httptest.NewServer(gatewayServer.Handler())
		gatewayURL = server.URL

		// Test payload
		uniqueID := uuid.New().String()
		prometheusPayload = createPrometheusAlertPayload(PrometheusAlertOptions{
			AlertName: "AuditTestAlert",
			Namespace: sharedNamespace,
			Severity:  "warning",
			Resource: ResourceIdentifier{
				Kind: "Pod",
				Name: "audit-test-pod-" + uniqueID[:8],
			},
			Labels: map[string]string{
				"audit_test": uniqueID,
			},
		})
	})

	AfterEach(func() {
		if server != nil {
			server.Close()
		}
	})

	// ━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━
	// SIGNAL INGESTION AUDITING (BR-GATEWAY-190)
	// ━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━

	Context("when a new signal is ingested (BR-GATEWAY-190)", func() {
		It("should create 'signal.received' audit event in Data Storage", func() {
			// TDD RED: This test is EXPECTED TO FAIL
			// Gateway does NOT currently emit audit events
			//
			// BUSINESS SCENARIO:
			// When Prometheus AlertManager sends an alert, the Gateway MUST:
			// 1. Process the signal (create RemediationRequest)
			// 2. Emit audit event to Data Storage for compliance tracking
			//
			// COMPLIANCE: SOC2, HIPAA require audit trails for all operations

			By("1. Send Prometheus alert to Gateway")
			resp := sendWebhook(gatewayURL, "/api/v1/signals/prometheus", prometheusPayload)
			Expect(resp.StatusCode).To(Equal(http.StatusCreated), "Signal should be processed")

			var gatewayResp gateway.ProcessingResponse
			err := json.Unmarshal(resp.Body, &gatewayResp)
			Expect(err).ToNot(HaveOccurred())
			correlationID := gatewayResp.RemediationRequestName // Use RR name as correlation

			By("2. Query Data Storage for audit event")
			// Query: GET /api/v1/audit-events?event_category=gateway&correlation_id={correlationID}
			queryURL := fmt.Sprintf("%s/api/v1/audit-events?event_category=gateway&correlation_id=%s",
				dataStorageURL, correlationID)

			// Wait for audit event to appear (async write may have small delay)
			var auditEvents []map[string]interface{}
			Eventually(func() int {
				auditResp, err := http.Get(queryURL)
				if err != nil {
					GinkgoWriter.Printf("Failed to query audit events: %v\n", err)
					return 0
				}
				defer auditResp.Body.Close()

				if auditResp.StatusCode != http.StatusOK {
					GinkgoWriter.Printf("Audit query returned status %d\n", auditResp.StatusCode)
					return 0
				}

				var result struct {
					Events []map[string]interface{} `json:"events"`
					Total  int                      `json:"total"`
				}
				if err := json.NewDecoder(auditResp.Body).Decode(&result); err != nil {
					GinkgoWriter.Printf("Failed to decode audit response: %v\n", err)
					return 0
				}

				auditEvents = result.Events
				return result.Total
			}, 10*time.Second, 500*time.Millisecond).Should(BeNumerically(">=", 1),
				"BR-GATEWAY-190: Gateway MUST emit 'signal.received' audit event")

			By("3. Verify audit event content")
			Expect(auditEvents).To(HaveLen(1), "Should have exactly 1 audit event for this correlation")

			event := auditEvents[0]
			Expect(event["event_category"]).To(Equal("gateway"),
				"event_category should be 'gateway'")
			Expect(event["event_type"]).To(Equal("signal.received"),
				"event_type should be 'signal.received'")
			Expect(event["event_outcome"]).To(Equal("success"),
				"event_outcome should be 'success' for processed signal")

			// Verify event_data contains Gateway-specific fields
			eventData, ok := event["event_data"].(map[string]interface{})
			Expect(ok).To(BeTrue(), "event_data should be a map")

			gatewayData, ok := eventData["gateway"].(map[string]interface{})
			Expect(ok).To(BeTrue(), "event_data.gateway should exist")

			Expect(gatewayData["signal_type"]).To(Equal("prometheus"),
				"signal_type should be 'prometheus'")
			Expect(gatewayData["alert_name"]).To(Equal("AuditTestAlert"),
				"alert_name should match")
			Expect(gatewayData["namespace"]).To(Equal(sharedNamespace),
				"namespace should match")
		})
	})

	// ━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━
	// DEDUPLICATION AUDITING (BR-GATEWAY-191)
	// ━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━

	Context("when a duplicate signal is detected (BR-GATEWAY-191)", func() {
		It("should create 'signal.deduplicated' audit event in Data Storage", func() {
			// TDD RED: This test is EXPECTED TO FAIL
			//
			// BUSINESS SCENARIO:
			// When a duplicate alert arrives, Gateway:
			// 1. Detects it's a duplicate (same fingerprint, active RR)
			// 2. Updates RR status (deduplication count)
			// 3. MUST emit audit event for compliance

			By("1. Send first alert (creates RR)")
			resp1 := sendWebhook(gatewayURL, "/api/v1/signals/prometheus", prometheusPayload)
			Expect(resp1.StatusCode).To(Equal(http.StatusCreated))

			var resp1Data gateway.ProcessingResponse
			err := json.Unmarshal(resp1.Body, &resp1Data)
			Expect(err).ToNot(HaveOccurred())
			correlationID := resp1Data.RemediationRequestName

			// Set RR to Pending (required for duplicate detection)
			crd := getCRDByName(ctx, testClient, sharedNamespace, correlationID)
			Expect(crd).ToNot(BeNil())
			crd.Status.OverallPhase = "Pending"
			err = testClient.Client.Status().Update(ctx, crd)
			Expect(err).ToNot(HaveOccurred())

			Eventually(func() string {
				c := getCRDByName(ctx, testClient, sharedNamespace, correlationID)
				if c == nil {
					return ""
				}
				return string(c.Status.OverallPhase)
			}, 3*time.Second, 500*time.Millisecond).Should(Equal("Pending"))

			By("2. Send duplicate alert (triggers deduplication)")
			resp2 := sendWebhook(gatewayURL, "/api/v1/signals/prometheus", prometheusPayload)
			Expect(resp2.StatusCode).To(Equal(http.StatusAccepted),
				"Duplicate should return 202 Accepted")

			By("3. Query Data Storage for deduplication audit event")
			queryURL := fmt.Sprintf("%s/api/v1/audit-events?event_category=gateway&event_type=signal.deduplicated&correlation_id=%s",
				dataStorageURL, correlationID)

			var auditEvents []map[string]interface{}
			Eventually(func() int {
				auditResp, err := http.Get(queryURL)
				if err != nil {
					return 0
				}
				defer auditResp.Body.Close()

				if auditResp.StatusCode != http.StatusOK {
					return 0
				}

				var result struct {
					Events []map[string]interface{} `json:"events"`
					Total  int                      `json:"total"`
				}
				if err := json.NewDecoder(auditResp.Body).Decode(&result); err != nil {
					return 0
				}

				auditEvents = result.Events
				return result.Total
			}, 10*time.Second, 500*time.Millisecond).Should(BeNumerically(">=", 1),
				"BR-GATEWAY-191: Gateway MUST emit 'signal.deduplicated' audit event")

			By("4. Verify deduplication audit event content")
			event := auditEvents[0]
			Expect(event["event_type"]).To(Equal("signal.deduplicated"))
			Expect(event["event_outcome"]).To(Equal("success"))

			eventData := event["event_data"].(map[string]interface{})
			gatewayData := eventData["gateway"].(map[string]interface{})

			Expect(gatewayData["deduplication_status"]).To(Equal("duplicate"),
				"deduplication_status should be 'duplicate'")
		})
	})

	// ━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━
	// STORM DETECTION AUDITING (BR-GATEWAY-192)
	// ━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━

	Context("when a storm is detected (BR-GATEWAY-192)", func() {
		It("should create 'storm.detected' audit event in Data Storage", func() {
			// TDD RED: This test is EXPECTED TO FAIL
			//
			// BUSINESS SCENARIO:
			// When multiple alerts fire in rapid succession (storm), Gateway:
			// 1. Detects storm pattern (high occurrence count)
			// 2. Aggregates alerts into storm
			// 3. MUST emit audit event for incident response tracking

			By("1. Send first alert (creates RR)")
			resp1 := sendWebhook(gatewayURL, "/api/v1/signals/prometheus", prometheusPayload)
			Expect(resp1.StatusCode).To(Equal(http.StatusCreated))

			var resp1Data gateway.ProcessingResponse
			err := json.Unmarshal(resp1.Body, &resp1Data)
			Expect(err).ToNot(HaveOccurred())
			correlationID := resp1Data.RemediationRequestName

			// Set RR to Pending
			crd := getCRDByName(ctx, testClient, sharedNamespace, correlationID)
			Expect(crd).ToNot(BeNil())
			crd.Status.OverallPhase = "Pending"
			err = testClient.Client.Status().Update(ctx, crd)
			Expect(err).ToNot(HaveOccurred())

			Eventually(func() string {
				c := getCRDByName(ctx, testClient, sharedNamespace, correlationID)
				if c == nil {
					return ""
				}
				return string(c.Status.OverallPhase)
			}, 3*time.Second, 500*time.Millisecond).Should(Equal("Pending"))

			By("2. Send 10 duplicate alerts (trigger storm detection)")
			for i := 0; i < 10; i++ {
				resp := sendWebhook(gatewayURL, "/api/v1/signals/prometheus", prometheusPayload)
				Expect(resp.StatusCode).To(Equal(http.StatusAccepted),
					fmt.Sprintf("Alert %d should be deduplicated", i+2))
			}

			By("3. Query Data Storage for storm audit event")
			queryURL := fmt.Sprintf("%s/api/v1/audit-events?event_category=gateway&event_type=storm.detected&correlation_id=%s",
				dataStorageURL, correlationID)

			var auditEvents []map[string]interface{}
			Eventually(func() int {
				auditResp, err := http.Get(queryURL)
				if err != nil {
					return 0
				}
				defer auditResp.Body.Close()

				if auditResp.StatusCode != http.StatusOK {
					return 0
				}

				var result struct {
					Events []map[string]interface{} `json:"events"`
					Total  int                      `json:"total"`
				}
				if err := json.NewDecoder(auditResp.Body).Decode(&result); err != nil {
					return 0
				}

				auditEvents = result.Events
				return result.Total
			}, 10*time.Second, 500*time.Millisecond).Should(BeNumerically(">=", 1),
				"BR-GATEWAY-192: Gateway MUST emit 'storm.detected' audit event")

			By("4. Verify storm audit event content")
			event := auditEvents[0]
			Expect(event["event_type"]).To(Equal("storm.detected"))
			Expect(event["event_outcome"]).To(Equal("success"))

			eventData := event["event_data"].(map[string]interface{})
			gatewayData := eventData["gateway"].(map[string]interface{})

			Expect(gatewayData["storm_detected"]).To(BeTrue(),
				"storm_detected should be true")
			Expect(gatewayData["storm_id"]).ToNot(BeEmpty(),
				"storm_id should be set")
		})
	})
})
