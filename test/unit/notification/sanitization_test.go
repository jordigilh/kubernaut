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

package notification

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/jordigilh/kubernaut/pkg/shared/sanitization"
)

// BR-NOT-054: Data Sanitization - BEHAVIOR & CORRECTNESS Testing
//
// FOCUS: Test WHAT the sanitizer does (behavior), NOT HOW it does it (regex patterns)
// BEHAVIOR: Does it redact secrets? Does it preserve non-sensitive content?
// CORRECTNESS: Are specific secret patterns detected? Is redaction complete?

var _ = Describe("BR-NOT-054: Data Sanitization", func() {
	var sanitizer *sanitization.Sanitizer

	BeforeEach(func() {
		sanitizer = sanitization.NewSanitizer()
	})

	// ==============================================
	// CATEGORY 1: Secret Pattern Detection (BEHAVIOR)
	// BR-NOT-054: System must redact all secret patterns
	// ==============================================

	Context("Secret Pattern Detection - BEHAVIOR", func() {
		// TABLE-DRIVEN: Secret pattern redaction (30+ patterns)
		DescribeTable("should redact secret patterns (BR-NOT-054: Pattern detection)",
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
			// PASSWORD PATTERNS: Common password formats
			Entry("password key-value", "password=secret123", true, "passwords in key-value pairs"),
			Entry("password JSON", `{"password":"secret123"}`, true, "passwords in JSON format"),
			Entry("password YAML", "password: secret123", true, "passwords in YAML format"),
			Entry("password URL", "https://user:pass123@example.com", true, "passwords in URLs"),
			Entry("passwd variant", "passwd=mypass", true, "passwd alternative spelling"),
			Entry("pwd variant", "pwd: secretpass", true, "pwd abbreviation"),

			// API KEY PATTERNS: Various API key formats
			Entry("apiKey camelCase", `apiKey: sk-abc123def`, true, "API keys in camelCase"),
			Entry("api_key snake_case", `api_key=xyz789`, true, "API keys in snake_case"),
			Entry("API_KEY uppercase", `API_KEY="token123"`, true, "API keys in UPPERCASE"),
			Entry("OpenAI key format", `sk-proj-abc123def456`, true, "OpenAI API key format"),

			// TOKEN PATTERNS: Authorization tokens
			Entry("Bearer token", `Authorization: Bearer xyz789abc`, true, "Bearer authorization tokens"),
			Entry("GitHub token", `token: ghp_abc123def456ghp_abc123def456ghp1234`, true, "GitHub personal access tokens"),
			Entry("access_token", `access_token=ya29.abc123`, true, "OAuth access tokens"),

			// CLOUD PROVIDER CREDENTIALS: AWS, GCP, Azure
			Entry("AWS access key", `AWS_ACCESS_KEY_ID=AKIAIOSFODNN7EXAMPLE`, true, "AWS IAM access keys"),
			Entry("AWS secret key", `AWS_SECRET_ACCESS_KEY=wJalrXUtnFEMI/K7MDENG/bPxRfiCYEXAMPLEKEY`, true, "AWS IAM secret keys"),

			// DATABASE CONNECTION STRINGS: Common DB URLs
			Entry("PostgreSQL URL", `postgresql://user:password123@localhost:5432/db`, true, "PostgreSQL connection strings"),
			Entry("MySQL URL", `mysql://root:secret@localhost/db`, true, "MySQL connection strings"),
			Entry("MongoDB URL", `mongodb://admin:pass123@localhost:27017/db`, true, "MongoDB connection strings"),

			// CERTIFICATE PATTERNS: PEM format certificates
			Entry("PEM certificate", "-----BEGIN CERTIFICATE-----\nMIIC...ABC\n-----END CERTIFICATE-----", true, "PEM format certificates"),
			Entry("private key", "-----BEGIN PRIVATE KEY-----\nABC123\n-----END PRIVATE KEY-----", true, "PEM format private keys"),

			// NON-SENSITIVE CONTENT: Should NOT be redacted
			Entry("normal error message", "Deployment failed: image pull error", false, "normal error messages without secrets"),
			Entry("URL without credentials", "https://example.com/api/v1", false, "URLs without embedded credentials"),
			Entry("environment variable name", "environment: production", false, "environment variable names (not values)"),
		)
	})

	// ==============================================
	// CATEGORY 2: Real-World Scenarios (CORRECTNESS)
	// BR-NOT-054: Sanitize complex real-world messages
	// ==============================================

	Context("Real-World Scenarios - CORRECTNESS", func() {
		// TABLE-DRIVEN: Real-world notification scenarios
		DescribeTable("should sanitize real-world notification scenarios (BR-NOT-054: Complete redaction)",
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
			Entry("database connection error with credentials",
				`Failed to connect to PostgreSQL: postgresql://admin:supersecret@localhost:5432/mydb - connection refused`,
				true,
				[]string{"supersecret"}, // Secrets to redact
				[]string{"postgresql://admin:", "@localhost:5432/mydb", "connection refused"}, // Context to preserve
				"Database error with embedded credentials"),

			Entry("Kubernetes Secret YAML manifest",
				`apiVersion: v1
kind: Secret
metadata:
  name: database-credentials
data:
  username: YWRtaW4=
  password: cGFzc3dvcmQxMjM=`,
				true,
				[]string{"YWRtaW4=", "cGFzc3dvcmQxMjM="}, // Base64 secrets
				[]string{"apiVersion: v1", "kind: Secret", "database-credentials"}, // Metadata
				"Kubernetes Secret YAML with base64 data"),

			Entry("API error response with token",
				`API call failed: 401 Unauthorized. Token: ghp_abc123def456xyz789ghp_abc123def456 is invalid or expired`,
				true,
				[]string{"ghp_abc123def456xyz789ghp_abc123def456"},                       // GitHub token
				[]string{"API call failed", "401 Unauthorized", "is invalid or expired"}, // Error context
				"API authentication error with token"),

			Entry("multiple secrets in one message",
				`Connection failed with password=secret123 and apiKey=xyz789 for user admin`,
				true,
				[]string{"secret123", "xyz789"}, // Multiple secrets
				[]string{"Connection failed", "for user admin"}, // Context
				"Multiple different secret types in one message"),

			Entry("OpenAI API error with key",
				`LLM error: Invalid API key sk-proj-abc123def456ghi789jkl012mno345pqr678`,
				true,
				[]string{"sk-proj-abc123def456ghi789jkl012mno345pqr678"}, // OpenAI key
				[]string{"LLM error", "Invalid API key"},                 // Error context
				"LLM API error with OpenAI key"),

			Entry("AWS credentials in environment",
				`Export failed: AWS_ACCESS_KEY_ID=AKIAIOSFODNN7EXAMPLE AWS_SECRET_ACCESS_KEY=wJalrXUtnFEMI/K7MDENG/bPxRfiCYEXAMPLEKEY`,
				true,
				[]string{"AKIAIOSFODNN7EXAMPLE", "wJalrXUtnFEMI/K7MDENG/bPxRfiCYEXAMPLEKEY"}, // AWS creds
				[]string{"Export failed", "AWS_ACCESS_KEY_ID=", "AWS_SECRET_ACCESS_KEY="},    // Variable names
				"AWS credentials in environment variables"),

			Entry("clean deployment error",
				`Deployment failed: image pull error for registry.example.com/app:v1.2.3`,
				false,
				[]string{}, // No secrets
				[]string{}, // No validation needed (unchanged)
				"Clean deployment error without credentials"),

			Entry("normal message with no secrets",
				`This is a clean message with no secrets`,
				false,
				[]string{}, // No secrets
				[]string{}, // No validation needed (unchanged)
				"Plain text message without sensitive data"),
		)
	})

	// ==============================================
	// CATEGORY 3: Edge Cases (CORRECTNESS)
	// BR-NOT-054: Handle edge cases correctly
	// ==============================================

	Context("Edge Cases - CORRECTNESS", func() {
		// TABLE-DRIVEN: Edge case handling
		DescribeTable("should handle edge cases correctly (BR-NOT-054: Edge case handling)",
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

			Entry("whitespace only",
				"   \n\t  ",
				"   \n\t  ",
				"Whitespace-only string should be unchanged"),

			Entry("secret at start of string",
				"password=secret123 followed by text",
				"password: [REDACTED] followed by text",
				"Secret at beginning of string"),

			Entry("secret at end of string",
				"text followed by password=secret123",
				"text followed by password: [REDACTED]",
				"Secret at end of string"),

			Entry("multiple occurrences of same secret",
				"password=secret123 and again password=secret123",
				"password: [REDACTED] and again password: [REDACTED]",
				"Multiple occurrences of same secret pattern"),

			Entry("secrets with special characters",
				`password="p@$$w0rd!"`,
				`password: [REDACTED]`,
				"Secrets containing special characters"),

			Entry("very long secret value",
				"apiKey="+string(make([]byte, 1000)),
				"apiKey: [REDACTED]",
				"Very long secret values (1000+ chars)"),
		)
	})

	// ==============================================
	// CATEGORY 4: Security Validation (BEHAVIOR)
	// BR-NOT-054: Ensure secrets are never exposed
	// ==============================================

	Context("Security Validation - BEHAVIOR", func() {
		It("should never expose original secret in output (BR-NOT-054: Security guarantee)", func() {
			// BEHAVIOR: Sanitizer guarantees secrets never appear in output
			// BUSINESS CONTEXT: Critical security requirement

			testSecrets := []string{
				"password=supersecret123",
				"apiKey=sk-proj-verysecretkey",
				"token=ghp_githubtoken123",
				"AWS_SECRET_ACCESS_KEY=wJalrXUtnFEMI/K7MDENG/bPxRfiCYEXAMPLEKEY", // 40 chars as required by pattern
			}

			for _, secretInput := range testSecrets {
				result := sanitizer.Sanitize(secretInput)

				// BEHAVIOR VALIDATION: Secret completely removed
				Expect(result).To(ContainSubstring("[REDACTED]"),
					"Secret should be redacted: %s", secretInput)

				// SECURITY VALIDATION: Original secret not in output
				// Extract the secret value (after '=')
				secretValue := secretInput[len(secretInput)-20:] // Last 20 chars
				Expect(result).ToNot(ContainSubstring(secretValue),
					"Original secret value must not appear in output")
			}
		})

		It("should handle secrets in nested structures (BR-NOT-054: Deep sanitization)", func() {
			// BEHAVIOR: Sanitizer finds secrets in nested JSON/YAML
			nestedJSON := `{
				"config": {
					"database": {
						"connection": "postgresql://user:secretpass@localhost/db"
					},
					"api": {
						"key": "sk-proj-apikey123"
					}
				}
			}`

			result := sanitizer.Sanitize(nestedJSON)

			// BEHAVIOR VALIDATION: All secrets redacted, structure preserved
			Expect(result).To(ContainSubstring("[REDACTED]"))
			Expect(result).ToNot(ContainSubstring("secretpass"))
			Expect(result).ToNot(ContainSubstring("sk-proj-apikey123"))
			Expect(result).To(ContainSubstring("\"database\""))
			Expect(result).To(ContainSubstring("\"api\""))
		})
	})
})
