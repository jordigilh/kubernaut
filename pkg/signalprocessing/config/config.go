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

// Package config provides configuration types for the SignalProcessing controller.
//
// Configuration Structure:
//   - EnrichmentConfig: K8s context enrichment settings (cache TTL, timeout)
//   - ClassifierConfig: Rego-based classification settings (ConfigMap, hot-reload)
//   - DataStorageConfig: Data Storage connectivity (ADR-030)
//   - ControllerConfig: Controller runtime settings (metrics, health probes, leader election)
//
// All configuration values are validated via Config.Validate() before use.
package config

import (
	"fmt"
	"os"
	"time"

	"gopkg.in/yaml.v3"

	sharedconfig "github.com/jordigilh/kubernaut/internal/config"
)

// Config holds the complete configuration for the SignalProcessing controller.
// ADR-030: YAML-based configuration with camelCase field names.
type Config struct {
	Enrichment  EnrichmentConfig              `yaml:"enrichment"`
	Classifier  ClassifierConfig              `yaml:"classifier"`
	DataStorage sharedconfig.DataStorageConfig `yaml:"datastorage"`
	Controller  ControllerConfig              `yaml:"controller"`
}

// EnrichmentConfig holds settings for K8s context enrichment.
// Per CRD_FIELD_NAMING_CONVENTION.md: YAML fields use camelCase.
type EnrichmentConfig struct {
	CacheTTL time.Duration `yaml:"cacheTtl"`
	Timeout  time.Duration `yaml:"timeout"`
}

// ClassifierConfig holds settings for Rego-based classification.
// Per CRD_FIELD_NAMING_CONVENTION.md: YAML fields use camelCase.
type ClassifierConfig struct {
	RegoConfigMapName string        `yaml:"regoConfigMapName"`
	RegoConfigMapKey  string        `yaml:"regoConfigMapKey"`
	HotReloadInterval time.Duration `yaml:"hotReloadInterval"`
}

// ControllerConfig holds controller runtime settings.
// Per CRD_FIELD_NAMING_CONVENTION.md: YAML fields use camelCase.
type ControllerConfig struct {
	MetricsAddr      string `yaml:"metricsAddr"`
	HealthProbeAddr  string `yaml:"healthProbeAddr"`
	LeaderElection   bool   `yaml:"leaderElection"`
	LeaderElectionID string `yaml:"leaderElectionId"`
}

// DefaultControllerConfig returns the default controller configuration.
func DefaultControllerConfig() *ControllerConfig {
	return &ControllerConfig{
		MetricsAddr:      ":8080",
		HealthProbeAddr:  ":8081",
		LeaderElection:   false,
		LeaderElectionID: "signalprocessing.kubernaut.ai",
	}
}

// DefaultConfig returns the default SignalProcessing controller configuration.
func DefaultConfig() *Config {
	return &Config{
		Enrichment: EnrichmentConfig{
			CacheTTL: 5 * time.Minute,
			Timeout:  10 * time.Second,
		},
		Classifier: ClassifierConfig{
			RegoConfigMapName: "signalprocessing-rego-policy",
			RegoConfigMapKey:  "policy.rego",
			HotReloadInterval: 30 * time.Second,
		},
		DataStorage: sharedconfig.DefaultDataStorageConfig(),
		Controller:  *DefaultControllerConfig(),
	}
}

// LoadFromFile loads configuration from a YAML file.
func LoadFromFile(path string) (*Config, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("failed to read config file: %w", err)
	}

	var cfg Config
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return nil, fmt.Errorf("failed to parse config file: %w", err)
	}

	return &cfg, nil
}

// Validate checks if the configuration is valid.
func (c *Config) Validate() error {
	if c.Enrichment.Timeout <= 0 {
		return fmt.Errorf("enrichment timeout must be positive, got %v", c.Enrichment.Timeout)
	}
	if c.Classifier.RegoConfigMapName == "" {
		return fmt.Errorf("Rego ConfigMap name is required")
	}
	if c.Classifier.HotReloadInterval <= 0 {
		return fmt.Errorf("hot-reload interval must be positive, got %v", c.Classifier.HotReloadInterval)
	}
	// ADR-030: Validate DataStorage section
	if err := sharedconfig.ValidateDataStorageConfig(&c.DataStorage); err != nil {
		return err
	}
	return nil
}
