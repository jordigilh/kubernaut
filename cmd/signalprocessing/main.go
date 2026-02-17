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

	// Standard Kubernetes imports
	"k8s.io/apimachinery/pkg/runtime"
	utilruntime "k8s.io/apimachinery/pkg/util/runtime"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/healthz"
	"sigs.k8s.io/controller-runtime/pkg/log/zap"
	metricsserver "sigs.k8s.io/controller-runtime/pkg/metrics/server"

	// SignalProcessing imports
	signalprocessingv1alpha1 "github.com/jordigilh/kubernaut/api/signalprocessing/v1alpha1"
	"github.com/jordigilh/kubernaut/internal/controller/signalprocessing"
	sharedaudit "github.com/jordigilh/kubernaut/pkg/audit"
	spaudit "github.com/jordigilh/kubernaut/pkg/signalprocessing/audit"
	"github.com/jordigilh/kubernaut/pkg/signalprocessing/classifier"
	"github.com/jordigilh/kubernaut/pkg/signalprocessing/config"
	"github.com/jordigilh/kubernaut/pkg/signalprocessing/detection"
	"github.com/jordigilh/kubernaut/pkg/signalprocessing/enricher"
	spmetrics "github.com/jordigilh/kubernaut/pkg/signalprocessing/metrics"
	"github.com/jordigilh/kubernaut/pkg/signalprocessing/ownerchain"
	"github.com/jordigilh/kubernaut/pkg/signalprocessing/rego"
	spstatus "github.com/jordigilh/kubernaut/pkg/signalprocessing/status"
)

var (
	scheme   = runtime.NewScheme()
	setupLog = ctrl.Log.WithName("setup")

	// Version information (DD-014: Binary Version Logging)
	// Set via -ldflags at build time
	version   = "dev"
	gitCommit = "unknown"
	buildDate = "unknown"
)

func init() {
	utilruntime.Must(clientgoscheme.AddToScheme(scheme))
	utilruntime.Must(signalprocessingv1alpha1.AddToScheme(scheme))
}

func main() {
	// ========================================
	// ADR-030: Configuration via YAML file
	// Single --config flag; all functional config in YAML ConfigMap
	// ========================================
	var configFile string
	flag.StringVar(&configFile, "config", "/etc/signalprocessing/config.yaml", "Path to configuration file")

	opts := zap.Options{
		Development: true,
	}
	opts.BindFlags(flag.CommandLine)
	flag.Parse()

	ctrl.SetLogger(zap.New(zap.UseFlagOptions(&opts)))

	// DD-014: Log version information on startup
	setupLog.Info("starting signalprocessing controller",
		"version", version,
		"gitCommit", gitCommit,
		"buildDate", buildDate,
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
	// REGO-BASED CLASSIFIERS SETUP (OPTIONAL)
	// ========================================
	// BR-SP-051-053: Environment classification
	// BR-SP-070-072: Priority assignment
	// BR-SP-002: Business unit classification
	//
	// Option B: Fail-fast on Rego syntax errors at startup
	// - Policy file NOT FOUND -> INFO log, use fallback (policies are optional)
	// - Policy file EXISTS but INVALID -> FATAL error, exit(1) (deployment bug)
	// - Runtime evaluation errors -> fallback per BR-SP-071 (handled in controller)
	//
	// Production deployments should mount Rego policies at /etc/signalprocessing/policies/

	ctx := ctrl.SetupSignalHandler()

	// ----------------------------------------
	// ENVIRONMENT CLASSIFIER (MANDATORY)
	// ----------------------------------------
	envClassifier, err := classifier.NewEnvironmentClassifier(
		ctx,
		"/etc/signalprocessing/policies/environment.rego",
		ctrl.Log.WithName("classifier.environment"),
	)
	if err != nil {
		setupLog.Error(err, "FATAL: environment policy is mandatory but failed to load",
			"policyPath", "/etc/signalprocessing/policies/environment.rego",
			"hint", "Ensure the policy file is mounted via ConfigMap/Secret")
		os.Exit(1)
	}
	setupLog.Info("environment classifier configured successfully")
	if err := envClassifier.StartHotReload(ctx); err != nil {
		setupLog.Error(err, "FATAL: environment policy hot-reload failed")
		os.Exit(1)
	}
	setupLog.Info("environment policy hot-reload started", "policyHash", envClassifier.GetPolicyHash())

	// ========================================
	// PRIORITY ENGINE (MANDATORY)
	// ========================================
	// BR-SP-070: Priority assignment via Rego
	// BR-SP-071: DEPRECATED - fallback removed per user decision 2025-12-20
	// Rationale: Hardcoded fallback creates silent behavior mismatch when
	// operator-defined policies fail to load. Priority policy is now MANDATORY
	// like CustomLabels (BR-SP-102).
	priorityEngine, err := classifier.NewPriorityEngine(
		ctx,
		"/etc/signalprocessing/policies/priority.rego",
		ctrl.Log.WithName("classifier.priority"),
	)
	if err != nil {
		// Priority policy is MANDATORY - both missing file and syntax error are fatal
		setupLog.Error(err, "FATAL: priority policy is mandatory but failed to load",
			"policyPath", "/etc/signalprocessing/policies/priority.rego",
			"hint", "Ensure the policy file is mounted via ConfigMap/Secret",
			"deprecation", "BR-SP-071 fallback removed - operators must define priority policies")
		os.Exit(1)
	}
	setupLog.Info("priority engine configured successfully")
	// BR-SP-072: Start hot-reload for priority policy
	if err := priorityEngine.StartHotReload(ctx); err != nil {
		setupLog.Error(err, "FATAL: priority policy hot-reload failed",
			"policyPath", "/etc/signalprocessing/policies/priority.rego")
		os.Exit(1)
	}
	setupLog.Info("priority policy hot-reload started", "policyHash", priorityEngine.GetPolicyHash())

	// ----------------------------------------
	// BUSINESS CLASSIFIER (MANDATORY)
	// ----------------------------------------
	businessClassifier, err := classifier.NewBusinessClassifier(
		ctx,
		"/etc/signalprocessing/policies/business.rego",
		ctrl.Log.WithName("classifier.business"),
	)
	if err != nil {
		setupLog.Error(err, "FATAL: business policy is mandatory but failed to load",
			"policyPath", "/etc/signalprocessing/policies/business.rego",
			"hint", "Ensure the policy file is mounted via ConfigMap/Secret")
		os.Exit(1)
	}
	setupLog.Info("business classifier configured successfully")

	// ========================================
	// SEVERITY CLASSIFIER (MANDATORY)
	// ========================================
	// BR-SP-105: Severity determination via Rego policy
	// DD-SEVERITY-001: Strategy B - Policy-defined fallback (operator control)
	severityClassifier := classifier.NewSeverityClassifier(
		mgr.GetClient(),
		ctrl.Log.WithName("classifier.severity"),
	)
	severityClassifier.SetPolicyPath("/etc/signalprocessing/policies/severity.rego")

	// Load policy from file
	severityPolicyContent, err := os.ReadFile("/etc/signalprocessing/policies/severity.rego")
	if err != nil {
		setupLog.Error(err, "FATAL: severity policy is mandatory but failed to load",
			"policyPath", "/etc/signalprocessing/policies/severity.rego",
			"hint", "Ensure the policy file is mounted via ConfigMap/Secret")
		os.Exit(1)
	}
	if err := severityClassifier.LoadRegoPolicy(string(severityPolicyContent)); err != nil {
		setupLog.Error(err, "FATAL: severity policy validation failed",
			"policyPath", "/etc/signalprocessing/policies/severity.rego",
			"hint", "Check Rego policy syntax and ensure it returns critical/warning/info")
		os.Exit(1)
	}
	setupLog.Info("severity classifier configured successfully")

	// BR-SP-072: Start hot-reload for severity policy
	if err := severityClassifier.StartHotReload(ctx); err != nil {
		setupLog.Error(err, "FATAL: severity policy hot-reload failed",
			"policyPath", "/etc/signalprocessing/policies/severity.rego")
		os.Exit(1)
	}
	setupLog.Info("severity policy hot-reload started", "policyHash", severityClassifier.GetPolicyHash())

	// ========================================
	// SIGNAL MODE CLASSIFIER (OPTIONAL)
	// ========================================
	// BR-SP-106: Predictive Signal Mode Classification
	// ADR-054: Uses YAML config (not Rego) -- simple key-value lookup
	// If config file is missing, all signals default to reactive mode (backwards compatible)
	signalModeClassifier := classifier.NewSignalModeClassifier(
		ctrl.Log.WithName("classifier.signalmode"),
	)

	signalModeConfigPath := "/etc/signalprocessing/predictive-signal-mappings.yaml"
	if envPath := os.Getenv("SIGNAL_MODE_CONFIG_PATH"); envPath != "" {
		signalModeConfigPath = envPath
	}
	if err := signalModeClassifier.LoadConfig(signalModeConfigPath); err != nil {
		// Missing config is non-fatal: all signals default to reactive
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
	// BR-SP-100: Owner chain traversal
	// BR-SP-101: Detected labels auto-detection
	// BR-SP-102: CustomLabels Rego extraction

	regoEngine := rego.NewEngine(
		ctrl.Log.WithName("rego.engine"),
		"/etc/signalprocessing/policies/customlabels.rego",
	)

	// BR-SP-072: Start hot-reload for CustomLabels policy
	// BR-SP-102: Rego CustomLabels extraction is MANDATORY - fail startup if not available
	if err := regoEngine.StartHotReload(ctx); err != nil {
		setupLog.Error(err, "FATAL: CustomLabels policy file required but failed to load",
			"policyPath", "/etc/signalprocessing/policies/customlabels.rego",
			"hint", "Ensure the policy file is mounted via ConfigMap/Secret")
		os.Exit(1)
	}
	setupLog.Info("CustomLabels policy hot-reload started", "policyHash", regoEngine.GetPolicyHash())

	ownerChainBuilder := ownerchain.NewBuilder(
		mgr.GetClient(),
		ctrl.Log.WithName("ownerchain"),
	)
	setupLog.Info("owner chain builder configured")

	labelDetector := detection.NewLabelDetector(
		mgr.GetClient(),
		ctrl.Log.WithName("detection"),
	)
	setupLog.Info("label detector configured")

	// BR-SP-001: Metrics for observability (DD-005)
	// Per AIAnalysis pattern: Use global ctrlmetrics.Registry for production
	spMetrics := spmetrics.NewMetrics() // Uses ctrlmetrics.Registry (global)
	setupLog.Info("signalprocessing metrics configured")

	// BR-SP-001: K8s context enricher with caching, timeout, metrics, degraded mode
	// ADR-030: Enrichment timeout from YAML config (not hardcoded)
	k8sEnricher := enricher.NewK8sEnricher(
		mgr.GetClient(),
		ctrl.Log.WithName("enricher"),
		spMetrics,
		cfg.Enrichment.Timeout,
	)
	setupLog.Info("k8s enricher configured", "enrichmentTimeout", cfg.Enrichment.Timeout)

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
		AuditClient:          auditClient,          // Legacy - kept for backwards compatibility during migration
		AuditManager:         auditManager,          // Phase 3 refactoring (2026-01-22)
		Metrics:              spMetrics,              // DD-005: Observability
		Recorder:             mgr.GetEventRecorderFor("signalprocessing-controller"),
		StatusManager:        statusManager,          // DD-PERF-001: Atomic status updates
		EnvClassifier:        envClassifier,
		PriorityAssigner:     priorityEngine,         // PriorityEngine implements PriorityAssigner interface
		BusinessClassifier:   businessClassifier,
		SeverityClassifier:   severityClassifier,     // BR-SP-105: Severity determination (DD-SEVERITY-001)
		SignalModeClassifier: signalModeClassifier,   // BR-SP-106: Predictive signal mode (ADR-054)
		RegoEngine:           regoEngine,
		OwnerChainBuilder:    ownerChainBuilder,
		LabelDetector:        labelDetector,
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
