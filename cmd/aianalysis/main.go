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
// This controller orchestrates AI-based incident analysis using the Kubernaut Agent.
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

	zaplog "go.uber.org/zap"
	"gopkg.in/yaml.v3"
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
	isv1alpha1 "github.com/jordigilh/kubernaut/api/investigationsession/v1alpha1"
	internalconfig "github.com/jordigilh/kubernaut/internal/config"
	config "github.com/jordigilh/kubernaut/internal/config/aianalysis"
	"github.com/jordigilh/kubernaut/internal/controller/aianalysis"
	"github.com/jordigilh/kubernaut/internal/version"
	"github.com/jordigilh/kubernaut/pkg/agentclient"
	"github.com/jordigilh/kubernaut/pkg/aianalysis/audit"
	"github.com/jordigilh/kubernaut/pkg/aianalysis/handlers"
	"github.com/jordigilh/kubernaut/pkg/aianalysis/metrics"
	"github.com/jordigilh/kubernaut/pkg/aianalysis/rego"
	aistatus "github.com/jordigilh/kubernaut/pkg/aianalysis/status"
	sharedaudit "github.com/jordigilh/kubernaut/pkg/audit"
	"github.com/jordigilh/kubernaut/pkg/shared/hotreload"
	scope "github.com/jordigilh/kubernaut/pkg/shared/scope"
	sharedtls "github.com/jordigilh/kubernaut/pkg/shared/tls"
)

var (
	scheme   = runtime.NewScheme()
	setupLog = ctrl.Log.WithName("setup")
)

func init() {
	utilruntime.Must(clientgoscheme.AddToScheme(scheme))
	utilruntime.Must(aianalysisv1.AddToScheme(scheme))
	utilruntime.Must(isv1alpha1.AddToScheme(scheme))
}

// loadAIAnalysisConfig loads and validates the AIAnalysis config (ADR-030),
// applies the config-driven log level (Issue #875), and discovers the
// controller namespace for CRD watch restriction (ADR-057). Exits the
// process on any failure, matching main()'s original fail-fast behavior.
func loadAIAnalysisConfig(configPath string, atomicLevel zaplog.AtomicLevel) (*config.Config, string) {
	setupLog.Info("Starting AI Analysis Controller",
		"version", version.Version,
		"gitCommit", version.GitCommit,
		"buildDate", version.BuildDate,
	)

	// ========================================
	// CONFIGURATION LOADING (ADR-030)
	// ========================================
	cfg, err := config.LoadFromFile(configPath)
	if err != nil {
		setupLog.Error(err, "Failed to load configuration -- aborting startup",
			"configPath", configPath)
		os.Exit(1)
	}
	if configPath != "" {
		setupLog.Info("Configuration loaded successfully", "configPath", configPath)
	} else {
		setupLog.Info("No config file specified, using defaults")
	}

	// Issue #875: Apply config-driven log level
	atomicLevel.SetLevel(cfg.Logging.ZapLevel())
	setupLog.Info("Log level configured from config file", "level", cfg.Logging.Level)

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

	return cfg, controllerNS
}

// buildAIAnalysisManager constructs the controller manager with the
// namespace-restricted AIAnalysis/InvestigationSession caches and
// metrics/health-probe/leader election settings from cfg, then registers
// the BR-INTERACTIVE-010 field indexes used for IS<->AA lookups by RR name.
// Exits the process on any failure, matching main()'s original fail-fast
// behavior.
func buildAIAnalysisManager(cfg *config.Config, controllerNS string) ctrl.Manager {
	mgr, err := ctrl.NewManager(ctrl.GetConfigOrDie(), ctrl.Options{
		Scheme: scheme,
		Cache: cache.Options{
			ByObject: map[crclient.Object]cache.ByObject{
				&aianalysisv1.AIAnalysis{}: {
					Namespaces: map[string]cache.Config{
						controllerNS: {},
					},
				},
				&isv1alpha1.InvestigationSession{}: {
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

	// BR-INTERACTIVE-010: Register field index for InvestigationSession lookups by RR name.
	if err := mgr.GetFieldIndexer().IndexField(context.Background(),
		&isv1alpha1.InvestigationSession{},
		handlers.ISFieldIndexRRName,
		func(obj crclient.Object) []string {
			is := obj.(*isv1alpha1.InvestigationSession)
			if is.Spec.RemediationRequestRef.Name == "" {
				return nil
			}
			return []string{is.Spec.RemediationRequestRef.Name}
		},
	); err != nil {
		setupLog.Error(err, "unable to register InvestigationSession field index")
		os.Exit(1)
	}

	// BR-INTERACTIVE-010: Register field index for AIAnalysis lookups by RR name (IS→AA mapping).
	if err := mgr.GetFieldIndexer().IndexField(context.Background(),
		&aianalysisv1.AIAnalysis{},
		aianalysis.AIAnalysisRRNameIndex(),
		func(obj crclient.Object) []string {
			aa := obj.(*aianalysisv1.AIAnalysis)
			if aa.Spec.RemediationRequestRef.Name == "" {
				return nil
			}
			return []string{aa.Spec.RemediationRequestRef.Name}
		},
	); err != nil {
		setupLog.Error(err, "unable to register AIAnalysis RR name field index")
		os.Exit(1)
	}

	return mgr
}

// aiAnalysisClients bundles the external clients wired at startup for the
// Investigating (BR-AI-007) and Analyzing (BR-AI-012) phase handlers, plus
// the P0 audit store/client (DD-AUDIT-003).
type aiAnalysisClients struct {
	agentClient   *agentclient.KubernautAgentClient
	regoEvaluator *rego.Evaluator
	auditStore    sharedaudit.AuditStore
	auditClient   *audit.AuditClient
}

// wireAIAnalysisClients constructs the Kubernaut Agent client (BR-AI-007,
// DD-HAPI-003), the Rego evaluator with startup policy validation (BR-AI-012,
// DD-AIANALYSIS-001/002, ADR-050), and the buffered audit store/client
// (DD-AUDIT-003, ADR-030). Exits the process on any failure, matching
// main()'s original fail-fast behavior. Rego hot-reloader and audit store
// cleanup remain the caller's responsibility during graceful shutdown.
func wireAIAnalysisClients(ctx context.Context, cfg *config.Config) *aiAnalysisClients {
	// BR-AA-HAPI-064: HTTP client timeout for session submit/poll/result calls.
	setupLog.Info("Creating Kubernaut Agent client",
		"url", cfg.Agent.URL,
		"timeout", cfg.Agent.Timeout,
		"sessionPollInterval", cfg.Agent.SessionPollInterval)
	agentClient, err := agentclient.NewKubernautAgentClient(agentclient.Config{
		BaseURL: cfg.Agent.URL,
		Timeout: cfg.Agent.Timeout,
	})
	if err != nil {
		setupLog.Error(err, "failed to create Kubernaut Agent client")
		os.Exit(1)
	}

	// DD-AIANALYSIS-001: Rego policy loading; ADR-050: fail-fast startup validation.
	setupLog.Info("Creating Rego evaluator", "policyPath", cfg.Rego.PolicyPath)
	regoEvaluator := rego.NewEvaluator(rego.Config{
		PolicyPath: cfg.Rego.PolicyPath,
	}, ctrl.Log.WithName("rego"))

	if err := regoEvaluator.StartHotReload(ctx); err != nil {
		setupLog.Error(err, "failed to load approval policy")
		os.Exit(1)
	}
	setupLog.Info("approval policy loaded successfully",
		"policyHash", regoEvaluator.GetPolicyHash())

	// Note: Rego hot-reloader cleanup is handled explicitly in graceful shutdown section.

	// DD-AUDIT-003 / DD-API-001: OpenAPI generated Data Storage client (MANDATORY).
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
	auditStore, err := sharedaudit.NewBufferedStore( //nolint:contextcheck // background audit writer goroutine is fire-and-forget by design; not tied to any single request
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

	return &aiAnalysisClients{
		agentClient:   agentClient,
		regoEvaluator: regoEvaluator,
		auditStore:    auditStore,
		auditClient:   auditClient,
	}
}

// setupAIAnalysisReconciler creates the metrics collector (DD-METRICS-001),
// the Investigating/Analyzing phase handlers (BR-AI-007, BR-AI-012), the
// atomic status manager (DD-PERF-001, AA-HAPI-001), and wires the
// AIAnalysisReconciler into mgr. Also registers the healthz/readyz checks,
// including the cache-sync-aware readyz check that prevents premature
// reconciliation before controller watches are established. Exits the
// process on any failure, matching main()'s original fail-fast behavior.
func setupAIAnalysisReconciler(mgr ctrl.Manager, cfg *config.Config, controllerNS string, clients *aiAnalysisClients) *metrics.Metrics {
	// DD-METRICS-001: Per V1.0 Service Maturity Requirements - P0 Blocker.
	setupLog.Info("Initializing AIAnalysis metrics (DD-METRICS-001)")
	aianalysisMetrics := metrics.NewMetrics()
	setupLog.Info("AIAnalysis metrics initialized and registered")

	controllerLog := ctrl.Log.WithName("controllers").WithName("AIAnalysis")
	eventRecorder := mgr.GetEventRecorderFor("aianalysis-controller")
	isChecker := handlers.NewK8sInvestigationSessionChecker(mgr.GetAPIReader(), controllerNS)
	isPhaseUpdater := handlers.NewK8sISPhaseUpdater(mgr.GetClient(), controllerNS)
	investigatingHandler := handlers.NewInvestigatingHandler(clients.agentClient, controllerLog, aianalysisMetrics, clients.auditClient,
		handlers.WithRecorder(eventRecorder),                            // DD-EVENT-001: Session lifecycle events
		handlers.WithSessionMode(),                                      // BR-AA-HAPI-064: Async submit/poll/result flow
		handlers.WithSessionPollInterval(cfg.Agent.SessionPollInterval), // BR-AA-HAPI-064.8: From config
		handlers.WithInvestigationSessionChecker(isChecker),             // BR-INTERACTIVE-010: IS CRD awareness
		handlers.WithISPhaseUpdater(isPhaseUpdater))                     // BR-INTERACTIVE-010: Set IS Active after submit
	analyzingHandler := handlers.NewAnalyzingHandler(clients.regoEvaluator, controllerLog, aianalysisMetrics, clients.auditClient).
		WithConfidenceThreshold(cfg.Rego.ConfidenceThreshold) // #225: operator-configurable threshold

	// DD-PERF-001: Atomic status updates reduce K8s API calls by 50-75%.
	// AA-HAPI-001: Pass APIReader to bypass cache for fresh refetches.
	statusManager := aistatus.NewManager(mgr.GetClient(), mgr.GetAPIReader())
	setupLog.Info("AIAnalysis status manager initialized (DD-PERF-001 + AA-HAPI-001)")

	aaReconciler := &aianalysis.AIAnalysisReconciler{
		Client:           mgr.GetClient(),
		Scheme:           mgr.GetScheme(),
		Recorder:         eventRecorder,
		Log:              controllerLog,
		Metrics:          aianalysisMetrics,   // DD-METRICS-001: Injected metrics (P0)
		StatusManager:    statusManager,       // DD-PERF-001: Atomic status updates
		AnalyzingHandler: analyzingHandler,    // BR-AI-012: Rego policy evaluation
		AuditClient:      clients.auditClient, // DD-AUDIT-003: P0 audit traces
		ISPhaseUpdater:   isPhaseUpdater,      // #1421: Cascade cancel to IS in terminal branch
	}
	aaReconciler.InvestigatingHandler.Store(investigatingHandler) // BR-AI-007: KA integration
	if err := aaReconciler.SetupWithManager(mgr); err != nil {
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
		checkCtx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
		defer cancel()
		if synced := mgr.GetCache().WaitForCacheSync(checkCtx); !synced {
			return fmt.Errorf("controller caches not yet synced")
		}
		return nil
	}

	if err := mgr.AddReadyzCheck("readyz", cacheSyncCheck); err != nil {
		setupLog.Error(err, "unable to set up ready check")
		os.Exit(1)
	}

	return aianalysisMetrics
}

// configureAIAnalysisTLSAndHotReload applies the OCP TLS security profile
// from config (Issue #748), starts the CA file watcher for client-side TLS
// hot-reload (Issue #756), and starts the log-level hot-reload watcher
// (Issue #875). Exits the process if the CA watcher fails to start,
// matching main()'s original fail-fast behavior. Returns a cleanup function
// that stops any watchers that were successfully started; callers should
// defer the returned function.
func configureAIAnalysisTLSAndHotReload(ctx context.Context, cfg *config.Config, configPath string, atomicLevel zaplog.AtomicLevel) func() {
	if err := sharedtls.SetDefaultSecurityProfileFromConfig(cfg.TLSProfile); err != nil {
		setupLog.Error(err, "Invalid TLS security profile in config, using default TLS 1.2")
	} else if cfg.TLSProfile != "" {
		setupLog.Info("TLS security profile active", "profile", cfg.TLSProfile)
	}

	var cleanups []func()

	caWatcher, caWatchErr := sharedtls.StartCAFileWatcher(ctx, setupLog)
	if caWatchErr != nil {
		setupLog.Error(caWatchErr, "Failed to start CA file watcher")
		os.Exit(1)
	}
	if caWatcher != nil {
		cleanups = append(cleanups, caWatcher.Stop)
	}

	logLevelWatcher, logWatchErr := hotreload.NewFileWatcher(
		configPath,
		func(newContent string) error {
			var partial struct {
				Logging internalconfig.LoggingConfig `yaml:"logging"`
			}
			if err := yaml.Unmarshal([]byte(newContent), &partial); err != nil {
				return fmt.Errorf("failed to parse config for log level reload: %w", err)
			}
			return internalconfig.ParseAndSetLevel(atomicLevel, partial.Logging.Level)
		},
		setupLog.WithName("log-level-watcher"),
	)
	if logWatchErr != nil {
		setupLog.Error(logWatchErr, "Failed to create log level file watcher")
	} else if err := logLevelWatcher.Start(ctx); err != nil {
		setupLog.Info("Log level file watcher failed to start", "error", err)
	} else {
		setupLog.Info("Log level hot-reload watcher started", "path", configPath)
		cleanups = append(cleanups, logLevelWatcher.Stop)
	}

	return func() {
		for _, c := range cleanups {
			c()
		}
	}
}

func main() {
	// gocritic:exitAfterDefer — run() returns an exit code instead of calling
	// os.Exit directly so deferred cleanup (cleanupHotReload) always runs.
	os.Exit(run())
}

func run() int {
	// ========================================
	// ADR-030: Configuration via YAML file
	// Single -config flag; all functional config in YAML ConfigMap
	// ========================================
	var configPath string
	flag.StringVar(&configPath, "config", config.DefaultConfigPath, "Path to YAML configuration file (optional, falls back to defaults)")

	flag.Parse()

	// Issue #875: Bootstrap logger at INFO for config loading
	atomicLevel := internalconfig.DefaultLoggingConfig().NewAtomicLevel()
	ctrl.SetLogger(zap.New(zap.Level(atomicLevel)))

	cfg, controllerNS := loadAIAnalysisConfig(configPath, atomicLevel)
	mgr := buildAIAnalysisManager(cfg, controllerNS)

	// ADR-050: Startup validation happens inside wireAIAnalysisClients and fails fast.
	ctx := context.Background()
	clients := wireAIAnalysisClients(ctx, cfg)
	regoEvaluator := clients.regoEvaluator
	auditStore := clients.auditStore

	setupAIAnalysisReconciler(mgr, cfg, controllerNS, clients)

	// Issue #756: Extract signal context for CA file watcher lifecycle
	ctx = ctrl.SetupSignalHandler()

	cleanupHotReload := configureAIAnalysisTLSAndHotReload(ctx, cfg, configPath, atomicLevel)
	defer cleanupHotReload()

	setupLog.Info("starting manager")
	if err := mgr.Start(ctx); err != nil {
		setupLog.Error(err, "problem running manager")
		return 1
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
			return 1
		}
		setupLog.Info("Audit store closed successfully, all events flushed")
	}

	setupLog.Info("AIAnalysis controller shutdown complete")
	return 0
}
