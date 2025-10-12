package main

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"syscall"
	"time"

	"go.uber.org/zap"

	"github.com/jordigilh/kubernaut/internal/toolset/k8s"
	"github.com/jordigilh/kubernaut/pkg/toolset/server"
)

func main() {
	// Initialize logger
	logger, err := zap.NewProduction()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Failed to initialize logger: %v\n", err)
		os.Exit(1)
	}
	defer func() {
		_ = logger.Sync()
	}()

	logger.Info("Starting Dynamic Toolset Service")

	// Create context that cancels on SIGINT/SIGTERM
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Setup signal handling
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)

	// Create Kubernetes client
	k8sConfig := k8s.Config{
		InCluster: true, // Production: use in-cluster config
	}

	client, err := k8s.NewClient(k8sConfig, logger)
	if err != nil {
		logger.Fatal("Failed to create Kubernetes client", zap.Error(err))
	}

	// Verify client is working by checking API server version
	serverVersion, err := client.Discovery().ServerVersion()
	if err != nil {
		logger.Fatal("Failed to connect to Kubernetes API server", zap.Error(err))
	}

	logger.Info("Kubernetes client initialized",
		zap.String("server_version", serverVersion.String()))

	// Create HTTP server with all components
	// BR-TOOLSET-036: Main application integration
	serverConfig := &server.Config{
		Port:              8080,
		MetricsPort:       9090,
		ShutdownTimeout:   30 * time.Second,
		DiscoveryInterval: 5 * time.Minute,
	}

	srv, err := server.NewServer(serverConfig, client)
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
