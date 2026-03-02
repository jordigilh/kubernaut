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

	tektonv1 "github.com/tektoncd/pipeline/pkg/apis/pipeline/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/runtime"
	utilruntime "k8s.io/apimachinery/pkg/util/runtime"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/cache"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/healthz"
	"sigs.k8s.io/controller-runtime/pkg/log/zap"
	metricsserver "sigs.k8s.io/controller-runtime/pkg/metrics/server"

	workflowexecutionv1alpha1 "github.com/jordigilh/kubernaut/api/workflowexecution/v1alpha1"
	scope "github.com/jordigilh/kubernaut/pkg/shared/scope"
	"github.com/jordigilh/kubernaut/internal/controller/workflowexecution"
	"github.com/jordigilh/kubernaut/pkg/audit"
	dsvalidation "github.com/jordigilh/kubernaut/pkg/datastorage/validation"
	weaudit "github.com/jordigilh/kubernaut/pkg/workflowexecution/audit"
	weclient "github.com/jordigilh/kubernaut/pkg/workflowexecution/client"
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
	// Only --config flag is supported. All other settings are in the YAML config file.
	// ========================================
	var configPath string
	flag.StringVar(&configPath, "config", weconfig.DefaultConfigPath, "Path to configuration file (optional, uses defaults if not provided)")

	opts := zap.Options{}
	opts.BindFlags(flag.CommandLine)
	flag.Parse()

	ctrl.SetLogger(zap.New(zap.UseFlagOptions(&opts)))

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

	// Validate configuration
	if err := cfg.Validate(); err != nil {
		setupLog.Error(err, "Configuration validation failed")
		os.Exit(1)
	}

	// ADR-057: Discover controller namespace for CRD watch restriction
	// Note: PipelineRun and Job (Tekton/K8s) are watched in kubernaut-workflows - not restricted
	controllerNS, err := scope.GetControllerNamespace()
	if err != nil {
		setupLog.Error(err, "unable to determine controller namespace")
		os.Exit(1)
	}

	execNS := cfg.Execution.Namespace

	mgr, err := ctrl.NewManager(ctrl.GetConfigOrDie(), ctrl.Options{
		Scheme: scheme,
		Cache: cache.Options{
			ByObject: map[client.Object]cache.ByObject{
				&workflowexecutionv1alpha1.WorkflowExecution{}: {
					Namespaces: map[string]cache.Config{
						controllerNS: {},
					},
				},
				&corev1.Secret{}: {
					Namespaces: map[string]cache.Config{
						execNS: {},
					},
				},
				&corev1.ConfigMap{}: {
					Namespaces: map[string]cache.Config{
						execNS: {},
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

	// Log configuration
	setupLog.Info("WorkflowExecution controller configuration",
		"executionNamespace", cfg.Execution.Namespace,
		"cooldownPeriod", cfg.Execution.CooldownPeriod,
		"serviceAccount", cfg.Execution.ServiceAccount,
		"metricsAddr", cfg.Controller.MetricsAddr,
		"healthProbeAddr", cfg.Controller.HealthProbeAddr,
		"dataStorageURL", cfg.DataStorage.URL,
	)

	// ========================================
	// DD-AUDIT-003 P0 MUST: Initialize AuditStore
	// Per DD-AUDIT-002: Use pkg/audit/ shared library
	// Per ADR-038: Async buffered audit ingestion
	// ========================================
	setupLog.Info("Initializing audit store (DD-AUDIT-003, DD-AUDIT-002)",
		"dataStorageURL", cfg.DataStorage.URL,
	)

	// Create OpenAPI client for Data Storage Service (DD-API-001 + DD-AUDIT-002 V2.0)
	// Uses generated OpenAPI client for type safety and contract validation
	dsClient, err := audit.NewOpenAPIClientAdapter(cfg.DataStorage.URL, cfg.DataStorage.Timeout)
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
	// Both Tekton and Job backends are always registered.
	// If a workflow uses executionEngine: "tekton", the operator is responsible
	// for ensuring Tekton Pipelines is installed in the cluster.
	// ========================================
	executorRegistry := weexecutor.NewRegistry()
	executorRegistry.Register("tekton", weexecutor.NewTektonExecutor(mgr.GetClient(), cfg.Execution.ServiceAccount))
	executorRegistry.Register("job", weexecutor.NewJobExecutor(mgr.GetClient(), cfg.Execution.ServiceAccount))
	setupLog.Info("Executor registry initialized", "engines", executorRegistry.Engines())

	// DD-WE-006: Create WorkflowQuerier for fetching dependencies from DS
	workflowQuerier, err := weclient.NewOgenWorkflowQuerierFromConfig(cfg.DataStorage.URL, cfg.DataStorage.Timeout)
	if err != nil {
		setupLog.Error(err, "Failed to create workflow querier (DD-WE-006) - continuing without dependency injection")
		// Non-fatal: controller will run without dependency injection
	} else {
		setupLog.Info("Workflow querier initialized (DD-WE-006)", "dataStorageURL", cfg.DataStorage.URL)
	}

	// DD-WE-006: Create DependencyValidator for execution-time validation (defense in depth).
	// Reuses the controller-runtime client from the manager.
	depValidator := dsvalidation.NewK8sDependencyValidator(mgr.GetClient())

	// Setup WorkflowExecution controller
	if err = (&workflowexecution.WorkflowExecutionReconciler{
		Client:              mgr.GetClient(),
		APIReader:           mgr.GetAPIReader(), // DD-STATUS-001: Cache-bypassed reads for race condition prevention
		Scheme:              mgr.GetScheme(),
		Recorder:            mgr.GetEventRecorderFor("workflowexecution-controller"),
		Metrics:             weMetrics,     // DD-METRICS-001: Injected metrics (P0 requirement)
		StatusManager:       statusManager, // DD-PERF-001: Atomic status updates
		ExecutionNamespace:  cfg.Execution.Namespace,
		CooldownPeriod:      cfg.Execution.CooldownPeriod,
		ServiceAccountName:  cfg.Execution.ServiceAccount,
		AuditStore:          auditStore,   // DD-AUDIT-003: Audit store for BR-WE-005
		PhaseManager:        phaseManager, // P0: Phase State Machine (validated transitions)
		AuditManager:        auditManager,    // P3: Audit Manager (typed audit methods)
		ExecutorRegistry:    executorRegistry, // BR-WE-014: Strategy pattern dispatch
		WorkflowQuerier:     workflowQuerier,  // DD-WE-006: DS workflow dependency fetcher
		DependencyValidator: depValidator,     // DD-WE-006: Execution-time validation
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
