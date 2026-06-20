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
// Services that participate in fleet operations (GW, RO, FMC Writer) import this
// package for consistent configuration and connection management.
package fleet

import "fmt"

// Supported backend types for federated scope checking.
const (
	// BackendFMC uses the Fleet Metadata Cache HTTP service (ADR-068).
	BackendFMC = "fmc"
	// BackendACM uses ACM Search GraphQL API (ADR-068).
	BackendACM = "acm"
	// BackendValkey uses direct Valkey connection (legacy, pre-ADR-068).
	BackendValkey = "valkey"
)

// supportedBackends is the set of valid Backend values.
var supportedBackends = map[string]bool{
	BackendFMC:    true,
	BackendACM:    true,
	BackendValkey: true,
}

// FleetConfig holds multi-cluster federation settings shared across all services.
// GW and RO use Backend + Endpoint to connect to the federated control plane;
// they never need to know the underlying storage (Valkey, etc.).
//
// References: ADR-065, ADR-068, BR-INTEGRATION-065
type FleetConfig struct {
	// Enabled activates federated scope checking.
	Enabled bool `yaml:"enabled"`

	// Backend selects the federated control plane adapter: "fmc", "acm", or "valkey" (legacy).
	Backend string `yaml:"backend,omitempty"`

	// Endpoint is the service address for the chosen backend.
	// For fmc: HTTP URL (e.g., "http://fmc.kubernaut.svc:8080")
	// For acm: GraphQL URL (e.g., "https://search-api.open-cluster-management.svc:4010")
	// For valkey: Valkey address (e.g., "valkey:6379") — legacy, maps to ValkeyAddr
	Endpoint string `yaml:"endpoint,omitempty"`

	// ValkeyAddr is the Valkey/Redis address for the fleet metadata cache.
	// Deprecated: Use Backend="valkey" + Endpoint instead. Kept for backward compatibility.
	ValkeyAddr string `yaml:"valkeyAddr,omitempty"`
}

// Validate checks that FleetConfig has all required fields when enabled.
func (c FleetConfig) Validate() error {
	if !c.Enabled {
		return nil
	}

	backend := c.effectiveBackend()
	endpoint := c.EffectiveEndpoint()

	if !supportedBackends[backend] {
		return fmt.Errorf("fleet: unsupported backend %q; must be one of: fmc, acm, valkey", backend)
	}

	if endpoint == "" {
		return fmt.Errorf("fleet: endpoint must not be empty when fleet is enabled (backend=%s)", backend)
	}

	return nil
}

// effectiveBackend returns the backend to use, defaulting to "valkey" for backward compat.
func (c FleetConfig) effectiveBackend() string {
	if c.Backend != "" {
		return c.Backend
	}
	if c.ValkeyAddr != "" {
		return BackendValkey
	}
	return ""
}

// EffectiveEndpoint returns the endpoint to use, falling back to ValkeyAddr for legacy configs.
func (c FleetConfig) EffectiveEndpoint() string {
	if c.Endpoint != "" {
		return c.Endpoint
	}
	return c.ValkeyAddr
}
