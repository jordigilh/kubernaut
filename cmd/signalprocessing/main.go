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
// 📋 Design Decisions:
//   - DD-006: Controller Scaffolding Strategy
//   - DD-005: Observability Standards (Metrics & Logging)
//   - DD-CRD-001: API Group Domain (.kubernaut.ai)
//   - DD-014: Binary Version Logging
//
// 🎯 Business Requirements:
//   - BR-SP-001: K8s Context Enrichment
//   - BR-SP-070: Priority Assignment (Rego)
//   - BR-SP-090: Categorization Audit Trail
//
// ========================================
package main

import (
	"flag"
	"os"
	"time"

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
	"github.com/jordigilh/kubernaut/pkg/signalprocessing/audit"
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
	var configFile string

	// Controller config from DefaultControllerConfig() - NOT from flags for safety
	ctrlCfg := config.DefaultControllerConfig()

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

	// Load configuration
	cfg, err := config.LoadFromFile(configFile)
	if err != nil {
		// In development/testing, use defaults if config file not found
		setupLog.Info("config file not found, using defaults", "path", configFile, "error", err.Error())
		cfg = &config.Config{} // Will fail validation - that's intentional
	}

	// Validate configuration (skip in development if empty)
	if cfg.Enrichment.Timeout > 0 {
		if err := cfg.Validate(); err != nil {
			setupLog.Error(err, "invalid configuration")
			os.Exit(1)
		}
	}

	setupLog.Info("configuration loaded",
		"config", configFile,
		"metrics-address", ctrlCfg.MetricsAddr,
		"probe-address", ctrlCfg.HealthProbeAddr,
		"leader-election", ctrlCfg.LeaderElection,
	)

	// Setup controller manager
	mgr, err := ctrl.NewManager(ctrl.GetConfigOrDie(), ctrl.Options{
		Scheme: scheme,
		Metrics: metricsserver.Options{
			BindAddress: ctrlCfg.MetricsAddr,
		},
		HealthProbeBindAddress: ctrlCfg.HealthProbeAddr,
		LeaderElection:         ctrlCfg.LeaderElection,
		LeaderElectionID:       ctrlCfg.LeaderElectionID,
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
	dataStorageURL := os.Getenv("DATA_STORAGE_URL")
	if dataStorageURL == "" {
		dataStorageURL = "http://datastorage-service:8080" // Default for in-cluster
	}
	setupLog.Info("configuring audit client", "dataStorageURL", dataStorageURL)

	// DD-API-001: Use OpenAPI client adapter (type-safe, contract-validated)
	dsClient, err := sharedaudit.NewOpenAPIClientAdapter(dataStorageURL, 5*time.Second)
	if err != nil {
		setupLog.Error(err, "FATAL: failed to create Data Storage client")
		os.Exit(1)
	}

	auditStore, err := sharedaudit.NewBufferedStore(
		dsClient,
		sharedaudit.RecommendedConfig("signalprocessing"), // DD-AUDIT-004: MEDIUM tier (30K buffer)
		"signalprocessing",
		ctrl.Log.WithName("audit"),
	)
	if err != nil {
		setupLog.Error(err, "FATAL: failed to create audit store - audit is MANDATORY per ADR-032")
		os.Exit(1)
	}

	// Create service-specific audit client (BR-SP-090)
	auditClient := audit.NewAuditClient(auditStore, ctrl.Log.WithName("audit"))
	setupLog.Info("audit client configured successfully")

	// ========================================
	// REGO-BASED CLASSIFIERS SETUP (OPTIONAL)
	// ========================================
	// BR-SP-051-053: Environment classification
	// BR-SP-070-072: Priority assignment
	// BR-SP-002: Business unit classification
	//
	// Option B: Fail-fast on Rego syntax errors at startup
	// - Policy file NOT FOUND → INFO log, use fallback (policies are optional)
	// - Policy file EXISTS but INVALID → FATAL error, exit(1) (deployment bug)
	// - Runtime evaluation errors → fallback per BR-SP-071 (handled in controller)
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
	k8sEnricher := enricher.NewK8sEnricher(
		mgr.GetClient(),
		ctrl.Log.WithName("enricher"),
		spMetrics,
		10*time.Second, // Enrichment timeout
	)
	setupLog.Info("k8s enricher configured")

	// ========================================
	// DD-PERF-001: Atomic Status Updates
	// Status Manager for reducing K8s API calls by 66-75%
	// Consolidates multiple status field updates into single atomic operations
	// SP-CACHE-001: Pass APIReader to bypass cache for fresh refetches
	// ========================================
	statusManager := spstatus.NewManager(mgr.GetClient(), mgr.GetAPIReader())
	setupLog.Info("SignalProcessing status manager initialized (DD-PERF-001 + SP-CACHE-001)")

	// Setup reconciler with ALL required components
	if err = (&signalprocessing.SignalProcessingReconciler{
		Client:             mgr.GetClient(),
		Scheme:             mgr.GetScheme(),
		AuditClient:        auditClient,
		Metrics:            spMetrics,       // DD-005: Observability
		Recorder:           mgr.GetEventRecorderFor("signalprocessing-controller"),
		StatusManager:      statusManager,   // DD-PERF-001: Atomic status updates
		EnvClassifier:      envClassifier,
		PriorityAssigner:   priorityEngine,  // PriorityEngine implements PriorityAssigner interface
		BusinessClassifier: businessClassifier,
		RegoEngine:         regoEngine,
		OwnerChainBuilder:  ownerChainBuilder,
		LabelDetector:      labelDetector,
		K8sEnricher:        k8sEnricher,
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
