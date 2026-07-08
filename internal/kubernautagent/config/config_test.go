/*
Copyright 2026 Jordi Gil.

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

package config_test

import (
	"os"
	"path/filepath"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/jordigilh/kubernaut/internal/kubernautagent/config"
	"github.com/jordigilh/kubernaut/pkg/shared/types"
)

var _ = Describe("Kubernaut Agent Configuration — #433", func() {

	Describe("UT-KA-433-001: Kubernaut Agent loads valid YAML configuration", func() {
		It("should parse all required fields from valid YAML", func() {
			yaml := []byte(`
ai:
  llm:
    provider: "openai"
  investigation:
    maxTurns: 15
runtime:
  server:
    address: "0.0.0.0"
    port: 8080
  session:
    ttl: 30m
  audit:
    enabled: true
`)
			cfg, err := config.Load(yaml)
			Expect(err).NotTo(HaveOccurred())
			Expect(cfg).NotTo(BeNil())
			Expect(cfg.AI.LLM.Provider).To(Equal("openai"))
			Expect(cfg.Runtime.Server.Port).To(Equal(8080))
			Expect(cfg.Runtime.Session.TTL).To(Equal(30 * time.Minute))
			Expect(cfg.Runtime.Audit.Enabled).To(BeTrue())
			Expect(cfg.AI.Investigation.MaxTurns).To(Equal(15))
		})
	})

	Describe("UT-KA-433-002: Kubernaut Agent applies correct defaults", func() {
		It("should fill defaults when optional fields are omitted", func() {
			yaml := []byte(`
ai:
  llm:
    endpoint: "http://localhost:11434/v1"
    model: "llama3"
`)
			cfg, err := config.Load(yaml)
			Expect(err).NotTo(HaveOccurred())
			Expect(cfg).NotTo(BeNil())

			Expect(cfg.Runtime.Server.Port).To(Equal(8443), "default port should be 8443")
			Expect(cfg.Runtime.Session.TTL).To(Equal(30*time.Minute), "default TTL should be 30m")
			Expect(cfg.AI.Investigation.MaxTurns).To(Equal(40), "default max turns should be 40")
			Expect(cfg.AI.Safety.Anomaly.MaxToolCallsPerTool).To(Equal(10), "UT-KA-860-006: default per-tool limit raised to 10 per #860")
			Expect(cfg.AI.Safety.Anomaly.MaxTotalToolCalls).To(Equal(30), "default total tool calls should be 30")
			Expect(cfg.AI.Safety.Anomaly.MaxRepeatedFailures).To(Equal(3), "default repeated failures should be 3")
		})
	})

	Describe("UT-KA-433-003: Kubernaut Agent rejects invalid config at startup", func() {
		It("should reject missing LLM endpoint for non-exempt providers (runtime validation)", func() {
			rtYAML := []byte(`
model: "mistral-large"
`)
			rt, err := config.LoadLLMRuntime(rtYAML)
			Expect(err).NotTo(HaveOccurred())
			err = rt.Validate("mistral")
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("endpoint"))
		})

		It("should accept empty LLM endpoint for openai (LangChainGo uses default)", func() {
			rtYAML := []byte(`
model: "gpt-4o"
`)
			rt, err := config.LoadLLMRuntime(rtYAML)
			Expect(err).NotTo(HaveOccurred())
			Expect(rt.Validate("openai")).NotTo(HaveOccurred())
		})

		It("should reject invalid max-turns (zero)", func() {
			yaml := []byte(`
ai:
  llm:
    provider: "openai"
  investigation:
    maxTurns: 0
`)
			cfg, err := config.Load(yaml)
			if err == nil && cfg != nil {
				err = cfg.Validate()
			}
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("ai.investigation.maxTurns"))
		})

		It("should reject negative max-turns", func() {
			yaml := []byte(`
ai:
  llm:
    endpoint: "http://localhost:11434/v1"
    model: "llama3"
  investigation:
    maxTurns: -5
`)
			cfg, err := config.Load(yaml)
			if err == nil && cfg != nil {
				err = cfg.Validate()
			}
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("ai.investigation.maxTurns"))
		})

		It("UT-KA-950-001: should reject zero maxToolOutputSize", func() {
			cfg := config.DefaultConfig()
			cfg.AI.Summarizer.MaxToolOutputSize = 0
			err := cfg.Validate()
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("maxToolOutputSize"))
		})

		It("UT-KA-950-002: should reject negative maxToolOutputSize", func() {
			cfg := config.DefaultConfig()
			cfg.AI.Summarizer.MaxToolOutputSize = -1
			err := cfg.Validate()
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("maxToolOutputSize"))
		})

		It("should reject missing LLM model in runtime config", func() {
			rtYAML := []byte(`
endpoint: "http://localhost:11434/v1"
`)
			rt, err := config.LoadLLMRuntime(rtYAML)
			Expect(err).NotTo(HaveOccurred())
			err = rt.Validate("openai")
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("model"))
		})
	})

	Describe("UT-KA-SO-CFG-002: customHeaders parsed from LLM runtime config", func() {
		It("should parse customHeaders from runtime YAML", func() {
			rtYAML := []byte(`
model: "gpt-4o"
customHeaders:
  - name: "X-Custom-Auth"
    value: "Bearer token123"
  - name: "X-Org-Id"
    value: "org-42"
`)
			rt, err := config.LoadLLMRuntime(rtYAML)
			Expect(err).NotTo(HaveOccurred())
			Expect(rt.CustomHeaders).To(HaveLen(2))
			Expect(rt.CustomHeaders[0].Name).To(Equal("X-Custom-Auth"))
			Expect(rt.CustomHeaders[0].Value).To(Equal("Bearer token123"))
			Expect(rt.CustomHeaders[1].Name).To(Equal("X-Org-Id"))
		})

		It("should default customHeaders to nil when omitted", func() {
			rtYAML := []byte(`
model: "gpt-4o"
`)
			rt, err := config.LoadLLMRuntime(rtYAML)
			Expect(err).NotTo(HaveOccurred())
			Expect(rt.CustomHeaders).To(BeEmpty())
		})
	})

	Describe("UT-KA-433W-010: DefaultConfig applies summarizer threshold", func() {
		It("should set Summarizer.Threshold to 8000", func() {
			cfg := config.DefaultConfig()
			Expect(cfg.AI.Summarizer.Threshold).To(Equal(8000))
		})
	})

	Describe("UT-KA-752-010: MaxToolOutputSize parsed from YAML configuration", func() {
		It("should parse max_tool_output_size from summarizer config", func() {
			yaml := []byte(`
ai:
  llm:
    endpoint: "http://localhost:11434/v1"
    model: "llama3"
  summarizer:
    threshold: 8000
    maxToolOutputSize: 50000
`)
			cfg, err := config.Load(yaml)
			Expect(err).NotTo(HaveOccurred())
			Expect(cfg.AI.Summarizer.MaxToolOutputSize).To(Equal(50000))
		})
	})

	Describe("UT-KA-752-011: MaxToolOutputSize default applied when absent", func() {
		It("should default MaxToolOutputSize to DefaultMaxToolOutputSize when not specified", func() {
			cfg := config.DefaultConfig()
			Expect(cfg.AI.Summarizer.MaxToolOutputSize).To(Equal(config.DefaultMaxToolOutputSize),
				"default MaxToolOutputSize should match DefaultMaxToolOutputSize constant")
		})
	})

	Describe("UT-KA-433W-011: DefaultConfig applies anomaly thresholds", func() {
		It("should set MaxToolCallsPerTool=10, MaxTotalToolCalls=30, MaxRepeatedFailures=3 (#860)", func() {
			cfg := config.DefaultConfig()
			Expect(cfg.AI.Safety.Anomaly.MaxToolCallsPerTool).To(Equal(10))
			Expect(cfg.AI.Safety.Anomaly.MaxTotalToolCalls).To(Equal(30))
			Expect(cfg.AI.Safety.Anomaly.MaxRepeatedFailures).To(Equal(3))
		})
	})

	Describe("Consolidated Config (SDK removed)", func() {

		Describe("UT-KA-CFG-CON-001: OAuth2 parsed from main config", func() {
			It("should parse OAuth2 enabled/tokenURL/scopes from main YAML", func() {
				cfgYAML := []byte(`
ai:
  llm:
    provider: "anthropic"
    model: "claude-sonnet-4-20250514"
    oauth2:
      enabled: true
      tokenURL: "https://keycloak.acme.com/realms/infra/protocol/openid-connect/token"
      credentialsDir: "/etc/kubernaut-agent/oauth2"
      scopes:
        - "openid"
        - "llm-gateway"
`)
				cfg, err := config.Load(cfgYAML)
				Expect(err).NotTo(HaveOccurred())
				Expect(cfg.AI.LLM.OAuth2.Enabled).To(BeTrue())
				Expect(cfg.AI.LLM.OAuth2.TokenURL).To(Equal("https://keycloak.acme.com/realms/infra/protocol/openid-connect/token"))
				Expect(cfg.AI.LLM.OAuth2.CredentialsDir).To(Equal("/etc/kubernaut-agent/oauth2"))
				Expect(cfg.AI.LLM.OAuth2.Scopes).To(Equal([]string{"openid", "llm-gateway"}))
			})
		})

		Describe("UT-KA-CFG-CON-003: customHeaders parsed from LLM runtime config", func() {
			It("should populate LLMRuntimeConfig.CustomHeaders from runtime YAML", func() {
				rtYAML := []byte(`
model: "claude-sonnet-4-20250514"
customHeaders:
  - name: "X-Custom-Auth"
    value: "Bearer token123"
  - name: "X-Org-Id"
    value: "org-42"
`)
				rt, err := config.LoadLLMRuntime(rtYAML)
				Expect(err).NotTo(HaveOccurred())
				Expect(rt.CustomHeaders).To(HaveLen(2))
				Expect(rt.CustomHeaders[0].Name).To(Equal("X-Custom-Auth"))
				Expect(rt.CustomHeaders[0].Value).To(Equal("Bearer token123"))
				Expect(rt.CustomHeaders[1].Name).To(Equal("X-Org-Id"))
				Expect(rt.CustomHeaders[1].Value).To(Equal("org-42"))
			})
		})
	})

	Describe("UT-KA-417-020: OAuth2Config parsed from main config", func() {
		It("should parse all OAuth2 fields from main YAML", func() {
			cfgYAML := []byte(`
ai:
  llm:
    provider: "anthropic"
    model: "claude-sonnet-4-20250514"
    oauth2:
      enabled: true
      tokenURL: "https://keycloak.acme.com/realms/infra/protocol/openid-connect/token"
      credentialsDir: "/etc/kubernaut-agent/oauth2"
      scopes:
        - "openid"
        - "llm-gateway"
`)
			cfg, err := config.Load(cfgYAML)
			Expect(err).NotTo(HaveOccurred())
			Expect(cfg.AI.LLM.OAuth2.Enabled).To(BeTrue())
			Expect(cfg.AI.LLM.OAuth2.TokenURL).To(Equal("https://keycloak.acme.com/realms/infra/protocol/openid-connect/token"))
			Expect(cfg.AI.LLM.OAuth2.CredentialsDir).To(Equal("/etc/kubernaut-agent/oauth2"))
			Expect(cfg.AI.LLM.OAuth2.Scopes).To(Equal([]string{"openid", "llm-gateway"}))
			// clientID/clientSecret are resolved from files at runtime, not YAML
			Expect(cfg.AI.LLM.OAuth2.ClientID).To(BeEmpty())
			Expect(cfg.AI.LLM.OAuth2.ClientSecret).To(BeEmpty())
		})
	})

	Describe("UT-KA-417-021: OAuth2Config defaults to disabled when omitted", func() {
		It("should have OAuth2 disabled with empty fields", func() {
			cfgYAML := []byte(`
ai:
  llm:
    provider: "openai"
    model: "gpt-4o"
`)
			cfg, err := config.Load(cfgYAML)
			Expect(err).NotTo(HaveOccurred())
			Expect(cfg.AI.LLM.OAuth2.Enabled).To(BeFalse())
			Expect(cfg.AI.LLM.OAuth2.TokenURL).To(BeEmpty())
			Expect(cfg.AI.LLM.OAuth2.CredentialsDir).To(BeEmpty())
			Expect(cfg.AI.LLM.OAuth2.Scopes).To(BeNil())
		})
	})

	Describe("UT-KA-417-022: Validate rejects missing tokenURL when oauth2 enabled", func() {
		It("should return error identifying missing tokenURL", func() {
			cfgYAML := []byte(`
ai:
  llm:
    provider: "anthropic"
    model: "claude-sonnet-4-20250514"
    oauth2:
      enabled: true
      credentialsDir: "/etc/kubernaut-agent/oauth2"
`)
			cfg, err := config.Load(cfgYAML)
			Expect(err).NotTo(HaveOccurred())
			err = cfg.Validate()
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("tokenURL"))
		})
	})

	Describe("UT-KA-417-023: Validate rejects missing credentialsDir when oauth2 enabled", func() {
		It("should return error identifying missing credentialsDir", func() {
			cfgYAML := []byte(`
ai:
  llm:
    provider: "anthropic"
    model: "claude-sonnet-4-20250514"
    oauth2:
      enabled: true
      tokenURL: "https://keycloak.acme.com/token"
`)
			cfg, err := config.Load(cfgYAML)
			Expect(err).NotTo(HaveOccurred())
			err = cfg.Validate()
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("credentialsDir"))
		})
	})

	Describe("UT-KA-417-025: Validate accepts oauth2 disabled with empty fields", func() {
		It("should not require OAuth2 fields when disabled", func() {
			cfgYAML := []byte(`
ai:
  llm:
    provider: "anthropic"
    model: "claude-sonnet-4-20250514"
    oauth2:
      enabled: false
`)
			cfg, err := config.Load(cfgYAML)
			Expect(err).NotTo(HaveOccurred())
			Expect(cfg.Validate()).NotTo(HaveOccurred())
		})

		It("should not require OAuth2 fields when section omitted", func() {
			cfgYAML := []byte(`
ai:
  llm:
    provider: "anthropic"
    model: "claude-sonnet-4-20250514"
`)
			cfg, err := config.Load(cfgYAML)
			Expect(err).NotTo(HaveOccurred())
			Expect(cfg.Validate()).NotTo(HaveOccurred())
		})
	})

	Describe("UT-KA-823-CFG-001: DefaultConfig includes rate limit defaults", func() {
		It("should set rate limit defaults matching production values", func() {
			cfg := config.DefaultConfig()
			Expect(cfg.Runtime.Server.RateLimit.RequestsPerSecond).To(Equal(5.0))
			Expect(cfg.Runtime.Server.RateLimit.Burst).To(Equal(10))
			Expect(cfg.Runtime.Server.RateLimit.CleanupInterval).To(Equal(5 * time.Minute))
			Expect(cfg.Runtime.Server.RateLimit.MaxAge).To(Equal(10 * time.Minute))
		})
	})

	Describe("UT-KA-823-CFG-002: Validate rejects invalid rate limit values", func() {
		It("should reject zero requestsPerSecond", func() {
			yaml := []byte(`
runtime:
  server:
    rateLimit:
      requestsPerSecond: 0
      burst: 10
ai:
  llm:
    provider: "openai"
  investigation:
    maxTurns: 40
`)
			cfg, err := config.Load(yaml)
			Expect(err).NotTo(HaveOccurred())
			Expect(cfg.Validate()).To(MatchError(ContainSubstring("runtime.server.rateLimit.requestsPerSecond must be positive")))
		})

		It("should reject zero burst", func() {
			yaml := []byte(`
runtime:
  server:
    rateLimit:
      requestsPerSecond: 5
      burst: 0
ai:
  llm:
    provider: "openai"
  investigation:
    maxTurns: 40
`)
			cfg, err := config.Load(yaml)
			Expect(err).NotTo(HaveOccurred())
			Expect(cfg.Validate()).To(MatchError(ContainSubstring("runtime.server.rateLimit.burst must be positive")))
		})
	})

	Describe("UT-KA-823-CFG-003: Rate limit config parsed from YAML", func() {
		It("should deserialize custom rate limit values", func() {
			yaml := []byte(`
runtime:
  server:
    rateLimit:
      requestsPerSecond: 20
      burst: 50
      cleanupInterval: 10m
      maxAge: 30m
ai:
  llm:
    provider: "openai"
  investigation:
    maxTurns: 40
`)
			cfg, err := config.Load(yaml)
			Expect(err).NotTo(HaveOccurred())
			Expect(cfg.Runtime.Server.RateLimit.RequestsPerSecond).To(Equal(20.0))
			Expect(cfg.Runtime.Server.RateLimit.Burst).To(Equal(50))
			Expect(cfg.Runtime.Server.RateLimit.CleanupInterval).To(Equal(10 * time.Minute))
			Expect(cfg.Runtime.Server.RateLimit.MaxAge).To(Equal(30 * time.Minute))
			Expect(cfg.Validate()).NotTo(HaveOccurred())
		})
	})

	Describe("UT-KA-CFG-OAUTH2-RESOLVE: ResolveOAuth2Credentials reads from mounted Secret files", func() {
		It("should resolve clientID and clientSecret from files", func() {
			dir, err := os.MkdirTemp("", "oauth2-test")
			Expect(err).NotTo(HaveOccurred())
			defer func() { _ = os.RemoveAll(dir) }()

			Expect(os.WriteFile(filepath.Join(dir, "client-id"), []byte("my-client\n"), 0600)).To(Succeed())
			Expect(os.WriteFile(filepath.Join(dir, "client-secret"), []byte("s3cret\n"), 0600)).To(Succeed())

			oauth2Cfg := &types.LLMOAuth2Config{
				Enabled:        true,
				TokenURL:       "https://keycloak.acme.com/token",
				CredentialsDir: dir,
			}
			Expect(oauth2Cfg.ResolveOAuth2Credentials()).To(Succeed())
			Expect(oauth2Cfg.ClientID).To(Equal("my-client"))
			Expect(oauth2Cfg.ClientSecret).To(Equal("s3cret"))
		})

		It("should return error when client-id file is missing", func() {
			dir, err := os.MkdirTemp("", "oauth2-test")
			Expect(err).NotTo(HaveOccurred())
			defer func() { _ = os.RemoveAll(dir) }()

			Expect(os.WriteFile(filepath.Join(dir, "client-secret"), []byte("s3cret"), 0600)).To(Succeed())

			oauth2Cfg := &types.LLMOAuth2Config{
				Enabled:        true,
				TokenURL:       "https://keycloak.acme.com/token",
				CredentialsDir: dir,
			}
			err = oauth2Cfg.ResolveOAuth2Credentials()
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("client-id"))
		})

		It("should return error when credentialsDir is empty", func() {
			oauth2Cfg := &types.LLMOAuth2Config{
				Enabled:  true,
				TokenURL: "https://keycloak.acme.com/token",
			}
			err := oauth2Cfg.ResolveOAuth2Credentials()
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("credentialsDir"))
		})

		It("should be a no-op when oauth2 is disabled", func() {
			oauth2Cfg := &types.LLMOAuth2Config{Enabled: false}
			Expect(oauth2Cfg.ResolveOAuth2Credentials()).To(Succeed())
			Expect(oauth2Cfg.ClientID).To(BeEmpty())
		})
	})
})

var _ = Describe("AlignmentCheck EffectiveLLM merge — BR-AI-601", func() {

	var (
		base    types.LLMConfig
		runtime config.LLMRuntimeConfig
	)

	BeforeEach(func() {
		base = types.LLMConfig{
			Provider:        "openai",
			AzureAPIVersion: "2024-02",
			VertexProject:   "proj-base",
			VertexLocation:  "us-central1",
			BedrockRegion:   "us-east-1",
			TLSCaFile:       "/certs/ca.pem",
			OAuth2:          types.LLMOAuth2Config{Enabled: true, TokenURL: "https://auth.example.com/token"},
		}
		runtime = config.LLMRuntimeConfig{
			Model:      "gpt-4o",
			Endpoint:   "https://api.openai.com/v1",
			APIKeyFile: "/etc/credentials/base-key",
		}
	})

	Describe("UT-GAP1-001: nil LLM override returns inputs unchanged", func() {
		It("should return base and runtime unchanged when LLM override is nil", func() {
			cfg := config.AlignmentCheckConfig{LLM: nil}
			sOut, rOut := cfg.EffectiveLLM(base, runtime)
			Expect(sOut).To(Equal(base))
			Expect(rOut).To(Equal(runtime))
		})
	})

	Describe("UT-GAP1-002: partial override (model only) inherits other fields", func() {
		It("should override model but inherit provider, endpoint, apiKeyFile from base/runtime", func() {
			cfg := config.AlignmentCheckConfig{
				LLM: &config.LLMOverrideConfig{Model: "claude-3-opus"},
			}
			sOut, rOut := cfg.EffectiveLLM(base, runtime)
			Expect(sOut.Provider).To(Equal("openai"))
			Expect(rOut.Model).To(Equal("claude-3-opus"))
			Expect(rOut.Endpoint).To(Equal("https://api.openai.com/v1"))
			Expect(rOut.APIKeyFile).To(Equal("/etc/credentials/base-key"))
		})
	})

	Describe("UT-GAP1-003: full override replaces all fields", func() {
		It("should replace all overridable fields from LLMOverrideConfig", func() {
			cfg := config.AlignmentCheckConfig{
				LLM: &config.LLMOverrideConfig{
					Provider:        "anthropic",
					Model:           "claude-3-haiku",
					Endpoint:        "https://api.anthropic.com",
					APIKeyFile:      "/etc/credentials/shadow-key",
					AzureAPIVersion: "2025-01",
					VertexProject:   "proj-shadow",
					VertexLocation:  "europe-west1",
					BedrockRegion:   "eu-west-1",
				},
			}
			sOut, rOut := cfg.EffectiveLLM(base, runtime)
			Expect(sOut.Provider).To(Equal("anthropic"))
			Expect(sOut.AzureAPIVersion).To(Equal("2025-01"))
			Expect(sOut.VertexProject).To(Equal("proj-shadow"))
			Expect(sOut.VertexLocation).To(Equal("europe-west1"))
			Expect(sOut.BedrockRegion).To(Equal("eu-west-1"))
			Expect(rOut.Model).To(Equal("claude-3-haiku"))
			Expect(rOut.Endpoint).To(Equal("https://api.anthropic.com"))
			Expect(rOut.APIKeyFile).To(Equal("/etc/credentials/shadow-key"))
		})
	})

	Describe("UT-GAP1-004: override does not bleed into TLSCaFile or OAuth2", func() {
		It("should not modify base fields that are not in LLMOverrideConfig", func() {
			cfg := config.AlignmentCheckConfig{
				LLM: &config.LLMOverrideConfig{Provider: "anthropic", Model: "claude-3-haiku"},
			}
			sOut, _ := cfg.EffectiveLLM(base, runtime)
			Expect(sOut.TLSCaFile).To(Equal("/certs/ca.pem"))
			Expect(sOut.OAuth2.Enabled).To(BeTrue())
			Expect(sOut.OAuth2.TokenURL).To(Equal("https://auth.example.com/token"))
		})
	})

	Describe("UT-AI-1616-006 (CM-6): EffectiveLLM applies the shadow override's Reasoning when set", func() {
		It("should override Reasoning, not inherit the base's", func() {
			base.Reasoning = &types.LLMReasoningConfig{Enabled: true, Effort: "low"}
			cfg := config.AlignmentCheckConfig{
				LLM: &config.LLMOverrideConfig{
					Reasoning: &types.LLMReasoningConfig{Enabled: true, Effort: "high"},
				},
			}
			sOut, _ := cfg.EffectiveLLM(base, runtime)
			Expect(sOut.Reasoning).NotTo(BeNil())
			Expect(sOut.Reasoning.Effort).To(Equal("high"), "the shadow override's Reasoning must win over the base's")
		})
	})

	Describe("UT-AI-1616-007 (CM-6): EffectiveLLM falls back to base Reasoning when the shadow override has none", func() {
		It("should leave base Reasoning unchanged when the shadow override does not set it", func() {
			baseReasoning := &types.LLMReasoningConfig{Enabled: true, Effort: "medium"}
			base.Reasoning = baseReasoning
			cfg := config.AlignmentCheckConfig{
				LLM: &config.LLMOverrideConfig{Model: "claude-3-opus"},
			}
			sOut, _ := cfg.EffectiveLLM(base, runtime)
			Expect(sOut.Reasoning).To(Equal(baseReasoning), "base Reasoning must be preserved when the shadow override doesn't set it")
		})
	})
})

var _ = Describe("FleetConfig.AlignmentCheck — BR-AI-601", func() {
	Describe("UT-SA-601-FLEET-010: fleet alignment check YAML round-trip", func() {
		It("should parse fleet-specific alignment check from YAML", func() {
			yamlData := []byte(`
integrations:
  fleet:
    endpoint: "https://mcp-gw.example.com"
    gatewayType: "eaigw"
    alignmentCheck:
      enabled: true
      mode: enforce
      timeout: 15s
      maxStepTokens: 800
      maxRetries: 2
      verdictTimeout: 45s
      llm:
        provider: anthropic
        model: claude-3-haiku
        endpoint: "https://api.anthropic.com/v1"
`)
			cfg, err := config.Load(yamlData)
			Expect(err).NotTo(HaveOccurred())
			Expect(cfg.Integrations.Fleet.AlignmentCheck).NotTo(BeNil())
			Expect(cfg.Integrations.Fleet.AlignmentCheck.Enabled).To(BeTrue())
			Expect(cfg.Integrations.Fleet.AlignmentCheck.Mode).To(Equal(config.AlignmentModeEnforce))
			Expect(cfg.Integrations.Fleet.AlignmentCheck.Timeout).To(Equal(15 * time.Second))
			Expect(cfg.Integrations.Fleet.AlignmentCheck.MaxStepTokens).To(Equal(800))
			Expect(cfg.Integrations.Fleet.AlignmentCheck.MaxRetries).To(Equal(2))
			Expect(cfg.Integrations.Fleet.AlignmentCheck.VerdictTimeout).To(Equal(45 * time.Second))
			Expect(cfg.Integrations.Fleet.AlignmentCheck.LLM).NotTo(BeNil())
			Expect(cfg.Integrations.Fleet.AlignmentCheck.LLM.Provider).To(Equal("anthropic"))
			Expect(cfg.Integrations.Fleet.AlignmentCheck.LLM.Model).To(Equal("claude-3-haiku"))
		})
	})

	Describe("UT-SA-601-FLEET-011: fleet alignment check defaults to nil (inherits global)", func() {
		It("should leave AlignmentCheck nil when not specified in fleet config", func() {
			yamlData := []byte(`
integrations:
  fleet:
    endpoint: "https://mcp-gw.example.com"
    gatewayType: "eaigw"
`)
			cfg, err := config.Load(yamlData)
			Expect(err).NotTo(HaveOccurred())
			Expect(cfg.Integrations.Fleet.AlignmentCheck).To(BeNil())
		})
	})
})

var _ = Describe("ShutdownConfig — BR-PLATFORM-1329", func() {

	Describe("UT-KA-1329-001: runtime.shutdown.drainSeconds YAML round-trip (CM-6)", func() {
		It("should parse drainSeconds from YAML", func() {
			yaml := []byte(`
runtime:
  shutdown:
    drainSeconds: 45
  session:
    ttl: 30m
`)
			cfg, err := config.Load(yaml)
			Expect(err).NotTo(HaveOccurred())
			Expect(cfg.Runtime.Shutdown.DrainSeconds).To(Equal(45),
				"CM-6: shutdown drain duration must be configurable via YAML config")
		})
	})

	Describe("UT-KA-1329-002: DefaultConfig sets drainSeconds to 30 (CM-6)", func() {
		It("should default to 30s matching operator default", func() {
			cfg := config.DefaultConfig()
			Expect(cfg.Runtime.Shutdown.DrainSeconds).To(Equal(30),
				"CM-6: default drain duration must match operator default of 30s")
		})
	})

	Describe("UT-KA-1329-003: Validate rejects drainSeconds <= 0 (SC-5)", func() {
		It("should reject zero drainSeconds", func() {
			cfg := config.DefaultConfig()
			cfg.Runtime.Shutdown.DrainSeconds = 0
			err := cfg.Validate()
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("runtime.shutdown.drainSeconds"),
				"SC-5: invalid drain duration must be rejected to prevent premature termination")
		})

		It("should reject negative drainSeconds", func() {
			cfg := config.DefaultConfig()
			cfg.Runtime.Shutdown.DrainSeconds = -5
			err := cfg.Validate()
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("runtime.shutdown.drainSeconds"))
		})

		It("should reject drainSeconds exceeding operator max of 300", func() {
			cfg := config.DefaultConfig()
			cfg.Runtime.Shutdown.DrainSeconds = 301
			err := cfg.Validate()
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("must not exceed 300"))
		})

		It("should accept drainSeconds at operator max boundary", func() {
			cfg := config.DefaultConfig()
			cfg.Runtime.Shutdown.DrainSeconds = 300
			Expect(cfg.Validate()).To(Succeed())
		})
	})
})

var _ = Describe("LoggingConfig Format — BR-PLATFORM-1330", func() {

	Describe("UT-KA-1330-005: runtime.logging.format YAML round-trip (CM-6)", func() {
		It("should parse format from YAML", func() {
			yaml := []byte(`
runtime:
  logging:
    level: INFO
    format: console
  session:
    ttl: 30m
`)
			cfg, err := config.Load(yaml)
			Expect(err).NotTo(HaveOccurred())
			Expect(cfg.Runtime.Logging.Format).To(Equal("console"),
				"CM-6: log format must be configurable via YAML config")
		})
	})

	Describe("UT-KA-1330-006: KA Validate delegates format validation (CM-6)", func() {
		It("should reject invalid format via shared LoggingConfig.Validate", func() {
			cfg := config.DefaultConfig()
			cfg.Runtime.Logging.Format = "yaml"
			err := cfg.Validate()
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("log format"),
				"CM-6: invalid format must be rejected by validation")
		})
	})
})

var _ = Describe("Config.Validate — GAP-C3 (#954)", func() {
	DescribeTable("should reject invalid config values",
		func(mutate func(*config.Config), substr string) {
			cfg := config.DefaultConfig()
			mutate(cfg)
			err := cfg.Validate()
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring(substr))
		},
		Entry("port=0", func(c *config.Config) { c.Runtime.Server.Port = 0 }, "port"),
		Entry("port=70000", func(c *config.Config) { c.Runtime.Server.Port = 70000 }, "port"),
		Entry("session TTL=-1", func(c *config.Config) { c.Runtime.Session.TTL = -1 * time.Second }, "ttl"),
		Entry("audit bufferSize=0 when enabled", func(c *config.Config) {
			c.Runtime.Audit.Enabled = true
			c.Runtime.Audit.BufferSize = 0
		}, "bufferSize"),
		Entry("audit batchSize=0 when enabled", func(c *config.Config) {
			c.Runtime.Audit.Enabled = true
			c.Runtime.Audit.BatchSize = 0
		}, "batchSize"),
		Entry("maxToolOutputSize=0", func(c *config.Config) {
			c.AI.Summarizer.MaxToolOutputSize = 0
		}, "maxToolOutputSize"),
		Entry("enrichment maxRetries=-1", func(c *config.Config) {
			c.AI.Enrichment.MaxRetries = -1
		}, "maxRetries"),
		Entry("anomaly maxToolCallsPerTool=0", func(c *config.Config) {
			c.AI.Safety.Anomaly.MaxToolCallsPerTool = 0
		}, "maxToolCallsPerTool"),
		Entry("anomaly maxTotalToolCalls=0", func(c *config.Config) {
			c.AI.Safety.Anomaly.MaxTotalToolCalls = 0
		}, "maxTotalToolCalls"),
		Entry("anomaly maxRepeatedFailures=0", func(c *config.Config) {
			c.AI.Safety.Anomaly.MaxRepeatedFailures = 0
		}, "maxRepeatedFailures"),
		Entry("alignment maxRetries=-1 when enabled", func(c *config.Config) {
			c.AI.AlignmentCheck.Enabled = true
			c.AI.AlignmentCheck.MaxRetries = -1
		}, "maxRetries"),
		Entry("shutdown drainSeconds=0", func(c *config.Config) {
			c.Runtime.Shutdown.DrainSeconds = 0
		}, "drainSeconds"),
		Entry("shutdown drainSeconds=301 exceeds max", func(c *config.Config) {
			c.Runtime.Shutdown.DrainSeconds = 301
		}, "drainSeconds"),
	)

	It("should accept DefaultConfig as valid", func() {
		cfg := config.DefaultConfig()
		Expect(cfg.Validate()).To(Succeed())
	})

	Context("UT-KA-054-CFG-001 [IA-5]: KA FleetOAuth2 config parses scopes and validates (BR-INTEGRATION-054)", func() {
		It("rejects fleet OAuth2 enabled without tokenURL", func() {
			cfg := config.DefaultConfig()
			cfg.Integrations.Fleet.OAuth2.Enabled = true
			cfg.Integrations.Fleet.OAuth2.CredentialsSecretRef = "fleet-creds"
			err := cfg.Validate()
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("tokenURL"))
		})

		It("rejects fleet OAuth2 enabled without credentialsSecretRef", func() {
			cfg := config.DefaultConfig()
			cfg.Integrations.Fleet.OAuth2.Enabled = true
			cfg.Integrations.Fleet.OAuth2.TokenURL = "https://dex.local/token"
			err := cfg.Validate()
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("credentialsSecretRef"))
		})

		It("accepts fleet OAuth2 enabled with all required fields", func() {
			cfg := config.DefaultConfig()
			cfg.Integrations.Fleet.OAuth2.Enabled = true
			cfg.Integrations.Fleet.OAuth2.TokenURL = "https://dex.local/token"
			cfg.Integrations.Fleet.OAuth2.CredentialsSecretRef = "fleet-creds"
			Expect(cfg.Validate()).To(Succeed())
		})

		It("parses Scopes from YAML config", func() {
			yamlContent := `
runtime:
  server:
    port: 9443
    maxConcurrentRequests: 20
    rateLimit:
      requestsPerSecond: 10.0
      burst: 20
  session:
    ttl: 30m
    maxConcurrentInvestigations: 5
  audit:
    enabled: false
    bufferSize: 100
    batchSize: 10
    flushInterval: 5s
  logging:
    level: info
ai:
  investigation:
    maxTurns: 25
  summarizer:
    maxToolOutputSize: 4096
  enrichment:
    maxRetries: 3
  safety:
    anomaly:
      maxToolCallsPerTool: 10
      maxTotalToolCalls: 50
      maxRepeatedFailures: 3
  llm:
    provider: openai
  alignmentCheck:
    enabled: false
integrations:
  fleet:
    endpoint: "http://mcp-gateway:1975/mcp"
    oauth2:
      enabled: true
      tokenURL: "https://dex.local/token"
      credentialsSecretRef: "fleet-creds"
      scopes:
        - openid
        - groups
        - fleet-read
`
			cfg, err := config.Load([]byte(yamlContent))
			Expect(err).ToNot(HaveOccurred())
			Expect(cfg.Integrations.Fleet.OAuth2.Scopes).To(Equal([]string{"openid", "groups", "fleet-read"}))
		})
	})

	Describe("KA FleetConfig.GatewayType (CM-6, ADR-068 #11)", func() {
		It("UT-KA-CFG-001: FleetConfig parses gatewayType from YAML", func() {
			yamlContent := `
runtime:
  server:
    port: 8443
    rateLimit:
      requestsPerSecond: 5
      burst: 10
  session:
    ttl: 30m
    maxConcurrentInvestigations: 10
  shutdown:
    drainSeconds: 30
ai:
  investigation:
    maxTurns: 40
  safety:
    anomaly:
      maxToolCallsPerTool: 10
      maxTotalToolCalls: 30
      maxRepeatedFailures: 3
integrations:
  fleet:
    endpoint: "http://mcp-gateway:1975/mcp"
    gatewayType: "kuadrant"
`
			cfg, err := config.Load([]byte(yamlContent))
			Expect(err).ToNot(HaveOccurred())
			Expect(cfg.Integrations.Fleet.GatewayType).To(Equal("kuadrant"),
				"gatewayType must be parsed from YAML")
		})

		It("UT-KA-CFG-002: FleetConfig with empty gatewayType means fleet disabled", func() {
			yamlContent := `
runtime:
  server:
    port: 8443
    rateLimit:
      requestsPerSecond: 5
      burst: 10
  session:
    ttl: 30m
    maxConcurrentInvestigations: 10
  shutdown:
    drainSeconds: 30
ai:
  investigation:
    maxTurns: 40
  safety:
    anomaly:
      maxToolCallsPerTool: 10
      maxTotalToolCalls: 30
      maxRepeatedFailures: 3
integrations:
  fleet:
    endpoint: "http://mcp-gateway:1975/mcp"
`
			cfg, err := config.Load([]byte(yamlContent))
			Expect(err).ToNot(HaveOccurred())
			Expect(cfg.Integrations.Fleet.GatewayType).To(BeEmpty(),
				"empty gatewayType means fleet is disabled")
		})
	})

	// Readiness gate Wave 1 (#1553): registerFleetTools (cmd/kubernautagent/
	// toolregistry.go) silently returns (nil, nil) -- fleet disabled -- when
	// either Endpoint or GatewayType is empty. Before this check, an operator
	// who set one but not the other got no startup error: KA just ran with
	// fleet silently off instead of failing closed on the misconfiguration.
	Describe("KA FleetConfig Endpoint/GatewayType pairing (#1553)", func() {
		It("UT-KA-CFG-003: rejects endpoint set without gatewayType", func() {
			cfg := config.DefaultConfig()
			cfg.Integrations.Fleet.Endpoint = "http://mcp-gateway:1975/mcp"
			err := cfg.Validate()
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("gatewayType"))
		})

		It("UT-KA-CFG-004: rejects gatewayType set without endpoint", func() {
			cfg := config.DefaultConfig()
			cfg.Integrations.Fleet.GatewayType = "kuadrant"
			err := cfg.Validate()
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("endpoint"))
		})

		It("UT-KA-CFG-005: accepts both endpoint and gatewayType set", func() {
			cfg := config.DefaultConfig()
			cfg.Integrations.Fleet.Endpoint = "http://mcp-gateway:1975/mcp"
			cfg.Integrations.Fleet.GatewayType = "kuadrant"
			Expect(cfg.Validate()).To(Succeed())
		})

		It("UT-KA-CFG-006: accepts neither endpoint nor gatewayType set (fleet disabled)", func() {
			cfg := config.DefaultConfig()
			Expect(cfg.Validate()).To(Succeed())
		})
	})
})
