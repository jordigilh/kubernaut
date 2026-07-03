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
	"crypto/tls"
	"crypto/x509"
	"fmt"
	"os"
	"strings"
	"time"

	sharedtls "github.com/jordigilh/kubernaut/pkg/shared/tls" // Issue #678: TLS config
	"gopkg.in/yaml.v3"
)

// ========================================
// DATA STORAGE SERVICE CONFIGURATION (ADR-030)
// 📋 Implementation Plan: Day 11 - GAP-03
// Authority: config/data-storage.yaml is source of truth
// Pattern: Context API config.go (authoritative reference)
// ========================================
//
// ADR-030 Configuration Management Standard:
// 1. YAML file is source of truth (loaded as ConfigMap in Kubernetes)
// 2. Secrets loaded from mounted files (ADR-030 Section 6)
// 3. Validate configuration before service startup
//
// Context API Pattern Applied:
// - LoadFromFile() reads YAML
// - LoadSecrets() loads secrets from mounted files
// - Validate() ensures configuration integrity
// ========================================

// Config represents the complete Data Storage service configuration
type Config struct {
	// Environment controls security defaults: "production" enforces TLS and restrictive CORS.
	// FED-C1/SC-8: In production, sslMode=disable and redis.tls.enabled=false are rejected.
	Environment string `yaml:"environment,omitempty"`

	Server    ServerConfig    `yaml:"server"`
	Logging   LoggingConfig   `yaml:"logging"`
	Database  DatabaseConfig  `yaml:"database"`
	Redis     RedisConfig     `yaml:"redis"`
	Retention RetentionConfig `yaml:"retention"`
	Audit     AuditConfig     `yaml:"audit,omitempty"`

	// TLSProfile selects the TLS security profile (Old/Intermediate/Modern).
	// Issue #748: OCP-only — set by kubernaut-operator from the cluster APIServer CR.
	TLSProfile string `yaml:"tlsProfile,omitempty"`
}

// IsProduction returns true when the environment is set to "production".
func (c *Config) IsProduction() bool {
	return strings.EqualFold(c.Environment, "production")
}

// ServerConfig contains HTTP server configuration
type ServerConfig struct {
	Port             int                 `yaml:"port"`
	Host             string              `yaml:"host"`
	MetricsPort      int                 `yaml:"metricsPort"`      // Dedicated Prometheus metrics port (default: 9090, Issue #283)
	HealthPort       int                 `yaml:"healthPort"`       // Dedicated health probe port (default: 8081, Issue #753)
	DisableProfiling bool                `yaml:"disableProfiling"` // Set true to suppress /debug/pprof/* on health port
	MaxBatchSize     int                 `yaml:"maxBatchSize"`     // Issue #667: Max events per batch API request (default: 500)
	ReadTimeout      string              `yaml:"readTimeout"`      // e.g., "30s"
	WriteTimeout     string              `yaml:"writeTimeout"`     // e.g., "30s"
	ShutdownTimeout  string              `yaml:"shutdownTimeout"`  // DD-007: graceful shutdown budget, e.g. "60s" (default: 60s, range: 30s–120s)
	TLS              sharedtls.TLSConfig `yaml:"tls,omitempty"`    // Issue #678: Optional inter-service TLS

	// #1048 Phase 4 / SC-5: Maximum request body size in bytes, e.g. "5242880" for 5 MiB
	// (default: 5242880 = 5 MiB, range: 1048576–52428800 = 1–50 MiB)
	MaxBodySize string `yaml:"maxBodySize,omitempty"`

	// #1048 Phase 4 / AC-4: CORS allowed origins.
	// Empty list denies all cross-origin requests (secure default).
	CORSAllowedOrigins []string `yaml:"corsAllowedOrigins,omitempty"`

	// #1048 Phase 5 / AU-9: Directory containing signing certificate (tls.crt, tls.key)
	// Default: /etc/certs. Configurable for Helm vs Kustomize path alignment.
	SignerCertDir string `yaml:"signerCertDir,omitempty"`

	// #1088 Phase 7 / SRE-L1: Time to wait for K8s endpoint removal propagation.
	// e.g., "5s" (default: 5s). Overrides the hardcoded endpointRemovalPropagationDelay.
	EndpointPropagationDelay string `yaml:"endpointPropagationDelay,omitempty"`

	// GAP-09 (Issue #1505) / SC-5: Per-IP rate limiting for the HTTP API.
	// Disabled by default for backward compatibility with existing deployments
	// that rely on an external ingress/proxy for rate limiting.
	RateLimit RateLimitConfig `yaml:"rateLimit,omitempty"`
}

// RateLimitConfig configures per-IP rate limiting on the Data Storage HTTP
// API (GAP-09, Issue #1505 / SC-5).
type RateLimitConfig struct {
	Enabled           bool    `yaml:"enabled"`
	RequestsPerSecond float64 `yaml:"requestsPerSecond,omitempty"`
	Burst             int     `yaml:"burst,omitempty"`
}

// GetRequestsPerSecond returns the configured per-IP request rate, defaulting
// to 50 req/s when unset or non-positive.
func (r *RateLimitConfig) GetRequestsPerSecond() float64 {
	if r.RequestsPerSecond <= 0 {
		return 50
	}
	return r.RequestsPerSecond
}

// GetBurst returns the configured per-IP burst size, defaulting to 100 when
// unset or non-positive.
func (r *RateLimitConfig) GetBurst() int {
	if r.Burst <= 0 {
		return 100
	}
	return r.Burst
}

// LoggingConfig contains logging configuration
type LoggingConfig struct {
	Level string `yaml:"level"` // debug, info, warn, error
}

// DatabaseConfig contains PostgreSQL database configuration
type DatabaseConfig struct {
	Host             string `yaml:"host"`
	Port             int    `yaml:"port"`
	Name             string `yaml:"name"`
	User             string `yaml:"user"`
	Password         string `yaml:"-"`                // NOT in YAML - loaded from secret file via LoadSecrets()
	SSLMode          string `yaml:"sslMode"`          // disable, require, verify-ca, verify-full
	MaxOpenConns     int    `yaml:"maxOpenConns"`     // Maximum open connections (default: 25)
	MaxIdleConns     int    `yaml:"maxIdleConns"`     // Maximum idle connections
	ConnMaxLifetime  string `yaml:"connMaxLifetime"`  // e.g., "5m"
	ConnMaxIdleTime  string `yaml:"connMaxIdleTime"`  // e.g., "10m"
	StatementTimeout string `yaml:"statementTimeout"` // Issue #667/M1: Per-statement timeout (default: "30s")
	LockTimeout      string `yaml:"lockTimeout"`      // Issue #667/M1: Per-lock wait timeout (default: "10s")

	// Secret file configuration (ADR-030 Section 6)
	SecretsFile string `yaml:"secretsFile"` // Full path to secret file, e.g., "/etc/secrets/database/credentials.yaml"
	UsernameKey string `yaml:"usernameKey"` // Key name for username in secret file (e.g., "username")
	PasswordKey string `yaml:"passwordKey"` // Key name for password in secret file (e.g., "password")
}

// RedisTLSConfig selects TLS settings for Redis/Valkey client connections (#1048 Phase 5 / AU-9).
// SC-8: TLS always validates the server certificate. For self-signed certs,
// mount the CA via caFile (Helm: redis.tls.caFile, Operator: kubernaut-operator#89).
type RedisTLSConfig struct {
	Enabled  bool   `yaml:"enabled"`
	CertFile string `yaml:"certFile"`
	KeyFile  string `yaml:"keyFile"`
	CAFile   string `yaml:"caFile"`
}

// BuildTLSConfig returns a tls.Config when TLS is enabled, or nil if disabled.
func (t *RedisTLSConfig) BuildTLSConfig() (*tls.Config, error) {
	if !t.Enabled {
		return nil, nil
	}

	tlsConfig := &tls.Config{
		MinVersion: tls.VersionTLS12,
	}

	if t.CAFile != "" {
		caCert, err := os.ReadFile(t.CAFile)
		if err != nil {
			return nil, fmt.Errorf("failed to read Redis CA file %s: %w", t.CAFile, err)
		}
		caCertPool := x509.NewCertPool()
		if !caCertPool.AppendCertsFromPEM(caCert) {
			return nil, fmt.Errorf("failed to parse CA certificate from %s", t.CAFile)
		}
		tlsConfig.RootCAs = caCertPool
	}

	if t.CertFile != "" && t.KeyFile != "" {
		cert, err := tls.LoadX509KeyPair(t.CertFile, t.KeyFile)
		if err != nil {
			return nil, fmt.Errorf("failed to load Redis client certificate: %w", err)
		}
		tlsConfig.Certificates = []tls.Certificate{cert}
	}

	return tlsConfig, nil
}

// RedisConfig contains Redis configuration for DLQ
type RedisConfig struct {
	Addr      string `yaml:"addr"`      // e.g., "localhost:6379"
	Password  string `yaml:"-"`         // NOT in YAML - loaded from secret file via LoadSecrets()
	DLQMaxLen int    `yaml:"dlqMaxLen"` // Maximum DLQ stream length

	// Secret file configuration (ADR-030 Section 6)
	SecretsFile string `yaml:"secretsFile"` // Full path to secret file, e.g., "/etc/secrets/redis/credentials.yaml"
	PasswordKey string `yaml:"passwordKey"` // Key name for password in secret file (e.g., "password")

	TLS RedisTLSConfig `yaml:"tls"` // #1048 Phase 5 / AU-9: Redis TLS for audit data transport
}

// AuditConfig contains audit hash-chain configuration.
// GAP-05 (Issue #1505): Optional keyed HMAC-SHA256 hash chain. When
// HashKeySecretsFile is unset, the datastorage falls back to the legacy
// unkeyed SHA256 algorithm for backward compatibility with environments that
// have not yet provisioned the audit HMAC key secret.
type AuditConfig struct {
	// Secret file configuration (ADR-030 Section 6), same pattern as
	// Database.SecretsFile / Redis.SecretsFile. Optional: leave unset to keep
	// using the legacy unkeyed SHA256 hash chain.
	HashKeySecretsFile string `yaml:"hashKeySecretsFile,omitempty"`
	HashKeyKey         string `yaml:"hashKeyKey,omitempty"` // Key name in the secret file (e.g., "hmacKey")

	// HMACKey is NOT in YAML - loaded from secret file via LoadSecrets().
	HMACKey []byte `yaml:"-"`
}

// RetentionConfig contains retention worker configuration.
// #1048 Phase 5 / AU-11, BR-AUDIT-009: Audit data retention enforcement.
type RetentionConfig struct {
	Enabled              bool   `yaml:"enabled"`              // Master switch (default: false, opt-in)
	Interval             string `yaml:"interval"`             // How often the worker runs (default: "24h")
	BatchSize            int    `yaml:"batchSize"`            // Max rows per DELETE batch (default: 1000)
	DefaultDays          int    `yaml:"defaultDays"`          // Application-level default retention (default: 2555 per ADR-034)
	PartitionDropEnabled bool   `yaml:"partitionDropEnabled"` // Whether to attempt DROP PARTITION on empty months
}

func (r *RetentionConfig) GetInterval() time.Duration {
	if r.Interval == "" {
		return 24 * time.Hour
	}
	d, err := time.ParseDuration(r.Interval)
	if err != nil {
		return 24 * time.Hour
	}
	return d
}

func (r *RetentionConfig) GetBatchSize() int {
	if r.BatchSize <= 0 {
		return 1000
	}
	return r.BatchSize
}

func (r *RetentionConfig) GetDefaultDays() int {
	if r.DefaultDays <= 0 {
		return 2555
	}
	if r.DefaultDays > 2555 {
		return 2555
	}
	return r.DefaultDays
}

// LoadFromFile loads configuration from a YAML file
// ADR-030: YAML file is the authoritative source of truth
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

// LoadSecrets, Validate, and the ServerConfig/DatabaseConfig accessor methods
// are defined in config_secrets.go, config_validate.go, and
// config_accessors.go respectively (GO-ANTIPATTERN-AUDIT-2026-07-01 Wave 3,
// pure code motion, no behavior change).
