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
	"strings"
	"time"
)

// Validate and its per-section validators (ADR-030: validate configuration
// before service startup). Split from config.go (GO-ANTIPATTERN-AUDIT-2026-07-01
// Wave 3, pure code motion, no behavior change); see config.go for the
// Config struct family and config_secrets.go for LoadSecrets.

// Validate checks if the configuration is valid and returns an error if not
// ADR-030: Validate configuration before service startup
//
// Decomposed into per-section validators (GO-ANTIPATTERN-AUDIT-2026-07-01
// Phase 4a) purely for readability/complexity — behavior, order, and error
// messages are unchanged from the pre-decomposition implementation.
func (c *Config) Validate() error {
	if err := c.validateDatabase(); err != nil {
		return err
	}
	if err := c.validateServer(); err != nil {
		return err
	}
	if err := c.validateRedis(); err != nil {
		return err
	}
	if err := c.validateProductionConstraints(); err != nil {
		return err
	}
	if err := c.validateRetention(); err != nil {
		return err
	}
	if err := c.validateLogging(); err != nil {
		return err
	}
	if err := c.validateTimeouts(); err != nil {
		return err
	}
	return c.validateCORS()
}

// validateDatabase checks required PostgreSQL connection fields and applies
// safe defaults for the connection pool and per-statement/per-lock timeouts.
func (c *Config) validateDatabase() error {
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
	return nil
}

// validateServer checks required HTTP server fields and applies safe
// defaults for the metrics/health ports and max batch size.
func (c *Config) validateServer() error {
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
	return nil
}

// validateRedis checks required Redis fields and rejects a half-configured
// TLS setup (enabled but no trust anchor) regardless of environment (SC-8).
func (c *Config) validateRedis() error {
	if c.Redis.Addr == "" {
		return fmt.Errorf("redis address required")
	}
	if c.Redis.TLS.Enabled {
		if c.Redis.TLS.CAFile == "" {
			return fmt.Errorf("redis TLS enabled but no caFile specified; mount the CA certificate (SC-8)")
		}
	}
	return nil
}

// validateProductionConstraints rejects insecure transport/CORS
// configurations when Environment is "production" (FED-C1/SC-8, AC-4).
func (c *Config) validateProductionConstraints() error {
	if !c.IsProduction() {
		return nil
	}
	if c.Database.SSLMode == "" || c.Database.SSLMode == "disable" {
		return fmt.Errorf("database sslMode must not be 'disable' in production (SC-8); use verify-full or verify-ca")
	}
	if !c.Redis.TLS.Enabled {
		return fmt.Errorf("redis TLS must be enabled in production (SC-8); set redis.tls.enabled=true")
	}
	for _, origin := range c.Server.CORSAllowedOrigins {
		if origin == "*" {
			return fmt.Errorf("CORS wildcard origin '*' is not allowed in production (AC-4); use explicit origins")
		}
	}
	return nil
}

// validateRetention checks the audit-retention worker configuration (AU-11).
func (c *Config) validateRetention() error {
	if c.Retention.Interval != "" {
		if _, err := time.ParseDuration(c.Retention.Interval); err != nil {
			return fmt.Errorf("invalid retention interval: %w", err)
		}
	}
	if c.Retention.DefaultDays < 0 {
		return fmt.Errorf("retention defaultDays must be non-negative, got: %d", c.Retention.DefaultDays)
	}
	return nil
}

// validateLogging checks the configured log level (case-insensitive per Issue #875).
func (c *Config) validateLogging() error {
	validLevels := map[string]bool{
		"DEBUG": true,
		"INFO":  true,
		"WARN":  true,
		"ERROR": true,
	}
	if c.Logging.Level != "" && !validLevels[strings.ToUpper(c.Logging.Level)] {
		return fmt.Errorf("invalid log level: %s (must be DEBUG, INFO, WARN, or ERROR)", c.Logging.Level)
	}
	return nil
}

// validateTimeouts parses every configurable duration/size field to fail
// fast at startup instead of silently defaulting at runtime (SI-10).
// durationField pairs a config field's human-readable name (used in error
// messages) with its raw string value, for uniform duration validation.
type durationField struct {
	name  string
	value string
}

// validateDurationFields fails fast (SI-10) on the first field whose value is
// a non-empty, non-parseable duration string.
func validateDurationFields(fields []durationField) error {
	for _, f := range fields {
		if f.value == "" {
			continue
		}
		if _, err := time.ParseDuration(f.value); err != nil {
			return fmt.Errorf("invalid %s: %w", f.name, err)
		}
	}
	return nil
}

func (c *Config) validateTimeouts() error {
	// SI-10: Fail fast on invalid timeout/shutdown/propagation durations
	// instead of silently defaulting at runtime where operators may not notice.
	if err := validateDurationFields([]durationField{
		{"readTimeout", c.Server.ReadTimeout},
		{"writeTimeout", c.Server.WriteTimeout},
		{"connMaxLifetime", c.Database.ConnMaxLifetime},
		{"connMaxIdleTime", c.Database.ConnMaxIdleTime},
		{"shutdownTimeout", c.Server.ShutdownTimeout},
		{"endpointPropagationDelay", c.Server.EndpointPropagationDelay},
	}); err != nil {
		return err
	}

	if c.Server.MaxBodySize != "" {
		var size int64
		if n, err := fmt.Sscanf(c.Server.MaxBodySize, "%d", &size); err != nil || n != 1 {
			return fmt.Errorf("invalid maxBodySize %q: must be an integer (bytes)", c.Server.MaxBodySize)
		}
	}
	return nil
}

// validateCORS checks each configured CORS origin's format, aligned with
// go-chi/cors semantics (AC-4/SI-10).
func (c *Config) validateCORS() error {
	for i, origin := range c.Server.CORSAllowedOrigins {
		trimmed := strings.TrimSpace(origin)
		if trimmed == "" {
			return fmt.Errorf("corsAllowedOrigins[%d]: empty or whitespace-only origin", i)
		}
		if trimmed == "*" {
			continue
		}
		lower := strings.ToLower(trimmed)
		if !strings.HasPrefix(lower, "http://") && !strings.HasPrefix(lower, "https://") {
			return fmt.Errorf("corsAllowedOrigins[%d] %q: must start with http:// or https:// (or be \"*\")", i, origin)
		}
	}
	return nil
}
