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

// Package main is the entry point for the AIAnalysis controller.
// This controller orchestrates AI-based incident analysis using HolmesGPT-API.
//
// Business Requirements: BR-AI-001 to BR-AI-083 (V1.0)
// Architecture: DD-CONTRACT-002, DD-AIANALYSIS-001
package main

import (
	"context"
	"flag"
	"fmt"
	"net/http"
	"os"
	"time"

	"k8s.io/apimachinery/pkg/runtime"
	utilruntime "k8s.io/apimachinery/pkg/util/runtime"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/cache"
	crclient "sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/healthz"
	"sigs.k8s.io/controller-runtime/pkg/log/zap"
	metricsserver "sigs.k8s.io/controller-runtime/pkg/metrics/server"

	aianalysisv1 "github.com/jordigilh/kubernaut/api/aianalysis/v1alpha1"
	scope "github.com/jordigilh/kubernaut/pkg/shared/scope"
	config "github.com/jordigilh/kubernaut/internal/config/aianalysis"
	"github.com/jordigilh/kubernaut/internal/controller/aianalysis"
	"github.com/jordigilh/kubernaut/pkg/aianalysis/audit"
	"github.com/jordigilh/kubernaut/pkg/aianalysis/handlers"
	"github.com/jordigilh/kubernaut/pkg/aianalysis/metrics"
	"github.com/jordigilh/kubernaut/pkg/aianalysis/rego"
	aistatus "github.com/jordigilh/kubernaut/pkg/aianalysis/status"
	sharedaudit "github.com/jordigilh/kubernaut/pkg/audit"
	client "github.com/jordigilh/kubernaut/pkg/holmesgpt/client"
)

var (
	scheme   = runtime.NewScheme()
	setupLog = ctrl.Log.WithName("setup")

	// Build information (set by ldflags) per DD-014
	Version   = "dev"
	GitCommit = "unknown"
	BuildTime = "unknown"
)

func init() {
	utilruntime.Must(clientgoscheme.AddToScheme(scheme))
	utilruntime.Must(aianalysisv1.AddToScheme(scheme))
}

func main() {
	// ========================================
	// ADR-030: Configuration via YAML file
	// Single -config flag; all functional config in YAML ConfigMap
	// ========================================
	var configPath string
	flag.StringVar(&configPath, "config", config.DefaultConfigPath, "Path to YAML configuration file (optional, falls back to defaults)")

	opts := zap.Options{Development: true}
	opts.BindFlags(flag.CommandLine)
	flag.Parse()

	ctrl.SetLogger(zap.New(zap.UseFlagOptions(&opts)))

	// DD-014: Log version information at startup
	setupLog.Info("Starting AI Analysis Controller",
		"version", Version,
		"gitCommit", GitCommit,
		"buildTime", BuildTime,
	)

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

	// ADR-057: Discover controller namespace for CRD watch restriction
	controllerNS, err := scope.GetControllerNamespace()
	if err != nil {
		setupLog.Error(err, "unable to determine controller namespace")
		os.Exit(1)
	}

	mgr, err := ctrl.NewManager(ctrl.GetConfigOrDie(), ctrl.Options{
		Scheme: scheme,
		Cache: cache.Options{
			ByObject: map[crclient.Object]cache.ByObject{
				&aianalysisv1.AIAnalysis{}: {
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
	// BR-AI-007: Wire HolmesGPT-API client for Investigating phase
	// DD-HAPI-003: Using generated OpenAPI client for type safety and contract compliance
	// ========================================
	// BR-AA-HAPI-064: HTTP client timeout for session submit/poll/result calls.
	// With asyncio.to_thread on the HAPI side, 202 responses are instant,
	// but a generous timeout guards against network latency edge cases.
	setupLog.Info("Creating HolmesGPT-API client (generated)",
		"url", cfg.HolmesGPT.URL,
		"timeout", cfg.HolmesGPT.Timeout,
		"sessionPollInterval", cfg.HolmesGPT.SessionPollInterval)
	holmesGPTClient, err := client.NewHolmesGPTClient(client.Config{
		BaseURL: cfg.HolmesGPT.URL,
		Timeout: cfg.HolmesGPT.Timeout,
	})
	if err != nil {
		setupLog.Error(err, "failed to create HolmesGPT-API client")
		os.Exit(1)
	}

	// ========================================
	// BR-AI-012: Wire Rego evaluator for Analyzing phase
	// DD-AIANALYSIS-001: Rego policy loading
	// ADR-050: Configuration Validation Strategy (fail-fast at startup)
	// DD-AIANALYSIS-002: Rego Policy Startup Validation
	// ========================================
	setupLog.Info("Creating Rego evaluator", "policyPath", cfg.Rego.PolicyPath)
	regoEvaluator := rego.NewEvaluator(rego.Config{
		PolicyPath: cfg.Rego.PolicyPath,
	}, ctrl.Log.WithName("rego"))

	// ADR-050: Startup validation - fails fast on invalid policy
	ctx := context.Background()
	if err := regoEvaluator.StartHotReload(ctx); err != nil {
		setupLog.Error(err, "failed to load approval policy")
		os.Exit(1)
	}
	setupLog.Info("approval policy loaded successfully",
		"policyHash", regoEvaluator.GetPolicyHash())

	// Note: Rego hot-reloader cleanup is handled explicitly in graceful shutdown section
	// See cleanup after mgr.Start() returns

	// ========================================
	// DD-AUDIT-003: Wire audit client for P0 audit traces
	// DD-API-001: Use OpenAPI generated client (MANDATORY)
	// ADR-030: DataStorage URL and buffer config from YAML config
	// ========================================
	setupLog.Info("Creating audit client",
		"dataStorageURL", cfg.DataStorage.URL,
		"dataStorageTimeout", cfg.DataStorage.Timeout)
	dsClient, err := sharedaudit.NewOpenAPIClientAdapter(cfg.DataStorage.URL, cfg.DataStorage.Timeout)
	if err != nil {
		setupLog.Error(err, "failed to create Data Storage client")
		os.Exit(1)
	}

	// ADR-030: Use buffer config from YAML (not hardcoded RecommendedConfig)
	auditConfig := sharedaudit.Config{
		BufferSize:    cfg.DataStorage.Buffer.BufferSize,
		BatchSize:     cfg.DataStorage.Buffer.BatchSize,
		FlushInterval: cfg.DataStorage.Buffer.FlushInterval,
		MaxRetries:    cfg.DataStorage.Buffer.MaxRetries,
	}
	auditStore, err := sharedaudit.NewBufferedStore(
		dsClient,
		auditConfig,
		"aianalysis",
		ctrl.Log.WithName("audit"),
	)
	if err != nil {
		setupLog.Error(err, "failed to create audit store - audit is a P0 requirement (BR-AI-090)")
		os.Exit(1)
	}

	// Create service-specific audit client (guaranteed non-nil)
	auditClient := audit.NewAuditClient(auditStore, ctrl.Log.WithName("audit"))
	setupLog.Info("Audit client initialized successfully",
		"bufferSize", auditConfig.BufferSize,
		"batchSize", auditConfig.BatchSize,
		"flushInterval", auditConfig.FlushInterval)

	// ========================================
	// DD-METRICS-001: Initialize Metrics
	// Per V1.0 Service Maturity Requirements - P0 Blocker
	// Metrics wired to controller via dependency injection
	// ========================================
	setupLog.Info("Initializing AIAnalysis metrics (DD-METRICS-001)")
	aianalysisMetrics := metrics.NewMetrics()
	setupLog.Info("AIAnalysis metrics initialized and registered")

	// ========================================
	// Create phase handlers (BR-AI-007, BR-AI-012)
	// DD-METRICS-001: Pass metrics to handlers
	// DD-AUDIT-003: Pass audit client to handlers
	// ========================================
	controllerLog := ctrl.Log.WithName("controllers").WithName("AIAnalysis")
	eventRecorder := mgr.GetEventRecorderFor("aianalysis-controller")
	investigatingHandler := handlers.NewInvestigatingHandler(holmesGPTClient, controllerLog, aianalysisMetrics, auditClient,
		handlers.WithRecorder(eventRecorder),                              // DD-EVENT-001: Session lifecycle events
		handlers.WithSessionMode(),                                        // BR-AA-HAPI-064: Async submit/poll/result flow
		handlers.WithSessionPollInterval(cfg.HolmesGPT.SessionPollInterval)) // BR-AA-HAPI-064.8: From config
	analyzingHandler := handlers.NewAnalyzingHandler(regoEvaluator, controllerLog, aianalysisMetrics, auditClient)

	// ========================================
	// DD-PERF-001: Atomic Status Updates
	// Status Manager for reducing K8s API calls by 50-75%
	// Consolidates multiple status field updates into single atomic operations
	// AA-HAPI-001: Pass APIReader to bypass cache for fresh refetches
	// ========================================
	statusManager := aistatus.NewManager(mgr.GetClient(), mgr.GetAPIReader())
	setupLog.Info("AIAnalysis status manager initialized (DD-PERF-001 + AA-HAPI-001)")

	if err = (&aianalysis.AIAnalysisReconciler{
		Client:               mgr.GetClient(),
		Scheme:               mgr.GetScheme(),
		Recorder:             eventRecorder,
		Log:                  controllerLog,
		Metrics:              aianalysisMetrics,    // DD-METRICS-001: Injected metrics (P0)
		StatusManager:        statusManager,        // DD-PERF-001: Atomic status updates
		InvestigatingHandler: investigatingHandler, // BR-AI-007: HolmesGPT integration
		AnalyzingHandler:     analyzingHandler,     // BR-AI-012: Rego policy evaluation
		AuditClient:          auditClient,          // DD-AUDIT-003: P0 audit traces
	}).SetupWithManager(mgr); err != nil {
		setupLog.Error(err, "unable to create controller", "controller", "AIAnalysis")
		os.Exit(1)
	}

	if err := mgr.AddHealthzCheck("healthz", healthz.Ping); err != nil {
		setupLog.Error(err, "unable to set up health check")
		os.Exit(1)
	}

	// Per RCA (Jan 31, 2026): Readyz must wait for controller caches to sync.
	// healthz.Ping returns 200 immediately, causing tests to start before watches are ready.
	// This leads to 10-15 second delay between test creation and first reconciliation.
	// Solution: Custom check that verifies manager's cache sync status.
	cacheSyncCheck := func(_ *http.Request) error {
		ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
		defer cancel()
		if synced := mgr.GetCache().WaitForCacheSync(ctx); !synced {
			return fmt.Errorf("controller caches not yet synced")
		}
		return nil
	}

	if err := mgr.AddReadyzCheck("readyz", cacheSyncCheck); err != nil {
		setupLog.Error(err, "unable to set up ready check")
		os.Exit(1)
	}

	setupLog.Info("starting manager")
	if err := mgr.Start(ctrl.SetupSignalHandler()); err != nil {
		setupLog.Error(err, "problem running manager")
		os.Exit(1)
	}

	// ========================================
	// DD-007 / ADR-032 §2: Graceful Shutdown
	// Per V1.0 Service Maturity Requirements - P0 Blocker
	// BR-AI-082: Handle SIGTERM within timeout
	// BR-AI-090: Complete in-flight work before exit
	// BR-AI-091: Flush audit buffer before exit
	// BR-AI-012: Stop Rego hot-reloader cleanly
	// ========================================
	setupLog.Info("Graceful shutdown initiated")

	// Step 1: Stop Rego hot-reloader (prevents new policy evaluations)
	// Rationale: Rego policies should not generate new audit events during audit flush
	setupLog.Info("Stopping Rego hot-reloader (DD-AIANALYSIS-002, BR-AI-012)")
	regoEvaluator.Stop()
	setupLog.Info("Rego hot-reloader stopped successfully")

	// Step 2: Flush audit events (ensures no audit loss)
	// Per ADR-032 §2: Audit loss is UNACCEPTABLE - flush errors are FATAL
	if auditStore != nil {
		setupLog.Info("Flushing audit events on shutdown (DD-007, ADR-032 §2, BR-AI-091)")
		if err := auditStore.Close(); err != nil {
			setupLog.Error(err, "FATAL: Failed to close audit store - audit loss detected")
			os.Exit(1)
		}
		setupLog.Info("Audit store closed successfully, all events flushed")
	}

	setupLog.Info("AIAnalysis controller shutdown complete")
}
