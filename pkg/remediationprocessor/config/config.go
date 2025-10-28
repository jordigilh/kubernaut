// Package config provides configuration management for remediationprocessor controller.
package config

import (
	"fmt"
	"os"
	"strconv"

	"gopkg.in/yaml.v3"
)

// Config represents the controller configuration.
type Config struct {
	// Common controller configuration
	Namespace      string `yaml:"namespace"`
	MetricsAddress string `yaml:"metrics_address"`
	HealthAddress  string `yaml:"health_address"`
	LeaderElection bool   `yaml:"leader_election"`
	LogLevel       string `yaml:"log_level"`
	MaxConcurrency int    `yaml:"max_concurrency"`

	// Kubernetes API configuration
	Kubernetes KubernetesConfig `yaml:"kubernetes"`

	// RemediationProcessor-specific configuration
	DataStorage    DataStorageConfig    `yaml:"data_storage"`
	Context        ContextAPIConfig     `yaml:"context"`
	Classification ClassificationConfig `yaml:"classification"`
}

// KubernetesConfig holds Kubernetes API client configuration.
type KubernetesConfig struct {
	QPS   float32 `yaml:"qps"`
	Burst int     `yaml:"burst"`
}

// DataStorageConfig holds PostgreSQL configuration for remediation history.
type DataStorageConfig struct {
	PostgresHost     string `yaml:"postgres_host"`
	PostgresPort     int    `yaml:"postgres_port"`
	PostgresUser     string `yaml:"postgres_user"`
	PostgresPassword string `yaml:"postgres_password"`
	PostgresDatabase string `yaml:"postgres_database"`
	SSLMode          string `yaml:"ssl_mode"`
	MaxConnections   int    `yaml:"max_connections"`
	MaxIdleConns     int    `yaml:"max_idle_conns"`
}

// ContextAPIConfig holds Context API client configuration for enrichment.
type ContextAPIConfig struct {
	Endpoint       string `yaml:"endpoint"`
	Timeout        int    `yaml:"timeout"`
	MaxRetries     int    `yaml:"max_retries"`
	RetryBackoffMs int    `yaml:"retry_backoff_ms"`
}

// ClassificationConfig holds semantic analysis and deduplication configuration.
type ClassificationConfig struct {
	SemanticThreshold float64 `yaml:"semantic_threshold"`
	TimeWindowMinutes int     `yaml:"time_window_minutes"`
	SimilarityEngine  string  `yaml:"similarity_engine"`
	BatchSize         int     `yaml:"batch_size"`
}

// LoadConfig loads configuration from a YAML file.
func LoadConfig(path string) (*Config, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("failed to read config file: %w", err)
	}

	var cfg Config
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return nil, fmt.Errorf("failed to parse config file: %w", err)
	}

	// Set defaults
	cfg.setDefaults()

	return &cfg, nil
}

// setDefaults sets default values for unspecified configuration.
func (c *Config) setDefaults() {
	if c.Namespace == "" {
		c.Namespace = "kubernaut-system"
	}
	if c.MetricsAddress == "" {
		c.MetricsAddress = ":8080"
	}
	if c.HealthAddress == "" {
		c.HealthAddress = ":8081"
	}
	if c.LogLevel == "" {
		c.LogLevel = "info"
	}
	if c.MaxConcurrency == 0 {
		c.MaxConcurrency = 10
	}
	if c.Kubernetes.QPS == 0 {
		c.Kubernetes.QPS = 20.0
	}
	if c.Kubernetes.Burst == 0 {
		c.Kubernetes.Burst = 30
	}

	// DataStorage defaults
	if c.DataStorage.PostgresPort == 0 {
		c.DataStorage.PostgresPort = 5432
	}
	if c.DataStorage.SSLMode == "" {
		c.DataStorage.SSLMode = "require"
	}
	if c.DataStorage.MaxConnections == 0 {
		c.DataStorage.MaxConnections = 25
	}
	if c.DataStorage.MaxIdleConns == 0 {
		c.DataStorage.MaxIdleConns = 5
	}

	// Context API defaults
	if c.Context.Timeout == 0 {
		c.Context.Timeout = 30
	}
	if c.Context.MaxRetries == 0 {
		c.Context.MaxRetries = 3
	}
	if c.Context.RetryBackoffMs == 0 {
		c.Context.RetryBackoffMs = 100
	}

	// Classification defaults
	if c.Classification.SemanticThreshold == 0 {
		c.Classification.SemanticThreshold = 0.85
	}
	if c.Classification.TimeWindowMinutes == 0 {
		c.Classification.TimeWindowMinutes = 60
	}
	if c.Classification.SimilarityEngine == "" {
		c.Classification.SimilarityEngine = "cosine"
	}
	if c.Classification.BatchSize == 0 {
		c.Classification.BatchSize = 100
	}
}

// Validate validates the configuration.
func (c *Config) Validate() error {
	if c.Namespace == "" {
		return fmt.Errorf("namespace is required")
	}
	if c.MetricsAddress == "" {
		return fmt.Errorf("metrics_address is required")
	}
	if c.HealthAddress == "" {
		return fmt.Errorf("health_address is required")
	}
	if c.LogLevel == "" {
		return fmt.Errorf("log_level is required")
	}
	if c.MaxConcurrency <= 0 {
		return fmt.Errorf("max_concurrency must be greater than 0")
	}

	// Validate Kubernetes config
	if c.Kubernetes.QPS <= 0 {
		return fmt.Errorf("kubernetes.qps must be greater than 0")
	}
	if c.Kubernetes.Burst <= 0 {
		return fmt.Errorf("kubernetes.burst must be greater than 0")
	}

	// Validate DataStorage config
	if c.DataStorage.PostgresHost == "" {
		return fmt.Errorf("data_storage.postgres_host is required")
	}
	if c.DataStorage.PostgresPort <= 0 {
		return fmt.Errorf("data_storage.postgres_port must be greater than 0")
	}
	if c.DataStorage.PostgresUser == "" {
		return fmt.Errorf("data_storage.postgres_user is required")
	}
	if c.DataStorage.PostgresDatabase == "" {
		return fmt.Errorf("data_storage.postgres_database is required")
	}
	if c.DataStorage.MaxConnections <= 0 {
		return fmt.Errorf("data_storage.max_connections must be greater than 0")
	}

	// Validate Context API config
	if c.Context.Endpoint == "" {
		return fmt.Errorf("context.endpoint is required")
	}
	if c.Context.Timeout <= 0 {
		return fmt.Errorf("context.timeout must be greater than 0")
	}
	if c.Context.MaxRetries < 0 {
		return fmt.Errorf("context.max_retries must be non-negative")
	}

	// Validate Classification config
	if c.Classification.SemanticThreshold <= 0 || c.Classification.SemanticThreshold > 1 {
		return fmt.Errorf("classification.semantic_threshold must be between 0 and 1")
	}
	if c.Classification.TimeWindowMinutes <= 0 {
		return fmt.Errorf("classification.time_window_minutes must be greater than 0")
	}
	if c.Classification.BatchSize <= 0 {
		return fmt.Errorf("classification.batch_size must be greater than 0")
	}

	return nil
}

// LoadFromEnv loads environment variable overrides.
func (c *Config) LoadFromEnv() error {
	// Common environment variables
	if ns := os.Getenv("CONTROLLER_NAMESPACE"); ns != "" {
		c.Namespace = ns
	}
	if addr := os.Getenv("METRICS_ADDRESS"); addr != "" {
		c.MetricsAddress = addr
	}
	if addr := os.Getenv("HEALTH_ADDRESS"); addr != "" {
		c.HealthAddress = addr
	}
	if level := os.Getenv("LOG_LEVEL"); level != "" {
		c.LogLevel = level
	}
	if concurrency := os.Getenv("MAX_CONCURRENCY"); concurrency != "" {
		val, err := strconv.Atoi(concurrency)
		if err != nil {
			return fmt.Errorf("invalid MAX_CONCURRENCY: %w", err)
		}
		c.MaxConcurrency = val
	}

	// Kubernetes API overrides
	if qps := os.Getenv("KUBERNETES_QPS"); qps != "" {
		val, err := strconv.ParseFloat(qps, 32)
		if err != nil {
			return fmt.Errorf("invalid KUBERNETES_QPS: %w", err)
		}
		c.Kubernetes.QPS = float32(val)
	}
	if burst := os.Getenv("KUBERNETES_BURST"); burst != "" {
		val, err := strconv.Atoi(burst)
		if err != nil {
			return fmt.Errorf("invalid KUBERNETES_BURST: %w", err)
		}
		c.Kubernetes.Burst = val
	}

	// DataStorage overrides
	if host := os.Getenv("POSTGRES_HOST"); host != "" {
		c.DataStorage.PostgresHost = host
	}
	if port := os.Getenv("POSTGRES_PORT"); port != "" {
		val, err := strconv.Atoi(port)
		if err != nil {
			return fmt.Errorf("invalid POSTGRES_PORT: %w", err)
		}
		c.DataStorage.PostgresPort = val
	}
	if user := os.Getenv("POSTGRES_USER"); user != "" {
		c.DataStorage.PostgresUser = user
	}
	if password := os.Getenv("POSTGRES_PASSWORD"); password != "" {
		c.DataStorage.PostgresPassword = password
	}
	if database := os.Getenv("POSTGRES_DATABASE"); database != "" {
		c.DataStorage.PostgresDatabase = database
	}
	if sslMode := os.Getenv("POSTGRES_SSL_MODE"); sslMode != "" {
		c.DataStorage.SSLMode = sslMode
	}

	// Context API overrides
	if endpoint := os.Getenv("CONTEXT_API_ENDPOINT"); endpoint != "" {
		c.Context.Endpoint = endpoint
	}
	if timeout := os.Getenv("CONTEXT_API_TIMEOUT"); timeout != "" {
		val, err := strconv.Atoi(timeout)
		if err != nil {
			return fmt.Errorf("invalid CONTEXT_API_TIMEOUT: %w", err)
		}
		c.Context.Timeout = val
	}

	// Classification overrides
	if threshold := os.Getenv("SEMANTIC_THRESHOLD"); threshold != "" {
		val, err := strconv.ParseFloat(threshold, 64)
		if err != nil {
			return fmt.Errorf("invalid SEMANTIC_THRESHOLD: %w", err)
		}
		c.Classification.SemanticThreshold = val
	}
	if window := os.Getenv("TIME_WINDOW_MINUTES"); window != "" {
		val, err := strconv.Atoi(window)
		if err != nil {
			return fmt.Errorf("invalid TIME_WINDOW_MINUTES: %w", err)
		}
		c.Classification.TimeWindowMinutes = val
	}

	return nil
}






