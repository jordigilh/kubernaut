package notification

import (
	"testing"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/jordigilh/kubernaut/pkg/notification/sanitization"
)

func TestSanitization(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Data Sanitization Suite")
}

var _ = Describe("Data Sanitization", func() {
	var sanitizer *sanitization.Sanitizer

	BeforeEach(func() {
		sanitizer = sanitization.NewSanitizer()
	})

	// ⭐ TABLE-DRIVEN: Secret pattern redaction (20+ patterns)
	DescribeTable("should redact secret patterns",
		func(input string, shouldContainRedacted bool, description string) {
			result := sanitizer.Sanitize(input)
			if shouldContainRedacted {
				Expect(result).To(ContainSubstring("***REDACTED***"), description)
				Expect(result).ToNot(Equal(input), "Input should be modified")
			} else {
				Expect(result).To(Equal(input), description)
			}
		},
		// Password patterns
		Entry("password key-value", "password=secret123", true, "passwords in key-value"),
		Entry("password JSON", `{"password":"secret123"}`, true, "passwords in JSON"),
		Entry("password YAML", "password: secret123", true, "passwords in YAML"),
		Entry("password URL", "https://user:pass123@example.com", true, "passwords in URLs"),
		Entry("passwd variant", "passwd=mypass", true, "passwd variant"),
		Entry("pwd variant", "pwd: secretpass", true, "pwd variant"),

		// API key patterns
		Entry("apiKey camelCase", `apiKey: sk-abc123def`, true, "API keys camelCase"),
		Entry("api_key snake_case", `api_key=xyz789`, true, "API keys snake_case"),
		Entry("API_KEY uppercase", `API_KEY="token123"`, true, "API keys uppercase"),
		Entry("OpenAI key", `sk-proj-abc123def456`, true, "OpenAI API keys"),

		// Token patterns
		Entry("Bearer token", `Authorization: Bearer xyz789abc`, true, "Bearer tokens"),
		Entry("token field", `token: ghp_abc123def456ghp_abc123def456ghp1234`, true, "GitHub tokens"),
		Entry("access_token", `access_token=ya29.abc123`, true, "access tokens"),

		// Cloud provider credentials
		Entry("AWS access key", `AWS_ACCESS_KEY_ID=AKIAIOSFODNN7EXAMPLE`, true, "AWS access keys"),
		Entry("AWS secret key", `AWS_SECRET_ACCESS_KEY=wJalrXUtnFEMI/K7MDENG/bPxRfiCYEXAMPLEKEY`, true, "AWS secret keys"),

		// Database connection strings
		Entry("PostgreSQL URL", `postgresql://user:password123@localhost:5432/db`, true, "PostgreSQL URLs"),
		Entry("MySQL URL", `mysql://root:secret@localhost/db`, true, "MySQL URLs"),
		Entry("MongoDB URL", `mongodb://admin:pass123@localhost:27017/db`, true, "MongoDB URLs"),

		// Certificate patterns
		Entry("PEM certificate", "-----BEGIN CERTIFICATE-----\nMIIC...ABC\n-----END CERTIFICATE-----", true, "PEM certificates"),
		Entry("private key", "-----BEGIN PRIVATE KEY-----\nABC123\n-----END PRIVATE KEY-----", true, "private keys"),

		// Non-sensitive content (should NOT be redacted)
		Entry("normal text", "Deployment failed: image pull error", false, "normal error messages"),
		Entry("URL without creds", "https://example.com/api/v1", false, "URLs without credentials"),
		Entry("environment name", "environment: production", false, "environment variable"),
	)

	Context("real-world notification scenarios", func() {
		It("should sanitize error messages with credentials", func() {
			input := `Failed to connect to PostgreSQL: postgresql://admin:supersecret@localhost:5432/mydb - connection refused`

			result := sanitizer.Sanitize(input)

			Expect(result).To(ContainSubstring("***REDACTED***"))
			Expect(result).ToNot(ContainSubstring("supersecret"))
			Expect(result).To(ContainSubstring("postgresql://admin:"))
			Expect(result).To(ContainSubstring("@localhost:5432/mydb"))
		})

		It("should sanitize Kubernetes Secret YAML", func() {
			input := `
apiVersion: v1
kind: Secret
metadata:
  name: database-credentials
data:
  username: YWRtaW4=
  password: cGFzc3dvcmQxMjM=
`

			result := sanitizer.Sanitize(input)

			Expect(result).To(ContainSubstring("***REDACTED***"))
			Expect(result).ToNot(ContainSubstring("YWRtaW4="))
			Expect(result).ToNot(ContainSubstring("cGFzc3dvcmQxMjM="))
		})

		It("should sanitize API error responses with tokens", func() {
			input := `API call failed: 401 Unauthorized. Token: ghp_abc123def456xyz789ghp_abc123def456 is invalid or expired`

			result := sanitizer.Sanitize(input)

			Expect(result).To(ContainSubstring("***REDACTED***"))
			Expect(result).ToNot(ContainSubstring("ghp_abc123def456"))
		})

		It("should preserve non-sensitive content", func() {
			input := `Deployment failed: image pull error for registry.example.com/app:v1.2.3`

			result := sanitizer.Sanitize(input)

			Expect(result).To(Equal(input)) // Should remain unchanged (no credentials)
		})

		It("should handle multiple secrets in one message", func() {
			input := `Connection failed with password=secret123 and apiKey=xyz789 for user admin`

			result := sanitizer.Sanitize(input)

			Expect(result).To(ContainSubstring("***REDACTED***"))
			Expect(result).ToNot(ContainSubstring("secret123"))
			Expect(result).ToNot(ContainSubstring("xyz789"))
		})

		It("should sanitize OpenAI API keys", func() {
			input := `LLM error: Invalid API key sk-proj-abc123def456ghi789jkl012mno345pqr678`

			result := sanitizer.Sanitize(input)

			Expect(result).To(ContainSubstring("***REDACTED***"))
			Expect(result).ToNot(ContainSubstring("sk-proj-"))
		})
	})

	Context("sanitization behavior verification", func() {
		It("should redact multiple secrets in one message", func() {
			input := `password=secret123 and apiKey=abc789`

			result := sanitizer.Sanitize(input)

			// Observable behavior: secrets are redacted
			Expect(result).To(ContainSubstring("***REDACTED***"))
			Expect(result).ToNot(ContainSubstring("secret123"), "password should be redacted")
			Expect(result).ToNot(ContainSubstring("abc789"), "API key should be redacted")
		})

		It("should not modify clean content", func() {
			input := `This is a clean message with no secrets`

			result := sanitizer.Sanitize(input)

			// Observable behavior: clean content unchanged
			Expect(result).To(Equal(input))
		})
	})
})
