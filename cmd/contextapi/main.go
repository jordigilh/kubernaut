package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"os/signal"
	"syscall"
	"time"

	"go.uber.org/zap"

	"github.com/jordigilh/kubernaut/pkg/contextapi/config"
	"github.com/jordigilh/kubernaut/pkg/contextapi/server"
)

var (
	version = "v0.1.0"
)

func main() {
	// Parse command-line flags
	configPath := flag.String("config", "config/context-api.yaml", "Path to configuration file")
	showVersion := flag.Bool("version", false, "Show version and exit")
	flag.Parse()

	if *showVersion {
		fmt.Printf("Context API Service %s\n", version)
		os.Exit(0)
	}

	// Initialize logger
	logger, err := zap.NewProduction()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Failed to initialize logger: %v\n", err)
		os.Exit(1)
	}
	defer func() {
		_ = logger.Sync()
	}()

	logger.Info("Starting Context API Service",
		zap.String("version", version),
		zap.String("config_path", *configPath))

	// Load configuration
	cfg, err := config.LoadFromFile(*configPath)
	if err != nil {
		logger.Fatal("Failed to load configuration",
			zap.Error(err),
			zap.String("config_path", *configPath))
	}

	// Override with environment variables
	cfg.LoadFromEnv()

	// Validate configuration
	if err := cfg.Validate(); err != nil {
		logger.Fatal("Invalid configuration", zap.Error(err))
	}

	logger.Info("Configuration loaded",
		zap.Int("server_port", cfg.Server.Port),
		zap.String("server_host", cfg.Server.Host),
		zap.String("db_host", cfg.Database.Host),
		zap.Int("db_port", cfg.Database.Port),
		zap.String("db_name", cfg.Database.Name),
		zap.String("redis_addr", cfg.Cache.RedisAddr),
		zap.Int("redis_db", cfg.Cache.RedisDB),
		zap.String("log_level", cfg.Logging.Level))

	// Build connection strings
	connStr := fmt.Sprintf("host=%s port=%d user=%s password=%s dbname=%s sslmode=%s",
		cfg.Database.Host, cfg.Database.Port, cfg.Database.User,
		cfg.Database.Password, cfg.Database.Name, cfg.Database.SSLMode)

	redisAddr := fmt.Sprintf("%s/%d", cfg.Cache.RedisAddr, cfg.Cache.RedisDB)

	// Create server configuration
	serverCfg := &server.Config{
		Port:         cfg.Server.Port,
		ReadTimeout:  cfg.Server.ReadTimeout,
		WriteTimeout: cfg.Server.WriteTimeout,
	}

	// Create server
	srv, err := server.NewServer(connStr, redisAddr, logger, serverCfg)
	if err != nil {
		logger.Fatal("Failed to create server", zap.Error(err))
	}

	// Start server in goroutine
	errChan := make(chan error, 1)
	go func() {
		logger.Info("Server starting",
			zap.String("address", fmt.Sprintf("%s:%d", cfg.Server.Host, cfg.Server.Port)))
		if err := srv.Start(); err != nil {
			errChan <- err
		}
	}()

	// Setup signal handling for graceful shutdown
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, os.Interrupt, syscall.SIGTERM)

	// Wait for shutdown signal or server error
	select {
	case err := <-errChan:
		logger.Fatal("Server failed", zap.Error(err))
	case sig := <-sigChan:
		logger.Info("Shutdown signal received", zap.String("signal", sig.String()))
	}

	// Graceful shutdown with 30-second timeout
	shutdownCtx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	logger.Info("Initiating graceful shutdown...")
	if err := srv.Shutdown(shutdownCtx); err != nil {
		logger.Error("Graceful shutdown failed", zap.Error(err))
		os.Exit(1)
	}

	logger.Info("Server shutdown complete")
}
