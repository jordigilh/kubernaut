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

	"github.com/go-logr/logr"
	"go.uber.org/zap"
	"gopkg.in/yaml.v3"

	internalconfig "github.com/jordigilh/kubernaut/internal/config"
	"github.com/jordigilh/kubernaut/internal/version"
	fleetclient "github.com/jordigilh/kubernaut/pkg/fleet/mcpclient"
	"github.com/jordigilh/kubernaut/pkg/gateway"
	"github.com/jordigilh/kubernaut/pkg/gateway/adapters"
	"github.com/jordigilh/kubernaut/pkg/gateway/config"
	kubelog "github.com/jordigilh/kubernaut/pkg/log"
	"github.com/jordigilh/kubernaut/pkg/shared/hotreload"
	sharedtls "github.com/jordigilh/kubernaut/pkg/shared/tls"
	"k8s.io/client-go/dynamic"
	"k8s.io/client-go/kubernetes"
	ctrl "sigs.k8s.io/controller-runtime"
)

// loadGatewayConfig loads and validates the gateway ServerConfig (ADR-030:
// single --config flag, all functional config in YAML ConfigMap; secrets via
// env per LoadFromEnv), and reconfigures the logger at the config-driven log
// level (Issue #877). Exits the process on load or validation failure,
// matching main()'s original fail-fast behavior.
func loadGatewayConfig(configPath string, bootstrapLogger logr.Logger) (*config.ServerConfig, logr.Logger, zap.AtomicLevel) {
	bootstrapLogger.Info("Starting Gateway Service",
		"version", version.Version,
		"gitCommit", version.GitCommit,
		"buildDate", version.BuildDate,
		"config_path", configPath)

	var serverCfg *config.ServerConfig
	if configPath != "" {
		var err error
		serverCfg, err = config.LoadFromFile(configPath)
		if err != nil {
			bootstrapLogger.Error(err, "Failed to load configuration",
				"config_path", configPath)
			os.Exit(1)
		}
		bootstrapLogger.Info("Configuration loaded successfully", "config_path", configPath)
	} else {
		bootstrapLogger.Info("No config file specified, using defaults")
		serverCfg = config.DefaultServerConfig()
	}

	// Issue #877: Apply config-driven log level
	atomicLevel := serverCfg.Logging.NewAtomicLevel()
	logger := kubelog.NewLoggerWithAtomicLevel(kubelog.Options{
		ServiceName: "gateway",
	}, atomicLevel)
	ctrl.SetLogger(logger)
	logger.Info("Log level configured from config file", "level", serverCfg.Logging.Level)

	// Override configuration with environment variables (e.g., secrets only per ADR-030)
	serverCfg.LoadFromEnv()

	if err := serverCfg.Validate(); err != nil {
		logger.Error(err, "Invalid configuration")
		os.Exit(1)
	}

	logger.Info("Configuration validated",
		"listen_addr", serverCfg.Server.ListenAddr,
		"data_storage_url", serverCfg.DataStorage.URL)

	return serverCfg, logger, atomicLevel
}

func main() {
	// ADR-030: Single --config flag; all functional config in YAML ConfigMap
	var configPath string
	flag.StringVar(&configPath, "config", config.DefaultConfigPath, "Path to YAML configuration file (optional, falls back to defaults)")
	flag.Parse()

	// Bootstrap logger at INFO for config loading
	bootstrapLevel := internalconfig.DefaultLoggingConfig().NewAtomicLevel()
	bootstrapLogger := kubelog.NewLoggerWithAtomicLevel(kubelog.Options{
		ServiceName: "gateway",
	}, bootstrapLevel)
	defer kubelog.Sync(bootstrapLogger)
	ctrl.SetLogger(bootstrapLogger)

	serverCfg, logger, atomicLevel := loadGatewayConfig(configPath, bootstrapLogger)

	// Create Gateway server
	srv, err := gateway.NewServer(serverCfg, logger.WithName("server"))
	if err != nil {
		logger.Error(err, "Failed to create Gateway server")
		os.Exit(1)
	}

	// Server lifecycle context — created early so the discovery refresh loop
	// can be started before the HTTP server goroutine.
	serverCtx, serverCancel := context.WithCancel(context.Background())
	defer serverCancel()

	// Issue #1029: Dynamic API resource registry — replaces static kindToGroup +
	// resourceCandidates + LabelFilter with fully dynamic discovery.
	apiRegistry, err := buildAPIRegistry(serverCtx, srv, logger)
	if err != nil {
		logger.Error(err, "Failed to initialize API resource registry")
		os.Exit(1)
	}

	// Register adapters (BR-GATEWAY-001, BR-GATEWAY-002) and optionally wire the
	// Fleet MCP Gateway for remote owner chain resolution (BR-INTEGRATION-065).
	fleetResilientClient, err := registerAdapters(serverCtx, srv, apiRegistry, serverCfg, logger)
	if err != nil {
		logger.Error(err, "Failed to register adapters")
		os.Exit(1)
	}

	// Start server in goroutine
	errChan := make(chan error, 1)

	// Issue #748/#877/#756: TLS security profile + log-level and CA-cert hot-reload watchers.
	stopHotReload := wireHotReload(serverCtx, serverCfg, configPath, atomicLevel, srv, logger)
	defer stopHotReload()

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

	// BR-INTEGRATION-054: Graceful shutdown for fleet MCP client
	if fleetResilientClient != nil {
		logger.Info("Closing fleet MCP Gateway connection")
		if err := fleetResilientClient.Close(); err != nil {
			logger.Error(err, "Failed to close fleet MCP client gracefully")
		}
	}

	// DD-GATEWAY-012: Redis close REMOVED - Gateway is now Redis-free
	logger.Info("Initiating graceful shutdown...")
	if err := srv.Stop(shutdownCtx); err != nil {
		logger.Error(err, "Graceful shutdown failed")
		os.Exit(1)
	}

	logger.Info("Gateway server shutdown complete")
}

// buildAPIRegistry builds the dynamic API resource registry (Issue #1029)
// used for discovery-driven owner-chain resolution, and starts its
// background refresh loop. Extracted from main() (GO-ANTIPATTERN-AUDIT-2026-07-01
// Wave 0a) — pure code motion, no behavior change.
func buildAPIRegistry(ctx context.Context, srv *gateway.Server, logger logr.Logger) (*adapters.APIResourceRegistry, error) {
	kubeConfig, err := ctrl.GetConfig()
	if err != nil {
		return nil, fmt.Errorf("failed to get kubernetes config for API discovery: %w", err)
	}
	k8sClientset, err := kubernetes.NewForConfig(kubeConfig)
	if err != nil {
		return nil, fmt.Errorf("failed to create kubernetes clientset for API discovery: %w", err)
	}
	dynClient, err := dynamic.NewForConfig(kubeConfig)
	if err != nil {
		return nil, fmt.Errorf("failed to create dynamic kubernetes client for existence checks: %w", err)
	}
	apiRegistry, err := adapters.NewAPIResourceRegistry(
		k8sClientset.Discovery(),
		adapters.WithRefreshInterval(5*time.Minute),
		adapters.WithCacheTTL(30*time.Second),
		adapters.WithDynamicClient(dynClient),
		adapters.WithRegistryLogger(logger.WithName("api-registry")),
		adapters.WithRefreshErrorCounter(srv.GetMetrics().DiscoveryRefreshErrorsTotal),
	)
	if err != nil {
		return nil, fmt.Errorf("dynamic resource discovery is unavailable; verify ServiceAccount RBAC for "+
			"system:discovery: %w", err)
	}
	apiRegistry.StartRefreshLoop(ctx)
	return apiRegistry, nil
}

// registerAdapters builds and registers the Prometheus and Kubernetes-Event
// signal adapters (BR-GATEWAY-001, BR-GATEWAY-002), including owner-chain
// resolution (BR-GATEWAY-004) and the optional Fleet MCP Gateway wiring for
// remote owner resolution (BR-INTEGRATION-065). Returns the Fleet resilient
// client (nil if Fleet isn't configured) so the caller can close it on
// shutdown. Extracted from main() (GO-ANTIPATTERN-AUDIT-2026-07-01 Wave 0a)
// — pure code motion, no behavior change.
func registerAdapters(
	ctx context.Context,
	srv *gateway.Server,
	apiRegistry *adapters.APIResourceRegistry,
	serverCfg *config.ServerConfig,
	logger logr.Logger,
) (*fleetclient.ResilientClient, error) {
	ownerResolver := adapters.NewK8sOwnerResolver(
		srv.GetCachedClient(),
		logger.WithName("owner-resolver"),
		adapters.WithFallbackReader(srv.GetAPIReader()),
		adapters.WithRegistry(apiRegistry),
	)

	// Prometheus AlertManager webhook adapter
	// Issue #63: alertname excluded from fingerprint; OwnerResolver resolves Pod→Deployment
	// Issue #1029: Dynamic API resource registry for multi-candidate scoring
	prometheusAdapter := adapters.NewPrometheusAdapter(ownerResolver, apiRegistry, logger)
	prometheusAdapter.SetOwnerResolutionMetric(srv.GetMetrics().OwnerResolutionTotal)
	prometheusAdapter.SetParseDroppedMetric(srv.GetMetrics().SignalsParseDroppedTotal)

	// BR-INTEGRATION-065: Fleet MCP Gateway for remote owner chain resolution.
	// When mcpGatewayEndpoint is configured, GW constructs a ReaderFactory that
	// dispatches owner resolution to the remote cluster's K8s API via MCP.
	fleetResilientClient := wireFleetOwnerResolution(ctx, srv, prometheusAdapter, serverCfg, logger)

	if err := srv.RegisterAdapter(prometheusAdapter); err != nil {
		return fleetResilientClient, fmt.Errorf("failed to register prometheus adapter: %w", err)
	}

	// Kubernetes Event webhook adapter
	k8sEventAdapter := adapters.NewKubernetesEventAdapter(ownerResolver)
	k8sEventAdapter.SetLogger(logger)
	if err := srv.RegisterAdapter(k8sEventAdapter); err != nil {
		return fleetResilientClient, fmt.Errorf("failed to register k8s event adapter: %w", err)
	}

	logger.Info("Registered all adapters",
		"adapter_count", 2,
		"adapters", []string{"prometheus", "kubernetes-event"})

	return fleetResilientClient, nil
}

// wireFleetOwnerResolution wires the Fleet MCP Gateway for remote owner
// chain resolution (BR-INTEGRATION-065) when configured, setting the
// Prometheus adapter's ReaderFactory on success. Returns the resilient
// client (nil if Fleet isn't configured or the connection fails) so the
// caller can close it on shutdown.
func wireFleetOwnerResolution(
	ctx context.Context,
	srv *gateway.Server,
	prometheusAdapter *adapters.PrometheusAdapter,
	serverCfg *config.ServerConfig,
	logger logr.Logger,
) *fleetclient.ResilientClient {
	if !serverCfg.Fleet.Enabled || serverCfg.Fleet.MCPGatewayEndpoint == "" {
		return nil
	}

	logger.Info("Fleet MCP Gateway configured for remote owner chain resolution",
		"endpoint", serverCfg.Fleet.MCPGatewayEndpoint,
		"oauth2Enabled", serverCfg.Fleet.OAuth2.Enabled)

	fleetOpts := []fleetclient.Option{}
	if serverCfg.Fleet.OAuth2.Enabled {
		fleetOpts = append(fleetOpts, buildFleetOAuth2Option(serverCfg, logger.WithName("fleet-oauth2")))
	}

	resilienceCfg := fleetclient.DefaultResilienceConfig()
	fleetResilientClient, fleetErr := fleetclient.NewResilient(
		ctx, serverCfg.Fleet.MCPGatewayEndpoint, resilienceCfg,
		logger.WithName("fleet-client"), fleetOpts...,
	)
	if fleetErr != nil {
		logger.Error(fleetErr, "Fleet MCP Gateway connection failed, remote owner resolution disabled",
			"endpoint", serverCfg.Fleet.MCPGatewayEndpoint)
		return nil
	}

	readerFactory := fleetclient.NewMCPReaderFactory(srv.GetCachedClient(), fleetResilientClient.Session())
	prometheusAdapter.SetReaderFactory(readerFactory)
	logger.Info("Fleet MCP Gateway connected, remote owner chain resolution enabled",
		"endpoint", serverCfg.Fleet.MCPGatewayEndpoint)
	return fleetResilientClient
}

// buildFleetOAuth2Option builds the reloadable OAuth2 transport option for
// the Fleet MCP client from the server config's credentials secret path.
func buildFleetOAuth2Option(serverCfg *config.ServerConfig, fleetLog logr.Logger) fleetclient.Option {
	basePath := "/etc/gateway/fleet-oauth2"
	if serverCfg.Fleet.OAuth2.CredentialsSecretRef != "" {
		basePath = "/etc/gateway/" + serverCfg.Fleet.OAuth2.CredentialsSecretRef
	}
	reloadCfg := fleetclient.ReloadableOAuth2Config{
		TokenURL:         serverCfg.Fleet.OAuth2.TokenURL,
		ClientIDPath:     basePath + "/client-id",
		ClientSecretPath: basePath + "/client-secret",
		Scopes:           fleetclient.DefaultFleetScopes(serverCfg.Fleet.OAuth2.Scopes),
	}
	return fleetclient.WithReloadableOAuth2Transport(reloadCfg, fleetLog)
}

// startLogLevelWatcher starts the config-file log-level hot-reload watcher
// (Issue #877) when a configPath is set. Returns the watcher's stop function
// and true on success; ok is false when no watcher was started (no
// configPath, or the watcher failed to create/start).
func startLogLevelWatcher(
	ctx context.Context,
	configPath string,
	atomicLevel zap.AtomicLevel,
	srv *gateway.Server,
	logger logr.Logger,
) (stop func(), ok bool) {
	if configPath == "" {
		return nil, false
	}

	logLevelWatcher, watchErr := hotreload.NewFileWatcher(
		configPath,
		func(newContent string) error {
			var partial struct {
				Logging internalconfig.LoggingConfig `yaml:"logging"`
			}
			reloadErr := func() error {
				if err := yaml.Unmarshal([]byte(newContent), &partial); err != nil {
					return fmt.Errorf("failed to parse config for log level reload: %w", err)
				}
				return internalconfig.ParseAndSetLevel(atomicLevel, partial.Logging.Level)
			}()
			// GAP-11 (Issue #1505): audit every log-level hot-reload attempt,
			// success or rejection (SOC2 CC7.2 change management).
			srv.EmitConfigReloadAudit(ctx, "log_level", reloadErr)
			return reloadErr
		},
		logger.WithName("log-level-watcher"),
	)
	if watchErr != nil {
		logger.Error(watchErr, "Failed to create log level file watcher")
		return nil, false
	}

	if err := logLevelWatcher.Start(ctx); err != nil {
		logger.Info("Log level file watcher failed to start", "error", err)
		return nil, false
	}

	logger.Info("Log level hot-reload watcher started", "path", configPath)
	return logLevelWatcher.Stop, true
}

// wireHotReload sets the initial TLS security profile and starts the
// log-level and CA-cert hot-reload file watchers (Issues #748, #877, #756).
// Returns a combined stop function the caller should defer. Exits the
// process on an invalid TLS profile or CA-watcher startup failure, matching
// main()'s original fail-fast behavior. Extracted from main()
// (GO-ANTIPATTERN-AUDIT-2026-07-01 Wave 0a) — pure code motion, no behavior
// change.
func wireHotReload(
	ctx context.Context,
	serverCfg *config.ServerConfig,
	configPath string,
	atomicLevel zap.AtomicLevel,
	srv *gateway.Server,
	logger logr.Logger,
) func() {
	// Issue #748: Load OCP TLS security profile from config before any TLS setup
	if err := sharedtls.SetDefaultSecurityProfileFromConfig(serverCfg.TLSProfile); err != nil {
		logger.Error(err, "Invalid TLS security profile in config — refusing to start with wrong TLS posture")
		os.Exit(1)
	} else if serverCfg.TLSProfile != "" {
		logger.Info("TLS security profile active", "profile", serverCfg.TLSProfile)
	}

	stopFns := make([]func(), 0, 2)

	// Issue #877: Log level hot-reload via FileWatcher
	if stop, ok := startLogLevelWatcher(ctx, configPath, atomicLevel, srv, logger); ok {
		stopFns = append(stopFns, stop)
	}

	// Issue #756: Start CA file watcher for client-side TLS hot-reload
	// GAP-11 (Issue #1505): audit every CA-cert hot-reload attempt.
	caWatcher, caWatchErr := sharedtls.StartCAFileWatcher(ctx, logger, func(reloadErr error) {
		srv.EmitConfigReloadAudit(ctx, "ca_cert", reloadErr)
	})
	if caWatchErr != nil {
		logger.Error(caWatchErr, "Failed to start CA file watcher")
		os.Exit(1)
	}
	if caWatcher != nil {
		stopFns = append(stopFns, caWatcher.Stop)
	}

	return func() {
		for _, stop := range stopFns {
			stop()
		}
	}
}
