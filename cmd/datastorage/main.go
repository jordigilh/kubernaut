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
	kubelog "github.com/jordigilh/kubernaut/pkg/log"
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
	// DD-005 v2.0: Use pkg/log shared library with logr interface
	logger := kubelog.NewLogger(kubelog.Options{
		Development: os.Getenv("ENV") != "production",
		Level:       0, // Info level
		ServiceName: "datastorage",
	})
	defer kubelog.Sync(logger)

	// ADR-030: Load configuration from YAML file (ConfigMap)
	// CONFIG_PATH environment variable is MANDATORY
	// Deployment/environment is responsible for setting this
	cfgPath := os.Getenv("CONFIG_PATH")
	if cfgPath == "" {
		logger.Error(fmt.Errorf("CONFIG_PATH not set"), "CONFIG_PATH environment variable required (ADR-030)",
			"env_var", "CONFIG_PATH",
			"reason", "Service must not guess config file location - deployment controls this",
			"example_local", "export CONFIG_PATH=config/data-storage.yaml",
			"example_k8s", "Set in Deployment manifest",
		)
		os.Exit(1)
	}

	logger.Info("Loading configuration from YAML file (ADR-030)",
		"config_path", cfgPath,
	)

	cfg, err := config.LoadFromFile(cfgPath)
	if err != nil {
		logger.Error(err, "Failed to load configuration file (ADR-030)",
			"config_path", cfgPath,
		)
		os.Exit(1)
	}

	// ADR-030 Section 6: Load secrets from mounted files
	logger.Info("Loading secrets from mounted files (ADR-030 Section 6)")
	if err := cfg.LoadSecrets(); err != nil {
		logger.Error(err, "Failed to load secrets (ADR-030 Section 6)")
		os.Exit(1)
	}

	// Validate configuration (after secrets are loaded)
	if err := cfg.Validate(); err != nil {
		logger.Error(err, "Invalid configuration (ADR-030)")
		os.Exit(1)
	}

	logger.Info("Configuration loaded successfully (ADR-030)",
		"service", "data-storage",
		"port", cfg.Server.Port,
		"database_host", cfg.Database.Host,
		"database_port", cfg.Database.Port,
		"redis_addr", cfg.Redis.Addr,
		"log_level", cfg.Logging.Level,
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

	// Gap 3.3: Pass DLQ max length for capacity monitoring
	dlqMaxLen := int64(cfg.Redis.DLQMaxLen)

	// SOC2 Gap #9: PostgreSQL with custom hash chains for tamper detection
	// Retry logic for PostgreSQL/Redis connection (E2E timing issue fix)
	// In Kind clusters, PostgreSQL may not be ready when DataStorage starts
	var srv *server.Server
	maxRetries := 10
	retryDelay := 2 * time.Second

	logger.Info("Connecting to PostgreSQL and Redis (with retry logic)...",
		"max_retries", maxRetries,
		"retry_delay", retryDelay)

	for attempt := 1; attempt <= maxRetries; attempt++ {
		var err error
		srv, err = server.NewServer(dbConnStr, cfg.Redis.Addr, cfg.Redis.Password, logger, cfg, serverCfg, dlqMaxLen)
		if err == nil {
			logger.Info("Successfully connected to PostgreSQL and Redis",
				"attempt", attempt)
			break
		}

		if attempt == maxRetries {
			logger.Error(err, "Failed to create server after all retries",
				"attempts", maxRetries)
			os.Exit(1)
		}

		logger.Info("Failed to connect, retrying...",
			"attempt", attempt,
			"max_retries", maxRetries,
			"error", err.Error(),
			"next_retry_in", retryDelay)
		time.Sleep(retryDelay)
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
		"port", cfg.Server.Port,
		"host", cfg.Server.Host,
		"shutdown_timeout", shutdownTimeout,
	)

	// Start server in goroutine
	serverErrors := make(chan error, 1)
	go func() {
		addr := fmt.Sprintf("%s:%d", cfg.Server.Host, cfg.Server.Port)
		logger.Info("HTTP server listening",
			"addr", addr,
		)
		serverErrors <- srv.Start()
	}()

	// Wait for shutdown signal or server error
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)

	select {
	case err := <-serverErrors:
		logger.Error(err, "Server error")
	case sig := <-sigChan:
		logger.Info("Shutdown signal received (DD-007)",
			"signal", sig.String(),
		)

		// DD-007: Graceful shutdown (already implemented in server.Shutdown)
		// 4-step pattern: flag set → endpoint propagation → drain → close resources
		shutdownCtx, shutdownCancel := context.WithTimeout(ctx, shutdownTimeout)
		defer shutdownCancel()

		if err := srv.Shutdown(shutdownCtx); err != nil {
			logger.Error(err, "Graceful shutdown failed (DD-007)")
		}
	}

	logger.Info("Data Storage service stopped (ADR-030 + DD-007)")
}
