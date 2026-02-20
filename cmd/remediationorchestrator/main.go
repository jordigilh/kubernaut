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

	aianalysisv1 "github.com/jordigilh/kubernaut/api/aianalysis/v1alpha1"
	eav1 "github.com/jordigilh/kubernaut/api/effectivenessassessment/v1alpha1"
	notificationv1 "github.com/jordigilh/kubernaut/api/notification/v1alpha1"
	remediationv1alpha1 "github.com/jordigilh/kubernaut/api/remediation/v1alpha1"
	signalprocessingv1 "github.com/jordigilh/kubernaut/api/signalprocessing/v1alpha1"
	workflowexecutionv1 "github.com/jordigilh/kubernaut/api/workflowexecution/v1alpha1"
	config "github.com/jordigilh/kubernaut/internal/config/remediationorchestrator"
	controller "github.com/jordigilh/kubernaut/internal/controller/remediationorchestrator"
	"github.com/jordigilh/kubernaut/pkg/audit"
	"github.com/jordigilh/kubernaut/pkg/remediationorchestrator/creator"
	rometrics "github.com/jordigilh/kubernaut/pkg/remediationorchestrator/metrics"
	//+kubebuilder:scaffold:imports
)

var (
	scheme   = runtime.NewScheme()
	setupLog = ctrl.Log.WithName("setup")
)

func init() {
	utilruntime.Must(clientgoscheme.AddToScheme(scheme))
	utilruntime.Must(remediationv1alpha1.AddToScheme(scheme))
	utilruntime.Must(signalprocessingv1.AddToScheme(scheme))
	utilruntime.Must(aianalysisv1.AddToScheme(scheme))
	utilruntime.Must(workflowexecutionv1.AddToScheme(scheme))
	utilruntime.Must(notificationv1.AddToScheme(scheme))
	utilruntime.Must(eav1.AddToScheme(scheme)) // ADR-EM-001: EA CRD scheme for EA creation on terminal phases
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
	// DD-API-001: Use OpenAPI client adapter (type-safe, contract-validated)
	// ADR-030: Use DataStorage URL from YAML config (not CLI flag or env var)
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

	// Create buffered audit store (fire-and-forget pattern, ADR-038)
	// ADR-030: Use buffer config from YAML (not hardcoded RecommendedConfig)
	auditConfig := audit.Config{
		BufferSize:    cfg.DataStorage.Buffer.BufferSize,
		BatchSize:     cfg.DataStorage.Buffer.BatchSize,
		FlushInterval: cfg.DataStorage.Buffer.FlushInterval,
		MaxRetries:    cfg.DataStorage.Buffer.MaxRetries,
	}

	// Create zap logger for audit store, then convert to logr.Logger via zapr adapter
	// DD-005 v2.0: pkg/audit uses logr.Logger for unified logging interface
	zapLogger, err := zaplog.NewProduction()
	if err != nil {
		setupLog.Error(err, "Failed to create zap logger for audit store")
		os.Exit(1)
	}
	auditLogger := zapr.NewLogger(zapLogger.Named("audit"))

	auditStore, err := audit.NewBufferedStore(dataStorageClient, auditConfig, "remediation-orchestrator", auditLogger)
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

	// Log configuration
	setupLog.Info("RemediationOrchestrator controller configuration",
		"metricsAddr", cfg.Controller.MetricsAddr,
		"healthProbeAddr", cfg.Controller.HealthProbeAddr,
		"globalTimeout", cfg.Timeouts.Global,
		"processingTimeout", cfg.Timeouts.Processing,
		"analyzingTimeout", cfg.Timeouts.Analyzing,
		"executingTimeout", cfg.Timeouts.Executing,
		"dataStorageURL", cfg.DataStorage.URL,
	)

	// ========================================
	// DD-METRICS-001: Initialize Metrics
	// Per V1.0 Maturity Requirements: Metrics wired to controller via dependency injection
	// ========================================
	setupLog.Info("Initializing remediationorchestrator metrics (DD-METRICS-001)")
	roMetrics := rometrics.NewMetrics()
	setupLog.Info("RemediationOrchestrator metrics initialized and registered")

	// ADR-EM-001: Create EA creator for EffectivenessAssessment CRD creation on terminal phases
	eaCreator := creator.NewEffectivenessAssessmentCreator(
		mgr.GetClient(),
		mgr.GetScheme(),
		roMetrics,
		mgr.GetEventRecorderFor("remediationorchestrator-controller"),
		cfg.EA.StabilizationWindow,
	)
	setupLog.Info("EffectivenessAssessment creator initialized (ADR-EM-001)",
		"stabilizationWindow", cfg.EA.StabilizationWindow)

	// Setup RemediationOrchestrator controller with audit store and comprehensive timeout config
	// ADR-030: Timeouts from YAML config (not CLI flags)
	roReconciler := controller.NewReconciler(
		mgr.GetClient(),
		mgr.GetAPIReader(), // DD-STATUS-001: API reader for cache-bypassed status refetches
		mgr.GetScheme(),
		auditStore,
		mgr.GetEventRecorderFor("remediationorchestrator-controller"), // V1.0 P1: EventRecorder for debugging
		roMetrics, // V1.0 P0: Metrics for observability (DD-METRICS-001)
		controller.TimeoutConfig{
			Global:     cfg.Timeouts.Global,
			Processing: cfg.Timeouts.Processing,
			Analyzing:  cfg.Timeouts.Analyzing,
			Executing:  cfg.Timeouts.Executing,
		},
		nil,       // Use default routing engine (production)
		eaCreator, // ADR-EM-001: EA creation on terminal phases
	)
	// DD-EM-002: Set REST mapper for pre-remediation hash Kind resolution
	roReconciler.SetRESTMapper(mgr.GetRESTMapper())
	if err = roReconciler.SetupWithManager(mgr); err != nil {
		setupLog.Error(err, "unable to create controller", "controller", "RemediationOrchestrator")
		os.Exit(1)
	}

	// REFACTOR: Setup RemediationApprovalRequest audit controller (BR-AUDIT-006)
	// This controller watches RAR for status.Decision changes and emits audit events
	// Enhanced with metrics for SOC 2 compliance tracking
	setupLog.Info("Setting up RemediationApprovalRequest audit controller (BR-AUDIT-006)")
	if err = controller.NewRARReconciler(
		mgr.GetClient(),
		mgr.GetScheme(),
		auditStore,
		roMetrics, // REFACTOR: Pass metrics for business value tracking
	).SetupWithManager(mgr); err != nil {
		setupLog.Error(err, "unable to create controller", "controller", "RemediationApprovalRequestAudit")
		os.Exit(1)
	}
	setupLog.Info("RemediationApprovalRequest audit controller ready with metrics")
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

	// Setup signal handler for graceful shutdown
	ctx := ctrl.SetupSignalHandler()

	if err := mgr.Start(ctx); err != nil {
		setupLog.Error(err, "problem running manager")
		os.Exit(1)
	}

	// ========================================
	// Graceful Shutdown: Flush Audit Events (DD-007)
	// BR-STORAGE-001: Complete audit trail with no data loss
	// ========================================
	setupLog.Info("Shutting down remediation orchestrator, flushing remaining audit events")
	if err := auditStore.Close(); err != nil {
		setupLog.Error(err, "Failed to close audit store gracefully")
		os.Exit(1)
	}
	setupLog.Info("Audit store closed successfully, all events flushed")
}
