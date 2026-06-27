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
	"testing"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/jordigilh/kubernaut/pkg/fleet/fmc/config"
)

func TestConfig(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "FMC Config Suite")
}

var _ = Describe("FMC ServiceConfig [BR-FLEET-054, ADR-030]", func() {
	Describe("DefaultServiceConfig", func() {
		It("UT-FMC-CFG-001: returns production defaults [SC-8]", func() {
			cfg := config.DefaultServiceConfig()

			Expect(cfg.Server.APIAddr).To(Equal(":8080"))
			Expect(cfg.Server.MetricsAddr).To(Equal(":8081"))
			Expect(cfg.MCPGateway.GatewayType).To(Equal("eaigw"))
			Expect(cfg.MCPGateway.Namespace).To(Equal("kubernaut-system"))
			Expect(cfg.Valkey.Addr).To(Equal("valkey:6379"))
			Expect(cfg.Sync.Interval).To(Equal(30 * time.Second))
			Expect(cfg.Sync.KeyTTL).To(Equal(45 * time.Second))
			Expect(cfg.Sync.ResourceKinds).To(ConsistOf(
				"Deployment", "StatefulSet", "DaemonSet", "Pod", "Service", "Node",
			))
			Expect(cfg.OAuth2.CredentialsDir).To(Equal("/etc/fmc/fleet-oauth2"))
			Expect(cfg.OAuth2.TokenURL).To(BeEmpty())
		})
	})

	Describe("LoadFromFile", func() {
		It("UT-FMC-CFG-002: returns defaults when path is empty [ADR-030]", func() {
			cfg, err := config.LoadFromFile("")
			Expect(err).NotTo(HaveOccurred())
			Expect(cfg.Valkey.Addr).To(Equal("valkey:6379"))
		})

		It("UT-FMC-CFG-003: parses valid YAML and overrides defaults [ADR-030]", func() {
			yamlContent := `
server:
  apiAddr: ":9090"
  metricsAddr: ":9091"
mcpGateway:
  endpoint: "http://gateway.svc:8080"
  gatewayType: "kuadrant"
  namespace: "fleet-system"
valkey:
  addr: "redis.fleet:6380"
sync:
  interval: 60s
  keyTtl: 90s
  resourceKinds:
    - Deployment
    - Pod
oauth2:
  tokenUrl: "https://keycloak.example.com/token"
  credentialsDir: "/custom/creds"
`
			tmpFile := writeYAMLToTemp(yamlContent)
			defer os.Remove(tmpFile)

			cfg, err := config.LoadFromFile(tmpFile)
			Expect(err).NotTo(HaveOccurred())
			Expect(cfg.Server.APIAddr).To(Equal(":9090"))
			Expect(cfg.Server.MetricsAddr).To(Equal(":9091"))
			Expect(cfg.MCPGateway.Endpoint).To(Equal("http://gateway.svc:8080"))
			Expect(cfg.MCPGateway.GatewayType).To(Equal("kuadrant"))
			Expect(cfg.MCPGateway.Namespace).To(Equal("fleet-system"))
			Expect(cfg.Valkey.Addr).To(Equal("redis.fleet:6380"))
			Expect(cfg.Sync.Interval).To(Equal(60 * time.Second))
			Expect(cfg.Sync.KeyTTL).To(Equal(90 * time.Second))
			Expect(cfg.Sync.ResourceKinds).To(Equal([]string{"Deployment", "Pod"}))
			Expect(cfg.OAuth2.TokenURL).To(Equal("https://keycloak.example.com/token"))
			Expect(cfg.OAuth2.CredentialsDir).To(Equal("/custom/creds"))
		})

		It("UT-FMC-CFG-004: partial YAML preserves unset defaults [ADR-030]", func() {
			yamlContent := `
mcpGateway:
  endpoint: "http://gw:8080"
oauth2:
  tokenUrl: "https://idp/token"
`
			tmpFile := writeYAMLToTemp(yamlContent)
			defer os.Remove(tmpFile)

			cfg, err := config.LoadFromFile(tmpFile)
			Expect(err).NotTo(HaveOccurred())
			Expect(cfg.MCPGateway.Endpoint).To(Equal("http://gw:8080"))
			Expect(cfg.Server.APIAddr).To(Equal(":8080"), "unset fields keep defaults")
			Expect(cfg.Valkey.Addr).To(Equal("valkey:6379"), "unset fields keep defaults")
			Expect(cfg.Sync.Interval).To(Equal(30 * time.Second), "unset fields keep defaults")
		})

		It("UT-FMC-CFG-005: returns error for non-existent file [IA-5]", func() {
			_, err := config.LoadFromFile("/nonexistent/config.yaml")
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("failed to read config file"))
		})

		It("UT-FMC-CFG-006: returns error for malformed YAML [SI-10]", func() {
			tmpFile := writeYAMLToTemp("invalid: [yaml: {broken")
			defer os.Remove(tmpFile)

			_, err := config.LoadFromFile(tmpFile)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("failed to parse config file"))
		})
	})

	Describe("Validate", func() {
		It("UT-FMC-CFG-007: passes with all required fields set [IA-5, SC-8]", func() {
			cfg := config.DefaultServiceConfig()
			cfg.MCPGateway.Endpoint = "http://gateway:8080"
			cfg.OAuth2.TokenURL = "https://idp/token"

			Expect(cfg.Validate()).To(Succeed())
		})

		It("UT-FMC-CFG-008: fails when mcpGateway.endpoint is empty [SC-7]", func() {
			cfg := config.DefaultServiceConfig()
			cfg.OAuth2.TokenURL = "https://idp/token"

			err := cfg.Validate()
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("mcpGateway.endpoint is required"))
		})

		It("UT-FMC-CFG-009: fails when valkey.addr is empty [SC-7]", func() {
			cfg := config.DefaultServiceConfig()
			cfg.MCPGateway.Endpoint = "http://gateway:8080"
			cfg.OAuth2.TokenURL = "https://idp/token"
			cfg.Valkey.Addr = ""

			err := cfg.Validate()
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("valkey.addr is required"))
		})

		It("UT-FMC-CFG-010: fails when oauth2.tokenUrl is empty — OAuth2 is mandatory [IA-5, SC-8]", func() {
			cfg := config.DefaultServiceConfig()
			cfg.MCPGateway.Endpoint = "http://gateway:8080"

			err := cfg.Validate()
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("oauth2.tokenUrl is required"))
			Expect(err.Error()).To(ContainSubstring("MCP Gateway requires authentication"))
		})
	})

	Describe("DefaultConfigPath", func() {
		It("UT-FMC-CFG-011: matches ADR-030 /etc/{service}/config.yaml convention", func() {
			Expect(config.DefaultConfigPath).To(Equal("/etc/fmc/config.yaml"))
		})
	})
})

func writeYAMLToTemp(content string) string {
	tmpDir := os.TempDir()
	tmpFile := filepath.Join(tmpDir, "fmc-test-config.yaml")
	err := os.WriteFile(tmpFile, []byte(content), 0644)
	Expect(err).NotTo(HaveOccurred())
	return tmpFile
}
