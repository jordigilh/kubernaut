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
	"fmt"
	"net/http"
	"os"
	"time"

	"github.com/go-logr/logr"
	"github.com/go-logr/zapr"
	zaplog "go.uber.org/zap"
	"gopkg.in/yaml.v3"
	"k8s.io/apimachinery/pkg/runtime"
	utilruntime "k8s.io/apimachinery/pkg/util/runtime"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/cache"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/healthz"
	"sigs.k8s.io/controller-runtime/pkg/log/zap"
	metricsserver "sigs.k8s.io/controller-runtime/pkg/metrics/server"

	eav1 "github.com/jordigilh/kubernaut/api/effectivenessassessment/v1alpha1"
	remediationv1alpha1 "github.com/jordigilh/kubernaut/api/remediation/v1alpha1"
	internalconfig "github.com/jordigilh/kubernaut/internal/config"
	config "github.com/jordigilh/kubernaut/internal/config/effectivenessmonitor"
	controller "github.com/jordigilh/kubernaut/internal/controller/effectivenessmonitor"
	"github.com/jordigilh/kubernaut/internal/version"
	"github.com/jordigilh/kubernaut/pkg/audit"
	emaudit "github.com/jordigilh/kubernaut/pkg/effectivenessmonitor/audit"
	emclient "github.com/jordigilh/kubernaut/pkg/effectivenessmonitor/client"
	emmetrics "github.com/jordigilh/kubernaut/pkg/effectivenessmonitor/metrics"
	"github.com/jordigilh/kubernaut/pkg/effectivenessmonitor/startup"
	"github.com/jordigilh/kubernaut/pkg/shared/auth"
	"github.com/jordigilh/kubernaut/pkg/shared/hotreload"
	scope "github.com/jordigilh/kubernaut/pkg/shared/scope"
	sharedtls "github.com/jordigilh/kubernaut/pkg/shared/tls"
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
	// ADR-030: Configuration via YAML file. Single --config flag; all
	// functional config lives in the YAML ConfigMap.
	var configPath string
	flag.StringVar(&configPath, "config", config.DefaultConfigPath, "Path to YAML configuration file (optional, falls back to defaults)")

	flag.Parse()

	// Issue #875: Bootstrap logger at INFO for config loading
	atomicLevel := internalconfig.DefaultLoggingConfig().NewAtomicLevel()
	ctrl.SetLogger(zap.New(zap.Level(atomicLevel)))

	setupLog.Info("Starting EffectivenessMonitor Controller",
		"version", version.Version,
		"gitCommit", version.GitCommit,
		"buildDate", version.BuildDate,
	)

	// CONFIGURATION LOADING (ADR-030) + ADR-057 namespace discovery
	cfg, controllerNS, err := loadConfigAndNamespace(configPath, atomicLevel, setupLog)
	if err != nil {
		setupLog.Error(err, "Failed to load configuration -- aborting startup",
			"configPath", configPath)
		os.Exit(1)
	}

	// Manager, audit store, and metrics (DD-AUDIT-003, DD-API-001, DD-METRICS-001).
	// Issue #331: Prometheus/AlertManager readiness is best-effort checked after
	// client init below; failures are logged as warnings, not fatal.
	mgr, auditStore, emMetrics, err := initCoreDependencies(cfg, controllerNS, setupLog)
	if err != nil {
		setupLog.Error(err, "Failed to initialize core dependencies")
		os.Exit(1)
	}

	// Issue #484: Create signal context early so the CA file watcher
	// can respect graceful shutdown from the start.
	ctx := ctrl.SetupSignalHandler()

	// EXTERNAL CLIENT INITIALIZATION + best-effort readiness check
	// (BR-EM-002, BR-EM-003, Issue #452, #484, Issue #331)
	promClient, amClient, stopCAWatcher, err := initExternalDependencies(ctx, cfg, setupLog)
	if err != nil {
		setupLog.Error(err, "Failed to initialize external dependencies")
		os.Exit(1)
	}
	defer stopCAWatcher()

	// AUDIT MANAGER + DS QUERIER + CONTROLLER SETUP
	if err := wireController(mgr, cfg, emMetrics, promClient, amClient, auditStore, setupLog); err != nil {
		setupLog.Error(err, "unable to wire controller")
		os.Exit(1)
	}
	//+kubebuilder:scaffold:builder

	if err := registerHealthChecks(mgr); err != nil {
		setupLog.Error(err, "unable to set up health checks")
		os.Exit(1)
	}

	// Issue #748/#756/#875: TLS security profile, CA file hot-reload, and
	// log-level hot-reload.
	stopHotReload := wireHotReload(ctx, cfg, configPath, atomicLevel, setupLog)
	defer stopHotReload()

	if err := runManagerUntilShutdown(ctx, mgr, auditStore, setupLog); err != nil {
		setupLog.Error(err, "problem running manager")
		os.Exit(1)
	}
}

// runManagerUntilShutdown starts mgr and blocks until ctx is cancelled, then
// flushes the audit store (DD-007 graceful shutdown). Extracted from main
// (Wave 6 6a GREEN: funlen remediation) — pure code motion, no behavior
// change.
func runManagerUntilShutdown(ctx context.Context, mgr ctrl.Manager, auditStore audit.AuditStore, logger logr.Logger) error {
	logger.Info("starting manager")
	if err := mgr.Start(ctx); err != nil {
		return fmt.Errorf("problem running manager: %w", err)
	}

	logger.Info("Shutting down effectiveness monitor, flushing remaining audit events")
	if err := auditStore.Close(); err != nil {
		return fmt.Errorf("failed to close audit store gracefully: %w", err)
	}
	logger.Info("Audit store closed successfully, all events flushed")
	return nil
}

// loadEffectivenessMonitorConfig loads the controller configuration from
// configPath (or defaults if empty), applies the config-driven log level
// (Issue #875) to atomicLevel, and validates the result (ADR-030).
func loadEffectivenessMonitorConfig(configPath string, atomicLevel zaplog.AtomicLevel) (*config.Config, error) {
	cfg, err := config.LoadFromFile(configPath)
	if err != nil {
		return nil, fmt.Errorf("failed to load configuration file %q: %w", configPath, err)
	}

	// Issue #875: Apply config-driven log level
	atomicLevel.SetLevel(cfg.Logging.ZapLevel())

	if err := cfg.Validate(); err != nil {
		return nil, fmt.Errorf("configuration validation failed: %w", err)
	}
	return cfg, nil
}

// loadConfigAndNamespace loads the controller configuration (ADR-030) and
// discovers the controller namespace for CRD watch restriction (ADR-057).
// Extracted from main (Wave 6 6a GREEN: funlen remediation) — pure code
// motion, no behavior change.
func loadConfigAndNamespace(configPath string, atomicLevel zaplog.AtomicLevel, logger logr.Logger) (*config.Config, string, error) {
	cfg, err := loadEffectivenessMonitorConfig(configPath, atomicLevel)
	if err != nil {
		return nil, "", err
	}
	if configPath != "" {
		logger.Info("Configuration loaded successfully", "configPath", configPath)
	} else {
		logger.Info("No config file specified, using defaults")
	}
	logger.Info("Log level configured from config file", "level", cfg.Logging.Level)

	controllerNS, err := scope.GetControllerNamespace()
	if err != nil {
		return nil, "", fmt.Errorf("unable to determine controller namespace: %w", err)
	}
	return cfg, controllerNS, nil
}

// initCoreDependencies builds the controller-runtime manager (ADR-057
// namespace-restricted cache), the DD-AUDIT-003/DD-API-001 buffered audit
// store, and the DD-METRICS-001 metrics registry. Extracted from main
// (Wave 6 6a GREEN: funlen remediation) — pure code motion, no behavior
// change.
func initCoreDependencies(cfg *config.Config, controllerNS string, logger logr.Logger) (ctrl.Manager, audit.AuditStore, *emmetrics.Metrics, error) {
	mgr, err := buildManager(cfg, controllerNS)
	if err != nil {
		return nil, nil, nil, fmt.Errorf("unable to start manager: %w", err)
	}

	auditStore, err := buildAuditStore(cfg)
	if err != nil {
		return nil, nil, nil, fmt.Errorf("failed to create audit store: %w", err)
	}
	logger.Info("Audit store initialized",
		"dataStorageURL", cfg.DataStorage.URL,
		"bufferSize", cfg.DataStorage.Buffer.BufferSize,
		"batchSize", cfg.DataStorage.Buffer.BatchSize,
		"flushInterval", cfg.DataStorage.Buffer.FlushInterval,
	)

	emMetrics := emmetrics.NewMetrics()
	logger.Info("EffectivenessMonitor metrics initialized and registered (DD-METRICS-001)")

	logger.Info("EffectivenessMonitor controller configuration",
		"metricsAddr", cfg.Controller.MetricsAddr,
		"healthProbeAddr", cfg.Controller.HealthProbeAddr,
		"stabilizationWindow", cfg.Assessment.StabilizationWindow,
		"validityWindow", cfg.Assessment.ValidityWindow,
		"prometheusEnabled", cfg.External.PrometheusEnabled,
		"alertManagerEnabled", cfg.External.AlertManagerEnabled,
		"dataStorageURL", cfg.DataStorage.URL,
	)

	return mgr, auditStore, emMetrics, nil
}

// buildManager constructs the controller-runtime manager, restricting the
// EffectivenessAssessment CRD watch to controllerNS (ADR-057).
func buildManager(cfg *config.Config, controllerNS string) (ctrl.Manager, error) {
	return ctrl.NewManager(ctrl.GetConfigOrDie(), ctrl.Options{
		Scheme: scheme,
		Cache: cache.Options{
			ByObject: map[client.Object]cache.ByObject{
				&eav1.EffectivenessAssessment{}: {
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
}

// buildAuditStore constructs the DD-AUDIT-003/DD-API-001 buffered audit
// store backed by the Data Storage Service.
func buildAuditStore(cfg *config.Config) (audit.AuditStore, error) {
	dataStorageClient, err := audit.NewOpenAPIClientAdapter(cfg.DataStorage.URL, cfg.DataStorage.Timeout)
	if err != nil {
		return nil, fmt.Errorf("failed to create data storage client (url=%s): %w", cfg.DataStorage.URL, err)
	}

	auditConfig := audit.Config{
		BufferSize:    cfg.DataStorage.Buffer.BufferSize,
		BatchSize:     cfg.DataStorage.Buffer.BatchSize,
		FlushInterval: cfg.DataStorage.Buffer.FlushInterval,
		MaxRetries:    cfg.DataStorage.Buffer.MaxRetries,
	}

	// DD-005 v2.0: pkg/audit uses logr.Logger for unified logging interface;
	// convert the zap production logger via the zapr adapter.
	zapLogger, err := zaplog.NewProduction()
	if err != nil {
		return nil, fmt.Errorf("failed to create zap logger for audit store: %w", err)
	}
	auditLogger := zapr.NewLogger(zapLogger.Named("audit"))

	auditStore, err := audit.NewBufferedStore(dataStorageClient, auditConfig, "effectiveness-monitor", auditLogger)
	if err != nil {
		return nil, fmt.Errorf("failed to create buffered audit store: %w", err)
	}
	return auditStore, nil
}

// initExternalDependencies builds the external HTTP client, the optional
// Prometheus/AlertManager clients, and performs the best-effort readiness
// check (Issue #331: these are optional enrichment sources, not startup
// dependencies — failures are logged as warnings inside CheckExternalServices,
// not fatal). Extracted from main (Wave 6 6a GREEN: funlen remediation) —
// pure code motion, no behavior change.
func initExternalDependencies(ctx context.Context, cfg *config.Config, logger logr.Logger) (emclient.PrometheusQuerier, emclient.AlertManagerClient, func(), error) {
	externalHTTPClient, stopCAWatcher, err := buildExternalHTTPClient(ctx, cfg, logger)
	if err != nil {
		return nil, nil, func() {}, fmt.Errorf("failed to initialize external HTTP client: %w", err)
	}

	promClient, amClient := buildExternalClients(cfg, externalHTTPClient, logger)

	startupCtx, startupCancel := context.WithTimeout(context.Background(), cfg.External.ConnectionTimeout+5*time.Second)
	defer startupCancel()

	readiness := startup.CheckExternalServices(startupCtx, logger, startup.ExternalServicesConfig{
		PrometheusEnabled:   cfg.External.PrometheusEnabled,
		PrometheusURL:       cfg.External.PrometheusURL,
		AlertManagerEnabled: cfg.External.AlertManagerEnabled,
		AlertManagerURL:     cfg.External.AlertManagerURL,
	}, promClient, amClient)
	if readiness.Error != nil {
		return nil, nil, stopCAWatcher, fmt.Errorf("external service configuration error: %w", readiness.Error)
	}

	return promClient, amClient, stopCAWatcher, nil
}

// wireController initializes the DD-AUDIT-003 audit manager and DD-EM-002
// DataStorage querier, constructs the EffectivenessMonitor reconciler, and
// registers it with mgr. Extracted from main (Wave 6 6a GREEN: funlen
// remediation) — pure code motion, no behavior change.
func wireController(mgr ctrl.Manager, cfg *config.Config, emMetrics *emmetrics.Metrics, promClient emclient.PrometheusQuerier, amClient emclient.AlertManagerClient, auditStore audit.AuditStore, logger logr.Logger) error {
	auditManager := emaudit.NewManager(auditStore, ctrl.Log.WithName("em-audit"))
	logger.Info("EM audit manager initialized (DD-AUDIT-003, Pattern 2)")

	dsQuerier, err := emclient.NewOgenDataStorageQuerier(cfg.DataStorage.URL, cfg.DataStorage.Timeout)
	if err != nil {
		return fmt.Errorf("failed to create DataStorage querier (url=%s): %w", cfg.DataStorage.URL, err)
	}
	logger.Info("DataStorage querier initialized (DD-API-001: ogen, DD-AUTH-005: SA auth)",
		"url", cfg.DataStorage.URL)

	emReconciler := controller.NewReconciler(
		controller.ReconcilerDeps{
			Client:             mgr.GetClient(),
			APIReader:          mgr.GetAPIReader(),
			Scheme:             mgr.GetScheme(),
			Recorder:           mgr.GetEventRecorderFor("effectivenessmonitor-controller"),
			Metrics:            emMetrics,
			PrometheusClient:   promClient,
			AlertManagerClient: amClient,
			AuditManager:       auditManager,
			DSQuerier:          dsQuerier,
		},
		func() controller.ReconcilerConfig {
			c := controller.DefaultReconcilerConfig()
			c.ValidityWindow = cfg.Assessment.ValidityWindow
			c.PrometheusEnabled = cfg.External.PrometheusEnabled
			c.AlertManagerEnabled = cfg.External.AlertManagerEnabled
			c.PrometheusLookback = cfg.External.PrometheusLookback
			c.RequeueAssessmentInProgress = cfg.External.ScrapeInterval
			return c
		}(),
	)
	emReconciler.SetRESTMapper(mgr.GetRESTMapper())
	if err := emReconciler.SetupWithManager(mgr, cfg.Assessment.MaxConcurrentReconciles); err != nil {
		return fmt.Errorf("unable to create controller %q: %w", "EffectivenessMonitor", err)
	}
	return nil
}

// waitForCAFile polls for caFile to exist and contain non-empty content,
// returning its PEM bytes. Issue #484: on OCP, the service-ca operator
// injects the CA bundle asynchronously after the ConfigMap volume is
// mounted, so callers must tolerate a brief delay before the file appears.
func waitForCAFile(caFile string, retryInterval, retryTimeout time.Duration, logger logr.Logger) ([]byte, error) {
	retryDeadline := time.Now().Add(retryTimeout)
	for {
		data, readErr := os.ReadFile(caFile)
		if readErr == nil && len(data) > 0 {
			return data, nil
		}
		if time.Now().After(retryDeadline) {
			if readErr != nil {
				return nil, fmt.Errorf("CA file %q not readable after %s timeout: %w", caFile, retryTimeout, readErr)
			}
			return nil, fmt.Errorf("CA file %q exists but is empty (0 bytes) after %s timeout", caFile, retryTimeout)
		}
		logger.Info("Waiting for CA file to be populated", "caFile", caFile, "retryIn", retryInterval)
		time.Sleep(retryInterval)
	}
}

// buildExternalHTTPClient constructs the HTTP client used for Prometheus/
// AlertManager enrichment calls (BR-EM-002, BR-EM-003). When
// cfg.External.TLSCaFile is set, it waits for the CA bundle (Issue #484),
// wires a hot-reloadable CA-trusting transport (Issue #756) wrapped with an
// SA bearer token (OCP monitoring endpoints), and returns a stop function
// for the CA watcher. When unset, it returns a plain client and a no-op stop
// function.
func buildExternalHTTPClient(ctx context.Context, cfg *config.Config, logger logr.Logger) (*http.Client, func(), error) {
	if cfg.External.TLSCaFile == "" {
		return &http.Client{Timeout: cfg.External.ConnectionTimeout}, func() {}, nil
	}

	const caRetryInterval = 2 * time.Second
	const caRetryTimeout = 30 * time.Second
	caPEM, err := waitForCAFile(cfg.External.TLSCaFile, caRetryInterval, caRetryTimeout, logger)
	if err != nil {
		return nil, nil, err
	}

	// Issue #756: Migrate to shared CAReloader for consistency
	caReloader, err := sharedtls.NewCAReloader(caPEM)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to initialize CA reloader for %q: %w", cfg.External.TLSCaFile, err)
	}

	emCAWatcher, err := hotreload.NewFileWatcher(
		cfg.External.TLSCaFile,
		caReloader.ReloadCallback,
		ctrl.Log.WithName("ca-reloader"),
	)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to create CA file watcher for %q: %w", cfg.External.TLSCaFile, err)
	}
	if err := emCAWatcher.Start(ctx); err != nil {
		return nil, nil, fmt.Errorf("failed to start CA file watcher for %q: %w", cfg.External.TLSCaFile, err)
	}

	// Wrap CAReloader (RoundTripper) with SA bearer token for OCP monitoring endpoints.
	saTransport := auth.NewAuthTransport(auth.NewDefaultTokenSource(), caReloader)
	httpClient := &http.Client{
		Transport: saTransport,
		Timeout:   cfg.External.ConnectionTimeout,
	}
	logger.Info("TLS HTTP client initialized with CA hot-reload and bearer token",
		"caFile", cfg.External.TLSCaFile,
		"timeout", cfg.External.ConnectionTimeout,
	)
	return httpClient, emCAWatcher.Stop, nil
}

// buildExternalClients constructs the optional Prometheus and AlertManager
// clients (BR-EM-002, BR-EM-003). Either return value is nil when the
// corresponding service is disabled in configuration.
func buildExternalClients(cfg *config.Config, httpClient *http.Client, logger logr.Logger) (emclient.PrometheusQuerier, emclient.AlertManagerClient) {
	var promClient emclient.PrometheusQuerier
	var amClient emclient.AlertManagerClient

	if cfg.External.PrometheusEnabled {
		promClient = emclient.NewPrometheusHTTPClient(cfg.External.PrometheusURL, httpClient)
		logger.Info("Prometheus HTTP client initialized",
			"url", cfg.External.PrometheusURL,
			"timeout", cfg.External.ConnectionTimeout,
		)
	} else {
		logger.Info("Prometheus disabled in configuration, metric comparison will be skipped")
	}

	if cfg.External.AlertManagerEnabled {
		amClient = emclient.NewAlertManagerHTTPClient(cfg.External.AlertManagerURL, httpClient)
		logger.Info("AlertManager HTTP client initialized",
			"url", cfg.External.AlertManagerURL,
			"timeout", cfg.External.ConnectionTimeout,
		)
	} else {
		logger.Info("AlertManager disabled in configuration, alert resolution check will be skipped")
	}

	return promClient, amClient
}

// registerHealthChecks wires the standard healthz/readyz probes.
func registerHealthChecks(mgr ctrl.Manager) error {
	if err := mgr.AddHealthzCheck("healthz", healthz.Ping); err != nil {
		return fmt.Errorf("unable to set up health check: %w", err)
	}
	if err := mgr.AddReadyzCheck("readyz", healthz.Ping); err != nil {
		return fmt.Errorf("unable to set up ready check: %w", err)
	}
	return nil
}

// wireHotReload applies the config-driven TLS security profile (Issue
// #748), starts the shared CA file watcher (Issue #756), and starts the
// config-driven log-level watcher (Issue #875). Callers must invoke the
// returned stop function on shutdown.
func wireHotReload(ctx context.Context, cfg *config.Config, configPath string, atomicLevel zaplog.AtomicLevel, logger logr.Logger) func() {
	stopFns := make([]func(), 0, 2)

	if err := sharedtls.SetDefaultSecurityProfileFromConfig(cfg.TLSProfile); err != nil {
		logger.Error(err, "Invalid TLS security profile in config, using default TLS 1.2")
	} else if cfg.TLSProfile != "" {
		logger.Info("TLS security profile active", "profile", cfg.TLSProfile)
	}

	caWatcher, caWatchErr := sharedtls.StartCAFileWatcher(ctx, logger)
	if caWatchErr != nil {
		logger.Error(caWatchErr, "Failed to start CA file watcher")
		os.Exit(1)
	}
	if caWatcher != nil {
		stopFns = append(stopFns, caWatcher.Stop)
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
		logger.WithName("log-level-watcher"),
	)
	if logWatchErr != nil {
		logger.Error(logWatchErr, "Failed to create log level file watcher")
	} else if err := logLevelWatcher.Start(ctx); err != nil {
		logger.Info("Log level file watcher failed to start", "error", err)
	} else {
		logger.Info("Log level hot-reload watcher started", "path", configPath)
		stopFns = append(stopFns, logLevelWatcher.Stop)
	}

	return func() {
		for _, stop := range stopFns {
			stop()
		}
	}
}
