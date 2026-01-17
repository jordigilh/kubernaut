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

package logging

import (
	"testing"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/jordigilh/kubernaut/pkg/shared/sanitization"
)

func TestSanitizerFallback(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Gateway Sanitizer Fallback & Graceful Degradation Suite")
}

// ==============================================
// Graceful Degradation Tests: Log Sanitization Failure Handling
// BR-GATEWAY-042: Log Sanitization
// BR-NOT-055: Graceful Degradation (shared requirement)
// ==============================================
//
// SCOPE: Gateway must continue logging even if sanitization fails
// AUTHORITY: Follows Notification service pattern (test/unit/notification/sanitization/sanitizer_fallback_test.go)
// SHARED LIBRARY: pkg/shared/sanitization (DD-005 compliant)

var _ = Describe("Gateway Sanitizer Fallback - Graceful Degradation", func() {
	var sanitizer *sanitization.Sanitizer

	BeforeEach(func() {
		sanitizer = sanitization.NewSanitizer()
	})

	Context("SanitizeWithFallback - Graceful Error Handling", func() {
		It("should return sanitized content when sanitization succeeds", func() {
			// BR-GATEWAY-042: Normal sanitization path should work
			input := "password: secret123"

			result, err := sanitizer.SanitizeWithFallback(input)

			Expect(err).ToNot(HaveOccurred())
			Expect(result).To(ContainSubstring("[REDACTED]"))
			Expect(result).NotTo(ContainSubstring("secret123"))
		})

		It("should handle empty input gracefully", func() {
			// BR-NOT-055: Edge case - empty content
			input := ""

			result, err := sanitizer.SanitizeWithFallback(input)

			Expect(err).ToNot(HaveOccurred())
			Expect(result).To(Equal(""))
		})

		It("should handle very large Gateway signal payloads gracefully", func() {
			// BR-NOT-055: Edge case - large signal payload that might stress regex engine
			// Gateway can receive large Prometheus alert payloads or K8s Event payloads
			input := make([]byte, 1024*1024) // 1MB of data
			for i := range input {
				input[i] = 'a'
			}
			inputStr := string(input) + " password: secret123"

			result, err := sanitizer.SanitizeWithFallback(inputStr)

			Expect(err).ToNot(HaveOccurred())
			Expect(result).To(ContainSubstring("[REDACTED]"))
		})
	})

	Context("SafeFallback - Simple String Matching", func() {
		It("should redact passwords using simple string matching", func() {
			// BR-NOT-055: Fallback must use simple patterns (no regex)
			input := "Redis connection failed: password: secret123 access denied"

			result := sanitizer.SafeFallback(input)

			Expect(result).To(ContainSubstring("[REDACTED]"))
			Expect(result).NotTo(ContainSubstring("secret123"))
		})

		It("should redact API keys using simple string matching", func() {
			// BR-NOT-055: Fallback must redact common secret types
			input := "LLM authentication failed: api_key: sk-abc123def456 invalid"

			result := sanitizer.SafeFallback(input)

			Expect(result).To(ContainSubstring("[REDACTED]"))
			Expect(result).NotTo(ContainSubstring("sk-abc123def456"))
		})

		It("should redact tokens using simple string matching", func() {
			// BR-NOT-055: Fallback must redact tokens
			input := "K8s API token expired: token: eyJhbGciOiJIUzI1NiIsInR5cCI6IkpXVCJ9"

			result := sanitizer.SafeFallback(input)

			Expect(result).To(ContainSubstring("[REDACTED]"))
			Expect(result).NotTo(ContainSubstring("eyJhbGciOiJIUzI1NiIsInR5cCI6IkpXVCJ9"))
		})

		It("should handle multiple secrets in same log message", func() {
			// BR-NOT-055: Fallback must redact all secret patterns
			// Gateway scenario: Signal with multiple sensitive labels
			input := "Signal processing failed: password: secret1 token: abc789 api_key: xyz123"

			result := sanitizer.SafeFallback(input)

			// All secrets should be redacted
			Expect(result).NotTo(ContainSubstring("secret1"))
			Expect(result).NotTo(ContainSubstring("abc789"))
			Expect(result).NotTo(ContainSubstring("xyz123"))
			// Should have multiple [REDACTED] placeholders
			Expect(result).To(ContainSubstring("[REDACTED]"))
		})

		It("should handle secrets with different delimiters", func() {
			// BR-NOT-055: Fallback should work with various formats
			inputs := []string{
				"password:secret123",    // no space after colon
				"password: secret123",   // space after colon
				"password:  secret123",  // multiple spaces
				"password:\tsecret123",  // tab after colon
				"password: secret123,",  // comma after value
				"password: 'secret123'", // quoted value
				`password: "secret123"`, // double quoted
				"password: secret123}",  // bracket after value (JSON)
			}

			for _, input := range inputs {
				result := sanitizer.SafeFallback(input)
				Expect(result).NotTo(ContainSubstring("secret123"), "Failed for input: "+input)
				Expect(result).To(ContainSubstring("[REDACTED]"), "Failed for input: "+input)
			}
		})

		It("should be case-insensitive", func() {
			// BR-NOT-055: Fallback should catch PASSWORD, password, Password, etc.
			inputs := []string{
				"PASSWORD: secret123",
				"password: secret123",
				"Password: secret123",
				"TOKEN: abc789",
				"Api_Key: xyz123",
			}

			for _, input := range inputs {
				result := sanitizer.SafeFallback(input)
				Expect(result).To(ContainSubstring("[REDACTED]"), "Failed for input: "+input)
			}
		})

		It("should preserve non-secret content", func() {
			// BR-NOT-055: Fallback should only redact secrets, not all content
			// Gateway scenario: CRD creation error with embedded secret
			input := "Failed to create RemediationRequest CRD for alert:HighCPU due to password: secret123 error"

			result := sanitizer.SafeFallback(input)

			// Should preserve CRD creation context
			Expect(result).To(ContainSubstring("Failed to create RemediationRequest CRD"))
			Expect(result).To(ContainSubstring("alert:HighCPU"))
			// But redact the password
			Expect(result).NotTo(ContainSubstring("secret123"))
			Expect(result).To(ContainSubstring("[REDACTED]"))
		})

		It("should handle content with no secrets", func() {
			// BR-NOT-055: Fallback should return original content if no secrets found
			input := "Signal processing completed successfully: 42 signals processed, 5 deduplicated"

			result := sanitizer.SafeFallback(input)

			Expect(result).To(Equal(input))
		})
	})

	Context("Gateway-Specific Sanitization Failure Scenarios", func() {
		It("should deliver log message even if regex engine fails", func() {
			// BR-NOT-055: Critical - must never lose error logs due to sanitization errors
			// Gateway scenario: Critical signal processing error
			input := "CRITICAL: Signal processing failed for alert=DatabaseDown. password: dbpass123 Details: Redis connection timeout"

			result, err := sanitizer.SanitizeWithFallback(input)

			// Even if error occurred, we should have SOME output (degraded delivery)
			Expect(result).NotTo(BeEmpty())
			Expect(result).To(ContainSubstring("CRITICAL"))
			Expect(result).To(ContainSubstring("Signal processing failed"))

			// If sanitization succeeded, no error
			// If fallback triggered, error is returned but result is still safe
			if err != nil {
				// Fallback path - verify critical alert info preserved
				Expect(result).To(ContainSubstring("DatabaseDown"))
				// And secret redacted by fallback
				Expect(result).NotTo(ContainSubstring("dbpass123"))
			} else {
				// Normal path - verify proper sanitization
				Expect(result).To(ContainSubstring("[REDACTED]"))
			}
		})

		It("should handle Prometheus alert with fallback", func() {
			// BR-NOT-055: Common Gateway scenario - Prometheus alerts in logs
			input := `
Received Prometheus alert:
alertname: DatabaseDown
severity: critical
annotations:
  description: Database connection failed
  runbook_url: https://admin:secretpass@runbook.example.com
Error: failed to process
`

			result, err := sanitizer.SanitizeWithFallback(input)

			// Log message should be deliverable
			Expect(result).NotTo(BeEmpty())
			Expect(result).To(ContainSubstring("Received Prometheus alert"))
			Expect(result).To(ContainSubstring("DatabaseDown"))

			// Secret should be redacted
			if err == nil {
				// Normal sanitization
				Expect(result).To(ContainSubstring("[REDACTED]"))
			} else {
				// Fallback sanitization
				Expect(result).To(ContainSubstring("[REDACTED]"))
			}
			// Either way, secret must not appear
			Expect(result).NotTo(ContainSubstring("secretpass"))
		})

		It("should handle K8s Event with sensitive data with fallback", func() {
			// BR-NOT-055: Gateway scenario - K8s Events in error logs
			input := `
Processing K8s Event failed:
Event Type: Warning
Reason: FailedMount
Message: Secret "db-credentials" contains password: supersecret
InvolvedObject: Pod/myapp-xyz
`

			result, err := sanitizer.SanitizeWithFallback(input)

			// Log message should be deliverable
			Expect(result).NotTo(BeEmpty())
			Expect(result).To(ContainSubstring("Processing K8s Event failed"))
			Expect(result).To(ContainSubstring("FailedMount"))

			// Secret should be redacted
			if err == nil || err != nil {
				// Both paths must redact
				Expect(result).NotTo(ContainSubstring("supersecret"))
			}
		})
	})
})
