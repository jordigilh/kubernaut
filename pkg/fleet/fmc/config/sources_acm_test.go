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

	"gopkg.in/yaml.v3"

	"github.com/jordigilh/kubernaut/pkg/fleet/fmc/config"
)

// DD-FLEET-017 D4.1a: `sources.acm` is FMC's cross-repo contract with the
// operator. The operator renders resolved artifact paths only, so FMC must
// receive paths, never policy, and must fail closed at startup rather than at
// the first sync.
var _ = Describe("FMC ACM source configuration (DD-FLEET-017 D4.1a)", func() {
	// baseConfig is a config that is already valid for every non-source key,
	// so a failing spec can only be failing because of the sources block.
	baseConfig := func() map[string]any {
		return map[string]any{
			"mcpGateway": map[string]any{
				"endpoint":    "https://mcp-gateway.kubernaut-system.svc:8080/mcp",
				"gatewayType": "eaigw",
				"namespace":   "kubernaut-system",
			},
			"valkey": map[string]any{"addr": "valkey:6379"},
			"oauth2": map[string]any{"tokenUrl": "https://keycloak/token"},
		}
	}

	// mustYAML renders the fixture map, keeping the fixture readable as Go
	// instead of a second YAML dialect inside the spec.
	mustYAML := func(body map[string]any) []byte {
		rendered, err := yaml.Marshal(body)
		Expect(err).ToNot(HaveOccurred())
		return rendered
	}

	writeConfig := func(sources any) string {
		body := baseConfig()
		if sources != nil {
			body["sources"] = sources
		}
		path := filepath.Join(GinkgoT().TempDir(), "config.yaml")
		Expect(os.WriteFile(path, mustYAML(body), 0o600)).To(Succeed())
		return path
	}

	// tokenFile creates a readable token file so the tokenPath checks exercise
	// readability, not just the path shape.
	tokenFile := func() string {
		path := filepath.Join(GinkgoT().TempDir(), "token")
		Expect(os.WriteFile(path, []byte("test-token-value"), 0o600)).To(Succeed())
		return path
	}

	Describe("presence", func() {
		// UT-FMC-SRC-2661-001
		It("UT-FMC-SRC-2661-001 [BR-INTEGRATION-065]: absent sources keeps today's MCP collector", func() {
			cfg, err := config.LoadFromFile(writeConfig(nil))

			Expect(err).ToNot(HaveOccurred())
			Expect(cfg.Sources.ACM).To(BeNil(),
				"no sources key must leave the rendered config identical to today's")
			Expect(cfg.Validate()).To(Succeed())
			Expect(cfg.SourceKind()).To(Equal(config.SourceMCP),
				"absence means the MCP/GitOps collector still runs")
		})

		// UT-FMC-SRC-2661-002
		It("UT-FMC-SRC-2661-002 [BR-INTEGRATION-065]: a complete sources.acm block selects the ACM collector", func() {
			token := tokenFile()
			cfg, err := config.LoadFromFile(writeConfig(map[string]any{
				"acm": map[string]any{
					"endpoint":  "https://search-api.example:4010",
					"tokenPath": token,
					"caFile":    "/etc/fleet-tls/acm-source/ca.crt",
				},
			}))

			Expect(err).ToNot(HaveOccurred())
			Expect(cfg.Sources.ACM).ToNot(BeNil())
			Expect(cfg.Sources.ACM.Endpoint).To(Equal("https://search-api.example:4010"))
			Expect(cfg.Sources.ACM.TokenPath).To(Equal(token))
			Expect(cfg.Sources.ACM.CAFile).To(Equal("/etc/fleet-tls/acm-source/ca.crt"))
			Expect(cfg.SourceKind()).To(Equal(config.SourceACM))
		})

		// UT-FMC-SRC-2661-003
		It("UT-FMC-SRC-2661-003 [BR-INTEGRATION-065]: caFile is optional and absent means system trust", func() {
			token := tokenFile()
			cfg, err := config.LoadFromFile(writeConfig(map[string]any{
				"acm": map[string]any{
					"endpoint":  "https://search-api.example:4010",
					"tokenPath": token,
				},
			}))

			Expect(err).ToNot(HaveOccurred())
			Expect(cfg.Sources.ACM.CAFile).To(BeEmpty())
			Expect(cfg.Validate()).To(Succeed(),
				"an absent caFile must fall back to system trust, not fail startup")
		})
	})

	Describe("fail closed at startup (D4.1a rule 3)", func() {
		// UT-FMC-SRC-2661-004
		DescribeTable("UT-FMC-SRC-2661-004 [FedRAMP SC-8, SI-10]: rejects a non-HTTPS, non-absolute or host-less endpoint",
			func(endpoint string) {
				cfg, err := config.LoadFromFile(writeConfig(map[string]any{
					"acm": map[string]any{"endpoint": endpoint, "tokenPath": tokenFile()},
				}))
				Expect(err).ToNot(HaveOccurred(), "loading must succeed; validation is the fail-closed gate")
				Expect(cfg.Validate()).To(MatchError(ContainSubstring("sources.acm.endpoint")))
			},
			Entry("plain http", "http://search-api.example:4010"),
			Entry("relative", "search-api.example:4010"),
			Entry("no host", "https:///graphql"),
			Entry("empty", ""),
		)

		// UT-FMC-SRC-2661-005
		DescribeTable("UT-FMC-SRC-2661-005 [FedRAMP IA-5]: rejects a tokenPath that is relative, missing or unreadable",
			func(tokenPath func() string) {
				cfg, err := config.LoadFromFile(writeConfig(map[string]any{
					"acm": map[string]any{"endpoint": "https://search-api.example:4010", "tokenPath": tokenPath()},
				}))
				Expect(err).ToNot(HaveOccurred())
				err = cfg.Validate()
				Expect(err).To(HaveOccurred())
				Expect(err.Error()).To(ContainSubstring("sources.acm.tokenPath"))
				Expect(err.Error()).ToNot(ContainSubstring("test-token-value"),
					"a token must never appear in an error")
			},
			Entry("relative", func() string { return "token" }),
			Entry("empty", func() string { return "" }),
			Entry("missing file", func() string { return filepath.Join(GinkgoT().TempDir(), "absent") }),
		)

		// UT-FMC-SRC-2661-006
		It("UT-FMC-SRC-2661-006 [FedRAMP SC-8]: rejects a relative caFile", func() {
			cfg, err := config.LoadFromFile(writeConfig(map[string]any{
				"acm": map[string]any{
					"endpoint":  "https://search-api.example:4010",
					"tokenPath": tokenFile(),
					"caFile":    "ca.crt",
				},
			}))
			Expect(err).ToNot(HaveOccurred())
			Expect(cfg.Validate()).To(MatchError(ContainSubstring("sources.acm.caFile")))
		})
	})

	Describe("strict decoding (D4.1a rule 3)", func() {
		// UT-FMC-SRC-2661-007
		It("UT-FMC-SRC-2661-007 [FedRAMP CM-6]: an unknown key under sources.acm fails loading, not silently disables a guard", func() {
			_, err := config.LoadFromFile(writeConfig(map[string]any{
				"acm": map[string]any{
					"endpoint":  "https://search-api.example:4010",
					"tokenPath": tokenFile(),
					// A plausible typo that would otherwise be dropped silently.
					"endpointURL": "https://ignored.example:4010",
				},
			}))

			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("endpointURL"))
		})

		// UT-FMC-SRC-2661-008
		It("UT-FMC-SRC-2661-008 [BR-INTEGRATION-065]: a second or unknown source is a configuration error", func() {
			_, err := config.LoadFromFile(writeConfig(map[string]any{
				"acm": map[string]any{
					"endpoint":  "https://search-api.example:4010",
					"tokenPath": tokenFile(),
				},
				"rancher": map[string]any{"endpoint": "https://rancher.example"},
			}))

			Expect(err).To(HaveOccurred(),
				"at most one source may be declared; a second one must not be ignored")
			Expect(err.Error()).To(ContainSubstring("rancher"))
		})
	})
})
