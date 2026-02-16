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

package config

import (
	"fmt"
	"os"
	"time"

	"gopkg.in/yaml.v3"

	sharedconfig "github.com/jordigilh/kubernaut/internal/config"
)

// ServerConfig is the top-level configuration for the Gateway service.
// Organized by Single Responsibility Principle for better maintainability.
type ServerConfig struct {
	// HTTP Server configuration
	Server ServerSettings `yaml:"server"`

	// Middleware configuration
	Middleware MiddlewareSettings `yaml:"middleware"`

	// DataStorage connectivity (ADR-030: audit trail + workflow catalog)
	DataStorage sharedconfig.DataStorageConfig `yaml:"datastorage"`

	// Business logic configuration
	Processing ProcessingSettings `yaml:"processing"`
}

// ServerSettings contains HTTP server configuration.
// Single Responsibility: HTTP server behavior
type ServerSettings struct {
	ListenAddr            string        `yaml:"listenAddr"`              // Default: ":8080"
	MaxConcurrentRequests int           `yaml:"maxConcurrentRequests"`   // Default: 100 (0 = unlimited)
	ReadTimeout           time.Duration `yaml:"readTimeout"`             // Default: 30s
	WriteTimeout          time.Duration `yaml:"writeTimeout"`            // Default: 30s
	IdleTimeout           time.Duration `yaml:"idleTimeout"`             // Default: 120s
}

// MiddlewareSettings contains middleware configuration.
// Single Responsibility: Request processing middleware
//
// Note: RateLimitSettings removed (2025-12-07)
// Rate limiting now delegated to Ingress/Route proxy per ADR-048.
// See: docs/architecture/decisions/ADR-048-rate-limiting-proxy-delegation.md
type MiddlewareSettings struct {
	// Empty - rate limiting delegated to proxy (ADR-048)
	// Future middleware config can be added here
}

// ProcessingSettings contains business logic configuration.
// Single Responsibility: Signal processing behavior
//
// Note: Environment and Priority settings removed (2025-12-06)
// Environment/Priority classification now owned by Signal Processing per DD-CATEGORIZATION-001.
// See: docs/handoff/NOTICE_GATEWAY_CLASSIFICATION_REMOVAL.md
type ProcessingSettings struct {
	Deduplication DeduplicationSettings `yaml:"deduplication"`
	CRD           CRDSettings           `yaml:"crd"`
	Retry         RetrySettings         `yaml:"retry"` // BR-GATEWAY-111: K8s API retry configuration
}

// DeduplicationSettings contains deduplication configuration.
//
// DEPRECATED: TTL-based deduplication removed in DD-GATEWAY-011
// Gateway now uses status-based deduplication via RemediationRequest CRD phase.
// The TTL field is preserved for backwards compatibility only - it is NOT used.
type DeduplicationSettings struct {
	// TTL for deduplication fingerprints
	//
	// DEPRECATED: No longer used (DD-GATEWAY-011)
	// Gateway uses RemediationRequest CRD phase for deduplication, not time-based expiration.
	// This field is parsed for backwards compatibility but has NO EFFECT on Gateway behavior.
	//
	// Migration: Remove this field from your configuration files.
	// Status-based deduplication is automatic and requires no configuration.
	TTL time.Duration `yaml:"ttl"` // DEPRECATED: No effect
}

// Note: EnvironmentSettings struct removed (2025-12-06)
// Environment classification now owned by Signal Processing per DD-CATEGORIZATION-001

// CRDSettings contains CRD creation configuration.
//
// Note: FallbackNamespace field removed (February 2026).
// Namespace fallback is deprecated per DD-GATEWAY-007 (DEPRECATED).
// ADR-053 scope validation now rejects signals to unmanaged namespaces upstream,
// making CRD namespace fallback redundant.
type CRDSettings struct {
	// Reserved for future CRD creation configuration
	// FallbackNamespace was removed - see DD-GATEWAY-007 deprecation notice
}

// RetrySettings configures retry behavior for transient K8s API errors.
// BR-GATEWAY-111: Retry Configuration
// BR-GATEWAY-112: Error Classification
// BR-GATEWAY-113: Exponential Backoff
type RetrySettings struct {
	// Maximum number of retry attempts for transient K8s API errors
	// Example: MaxAttempts=3 means 1 original attempt + 2 retries
	// Default: 3
	// Reliability-First: Always retry transient errors (429, 503, 504, timeouts, network errors)
	MaxAttempts int `yaml:"maxAttempts"`

	// Initial backoff duration (doubles with each retry)
	// Example: 100ms → 200ms → 400ms → 800ms (exponential backoff)
	// Default: 100ms
	// Reliability-First: Fast initial retry, exponential backoff for persistent failures
	InitialBackoff time.Duration `yaml:"initialBackoff"`

	// Maximum backoff duration (cap for exponential backoff)
	// Prevents excessive wait times during retry storms
	// Default: 5s
	// Reliability-First: Reasonable cap to avoid blocking too long
	MaxBackoff time.Duration `yaml:"maxBackoff"`
}

// DefaultRetrySettings returns sensible defaults for Phase 1 (synchronous retry).
// BR-GATEWAY-111: Default Configuration (Reliability-First Design)
//
// Design Philosophy: Maximize reliability by default. These defaults work for 99% of use cases.
// Only tune if you have specific performance requirements or constraints.
func DefaultRetrySettings() RetrySettings {
	return RetrySettings{
		MaxAttempts:    3,                      // 1 original + 2 retries
		InitialBackoff: 100 * time.Millisecond, // Fast initial retry
		MaxBackoff:     5 * time.Second,        // Reasonable cap
	}
}

// Validate checks if retry settings are valid.
// GAP-8: Enhanced Configuration Validation (Reliability-First Design)
// Provides comprehensive validation with actionable error messages using structured errors
func (r *RetrySettings) Validate() error {
	// MaxAttempts validation with reasonable range
	if r.MaxAttempts < 1 {
		err := NewConfigError(
			"processing.retry.maxAttempts",
			fmt.Sprintf("%d", r.MaxAttempts),
			"must be >= 1",
			"Use 3-5 for production (recommended: 3)",
		)
		err.Impact = "Retry logic will not function properly"
		err.Documentation = "docs/services/stateless/gateway-service/configuration.md#retry"
		return err
	}
	if r.MaxAttempts > 10 {
		err := NewConfigError(
			"processing.retry.maxAttempts",
			fmt.Sprintf("%d", r.MaxAttempts),
			"exceeds recommended maximum (10)",
			"Reduce to 3-5 to avoid excessive retry delays",
		)
		err.Impact = "May cause slow request processing during failures"
		err.Documentation = "docs/services/stateless/gateway-service/configuration.md#retry"
		return err
	}

	// Initial backoff validation
	if r.InitialBackoff < 0 {
		err := NewConfigError(
			"processing.retry.initialBackoff",
			r.InitialBackoff.String(),
			"must be >= 0",
			"Use 100ms-500ms (recommended: 100ms)",
		)
		err.Impact = "Negative backoff is invalid"
		err.Documentation = "docs/services/stateless/gateway-service/configuration.md#retry"
		return err
	}
	if r.InitialBackoff > 5*time.Second {
		err := NewConfigError(
			"processing.retry.initialBackoff",
			r.InitialBackoff.String(),
			"exceeds recommended maximum (5s)",
			"Reduce to 100ms-500ms for faster failure detection",
		)
		err.Impact = "High initial backoff may cause slow failure detection"
		err.Documentation = "docs/services/stateless/gateway-service/configuration.md#retry"
		return err
	}

	// Max backoff validation
	if r.MaxBackoff < r.InitialBackoff {
		err := NewConfigError(
			"processing.retry.maxBackoff",
			fmt.Sprintf("%v (initial: %v)", r.MaxBackoff, r.InitialBackoff),
			"must be >= initialBackoff",
			fmt.Sprintf("Set maxBackoff to at least %v", r.InitialBackoff),
		)
		err.Impact = "Invalid exponential backoff configuration"
		err.Documentation = "docs/services/stateless/gateway-service/configuration.md#retry"
		return err
	}
	if r.MaxBackoff > 30*time.Second {
		err := NewConfigError(
			"processing.retry.maxBackoff",
			r.MaxBackoff.String(),
			"exceeds recommended maximum (30s)",
			"Reduce to 5s (recommended) to avoid long request delays",
		)
		err.Impact = "Excessive backoff may cause long request delays"
		err.Documentation = "docs/services/stateless/gateway-service/configuration.md#retry"
		return err
	}

	return nil
}

// LoadFromFile loads configuration from a YAML file
func LoadFromFile(path string) (*ServerConfig, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("failed to read config file: %w", err)
	}

	var cfg ServerConfig
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return nil, fmt.Errorf("failed to parse config: %w", err)
	}

	// Apply retry defaults if not configured
	// BR-GATEWAY-111: Default retry configuration
	if cfg.Processing.Retry.MaxAttempts == 0 {
		defaults := DefaultRetrySettings()
		cfg.Processing.Retry = defaults
	}

	// Apply server defaults for concurrency limiting
	// ADR-048-ADDENDUM-001: Default to 100 concurrent requests (defense-in-depth with Nginx/HAProxy)
	// 0 = unlimited (not recommended for production)
	// 100 = sensible default that prevents per-pod overload
	if cfg.Server.MaxConcurrentRequests == 0 {
		cfg.Server.MaxConcurrentRequests = 100
	}

	return &cfg, nil
}

// LoadFromEnv overrides configuration values with environment variables.
// ADR-030: Environment variables ONLY for secrets (never for functional config).
func (c *ServerConfig) LoadFromEnv() {
	// ADR-030: DataStorage URL now comes from YAML ConfigMap only (not env vars).
	// GATEWAY_DATA_STORAGE_URL env override removed -- use datastorage.url in YAML.

	// Processing settings (DEPRECATED: TTL has no effect per DD-GATEWAY-011)
	if dedupTTLStr := os.Getenv("GATEWAY_DEDUP_TTL"); dedupTTLStr != "" {
		if dedupTTL, err := time.ParseDuration(dedupTTLStr); err == nil {
			c.Processing.Deduplication.TTL = dedupTTL
		}
	}
}

// Validate checks if the configuration is valid.
// GAP-8: Enhanced Configuration Validation (Production-Ready)
// Provides comprehensive validation with actionable error messages using structured errors
func (c *ServerConfig) Validate() error {
	// Server validation with comprehensive timeout checks
	if c.Server.ListenAddr == "" {
		err := NewConfigError(
			"server.listenAddr",
			"(empty)",
			"is required",
			"Use ':8080' or '0.0.0.0:8080'",
		)
		err.Impact = "Gateway server will fail to start"
		err.Documentation = "docs/services/stateless/gateway-service/configuration.md#server"
		return err
	}

	// MaxConcurrentRequests validation (0 = unlimited, >0 = throttle enabled)
	// ADR-048-ADDENDUM-001: Defense-in-depth with Nginx/HAProxy
	if c.Server.MaxConcurrentRequests < 0 {
		err := NewConfigError(
			"server.maxConcurrentRequests",
			fmt.Sprintf("%d", c.Server.MaxConcurrentRequests),
			"must be >= 0",
			"Use 100 (recommended), 0 for unlimited",
		)
		err.Impact = "Invalid throttle configuration"
		err.Documentation = "docs/services/stateless/gateway-service/configuration.md#server"
		return err
	}
	if c.Server.MaxConcurrentRequests > 10000 {
		err := NewConfigError(
			"server.maxConcurrentRequests",
			fmt.Sprintf("%d", c.Server.MaxConcurrentRequests),
			"exceeds recommended maximum (10000)",
			"Use 100-1000 for production (recommended: 100)",
		)
		err.Impact = "May not provide effective overload protection"
		err.Documentation = "docs/services/stateless/gateway-service/configuration.md#server"
		return err
	}

	// Server timeout validation (prevent misconfiguration)
	if c.Server.ReadTimeout > 0 && c.Server.ReadTimeout < 5*time.Second {
		err := NewConfigError(
			"server.readTimeout",
			c.Server.ReadTimeout.String(),
			"is too low (< 5s)",
			"Use 30s (recommended) to prevent webhook timeouts",
		)
		err.Impact = "Webhook requests may timeout prematurely"
		err.Documentation = "docs/services/stateless/gateway-service/configuration.md#server"
		return err
	}
	if c.Server.WriteTimeout > 0 && c.Server.WriteTimeout < 5*time.Second {
		err := NewConfigError(
			"server.writeTimeout",
			c.Server.WriteTimeout.String(),
			"is too low (< 5s)",
			"Use 30s (recommended) to prevent response failures",
		)
		err.Impact = "Response writes may fail prematurely"
		err.Documentation = "docs/services/stateless/gateway-service/configuration.md#server"
		return err
	}
	if c.Server.IdleTimeout > 0 && c.Server.IdleTimeout < 30*time.Second {
		err := NewConfigError(
			"server.idleTimeout",
			c.Server.IdleTimeout.String(),
			"is too low (< 30s)",
			"Use 120s (recommended) to reduce connection churn",
		)
		err.Impact = "May increase connection establishment overhead"
		err.Documentation = "docs/services/stateless/gateway-service/configuration.md#server"
		return err
	}

	// Deduplication TTL validation
	// DEPRECATED: TTL-based deduplication removed in DD-GATEWAY-011
	// Gateway now uses status-based deduplication via RemediationRequest CRD phase.
	// Validation kept for backwards compatibility only - field has NO EFFECT.
	if c.Processing.Deduplication.TTL < 0 {
		err := NewConfigError(
			"processing.deduplication.ttl",
			c.Processing.Deduplication.TTL.String(),
			"must be >= 0",
			"DEPRECATED: This field no longer affects Gateway behavior (DD-GATEWAY-011). Remove from config.",
		)
		err.Impact = "Negative TTL is invalid (but field is deprecated and unused)"
		err.Documentation = "docs/services/stateless/gateway-service/configuration.md#deduplication"
		return err
	}
	if c.Processing.Deduplication.TTL > 0 && c.Processing.Deduplication.TTL < 10*time.Second {
		err := NewConfigError(
			"processing.deduplication.ttl",
			c.Processing.Deduplication.TTL.String(),
			"below minimum threshold (< 10s)",
			"DEPRECATED: This field no longer affects Gateway behavior (DD-GATEWAY-011). Remove from config.",
		)
		err.Impact = "Invalid range (but field is deprecated and unused)"
		err.Documentation = "docs/services/stateless/gateway-service/configuration.md#deduplication"
		return err
	}
	if c.Processing.Deduplication.TTL > 24*time.Hour {
		err := NewConfigError(
			"processing.deduplication.ttl",
			c.Processing.Deduplication.TTL.String(),
			"exceeds recommended maximum (24h)",
			"DEPRECATED: This field no longer affects Gateway behavior (DD-GATEWAY-011). Remove from config.",
		)
		err.Impact = "Exceeds maximum (but field is deprecated and unused)"
		err.Documentation = "docs/services/stateless/gateway-service/configuration.md#deduplication"
		return err
	}

	// ADR-030: Validate DataStorage section
	if err := sharedconfig.ValidateDataStorageConfig(&c.DataStorage); err != nil {
		return err
	}

	// Middleware validation
	// Rate limiting removed (ADR-048) - delegated to proxy
	// No validation needed for middleware

	// Retry validation (GAP-8: Enhanced Configuration Validation)
	if err := c.Processing.Retry.Validate(); err != nil {
		return err // Already a structured ConfigError
	}

	return nil
}
