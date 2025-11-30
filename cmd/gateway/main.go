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

	goredis "github.com/redis/go-redis/v9"

	rediscache "github.com/jordigilh/kubernaut/pkg/cache/redis"
	"github.com/jordigilh/kubernaut/pkg/gateway"
	"github.com/jordigilh/kubernaut/pkg/gateway/adapters"
	"github.com/jordigilh/kubernaut/pkg/gateway/config"
	kubelog "github.com/jordigilh/kubernaut/pkg/log"
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

	// DD-005: Initialize logger using shared logging library (logr.Logger interface)
	logger := kubelog.NewLogger(kubelog.Options{
		Development: false,
		Level:       0, // INFO
		ServiceName: "gateway",
	})
	defer kubelog.Sync(logger)

	logger.Info("Starting Gateway Service",
		"version", version,
		"git_commit", gitCommit,
		"build_date", buildDate,
		"config_path", *configPath,
		"listen_addr", *listenAddr,
		"redis_addr", *redisAddr)

	// Initialize Redis client (DD-CACHE-001: Shared Redis Library)
	// Uses pkg/cache/redis for connection management with graceful degradation
	redisClient := rediscache.NewClient(&goredis.Options{
		Addr:         *redisAddr,
		DB:           *redisDB,
		DialTimeout:  5 * time.Second,
		ReadTimeout:  3 * time.Second,
		WriteTimeout: 3 * time.Second,
		PoolSize:     10,
		MinIdleConns: 2,
	}, logger.WithName("redis"))

	// Test Redis connection (optional - Gateway can start without Redis)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := redisClient.EnsureConnection(ctx); err != nil {
		logger.Info("Redis unavailable at startup, will retry on first operation",
			"error", err,
			"redis_addr", *redisAddr)
	} else {
		logger.Info("Connected to Redis", "redis_addr", *redisAddr)
	}

	// Load configuration from YAML file
	serverCfg, err := config.LoadFromFile(*configPath)
	if err != nil {
		logger.Error(err, "Failed to load configuration",
			"config_path", *configPath)
		os.Exit(1)
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
		logger.Error(err, "Invalid configuration")
		os.Exit(1)
	}

	logger.Info("Configuration loaded successfully",
		"listen_addr", serverCfg.Server.ListenAddr,
		"redis_addr", serverCfg.Infrastructure.Redis.Addr,
		"redis_db", serverCfg.Infrastructure.Redis.DB)

	// Create Gateway server
	srv, err := gateway.NewServer(serverCfg, logger.WithName("server"))
	if err != nil {
		logger.Error(err, "Failed to create Gateway server")
		os.Exit(1)
	}

	// Register adapters (BR-GATEWAY-001, BR-GATEWAY-002)
	// Prometheus AlertManager webhook adapter
	prometheusAdapter := adapters.NewPrometheusAdapter()
	if err := srv.RegisterAdapter(prometheusAdapter); err != nil {
		logger.Error(err, "Failed to register Prometheus adapter")
		os.Exit(1)
	}

	// Kubernetes Event webhook adapter
	k8sEventAdapter := adapters.NewKubernetesEventAdapter()
	if err := srv.RegisterAdapter(k8sEventAdapter); err != nil {
		logger.Error(err, "Failed to register K8s Event adapter")
		os.Exit(1)
	}

	logger.Info("Registered all adapters",
		"adapter_count", 2,
		"adapters", []string{"prometheus", "kubernetes-event"})

	// Start server in goroutine
	errChan := make(chan error, 1)
	serverCtx, serverCancel := context.WithCancel(context.Background())
	defer serverCancel()

	go func() {
		logger.Info("Gateway server starting", "address", *listenAddr)
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
		logger.Error(err, "Gateway server failed")
		os.Exit(1)
	case sig := <-sigChan:
		logger.Info("Shutdown signal received", "signal", sig.String())
	}

	// Graceful shutdown with 30-second timeout
	shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer shutdownCancel()

	logger.Info("Initiating graceful shutdown...")
	if err := srv.Stop(shutdownCtx); err != nil {
		logger.Error(err, "Graceful shutdown failed")
		os.Exit(1)
	}

	// Close Redis connection
	if err := redisClient.Close(); err != nil {
		logger.Error(err, "Failed to close Redis connection")
	}

	logger.Info("Gateway server shutdown complete")
}
