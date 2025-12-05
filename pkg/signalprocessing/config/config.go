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
//   - AuditConfig: Audit trail buffering settings (buffer size, flush interval)
//   - ControllerConfig: Controller runtime settings (metrics, health probes, leader election)
//
// All configuration values are validated via Config.Validate() before use.
package config

import (
	"fmt"
	"os"
	"time"

	"gopkg.in/yaml.v3"
)

// Config holds the complete configuration for the SignalProcessing controller.
type Config struct {
	Enrichment EnrichmentConfig
	Classifier ClassifierConfig
	Audit      AuditConfig
}

// EnrichmentConfig holds settings for K8s context enrichment.
type EnrichmentConfig struct {
	CacheTTL time.Duration
	Timeout  time.Duration
}

// ClassifierConfig holds settings for Rego-based classification.
type ClassifierConfig struct {
	RegoConfigMapName string
	RegoConfigMapKey  string
	HotReloadInterval time.Duration
}

// AuditConfig holds settings for audit trail buffering.
type AuditConfig struct {
	DataStorageURL string
	Timeout        time.Duration
	BufferSize     int
	FlushInterval  time.Duration
}

// ControllerConfig holds controller runtime settings.
type ControllerConfig struct {
	MetricsAddr      string
	HealthProbeAddr  string
	LeaderElection   bool
	LeaderElectionID string
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
	if c.Audit.BufferSize <= 0 {
		return fmt.Errorf("audit buffer size must be positive, got %d", c.Audit.BufferSize)
	}
	if c.Audit.FlushInterval <= 0 {
		return fmt.Errorf("audit flush interval must be positive, got %v", c.Audit.FlushInterval)
	}
	return nil
}
