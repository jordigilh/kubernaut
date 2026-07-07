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

package fleet_test

import (
	"testing"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"gopkg.in/yaml.v3"

	"github.com/jordigilh/kubernaut/pkg/fleet"
)

func TestFleet(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Fleet Package Suite")
}

var _ = Describe("FleetConfig shared type (Phase E)", func() {
	It("UT-FLEET-CFG-001 [CM-6]: FleetConfig provides unified configuration via Backend+Endpoint", func() {
		cfg := fleet.FleetConfig{
			Enabled:  true,
			Backend:  "fleetmetadatacache",
			Endpoint: "http://fmc:8080",
		}

		Expect(cfg.Enabled).To(BeTrue())
		Expect(cfg.Backend).To(Equal("fleetmetadatacache"))
		Expect(cfg.Endpoint).To(Equal("http://fmc:8080"))
	})

	It("UT-FLEET-CFG-002 [CM-6]: Validate rejects empty Endpoint for non-FMC backends", func() {
		cfg := fleet.FleetConfig{
			Enabled: true,
			Backend: "acm",
		}

		err := cfg.Validate()
		Expect(err).To(HaveOccurred(),
			"FleetConfig.Validate must reject empty Endpoint for acm backend (CM-6)")
	})

	It("UT-FLEET-CFG-003 [CM-6]: Validate accepts disabled fleet without Backend/Endpoint", func() {
		cfg := fleet.FleetConfig{
			Enabled: false,
		}

		err := cfg.Validate()
		Expect(err).ToNot(HaveOccurred(),
			"disabled fleet should not require Backend or Endpoint")
	})
})

var _ = Describe("FleetConfig — BackendValkey removal (Phase 3)", func() {
	It("UT-SF-054-002 [CM-6]: Validate rejects BackendValkey as unsupported", func() {
		cfg := fleet.FleetConfig{
			Enabled:  true,
			Backend:  "valkey",
			Endpoint: "valkey:6379",
		}

		err := cfg.Validate()
		Expect(err).To(HaveOccurred(),
			"valkey backend must be rejected after legacy removal")
		Expect(err.Error()).To(ContainSubstring("unsupported backend"))
	})

	It("UT-SF-054-003 [CM-6]: EffectiveEndpoint returns explicit Endpoint when set, auto-derives for fmc when empty", func() {
		cfg := fleet.FleetConfig{
			Enabled:  true,
			Backend:  "fleetmetadatacache",
			Endpoint: "http://fmc:8080",
		}
		Expect(cfg.EffectiveEndpoint()).To(Equal("http://fmc:8080"))

		cfgEmpty := fleet.FleetConfig{
			Enabled: true,
			Backend: "fleetmetadatacache",
		}
		Expect(cfgEmpty.EffectiveEndpoint()).To(ContainSubstring("fleetmetadatacache-service"),
			"FMC backend auto-derives endpoint from namespace when Endpoint is empty")

		cfgACMEmpty := fleet.FleetConfig{
			Enabled: true,
			Backend: "acm",
		}
		Expect(cfgACMEmpty.EffectiveEndpoint()).To(BeEmpty(),
			"non-FMC backends must return empty when Endpoint is not set")
	})
})

var _ = Describe("FleetConfig adapter pattern (Phase 2)", func() {
	It("UT-FLEET-CFG-010 [CM-6]: FleetConfig exposes Backend and Endpoint fields", func() {
		cfg := fleet.FleetConfig{
			Enabled:  true,
			Backend:  "fleetmetadatacache",
			Endpoint: "http://fmc.kubernaut.svc:8080",
		}

		Expect(cfg.Backend).To(Equal("fleetmetadatacache"))
		Expect(cfg.Endpoint).To(Equal("http://fmc.kubernaut.svc:8080"))
	})

	It("UT-FLEET-CFG-011 [CM-6]: Validate rejects empty Endpoint for non-FMC backends", func() {
		cfg := fleet.FleetConfig{
			Enabled:  true,
			Backend:  "acm",
			Endpoint: "",
		}

		err := cfg.Validate()
		Expect(err).To(HaveOccurred(),
			"must reject empty Endpoint for acm backend")
	})

	It("UT-FLEET-CFG-012 [CM-6]: Validate accepts disabled fleet without Backend/Endpoint", func() {
		cfg := fleet.FleetConfig{
			Enabled: false,
		}

		err := cfg.Validate()
		Expect(err).ToNot(HaveOccurred())
	})

	It("UT-FLEET-CFG-013 [CM-6]: Validate rejects unsupported Backend value", func() {
		cfg := fleet.FleetConfig{
			Enabled:  true,
			Backend:  "unsupported",
			Endpoint: "http://something:8080",
		}

		err := cfg.Validate()
		Expect(err).To(HaveOccurred(),
			"must reject unknown backend types")
	})

	It("UT-FLEET-CFG-014 [CM-6]: Validate accepts fmc backend", func() {
		cfg := fleet.FleetConfig{
			Enabled:  true,
			Backend:  "fleetmetadatacache",
			Endpoint: "http://fmc.kubernaut.svc:8080",
		}

		err := cfg.Validate()
		Expect(err).ToNot(HaveOccurred())
	})

	It("UT-FLEET-CFG-015 [CM-6]: Validate accepts acm backend", func() {
		cfg := fleet.FleetConfig{
			Enabled:   true,
			Backend:   "acm",
			Endpoint:  "https://search-search-api.open-cluster-management.svc:4010",
			TokenPath: "/etc/gateway/acm-token/token",
		}

		err := cfg.Validate()
		Expect(err).ToNot(HaveOccurred())
	})
})

// #1556: ACM Search mandatorily requires bearer-token auth, but the acm.Client
// adapter never sent an Authorization header and nothing forced operators to
// configure one. This left every ACM-backed deployment silently unauthenticated.
// TokenPath hard-requirement closes that gap at config-validation time instead
// of failing at request time inside the ACM Search backend.
var _ = Describe("FleetConfig.TokenPath — acm backend bearer-token requirement (BR-INTEGRATION-065, #1556)", func() {
	It("UT-FLEET-CFG-070 [IA-5,AC-4]: Validate rejects acm backend with empty TokenPath", func() {
		cfg := fleet.FleetConfig{
			Enabled:  true,
			Backend:  "acm",
			Endpoint: "https://search-search-api.open-cluster-management.svc:4010",
		}

		err := cfg.Validate()
		Expect(err).To(HaveOccurred(),
			"IA-5/AC-4: ACM Search mandatorily requires bearer-token auth; starting "+
				"without TokenPath would silently send unauthenticated requests")
		Expect(err.Error()).To(ContainSubstring("tokenPath"))
	})

	It("UT-FLEET-CFG-071 [IA-5,AC-4]: Validate accepts acm backend with TokenPath set", func() {
		cfg := fleet.FleetConfig{
			Enabled:   true,
			Backend:   "acm",
			Endpoint:  "https://search-search-api.open-cluster-management.svc:4010",
			TokenPath: "/etc/gateway/acm-token/token",
		}

		Expect(cfg.Validate()).ToNot(HaveOccurred())
	})
})

var _ = Describe("FleetConfig MCPGatewayType (MCP Gateway Adapter)", func() {
	It("UT-FLEET-CFG-030 [CM-6]: Validate accepts valid MCPGatewayType eaigw", func() {
		cfg := fleet.FleetConfig{
			Enabled:            true,
			Backend:            "fleetmetadatacache",
			Endpoint:           "http://fmc:8080",
			MCPGatewayEndpoint: "http://gw:8080/mcp",
			MCPGatewayType:     fleet.GatewayEAIGW,
		}
		Expect(cfg.Validate()).ToNot(HaveOccurred())
	})

	It("UT-FLEET-CFG-031 [CM-6]: Validate accepts valid MCPGatewayType kuadrant", func() {
		cfg := fleet.FleetConfig{
			Enabled:            true,
			Backend:            "fleetmetadatacache",
			Endpoint:           "http://fmc:8080",
			MCPGatewayEndpoint: "http://gw:8080/mcp",
			MCPGatewayType:     fleet.GatewayKuadrant,
		}
		Expect(cfg.Validate()).ToNot(HaveOccurred())
	})

	It("UT-FLEET-CFG-032 [CM-6]: Validate rejects unsupported MCPGatewayType with descriptive error", func() {
		cfg := fleet.FleetConfig{
			Enabled:            true,
			Backend:            "fleetmetadatacache",
			Endpoint:           "http://fmc:8080",
			MCPGatewayEndpoint: "http://gw:8080/mcp",
			MCPGatewayType:     fleet.MCPGatewayType("invalid-gw"),
		}
		err := cfg.Validate()
		Expect(err).To(HaveOccurred())
		Expect(err.Error()).To(ContainSubstring("unsupported mcpGatewayType"))
	})

	It("UT-FLEET-CFG-033 [CM-6]: EffectiveMCPGatewayType returns empty when field is empty (fleet disabled)", func() {
		cfg := fleet.FleetConfig{}
		Expect(cfg.EffectiveMCPGatewayType()).To(Equal(fleet.MCPGatewayType("")),
			"empty MCPGatewayType means fleet is disabled, no default")
	})

	It("UT-FLEET-CFG-034 [CM-6]: Validate skips MCPGatewayType check when MCPGatewayEndpoint is empty", func() {
		cfg := fleet.FleetConfig{
			Enabled:  true,
			Backend:  "fleetmetadatacache",
			Endpoint: "http://fmc:8080",
		}
		Expect(cfg.Validate()).ToNot(HaveOccurred())
	})

	It("UT-FLEET-CFG-035 [CM-6]: Empty MCPGatewayType with non-empty endpoint fails validation", func() {
		cfg := fleet.FleetConfig{
			Enabled:            true,
			Backend:            "fleetmetadatacache",
			Endpoint:           "http://fmc:8080",
			MCPGatewayEndpoint: "http://gw:8080/mcp",
			MCPGatewayType:     "",
		}
		err := cfg.Validate()
		Expect(err).To(HaveOccurred())
		Expect(err.Error()).To(ContainSubstring("mcpGatewayType is required when fleet is enabled"))
	})

	// IT-AF-054-005/IT-EM-054-004 regression: AF and EM only ever set
	// MCPGatewayEndpoint (remote K8s reads) — they never call the Backend/
	// Endpoint federated scope-check adapter (that's GW/RO's job; AF/EM
	// discover clusters directly via ClusterRegistry watching Backend CRDs).
	// Before this fix, Validate() unconditionally required Backend+Endpoint
	// whenever Enabled=true, which would have broken AF/EM startup the
	// moment an operator enabled fleet MCP routing without also configuring
	// an unused FMC/ACM backend.
	It("UT-FLEET-CFG-036 [CM-6]: Validate accepts MCPGatewayEndpoint-only config without Backend/Endpoint", func() {
		cfg := fleet.FleetConfig{
			Enabled:            true,
			MCPGatewayEndpoint: "http://gw:8080/mcp",
			MCPGatewayType:     fleet.GatewayEAIGW,
		}
		Expect(cfg.Validate()).ToNot(HaveOccurred(),
			"AF/EM only need MCPGatewayEndpoint for remote reads; requiring an unused Backend/Endpoint blocks their startup")
	})

	It("UT-FLEET-CFG-037 [CM-6]: Validate rejects Enabled=true with neither Backend/Endpoint nor MCPGatewayEndpoint configured", func() {
		cfg := fleet.FleetConfig{
			Enabled: true,
		}
		err := cfg.Validate()
		Expect(err).To(HaveOccurred(),
			"enabling fleet without configuring either capability is a misconfiguration")
	})
})

// GW and RO each rely on BOTH FleetConfig capabilities to operate correctly:
// Backend/Endpoint (federated scope-check: "is this resource managed?") and
// MCPGatewayEndpoint (remote reads: GW's owner-chain metadata, RO's
// pre-remediation spec hash). Configuring only one leaves them silently
// degraded to local-only behavior for fleet-routed resources — exactly the
// class of bug this investigation started with. ValidateFullFederation is a
// stricter, opt-in check services like GW/RO call in addition to Validate().
var _ = Describe("FleetConfig.ValidateFullFederation — GW/RO dual-capability requirement", func() {
	It("UT-FLEET-CFG-040: accepts disabled fleet without either capability", func() {
		cfg := fleet.FleetConfig{Enabled: false}
		Expect(cfg.ValidateFullFederation()).ToNot(HaveOccurred())
	})

	It("UT-FLEET-CFG-041: rejects Enabled=true with only Backend/Endpoint configured (no MCPGatewayEndpoint)", func() {
		cfg := fleet.FleetConfig{
			Enabled:  true,
			Backend:  "fleetmetadatacache",
			Endpoint: "http://fmc:8080",
		}
		err := cfg.ValidateFullFederation()
		Expect(err).To(HaveOccurred(),
			"GW/RO cannot operate without degradation unless both capabilities are configured")
		Expect(err.Error()).To(ContainSubstring("mcpGatewayEndpoint"))
	})

	It("UT-FLEET-CFG-042: rejects Enabled=true with only MCPGatewayEndpoint configured (no Backend/Endpoint)", func() {
		cfg := fleet.FleetConfig{
			Enabled:            true,
			MCPGatewayEndpoint: "http://gw:8080/mcp",
			MCPGatewayType:     fleet.GatewayEAIGW,
		}
		err := cfg.ValidateFullFederation()
		Expect(err).To(HaveOccurred())
		Expect(err.Error()).To(ContainSubstring("backend"))
	})

	It("UT-FLEET-CFG-043: accepts Enabled=true with both capabilities fully configured", func() {
		cfg := fleet.FleetConfig{
			Enabled:            true,
			Backend:            "fleetmetadatacache",
			Endpoint:           "http://fmc:8080",
			MCPGatewayEndpoint: "http://gw:8080/mcp",
			MCPGatewayType:     fleet.GatewayEAIGW,
		}
		Expect(cfg.ValidateFullFederation()).ToNot(HaveOccurred())
	})
})

// Root cause of the "tls: failed to verify certificate: x509: certificate
// signed by unknown authority" failures observed against an in-cluster
// Keycloak/Dex OIDC provider from GW/RO/SP/WE/EM/AF: FleetOAuth2Config had
// no way to name a CA bundle for the token-fetch HTTP client, unlike FMC's
// own (separate) OAuth2Config.TlsCaFile. This Describe block locks in the
// field's presence and YAML round-trip so a future refactor cannot silently
// drop it again.
var _ = Describe("FleetOAuth2Config.TLSCAFile (BR-INTEGRATION-065)", func() {
	It("UT-FLEET-CFG-050 [SC-8]: TLSCAFile is settable and defaults to empty (system CA trust)", func() {
		cfg := fleet.FleetOAuth2Config{
			Enabled:  true,
			TokenURL: "https://keycloak:8443/realms/kubernaut-fleet/protocol/openid-connect/token",
		}
		Expect(cfg.TLSCAFile).To(BeEmpty(),
			"zero-value TLSCAFile must fall back to the system CA trust store")

		cfg.TLSCAFile = "/etc/gateway/tls-ca/ca.crt"
		Expect(cfg.TLSCAFile).To(Equal("/etc/gateway/tls-ca/ca.crt"))
	})

	It("UT-FLEET-CFG-051 [SC-8]: TLSCAFile survives a YAML round-trip via the tlsCAFile key", func() {
		cfg := fleet.FleetOAuth2Config{
			Enabled:   true,
			TokenURL:  "https://keycloak:8443/token",
			TLSCAFile: "/etc/gateway/tls-ca/ca.crt",
		}
		data, err := yaml.Marshal(cfg)
		Expect(err).ToNot(HaveOccurred())
		Expect(string(data)).To(ContainSubstring("tlsCAFile: /etc/gateway/tls-ca/ca.crt"))

		var roundTripped fleet.FleetOAuth2Config
		Expect(yaml.Unmarshal(data, &roundTripped)).To(Succeed())
		Expect(roundTripped.TLSCAFile).To(Equal(cfg.TLSCAFile))
	})
})

// Readiness gate Wave 1 (#1553): FleetConfig.Validate() historically never
// checked OAuth2 field consistency, unlike the local FleetOAuth2 pairing
// check that SP/WE/KA already have. This left GW/RO/AF/EM able to start
// with oauth2.enabled=true but a missing tokenURL/credentialsSecretRef,
// silently sending unauthenticated requests to the MCP Gateway instead of
// failing closed at startup. Mirrors the existing SP/WE check exactly
// (pkg/signalprocessing/config/config.go, pkg/workflowexecution/config/config.go).
var _ = Describe("FleetConfig.Validate OAuth2 pairing (BR-INTEGRATION-065, #1553)", func() {
	It("UT-FLEET-CFG-060: accepts fleet disabled with oauth2.enabled=true but no tokenURL (Validate short-circuits on !Enabled)", func() {
		cfg := fleet.FleetConfig{
			Enabled: false,
			OAuth2:  fleet.FleetOAuth2Config{Enabled: true},
		}
		Expect(cfg.Validate()).To(Succeed())
	})

	It("UT-FLEET-CFG-061: accepts fleet enabled with oauth2.enabled=false regardless of empty OAuth2 fields", func() {
		cfg := fleet.FleetConfig{
			Enabled:            true,
			MCPGatewayEndpoint: "https://mcp-gateway:8443",
			MCPGatewayType:     "eaigw",
			OAuth2:             fleet.FleetOAuth2Config{Enabled: false},
		}
		Expect(cfg.Validate()).To(Succeed())
	})

	It("UT-FLEET-CFG-062: rejects oauth2.enabled=true with empty tokenURL", func() {
		cfg := fleet.FleetConfig{
			Enabled:            true,
			MCPGatewayEndpoint: "https://mcp-gateway:8443",
			MCPGatewayType:     "eaigw",
			OAuth2: fleet.FleetOAuth2Config{
				Enabled:              true,
				CredentialsSecretRef: "fleet-oauth2-creds",
			},
		}
		err := cfg.Validate()
		Expect(err).To(HaveOccurred())
		Expect(err.Error()).To(ContainSubstring("tokenURL"))
	})

	It("UT-FLEET-CFG-063: rejects oauth2.enabled=true with empty credentialsSecretRef", func() {
		cfg := fleet.FleetConfig{
			Enabled:            true,
			MCPGatewayEndpoint: "https://mcp-gateway:8443",
			MCPGatewayType:     "eaigw",
			OAuth2: fleet.FleetOAuth2Config{
				Enabled:  true,
				TokenURL: "https://keycloak:8443/realms/kubernaut-fleet/protocol/openid-connect/token",
			},
		}
		err := cfg.Validate()
		Expect(err).To(HaveOccurred())
		Expect(err.Error()).To(ContainSubstring("credentialsSecretRef"))
	})

	It("UT-FLEET-CFG-064: accepts oauth2.enabled=true with both tokenURL and credentialsSecretRef set", func() {
		cfg := fleet.FleetConfig{
			Enabled:            true,
			MCPGatewayEndpoint: "https://mcp-gateway:8443",
			MCPGatewayType:     "eaigw",
			OAuth2: fleet.FleetOAuth2Config{
				Enabled:              true,
				TokenURL:             "https://keycloak:8443/realms/kubernaut-fleet/protocol/openid-connect/token",
				CredentialsSecretRef: "fleet-oauth2-creds",
			},
		}
		Expect(cfg.Validate()).To(Succeed())
	})
})

var _ = Describe("FleetConfig FMC endpoint auto-derivation (BR-INTEGRATION-065)", func() {
	It("UT-FLEET-CFG-020 [CM-6]: EffectiveEndpoint derives FMC URL from POD_NAMESPACE when Endpoint is empty", func() {
		GinkgoT().Setenv("POD_NAMESPACE", "kubernaut-system")

		cfg := fleet.FleetConfig{
			Enabled: true,
			Backend: "fleetmetadatacache",
		}

		Expect(cfg.EffectiveEndpoint()).To(Equal("http://fleetmetadatacache-service.kubernaut-system.svc.cluster.local:8080"),
			"FMC endpoint must be auto-derived from POD_NAMESPACE when not explicitly set")
	})

	It("UT-FLEET-CFG-021 [CM-6]: EffectiveEndpoint returns explicit Endpoint even when POD_NAMESPACE is set", func() {
		GinkgoT().Setenv("POD_NAMESPACE", "kubernaut-system")

		cfg := fleet.FleetConfig{
			Enabled:  true,
			Backend:  "fleetmetadatacache",
			Endpoint: "http://custom-fmc:9090",
		}

		Expect(cfg.EffectiveEndpoint()).To(Equal("http://custom-fmc:9090"),
			"explicit Endpoint must take precedence over auto-derivation")
	})

	It("UT-FLEET-CFG-022 [CM-6]: EffectiveEndpoint falls back to 'default' namespace when POD_NAMESPACE is unset", func() {
		GinkgoT().Setenv("POD_NAMESPACE", "")

		cfg := fleet.FleetConfig{
			Enabled: true,
			Backend: "fleetmetadatacache",
		}

		Expect(cfg.EffectiveEndpoint()).To(Equal("http://fleetmetadatacache-service.default.svc.cluster.local:8080"),
			"must use 'default' namespace when POD_NAMESPACE is not set and SA mount unavailable")
	})

	It("UT-FLEET-CFG-023 [CM-6]: EffectiveEndpoint does NOT auto-derive for acm backend", func() {
		GinkgoT().Setenv("POD_NAMESPACE", "kubernaut-system")

		cfg := fleet.FleetConfig{
			Enabled: true,
			Backend: "acm",
		}

		Expect(cfg.EffectiveEndpoint()).To(BeEmpty(),
			"auto-derivation must only apply to fmc backend, not acm")
	})

	It("UT-FLEET-CFG-024 [CM-6]: Validate accepts fmc backend without explicit Endpoint (auto-derived)", func() {
		GinkgoT().Setenv("POD_NAMESPACE", "kubernaut-system")

		cfg := fleet.FleetConfig{
			Enabled: true,
			Backend: "fleetmetadatacache",
		}

		err := cfg.Validate()
		Expect(err).ToNot(HaveOccurred(),
			"Validate must accept fmc backend with auto-derived endpoint")
	})

	It("UT-FLEET-CFG-025 [CM-6]: Validate still rejects acm backend without explicit Endpoint", func() {
		GinkgoT().Setenv("POD_NAMESPACE", "kubernaut-system")

		cfg := fleet.FleetConfig{
			Enabled: true,
			Backend: "acm",
		}

		err := cfg.Validate()
		Expect(err).To(HaveOccurred(),
			"acm backend must still require explicit Endpoint")
	})
})
