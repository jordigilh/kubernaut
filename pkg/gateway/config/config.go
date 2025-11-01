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
	"time"

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
	Redis *RedisOptions `yaml:"redis"`
}

// RedisOptions contains Redis connection configuration
// (simplified from go-redis/redis.Options for YAML unmarshaling)
type RedisOptions struct {
	Addr         string        `yaml:"addr"`
	DB           int           `yaml:"db"`
	Password     string        `yaml:"password,omitempty"`
	DialTimeout  time.Duration `yaml:"dial_timeout"`
	ReadTimeout  time.Duration `yaml:"read_timeout"`
	WriteTimeout time.Duration `yaml:"write_timeout"`
	PoolSize     int           `yaml:"pool_size"`
	MinIdleConns int           `yaml:"min_idle_conns"`
}

// ProcessingSettings contains business logic configuration.
// Single Responsibility: Signal processing behavior
type ProcessingSettings struct {
	Deduplication DeduplicationSettings `yaml:"deduplication"`
	Storm         StormSettings         `yaml:"storm"`
	Environment   EnvironmentSettings   `yaml:"environment"`
	Priority      PrioritySettings      `yaml:"priority"`
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

	return nil
}
