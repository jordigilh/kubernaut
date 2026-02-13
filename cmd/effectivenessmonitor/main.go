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
	"flag"
	"os"

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
	"github.com/jordigilh/kubernaut/internal/config"
	controller "github.com/jordigilh/kubernaut/internal/controller/effectivenessmonitor"
	"github.com/jordigilh/kubernaut/pkg/audit"
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
	var metricsAddr string
	var probeAddr string
	var enableLeaderElection bool
	var configPath string

	flag.StringVar(&metricsAddr, "metrics-bind-address", ":9090", "The address the metric endpoint binds to.")
	flag.StringVar(&probeAddr, "health-probe-bind-address", ":8081", "The address the probe endpoint binds to.")
	flag.BoolVar(&enableLeaderElection, "leader-elect", false,
		"Enable leader election for controller manager. "+
			"Enabling this will ensure there is only one active controller manager.")
	flag.StringVar(&configPath, "config", "", "Path to YAML configuration file (optional, falls back to defaults)")

	opts := zap.Options{
		Development: true,
	}
	opts.BindFlags(flag.CommandLine)
	flag.Parse()

	ctrl.SetLogger(zap.New(zap.UseFlagOptions(&opts)))

	// ========================================
	// CONFIGURATION LOADING (ADR-030)
	// ========================================
	cfg, err := config.LoadEMConfigFromFile(configPath)
	if err != nil {
		setupLog.Error(err, "Failed to load configuration from file, using defaults",
			"configPath", configPath)
		cfg = config.DefaultEMConfig()
	} else if configPath != "" {
		setupLog.Info("Configuration loaded successfully", "configPath", configPath)
	} else {
		setupLog.Info("No config file specified, using defaults")
	}

	// ========================================
	// FAIL-FAST STARTUP: Validate External Dependencies
	// Per ADR-EM-001: EM must verify Prometheus/AlertManager connectivity at startup
	// ========================================
	// TODO: Implement fail-fast health checks for Prometheus and AlertManager
	// This will be wired in TDD GREEN phase (BR-EM-010: Fail-fast startup)

	mgr, err := ctrl.NewManager(ctrl.GetConfigOrDie(), ctrl.Options{
		Scheme: scheme,
		Metrics: metricsserver.Options{
			BindAddress: metricsAddr,
		},
		HealthProbeBindAddress: probeAddr,
		LeaderElection:         enableLeaderElection,
		LeaderElectionID:       "effectivenessmonitor.kubernaut.ai",
	})
	if err != nil {
		setupLog.Error(err, "unable to start manager")
		os.Exit(1)
	}

	// ========================================
	// AUDIT STORE INITIALIZATION (DD-AUDIT-003, DD-API-001)
	// ========================================
	dataStorageClient, err := audit.NewOpenAPIClientAdapter(cfg.Audit.DataStorageURL, cfg.Audit.Timeout)
	if err != nil {
		setupLog.Error(err, "Failed to create Data Storage client",
			"url", cfg.Audit.DataStorageURL,
			"timeout", cfg.Audit.Timeout)
		os.Exit(1)
	}
	setupLog.Info("Data Storage client initialized",
		"url", cfg.Audit.DataStorageURL,
		"timeout", cfg.Audit.Timeout)

	auditConfig := audit.Config{
		BufferSize:    cfg.Audit.Buffer.BufferSize,
		BatchSize:     cfg.Audit.Buffer.BatchSize,
		FlushInterval: cfg.Audit.Buffer.FlushInterval,
		MaxRetries:    cfg.Audit.Buffer.MaxRetries,
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
		"dataStorageURL", cfg.Audit.DataStorageURL,
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
		"metricsAddr", metricsAddr,
		"probeAddr", probeAddr,
		"stabilizationWindow", cfg.Assessment.StabilizationWindow,
		"validityWindow", cfg.Assessment.ValidityWindow,
		"scoringThreshold", cfg.Assessment.ScoringThreshold,
		"prometheusEnabled", cfg.External.PrometheusEnabled,
		"alertManagerEnabled", cfg.External.AlertManagerEnabled,
		"dataStorageURL", cfg.Audit.DataStorageURL,
	)

	// ========================================
	// CONTROLLER SETUP
	// ========================================
	// TODO: Prometheus and AlertManager clients will be created and injected in TDD GREEN phase
	// For now, pass nil - the reconciler handles nil clients gracefully when disabled.
	if err = controller.NewReconciler(
		mgr.GetClient(),
		mgr.GetScheme(),
		mgr.GetEventRecorderFor("effectivenessmonitor-controller"),
		emMetrics,
		nil, // PrometheusQuerier - wired in TDD GREEN phase
		nil, // AlertManagerClient - wired in TDD GREEN phase
		controller.ReconcilerConfig{
			PrometheusEnabled:   cfg.External.PrometheusEnabled,
			AlertManagerEnabled: cfg.External.AlertManagerEnabled,
		},
	).SetupWithManager(mgr); err != nil {
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
