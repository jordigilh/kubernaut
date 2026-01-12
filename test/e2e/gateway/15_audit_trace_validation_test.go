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
	"net/http"
	"time"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/go-logr/logr"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"sigs.k8s.io/controller-runtime/pkg/client"

	dsgen "github.com/jordigilh/kubernaut/pkg/datastorage/ogen-client"
	"github.com/jordigilh/kubernaut/pkg/gateway"
	"github.com/jordigilh/kubernaut/test/shared/validators"

	"github.com/google/uuid"
)

// Test 15: Audit Trace Validation (DD-AUDIT-003)
// Validates that Gateway emits audit events to Data Storage service:
// - Signal ingestion creates 'gateway.signal.received' audit event
// - Audit events are queryable from Data Storage API
// - Audit event content matches ADR-034 schema
//
// Business Requirements:
// - BR-GATEWAY-190: All signal ingestion MUST create audit trail
// - ADR-032 §1.5: "Every alert/signal processed (SignalProcessing, Gateway)"
// - ADR-032 §3: Gateway is P0 (Business-Critical) - MUST have audit
//
// This test validates the E2E integration between Gateway and Data Storage
// for audit trail functionality, ensuring production-ready audit compliance.
var _ = Describe("Test 15: Audit Trace Validation (DD-AUDIT-003)", Ordered, func() {
	var (
		testCtx       context.Context
		testCancel    context.CancelFunc
		testLogger    logr.Logger
		testNamespace string
		httpClient    *http.Client
		k8sClient     client.Client
		auditClient   *dsgen.Client
	)

	BeforeAll(func() {
		testCtx, testCancel = context.WithTimeout(ctx, 5*time.Minute)
		testLogger = logger.WithValues("test", "audit-trace")
		httpClient = &http.Client{Timeout: 10 * time.Second}

		testLogger.Info("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")
		testLogger.Info("Test 15: Audit Trace Validation (DD-AUDIT-003) - Setup")
		testLogger.Info("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")

		// Setup OpenAPI audit client for Data Storage
		// Per SERVICE_MATURITY_REQUIREMENTS.md v1.2.0: MUST use OpenAPI client for audit tests
		dataStorageURL := "http://127.0.0.1:18091" // Kind hostPort maps to NodePort 30081 - Use 127.0.0.1 for CI/CD IPv4 compatibility
		auditClient, _ = dsgen.NewClient(dataStorageURL)

		testLogger.Info("OpenAPI audit client initialized", "dataStorageURL", dataStorageURL)

		// Generate unique namespace
		processID := GinkgoParallelProcess()
		testNamespace = fmt.Sprintf("audit-%d-%s", processID, uuid.New().String()[:8])
		testLogger.Info("Creating test namespace...", "namespace", testNamespace)

		// Create namespace
		ns := &corev1.Namespace{
			ObjectMeta: metav1.ObjectMeta{Name: testNamespace},
		}
		k8sClient = getKubernetesClient()
		Expect(k8sClient.Create(testCtx, ns)).To(Succeed())

		testLogger.Info("✅ Test namespace ready", "namespace", testNamespace)
	})

	AfterAll(func() {
		testLogger.Info("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")
		testLogger.Info("Test 15: Audit Trace Validation - Cleanup")
		testLogger.Info("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")

		if CurrentSpecReport().Failed() {
			testLogger.Info("⚠️  Test FAILED - Preserving namespace for debugging",
				"namespace", testNamespace)
			if testCancel != nil {
				testCancel()
			}
			return
		}

		ns := &corev1.Namespace{
			ObjectMeta: metav1.ObjectMeta{Name: testNamespace},
		}
		_ = k8sClient.Delete(testCtx, ns)

		if testCancel != nil {
			testCancel()
		}

		testLogger.Info("✅ Cleanup complete")
	})

	// ━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━
	// AUDIT TRACE VALIDATION
	// ━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━

	It("should emit audit event to Data Storage when signal is ingested (BR-GATEWAY-190)", func() {
		// This test validates the CRITICAL integration between Gateway and Data Storage
		// for audit trail functionality. This is BLOCKING V1.0 per ADR-032 §1.5.
		//
		// BUSINESS SCENARIO:
		// When Prometheus AlertManager sends an alert, the Gateway MUST:
		// 1. Process the signal (create RemediationRequest)
		// 2. Emit audit event to Data Storage for compliance tracking
		// 3. Make audit event queryable via Data Storage API
		//
		// COMPLIANCE: SOC2, HIPAA require audit trails for all operations

		By("1. Send Prometheus alert to Gateway")
		alertPayload := createPrometheusWebhookPayload(PrometheusAlertPayload{
			AlertName: "AuditTestAlert",
			Namespace: testNamespace,
			Severity:  "critical",
			Annotations: map[string]string{
				"summary":     "Test alert for audit trace validation",
				"description": "This alert tests audit event emission",
			},
		})

		// Use the package-level gatewayURL variable (set in gateway_e2e_suite_test.go)
		// gatewayURL = "http://127.0.0.1:8080" (extraPortMapping hostPort - Use 127.0.0.1 for CI/CD IPv4 compatibility)
		resp, err := func() (*http.Response, error) {
			req23, err := http.NewRequest("POST", gatewayURL+"/api/v1/signals/prometheus", bytes.NewBuffer(alertPayload))
			if err != nil {
				return nil, err
			}
			req23.Header.Set("Content-Type", "application/json")
			req23.Header.Set("X-Timestamp", fmt.Sprintf("%d", time.Now().Unix()))
			return httpClient.Do(req23)
		}()
		Expect(err).ToNot(HaveOccurred())
		defer func() { _ = resp.Body.Close() }()

		testLogger.Info("Gateway response received",
			"status", resp.StatusCode,
			"expected", http.StatusCreated)

		Expect(resp.StatusCode).To(Equal(http.StatusCreated),
			"Signal should be processed successfully")

		var gatewayResp struct {
			Status                 string `json:"status"`
			RemediationRequestName string `json:"remediationRequestName"`
			Fingerprint            string `json:"fingerprint"`
		}
		err = json.NewDecoder(resp.Body).Decode(&gatewayResp)
		Expect(err).ToNot(HaveOccurred())
		Expect(gatewayResp.RemediationRequestName).ToNot(BeEmpty(),
			"Gateway should return RR name")

		correlationID := gatewayResp.RemediationRequestName
		fingerprint := gatewayResp.Fingerprint

		testLogger.Info("✅ Signal processed by Gateway",
			"correlationID", correlationID,
			"fingerprint", fingerprint)

		By("2. Query Data Storage for audit events via OpenAPI client (DD-AUDIT-003)")
		// Per SERVICE_MATURITY_REQUIREMENTS.md v1.2.0: MUST use OpenAPI client for audit tests
		testLogger.Info("Querying Data Storage for audit events via OpenAPI client",
			"correlationID", correlationID)

		// Wait for audit events to appear (async write may have small delay)
		var auditEvents []dsgen.AuditEvent
		Eventually(func() int {
			// Query using OpenAPI client with typed parameters
			// Note: No "Service" parameter - use EventCategory instead
			eventCategory := "gateway"
			resp, err := auditClient.QueryAuditEvents(testCtx, dsgen.QueryAuditEventsParams{
				EventCategory: dsgen.NewOptString(eventCategory),
				CorrelationID: dsgen.NewOptString(correlationID),
			})
			if err != nil {
				testLogger.Info("Failed to query audit events (will retry)", "error", err)
				return 0
			}

			// Access typed response directly (ogen pattern)
			auditEvents = resp.Data
			total := 0
			if resp.Pagination.Set && resp.Pagination.Value.Total.Set {
				total = resp.Pagination.Value.Total.Value
			}
			testLogger.Info("Audit events found", "count", total)
			return total
		}, 30*time.Second, 2*time.Second).Should(Equal(2),
			"BR-GATEWAY-190: Gateway MUST emit exactly 2 audit events (signal.received + crd.created) to Data Storage (DD-TESTING-001)")

		testLogger.Info("✅ Audit events found in Data Storage", "eventCount", len(auditEvents))

		By("3. Validate 'signal.received' audit event using validators.ValidateAuditEvent (P0 requirement)")
		// Gateway emits BOTH 'signal.received' AND 'crd.created' events per DD-AUDIT-003
		// We need to find the 'signal.received' event specifically
		Expect(auditEvents).To(HaveLen(2), "Should have 2 audit events: signal.received + crd.created")

		// Find the 'gateway.signal.received' event
		var signalEvent *dsgen.AuditEvent
		for i := range auditEvents {
			if auditEvents[i].EventType == gateway.EventTypeSignalReceived {
				signalEvent = &auditEvents[i]
				break
			}
		}
		Expect(signalEvent).ToNot(BeNil(), "Should find 'gateway.signal.received' event")

		// ━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━
		// STRUCTURED AUDIT VALIDATION (SERVICE_MATURITY_REQUIREMENTS.md v1.2.0)
		// Per v1.2.0 update (2025-12-20): MUST use validators.ValidateAuditEvent
		// ━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━

		validators.ValidateAuditEvent(*signalEvent, validators.ExpectedAuditEvent{
			EventType:     gateway.EventTypeSignalReceived,
			EventCategory: dsgen.AuditEventEventCategoryGateway,
			EventAction:   "received",
			EventOutcome:  validators.EventOutcomePtr(dsgen.AuditEventEventOutcomeSuccess),
			CorrelationID: correlationID,
			ResourceType:  validators.StringPtr("Signal"),
			ResourceID:    validators.StringPtr(fingerprint),
			Namespace:     validators.StringPtr(testNamespace),
		})

		testLogger.Info("✅ All critical ADR-034 fields validated via validators.ValidateAuditEvent",
			"event_type", signalEvent.EventType,
			"correlation_id", correlationID,
			"fingerprint", fingerprint)

		By("4. Verify Gateway-specific event_data fields")
		// Access strongly-typed Gateway payload (ogen discriminated union)
		gatewayPayload := signalEvent.EventData.GatewayAuditPayload

		// Validate Gateway-specific fields exist (using strongly-typed payload)
		Expect(gatewayPayload.SignalType).ToNot(BeEmpty(),
			"Gateway event_data should include signal_type (e.g., 'prometheus-alert')")
		Expect(gatewayPayload.AlertName).To(Equal("AuditTestAlert"),
			"Gateway event_data should include alert_name")
		Expect(gatewayPayload.Namespace).To(Equal(testNamespace),
			"Gateway event_data should include namespace")
		Expect(gatewayPayload.RemediationRequest.Set).To(BeTrue(),
			"Gateway event_data should include remediation_request reference")
		Expect(gatewayPayload.DeduplicationStatus.Set).To(BeTrue(), "DeduplicationStatus should be set")
		Expect(string(gatewayPayload.DeduplicationStatus.Value)).To(Equal("new"),
			"Gateway event_data should mark first signal as 'new'")

		testLogger.Info("✅ All Gateway-specific event_data fields validated")

		By("5. Verify 'crd.created' audit event using validators.ValidateAuditEvent (DD-AUDIT-003)")
		// Find the 'gateway.crd.created' event
		var crdEvent *dsgen.AuditEvent
		for i := range auditEvents {
			if auditEvents[i].EventType == gateway.EventTypeCRDCreated {
				crdEvent = &auditEvents[i]
				break
			}
		}
		Expect(crdEvent).ToNot(BeNil(), "Should find 'gateway.crd.created' event")

		// Validate using validators.ValidateAuditEvent (P0 requirement per v1.2.0)
		validators.ValidateAuditEvent(*crdEvent, validators.ExpectedAuditEvent{
			EventType:     gateway.EventTypeCRDCreated,
			EventCategory: dsgen.AuditEventEventCategoryGateway,
			EventAction:   "created",
			EventOutcome:  validators.EventOutcomePtr(dsgen.AuditEventEventOutcomeSuccess),
			CorrelationID: correlationID,
			ResourceType:  validators.StringPtr("RemediationRequest"),
			Namespace:     validators.StringPtr(testNamespace),
		})

		testLogger.Info("✅ CRD creation audit event validated via validators.ValidateAuditEvent")

		By("6. BUSINESS OUTCOME: Complete audit trail for compliance")
		// This test proves that Gateway successfully integrates with Data Storage
		// for audit trail functionality, satisfying:
		// ✅ ADR-032 §1.5: "Every alert/signal processed (SignalProcessing, Gateway)"
		// ✅ ADR-032 §3: Gateway is P0 (Business-Critical) with mandatory audit
		// ✅ BR-GATEWAY-190: All signal ingestion creates audit trail
		// ✅ ADR-034: Audit events follow standardized schema
		// ✅ SOC2/HIPAA: Audit trails are queryable for compliance reporting
		// ✅ SERVICE_MATURITY_REQUIREMENTS.md v1.2.0: Uses validators.ValidateAuditEvent (P0)

		testLogger.Info("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")
		testLogger.Info("✅ AUDIT TRACE VALIDATION COMPLETE")
		testLogger.Info("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")
		testLogger.Info("Gateway ➔ Data Storage audit integration: PRODUCTION-READY")
		testLogger.Info("  • Audit events emitted: ✅")
		testLogger.Info("  • ADR-034 schema compliance: ✅ (validated via testutil)")
		testLogger.Info("  • Data Storage queryable: ✅ (via OpenAPI client)")
		testLogger.Info("  • Gateway-specific metadata: ✅")
		testLogger.Info("  • Correlation tracking: ✅")
		testLogger.Info("  • P0 testutil validator: ✅ (SERVICE_MATURITY_REQUIREMENTS.md v1.2.0)")
		testLogger.Info("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")
	})
})

// NOTE: Removed local createPrometheusAlert() - now using shared createPrometheusWebhookPayload() from deduplication_helpers.go
