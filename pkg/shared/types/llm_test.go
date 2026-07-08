package types_test

import (
	"os"
	"path/filepath"
	"testing"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/jordigilh/kubernaut/pkg/shared/types"
	"gopkg.in/yaml.v3"
)

func TestLLMConfig(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Shared LLM Config Suite")
}

var _ = Describe("UT-SH-1488-001: LLMConfig YAML deserialization", func() {
	It("should deserialize all fields from YAML", func() {
		input := `
provider: openai_compatible
model: gpt-4o
endpoint: "http://llm-gateway:8080/v1"
apiKeyFile: "/etc/credentials/api_key"
vertexProject: my-project
vertexLocation: us-central1
azureApiVersion: "2024-02-01"
bedrockRegion: us-east-1
temperature: 0.5
maxRetries: 5
timeoutSeconds: 60
tlsCaFile: "/etc/tls/ca.crt"
tlsCertFile: "/etc/tls/tls.crt"
tlsKeyFile: "/etc/tls/tls.key"
oauth2:
  enabled: true
  tokenURL: "https://auth.example.com/token"
  scopes:
    - "llm.read"
  credentialsDir: "/etc/oauth2"
circuitBreaker:
  enabled: true
  maxRequests: 10
  interval: 30s
  timeout: 60s
  failureThreshold: 5
  failureRatio: 0.5
customHeaders:
  - name: X-Custom
    value: static-val
  - name: X-Secret
    secretKeyRef: SECRET_ENV
  - name: X-File
    filePath: "/etc/headers/token"
`
		var cfg types.LLMConfig
		Expect(yaml.Unmarshal([]byte(input), &cfg)).To(Succeed())

		Expect(cfg.Provider).To(Equal("openai_compatible"))
		Expect(cfg.Model).To(Equal("gpt-4o"))
		Expect(cfg.Endpoint).To(Equal("http://llm-gateway:8080/v1"))
		Expect(cfg.APIKeyFile).To(Equal("/etc/credentials/api_key"))
		Expect(cfg.VertexProject).To(Equal("my-project"))
		Expect(cfg.VertexLocation).To(Equal("us-central1"))
		Expect(cfg.AzureAPIVersion).To(Equal("2024-02-01"))
		Expect(cfg.BedrockRegion).To(Equal("us-east-1"))
		Expect(cfg.Temperature).NotTo(BeNil())
		Expect(*cfg.Temperature).To(BeNumerically("~", 0.5, 0.001))
		Expect(cfg.MaxRetries).NotTo(BeNil())
		Expect(*cfg.MaxRetries).To(Equal(5))
		Expect(cfg.TimeoutSeconds).To(Equal(60))
		Expect(cfg.TLSCaFile).To(Equal("/etc/tls/ca.crt"))
		Expect(cfg.TLSCertFile).To(Equal("/etc/tls/tls.crt"))
		Expect(cfg.TLSKeyFile).To(Equal("/etc/tls/tls.key"))

		Expect(cfg.OAuth2.Enabled).To(BeTrue())
		Expect(cfg.OAuth2.TokenURL).To(Equal("https://auth.example.com/token"))
		Expect(cfg.OAuth2.Scopes).To(ConsistOf("llm.read"))
		Expect(cfg.OAuth2.CredentialsDir).To(Equal("/etc/oauth2"))

		Expect(cfg.CircuitBreaker.Enabled).To(BeTrue())
		Expect(cfg.CircuitBreaker.MaxRequests).To(Equal(uint32(10)))
		Expect(cfg.CircuitBreaker.FailureThreshold).To(Equal(uint32(5)))
		Expect(cfg.CircuitBreaker.FailureRatio).To(BeNumerically("~", 0.5, 0.001))

		Expect(cfg.CustomHeaders).To(HaveLen(3))
		Expect(cfg.CustomHeaders[0].Name).To(Equal("X-Custom"))
		Expect(cfg.CustomHeaders[0].Value).To(Equal("static-val"))
		Expect(cfg.CustomHeaders[1].SecretKeyRef).To(Equal("SECRET_ENV"))
		Expect(cfg.CustomHeaders[2].FilePath).To(Equal("/etc/headers/token"))

		// APIKey must not be deserialized from YAML (yaml:"-")
		Expect(cfg.APIKey).To(BeEmpty())
	})
})

var _ = Describe("UT-SH-1488-002: Provider constants", func() {
	It("should define all provider constants matching AF+KA union", func() {
		Expect(types.LLMProviderVertexAI).To(Equal("vertex_ai"))
		Expect(types.LLMProviderGemini).To(Equal("gemini"))
		Expect(types.LLMProviderAnthropic).To(Equal("anthropic"))
		Expect(types.LLMProviderOpenAI).To(Equal("openai"))
		Expect(types.LLMProviderOpenAICompatible).To(Equal("openai_compatible"))
	})
})

var _ = Describe("UT-SH-1488-003: ResolveAPIKey reads file", func() {
	It("should read API key from file and trim whitespace", func() {
		dir := GinkgoT().TempDir()
		keyFile := filepath.Join(dir, "api_key")
		Expect(os.WriteFile(keyFile, []byte("  sk-test-key-123  \n"), 0600)).To(Succeed())

		cfg := types.LLMConfig{APIKeyFile: keyFile}
		Expect(cfg.ResolveAPIKey()).To(Succeed())
		Expect(cfg.APIKey).To(Equal("sk-test-key-123"))
	})
})

var _ = Describe("UT-SH-1488-004: ResolveAPIKey no-op when empty", func() {
	It("should be a no-op when APIKeyFile is empty", func() {
		cfg := types.LLMConfig{}
		Expect(cfg.ResolveAPIKey()).To(Succeed())
		Expect(cfg.APIKey).To(BeEmpty())
	})
})

var _ = Describe("UT-SH-1488-005: Validate accepts vertex_ai", func() {
	It("should accept vertex_ai with required fields", func() {
		cfg := types.LLMConfig{
			Provider:       types.LLMProviderVertexAI,
			Model:          "gemini-2.0-flash",
			VertexProject:  "my-project",
			VertexLocation: "us-central1",
		}
		Expect(cfg.Validate("llm")).To(Succeed())
	})
})

var _ = Describe("UT-SH-1488-006: Validate rejects unknown provider", func() {
	It("should reject unknown provider", func() {
		cfg := types.LLMConfig{
			Provider: "unknown_provider",
			Model:    "some-model",
		}
		err := cfg.Validate("llm")
		Expect(err).To(HaveOccurred())
		Expect(err.Error()).To(ContainSubstring("provider"))
	})

	It("should return nil for empty provider (not configured)", func() {
		cfg := types.LLMConfig{}
		Expect(cfg.Validate("llm")).To(Succeed())
	})
})

var _ = Describe("UT-SH-1488-007: Validate rejects openai without endpoint", func() {
	It("should reject openai without endpoint", func() {
		cfg := types.LLMConfig{
			Provider:   types.LLMProviderOpenAI,
			Model:      "gpt-4o",
			APIKeyFile: "/etc/credentials/key",
		}
		err := cfg.Validate("llm")
		Expect(err).To(HaveOccurred())
		Expect(err.Error()).To(ContainSubstring("endpoint"))
	})

	It("should reject openai_compatible without endpoint", func() {
		cfg := types.LLMConfig{
			Provider: types.LLMProviderOpenAICompatible,
			Model:    "llama3",
		}
		err := cfg.Validate("llm")
		Expect(err).To(HaveOccurred())
		Expect(err.Error()).To(ContainSubstring("endpoint"))
	})
})

var _ = Describe("UT-SH-1488-008: Validate rejects openai without auth", func() {
	It("should reject openai without apiKeyFile or oauth2", func() {
		cfg := types.LLMConfig{
			Provider: types.LLMProviderOpenAI,
			Model:    "gpt-4o",
			Endpoint: "https://api.openai.com/v1",
		}
		err := cfg.Validate("llm")
		Expect(err).To(HaveOccurred())
		Expect(err.Error()).To(ContainSubstring("apiKeyFile"))
	})
})

var _ = Describe("UT-SH-1488-009: Validate accepts openai_compatible keyless", func() {
	It("should accept openai_compatible without apiKeyFile", func() {
		cfg := types.LLMConfig{
			Provider: types.LLMProviderOpenAICompatible,
			Model:    "llama3",
			Endpoint: "http://llamastack:8080/v1",
		}
		Expect(cfg.Validate("llm")).To(Succeed())
	})
})

var _ = Describe("UT-SH-1488-010: YAML round-trip AF format", func() {
	It("should round-trip AF-style config preserving all fields", func() {
		temp := 0.7
		retries := 3
		original := types.LLMConfig{
			Provider:       types.LLMProviderOpenAICompatible,
			Model:          "gpt-4o",
			Endpoint:       "http://llm:8080/v1",
			APIKeyFile:     "/etc/creds/key",
			TimeoutSeconds: 120,
			Temperature:    &temp,
			MaxRetries:     &retries,
			CustomHeaders: []types.LLMHeaderDef{
				{Name: "X-Test", Value: "val"},
			},
		}

		data, err := yaml.Marshal(&original)
		Expect(err).NotTo(HaveOccurred())

		var roundTripped types.LLMConfig
		Expect(yaml.Unmarshal(data, &roundTripped)).To(Succeed())

		Expect(roundTripped.Provider).To(Equal(original.Provider))
		Expect(roundTripped.Model).To(Equal(original.Model))
		Expect(roundTripped.Endpoint).To(Equal(original.Endpoint))
		Expect(roundTripped.APIKeyFile).To(Equal(original.APIKeyFile))
		Expect(roundTripped.TimeoutSeconds).To(Equal(original.TimeoutSeconds))
		Expect(*roundTripped.Temperature).To(BeNumerically("~", *original.Temperature, 0.001))
		Expect(*roundTripped.MaxRetries).To(Equal(*original.MaxRetries))
		Expect(roundTripped.CustomHeaders).To(HaveLen(1))
		// APIKey must not survive round-trip
		Expect(roundTripped.APIKey).To(BeEmpty())
	})
})

var _ = Describe("UT-SH-1488-012: Validate TLS cert pair", func() {
	It("should reject TLS cert without key", func() {
		cfg := types.LLMConfig{
			Provider:    types.LLMProviderOpenAICompatible,
			Model:       "llama3",
			Endpoint:    "https://llm:8080/v1",
			TLSCertFile: "/etc/tls/tls.crt",
		}
		err := cfg.Validate("llm")
		Expect(err).To(HaveOccurred())
		Expect(err.Error()).To(ContainSubstring("tlsCertFile"))
	})

	It("should accept TLS cert+key pair together", func() {
		cfg := types.LLMConfig{
			Provider:    types.LLMProviderOpenAICompatible,
			Model:       "llama3",
			Endpoint:    "https://llm:8080/v1",
			TLSCaFile:   "/etc/tls/ca.crt",
			TLSCertFile: "/etc/tls/tls.crt",
			TLSKeyFile:  "/etc/tls/tls.key",
		}
		Expect(cfg.Validate("llm")).To(Succeed())
	})
})

var _ = Describe("UT-SH-1488-013: Validate OAuth2 constraints", func() {
	It("should reject OAuth2 enabled without tokenURL", func() {
		cfg := types.LLMConfig{
			Provider: types.LLMProviderGemini,
			Model:    "gemini-pro",
			OAuth2: types.LLMOAuth2Config{
				Enabled:        true,
				CredentialsDir: "/etc/oauth2",
			},
		}
		err := cfg.Validate("llm")
		Expect(err).To(HaveOccurred())
		Expect(err.Error()).To(ContainSubstring("tokenURL"))
	})
})

var _ = Describe("UT-SH-1488-015: LLMHeaderDef validation", func() {
	It("should reject header with no value source", func() {
		header := types.LLMHeaderDef{Name: "X-Test"}
		Expect(header.ValidateSource()).To(HaveOccurred())
	})

	It("should reject header with multiple value sources", func() {
		header := types.LLMHeaderDef{
			Name:     "X-Test",
			Value:    "val",
			FilePath: "/etc/token",
		}
		Expect(header.ValidateSource()).To(HaveOccurred())
	})

	It("should accept header with exactly one value source", func() {
		Expect(types.LLMHeaderDef{Name: "X-A", Value: "v"}.ValidateSource()).To(Succeed())
		Expect(types.LLMHeaderDef{Name: "X-B", SecretKeyRef: "ENV"}.ValidateSource()).To(Succeed())
		Expect(types.LLMHeaderDef{Name: "X-C", FilePath: "/f"}.ValidateSource()).To(Succeed())
	})
})

var _ = Describe("UT-SH-AI-086-001: LLMReasoningConfig — BR-AI-086 AC2/AC5", func() {
	It("should leave Reasoning nil by default when omitted from YAML (disabled by default per AC2)", func() {
		input := `
provider: anthropic
model: claude-sonnet-4-6
apiKeyFile: "/etc/credentials/anthropic_key"
`
		var cfg types.LLMConfig
		Expect(yaml.Unmarshal([]byte(input), &cfg)).To(Succeed())
		Expect(cfg.Reasoning).To(BeNil())
	})

	It("should deserialize reasoning fields on LLMConfig from YAML", func() {
		input := `
provider: anthropic
model: claude-sonnet-4-6
apiKeyFile: "/etc/credentials/anthropic_key"
reasoning:
  enabled: true
  budgetTokens: 2048
  capabilityOverride: force_on
`
		var cfg types.LLMConfig
		Expect(yaml.Unmarshal([]byte(input), &cfg)).To(Succeed())
		Expect(cfg.Reasoning).NotTo(BeNil())
		Expect(cfg.Reasoning.Enabled).To(BeTrue())
		Expect(cfg.Reasoning.BudgetTokens).To(Equal(2048))
		Expect(cfg.Reasoning.CapabilityOverride).To(Equal("force_on"))
	})

	It("should round-trip Reasoning through YAML marshal/unmarshal unchanged", func() {
		original := types.LLMConfig{
			Provider: "anthropic",
			Model:    "claude-sonnet-4-6",
			Reasoning: &types.LLMReasoningConfig{
				Enabled:            true,
				BudgetTokens:       1024,
				CapabilityOverride: "force_off",
			},
		}

		data, err := yaml.Marshal(&original)
		Expect(err).NotTo(HaveOccurred())

		var restored types.LLMConfig
		Expect(yaml.Unmarshal(data, &restored)).To(Succeed())

		Expect(restored.Reasoning).NotTo(BeNil())
		Expect(*restored.Reasoning).To(Equal(*original.Reasoning))
	})
})

var _ = Describe("UT-SH-AI-086-016: LLMReasoningConfig.Effort — unified cross-provider effort knob (#1604)", func() {
	It("should deserialize the effort field from YAML", func() {
		input := `
provider: openai
model: gpt-5
endpoint: "https://api.openai.com/v1"
apiKeyFile: "/etc/credentials/openai_key"
reasoning:
  enabled: true
  effort: high
`
		var cfg types.LLMConfig
		Expect(yaml.Unmarshal([]byte(input), &cfg)).To(Succeed())
		Expect(cfg.Reasoning).NotTo(BeNil())
		Expect(cfg.Reasoning.Effort).To(Equal("high"))
	})

	It("should default Effort to empty (vendor default applies) when omitted", func() {
		input := `
provider: openai
model: gpt-5
endpoint: "https://api.openai.com/v1"
apiKeyFile: "/etc/credentials/openai_key"
reasoning:
  enabled: true
`
		var cfg types.LLMConfig
		Expect(yaml.Unmarshal([]byte(input), &cfg)).To(Succeed())
		Expect(cfg.Reasoning.Effort).To(BeEmpty())
	})

	DescribeTable("Validate should accept every canonical effort value for a non-Anthropic provider",
		func(effort string) {
			cfg := types.LLMConfig{
				Provider:   types.LLMProviderOpenAI,
				Model:      "gpt-5",
				Endpoint:   "https://api.openai.com/v1",
				APIKeyFile: "/etc/credentials/openai_key",
				Reasoning:  &types.LLMReasoningConfig{Enabled: true, Effort: effort},
			}
			Expect(cfg.Validate("llm")).To(Succeed())
		},
		Entry("empty (vendor default)", ""),
		Entry("none", "none"),
		Entry("minimal", "minimal"),
		Entry("low", "low"),
		Entry("medium", "medium"),
		Entry("high", "high"),
		Entry("xhigh", "xhigh"),
	)

	It("should reject an unrecognized effort value", func() {
		cfg := types.LLMConfig{
			Provider:   types.LLMProviderOpenAI,
			Model:      "gpt-5",
			Endpoint:   "https://api.openai.com/v1",
			APIKeyFile: "/etc/credentials/openai_key",
			Reasoning:  &types.LLMReasoningConfig{Enabled: true, Effort: "extreme"},
		}
		err := cfg.Validate("llm")
		Expect(err).To(HaveOccurred())
		Expect(err.Error()).To(ContainSubstring("effort"))
	})

	DescribeTable("Validate should fail closed on effort: none for Anthropic-family providers while enabled",
		func(provider string) {
			cfg := types.LLMConfig{
				Provider:       provider,
				Model:          "claude-sonnet-4-6",
				APIKeyFile:     "/etc/credentials/anthropic_key",
				VertexProject:  "my-project",
				VertexLocation: "us-central1",
				Reasoning:      &types.LLMReasoningConfig{Enabled: true, Effort: "none"},
			}
			err := cfg.Validate("llm")
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("effort: none"))
			Expect(err.Error()).To(ContainSubstring("enabled: false"))
		},
		Entry("anthropic (native)", types.LLMProviderAnthropic),
		Entry("vertex_ai (Claude-on-Vertex)", types.LLMProviderVertexAI),
	)

	It("should accept effort: none for Anthropic when reasoning is disabled (no contradiction)", func() {
		cfg := types.LLMConfig{
			Provider:   types.LLMProviderAnthropic,
			Model:      "claude-sonnet-4-6",
			APIKeyFile: "/etc/credentials/anthropic_key",
			Reasoning:  &types.LLMReasoningConfig{Enabled: false, Effort: "none"},
		}
		Expect(cfg.Validate("llm")).To(Succeed())
	})

	It("should accept effort: xhigh for Anthropic (a ceiling clamp, not a contradiction)", func() {
		cfg := types.LLMConfig{
			Provider:   types.LLMProviderAnthropic,
			Model:      "claude-sonnet-4-6",
			APIKeyFile: "/etc/credentials/anthropic_key",
			Reasoning:  &types.LLMReasoningConfig{Enabled: true, Effort: "xhigh"},
		}
		Expect(cfg.Validate("llm")).To(Succeed())
	})

	It("should round-trip Effort through YAML marshal/unmarshal unchanged", func() {
		original := types.LLMConfig{
			Provider: "openai_compatible",
			Model:    "deepseek-v4-pro",
			Endpoint: "https://api.deepseek.com",
			Reasoning: &types.LLMReasoningConfig{
				Enabled: true,
				Effort:  "high",
			},
		}

		data, err := yaml.Marshal(&original)
		Expect(err).NotTo(HaveOccurred())

		var restored types.LLMConfig
		Expect(yaml.Unmarshal(data, &restored)).To(Succeed())

		Expect(restored.Reasoning).NotTo(BeNil())
		Expect(*restored.Reasoning).To(Equal(*original.Reasoning))
	})
})
