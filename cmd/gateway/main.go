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
)

var (
	version = "v0.1.0"
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
		fmt.Printf("Gateway Service %s\n", version)
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

	// Create server configuration (nested structure)
	serverCfg := &gateway.ServerConfig{
		Server: gateway.ServerSettings{
			ListenAddr:   *listenAddr,
			ReadTimeout:  30 * time.Second,
			WriteTimeout: 30 * time.Second,
			IdleTimeout:  120 * time.Second,
		},

		Middleware: gateway.MiddlewareSettings{
			RateLimit: gateway.RateLimitSettings{
				RequestsPerMinute: 100,
				Burst:             10,
			},
		},

		Infrastructure: gateway.InfrastructureSettings{
			Redis: &goredis.Options{
				Addr:         *redisAddr,
				DB:           *redisDB,
				DialTimeout:  5 * time.Second,
				ReadTimeout:  3 * time.Second,
				WriteTimeout: 3 * time.Second,
				PoolSize:     10,
				MinIdleConns: 2,
			},
		},

		Processing: gateway.ProcessingSettings{
			Deduplication: gateway.DeduplicationSettings{
				TTL: 5 * time.Minute,
			},
			Storm: gateway.StormSettings{
				RateThreshold:     10, // 10 alerts/minute
				PatternThreshold:  5,  // 5 similar alerts
				AggregationWindow: 1 * time.Minute,
			},
			Environment: gateway.EnvironmentSettings{
				CacheTTL:           30 * time.Second,
				ConfigMapNamespace: "kubernaut-system",
				ConfigMapName:      "kubernaut-environment-overrides",
			},
		},
	}

	// Create Gateway server
	srv, err := gateway.NewServer(serverCfg, logger)
	if err != nil {
		logger.Fatal("Failed to create Gateway server", zap.Error(err))
	}

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
