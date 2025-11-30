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
	"net/http"
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
	"github.com/jordigilh/kubernaut/pkg/notification/delivery"
	"github.com/jordigilh/kubernaut/pkg/notification/sanitization"
	//+kubebuilder:scaffold:imports
)

var (
	scheme   = runtime.NewScheme()
	setupLog = ctrl.Log.WithName("setup")
)

func init() {
	utilruntime.Must(clientgoscheme.AddToScheme(scheme))

	utilruntime.Must(notificationv1alpha1.AddToScheme(scheme))
	//+kubebuilder:scaffold:scheme
}

func main() {
	var metricsAddr string
	var enableLeaderElection bool
	var probeAddr string

	flag.StringVar(&metricsAddr, "metrics-bind-address", ":9090", "The address the metric endpoint binds to.")
	flag.StringVar(&probeAddr, "health-probe-bind-address", ":8081", "The address the probe endpoint binds to.")
	flag.BoolVar(&enableLeaderElection, "leader-elect", false,
		"Enable leader election for controller manager. "+
			"Enabling this will ensure there is only one active controller manager.")
	opts := zap.Options{
		Development: true,
	}
	opts.BindFlags(flag.CommandLine)
	flag.Parse()

	ctrl.SetLogger(zap.New(zap.UseFlagOptions(&opts)))

	mgr, err := ctrl.NewManager(ctrl.GetConfigOrDie(), ctrl.Options{
		Scheme: scheme,
		Metrics: metricsserver.Options{
			BindAddress: metricsAddr,
		},
		HealthProbeBindAddress: probeAddr,
		LeaderElection:         enableLeaderElection,
		LeaderElectionID:       "notification.kubernaut.ai",
	})
	if err != nil {
		setupLog.Error(err, "unable to start manager")
		os.Exit(1)
	}

	// Initialize delivery services
	consoleService := delivery.NewConsoleDeliveryService()

	// TODO: Slack webhook URL should come from Kubernetes Secret in production
	// For now, we'll use an environment variable for development
	slackWebhookURL := os.Getenv("SLACK_WEBHOOK_URL")
	if slackWebhookURL == "" {
		setupLog.Info("SLACK_WEBHOOK_URL not set, Slack delivery will be disabled")
	}
	slackService := delivery.NewSlackDeliveryService(slackWebhookURL)

	// ========================================
	// DD-NOT-002 V3.0: E2E File Delivery Service (Testing Infrastructure)
	// Feature Flag: E2E_FILE_OUTPUT
	// ========================================
	// FileService writes notifications to JSON files for E2E test validation.
	// This is E2E testing infrastructure only and should NOT be used in production.
	//
	// SAFETY GUARANTEE (V3.0 Error Handling Philosophy):
	// - FileService failures do NOT block production notifications
	// - Controller checks `if r.FileService != nil` before delivery
	// - Production deployments have FileService = nil (feature flag disabled)
	//
	// Usage:
	//   export E2E_FILE_OUTPUT=/tmp/kubernaut-e2e-notifications
	//   make test-e2e-notification-files
	var fileService *delivery.FileDeliveryService
	e2eFileOutput := os.Getenv("E2E_FILE_OUTPUT")
	if e2eFileOutput != "" {
		setupLog.Info("E2E file delivery enabled (testing infrastructure only)",
			"outputDir", e2eFileOutput,
			"feature", "DD-NOT-002")
		fileService = delivery.NewFileDeliveryService(e2eFileOutput)
	}

	// Initialize data sanitization
	sanitizer := sanitization.NewSanitizer()

	// ========================================
	// v1.1: Initialize Audit Store for ADR-034 Integration
	// BR-NOT-062: Unified Audit Table Integration
	// BR-NOT-063: Graceful Audit Degradation
	// See: DD-NOT-001-ADR034-AUDIT-INTEGRATION-v2.0-FULL.md
	// ========================================

	// Get Data Storage Service URL from environment
	dataStorageURL := os.Getenv("DATA_STORAGE_URL")
	if dataStorageURL == "" {
		dataStorageURL = "http://datastorage-service.kubernaut.svc.cluster.local:8080"
		setupLog.Info("DATA_STORAGE_URL not set, using default", "url", dataStorageURL)
	}

	// Create HTTP client for Data Storage Service
	httpClient := &http.Client{
		Timeout: 5 * time.Second,
	}
	dataStorageClient := audit.NewHTTPDataStorageClient(dataStorageURL, httpClient)

	// Create buffered audit store (fire-and-forget pattern, ADR-038)
	auditConfig := audit.Config{
		BufferSize:    10000,           // In-memory buffer size
		BatchSize:     100,             // Batch size for Data Storage writes
		FlushInterval: 5 * time.Second, // Flush interval
		MaxRetries:    3,               // Max retry attempts for failed writes
	}

	// Create zap logger for audit store, then convert to logr.Logger via zapr adapter
	// DD-005 v2.0: pkg/audit uses logr.Logger for unified logging interface
	zapLogger, err := zaplog.NewProduction()
	if err != nil {
		setupLog.Error(err, "Failed to create zap logger for audit store")
		os.Exit(1)
	}
	auditLogger := zapr.NewLogger(zapLogger.Named("audit"))

	auditStore, err := audit.NewBufferedStore(dataStorageClient, auditConfig, "notification-controller", auditLogger)
	if err != nil {
		setupLog.Error(err, "Failed to create audit store")
		os.Exit(1)
	}

	// Create audit helpers
	auditHelpers := notification.NewAuditHelpers("notification-controller")

	setupLog.Info("Audit store initialized", "bufferSize", auditConfig.BufferSize, "batchSize", auditConfig.BatchSize)

	// Initialize metrics with zero values to ensure they appear in Prometheus immediately
	// This is critical for E2E metrics validation tests
	notification.UpdatePhaseCount("default", "Pending", 0)
	notification.RecordDeliveryAttempt("default", "console", "success")
	notification.RecordDeliveryDuration("default", "console", 0)
	setupLog.Info("Notification metrics initialized")

	// Setup controller with delivery services + sanitization + audit + E2E file service
	if err = (&notification.NotificationRequestReconciler{
		Client:         mgr.GetClient(),
		Scheme:         mgr.GetScheme(),
		ConsoleService: consoleService,
		SlackService:   slackService,
		FileService:    fileService, // NEW - E2E file delivery (DD-NOT-002)
		Sanitizer:      sanitizer,
		AuditStore:     auditStore,   // Audit store
		AuditHelpers:   auditHelpers, // Audit helpers
	}).SetupWithManager(mgr); err != nil {
		setupLog.Error(err, "unable to create controller", "controller", "NotificationRequest")
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

	// Setup signal handler for graceful shutdown
	ctx := ctrl.SetupSignalHandler()

	if err := mgr.Start(ctx); err != nil {
		setupLog.Error(err, "problem running manager")
		os.Exit(1)
	}

	// ========================================
	// Graceful Shutdown: Flush Audit Events (DD-007)
	// BR-NOT-063: Graceful Audit Degradation
	// ========================================
	setupLog.Info("Shutting down notification controller, flushing remaining audit events")
	if err := auditStore.Close(); err != nil {
		setupLog.Error(err, "Failed to close audit store gracefully")
		os.Exit(1)
	}
	setupLog.Info("Audit store closed successfully, all events flushed")
}
