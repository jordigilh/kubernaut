package main

import (
	"context"
	"flag"
	"os"
	"strconv"
	"time"

	notificationv1 "github.com/jordigilh/kubernaut/api/notification/v1alpha1"
	remediationv1 "github.com/jordigilh/kubernaut/api/remediation/v1alpha1"
	workflowexecutionv1 "github.com/jordigilh/kubernaut/api/workflowexecution/v1alpha1"
	"github.com/jordigilh/kubernaut/pkg/audit"
	"github.com/jordigilh/kubernaut/pkg/webhooks"

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
	var webhookPort int
	var certDir string
	var dataStorageURL string

	// CLI flags with production defaults
	flag.IntVar(&webhookPort, "webhook-port", 9443, "The port the webhook server binds to.")
	flag.StringVar(&certDir, "cert-dir", "/tmp/k8s-webhook-server/serving-certs", "The directory containing TLS certificates.")
	flag.StringVar(&dataStorageURL, "data-storage-url", "http://datastorage-service:8080", "Data Storage service URL for audit events.")
	flag.Parse()

	// Allow environment variable overrides
	if envPort := os.Getenv("WEBHOOK_PORT"); envPort != "" {
		if port, err := strconv.Atoi(envPort); err == nil {
			webhookPort = port
		}
	}
	if envURL := os.Getenv("DATA_STORAGE_URL"); envURL != "" {
		dataStorageURL = envURL
	}

	ctrl.SetLogger(zap.New(zap.UseDevMode(true)))

	setupLog.Info("Webhook server configuration",
		"webhook_port", webhookPort,
		"cert_dir", certDir,
		"data_storage_url", dataStorageURL)

	// Create manager with health probe server
	// Note: Metrics disabled (audit traces sufficient per WEBHOOK_METRICS_TRIAGE.md)
	//       but health probes enabled on separate port for Kubernetes liveness/readiness
	mgr, err := ctrl.NewManager(ctrl.GetConfigOrDie(), ctrl.Options{
		Scheme: scheme,
		Metrics: metricsserver.Options{
			BindAddress: "0", // Disable metrics endpoint
		},
		HealthProbeBindAddress: ":8081", // Health probes on separate HTTP port
		WebhookServer: webhook.NewServer(webhook.Options{
			Port:    webhookPort,
			CertDir: certDir,
		}),
	})
	if err != nil {
		setupLog.Error(err, "unable to create manager")
		os.Exit(1)
	}

	// Create REAL audit store (DD-WEBHOOK-003: Webhooks write complete audit events)
	setupLog.Info("Initializing audit store", "data_storage_url", dataStorageURL)
	dsClient, err := audit.NewOpenAPIClientAdapter(dataStorageURL, 30*time.Second)
	if err != nil {
		setupLog.Error(err, "failed to create Data Storage client adapter")
		os.Exit(1)
	}

	auditConfig := audit.DefaultConfig()
	auditConfig.FlushInterval = 5 * time.Second // Production flush interval
	auditConfig.BufferSize = 1000               // Production buffer size

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
	wfeHandler := webhooks.NewWorkflowExecutionAuthHandler(auditStore)
	if err := wfeHandler.InjectDecoder(decoder); err != nil {
		setupLog.Error(err, "failed to inject decoder into WorkflowExecution handler")
		os.Exit(1)
	}
	webhookServer.Register("/mutate-workflowexecution", &webhook.Admission{Handler: wfeHandler})
	setupLog.Info("Registered WorkflowExecution webhook handler with audit store")

	// Register RemediationApprovalRequest handler (DD-WEBHOOK-003: Complete audit events)
	rarHandler := webhooks.NewRemediationApprovalRequestAuthHandler(auditStore)
	if err := rarHandler.InjectDecoder(decoder); err != nil {
		setupLog.Error(err, "failed to inject decoder into RemediationApprovalRequest handler")
		os.Exit(1)
	}
	webhookServer.Register("/mutate-remediationapprovalrequest", &webhook.Admission{Handler: rarHandler})
	setupLog.Info("Registered RemediationApprovalRequest webhook handler with audit store")

	// Register RemediationRequest status handler (Gap #8: TimeoutConfig mutation audit)
	rrHandler := webhooks.NewRemediationRequestStatusHandler(auditStore)
	if err := rrHandler.InjectDecoder(decoder); err != nil {
		setupLog.Error(err, "failed to inject decoder into RemediationRequest handler")
		os.Exit(1)
	}
	webhookServer.Register("/mutate-remediationrequest", &webhook.Admission{Handler: rrHandler})
	setupLog.Info("Registered RemediationRequest webhook handler with audit store (Gap #8)")

	// Register NotificationRequest DELETE handler (DD-WEBHOOK-003: Complete audit events)
	// Note: This handler writes audit traces for DELETE attribution (K8s prevents object mutation during DELETE)
	nrHandler := webhooks.NewNotificationRequestDeleteHandler(auditStore)
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

	setupLog.Info("Starting webhook server", "port", webhookPort)
	if err := mgr.Start(ctx); err != nil {
		setupLog.Error(err, "problem running manager")
		os.Exit(1)
	}
}
