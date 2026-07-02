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
}

// Validate checks that FleetConfig has all required fields when enabled.
func (c FleetConfig) Validate() error {
	if !c.Enabled {
		return nil
	}

	backend := c.effectiveBackend()
	endpoint := c.EffectiveEndpoint()

	if !supportedBackends[backend] {
		return fmt.Errorf("fleet: unsupported backend %q; must be one of: fleetmetadatacache, acm", backend)
	}

	if endpoint == "" {
		return fmt.Errorf("fleet: endpoint must not be empty when fleet is enabled (backend=%s)", backend)
	}

	if c.MCPGatewayEndpoint != "" {
		if c.MCPGatewayType == "" {
			return fmt.Errorf("fleet: mcpGatewayType is required when fleet is enabled (mcpGatewayEndpoint is set)")
		}
		if !registry.SupportedGateways[c.MCPGatewayType] {
			return fmt.Errorf("fleet: unsupported mcpGatewayType %q; must be one of: eaigw, kuadrant", c.MCPGatewayType)
		}
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
