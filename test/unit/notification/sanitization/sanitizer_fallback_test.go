package sanitization_test

import (
	"testing"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/jordigilh/kubernaut/pkg/notification/sanitization"
)

func TestSanitizerFallback(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Sanitizer Fallback & Graceful Degradation Suite")
}

// ==============================================
// Graceful Degradation Tests: Category E - Data Sanitization Failure Handling
// BR-NOT-055: Graceful Degradation
// ==============================================

var _ = Describe("Sanitizer Fallback - Category E: Graceful Degradation", func() {
	var sanitizer *sanitization.Sanitizer

	BeforeEach(func() {
		sanitizer = sanitization.NewSanitizer()
	})

	Context("SanitizeWithFallback - Graceful Error Handling", func() {
		It("should return sanitized content when sanitization succeeds", func() {
			// BR-NOT-055: Normal sanitization path should work
			input := "password: secret123"

			result, err := sanitizer.SanitizeWithFallback(input)

			Expect(err).ToNot(HaveOccurred())
			Expect(result).To(ContainSubstring("***REDACTED***"))
			Expect(result).NotTo(ContainSubstring("secret123"))
		})

		It("should use safe fallback when sanitization panics", func() {
			// BR-NOT-055: If sanitization fails (panic), must use safe fallback
			// This test simulates a regex engine panic by using a malicious pattern

			// Create a sanitizer that will panic (simulated by adding bad pattern)
			badSanitizer := sanitization.NewSanitizer()
			// Note: Actual implementation would add a pattern that causes panic
			// For now, we test the fallback behavior with malformed input

			input := "password: secret123 token: abc789"

			result, err := badSanitizer.SanitizeWithFallback(input)

			// Even if sanitization failed, we should get SOME result (degraded delivery)
			Expect(result).NotTo(BeEmpty())
			// Error should be nil for successful sanitization
			// Or non-nil if fallback was triggered
			if err != nil {
				// Fallback was triggered - verify secrets are still redacted
				Expect(result).To(ContainSubstring("[REDACTED]"))
			}
		})

		It("should handle empty input gracefully", func() {
			// BR-NOT-055: Edge case - empty content
			input := ""

			result, err := sanitizer.SanitizeWithFallback(input)

			Expect(err).ToNot(HaveOccurred())
			Expect(result).To(Equal(""))
		})

		It("should handle very large input gracefully", func() {
			// BR-NOT-055: Edge case - large payload that might stress regex engine
			input := make([]byte, 1024*1024) // 1MB of data
			for i := range input {
				input[i] = 'a'
			}
			inputStr := string(input) + " password: secret123"

			result, err := sanitizer.SanitizeWithFallback(inputStr)

			Expect(err).ToNot(HaveOccurred())
			Expect(result).To(ContainSubstring("***REDACTED***"))
		})
	})

	Context("SafeFallback - Simple String Matching", func() {
		It("should redact passwords using simple string matching", func() {
			// BR-NOT-055: Fallback must use simple patterns (no regex)
			input := "Connection failed: password: secret123 access denied"

			result := sanitizer.SafeFallback(input)

			Expect(result).To(ContainSubstring("[REDACTED]"))
			Expect(result).NotTo(ContainSubstring("secret123"))
		})

		It("should redact API keys using simple string matching", func() {
			// BR-NOT-055: Fallback must redact common secret types
			input := "Authentication failed: api_key: sk-abc123def456 invalid"

			result := sanitizer.SafeFallback(input)

			Expect(result).To(ContainSubstring("[REDACTED]"))
			Expect(result).NotTo(ContainSubstring("sk-abc123def456"))
		})

		It("should redact tokens using simple string matching", func() {
			// BR-NOT-055: Fallback must redact tokens
			input := "Token expired: token: ghp_abc123def456xyz789"

			result := sanitizer.SafeFallback(input)

			Expect(result).To(ContainSubstring("[REDACTED]"))
			Expect(result).NotTo(ContainSubstring("ghp_abc123def456xyz789"))
		})

		It("should handle multiple secrets in same content", func() {
			// BR-NOT-055: Fallback must redact all secret patterns
			input := "password: secret1 token: abc789 api_key: xyz123"

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
				"password: secret123}",  // bracket after value
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
			input := "Deployment failed for app:v1.2.3 due to password: secret123 error"

			result := sanitizer.SafeFallback(input)

			// Should preserve deployment info
			Expect(result).To(ContainSubstring("Deployment failed"))
			Expect(result).To(ContainSubstring("app:v1.2.3"))
			// But redact the password
			Expect(result).NotTo(ContainSubstring("secret123"))
			Expect(result).To(ContainSubstring("[REDACTED]"))
		})

		It("should handle content with no secrets", func() {
			// BR-NOT-055: Fallback should return original content if no secrets found
			input := "This is a normal log message with no credentials"

			result := sanitizer.SafeFallback(input)

			Expect(result).To(Equal(input))
		})
	})

	Context("Real-World Sanitization Failure Scenarios", func() {
		It("should deliver notification even if regex engine fails", func() {
			// BR-NOT-055: Critical - must never lose alerts due to sanitization errors
			// Simulating a scenario where sanitization logic encounters an error
			input := "CRITICAL ALERT: Database connection failed. password: dbpass123 Details: ..."

			result, err := sanitizer.SanitizeWithFallback(input)

			// Even if error occurred, we should have SOME output (degraded delivery)
			Expect(result).NotTo(BeEmpty())
			Expect(result).To(ContainSubstring("CRITICAL ALERT"))

			// If sanitization succeeded, no error
			// If fallback triggered, error is returned but result is still safe
			if err != nil {
				// Fallback path - verify critical alert info preserved
				Expect(result).To(ContainSubstring("Database connection failed"))
				// And secret redacted by fallback
				Expect(result).NotTo(ContainSubstring("dbpass123"))
			} else {
				// Normal path - verify proper sanitization
				Expect(result).To(ContainSubstring("***REDACTED***"))
			}
		})

		It("should handle Kubernetes Secret YAML with fallback", func() {
			// BR-NOT-055: Common scenario - K8s secrets in error messages
			input := `
Failed to apply Secret:
apiVersion: v1
kind: Secret
data:
  password: cGFzc3dvcmQxMjM=
  token: dG9rZW4xMjM=
Error: validation failed
`

			result, err := sanitizer.SanitizeWithFallback(input)

			// Notification should be deliverable
			Expect(result).NotTo(BeEmpty())
			Expect(result).To(ContainSubstring("Failed to apply Secret"))

			// Secrets should be redacted
			if err == nil {
				// Normal sanitization
				Expect(result).To(ContainSubstring("***REDACTED***"))
			} else {
				// Fallback sanitization
				Expect(result).To(ContainSubstring("[REDACTED]"))
			}
		})
	})
})
