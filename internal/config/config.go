package config

import (
	"fmt"
	"os"
	"strconv"
	"time"

	"gopkg.in/yaml.v3"
)

type Config struct {
	App        AppConfig        `yaml:"app"`
	Server     ServerConfig     `yaml:"server"`
	Logging    LoggingConfig    `yaml:"logging"`
	SLM        SLMConfig        `yaml:"slm"`
	Kubernetes KubernetesConfig `yaml:"kubernetes"`
	Actions    ActionsConfig    `yaml:"actions"`
	Webhook    WebhookConfig    `yaml:"webhook"`
	Database   DatabaseConfig   `yaml:"database"`
	Filters    []FilterConfig   `yaml:"filters"`
}

type AppConfig struct {
	Name    string `yaml:"name"`
	Version string `yaml:"version"`
}

type ServerConfig struct {
	WebhookPort string `yaml:"webhook_port"`
	MetricsPort string `yaml:"metrics_port"`
	HealthPort  string `yaml:"health_port"`
}

type LoggingConfig struct {
	Level  string `yaml:"level"`
	Format string `yaml:"format"`
}

type SLMConfig struct {
	Endpoint       string        `yaml:"endpoint"`
	Model          string        `yaml:"model"`
	APIKey         string        `yaml:"api_key"`
	Timeout        time.Duration `yaml:"timeout"`
	RetryCount     int           `yaml:"retry_count"`
	Provider       string        `yaml:"provider"`         // Only "localai" supported
	Temperature    float32       `yaml:"temperature"`      // Model temperature (0.0-1.0)
	MaxTokens      int           `yaml:"max_tokens"`       // Maximum tokens for response
	MaxContextSize int           `yaml:"max_context_size"` // Maximum context size in tokens (0 = unlimited)
}

type KubernetesConfig struct {
	Context        string `yaml:"context"`
	Namespace      string `yaml:"namespace"`
	ServiceAccount string `yaml:"service_account"`
}

type ActionsConfig struct {
	DryRun         bool          `yaml:"dry_run"`
	MaxConcurrent  int           `yaml:"max_concurrent"`
	CooldownPeriod time.Duration `yaml:"cooldown_period"`
}

type WebhookConfig struct {
	Port string            `yaml:"port"`
	Path string            `yaml:"path"`
	Auth WebhookAuthConfig `yaml:"auth"`
}

type WebhookAuthConfig struct {
	Type  string `yaml:"type"`
	Token string `yaml:"token"`
}

type DatabaseConfig struct {
	Enabled                bool   `yaml:"enabled"`
	Host                   string `yaml:"host"`
	Port                   string `yaml:"port"`
	Database               string `yaml:"database"`
	Username               string `yaml:"username"`
	Password               string `yaml:"password"`
	SSLMode                string `yaml:"ssl_mode"`
	MaxOpenConns           int    `yaml:"max_open_conns"`
	MaxIdleConns           int    `yaml:"max_idle_conns"`
	ConnMaxLifetimeMinutes int    `yaml:"conn_max_lifetime_minutes"`
}

type FilterConfig struct {
	Name       string              `yaml:"name"`
	Conditions map[string][]string `yaml:"conditions"`
}

func Load(configFile string) (*Config, error) {
	config := &Config{
		App: AppConfig{
			Name:    "prometheus-alerts-slm",
			Version: "1.0.0",
		},
		Server: ServerConfig{
			WebhookPort: "8080",
			MetricsPort: "9090",
			HealthPort:  "8081",
		},
		Logging: LoggingConfig{
			Level:  "info",
			Format: "json",
		},
		SLM: SLMConfig{
			Model:          "granite3.1-dense:8b",
			Provider:       "localai",
			Endpoint:       "http://localhost:11434",
			Timeout:        30 * time.Second,
			RetryCount:     3,
			Temperature:    0.3,
			MaxTokens:      500,
			MaxContextSize: 2000, // Optimal context size for decision quality
		},
		Kubernetes: KubernetesConfig{
			Namespace:      "default",
			ServiceAccount: "prometheus-alerts-slm",
		},
		Actions: ActionsConfig{
			DryRun:         false,
			MaxConcurrent:  5,
			CooldownPeriod: 5 * time.Minute,
		},
		Webhook: WebhookConfig{
			Port: "8080",
			Path: "/alerts",
			Auth: WebhookAuthConfig{
				Type: "bearer",
			},
		},
		Database: DatabaseConfig{
			Enabled:                false, // Disabled by default
			Host:                   "localhost",
			Port:                   "5432",
			Database:               "action_history",
			Username:               "slm_user",
			Password:               "slm_password",
			SSLMode:                "disable",
			MaxOpenConns:           10,
			MaxIdleConns:           5,
			ConnMaxLifetimeMinutes: 5,
		},
	}

	// Load from file if specified
	if configFile != "" {
		data, err := os.ReadFile(configFile)
		if err != nil {
			return nil, fmt.Errorf("failed to read config file %s: %w", configFile, err)
		}

		if err := yaml.Unmarshal(data, config); err != nil {
			return nil, fmt.Errorf("failed to parse config file %s: %w", configFile, err)
		}
	}

	// Override with environment variables
	if err := loadFromEnv(config); err != nil {
		return nil, fmt.Errorf("failed to load environment variables: %w", err)
	}

	// Validate configuration
	if err := validate(config); err != nil {
		return nil, fmt.Errorf("invalid configuration: %w", err)
	}

	return config, nil
}

func loadFromEnv(config *Config) error {
	// SLM Configuration
	if endpoint := os.Getenv("SLM_ENDPOINT"); endpoint != "" {
		config.SLM.Endpoint = endpoint
	}
	if apiKey := os.Getenv("SLM_API_KEY"); apiKey != "" {
		config.SLM.APIKey = apiKey
	}
	if model := os.Getenv("SLM_MODEL"); model != "" {
		config.SLM.Model = model
	}
	if provider := os.Getenv("SLM_PROVIDER"); provider != "" {
		config.SLM.Provider = provider
	}
	// Mock functionality removed - only LocalAI supported
	if temp := os.Getenv("SLM_TEMPERATURE"); temp != "" {
		if val, err := strconv.ParseFloat(temp, 32); err == nil {
			config.SLM.Temperature = float32(val)
		}
	}
	if maxTokens := os.Getenv("SLM_MAX_TOKENS"); maxTokens != "" {
		if val, err := strconv.Atoi(maxTokens); err == nil {
			config.SLM.MaxTokens = val
		}
	}
	if maxContextSize := os.Getenv("SLM_MAX_CONTEXT_SIZE"); maxContextSize != "" {
		if val, err := strconv.Atoi(maxContextSize); err == nil {
			config.SLM.MaxContextSize = val
		}
	}

	// Kubernetes Configuration
	if context := os.Getenv("KUBE_CONTEXT"); context != "" {
		config.Kubernetes.Context = context
	}
	if namespace := os.Getenv("KUBE_NAMESPACE"); namespace != "" {
		config.Kubernetes.Namespace = namespace
	}

	// Application Configuration
	if port := os.Getenv("WEBHOOK_PORT"); port != "" {
		config.Server.WebhookPort = port
		config.Webhook.Port = port
	}
	if port := os.Getenv("METRICS_PORT"); port != "" {
		config.Server.MetricsPort = port
	}
	if level := os.Getenv("LOG_LEVEL"); level != "" {
		config.Logging.Level = level
	}
	if token := os.Getenv("WEBHOOK_AUTH_TOKEN"); token != "" {
		config.Webhook.Auth.Token = token
	}

	// Actions Configuration
	if dryRun := os.Getenv("DRY_RUN"); dryRun == "true" {
		config.Actions.DryRun = true
	}
	if maxConcurrent := os.Getenv("MAX_CONCURRENT_ACTIONS"); maxConcurrent != "" {
		if val, err := strconv.Atoi(maxConcurrent); err == nil {
			config.Actions.MaxConcurrent = val
		}
	}

	return nil
}

func validate(config *Config) error {
	// Only LocalAI provider supported
	if config.SLM.Provider != "localai" {
		return fmt.Errorf("unsupported SLM provider: %s (only 'localai' supported)", config.SLM.Provider)
	}

	if config.SLM.Endpoint == "" {
		config.SLM.Endpoint = "http://localhost:8080" // LocalAI default
	}
	if config.SLM.Model == "" {
		return fmt.Errorf("SLM model is required for LocalAI provider")
	}

	// Validate temperature range
	if config.SLM.Temperature < 0.0 || config.SLM.Temperature > 1.0 {
		return fmt.Errorf("SLM temperature must be between 0.0 and 1.0")
	}

	// Validate max tokens
	if config.SLM.MaxTokens <= 0 {
		return fmt.Errorf("SLM max tokens must be greater than 0")
	}

	if config.Kubernetes.Namespace == "" {
		return fmt.Errorf("Kubernetes namespace is required")
	}

	if config.Actions.MaxConcurrent <= 0 {
		return fmt.Errorf("max concurrent actions must be greater than 0")
	}

	return nil
}
