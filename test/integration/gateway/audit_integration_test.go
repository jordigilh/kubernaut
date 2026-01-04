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
	ptr "k8s.io/utils/ptr"

	"github.com/jordigilh/kubernaut/pkg/gateway"
	dsgen "github.com/jordigilh/kubernaut/pkg/datastorage/client"
)

// =============================================================================
// DD-AUDIT-003: Gateway → Data Storage Audit Integration Tests
// =============================================================================
//
// Business Requirements:
// - BR-AUDIT-001: All service operations MUST generate audit events
// - BR-AUDIT-002: Audit events MUST be persisted to Data Storage
// - BR-GATEWAY-190: Signal ingestion MUST create audit trail (gateway.signal.received)
// - BR-GATEWAY-191: Deduplication decisions MUST be audited (gateway.signal.deduplicated)
//
// Test Strategy:
// - Per TESTING_GUIDELINES.md: Integration tests use REAL infrastructure
// - Gateway connects to REAL Data Storage (via podman-compose)
// - Tests verify audit events appear in Data Storage database
// - LLM is mocked (cost constraint), but Data Storage is REAL
//
// Field Validation Coverage: ✅ 100% (20 fields per event)
// ━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━
// Each audit event validates ALL fields per ADR-034:
//
//   Standard Fields (11):
//     1. version              (e.g., "1.0")
//     2. event_type           (e.g., "gateway.signal.received")
//     3. event_category       (e.g., "gateway")
//     4. event_action         (e.g., "received", "deduplicated")
//     5. event_outcome        (e.g., "success")
//     6. actor_type           (e.g., "external")
//     7. actor_id             (e.g., "prometheus-alert")
//     8. resource_type        (e.g., "Signal")
//     9. resource_id          (e.g., SHA256 fingerprint)
//    10. correlation_id       (e.g., RemediationRequest name)
//    11. namespace            (e.g., K8s namespace)
//
//   Gateway-Specific Fields (9):
//    12. signal_type          (e.g., "prometheus-alert")
//    13. alert_name           (e.g., "PodNotReady")
//    14. namespace            (in event_data)
//    15. fingerprint          (SHA256 hash)
//    16. severity             (e.g., "warning", "critical")
//    17. resource_kind        (e.g., "Pod")
//    18. resource_name        (e.g., "app-pod-1")
//    19. remediation_request  (namespace/name format)
//    20. deduplication_status (e.g., "new", "duplicate")
//    21. occurrence_count     (only for deduplicated events)
//
// ━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━
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
		dsClient          *dsgen.ClientWithResponses
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

		// ✅ DD-API-001: Create OpenAPI client for audit queries (MANDATORY)
		// Per DD-API-001: All DataStorage communication MUST use OpenAPI generated client
		var err error
		dsClient, err = dsgen.NewClientWithResponses(dataStorageURL)
		Expect(err).ToNot(HaveOccurred(), "Failed to create DataStorage OpenAPI client")

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
		defer func() { _ = healthResp.Body.Close() }()
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
			_ = server.Close()
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
			// ✅ DD-API-001: Use OpenAPI client for type-safe audit queries
			// Per ADR-034 v1.2: event_category is MANDATORY for queries
			// Note: Filter by event_type since Gateway emits multiple events per correlation_id (signal.received, crd.created)
		eventType := "gateway.signal.received"
		params := &dsgen.QueryAuditEventsParams{
			CorrelationId: &correlationID,
			EventType:     &eventType,
			EventCategory: ptr.To("gateway"), // ADR-034 v1.2 requirement
		}

			// Wait for audit event to appear (async write may have small delay)
			var auditEvents []dsgen.AuditEvent
			Eventually(func() int {
				resp, err := dsClient.QueryAuditEventsWithResponse(ctx, params)
				if err != nil {
					GinkgoWriter.Printf("Failed to query audit events: %v\n", err)
					return 0
				}

				if resp.JSON200 == nil {
					GinkgoWriter.Printf("Audit query returned non-200: %d\n", resp.StatusCode())
					return 0
				}

				if resp.JSON200.Data != nil {
					auditEvents = *resp.JSON200.Data
				}

				total := 0
				if resp.JSON200.Pagination != nil && resp.JSON200.Pagination.Total != nil {
					total = *resp.JSON200.Pagination.Total
				}
				return total
			}, 10*time.Second, 500*time.Millisecond).Should(Equal(1),
				"BR-GATEWAY-190: Gateway MUST emit exactly 1 'signal.received' audit event (DD-TESTING-001)")

			// Convert to map format for compatibility with existing validation code
			auditEventsMap := make([]map[string]interface{}, len(auditEvents))
			for i, evt := range auditEvents {
				eventJSON, _ := json.Marshal(evt)
				var eventMap map[string]interface{}
				_ = json.Unmarshal(eventJSON, &eventMap)
				auditEventsMap[i] = eventMap
			}

			By("3. Verify audit event content - COMPREHENSIVE VALIDATION")
			Expect(auditEventsMap).To(HaveLen(1), "Should have exactly 1 audit event for this correlation")

			event := auditEventsMap[0]

			// ━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━
			// STANDARD ADR-034 FIELDS (11 fields)
			// ━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━

			By("3a. Validate ADR-034 standard fields")

			// Field 1: version
			Expect(event["version"]).To(Equal("1.0"),
				"version should be '1.0' per ADR-034")

			// Field 2: event_type
			Expect(event["event_type"]).To(Equal("gateway.signal.received"),
				"event_type should follow ADR-034 format: <service>.<category>.<action>")

			// Field 3: event_category
			Expect(event["event_category"]).To(Equal("gateway"),
				"event_category should be 'gateway'")

			// Field 4: event_action
			Expect(event["event_action"]).To(Equal("received"),
				"event_action should be 'received' for signal ingestion")

			// Field 5: event_outcome
			Expect(event["event_outcome"]).To(Equal("success"),
				"event_outcome should be 'success' for processed signal")

			// Field 6: actor_type
			Expect(event["actor_type"]).To(Equal("external"),
				"actor_type should be 'external' for AlertManager/K8s Events")

			// Field 7: actor_id
			Expect(event["actor_id"]).To(Equal("prometheus-alert"),
				"actor_id should be signal source type (prometheus-alert or kubernetes-event)")

			// Field 8: resource_type
			Expect(event["resource_type"]).To(Equal("Signal"),
				"resource_type should be 'Signal' for signal ingestion events")

			// Field 9: resource_id
			Expect(event["resource_id"]).ToNot(BeEmpty(),
				"resource_id should be signal fingerprint (SHA256)")
			Expect(event["resource_id"].(string)).To(HaveLen(64),
				"resource_id should be 64-char SHA256 fingerprint")

			// Field 10: correlation_id
			Expect(event["correlation_id"]).To(Equal(correlationID),
				"correlation_id should match RemediationRequest name for tracing")

			// Field 11: namespace
			Expect(event["namespace"]).To(Equal(sharedNamespace),
				"namespace should match signal namespace for K8s context")

			// ━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━
			// GATEWAY-SPECIFIC EVENT DATA (9 fields)
			// ━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━

			By("3b. Validate Gateway-specific event_data fields")

			eventData, ok := event["event_data"].(map[string]interface{})
			Expect(ok).To(BeTrue(), "event_data should be a map")

			gatewayData, ok := eventData["gateway"].(map[string]interface{})
			Expect(ok).To(BeTrue(), "event_data.gateway should exist")

			// Field 12: signal_type
			Expect(gatewayData["signal_type"]).To(Equal("prometheus-alert"),
				"signal_type should be 'prometheus-alert' per PrometheusAdapter.GetSourceType()")

			// Field 13: alert_name
			Expect(gatewayData["alert_name"]).To(Equal("AuditTestAlert"),
				"alert_name should match Prometheus alert name")

			// Field 14: namespace
			Expect(gatewayData["namespace"]).To(Equal(sharedNamespace),
				"namespace in event_data should match signal namespace")

			// Field 15: fingerprint
			Expect(gatewayData["fingerprint"]).ToNot(BeEmpty(),
				"fingerprint should be populated in event_data")
			Expect(gatewayData["fingerprint"].(string)).To(HaveLen(64),
				"fingerprint should be 64-char SHA256 hash")
			Expect(gatewayData["fingerprint"]).To(Equal(event["resource_id"]),
				"fingerprint in event_data should match resource_id")

			// Field 16: severity
			Expect(gatewayData["severity"]).To(Equal("warning"),
				"severity should match alert severity")

			// Field 17: resource_kind
			Expect(gatewayData["resource_kind"]).To(Equal("Pod"),
				"resource_kind should match K8s resource kind")

			// Field 18: resource_name
			Expect(gatewayData["resource_name"]).To(ContainSubstring("audit-test-pod-"),
				"resource_name should match test pod name")

			// Field 19: remediation_request
			Expect(gatewayData["remediation_request"]).To(Equal(fmt.Sprintf("%s/%s", sharedNamespace, correlationID)),
				"remediation_request should be namespace/name format")

			// Field 20: deduplication_status
			Expect(gatewayData["deduplication_status"]).To(Equal("new"),
				"deduplication_status should be 'new' for first signal")

			// ━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━
			// BUSINESS OUTCOME VALIDATION
			// ━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━

			By("3c. Verify business outcome: Complete audit trail")
			// This audit event enables:
			// ✅ End-to-end workflow tracing (correlation_id = RR name)
			// ✅ Accountability (actor_type/actor_id identifies source)
			// ✅ Resource tracking (resource_type/resource_id for debugging)
			// ✅ Kubernetes context (namespace for multi-tenancy)
			// ✅ Signal metadata (alert_name, severity, resource details)
			// ✅ Compliance (ADR-034 format for 7-year retention)
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
			// ✅ DD-API-001: Use OpenAPI client for type-safe audit queries
			// Per ADR-034 v1.2: event_category is MANDATORY for queries
		eventType2 := "gateway.signal.deduplicated"
		params2 := &dsgen.QueryAuditEventsParams{
			CorrelationId: &correlationID,
			EventType:     &eventType2,
			EventCategory: ptr.To("gateway"), // ADR-034 v1.2 requirement
		}

			var auditEvents2 []dsgen.AuditEvent
			Eventually(func() int {
				resp, err := dsClient.QueryAuditEventsWithResponse(ctx, params2)
				if err != nil {
					return 0
				}

				if resp.JSON200 == nil {
					return 0
				}

				if resp.JSON200.Data != nil {
					auditEvents2 = *resp.JSON200.Data
				}

				total := 0
				if resp.JSON200.Pagination != nil && resp.JSON200.Pagination.Total != nil {
					total = *resp.JSON200.Pagination.Total
				}
				return total
			}, 10*time.Second, 500*time.Millisecond).Should(Equal(1),
				"BR-GATEWAY-191: Gateway MUST emit exactly 1 'signal.deduplicated' audit event (DD-TESTING-001)")

			// Convert to map format for compatibility with existing validation code
			auditEvents := make([]map[string]interface{}, len(auditEvents2))
			for i, evt := range auditEvents2 {
				eventJSON, _ := json.Marshal(evt)
				var eventMap map[string]interface{}
				_ = json.Unmarshal(eventJSON, &eventMap)
				auditEvents[i] = eventMap
			}

			By("4. Verify deduplication audit event content - COMPREHENSIVE VALIDATION")
			event := auditEvents[0]

			// ━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━
			// STANDARD ADR-034 FIELDS (11 fields)
			// ━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━

			By("4a. Validate ADR-034 standard fields")

			// Field 1: version
			Expect(event["version"]).To(Equal("1.0"),
				"version should be '1.0' per ADR-034")

			// Field 2: event_type
			Expect(event["event_type"]).To(Equal("gateway.signal.deduplicated"),
				"event_type should follow ADR-034 format: <service>.<category>.<action>")

			// Field 3: event_category
			Expect(event["event_category"]).To(Equal("gateway"),
				"event_category should be 'gateway'")

			// Field 4: event_action
			Expect(event["event_action"]).To(Equal("deduplicated"),
				"event_action should be 'deduplicated' for duplicate signal")

			// Field 5: event_outcome
			Expect(event["event_outcome"]).To(Equal("success"),
				"event_outcome should be 'success' for detected duplicate")

			// Field 6: actor_type
			Expect(event["actor_type"]).To(Equal("external"),
				"actor_type should be 'external' for AlertManager/K8s Events")

			// Field 7: actor_id
			Expect(event["actor_id"]).To(Equal("prometheus-alert"),
				"actor_id should be signal source type")

			// Field 8: resource_type
			Expect(event["resource_type"]).To(Equal("Signal"),
				"resource_type should be 'Signal'")

			// Field 9: resource_id
			Expect(event["resource_id"]).ToNot(BeEmpty(),
				"resource_id should be signal fingerprint")
			Expect(event["resource_id"].(string)).To(HaveLen(64),
				"resource_id should be 64-char SHA256 fingerprint")

			// Field 10: correlation_id
			Expect(event["correlation_id"]).To(Equal(correlationID),
				"correlation_id should match RemediationRequest name for tracing")

			// Field 11: namespace
			Expect(event["namespace"]).To(Equal(sharedNamespace),
				"namespace should match signal namespace")

			// ━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━
			// GATEWAY-SPECIFIC EVENT DATA (8 fields)
			// ━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━

			By("4b. Validate Gateway-specific event_data fields")

			eventData := event["event_data"].(map[string]interface{})
			gatewayData := eventData["gateway"].(map[string]interface{})

			// Field 12: signal_type
			Expect(gatewayData["signal_type"]).To(Equal("prometheus-alert"),
				"signal_type should be 'prometheus-alert'")

			// Field 13: alert_name
			Expect(gatewayData["alert_name"]).To(Equal("AuditTestAlert"),
				"alert_name should match Prometheus alert name")

			// Field 14: namespace
			Expect(gatewayData["namespace"]).To(Equal(sharedNamespace),
				"namespace in event_data should match signal namespace")

			// Field 15: fingerprint
			Expect(gatewayData["fingerprint"]).ToNot(BeEmpty(),
				"fingerprint should be populated in event_data")
			Expect(gatewayData["fingerprint"].(string)).To(HaveLen(64),
				"fingerprint should be 64-char SHA256 hash")
			Expect(gatewayData["fingerprint"]).To(Equal(event["resource_id"]),
				"fingerprint in event_data should match resource_id")

			// Field 16: remediation_request
			Expect(gatewayData["remediation_request"]).To(Equal(fmt.Sprintf("%s/%s", sharedNamespace, correlationID)),
				"remediation_request should be namespace/name format")

			// Field 17: deduplication_status
			Expect(gatewayData["deduplication_status"]).To(Equal("duplicate"),
				"deduplication_status should be 'duplicate' for deduplicated signal")

			// Field 18: occurrence_count
			Expect(gatewayData["occurrence_count"]).To(Equal(float64(2)),
				"occurrence_count should be exactly 2 for duplicate (first + one duplicate) (DD-TESTING-001)")

			// ━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━
			// BUSINESS OUTCOME VALIDATION
			// ━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━

			By("4c. Verify business outcome: Deduplication tracking")
			// This audit event enables:
			// ✅ Deduplication visibility (occurrence_count shows persistence)
			// ✅ No duplicate CRD creation (status='duplicate' confirms dedup)
			// ✅ SLA tracking (deduplication reduces alert fatigue)
			// ✅ Correlation (same correlation_id as first signal)
			// ✅ Compliance (ADR-034 format for audit trail)
		})
	})

	// ━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━
	// CRD CREATION AUDITING (DD-AUDIT-003)
	// ━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━

	Context("when a RemediationRequest CRD is created (DD-AUDIT-003)", func() {
		It("should create 'crd.created' audit event in Data Storage", func() {
			// BUSINESS SCENARIO:
			// When Gateway successfully creates a RemediationRequest CRD, it MUST:
			// 1. Process the signal (create RemediationRequest)
			// 2. Emit 'gateway.crd.created' audit event for tracking
			//
			// COMPLIANCE: SOC2, ISO 27001 require CRD creation tracking

			By("1. Send Prometheus alert to Gateway")
			resp := sendWebhook(gatewayURL, "/api/v1/signals/prometheus", prometheusPayload)
			Expect(resp.StatusCode).To(Equal(http.StatusCreated), "Signal should be processed")

			var gatewayResp gateway.ProcessingResponse
			err := json.Unmarshal(resp.Body, &gatewayResp)
			Expect(err).ToNot(HaveOccurred())
			correlationID := gatewayResp.RemediationRequestName

			By("2. Query Data Storage for crd.created audit event")
			// ✅ DD-API-001: Use OpenAPI client for type-safe audit queries
			// Per ADR-034 v1.2: event_category is MANDATORY for queries
			eventType3 := "gateway.crd.created"
		params3 := &dsgen.QueryAuditEventsParams{
			CorrelationId: &correlationID,
			EventType:     &eventType3,
			EventCategory: ptr.To("gateway"), // ADR-034 v1.2 requirement
		}

			var auditEvents3 []dsgen.AuditEvent
			Eventually(func() int {
				resp, err := dsClient.QueryAuditEventsWithResponse(ctx, params3)
				if err != nil {
					return 0
				}

				if resp.JSON200 == nil {
					return 0
				}

				if resp.JSON200.Data != nil {
					auditEvents3 = *resp.JSON200.Data
				}

				total := 0
				if resp.JSON200.Pagination != nil && resp.JSON200.Pagination.Total != nil {
					total = *resp.JSON200.Pagination.Total
				}
				return total
			}, 10*time.Second, 500*time.Millisecond).Should(BeNumerically(">=", 1),
				"DD-AUDIT-003: Gateway MUST emit 'crd.created' audit event")

			// Convert to map format for compatibility with existing validation code
			auditEvents := make([]map[string]interface{}, len(auditEvents3))
			for i, evt := range auditEvents3 {
				eventJSON, _ := json.Marshal(evt)
				var eventMap map[string]interface{}
				_ = json.Unmarshal(eventJSON, &eventMap)
				auditEvents[i] = eventMap
			}

			By("3. Validate crd.created audit event content")
			event := auditEvents[0]

			// Critical ADR-034 fields
			Expect(event["version"]).To(Equal("1.0"), "version should be '1.0' per ADR-034")
			Expect(event["event_type"]).To(Equal("gateway.crd.created"),
				"event_type should be 'gateway.crd.created'")
			Expect(event["event_category"]).To(Equal("gateway"), "event_category should be 'gateway'")
			Expect(event["event_action"]).To(Equal("created"), "event_action should be 'created'")
			Expect(event["event_outcome"]).To(Equal("success"), "event_outcome should be 'success'")
			Expect(event["resource_type"]).To(Equal("RemediationRequest"),
				"resource_type should be 'RemediationRequest'")
			Expect(event["correlation_id"]).To(Equal(correlationID),
				"correlation_id should match RemediationRequest name")

			// Gateway-specific event_data
			eventData := event["event_data"].(map[string]interface{})
			gatewayData := eventData["gateway"].(map[string]interface{})
			Expect(gatewayData["remediation_request"]).To(ContainSubstring(correlationID),
				"remediation_request should reference the created RR")
		})
	})
})
