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

	mustYAML := func(body map[string]any) []byte {
		rendered, err := yaml.Marshal(body)
		Expect(err).ToNot(HaveOccurred())
		return rendered
	}

	writeFile := func(contents []byte) string {
		path := filepath.Join(GinkgoT().TempDir(), "config.yaml")
		Expect(os.WriteFile(path, contents, 0o600)).To(Succeed())
		return path
	}

	writeConfig := func(sources any) string {
		body := baseConfig()
		if sources != nil {
			body["sources"] = sources
		}
		return writeFile(mustYAML(body))
	}

	// writeRawConfig exists for YAML the fixture map cannot express: merge keys
	// and anchors, which are exactly the inputs that could hide a key from a
	// literal-only check.
	writeRawConfig := func(sourcesYAML string) string {
		body := mustYAML(baseConfig())
		return writeFile(append(body, []byte(sourcesYAML)...))
	}

	// tokenFile creates a readable token file so the tokenPath checks exercise
	// the read path, not just the path shape.
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

		// UT-FMC-SRC-2661-009
		DescribeTable("UT-FMC-SRC-2661-009 [BR-INTEGRATION-065]: a present sources block must name the source it selects",
			func(sourcesYAML, wantSubstring string) {
				_, err := config.LoadFromFile(writeRawConfig(sourcesYAML))

				Expect(err).To(HaveOccurred(),
					"presence selects a source; it must never resolve back to MCP silently")
				Expect(err.Error()).To(ContainSubstring(wantSubstring))
			},
			Entry("empty mapping", "sources: {}\n", "names no source"),
			Entry("null sources", "sources: null\n", "declared but empty"),
			Entry("null acm", "sources:\n  acm: null\n", "names no source"),
		)

		// UT-FMC-SRC-2661-009 (continued): a named but incomplete ACM source is
		// incomplete, not absent — it must fail validation, never load as MCP.
		// Compatibility coverage: the pre-declared implementation also selected
		// ACM here, so this entry guards the shape, it is not regression evidence
		// for the `declared` flag (the empty-map and null entries above are).
		It("UT-FMC-SRC-2661-009 [BR-INTEGRATION-065]: an empty acm mapping loads as ACM and fails validation", func() {
			cfg, err := config.LoadFromFile(writeRawConfig("sources:\n  acm: {}\n"))

			Expect(err).ToNot(HaveOccurred(),
				"an empty mapping is a present source; incompleteness is a validation failure")
			Expect(cfg.SourceKind()).To(Equal(config.SourceACM),
				"a named source must never resolve back to MCP")
			Expect(cfg.Validate()).To(MatchError(ContainSubstring("sources.acm.endpoint")))
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
		DescribeTable("UT-FMC-SRC-2661-005 [FedRAMP IA-5]: rejects a tokenPath that is relative, empty, missing or a directory",
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
			Entry("directory", func() string { return GinkgoT().TempDir() }),
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

	Describe("closed key set (D4.1a rules 2-3)", func() {
		// UT-FMC-SRC-2661-007
		DescribeTable("UT-FMC-SRC-2661-007 [FedRAMP CM-6]: an unknown or injected key under sources.acm fails loading, not silently disables a guard",
			func(sourcesYAML func(token string) string, wantSubstring string) {
				_, err := config.LoadFromFile(writeRawConfig(sourcesYAML(tokenFile())))

				Expect(err).To(HaveOccurred())
				Expect(err.Error()).To(ContainSubstring(wantSubstring),
					"the error must name the block the operator has to fix")
			},
			Entry("literal typo", func(token string) string {
				// A plausible typo that would otherwise be dropped silently.
				return "sources:\n  acm:\n    endpoint: https://search-api.example:4010\n" +
					"    tokenPath: " + token + "\n    endpointURL: https://ignored.example:4010\n"
			}, "sources.acm"),
			Entry("unknown key injected through a YAML merge key", func(token string) string {
				// The lenient decode resolves the anchor, so a literal-key scan
				// would never see endpointURL.
				return "acmDefaults: &acmDefaults\n" +
					"  endpoint: https://search-api.example:4010\n" +
					"  tokenPath: " + token + "\n" +
					"  endpointURL: https://ignored.example:4010\n" +
					"sources:\n  acm:\n    <<: *acmDefaults\n"
			}, "sources.acm"),
			Entry("quoted merge key is a literal, unknown key", func(token string) string {
				// `"<<"` is tagged !!str, not !!merge, so it must be rejected as
				// an unknown key rather than skipped as a merge.
				return "sources:\n  acm:\n    endpoint: https://search-api.example:4010\n" +
					"    tokenPath: " + token + "\n    \"<<\": typo\n"
			}, "sources.acm"),
			Entry("sources is a scalar", func(string) string { return "sources: acm\n" }, "sources must be a mapping"),
			Entry("sources.acm is a scalar", func(string) string { return "sources:\n  acm: https://search-api.example:4010\n" }, "sources.acm"),
		)

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

		// UT-FMC-SRC-2661-010 (precedence): flattening must reproduce YAML's
		// merge rules, or a merged credential/endpoint could silently win.
		It("UT-FMC-SRC-2661-010 [FedRAMP CM-6]: a direct key wins over a merge, and an earlier merge wins over a later one", func() {
			token := tokenFile()

			direct, err := config.LoadFromFile(writeRawConfig(
				"first: &first\n  endpoint: https://first.example:4010\n" +
					"second: &second\n  endpoint: https://second.example:4010\n" +
					"sources:\n  acm:\n    tokenPath: " + token + "\n" +
					"    endpoint: https://direct.example:4010\n" +
					"    <<: [*first, *second]\n"))
			Expect(err).ToNot(HaveOccurred())
			Expect(direct.Sources.ACM.Endpoint).To(Equal("https://direct.example:4010"),
				"a directly declared key must win over merged keys")

			mergedOnly, err := config.LoadFromFile(writeRawConfig(
				"first: &first\n  endpoint: https://first.example:4010\n" +
					"second: &second\n  endpoint: https://second.example:4010\n" +
					"sources:\n  acm:\n    tokenPath: " + token + "\n" +
					"    <<: [*first, *second]\n"))
			Expect(err).ToNot(HaveOccurred())
			Expect(mergedOnly.Sources.ACM.Endpoint).To(Equal("https://first.example:4010"),
				"the earlier merge target must win, matching yaml.v3")
		})

		// UT-FMC-SRC-2661-010
		It("UT-FMC-SRC-2661-010 [FedRAMP CM-6]: a merged sources block is validated, not skipped", func() {
			// The merge injects sources itself, so the block never appears as a
			// literal root key.
			_, err := config.LoadFromFile(writeRawConfig(
				"shared: &shared\n  sources:\n    rancher:\n      endpoint: https://rancher.example\n" +
					"<<: *shared\n"))

			Expect(err).To(HaveOccurred(),
				"a merged sources block must reach the closed-key check")
			Expect(err.Error()).To(ContainSubstring("rancher"))
		})
	})
})
