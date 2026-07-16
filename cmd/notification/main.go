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

	notificationv1alpha1 "github.com/jordigilh/kubernaut/api/notification/v1alpha1"
	internalconfig "github.com/jordigilh/kubernaut/internal/config"
	"github.com/jordigilh/kubernaut/internal/controller/notification"
	"github.com/jordigilh/kubernaut/internal/version"
	"github.com/jordigilh/kubernaut/pkg/audit"
	ogenclient "github.com/jordigilh/kubernaut/pkg/datastorage/ogen-client"
	kubelog "github.com/jordigilh/kubernaut/pkg/log"
	notificationaudit "github.com/jordigilh/kubernaut/pkg/notification/audit"
	notificationconfig "github.com/jordigilh/kubernaut/pkg/notification/config"
	"github.com/jordigilh/kubernaut/pkg/notification/credentials"
	"github.com/jordigilh/kubernaut/pkg/notification/delivery"
	"github.com/jordigilh/kubernaut/pkg/notification/enrichment"
	notificationmetrics "github.com/jordigilh/kubernaut/pkg/notification/metrics"
	notificationstatus "github.com/jordigilh/kubernaut/pkg/notification/status"
	"github.com/jordigilh/kubernaut/pkg/shared/auth"
	"github.com/jordigilh/kubernaut/pkg/shared/circuitbreaker"
	"github.com/jordigilh/kubernaut/pkg/shared/hotreload"
	"github.com/jordigilh/kubernaut/pkg/shared/sanitization"
	scope "github.com/jordigilh/kubernaut/pkg/shared/scope"
	sharedtls "github.com/jordigilh/kubernaut/pkg/shared/tls" // Issue #678: Inter-service TLS
	//+kubebuilder:scaffold:imports
)

var (
	scheme = runtime.NewScheme()
)

func init() {
	utilruntime.Must(clientgoscheme.AddToScheme(scheme))

	utilruntime.Must(notificationv1alpha1.AddToScheme(scheme))
	//+kubebuilder:scaffold:scheme
}

// validateFileOutputDirectory validates that the file output directory exists and is writable.
//
// TDD GREEN: Startup validation (R2 approved)
// Prevents runtime failures by validating directory at startup.
//
// Validation checks:
// - Creates directory if it doesn't exist (mkdir -p behavior)
// - Path is a directory (not a file)
// - Directory is writable (creates and removes a test file)
//
// Returns error if validation fails.
func validateFileOutputDirectory(dir string) error {
	// Create directory if it doesn't exist (mkdir -p behavior)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return fmt.Errorf("failed to create directory %s: %w", dir, err)
	}

	// Verify it's a directory (not a file)
	info, err := os.Stat(dir)
	if err != nil {
		return fmt.Errorf("failed to stat directory %s: %w", dir, err)
	}
	if !info.IsDir() {
		return fmt.Errorf("path is not a directory: %s", dir)
	}

	// Check it's writable (create temp file)
	testFile := dir + "/.write-test"
	if err := os.WriteFile(testFile, []byte("test"), 0o644); err != nil {
		return fmt.Errorf("directory not writable: %w", err)
	}
	if err := os.Remove(testFile); err != nil {
		return fmt.Errorf("failed to remove test file: %w", err)
	}

	return nil
}

// stateToString converts a circuit breaker State to a human-readable string.
func stateToString(state circuitbreaker.State) string {
	switch state {
	case circuitbreaker.StateClosed:
		return "closed"
	case circuitbreaker.StateOpen:
		return "open"
	case circuitbreaker.StateHalfOpen:
		return "half-open"
	default:
		return "unknown"
	}
}

// loadNotificationConfig loads and validates the notification controller
// config (ADR-030), discovers the controller namespace for CRD watch
// restriction (ADR-057), and reconfigures the logger at the config-driven
// log level (Issue #878). Exits the process on any failure, matching main()'s
// original fail-fast behavior.
func loadNotificationConfig(configPath string, bootstrapLogger logr.Logger) (*notificationconfig.Config, string, logr.Logger, zaplog.AtomicLevel) {
	bootstrapLogger.Info("Starting Notification Controller",
		"version", version.Version,
		"gitCommit", version.GitCommit,
		"buildDate", version.BuildDate,
	)

	bootstrapLogger.Info("Loading configuration from YAML file (ADR-030)",
		"config_path", configPath)

	// ADR-030: Load configuration from YAML file
	cfg, err := notificationconfig.LoadFromFile(configPath)
	if err != nil {
		bootstrapLogger.Error(err, "Failed to load configuration file (ADR-030)",
			"config_path", configPath)
		os.Exit(1)
	}

	// ADR-030: Validate configuration (fail-fast)
	if err := cfg.Validate(); err != nil {
		bootstrapLogger.Error(err, "Invalid configuration (ADR-030)")
		os.Exit(1)
	}

	// ADR-057: Discover controller namespace for CRD watch restriction
	controllerNS, err := scope.GetControllerNamespace()
	if err != nil {
		bootstrapLogger.Error(err, "Unable to determine controller namespace")
		os.Exit(1)
	}

	// Issue #878: Apply config-driven log level
	atomicLevel := cfg.Logging.NewAtomicLevel()
	logger := kubelog.NewLoggerWithAtomicLevel(kubelog.Options{
		ServiceName: "notification",
	}, atomicLevel)
	ctrl.SetLogger(zap.New(zap.Level(atomicLevel)))

	logger.Info("Configuration loaded successfully (ADR-030)",
		"service", "notification",
		"log_level", cfg.Logging.Level,
		"metrics_addr", cfg.Controller.MetricsAddr,
		"health_probe_addr", cfg.Controller.HealthProbeAddr,
		"data_storage_url", cfg.DataStorage.URL,
		"credentials_dir", cfg.Delivery.Credentials.Dir)

	return cfg, controllerNS, logger, atomicLevel
}

// setupNotificationReconciler builds the NotificationRequestReconciler with
// its delivery/sanitization/audit/metrics/EventRecorder/statusManager/
// deliveryOrchestrator/circuitBreaker dependencies, registers it with the
// manager, and wires the healthz/readyz checks. Exits the process on any
// failure, matching main()'s original fail-fast behavior.
func setupNotificationReconciler(
	mgr ctrl.Manager,
	cfg *notificationconfig.Config,
	ds *deliveryServices,
	auditStore audit.AuditStore,
	auditManager *notificationaudit.Manager,
	ob *orchestratorBundle,
	logger logr.Logger,
) *notification.NotificationRequestReconciler {
	reconciler := &notification.NotificationRequestReconciler{
		Client:               mgr.GetClient(),
		APIReader:            mgr.GetAPIReader(), // DD-STATUS-001: Cache-bypassed reader
		Scheme:               mgr.GetScheme(),
		ConsoleService:       ds.console,
		FileService:          ds.file,                    // DD-NOT-006: File delivery
		DeliveryOrchestrator: ob.orchestrator,            // Pattern 3: Delivery Orchestrator (P0)
		CredentialResolver:   ds.credResolver,            // BR-NOT-104: Per-receiver credential resolution
		DeliveryTimeout:      cfg.Delivery.Slack.Timeout, // HTTP timeout for webhook delivery channels
		Sanitizer:            ob.sanitizer,
		CircuitBreaker:       ob.circuitBreaker,                                  // BR-NOT-055: Circuit breaker with gobreaker
		Metrics:              ob.metrics,                                         // DD-METRICS-001: Injected metrics
		Recorder:             mgr.GetEventRecorderFor("notification-controller"), // P1: EventRecorder
		AuditStore:           auditStore,                                         // ADR-032: Audit store
		AuditManager:         auditManager,                                       // Direct audit manager (no wrapper)
		StatusManager:        ob.statusManager,                                   // Pattern 2: Status Manager (P1)
	}
	if err := reconciler.SetupWithManager(mgr); err != nil {
		logger.Error(err, "Unable to create controller", "controller", "NotificationRequest")
		os.Exit(1)
	}
	//+kubebuilder:scaffold:builder

	if err := mgr.AddHealthzCheck("healthz", healthz.Ping); err != nil {
		logger.Error(err, "Unable to set up health check")
		os.Exit(1)
	}
	if err := mgr.AddReadyzCheck("readyz", healthz.Ping); err != nil {
		logger.Error(err, "Unable to set up ready check")
		os.Exit(1)
	}

	return reconciler
}

func main() {
	// gocritic:exitAfterDefer — run() returns an exit code instead of calling
	// os.Exit directly so deferred cleanup (kubelog.Sync, stopHotReload)
	// always runs.
	os.Exit(run())
}

func run() int {
	// ========================================
	// ADR-030: Configuration Management
	// MANDATORY: Use -config flag with K8s env substitution
	// ========================================
	var configPath string
	flag.StringVar(&configPath, "config",
		"/etc/notification/config.yaml",
		"Path to configuration file (ADR-030)")
	flag.Parse()

	// Bootstrap logger at INFO for config loading
	bootstrapLevel := internalconfig.DefaultLoggingConfig().NewAtomicLevel()
	bootstrapLogger := kubelog.NewLoggerWithAtomicLevel(kubelog.Options{
		ServiceName: "notification",
	}, bootstrapLevel)
	defer kubelog.Sync(bootstrapLogger)

	cfg, controllerNS, logger, atomicLevel := loadNotificationConfig(configPath, bootstrapLogger)

	// ADR-030: Use configuration values for controller manager
	// #244: ConfigMap cache removed — routing config now loaded via FileWatcher
	mgr, err := buildManager(cfg, controllerNS)
	if err != nil {
		logger.Error(err, "Unable to start manager")
		return 1
	}

	// ========================================
	// Initialize Delivery Services (ADR-030)
	// ========================================
	ds, err := buildDeliveryServices(cfg, logger)
	if err != nil {
		logger.Error(err, "Failed to initialize delivery services")
		return 1
	}

	// Initialize data sanitization
	sanitizer := sanitization.NewSanitizer()

	// ========================================
	// ADR-032: Audit Store for Data Storage Integration
	// BR-NOT-062: Unified Audit Table Integration
	// BR-NOT-063: Graceful Audit Degradation
	// ADR-030: Configuration from YAML (data_storage_url)
	// ========================================
	auditStore, err := buildAuditStore(cfg)
	if err != nil {
		logger.Error(err, "Failed to create audit store")
		return 1
	}

	// Create audit manager (direct usage, no wrapper needed)
	auditManager := notificationaudit.NewManager("notification-controller")

	logger.Info("Audit store initialized",
		"buffer_size", cfg.DataStorage.Buffer.BufferSize,
		"batch_size", cfg.DataStorage.Buffer.BatchSize)

	// ========================================
	// DD-METRICS-001 / Pattern 2 / BR-NOT-055 / Pattern 3 / #553:
	// Metrics, Status Manager, Circuit Breaker, Delivery Orchestrator,
	// channel registration, and workflow-name enrichment.
	// ========================================
	ob, err := buildDeliveryOrchestrator(mgr, cfg, ds, sanitizer, controllerNS, logger)
	if err != nil {
		logger.Error(err, "Failed to build delivery orchestrator")
		return 1
	}

	reconciler := setupNotificationReconciler(mgr, cfg, ds, auditStore, auditManager, ob, logger)

	logger.Info("Starting manager")

	// Setup signal handler for graceful shutdown
	ctx := ctrl.SetupSignalHandler()

	// BR-NOT-104-002/#244/#878/#748/#756: credential, routing, log-level
	// hot-reload watchers plus TLS security profile + CA-cert hot-reload.
	stopHotReload := wireHotReload(ctx, hotReloadParams{
		cfg:          cfg,
		configPath:   configPath,
		atomicLevel:  atomicLevel,
		credResolver: ds.credResolver,
		reconciler:   reconciler,
	}, logger)
	defer stopHotReload()

	if err := mgr.Start(ctx); err != nil {
		logger.Error(err, "Problem running manager")
		return 1
	}

	// ========================================
	// Graceful Shutdown: Flush Audit Events (DD-007)
	// BR-NOT-063: Graceful Audit Degradation
	// ========================================
	logger.Info("Shutting down notification controller, flushing remaining audit events")
	if err := auditStore.Close(); err != nil {
		logger.Error(err, "Failed to close audit store gracefully")
		return 1
	}
	logger.Info("Audit store closed successfully, all events flushed")
	return 0
}

// buildManager constructs the controller-runtime manager with the
// namespace-restricted NotificationRequest cache (ADR-057) and
// metrics/health-probe/leader election settings from cfg. Extracted from
// main() (GO-ANTIPATTERN-AUDIT-2026-07-01 Wave 0a) — pure code motion, no
// behavior change.
func buildManager(cfg *notificationconfig.Config, controllerNS string) (ctrl.Manager, error) {
	return ctrl.NewManager(ctrl.GetConfigOrDie(), ctrl.Options{
		Scheme: scheme,
		Cache: cache.Options{
			ByObject: map[client.Object]cache.ByObject{
				&notificationv1alpha1.NotificationRequest{}: {
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

// deliveryServices groups the notification delivery channels + credential
// resolver built at startup (Options-pattern result struct, AGENTS.md's
// 8+-param rule mirrored for return values to avoid a 4-value return).
type deliveryServices struct {
	console      *delivery.ConsoleDeliveryService
	file         *delivery.FileDeliveryService
	log          *delivery.LogDeliveryService
	credResolver *credentials.Resolver
}

// buildDeliveryServices constructs the console (always-on), file (DD-NOT-006),
// and log (BR-NOT-053) delivery services, plus the BR-NOT-104 per-receiver
// credential resolver (degrades gracefully to nil — Slack delivery disabled
// — rather than failing startup). Extracted from main()
// (GO-ANTIPATTERN-AUDIT-2026-07-01 Wave 0a) — pure code motion, no behavior
// change.
func buildDeliveryServices(cfg *notificationconfig.Config, logger logr.Logger) (*deliveryServices, error) {
	ds := &deliveryServices{
		console: delivery.NewConsoleDeliveryService(),
	}
	logger.Info("Console delivery service initialized",
		"enabled", cfg.Delivery.Console.Enabled)

	// BR-NOT-104: Initialize credential resolver for per-receiver Slack delivery
	credResolver, err := credentials.NewResolver(cfg.Delivery.Credentials.Dir, logger)
	if err != nil {
		logger.Info("Credential resolver initialization failed (Slack delivery disabled until credentials available)",
			"error", err,
			"dir", cfg.Delivery.Credentials.Dir)
	} else {
		ds.credResolver = credResolver
		logger.Info("Credential resolver initialized",
			"dir", cfg.Delivery.Credentials.Dir,
			"credentialCount", credResolver.Count())
	}

	// ========================================
	// File Delivery Service (ADR-030 Configuration)
	// DD-NOT-006: Production feature for audit trails
	// ========================================
	if cfg.Delivery.File.OutputDir != "" {
		// Validate directory is writable at startup
		if err := validateFileOutputDirectory(cfg.Delivery.File.OutputDir); err != nil {
			return nil, fmt.Errorf("file output directory validation failed (directory=%s): %w",
				cfg.Delivery.File.OutputDir, err)
		}
		ds.file = delivery.NewFileDeliveryService(cfg.Delivery.File.OutputDir, cfg.Delivery.File.Format, cfg.Delivery.File.Timeout)
		logger.Info("File delivery service initialized",
			"output_dir", cfg.Delivery.File.OutputDir,
			"format", cfg.Delivery.File.Format,
			"timeout", cfg.Delivery.File.Timeout)
	}

	// ========================================
	// Log Delivery Service (ADR-030 Configuration)
	// DD-NOT-006: Production feature for observability
	// BR-NOT-053: Structured log delivery to stdout
	// ========================================
	if cfg.Delivery.Log.Enabled {
		ds.log = delivery.NewLogDeliveryService(cfg.Delivery.Log.Format)
		logger.Info("Log delivery service initialized",
			"enabled", cfg.Delivery.Log.Enabled,
			"format", cfg.Delivery.Log.Format)
	}

	return ds, nil
}

// buildAuditStore constructs the DataStorage-backed, buffered audit store
// (ADR-032, BR-NOT-062, BR-NOT-063, ADR-038) used for fire-and-forget audit
// event emission. Extracted from main() (GO-ANTIPATTERN-AUDIT-2026-07-01
// Wave 0a) — pure code motion, no behavior change.
func buildAuditStore(cfg *notificationconfig.Config) (audit.AuditStore, error) {
	// Create Data Storage client with OpenAPI generated client (DD-API-001)
	// ADR-030: Use data_storage_url from configuration (required by Validate)
	dataStorageClient, err := audit.NewOpenAPIClientAdapter(
		cfg.DataStorage.URL,
		cfg.DataStorage.Timeout)
	if err != nil {
		return nil, fmt.Errorf("failed to create data storage client (url=%s): %w", cfg.DataStorage.URL, err)
	}

	// Create buffered audit store (fire-and-forget pattern, ADR-038)
	// ADR-030: Use buffer config from YAML ConfigMap
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
		return nil, fmt.Errorf("failed to create zap logger for audit store: %w", err)
	}
	auditLogger := zapr.NewLogger(zapLogger.Named("audit"))

	auditStore, err := audit.NewBufferedStore(dataStorageClient, auditConfig, "notification-controller", auditLogger)
	if err != nil {
		return nil, fmt.Errorf("failed to create buffered audit store: %w", err)
	}
	return auditStore, nil
}

// orchestratorBundle groups the delivery orchestrator and its collaborators
// built together in buildDeliveryOrchestrator (Options-pattern result
// struct, mirrors deliveryServices above).
type orchestratorBundle struct {
	orchestrator   *delivery.Orchestrator
	metrics        *notificationmetrics.Metrics
	statusManager  *notificationstatus.Manager
	circuitBreaker *circuitbreaker.Manager
	sanitizer      *sanitization.Sanitizer
}

// buildDeliveryOrchestrator wires DD-METRICS-001 metrics, the Pattern-2
// status manager, the BR-NOT-055 per-channel circuit breaker, the Pattern-3
// delivery orchestrator (with DD-NOT-007 startup channel registration), and
// the #553 DataStorage-backed workflow-name enricher. Extracted from main()
// (GO-ANTIPATTERN-AUDIT-2026-07-01 Wave 0a) — pure code motion, no behavior
// change.
func buildDeliveryOrchestrator(
	mgr ctrl.Manager,
	cfg *notificationconfig.Config,
	ds *deliveryServices,
	sanitizer *sanitization.Sanitizer,
	controllerNS string,
	logger logr.Logger,
) (*orchestratorBundle, error) {
	// Create metrics recorder for dependency injection (DD-METRICS-001 compliance)
	metricsRecorder := notificationmetrics.NewMetrics()

	// Initialize metrics with zero values to ensure they appear in Prometheus immediately
	// This is critical for E2E metrics validation tests
	metricsRecorder.UpdatePhaseCount(controllerNS, "Pending", 0)
	metricsRecorder.RecordDeliveryAttempt(controllerNS, "console", "success")
	metricsRecorder.RecordDeliveryDuration(controllerNS, "console", 0)
	logger.Info("Notification metrics initialized (DD-METRICS-001 compliant)")

	// Create status manager for centralized status updates with retry logic
	// Replaces controller's custom updateStatusWithRetry() method (~100 lines saved)
	statusManager := notificationstatus.NewManager(mgr.GetClient(), mgr.GetAPIReader())
	logger.Info("Status Manager initialized (Pattern 2 - P1)")

	// Initialize circuit breaker with github.com/sony/gobreaker
	// Provides per-channel isolation (Slack, console, webhooks)
	//
	// Circuit Breaker Configuration:
	// - Failure threshold: 3 consecutive failures trigger open state
	// - Recovery timeout: 30s before testing recovery (half-open state)
	// - Success threshold: 2 successful calls in half-open close circuit
	//
	// See: docs/services/crd-controllers/06-notification/README.md#circuit-breaker
	circuitBreakerManager := circuitbreaker.NewManager(circuitbreaker.ManagerConfig{
		MaxRequests:                 2,
		Interval:                    10 * time.Second,
		Timeout:                     30 * time.Second,
		ConsecutiveFailureThreshold: 3,
		OnStateChange: func(name string, from, to circuitbreaker.State) {
			logger.Info("Circuit breaker state changed",
				"channel", name,
				"from", stateToString(from),
				"to", stateToString(to))

			if metricsRecorder != nil {
				metricsRecorder.UpdateCircuitBreakerState(name, int(to))
			}
		},
	})
	logger.Info("Circuit Breaker Manager initialized",
		"failure_threshold", 3,
		"recovery_timeout", "30s",
		"half_open_max_requests", 2)

	// Pattern 3: Delivery Orchestrator (P0 - High Impact)
	deliveryOrchestrator := delivery.NewOrchestrator(
		sanitizer,
		metricsRecorder,
		statusManager,
		ctrl.Log.WithName("delivery-orchestrator"),
	)

	// DD-NOT-007: Register non-credential channels at startup
	// BR-NOT-104: Slack channels registered per-receiver on routing config load (not at startup)
	deliveryOrchestrator.RegisterChannel(string(notificationv1alpha1.ChannelConsole), ds.console)
	deliveryOrchestrator.RegisterChannel(string(notificationv1alpha1.ChannelFile), ds.file)
	deliveryOrchestrator.RegisterChannel(string(notificationv1alpha1.ChannelLog), ds.log)

	// #244: Per-receiver Slack channels are registered dynamically by
	// ReloadRoutingFromContent (triggered by FileWatcher).
	startupChannels := []string{"console", "file", "log"}
	logger.Info("Delivery Orchestrator initialized with registration pattern (DD-NOT-007)")
	logger.Info("Registered startup channels (per-receiver Slack registered on routing config load)",
		"channels", startupChannels)

	if err := wireNotificationEnrichment(cfg, deliveryOrchestrator, logger); err != nil {
		return nil, err
	}

	return &orchestratorBundle{
		orchestrator:   deliveryOrchestrator,
		metrics:        metricsRecorder,
		statusManager:  statusManager,
		circuitBreaker: circuitBreakerManager,
		sanitizer:      sanitizer,
	}, nil
}

// wireNotificationEnrichment sets up the #553 workflow-name enrichment
// pipeline, which resolves workflow UUIDs to human-readable names in
// notification bodies before delivery via the DataStorage catalog API
// (Issue #853: RetryTransport-wrapped for transient failure resilience).
func wireNotificationEnrichment(cfg *notificationconfig.Config, deliveryOrchestrator *delivery.Orchestrator, logger logr.Logger) error {
	dsBaseTransport, err := sharedtls.DefaultBaseTransportWithRetry()
	if err != nil {
		return fmt.Errorf("failed to create TLS-aware base transport for DS enrichment client: %w", err)
	}
	dsOgenClient, err := ogenclient.NewClient(
		cfg.DataStorage.URL,
		ogenclient.WithClient(&http.Client{
			Timeout:   cfg.DataStorage.Timeout,
			Transport: auth.NewAuthTransport(auth.NewDefaultTokenSource(), dsBaseTransport),
		}),
	)
	if err != nil {
		return fmt.Errorf("failed to create datastorage ogen client for workflow name resolution: %w", err)
	}

	dsResolver := enrichment.NewDataStorageResolver(dsOgenClient, ctrl.Log.WithName("workflow-resolver"))
	notifEnricher := enrichment.NewEnricher(dsResolver, ctrl.Log.WithName("notification-enricher"))
	deliveryOrchestrator.SetEnricher(notifEnricher)
	logger.Info("Notification enricher initialized (#553: workflow name resolution)")

	return nil
}

// hotReloadParams groups wireHotReload's dependencies (Options pattern,
// AGENTS.md's 8+-param rule).
type hotReloadParams struct {
	cfg          *notificationconfig.Config
	configPath   string
	atomicLevel  zaplog.AtomicLevel
	credResolver *credentials.Resolver
	reconciler   *notification.NotificationRequestReconciler
}

// wireHotReload starts the BR-NOT-104-002 credential watcher, the #244
// routing-config watcher, the #878 log-level watcher, and the #748/#756 TLS
// security profile + CA-cert watcher, in the same startup order as the
// original inline code (credentials → routing → log-level → TLS/CA).
// Returns a combined stop function the caller should defer. Exits the
// process if the routing watcher fails to start, matching main()'s original
// fail-fast behavior (routing config is required for delivery). Extracted
// from main() (GO-ANTIPATTERN-AUDIT-2026-07-01 Wave 0a) — pure code motion,
// no behavior change.
func wireHotReload(ctx context.Context, p hotReloadParams, logger logr.Logger) func() {
	stopFns := make([]func(), 0, 4)

	if stop, ok := startCredentialWatcher(ctx, p.credResolver, logger); ok {
		stopFns = append(stopFns, stop)
	}
	stopFns = append(stopFns, startRoutingWatcher(ctx, p.reconciler, logger))
	if stop, ok := startNotificationLogLevelWatcher(ctx, p.configPath, p.atomicLevel, logger); ok {
		stopFns = append(stopFns, stop)
	}
	if stop, ok := startNotificationCAWatcher(ctx, p.cfg, logger); ok {
		stopFns = append(stopFns, stop)
	}

	return func() {
		for _, stop := range stopFns {
			stop()
		}
	}
}

// startCredentialWatcher starts the BR-NOT-104-002 per-receiver credential
// hot-reload watcher. ok is false when no watcher is configured
// (credResolver is nil).
func startCredentialWatcher(ctx context.Context, credResolver *credentials.Resolver, logger logr.Logger) (stop func(), ok bool) {
	if credResolver == nil {
		return nil, false
	}

	if err := credResolver.StartWatching(ctx); err != nil {
		logger.Error(err, "Failed to start credential watcher (hot-reload disabled)")
	} else {
		logger.Info("Credential file watcher started (BR-NOT-104-002)")
	}
	return func() {
		if err := credResolver.Close(); err != nil {
			logger.Error(err, "Failed to close credential resolver")
		}
	}, true
}

// startRoutingWatcher starts the #244 FileWatcher for routing config
// hot-reload (replaces the ConfigMap informer). Exits the process if the
// watcher fails to create or start, matching main()'s original fail-fast
// behavior (routing config is required for delivery).
func startRoutingWatcher(ctx context.Context, reconciler *notification.NotificationRequestReconciler, logger logr.Logger) func() {
	routingConfigPath := "/etc/notification-routing/routing.yaml"
	if envPath := os.Getenv("ROUTING_CONFIG_PATH"); envPath != "" {
		routingConfigPath = envPath
	}
	routingWatcher, err := hotreload.NewFileWatcher(
		routingConfigPath,
		func(newContent string) error {
			return reconciler.ReloadRoutingFromContent(newContent)
		},
		ctrl.Log.WithName("routing-watcher"),
	)
	if err != nil {
		logger.Error(err, "Failed to create routing file watcher")
		os.Exit(1)
	}
	if err := routingWatcher.Start(ctx); err != nil {
		logger.Error(err, "Routing file watcher failed to start -- aborting startup")
		os.Exit(1)
	}
	logger.Info("Routing file watcher started (#244)", "path", routingConfigPath)
	return routingWatcher.Stop
}

// startNotificationLogLevelWatcher starts the Issue #878 config-file
// log-level hot-reload watcher. ok is false when the watcher failed to
// create or start.
func startNotificationLogLevelWatcher(ctx context.Context, configPath string, atomicLevel zaplog.AtomicLevel, logger logr.Logger) (stop func(), ok bool) {
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
		return nil, false
	}

	if err := logLevelWatcher.Start(ctx); err != nil {
		logger.Info("Log level file watcher failed to start", "error", err)
		return nil, false
	}

	logger.Info("Log level hot-reload watcher started", "path", configPath)
	return logLevelWatcher.Stop, true
}

// startNotificationCAWatcher applies the Issue #748 OCP TLS security profile
// and starts the Issue #756 CA-cert hot-reload watcher for client-side TLS.
// Exits the process if the CA watcher fails to start, matching main()'s
// original fail-fast behavior. ok is false when no watcher was started.
func startNotificationCAWatcher(ctx context.Context, cfg *notificationconfig.Config, logger logr.Logger) (stop func(), ok bool) {
	// Issue #748: Load OCP TLS security profile from config before any TLS setup
	if err := sharedtls.SetDefaultSecurityProfileFromConfig(cfg.TLSProfile); err != nil {
		logger.Error(err, "Invalid TLS security profile in config, using default TLS 1.2")
	} else if cfg.TLSProfile != "" {
		logger.Info("TLS security profile active", "profile", cfg.TLSProfile)
	}

	// Issue #756: Start CA file watcher for client-side TLS hot-reload
	caWatcher, caWatchErr := sharedtls.StartCAFileWatcher(ctx, logger)
	if caWatchErr != nil {
		logger.Error(caWatchErr, "Failed to start CA file watcher")
		os.Exit(1)
	}
	if caWatcher == nil {
		return nil, false
	}
	return caWatcher.Stop, true
}
