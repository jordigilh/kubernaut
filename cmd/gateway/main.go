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

	"gopkg.in/yaml.v3"

	internalconfig "github.com/jordigilh/kubernaut/internal/config"
	"github.com/jordigilh/kubernaut/internal/version"
	"github.com/jordigilh/kubernaut/pkg/gateway"
	"github.com/jordigilh/kubernaut/pkg/gateway/adapters"
	"github.com/jordigilh/kubernaut/pkg/gateway/config"
	kubelog "github.com/jordigilh/kubernaut/pkg/log"
	"github.com/jordigilh/kubernaut/pkg/shared/hotreload"
	sharedtls "github.com/jordigilh/kubernaut/pkg/shared/tls"
	ctrl "sigs.k8s.io/controller-runtime"
)

func main() {
	// ADR-030: Single --config flag; all functional config in YAML ConfigMap
	var configPath string
	flag.StringVar(&configPath, "config", config.DefaultConfigPath, "Path to YAML configuration file (optional, falls back to defaults)")
	flag.Parse()

	// Bootstrap logger at INFO for config loading
	bootstrapLevel := internalconfig.DefaultLoggingConfig().NewAtomicLevel()
	logger := kubelog.NewLoggerWithAtomicLevel(kubelog.Options{
		ServiceName: "gateway",
	}, bootstrapLevel)
	defer kubelog.Sync(logger)

	ctrl.SetLogger(logger)

	logger.Info("Starting Gateway Service",
		"version", version.Version,
		"gitCommit", version.GitCommit,
		"buildDate", version.BuildDate,
		"config_path", configPath)

	// ADR-030: Load configuration from YAML file
	var serverCfg *config.ServerConfig
	if configPath != "" {
		var err error
		serverCfg, err = config.LoadFromFile(configPath)
		if err != nil {
			logger.Error(err, "Failed to load configuration",
				"config_path", configPath)
			os.Exit(1)
		}
		logger.Info("Configuration loaded successfully", "config_path", configPath)
	} else {
		logger.Info("No config file specified, using defaults")
		serverCfg = config.DefaultServerConfig()
	}

	// Issue #877: Apply config-driven log level
	atomicLevel := serverCfg.Logging.NewAtomicLevel()
	logger = kubelog.NewLoggerWithAtomicLevel(kubelog.Options{
		ServiceName: "gateway",
	}, atomicLevel)
	ctrl.SetLogger(logger)
	logger.Info("Log level configured from config file", "level", serverCfg.Logging.Level)

	// Override configuration with environment variables (e.g., secrets only per ADR-030)
	serverCfg.LoadFromEnv()

	// Validate configuration
	if err := serverCfg.Validate(); err != nil {
		logger.Error(err, "Invalid configuration")
		os.Exit(1)
	}

	logger.Info("Configuration validated",
		"listen_addr", serverCfg.Server.ListenAddr,
		"data_storage_url", serverCfg.DataStorage.URL)

	// Create Gateway server
	srv, err := gateway.NewServer(serverCfg, logger.WithName("server"))
	if err != nil {
		logger.Error(err, "Failed to create Gateway server")
		os.Exit(1)
	}

	// Register adapters (BR-GATEWAY-001, BR-GATEWAY-002)
	// BR-GATEWAY-004: Owner chain resolution for signal deduplication (Issue #63).
	// Uses the same ctrlClient as scope management (ADR-053) — metadata-only informer
	// cache, zero additional API calls. Shared across all adapters.
	ownerResolver := adapters.NewK8sOwnerResolver(
		srv.GetCachedClient(),
		logger.WithName("owner-resolver"),
		adapters.WithFallbackReader(srv.GetAPIReader()),
	)

	// Prometheus AlertManager webhook adapter
	// Issue #63: alertname excluded from fingerprint; OwnerResolver resolves Pod→Deployment
	// Issue #191 / BR-GATEWAY-184: Filter monitoring metadata labels during target extraction
	labelFilter := adapters.NewMonitoringMetadataFilter(logger)
	prometheusAdapter := adapters.NewPrometheusAdapter(ownerResolver, labelFilter, logger)
	if err := srv.RegisterAdapter(prometheusAdapter); err != nil {
		logger.Error(err, "Failed to register Prometheus adapter")
		os.Exit(1)
	}

	// Kubernetes Event webhook adapter
	k8sEventAdapter := adapters.NewKubernetesEventAdapter(ownerResolver)
	k8sEventAdapter.SetLogger(logger)
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

	// Issue #748: Load OCP TLS security profile from config before any TLS setup
	if err := sharedtls.SetDefaultSecurityProfileFromConfig(serverCfg.TLSProfile); err != nil {
		logger.Error(err, "Invalid TLS security profile in config, using default TLS 1.2")
	} else if serverCfg.TLSProfile != "" {
		logger.Info("TLS security profile active", "profile", serverCfg.TLSProfile)
	}

	// Issue #877: Log level hot-reload via FileWatcher
	if configPath != "" {
		logLevelWatcher, watchErr := hotreload.NewFileWatcher(
			configPath,
			func(newContent string) error {
				var partial struct {
					Logging internalconfig.LoggingConfig `yaml:"logging"`
				}
				if err := yaml.Unmarshal([]byte(newContent), &partial); err != nil {
					return fmt.Errorf("failed to parse config for log level reload: %w", err)
				}
				return internalconfig.ParseAndSetLevel(atomicLevel, partial.Logging.Level)
			},
			logger.WithName("log-level-watcher"),
		)
		if watchErr != nil {
			logger.Error(watchErr, "Failed to create log level file watcher")
		} else {
			if err := logLevelWatcher.Start(serverCtx); err != nil {
				logger.Info("Log level file watcher failed to start", "error", err)
			} else {
				logger.Info("Log level hot-reload watcher started", "path", configPath)
				defer logLevelWatcher.Stop()
			}
		}
	}

	// Issue #756: Start CA file watcher for client-side TLS hot-reload
	caWatcher, caWatchErr := sharedtls.StartCAFileWatcher(serverCtx, logger)
	if caWatchErr != nil {
		logger.Error(caWatchErr, "Failed to start CA file watcher")
		os.Exit(1)
	}
	if caWatcher != nil {
		defer caWatcher.Stop()
	}

	go func() {
		logger.Info("Gateway server starting", "address", serverCfg.Server.ListenAddr)
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

	// DD-GATEWAY-012: Redis close REMOVED - Gateway is now Redis-free
	logger.Info("Initiating graceful shutdown...")
	if err := srv.Stop(shutdownCtx); err != nil {
		logger.Error(err, "Graceful shutdown failed")
		os.Exit(1)
	}

	logger.Info("Gateway server shutdown complete")
}
