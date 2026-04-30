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

	Describe("UT-KA-SO-CFG-001: structuredOutput parsed from main config", func() {
		It("should parse structuredOutput=true from main YAML", func() {
			cfgYAML := []byte(`
ai:
  llm:
    provider: "anthropic"
    model: "claude-sonnet-4-20250514"
    structuredOutput: true
`)
			cfg, err := config.Load(cfgYAML)
			Expect(err).NotTo(HaveOccurred())
			Expect(cfg.AI.LLM.StructuredOutput).To(BeTrue())
		})

		It("should default structuredOutput to false when omitted", func() {
			cfgYAML := []byte(`
ai:
  llm:
    provider: "openai"
    model: "gpt-4o"
`)
			cfg, err := config.Load(cfgYAML)
			Expect(err).NotTo(HaveOccurred())
			Expect(cfg.AI.LLM.StructuredOutput).To(BeFalse())
		})
	})

	Describe("UT-KA-SO-CFG-002: customHeaders parsed from main config", func() {
		It("should parse customHeaders from main YAML", func() {
			cfgYAML := []byte(`
ai:
  llm:
    provider: "openai"
    model: "gpt-4o"
    customHeaders:
      - name: "X-Custom-Auth"
        value: "Bearer token123"
      - name: "X-Org-Id"
        value: "org-42"
`)
			cfg, err := config.Load(cfgYAML)
			Expect(err).NotTo(HaveOccurred())
			Expect(cfg.AI.LLM.CustomHeaders).To(HaveLen(2))
			Expect(cfg.AI.LLM.CustomHeaders[0].Name).To(Equal("X-Custom-Auth"))
			Expect(cfg.AI.LLM.CustomHeaders[0].Value).To(Equal("Bearer token123"))
			Expect(cfg.AI.LLM.CustomHeaders[1].Name).To(Equal("X-Org-Id"))
		})

		It("should default customHeaders to nil when omitted", func() {
			cfgYAML := []byte(`
ai:
  llm:
    provider: "openai"
    model: "gpt-4o"
`)
			cfg, err := config.Load(cfgYAML)
			Expect(err).NotTo(HaveOccurred())
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

		Describe("UT-KA-CFG-CON-002: structuredOutput parsed from main config", func() {
			It("should populate cfg.AI.LLM.StructuredOutput from main YAML", func() {
				cfgYAML := []byte(`
ai:
  llm:
    provider: "anthropic"
    model: "claude-sonnet-4-20250514"
    structuredOutput: true
`)
				cfg, err := config.Load(cfgYAML)
				Expect(err).NotTo(HaveOccurred())
				Expect(cfg.AI.LLM.StructuredOutput).To(BeTrue())
			})
		})

		Describe("UT-KA-CFG-CON-003: customHeaders parsed from main config", func() {
			It("should populate cfg.AI.LLM.CustomHeaders from main YAML", func() {
				cfgYAML := []byte(`
ai:
  llm:
    provider: "anthropic"
    model: "claude-sonnet-4-20250514"
    customHeaders:
      - name: "X-Custom-Auth"
        value: "Bearer token123"
      - name: "X-Org-Id"
        value: "org-42"
`)
				cfg, err := config.Load(cfgYAML)
				Expect(err).NotTo(HaveOccurred())
				Expect(cfg.AI.LLM.CustomHeaders).To(HaveLen(2))
				Expect(cfg.AI.LLM.CustomHeaders[0].Name).To(Equal("X-Custom-Auth"))
				Expect(cfg.AI.LLM.CustomHeaders[0].Value).To(Equal("Bearer token123"))
				Expect(cfg.AI.LLM.CustomHeaders[1].Name).To(Equal("X-Org-Id"))
				Expect(cfg.AI.LLM.CustomHeaders[1].Value).To(Equal("org-42"))
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

	Describe("UT-KA-CFG-OAUTH2-RESOLVE: ResolveOAuth2Credentials reads from mounted Secret files", func() {
		It("should resolve clientID and clientSecret from files", func() {
			dir, err := os.MkdirTemp("", "oauth2-test")
			Expect(err).NotTo(HaveOccurred())
			defer os.RemoveAll(dir)

			Expect(os.WriteFile(filepath.Join(dir, "client-id"), []byte("my-client\n"), 0600)).To(Succeed())
			Expect(os.WriteFile(filepath.Join(dir, "client-secret"), []byte("s3cret\n"), 0600)).To(Succeed())

			oauth2Cfg := &config.OAuth2Config{
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
			defer os.RemoveAll(dir)

			Expect(os.WriteFile(filepath.Join(dir, "client-secret"), []byte("s3cret"), 0600)).To(Succeed())

			oauth2Cfg := &config.OAuth2Config{
				Enabled:        true,
				TokenURL:       "https://keycloak.acme.com/token",
				CredentialsDir: dir,
			}
			err = oauth2Cfg.ResolveOAuth2Credentials()
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("client-id"))
		})

		It("should return error when credentialsDir is empty", func() {
			oauth2Cfg := &config.OAuth2Config{
				Enabled:  true,
				TokenURL: "https://keycloak.acme.com/token",
			}
			err := oauth2Cfg.ResolveOAuth2Credentials()
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("credentialsDir"))
		})

		It("should be a no-op when oauth2 is disabled", func() {
			oauth2Cfg := &config.OAuth2Config{Enabled: false}
			Expect(oauth2Cfg.ResolveOAuth2Credentials()).To(Succeed())
			Expect(oauth2Cfg.ClientID).To(BeEmpty())
		})
	})
})
