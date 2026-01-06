package main

import (
	"context"
	"flag"
	"os"
	"strconv"

	notificationv1 "github.com/jordigilh/kubernaut/api/notification/v1alpha1"
	remediationv1 "github.com/jordigilh/kubernaut/api/remediation/v1alpha1"
	workflowexecutionv1 "github.com/jordigilh/kubernaut/api/workflowexecution/v1alpha1"
	"github.com/jordigilh/kubernaut/pkg/audit"
	"github.com/jordigilh/kubernaut/pkg/webhooks"

	"k8s.io/apimachinery/pkg/runtime"
	ctrl "sigs.k8s.io/controller-runtime"
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

// TODO: Implement proper audit manager that writes to Data Storage
// Currently using no-op implementation - handlers will be enhanced to write audit events per DD-WEBHOOK-003
type noOpAuditManager struct{}

func (n *noOpAuditManager) RecordEvent(ctx context.Context, event audit.AuditEvent) error {
	// TODO: Implement actual audit event writing to Data Storage
	// Per DD-WEBHOOK-003: Webhooks should write complete audit events (WHO + WHAT + ACTION)
	setupLog.Info("Audit event recorded (no-op)", "event_type", event.EventType, "actor_id", event.ActorID)
	return nil
}

func main() {
	var webhookPort int
	var certDir string

	// CLI flags with production defaults (per WEBHOOK_METRICS_TRIAGE.md)
	flag.IntVar(&webhookPort, "webhook-port", 9443, "The port the webhook server binds to.")
	flag.StringVar(&certDir, "cert-dir", "/tmp/k8s-webhook-server/serving-certs", "The directory containing TLS certificates.")
	flag.Parse()

	// Allow environment variable overrides
	if envPort := os.Getenv("WEBHOOK_PORT"); envPort != "" {
		if port, err := strconv.Atoi(envPort); err == nil {
			webhookPort = port
		}
	}

	ctrl.SetLogger(zap.New(zap.UseDevMode(true)))

	setupLog.Info("Webhook server configuration",
		"webhook_port", webhookPort,
		"cert_dir", certDir)

	// Create manager (NO METRICS - audit traces sufficient per WEBHOOK_METRICS_TRIAGE.md)
	mgr, err := ctrl.NewManager(ctrl.GetConfigOrDie(), ctrl.Options{
		Scheme: scheme,
		Metrics: metricsserver.Options{
			BindAddress: "0", // Disable metrics endpoint
		},
		WebhookServer: webhook.NewServer(webhook.Options{
			Port:    webhookPort,
			CertDir: certDir,
		}),
	})
	if err != nil {
		setupLog.Error(err, "unable to create manager")
		os.Exit(1)
	}

	// Get webhook server
	webhookServer := mgr.GetWebhookServer()

	// Create decoder for webhook handlers
	decoder := admission.NewDecoder(scheme)

	// Register WorkflowExecution handler
	// TODO: Enhance to write audit events per DD-WEBHOOK-003
	wfeHandler := webhooks.NewWorkflowExecutionAuthHandler()
	if err := wfeHandler.InjectDecoder(decoder); err != nil {
		setupLog.Error(err, "failed to inject decoder into WorkflowExecution handler")
		os.Exit(1)
	}
	webhookServer.Register("/mutate-workflowexecution", &webhook.Admission{Handler: wfeHandler})
	setupLog.Info("Registered WorkflowExecution webhook handler")

	// Register RemediationApprovalRequest handler
	// TODO: Enhance to write audit events per DD-WEBHOOK-003
	rarHandler := webhooks.NewRemediationApprovalRequestAuthHandler()
	if err := rarHandler.InjectDecoder(decoder); err != nil {
		setupLog.Error(err, "failed to inject decoder into RemediationApprovalRequest handler")
		os.Exit(1)
	}
	webhookServer.Register("/mutate-remediationapprovalrequest", &webhook.Admission{Handler: rarHandler})
	setupLog.Info("Registered RemediationApprovalRequest webhook handler")

	// Register NotificationRequest DELETE handler
	// Note: This handler writes audit traces for DELETE attribution (K8s prevents object mutation during DELETE)
	// TODO: Replace noOpAuditManager with real audit store per DD-WEBHOOK-003
	auditMgr := &noOpAuditManager{}
	nrHandler := webhooks.NewNotificationRequestDeleteHandler(auditMgr)
	if err := nrHandler.InjectDecoder(decoder); err != nil {
		setupLog.Error(err, "failed to inject decoder into NotificationRequest handler")
		os.Exit(1)
	}
	webhookServer.Register("/validate-notificationrequest-delete", &webhook.Admission{Handler: nrHandler})
	setupLog.Info("Registered NotificationRequest DELETE webhook handler")

	setupLog.Info("Starting webhook server", "port", webhookPort)
	if err := mgr.Start(ctrl.SetupSignalHandler()); err != nil {
		setupLog.Error(err, "problem running manager")
		os.Exit(1)
	}
}

