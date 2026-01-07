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

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/google/uuid"
	ptr "k8s.io/utils/ptr"

	gateway "github.com/jordigilh/kubernaut/pkg/gateway"
	"github.com/jordigilh/kubernaut/pkg/gateway/types"
	dsgen "github.com/jordigilh/kubernaut/pkg/datastorage/client"
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
		dsClient       *dsgen.ClientWithResponses
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
		dsClient, err = dsgen.NewClientWithResponses(dataStorageURL)
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
				SourceType: "prometheus-alert",
			}

			By("2. Call Gateway business logic - expect failure")
			_, err := gatewayServer.ProcessSignal(ctx, signal)
			Expect(err).To(HaveOccurred(), "ProcessSignal should fail due to invalid namespace")
			Expect(err.Error()).To(ContainSubstring("namespace"), "Error should mention namespace issue")

			By("3. Query Data Storage for 'gateway.crd.creation_failed' audit event")
			eventType := "gateway.crd.creation_failed"
			params := &dsgen.QueryAuditEventsParams{
				CorrelationId: &fingerprint,
				EventType:     &eventType,
				EventCategory: ptr.To("gateway"),
			}

			var auditEvents []dsgen.AuditEvent
			Eventually(func() int {
				resp, err := dsClient.QueryAuditEventsWithResponse(ctx, params)
				if err != nil {
					GinkgoWriter.Printf("Failed to query audit events: %v\n", err)
					return 0
				}
				if resp.JSON200 == nil {
					return 0
				}
				if resp.JSON200.Data != nil {
					auditEvents = *resp.JSON200.Data
				}
				if resp.JSON200.Pagination != nil && resp.JSON200.Pagination.Total != nil {
					return *resp.JSON200.Pagination.Total
				}
				return 0
			}, 10*time.Second, 500*time.Millisecond).Should(Equal(1),
				"Should find exactly 1 'crd.creation_failed' audit event")

			By("4. Validate error_details structure (Gap #7)")
			Expect(auditEvents).To(HaveLen(1), "Should have exactly 1 audit event")

			// Convert to map for easier validation
			eventJSON, err := json.Marshal(auditEvents[0])
			Expect(err).ToNot(HaveOccurred())
			var eventMap map[string]interface{}
			err = json.Unmarshal(eventJSON, &eventMap)
			Expect(err).ToNot(HaveOccurred())

			// Validate standard ADR-034 fields
			Expect(eventMap["version"]).To(Equal("1.0"))
			Expect(eventMap["event_type"]).To(Equal("gateway.crd.creation_failed"))
			Expect(eventMap["event_category"]).To(Equal("gateway"))
			Expect(eventMap["event_action"]).To(Equal("created"))
			Expect(eventMap["event_outcome"]).To(Equal("failure"))
			Expect(eventMap["correlation_id"]).To(Equal(fingerprint))

			// Validate error_details structure (Gap #7)
			eventData, ok := eventMap["event_data"].(map[string]interface{})
			Expect(ok).To(BeTrue(), "event_data should be a map")

			errorDetails, ok := eventData["error_details"].(map[string]interface{})
			Expect(ok).To(BeTrue(), "error_details should exist in event_data (Gap #7)")

			// Gap #7: Standardized error_details fields
			Expect(errorDetails).To(HaveKey("message"), "error_details.message required")
			Expect(errorDetails).To(HaveKey("code"), "error_details.code required")
			Expect(errorDetails).To(HaveKey("component"), "error_details.component required")
			Expect(errorDetails["component"]).To(Equal("gateway"), "error_details.component should be 'gateway'")
			Expect(errorDetails).To(HaveKey("retry_possible"), "error_details.retry_possible required")

			// Validate error message contains meaningful context
			message := errorDetails["message"].(string)
			Expect(message).ToNot(BeEmpty(), "error_details.message should not be empty")
			Expect(message).To(Or(
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

