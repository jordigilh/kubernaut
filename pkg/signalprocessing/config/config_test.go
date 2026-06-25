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

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	spconfig "github.com/jordigilh/kubernaut/pkg/signalprocessing/config"
)

var _ = Describe("UT-SP-054-CFG-001 [IA-5]: SP FleetOAuth2 config parses, validates, and defaults correctly at startup (BR-INTEGRATION-054)", func() {
	Context("Validation", func() {
		It("accepts valid config with Fleet OAuth2 enabled", func() {
			cfg := spconfig.DefaultConfig()
			cfg.Fleet.OAuth2.Enabled = true
			cfg.Fleet.OAuth2.TokenURL = "https://dex.local/token"
			cfg.Fleet.OAuth2.CredentialsSecretRef = "fleet-oauth2"

			err := cfg.Validate()
			Expect(err).ToNot(HaveOccurred())
		})

		It("rejects OAuth2 enabled without tokenURL", func() {
			cfg := spconfig.DefaultConfig()
			cfg.Fleet.OAuth2.Enabled = true
			cfg.Fleet.OAuth2.CredentialsSecretRef = "fleet-oauth2"

			err := cfg.Validate()
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("tokenURL"))
		})

		It("rejects OAuth2 enabled without credentialsSecretRef", func() {
			cfg := spconfig.DefaultConfig()
			cfg.Fleet.OAuth2.Enabled = true
			cfg.Fleet.OAuth2.TokenURL = "https://dex.local/token"

			err := cfg.Validate()
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("credentialsSecretRef"))
		})

		It("accepts config with OAuth2 disabled (no fleet fields required)", func() {
			cfg := spconfig.DefaultConfig()
			cfg.Fleet.OAuth2.Enabled = false

			err := cfg.Validate()
			Expect(err).ToNot(HaveOccurred())
		})
	})

	Context("YAML parsing with scopes", func() {
		It("parses explicit scopes from YAML", func() {
			yamlContent := `
enrichment:
  cacheTtl: 5m
  timeout: 10s
classifier:
  regoConfigMapName: signalprocessing-policy
  regoConfigMapKey: policy.rego
  hotReloadInterval: 30s
fleet:
  endpoint: "http://mcp-gateway:1975/mcp"
  oauth2:
    enabled: true
    tokenURL: "https://dex.local/token"
    credentialsSecretRef: "fleet-creds"
    scopes:
      - openid
      - groups
      - fleet-admin
`
			tmpDir, err := os.MkdirTemp("", "sp-cfg-*")
			Expect(err).ToNot(HaveOccurred())
			defer func() { _ = os.RemoveAll(tmpDir) }()

			cfgPath := filepath.Join(tmpDir, "config.yaml")
			Expect(os.WriteFile(cfgPath, []byte(yamlContent), 0o600)).To(Succeed())

			cfg, err := spconfig.LoadFromFile(cfgPath)
			Expect(err).ToNot(HaveOccurred())
			Expect(cfg.Fleet.OAuth2.Scopes).To(Equal([]string{"openid", "groups", "fleet-admin"}))
			Expect(cfg.Fleet.OAuth2.Enabled).To(BeTrue())
			Expect(cfg.Fleet.Endpoint).To(Equal("http://mcp-gateway:1975/mcp"))
		})

		It("defaults to nil scopes when omitted from YAML", func() {
			yamlContent := `
enrichment:
  cacheTtl: 5m
  timeout: 10s
classifier:
  regoConfigMapName: signalprocessing-policy
  regoConfigMapKey: policy.rego
  hotReloadInterval: 30s
fleet:
  endpoint: "http://mcp-gateway:1975/mcp"
  oauth2:
    enabled: true
    tokenURL: "https://dex.local/token"
    credentialsSecretRef: "fleet-creds"
`
			tmpDir, err := os.MkdirTemp("", "sp-cfg-*")
			Expect(err).ToNot(HaveOccurred())
			defer func() { _ = os.RemoveAll(tmpDir) }()

			cfgPath := filepath.Join(tmpDir, "config.yaml")
			Expect(os.WriteFile(cfgPath, []byte(yamlContent), 0o600)).To(Succeed())

			cfg, err := spconfig.LoadFromFile(cfgPath)
			Expect(err).ToNot(HaveOccurred())
			Expect(cfg.Fleet.OAuth2.Scopes).To(BeNil(),
				"scopes should be nil when not specified, letting DefaultFleetScopes provide defaults at runtime")
		})
	})
})
