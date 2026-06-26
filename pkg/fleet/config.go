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
)

// Supported backend types for federated scope checking.
const (
	// BackendFMC uses the Fleet Metadata Cache HTTP service (ADR-068).
	BackendFMC = "fmc"
	// BackendACM uses ACM Search GraphQL API (ADR-068).
	BackendACM = "acm"
)

// supportedBackends is the set of valid Backend values.
var supportedBackends = map[string]bool{
	BackendFMC: true,
	BackendACM: true,
}

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

	// MCPGatewayEndpoint is the Envoy AI Gateway SSE endpoint for remote K8s reads.
	// When set, services that need remote cluster data (GW owner chain, SP enrichment)
	// connect to the MCP Gateway to issue K8s MCP tool calls against managed clusters.
	MCPGatewayEndpoint string `yaml:"mcpGatewayEndpoint,omitempty"`

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
		return fmt.Errorf("fleet: unsupported backend %q; must be one of: fmc, acm", backend)
	}

	if endpoint == "" {
		return fmt.Errorf("fleet: endpoint must not be empty when fleet is enabled (backend=%s)", backend)
	}

	return nil
}

// effectiveBackend returns the configured backend, or empty if not set.
func (c FleetConfig) effectiveBackend() string {
	return c.Backend
}

// FMC service discovery constants.
const (
	fmcServiceName = "fmc-service"
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
