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
	"time"

	tektonv1 "github.com/tektoncd/pipeline/pkg/apis/pipeline/v1"
	"k8s.io/apimachinery/pkg/runtime"
	utilruntime "k8s.io/apimachinery/pkg/util/runtime"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/healthz"
	"sigs.k8s.io/controller-runtime/pkg/log/zap"
	metricsserver "sigs.k8s.io/controller-runtime/pkg/metrics/server"

	workflowexecutionv1alpha1 "github.com/jordigilh/kubernaut/api/workflowexecution/v1alpha1"
	"github.com/jordigilh/kubernaut/internal/controller/workflowexecution"
	"github.com/jordigilh/kubernaut/pkg/audit"
	weaudit "github.com/jordigilh/kubernaut/pkg/workflowexecution/audit"
	weconfig "github.com/jordigilh/kubernaut/pkg/workflowexecution/config"
	weexecutor "github.com/jordigilh/kubernaut/pkg/workflowexecution/executor"
	wemetrics "github.com/jordigilh/kubernaut/pkg/workflowexecution/metrics"
	wephase "github.com/jordigilh/kubernaut/pkg/workflowexecution/phase"
	westatus "github.com/jordigilh/kubernaut/pkg/workflowexecution/status"
	//+kubebuilder:scaffold:imports
)

var (
	scheme   = runtime.NewScheme()
	setupLog = ctrl.Log.WithName("setup")
)

func init() {
	utilruntime.Must(clientgoscheme.AddToScheme(scheme))
	utilruntime.Must(workflowexecutionv1alpha1.AddToScheme(scheme))
	utilruntime.Must(tektonv1.AddToScheme(scheme))
	//+kubebuilder:scaffold:scheme
}

func main() {
	// ========================================
	// CONFIGURATION LOADING (ADR-030)
	// Priority: CLI flags > Config file > Defaults
	// ========================================
	var configPath string
	flag.StringVar(&configPath, "config", "", "Path to configuration file (optional, uses defaults if not provided)")

	// CLI flags for backwards compatibility and config overrides
	var metricsAddr string
	var probeAddr string
	var enableLeaderElection bool
	var executionNamespace string
	var cooldownPeriodMinutes int
	var serviceAccountName string
	var baseCooldownSeconds int
	var maxCooldownMinutes int
	var maxBackoffExponent int
	var maxConsecutiveFailures int
	var dataStorageURL string

	flag.StringVar(&metricsAddr, "metrics-bind-address", "", "Metrics endpoint address (overrides config)")
	flag.StringVar(&probeAddr, "health-probe-bind-address", "", "Health probe address (overrides config)")
	flag.BoolVar(&enableLeaderElection, "leader-elect", false, "Enable leader election (overrides config)")
	flag.StringVar(&executionNamespace, "execution-namespace", "", "PipelineRun namespace (overrides config, DD-WE-002)")
	flag.IntVar(&cooldownPeriodMinutes, "cooldown-period", 0, "Cooldown period in minutes (overrides config, DD-WE-001)")
	flag.StringVar(&serviceAccountName, "service-account", "", "ServiceAccount name (overrides config)")
	flag.IntVar(&baseCooldownSeconds, "base-cooldown-seconds", 0, "Base cooldown in seconds (overrides config, DD-WE-004)")
	flag.IntVar(&maxCooldownMinutes, "max-cooldown-minutes", 0, "Max cooldown in minutes (overrides config, DD-WE-004)")
	flag.IntVar(&maxBackoffExponent, "max-backoff-exponent", 0, "Max backoff exponent (overrides config, DD-WE-004)")
	flag.IntVar(&maxConsecutiveFailures, "max-consecutive-failures", 0, "Max consecutive failures (overrides config, DD-WE-004)")
	flag.StringVar(&dataStorageURL, "datastorage-url", "", "Data Storage URL (overrides config, DD-AUDIT-003)")

	opts := zap.Options{
		Development: true,
	}
	opts.BindFlags(flag.CommandLine)
	flag.Parse()

	// Load configuration (file if provided, otherwise defaults)
	var cfg *weconfig.Config
	var err error
	if configPath != "" {
		cfg, err = weconfig.LoadFromFile(configPath)
		if err != nil {
			setupLog.Error(err, "Failed to load configuration file", "path", configPath)
			os.Exit(1)
		}
		setupLog.Info("Configuration loaded from file", "path", configPath)
	} else {
		cfg = weconfig.DefaultConfig()
		setupLog.Info("Using default configuration (no config file provided)")
	}

	// Apply CLI flag overrides (backwards compatibility)
	if metricsAddr != "" {
		cfg.Controller.MetricsAddr = metricsAddr
	}
	if probeAddr != "" {
		cfg.Controller.HealthProbeAddr = probeAddr
	}
	if enableLeaderElection {
		cfg.Controller.LeaderElection = true
	}
	if executionNamespace != "" {
		cfg.Execution.Namespace = executionNamespace
	}
	if cooldownPeriodMinutes > 0 {
		cfg.Execution.CooldownPeriod = time.Duration(cooldownPeriodMinutes) * time.Minute
	}
	if serviceAccountName != "" {
		cfg.Execution.ServiceAccount = serviceAccountName
	}
	if baseCooldownSeconds > 0 {
		cfg.Backoff.BaseCooldown = time.Duration(baseCooldownSeconds) * time.Second
	}
	if maxCooldownMinutes > 0 {
		cfg.Backoff.MaxCooldown = time.Duration(maxCooldownMinutes) * time.Minute
	}
	if maxBackoffExponent > 0 {
		cfg.Backoff.MaxExponent = maxBackoffExponent
	}
	if maxConsecutiveFailures > 0 {
		cfg.Backoff.MaxConsecutiveFailures = maxConsecutiveFailures
	}
	if dataStorageURL != "" {
		cfg.Audit.DataStorageURL = dataStorageURL
	}

	// Validate configuration
	if err := cfg.Validate(); err != nil {
		setupLog.Error(err, "Configuration validation failed")
		os.Exit(1)
	}

	ctrl.SetLogger(zap.New(zap.UseFlagOptions(&opts)))

	// ========================================
	// ADR-030: Validate Tekton CRDs are available
	// The controller MUST crash if Tekton is not installed
	// ========================================
	setupLog.Info("Validating Tekton Pipelines availability (ADR-030)")
	if err := checkTektonAvailable(); err != nil {
		setupLog.Error(err, "Required dependency check failed: Tekton Pipelines not available")
		setupLog.Info("Tekton Pipelines CRDs must be installed before starting this controller")
		os.Exit(1)
	}
	setupLog.Info("Tekton Pipelines CRDs verified")

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

	// Log configuration
	setupLog.Info("WorkflowExecution controller configuration",
		"executionNamespace", cfg.Execution.Namespace,
		"cooldownPeriod", cfg.Execution.CooldownPeriod,
		"serviceAccount", cfg.Execution.ServiceAccount,
		"metricsAddr", cfg.Controller.MetricsAddr,
		"healthProbeAddr", cfg.Controller.HealthProbeAddr,
		"dataStorageURL", cfg.Audit.DataStorageURL,
		// DD-WE-004: Exponential Backoff Configuration
		"baseCooldown", cfg.Backoff.BaseCooldown,
		"maxCooldown", cfg.Backoff.MaxCooldown,
		"maxBackoffExponent", cfg.Backoff.MaxExponent,
		"maxConsecutiveFailures", cfg.Backoff.MaxConsecutiveFailures,
	)

	// ========================================
	// DD-AUDIT-003 P0 MUST: Initialize AuditStore
	// Per DD-AUDIT-002: Use pkg/audit/ shared library
	// Per ADR-038: Async buffered audit ingestion
	// ========================================
	setupLog.Info("Initializing audit store (DD-AUDIT-003, DD-AUDIT-002)",
		"dataStorageURL", cfg.Audit.DataStorageURL,
	)

	// Create OpenAPI client for Data Storage Service (DD-API-001 + DD-AUDIT-002 V2.0)
	// Uses generated OpenAPI client for type safety and contract validation
	dsClient, err := audit.NewOpenAPIClientAdapter(cfg.Audit.DataStorageURL, cfg.Audit.Timeout)
	if err != nil {
		setupLog.Error(err, "FATAL: failed to create Data Storage client - DD-API-001 compliance required")
		os.Exit(1)
	}

	// Create buffered audit store using shared library (DD-AUDIT-002)
	// Use recommended config for workflowexecution service
	auditConfig := audit.RecommendedConfig("workflowexecution")
	auditStore, err := audit.NewBufferedStore(
		dsClient,
		auditConfig,
		"workflowexecution",
		ctrl.Log.WithName("audit"),
	)
	if err != nil {
		// Audit is MANDATORY per ADR-032 §2 - controller MUST crash if audit unavailable
		// Per ADR-032 §3: WorkflowExecution is P0 (Business-Critical) - NO graceful degradation
		// Rationale: Audit unavailability is a deployment/configuration error, not a transient failure
		// The correct response is to crash and let Kubernetes orchestration detect the misconfiguration
		setupLog.Error(err, "FATAL: failed to create audit store - audit is MANDATORY per ADR-032 §2")
		os.Exit(1) // Crash on init failure - NO RECOVERY ALLOWED
	}
	setupLog.Info("Audit store initialized successfully",
		"buffer_size", auditConfig.BufferSize,
		"batch_size", auditConfig.BatchSize,
		"flush_interval", auditConfig.FlushInterval,
	)

	// ========================================
	// DD-METRICS-001: Dependency-Injected Metrics
	// Per SERVICE_MATURITY_REQUIREMENTS.md v1.2.0: Metrics MUST be wired to controller
	// NewMetrics() automatically registers with controller-runtime registry
	// ========================================
	weMetrics := wemetrics.NewMetrics()
	setupLog.Info("WorkflowExecution metrics initialized and registered (DD-METRICS-001)")

	// ========================================
	// DD-PERF-001: Atomic Status Updates
	// Status Manager for reducing K8s API calls by 50%+
	// Consolidates multiple status field updates into single atomic operations
	// ========================================
	statusManager := westatus.NewManager(mgr.GetClient())
	setupLog.Info("WorkflowExecution status manager initialized (DD-PERF-001)")

	// ========================================
	// CONTROLLER_REFACTORING_PATTERN_LIBRARY.md §1: Phase State Machine (P0)
	// Phase Manager for validated phase transitions and terminal state checking
	// ========================================
	phaseManager := wephase.NewManager()
	setupLog.Info("WorkflowExecution phase manager initialized (P0: Phase State Machine)")

	// ========================================
	// CONTROLLER_REFACTORING_PATTERN_LIBRARY.md §7: Audit Manager (P3)
	// Audit Manager for typed audit event methods and better testability
	// ========================================
	auditManager := weaudit.NewManager(auditStore, ctrl.Log.WithName("audit-manager"))
	setupLog.Info("WorkflowExecution audit manager initialized (P3: Audit Manager)")

	// ========================================
	// BR-WE-014: Executor Registry (Strategy Pattern)
	// Registers Tekton and Job execution backends for dispatch
	// ========================================
	executorRegistry := weexecutor.NewRegistry()
	executorRegistry.Register("tekton", weexecutor.NewTektonExecutor(mgr.GetClient(), cfg.Execution.ServiceAccount))
	executorRegistry.Register("job", weexecutor.NewJobExecutor(mgr.GetClient(), cfg.Execution.ServiceAccount))
	setupLog.Info("Executor registry initialized", "engines", executorRegistry.Engines())

	// Setup WorkflowExecution controller
	if err = (&workflowexecution.WorkflowExecutionReconciler{
		Client:             mgr.GetClient(),
		APIReader:          mgr.GetAPIReader(), // DD-STATUS-001: Cache-bypassed reads for race condition prevention
		Scheme:             mgr.GetScheme(),
		Recorder:           mgr.GetEventRecorderFor("workflowexecution-controller"),
		Metrics:            weMetrics,     // DD-METRICS-001: Injected metrics (P0 requirement)
		StatusManager:      statusManager, // DD-PERF-001: Atomic status updates
		ExecutionNamespace: cfg.Execution.Namespace,
		CooldownPeriod:     cfg.Execution.CooldownPeriod,
		ServiceAccountName: cfg.Execution.ServiceAccount,
		AuditStore:         auditStore,   // DD-AUDIT-003: Audit store for BR-WE-005
		PhaseManager:       phaseManager, // P0: Phase State Machine (validated transitions)
		AuditManager:       auditManager,    // P3: Audit Manager (typed audit methods)
		ExecutorRegistry:   executorRegistry, // BR-WE-014: Strategy pattern dispatch
		// DD-WE-004: Exponential Backoff Configuration (BR-WE-012)
		// V1.0: DEPRECATED - Routing moved to RO per DD-RO-002 Phase 3 (Dec 19, 2025)
		BaseCooldownPeriod:     cfg.Backoff.BaseCooldown,
		MaxCooldownPeriod:      cfg.Backoff.MaxCooldown,
		MaxBackoffExponent:     cfg.Backoff.MaxExponent,
		MaxConsecutiveFailures: cfg.Backoff.MaxConsecutiveFailures,
	}).SetupWithManager(mgr); err != nil {
		setupLog.Error(err, "unable to create controller", "controller", "WorkflowExecution")
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
	if err := mgr.Start(ctrl.SetupSignalHandler()); err != nil {
		setupLog.Error(err, "problem running manager")
		os.Exit(1)
	}

	// ========================================
	// DD-AUDIT-002: Graceful Shutdown - Flush Audit Events
	// Per DD-007: Kubernetes-aware graceful shutdown
	// ========================================
	if auditStore != nil {
		setupLog.Info("Flushing audit events on shutdown (DD-AUDIT-002)")
		if err := auditStore.Close(); err != nil {
			setupLog.Error(err, "Failed to close audit store")
		} else {
			setupLog.Info("Audit store closed successfully")
		}
	}
}

// ========================================
// ADR-030: Tekton CRD Availability Check
// Controller MUST crash if Tekton is not installed
// ========================================

// checkTektonAvailable verifies that Tekton Pipeline CRDs are installed
// Returns error if Tekton CRDs are not available
func checkTektonAvailable() error {
	config := ctrl.GetConfigOrDie()
	k8sClient, err := client.New(config, client.Options{Scheme: scheme})
	if err != nil {
		return fmt.Errorf("failed to create client for Tekton check: %w", err)
	}

	// Try to list PipelineRuns - this verifies the CRD exists
	var prList tektonv1.PipelineRunList
	if err := k8sClient.List(context.Background(), &prList, client.Limit(1)); err != nil {
		return fmt.Errorf("Tekton PipelineRun CRD not available: %w", err)
	}

	return nil
}
