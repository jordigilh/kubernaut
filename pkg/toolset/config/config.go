package config

import (
	"fmt"
	"os"
	"time"

	"gopkg.in/yaml.v3"
)

// Config represents the Dynamic Toolset service configuration
// ADR-030: Configuration loaded from YAML file (ConfigMap in Kubernetes)
//
// NOTE: This structure will become the .spec of the ToolsetConfig CRD in V1.1 (BR-TOOLSET-044)
// Server configuration (port, metrics_port, shutdown_timeout) is handled via deployment env vars/flags
type Config struct {
	ServiceDiscovery ServiceDiscoveryConfig `yaml:"service_discovery"`
}

// ServiceDiscoveryConfig contains service discovery configuration
// BR-TOOLSET-016: Service discovery configuration
// BR-TOOLSET-019: Multi-namespace service discovery
// BR-TOOLSET-026: Service discovery with health validation
type ServiceDiscoveryConfig struct {
	DiscoveryInterval   time.Duration `yaml:"discovery_interval"`    // How often to scan for services
	HealthCheckInterval time.Duration `yaml:"health_check_interval"` // How often to health check discovered services
	Namespaces          []string      `yaml:"namespaces"`            // Empty means all namespaces
	SkipHealthCheck     bool          `yaml:"skip_health_check"`     // Skip health checks (useful for testing)
}

// LoadFromFile loads configuration from a YAML file
// This follows the same pattern as Context API and Gateway services (ADR-030)
func LoadFromFile(path string) (*Config, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("failed to read config file: %w", err)
	}

	var cfg Config
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return nil, fmt.Errorf("failed to parse config: %w", err)
	}

	return &cfg, nil
}

// Validate checks if the configuration is valid and returns an error if not
func (c *Config) Validate() error {
	// Validate service discovery configuration
	if c.ServiceDiscovery.DiscoveryInterval == 0 {
		return fmt.Errorf("discovery_interval required")
	}
	if c.ServiceDiscovery.DiscoveryInterval < 1*time.Second {
		return fmt.Errorf("discovery_interval must be at least 1 second")
	}
	if c.ServiceDiscovery.HealthCheckInterval == 0 {
		return fmt.Errorf("health_check_interval required")
	}
	if c.ServiceDiscovery.HealthCheckInterval < 1*time.Second {
		return fmt.Errorf("health_check_interval must be at least 1 second")
	}

	return nil
}
