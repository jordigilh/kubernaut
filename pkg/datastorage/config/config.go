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
	"encoding/json"
	"fmt"
	"os"
	"time"

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
	Server   ServerConfig   `yaml:"server"`
	Logging  LoggingConfig  `yaml:"logging"`
	Database DatabaseConfig `yaml:"database"`
	Redis    RedisConfig    `yaml:"redis"`
}

// ServerConfig contains HTTP server configuration
type ServerConfig struct {
	Port         int    `yaml:"port"`
	Host         string `yaml:"host"`
	ReadTimeout  string `yaml:"read_timeout"`  // e.g., "30s"
	WriteTimeout string `yaml:"write_timeout"` // e.g., "30s"
}

// LoggingConfig contains logging configuration
type LoggingConfig struct {
	Level  string `yaml:"level"`  // debug, info, warn, error
	Format string `yaml:"format"` // json, console
}

// DatabaseConfig contains PostgreSQL database configuration
type DatabaseConfig struct {
	Host            string `yaml:"host"`
	Port            int    `yaml:"port"`
	Name            string `yaml:"name"`
	User            string `yaml:"user"`
	Password        string `yaml:"-"`                // NOT in YAML - loaded from secret file via LoadSecrets()
	SSLMode         string `yaml:"sslMode"`          // disable, require, verify-ca, verify-full
	MaxOpenConns    int    `yaml:"maxOpenConns"`     // Maximum open connections
	MaxIdleConns    int    `yaml:"maxIdleConns"`     // Maximum idle connections
	ConnMaxLifetime string `yaml:"connMaxLifetime"`  // e.g., "5m"
	ConnMaxIdleTime string `yaml:"connMaxIdleTime"`  // e.g., "10m"

	// Secret file configuration (ADR-030 Section 6)
	SecretsFile string `yaml:"secretsFile"` // Full path to secret file, e.g., "/etc/secrets/database/credentials.yaml"
	UsernameKey string `yaml:"usernameKey"` // Key name for username in secret file (e.g., "username")
	PasswordKey string `yaml:"passwordKey"` // Key name for password in secret file (e.g., "password")
}

// RedisConfig contains Redis configuration for DLQ
type RedisConfig struct {
	Addr             string `yaml:"addr"`             // e.g., "localhost:6379"
	DB               int    `yaml:"db"`               // Redis database number
	Password         string `yaml:"-"`                // NOT in YAML - loaded from secret file via LoadSecrets()
	DLQStreamName    string `yaml:"dlqStreamName"`    // DD-009: Dead Letter Queue stream name
	DLQMaxLen        int    `yaml:"dlqMaxLen"`        // Maximum DLQ stream length
	DLQConsumerGroup string `yaml:"dlqConsumerGroup"` // DLQ consumer group name

	// Secret file configuration (ADR-030 Section 6)
	SecretsFile string `yaml:"secretsFile"` // Full path to secret file, e.g., "/etc/secrets/redis/credentials.yaml"
	PasswordKey string `yaml:"passwordKey"` // Key name for password in secret file (e.g., "password")
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

	// Validate server configuration
	if c.Server.Port == 0 {
		return fmt.Errorf("server port required")
	}
	if c.Server.Port < 1024 || c.Server.Port > 65535 {
		return fmt.Errorf("server port must be between 1024 and 65535, got: %d", c.Server.Port)
	}

	// Validate Redis configuration
	if c.Redis.Addr == "" {
		return fmt.Errorf("redis address required")
	}

	// Validate logging configuration
	validLevels := map[string]bool{
		"debug": true,
		"info":  true,
		"warn":  true,
		"error": true,
	}
	if c.Logging.Level != "" && !validLevels[c.Logging.Level] {
		return fmt.Errorf("invalid log level: %s (must be debug, info, warn, or error)", c.Logging.Level)
	}

	validFormats := map[string]bool{
		"json":    true,
		"console": true,
	}
	if c.Logging.Format != "" && !validFormats[c.Logging.Format] {
		return fmt.Errorf("invalid log format: %s (must be json or console)", c.Logging.Format)
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

// GetConnectionString returns the PostgreSQL connection string
func (c *DatabaseConfig) GetConnectionString() string {
	return fmt.Sprintf("host=%s port=%d dbname=%s user=%s password=%s sslmode=%s",
		c.Host, c.Port, c.Name, c.User, c.Password, c.SSLMode)
}
