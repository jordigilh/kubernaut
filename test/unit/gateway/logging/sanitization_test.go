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
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/jordigilh/kubernaut/pkg/shared/sanitization"
)

// ========================================
// BR-GATEWAY-042: LOG SANITIZATION UNIT TESTS
// 📋 Testing Principle: Behavior + Correctness + Security
// ========================================
//
// SCOPE: Gateway-specific log sanitization patterns
// AUTHORITY: Follows Notification service pattern (test/unit/notification/sanitization_test.go)
// SHARED LIBRARY: pkg/shared/sanitization (DD-005 compliant)
//
// TEST STRATEGY:
// - Test shared library directly (no custom implementation)
// - Focus on Gateway-specific real-world scenarios
// - Validate security guarantees (secrets never exposed)
// - Table-driven tests for comprehensive coverage

var _ = Describe("BR-GATEWAY-042: Log Sanitization", func() {
	var sanitizer *sanitization.Sanitizer

	BeforeEach(func() {
		// DD-005: Use shared sanitization library
		sanitizer = sanitization.NewSanitizer()
	})

	// ==============================================
	// CATEGORY 1: Secret Pattern Detection (BEHAVIOR)
	// BR-GATEWAY-042: Gateway must redact all secret patterns from logs
	// ==============================================

	Context("Secret Pattern Detection - BEHAVIOR", func() {
		// TABLE-DRIVEN: Core secret patterns (8 critical patterns)
		DescribeTable("should redact secret patterns (BR-GATEWAY-042: Pattern detection)",
			func(input string, shouldContainRedacted bool, description string) {
				// BEHAVIOR: Sanitizer detects and redacts secrets
				result := sanitizer.Sanitize(input)

				if shouldContainRedacted {
					// BEHAVIOR VALIDATION: Secret was detected and redacted
					Expect(result).To(ContainSubstring("[REDACTED]"),
						"%s - should contain redaction marker", description)
					Expect(result).ToNot(Equal(input),
						"%s - input should be modified", description)
				} else {
					// BEHAVIOR VALIDATION: Non-sensitive content unchanged
					Expect(result).To(Equal(input),
						"%s - should not modify non-sensitive content", description)
				}
			},
			// EMAIL ADDRESSES: PII protection
			Entry("BR-GATEWAY-042.1: Email in log message",
				"User john.doe@example.com attempted login",
				true, "email addresses must be redacted (PII)"),

			// IP ADDRESSES: Internal infrastructure protection
			Entry("BR-GATEWAY-042.2: Internal IP address",
				"Connection from 192.168.1.100 rejected",
				true, "internal IP addresses must be redacted"),

			// API KEYS: OpenAI pattern (Gateway uses LLM)
			Entry("BR-GATEWAY-042.3: OpenAI API key",
				"LLM error with key sk-proj-abc123def456ghi789jkl012",
				true, "OpenAI API keys must be redacted"),

			// API KEYS: Generic pattern
			Entry("BR-GATEWAY-042.4: Generic API key",
				`config: {"apiKey": "xyz789abc123def456"}`,
				true, "generic API keys must be redacted"),

			// PASSWORDS: Connection strings
			Entry("BR-GATEWAY-042.5: Password in connection string",
				"Failed to connect: redis://user:secretpass@localhost:6379",
				true, "passwords in connection strings must be redacted"),

			// TOKENS: Bearer tokens (Gateway receives these)
			Entry("BR-GATEWAY-042.6: Bearer token in header",
				"Authorization: Bearer eyJhbGciOiJIUzI1NiIsInR5cCI6IkpXVCJ9",
				true, "Bearer tokens must be redacted"),

			// KUBERNETES SECRETS: Base64 encoded
			Entry("BR-GATEWAY-042.7: Kubernetes Secret data",
				"Secret data: password: cGFzc3dvcmQxMjM=",
				true, "base64 encoded secrets must be redacted"),

			// NON-SENSITIVE CONTENT: Should NOT be redacted
			Entry("BR-GATEWAY-042.8: Normal error message",
				"Signal processing failed: invalid severity level",
				false, "normal error messages without secrets"),
		)
	})

	// ==============================================
	// CATEGORY 2: Gateway Real-World Scenarios (CORRECTNESS)
	// BR-GATEWAY-042: Sanitize Gateway-specific messages
	// ==============================================

	Context("Gateway Real-World Scenarios - CORRECTNESS", func() {
		// TABLE-DRIVEN: Gateway-specific scenarios (5 scenarios)
		DescribeTable("should sanitize Gateway real-world scenarios (BR-GATEWAY-042: Complete redaction)",
			func(input string, expectedRedacted bool, shouldNotContain []string, shouldContain []string, scenario string) {
				// CORRECTNESS: Complete sanitization of complex messages
				result := sanitizer.Sanitize(input)

				if expectedRedacted {
					// CORRECTNESS VALIDATION: Redaction marker present
					Expect(result).To(ContainSubstring("[REDACTED]"),
						"%s - should contain redaction marker", scenario)

					// CORRECTNESS VALIDATION: Secrets completely removed
					for _, secret := range shouldNotContain {
						Expect(result).ToNot(ContainSubstring(secret),
							"%s - should not contain secret: %s", scenario, secret)
					}

					// CORRECTNESS VALIDATION: Non-sensitive context preserved
					for _, context := range shouldContain {
						Expect(result).To(ContainSubstring(context),
							"%s - should preserve context: %s", scenario, context)
					}
				} else {
					// CORRECTNESS VALIDATION: Clean content unchanged
					Expect(result).To(Equal(input),
						"%s - should not modify clean content", scenario)
				}
			},
			Entry("DataStorage API error with credentials",
				`DataStorage API call failed: 401 Unauthorized. URL: https://admin:dbpass123@datastorage:8080/api/v1/events`,
				true,
				[]string{"dbpass123"}, // Secrets to redact
				[]string{"DataStorage API call failed", "401 Unauthorized", "@datastorage:8080"}, // Context to preserve
				"DataStorage connection error with embedded credentials"),

			Entry("Redis connection error with password",
				`Failed to connect to Redis: redis://default:supersecret@localhost:6379/0 - connection refused`,
				true,
				[]string{"supersecret"}, // Redis password
				[]string{"Failed to connect to Redis", "connection refused", "@localhost:6379"}, // Error context
				"Redis connection error with password in URL"),

			Entry("Kubernetes API error with ServiceAccount token",
				`K8s API call failed: 403 Forbidden. Token: eyJhbGciOiJIUzI1NiIsInR5cCI6IkpXVCJ9.eyJzdWIiOiIxMjM0NTY3ODkwIn0.abc123 is invalid`,
				true,
				[]string{"eyJhbGciOiJIUzI1NiIsInR5cCI6IkpXVCJ9.eyJzdWIiOiIxMjM0NTY3ODkwIn0.abc123"}, // JWT token
				[]string{"K8s API call failed", "403 Forbidden", "is invalid"}, // Error context
				"Kubernetes API authentication error with JWT token"),

			Entry("Prometheus alert with sensitive labels",
				`Received alert: AlertName=DatabaseDown, Labels={password="dbpass", api_key="sk-abc123", severity="critical"}`,
				true,
				[]string{"dbpass", "sk-abc123"}, // Sensitive label values
				[]string{"AlertName=DatabaseDown", "severity=\"critical\""}, // Non-sensitive labels
				"Prometheus alert with sensitive data in labels"),

			Entry("Multiple secrets in CRD creation error",
				`Failed to create RemediationRequest CRD: metadata contains password=secret123 and token=ghp_xyz789abc for user admin@example.com`,
				true,
				[]string{"secret123", "ghp_xyz789abc", "admin@example.com"}, // Multiple secrets + PII
				[]string{"Failed to create RemediationRequest CRD", "for user"}, // Context
				"CRD creation error with multiple secret types and PII"),
		)
	})

	// ==============================================
	// CATEGORY 3: Edge Cases (CORRECTNESS)
	// BR-GATEWAY-042: Handle edge cases correctly
	// ==============================================

	Context("Edge Cases - CORRECTNESS", func() {
		// TABLE-DRIVEN: Edge case handling (4 tests)
		DescribeTable("should handle edge cases correctly (BR-GATEWAY-042: Edge case handling)",
			func(input string, expectedOutput string, scenario string) {
				// CORRECTNESS: Edge cases handled properly
				result := sanitizer.Sanitize(input)

				// CORRECTNESS VALIDATION: Exact expected output
				Expect(result).To(Equal(expectedOutput),
					"%s - should handle edge case correctly", scenario)
			},
			Entry("empty string",
				"",
				"",
				"Empty string should remain empty"),

			Entry("very long API key",
				"apiKey="+string(make([]byte, 1000)),
				"apiKey: [REDACTED]",
				"Very long API keys (1000+ chars) should be redacted"),

			Entry("multiple occurrences of same secret",
				"password=secret123 and again password=secret123",
				"password: [REDACTED] and again password: [REDACTED]",
				"Multiple occurrences of same secret pattern"),

			Entry("secrets with special characters",
				`password="p@$$w0rd!"`,
				`password: [REDACTED]`,
				"Secrets containing special characters"),
		)
	})

	// ==============================================
	// CATEGORY 4: Security Validation (BEHAVIOR)
	// BR-GATEWAY-042: Ensure secrets are never exposed
	// ==============================================

	Context("Security Validation - BEHAVIOR", func() {
		It("should never expose original secret in output (BR-GATEWAY-042: Security guarantee)", func() {
			// BEHAVIOR: Sanitizer guarantees secrets never appear in output
			// BUSINESS CONTEXT: Critical security requirement (CVSS 5.3)

			testSecrets := []string{
				"password=supersecret123",
				"apiKey=sk-proj-verysecretkey",
				"token=ghp_githubtoken123",
				"redis://user:redispass123@localhost:6379",
			}

			for _, secretInput := range testSecrets {
				result := sanitizer.Sanitize(secretInput)

				// BEHAVIOR VALIDATION: Secret completely removed
				Expect(result).To(ContainSubstring("[REDACTED]"),
					"Secret should be redacted: %s", secretInput)

				// SECURITY VALIDATION: Original secret not in output
				// Extract potential secret values for validation
				if len(secretInput) > 20 {
					secretValue := secretInput[len(secretInput)-20:] // Last 20 chars
					Expect(result).ToNot(ContainSubstring(secretValue),
						"Original secret value must not appear in output")
				}
			}
		})

		It("should handle Gateway signal payloads with secrets (BR-GATEWAY-042: Deep sanitization)", func() {
			// BEHAVIOR: Sanitizer finds secrets in nested JSON signal payloads
			signalPayload := `{
				"alert": {
					"labels": {
						"alertname": "HighCPU",
						"severity": "critical"
					},
					"annotations": {
						"description": "CPU usage high",
						"runbook_url": "https://admin:secretpass@runbook.example.com/cpu"
					}
				}
			}`

			result := sanitizer.Sanitize(signalPayload)

			// BEHAVIOR VALIDATION: All secrets redacted, structure preserved
			Expect(result).To(ContainSubstring("[REDACTED]"))
			Expect(result).ToNot(ContainSubstring("secretpass"))
			Expect(result).To(ContainSubstring("\"alertname\""))
			Expect(result).To(ContainSubstring("\"severity\""))
		})
	})
})
