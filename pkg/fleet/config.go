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

// Package fleet provides shared types and factories for multi-cluster federation.
// Services that participate in fleet operations (GW, RO, FMC) import this
// package for consistent configuration and connection management.
package fleet

import (
	"fmt"
	"os"
	"strings"

	"github.com/jordigilh/kubernaut/pkg/fleet/registry"
)

// Supported backend types for federated scope checking.
const (
	// BackendFMC uses the Fleet Metadata Cache HTTP service (ADR-068).
	BackendFMC = "fleetmetadatacache"
	// BackendACM uses ACM Search GraphQL API (ADR-068).
	BackendACM = "acm"
)

// supportedBackends is the set of valid Backend values.
var supportedBackends = map[string]bool{
	BackendFMC: true,
	BackendACM: true,
}

// MCPGatewayType is a type alias for registry.MCPGatewayType, kept here so
// consumers of FleetConfig do not need to import registry directly.
type MCPGatewayType = registry.MCPGatewayType

const (
	// GatewayEAIGW selects Envoy AI Gateway (gateway.envoyproxy.io Backend CRDs).
	GatewayEAIGW = registry.GatewayEAIGW
	// GatewayKuadrant selects Kuadrant MCP Gateway (mcp.kuadrant.io MCPServerRegistration CRDs).
	GatewayKuadrant = registry.GatewayKuadrant
)

// FleetConfig holds multi-cluster federation settings shared across all services.
// GW and RO use Backend + Endpoint to connect to the federated control plane;
// they never need to know the underlying storage (Valkey, etc.).
//
// References: ADR-065, ADR-068, BR-INTEGRATION-065
type FleetConfig struct {
	// Enabled activates federated scope checking.
	Enabled bool `yaml:"enabled"`

	// Backend selects the federated control plane adapter: "fmc" or "acm".
	Backend string `yaml:"backend,omitempty"`

	// Endpoint is the service address for the chosen backend.
	// For fmc: HTTP URL (e.g., "http://fmc.kubernaut.svc:8080")
	// For acm: GraphQL URL (e.g., "https://search-api.open-cluster-management.svc:4010")
	Endpoint string `yaml:"endpoint,omitempty"`

	// MCPGatewayEndpoint is the MCP Gateway SSE endpoint for remote K8s reads.
	// When set, services that need remote cluster data (GW owner chain, SP enrichment)
	// connect to the MCP Gateway to issue K8s MCP tool calls against managed clusters.
	MCPGatewayEndpoint string `yaml:"mcpGatewayEndpoint,omitempty"`

	// MCPGatewayType selects the MCP Gateway implementation: "eaigw" (Envoy AI Gateway)
	// or "kuadrant" (Kuadrant MCP Gateway). Defaults to "eaigw" when empty.
	MCPGatewayType MCPGatewayType `yaml:"mcpGatewayType,omitempty"`

	// TLSCAFile is the path to the CA certificate bundle for verifying TLS connections
	// to the fleet backend (ACM Search, FMC). When set, the ACM client uses this CA
	// instead of InsecureSkipVerify. Typically mounted from the service-ca operator.
	// +optional
	TLSCAFile string `yaml:"tlsCAFile,omitempty"`

	// OAuth2 holds optional OAuth2 credentials for MCP Gateway authentication.
	OAuth2 FleetOAuth2Config `yaml:"oauth2,omitempty"`
}

// FleetOAuth2Config holds OAuth2 credentials for MCP Gateway authentication.
type FleetOAuth2Config struct {
	Enabled              bool     `yaml:"enabled"`
	TokenURL             string   `yaml:"tokenURL"`
	CredentialsSecretRef string   `yaml:"credentialsSecretRef"`
	Scopes               []string `yaml:"scopes,omitempty"`

	// TLSCAFile is the path to a CA certificate bundle used to verify the
	// TokenURL's TLS certificate when the OAuth2 provider (e.g. Keycloak,
	// Dex) presents a certificate not signed by a public/system CA — for
	// example a cluster-local or self-signed issuer. When empty, the
	// token-fetch HTTP client falls back to the system CA trust store.
	// Mirrors fmc/config.OAuth2Config.TlsCaFile, which every GW/RO/SP/WE/
	// EM/AF caller was missing until this field was added (root cause of
	// "tls: failed to verify certificate: x509: certificate signed by
	// unknown authority" against an in-cluster OIDC provider).
	TLSCAFile string `yaml:"tlsCAFile,omitempty"`
}

// MCPGatewayConfig holds MCP Gateway connectivity settings shared by services
// that discover managed clusters via a registry.ClusterRegistry (FMC,
// SignalProcessing). Promoted from the FMC-private MCPGatewayConfig
// (BR-FLEET-003, #1511) so every fleet-aware service configures the
// endpoint/gatewayType/namespace triad identically instead of duplicating it.
type MCPGatewayConfig struct {
	Endpoint    string `yaml:"endpoint"`
	GatewayType string `yaml:"gatewayType"`
	Namespace   string `yaml:"namespace"`
}

// Validate checks that FleetConfig has all required fields when enabled.
//
// FleetConfig bundles two independent capabilities under the Enabled flag:
//   - Backend + Endpoint: the federated scope-check adapter (FMC HTTP API or
//     ACM GraphQL), used by GW/RO to resolve owner chains and populate
//     spec.clusterID.
//   - MCPGatewayEndpoint + MCPGatewayType: remote K8s reads via the MCP
//     Gateway, used by AF/EM/SP for kubectl-style tool calls and target
//     reads against managed clusters.
//
// A service only needs to configure the capability it actually uses — AF
// and EM, for example, never call the Backend/Endpoint scope-check adapter
// (they discover clusters directly via ClusterRegistry watching Backend
// CRDs), so requiring Backend/Endpoint whenever Enabled=true would force an
// unused dependency on every MCPGatewayEndpoint-only deployment.
func (c FleetConfig) Validate() error {
	if !c.Enabled {
		return nil
	}

	backendConfigured := c.Backend != "" || c.Endpoint != ""

	if backendConfigured {
		backend := c.effectiveBackend()
		endpoint := c.EffectiveEndpoint()

		if !supportedBackends[backend] {
			return fmt.Errorf("fleet: unsupported backend %q; must be one of: fleetmetadatacache, acm", backend)
		}

		if endpoint == "" {
			return fmt.Errorf("fleet: endpoint must not be empty when fleet is enabled (backend=%s)", backend)
		}
	}

	if c.MCPGatewayEndpoint != "" {
		if c.MCPGatewayType == "" {
			return fmt.Errorf("fleet: mcpGatewayType is required when fleet is enabled (mcpGatewayEndpoint is set)")
		}
		if !registry.SupportedGateways[c.MCPGatewayType] {
			return fmt.Errorf("fleet: unsupported mcpGatewayType %q; must be one of: eaigw, kuadrant", c.MCPGatewayType)
		}
	}

	if !backendConfigured && c.MCPGatewayEndpoint == "" {
		return fmt.Errorf("fleet: enabled requires either backend+endpoint or mcpGatewayEndpoint to be configured")
	}

	// #1553: mirrors the OAuth2 pairing check SP/WE/KA already have locally
	// (pkg/signalprocessing/config, pkg/workflowexecution/config). Without
	// this, GW/RO/AF/EM could start with oauth2.enabled=true but a missing
	// tokenURL/credentialsSecretRef and silently send unauthenticated
	// requests to the MCP Gateway instead of failing closed at startup.
	if c.OAuth2.Enabled {
		if c.OAuth2.TokenURL == "" {
			return fmt.Errorf("fleet: oauth2.tokenURL is required when oauth2.enabled=true")
		}
		if c.OAuth2.CredentialsSecretRef == "" {
			return fmt.Errorf("fleet: oauth2.credentialsSecretRef is required when oauth2.enabled=true")
		}
	}

	return nil
}

// ValidateFullFederation is a stricter, opt-in check for services that need
// BOTH FleetConfig capabilities to operate correctly: GW (owner-chain
// metadata resolution) and RO (pre-remediation spec hash computation) each
// require the Backend/Endpoint scope-check adapter AND MCPGatewayEndpoint
// remote reads — one does not work without the other for these two
// services. Configuring only one leaves them silently degraded to
// local-only behavior for fleet-routed resources, which defeats the
// purpose of enabling fleet at all. Call this in addition to Validate()
// (which only confirms whatever is configured is well-formed); GW/RO
// callers should treat a non-nil error here as fatal at startup.
func (c FleetConfig) ValidateFullFederation() error {
	if !c.Enabled {
		return nil
	}

	if c.Backend == "" && c.Endpoint == "" {
		return fmt.Errorf("fleet: backend+endpoint is required when fleet is enabled " +
			"(federated scope-check; without it, resource ownership cannot be determined)")
	}

	if c.MCPGatewayEndpoint == "" {
		return fmt.Errorf("fleet: mcpGatewayEndpoint is required when fleet is enabled " +
			"(remote reads; without it, fleet-routed resources silently degrade to local-only reads)")
	}

	return nil
}

// EffectiveMCPGatewayType returns the configured MCPGatewayType.
// Returns empty string when MCPGatewayType is not set, indicating fleet is
// disabled. Callers must check for empty before using the value.
func (c FleetConfig) EffectiveMCPGatewayType() MCPGatewayType {
	return c.MCPGatewayType
}

// effectiveBackend returns the configured backend, or empty if not set.
func (c FleetConfig) effectiveBackend() string {
	return c.Backend
}

// FMC service discovery constants.
const (
	fmcServiceName = "fleetmetadatacache-service"
	fmcServicePort = "8080"
)

// EffectiveEndpoint returns the configured endpoint, or derives it for the FMC
// backend when no explicit endpoint is set. Auto-derivation uses the same
// namespace detection pattern as DataStorage (POD_NAMESPACE env > SA mount > "default").
func (c FleetConfig) EffectiveEndpoint() string {
	if c.Endpoint != "" {
		return c.Endpoint
	}
	if c.Backend == BackendFMC {
		ns := detectNamespace()
		return fmt.Sprintf("http://%s.%s.svc.cluster.local:%s", fmcServiceName, ns, fmcServicePort)
	}
	return ""
}

// detectNamespace returns the Kubernetes namespace this pod is running in.
// Priority: POD_NAMESPACE env var > ServiceAccount mount > "default".
func detectNamespace() string {
	if ns := os.Getenv("POD_NAMESPACE"); ns != "" {
		return ns
	}
	if data, err := os.ReadFile("/var/run/secrets/kubernetes.io/serviceaccount/namespace"); err == nil {
		if ns := strings.TrimSpace(string(data)); ns != "" {
			return ns
		}
	}
	return "default"
}
