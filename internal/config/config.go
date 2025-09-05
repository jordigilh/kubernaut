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
	SLM        LLMConfig        `yaml:"slm"`
	Kubernetes KubernetesConfig `yaml:"kubernetes"`
	Actions    ActionsConfig    `yaml:"actions"`
	Webhook    WebhookConfig    `yaml:"webhook"`
	Database   DatabaseConfig   `yaml:"database"`
	VectorDB   VectorDBConfig   `yaml:"vectordb"`
	Monitoring MonitoringConfig `yaml:"monitoring"`
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

type LLMConfig struct {
	Endpoint       string        `yaml:"endpoint"`
	Model          string        `yaml:"model"`
	APIKey         string        `yaml:"api_key"`
	Timeout        time.Duration `yaml:"timeout"`
	RetryCount     int           `yaml:"retry_count"`
	Provider       string        `yaml:"provider"`         // Supports "localai", "ramalama", "ollama"
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

type VectorDBConfig struct {
	Enabled          bool                   `yaml:"enabled"`
	Backend          string                 `yaml:"backend"`           // "postgresql", "pinecone", "weaviate", "memory"
	EmbeddingService EmbeddingConfig        `yaml:"embedding_service"` // Embedding service configuration
	PostgreSQL       PostgreSQLVectorConfig `yaml:"postgresql"`        // PostgreSQL-specific config (when backend=postgresql)
	Pinecone         PineconeConfig         `yaml:"pinecone"`          // Pinecone-specific config (when backend=pinecone)
	Weaviate         WeaviateConfig         `yaml:"weaviate"`          // Weaviate-specific config (when backend=weaviate)
	Cache            VectorCacheConfig      `yaml:"cache"`             // Caching configuration
}

type EmbeddingConfig struct {
	Service   string `yaml:"service"`   // "local", "openai", "huggingface", "hybrid"
	Dimension int    `yaml:"dimension"` // Embedding dimension (default: 384)
	Model     string `yaml:"model"`     // Model name (e.g., "all-MiniLM-L6-v2", "text-embedding-ada-002")
	APIKey    string `yaml:"api_key"`   // API key for external services
	Endpoint  string `yaml:"endpoint"`  // Custom endpoint for self-hosted services
}

type PostgreSQLVectorConfig struct {
	// Uses same connection as main database by default
	// Can override with separate connection if needed
	UseMainDB  bool   `yaml:"use_main_db"` // Use main database connection
	Host       string `yaml:"host"`        // Override host
	Port       string `yaml:"port"`        // Override port
	Database   string `yaml:"database"`    // Override database
	Username   string `yaml:"username"`    // Override username
	Password   string `yaml:"password"`    // Override password
	IndexLists int    `yaml:"index_lists"` // IVFFlat index lists parameter (default: 100)
}

type PineconeConfig struct {
	APIKey      string `yaml:"api_key"`
	Environment string `yaml:"environment"`
	IndexName   string `yaml:"index_name"`
	Namespace   string `yaml:"namespace"`
}

type WeaviateConfig struct {
	Host   string `yaml:"host"`
	APIKey string `yaml:"api_key"`
	Class  string `yaml:"class"`  // Weaviate class name for patterns
	Scheme string `yaml:"scheme"` // http or https
}

type VectorCacheConfig struct {
	Enabled   bool          `yaml:"enabled"`
	TTL       time.Duration `yaml:"ttl"`        // Time to live for cached embeddings
	MaxSize   int           `yaml:"max_size"`   // Maximum cache entries
	CacheType string        `yaml:"cache_type"` // "memory", "redis" (future)
}

type MonitoringConfig struct {
	UseProductionClients bool                `yaml:"use_production_clients"`
	AlertManager         AlertManagerConfig  `yaml:"alertmanager"`
	Prometheus           PrometheusConfig    `yaml:"prometheus"`
	Effectiveness        EffectivenessConfig `yaml:"effectiveness"`
}

type AlertManagerConfig struct {
	Enabled  bool          `yaml:"enabled"`
	Endpoint string        `yaml:"endpoint"`
	Timeout  time.Duration `yaml:"timeout"`
}

type PrometheusConfig struct {
	Enabled  bool          `yaml:"enabled"`
	Endpoint string        `yaml:"endpoint"`
	Timeout  time.Duration `yaml:"timeout"`
}

type EffectivenessConfig struct {
	Enabled            bool          `yaml:"enabled"`
	AssessmentDelay    time.Duration `yaml:"assessment_delay"`
	ProcessingInterval time.Duration `yaml:"processing_interval"`

	// Phase 2 Enhanced Assessment Settings
	EnableEnhancedAssessment  bool    `yaml:"enable_enhanced_assessment"`
	EnablePatternLearning     bool    `yaml:"enable_pattern_learning"`
	EnablePredictiveAnalytics bool    `yaml:"enable_predictive_analytics"`
	EnableCostAnalysis        bool    `yaml:"enable_cost_analysis"`
	MinSimilarityThreshold    float64 `yaml:"min_similarity_threshold"`
	PredictionModel           string  `yaml:"prediction_model"`
	AsyncProcessing           bool    `yaml:"async_processing"`
	BatchSize                 int     `yaml:"batch_size"`

	// Vector Database Settings
	VectorDB VectorDBConfig `yaml:"vector_db"`
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
		SLM: LLMConfig{
			Model:          "granite3.1-dense:8b",
			Provider:       "ollama",
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
		Monitoring: MonitoringConfig{
			UseProductionClients: false, // Use stub clients by default
			AlertManager: AlertManagerConfig{
				Enabled:  false,
				Endpoint: "http://localhost:9093",
				Timeout:  30 * time.Second,
			},
			Prometheus: PrometheusConfig{
				Enabled:  false,
				Endpoint: "http://localhost:9090",
				Timeout:  30 * time.Second,
			},
			Effectiveness: EffectivenessConfig{
				Enabled:            true,
				AssessmentDelay:    10 * time.Minute,
				ProcessingInterval: 2 * time.Minute,

				// Phase 2 Enhanced Assessment Defaults (disabled by default)
				EnableEnhancedAssessment:  false,
				EnablePatternLearning:     false,
				EnablePredictiveAnalytics: false,
				EnableCostAnalysis:        false,
				MinSimilarityThreshold:    0.3,
				PredictionModel:           "similarity",
				AsyncProcessing:           true,
				BatchSize:                 10,

				// Vector Database Defaults
				VectorDB: VectorDBConfig{
					Enabled: false,
					Backend: "memory",
					EmbeddingService: EmbeddingConfig{
						Service:   "local",
						Dimension: 384,
						Model:     "all-MiniLM-L6-v2",
					},
					PostgreSQL: PostgreSQLVectorConfig{
						UseMainDB:  true,
						IndexLists: 100,
					},
					Cache: VectorCacheConfig{
						Enabled:   false,
						MaxSize:   1000,
						CacheType: "memory",
					},
				},
			},
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
	// Validate supported providers
	supportedProviders := []string{"localai", "ramalama", "ollama"}
	validProvider := false
	for _, provider := range supportedProviders {
		if config.SLM.Provider == provider {
			validProvider = true
			break
		}
	}
	if !validProvider {
		return fmt.Errorf("unsupported SLM provider: %s, supported: %v", config.SLM.Provider, supportedProviders)
	}

	if config.SLM.Endpoint == "" {
		config.SLM.Endpoint = "http://localhost:11434" // Default endpoint
	}
	if config.SLM.Model == "" {
		return fmt.Errorf("SLM model is required")
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
