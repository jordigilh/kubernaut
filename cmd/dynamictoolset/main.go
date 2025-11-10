package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"os/signal"
	"strconv"
	"syscall"
	"time"

	"go.uber.org/zap"

	"github.com/jordigilh/kubernaut/pkg/k8sutil"
	"github.com/jordigilh/kubernaut/pkg/toolset/config"
	"github.com/jordigilh/kubernaut/pkg/toolset/server"
)

var (
	version = "v0.1.0"
)

func main() {
	// Check for CONFIG_FILE environment variable first (for containerized deployments)
	defaultConfigPath := os.Getenv("CONFIG_FILE")
	if defaultConfigPath == "" {
		defaultConfigPath = "config/dynamic-toolset-config.yaml"
	}

	// Parse command-line flags
	configPath := flag.String("config", defaultConfigPath, "Path to configuration file")
	showVersion := flag.Bool("version", false, "Show version and exit")
	flag.Parse()

	if *showVersion {
		fmt.Printf("Dynamic Toolset Service %s\n", version)
		os.Exit(0)
	}

	// Initialize logger (JSON format by default, per DD-005)
	logger, err := zap.NewProduction()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Failed to initialize logger: %v\n", err)
		os.Exit(1)
	}
	defer func() {
		_ = logger.Sync()
	}()

	logger.Info("Starting Dynamic Toolset Service",
		zap.String("version", version),
		zap.String("config_path", *configPath))

	// Load configuration from file (ADR-030)
	cfg, err := config.LoadFromFile(*configPath)
	if err != nil {
		logger.Fatal("Failed to load configuration",
			zap.Error(err),
			zap.String("config_path", *configPath))
	}

	// Validate configuration
	if err := cfg.Validate(); err != nil {
		logger.Fatal("Invalid configuration", zap.Error(err))
	}

	logger.Info("Configuration loaded",
		zap.Duration("discovery_interval", cfg.ServiceDiscovery.DiscoveryInterval),
		zap.Duration("health_check_interval", cfg.ServiceDiscovery.HealthCheckInterval),
		zap.Int("namespace_count", len(cfg.ServiceDiscovery.Namespaces)))

	// Server configuration from environment variables (deployment-specific)
	port := getEnvInt("SERVER_PORT", 8080)
	metricsPort := getEnvInt("METRICS_PORT", 9090)
	shutdownTimeout := getEnvDuration("SHUTDOWN_TIMEOUT", 30*time.Second)

	// Create context that cancels on SIGINT/SIGTERM
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Setup signal handling
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)

	// Create Kubernetes client using standard helper (DD-013)
	clientset, err := k8sutil.NewClientset()
	if err != nil {
		logger.Fatal("Failed to create Kubernetes client", zap.Error(err))
	}

	// Verify client is working by checking API server version
	serverVersion, err := clientset.Discovery().ServerVersion()
	if err != nil {
		logger.Fatal("Failed to connect to Kubernetes API server", zap.Error(err))
	}

	logger.Info("Kubernetes client initialized",
		zap.String("server_version", serverVersion.String()))

	// Get current namespace (for ConfigMap creation)
	// In Kubernetes, the namespace is available at /var/run/secrets/kubernetes.io/serviceaccount/namespace
	// For local development, fallback to "kubernaut-system"
	namespace := getNamespace()
	logger.Info("Running in namespace", zap.String("namespace", namespace))

	// Create HTTP server with all components
	// BR-TOOLSET-036: Main application integration
	serverConfig := &server.Config{
		Port:              port,
		MetricsPort:       metricsPort,
		ShutdownTimeout:   shutdownTimeout,
		DiscoveryInterval: cfg.ServiceDiscovery.DiscoveryInterval,
		Namespace:         namespace,
	}

	srv, err := server.NewServer(serverConfig, clientset)
	if err != nil {
		logger.Fatal("Failed to create HTTP server", zap.Error(err))
	}

	logger.Info("HTTP server created",
		zap.Int("port", serverConfig.Port),
		zap.Int("metrics_port", serverConfig.MetricsPort),
		zap.Duration("discovery_interval", serverConfig.DiscoveryInterval))

	// Start HTTP server in background
	go func() {
		logger.Info("Starting HTTP server", zap.Int("port", serverConfig.Port))
		if err := srv.Start(ctx); err != nil && err != context.Canceled {
			logger.Error("HTTP server error", zap.Error(err))
			cancel()
		}
	}()

	// Wait for shutdown signal
	select {
	case sig := <-sigCh:
		logger.Info("Received shutdown signal", zap.String("signal", sig.String()))
		cancel()
	case <-ctx.Done():
		logger.Info("Context canceled")
	}

	// Graceful shutdown
	logger.Info("Shutting down Dynamic Toolset Service")

	shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), serverConfig.ShutdownTimeout)
	defer shutdownCancel()

	if err := srv.Shutdown(shutdownCtx); err != nil {
		logger.Error("Error during shutdown", zap.Error(err))
	}

	logger.Info("Dynamic Toolset Service stopped")
}

// getEnvInt reads an integer from environment variable with a default fallback
func getEnvInt(key string, defaultValue int) int {
	if val := os.Getenv(key); val != "" {
		if parsed, err := strconv.Atoi(val); err == nil {
			return parsed
		}
	}
	return defaultValue
}

// getEnvDuration reads a duration from environment variable with a default fallback
func getEnvDuration(key string, defaultValue time.Duration) time.Duration {
	if val := os.Getenv(key); val != "" {
		if parsed, err := time.ParseDuration(val); err == nil {
			return parsed
		}
	}
	return defaultValue
}

// getNamespace returns the current namespace where the pod is running
// In Kubernetes, this is available at /var/run/secrets/kubernetes.io/serviceaccount/namespace
// For local development, fallback to "kubernaut-system"
func getNamespace() string {
	namespaceFile := "/var/run/secrets/kubernetes.io/serviceaccount/namespace"
	if data, err := os.ReadFile(namespaceFile); err == nil {
		return string(data)
	}
	// Fallback for local development
	if ns := os.Getenv("POD_NAMESPACE"); ns != "" {
		return ns
	}
	return "kubernaut-system"
}
