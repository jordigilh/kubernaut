package config

import (
	"fmt"
	"os"
	"time"

	"gopkg.in/yaml.v3"
)

// Config represents the complete Context API service configuration
// ADR-032: Context API uses Data Storage Service API Gateway (no direct DB access)
type Config struct {
	Server      ServerConfig      `yaml:"server"`
	Cache       CacheConfig       `yaml:"cache"`
	DataStorage DataStorageConfig `yaml:"data_storage"` // ADR-032: Data Storage Service API Gateway
	Logging     LoggingConfig     `yaml:"logging"`
}

// ServerConfig contains HTTP server configuration
type ServerConfig struct {
	Port         int           `yaml:"port"`
	Host         string        `yaml:"host"`
	ReadTimeout  time.Duration `yaml:"read_timeout"`
	WriteTimeout time.Duration `yaml:"write_timeout"`
}

// CacheConfig contains Redis and LRU cache configuration
type CacheConfig struct {
	RedisAddr  string        `yaml:"redis_addr"`
	RedisDB    int           `yaml:"redis_db"`
	LRUSize    int           `yaml:"lru_size"`
	DefaultTTL time.Duration `yaml:"default_ttl"`
}

// DataStorageConfig contains Data Storage Service configuration (ADR-032)
type DataStorageConfig struct {
	BaseURL string        `yaml:"base_url"` // Data Storage Service base URL
	Timeout time.Duration `yaml:"timeout"`  // Request timeout
}

// LoggingConfig contains logging configuration
type LoggingConfig struct {
	Level  string `yaml:"level"`
	Format string `yaml:"format"`
}

// LoadFromFile loads configuration from a YAML file
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

// Validate checks if the configuration is valid and returns an error if not
func (c *Config) Validate() error {
	// NOTE: Database validation removed per ADR-032
	// Context API uses Data Storage Service API Gateway (no direct DB access)
	// Database config fields remain for backward compatibility but are not validated

	// Validate server configuration
	if c.Server.Port == 0 {
		return fmt.Errorf("server port required")
	}
	if c.Server.Port < 1024 || c.Server.Port > 65535 {
		return fmt.Errorf("server port must be between 1024 and 65535")
	}

	// Validate cache configuration
	if c.Cache.RedisAddr == "" {
		return fmt.Errorf("redis address required")
	}
	if c.Cache.LRUSize < 0 {
		return fmt.Errorf("LRU cache size must be non-negative")
	}

	// Validate Data Storage configuration (ADR-032: Context API uses Data Storage Service API Gateway)
	if c.DataStorage.BaseURL == "" {
		return fmt.Errorf("DataStorageBaseURL is required - Context API must use Data Storage Service (ADR-032)")
	}
	if c.DataStorage.Timeout == 0 {
		return fmt.Errorf("DataStorageTimeout is required")
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

	return nil
}
