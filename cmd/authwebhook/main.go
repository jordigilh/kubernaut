package main

import (
	"context"
	"flag"
	"os"
	"time"

	notificationv1 "github.com/jordigilh/kubernaut/api/notification/v1alpha1"
	remediationv1 "github.com/jordigilh/kubernaut/api/remediation/v1alpha1"
	workflowexecutionv1 "github.com/jordigilh/kubernaut/api/workflowexecution/v1alpha1"
	"github.com/jordigilh/kubernaut/pkg/audit"
	"github.com/jordigilh/kubernaut/pkg/authwebhook"
	awconfig "github.com/jordigilh/kubernaut/pkg/authwebhook/config"

	"k8s.io/apimachinery/pkg/runtime"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/healthz"
	"sigs.k8s.io/controller-runtime/pkg/log/zap"
	metricsserver "sigs.k8s.io/controller-runtime/pkg/metrics/server"
	"sigs.k8s.io/controller-runtime/pkg/webhook"
	"sigs.k8s.io/controller-runtime/pkg/webhook/admission"
)

var (
	scheme   = runtime.NewScheme()
	setupLog = ctrl.Log.WithName("setup")
)

func init() {
	_ = workflowexecutionv1.AddToScheme(scheme)
	_ = remediationv1.AddToScheme(scheme)
	_ = notificationv1.AddToScheme(scheme)
}

func main() {
	// ADR-030: YAML-based configuration via -config flag
	var configPath string
	flag.StringVar(&configPath, "config", "", "Path to YAML configuration file (ADR-030)")
	flag.Parse()

	ctrl.SetLogger(zap.New(zap.UseDevMode(true)))

	// Load configuration: YAML file if provided, defaults otherwise
	var cfg *awconfig.Config
	if configPath != "" {
		var err error
		cfg, err = awconfig.LoadFromFile(configPath)
		if err != nil {
			setupLog.Error(err, "Failed to load configuration file", "path", configPath)
			os.Exit(1)
		}
		setupLog.Info("Configuration loaded from file", "path", configPath)
	} else {
		cfg = awconfig.DefaultConfig()
		setupLog.Info("Using default configuration (no -config flag provided)")
	}

	// ADR-030: Validate configuration before startup
	if err := cfg.Validate(); err != nil {
		setupLog.Error(err, "Configuration validation failed")
		os.Exit(1)
	}

	setupLog.Info("Webhook server configuration",
		"webhook_port", cfg.Webhook.Port,
		"cert_dir", cfg.Webhook.CertDir,
		"data_storage_url", cfg.DataStorage.URL)

	// Create manager with health probe server
	// Note: Metrics disabled (audit traces sufficient per WEBHOOK_METRICS_TRIAGE.md)
	//       but health probes enabled on separate port for Kubernetes liveness/readiness
	mgr, err := ctrl.NewManager(ctrl.GetConfigOrDie(), ctrl.Options{
		Scheme: scheme,
		Metrics: metricsserver.Options{
			BindAddress: "0", // Disable metrics endpoint
		},
		HealthProbeBindAddress: cfg.Webhook.HealthProbeAddr,
		WebhookServer: webhook.NewServer(webhook.Options{
			Port:    cfg.Webhook.Port,
			CertDir: cfg.Webhook.CertDir,
		}),
	})
	if err != nil {
		setupLog.Error(err, "unable to create manager")
		os.Exit(1)
	}

	// Create REAL audit store (DD-WEBHOOK-003: Webhooks write complete audit events)
	// ADR-030: Use DataStorage URL and buffer config from YAML ConfigMap
	setupLog.Info("Initializing audit store", "data_storage_url", cfg.DataStorage.URL)
	dsClient, err := audit.NewOpenAPIClientAdapter(cfg.DataStorage.URL, cfg.DataStorage.Timeout)
	if err != nil {
		setupLog.Error(err, "failed to create Data Storage client adapter")
		os.Exit(1)
	}

	auditConfig := audit.Config{
		BufferSize:    cfg.DataStorage.Buffer.BufferSize,
		BatchSize:     cfg.DataStorage.Buffer.BatchSize,
		FlushInterval: cfg.DataStorage.Buffer.FlushInterval,
		MaxRetries:    cfg.DataStorage.Buffer.MaxRetries,
	}

	auditStore, err := audit.NewBufferedStore(
		dsClient,
		auditConfig,
		"authwebhook",
		ctrl.Log.WithName("audit"),
	)
	if err != nil {
		setupLog.Error(err, "failed to create audit store")
		os.Exit(1)
	}
	setupLog.Info("Audit store initialized", "service", "authwebhook", "buffer_size", auditConfig.BufferSize)

	// Graceful shutdown: Flush audit store before exit
	ctx := ctrl.SetupSignalHandler()
	go func() {
		<-ctx.Done()
		setupLog.Info("Flushing audit store before shutdown...")
		flushCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		if err := auditStore.Flush(flushCtx); err != nil {
			setupLog.Error(err, "failed to flush audit store during shutdown")
		} else {
			setupLog.Info("Audit store flushed successfully")
		}
	}()

	// Get webhook server
	webhookServer := mgr.GetWebhookServer()

	// Create decoder for webhook handlers
	decoder := admission.NewDecoder(scheme)

	// Register WorkflowExecution handler (DD-WEBHOOK-003: Complete audit events)
	wfeHandler := authwebhook.NewWorkflowExecutionAuthHandler(auditStore)
	if err := wfeHandler.InjectDecoder(decoder); err != nil {
		setupLog.Error(err, "failed to inject decoder into WorkflowExecution handler")
		os.Exit(1)
	}
	webhookServer.Register("/mutate-workflowexecution", &webhook.Admission{Handler: wfeHandler})
	setupLog.Info("Registered WorkflowExecution webhook handler with audit store")

	// Register RemediationApprovalRequest handler (DD-WEBHOOK-003: Complete audit events)
	rarHandler := authwebhook.NewRemediationApprovalRequestAuthHandler(auditStore)
	if err := rarHandler.InjectDecoder(decoder); err != nil {
		setupLog.Error(err, "failed to inject decoder into RemediationApprovalRequest handler")
		os.Exit(1)
	}
	webhookServer.Register("/mutate-remediationapprovalrequest", &webhook.Admission{Handler: rarHandler})
	setupLog.Info("Registered RemediationApprovalRequest webhook handler with audit store")

	// Register RemediationRequest status handler (Gap #8: TimeoutConfig mutation audit)
	rrHandler := authwebhook.NewRemediationRequestStatusHandler(auditStore)
	if err := rrHandler.InjectDecoder(decoder); err != nil {
		setupLog.Error(err, "failed to inject decoder into RemediationRequest handler")
		os.Exit(1)
	}
	webhookServer.Register("/mutate-remediationrequest", &webhook.Admission{Handler: rrHandler})
	setupLog.Info("Registered RemediationRequest webhook handler with audit store (Gap #8)")

	// Register NotificationRequest DELETE handler (DD-WEBHOOK-003: Complete audit events)
	// Note: This handler writes audit traces for DELETE attribution (K8s prevents object mutation during DELETE)
	nrHandler := authwebhook.NewNotificationRequestDeleteHandler(auditStore)
	if err := nrHandler.InjectDecoder(decoder); err != nil {
		setupLog.Error(err, "failed to inject decoder into NotificationRequest handler")
		os.Exit(1)
	}
	webhookServer.Register("/validate-notificationrequest-delete", &webhook.Admission{Handler: nrHandler})
	setupLog.Info("Registered NotificationRequest DELETE webhook handler with audit store")

	// Register health check endpoints for liveness and readiness probes
	// These are required by Kubernetes deployment health checks
	// Uses standard healthz.Ping checker (same pattern as other controllers)
	if err := mgr.AddHealthzCheck("healthz", healthz.Ping); err != nil {
		setupLog.Error(err, "unable to set up health check")
		os.Exit(1)
	}
	if err := mgr.AddReadyzCheck("readyz", healthz.Ping); err != nil {
		setupLog.Error(err, "unable to set up ready check")
		os.Exit(1)
	}
	setupLog.Info("Registered health check endpoints", "liveness", "/healthz", "readiness", "/readyz")

	setupLog.Info("Starting webhook server", "port", cfg.Webhook.Port)
	if err := mgr.Start(ctx); err != nil {
		setupLog.Error(err, "problem running manager")
		os.Exit(1)
	}
}
