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
	"strconv"
	"strings"
	"time"

	rediscache "github.com/jordigilh/kubernaut/pkg/cache/redis"
	"gopkg.in/yaml.v3"
)

// ServerConfig is the top-level configuration for the Gateway service.
// Organized by Single Responsibility Principle for better maintainability.
type ServerConfig struct {
	// HTTP Server configuration
	Server ServerSettings `yaml:"server"`

	// Middleware configuration
	Middleware MiddlewareSettings `yaml:"middleware"`

	// Infrastructure dependencies
	Infrastructure InfrastructureSettings `yaml:"infrastructure"`

	// Business logic configuration
	Processing ProcessingSettings `yaml:"processing"`
}

// ServerSettings contains HTTP server configuration.
// Single Responsibility: HTTP server behavior
type ServerSettings struct {
	ListenAddr   string        `yaml:"listen_addr"`   // Default: ":8080"
	ReadTimeout  time.Duration `yaml:"read_timeout"`  // Default: 30s
	WriteTimeout time.Duration `yaml:"write_timeout"` // Default: 30s
	IdleTimeout  time.Duration `yaml:"idle_timeout"`  // Default: 120s
}

// MiddlewareSettings contains middleware configuration.
// Single Responsibility: Request processing middleware
type MiddlewareSettings struct {
	RateLimit RateLimitSettings `yaml:"rate_limit"`
}

// RateLimitSettings contains rate limiting configuration.
type RateLimitSettings struct {
	RequestsPerMinute int `yaml:"requests_per_minute"` // Default: 100
	Burst             int `yaml:"burst"`               // Default: 10
}

// InfrastructureSettings contains external dependency configuration.
// Single Responsibility: Infrastructure connections
type InfrastructureSettings struct {
	// Redis uses the shared Redis configuration from pkg/cache/redis (DD-CACHE-001)
	Redis *rediscache.Options `yaml:"redis"`
}

// ProcessingSettings contains business logic configuration.
// Single Responsibility: Signal processing behavior
type ProcessingSettings struct {
	Deduplication DeduplicationSettings `yaml:"deduplication"`
	Storm         StormSettings         `yaml:"storm"`
	Environment   EnvironmentSettings   `yaml:"environment"`
	Priority      PrioritySettings      `yaml:"priority"`
	CRD           CRDSettings           `yaml:"crd"`
	Retry         RetrySettings         `yaml:"retry"` // BR-GATEWAY-111: K8s API retry configuration
}

// PrioritySettings contains priority assignment configuration.
type PrioritySettings struct {
	// PolicyPath is the path to the Rego policy file for priority assignment
	PolicyPath string `yaml:"policy_path"`
}

// DeduplicationSettings contains deduplication configuration.
type DeduplicationSettings struct {
	// TTL for deduplication fingerprints
	// For testing: set to 5*time.Second for fast tests
	// For production: use default (0) for 5-minute TTL
	TTL time.Duration `yaml:"ttl"` // Default: 5m
}

// StormSettings contains storm detection configuration.
type StormSettings struct {
	// ===== EXISTING FIELDS (keep as-is) =====
	// Rate threshold for rate-based storm detection
	// For testing: set to 2-3 for early storm detection in tests
	// For production: use default (0) for 10 alerts/minute
	RateThreshold int `yaml:"rate_threshold"` // Default: 10 alerts/minute

	// Pattern threshold for pattern-based storm detection
	// For testing: set to 2-3 for early storm detection in tests
	// For production: use default (0) for 5 similar alerts
	PatternThreshold int `yaml:"pattern_threshold"` // Default: 5 similar alerts

	// Aggregation window for storm aggregation
	// For testing: set to 5*time.Second for fast integration tests
	// For production: use default (0) for 1-minute windows
	AggregationWindow time.Duration `yaml:"aggregation_window"` // Default: 1m

	// ===== NEW FIELDS for DD-GATEWAY-008 =====
	// Buffered first-alert configuration
	BufferThreshold int `yaml:"buffer_threshold"` // Alerts before creating window (default: 5)

	// Sliding window configuration
	InactivityTimeout time.Duration `yaml:"inactivity_timeout"`  // Sliding window timeout (default: 60s)
	MaxWindowDuration time.Duration `yaml:"max_window_duration"` // Max window duration (default: 5m)

	// Multi-tenant isolation configuration
	DefaultMaxSize     int            `yaml:"default_max_size"`     // Default namespace buffer size (default: 1000)
	GlobalMaxSize      int            `yaml:"global_max_size"`      // Global buffer limit (default: 5000)
	PerNamespaceLimits map[string]int `yaml:"per_namespace_limits"` // Per-namespace overrides

	// Overflow handling configuration
	SamplingThreshold float64 `yaml:"sampling_threshold"` // Utilization to trigger sampling (default: 0.95)
	SamplingRate      float64 `yaml:"sampling_rate"`      // Sample rate when threshold reached (default: 0.5)
}

// EnvironmentSettings contains environment classification configuration.
type EnvironmentSettings struct {
	// Cache TTL for namespace label cache
	// For testing: set to 5*time.Second for fast cache expiry in tests
	// For production: use default (0) for 30-second TTL
	CacheTTL time.Duration `yaml:"cache_ttl"` // Default: 30s

	// ConfigMap for environment overrides
	ConfigMapNamespace string `yaml:"configmap_namespace"` // Default: "kubernaut-system"
	ConfigMapName      string `yaml:"configmap_name"`      // Default: "kubernaut-environment-overrides"
}

// CRDSettings contains CRD creation configuration.
type CRDSettings struct {
	// Fallback namespace for CRD creation when target namespace doesn't exist
	// This handles cluster-scoped signals (e.g., NodeNotReady) that don't have a namespace
	// Default: auto-detect from pod's namespace (/var/run/secrets/kubernetes.io/serviceaccount/namespace)
	// Override: set explicitly for multi-tenant or special scenarios
	// If auto-detect fails (non-K8s environment), falls back to "kubernaut-system"
	FallbackNamespace string `yaml:"fallback_namespace"` // Default: auto-detect pod namespace
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
	MaxAttempts int `yaml:"max_attempts"`

	// Initial backoff duration (doubles with each retry)
	// Example: 100ms → 200ms → 400ms → 800ms (exponential backoff)
	// Default: 100ms
	// Reliability-First: Fast initial retry, exponential backoff for persistent failures
	InitialBackoff time.Duration `yaml:"initial_backoff"`

	// Maximum backoff duration (cap for exponential backoff)
	// Prevents excessive wait times during retry storms
	// Default: 5s
	// Reliability-First: Reasonable cap to avoid blocking too long
	MaxBackoff time.Duration `yaml:"max_backoff"`
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
// GAP 8: Configuration Validation (Reliability-First Design)
func (r *RetrySettings) Validate() error {
	// MaxAttempts validation
	if r.MaxAttempts < 1 {
		return fmt.Errorf("retry.max_attempts must be >= 1, got %d", r.MaxAttempts)
	}

	// Backoff duration validation
	if r.InitialBackoff < 0 {
		return fmt.Errorf("retry.initial_backoff must be >= 0, got %v", r.InitialBackoff)
	}
	if r.MaxBackoff < r.InitialBackoff {
		return fmt.Errorf("retry.max_backoff (%v) must be >= initial_backoff (%v)",
			r.MaxBackoff, r.InitialBackoff)
	}

	return nil
}

// GetPodNamespace auto-detects the pod's namespace from the service account mount.
// This is the standard Kubernetes pattern for in-cluster namespace detection.
//
// Returns:
//   - Pod's namespace if running in Kubernetes cluster
//   - "kubernaut-system" as fallback for non-K8s environments (e.g., local dev)
func GetPodNamespace() string {
	// Standard Kubernetes service account namespace file
	const namespaceFile = "/var/run/secrets/kubernetes.io/serviceaccount/namespace"

	data, err := os.ReadFile(namespaceFile)
	if err != nil {
		// Not running in Kubernetes cluster (e.g., local development)
		// Fall back to default production namespace
		return "kubernaut-system"
	}

	namespace := strings.TrimSpace(string(data))
	if namespace == "" {
		// Empty namespace file (should never happen, but be defensive)
		return "kubernaut-system"
	}

	return namespace
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

	// Apply smart defaults for CRD fallback namespace
	// If not explicitly configured, auto-detect from pod's namespace
	if cfg.Processing.CRD.FallbackNamespace == "" {
		cfg.Processing.CRD.FallbackNamespace = GetPodNamespace()
	}

	// Apply retry defaults if not configured
	// BR-GATEWAY-111: Default retry configuration
	if cfg.Processing.Retry.MaxAttempts == 0 {
		defaults := DefaultRetrySettings()
		cfg.Processing.Retry = defaults
	}

	return &cfg, nil
}

// LoadFromEnv overrides configuration values with environment variables
// This allows secrets and deployment-specific values to override YAML configuration
func (c *ServerConfig) LoadFromEnv() {
	// Server settings
	if addr := os.Getenv("GATEWAY_LISTEN_ADDR"); addr != "" {
		c.Server.ListenAddr = addr
	}

	// Redis settings
	if redisAddr := os.Getenv("GATEWAY_REDIS_ADDR"); redisAddr != "" {
		c.Infrastructure.Redis.Addr = redisAddr
	}

	if redisPassword := os.Getenv("GATEWAY_REDIS_PASSWORD"); redisPassword != "" {
		c.Infrastructure.Redis.Password = redisPassword
	}

	if redisDBStr := os.Getenv("GATEWAY_REDIS_DB"); redisDBStr != "" {
		if redisDB, err := strconv.Atoi(redisDBStr); err == nil {
			c.Infrastructure.Redis.DB = redisDB
		}
	}

	// Middleware settings
	if rpmStr := os.Getenv("GATEWAY_RATE_LIMIT_RPM"); rpmStr != "" {
		if rpm, err := strconv.Atoi(rpmStr); err == nil {
			c.Middleware.RateLimit.RequestsPerMinute = rpm
		}
	}

	// Processing settings
	if dedupTTLStr := os.Getenv("GATEWAY_DEDUP_TTL"); dedupTTLStr != "" {
		if dedupTTL, err := time.ParseDuration(dedupTTLStr); err == nil {
			c.Processing.Deduplication.TTL = dedupTTL
		}
	}
}

// Validate checks if the configuration is valid
func (c *ServerConfig) Validate() error {
	// Server validation
	if c.Server.ListenAddr == "" {
		return fmt.Errorf("server.listen_addr required")
	}

	// Infrastructure validation
	if c.Infrastructure.Redis == nil {
		return fmt.Errorf("infrastructure.redis required")
	}
	if c.Infrastructure.Redis.Addr == "" {
		return fmt.Errorf("infrastructure.redis address required")
	}

	// Middleware validation
	if c.Middleware.RateLimit.RequestsPerMinute < 0 {
		return fmt.Errorf("middleware.rate_limit.requests_per_minute must be positive")
	}
	if c.Middleware.RateLimit.Burst < 0 {
		return fmt.Errorf("middleware.rate_limit.burst must be non-negative")
	}

	// Processing validation
	if c.Processing.Storm.RateThreshold < 0 {
		return fmt.Errorf("processing.storm.rate_threshold must be positive")
	}
	if c.Processing.Storm.PatternThreshold < 0 {
		return fmt.Errorf("processing.storm.pattern_threshold must be positive")
	}

	// Retry validation (GAP 8: Configuration Validation)
	if err := c.Processing.Retry.Validate(); err != nil {
		return fmt.Errorf("retry configuration invalid: %w", err)
	}

	return nil
}
