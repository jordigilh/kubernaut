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
	"strings"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

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
// - Integration tier: Requires real Data Storage + Gateway for error scenarios
// - OpenAPI client MANDATORY for all audit queries (DD-API-001)
// - Eventually() MANDATORY for async operations (NO time.Sleep())
// - Tests MUST Fail() if infrastructure unavailable (NO Skip())
//
// Error Scenarios Tested:
// - Scenario 1: K8s CRD creation failure (ERR_K8S_*)
// - Scenario 2: Invalid signal format (ERR_INVALID_*)
//
// To run these tests:
//   make test-integration-gateway
//
// =============================================================================

var _ = Describe("BR-AUDIT-005 Gap #7: Gateway Error Audit Standardization", func() {
	var (
		dsClient     *dsgen.ClientWithResponses
		ctx          context.Context
		gatewayURL   string
		dataStorageURL string
	)

	BeforeEach(func() {
		ctx = context.Background()
		gatewayURL = os.Getenv("GATEWAY_URL")
		dataStorageURL = os.Getenv("DATA_STORAGE_URL")

		if gatewayURL == "" {
			Fail("GATEWAY_URL environment variable not set")
		}
		if dataStorageURL == "" {
			Fail("DATA_STORAGE_URL environment variable not set")
		}

		// DD-API-001: Use OpenAPI client for Data Storage
		var err error
		dsClient, err = dsgen.NewClientWithResponses(dataStorageURL)
		Expect(err).ToNot(HaveOccurred())

		// REQUIRED: Fail if Data Storage unavailable
		resp, err := http.Get(dataStorageURL + "/health")
		if err != nil || resp.StatusCode != http.StatusOK {
			Fail(fmt.Sprintf("Data Storage not available at %s - cannot run audit tests", dataStorageURL))
		}
	})

	Context("Gap #7 Scenario 1: K8s CRD Creation Failure", func() {
		It("should emit standardized error_details on CRD creation failure", func() {
			// Given: Simulated K8s API failure (Gateway will fail to create RR CRD)
			// Note: This test requires Gateway to be configured to fail CRD creation
			// TODO: Determine how to trigger K8s CRD creation failure in test environment

			Skip("Implementation pending: Need mechanism to trigger K8s CRD creation failure")

			// When: Gateway receives signal but fails to create RR CRD
			// Then: Should emit gateway.crd.creation_failed with error_details

			// Verify error_details structure (WILL FAIL - not standardized yet)
			// Expect(eventData).To(HaveKey("error_details"))
			// errorDetails := eventData["error_details"].(map[string]interface{})
			// Expect(errorDetails).To(HaveKey("message"))
			// Expect(errorDetails).To(HaveKey("code"))
			// Expect(errorDetails).To(HaveKey("component"))
			// Expect(errorDetails["component"]).To(Equal("gateway"))
			// Expect(errorDetails).To(HaveKey("retry_possible"))
		})
	})

	Context("Gap #7 Scenario 2: Invalid Signal Format", func() {
		It("should emit standardized error_details on invalid signal format", func() {
			// Given: Invalid JSON signal (malformed, missing required fields)
			invalidPayload := `{
				"invalid": "structure",
				"missing": "required_fields"
			}`

			// When: Gateway processes invalid signal (business operation)
			resp, err := postToGateway(gatewayURL+"/webhook/kubernetes-events", invalidPayload)
			Expect(err).ToNot(HaveOccurred())

			// Gateway should reject with 400 Bad Request (RFC7807 response)
			// But should also emit audit event for invalid signal
			Expect(resp.StatusCode).To(Equal(http.StatusBadRequest))

			// Extract correlation_id if available, otherwise use fingerprint from error response
			// For invalid signals, Gateway may not create RR, so correlation_id may not exist
			// This test may need to be adjusted based on Gateway's error handling

			Skip("Implementation pending: Determine how Gateway emits audit for invalid signals")

			// Then: Should have error event with standardized error_details
			// eventType := "gateway.signal.failed" // or similar
			// Eventually(func() int {
			// 	resp, _ := dsClient.QueryAuditEventsWithResponse(ctx, &dsgen.QueryAuditEventsParams{
			// 		EventType: &eventType,
			// 		// CorrelationId: &correlationID, // May not exist for invalid signals
			// 	})
			// 	if resp.JSON200 == nil {
			// 		return 0
			// 	}
			// 	return *resp.JSON200.Pagination.Total
			// }, 30*time.Second, 1*time.Second).Should(Equal(1),
			// 	"Should find exactly 1 error event for invalid signal")

			// Validate Gap #7: error_details (WILL FAIL - not standardized yet)
			// errorDetails := eventData["error_details"].(map[string]interface{})
			// Expect(errorDetails).To(HaveKey("message"))
			// Expect(errorDetails["message"]).To(ContainSubstring("invalid"))
			// Expect(errorDetails).To(HaveKey("code"))
			// Expect(errorDetails["code"]).To(Equal("ERR_INVALID_PAYLOAD"))
			// Expect(errorDetails).To(HaveKey("component"))
			// Expect(errorDetails["component"]).To(Equal("gateway"))
			// Expect(errorDetails).To(HaveKey("retry_possible"))
			// Expect(errorDetails["retry_possible"]).To(BeFalse()) // Invalid payload not retryable
		})
	})
})

