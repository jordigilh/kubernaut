package main

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/sirupsen/logrus"
	"gopkg.in/yaml.v3"

	"github.com/jordigilh/kubernaut/internal/gateway/redis"
	"github.com/jordigilh/kubernaut/pkg/gateway"
	"github.com/jordigilh/kubernaut/pkg/gateway/adapters"
)

func main() {
	// Initialize logger
	logger := logrus.New()
	logger.SetFormatter(&logrus.JSONFormatter{})
	logger.SetOutput(os.Stdout)

	// Set log level from environment (default: info)
	logLevel := os.Getenv("LOG_LEVEL")
	if logLevel == "" {
		logLevel = "info"
	}
	level, err := logrus.ParseLevel(logLevel)
	if err != nil {
		logger.WithField("log_level", logLevel).Warn("Invalid log level, using info")
		level = logrus.InfoLevel
	}
	logger.SetLevel(level)

	logger.Info("Starting Gateway Service")

	// Load configuration
	config, err := loadConfig()
	if err != nil {
		logger.WithError(err).Fatal("Failed to load configuration")
	}

	logger.WithFields(logrus.Fields{
		"listen_addr":             config.ListenAddr,
		"rate_limit":              config.RateLimitRequestsPerMinute,
		"redis_addr":              config.Redis.Addr,
		"deduplication_ttl":       config.DeduplicationTTL,
		"storm_rate_threshold":    config.StormRateThreshold,
		"storm_pattern_threshold": config.StormPatternThreshold,
		"env_configmap":           fmt.Sprintf("%s/%s", config.EnvConfigMapNamespace, config.EnvConfigMapName),
	}).Info("Configuration loaded")

	// Create Gateway server
	server, err := gateway.NewServer(config, logger)
	if err != nil {
		logger.WithError(err).Fatal("Failed to create Gateway server")
	}

	logger.Info("Gateway server created successfully")

	// Register signal adapters
	// BR-GATEWAY-001: Prometheus AlertManager webhook support
	prometheusAdapter := adapters.NewPrometheusAdapter()
	if err := server.RegisterAdapter(prometheusAdapter); err != nil {
		logger.WithError(err).Fatal("Failed to register Prometheus adapter")
	}
	logger.WithField("adapter", "prometheus").Info("Adapter registered")

	// BR-GATEWAY-002: Kubernetes Event integration (optional)
	// Uncomment when needed:
	// kubernetesAdapter := adapters.NewKubernetesEventAdapter()
	// if err := server.RegisterAdapter(kubernetesAdapter); err != nil {
	// 	logger.WithError(err).Fatal("Failed to register Kubernetes Event adapter")
	// }
	// logger.WithField("adapter", "kubernetes-events").Info("Adapter registered")

	// Create context for graceful shutdown
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Setup signal handling for graceful shutdown
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)

	// Start Gateway server in background
	go func() {
		logger.WithField("addr", config.ListenAddr).Info("Starting HTTP server")
		if err := server.Start(ctx); err != nil && err.Error() != "http: Server closed" {
			logger.WithError(err).Error("HTTP server error")
			cancel()
		}
	}()

	// Wait for server to be ready (simple health check)
	time.Sleep(500 * time.Millisecond)
	logger.Info("Gateway service is ready to receive alerts")

	// Wait for shutdown signal
	select {
	case sig := <-sigCh:
		logger.WithField("signal", sig.String()).Info("Received shutdown signal")
		cancel()
	case <-ctx.Done():
		logger.Info("Context canceled")
	}

	// Graceful shutdown
	logger.Info("Shutting down Gateway service")

	shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer shutdownCancel()

	if err := server.Stop(shutdownCtx); err != nil {
		logger.WithError(err).Error("Error during shutdown")
		os.Exit(1)
	}

	logger.Info("Gateway service stopped successfully")
}

// loadConfig loads configuration from environment variables and config file
//
// Configuration priority (highest to lowest):
// 1. Environment variables
// 2. Config file (gateway-config.yaml)
// 3. Default values
//
// Environment variables:
// - GATEWAY_LISTEN_ADDR (default: ":8080")
// - GATEWAY_REDIS_ADDR (default: "localhost:6379")
// - GATEWAY_REDIS_PASSWORD (default: "")
// - GATEWAY_REDIS_DB (default: 0)
// - GATEWAY_RATE_LIMIT (default: 100)
// - GATEWAY_RATE_LIMIT_BURST (default: 20)
// - GATEWAY_DEDUP_TTL_SECONDS (default: 300 = 5 minutes)
// - GATEWAY_STORM_RATE_THRESHOLD (default: 10)
// - GATEWAY_STORM_PATTERN_THRESHOLD (default: 5)
// - GATEWAY_STORM_WINDOW_SECONDS (default: 60)
// - GATEWAY_ENV_CACHE_TTL_SECONDS (default: 30)
// - GATEWAY_ENV_CONFIGMAP_NAMESPACE (default: "kubernaut-system")
// - GATEWAY_ENV_CONFIGMAP_NAME (default: "kubernaut-environment-overrides")
func loadConfig() (*gateway.ServerConfig, error) {
	config := &gateway.ServerConfig{
		// Default values
		ListenAddr:   getEnv("GATEWAY_LISTEN_ADDR", ":8080"),
		ReadTimeout:  30 * time.Second,
		WriteTimeout: 30 * time.Second,
		IdleTimeout:  120 * time.Second,

		RateLimitRequestsPerMinute: getEnvInt("GATEWAY_RATE_LIMIT", 100),
		RateLimitBurst:             getEnvInt("GATEWAY_RATE_LIMIT_BURST", 20),

		Redis: &redis.Config{
			Addr:     getEnv("GATEWAY_REDIS_ADDR", "localhost:6379"),
			Password: getEnv("GATEWAY_REDIS_PASSWORD", ""),
			DB:       getEnvInt("GATEWAY_REDIS_DB", 0),
			PoolSize: 100, // High pool size for production
		},

		DeduplicationTTL:       time.Duration(getEnvInt("GATEWAY_DEDUP_TTL_SECONDS", 300)) * time.Second,
		StormRateThreshold:     getEnvInt("GATEWAY_STORM_RATE_THRESHOLD", 10),
		StormPatternThreshold:  getEnvInt("GATEWAY_STORM_PATTERN_THRESHOLD", 5),
		StormAggregationWindow: time.Duration(getEnvInt("GATEWAY_STORM_WINDOW_SECONDS", 60)) * time.Second,
		EnvironmentCacheTTL:    time.Duration(getEnvInt("GATEWAY_ENV_CACHE_TTL_SECONDS", 30)) * time.Second,

		EnvConfigMapNamespace: getEnv("GATEWAY_ENV_CONFIGMAP_NAMESPACE", "kubernaut-system"),
		EnvConfigMapName:      getEnv("GATEWAY_ENV_CONFIGMAP_NAME", "kubernaut-environment-overrides"),
	}

	// Optionally load from YAML config file
	configFile := getEnv("GATEWAY_CONFIG_FILE", "")
	if configFile != "" {
		if err := loadConfigFile(configFile, config); err != nil {
			return nil, fmt.Errorf("failed to load config file: %w", err)
		}
	}

	return config, nil
}

// loadConfigFile loads configuration from a YAML file
func loadConfigFile(path string, config *gateway.ServerConfig) error {
	data, err := os.ReadFile(path)
	if err != nil {
		return fmt.Errorf("failed to read config file: %w", err)
	}

	if err := yaml.Unmarshal(data, config); err != nil {
		return fmt.Errorf("failed to parse config file: %w", err)
	}

	return nil
}

// getEnv returns environment variable value or default
func getEnv(key, defaultValue string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return defaultValue
}

// getEnvInt returns environment variable as int or default
func getEnvInt(key string, defaultValue int) int {
	if value := os.Getenv(key); value != "" {
		var intValue int
		if _, err := fmt.Sscanf(value, "%d", &intValue); err == nil {
			return intValue
		}
	}
	return defaultValue
}
