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
package config

import (
	"fmt"
	"time"
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
	return nil
}
