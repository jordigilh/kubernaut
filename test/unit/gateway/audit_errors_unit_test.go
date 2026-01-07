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

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/jordigilh/kubernaut/pkg/gateway/adapters"
	"github.com/jordigilh/kubernaut/pkg/gateway/types"
)

// =============================================================================
// BR-AUDIT-005 Gap #7: Gateway Adapter Validation Error Audit - Unit Tests
// =============================================================================
//
// Business Requirements:
// - BR-AUDIT-005 v2.0 Gap #7: Standardized error details for validation failures
// - SOC2 Type II: Audit trail for all signal rejection reasons
//
// Test Strategy:
// - Unit tier: Pure validation logic without infrastructure
// - Mock audit store to verify emission
// - Fast execution (<100ms per test)
// - Tests MUST Fail() if not implemented (NO Skip())
//
// Why Unit Tests (Not Integration):
// - Adapter validation is pure logic (no K8s, no DataStorage needed)
// - Fast feedback loop for developers
// - Proper test distribution: 70% unit, >50% integration
//
// Validation Scenarios Tested:
// - Empty fingerprint (required field)
// - Empty alert name (required field)
// - Invalid severity (enum validation)
// - Missing namespace (required field)
//
// To run these tests:
//   go test ./test/unit/gateway/... -ginkgo.focus="Gap #7"
//
// =============================================================================

var _ = Describe("BR-AUDIT-005 Gap #7: Adapter Validation Error Audit", func() {
	var (
		ctx              context.Context
		prometheusAdapter adapters.SignalAdapter
	)

	BeforeEach(func() {
		ctx = context.Background()
		prometheusAdapter = adapters.NewPrometheusAdapter()
	})

	Context("Gap #7 Scenario 2: Adapter Validation Failure", func() {
		It("should detect validation failure for empty fingerprint", func() {
			Fail("IMPLEMENTATION REQUIRED: Adapter validation error audit emission\n" +
				"  Business Flow:\n" +
				"    1. Adapter validates signal (Adapter.Validate())\n" +
				"    2. Validation fails (empty fingerprint)\n" +
				"    3. Server emits 'gateway.signal.validation_failed' audit event\n" +
				"    4. Event includes standardized error_details (Gap #7)\n" +
				"  Implementation Steps:\n" +
				"    1. Create emitSignalValidationFailedAudit() in pkg/gateway/server.go\n" +
				"    2. Call from readParseValidateSignal() on validation error\n" +
				"    3. Use sharedaudit.NewErrorDetailsFromValidationError()\n" +
				"  Test Implementation:\n" +
				"    1. Create signal with empty fingerprint\n" +
				"    2. Call adapter.Validate(signal)\n" +
				"    3. Verify validation error occurs\n" +
				"    4. Mock audit store verifies emission\n" +
				"    5. Validate error_details structure (message, code, component, retry_possible)\n" +
				"  Tracking: BR-AUDIT-005 Gap #7")

			// TDD RED: Define expected behavior
			invalidSignal := &types.NormalizedSignal{
				Fingerprint:  "", // INVALID: empty fingerprint
				AlertName:    "TestAlert",
				Namespace:    "default",
				Severity:     "warning",
				SourceType:   "prometheus-alert",
				Resource: types.ResourceIdentifier{
					Kind: "Pod",
					Name: "test-pod",
				},
			}

			// Adapter validation should fail
			err := prometheusAdapter.Validate(invalidSignal)
			Expect(err).To(HaveOccurred(), "Validation should fail for empty fingerprint")
			Expect(err.Error()).To(ContainSubstring("fingerprint"), "Error should mention fingerprint")

			// TODO: After implementation
			// 1. Create mock audit store
			// 2. Create server with mock audit store
			// 3. Call server.emitSignalValidationFailedAudit(ctx, invalidSignal, "prometheus", err)
			// 4. Verify mock audit store received event with:
			//    - event_type: "gateway.signal.validation_failed"
			//    - event_outcome: "failure"
			//    - error_details.message: contains "fingerprint"
			//    - error_details.code: validation error code
			//    - error_details.component: "gateway"
			//    - error_details.retry_possible: false (validation errors not retryable)
		})

		It("should detect validation failure for empty alert name", func() {
			invalidSignal := &types.NormalizedSignal{
				Fingerprint:  "a1b2c3d4e5f6789012345678901234567890abcdef1234567890abcdef123456",
				AlertName:    "", // INVALID: empty alert name
				Namespace:    "default",
				Severity:     "warning",
				SourceType:   "prometheus-alert",
				Resource: types.ResourceIdentifier{
					Kind: "Pod",
					Name: "test-pod",
				},
			}

			err := prometheusAdapter.Validate(invalidSignal)
			Expect(err).To(HaveOccurred(), "Validation should fail for empty alert name")
			Expect(err.Error()).To(ContainSubstring("alertName"), "Error should mention alertName")
		})

		It("should detect validation failure for invalid severity", func() {
			invalidSignal := &types.NormalizedSignal{
				Fingerprint:  "a1b2c3d4e5f6789012345678901234567890abcdef1234567890abcdef123456",
				AlertName:    "TestAlert",
				Namespace:    "default",
				Severity:     "invalid-severity", // INVALID: not in enum
				SourceType:   "prometheus-alert",
				Resource: types.ResourceIdentifier{
					Kind: "Pod",
					Name: "test-pod",
				},
			}

			err := prometheusAdapter.Validate(invalidSignal)
			Expect(err).To(HaveOccurred(), "Validation should fail for invalid severity")
			Expect(err.Error()).To(Or(
				ContainSubstring("severity"),
				ContainSubstring("invalid"),
			), "Error should mention severity issue")
		})

		It("should detect validation failure for empty namespace", func() {
			invalidSignal := &types.NormalizedSignal{
				Fingerprint:  "a1b2c3d4e5f6789012345678901234567890abcdef1234567890abcdef123456",
				AlertName:    "TestAlert",
				Namespace:    "", // INVALID: empty namespace
				Severity:     "warning",
				SourceType:   "prometheus-alert",
				Resource: types.ResourceIdentifier{
					Kind: "Pod",
					Name: "test-pod",
				},
			}

			err := prometheusAdapter.Validate(invalidSignal)
			Expect(err).To(HaveOccurred(), "Validation should fail for empty namespace")
			Expect(err.Error()).To(ContainSubstring("namespace"), "Error should mention namespace")
		})
	})

	Context("Gap #7: Error Details Structure", func() {
		It("should include all required Gap #7 fields in error_details", func() {
			Fail("IMPLEMENTATION REQUIRED: Verify error_details structure\n" +
				"  Required Fields (Gap #7):\n" +
				"    - message: Human-readable error description\n" +
				"    - code: Machine-readable error code (e.g., ERR_VALIDATION_EMPTY_FINGERPRINT)\n" +
				"    - component: 'gateway' (identifies source service)\n" +
				"    - retry_possible: false (validation errors not retryable)\n" +
				"  Implementation:\n" +
				"    1. Use sharedaudit.NewErrorDetailsFromValidationError('gateway', err)\n" +
				"    2. Map validation errors to specific error codes\n" +
				"    3. Set retry_possible based on error type\n" +
				"  Tracking: BR-AUDIT-005 Gap #7")

			// TODO: After implementation
			// Create test that verifies error_details structure matches Gap #7 requirements
			// This should use mock audit store to inspect the emitted event
		})
	})
})

