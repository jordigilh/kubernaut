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

package registry

import (
	"fmt"

	"github.com/go-logr/logr"
	"k8s.io/client-go/dynamic"
)

// MCPGatewayType selects which MCP Gateway implementation to use for cluster
// registry discovery. Typed enum prevents invalid values at compile time for
// Go callers; YAML/env-based config is validated at FleetConfig.Validate() time.
type MCPGatewayType string

const (
	// GatewayEAIGW selects Envoy AI Gateway (gateway.envoyproxy.io Backend CRDs).
	GatewayEAIGW MCPGatewayType = "eaigw"
	// GatewayKuadrant selects Kuadrant MCP Gateway (mcp.kuadrant.io MCPServerRegistration CRDs).
	GatewayKuadrant MCPGatewayType = "kuadrant"
)

// SupportedGateways is the set of valid MCPGatewayType values.
var SupportedGateways = map[MCPGatewayType]bool{
	GatewayEAIGW:    true,
	GatewayKuadrant: true,
}

// RegistryConfig is the gateway-neutral configuration for any ClusterRegistry.
type RegistryConfig = EAIGWRegistryConfig

// NewClusterRegistry creates the appropriate ClusterRegistry implementation
// based on the MCPGatewayType. Invalid gateway types are rejected with an error.
func NewClusterRegistry(gatewayType MCPGatewayType, client dynamic.Interface, cfg RegistryConfig, metrics *Metrics, logger logr.Logger) (ClusterRegistry, error) {
	switch gatewayType {
	case GatewayEAIGW:
		return NewEAIGWRegistry(client, cfg, metrics, logger), nil
	case GatewayKuadrant:
		return NewKuadrantRegistry(client, cfg, metrics, logger), nil
	default:
		return nil, fmt.Errorf("unsupported MCP gateway type %q; must be one of: eaigw, kuadrant", gatewayType)
	}
}
