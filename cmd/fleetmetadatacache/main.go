/*
Copyright 2026 Jordi Gil.

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

// FMC: Fleet Metadata Cache service.
// Writes: Polls remote clusters via MCP Gateway for resources labeled kubernaut.ai/managed=true
// and writes their metadata to Valkey.
// Reads: Serves an HTTP API for federated scope checking (ADR-068), so GW/RO
// query FMC instead of connecting to Valkey directly.
package main

import (
	"context"
	"flag"
	"net/http"
	"os"
	"os/signal"
	"sync/atomic"
	"syscall"
	"time"

	"github.com/go-logr/zapr"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"go.uber.org/zap"
	"k8s.io/client-go/dynamic"
	ctrl "sigs.k8s.io/controller-runtime"

	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/jordigilh/kubernaut/pkg/fleet"
	"github.com/jordigilh/kubernaut/pkg/fleet/fmc"
	fmcconfig "github.com/jordigilh/kubernaut/pkg/fleet/fmc/config"
	"github.com/jordigilh/kubernaut/pkg/fleet/mcpclient"
	"github.com/jordigilh/kubernaut/pkg/fleet/registry"
	"github.com/jordigilh/kubernaut/pkg/fleet/scopecache"
)

func main() {
	var configPath string
	flag.StringVar(&configPath, "config", fmcconfig.DefaultConfigPath, "Path to YAML config file (ADR-030)")
	flag.Parse()

	zapLogger, _ := zap.NewProduction()
	defer func() { _ = zapLogger.Sync() }()
	logger := zapr.NewLogger(zapLogger)

	cfg, err := fmcconfig.LoadFromFile(configPath)
	if err != nil {
		logger.Error(err, "Failed to load configuration", "path", configPath)
		os.Exit(1)
	}
	if err := cfg.Validate(); err != nil {
		logger.Error(err, "Invalid configuration")
		os.Exit(1)
	}

	logger.Info("FMC starting",
		"syncInterval", cfg.Sync.Interval,
		"valkeyAddr", cfg.Valkey.Addr,
		"mcpEndpoint", cfg.MCPGateway.Endpoint,
		"gatewayType", cfg.MCPGateway.GatewayType,
		"apiAddr", cfg.Server.APIAddr,
		"metricsAddr", cfg.Server.MetricsAddr)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGTERM, syscall.SIGINT)

	reg := prometheus.NewRegistry()
	metrics := fmc.NewMetrics(reg)

	reloadCfg := mcpclient.ReloadableOAuth2Config{
		TokenURL:         cfg.OAuth2.TokenURL,
		ClientIDPath:     cfg.OAuth2.CredentialsDir + "/client-id",
		ClientSecretPath: cfg.OAuth2.CredentialsDir + "/client-secret",
		Scopes:           cfg.OAuth2.Scopes,
		TokenTimeout:     cfg.OAuth2.TokenTimeout,
		TlsCaFile:        cfg.OAuth2.TlsCaFile,
	}
	opts := []mcpclient.Option{
		mcpclient.WithReloadableOAuth2Transport(reloadCfg, logger),
	}
	logger.Info("OAuth2 authentication configured for MCP Gateway",
		"tokenURL", cfg.OAuth2.TokenURL,
		"credentialsDir", cfg.OAuth2.CredentialsDir)

	resilienceCfg := mcpclient.DefaultResilienceConfig()
	mcpClient, err := mcpclient.NewResilient(ctx, cfg.MCPGateway.Endpoint, resilienceCfg, logger, opts...)
	if err != nil {
		logger.Error(err, "Failed to connect to MCP Gateway")
		os.Exit(1)
	}
	defer func() { _ = mcpClient.Close() }()

	k8sCfg, err := ctrl.GetConfig()
	if err != nil {
		logger.Error(err, "Failed to get Kubernetes config")
		os.Exit(1)
	}
	dynClient, err := dynamic.NewForConfig(k8sCfg)
	if err != nil {
		logger.Error(err, "Failed to create dynamic Kubernetes client")
		os.Exit(1)
	}

	writer := fmc.NewValkeyWriter(cfg.Valkey.Addr)
	defer func() { _ = writer.Close() }()

	cacheReader := scopecache.NewValkeyCacheReader(cfg.Valkey.Addr)
	defer func() { _ = cacheReader.Close() }()

	clusterRegistry, err := registry.NewClusterRegistry(registry.MCPGatewayType(cfg.MCPGateway.GatewayType), dynClient, registry.RegistryConfig{
		Namespace: cfg.MCPGateway.Namespace,
	}, registry.NewMetricsWithRegistry(reg), logger)
	if err != nil {
		logger.Error(err, "Failed to create cluster registry", "gatewayType", cfg.MCPGateway.GatewayType)
		os.Exit(1)
	}
	if err := clusterRegistry.Start(ctx); err != nil {
		logger.Error(err, "Failed to start cluster registry")
		os.Exit(1)
	}
	defer clusterRegistry.Stop()

	syncerConfig := fmc.Config{
		SyncInterval:       cfg.Sync.Interval,
		KeyTTL:             cfg.Sync.KeyTTL,
		ResourceKinds:      cfg.Sync.ResourceKinds,
		WaitForBrokerReady: cfg.Sync.WaitForBrokerReady,
	}

	sessionProvider := mcpClient.SessionProvider()
	readerFactory := fleet.ReaderFactoryFunc(func(_ context.Context, clusterID string) (client.Reader, error) {
		var opts []mcpclient.Option
		if info, found := clusterRegistry.Get(clusterID); found && info.ToolPrefix != "" {
			opts = append(opts, mcpclient.WithToolPrefix(info.ToolPrefix))
		}
		return mcpclient.NewFromSessionProvider(sessionProvider, clusterID, opts...), nil
	})
	syncer := fmc.NewSyncerWithReaderFactory(clusterRegistry, readerFactory, writer, syncerConfig, logger, metrics)

	var ready atomic.Bool

	scopeClient := scopecache.NewClient(cacheReader)
	apiHandler := fmc.NewHandler(scopeClient, clusterRegistry, logger)
	apiMux := http.NewServeMux()
	apiHandler.RegisterRoutes(apiMux)
	apiMux.HandleFunc("/healthz", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("ok"))
	})
	apiMux.HandleFunc("/readyz", fmc.ReadyzHandler(ready.Load, cacheReader))

	apiServer := &http.Server{
		Addr:              cfg.Server.APIAddr,
		Handler:           apiMux,
		ReadHeaderTimeout: 5 * time.Second,
	}

	metricsMux := http.NewServeMux()
	metricsMux.Handle("/metrics", promhttp.HandlerFor(reg, promhttp.HandlerOpts{}))

	metricsServer := &http.Server{
		Addr:              cfg.Server.MetricsAddr,
		Handler:           metricsMux,
		ReadHeaderTimeout: 5 * time.Second,
	}

	apiErrors := make(chan error, 1)
	go func() {
		logger.Info("API server listening", "addr", apiServer.Addr)
		if err := apiServer.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			apiErrors <- err
		}
	}()

	metricsErrors := make(chan error, 1)
	go func() {
		logger.Info("Metrics server listening", "addr", metricsServer.Addr)
		if err := metricsServer.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			metricsErrors <- err
		}
	}()

	go func() {
		if err := syncer.Run(ctx); err != nil {
			logger.Error(err, "Syncer stopped with error")
			cancel()
		}
	}()

	ready.Store(mcpClient.Ready())
	logger.Info("FMC ready", "mcpConnected", mcpClient.Ready())

	select {
	case <-sigCh:
		logger.Info("Received shutdown signal")
	case err := <-apiErrors:
		logger.Error(err, "API server failed")
	case err := <-metricsErrors:
		logger.Error(err, "Metrics server failed")
	}
	cancel()

	shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer shutdownCancel()
	if err := apiServer.Shutdown(shutdownCtx); err != nil {
		logger.Error(err, "API server shutdown failed")
	}
	if err := metricsServer.Shutdown(shutdownCtx); err != nil {
		logger.Error(err, "Metrics server shutdown failed")
	}
}
