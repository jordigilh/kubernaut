package sanitization_test

import (
	"context"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/jordigilh/kubernaut/pkg/kubernautagent/tools/sanitization"
)

var _ = Describe("Kubernaut Agent G4 Credential Scrubbing — #433", func() {

	var (
		stage sanitization.Stage
		ctx   context.Context
	)

	BeforeEach(func() {
		stage = sanitization.NewCredentialSanitizer()
		ctx = context.Background()
	})

	Describe("UT-KA-433-048: Scrubs database URL patterns", func() {
		It("should scrub postgres:// URLs", func() {
			input := `Connection error: postgresql://admin:s3cr3tP@ss@db-host:5432/mydb`
			result, err := stage.Sanitize(ctx, input)
			Expect(err).NotTo(HaveOccurred())
			Expect(result).NotTo(ContainSubstring("s3cr3tP@ss"))
			Expect(result).To(ContainSubstring("[REDACTED]"))
			Expect(result).To(ContainSubstring("db-host"))
		})

		It("should scrub mysql:// URLs", func() {
			input := `mysql://root:hunter2@mysql-host:3306/app`
			result, err := stage.Sanitize(ctx, input)
			Expect(err).NotTo(HaveOccurred())
			Expect(result).NotTo(ContainSubstring("hunter2"))
			Expect(result).To(ContainSubstring("[REDACTED]"))
		})

		It("should scrub redis:// URLs", func() {
			input := `redis://default:redispass@cache:6379/0`
			result, err := stage.Sanitize(ctx, input)
			Expect(err).NotTo(HaveOccurred())
			Expect(result).NotTo(ContainSubstring("redispass"))
		})
	})

	Describe("UT-KA-433-049: Scrubs API key patterns", func() {
		It("should scrub OpenAI API keys (sk-...)", func() {
			input := `Config loaded: api_key=sk-proj-abc123def456ghi789jkl012` // pre-commit:allow-sensitive
			result, err := stage.Sanitize(ctx, input)
			Expect(err).NotTo(HaveOccurred())
			Expect(result).NotTo(ContainSubstring("sk-proj-abc123")) // pre-commit:allow-sensitive
			Expect(result).To(ContainSubstring("[REDACTED]"))
		})

		It("should scrub generic api_key fields", func() {
			input := `{"api_key": "my-secret-api-key-12345"}`
			result, err := stage.Sanitize(ctx, input)
			Expect(err).NotTo(HaveOccurred())
			Expect(result).NotTo(ContainSubstring("my-secret-api-key-12345"))
		})
	})

	Describe("UT-KA-433-050: Scrubs bearer token patterns", func() {
		It("should scrub Bearer tokens", func() {
			input := `Authorization: Bearer eyJhbGciOiJSUzI1NiIsInR5cCI6IkpXVCJ9.eyJpc3MiOiJrdWJlcm5ldGVzLyJ9.signature`
			result, err := stage.Sanitize(ctx, input)
			Expect(err).NotTo(HaveOccurred())
			Expect(result).NotTo(ContainSubstring("eyJhbGciOiJSUzI1NiI"))
			Expect(result).To(ContainSubstring("[REDACTED]"))
		})
	})

	Describe("UT-KA-433-051: Covers all 17 BR-HAPI-211/DD-005 pattern categories", func() {
		DescribeTable("should scrub each credential category",
			func(input, mustNotContain string) {
				result, err := stage.Sanitize(ctx, input)
				Expect(err).NotTo(HaveOccurred())
				Expect(result).NotTo(ContainSubstring(mustNotContain),
					"credential should be scrubbed: %s", mustNotContain)
				Expect(result).To(ContainSubstring("[REDACTED]"))
			},
			Entry("password-json", `{"password":"supersecret123"}`, "supersecret123"),
			Entry("password-plain", `password=mysecretpwd`, "mysecretpwd"),
			Entry("password-url", `postgres://user:urlpass@host`, "urlpass"),
			Entry("api-key-json", `{"api_key":"key-abc-123-xyz"}`, "key-abc-123-xyz"),
			Entry("api-key-plain", `apikey=sk-live-test123`, "sk-live-test123"),
			Entry("openai-key", `key is sk-proj-Abc123Def456Ghi`, "sk-proj-Abc123Def456Ghi"), // pre-commit:allow-sensitive
			Entry("bearer-token", `Bearer eyJhbGciOiJIUzI1NiJ9.payload.sig`, "eyJhbGciOiJIUzI1NiJ9"),
			Entry("github-token", `token: ghp_ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghij`, "ghp_ABCDEFGHIJKLMNOPQRSTUVWXYZ"), // pre-commit:allow-sensitive
			Entry("token-json", `{"token":"tok_abc123xyz"}`, "tok_abc123xyz"),
			Entry("secret-json", `{"client_secret":"cs_live_abc"}`, "cs_live_abc"),
			Entry("secret-plain", `client_secret=mysecretvalue`, "mysecretvalue"),
			Entry("authorization-header", `authorization: Basic dXNlcjpwYXNz`, "dXNlcjpwYXNz"),
			Entry("aws-access-key", `AWS_ACCESS_KEY_ID=AKIAIOSFODNN7EXAMPLE`, "AKIAIOSFODNN7EXAMPLE"), // pre-commit:allow-sensitive
			Entry("postgresql-url", `postgresql://admin:dbpass@host:5432`, "dbpass"),
			Entry("redis-url", `redis://user:rpass@host:6379`, "rpass"),
			Entry("private-key", "-----BEGIN PRIVATE KEY-----\nMIIEv...\n-----END PRIVATE KEY-----", "MIIEv"),
			Entry("k8s-secret-data", "  password: c2VjcmV0MTIz\n", "c2VjcmV0MTIz"),
		)
	})

	Describe("UT-KA-433-052: Preserves non-credential content unchanged", func() {
		It("should not modify normal log lines", func() {
			input := `2026-03-04T10:00:00Z INFO Pod web-abc123 started successfully in namespace production.
Container ready after 5s. Memory limit: 256Mi, CPU limit: 500m.
Events: Normal Scheduled, Normal Pulled, Normal Created, Normal Started.`
			result, err := stage.Sanitize(ctx, input)
			Expect(err).NotTo(HaveOccurred())
			Expect(result).To(Equal(input), "non-credential content must not be modified")
		})

		It("should not scrub the word 'password' without a value", func() {
			input := `The error indicates a password authentication failure for the user.`
			result, err := stage.Sanitize(ctx, input)
			Expect(err).NotTo(HaveOccurred())
			Expect(result).To(ContainSubstring("password authentication failure"))
		})
	})

	Describe("UT-KA-433-053: Single-call scrubbing latency < 10ms", func() {
		It("should complete G4 scrubbing in under 10ms per call", func() {
			input := "Connection: postgresql://admin:s3cr3t@host:5432/db\n" +
				`{"password":"abc","api_key":"sk-proj-xyz","token":"eyJ..."}` + "\n" +
				"Bearer eyJhbGciOiJIUzI1NiJ9.payload.sig\n" +
				"AWS_ACCESS_KEY_ID=AKIAIOSFODNN7EXAMPLE\n" +   // pre-commit:allow-sensitive
				"ghp_ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghij" // pre-commit:allow-sensitive

			start := time.Now()
			iterations := 100
			for i := 0; i < iterations; i++ {
				_, err := stage.Sanitize(ctx, input)
				Expect(err).NotTo(HaveOccurred())
			}
			elapsed := time.Since(start)
			avgLatency := elapsed / time.Duration(iterations)
			Expect(avgLatency).To(BeNumerically("<", 10*time.Millisecond),
				"average G4 latency should be under 10ms, got %v", avgLatency)
		})
	})
})
