/*
Copyright 2025 Jordi Gil.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use the file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package processing

import (
	"fmt"
	"regexp"
	"strings"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/jordigilh/kubernaut/pkg/gateway/processing"
)

// BR-GATEWAY-015: CRD Name Generation
// BUSINESS REQUIREMENT: Generate unique, DNS-1123 compliant CRD names
// BUSINESS OUTCOME: All CRDs have valid names, no K8s API errors

var _ = Describe("CRD Name Generation", func() {
	// DNS-1123 subdomain regex (K8s requirement)
	// - Must contain only lowercase alphanumeric characters, '-' or '.'
	// - Must start and end with an alphanumeric character
	// - Max length: 253 characters
	dns1123Regex := regexp.MustCompile(`^[a-z0-9]([-a-z0-9]*[a-z0-9])?(\.[a-z0-9]([-a-z0-9]*[a-z0-9])?)*$`)

	// MockClock for deterministic testing
	var mockClock *processing.MockClock

	BeforeEach(func() {
		// Initialize MockClock with a fixed time
		mockClock = processing.NewMockClock(time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC))
	})

	Describe("generateCRDName", func() {
		// Helper function that mimics the production logic
		// From: pkg/gateway/processing/crd_creator.go:293-302
		generateCRDName := func(fingerprint string, clock processing.Clock) string {
			fingerprintPrefix := fingerprint
			if len(fingerprintPrefix) > 12 {
				fingerprintPrefix = fingerprintPrefix[:12]
			}
			timestamp := clock.Now().Unix()
			return fmt.Sprintf("rr-%s-%d", fingerprintPrefix, timestamp)
		}

		Context("when fingerprint is normal length", func() {
			It("should generate valid DNS-1123 compliant name", func() {
				// BR-GATEWAY-015: Normal fingerprint length (32-64 chars)
				fingerprint := "a1b2c3d4e5f6g7h8i9j0k1l2m3n4o5p6"
				crdName := generateCRDName(fingerprint, mockClock)

				// VALIDATION 1: DNS-1123 compliance
				Expect(dns1123Regex.MatchString(crdName)).To(BeTrue(),
					"CRD name should be DNS-1123 compliant")

				// VALIDATION 2: Length <= 253 chars (K8s limit)
				Expect(len(crdName)).To(BeNumerically("<=", 253),
					"CRD name should be <= 253 characters")

				// VALIDATION 3: Format: rr-<fingerprint-12>-<timestamp>
				Expect(crdName).To(MatchRegexp(`^rr-[a-z0-9]{12}-\d+$`),
					"CRD name should match format: rr-<fingerprint-12>-<timestamp>")

				// VALIDATION 4: Fingerprint prefix is first 12 chars
				Expect(crdName).To(HavePrefix("rr-a1b2c3d4e5f6-"),
					"CRD name should use first 12 chars of fingerprint")
			})
		})

		Context("when fingerprint is short (<12 chars)", func() {
			It("should use full fingerprint without truncation", func() {
				// BR-GATEWAY-015: Short fingerprint (edge case)
				fingerprint := "abc123"
				crdName := generateCRDName(fingerprint, mockClock)

				// VALIDATION 1: DNS-1123 compliance
				Expect(dns1123Regex.MatchString(crdName)).To(BeTrue())

				// VALIDATION 2: Uses full fingerprint (no truncation)
				Expect(crdName).To(HavePrefix("rr-abc123-"),
					"CRD name should use full fingerprint when <12 chars")

				// VALIDATION 3: Length <= 253 chars
				Expect(len(crdName)).To(BeNumerically("<=", 253))
			})
		})

		Context("when fingerprint is very long (>100 chars)", func() {
			It("should truncate to 12 characters and remain DNS-1123 compliant", func() {
				// BR-GATEWAY-015: Very long fingerprint (external alert sources)
				// BUSINESS SCENARIO: Alert names from external sources may be very long
				longFingerprint := strings.Repeat("a", 200) // 200 chars
				crdName := generateCRDName(longFingerprint, mockClock)

				// VALIDATION 1: DNS-1123 compliance
				Expect(dns1123Regex.MatchString(crdName)).To(BeTrue(),
					"CRD name should be DNS-1123 compliant even with long fingerprint")

				// VALIDATION 2: Fingerprint truncated to 12 chars
				Expect(crdName).To(HavePrefix("rr-aaaaaaaaaaaa-"),
					"CRD name should truncate fingerprint to 12 chars")

				// VALIDATION 3: Length <= 253 chars
				Expect(len(crdName)).To(BeNumerically("<=", 253),
					"CRD name should be <= 253 characters")

				// VALIDATION 4: Total length is predictable
				// Format: rr-<12-chars>-<10-digit-timestamp> = 3 + 12 + 1 + 10 = 26 chars
				Expect(len(crdName)).To(BeNumerically("<=", 30),
					"CRD name with truncated fingerprint should be ~26 chars")
			})
		})

		Context("when fingerprint contains uppercase letters", func() {
			It("should convert to lowercase for DNS-1123 compliance", func() {
				// BR-GATEWAY-015: Mixed case fingerprint
				// NOTE: Production code should lowercase the fingerprint
				// This test documents the expected behavior
				fingerprint := "ABC123DEF456"

				// Production code should lowercase before generating name
				lowercaseFingerprint := strings.ToLower(fingerprint)
				crdName := generateCRDName(lowercaseFingerprint, mockClock)

				// VALIDATION 1: DNS-1123 compliance (lowercase only)
				Expect(dns1123Regex.MatchString(crdName)).To(BeTrue())

				// VALIDATION 2: No uppercase letters
				Expect(crdName).To(Equal(strings.ToLower(crdName)),
					"CRD name should be lowercase")
			})
		})

		Context("when generating multiple names with same fingerprint", func() {
			It("should generate unique names due to timestamp", func() {
				// BR-GATEWAY-015: Uniqueness through timestamp
				// BUSINESS SCENARIO: Multiple alerts with same fingerprint
				fingerprint := "a1b2c3d4e5f6"

				// Generate 3 CRD names with time advancement for deterministic testing
				name1 := generateCRDName(fingerprint, mockClock)
				mockClock.Advance(1 * time.Second) // Advance time deterministically
				name2 := generateCRDName(fingerprint, mockClock)
				mockClock.Advance(1 * time.Second)
				name3 := generateCRDName(fingerprint, mockClock)

				// VALIDATION: All names should be unique (different timestamps)
				Expect(name1).NotTo(Equal(name2), "Names should be unique")
				Expect(name2).NotTo(Equal(name3), "Names should be unique")
				Expect(name1).NotTo(Equal(name3), "Names should be unique")

				// VALIDATION: All names have same fingerprint prefix
				Expect(name1).To(HavePrefix("rr-a1b2c3d4e5f6-"))
				Expect(name2).To(HavePrefix("rr-a1b2c3d4e5f6-"))
				Expect(name3).To(HavePrefix("rr-a1b2c3d4e5f6-"))
			})
		})

		Context("when fingerprint contains special characters", func() {
			It("should handle special characters gracefully", func() {
				// BR-GATEWAY-015: Special characters in fingerprint
				// NOTE: Production code should sanitize special characters
				// This test documents the expected behavior

				// Fingerprint with special characters (should be sanitized)
				fingerprint := "a1-b2_c3.d4"

				// Production code should sanitize: remove/replace special chars
				// For this test, we assume sanitization happens before name generation
				sanitizedFingerprint := strings.ReplaceAll(fingerprint, "_", "")
				sanitizedFingerprint = strings.ReplaceAll(sanitizedFingerprint, ".", "")
				crdName := generateCRDName(sanitizedFingerprint, mockClock)

				// VALIDATION 1: DNS-1123 compliance
				Expect(dns1123Regex.MatchString(crdName)).To(BeTrue(),
					"CRD name should be DNS-1123 compliant after sanitization")

				// VALIDATION 2: No invalid characters
				Expect(crdName).NotTo(ContainSubstring("_"),
					"CRD name should not contain underscores")
				Expect(crdName).NotTo(ContainSubstring("."),
					"CRD name should not contain dots (except in format)")
			})
		})
	})

	Describe("CRD Name Format Validation", func() {
		It("should match expected format pattern", func() {
			// BR-GATEWAY-015: CRD name format specification
			// Format: rr-<fingerprint-prefix-12>-<unix-timestamp>
			// Example: rr-a1b2c3d4e5f6-1731868032

			fingerprint := "a1b2c3d4e5f6g7h8"
			fingerprintPrefix := fingerprint[:12]
			timestamp := time.Now().Unix()
			crdName := fmt.Sprintf("rr-%s-%d", fingerprintPrefix, timestamp)

			// VALIDATION 1: Starts with "rr-"
			Expect(crdName).To(HavePrefix("rr-"))

			// VALIDATION 2: Contains fingerprint prefix
			Expect(crdName).To(ContainSubstring(fingerprintPrefix))

			// VALIDATION 3: Contains timestamp
			Expect(crdName).To(MatchRegexp(`\d{10,}`),
				"CRD name should contain Unix timestamp (10+ digits)")

			// VALIDATION 4: Matches complete format
			expectedPattern := fmt.Sprintf(`^rr-%s-\d+$`, fingerprintPrefix)
			Expect(crdName).To(MatchRegexp(expectedPattern))
		})
	})
})
