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

// Package config implements ADR-030 compliant YAML configuration for the FMC service.
// Field names use camelCase per CRD_FIELD_NAMING_CONVENTION.md V1.1.
package config

import (
	"fmt"
	"os"
	"time"

	"gopkg.in/yaml.v3"

	sharedconfig "github.com/jordigilh/kubernaut/internal/config"
	"github.com/jordigilh/kubernaut/pkg/fleet"
	"github.com/jordigilh/kubernaut/pkg/fleet/registry"
	sharedtls "github.com/jordigilh/kubernaut/pkg/shared/tls"
)

// DefaultConfigPath is the standard Kubernetes ConfigMap mount path for FMC.
// ADR-030: All services MUST use /etc/{service}/config.yaml as the default.
const DefaultConfigPath = "/etc/fleetmetadatacache/config.yaml"

// ServiceConfig is the top-level configuration for the FMC service.
type ServiceConfig struct {
	Server     ServerConfig           `yaml:"server"`
	MCPGateway fleet.MCPGatewayConfig `yaml:"mcpGateway"`
	Valkey     ValkeyConfig           `yaml:"valkey"`
	Sync       SyncConfig             `yaml:"sync"`
	OAuth2     OAuth2Config           `yaml:"oauth2"`

	// Debug gates diagnostic surfaces (currently: /debug/pprof/* on the
	// dedicated health port). BR-PLATFORM-012, Issue #2275: replaces the
	// pre-#2275 hardcoded enableProfiling=true in buildFMCServers --
	// FMC now defaults to secure (profiling OFF), consistent with the
	// other 12 services, rather than being permanently profiling-on.
	Debug sharedconfig.DebugConfig `yaml:"debug"`

	// TLSProfile selects the TLS security profile (Old/Intermediate/Modern).
	// Issue #748: OCP-only — set by kubernaut-operator from the cluster APIServer CR.
	TLSProfile string `yaml:"tlsProfile,omitempty"`
}

// ServerConfig contains HTTP server settings.
// Issue #753: 3-port standard — API (TLS-capable), Health (plain HTTP,
// kubelet-only), Metrics (plain HTTP, Prometheus-only).
type ServerConfig struct {
	APIAddr     string `yaml:"apiAddr"`
	HealthAddr  string `yaml:"healthAddr"`
	MetricsAddr string `yaml:"metricsAddr"`

	// TLS configures optional server-side TLS for the API port only.
	// Issue #493/#1683: when unset (CertDir==""), the API server falls back
	// to plain HTTP (ConfigureConditionalTLS fail-open bootstrap behavior).
	TLS sharedtls.TLSConfig `yaml:"tls,omitempty"`
}

// ValkeyConfig contains Valkey cache connectivity.
type ValkeyConfig struct {
	Addr string          `yaml:"addr"`
	TLS  ValkeyTLSConfig `yaml:"tls,omitempty"`
}

// ValkeyTLSConfig selects TLS settings for FMC's Valkey client connection.
// DD-PLATFORM-006 DA9 follow-up (round-16 RCA, PR #1790): DA9's original
// audit of Valkey-consuming Go clients covered DataStorage and the
// APIFrontend replay cache, but missed FMC -- a third client whose
// pkg/fleet/fmc.ValkeyWriter / pkg/fleet/scopecache.ValkeyCacheReader had
// zero TLS support, silently broken the moment DA8 made the chart's Valkey
// TLS-only (confirmed live: valkey-server logs "SSL routines::wrong version
// number" for every FMC connection attempt, and FMC's own /readyz -- gated
// on a Valkey PING -- never turns healthy, permanently blocking Gateway/RO's
// dependent readiness in fleet-enabled installs). Mirrors
// pkg/datastorage/config.RedisTLSConfig's and
// pkg/apifrontend/config.ReplayCacheTLSConfig's field shape for
// cross-service consistency (SC-8: TLS always validates the server
// certificate via CAFile; CertFile/KeyFile are optional mTLS, unused against
// the chart's own Valkey which does not require client certs).
type ValkeyTLSConfig struct {
	Enabled  bool   `yaml:"enabled"`
	CAFile   string `yaml:"caFile,omitempty"`
	CertFile string `yaml:"certFile,omitempty"`
	KeyFile  string `yaml:"keyFile,omitempty"`
}

// SyncConfig contains syncer timing and resource settings.
type SyncConfig struct {
	Interval           time.Duration `yaml:"interval"`
	KeyTTL             time.Duration `yaml:"keyTtl"`
	ResourceKinds      []string      `yaml:"resourceKinds"`
	WaitForBrokerReady bool          `yaml:"waitForBrokerReady"`
}

// OAuth2Config contains OAuth2 client_credentials settings.
// OAuth2 is mandatory for FMC — the MCP Gateway always requires authentication
// in production fleet deployments. There is no unauthenticated fallback.
type OAuth2Config struct {
	TokenURL       string        `yaml:"tokenUrl"`
	CredentialsDir string        `yaml:"credentialsDir"`
	Scopes         []string      `yaml:"scopes"`
	TokenTimeout   time.Duration `yaml:"tokenTimeout"`
	TlsCaFile      string        `yaml:"tlsCaFile"`
}

// DefaultServiceConfig returns production defaults.
func DefaultServiceConfig() *ServiceConfig {
	return &ServiceConfig{
		Server: ServerConfig{
			APIAddr:     ":8080",
			HealthAddr:  ":8081",
			MetricsAddr: ":9090",
		},
		MCPGateway: fleet.MCPGatewayConfig{
			GatewayType: "eaigw",
			Namespace:   "kubernaut-system",
		},
		Valkey: ValkeyConfig{
			Addr: "valkey:6379",
		},
		Sync: SyncConfig{
			Interval:           30 * time.Second,
			KeyTTL:             45 * time.Second,
			ResourceKinds:      []string{"Deployment", "StatefulSet", "DaemonSet", "Pod", "Service", "Node"},
			WaitForBrokerReady: true,
		},
		OAuth2: OAuth2Config{
			CredentialsDir: "/etc/fleetmetadatacache/fleet-oauth2",
			Scopes:         []string{"openid", "groups"},
			TokenTimeout:   10 * time.Second,
		},
		Debug: sharedconfig.DefaultDebugConfig(),
	}
}

// LoadFromFile reads a YAML config file and unmarshals it into ServiceConfig.
// ADR-030: All services MUST use this pattern for configuration loading.
func LoadFromFile(path string) (*ServiceConfig, error) {
	cfg := DefaultServiceConfig()

	if path == "" {
		return cfg, nil
	}

	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("failed to read config file %s: %w", path, err)
	}

	if err := yaml.Unmarshal(data, cfg); err != nil {
		return nil, fmt.Errorf("failed to parse config file %s: %w", path, err)
	}

	return cfg, nil
}

// Validate checks required fields and returns an error if any are missing.
func (c *ServiceConfig) Validate() error {
	if c.MCPGateway.Endpoint == "" {
		return fmt.Errorf("mcpGateway.endpoint is required")
	}
	// #1707 follow-up: catch an empty/unsupported gatewayType here, at
	// config-validation time, instead of letting it flow to
	// registry.NewClusterRegistry() (cmd/fleetmetadatacache/main.go), which
	// rejects it with a generic error deep in the startup path. Mirrors
	// pkg/fleet.FleetConfig.Validate()'s MCPGatewayType check used by GW/RO.
	if c.MCPGateway.GatewayType == "" {
		return fmt.Errorf("mcpGateway.gatewayType is required")
	}
	if !registry.SupportedGateways[registry.MCPGatewayType(c.MCPGateway.GatewayType)] {
		return fmt.Errorf("unsupported mcpGateway.gatewayType %q; must be one of: eaigw, kuadrant", c.MCPGateway.GatewayType)
	}
	if c.Valkey.Addr == "" {
		return fmt.Errorf("valkey.addr is required")
	}
	if c.Valkey.TLS.Enabled && c.Valkey.TLS.CAFile == "" {
		return fmt.Errorf("valkey TLS enabled but no caFile specified; mount the CA certificate (SC-8)")
	}
	if c.OAuth2.TokenURL == "" {
		return fmt.Errorf("oauth2.tokenUrl is required — MCP Gateway requires authentication")
	}
	return nil
}
