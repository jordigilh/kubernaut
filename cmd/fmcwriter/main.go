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

// FMC Writer: Fleet Metadata Cache writer service.
// Polls remote clusters via MCP Gateway for resources labeled kubernaut.ai/managed=true
// and writes their metadata to Valkey for low-latency federated scope checking.
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

	"github.com/jordigilh/kubernaut/pkg/fleet/fmcwriter"
	"github.com/jordigilh/kubernaut/pkg/fleet/mcpclient"
	"github.com/jordigilh/kubernaut/pkg/fleet/registry"
)

func main() {
	zapLogger, _ := zap.NewProduction()
	defer func() { _ = zapLogger.Sync() }()
	logger := zapr.NewLogger(zapLogger)

	cfg := loadConfig()
	logger.Info("FMC Writer starting",
		"syncInterval", cfg.SyncInterval,
		"valkeyAddr", cfg.ValkeyAddr,
		"mcpEndpoint", cfg.MCPGatewayEndpoint,
		"metricsAddr", cfg.MetricsAddr)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGTERM, syscall.SIGINT)

	reg := prometheus.NewRegistry()
	metrics := fmcwriter.NewMetrics(reg)

	mcpClient, err := mcpclient.New(ctx, cfg.MCPGatewayEndpoint)
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

	writer := fmcwriter.NewValkeyWriter(cfg.ValkeyAddr)
	defer func() { _ = writer.Close() }()

	clusterRegistry := registry.NewCRDWatcher(dynClient, registry.CRDWatcherConfig{
		Namespace: cfg.Namespace,
	}, registry.NewMetricsWithRegistry(reg), logger)
	if err := clusterRegistry.Start(ctx); err != nil {
		logger.Error(err, "Failed to start cluster registry")
		os.Exit(1)
	}
	defer clusterRegistry.Stop()

	syncerConfig := fmcwriter.Config{
		SyncInterval:  cfg.SyncInterval,
		KeyTTL:        cfg.KeyTTL,
		ResourceKinds: cfg.ResourceKinds,
	}

	syncer := fmcwriter.NewSyncer(clusterRegistry, mcpClient, writer, syncerConfig, logger, metrics)

	var ready atomic.Bool

	mux := http.NewServeMux()
	mux.HandleFunc("/healthz", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("ok"))
	})
	mux.HandleFunc("/readyz", func(w http.ResponseWriter, _ *http.Request) {
		if ready.Load() {
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte("ok"))
		} else {
			w.WriteHeader(http.StatusServiceUnavailable)
			_, _ = w.Write([]byte("not ready"))
		}
	})
	mux.Handle("/metrics", promhttp.HandlerFor(reg, promhttp.HandlerOpts{}))

	server := &http.Server{
		Addr:    cfg.MetricsAddr,
		Handler: mux,
	}

	go func() {
		if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			logger.Error(err, "Metrics server failed")
		}
	}()

	go func() {
		if err := syncer.Run(ctx); err != nil {
			logger.Error(err, "Syncer stopped with error")
			cancel()
		}
	}()

	ready.Store(true)
	logger.Info("FMC Writer ready")

	<-sigCh
	logger.Info("Received shutdown signal")
	cancel()

	shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer shutdownCancel()
	_ = server.Shutdown(shutdownCtx)
}

type config struct {
	MCPGatewayEndpoint string
	ValkeyAddr         string
	Namespace          string
	SyncInterval       time.Duration
	KeyTTL             time.Duration
	MetricsAddr        string
	ResourceKinds      []string
}

func loadConfig() config {
	cfg := config{
		MCPGatewayEndpoint: envOrDefault("FMC_MCP_GATEWAY_ENDPOINT", "http://mcp-gateway:8080/mcp"),
		ValkeyAddr:         envOrDefault("FMC_VALKEY_ADDR", "valkey:6379"),
		Namespace:          envOrDefault("FMC_NAMESPACE", "kubernaut-system"),
		SyncInterval:       parseDuration("FMC_SYNC_INTERVAL", 30*time.Second),
		KeyTTL:             parseDuration("FMC_KEY_TTL", 45*time.Second),
		MetricsAddr:        envOrDefault("FMC_METRICS_ADDR", ":8081"),
		ResourceKinds:      []string{"Deployment", "StatefulSet", "DaemonSet", "Pod", "Service", "Node"},
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
