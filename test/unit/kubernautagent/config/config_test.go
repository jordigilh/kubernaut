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
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/jordigilh/kubernaut/internal/kubernautagent/config"
)

var _ = Describe("Kubernaut Agent Configuration — #433", func() {

	Describe("UT-KA-433-001: Kubernaut Agent loads valid YAML configuration", func() {
		It("should parse all required fields from valid YAML", func() {
			yaml := []byte(`
llm:
  endpoint: "http://localhost:11434/v1"
  model: "llama3"
  api_key: "test-key"
server:
  address: "0.0.0.0"
  port: 8080
session:
  ttl: 30m
audit:
  enabled: true
  endpoint: "http://datastorage:8080"
investigator:
  max_turns: 15
`)
			cfg, err := config.Load(yaml)
			Expect(err).NotTo(HaveOccurred())
			Expect(cfg).NotTo(BeNil())
			Expect(cfg.LLM.Endpoint).To(Equal("http://localhost:11434/v1"))
			Expect(cfg.LLM.Model).To(Equal("llama3"))
			Expect(cfg.LLM.APIKey).To(Equal("test-key"))
			Expect(cfg.Server.Port).To(Equal(8080))
			Expect(cfg.Session.TTL).To(Equal(30 * time.Minute))
			Expect(cfg.Audit.Enabled).To(BeTrue())
			Expect(cfg.Investigator.MaxTurns).To(Equal(15))
		})
	})

	Describe("UT-KA-433-002: Kubernaut Agent applies correct defaults", func() {
		It("should fill defaults when optional fields are omitted", func() {
			yaml := []byte(`
llm:
  endpoint: "http://localhost:11434/v1"
  model: "llama3"
`)
			cfg, err := config.Load(yaml)
			Expect(err).NotTo(HaveOccurred())
			Expect(cfg).NotTo(BeNil())

			Expect(cfg.Server.Port).To(Equal(8080), "default port should be 8080")
			Expect(cfg.Session.TTL).To(Equal(30*time.Minute), "default TTL should be 30m")
			Expect(cfg.Investigator.MaxTurns).To(Equal(15), "default max turns should be 15")
			Expect(cfg.Anomaly.MaxToolCallsPerTool).To(Equal(5), "default per-tool limit should be 5")
			Expect(cfg.Anomaly.MaxTotalToolCalls).To(Equal(30), "default total tool calls should be 30")
			Expect(cfg.Anomaly.MaxRepeatedFailures).To(Equal(3), "default repeated failures should be 3")
		})
	})

	Describe("UT-KA-433-003: Kubernaut Agent rejects invalid config at startup", func() {
		It("should reject missing LLM endpoint for non-exempt providers", func() {
			yaml := []byte(`
llm:
  provider: "mistral"
  model: "mistral-large"
`)
			cfg, err := config.Load(yaml)
			if err == nil && cfg != nil {
				err = cfg.Validate()
			}
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("endpoint"))
		})

		It("should accept empty LLM endpoint for openai (LangChainGo uses default)", func() {
			yaml := []byte(`
llm:
  provider: "openai"
  model: "gpt-4o"
`)
			cfg, err := config.Load(yaml)
			Expect(err).NotTo(HaveOccurred())
			Expect(cfg.Validate()).NotTo(HaveOccurred())
		})

		It("should reject invalid max-turns (zero)", func() {
			yaml := []byte(`
llm:
  endpoint: "http://localhost:11434/v1"
  model: "llama3"
investigator:
  max_turns: 0
`)
			cfg, err := config.Load(yaml)
			if err == nil && cfg != nil {
				err = cfg.Validate()
			}
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("max_turns"))
		})

		It("should reject negative max-turns", func() {
			yaml := []byte(`
llm:
  endpoint: "http://localhost:11434/v1"
  model: "llama3"
investigator:
  max_turns: -5
`)
			cfg, err := config.Load(yaml)
			if err == nil && cfg != nil {
				err = cfg.Validate()
			}
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("max_turns"))
		})

		It("should reject missing LLM model", func() {
			yaml := []byte(`
llm:
  endpoint: "http://localhost:11434/v1"
`)
			cfg, err := config.Load(yaml)
			if err == nil && cfg != nil {
				err = cfg.Validate()
			}
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("llm.model"))
		})
	})

	Describe("UT-KA-SO-CFG-001: structured_output flag sourced from SDK config", func() {
		It("should parse structured_output=true from SDK YAML via MergeSDKConfig", func() {
			mainYAML := []byte(`
llm:
  provider: "anthropic"
  model: "claude-sonnet-4-20250514"
`)
			sdkYAML := []byte(`
llm:
  provider: "anthropic"
  model: "claude-sonnet-4-20250514"
  structured_output: true
`)
			cfg, err := config.Load(mainYAML)
			Expect(err).NotTo(HaveOccurred())
			Expect(cfg.MergeSDKConfig(sdkYAML)).To(Succeed())
			Expect(cfg.LLM.StructuredOutput).To(BeTrue())
		})

		It("should default structured_output to false when omitted from SDK config", func() {
			mainYAML := []byte(`
llm:
  provider: "openai"
  model: "gpt-4o"
`)
			sdkYAML := []byte(`
llm:
  provider: "openai"
  model: "gpt-4o"
`)
			cfg, err := config.Load(mainYAML)
			Expect(err).NotTo(HaveOccurred())
			Expect(cfg.MergeSDKConfig(sdkYAML)).To(Succeed())
			Expect(cfg.LLM.StructuredOutput).To(BeFalse())
		})
	})

	Describe("UT-KA-SO-CFG-002: custom_headers sourced from SDK config", func() {
		It("should parse custom_headers from SDK YAML via MergeSDKConfig", func() {
			mainYAML := []byte(`
llm:
  provider: "openai"
  model: "gpt-4o"
`)
			sdkYAML := []byte(`
llm:
  provider: "openai"
  model: "gpt-4o"
  custom_headers:
    - name: "X-Custom-Auth"
      value: "Bearer token123"
    - name: "X-Org-Id"
      value: "org-42"
`)
			cfg, err := config.Load(mainYAML)
			Expect(err).NotTo(HaveOccurred())
			Expect(cfg.MergeSDKConfig(sdkYAML)).To(Succeed())
			Expect(cfg.LLM.CustomHeaders).To(HaveLen(2))
			Expect(cfg.LLM.CustomHeaders[0].Name).To(Equal("X-Custom-Auth"))
			Expect(cfg.LLM.CustomHeaders[0].Value).To(Equal("Bearer token123"))
			Expect(cfg.LLM.CustomHeaders[1].Name).To(Equal("X-Org-Id"))
		})

		It("should default custom_headers to nil when omitted from SDK config", func() {
			mainYAML := []byte(`
llm:
  provider: "openai"
  model: "gpt-4o"
`)
			sdkYAML := []byte(`
llm:
  provider: "openai"
  model: "gpt-4o"
`)
			cfg, err := config.Load(mainYAML)
			Expect(err).NotTo(HaveOccurred())
			Expect(cfg.MergeSDKConfig(sdkYAML)).To(Succeed())
			Expect(cfg.LLM.CustomHeaders).To(BeEmpty())
		})
	})

	Describe("UT-KA-433W-010: DefaultConfig applies summarizer threshold", func() {
		It("should set Summarizer.Threshold to 8000", func() {
			cfg := config.DefaultConfig()
			Expect(cfg.Summarizer.Threshold).To(Equal(8000))
		})
	})

	Describe("UT-KA-433W-011: DefaultConfig applies anomaly thresholds", func() {
		It("should set MaxToolCallsPerTool=5, MaxTotalToolCalls=30, MaxRepeatedFailures=3", func() {
			cfg := config.DefaultConfig()
			Expect(cfg.Anomaly.MaxToolCallsPerTool).To(Equal(5))
			Expect(cfg.Anomaly.MaxTotalToolCalls).To(Equal(30))
			Expect(cfg.Anomaly.MaxRepeatedFailures).To(Equal(3))
		})
	})

	Describe("MergeSDKConfig", func() {
		var mainYAML []byte

		BeforeEach(func() {
			mainYAML = []byte(`
llm:
  provider: "anthropic"
  model: "claude-sonnet-4-20250514"
`)
		})

		Describe("UT-KA-CFG-SDK-001: SDK oauth2 config merges into main config", func() {
			It("should populate cfg.LLM.OAuth2 from SDK YAML", func() {
				sdkYAML := []byte(`
llm:
  provider: "anthropic"
  model: "claude-sonnet-4-20250514"
  oauth2:
    enabled: true
    token_url: "https://keycloak.acme.com/realms/infra/protocol/openid-connect/token"
    client_id: "kubernaut-agent"
    client_secret: "s3cret"
    scopes:
      - "openid"
      - "llm-gateway"
`)
				cfg, err := config.Load(mainYAML)
				Expect(err).NotTo(HaveOccurred())
				Expect(cfg.MergeSDKConfig(sdkYAML)).To(Succeed())
				Expect(cfg.LLM.OAuth2.Enabled).To(BeTrue())
				Expect(cfg.LLM.OAuth2.TokenURL).To(Equal("https://keycloak.acme.com/realms/infra/protocol/openid-connect/token"))
				Expect(cfg.LLM.OAuth2.ClientID).To(Equal("kubernaut-agent"))
				Expect(cfg.LLM.OAuth2.ClientSecret).To(Equal("s3cret"))
				Expect(cfg.LLM.OAuth2.Scopes).To(Equal([]string{"openid", "llm-gateway"}))
			})
		})

		Describe("UT-KA-CFG-SDK-002: SDK structured_output merges into main config", func() {
			It("should populate cfg.LLM.StructuredOutput from SDK YAML", func() {
				sdkYAML := []byte(`
llm:
  provider: "anthropic"
  model: "claude-sonnet-4-20250514"
  structured_output: true
`)
				cfg, err := config.Load(mainYAML)
				Expect(err).NotTo(HaveOccurred())
				Expect(cfg.MergeSDKConfig(sdkYAML)).To(Succeed())
				Expect(cfg.LLM.StructuredOutput).To(BeTrue())
			})
		})

		Describe("UT-KA-CFG-SDK-003: SDK custom_headers merge into main config", func() {
			It("should populate cfg.LLM.CustomHeaders from SDK YAML", func() {
				sdkYAML := []byte(`
llm:
  provider: "anthropic"
  model: "claude-sonnet-4-20250514"
  custom_headers:
    - name: "X-Custom-Auth"
      value: "Bearer token123"
    - name: "X-Org-Id"
      value: "org-42"
`)
				cfg, err := config.Load(mainYAML)
				Expect(err).NotTo(HaveOccurred())
				Expect(cfg.MergeSDKConfig(sdkYAML)).To(Succeed())
				Expect(cfg.LLM.CustomHeaders).To(HaveLen(2))
				Expect(cfg.LLM.CustomHeaders[0].Name).To(Equal("X-Custom-Auth"))
				Expect(cfg.LLM.CustomHeaders[0].Value).To(Equal("Bearer token123"))
				Expect(cfg.LLM.CustomHeaders[1].Name).To(Equal("X-Org-Id"))
				Expect(cfg.LLM.CustomHeaders[1].Value).To(Equal("org-42"))
			})
		})

		Describe("UT-KA-CFG-SDK-004: Main config YAML with llm.oauth2 is ignored by config.Load()", func() {
			It("should NOT parse oauth2 from main config YAML", func() {
				mainWithOAuth := []byte(`
llm:
  provider: "anthropic"
  model: "claude-sonnet-4-20250514"
  oauth2:
    enabled: true
    token_url: "https://keycloak.acme.com/token"
    client_id: "ka"
    client_secret: "secret"
  structured_output: true
  custom_headers:
    - name: "X-Test"
      value: "should-be-ignored"
`)
				cfg, err := config.Load(mainWithOAuth)
				Expect(err).NotTo(HaveOccurred())
				Expect(cfg.LLM.OAuth2.Enabled).To(BeFalse(), "oauth2 must not be parsed from main config")
				Expect(cfg.LLM.OAuth2.TokenURL).To(BeEmpty())
				Expect(cfg.LLM.OAuth2.ClientID).To(BeEmpty())
				Expect(cfg.LLM.OAuth2.ClientSecret).To(BeEmpty())
				Expect(cfg.LLM.OAuth2.Scopes).To(BeNil())
				Expect(cfg.LLM.StructuredOutput).To(BeFalse(), "structured_output must not be parsed from main config")
				Expect(cfg.LLM.CustomHeaders).To(BeEmpty(), "custom_headers must not be parsed from main config")
			})
		})

		Describe("UT-KA-CFG-SDK-005: Provider/model merge still works (regression)", func() {
			It("should merge provider and model from SDK when main uses defaults", func() {
				minimalMain := []byte(`
llm:
  endpoint: "http://localhost:11434/v1"
`)
				sdkYAML := []byte(`
llm:
  provider: "anthropic"
  model: "claude-sonnet-4-20250514"
`)
				cfg, err := config.Load(minimalMain)
				Expect(err).NotTo(HaveOccurred())
				Expect(cfg.MergeSDKConfig(sdkYAML)).To(Succeed())
				Expect(cfg.LLM.Provider).To(Equal("anthropic"))
				Expect(cfg.LLM.Model).To(Equal("claude-sonnet-4-20250514"))
			})
		})

		Describe("UT-KA-CFG-SDK-006: Malformed SDK YAML returns parse error", func() {
			It("should return error for invalid YAML", func() {
				cfg, err := config.Load(mainYAML)
				Expect(err).NotTo(HaveOccurred())
				err = cfg.MergeSDKConfig([]byte(`{invalid yaml: [`))
				Expect(err).To(HaveOccurred())
				Expect(err.Error()).To(ContainSubstring("parsing SDK config"))
			})
		})
	})

	Describe("UT-KA-417-020: OAuth2Config sourced from SDK config", func() {
		It("should parse all OAuth2 fields from SDK YAML via MergeSDKConfig", func() {
			mainYAML := []byte(`
llm:
  provider: "anthropic"
  model: "claude-sonnet-4-20250514"
`)
			sdkYAML := []byte(`
llm:
  provider: "anthropic"
  model: "claude-sonnet-4-20250514"
  oauth2:
    enabled: true
    token_url: "https://keycloak.acme.com/realms/infra/protocol/openid-connect/token"
    client_id: "kubernaut-agent"
    client_secret: "s3cret"
    scopes:
      - "openid"
      - "llm-gateway"
`)
			cfg, err := config.Load(mainYAML)
			Expect(err).NotTo(HaveOccurred())
			Expect(cfg.MergeSDKConfig(sdkYAML)).To(Succeed())
			Expect(cfg.LLM.OAuth2.Enabled).To(BeTrue())
			Expect(cfg.LLM.OAuth2.TokenURL).To(Equal("https://keycloak.acme.com/realms/infra/protocol/openid-connect/token"))
			Expect(cfg.LLM.OAuth2.ClientID).To(Equal("kubernaut-agent"))
			Expect(cfg.LLM.OAuth2.ClientSecret).To(Equal("s3cret"))
			Expect(cfg.LLM.OAuth2.Scopes).To(Equal([]string{"openid", "llm-gateway"}))
		})
	})

	Describe("UT-KA-417-021: OAuth2Config defaults to disabled when omitted from SDK", func() {
		It("should have OAuth2 disabled with empty fields after SDK merge", func() {
			mainYAML := []byte(`
llm:
  provider: "openai"
  model: "gpt-4o"
`)
			sdkYAML := []byte(`
llm:
  provider: "openai"
  model: "gpt-4o"
`)
			cfg, err := config.Load(mainYAML)
			Expect(err).NotTo(HaveOccurred())
			Expect(cfg.MergeSDKConfig(sdkYAML)).To(Succeed())
			Expect(cfg.LLM.OAuth2.Enabled).To(BeFalse())
			Expect(cfg.LLM.OAuth2.TokenURL).To(BeEmpty())
			Expect(cfg.LLM.OAuth2.ClientID).To(BeEmpty())
			Expect(cfg.LLM.OAuth2.ClientSecret).To(BeEmpty())
			Expect(cfg.LLM.OAuth2.Scopes).To(BeNil())
		})
	})

	Describe("UT-KA-417-022: Validate rejects missing token_url when oauth2 enabled", func() {
		It("should return error identifying missing token_url", func() {
			mainYAML := []byte(`
llm:
  provider: "anthropic"
  model: "claude-sonnet-4-20250514"
`)
			sdkYAML := []byte(`
llm:
  provider: "anthropic"
  model: "claude-sonnet-4-20250514"
  oauth2:
    enabled: true
    client_id: "kubernaut-agent"
    client_secret: "s3cret"
`)
			cfg, err := config.Load(mainYAML)
			Expect(err).NotTo(HaveOccurred())
			Expect(cfg.MergeSDKConfig(sdkYAML)).To(Succeed())
			err = cfg.Validate()
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("token_url"))
		})
	})

	Describe("UT-KA-417-023: Validate rejects missing client_id when oauth2 enabled", func() {
		It("should return error identifying missing client_id", func() {
			mainYAML := []byte(`
llm:
  provider: "anthropic"
  model: "claude-sonnet-4-20250514"
`)
			sdkYAML := []byte(`
llm:
  provider: "anthropic"
  model: "claude-sonnet-4-20250514"
  oauth2:
    enabled: true
    token_url: "https://keycloak.acme.com/token"
    client_secret: "s3cret"
`)
			cfg, err := config.Load(mainYAML)
			Expect(err).NotTo(HaveOccurred())
			Expect(cfg.MergeSDKConfig(sdkYAML)).To(Succeed())
			err = cfg.Validate()
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("client_id"))
		})
	})

	Describe("UT-KA-417-024: Validate rejects missing client_secret when oauth2 enabled", func() {
		It("should return error identifying missing client_secret", func() {
			mainYAML := []byte(`
llm:
  provider: "anthropic"
  model: "claude-sonnet-4-20250514"
`)
			sdkYAML := []byte(`
llm:
  provider: "anthropic"
  model: "claude-sonnet-4-20250514"
  oauth2:
    enabled: true
    token_url: "https://keycloak.acme.com/token"
    client_id: "kubernaut-agent"
`)
			cfg, err := config.Load(mainYAML)
			Expect(err).NotTo(HaveOccurred())
			Expect(cfg.MergeSDKConfig(sdkYAML)).To(Succeed())
			err = cfg.Validate()
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("client_secret"))
		})
	})

	Describe("UT-KA-417-025: Validate accepts oauth2 disabled with empty fields", func() {
		It("should not require OAuth2 fields when disabled in SDK config", func() {
			mainYAML := []byte(`
llm:
  provider: "anthropic"
  model: "claude-sonnet-4-20250514"
`)
			sdkYAML := []byte(`
llm:
  provider: "anthropic"
  model: "claude-sonnet-4-20250514"
  oauth2:
    enabled: false
`)
			cfg, err := config.Load(mainYAML)
			Expect(err).NotTo(HaveOccurred())
			Expect(cfg.MergeSDKConfig(sdkYAML)).To(Succeed())
			Expect(cfg.Validate()).NotTo(HaveOccurred())
		})

		It("should not require OAuth2 fields when section omitted from SDK config", func() {
			mainYAML := []byte(`
llm:
  provider: "anthropic"
  model: "claude-sonnet-4-20250514"
`)
			sdkYAML := []byte(`
llm:
  provider: "anthropic"
  model: "claude-sonnet-4-20250514"
`)
			cfg, err := config.Load(mainYAML)
			Expect(err).NotTo(HaveOccurred())
			Expect(cfg.MergeSDKConfig(sdkYAML)).To(Succeed())
			Expect(cfg.Validate()).NotTo(HaveOccurred())
		})
	})
})

var _ = Describe("Conversation LLM Config — #592", func() {

	Describe("UT-CS-592-023: EffectiveLLM defaults to investigation model when LLM field is nil", func() {
		It("should return the base LLM config unchanged", func() {
			conv := config.ConversationConfig{
				Enabled: true,
				LLM:     nil,
			}
			base := config.LLMConfig{
				Provider: "openai",
				Endpoint: "https://api.openai.com/v1",
				Model:    "gpt-4o",
				APIKey:   "sk-base-key",
			}

			effective := conv.EffectiveLLM(base)

			Expect(effective.Provider).To(Equal("openai"),
				"nil conversation LLM must inherit base provider")
			Expect(effective.Model).To(Equal("gpt-4o"),
				"nil conversation LLM must inherit base model")
			Expect(effective.Endpoint).To(Equal("https://api.openai.com/v1"),
				"nil conversation LLM must inherit base endpoint")
			Expect(effective.APIKey).To(Equal("sk-base-key"),
				"nil conversation LLM must inherit base API key")
		})
	})

	Describe("UT-CS-592-024: EffectiveLLM uses override when set", func() {
		It("should merge conversation overrides over base config", func() {
			conv := config.ConversationConfig{
				Enabled: true,
				LLM: &config.LLMConfig{
					Model: "gpt-4o-mini",
				},
			}
			base := config.LLMConfig{
				Provider: "openai",
				Endpoint: "https://api.openai.com/v1",
				Model:    "gpt-4o",
				APIKey:   "sk-base-key",
			}

			effective := conv.EffectiveLLM(base)

			Expect(effective.Model).To(Equal("gpt-4o-mini"),
				"conversation LLM override must take precedence for Model")
			Expect(effective.Provider).To(Equal("openai"),
				"unset fields must inherit from base")
			Expect(effective.Endpoint).To(Equal("https://api.openai.com/v1"),
				"unset fields must inherit from base")
			Expect(effective.APIKey).To(Equal("sk-base-key"),
				"unset fields must inherit from base")
		})
	})
})
