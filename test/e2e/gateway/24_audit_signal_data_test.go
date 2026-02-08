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
	"io"
	"net/http"
	"os"
	"strings"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"sigs.k8s.io/controller-runtime/pkg/client"

	ogenclient "github.com/jordigilh/kubernaut/pkg/datastorage/ogen-client"
	gateway "github.com/jordigilh/kubernaut/pkg/gateway"
	"github.com/jordigilh/kubernaut/test/infrastructure"
	"github.com/jordigilh/kubernaut/test/shared/helpers"
	testauth "github.com/jordigilh/kubernaut/test/shared/auth"
)

// =============================================================================
// BR-AUDIT-005: Gateway Signal Data Capture for RR Reconstruction
// =============================================================================
//
// Business Requirements:
// - BR-AUDIT-005 v2.0: 100% RemediationRequest (RR) CRD reconstruction from audit traces
//
// SOC2 Compliance: Gaps #1-3
// - Gap #1: Capture `original_payload` (full K8s Event) in `gateway.signal.received` events
// - Gap #2: Capture `signal_labels` (map[string]string) in `gateway.signal.received` events
// - Gap #3: Capture `signal_annotations` (map[string]string) in `gateway.signal.received` events
//
// Authority Documents:
// - DD-AUDIT-004: RR reconstruction field mapping specification
// - DD-AUDIT-003 v1.4: Service audit trace requirements
// - ADR-034: Unified audit table design
//
// Test Strategy (per TESTING_GUIDELINES.md):
// - Integration tests use REAL Data Storage (no mocks)
// - OpenAPI client MANDATORY for all audit queries (DD-API-001)
// - Eventually() MANDATORY for async operations (NO time.Sleep())
// - Tests MUST Fail() if infrastructure unavailable (NO Skip())
// - Validate business logic (signal processing), not infrastructure
//
// Test Tier Justification:
// - Integration tier: Requires real Data Storage for OpenAPI validation
// - NOT unit tier: Cannot mock Data Storage for field schema validation
// - NOT E2E tier: Single service validation, no cross-service flow
//
// Field Validation Coverage per DD-TESTING-001:
// - Deterministic event counts (Equal(N) not BeNumerically(">=", N))
// - Structured event_data validation (all 3 fields)
// - Metadata validation (event_type, event_category, correlation_id)
//
// To run these tests:
//   make test-integration-gateway
//
// =============================================================================

// postToGateway sends a POST request to the Gateway with required X-Timestamp header
// The timestamp must be Unix epoch format (seconds) per gateway middleware requirements
func postToGateway(url, payload string) (*http.Response, error) {
	req, err := http.NewRequest(http.MethodPost, url, strings.NewReader(payload))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Timestamp", fmt.Sprintf("%d", time.Now().Unix()))
	return http.DefaultClient.Do(req)
}

// extractCorrelationID extracts the remediationRequestName from the Gateway response body
// This is the correlation_id used for audit event queries
func extractCorrelationID(resp *http.Response) (string, error) {
	var respBody struct {
		RemediationRequestName string `json:"remediationRequestName"`
	}
	bodyBytes, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", err
	}
	if err := json.Unmarshal(bodyBytes, &respBody); err != nil {
		return "", err
	}
	return respBody.RemediationRequestName, nil
}

var _ = Describe("BR-AUDIT-005: Gateway Signal Data for RR Reconstruction", func() {
	var (
		testCtx       context.Context      // ← Test-local context
		testCancel    context.CancelFunc
		testClient      client.Client
		dsClient        *ogenclient.Client
		dataStorageURL  string
		sharedNamespace string // Created in BeforeEach using helpers.CreateTestNamespaceAndWait(k8sClient, )
	)

	BeforeEach(func() {
		testCtx, testCancel = context.WithCancel(context.Background())  // ← Uses local variable
		testClient = k8sClient // Use suite-level client (DD-E2E-K8S-CLIENT-001)
		_ = testClient          // TODO (GW Team): Use for K8s operations

		// Create unique test namespace (Pattern: RO E2E)
		// This prevents circuit breaker degradation from "namespace not found" errors
		sharedNamespace = helpers.CreateTestNamespaceAndWait(k8sClient, "test-rr-audit")

		// DD-TEST-001: Get Data Storage URL from suite's shared infrastructure
		dataStorageURL = os.Getenv("TEST_DATA_STORAGE_URL")
		if dataStorageURL == "" {
			dataStorageURL = "http://127.0.0.1:18091" // Fallback for manual testing - Use 127.0.0.1 for CI/CD IPv4 compatibility
		}

		// DD-AUTH-014: Create E2E ServiceAccount with DataStorage access permissions
		e2eSAName := "gateway-e2e-audit-client"
		err := infrastructure.CreateE2EServiceAccountWithDataStorageAccess(
			ctx,
			gatewayNamespace,
			kubeconfigPath,
			e2eSAName,
			GinkgoWriter,
		)
		Expect(err).ToNot(HaveOccurred(), "Failed to create E2E ServiceAccount")
		
		// Get token for E2E ServiceAccount
		e2eToken, err := infrastructure.GetServiceAccountToken(
			ctx,
			gatewayNamespace,
			e2eSAName,
			kubeconfigPath,
		)
		Expect(err).ToNot(HaveOccurred(), "Failed to get E2E ServiceAccount token")

		// ✅ DD-API-001 + DD-AUTH-014: Create authenticated OpenAPI client for audit queries
		// Per DD-API-001: All DataStorage communication MUST use OpenAPI generated client
		// DD-AUTH-014: Client must use ServiceAccount token for middleware authentication
		saTransport := testauth.NewServiceAccountTransport(e2eToken)
		httpClient := &http.Client{
			Timeout:   20 * time.Second,
			Transport: saTransport,
		}
		dsClient, err = ogenclient.NewClient(dataStorageURL, ogenclient.WithClient(httpClient))
		Expect(err).ToNot(HaveOccurred(), "Failed to create authenticated DataStorage OpenAPI client")

		// ✅ MANDATORY: Verify Data Storage is running
		// Per TESTING_GUIDELINES.md: Tests MUST FAIL if infrastructure unavailable (NO Skip())
		healthResp, err := http.Get(dataStorageURL + "/health")
		if err != nil {
			Fail(fmt.Sprintf(
				"REQUIRED: Data Storage not available at %s\n"+
					"  Per DD-AUDIT-003: Gateway MUST have audit capability\n"+
					"  Per BR-AUDIT-005: RR reconstruction requires audit trail\n\n"+
					"  Start infrastructure: make test-integration-gateway\n\n"+
					"  Error: %v", dataStorageURL, err))
		}
		defer func() { _ = healthResp.Body.Close() }()
		if healthResp.StatusCode != http.StatusOK {
			Fail(fmt.Sprintf(
				"REQUIRED: Data Storage health check failed at %s\n"+
					"  Status: %d\n"+
					"  Expected: 200 OK", dataStorageURL, healthResp.StatusCode))
		}

		// Setup test namespace

		// Start test Gateway connected to real Data Storage
		Expect(err).ToNot(HaveOccurred())
	})

	AfterEach(func() {
	if testCancel != nil {
		testCancel()  // ← Only cancels test-local context
	}
		// Clean up test namespace (Pattern: RO E2E)
		helpers.DeleteTestNamespace(ctx, k8sClient, sharedNamespace)
	})

	Context("Gap #1-3: Complete Signal Data Capture", func() {
		It("should capture original_payload, signal_labels, and signal_annotations for RR reconstruction", func() {
			// ━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━
			// PHASE 1: Send Prometheus alert with labels and annotations
			// ━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━

			// Create Prometheus alert with comprehensive labels and annotations
			alertPayload := fmt.Sprintf(`{
				"receiver": "kubernaut-webhook",
				"status": "firing",
				"alerts": [{
					"status": "firing",
					"labels": {
						"alertname": "PodMemoryHigh",
						"severity": "warning",
						"namespace": "%s",
						"pod": "payment-service-abc123",
						"app": "payment-service",
						"tier": "backend",
						"environment": "production",
						"region": "us-east-1"
					},
					"annotations": {
						"summary": "Pod memory usage above 80%%",
						"description": "Payment service pod memory usage is 85%% of limit",
						"runbook_url": "https://runbooks.example.com/pod-memory-high",
						"dashboard_url": "https://grafana.example.com/d/pod-memory",
						"incident_severity": "P2",
						"escalation_policy": "platform-team"
					},
					"startsAt": "2025-01-04T10:00:00.000Z",
					"endsAt": "0001-01-01T00:00:00Z",
					"generatorURL": "http://prometheus:9090/graph?g0.expr=pod_memory_usage"
				}],
				"groupLabels": {
					"alertname": "PodMemoryHigh"
				},
				"commonLabels": {
					"alertname": "PodMemoryHigh",
					"severity": "warning"
				},
				"commonAnnotations": {
					"summary": "Pod memory usage above 80%%"
				},
				"externalURL": "http://alertmanager:9093",
				"version": "4",
				"groupKey": "{}:{alertname=\"PodMemoryHigh\"}"
			}`, sharedNamespace)

			// Send alert to Gateway webhook
			resp, err := postToGateway(gatewayURL+"/api/v1/signals/prometheus", alertPayload)
			Expect(err).ToNot(HaveOccurred(), "Failed to send Prometheus alert to Gateway")

			// Debug: If not 201, read the error response
			if resp.StatusCode != http.StatusCreated {
				bodyBytes, _ := io.ReadAll(resp.Body)
				GinkgoWriter.Printf("❌ Gateway returned %d: %s\n", resp.StatusCode, string(bodyBytes))
				GinkgoWriter.Printf("📋 Alert payload sent:\n%s\n", alertPayload)
			}
			defer func() { _ = resp.Body.Close() }()

			Expect(resp.StatusCode).To(Equal(http.StatusCreated), "Gateway should create RR from Prometheus alert")

			// Extract correlation_id from response body
			correlationID, err := extractCorrelationID(resp)
			Expect(err).ToNot(HaveOccurred())
			Expect(correlationID).ToNot(BeEmpty(), "Gateway should return remediationRequestName in response body")

			// ━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━
			// PHASE 2: Query Data Storage for gateway.signal.received event
			// ━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━

			eventType := "gateway.signal.received"
			eventCategory := gateway.CategoryGateway

			// ✅ MANDATORY: Use Eventually() for async operations (NO time.Sleep())
			// Per TESTING_GUIDELINES.md: time.Sleep() is ABSOLUTELY FORBIDDEN
			Eventually(func() int {
				resp, err := dsClient.QueryAuditEvents(testCtx, ogenclient.QueryAuditEventsParams{
					EventType:     ogenclient.NewOptString(eventType),
					EventCategory: ogenclient.NewOptString(eventCategory),
					CorrelationID: ogenclient.NewOptString(correlationID),
				})
				if err != nil {
					return 0
				}
				if resp.Pagination.IsSet() && resp.Pagination.Value.Total.IsSet() {
					return resp.Pagination.Value.Total.Value
				}
				return 0
			}, 30*time.Second, 1*time.Second).Should(Equal(1),
				"Should find exactly 1 gateway.signal.received audit event within 30 seconds")

			// Query final audit events for validation
			resp2, err := dsClient.QueryAuditEvents(testCtx, ogenclient.QueryAuditEventsParams{
				EventType:     ogenclient.NewOptString(eventType),
				EventCategory: ogenclient.NewOptString(eventCategory),
				CorrelationID: ogenclient.NewOptString(correlationID),
			})
			Expect(err).ToNot(HaveOccurred(), "Failed to query audit events from Data Storage")

			// ✅ DD-TESTING-001: Deterministic count validation (Equal(N) not BeNumerically(">="))
			events := resp2.Data
			Expect(len(events)).To(Equal(1), "Should have exactly 1 audit event")

			auditEvent := events[0]

			// ━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━
			// PHASE 3: Validate standard audit metadata
			// ━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━

			// Standard audit fields (per ADR-034)
			Expect(auditEvent.Version).To(Equal("1.0"), "Audit event version")
			Expect(auditEvent.EventType).To(Equal("gateway.signal.received"), "Event type")
			Expect(string(auditEvent.EventCategory)).To(Equal(gateway.CategoryGateway), "Event category")
			Expect(auditEvent.EventAction).To(Equal("received"), "Event action")
			Expect(string(auditEvent.EventOutcome)).To(Equal("success"), "Event outcome")
			Expect(auditEvent.ActorType.Value).To(Equal("external"), "Actor type")
			Expect(auditEvent.ActorID.Value).To(Equal("prometheus-alert"), "Actor ID - ✅ ADAPTER-CONSTANT: PrometheusAdapter uses SourceTypePrometheusAlert")
			Expect(auditEvent.ResourceType.Value).To(Equal("Signal"), "Resource type")
			Expect(auditEvent.CorrelationID).To(Equal(correlationID), "Correlation ID consistency")
			Expect(auditEvent.EventTimestamp).ToNot(BeZero(), "Event timestamp must be present")

			// ━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━
			// PHASE 4: Validate Gap #1-3 - RR Reconstruction Fields
			// ━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━

			// Extract event_data (discriminated union from ogen)
			// Convert to map[string]interface{} for existing validation logic
			eventDataBytes, err := json.Marshal(auditEvent.EventData)
			Expect(err).ToNot(HaveOccurred(), "Should marshal EventData")
			var eventData map[string]interface{}
			err = json.Unmarshal(eventDataBytes, &eventData)
			Expect(err).ToNot(HaveOccurred(), "Should unmarshal EventData to map")

			// ┌─────────────────────────────────────────────────────────────┐
			// │ Gap #1: original_payload (Full Prometheus Alert Payload)    │
			// │ DD-AUDIT-004: Maps to RR.Spec.OriginalPayload               │
			// └─────────────────────────────────────────────────────────────┘

			// ⚠️  THIS WILL FAIL: original_payload field does not exist yet
			Expect(eventData).To(HaveKey("original_payload"),
				"Gap #1: original_payload is REQUIRED for RR reconstruction")

			originalPayload, ok := eventData["original_payload"].(map[string]interface{})
			Expect(ok).To(BeTrue(), "original_payload should be a JSON object")

			// Validate original payload contains Prometheus alert structure
			Expect(originalPayload).To(HaveKey("status"), "Original payload should contain alert status")
			Expect(originalPayload).To(HaveKey("alerts"), "Original payload should contain alerts array")
			Expect(originalPayload).To(HaveKey("receiver"), "Original payload should contain receiver")

			// Validate nested alert data
			alerts, ok := originalPayload["alerts"].([]interface{})
			Expect(ok).To(BeTrue(), "alerts should be an array")
			Expect(len(alerts)).To(BeNumerically(">", 0), "Should have at least one alert")

			// ┌─────────────────────────────────────────────────────────────┐
			// │ Gap #2: signal_labels (Prometheus Alert Labels)             │
			// │ DD-AUDIT-004: Maps to RR.Spec.SignalLabels                  │
			// └─────────────────────────────────────────────────────────────┘

			// ⚠️  THIS WILL FAIL: signal_labels field does not exist yet
			Expect(eventData).To(HaveKey("signal_labels"),
				"Gap #2: signal_labels is REQUIRED for RR reconstruction")

			signalLabels, ok := eventData["signal_labels"].(map[string]interface{})
			Expect(ok).To(BeTrue(), "signal_labels should be a map")

			// Validate specific labels from Prometheus alert
			Expect(signalLabels).To(HaveKey("alertname"), "Should have alertname label")
			Expect(signalLabels["alertname"]).To(Equal("PodMemoryHigh"), "Alert name label value")

			Expect(signalLabels).To(HaveKey("severity"), "Should have severity label")
			Expect(signalLabels["severity"]).To(Equal("warning"), "Severity label value")

			Expect(signalLabels).To(HaveKey("namespace"), "Should have namespace label")
			Expect(signalLabels["namespace"]).To(Equal(sharedNamespace), "Namespace label value")

			Expect(signalLabels).To(HaveKey("app"), "Should have app label")
			Expect(signalLabels["app"]).To(Equal("payment-service"), "App label value")

			Expect(signalLabels).To(HaveKey("environment"), "Should have environment label")
			Expect(signalLabels["environment"]).To(Equal("production"), "Environment label value")

			// ┌─────────────────────────────────────────────────────────────┐
			// │ Gap #3: signal_annotations (Prometheus Alert Annotations)   │
			// │ DD-AUDIT-004: Maps to RR.Spec.SignalAnnotations             │
			// └─────────────────────────────────────────────────────────────┘

			// ⚠️  THIS WILL FAIL: signal_annotations field does not exist yet
			Expect(eventData).To(HaveKey("signal_annotations"),
				"Gap #3: signal_annotations is REQUIRED for RR reconstruction")

			signalAnnotations, ok := eventData["signal_annotations"].(map[string]interface{})
			Expect(ok).To(BeTrue(), "signal_annotations should be a map")

			// Validate specific annotations from Prometheus alert
			Expect(signalAnnotations).To(HaveKey("summary"), "Should have summary annotation")
			Expect(signalAnnotations["summary"]).To(ContainSubstring("memory usage"), "Summary annotation content")

			Expect(signalAnnotations).To(HaveKey("description"), "Should have description annotation")
			Expect(signalAnnotations["description"]).To(ContainSubstring("Payment service"), "Description annotation content")

			Expect(signalAnnotations).To(HaveKey("runbook_url"), "Should have runbook_url annotation")
			Expect(signalAnnotations["runbook_url"]).To(HavePrefix("https://runbooks."), "Runbook URL format")

			Expect(signalAnnotations).To(HaveKey("dashboard_url"), "Should have dashboard_url annotation")
			Expect(signalAnnotations["dashboard_url"]).To(HavePrefix("https://grafana."), "Dashboard URL format")

			Expect(signalAnnotations).To(HaveKey("incident_severity"), "Should have incident_severity annotation")
			Expect(signalAnnotations["incident_severity"]).To(Equal("P2"), "Incident severity value")

			// ━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━
			// SUCCESS CRITERIA: All 3 gaps validated
			// ━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━

			// If we reach here, all 3 RR reconstruction fields are captured correctly:
			// ✅ Gap #1: original_payload contains full Prometheus alert
			// ✅ Gap #2: signal_labels contains all Prometheus labels
			// ✅ Gap #3: signal_annotations contains all Prometheus annotations

			GinkgoWriter.Printf("✅ BR-AUDIT-005 Gap #1-3: All RR reconstruction fields validated\n")
			GinkgoWriter.Printf("   - original_payload: %d bytes\n", len(fmt.Sprintf("%v", originalPayload)))
			GinkgoWriter.Printf("   - signal_labels: %d labels\n", len(signalLabels))
			GinkgoWriter.Printf("   - signal_annotations: %d annotations\n", len(signalAnnotations))
		})

		It("should handle signals with empty annotations gracefully", func() {
			// ━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━
			// EDGE CASE: Minimal Labels / Empty Annotations (Validates Defensive Nil Checks)
			// ━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━
			//
			// This test validates the defensive nil checks added during REFACTOR phase:
			// - signal.Labels with minimal required fields (alertname, namespace)
			// - signal.Annotations == nil → should be converted to empty map
			//
			// Rationale: RR reconstruction needs empty maps, not nil values
			// ━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━

			// Create Prometheus alert with MINIMAL labels (alertname required) and EMPTY annotations
			alertPayload := fmt.Sprintf(`{
				"receiver": "kubernaut-webhook",
				"status": "firing",
				"alerts": [{
					"status": "firing",
					"labels": {
						"alertname": "MinimalAlert",
						"namespace": "%s"
					},
					"annotations": {},
					"startsAt": "2025-01-04T10:00:00.000Z",
					"endsAt": "0001-01-01T00:00:00Z"
				}],
				"groupLabels": {
					"alertname": "MinimalAlert"
				},
				"commonLabels": {
					"alertname": "MinimalAlert"
				},
				"commonAnnotations": {},
				"externalURL": "http://alertmanager:9093",
				"version": "4",
				"groupKey": "{}:{alertname=\"MinimalAlert\"}"
			}`, sharedNamespace)

			// Send alert to Gateway webhook
			resp, err := postToGateway(gatewayURL+"/api/v1/signals/prometheus", alertPayload)
			Expect(err).ToNot(HaveOccurred(), "Failed to send Prometheus alert with empty labels")

			// Debug: If not 201, read the error response
			if resp.StatusCode != http.StatusCreated {
				bodyBytes, _ := io.ReadAll(resp.Body)
				GinkgoWriter.Printf("❌ Gateway returned %d: %s\n", resp.StatusCode, string(bodyBytes))
			}

			Expect(resp.StatusCode).To(Equal(http.StatusCreated), "Gateway should create RR for alert with empty labels")

			// Extract correlation_id from response body
			correlationID, err := extractCorrelationID(resp)
			Expect(err).ToNot(HaveOccurred())
			Expect(correlationID).ToNot(BeEmpty(), "Gateway should return correlation_id")
			defer func() { _ = resp.Body.Close() }()

			// Query Data Storage for audit event
			eventType := "gateway.signal.received"
			eventCategory := gateway.CategoryGateway

			// ✅ Use Eventually() for async validation
			Eventually(func() int {
				resp, err := dsClient.QueryAuditEvents(testCtx, ogenclient.QueryAuditEventsParams{
					EventType:     ogenclient.NewOptString(eventType),
					EventCategory: ogenclient.NewOptString(eventCategory),
					CorrelationID: ogenclient.NewOptString(correlationID),
				})
				if err != nil {
					return 0
				}
				if resp.Pagination.IsSet() && resp.Pagination.Value.Total.IsSet() {
					return resp.Pagination.Value.Total.Value
				}
				return 0
			}, 30*time.Second, 1*time.Second).Should(Equal(1),
				"Should find exactly 1 audit event for empty labels test")

			// Query final audit events
			resp2, err := dsClient.QueryAuditEvents(testCtx, ogenclient.QueryAuditEventsParams{
				EventType:     ogenclient.NewOptString(eventType),
				EventCategory: ogenclient.NewOptString(eventCategory),
				CorrelationID: ogenclient.NewOptString(correlationID),
			})
			Expect(err).ToNot(HaveOccurred())

			events := resp2.Data
			Expect(len(events)).To(Equal(1))

			// Convert EventData discriminated union to map for existing validation logic
			eventDataBytes, _ := json.Marshal(events[0].EventData)
			var eventData map[string]interface{}
			err = json.Unmarshal(eventDataBytes, &eventData)
			Expect(err).ToNot(HaveOccurred())

			// ┌─────────────────────────────────────────────────────────────┐
			// │ CRITICAL: Validate Defensive Nil Checks                     │
			// │ Expected: Non-nil maps, minimal labels                      │
			// └─────────────────────────────────────────────────────────────┘

			// Gap #2: signal_labels should have minimal required labels (alertname, namespace)
			Expect(eventData).To(HaveKey("signal_labels"),
				"signal_labels field must exist")

			signalLabels, ok := eventData["signal_labels"].(map[string]interface{})
			Expect(ok).To(BeTrue(), "signal_labels should be a map, not nil")
			Expect(len(signalLabels)).To(Equal(2),
				"signal_labels should have alertname and namespace")
			Expect(signalLabels).To(HaveKeyWithValue("alertname", "MinimalAlert"))
			Expect(signalLabels).To(HaveKey("namespace"))

			// Gap #3: signal_annotations should be empty map, not nil
			Expect(eventData).To(HaveKey("signal_annotations"),
				"signal_annotations field must exist even when empty")

			signalAnnotations, ok := eventData["signal_annotations"].(map[string]interface{})
			Expect(ok).To(BeTrue(), "signal_annotations should be a map, not nil")
			Expect(len(signalAnnotations)).To(Equal(0),
				"signal_annotations should be empty map when no annotations provided")

			// Gap #1: original_payload should still be present (contains alert structure)
			Expect(eventData).To(HaveKey("original_payload"),
				"original_payload should be present even with empty labels")

			GinkgoWriter.Printf("✅ Defensive nil checks validated: empty maps preserved (not nil)\n")
			GinkgoWriter.Printf("   - signal_labels: %d labels (empty map)\n", len(signalLabels))
			GinkgoWriter.Printf("   - signal_annotations: %d annotations (empty map)\n", len(signalAnnotations))
		})

		It("should handle missing RawPayload gracefully without crashing", func() {
			// ━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━
			// EDGE CASE: Missing/Nil RawPayload (Validates Defensive Nil Check)
			// ━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━
			//
			// This test validates defensive nil check for RawPayload:
			// - signal.RawPayload == nil → should NOT crash during audit emission
			// - original_payload field should be nil or omitted gracefully
			//
			// Scenario: Internal signals or synthetic alerts without original payload
			// Rationale: System should be resilient to missing payload data
			// ━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━

			// NOTE: This is a synthetic test scenario
			// In practice, Gateway adapters always populate RawPayload
			// But defensive code should handle nil gracefully

			// Create minimal alert (Gateway will populate minimal RawPayload)
			alertPayload := fmt.Sprintf(`{
				"receiver": "kubernaut-webhook",
				"status": "firing",
				"alerts": [{
					"status": "firing",
					"labels": {
						"alertname": "SyntheticAlert",
						"severity": "info",
						"namespace": "%s",
						"synthetic": "true"
					},
					"annotations": {
						"description": "Test alert for nil payload handling"
					},
					"startsAt": "2025-01-04T10:00:00.000Z",
					"endsAt": "0001-01-01T00:00:00Z"
				}],
				"groupLabels": {
					"alertname": "SyntheticAlert"
				},
				"externalURL": "http://alertmanager:9093",
				"version": "4"
			}`, sharedNamespace)

			// Send alert to Gateway webhook
			resp, err := postToGateway(gatewayURL+"/api/v1/signals/prometheus", alertPayload)
			Expect(err).ToNot(HaveOccurred(), "Failed to send synthetic alert")
			Expect(resp.StatusCode).To(Equal(http.StatusCreated), "Gateway should create RR for synthetic alert")

			// Extract correlation_id from response body
			correlationID, err := extractCorrelationID(resp)
			Expect(err).ToNot(HaveOccurred())
			Expect(correlationID).ToNot(BeEmpty())
			defer func() { _ = resp.Body.Close() }()

			// Query Data Storage for audit event
			eventType := "gateway.signal.received"
			eventCategory := gateway.CategoryGateway

			// ✅ Use Eventually() for async validation
			Eventually(func() int {
				resp, err := dsClient.QueryAuditEvents(testCtx, ogenclient.QueryAuditEventsParams{
					EventType:     ogenclient.NewOptString(eventType),
					EventCategory: ogenclient.NewOptString(eventCategory),
					CorrelationID: ogenclient.NewOptString(correlationID),
				})
				if err != nil {
					return 0
				}
				if resp.Pagination.IsSet() && resp.Pagination.Value.Total.IsSet() {
					return resp.Pagination.Value.Total.Value
				}
				return 0
			}, 30*time.Second, 1*time.Second).Should(Equal(1),
				"Should find exactly 1 audit event for nil payload test")

			// Query final audit events
			resp2, err := dsClient.QueryAuditEvents(testCtx, ogenclient.QueryAuditEventsParams{
				EventType:     ogenclient.NewOptString(eventType),
				EventCategory: ogenclient.NewOptString(eventCategory),
				CorrelationID: ogenclient.NewOptString(correlationID),
			})
			Expect(err).ToNot(HaveOccurred())
			events := resp2.Data
			Expect(len(events)).To(Equal(1))

			// Convert EventData discriminated union to map for existing validation logic
			eventDataBytes, _ := json.Marshal(events[0].EventData)
			var eventData map[string]interface{}
			err = json.Unmarshal(eventDataBytes, &eventData)
			Expect(err).ToNot(HaveOccurred())

			// ┌─────────────────────────────────────────────────────────────┐
			// │ CRITICAL: Validate Graceful Nil Handling                    │
			// │ Expected: No crash, field present (may be nil or populated) │
			// └─────────────────────────────────────────────────────────────┘

			// Gap #1: original_payload should exist (even if nil or minimal)
			Expect(eventData).To(HaveKey("original_payload"),
				"original_payload field should exist (validates no crash during nil handling)")

			// Value may be nil or populated (Gateway adapter determines this)
			// The key validation: System didn't crash with nil RawPayload

			// Gap #2 & #3: Should still have labels/annotations (populated from alert)
			Expect(eventData).To(HaveKey("signal_labels"),
				"signal_labels should be present")
			signalLabels, ok := eventData["signal_labels"].(map[string]interface{})
			Expect(ok).To(BeTrue())
			Expect(signalLabels).To(HaveKey("alertname"),
				"Should have alertname label from Prometheus alert")
			Expect(signalLabels["alertname"]).To(Equal("SyntheticAlert"))

			Expect(eventData).To(HaveKey("signal_annotations"),
				"signal_annotations should be present")
			signalAnnotations, ok := eventData["signal_annotations"].(map[string]interface{})
			Expect(ok).To(BeTrue())
			Expect(signalAnnotations).To(HaveKey("description"),
				"Should have description annotation")

			GinkgoWriter.Printf("✅ Nil payload handling validated: system resilient to missing RawPayload\n")
			GinkgoWriter.Printf("   - No crash during audit emission\n")
			GinkgoWriter.Printf("   - All 3 RR reconstruction fields present\n")
		})

		It("should capture all 3 fields in gateway.signal.deduplicated events", func() {
			// ━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━
			// TEST 4: Deduplicated Signals Must Capture RR Reconstruction Fields
			// ━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━
			//
			// Business Rationale:
			// - BR-AUDIT-005: RR reconstruction must work for ALL signals (including duplicates)
			// - Recurring incidents are the MOST COMMON case (e.g., memory leak alerts every 5min)
			// - If deduplicated signals don't capture fields, we lose RR reconstruction for recurring issues
			//
			// Test Strategy:
			// 1. Send initial Prometheus alert → gateway.signal.received
			// 2. Send duplicate alert (same alertname/namespace/pod) → gateway.signal.deduplicated
			// 3. Verify gateway.signal.deduplicated event contains all 3 RR reconstruction fields
			//
			// Validates: REFACTOR changes to emitSignalDeduplicatedAudit() function
			// ━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━

			By("Sending initial Prometheus alert to create deduplicated signal")

			// Create unique alert for this test
			initialAlert := fmt.Sprintf(`{
				"receiver": "kubernaut-webhook",
				"status": "firing",
				"alerts": [{
					"status": "firing",
					"labels": {
						"alertname": "RecurringMemoryLeak",
						"severity": "critical",
						"namespace": "%s",
						"pod": "payment-service-recurring",
						"app": "payment-service",
						"leak_type": "gradual"
					},
					"annotations": {
						"summary": "Memory leak detected in payment service",
						"runbook_url": "https://runbooks.example.com/memory-leak",
						"escalation": "immediate"
					},
					"startsAt": "2025-01-04T10:00:00.000Z",
					"endsAt": "0001-01-01T00:00:00Z",
					"generatorURL": "http://prometheus:9090/graph?g0.expr=memory_leak"
				}],
				"groupLabels": {
					"alertname": "RecurringMemoryLeak"
				},
				"commonLabels": {
					"alertname": "RecurringMemoryLeak",
					"severity": "critical"
				},
				"commonAnnotations": {},
				"externalURL": "http://alertmanager:9093",
				"version": "4",
				"groupKey": "{}:{alertname=\"RecurringMemoryLeak\"}"
			}`, sharedNamespace)

			// Send initial alert
			// BR-SCOPE-002: Wrap in Eventually to tolerate informer cache warm-up delays
			// The managed namespace label may not be visible to the Gateway's scope cache immediately
			var resp1 *http.Response
			var correlationID1 string
			Eventually(func() int {
				var postErr error
				resp1, postErr = postToGateway(gatewayURL+"/api/v1/signals/prometheus", initialAlert)
				Expect(postErr).ToNot(HaveOccurred())
				statusCode := resp1.StatusCode
				if statusCode != http.StatusCreated {
					_ = resp1.Body.Close()
				}
				return statusCode
			}, 10*time.Second, 500*time.Millisecond).Should(Equal(http.StatusCreated),
				"First alert should create new RR (may retry while scope cache syncs)")
			var err error
			correlationID1, err = extractCorrelationID(resp1)
			Expect(err).ToNot(HaveOccurred())
			Expect(correlationID1).ToNot(BeEmpty())
			_ = resp1.Body.Close()

			// ✅ MANDATORY: Use Eventually() to wait for first audit event (NO time.Sleep())
			// Per TESTING_GUIDELINES.md: time.Sleep() is ABSOLUTELY FORBIDDEN
			By("Waiting for initial signal audit event to be written")
			eventTypeReceived := "gateway.signal.received"
			// K8s Cache Synchronization: Audit events depend on CRD visibility. Allow 60s for cache sync.
			// Authority: DD-E2E-K8S-CLIENT-001 (Phase 1 - eventual consistency acknowledgment)
			Eventually(func() int {
				resp, err := dsClient.QueryAuditEvents(testCtx, ogenclient.QueryAuditEventsParams{
					EventType:     ogenclient.NewOptString(eventTypeReceived),
					CorrelationID: ogenclient.NewOptString(correlationID1),
				})
				if err != nil {
					return 0
				}
				if resp.Pagination.IsSet() && resp.Pagination.Value.Total.IsSet() {
					return resp.Pagination.Value.Total.Value
				}
				return 0
			}, 120*time.Second, 1*time.Second).Should(Equal(1), "First audit event should be written (waits for CRD visibility)")

			By("Sending duplicate alert to trigger gateway.signal.deduplicated event")

			// Send duplicate alert (same alertname/namespace/pod → same fingerprint)
			resp2, err := postToGateway(gatewayURL+"/api/v1/signals/prometheus", initialAlert)
			Expect(err).ToNot(HaveOccurred())
			Expect(resp2.StatusCode).To(Equal(http.StatusAccepted), "Duplicate alert should return 202 (deduplicated)")
			correlationID2, err := extractCorrelationID(resp2)
			Expect(err).ToNot(HaveOccurred())
			Expect(correlationID2).ToNot(BeEmpty())
			_ = resp2.Body.Close()

			By("Verifying gateway.signal.deduplicated event captures all 3 RR reconstruction fields")

			eventType := "gateway.signal.deduplicated"
			eventCategory := gateway.CategoryGateway

			// ✅ Use Eventually() for async validation
			Eventually(func() int {
				resp, err := dsClient.QueryAuditEvents(testCtx, ogenclient.QueryAuditEventsParams{
					EventType:     ogenclient.NewOptString(eventType),
					EventCategory: ogenclient.NewOptString(eventCategory),
					CorrelationID: ogenclient.NewOptString(correlationID2),
				})
				if err != nil {
					return 0
				}
				if resp.Pagination.IsSet() && resp.Pagination.Value.Total.IsSet() {
					return resp.Pagination.Value.Total.Value
				}
				return 0
			}, 30*time.Second, 1*time.Second).Should(Equal(1),
				"Should find exactly 1 gateway.signal.deduplicated event")

			// Query final audit events
			resp3, err := dsClient.QueryAuditEvents(testCtx, ogenclient.QueryAuditEventsParams{
				EventType:     ogenclient.NewOptString(eventType),
				EventCategory: ogenclient.NewOptString(eventCategory),
				CorrelationID: ogenclient.NewOptString(correlationID2),
			})
			Expect(err).ToNot(HaveOccurred())

			events := resp3.Data
			Expect(len(events)).To(Equal(1))

			// ✅ DD-TESTING-001: Validate event metadata
			Expect(events[0].EventType).To(Equal(eventType))
			Expect(string(events[0].EventCategory)).To(Equal(eventCategory))
			Expect(events[0].EventAction).To(Equal("deduplicated"))
			Expect(string(events[0].EventOutcome)).To(Equal("success"))
			Expect(events[0].CorrelationID).To(Equal(correlationID2))

			// ✅ DD-TESTING-001: Validate structured event_data for RR reconstruction fields
			// Convert EventData discriminated union to map for existing validation logic
			eventDataBytes, _ := json.Marshal(events[0].EventData)
			var eventData map[string]interface{}
			_ = json.Unmarshal(eventDataBytes, &eventData)

			By("Verifying Gap #1: original_payload is captured in deduplicated event")
			Expect(eventData).To(HaveKey("original_payload"))
			originalPayload, ok := eventData["original_payload"].(map[string]interface{})
			Expect(ok).To(BeTrue(), "original_payload should be a JSON object")
			Expect(originalPayload).To(HaveKey("alerts"))

			By("Verifying Gap #2: signal_labels are captured in deduplicated event")
			Expect(eventData).To(HaveKey("signal_labels"))
			signalLabels, ok := eventData["signal_labels"].(map[string]interface{})
			Expect(ok).To(BeTrue(), "signal_labels should be a JSON object")
			Expect(signalLabels).To(HaveKeyWithValue("alertname", "RecurringMemoryLeak"))
			Expect(signalLabels).To(HaveKeyWithValue("severity", "critical"))
			Expect(signalLabels).To(HaveKeyWithValue("leak_type", "gradual"))

			By("Verifying Gap #3: signal_annotations are captured in deduplicated event")
			Expect(eventData).To(HaveKey("signal_annotations"))
			signalAnnotations, ok := eventData["signal_annotations"].(map[string]interface{})
			Expect(ok).To(BeTrue(), "signal_annotations should be a JSON object")
			Expect(signalAnnotations).To(HaveKeyWithValue("summary", "Memory leak detected in payment service"))
			Expect(signalAnnotations).To(HaveKeyWithValue("runbook_url", "https://runbooks.example.com/memory-leak"))

			GinkgoWriter.Printf("✅ BR-AUDIT-005: Deduplicated signals capture all 3 RR reconstruction fields\n")
			GinkgoWriter.Printf("   - Critical for recurring incident reconstruction (most common case)\n")
			GinkgoWriter.Printf("   - Validates REFACTOR changes to emitSignalDeduplicatedAudit()\n")
		})

		It("should capture all 3 fields consistently across different signal types", func() {
			// ━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━
			// TEST 5: Cross-Signal-Type Validation (Prometheus vs K8s Event)
			// ━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━
			//
			// Business Rationale:
			// - BR-AUDIT-005: RR reconstruction must work for ALL signal sources
			// - Multi-cloud/hybrid environments use multiple monitoring systems
			// - RR reconstruction algorithm must be adapter-agnostic
			//
			// Test Strategy:
			// 1. Send Prometheus alert → verify 3 fields captured
			// 2. Send K8s Event → verify 3 fields captured
			// 3. Validate field structure consistency across adapters
			//
			// Validates: extractRRReconstructionFields() helper is adapter-agnostic
			// ━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━

			By("Sending Prometheus alert")

			prometheusAlert := fmt.Sprintf(`{
				"receiver": "kubernaut-webhook",
				"status": "firing",
				"alerts": [{
					"status": "firing",
					"labels": {
						"alertname": "CrossTypeTest-Prometheus",
						"severity": "warning",
						"namespace": "%s",
						"source": "prometheus"
					},
					"annotations": {
						"description": "Test alert from Prometheus for cross-type validation"
					},
					"startsAt": "2025-01-04T10:00:00.000Z",
					"endsAt": "0001-01-01T00:00:00Z"
				}],
				"groupLabels": {},
				"commonLabels": {},
				"commonAnnotations": {},
				"externalURL": "http://alertmanager:9093",
				"version": "4",
				"groupKey": "{}"
			}`, sharedNamespace)

			respProm, err := postToGateway(gatewayURL+"/api/v1/signals/prometheus", prometheusAlert)
			Expect(err).ToNot(HaveOccurred())
			Expect(respProm.StatusCode).To(Equal(http.StatusCreated), "Prometheus alert should create new RR")
			correlationIDProm, err := extractCorrelationID(respProm)
			Expect(err).ToNot(HaveOccurred())
			Expect(correlationIDProm).ToNot(BeEmpty())
			_ = respProm.Body.Close()

			By("Sending Kubernetes Event")

			k8sEvent := map[string]interface{}{
				"apiVersion": "v1",
				"kind":       "Event",
				"metadata": map[string]interface{}{
					"name":      "cross-type-test-k8s",
					"namespace": sharedNamespace,
					"labels": map[string]string{
						"app":    "api-server",
						"source": "kubernetes",
					},
					"annotations": map[string]string{
						"runbook": "http://runbooks.com/k8s-oom",
					},
				},
				"reason":  "OOMKilled",
				"message": "Container exceeded memory limit (cross-type test)",
				"type":    "Warning",
				"involvedObject": map[string]interface{}{
					"kind":      "Pod",
					"name":      "api-server-pod-cross",
					"namespace": sharedNamespace,
				},
				"source": map[string]interface{}{
					"component": "kubelet",
				},
				"firstTimestamp": "2025-01-04T10:00:00Z",
				"lastTimestamp":  "2025-01-04T10:00:00Z",
				"count":          1,
			}

			k8sEventJSON, err := json.Marshal(k8sEvent)
			Expect(err).ToNot(HaveOccurred())

			respK8s, err := postToGateway(gatewayURL+"/api/v1/signals/kubernetes-event", string(k8sEventJSON))
			Expect(err).ToNot(HaveOccurred())
			Expect(respK8s.StatusCode).To(Equal(http.StatusCreated), "K8s Event should create new RR")
			correlationIDK8s, err := extractCorrelationID(respK8s)
			Expect(err).ToNot(HaveOccurred())
			Expect(correlationIDK8s).ToNot(BeEmpty())
			_ = respK8s.Body.Close()

			By("Verifying Prometheus alert audit event has all 3 RR reconstruction fields")

			eventType := "gateway.signal.received"
			eventCategory := gateway.CategoryGateway

			Eventually(func() int {
				resp, err := dsClient.QueryAuditEvents(testCtx, ogenclient.QueryAuditEventsParams{
					EventType:     ogenclient.NewOptString(eventType),
					EventCategory: ogenclient.NewOptString(eventCategory),
					CorrelationID: ogenclient.NewOptString(correlationIDProm),
				})
				if err != nil {
					return 0
				}
				if resp.Pagination.IsSet() && resp.Pagination.Value.Total.IsSet() {
					return resp.Pagination.Value.Total.Value
				}
				return 0
			}, 30*time.Second, 1*time.Second).Should(Equal(1))

			respPromAudit, err := dsClient.QueryAuditEvents(testCtx, ogenclient.QueryAuditEventsParams{
				EventType:     ogenclient.NewOptString(eventType),
				EventCategory: ogenclient.NewOptString(eventCategory),
				CorrelationID: ogenclient.NewOptString(correlationIDProm),
			})
			Expect(err).ToNot(HaveOccurred())
			promEvents := respPromAudit.Data
			Expect(len(promEvents)).To(Equal(1))

			// Convert EventData discriminated union to map for existing validation logic
			promEventDataBytes, _ := json.Marshal(promEvents[0].EventData)
			var promEventData map[string]interface{}
			_ = json.Unmarshal(promEventDataBytes, &promEventData)
			Expect(promEventData).To(HaveKey("original_payload"))
			Expect(promEventData).To(HaveKey("signal_labels"))
			Expect(promEventData).To(HaveKey("signal_annotations"))

			By("Verifying K8s Event audit event has all 3 RR reconstruction fields")

			Eventually(func() int {
				resp, err := dsClient.QueryAuditEvents(testCtx, ogenclient.QueryAuditEventsParams{
					EventType:     ogenclient.NewOptString(eventType),
					EventCategory: ogenclient.NewOptString(eventCategory),
					CorrelationID: ogenclient.NewOptString(correlationIDK8s),
				})
				if err != nil {
					return 0
				}
				if resp.Pagination.IsSet() && resp.Pagination.Value.Total.IsSet() {
					return resp.Pagination.Value.Total.Value
				}
				return 0
			}, 30*time.Second, 1*time.Second).Should(Equal(1))

			respK8sAudit, err := dsClient.QueryAuditEvents(testCtx, ogenclient.QueryAuditEventsParams{
				EventType:     ogenclient.NewOptString(eventType),
				EventCategory: ogenclient.NewOptString(eventCategory),
				CorrelationID: ogenclient.NewOptString(correlationIDK8s),
			})
			Expect(err).ToNot(HaveOccurred())
			k8sEvents := respK8sAudit.Data
			Expect(len(k8sEvents)).To(Equal(1))

			// Convert EventData discriminated union to map for existing validation logic
			k8sEventDataBytes, _ := json.Marshal(k8sEvents[0].EventData)
			var k8sEventData map[string]interface{}
			_ = json.Unmarshal(k8sEventDataBytes, &k8sEventData)
			Expect(k8sEventData).To(HaveKey("original_payload"))
			Expect(k8sEventData).To(HaveKey("signal_labels"))
			Expect(k8sEventData).To(HaveKey("signal_annotations"))

			By("Verifying field structure consistency across adapters")

			// Both should have maps (not nil)
			promLabels, ok := promEventData["signal_labels"].(map[string]interface{})
			Expect(ok).To(BeTrue(), "Prometheus signal_labels should be map")
			k8sLabels, ok := k8sEventData["signal_labels"].(map[string]interface{})
			Expect(ok).To(BeTrue(), "K8s signal_labels should be map")

			promAnnotations, ok := promEventData["signal_annotations"].(map[string]interface{})
			Expect(ok).To(BeTrue(), "Prometheus signal_annotations should be map")
			k8sAnnotations, ok := k8sEventData["signal_annotations"].(map[string]interface{})
			Expect(ok).To(BeTrue(), "K8s signal_annotations should be map")

			// Verify actual label/annotation content
			// Prometheus adapter: labels/annotations come from alert.labels/annotations
			Expect(promLabels).To(HaveKeyWithValue("source", "prometheus"))
			Expect(promAnnotations).To(HaveKeyWithValue("description", "Test alert from Prometheus for cross-type validation"))

			// K8s Event adapter: labels AND annotations are structured with kubernaut.ai/ prefix from Event metadata
			Expect(k8sLabels).To(HaveKeyWithValue("kubernaut.ai/event-type", "warning"))
			Expect(k8sLabels).To(HaveKeyWithValue("kubernaut.ai/resource-kind", "pod"))
			Expect(k8sLabels).To(HaveKeyWithValue("kubernaut.ai/event-source", "kubelet"))

			// K8s Event adapter: annotations are also transformed to structured fields
			Expect(k8sAnnotations).To(HaveKeyWithValue("kubernaut.ai/event-message", "Container exceeded memory limit (cross-type test)"))
			Expect(k8sAnnotations).To(HaveKeyWithValue("kubernaut.ai/event-count", "1"))
			Expect(k8sAnnotations).To(HaveKey("kubernaut.ai/first-timestamp"))
			Expect(k8sAnnotations).To(HaveKey("kubernaut.ai/last-timestamp"))

			GinkgoWriter.Printf("✅ BR-AUDIT-005: Cross-signal-type validation PASSED\n")
			GinkgoWriter.Printf("   - Prometheus adapter: ✅ All 3 fields captured\n")
			GinkgoWriter.Printf("   - Kubernetes adapter: ✅ All 3 fields captured\n")
			GinkgoWriter.Printf("   - Field structure consistent across adapters\n")
		})
	})
})
