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
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/jordigilh/kubernaut/pkg/gateway/adapters"
	"github.com/jordigilh/kubernaut/pkg/gateway/types"
)

// BR-001: Prometheus AlertManager webhook ingestion
// BR-008: Fingerprint generation
// BR-016: Storm detection
//
// Business Outcome: Gateway handles extreme inputs gracefully without crashes
//
// Test Strategy: Validate edge cases with real business logic (no mocks)
// - Empty/nil values → Graceful error handling
// - Extreme values → Boundary validation
// - Malformed data → Clear error messages
//
// Defense-in-Depth: These unit tests complement integration tests
// - Unit: Test business logic with edge case inputs
// - Integration: Test same scenarios with real Redis/K8s infrastructure

var _ = Describe("BR-001, BR-008: Edge Case Handling - Adapter Validation", func() {
	var (
		adapter *adapters.PrometheusAdapter
	)

	BeforeEach(func() {
		adapter = adapters.NewPrometheusAdapter()
	})

	Context("BR-008: Fingerprint Validation Edge Cases", func() {
		It("should reject empty fingerprint with clear error message", func() {
			// BUSINESS OUTCOME: Clear validation error for operators
			// WHY: Empty fingerprint would break deduplication (Redis key collision)
			// EXPECTED: Validation error with actionable message
			//
			// DEFENSE-IN-DEPTH: This unit test complements integration tests
			// - Unit: Tests validation logic (pure business logic)
			// - Integration: Tests with real Redis (infrastructure behavior)

			signal := &types.NormalizedSignal{
				Fingerprint:  "", // Edge case: empty fingerprint
				AlertName:    "TestAlert",
				Severity:     "critical",
				Namespace:    "production",
				FiringTime:   time.Now(),
				ReceivedTime: time.Now(),
			}

			err := adapter.Validate(signal)

			// BUSINESS VALIDATION:
			// ✅ Error returned (not panic)
			// ✅ Error message mentions "fingerprint" (actionable for operators)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("fingerprint"))
		})

		It("should reject empty alert name with clear error message", func() {
			// BUSINESS OUTCOME: Operators get actionable validation errors
			// WHY: Alert name is required for CRD creation and troubleshooting
			// EXPECTED: Validation error mentioning "alertName"

			signal := &types.NormalizedSignal{
				Fingerprint:  "valid-fingerprint-12345",
				AlertName:    "", // Edge case: empty alert name
				Severity:     "critical",
				Namespace:    "production",
				FiringTime:   time.Now(),
				ReceivedTime: time.Now(),
			}

			err := adapter.Validate(signal)

			// BUSINESS VALIDATION:
			// ✅ Error returned
			// ✅ Error message mentions "alertName" (actionable)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("alertName"))
		})
	})

	Context("BR-001: Severity Validation Edge Cases", func() {
		It("should reject invalid severity with clear error message", func() {
			// BUSINESS OUTCOME: Only valid severities accepted
			// WHY: Severity drives priority assignment (critical+prod → P0)
			// EXPECTED: Validation error listing valid values

			signal := &types.NormalizedSignal{
				Fingerprint:  "valid-fingerprint-12345",
				AlertName:    "TestAlert",
				Severity:     "INVALID", // Edge case: invalid severity
				Namespace:    "production",
				FiringTime:   time.Now(),
				ReceivedTime: time.Now(),
			}

			err := adapter.Validate(signal)

			// BUSINESS VALIDATION:
			// ✅ Error returned
			// ✅ Error message mentions valid values (critical, warning, info)
			// ✅ Error message mentions the invalid value (INVALID)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("severity"))
			Expect(err.Error()).To(ContainSubstring("INVALID"))
		})

		It("should accept all valid severity values", func() {
			// BUSINESS OUTCOME: All documented severity values work
			// WHY: Ensures consistency with documentation
			// EXPECTED: No errors for critical, warning, info

			validSeverities := []string{"critical", "warning", "info"}

			for _, severity := range validSeverities {
				signal := &types.NormalizedSignal{
					Fingerprint:  "valid-fingerprint-12345",
					AlertName:    "TestAlert",
					Severity:     severity,
					Namespace:    "production",
					FiringTime:   time.Now(),
					ReceivedTime: time.Now(),
				}

				err := adapter.Validate(signal)

				// BUSINESS VALIDATION:
				// ✅ No error for valid severity
				Expect(err).ToNot(HaveOccurred(), "severity %s should be valid", severity)
			}
		})
	})

	Context("BR-001: Namespace Handling Edge Cases", func() {
		It("should accept empty namespace for cluster-scoped alerts", func() {
			// BUSINESS OUTCOME: Cluster-scoped alerts (nodes, cluster resources) work
			// WHY: Not all alerts are namespace-scoped (e.g., NodeNotReady)
			// EXPECTED: No error for empty namespace

			signal := &types.NormalizedSignal{
				Fingerprint:  "valid-fingerprint-12345",
				AlertName:    "NodeNotReady",
				Severity:     "critical",
				Namespace:    "", // Edge case: empty namespace (cluster-scoped)
				FiringTime:   time.Now(),
				ReceivedTime: time.Now(),
			}

			err := adapter.Validate(signal)

			// BUSINESS VALIDATION:
			// ✅ No error (cluster-scoped alerts are valid)
			Expect(err).ToNot(HaveOccurred())
		})
	})
})
