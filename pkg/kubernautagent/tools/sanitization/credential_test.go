package sanitization_test

import (
	"context"

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
			input := `mysql://root:hunter2@mysql-host:3306/app` // notsecret
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
			input := `Authorization: Bearer eyJhbGciOiJSUzI1NiIsInR5cCI6IkpXVCJ9.eyJpc3MiOiJrdWJlcm5ldGVzLyJ9.signature` // notsecret
			result, err := stage.Sanitize(ctx, input)
			Expect(err).NotTo(HaveOccurred())
			Expect(result).NotTo(ContainSubstring("eyJhbGciOiJSUzI1NiI"))
			Expect(result).To(ContainSubstring("[REDACTED]"))
		})
	})

	Describe("UT-KA-433-051: Covers all 17 BR-KA-211/DD-005 pattern categories", func() {
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
			Entry("password-url", `postgres://user:urlpass@host`, "urlpass"), // notsecret
			Entry("api-key-json", `{"api_key":"key-abc-123-xyz"}`, "key-abc-123-xyz"),
			Entry("api-key-plain", `apikey=sk-live-test123`, "sk-live-test123"),
			Entry("openai-key", `key is sk-proj-Abc123Def456Ghi`, "sk-proj-Abc123Def456Ghi"), // pre-commit:allow-sensitive
			Entry("bearer-token", `Bearer eyJhbGciOiJIUzI1NiJ9.payload.sig`, "eyJhbGciOiJIUzI1NiJ9"),
			Entry("github-token", `token: ghp_ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghij`, "ghp_ABCDEFGHIJKLMNOPQRSTUVWXYZ"), // notsecret pre-commit:allow-sensitive
			Entry("token-json", `{"token":"tok_abc123xyz"}`, "tok_abc123xyz"),
			Entry("secret-json", `{"client_secret":"cs_live_abc"}`, "cs_live_abc"),
			Entry("secret-plain", `client_secret=mysecretvalue`, "mysecretvalue"),
			Entry("authorization-header", `authorization: Basic dXNlcjpwYXNz`, "dXNlcjpwYXNz"),
			Entry("aws-access-key", `AWS_ACCESS_KEY_ID=AKIAIOSFODNN7EXAMPLE`, "AKIAIOSFODNN7EXAMPLE"), // pre-commit:allow-sensitive
			Entry("postgresql-url", `postgresql://admin:dbpass@host:5432`, "dbpass"),
			Entry("redis-url", `redis://user:rpass@host:6379`, "rpass"),
			Entry("private-key", "-----BEGIN PRIVATE KEY-----\nMIIEv...\n-----END PRIVATE KEY-----", "MIIEv"),
			Entry("password-plain-base64-shaped", "  password: c2VjcmV0MTIz\n", "c2VjcmV0MTIz"),
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

	Describe("UT-KA-2485-001 (BR-KA-211 FR-1/FR-4): Preserves non-Secret Kubernetes identifiers", func() {
		DescribeTable("should not apply base64-shaped Secret heuristics to Kubernetes metadata",
			func(input string) {
				result, err := stage.Sanitize(ctx, input)
				Expect(err).NotTo(HaveOccurred())
				Expect(result).To(Equal(input),
					"non-Secret Kubernetes metadata must remain unchanged")
			},
			Entry("Node taint key", "spec:\n  taints:\n  - effect: NoSchedule\n    key: maintenance\n    value: scheduled\n"),
			Entry("Pod toleration key", "spec:\n  tolerations:\n  - effect: NoSchedule\n    key: dedicated\n"),
			Entry("node affinity match expression key", "spec:\n  affinity:\n    nodeAffinity:\n      requiredDuringSchedulingIgnoredDuringExecution:\n        nodeSelectorTerms:\n        - matchExpressions:\n          - operator: In\n            key: maintenance\n"),
			Entry("label selector match expression key", "spec:\n  selector:\n    matchExpressions:\n    - operator: In\n      key: maintenance\n"),
			Entry("SecretKeySelector reference key", "spec:\n  containers:\n  - name: app\n    env:\n    - name: VALUE\n      valueFrom:\n        secretKeyRef:\n          name: app-credentials\n          key: password\n"),
			Entry("ConfigMap data entry named key", "apiVersion: v1\nkind: ConfigMap\ndata:\n  key: maintenance\n"),
			Entry("TokenReview identity username", "apiVersion: authentication.k8s.io/v1\nkind: TokenReview\nstatus:\n  user:\n    username: maintenance\n"),
			Entry("CertificateSigningRequest identity username", "apiVersion: certificates.k8s.io/v1\nkind: CertificateSigningRequest\nspec:\n  username: maintenance\n"),
		)
	})

	Describe("UT-KA-2485-004 (BR-KA-211 FR-1/FR-2): Preserves structured Secret references", func() {
		It("should not consume a nested Secret volume mapping as a credential value", func() {
			input := "spec:\n  volumes:\n  - name: app-creds\n    secret:\n      secretName: app-credentials\n      defaultMode: 420\n"

			result, err := stage.Sanitize(ctx, input)
			Expect(err).NotTo(HaveOccurred())
			Expect(result).To(Equal(input))
		})

		It("should continue to redact a credential-bearing TokenReview token", func() {
			input := "apiVersion: authentication.k8s.io/v1\nkind: TokenReview\nspec:\n  token: dG9rZW4tc2VudGluZWw=\n"

			result, err := stage.Sanitize(ctx, input)
			Expect(err).NotTo(HaveOccurred())
			Expect(result).NotTo(ContainSubstring("dG9rZW4tc2VudGluZWw="))
			Expect(result).To(ContainSubstring("[REDACTED]"))
		})
	})

	Describe("UT-KA-2485-006 (BR-KA-211 FR-1/FR-4): Preserves additional Kubernetes metadata keys", func() {
		DescribeTable("should not apply the Kubernetes Secret-data heuristic to identifiers",
			func(input string) {
				result, err := stage.Sanitize(ctx, input)
				Expect(err).NotTo(HaveOccurred())
				Expect(result).To(Equal(input),
					"Kubernetes identifiers must remain unchanged")
			},
			Entry("topology spread topologyKey", "spec:\n  topologySpreadConstraints:\n  - maxSkew: 1\n    topologyKey: topology.kubernetes.io/zone\n    whenUnsatisfiable: DoNotSchedule\n"),
			Entry("metadata label literally named key", "metadata:\n  labels:\n    key: maintenance\n"),
			Entry("metadata annotation literally named key", "metadata:\n  annotations:\n    key: maintenance\n"),
			Entry("ConfigMapKeySelector key", "spec:\n  containers:\n  - name: app\n    env:\n    - name: VALUE\n      valueFrom:\n        configMapKeyRef:\n          name: app-config\n          key: password\n"),
			Entry("Secret volume item key", "spec:\n  volumes:\n  - name: app-creds\n    secret:\n      secretName: app-credentials\n      items:\n      - key: password\n        path: password\n"),
			Entry("projected Secret item key", "spec:\n  volumes:\n  - name: projected-creds\n    projected:\n      sources:\n      - secret:\n          name: app-credentials\n          items:\n          - key: password\n            path: password\n"),
			Entry("projected ConfigMap item key", "spec:\n  volumes:\n  - name: projected-config\n    projected:\n      sources:\n      - configMap:\n          name: app-config\n          items:\n          - key: maintenance\n            path: maintenance\n"),
			Entry("SelfSubjectReview identity username", "apiVersion: authentication.k8s.io/v1\nkind: SelfSubjectReview\nstatus:\n  userInfo:\n    username: maintenance\n"),
		)
	})

})
