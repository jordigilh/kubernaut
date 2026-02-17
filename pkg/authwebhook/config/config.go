/*
Copyright 2025 Jordi Gil.

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

// Package config provides configuration types for the AuthWebhook admission controller.
//
// Configuration Structure (ADR-030):
//   - WebhookConfig: Webhook server settings (port, cert directory)
//   - DataStorageConfig: Data Storage connectivity (audit trail)
//
// Migrated from CLI flags/env vars to ADR-030 compliant YAML ConfigMap.
package config

import (
	"fmt"
	"os"

	"gopkg.in/yaml.v3"

	sharedconfig "github.com/jordigilh/kubernaut/internal/config"
)

// DefaultConfigPath is the standard Kubernetes ConfigMap mount path for this service.
// ADR-030: All services MUST use /etc/{service}/config.yaml as the default.
const DefaultConfigPath = "/etc/authwebhook/config.yaml"

// Config holds the complete configuration for the AuthWebhook admission controller.
// ADR-030: YAML-based configuration with camelCase field names.
type Config struct {
	// Webhook server settings (port, TLS cert directory)
	Webhook WebhookConfig `yaml:"webhook"`

	// DataStorage connectivity (ADR-030: audit trail + workflow catalog)
	DataStorage sharedconfig.DataStorageConfig `yaml:"datastorage"`
}

// WebhookConfig holds settings for the admission webhook server.
// Per CRD_FIELD_NAMING_CONVENTION.md: YAML fields use camelCase.
type WebhookConfig struct {
	// Port the webhook server binds to. Default: 9443
	Port int `yaml:"port"`

	// CertDir is the directory containing TLS certificates.
	// Default: /tmp/k8s-webhook-server/serving-certs
	CertDir string `yaml:"certDir"`

	// HealthProbeAddr is the address for health and readiness probes.
	// Default: :8081
	HealthProbeAddr string `yaml:"healthProbeAddr"`
}

// DefaultConfig returns the default AuthWebhook configuration.
func DefaultConfig() *Config {
	return &Config{
		Webhook: WebhookConfig{
			Port:            9443,
			CertDir:         "/tmp/k8s-webhook-server/serving-certs",
			HealthProbeAddr: ":8081",
		},
		DataStorage: sharedconfig.DefaultDataStorageConfig(),
	}
}

// LoadFromFile loads configuration from a YAML file with defaults.
// ADR-030: Service Configuration Management pattern.
// Graceful degradation: Falls back to defaults if file not found or invalid.
func LoadFromFile(path string) (*Config, error) {
	cfg := DefaultConfig()

	if path == "" {
		return cfg, nil
	}

	data, err := os.ReadFile(path)
	if err != nil {
		return cfg, nil
	}

	if err := yaml.Unmarshal(data, cfg); err != nil {
		return cfg, nil
	}

	return cfg, nil
}

// Validate checks if the configuration is valid.
// ADR-030: Fail fast on invalid configuration.
func (c *Config) Validate() error {
	if c.Webhook.Port <= 0 || c.Webhook.Port > 65535 {
		return fmt.Errorf("webhook.port must be between 1 and 65535, got %d", c.Webhook.Port)
	}
	if c.Webhook.CertDir == "" {
		return fmt.Errorf("webhook.certDir is required")
	}
	if c.Webhook.HealthProbeAddr == "" {
		return fmt.Errorf("webhook.healthProbeAddr is required")
	}

	// ADR-030: Validate DataStorage section
	return sharedconfig.ValidateDataStorageConfig(&c.DataStorage)
}
