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

package authwebhook

import (
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

// TDD RED Phase: Validator Tests
// BR-WE-013: Validate clearance requests before authentication
// Tests written BEFORE implementation exists
var _ = Describe("Validator", func() {
	Context("ValidateReason", func() {
		It("should accept valid reason with sufficient length", func() {
			// BUSINESS OUTCOME: Ensure operators provide meaningful explanations
			err := ValidateReason("Fixed RBAC permissions in target namespace", 10)

			Expect(err).ToNot(HaveOccurred())
		})

		It("should reject empty reason", func() {
			// BUSINESS OUTCOME: Prevent accidental block clears without explanation
			err := ValidateReason("", 10)

			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("reason is required"))
		})

		It("should reject reason shorter than minimum length", func() {
			// BUSINESS OUTCOME: Ensure meaningful explanations (not "ok" or "done")
			err := ValidateReason("Fixed", 10)

			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("must be at least 10 characters"))
			Expect(err.Error()).To(ContainSubstring("got 5"))
		})

		It("should reject reason with only whitespace", func() {
			// BUSINESS OUTCOME: Prevent meaningless reasons
			err := ValidateReason("          ", 10)

			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("cannot be only whitespace"))
		})

		It("should accept reason exactly at minimum length", func() {
			// BUSINESS OUTCOME: Boundary case validation
			err := ValidateReason("1234567890", 10)

			Expect(err).ToNot(HaveOccurred())
		})

		It("should accept reason with newlines and special characters", func() {
			// BUSINESS OUTCOME: Support detailed multi-line explanations
			reason := "Fixed permissions.\nVerified cluster state.\nSafe to proceed!"
			err := ValidateReason(reason, 10)

			Expect(err).ToNot(HaveOccurred())
		})
	})

	Context("ValidateTimestamp", func() {
		It("should accept valid timestamp in the past", func() {
			// BUSINESS OUTCOME: Accept legitimate clearance requests
			ts := time.Now().Add(-5 * time.Minute)
			err := ValidateTimestamp(ts)

			Expect(err).ToNot(HaveOccurred())
		})

		It("should accept timestamp from exactly now", func() {
			// BUSINESS OUTCOME: Accept immediate clearance requests
			ts := time.Now()
			err := ValidateTimestamp(ts)

			Expect(err).ToNot(HaveOccurred())
		})

		It("should reject zero timestamp", func() {
			// BUSINESS OUTCOME: Prevent malformed requests
			err := ValidateTimestamp(time.Time{})

			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("timestamp is required"))
		})

		It("should reject future timestamp", func() {
			// BUSINESS OUTCOME: Prevent time manipulation attacks
			ts := time.Now().Add(1 * time.Hour)
			err := ValidateTimestamp(ts)

			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("cannot be in the future"))
		})
	})
})

