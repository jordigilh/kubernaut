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
	"net/http"
	"os"
	"os/signal"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/go-logr/logr"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	zaplog "go.uber.org/zap"

	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"

	"gopkg.in/yaml.v3"

	internalconfig "github.com/jordigilh/kubernaut/internal/config"
	"github.com/jordigilh/kubernaut/internal/version"
	"github.com/jordigilh/kubernaut/pkg/datastorage/config"
	"github.com/jordigilh/kubernaut/pkg/datastorage/server"
	kubelog "github.com/jordigilh/kubernaut/pkg/log"
	"github.com/jordigilh/kubernaut/pkg/shared/auth"
	"github.com/jordigilh/kubernaut/pkg/shared/health"
	"github.com/jordigilh/kubernaut/pkg/shared/hotreload"
	sharedtls "github.com/jordigilh/kubernaut/pkg/shared/tls"
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
	// gocritic:exitAfterDefer — run() returns an exit code instead of calling
	// os.Exit directly so deferred cleanup (kubelog.Sync, cancel, stopHotReload)
	// always runs.
	os.Exit(run())
}

func run() int {
	// Bootstrap logger at INFO for config loading
	bootstrapLevel := internalconfig.DefaultLoggingConfig().NewAtomicLevel()
	logger := kubelog.NewLoggerWithAtomicLevel(kubelog.Options{
		ServiceName: "datastorage",
	}, bootstrapLevel)
	defer kubelog.Sync(logger)

	logger.Info("Starting DataStorage Service",
		"version", version.Version,
		"gitCommit", version.GitCommit,
		"buildDate", version.BuildDate,
	)

	// ADR-030: Load configuration from YAML file (ConfigMap), load secrets,
	// validate, and apply PORT/HEALTH_PORT env var overrides (DD-AUTH-014, #753).
	cfgPath := os.Getenv("CONFIG_PATH")
	if cfgPath == "" {
		logger.Error(fmt.Errorf("CONFIG_PATH not set"), "CONFIG_PATH environment variable required (ADR-030)",
			"env_var", "CONFIG_PATH",
			"reason", "Service must not guess config file location - deployment controls this",
			"example_local", "export CONFIG_PATH=config/data-storage.yaml",
			"example_k8s", "Set in Deployment manifest",
		)
		return 1
	}
	cfg, err := loadDataStorageConfig(cfgPath, logger)
	if err != nil {
		// loadDataStorageConfig already logs the specific failure (file load,
		// secrets, or validation) before returning.
		return 1
	}

	// Issue #875: Apply config-driven log level using shared LoggingConfig mapping
	dsLogging := internalconfig.LoggingConfig{Level: strings.ToUpper(cfg.Logging.Level)}
	atomicLevel := dsLogging.NewAtomicLevel()
	logger = kubelog.NewLoggerWithAtomicLevel(kubelog.Options{
		ServiceName: "datastorage",
	}, atomicLevel)

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

	// DD-AUTH-014: Create Kubernetes client + authenticator/authorizer for auth middleware.
	authDeps, err := buildK8sAuthDeps(logger)
	if err != nil {
		logger.Error(err, "Failed to build Kubernetes auth dependencies (DD-AUTH-014)")
		return 1
	}

	// Create HTTP server with database connection + Redis for DLQ (SOC2 Gap #9),
	// retrying while PostgreSQL/Redis become ready (E2E timing issue fix).
	srv, err := buildServerWithRetry(serverParams{
		cfg:       cfg,
		dbConnStr: dbConnStr,
		authDeps:  authDeps,
	}, logger)
	if err != nil {
		logger.Error(err, "Failed to create server after all retries")
		return 1
	}

	// DD-007: Graceful shutdown timeout — read from config file (ADR-030).
	// Default 60s accommodates internal budgets:
	//   endpoint propagation (5s) + HTTP drain (30s) + DLQ drain (10s) + resource close (~1s)
	//   + health/metrics shutdown (2s each) = ~50s, with 10s headroom.
	// GetShutdownTimeout clamps to [30s, 120s] for safety.
	// terminationGracePeriodSeconds in deployment.yaml must be >= this value + buffer (90s).
	shutdownTimeout := logEffectiveServerConfig(cfg, logger)

	// Issue #283/#753: Dedicated Prometheus metrics + health probe servers
	// on standardised ports (CONFIG_STANDARDS.md).
	obs := startObservabilityServers(cfg, srv, logger)

	// Issue #748/#756/#875: TLS security profile + CA-cert and log-level hot-reload watchers.
	stopHotReload := wireHotReload(ctx, cfg, cfgPath, atomicLevel, logger)
	defer stopHotReload()

	runAndWaitForShutdown(ctx, cfg, srv, obs, shutdownTimeout, logger)

	logger.Info("Data Storage service stopped (ADR-030 + DD-007)")
	return 0
}

// logEffectiveServerConfig logs clamp warnings for shutdownTimeout/maxBodySize,
// the CORS wildcard warning, and the final startup summary, returning the
// effective (clamped) shutdown timeout for use by the caller. Extracted from
// main() (GO-ANTIPATTERN-AUDIT-2026-07-01 Wave 3) — pure code motion, no
// behavior change.
func logEffectiveServerConfig(cfg *config.Config, logger logr.Logger) time.Duration {
	// DD-007: Graceful shutdown timeout — read from config file (ADR-030).
	// Default 60s accommodates internal budgets:
	//   endpoint propagation (5s) + HTTP drain (30s) + DLQ drain (10s) + resource close (~1s)
	//   + health/metrics shutdown (2s each) = ~50s, with 10s headroom.
	// GetShutdownTimeout clamps to [30s, 120s] for safety.
	// terminationGracePeriodSeconds in deployment.yaml must be >= this value + buffer (90s).
	shutdownTimeout := cfg.Server.GetShutdownTimeout()
	if cfg.Server.ShutdownTimeout != "" && cfg.Server.ShutdownTimeout != shutdownTimeout.String() {
		logger.Info("Configured shutdownTimeout was clamped to safe range",
			"severity", "warning",
			"configured", cfg.Server.ShutdownTimeout,
			"effective", shutdownTimeout,
			"range", "30s–120s")
	}

	// #1048 Phase 4 / SRE-A1: Log when maxBodySize is clamped (matching shutdownTimeout pattern).
	effectiveMaxBody := cfg.Server.GetMaxBodySize()
	if cfg.Server.MaxBodySize != "" && fmt.Sprintf("%d", effectiveMaxBody) != cfg.Server.MaxBodySize {
		logger.Info("Configured maxBodySize was clamped to safe range",
			"severity", "warning",
			"configured", cfg.Server.MaxBodySize,
			"effective_bytes", effectiveMaxBody,
			"range", "1048576–52428800 (1–50 MiB)")
	}

	// #1048 Phase 4 / SRE-P2: Log effective CORS origins and max body size at startup.
	corsOrigins := cfg.Server.GetCORSAllowedOrigins()
	for _, o := range corsOrigins {
		if o == "*" {
			logger.Info("CORS allows all origins — not recommended for production",
				"severity", "warning",
				"cors_allowed_origins", corsOrigins)
			break
		}
	}

	logger.Info("Starting Data Storage service (ADR-030 + DD-007)",
		"port", cfg.Server.Port,
		"metricsPort", cfg.Server.MetricsPort,
		"healthPort", cfg.Server.HealthPort,
		"host", cfg.Server.Host,
		"shutdown_timeout", shutdownTimeout,
		"recommended_k8s_termination_grace_period", shutdownTimeout+30*time.Second,
		"max_body_size", effectiveMaxBody,
		"cors_allowed_origins", corsOrigins,
	)

	return shutdownTimeout
}

// runAndWaitForShutdown starts the API server in a background goroutine, then
// blocks until either a server/observability error or an OS shutdown signal
// is received, running the DD-007 graceful shutdown sequence in response.
// Extracted from main() (GO-ANTIPATTERN-AUDIT-2026-07-01 Wave 3) — pure code
// motion, no behavior change.
func runAndWaitForShutdown(ctx context.Context, cfg *config.Config, srv *server.Server, obs *observabilityServers, shutdownTimeout time.Duration, logger logr.Logger) {
	serverErrors := make(chan error, 1)
	go func() {
		addr := fmt.Sprintf("%s:%d", cfg.Server.Host, cfg.Server.Port)
		logger.Info("HTTP server listening",
			"addr", addr,
		)
		serverErrors <- srv.Start()
	}()

	// #1048 Phase 3: Shutdown order — API first, then health/metrics.
	// API server shutdown (srv.Shutdown) handles DD-007 internally:
	//   set readiness flag → wait for propagation → drain HTTP → drain DLQ → close DB.
	// Health and metrics servers stay alive during the entire API drain window
	// so liveness probes succeed and K8s does not force-kill the pod.
	const (
		healthShutdownTimeout  = 2 * time.Second
		metricsShutdownTimeout = 2 * time.Second
	)
	gracefulShutdown := func(shutdownCtx context.Context) {
		if err := srv.Shutdown(shutdownCtx); err != nil {
			logger.Error(err, "Graceful shutdown failed (DD-007)")
		}

		healthCtx, healthCancel := context.WithTimeout(context.Background(), healthShutdownTimeout)
		defer healthCancel()
		if err := obs.health.Shutdown(healthCtx); err != nil {
			logger.Error(err, "Health server shutdown failed")
		}

		metricsCtx, metricsCancel := context.WithTimeout(context.Background(), metricsShutdownTimeout)
		defer metricsCancel()
		if err := obs.metrics.Shutdown(metricsCtx); err != nil {
			logger.Error(err, "Metrics server shutdown failed")
		}
	}

	// Wait for shutdown signal or server error
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)

	select {
	case err := <-serverErrors:
		logger.Error(err, "Server error")
		shutdownCtx, shutdownCancel := context.WithTimeout(ctx, shutdownTimeout)
		defer shutdownCancel()
		gracefulShutdown(shutdownCtx)
	case err := <-obs.metricsErrors:
		logger.Error(err, "Metrics server error")
		shutdownCtx, shutdownCancel := context.WithTimeout(ctx, shutdownTimeout)
		defer shutdownCancel()
		gracefulShutdown(shutdownCtx)
	case err := <-obs.healthErrors:
		logger.Error(err, "Health server error")
		shutdownCtx, shutdownCancel := context.WithTimeout(ctx, shutdownTimeout)
		defer shutdownCancel()
		gracefulShutdown(shutdownCtx)
	case sig := <-sigChan:
		logger.Info("Shutdown signal received (DD-007)",
			"signal", sig.String(),
		)

		shutdownCtx, shutdownCancel := context.WithTimeout(ctx, shutdownTimeout)
		defer shutdownCancel()
		gracefulShutdown(shutdownCtx)
	}
}

// loadDataStorageConfig loads the YAML config file (ADR-030), loads secrets
// from mounted files (ADR-030 Section 6), validates the result, and applies
// the DD-AUTH-014 PORT and Issue #753 HEALTH_PORT environment variable
// overrides used by host-network integration tests. Extracted from main()
// (GO-ANTIPATTERN-AUDIT-2026-07-01 Wave 0a) — pure code motion, no behavior
// change.
func loadDataStorageConfig(cfgPath string, logger logr.Logger) (*config.Config, error) {
	logger.Info("Loading configuration from YAML file (ADR-030)",
		"config_path", cfgPath,
	)

	cfg, err := config.LoadFromFile(cfgPath)
	if err != nil {
		logger.Error(err, "Failed to load configuration file (ADR-030)",
			"config_path", cfgPath,
		)
		return nil, fmt.Errorf("failed to load configuration file (config_path=%s): %w", cfgPath, err)
	}

	// ADR-030 Section 6: Load secrets from mounted files
	logger.Info("Loading secrets from mounted files (ADR-030 Section 6)")
	if err := cfg.LoadSecrets(); err != nil {
		logger.Error(err, "Failed to load secrets (ADR-030 Section 6)")
		return nil, fmt.Errorf("failed to load secrets (ADR-030 Section 6): %w", err)
	}

	// Validate configuration (after secrets are loaded)
	if err := cfg.Validate(); err != nil {
		logger.Error(err, "Invalid configuration (ADR-030)")
		return nil, fmt.Errorf("invalid configuration (ADR-030): %w", err)
	}

	// DD-AUTH-014: Allow PORT environment variable to override config (SME Option D)
	// This enables --network=host mode in integration tests where container must listen on external port
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

	// Issue #753: Allow HEALTH_PORT override for host-network integration tests
	if healthPortEnv := os.Getenv("HEALTH_PORT"); healthPortEnv != "" {
		if port, err := strconv.Atoi(healthPortEnv); err == nil {
			cfg.Server.HealthPort = port
			logger.Info("Health port overridden by HEALTH_PORT environment variable",
				"healthPort", port,
			)
		} else {
			logger.Error(err, "Invalid HEALTH_PORT environment variable, using config value",
				"health_port_env", healthPortEnv,
				"config_health_port", cfg.Server.HealthPort,
			)
		}
	}

	return cfg, nil
}

// k8sAuthDeps groups the Kubernetes rest.Config and DD-AUTH-014
// authenticator/authorizer/namespace built together in buildK8sAuthDeps
// (Options-pattern result struct, AGENTS.md's 8+-param rule).
type k8sAuthDeps struct {
	k8sConfig     *rest.Config
	authenticator *auth.K8sAuthenticator
	authorizer    *auth.K8sAuthorizer
	authNamespace string
}

// buildK8sAuthDeps discovers the Kubernetes rest.Config (KUBECONFIG env var
// for integration tests, else in-cluster), tunes it for high concurrency,
// and builds the DD-AUTH-014 authenticator/authorizer plus the auth
// namespace (POD_NAMESPACE env var → ServiceAccount mount → "default").
// Extracted from main() (GO-ANTIPATTERN-AUDIT-2026-07-01 Wave 0a) — pure
// code motion, no behavior change.
func buildK8sAuthDeps(logger logr.Logger) (*k8sAuthDeps, error) {
	logger.Info("Creating Kubernetes client for auth middleware (DD-AUTH-014)")

	var k8sConfig *rest.Config
	var err error
	if kubeconfigPath := os.Getenv("KUBECONFIG"); kubeconfigPath != "" {
		logger.Info("Using KUBECONFIG from environment (DD-AUTH-014)",
			"kubeconfig_path", kubeconfigPath,
			"use_case", "integration tests with envtest",
		)
		k8sConfig, err = clientcmd.BuildConfigFromFlags("", kubeconfigPath)
		if err != nil {
			return nil, fmt.Errorf("failed to load kubeconfig from KUBECONFIG env var (DD-AUTH-014, path=%s): %w",
				kubeconfigPath, err)
		}
	} else {
		logger.Info("Using in-cluster config (DD-AUTH-014)",
			"service_account_path", "/var/run/secrets/kubernetes.io/serviceaccount/",
			"use_case", "production deployment",
		)
		k8sConfig, err = rest.InClusterConfig()
		if err != nil {
			return nil, fmt.Errorf("failed to load in-cluster kubernetes config (DD-AUTH-014): %w", err)
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
		return nil, fmt.Errorf("failed to create kubernetes client (DD-AUTH-014): %w", err)
	}

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
	} else if podNamespace, readErr := os.ReadFile("/var/run/secrets/kubernetes.io/serviceaccount/namespace"); readErr == nil {
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

	return &k8sAuthDeps{
		k8sConfig:     k8sConfig,
		authenticator: auth.NewK8sAuthenticator(k8sClient),
		authorizer:    auth.NewK8sAuthorizer(k8sClient),
		authNamespace: authNamespace,
	}, nil
}

// serverParams groups buildServerWithRetry's dependencies (Options pattern,
// AGENTS.md's 8+-param rule).
type serverParams struct {
	cfg       *config.Config
	dbConnStr string
	authDeps  *k8sAuthDeps
}

// buildServerWithRetry constructs the DataStorage HTTP server, retrying the
// PostgreSQL/Redis connection (SOC2 Gap #9's hash-chained audit store; DLQ
// via Redis) up to 10 times with a 2s delay — accommodates Kind clusters
// where PostgreSQL may not be ready when DataStorage starts (E2E timing
// issue fix). Extracted from main() (GO-ANTIPATTERN-AUDIT-2026-07-01 Wave 0a)
// — pure code motion, no behavior change.
func buildServerWithRetry(p serverParams, logger logr.Logger) (*server.Server, error) {
	serverCfg := &server.Config{
		Port:         p.cfg.Server.Port,
		ReadTimeout:  p.cfg.Server.GetReadTimeout(),
		WriteTimeout: p.cfg.Server.GetWriteTimeout(),
		TLS:          p.cfg.Server.TLS,
	}

	// Gap 3.3: Pass DLQ max length for capacity monitoring
	dlqMaxLen := int64(p.cfg.Redis.DLQMaxLen)

	const (
		maxRetries = 10
		retryDelay = 2 * time.Second
	)

	logger.Info("Connecting to PostgreSQL and Redis (with retry logic)...",
		"max_retries", maxRetries,
		"retry_delay", retryDelay)

	var srv *server.Server
	var err error
	for attempt := 1; attempt <= maxRetries; attempt++ {
		srv, err = server.NewServer(server.ServerDeps{
			DBConnStr:     p.dbConnStr,
			RedisAddr:     p.cfg.Redis.Addr,
			RedisPassword: p.cfg.Redis.Password,
			Logger:        logger,
			AppConfig:     p.cfg,
			ServerConfig:  serverCfg,
			DLQMaxLen:     dlqMaxLen,
			Authenticator: p.authDeps.authenticator,
			Authorizer:    p.authDeps.authorizer,
			AuthNamespace: p.authDeps.authNamespace,
		})
		if err == nil {
			logger.Info("Successfully connected to PostgreSQL and Redis",
				"attempt", attempt)
			return srv, nil
		}

		if attempt == maxRetries {
			return nil, fmt.Errorf("failed to create server after %d attempts: %w", maxRetries, err)
		}

		logger.Info("Failed to connect, retrying...",
			"attempt", attempt,
			"max_retries", maxRetries,
			"error", err.Error(),
			"next_retry_in", retryDelay)
		time.Sleep(retryDelay)
	}
	// Unreachable: the loop above always returns on the final attempt.
	return nil, fmt.Errorf("failed to create server after %d attempts: %w", maxRetries, err)
}

// observabilityServers groups the Prometheus metrics and health-probe
// servers plus their async error channels, built together in
// startObservabilityServers (Options-pattern result struct).
type observabilityServers struct {
	metrics       *http.Server
	health        *http.Server
	metricsErrors chan error
	healthErrors  chan error
}

// startObservabilityServers starts the Issue #283 Prometheus metrics server
// and the Issue #753 health-probe server, each on their own goroutine,
// reporting unexpected listener errors on buffered channels. Extracted from
// main() (GO-ANTIPATTERN-AUDIT-2026-07-01 Wave 0a) — pure code motion, no
// behavior change.
func startObservabilityServers(cfg *config.Config, srv *server.Server, logger logr.Logger) *observabilityServers {
	metricsMux := http.NewServeMux()
	metricsMux.Handle("/metrics", promhttp.Handler())
	metricsServer := &http.Server{
		Addr:              fmt.Sprintf(":%d", cfg.Server.MetricsPort),
		Handler:           metricsMux,
		ReadHeaderTimeout: 5 * time.Second,
		ReadTimeout:       10 * time.Second,
		WriteTimeout:      10 * time.Second,
		IdleTimeout:       120 * time.Second,
	}

	metricsErrors := make(chan error, 1)
	go func() {
		logger.Info("Metrics server listening", "addr", metricsServer.Addr)
		if err := metricsServer.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			metricsErrors <- err
		}
	}()

	// Issue #753: Dedicated health probe server on standardised port (CONFIG_STANDARDS.md)
	healthServer := health.NewHealthServer(
		fmt.Sprintf(":%d", cfg.Server.HealthPort),
		srv.LivenessHandler(),
		srv.ReadinessHandler(),
		!cfg.Server.DisableProfiling,
	)

	healthErrors := make(chan error, 1)
	go func() {
		logger.Info("Health server listening", "addr", healthServer.Addr)
		if err := healthServer.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			healthErrors <- err
		}
	}()

	return &observabilityServers{
		metrics:       metricsServer,
		health:        healthServer,
		metricsErrors: metricsErrors,
		healthErrors:  healthErrors,
	}
}

// wireHotReload sets the initial TLS security profile (Issue #748) and
// starts the CA-cert (Issue #756) and log-level (Issue #875) hot-reload file
// watchers. Returns a combined stop function the caller should defer.
// Extracted from main() (GO-ANTIPATTERN-AUDIT-2026-07-01 Wave 0a) — pure
// code motion, no behavior change.
func wireHotReload(
	ctx context.Context,
	cfg *config.Config,
	cfgPath string,
	atomicLevel zaplog.AtomicLevel,
	logger logr.Logger,
) func() {
	// Issue #748: Load OCP TLS security profile from config before any TLS setup
	if err := sharedtls.SetDefaultSecurityProfileFromConfig(cfg.TLSProfile); err != nil {
		logger.Error(err, "Invalid TLS security profile in config, using default TLS 1.2")
	} else if cfg.TLSProfile != "" {
		logger.Info("TLS security profile active", "profile", cfg.TLSProfile)
	}

	stopFns := make([]func(), 0, 2)

	// Issue #756: Start CA file watcher for client-side TLS hot-reload
	caWatcher, caWatchErr := sharedtls.StartCAFileWatcher(ctx, logger)
	if caWatchErr != nil {
		logger.Error(caWatchErr, "Failed to start CA file watcher")
		os.Exit(1)
	}
	if caWatcher != nil {
		stopFns = append(stopFns, caWatcher.Stop)
	}

	if stop := wireLogLevelHotReload(ctx, cfgPath, atomicLevel, logger); stop != nil {
		stopFns = append(stopFns, stop)
	}

	return func() {
		for _, stop := range stopFns {
			stop()
		}
	}
}

// wireLogLevelHotReload starts the Issue #875 config-file watcher that
// hot-reloads the log level on change, returning its Stop function (or nil if
// cfgPath is empty or the watcher failed to start). Extracted from
// wireHotReload (GO-ANTIPATTERN-AUDIT-2026-07-01 Wave 3) — pure code motion,
// no behavior change.
func wireLogLevelHotReload(ctx context.Context, cfgPath string, atomicLevel zaplog.AtomicLevel, logger logr.Logger) func() {
	if cfgPath == "" {
		return nil
	}

	logLevelWatcher, logWatchErr := hotreload.NewFileWatcher(
		cfgPath,
		func(newContent string) error {
			var partial struct {
				Logging struct {
					Level string `yaml:"level"`
				} `yaml:"logging"`
			}
			if err := yaml.Unmarshal([]byte(newContent), &partial); err != nil {
				return fmt.Errorf("failed to parse config for log level reload: %w", err)
			}
			lvl := internalconfig.LoggingConfig{Level: strings.ToUpper(partial.Logging.Level)}
			return internalconfig.ParseAndSetLevel(atomicLevel, lvl.Level)
		},
		logger.WithName("log-level-watcher"),
	)
	if logWatchErr != nil {
		logger.Error(logWatchErr, "Failed to create log level file watcher")
		return nil
	}

	if err := logLevelWatcher.Start(ctx); err != nil {
		logger.Info("Log level file watcher failed to start", "error", err)
		return nil
	}

	logger.Info("Log level hot-reload watcher started", "path", cfgPath)
	return logLevelWatcher.Stop
}
