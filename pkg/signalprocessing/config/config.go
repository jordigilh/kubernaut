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

// Package config provides configuration types for Signal Processing controller.
// Design Decision: DD-006 Controller Scaffolding
// Business Requirements: BR-SP-001 (Enrichment), BR-SP-070 (Classification), BR-SP-090 (Audit)
package config

import (
	"fmt"
	"net/url"
	"os"
	"time"

	"gopkg.in/yaml.v3"
)

// Config holds all configuration for the Signal Processing controller.
// Note: MetricsAddr, HealthProbeAddr, and LeaderElection are NOT configurable
// in YAML - they are hardcoded or set via CLI flags for safety.
type Config struct {
	// Signal Processing specific configuration
	Enrichment EnrichmentConfig `yaml:"enrichment"`
	Classifier ClassifierConfig `yaml:"classifier"`
	Audit      AuditConfig      `yaml:"audit"`
}

// ControllerConfig holds controller-manager options (CLI flags, not YAML).
// These are NOT exposed in config.yaml for safety reasons.
type ControllerConfig struct {
	// MetricsAddr is the address for Prometheus metrics endpoint.
	// Default: ":9090" - hardcoded, not configurable.
	MetricsAddr string

	// HealthProbeAddr is the address for health probe endpoint.
	// Default: ":8081" - hardcoded, not configurable.
	HealthProbeAddr string

	// LeaderElection is ALWAYS enabled for CRD controllers in production.
	// This prevents split-brain scenarios. Not configurable.
	LeaderElection bool

	// LeaderElectionID uniquely identifies this controller for leader election.
	LeaderElectionID string
}

// EnrichmentConfig configures K8s context enrichment behavior.
// BR-SP-001: K8s Context Enrichment
type EnrichmentConfig struct {
	// CacheTTL is how long to cache K8s context lookups.
	CacheTTL time.Duration `yaml:"cache_ttl"`

	// Timeout is the maximum time for enrichment operations.
	// SLO: <2 seconds P95
	Timeout time.Duration `yaml:"timeout"`
}

// ClassifierConfig configures Rego policy-based classification.
// BR-SP-070: Priority Assignment (Rego)
type ClassifierConfig struct {
	// RegoPolicyPath is the path to Rego policy files (for local development).
	RegoPolicyPath string `yaml:"rego_policy_path"`

	// RegoConfigMapName is the ConfigMap containing Rego policies.
	RegoConfigMapName string `yaml:"rego_configmap_name"`

	// RegoConfigMapKey is the key within the ConfigMap for policy content.
	RegoConfigMapKey string `yaml:"rego_configmap_key"`

	// HotReloadInterval is how often to check for policy updates.
	// Minimum 10s to avoid excessive ConfigMap watches.
	HotReloadInterval time.Duration `yaml:"hot_reload_interval"`
}

// AuditConfig configures audit trail persistence via Data Storage Service.
// ADR-032, ADR-034: Audit is MANDATORY - no "enabled" flag.
// BR-SP-090: Categorization Audit Trail
type AuditConfig struct {
	// DataStorageURL is the base URL for Data Storage Service REST API.
	// ADR-032: All audit writes go through Data Storage Service (no direct DB access).
	DataStorageURL string `yaml:"data_storage_url"`

	// Timeout is the maximum time for audit write operations.
	// ADR-038: Fire-and-forget pattern means this is for buffer flush, not blocking.
	Timeout time.Duration `yaml:"timeout"`

	// BufferSize is the in-memory buffer size for fire-and-forget audit writes.
	// ADR-038: Events buffered locally, flushed asynchronously.
	BufferSize int `yaml:"buffer_size"`

	// FlushInterval is how often to flush buffered audit events.
	FlushInterval time.Duration `yaml:"flush_interval"`
}

// LoadFromFile loads configuration from a YAML file.
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

// Validate validates the configuration.
// Returns error if configuration is invalid.
func (c *Config) Validate() error {
	// Validate enrichment config
	if err := c.Enrichment.Validate(); err != nil {
		return fmt.Errorf("enrichment config: %w", err)
	}

	// Validate classifier config
	if err := c.Classifier.Validate(); err != nil {
		return fmt.Errorf("classifier config: %w", err)
	}

	// Validate audit config
	if err := c.Audit.Validate(); err != nil {
		return fmt.Errorf("audit config: %w", err)
	}

	return nil
}

// Validate validates enrichment configuration.
func (e *EnrichmentConfig) Validate() error {
	if e.Timeout < time.Second {
		return fmt.Errorf("timeout must be at least 1s, got %v", e.Timeout)
	}
	if e.CacheTTL < 0 {
		return fmt.Errorf("cache_ttl cannot be negative, got %v", e.CacheTTL)
	}
	return nil
}

// Validate validates classifier configuration.
func (cl *ClassifierConfig) Validate() error {
	if cl.RegoConfigMapName == "" {
		return fmt.Errorf("rego_configmap_name must be specified")
	}
	if cl.RegoConfigMapKey == "" {
		return fmt.Errorf("rego_configmap_key must be specified")
	}
	if cl.HotReloadInterval < 10*time.Second {
		return fmt.Errorf("hot_reload_interval must be at least 10s, got %v", cl.HotReloadInterval)
	}
	return nil
}

// Validate validates audit configuration.
func (a *AuditConfig) Validate() error {
	if a.DataStorageURL == "" {
		return fmt.Errorf("data_storage_url must be specified")
	}
	// Validate URL format - url.Parse is lenient, so also check for scheme
	parsedURL, err := url.Parse(a.DataStorageURL)
	if err != nil || parsedURL.Scheme == "" || parsedURL.Host == "" {
		return fmt.Errorf("data_storage_url is not a valid URL: %s", a.DataStorageURL)
	}
	if a.Timeout < time.Second {
		return fmt.Errorf("timeout must be at least 1s, got %v", a.Timeout)
	}
	if a.BufferSize < 100 || a.BufferSize > 10000 {
		return fmt.Errorf("buffer_size must be between 100 and 10000, got %d", a.BufferSize)
	}
	if a.FlushInterval < time.Second || a.FlushInterval > 30*time.Second {
		return fmt.Errorf("flush_interval must be between 1s and 30s, got %v", a.FlushInterval)
	}
	return nil
}

// DefaultControllerConfig returns default controller configuration.
// These values are hardcoded for safety - not configurable via YAML.
func DefaultControllerConfig() ControllerConfig {
	return ControllerConfig{
		MetricsAddr:      ":9090",
		HealthProbeAddr:  ":8081",
		LeaderElection:   true,
		LeaderElectionID: "signalprocessing.kubernaut.ai",
	}
}
