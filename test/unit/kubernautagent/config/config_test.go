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
ai:
  llm:
    endpoint: "http://localhost:11434/v1"
    model: "llama3"
    apiKey: "test-key"
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
    endpoint: "http://datastorage:8080"
`)
			cfg, err := config.Load(yaml)
			Expect(err).NotTo(HaveOccurred())
			Expect(cfg).NotTo(BeNil())
			Expect(cfg.AI.LLM.Endpoint).To(Equal("http://localhost:11434/v1"))
			Expect(cfg.AI.LLM.Model).To(Equal("llama3"))
			Expect(cfg.AI.LLM.APIKey).To(Equal("test-key"))
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

			Expect(cfg.Runtime.Server.Port).To(Equal(8080), "default port should be 8080")
			Expect(cfg.Runtime.Session.TTL).To(Equal(30*time.Minute), "default TTL should be 30m")
			Expect(cfg.AI.Investigation.MaxTurns).To(Equal(40), "default max turns should be 40")
			Expect(cfg.AI.Safety.Anomaly.MaxToolCallsPerTool).To(Equal(10), "UT-KA-860-006: default per-tool limit raised to 10 per #860")
			Expect(cfg.AI.Safety.Anomaly.MaxTotalToolCalls).To(Equal(30), "default total tool calls should be 30")
			Expect(cfg.AI.Safety.Anomaly.MaxRepeatedFailures).To(Equal(3), "default repeated failures should be 3")
		})
	})

	Describe("UT-KA-433-003: Kubernaut Agent rejects invalid config at startup", func() {
		It("should reject missing LLM endpoint for non-exempt providers", func() {
			yaml := []byte(`
ai:
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
ai:
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
ai:
  llm:
    endpoint: "http://localhost:11434/v1"
    model: "llama3"
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

		It("should reject missing LLM model", func() {
			yaml := []byte(`
ai:
  llm:
    endpoint: "http://localhost:11434/v1"
`)
			cfg, err := config.Load(yaml)
			if err == nil && cfg != nil {
				err = cfg.Validate()
			}
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("ai.llm.model"))
		})
	})

	Describe("UT-KA-SO-CFG-001: structured_output flag sourced from SDK config", func() {
		It("should parse structured_output=true from SDK YAML via MergeSDKConfig", func() {
			mainYAML := []byte(`
ai:
  llm:
    provider: "anthropic"
    model: "claude-sonnet-4-20250514"
`)
			sdkYAML := []byte(`
llm:
  provider: "anthropic"
  model: "claude-sonnet-4-20250514"
  structuredOutput: true
`)
			cfg, err := config.Load(mainYAML)
			Expect(err).NotTo(HaveOccurred())
			Expect(cfg.MergeSDKConfig(sdkYAML)).To(Succeed())
			Expect(cfg.AI.LLM.StructuredOutput).To(BeTrue())
		})

		It("should default structured_output to false when omitted from SDK config", func() {
			mainYAML := []byte(`
ai:
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
			Expect(cfg.AI.LLM.StructuredOutput).To(BeFalse())
		})
	})

	Describe("UT-KA-SO-CFG-002: custom_headers sourced from SDK config", func() {
		It("should parse custom_headers from SDK YAML via MergeSDKConfig", func() {
			mainYAML := []byte(`
ai:
  llm:
    provider: "openai"
    model: "gpt-4o"
`)
			sdkYAML := []byte(`
llm:
  provider: "openai"
  model: "gpt-4o"
  customHeaders:
    - name: "X-Custom-Auth"
      value: "Bearer token123"
    - name: "X-Org-Id"
      value: "org-42"
`)
			cfg, err := config.Load(mainYAML)
			Expect(err).NotTo(HaveOccurred())
			Expect(cfg.MergeSDKConfig(sdkYAML)).To(Succeed())
			Expect(cfg.AI.LLM.CustomHeaders).To(HaveLen(2))
			Expect(cfg.AI.LLM.CustomHeaders[0].Name).To(Equal("X-Custom-Auth"))
			Expect(cfg.AI.LLM.CustomHeaders[0].Value).To(Equal("Bearer token123"))
			Expect(cfg.AI.LLM.CustomHeaders[1].Name).To(Equal("X-Org-Id"))
		})

		It("should default custom_headers to nil when omitted from SDK config", func() {
			mainYAML := []byte(`
ai:
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
			Expect(cfg.AI.LLM.CustomHeaders).To(BeEmpty())
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

	Describe("MergeSDKConfig", func() {
		var mainYAML []byte

		BeforeEach(func() {
			mainYAML = []byte(`
ai:
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
    tokenURL: "https://keycloak.acme.com/realms/infra/protocol/openid-connect/token"
    clientID: "kubernaut-agent"
    clientSecret: "s3cret"
    scopes:
      - "openid"
      - "llm-gateway"
`)
				cfg, err := config.Load(mainYAML)
				Expect(err).NotTo(HaveOccurred())
				Expect(cfg.MergeSDKConfig(sdkYAML)).To(Succeed())
				Expect(cfg.AI.LLM.OAuth2.Enabled).To(BeTrue())
				Expect(cfg.AI.LLM.OAuth2.TokenURL).To(Equal("https://keycloak.acme.com/realms/infra/protocol/openid-connect/token"))
				Expect(cfg.AI.LLM.OAuth2.ClientID).To(Equal("kubernaut-agent"))
				Expect(cfg.AI.LLM.OAuth2.ClientSecret).To(Equal("s3cret"))
				Expect(cfg.AI.LLM.OAuth2.Scopes).To(Equal([]string{"openid", "llm-gateway"}))
			})
		})

		Describe("UT-KA-CFG-SDK-002: SDK structured_output merges into main config", func() {
			It("should populate cfg.LLM.StructuredOutput from SDK YAML", func() {
				sdkYAML := []byte(`
llm:
  provider: "anthropic"
  model: "claude-sonnet-4-20250514"
  structuredOutput: true
`)
				cfg, err := config.Load(mainYAML)
				Expect(err).NotTo(HaveOccurred())
				Expect(cfg.MergeSDKConfig(sdkYAML)).To(Succeed())
				Expect(cfg.AI.LLM.StructuredOutput).To(BeTrue())
			})
		})

		Describe("UT-KA-CFG-SDK-003: SDK custom_headers merge into main config", func() {
			It("should populate cfg.LLM.CustomHeaders from SDK YAML", func() {
				sdkYAML := []byte(`
llm:
  provider: "anthropic"
  model: "claude-sonnet-4-20250514"
  customHeaders:
    - name: "X-Custom-Auth"
      value: "Bearer token123"
    - name: "X-Org-Id"
      value: "org-42"
`)
				cfg, err := config.Load(mainYAML)
				Expect(err).NotTo(HaveOccurred())
				Expect(cfg.MergeSDKConfig(sdkYAML)).To(Succeed())
				Expect(cfg.AI.LLM.CustomHeaders).To(HaveLen(2))
				Expect(cfg.AI.LLM.CustomHeaders[0].Name).To(Equal("X-Custom-Auth"))
				Expect(cfg.AI.LLM.CustomHeaders[0].Value).To(Equal("Bearer token123"))
				Expect(cfg.AI.LLM.CustomHeaders[1].Name).To(Equal("X-Org-Id"))
				Expect(cfg.AI.LLM.CustomHeaders[1].Value).To(Equal("org-42"))
			})
		})

		Describe("UT-KA-CFG-SDK-004: Main config YAML with llm.oauth2 is ignored by config.Load()", func() {
			It("should NOT parse oauth2 from main config YAML", func() {
				mainWithOAuth := []byte(`
ai:
  llm:
    provider: "anthropic"
    model: "claude-sonnet-4-20250514"
    oauth2:
      enabled: true
      tokenURL: "https://keycloak.acme.com/token"
      clientID: "ka"
      clientSecret: "secret"
    structuredOutput: true
    customHeaders:
      - name: "X-Test"
        value: "should-be-ignored"
`)
				cfg, err := config.Load(mainWithOAuth)
				Expect(err).NotTo(HaveOccurred())
				Expect(cfg.AI.LLM.OAuth2.Enabled).To(BeFalse(), "oauth2 must not be parsed from main config")
				Expect(cfg.AI.LLM.OAuth2.TokenURL).To(BeEmpty())
				Expect(cfg.AI.LLM.OAuth2.ClientID).To(BeEmpty())
				Expect(cfg.AI.LLM.OAuth2.ClientSecret).To(BeEmpty())
				Expect(cfg.AI.LLM.OAuth2.Scopes).To(BeNil())
				Expect(cfg.AI.LLM.StructuredOutput).To(BeFalse(), "structured_output must not be parsed from main config")
				Expect(cfg.AI.LLM.CustomHeaders).To(BeEmpty(), "custom_headers must not be parsed from main config")
			})
		})

		Describe("UT-KA-CFG-SDK-005: Provider/model merge still works (regression)", func() {
			It("should merge provider and model from SDK when main uses defaults", func() {
				minimalMain := []byte(`
ai:
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
				Expect(cfg.AI.LLM.Provider).To(Equal("anthropic"))
				Expect(cfg.AI.LLM.Model).To(Equal("claude-sonnet-4-20250514"))
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
ai:
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
    tokenURL: "https://keycloak.acme.com/realms/infra/protocol/openid-connect/token"
    clientID: "kubernaut-agent"
    clientSecret: "s3cret"
    scopes:
      - "openid"
      - "llm-gateway"
`)
			cfg, err := config.Load(mainYAML)
			Expect(err).NotTo(HaveOccurred())
			Expect(cfg.MergeSDKConfig(sdkYAML)).To(Succeed())
			Expect(cfg.AI.LLM.OAuth2.Enabled).To(BeTrue())
			Expect(cfg.AI.LLM.OAuth2.TokenURL).To(Equal("https://keycloak.acme.com/realms/infra/protocol/openid-connect/token"))
			Expect(cfg.AI.LLM.OAuth2.ClientID).To(Equal("kubernaut-agent"))
			Expect(cfg.AI.LLM.OAuth2.ClientSecret).To(Equal("s3cret"))
			Expect(cfg.AI.LLM.OAuth2.Scopes).To(Equal([]string{"openid", "llm-gateway"}))
		})
	})

	Describe("UT-KA-417-021: OAuth2Config defaults to disabled when omitted from SDK", func() {
		It("should have OAuth2 disabled with empty fields after SDK merge", func() {
			mainYAML := []byte(`
ai:
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
			Expect(cfg.AI.LLM.OAuth2.Enabled).To(BeFalse())
			Expect(cfg.AI.LLM.OAuth2.TokenURL).To(BeEmpty())
			Expect(cfg.AI.LLM.OAuth2.ClientID).To(BeEmpty())
			Expect(cfg.AI.LLM.OAuth2.ClientSecret).To(BeEmpty())
			Expect(cfg.AI.LLM.OAuth2.Scopes).To(BeNil())
		})
	})

	Describe("UT-KA-417-022: Validate rejects missing token_url when oauth2 enabled", func() {
		It("should return error identifying missing token_url", func() {
			mainYAML := []byte(`
ai:
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
    clientID: "kubernaut-agent"
    clientSecret: "s3cret"
`)
			cfg, err := config.Load(mainYAML)
			Expect(err).NotTo(HaveOccurred())
			Expect(cfg.MergeSDKConfig(sdkYAML)).To(Succeed())
			err = cfg.Validate()
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("tokenURL"))
		})
	})

	Describe("UT-KA-417-023: Validate rejects missing client_id when oauth2 enabled", func() {
		It("should return error identifying missing client_id", func() {
			mainYAML := []byte(`
ai:
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
    tokenURL: "https://keycloak.acme.com/token"
    clientSecret: "s3cret"
`)
			cfg, err := config.Load(mainYAML)
			Expect(err).NotTo(HaveOccurred())
			Expect(cfg.MergeSDKConfig(sdkYAML)).To(Succeed())
			err = cfg.Validate()
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("clientID"))
		})
	})

	Describe("UT-KA-417-024: Validate rejects missing client_secret when oauth2 enabled", func() {
		It("should return error identifying missing client_secret", func() {
			mainYAML := []byte(`
ai:
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
    tokenURL: "https://keycloak.acme.com/token"
    clientID: "kubernaut-agent"
`)
			cfg, err := config.Load(mainYAML)
			Expect(err).NotTo(HaveOccurred())
			Expect(cfg.MergeSDKConfig(sdkYAML)).To(Succeed())
			err = cfg.Validate()
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("clientSecret"))
		})
	})

	Describe("UT-KA-417-025: Validate accepts oauth2 disabled with empty fields", func() {
		It("should not require OAuth2 fields when disabled in SDK config", func() {
			mainYAML := []byte(`
ai:
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
ai:
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
