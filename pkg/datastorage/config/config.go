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
	"encoding/json"
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
	Server    ServerConfig    `yaml:"server"`
	Logging   LoggingConfig   `yaml:"logging"`
	Database  DatabaseConfig  `yaml:"database"`
	Redis     RedisConfig     `yaml:"redis"`
	Retention RetentionConfig `yaml:"retention"`

	// TLSProfile selects the TLS security profile (Old/Intermediate/Modern).
	// Issue #748: OCP-only — set by kubernaut-operator from the cluster APIServer CR.
	TLSProfile string `yaml:"tlsProfile,omitempty"`
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

	// #1048 Phase 4 / AC-4: CORS allowed origins (default: ["*"] with startup warning)
	CORSAllowedOrigins []string `yaml:"corsAllowedOrigins,omitempty"`

	// #1048 Phase 5 / AU-9: Directory containing signing certificate (tls.crt, tls.key)
	// Default: /etc/certs. Configurable for Helm vs Kustomize path alignment.
	SignerCertDir string `yaml:"signerCertDir,omitempty"`

	// #1088 Phase 7 / SRE-L1: Time to wait for K8s endpoint removal propagation.
	// e.g., "5s" (default: 5s). Overrides the hardcoded endpointRemovalPropagationDelay.
	EndpointPropagationDelay string `yaml:"endpointPropagationDelay,omitempty"`
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
type RedisTLSConfig struct {
	Enabled            bool   `yaml:"enabled"`
	CertFile           string `yaml:"certFile"`
	KeyFile            string `yaml:"keyFile"`
	CAFile             string `yaml:"caFile"`
	InsecureSkipVerify bool   `yaml:"insecureSkipVerify"`
}

// BuildTLSConfig returns a tls.Config when TLS is enabled, or nil if disabled.
func (t *RedisTLSConfig) BuildTLSConfig() (*tls.Config, error) {
	if !t.Enabled {
		return nil, nil
	}

	tlsConfig := &tls.Config{
		InsecureSkipVerify: t.InsecureSkipVerify, //nolint:gosec // G402: operator-controlled flag for dev/test environments
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

// LoadSecrets loads secrets from mounted Kubernetes Secret files (ADR-030 Section 6)
// It supports both YAML and JSON secret files.
// This function REQUIRES secretsFile to be configured for both database and redis.
func (c *Config) LoadSecrets() error {
	// Load database secrets (REQUIRED)
	if c.Database.SecretsFile == "" {
		return fmt.Errorf("database secretsFile required (ADR-030 Section 6)")
	}
	if c.Database.PasswordKey == "" {
		return fmt.Errorf("database passwordKey required (ADR-030 Section 6)")
	}

	dbSecrets, err := loadSecretFile(c.Database.SecretsFile)
	if err != nil {
		return fmt.Errorf("failed to load database secrets from %s: %w",
			c.Database.SecretsFile, err)
	}

	// Extract password using configured key
	password, ok := dbSecrets[c.Database.PasswordKey]
	if !ok {
		return fmt.Errorf("password key '%s' not found in database secret file %s",
			c.Database.PasswordKey, c.Database.SecretsFile)
	}

	passwordStr, isString := password.(string)
	if !isString {
		return fmt.Errorf("database password key '%s' in secret file is not a string",
			c.Database.PasswordKey)
	}

	c.Database.Password = passwordStr

	// Optional: Override username from secret if key specified
	if c.Database.UsernameKey != "" {
		if username, ok := dbSecrets[c.Database.UsernameKey]; ok {
			if usernameStr, isString := username.(string); isString {
				c.Database.User = usernameStr
			} else {
				return fmt.Errorf("database username key '%s' in secret file is not a string",
					c.Database.UsernameKey)
			}
		}
	}

	// Load Redis secrets (REQUIRED)
	if c.Redis.SecretsFile == "" {
		return fmt.Errorf("redis secretsFile required (ADR-030 Section 6)")
	}
	if c.Redis.PasswordKey == "" {
		return fmt.Errorf("redis passwordKey required (ADR-030 Section 6)")
	}

	redisSecrets, err := loadSecretFile(c.Redis.SecretsFile)
	if err != nil {
		return fmt.Errorf("failed to load redis secrets from %s: %w",
			c.Redis.SecretsFile, err)
	}

	// Extract Redis password
	redisPassword, ok := redisSecrets[c.Redis.PasswordKey]
	if !ok {
		return fmt.Errorf("password key '%s' not found in redis secret file %s",
			c.Redis.PasswordKey, c.Redis.SecretsFile)
	}

	redisPasswordStr, isString := redisPassword.(string)
	if !isString {
		return fmt.Errorf("redis password key '%s' in secret file is not a string",
			c.Redis.PasswordKey)
	}

	c.Redis.Password = redisPasswordStr

	return nil
}

// loadSecretFile unmarshals a secret file (supports YAML and JSON)
// This is a helper function for LoadSecrets()
func loadSecretFile(secretFilePath string) (map[string]interface{}, error) {
	data, err := os.ReadFile(secretFilePath)
	if err != nil {
		return nil, fmt.Errorf("failed to read secret file %s: %w", secretFilePath, err)
	}

	var secrets map[string]interface{}

	// Try YAML first
	if err := yaml.Unmarshal(data, &secrets); err == nil {
		return secrets, nil
	}

	// Fallback to JSON
	if err := json.Unmarshal(data, &secrets); err != nil {
		return nil, fmt.Errorf("failed to parse secret file as YAML or JSON: %w", err)
	}

	return secrets, nil
}

// Validate checks if the configuration is valid and returns an error if not
// ADR-030: Validate configuration before service startup
func (c *Config) Validate() error {
	// Validate database configuration
	if c.Database.Host == "" {
		return fmt.Errorf("database host required")
	}
	if c.Database.Port == 0 {
		return fmt.Errorf("database port required")
	}
	if c.Database.Port < 1 || c.Database.Port > 65535 {
		return fmt.Errorf("database port must be between 1 and 65535, got: %d", c.Database.Port)
	}
	if c.Database.Name == "" {
		return fmt.Errorf("database name required")
	}
	if c.Database.User == "" {
		return fmt.Errorf("database user required")
	}

	// Issue #667/M3: Default MaxOpenConns to 25 to prevent unlimited connections
	if c.Database.MaxOpenConns == 0 {
		c.Database.MaxOpenConns = 25
	}
	if c.Database.MaxOpenConns < 0 {
		return fmt.Errorf("database maxOpenConns must be positive, got: %d", c.Database.MaxOpenConns)
	}

	// Issue #667/M1: Default statement_timeout and lock_timeout for PG-level safety
	if c.Database.StatementTimeout == "" {
		c.Database.StatementTimeout = "30s"
	}
	if _, err := time.ParseDuration(c.Database.StatementTimeout); err != nil {
		return fmt.Errorf("invalid statementTimeout: %w", err)
	}
	if c.Database.LockTimeout == "" {
		c.Database.LockTimeout = "10s"
	}
	if _, err := time.ParseDuration(c.Database.LockTimeout); err != nil {
		return fmt.Errorf("invalid lockTimeout: %w", err)
	}

	// Validate server configuration
	if c.Server.Port == 0 {
		return fmt.Errorf("server port required")
	}
	if c.Server.Port < 1024 || c.Server.Port > 65535 {
		return fmt.Errorf("server port must be between 1024 and 65535, got: %d", c.Server.Port)
	}

	// Issue #283: Default metricsPort to 9090 (Kubernaut standard) when omitted
	if c.Server.MetricsPort == 0 {
		c.Server.MetricsPort = 9090
	}

	// Issue #753: Default healthPort to 8081 (CONFIG_STANDARDS.md) when omitted
	if c.Server.HealthPort == 0 {
		c.Server.HealthPort = 8081
	}

	// Issue #667 / BR-STORAGE-043: Default MaxBatchSize to 500 when omitted
	if c.Server.MaxBatchSize == 0 {
		c.Server.MaxBatchSize = 500
	}
	if c.Server.MaxBatchSize < 0 {
		return fmt.Errorf("server maxBatchSize must be positive, got: %d", c.Server.MaxBatchSize)
	}
	if c.Server.MetricsPort < 1024 || c.Server.MetricsPort > 65535 {
		return fmt.Errorf("server metricsPort must be between 1024 and 65535, got: %d", c.Server.MetricsPort)
	}

	// Validate Redis configuration
	if c.Redis.Addr == "" {
		return fmt.Errorf("redis address required")
	}
	if c.Redis.TLS.Enabled {
		if c.Redis.TLS.CAFile == "" && !c.Redis.TLS.InsecureSkipVerify {
			return fmt.Errorf("redis TLS enabled but no caFile specified and insecureSkipVerify is false")
		}
	}

	// Validate retention configuration
	if c.Retention.Interval != "" {
		if _, err := time.ParseDuration(c.Retention.Interval); err != nil {
			return fmt.Errorf("invalid retention interval: %w", err)
		}
	}
	if c.Retention.DefaultDays < 0 {
		return fmt.Errorf("retention defaultDays must be non-negative, got: %d", c.Retention.DefaultDays)
	}

	// Validate logging configuration (case-insensitive per Issue #875)
	validLevels := map[string]bool{
		"DEBUG": true,
		"INFO":  true,
		"WARN":  true,
		"ERROR": true,
	}
	if c.Logging.Level != "" && !validLevels[strings.ToUpper(c.Logging.Level)] {
		return fmt.Errorf("invalid log level: %s (must be DEBUG, INFO, WARN, or ERROR)", c.Logging.Level)
	}

	// Validate timeout durations (parse to ensure valid format)
	if c.Server.ReadTimeout != "" {
		if _, err := time.ParseDuration(c.Server.ReadTimeout); err != nil {
			return fmt.Errorf("invalid readTimeout: %w", err)
		}
	}
	if c.Server.WriteTimeout != "" {
		if _, err := time.ParseDuration(c.Server.WriteTimeout); err != nil {
			return fmt.Errorf("invalid writeTimeout: %w", err)
		}
	}
	if c.Database.ConnMaxLifetime != "" {
		if _, err := time.ParseDuration(c.Database.ConnMaxLifetime); err != nil {
			return fmt.Errorf("invalid connMaxLifetime: %w", err)
		}
	}
	if c.Database.ConnMaxIdleTime != "" {
		if _, err := time.ParseDuration(c.Database.ConnMaxIdleTime); err != nil {
			return fmt.Errorf("invalid connMaxIdleTime: %w", err)
		}
	}

	return nil
}

// GetReadTimeout returns the read timeout as a time.Duration
func (c *ServerConfig) GetReadTimeout() time.Duration {
	if c.ReadTimeout == "" {
		return 30 * time.Second // default
	}
	duration, err := time.ParseDuration(c.ReadTimeout)
	if err != nil {
		return 30 * time.Second // fallback to default
	}
	return duration
}

// GetWriteTimeout returns the write timeout as a time.Duration
func (c *ServerConfig) GetWriteTimeout() time.Duration {
	if c.WriteTimeout == "" {
		return 30 * time.Second // default
	}
	duration, err := time.ParseDuration(c.WriteTimeout)
	if err != nil {
		return 30 * time.Second // fallback to default
	}
	return duration
}

// GetShutdownTimeout returns the graceful shutdown budget as a time.Duration.
// DD-007: Defaults to 60s. Clamped to [30s, 120s] for safety — a value below 30s
// risks data loss (DLQ drain alone needs 10s), a value above 120s wastes K8s resources.
func (c *ServerConfig) GetShutdownTimeout() time.Duration {
	const (
		defaultTimeout = 60 * time.Second
		minTimeout     = 30 * time.Second
		maxTimeout     = 120 * time.Second
	)
	if c.ShutdownTimeout == "" {
		return defaultTimeout
	}
	duration, err := time.ParseDuration(c.ShutdownTimeout)
	if err != nil {
		return defaultTimeout
	}
	if duration < minTimeout {
		return minTimeout
	}
	if duration > maxTimeout {
		return maxTimeout
	}
	return duration
}

// GetEndpointPropagationDelay returns the endpoint propagation delay as time.Duration.
// #1088 Phase 7 / SRE-L1: Defaults to 5s. Clamped to [1s, 30s] — below 1s risks
// receiving traffic before K8s propagation; above 30s wastes shutdown budget.
func (c *ServerConfig) GetEndpointPropagationDelay() time.Duration {
	const (
		defaultDelay = 5 * time.Second
		minDelay     = 1 * time.Second
		maxDelay     = 30 * time.Second
	)
	if c.EndpointPropagationDelay == "" {
		return defaultDelay
	}
	duration, err := time.ParseDuration(c.EndpointPropagationDelay)
	if err != nil {
		return defaultDelay
	}
	if duration < minDelay {
		return minDelay
	}
	if duration > maxDelay {
		return maxDelay
	}
	return duration
}

// GetMaxBodySize returns the maximum request body size in bytes.
// #1048 Phase 4 / SC-5: Defaults to 5 MiB. Clamped to [1 MiB, 50 MiB].
// 5 MiB accommodates batch audit event requests (500 events × ~2-5 KB each).
func (c *ServerConfig) GetMaxBodySize() int64 {
	const (
		mib         = 1 << 20
		defaultSize = 5 * mib
		minSize     = 1 * mib
		maxSize     = 50 * mib
	)
	if c.MaxBodySize == "" {
		return int64(defaultSize)
	}
	var size int64
	if n, err := fmt.Sscanf(c.MaxBodySize, "%d", &size); err == nil && n == 1 {
		// Plain integer in bytes
	} else {
		return int64(defaultSize)
	}
	if size < int64(minSize) {
		return int64(minSize)
	}
	if size > int64(maxSize) {
		return int64(maxSize)
	}
	return size
}

// GetCORSAllowedOrigins returns the configured CORS origins.
// #1048 Phase 4 / AC-4: Defaults to ["*"] for backward compatibility.
// Operators should configure explicit origins for production.
func (c *ServerConfig) GetCORSAllowedOrigins() []string {
	if len(c.CORSAllowedOrigins) == 0 {
		return []string{"*"}
	}
	return c.CORSAllowedOrigins
}

// GetSignerCertDir returns the directory holding tls.crt/tls.key for audit export signing.
// #1048 Phase 5 / AU-9: Defaults to /etc/certs when unset.
func (c *ServerConfig) GetSignerCertDir() string {
	if c.SignerCertDir == "" {
		return "/etc/certs"
	}
	return c.SignerCertDir
}

// GetConnectionString returns the PostgreSQL connection string with PG-level timeouts.
// Issue #667/M1: statement_timeout and lock_timeout are set as DSN options so PostgreSQL
// itself enforces limits, independent of Go-side context cancellation.
func (c *DatabaseConfig) GetConnectionString() string {
	dsn := fmt.Sprintf("host=%s port=%d dbname=%s user=%s password=%s sslmode=%s",
		c.Host, c.Port, c.Name, c.User, c.Password, c.SSLMode)

	var pgOpts []string
	if c.StatementTimeout != "" {
		if d, err := time.ParseDuration(c.StatementTimeout); err == nil {
			pgOpts = append(pgOpts, fmt.Sprintf("-c statement_timeout=%d", d.Milliseconds()))
		}
	}
	if c.LockTimeout != "" {
		if d, err := time.ParseDuration(c.LockTimeout); err == nil {
			pgOpts = append(pgOpts, fmt.Sprintf("-c lock_timeout=%d", d.Milliseconds()))
		}
	}
	if len(pgOpts) > 0 {
		dsn += fmt.Sprintf(" options='%s'", strings.Join(pgOpts, " "))
	}
	return dsn
}
