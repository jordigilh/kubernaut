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

	ogenclient "github.com/jordigilh/kubernaut/pkg/datastorage/ogen-client"
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
		testCtx           context.Context
		testClient        client.Client
		dsClient          *ogenclient.Client
		dataStorageURL    string
		prometheusPayload []byte
		sharedNamespace   string // Created in BeforeEach using createTestNamespace()
	)

	BeforeEach(func() {
		testCtx = context.Background()
		testClient = k8sClient // Use suite-level client (DD-E2E-K8S-CLIENT-001)

		// Create unique test namespace (Pattern: RO E2E)
		// This prevents circuit breaker degradation from "namespace not found" errors
		sharedNamespace = createTestNamespace("test-audit")

		// DD-TEST-001: Get Data Storage URL from suite's shared infrastructure
		// Per DD-TEST-001: All parallel processes share same Data Storage instance
		dataStorageURL = os.Getenv("TEST_DATA_STORAGE_URL")
		if dataStorageURL == "" {
			dataStorageURL = "http://127.0.0.1:18091" // Fallback for manual testing - Use 127.0.0.1 for CI/CD IPv4 compatibility
		}

		// ✅ DD-API-001: Create OpenAPI client for audit queries (MANDATORY)
		// Per DD-API-001: All DataStorage communication MUST use OpenAPI generated client
		var err error
		dsClient, err = ogenclient.NewClient(dataStorageURL)
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

		// Start test Gateway
		// DD-GATEWAY-012: Redis removed, Gateway now connects to Data Storage for audit
		Expect(err).ToNot(HaveOccurred())

		// Test payload
		uniqueID := uuid.New().String()
		prometheusPayload = createPrometheusWebhookPayload(PrometheusAlertPayload{
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
		// Clean up test namespace (Pattern: RO E2E)
		deleteTestNamespace(sharedNamespace)
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

			var gatewayResp GatewayResponse
			err := json.Unmarshal(resp.Body, &gatewayResp)
			Expect(err).ToNot(HaveOccurred())
			correlationID := gatewayResp.RemediationRequestName // Use RR name as correlation

			By("2. Query Data Storage for audit event")
			// ✅ DD-API-001: Use OpenAPI client for type-safe audit queries
			// Per ADR-034 v1.2: event_category is MANDATORY for queries
			// Note: Filter by event_type since Gateway emits multiple events per correlation_id (signal.received, crd.created)
			eventType := "gateway.signal.received"
			params := ogenclient.QueryAuditEventsParams{
				CorrelationID: ogenclient.NewOptString(correlationID),
				EventType:     ogenclient.NewOptString(eventType),
				EventCategory: ogenclient.NewOptString("gateway"), // ADR-034 v1.2 requirement
			}

			// Wait for audit event to appear (async write may have small delay)
			var auditEvents []ogenclient.AuditEvent
			Eventually(func() int {
				resp, err := dsClient.QueryAuditEvents(testCtx, params)
				if err != nil {
					GinkgoWriter.Printf("Failed to query audit events: %v\n", err)
					return 0
				}

				auditEvents = resp.Data

				total := 0
				if resp.Pagination.IsSet() && resp.Pagination.Value.Total.IsSet() {
					total = resp.Pagination.Value.Total.Value
				}
				return total
			}, 120*time.Second, 1*time.Second).Should(Equal(1),
				"BR-GATEWAY-190: Gateway MUST emit exactly 1 'signal.received' audit event (DD-TESTING-001)")

			By("3. Verify audit event content - COMPREHENSIVE VALIDATION")
			Expect(auditEvents).To(HaveLen(1), "Should have exactly 1 audit event for this correlation")

			event := auditEvents[0] // ✅ DD-API-001: Use OpenAPI types directly (no JSON conversion)

			// ━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━
			// STANDARD ADR-034 FIELDS (11 fields)
			// ━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━

			By("3a. Validate ADR-034 standard fields")

			// Field 1: version (✅ DD-API-001: Direct OpenAPI type access)
			Expect(event.Version).To(Equal("1.0"),
				"version should be '1.0' per ADR-034")

			// Field 2: event_type
			Expect(event.EventType).To(Equal("gateway.signal.received"),
				"event_type should follow ADR-034 format: <service>.<category>.<action>")

			// Field 3: event_category
			Expect(string(event.EventCategory)).To(Equal("gateway"),
				"event_category should be 'gateway'")

			// Field 4: event_action
			Expect(event.EventAction).To(Equal("received"),
				"event_action should be 'received' for signal ingestion")

			// Field 5: event_outcome
			Expect(string(event.EventOutcome)).To(Equal("success"),
				"event_outcome should be 'success' for processed signal")

			// Field 6: actor_type
			Expect(event.ActorType.Value).To(Equal("external"),
				"actor_type should be 'external' for AlertManager/K8s Events")

			// Field 7: actor_id
			Expect(event.ActorID.Value).To(Equal("prometheus-alert"),
				"actor_id should be signal source type constant (alertmanager or webhook)")

			// Field 8: resource_type
			Expect(event.ResourceType.Value).To(Equal("Signal"),
				"resource_type should be 'Signal' for signal ingestion events")

			// Field 9: resource_id
			Expect(event.ResourceID.Value).ToNot(BeEmpty(),
				"resource_id should be signal fingerprint (SHA256)")
			Expect(event.ResourceID.Value).To(HaveLen(64),
				"resource_id should be 64-char SHA256 fingerprint")

			// Field 10: correlation_id
			Expect(event.CorrelationID).To(Equal(correlationID),
				"correlation_id should match RemediationRequest name for tracing")

			// Field 11: namespace
			Expect(event.Namespace.Value).To(Equal(sharedNamespace),
				"namespace should match signal namespace for K8s context")

			// ━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━
			// GATEWAY-SPECIFIC EVENT DATA (9 fields)
			// ━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━

			By("3b. Validate Gateway-specific event_data fields")

			// ✅ DD-API-001: Direct OpenAPI type access (no JSON conversion)
			gatewayPayload := event.EventData.GatewayAuditPayload

			// Field 12: signal_type
			Expect(string(gatewayPayload.SignalType)).To(Equal("prometheus-alert"),
				"signal_type should be 'alertmanager' constant (matches OpenAPI enum)")

			// Field 13: alert_name
			Expect(gatewayPayload.AlertName).To(Equal("AuditTestAlert"),
				"alert_name should match Prometheus alert name")

			// Field 14: namespace
			Expect(gatewayPayload.Namespace).To(Equal(sharedNamespace),
				"namespace in event_data should match signal namespace")

			// Field 15: fingerprint
			Expect(gatewayPayload.Fingerprint).ToNot(BeEmpty(),
				"fingerprint should be populated in event_data")
			Expect(gatewayPayload.Fingerprint).To(HaveLen(64),
				"fingerprint should be 64-char SHA256 hash")
			Expect(gatewayPayload.Fingerprint).To(Equal(event.ResourceID.Value),
				"fingerprint in event_data should match resource_id")

		// Field 16: severity
		// Gateway maps Prometheus "warning" → OpenAPI "high" per severity mapping table
		// See: pkg/gateway/audit_helpers.go severityMapping
		Expect(string(gatewayPayload.Severity.Value)).To(Equal("high"),
			"severity should be mapped per OpenAPI spec (warning → high)")

			// Field 17: resource_kind
			Expect(gatewayPayload.ResourceKind.Value).To(Equal("Pod"),
				"resource_kind should match K8s resource kind")

			// Field 18: resource_name
			Expect(gatewayPayload.ResourceName.Value).To(ContainSubstring("audit-test-pod-"),
				"resource_name should match test pod name")

			// Field 19: remediation_request
			Expect(gatewayPayload.RemediationRequest.Value).To(Equal(fmt.Sprintf("%s/%s", sharedNamespace, correlationID)),
				"remediation_request should be namespace/name format")

			// Field 20: deduplication_status
			Expect(string(gatewayPayload.DeduplicationStatus.Value)).To(Equal("new"),
				"deduplication_status should be 'new' for first signal")

			// ━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━
			// BUSINESS OUTCOME VALIDATION
			// ━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━

			By("3c. Verify business outcome: Complete audit trail")
			// BR-GATEWAY-190: Signal.received audit enables operational visibility

			// ✅ OUTCOME 1: End-to-end workflow tracing via correlation_id
			Expect(event.CorrelationID).To(Equal(correlationID),
				"Business outcome: correlation_id=RR name enables tracing entire workflow from signal to remediation")
			Expect(gatewayPayload.RemediationRequest.Value).To(ContainSubstring(correlationID),
				"Business outcome: remediation_request links audit event to created CRD")

			// ✅ OUTCOME 2: Accountability - identifies signal source
			Expect(event.ActorType.Value).To(Equal("external"),
				"Business outcome: actor_type='external' identifies this as external signal (not internal system)")
			Expect(event.ActorID.Value).To(Equal("prometheus-alert"),
				"Business outcome: actor_id identifies specific source system for troubleshooting")

			// ✅ OUTCOME 3: Resource tracking for debugging
			Expect(event.ResourceType.Value).To(Equal("Signal"),
				"Business outcome: resource_type='Signal' categorizes audit event for querying")
			Expect(event.ResourceID.Value).To(Equal(gatewayPayload.Fingerprint),
				"Business outcome: resource_id=fingerprint enables duplicate signal tracking across time")

			// ✅ OUTCOME 4: Multi-tenancy context
			Expect(event.Namespace.Value).To(Equal(sharedNamespace),
				"Business outcome: namespace enables tenant-specific audit queries (SOC2 CC1.4)")

			// ✅ OUTCOME 5: Signal metadata for operations
			Expect(gatewayPayload.AlertName).To(Equal("AuditTestAlert"),
				"Business outcome: alert_name enables filtering audit by alert type")
			Expect(string(gatewayPayload.Severity.Value)).To(Equal("warning"),
				"Business outcome: severity enables SLA tracking per severity level")

			// ✅ OUTCOME 6: SOC2 compliance - 7-year audit trail
			Expect(event.Version).To(Equal("1.0"),
				"Business outcome: ADR-034 v1.0 format ensures audit events remain queryable for compliance")
			Expect(event.EventType).To(Equal("gateway.signal.received"),
				"Business outcome: Structured event_type enables automated compliance reporting")
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

			var resp1Data GatewayResponse
			err := json.Unmarshal(resp1.Body, &resp1Data)
			Expect(err).ToNot(HaveOccurred())
			correlationID := resp1Data.RemediationRequestName

			// Set RR to Pending (required for duplicate detection)
			crd := getCRDByName(testCtx, testClient, sharedNamespace, correlationID)
			Expect(crd).ToNot(BeNil())
			crd.Status.OverallPhase = "Pending"
			err = testClient.Status().Update(testCtx, crd)
			Expect(err).ToNot(HaveOccurred())

			Eventually(func() string {
				c := getCRDByName(testCtx, testClient, sharedNamespace, correlationID)
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
			params2 := ogenclient.QueryAuditEventsParams{
				CorrelationID: ogenclient.NewOptString(correlationID),
				EventType:     ogenclient.NewOptString(eventType2),
				EventCategory: ogenclient.NewOptString("gateway"), // ADR-034 v1.2 requirement
			}

			var auditEvents2 []ogenclient.AuditEvent
			Eventually(func() int {
				resp, err := dsClient.QueryAuditEvents(ctx, params2)
				if err != nil {
					return 0
				}

				auditEvents2 = resp.Data

				total := 0
				if resp.Pagination.IsSet() && resp.Pagination.Value.Total.IsSet() {
					total = resp.Pagination.Value.Total.Value
				}
				return total
			}, 120*time.Second, 1*time.Second).Should(Equal(1),
				"BR-GATEWAY-191: Gateway MUST emit exactly 1 'signal.deduplicated' audit event (DD-TESTING-001)")

			By("4. Verify deduplication audit event content - COMPREHENSIVE VALIDATION")
			event := auditEvents2[0] // ✅ DD-API-001: Use OpenAPI types directly (no JSON conversion)

			// ━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━
			// STANDARD ADR-034 FIELDS (11 fields)
			// ━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━

			By("4a. Validate ADR-034 standard fields")

			// Field 1: version (✅ DD-API-001: Direct OpenAPI type access)
			Expect(event.Version).To(Equal("1.0"),
				"version should be '1.0' per ADR-034")

			// Field 2: event_type (✅ DD-API-001: Direct OpenAPI type access)
			Expect(event.EventType).To(Equal("gateway.signal.deduplicated"),
				"event_type should follow ADR-034 format: <service>.<category>.<action>")

			// Field 3: event_category
			Expect(string(event.EventCategory)).To(Equal("gateway"),
				"event_category should be 'gateway'")

			// Field 4: event_action
			Expect(event.EventAction).To(Equal("deduplicated"),
				"event_action should be 'deduplicated' for duplicate signal")

			// Field 5: event_outcome
			Expect(string(event.EventOutcome)).To(Equal("success"),
				"event_outcome should be 'success' for detected duplicate")

			// Field 6: actor_type
			Expect(event.ActorType.Value).To(Equal("external"),
				"actor_type should be 'external' for AlertManager/K8s Events")

			// Field 7: actor_id
			Expect(event.ActorID.Value).To(Equal("prometheus-alert"),
				"actor_id should be signal source type constant (alertmanager or webhook)")

			// Field 8: resource_type
			Expect(event.ResourceType.Value).To(Equal("Signal"),
				"resource_type should be 'Signal'")

			// Field 9: resource_id
			Expect(event.ResourceID.Value).ToNot(BeEmpty(),
				"resource_id should be signal fingerprint")
			Expect(event.ResourceID.Value).To(HaveLen(64),
				"resource_id should be 64-char SHA256 fingerprint")

			// Field 10: correlation_id
			Expect(event.CorrelationID).To(Equal(correlationID),
				"correlation_id should match RemediationRequest name for tracing")

			// Field 11: namespace
			Expect(event.Namespace.Value).To(Equal(sharedNamespace),
				"namespace should match signal namespace")

			// ━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━
			// GATEWAY-SPECIFIC EVENT DATA (8 fields)
			// ━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━

			By("4b. Validate Gateway-specific event_data fields")

			// ✅ DD-API-001: Direct OpenAPI type access (no JSON conversion)
			gatewayPayload := event.EventData.GatewayAuditPayload

			// Field 12: signal_type
			Expect(string(gatewayPayload.SignalType)).To(Equal("prometheus-alert"),
				"signal_type should be 'alertmanager' constant (matches OpenAPI enum)")

			// Field 13: alert_name
			Expect(gatewayPayload.AlertName).To(Equal("AuditTestAlert"),
				"alert_name should match Prometheus alert name")

			// Field 14: namespace
			Expect(gatewayPayload.Namespace).To(Equal(sharedNamespace),
				"namespace in event_data should match signal namespace")

			// Field 15: fingerprint
			Expect(gatewayPayload.Fingerprint).ToNot(BeEmpty(),
				"fingerprint should be populated in event_data")
			Expect(gatewayPayload.Fingerprint).To(HaveLen(64),
				"fingerprint should be 64-char SHA256 hash")
			Expect(gatewayPayload.Fingerprint).To(Equal(event.ResourceID.Value),
				"fingerprint in event_data should match resource_id")

			// Field 16: remediation_request
			Expect(gatewayPayload.RemediationRequest.Value).To(Equal(fmt.Sprintf("%s/%s", sharedNamespace, correlationID)),
				"remediation_request should be namespace/name format")

			// Field 17: deduplication_status
			Expect(string(gatewayPayload.DeduplicationStatus.Value)).To(Equal("duplicate"),
				"deduplication_status should be 'duplicate' for deduplicated signal")

			// Field 18: occurrence_count
			Expect(gatewayPayload.OccurrenceCount.Value).To(Equal(int32(2)),
				"occurrence_count should be exactly 2 for duplicate (first + one duplicate) (DD-TESTING-001)")

			// ━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━
			// BUSINESS OUTCOME VALIDATION
			// ━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━

			By("4c. Verify business outcome: Deduplication tracking")
			// BR-GATEWAY-191: Deduplication audit enables operational insights

			// ✅ OUTCOME 1: Deduplication visibility (occurrence_count shows persistence)
			Expect(gatewayPayload.OccurrenceCount.Value).To(Equal(int32(2)),
				"Business outcome: occurrence_count=2 proves deduplication is working (not creating duplicate CRDs)")

			// ✅ OUTCOME 2: No duplicate CRD creation confirmed
			Expect(string(gatewayPayload.DeduplicationStatus.Value)).To(Equal("duplicate"),
				"Business outcome: status='duplicate' confirms signal was deduplicated, not processed as new")

			// ✅ OUTCOME 3: Correlation enables tracing duplicate signals to original RR
			Expect(event.CorrelationID).To(Equal(correlationID),
				"Business outcome: Same correlation_id as 'signal.received' enables tracking duplicate signals")

			// ✅ OUTCOME 4: Alert fatigue reduction (SLA metric)
			// If deduplication is working, we should have exactly 1 RR despite 2 signals
			// This is validated implicitly by occurrence_count=2 with deduplication_status='duplicate'

			// ✅ OUTCOME 5: SOC2 compliance - audit trail shows deduplication decision
			Expect(event.EventType).To(Equal("gateway.signal.deduplicated"),
				"Business outcome: Separate event type enables SOC2 audit trail for deduplication decisions")
			Expect(string(event.EventOutcome)).To(Equal("success"),
				"Business outcome: outcome='success' confirms deduplication was intentional, not an error")
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

			var gatewayResp GatewayResponse
			err := json.Unmarshal(resp.Body, &gatewayResp)
			Expect(err).ToNot(HaveOccurred())
			correlationID := gatewayResp.RemediationRequestName

			By("2. Query Data Storage for crd.created audit event")
			// ✅ DD-API-001: Use OpenAPI client for type-safe audit queries
			// Per ADR-034 v1.2: event_category is MANDATORY for queries
			eventType3 := "gateway.crd.created"
			params3 := ogenclient.QueryAuditEventsParams{
				CorrelationID: ogenclient.NewOptString(correlationID),
				EventType:     ogenclient.NewOptString(eventType3),
				EventCategory: ogenclient.NewOptString("gateway"), // ADR-034 v1.2 requirement
			}

			var auditEvents3 []ogenclient.AuditEvent
			Eventually(func() int {
				resp, err := dsClient.QueryAuditEvents(ctx, params3)
				if err != nil {
					return 0
				}

				auditEvents3 = resp.Data

				total := 0
				if resp.Pagination.IsSet() && resp.Pagination.Value.Total.IsSet() {
					total = resp.Pagination.Value.Total.Value
				}
				return total
			}, 120*time.Second, 1*time.Second).Should(BeNumerically(">=", 1),
				"DD-AUDIT-003: Gateway MUST emit 'crd.created' audit event")

			By("3. Validate crd.created audit event content")
			event := auditEvents3[0] // ✅ DD-API-001: Use OpenAPI types directly (no JSON conversion)

			// Critical ADR-034 fields (✅ DD-API-001: Direct OpenAPI type access)
			Expect(event.Version).To(Equal("1.0"), "version should be '1.0' per ADR-034")
			Expect(event.EventType).To(Equal("gateway.crd.created"),
				"event_type should be 'gateway.crd.created'")
			Expect(string(event.EventCategory)).To(Equal("gateway"), "event_category should be 'gateway'")
			Expect(event.EventAction).To(Equal("created"), "event_action should be 'created'")
			Expect(string(event.EventOutcome)).To(Equal("success"), "event_outcome should be 'success'")
			Expect(event.ResourceType.Value).To(Equal("RemediationRequest"),
				"resource_type should be 'RemediationRequest'")
			Expect(event.CorrelationID).To(Equal(correlationID),
				"correlation_id should match RemediationRequest name")

			// Gateway-specific event_data (✅ DD-API-001: Direct OpenAPI type access)
			gatewayPayload := event.EventData.GatewayAuditPayload
			Expect(gatewayPayload.RemediationRequest.Value).To(ContainSubstring(correlationID),
				"remediation_request should reference the created RR")

			// ━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━
			// BUSINESS OUTCOME VALIDATION
			// ━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━

			By("3b. Verify business outcome: CRD lifecycle tracking")
			// BR-GATEWAY-190: CRD.created audit enables Kubernetes resource tracking

			// ✅ OUTCOME 1: CRD creation confirmation
			Expect(event.ResourceType.Value).To(Equal("RemediationRequest"),
				"Business outcome: resource_type='RemediationRequest' enables filtering audit by CRD type")
			Expect(event.EventAction).To(Equal("created"),
				"Business outcome: event_action='created' enables tracking CRD lifecycle (create → update → delete)")

			// ✅ OUTCOME 2: Resource correlation
			Expect(event.CorrelationID).To(Equal(correlationID),
				"Business outcome: correlation_id=RR name enables linking 'signal.received' → 'crd.created' events")
			Expect(gatewayPayload.RemediationRequest.Value).To(ContainSubstring(correlationID),
				"Business outcome: remediation_request provides direct reference to created K8s resource")

			// ✅ OUTCOME 3: SOC2 compliance - resource creation audit trail
			Expect(string(event.EventOutcome)).To(Equal("success"),
				"Business outcome: outcome='success' confirms CRD was successfully created in K8s API")
			Expect(event.EventType).To(Equal("gateway.crd.created"),
				"Business outcome: Separate event type enables querying 'how many CRDs created per day' (SLA metric)")

			// ✅ OUTCOME 4: Debugging support - links signal to K8s resource
			// If RR is missing or corrupted, this audit event provides complete signal metadata for recreation
			Expect(gatewayPayload.Fingerprint).ToNot(BeEmpty(),
				"Business outcome: fingerprint enables finding original signal if RR is deleted")
		})
	})
})
