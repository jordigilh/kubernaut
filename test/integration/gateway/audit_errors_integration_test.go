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
	"fmt"
	"net/http"
	"os"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/google/uuid"

	gateway "github.com/jordigilh/kubernaut/pkg/gateway"
	"github.com/jordigilh/kubernaut/pkg/gateway/types"
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
		testClient     *K8sTestClient
		gatewayServer  *gateway.Server
		dataStorageURL string
		testNamespace  string
		dsClient       *ogenclient.Client
	)

	BeforeEach(func() {
		ctx = context.Background()
		testClient = SetupK8sTestClient(ctx)

		// DD-TEST-001: Get Data Storage URL from suite's shared infrastructure
		dataStorageURL = os.Getenv("TEST_DATA_STORAGE_URL")
		if dataStorageURL == "" {
			dataStorageURL = "http://localhost:18090" // Fallback for manual testing
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

		// Setup isolated test namespace
		testNamespace = fmt.Sprintf("test-error-audit-%d-%s", GinkgoParallelProcess(), uuid.New().String()[:8])
		EnsureTestNamespace(ctx, testClient, testNamespace)
		RegisterTestNamespace(testNamespace)

		// Create Gateway server (business logic only, no HTTP server)
		gatewayServer, err = StartTestGateway(ctx, testClient, dataStorageURL)
		Expect(err).ToNot(HaveOccurred())
	})

	AfterEach(func() {
		if gatewayServer != nil {
			_ = gatewayServer.Stop(ctx)
		}
	})

	Context("Gap #7 Scenario 1: K8s CRD Creation Failure", func() {
		It("should emit standardized error_details on CRD creation failure", func() {
			By("1. Create signal with non-existent namespace")
			// Use a namespace that definitely doesn't exist
			invalidNamespace := "non-existent-ns-" + uuid.New().String()
			fingerprint := "test-fp-" + uuid.New().String()[:16]

			signal := &types.NormalizedSignal{
				Fingerprint: fingerprint,
				AlertName:   "CRDCreationFailureTest",
				Namespace:   invalidNamespace, // Non-existent namespace causes K8s API error
				Severity:    "warning",
				Resource: types.ResourceIdentifier{
					Kind: "Pod",
					Name: "test-pod",
			},
			SourceType: "alertmanager", // ✅ ADAPTER-CONSTANT: PrometheusAdapter uses SourceTypeAlertManager
		}

			By("2. Call Gateway business logic - expect failure")
			_, err := gatewayServer.ProcessSignal(ctx, signal)
			Expect(err).To(HaveOccurred(), "ProcessSignal should fail due to invalid namespace")
			Expect(err.Error()).To(ContainSubstring("namespace"), "Error should mention namespace issue")

		By("3. Query Data Storage for 'gateway.crd.failed' audit event")
		eventType := "gateway.crd.failed" // ✅ FIX: Gateway emits EventTypeCRDFailed = "gateway.crd.failed"
		params := ogenclient.QueryAuditEventsParams{
			CorrelationID: ogenclient.NewOptString(fingerprint),
			EventType:     ogenclient.NewOptString(eventType),
			EventCategory: ogenclient.NewOptString("gateway"),
		}

		var auditEvents []ogenclient.AuditEvent
		Eventually(func() int {
			resp, err := dsClient.QueryAuditEvents(ctx, params)
			if err != nil {
				GinkgoWriter.Printf("Failed to query audit events: %v\n", err)
				return 0
			}
			auditEvents = resp.Data
			if resp.Pagination.IsSet() && resp.Pagination.Value.Total.IsSet() {
				return resp.Pagination.Value.Total.Value
			}
			return 0
			}, 10*time.Second, 500*time.Millisecond).Should(Equal(1),
				"Should find exactly 1 'gateway.crd.failed' audit event")

		By("4. Validate error_details structure (Gap #7)")
		Expect(auditEvents).To(HaveLen(1), "Should have exactly 1 audit event")

		// ✅ DD-API-001: Use OpenAPI types directly (no JSON conversion)
		event := auditEvents[0]

		// Validate standard ADR-034 fields
		Expect(event.Version).To(Equal("1.0"))
		Expect(event.EventType).To(Equal("gateway.crd.failed")) // ✅ FIX: Correct event type
		Expect(string(event.EventCategory)).To(Equal("gateway"))
		Expect(event.EventAction).To(Equal("created"))
		Expect(string(event.EventOutcome)).To(Equal("failure"))
		Expect(event.CorrelationID).To(Equal(fingerprint))

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

