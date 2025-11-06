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
	"fmt"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/jordigilh/kubernaut/pkg/datastorage/config"
	"github.com/jordigilh/kubernaut/pkg/datastorage/server"
	"go.uber.org/zap"
)

// ========================================
// DATA STORAGE SERVICE - MAIN ENTRY POINT
// 📋 Implementation Plan: Day 11 - ADR-030 + DD-007
// Authority: config/data-storage.yaml (source of truth)
// Pattern: Context API main.go (authoritative reference)
// ========================================
//
// ADR-030 Configuration Management:
// 1. Load from YAML file (ConfigMap in Kubernetes)
// 2. Override with environment variables (secrets only)
// 3. Validate configuration before startup
//
// DD-007 Graceful Shutdown:
// 4-step Kubernetes-aware shutdown pattern
// ========================================

func main() {
	// Initialize logger first (before config loading for error reporting)
	logger, err := zap.NewProduction()
	if err != nil {
		panic("failed to create logger: " + err.Error())
	}
	defer func() {
		_ = logger.Sync() // Ignore sync errors on shutdown
	}()

	// ADR-030: Load configuration from YAML file (ConfigMap)
	// CONFIG_PATH environment variable is MANDATORY
	// Deployment/environment is responsible for setting this
	cfgPath := os.Getenv("CONFIG_PATH")
	if cfgPath == "" {
		logger.Fatal("CONFIG_PATH environment variable required (ADR-030)",
			zap.String("env_var", "CONFIG_PATH"),
			zap.String("reason", "Service must not guess config file location - deployment controls this"),
			zap.String("example_local", "export CONFIG_PATH=config/data-storage.yaml"),
			zap.String("example_k8s", "Set in Deployment manifest"),
		)
	}

	logger.Info("Loading configuration from YAML file (ADR-030)",
		zap.String("config_path", cfgPath),
	)

	cfg, err := config.LoadFromFile(cfgPath)
	if err != nil {
		logger.Fatal("Failed to load configuration file (ADR-030)",
			zap.Error(err),
			zap.String("config_path", cfgPath),
		)
	}

	// ADR-030 Section 6: Load secrets from mounted files
	logger.Info("Loading secrets from mounted files (ADR-030 Section 6)")
	if err := cfg.LoadSecrets(); err != nil {
		logger.Fatal("Failed to load secrets (ADR-030 Section 6)",
			zap.Error(err),
		)
	}

	// Validate configuration (after secrets are loaded)
	if err := cfg.Validate(); err != nil {
		logger.Fatal("Invalid configuration (ADR-030)",
			zap.Error(err),
		)
	}

	logger.Info("Configuration loaded successfully (ADR-030)",
		zap.String("service", "data-storage"),
		zap.Int("port", cfg.Server.Port),
		zap.String("database_host", cfg.Database.Host),
		zap.Int("database_port", cfg.Database.Port),
		zap.String("redis_addr", cfg.Redis.Addr),
		zap.String("log_level", cfg.Logging.Level),
	)

	// Context management for graceful shutdown
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Build PostgreSQL connection string from config
	dbConnStr := cfg.Database.GetConnectionString()

	// Create HTTP server with database connection + Redis for DLQ
	serverCfg := &server.Config{
		Port:         cfg.Server.Port,
		ReadTimeout:  cfg.Server.GetReadTimeout(),
		WriteTimeout: cfg.Server.GetWriteTimeout(),
	}

	srv, err := server.NewServer(dbConnStr, cfg.Redis.Addr, cfg.Redis.Password, logger, serverCfg)
	if err != nil {
		logger.Fatal("Failed to create server",
			zap.Error(err),
		)
	}

	// DD-007: Graceful shutdown timeout (Kubernetes terminationGracePeriodSeconds)
	// Default: 30 seconds to allow endpoint removal + connection drain
	shutdownTimeout := 30 * time.Second
	if timeoutEnv := os.Getenv("SHUTDOWN_TIMEOUT"); timeoutEnv != "" {
		if timeout, err := time.ParseDuration(timeoutEnv); err == nil {
			shutdownTimeout = timeout
		}
	}

	logger.Info("Starting Data Storage service (ADR-030 + DD-007)",
		zap.Int("port", cfg.Server.Port),
		zap.String("host", cfg.Server.Host),
		zap.Duration("shutdown_timeout", shutdownTimeout),
	)

	// Start server in goroutine
	serverErrors := make(chan error, 1)
	go func() {
		addr := fmt.Sprintf("%s:%d", cfg.Server.Host, cfg.Server.Port)
		logger.Info("HTTP server listening",
			zap.String("addr", addr),
		)
		serverErrors <- srv.Start()
	}()

	// Wait for shutdown signal or server error
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)

	select {
	case err := <-serverErrors:
		logger.Error("Server error",
			zap.Error(err),
		)
	case sig := <-sigChan:
		logger.Info("Shutdown signal received (DD-007)",
			zap.String("signal", sig.String()),
		)

		// DD-007: Graceful shutdown (already implemented in server.Shutdown)
		// 4-step pattern: flag set → endpoint propagation → drain → close resources
		shutdownCtx, shutdownCancel := context.WithTimeout(ctx, shutdownTimeout)
		defer shutdownCancel()

		if err := srv.Shutdown(shutdownCtx); err != nil {
			logger.Error("Graceful shutdown failed (DD-007)",
				zap.Error(err),
			)
		}
	}

	logger.Info("Data Storage service stopped (ADR-030 + DD-007)")
}
