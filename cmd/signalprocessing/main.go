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

// ========================================
// SIGNAL PROCESSING CONTROLLER (DD-006)
// ========================================
//
// Design Decisions:
//   - DD-006: Controller Scaffolding Strategy
//   - DD-005: Observability Standards (Metrics & Logging)
//   - DD-CRD-001: API Group Domain (.kubernaut.ai)
//   - DD-014: Binary Version Logging
//
// Business Requirements:
//   - BR-SP-001: K8s Context Enrichment
//   - BR-SP-070: Priority Assignment (Rego)
//   - BR-SP-090: Categorization Audit Trail
//
// ========================================
package main

import (
	"flag"
	"os"
	"path/filepath"

	// Standard Kubernetes imports
	"k8s.io/apimachinery/pkg/runtime"
	utilruntime "k8s.io/apimachinery/pkg/util/runtime"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/cache"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/healthz"
	"sigs.k8s.io/controller-runtime/pkg/log/zap"
	metricsserver "sigs.k8s.io/controller-runtime/pkg/metrics/server"

	// Kubernaut API imports
	remediationv1alpha1 "github.com/jordigilh/kubernaut/api/remediation/v1alpha1"
	signalprocessingv1alpha1 "github.com/jordigilh/kubernaut/api/signalprocessing/v1alpha1"
	"github.com/jordigilh/kubernaut/internal/version"
	scope "github.com/jordigilh/kubernaut/pkg/shared/scope"
	"github.com/jordigilh/kubernaut/internal/controller/signalprocessing"
	sharedaudit "github.com/jordigilh/kubernaut/pkg/audit"
	spaudit "github.com/jordigilh/kubernaut/pkg/signalprocessing/audit"
	"github.com/jordigilh/kubernaut/pkg/signalprocessing/classifier"
	"github.com/jordigilh/kubernaut/pkg/signalprocessing/config"
	"github.com/jordigilh/kubernaut/pkg/signalprocessing/enricher"
	"github.com/jordigilh/kubernaut/pkg/signalprocessing/evaluator"
	spmetrics "github.com/jordigilh/kubernaut/pkg/signalprocessing/metrics"
	spstatus "github.com/jordigilh/kubernaut/pkg/signalprocessing/status"
	sharedtls "github.com/jordigilh/kubernaut/pkg/shared/tls"
)

var (
	scheme   = runtime.NewScheme()
	setupLog = ctrl.Log.WithName("setup")
)

func init() {
	utilruntime.Must(clientgoscheme.AddToScheme(scheme))
	utilruntime.Must(signalprocessingv1alpha1.AddToScheme(scheme))
	utilruntime.Must(remediationv1alpha1.AddToScheme(scheme))
}

func main() {
	// ========================================
	// ADR-030: Configuration via YAML file
	// Single --config flag; all functional config in YAML ConfigMap
	// ========================================
	var configFile string
	flag.StringVar(&configFile, "config", config.DefaultConfigPath, "Path to configuration file")

	opts := zap.Options{
		Development: true,
	}
	opts.BindFlags(flag.CommandLine)
	flag.Parse()

	ctrl.SetLogger(zap.New(zap.UseFlagOptions(&opts)))

	setupLog.Info("Starting SignalProcessing Controller",
		"version", version.Version,
		"gitCommit", version.GitCommit,
		"buildDate", version.BuildDate,
	)

	// ========================================
	// CONFIGURATION LOADING (ADR-030)
	// ========================================
	cfg, err := config.LoadFromFile(configFile)
	if err != nil {
		setupLog.Error(err, "Failed to load configuration from file, using defaults",
			"configPath", configFile)
		cfg = config.DefaultConfig()
	} else {
		setupLog.Info("Configuration loaded successfully", "configPath", configFile)
	}

	if err := cfg.Validate(); err != nil {
		setupLog.Error(err, "invalid configuration")
		os.Exit(1)
	}

	// ADR-057: Discover controller namespace for CRD watch restriction
	controllerNS, err := scope.GetControllerNamespace()
	if err != nil {
		setupLog.Error(err, "unable to determine controller namespace")
		os.Exit(1)
	}

	setupLog.Info("SignalProcessing controller configuration",
		"metricsAddr", cfg.Controller.MetricsAddr,
		"healthProbeAddr", cfg.Controller.HealthProbeAddr,
		"enrichmentTimeout", cfg.Enrichment.Timeout,
		"dataStorageURL", cfg.DataStorage.URL,
	)

	// Setup controller manager
	// ADR-030: Controller settings from YAML config (not hardcoded defaults)
	mgr, err := ctrl.NewManager(ctrl.GetConfigOrDie(), ctrl.Options{
		Scheme: scheme,
		Cache: cache.Options{
			ByObject: map[client.Object]cache.ByObject{
				&signalprocessingv1alpha1.SignalProcessing{}: {
					Namespaces: map[string]cache.Config{
						controllerNS: {},
					},
				},
			},
		},
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
	// AUDIT CLIENT SETUP (MANDATORY per ADR-032)
	// ========================================
	// ADR-032: Audit is MANDATORY - controller will crash if not configured
	// ADR-038: Fire-and-forget pattern via BufferedStore
	// ADR-030: DataStorage URL from YAML config (not env var)
	setupLog.Info("configuring audit client", "dataStorageURL", cfg.DataStorage.URL)

	// DD-API-001: Use OpenAPI client adapter (type-safe, contract-validated)
	dsClient, err := sharedaudit.NewOpenAPIClientAdapter(cfg.DataStorage.URL, cfg.DataStorage.Timeout)
	if err != nil {
		setupLog.Error(err, "FATAL: failed to create Data Storage client")
		os.Exit(1)
	}

	// ADR-030: Audit buffer config from YAML (not RecommendedConfig)
	auditConfig := sharedaudit.Config{
		BufferSize:    cfg.DataStorage.Buffer.BufferSize,
		BatchSize:     cfg.DataStorage.Buffer.BatchSize,
		FlushInterval: cfg.DataStorage.Buffer.FlushInterval,
		MaxRetries:    cfg.DataStorage.Buffer.MaxRetries,
	}

	auditStore, err := sharedaudit.NewBufferedStore(
		dsClient,
		auditConfig,
		"signalprocessing",
		ctrl.Log.WithName("audit"),
	)
	if err != nil {
		setupLog.Error(err, "FATAL: failed to create audit store - audit is MANDATORY per ADR-032")
		os.Exit(1)
	}

	setupLog.Info("Audit store initialized",
		"dataStorageURL", cfg.DataStorage.URL,
		"bufferSize", auditConfig.BufferSize,
		"batchSize", auditConfig.BatchSize,
		"flushInterval", auditConfig.FlushInterval,
	)

	// Create service-specific audit client (BR-SP-090)
	auditClient := spaudit.NewAuditClient(auditStore, ctrl.Log.WithName("audit"))
	setupLog.Info("audit client configured successfully")

	// ========================================
	// ADR-060: UNIFIED REGO EVALUATOR SETUP
	// ========================================
	// Replaces 5 separate classifiers (environment, priority, severity, business, customlabels)
	// with a single evaluator backed by one policy.rego file.
	//
	// Issue #419: Policy path is now constructed from cfg.Classifier instead of hardcoded.
	// The Helm chart mounts the ConfigMap at /etc/signalprocessing/policies/, and the
	// regoConfigMapKey determines the filename within that mount.

	ctx := ctrl.SetupSignalHandler()

	policyPath := filepath.Join("/etc/signalprocessing/policies", cfg.Classifier.RegoConfigMapKey)
	policyEvaluator := evaluator.New(
		policyPath,
		ctrl.Log.WithName("evaluator"),
	)
	if err := policyEvaluator.StartHotReload(ctx); err != nil {
		setupLog.Error(err, "FATAL: unified policy is mandatory but failed to load",
			"policyPath", policyPath,
			"configMapName", cfg.Classifier.RegoConfigMapName,
			"configMapKey", cfg.Classifier.RegoConfigMapKey,
			"hint", "Ensure the policy ConfigMap is mounted at /etc/signalprocessing/policies/")
		os.Exit(1)
	}
	defer policyEvaluator.Stop()
	setupLog.Info("Unified policy evaluator started",
		"policyPath", policyPath,
		"policyHash", policyEvaluator.GetPolicyHash())

	// ========================================
	// SIGNAL MODE CLASSIFIER (OPTIONAL)
	// ========================================
	// BR-SP-106: Proactive Signal Mode Classification
	// ADR-054: Uses YAML config (not Rego) -- simple key-value lookup
	signalModeClassifier := classifier.NewSignalModeClassifier(
		ctrl.Log.WithName("classifier.signalmode"),
	)

	signalModeConfigPath := "/etc/signalprocessing/proactive-signal-mappings.yaml"
	if envPath := os.Getenv("SIGNAL_MODE_CONFIG_PATH"); envPath != "" {
		signalModeConfigPath = envPath
	}
	if err := signalModeClassifier.LoadConfig(signalModeConfigPath); err != nil {
		setupLog.Info("signal mode config not found, all signals will default to reactive mode",
			"configPath", signalModeConfigPath,
			"error", err.Error())
	} else {
		setupLog.Info("signal mode classifier configured successfully",
			"configPath", signalModeConfigPath)
	}

	// ========================================
	// ENRICHMENT COMPONENTS SETUP
	// ========================================
	// BR-SP-001: Kubernetes context enrichment
	// BR-SP-001: Metrics for observability (DD-005)
	// Per AIAnalysis pattern: Use global ctrlmetrics.Registry for production
	spMetrics := spmetrics.NewMetrics() // Uses ctrlmetrics.Registry (global)
	setupLog.Info("signalprocessing metrics configured")

	// BR-SP-001: K8s context enricher with caching, timeout, metrics, degraded mode
	// ADR-030: Enrichment timeout and cache TTL from YAML config (not hardcoded)
	k8sEnricher := enricher.NewK8sEnricher(
		mgr.GetClient(),
		mgr.GetAPIReader(),
		ctrl.Log.WithName("enricher"),
		spMetrics,
		cfg.Enrichment.Timeout,
		cfg.Enrichment.CacheTTL,
	)
	setupLog.Info("k8s enricher configured",
		"enrichmentTimeout", cfg.Enrichment.Timeout,
		"cacheTTL", cfg.Enrichment.CacheTTL)

	// ========================================
	// DD-PERF-001: Atomic Status Updates
	// Status Manager for reducing K8s API calls by 66-75%
	// Consolidates multiple status field updates into single atomic operations
	// SP-CACHE-001: Pass APIReader to bypass cache for fresh refetches
	// ========================================
	statusManager := spstatus.NewManager(mgr.GetClient(), mgr.GetAPIReader())
	setupLog.Info("SignalProcessing status manager initialized (DD-PERF-001 + SP-CACHE-001)")

	// ========================================
	// Phase 3 Refactoring: Audit Manager (2026-01-22)
	// Wraps AuditClient with ADR-032 enforcement
	// Follows RO/WE/AIA/NT pattern for consistency
	// ========================================
	auditManager := spaudit.NewManager(auditClient)
	setupLog.Info("SignalProcessing audit manager initialized (Phase 3 refactoring)")

	// Setup reconciler with ALL required components
	if err = (&signalprocessing.SignalProcessingReconciler{
		Client:               mgr.GetClient(),
		Scheme:               mgr.GetScheme(),
		AuditManager:         auditManager,
		Metrics:              spMetrics,
		Recorder:             mgr.GetEventRecorderFor("signalprocessing-controller"),
		StatusManager:        statusManager,
		PolicyEvaluator:      policyEvaluator,        // ADR-060: Unified evaluator
		SignalModeClassifier: signalModeClassifier,   // BR-SP-106: Proactive signal mode (ADR-054)
		K8sEnricher:          k8sEnricher,
	}).SetupWithManager(mgr); err != nil {
		setupLog.Error(err, "unable to create controller", "controller", "SignalProcessing")
		os.Exit(1)
	}

	// Setup health checks
	if err := mgr.AddHealthzCheck("healthz", healthz.Ping); err != nil {
		setupLog.Error(err, "unable to set up health check")
		os.Exit(1)
	}
	if err := mgr.AddReadyzCheck("readyz", healthz.Ping); err != nil {
		setupLog.Error(err, "unable to set up ready check")
		os.Exit(1)
	}

	// Issue #748: Load OCP TLS security profile from config before any TLS setup
	if err := sharedtls.SetDefaultSecurityProfileFromConfig(cfg.TLSProfile); err != nil {
		setupLog.Error(err, "Invalid TLS security profile in config, using default TLS 1.2")
	} else if cfg.TLSProfile != "" {
		setupLog.Info("TLS security profile active", "profile", cfg.TLSProfile)
	}

	// Issue #756: Start CA file watcher for client-side TLS hot-reload
	caWatcher, caWatchErr := sharedtls.StartCAFileWatcher(ctx, setupLog)
	if caWatchErr != nil {
		setupLog.Error(caWatchErr, "Failed to start CA file watcher")
		os.Exit(1)
	}
	if caWatcher != nil {
		defer caWatcher.Stop()
	}

	setupLog.Info("starting manager")
	if err := mgr.Start(ctx); err != nil {
		setupLog.Error(err, "problem running manager")
		os.Exit(1)
	}

	// ========================================
	// Graceful Shutdown: Flush Audit Events (DD-007)
	// ADR-032 §2: No Audit Loss - MUST flush pending events
	// ========================================
	setupLog.Info("Shutting down signalprocessing controller, flushing remaining audit events")
	if err := auditStore.Close(); err != nil {
		setupLog.Error(err, "Failed to close audit store gracefully")
		os.Exit(1)
	}
	setupLog.Info("Audit store closed successfully, all events flushed")
}
