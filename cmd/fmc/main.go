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
	"fmt"
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

	"github.com/jordigilh/kubernaut/pkg/fleet/fmc"
	"github.com/jordigilh/kubernaut/pkg/fleet/mcpclient"
	"github.com/jordigilh/kubernaut/pkg/fleet/registry"
	"github.com/jordigilh/kubernaut/pkg/fleet/scopecache"
)

func main() {
	zapLogger, _ := zap.NewProduction()
	defer func() { _ = zapLogger.Sync() }()
	logger := zapr.NewLogger(zapLogger)

	cfg := loadConfig()
	logger.Info("FMC starting",
		"syncInterval", cfg.SyncInterval,
		"valkeyAddr", cfg.ValkeyAddr,
		"mcpEndpoint", cfg.MCPGatewayEndpoint,
		"apiAddr", cfg.APIAddr,
		"metricsAddr", cfg.MetricsAddr)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGTERM, syscall.SIGINT)

	reg := prometheus.NewRegistry()
	metrics := fmc.NewMetrics(reg)

	var opts []mcpclient.Option
	if cfg.OAuth2Enabled {
		reloadCfg := mcpclient.ReloadableOAuth2Config{
			TokenURL:         cfg.OAuth2TokenURL,
			ClientIDPath:     cfg.OAuth2SecretPath + "/client-id",
			ClientSecretPath: cfg.OAuth2SecretPath + "/client-secret",
			Scopes:           []string{"fleet"},
			TokenTimeout:     10 * time.Second,
		}
		opts = append(opts, mcpclient.WithReloadableOAuth2Transport(reloadCfg, logger))
		logger.Info("OAuth2 authentication configured for MCP Gateway",
			"tokenURL", cfg.OAuth2TokenURL,
			"secretPath", cfg.OAuth2SecretPath)
	}

	resilienceCfg := mcpclient.DefaultResilienceConfig()
	mcpClient, err := mcpclient.NewResilient(ctx, cfg.MCPGatewayEndpoint, resilienceCfg, logger, opts...)
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

	writer := fmc.NewValkeyWriter(cfg.ValkeyAddr)
	defer func() { _ = writer.Close() }()

	cacheReader := scopecache.NewValkeyCacheReader(cfg.ValkeyAddr)
	defer func() { _ = cacheReader.Close() }()

	clusterRegistry := registry.NewCRDWatcher(dynClient, registry.CRDWatcherConfig{
		Namespace: cfg.Namespace,
	}, registry.NewMetricsWithRegistry(reg), logger)
	if err := clusterRegistry.Start(ctx); err != nil {
		logger.Error(err, "Failed to start cluster registry")
		os.Exit(1)
	}
	defer clusterRegistry.Stop()

	syncerConfig := fmc.Config{
		SyncInterval:  cfg.SyncInterval,
		KeyTTL:        cfg.KeyTTL,
		ResourceKinds: cfg.ResourceKinds,
	}

	readerFactory := func(_ context.Context, clusterID string) (client.Reader, error) {
		return mcpclient.NewFromSession(mcpClient.Session(), clusterID), nil
	}
	syncer := fmc.NewSyncerWithReaderFactory(clusterRegistry, readerFactory, writer, syncerConfig, logger, metrics)

	var ready atomic.Bool

	// API server: scope check + cluster listing (ADR-068)
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
		Addr:              cfg.APIAddr,
		Handler:           apiMux,
		ReadHeaderTimeout: 5 * time.Second,
	}

	// Metrics server: Prometheus + health probes (operational)
	metricsMux := http.NewServeMux()
	metricsMux.Handle("/metrics", promhttp.HandlerFor(reg, promhttp.HandlerOpts{}))

	metricsServer := &http.Server{
		Addr:              cfg.MetricsAddr,
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

type config struct {
	MCPGatewayEndpoint string
	ValkeyAddr         string
	Namespace          string
	SyncInterval       time.Duration
	KeyTTL             time.Duration
	APIAddr            string
	MetricsAddr        string
	ResourceKinds      []string
	OAuth2Enabled      bool
	OAuth2TokenURL     string
	OAuth2SecretPath   string
}

func loadConfig() config {
	cfg := config{
		MCPGatewayEndpoint: envOrDefault("FMC_MCP_GATEWAY_ENDPOINT", "http://envoy-ai-gateway:8080/mcp"),
		ValkeyAddr:         envOrDefault("FMC_VALKEY_ADDR", "valkey:6379"),
		Namespace:          envOrDefault("FMC_NAMESPACE", "kubernaut-system"),
		SyncInterval:       parseDuration("FMC_SYNC_INTERVAL", 30*time.Second),
		KeyTTL:             parseDuration("FMC_KEY_TTL", 45*time.Second),
		APIAddr:            envOrDefault("FMC_API_ADDR", ":8080"),
		MetricsAddr:        envOrDefault("FMC_METRICS_ADDR", ":8081"),
		ResourceKinds:      []string{"Deployment", "StatefulSet", "DaemonSet", "Pod", "Service", "Node"},
		OAuth2Enabled:      os.Getenv("FMC_OAUTH2_ENABLED") == "true",
		OAuth2TokenURL:     envOrDefault("FMC_OAUTH2_TOKEN_URL", ""),
		OAuth2SecretPath:   envOrDefault("FMC_OAUTH2_SECRET_PATH", "/etc/fmc/fleet-oauth2"),
	}
	return cfg
}

func envOrDefault(key, defaultVal string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return defaultVal
}

func parseDuration(key string, defaultVal time.Duration) time.Duration {
	v := os.Getenv(key)
	if v == "" {
		return defaultVal
	}
	d, err := time.ParseDuration(v)
	if err != nil {
		fmt.Fprintf(os.Stderr, "WARNING: invalid duration for %s=%q, using default %v\n", key, v, defaultVal)
		return defaultVal
	}
	return d
}
