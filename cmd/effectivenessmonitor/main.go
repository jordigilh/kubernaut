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

package main

import (
	"context"
	"flag"
	"os"
	"time"

	"github.com/go-logr/zapr"
	zaplog "go.uber.org/zap"
	"k8s.io/apimachinery/pkg/runtime"
	utilruntime "k8s.io/apimachinery/pkg/util/runtime"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/healthz"
	"sigs.k8s.io/controller-runtime/pkg/log/zap"
	metricsserver "sigs.k8s.io/controller-runtime/pkg/metrics/server"

	eav1 "github.com/jordigilh/kubernaut/api/effectivenessassessment/v1alpha1"
	remediationv1alpha1 "github.com/jordigilh/kubernaut/api/remediation/v1alpha1"
	config "github.com/jordigilh/kubernaut/internal/config/effectivenessmonitor"
	controller "github.com/jordigilh/kubernaut/internal/controller/effectivenessmonitor"
	"github.com/jordigilh/kubernaut/pkg/audit"
	emaudit "github.com/jordigilh/kubernaut/pkg/effectivenessmonitor/audit"
	emclient "github.com/jordigilh/kubernaut/pkg/effectivenessmonitor/client"
	emmetrics "github.com/jordigilh/kubernaut/pkg/effectivenessmonitor/metrics"
	//+kubebuilder:scaffold:imports
)

var (
	scheme   = runtime.NewScheme()
	setupLog = ctrl.Log.WithName("setup")
)

func init() {
	utilruntime.Must(clientgoscheme.AddToScheme(scheme))
	utilruntime.Must(eav1.AddToScheme(scheme))
	utilruntime.Must(remediationv1alpha1.AddToScheme(scheme))
	//+kubebuilder:scaffold:scheme
}

func main() {
	// ========================================
	// ADR-030: Configuration via YAML file
	// Single --config flag; all functional config in YAML ConfigMap
	// ========================================
	var configPath string
	flag.StringVar(&configPath, "config", config.DefaultConfigPath, "Path to YAML configuration file (optional, falls back to defaults)")

	opts := zap.Options{
		Development: true,
	}
	opts.BindFlags(flag.CommandLine)
	flag.Parse()

	ctrl.SetLogger(zap.New(zap.UseFlagOptions(&opts)))

	// ========================================
	// CONFIGURATION LOADING (ADR-030)
	// ========================================
	cfg, err := config.LoadFromFile(configPath)
	if err != nil {
		setupLog.Error(err, "Failed to load configuration from file, using defaults",
			"configPath", configPath)
		cfg = config.DefaultConfig()
	} else if configPath != "" {
		setupLog.Info("Configuration loaded successfully", "configPath", configPath)
	} else {
		setupLog.Info("No config file specified, using defaults")
	}

	// Validate configuration (ADR-030)
	if err := cfg.Validate(); err != nil {
		setupLog.Error(err, "Configuration validation failed")
		os.Exit(1)
	}

	// ========================================
	// FAIL-FAST STARTUP: Validate External Dependencies
	// Per ADR-EM-001: EM must verify Prometheus/AlertManager connectivity at startup
	// Implemented below: promClient.Ready() and amClient.Ready() with os.Exit(1) on failure
	// ========================================

	mgr, err := ctrl.NewManager(ctrl.GetConfigOrDie(), ctrl.Options{
		Scheme: scheme,
		Metrics: metricsserver.Options{
			BindAddress: cfg.Controller.MetricsAddr,
		},
		HealthProbeBindAddress: cfg.Controller.HealthProbeAddr,
		LeaderElection:         cfg.Controller.LeaderElection,
		LeaderElectionID:       cfg.Controller.LeaderElectionID,
	})
	if err != nil {
		setupLog.Error(err, "unable to start manager")
		os.Exit(1)
	}

	// ========================================
	// AUDIT STORE INITIALIZATION (DD-AUDIT-003, DD-API-001)
	// ========================================
	dataStorageClient, err := audit.NewOpenAPIClientAdapter(cfg.DataStorage.URL, cfg.DataStorage.Timeout)
	if err != nil {
		setupLog.Error(err, "Failed to create Data Storage client",
			"url", cfg.DataStorage.URL,
			"timeout", cfg.DataStorage.Timeout)
		os.Exit(1)
	}
	setupLog.Info("Data Storage client initialized",
		"url", cfg.DataStorage.URL,
		"timeout", cfg.DataStorage.Timeout)

	auditConfig := audit.Config{
		BufferSize:    cfg.DataStorage.Buffer.BufferSize,
		BatchSize:     cfg.DataStorage.Buffer.BatchSize,
		FlushInterval: cfg.DataStorage.Buffer.FlushInterval,
		MaxRetries:    cfg.DataStorage.Buffer.MaxRetries,
	}

	zapLogger, err := zaplog.NewProduction()
	if err != nil {
		setupLog.Error(err, "Failed to create zap logger for audit store")
		os.Exit(1)
	}
	auditLogger := zapr.NewLogger(zapLogger.Named("audit"))

	auditStore, err := audit.NewBufferedStore(dataStorageClient, auditConfig, "effectiveness-monitor", auditLogger)
	if err != nil {
		setupLog.Error(err, "Failed to create audit store")
		os.Exit(1)
	}

	setupLog.Info("Audit store initialized",
		"dataStorageURL", cfg.DataStorage.URL,
		"bufferSize", auditConfig.BufferSize,
		"batchSize", auditConfig.BatchSize,
		"flushInterval", auditConfig.FlushInterval,
	)

	// ========================================
	// DD-METRICS-001: Initialize Metrics
	// ========================================
	setupLog.Info("Initializing effectivenessmonitor metrics (DD-METRICS-001)")
	emMetrics := emmetrics.NewMetrics()
	setupLog.Info("EffectivenessMonitor metrics initialized and registered")

	// Log configuration
	setupLog.Info("EffectivenessMonitor controller configuration",
		"metricsAddr", cfg.Controller.MetricsAddr,
		"healthProbeAddr", cfg.Controller.HealthProbeAddr,
		"stabilizationWindow", cfg.Assessment.StabilizationWindow,
		"validityWindow", cfg.Assessment.ValidityWindow,
		"prometheusEnabled", cfg.External.PrometheusEnabled,
		"alertManagerEnabled", cfg.External.AlertManagerEnabled,
		"dataStorageURL", cfg.DataStorage.URL,
	)

	// ========================================
	// EXTERNAL CLIENT INITIALIZATION (BR-EM-002, BR-EM-003)
	// ========================================
	var promClient emclient.PrometheusQuerier
	var amClient emclient.AlertManagerClient

	if cfg.External.PrometheusEnabled {
		promClient = emclient.NewPrometheusHTTPClient(
			cfg.External.PrometheusURL,
			cfg.External.ConnectionTimeout,
		)
		setupLog.Info("Prometheus HTTP client initialized",
			"url", cfg.External.PrometheusURL,
			"timeout", cfg.External.ConnectionTimeout,
		)
	} else {
		setupLog.Info("Prometheus disabled in configuration, metric comparison will be skipped")
	}

	if cfg.External.AlertManagerEnabled {
		amClient = emclient.NewAlertManagerHTTPClient(
			cfg.External.AlertManagerURL,
			cfg.External.ConnectionTimeout,
		)
		setupLog.Info("AlertManager HTTP client initialized",
			"url", cfg.External.AlertManagerURL,
			"timeout", cfg.External.ConnectionTimeout,
		)
	} else {
		setupLog.Info("AlertManager disabled in configuration, alert resolution check will be skipped")
	}

	// ========================================
	// FAIL-FAST READINESS CHECK (E2E-EM-FF-001)
	// ========================================
	// Verify connectivity to enabled external services at startup.
	// If an enabled service is unreachable, the controller exits immediately
	// rather than running in a degraded state with silent failures.
	startupCtx, startupCancel := context.WithTimeout(context.Background(), cfg.External.ConnectionTimeout+5*time.Second)
	defer startupCancel()

	if cfg.External.PrometheusEnabled && promClient != nil {
		setupLog.Info("Fail-fast: verifying Prometheus connectivity...")
		if err := promClient.Ready(startupCtx); err != nil {
			setupLog.Error(err, "FATAL: Prometheus is enabled but unreachable at startup",
				"url", cfg.External.PrometheusURL,
			)
			os.Exit(1)
		}
		setupLog.Info("Prometheus connectivity verified")
	}

	if cfg.External.AlertManagerEnabled && amClient != nil {
		setupLog.Info("Fail-fast: verifying AlertManager connectivity...")
		if err := amClient.Ready(startupCtx); err != nil {
			setupLog.Error(err, "FATAL: AlertManager is enabled but unreachable at startup",
				"url", cfg.External.AlertManagerURL,
			)
			os.Exit(1)
		}
		setupLog.Info("AlertManager connectivity verified")
	}

	// ========================================
	// AUDIT MANAGER INITIALIZATION (DD-AUDIT-003, Pattern 2)
	// ========================================
	auditManager := emaudit.NewManager(auditStore, ctrl.Log.WithName("em-audit"))
	setupLog.Info("EM audit manager initialized (DD-AUDIT-003, Pattern 2)")

	// ========================================
	// DS QUERIER INITIALIZATION (DD-EM-002: pre-remediation hash lookup)
	// ========================================
	var dsQuerier emclient.DataStorageQuerier
	dsQuerier = emclient.NewDataStorageHTTPQuerier(cfg.DataStorage.URL)
	setupLog.Info("DataStorage querier initialized for pre-remediation hash lookup",
		"url", cfg.DataStorage.URL)

	// ========================================
	// CONTROLLER SETUP
	// ========================================
	emReconciler := controller.NewReconciler(
		mgr.GetClient(),
		mgr.GetScheme(),
		mgr.GetEventRecorderFor("effectivenessmonitor-controller"),
		emMetrics,
		promClient,
		amClient,
		auditManager,
		dsQuerier,
		func() controller.ReconcilerConfig {
			c := controller.DefaultReconcilerConfig()
			c.ValidityWindow = cfg.Assessment.ValidityWindow
			c.PrometheusEnabled = cfg.External.PrometheusEnabled
			c.AlertManagerEnabled = cfg.External.AlertManagerEnabled
			return c
		}(),
	)
	emReconciler.SetRESTMapper(mgr.GetRESTMapper())
	if err = emReconciler.SetupWithManager(mgr); err != nil {
		setupLog.Error(err, "unable to create controller", "controller", "EffectivenessMonitor")
		os.Exit(1)
	}
	//+kubebuilder:scaffold:builder

	if err := mgr.AddHealthzCheck("healthz", healthz.Ping); err != nil {
		setupLog.Error(err, "unable to set up health check")
		os.Exit(1)
	}
	if err := mgr.AddReadyzCheck("readyz", healthz.Ping); err != nil {
		setupLog.Error(err, "unable to set up ready check")
		os.Exit(1)
	}

	setupLog.Info("starting manager")

	ctx := ctrl.SetupSignalHandler()

	if err := mgr.Start(ctx); err != nil {
		setupLog.Error(err, "problem running manager")
		os.Exit(1)
	}

	// ========================================
	// Graceful Shutdown: Flush Audit Events (DD-007)
	// ========================================
	setupLog.Info("Shutting down effectiveness monitor, flushing remaining audit events")
	if err := auditStore.Close(); err != nil {
		setupLog.Error(err, "Failed to close audit store gracefully")
		os.Exit(1)
	}
	setupLog.Info("Audit store closed successfully, all events flushed")
}
