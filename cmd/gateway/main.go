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

package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"os/signal"
	"syscall"
	"time"

	goredis "github.com/go-redis/redis/v8"
	"go.uber.org/zap"

	"github.com/jordigilh/kubernaut/pkg/gateway"
	"github.com/jordigilh/kubernaut/pkg/gateway/adapters"
	"github.com/jordigilh/kubernaut/pkg/gateway/config"
)

var (
	// version is the semantic version, set at build time
	version = "v0.1.0"
	// gitCommit is the git commit hash, set at build time via -ldflags
	gitCommit = "unknown"
	// buildDate is the build timestamp, set at build time via -ldflags
	buildDate = "unknown"
)

func main() {
	// Parse command-line flags
	configPath := flag.String("config", "config/gateway.yaml", "Path to configuration file")
	showVersion := flag.Bool("version", false, "Show version and exit")
	listenAddr := flag.String("listen", ":8080", "HTTP server listen address")
	redisAddr := flag.String("redis", "localhost:6379", "Redis server address")
	redisDB := flag.Int("redis-db", 0, "Redis database number")
	flag.Parse()

	if *showVersion {
		fmt.Printf("Gateway Service %s-%s (built: %s)\n", version, gitCommit, buildDate)
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

	logger.Info("Starting Gateway Service",
		zap.String("version", version),
		zap.String("git_commit", gitCommit),
		zap.String("build_date", buildDate),
		zap.String("config_path", *configPath),
		zap.String("listen_addr", *listenAddr),
		zap.String("redis_addr", *redisAddr))

	// Initialize Redis client
	redisClient := goredis.NewClient(&goredis.Options{
		Addr:         *redisAddr,
		DB:           *redisDB,
		DialTimeout:  5 * time.Second,
		ReadTimeout:  3 * time.Second,
		WriteTimeout: 3 * time.Second,
		PoolSize:     10,
		MinIdleConns: 2,
	})

	// Test Redis connection
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := redisClient.Ping(ctx).Err(); err != nil {
		logger.Fatal("Failed to connect to Redis",
			zap.Error(err),
			zap.String("redis_addr", *redisAddr))
	}
	logger.Info("Connected to Redis", zap.String("redis_addr", *redisAddr))

	// Load configuration from YAML file
	serverCfg, err := config.LoadFromFile(*configPath)
	if err != nil {
		logger.Fatal("Failed to load configuration",
			zap.String("config_path", *configPath),
			zap.Error(err))
	}

	// Override configuration with environment variables (e.g., secrets)
	serverCfg.LoadFromEnv()

	// Override with command-line flags (highest priority)
	if *listenAddr != ":8080" {
		serverCfg.Server.ListenAddr = *listenAddr
	}
	if *redisAddr != "localhost:6379" {
		serverCfg.Infrastructure.Redis.Addr = *redisAddr
	}
	if *redisDB != 0 {
		serverCfg.Infrastructure.Redis.DB = *redisDB
	}

	// Validate configuration
	if err := serverCfg.Validate(); err != nil {
		logger.Fatal("Invalid configuration", zap.Error(err))
	}

	logger.Info("Configuration loaded successfully",
		zap.String("listen_addr", serverCfg.Server.ListenAddr),
		zap.String("redis_addr", serverCfg.Infrastructure.Redis.Addr),
		zap.Int("redis_db", serverCfg.Infrastructure.Redis.DB))

	// Create Gateway server
	srv, err := gateway.NewServer(serverCfg, logger)
	if err != nil {
		logger.Fatal("Failed to create Gateway server", zap.Error(err))
	}

	// Register adapters (BR-GATEWAY-001, BR-GATEWAY-002)
	// Prometheus AlertManager webhook adapter
	prometheusAdapter := adapters.NewPrometheusAdapter()
	if err := srv.RegisterAdapter(prometheusAdapter); err != nil {
		logger.Fatal("Failed to register Prometheus adapter", zap.Error(err))
	}

	// Kubernetes Event webhook adapter
	k8sEventAdapter := adapters.NewKubernetesEventAdapter()
	if err := srv.RegisterAdapter(k8sEventAdapter); err != nil {
		logger.Fatal("Failed to register K8s Event adapter", zap.Error(err))
	}

	logger.Info("Registered all adapters",
		zap.Int("adapter_count", 2),
		zap.Strings("adapters", []string{"prometheus", "kubernetes-event"}))

	// Start server in goroutine
	errChan := make(chan error, 1)
	serverCtx, serverCancel := context.WithCancel(context.Background())
	defer serverCancel()

	go func() {
		logger.Info("Gateway server starting", zap.String("address", *listenAddr))
		if err := srv.Start(serverCtx); err != nil {
			errChan <- err
		}
	}()

	// Setup signal handling for graceful shutdown
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, os.Interrupt, syscall.SIGTERM)

	// Wait for shutdown signal or server error
	select {
	case err := <-errChan:
		logger.Fatal("Gateway server failed", zap.Error(err))
	case sig := <-sigChan:
		logger.Info("Shutdown signal received", zap.String("signal", sig.String()))
	}

	// Graceful shutdown with 30-second timeout
	shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer shutdownCancel()

	logger.Info("Initiating graceful shutdown...")
	if err := srv.Stop(shutdownCtx); err != nil {
		logger.Error("Graceful shutdown failed", zap.Error(err))
		os.Exit(1)
	}

	// Close Redis connection
	if err := redisClient.Close(); err != nil {
		logger.Error("Failed to close Redis connection", zap.Error(err))
	}

	logger.Info("Gateway server shutdown complete")
}
