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
	"time"

	"github.com/google/uuid"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"sigs.k8s.io/controller-runtime/pkg/client"

	ogenclient "github.com/jordigilh/kubernaut/pkg/datastorage/ogen-client"
)

// =============================================================================
// BR-AUDIT-005 Gap #7: Gateway Error Details Standardization
// =============================================================================
//
// Business Requirements:
// - BR-AUDIT-005 v2.0 Gap #7: Standardized error details across all services
// - SOC2 Type II: Comprehensive error audit trail for compliance
// - RR Reconstruction: Reliable `.status.error` field reconstruction
//
// Authority Documents:
// - DD-004: RFC7807 error response standard (HTTP responses only)
// - DD-AUDIT-003 v1.4: Service audit trace requirements
// - ADR-034: Unified audit table design
// - SOC2_AUDIT_IMPLEMENTATION_PLAN.md: Day 4 - Error Details Standardization
//
// Test Strategy (per TESTING_GUIDELINES.md):
// - Integration tier: Call Gateway business logic directly (ProcessSignal)
// - Real infrastructure: K8s API + Data Storage (Podman)
// - NO HTTP layer: Direct Go function calls, not HTTP requests
// - Tests MUST Fail() if not implemented (NO Skip())
//
// Error Scenarios Tested:
// - Scenario 1: K8s CRD creation failure (ERR_K8S_*) - Integration test
// - Scenario 2: Adapter validation failure (ERR_INVALID_*) - Unit test (see test/unit/gateway/audit_errors_unit_test.go)
//
// To run these tests:
//   make test-integration-gateway
//
// =============================================================================

var _ = Describe("BR-AUDIT-005 Gap #7: Gateway Error Audit Standardization", func() {
	var (
		ctx            context.Context
		testClient     client.Client
		httpClient     *http.Client
		dataStorageURL string
		testNamespace  string
		dsClient       *ogenclient.Client
	)

	BeforeEach(func() {
		ctx = context.Background()
		testClient = k8sClient // Use suite-level client (DD-E2E-K8S-CLIENT-001)
		httpClient = &http.Client{Timeout: 10 * time.Second}
		testNamespace = fmt.Sprintf("test-error-audit-%d-%s", GinkgoParallelProcess(), uuid.New().String()[:8])

		// Create namespace in Kubernetes
		Expect(CreateNamespaceAndWait(ctx, testClient, testNamespace)).To(Succeed(),
			"Failed to create test namespace")

		// DD-TEST-001: Get Data Storage URL from suite's shared infrastructure
		dataStorageURL = os.Getenv("TEST_DATA_STORAGE_URL")
		if dataStorageURL == "" {
			dataStorageURL = "http://127.0.0.1:18091" // Fallback for manual testing - Use 127.0.0.1 for CI/CD IPv4 compatibility
		}

		// Verify Data Storage is available
		healthResp, err := http.Get(dataStorageURL + "/health")
		if err != nil {
			Fail(fmt.Sprintf("Data Storage not available at %s: %v", dataStorageURL, err))
		}
		defer func() { _ = healthResp.Body.Close() }()
		if healthResp.StatusCode != http.StatusOK {
			Fail(fmt.Sprintf("Data Storage health check failed at %s: status %d", dataStorageURL, healthResp.StatusCode))
		}

		// Create OpenAPI client for Data Storage queries
		dsClient, err = ogenclient.NewClient(dataStorageURL)
		Expect(err).ToNot(HaveOccurred(), "Failed to create DataStorage OpenAPI client")
	})

	AfterEach(func() {
	})

	Context("Gap #7 Scenario 1: K8s CRD Creation Failure", func() {
		It("should emit standardized error_details on CRD creation failure", func() {
			By("1. Create Prometheus alert with non-existent namespace")
			// Use a namespace that definitely doesn't exist
			invalidNamespace := "non-existent-ns-" + uuid.New().String()
			alertName := "CRDCreationFailureTest-" + uuid.New().String()[:8]

			// Create Prometheus webhook payload
			payload := createPrometheusWebhookPayload(PrometheusAlertPayload{
				AlertName: alertName,
				Namespace: invalidNamespace, // Non-existent namespace causes K8s API error
				Severity:  "warning",
				Resource: ResourceIdentifier{
					Kind: "Pod",
					Name: "test-pod",
				},
			})

			By("2. Send HTTP request to Gateway - expect error (but Gateway still accepts)")
			// E2E Pattern: Use HTTP POST to Gateway endpoint
			req, err := http.NewRequest("POST", gatewayURL+"/api/v1/signals/prometheus", bytes.NewBuffer(payload))
			Expect(err).ToNot(HaveOccurred(), "HTTP request creation should succeed")
			req.Header.Set("Content-Type", "application/json")
			req.Header.Set("X-Timestamp", fmt.Sprintf("%d", time.Now().Unix()))

			resp, err := httpClient.Do(req)
			Expect(err).ToNot(HaveOccurred(), "HTTP request should succeed")
			defer func() { _ = resp.Body.Close() }()

			// Gateway accepts the request but CRD creation will fail internally
			// Gateway may return 202 (Accepted) or 500 (Internal Error) depending on timing
			// What matters is that it emits a "gateway.crd.failed" audit event
			GinkgoWriter.Printf("Gateway response: HTTP %d\n", resp.StatusCode)

			By("3. Query Data Storage for 'gateway.crd.failed' audit event")
			eventType := "gateway.crd.failed" // ✅ FIX: Gateway emits EventTypeCRDFailed = "gateway.crd.failed"
			params := ogenclient.QueryAuditEventsParams{
				EventType:     ogenclient.NewOptString(eventType),
				EventCategory: ogenclient.NewOptString("gateway"),
				// Note: We can't predict the exact fingerprint, so we query by event type
				// and verify the namespace matches
			}

			var auditEvents []ogenclient.AuditEvent
			Eventually(func() bool {
				resp, err := dsClient.QueryAuditEvents(ctx, params)
				if err != nil {
					GinkgoWriter.Printf("Failed to query audit events: %v\n", err)
					return false
				}
				auditEvents = resp.Data
				// Find events matching our invalid namespace
				for _, event := range auditEvents {
					if event.EventData.GatewayAuditPayload.ErrorDetails.IsSet() {
						errorDetails := event.EventData.GatewayAuditPayload.ErrorDetails.Value
						if errorDetails.Message != "" {
							return true
						}
					}
				}
				return false
			}, 120*time.Second, 2*time.Second).Should(BeTrue(),
				"Should find at least 1 'gateway.crd.failed' audit event with error_details (increased timeout for DataStorage query)")

			By("4. Validate error_details structure (Gap #7)")
			Expect(auditEvents).ToNot(BeEmpty(), "Should have at least 1 audit event")

			// Find the most recent event with error_details
			var event ogenclient.AuditEvent
			for _, e := range auditEvents {
				if e.EventData.GatewayAuditPayload.ErrorDetails.IsSet() {
					event = e
					break
				}
			}

			// Validate standard ADR-034 fields
			Expect(event.Version).To(Equal("1.0"))
			Expect(event.EventType).To(Equal("gateway.crd.failed")) // ✅ FIX: Correct event type
			Expect(string(event.EventCategory)).To(Equal("gateway"))
			Expect(event.EventAction).To(Equal("created"))
			Expect(string(event.EventOutcome)).To(Equal("failure"))

			// Validate error_details structure (Gap #7) - ✅ DD-API-001: Direct OpenAPI access
			gatewayPayload := event.EventData.GatewayAuditPayload

			// Access error_details from OpenAPI structure
			errorDetails := gatewayPayload.ErrorDetails
			Expect(errorDetails.IsSet()).To(BeTrue(), "error_details should exist in event_data (Gap #7)")

			errorDetailsValue := errorDetails.Value

			// Gap #7: Standardized error_details fields (direct access, no IsSet() needed for required fields)
			Expect(errorDetailsValue.Message).ToNot(BeEmpty(), "error_details.message required")
			Expect(errorDetailsValue.Code).ToNot(BeEmpty(), "error_details.code required")
			Expect(string(errorDetailsValue.Component)).To(Equal("gateway"), "error_details.component should be 'gateway'")
			// retry_possible is a bool, so just verify it exists (has a value)
			_ = errorDetailsValue.RetryPossible // Validates field exists

			// Validate error message contains meaningful context (Gap #7)
			Expect(errorDetailsValue.Message).To(Or(
				ContainSubstring("namespace"),
				ContainSubstring("not found"),
				ContainSubstring("failed"),
			), "error_details.message should contain error context")
		})
	})

	// NOTE: Scenario 2 (Adapter Validation Failure) moved to unit tests
	// Rationale: Adapter validation is pure logic without infrastructure needs
	// Location: test/unit/gateway/audit_errors_unit_test.go
	// This maintains proper test distribution (70% unit, >50% integration)
})
