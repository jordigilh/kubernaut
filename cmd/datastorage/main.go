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
	"strconv"
	"strings"
	"syscall"
	"time"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/jordigilh/kubernaut/pkg/datastorage/config"
	"github.com/jordigilh/kubernaut/pkg/datastorage/server"
	dsvalidation "github.com/jordigilh/kubernaut/pkg/datastorage/validation"
	kubelog "github.com/jordigilh/kubernaut/pkg/log"
	"github.com/jordigilh/kubernaut/pkg/shared/auth"
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

	// DD-AUTH-014: Allow PORT environment variable to override config (SME Option D)
	// This enables --network=host mode in integration tests where container must listen on external port
	// Reference: docs/handoff/DD_AUTH_014_ENVTEST_IPV6_BLOCKER.md (Option D)
	if portEnv := os.Getenv("PORT"); portEnv != "" {
		if port, err := strconv.Atoi(portEnv); err == nil {
			cfg.Server.Port = port
			logger.Info("Port overridden by PORT environment variable (DD-AUTH-014)",
				"port", port,
				"reason", "integration test host networking",
			)
		} else {
			logger.Error(err, "Invalid PORT environment variable, using config value",
				"port_env", portEnv,
				"config_port", cfg.Server.Port,
			)
		}
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

	// DD-AUTH-014: Create Kubernetes client for authentication and authorization
	// Priority:
	//   1. KUBECONFIG env var (integration tests with envtest)
	//   2. In-cluster config (production with ServiceAccount)
	logger.Info("Creating Kubernetes client for auth middleware (DD-AUTH-014)")
	var k8sConfig *rest.Config
	if kubeconfigPath := os.Getenv("KUBECONFIG"); kubeconfigPath != "" {
		logger.Info("Using KUBECONFIG from environment (DD-AUTH-014)",
			"kubeconfig_path", kubeconfigPath,
			"use_case", "integration tests with envtest",
		)
		k8sConfig, err = clientcmd.BuildConfigFromFlags("", kubeconfigPath)
		if err != nil {
			logger.Error(err, "Failed to load kubeconfig from KUBECONFIG env var (DD-AUTH-014)",
				"kubeconfig_path", kubeconfigPath,
			)
			os.Exit(1)
		}
	} else {
		logger.Info("Using in-cluster config (DD-AUTH-014)",
			"service_account_path", "/var/run/secrets/kubernetes.io/serviceaccount/",
			"use_case", "production deployment",
		)
		k8sConfig, err = rest.InClusterConfig()
		if err != nil {
			logger.Error(err, "Failed to load in-cluster Kubernetes config (DD-AUTH-014)",
				"error", err.Error(),
				"note", "Ensure ServiceAccount is mounted and has TokenReview/SAR permissions",
			)
			os.Exit(1)
		}
	}

	// CRITICAL: Configure K8s client for high concurrency (DD-AUTH-014)
	// Default rest.Config has:
	//   - No timeout (causing indefinite hangs)
	//   - QPS=5, Burst=10 (causing rate limiter queue saturation under load)
	// E2E tests run 11 parallel pytest workers, each calling TokenReview/SAR
	// Pattern learned from Gateway integration tests (test/integration/gateway/suite_test.go:230-231)
	// Without these settings, "client rate limiter Wait returned an error: context canceled" occurs
	k8sConfig.Timeout = 30 * time.Second // 30s timeout for K8s API operations
	k8sConfig.QPS = 1000.0               // 1000 queries per second (matches Gateway tests)
	k8sConfig.Burst = 2000               // 2000 burst capacity (matches Gateway tests)
	
	k8sClient, err := kubernetes.NewForConfig(k8sConfig)
	if err != nil {
		logger.Error(err, "Failed to create Kubernetes client (DD-AUTH-014)")
		os.Exit(1)
	}

	// DD-AUTH-014: Create authenticator and authorizer with real Kubernetes APIs
	authenticator := auth.NewK8sAuthenticator(k8sClient)
	authorizer := auth.NewK8sAuthorizer(k8sClient)

	// Determine namespace for auth operations
	// Priority:
	//   1. POD_NAMESPACE env var (integration tests)
	//   2. ServiceAccount mount (production)
	//   3. Default to "default" as fallback
	var authNamespace string
	if envNs := os.Getenv("POD_NAMESPACE"); envNs != "" {
		authNamespace = envNs
		logger.Info("Using namespace from POD_NAMESPACE env var (DD-AUTH-014)",
			"namespace", authNamespace,
			"use_case", "integration tests",
		)
	} else if podNamespace, err := os.ReadFile("/var/run/secrets/kubernetes.io/serviceaccount/namespace"); err == nil {
		authNamespace = strings.TrimSpace(string(podNamespace))
		logger.Info("Using namespace from ServiceAccount mount (DD-AUTH-014)",
			"namespace", authNamespace,
			"use_case", "production deployment",
		)
	} else {
		authNamespace = "default"
		logger.Info("Using default namespace (DD-AUTH-014)",
			"namespace", authNamespace,
			"reason", "ServiceAccount mount not found, POD_NAMESPACE not set",
		)
	}

	logger.Info("Kubernetes authenticator and authorizer created (DD-AUTH-014)",
		"type", "K8sAuthenticator + K8sAuthorizer",
		"api_server", k8sConfig.Host,
		"auth_namespace", authNamespace,
	)

	// DD-WE-006: Create controller-runtime client for dependency validation.
	// Reuses the existing rest.Config (k8sConfig) that was already created for auth.
	crScheme := runtime.NewScheme()
	if err := corev1.AddToScheme(crScheme); err != nil {
		logger.Error(err, "Failed to add core/v1 to scheme for dependency validator")
		os.Exit(1)
	}
	crClient, err := client.New(k8sConfig, client.Options{Scheme: crScheme})
	if err != nil {
		logger.Error(err, "Failed to create controller-runtime client for dependency validation (DD-WE-006)")
		os.Exit(1)
	}
	depValidator := dsvalidation.NewK8sDependencyValidator(crClient)
	executionNamespace := "kubernaut-workflows"
	logger.Info("Dependency validator initialized (DD-WE-006)",
		"executionNamespace", executionNamespace,
	)

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
		srv, err = server.NewServer(server.ServerDeps{
			DBConnStr:     dbConnStr,
			RedisAddr:     cfg.Redis.Addr,
			RedisPassword: cfg.Redis.Password,
			Logger:        logger,
			AppConfig:     cfg,
			ServerConfig:  serverCfg,
			DLQMaxLen:     dlqMaxLen,
			Authenticator: authenticator,
			Authorizer:    authorizer,
			AuthNamespace: authNamespace,
			HandlerOpts: []server.HandlerOption{
				server.WithDependencyValidator(depValidator, executionNamespace),
			},
		})
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
