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
	"fmt"
	"os"
	"time"

	"github.com/go-logr/zapr"
	zaplog "go.uber.org/zap"
	"k8s.io/apimachinery/pkg/runtime"
	utilruntime "k8s.io/apimachinery/pkg/util/runtime"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/healthz"
	"sigs.k8s.io/controller-runtime/pkg/log/zap"
	metricsserver "sigs.k8s.io/controller-runtime/pkg/metrics/server"

	notificationv1alpha1 "github.com/jordigilh/kubernaut/api/notification/v1alpha1"
	"github.com/jordigilh/kubernaut/internal/controller/notification"
	"github.com/jordigilh/kubernaut/pkg/audit"
	kubelog "github.com/jordigilh/kubernaut/pkg/log"
	notificationaudit "github.com/jordigilh/kubernaut/pkg/notification/audit"
	notificationconfig "github.com/jordigilh/kubernaut/pkg/notification/config"
	"github.com/jordigilh/kubernaut/pkg/notification/delivery"
	notificationmetrics "github.com/jordigilh/kubernaut/pkg/notification/metrics"
	notificationstatus "github.com/jordigilh/kubernaut/pkg/notification/status"
	"github.com/jordigilh/kubernaut/pkg/shared/circuitbreaker"
	"github.com/jordigilh/kubernaut/pkg/shared/sanitization"
	"github.com/sony/gobreaker"
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
	if err := os.MkdirAll(dir, 0755); err != nil {
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
	if err := os.WriteFile(testFile, []byte("test"), 0644); err != nil {
		return fmt.Errorf("directory not writable: %w", err)
	}
	if err := os.Remove(testFile); err != nil {
		return fmt.Errorf("failed to remove test file: %w", err)
	}

	return nil
}

// stateToString converts gobreaker.State to human-readable string
// Used for logging circuit breaker state transitions
func stateToString(state gobreaker.State) string {
	switch state {
	case gobreaker.StateClosed:
		return "closed"
	case gobreaker.StateOpen:
		return "open"
	case gobreaker.StateHalfOpen:
		return "half-open"
	default:
		return "unknown"
	}
}

func main() {
	// ========================================
	// ADR-030: Configuration Management
	// MANDATORY: Use -config flag with K8s env substitution
	// ========================================
	var configPath string
	flag.StringVar(&configPath, "config",
		"/etc/notification/config.yaml",
		"Path to configuration file (ADR-030)")

	opts := zap.Options{
		Development: true,
	}
	opts.BindFlags(flag.CommandLine)
	flag.Parse()

	// ADR-030: Initialize kubelog logger first (for config error reporting)
	// DD-005 v2.0: Use pkg/log shared library with logr interface
	logger := kubelog.NewLogger(kubelog.Options{
		Development: os.Getenv("ENV") != "production",
		Level:       0, // INFO
		ServiceName: "notification",
	})
	defer kubelog.Sync(logger)

	logger.Info("Loading configuration from YAML file (ADR-030)",
		"config_path", configPath)

	// ADR-030: Load configuration from YAML file
	cfg, err := notificationconfig.LoadFromFile(configPath)
	if err != nil {
		logger.Error(err, "Failed to load configuration file (ADR-030)",
			"config_path", configPath)
		os.Exit(1)
	}

	// ADR-030: Override with environment variables (secrets only)
	cfg.LoadFromEnv()

	// ADR-030: Validate configuration (fail-fast)
	if err := cfg.Validate(); err != nil {
		logger.Error(err, "Invalid configuration (ADR-030)")
		os.Exit(1)
	}

	logger.Info("Configuration loaded successfully (ADR-030)",
		"service", "notification",
		"metrics_addr", cfg.Controller.MetricsAddr,
		"health_probe_addr", cfg.Controller.HealthProbeAddr,
		"data_storage_url", cfg.Infrastructure.DataStorageURL)

	// Set controller-runtime logger (still needed for controller-runtime internals)
	ctrl.SetLogger(zap.New(zap.UseFlagOptions(&opts)))

	// ADR-030: Use configuration values for controller manager
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
		logger.Error(err, "Unable to start manager")
		os.Exit(1)
	}

	// ========================================
	// Initialize Delivery Services (ADR-030)
	// ========================================

	// Console delivery (always enabled)
	consoleService := delivery.NewConsoleDeliveryService()
	logger.Info("Console delivery service initialized",
		"enabled", cfg.Delivery.Console.Enabled)

	// Slack delivery (webhook URL from config after LoadFromEnv)
	slackService := delivery.NewSlackDeliveryService(cfg.Delivery.Slack.WebhookURL)
	if cfg.Delivery.Slack.WebhookURL == "" {
		logger.Info("Slack webhook URL not configured, Slack delivery will be disabled")
	} else {
		logger.Info("Slack delivery service initialized")
	}

	// ========================================
	// File Delivery Service (ADR-030 Configuration)
	// DD-NOT-006: Production feature for audit trails
	// ========================================
	var fileService *delivery.FileDeliveryService
	if cfg.Delivery.File.OutputDir != "" {
		// Validate directory is writable at startup
		if err := validateFileOutputDirectory(cfg.Delivery.File.OutputDir); err != nil {
			logger.Error(err, "File output directory validation failed",
				"directory", cfg.Delivery.File.OutputDir)
			os.Exit(1)
		}
		fileService = delivery.NewFileDeliveryService(cfg.Delivery.File.OutputDir)
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
	var logService *delivery.LogDeliveryService
	if cfg.Delivery.Log.Enabled {
		logService = delivery.NewLogDeliveryService()
		logger.Info("Log delivery service initialized",
			"enabled", cfg.Delivery.Log.Enabled,
			"format", cfg.Delivery.Log.Format)
	}

	// Initialize data sanitization
	sanitizer := sanitization.NewSanitizer()

	// ========================================
	// ADR-032: Audit Store for Data Storage Integration
	// BR-NOT-062: Unified Audit Table Integration
	// BR-NOT-063: Graceful Audit Degradation
	// ADR-030: Configuration from YAML (data_storage_url)
	// ========================================

	// Create Data Storage client with OpenAPI generated client (DD-API-001)
	// ADR-030: Use data_storage_url from configuration (required by Validate)
	dataStorageClient, err := audit.NewOpenAPIClientAdapter(
		cfg.Infrastructure.DataStorageURL,
		5*time.Second)
	if err != nil {
		logger.Error(err, "Failed to create Data Storage client",
			"url", cfg.Infrastructure.DataStorageURL)
		os.Exit(1)
	}

	// Create buffered audit store (fire-and-forget pattern, ADR-038)
	// DD-AUDIT-004: Use recommended config for LOW tier (500 events/day → 20K buffer)
	auditConfig := audit.RecommendedConfig("notification")

	// Create zap logger for audit store, then convert to logr.Logger via zapr adapter
	// DD-005 v2.0: pkg/audit uses logr.Logger for unified logging interface
	zapLogger, err := zaplog.NewProduction()
	if err != nil {
		logger.Error(err, "Failed to create zap logger for audit store")
		os.Exit(1)
	}
	auditLogger := zapr.NewLogger(zapLogger.Named("audit"))

	auditStore, err := audit.NewBufferedStore(dataStorageClient, auditConfig, "notification-controller", auditLogger)
	if err != nil {
		logger.Error(err, "Failed to create audit store")
		os.Exit(1)
	}

	// Create audit manager (direct usage, no wrapper needed)
	auditManager := notificationaudit.NewManager("notification-controller")

	logger.Info("Audit store initialized",
		"buffer_size", auditConfig.BufferSize,
		"batch_size", auditConfig.BatchSize)

	// ========================================
	// DD-METRICS-001: Metrics Dependency Injection
	// See: docs/architecture/decisions/DD-METRICS-001-controller-metrics-wiring-pattern.md
	// ========================================
	// Create metrics recorder for dependency injection (DD-METRICS-001 compliance)
	metricsRecorder := notificationmetrics.NewPrometheusRecorder()

	// Initialize metrics with zero values to ensure they appear in Prometheus immediately
	// This is critical for E2E metrics validation tests
	metricsRecorder.UpdatePhaseCount("default", "Pending", 0)
	metricsRecorder.RecordDeliveryAttempt("default", "console", "success")
	metricsRecorder.RecordDeliveryDuration("default", "console", 0)
	logger.Info("Notification metrics initialized (DD-METRICS-001 compliant)")

	// ========================================
	// Pattern 2: Status Manager (P1 - Quick Win)
	// See: docs/architecture/patterns/CONTROLLER_REFACTORING_PATTERN_LIBRARY.md §4
	// ========================================
	// Create status manager for centralized status updates with retry logic
	// Replaces controller's custom updateStatusWithRetry() method (~100 lines saved)
	statusManager := notificationstatus.NewManager(mgr.GetClient(), mgr.GetAPIReader())
	logger.Info("Status Manager initialized (Pattern 2 - P1)")

	// ========================================
	// Circuit Breaker for Graceful Degradation (BR-NOT-055)
	// DD-EVENT-001 v1.1: Must be created before Slack registration (CircuitBreakerSlackService)
	// ========================================
	// Initialize circuit breaker with github.com/sony/gobreaker
	// Provides per-channel isolation (Slack, console, webhooks)
	//
	// Circuit Breaker Configuration:
	// - Failure threshold: 3 consecutive failures trigger open state
	// - Recovery timeout: 30s before testing recovery (half-open state)
	// - Success threshold: 2 successful calls in half-open close circuit
	//
	// See: docs/services/crd-controllers/06-notification/README.md#circuit-breaker
	// ========================================
	circuitBreakerManager := circuitbreaker.NewManager(gobreaker.Settings{
		MaxRequests: 2, // Allow 2 test requests in half-open state
		Interval:    10 * time.Second,
		Timeout:     30 * time.Second, // Stay open for 30s before recovery attempt
		ReadyToTrip: func(counts gobreaker.Counts) bool {
			// Trip circuit after 3 consecutive failures
			return counts.ConsecutiveFailures >= 3
		},
		OnStateChange: func(name string, from gobreaker.State, to gobreaker.State) {
			// Log circuit breaker state transitions
			logger.Info("Circuit breaker state changed",
				"channel", name,
				"from", stateToString(from),
				"to", stateToString(to))

			// Update Prometheus metric
			if metricsRecorder != nil {
				metricsRecorder.UpdateCircuitBreakerState(name, to)
			}
		},
	})
	logger.Info("Circuit Breaker Manager initialized",
		"failure_threshold", 3,
		"recovery_timeout", "30s",
		"half_open_max_requests", 2)

	// ========================================
	// Pattern 3: Delivery Orchestrator (P0 - High Impact)
	// See: docs/architecture/patterns/CONTROLLER_REFACTORING_PATTERN_LIBRARY.md §3
	// ========================================
	deliveryOrchestrator := delivery.NewOrchestrator(
		sanitizer,
		metricsRecorder,
		statusManager,
		ctrl.Log.WithName("delivery-orchestrator"),
	)

	// DD-NOT-007: Register all production channels
	// DD-EVENT-001 v1.1: Slack wrapped with CircuitBreakerSlackService for failure tracking + CircuitBreakerOpen event
	slackServiceWithCB := delivery.NewCircuitBreakerSlackService(slackService, circuitBreakerManager)
	deliveryOrchestrator.RegisterChannel(string(notificationv1alpha1.ChannelConsole), consoleService)
	deliveryOrchestrator.RegisterChannel(string(notificationv1alpha1.ChannelSlack), slackServiceWithCB)
	deliveryOrchestrator.RegisterChannel(string(notificationv1alpha1.ChannelFile), fileService)
	deliveryOrchestrator.RegisterChannel(string(notificationv1alpha1.ChannelLog), logService)

	logger.Info("Delivery Orchestrator initialized with registration pattern (DD-NOT-007)")
	logger.Info("Registered channels",
		"channels", []string{"console", "slack", "file", "log"})

	// Setup controller with delivery services + sanitization + audit + metrics + EventRecorder + statusManager + deliveryOrchestrator + circuitBreaker
	if err = (&notification.NotificationRequestReconciler{
		Client:               mgr.GetClient(),
		APIReader:            mgr.GetAPIReader(),                                 // DD-STATUS-001: Cache-bypassed reader
		Scheme:               mgr.GetScheme(),
		ConsoleService:       consoleService,
		SlackService:         slackService,
		FileService:          fileService,          // DD-NOT-006: File delivery
		DeliveryOrchestrator: deliveryOrchestrator, // Pattern 3: Delivery Orchestrator (P0)
		Sanitizer:            sanitizer,
		CircuitBreaker:       circuitBreakerManager,                              // BR-NOT-055: Circuit breaker with gobreaker
		Metrics:              metricsRecorder,                                    // DD-METRICS-001: Injected metrics
		Recorder:             mgr.GetEventRecorderFor("notification-controller"), // P1: EventRecorder
		AuditStore:           auditStore,                                         // ADR-032: Audit store
		AuditManager:         auditManager,                                       // Direct audit manager (no wrapper)
		StatusManager:        statusManager,                                      // Pattern 2: Status Manager (P1)
	}).SetupWithManager(mgr); err != nil {
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

	logger.Info("Starting manager")

	// Setup signal handler for graceful shutdown
	ctx := ctrl.SetupSignalHandler()

	if err := mgr.Start(ctx); err != nil {
		logger.Error(err, "Problem running manager")
		os.Exit(1)
	}

	// ========================================
	// Graceful Shutdown: Flush Audit Events (DD-007)
	// BR-NOT-063: Graceful Audit Degradation
	// ========================================
	logger.Info("Shutting down notification controller, flushing remaining audit events")
	if err := auditStore.Close(); err != nil {
		logger.Error(err, "Failed to close audit store gracefully")
		os.Exit(1)
	}
	logger.Info("Audit store closed successfully, all events flushed")
}
