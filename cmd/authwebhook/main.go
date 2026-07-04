package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"time"

	zap2 "go.uber.org/zap"
	"gopkg.in/yaml.v3"

	actiontypev1 "github.com/jordigilh/kubernaut/api/actiontype/v1alpha1"
	notificationv1 "github.com/jordigilh/kubernaut/api/notification/v1alpha1"
	remediationv1 "github.com/jordigilh/kubernaut/api/remediation/v1alpha1"
	remediationworkflowv1 "github.com/jordigilh/kubernaut/api/remediationworkflow/v1alpha1"
	workflowexecutionv1 "github.com/jordigilh/kubernaut/api/workflowexecution/v1alpha1"
	internalconfig "github.com/jordigilh/kubernaut/internal/config"
	"github.com/jordigilh/kubernaut/internal/version"
	"github.com/jordigilh/kubernaut/pkg/audit"
	"github.com/jordigilh/kubernaut/pkg/authwebhook"
	awconfig "github.com/jordigilh/kubernaut/pkg/authwebhook/config"
	"github.com/jordigilh/kubernaut/pkg/shared/hotreload"
	sharedtls "github.com/jordigilh/kubernaut/pkg/shared/tls"

	"k8s.io/apimachinery/pkg/runtime"
	utilruntime "k8s.io/apimachinery/pkg/util/runtime"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
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
	utilruntime.Must(workflowexecutionv1.AddToScheme(scheme))
	utilruntime.Must(remediationv1.AddToScheme(scheme))
	utilruntime.Must(notificationv1.AddToScheme(scheme))
	utilruntime.Must(remediationworkflowv1.AddToScheme(scheme))
	utilruntime.Must(actiontypev1.AddToScheme(scheme))
}

// loadAuthWebhookConfig loads the YAML config (or defaults, ADR-030),
// validates it, and applies the config-driven log level (Issue #876).
// Exits the process on any failure, matching main()'s original fail-fast
// behavior.
func loadAuthWebhookConfig(configPath string, atomicLevel zap2.AtomicLevel) *awconfig.Config {
	setupLog.Info("Starting AuthWebhook",
		"version", version.Version,
		"gitCommit", version.GitCommit,
		"buildDate", version.BuildDate,
	)

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

	if err := cfg.Validate(); err != nil {
		setupLog.Error(err, "Configuration validation failed")
		os.Exit(1)
	}

	atomicLevel.SetLevel(cfg.Logging.ZapLevel())
	setupLog.Info("Log level configured from config file", "level", cfg.Logging.Level)

	setupLog.Info("Webhook server configuration",
		"webhook_port", cfg.Webhook.Port,
		"cert_dir", cfg.Webhook.CertDir,
		"data_storage_url", cfg.DataStorage.URL)

	return cfg
}

// buildAuthWebhookManager creates the controller-runtime manager with the
// webhook server and health-probe bind address from cfg (metrics disabled
// per WEBHOOK_METRICS_TRIAGE.md), then registers the BR-WORKFLOW-007 field
// index used by the RemediationWorkflow handler to look up ActionType CRDs
// by spec.name. Exits the process on any failure, matching main()'s
// original fail-fast behavior.
func buildAuthWebhookManager(cfg *awconfig.Config) ctrl.Manager {
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

	if err := mgr.GetFieldIndexer().IndexField(
		context.Background(),
		&actiontypev1.ActionType{},
		".spec.name",
		func(obj client.Object) []string {
			at, ok := obj.(*actiontypev1.ActionType)
			if !ok || at.Spec.Name == "" {
				return nil
			}
			return []string{at.Spec.Name}
		},
	); err != nil {
		setupLog.Error(err, "failed to create field index on .spec.name for ActionType")
		os.Exit(1)
	}

	return mgr
}

// wireAuthWebhookAuditStore creates the real, buffered audit store
// (DD-WEBHOOK-003, ADR-030) and starts a background goroutine that flushes
// it when ctx is cancelled during graceful shutdown. Exits the process on
// any failure, matching main()'s original fail-fast behavior.
func wireAuthWebhookAuditStore(ctx context.Context, cfg *awconfig.Config) audit.AuditStore {
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

	return auditStore
}

// registerAuthWebhookHandlers wires the webhook admission handlers
// (WorkflowExecution, RemediationApprovalRequest, RemediationRequest,
// NotificationRequest DELETE, RemediationWorkflow, ActionType), the
// RemediationWorkflow finalizer reconciler (Issue #418), the startup
// reconciler (Issue #548, #1246), and the healthz/readyz checks. Exits the
// process on any failure, matching main()'s original fail-fast behavior.
func registerAuthWebhookHandlers(mgr ctrl.Manager, cfg *awconfig.Config, auditStore audit.AuditStore) {
	registerAuthWebhookCRUDHandlers(mgr, auditStore)
	registerAuthWebhookWorkflowHandlers(mgr, cfg, auditStore)

	if err := mgr.AddHealthzCheck("healthz", healthz.Ping); err != nil {
		setupLog.Error(err, "unable to set up health check")
		os.Exit(1)
	}
	if err := mgr.AddReadyzCheck("readyz", healthz.Ping); err != nil {
		setupLog.Error(err, "unable to set up ready check")
		os.Exit(1)
	}
	setupLog.Info("Registered health check endpoints", "liveness", "/healthz", "readiness", "/readyz")
}

// registerAuthWebhookCRUDHandlers wires the WorkflowExecution,
// RemediationApprovalRequest, RemediationRequest, and NotificationRequest
// DELETE admission handlers (DD-WEBHOOK-003, Gap #8). Exits the process on
// any failure, matching main()'s original fail-fast behavior.
func registerAuthWebhookCRUDHandlers(mgr ctrl.Manager, auditStore audit.AuditStore) {
	webhookServer := mgr.GetWebhookServer()
	decoder := admission.NewDecoder(scheme)

	wfeHandler := authwebhook.NewWorkflowExecutionAuthHandler(auditStore)
	if err := wfeHandler.InjectDecoder(decoder); err != nil {
		setupLog.Error(err, "failed to inject decoder into WorkflowExecution handler")
		os.Exit(1)
	}
	webhookServer.Register("/mutate-workflowexecution", &webhook.Admission{Handler: wfeHandler})
	setupLog.Info("Registered WorkflowExecution webhook handler with audit store")

	// I1: Use mgr.GetClient() (cached) for consistency with other webhook handlers
	// DD-AUTH-MCP-001 v3.0: Derive trusted AF SA from POD_NAMESPACE (K8s downward API)
	podNamespace := os.Getenv("POD_NAMESPACE")
	if podNamespace == "" {
		podNamespace = "kubernaut-system"
	}
	setupLog.Info("Trusted intermediary configured",
		"afSA", authwebhook.BuildTrustedAFSA(podNamespace),
		"derivedFrom", "POD_NAMESPACE",
	)
	rarHandler := authwebhook.NewRemediationApprovalRequestAuthHandler(auditStore, mgr.GetClient(), podNamespace)
	if err := rarHandler.InjectDecoder(decoder); err != nil {
		setupLog.Error(err, "failed to inject decoder into RemediationApprovalRequest handler")
		os.Exit(1)
	}
	webhookServer.Register("/mutate-remediationapprovalrequest", &webhook.Admission{Handler: rarHandler})
	setupLog.Info("Registered RemediationApprovalRequest webhook handler with audit store")

	// Gap #8: TimeoutConfig mutation audit.
	rrHandler := authwebhook.NewRemediationRequestStatusHandler(auditStore)
	if err := rrHandler.InjectDecoder(decoder); err != nil {
		setupLog.Error(err, "failed to inject decoder into RemediationRequest handler")
		os.Exit(1)
	}
	webhookServer.Register("/mutate-remediationrequest", &webhook.Admission{Handler: rrHandler})
	setupLog.Info("Registered RemediationRequest webhook handler with audit store (Gap #8)")

	// Note: This handler writes audit traces for DELETE attribution (K8s prevents object mutation during DELETE)
	nrHandler := authwebhook.NewNotificationRequestDeleteHandler(auditStore)
	if err := nrHandler.InjectDecoder(decoder); err != nil {
		setupLog.Error(err, "failed to inject decoder into NotificationRequest handler")
		os.Exit(1)
	}
	webhookServer.Register("/validate-notificationrequest-delete", &webhook.Admission{Handler: nrHandler})
	setupLog.Info("Registered NotificationRequest DELETE webhook handler with audit store")
}

// registerAuthWebhookWorkflowHandlers wires the RemediationWorkflow and
// ActionType admission handlers (ADR-058, ADR-059), the RemediationWorkflow
// finalizer reconciler (Issue #418), and the startup reconciler that syncs
// existing CRDs with DataStorage on boot (Issue #548, #1246). Exits the
// process on any failure, matching main()'s original fail-fast behavior.
func registerAuthWebhookWorkflowHandlers(mgr ctrl.Manager, cfg *awconfig.Config, auditStore audit.AuditStore) {
	webhookServer := mgr.GetWebhookServer()

	rwDSClient, err := authwebhook.NewDSClientAdapter(
		cfg.DataStorage.URL,
		cfg.DataStorage.Timeout,
		ctrl.Log.WithName("rw-ds-client"),
	)
	if err != nil {
		setupLog.Error(err, "failed to create DS client adapter for RemediationWorkflow handler")
		os.Exit(1)
	}
	rwHandler := authwebhook.NewRemediationWorkflowHandler(
		rwDSClient, auditStore, mgr.GetClient(),
		authwebhook.WithActionTypeWorkflowCounter(rwDSClient),
	)
	webhookServer.Register("/validate-remediationworkflow", &webhook.Admission{Handler: rwHandler})
	setupLog.Info("Registered RemediationWorkflow webhook handler with DS client and audit store")

	// Issue #418: Finalizer-based reconciler guarantees DS catalog consistency
	// on RW deletion (replaces fire-and-forget goroutines for the DELETE path).
	rwReconciler := &authwebhook.RemediationWorkflowReconciler{
		Client:    mgr.GetClient(),
		Log:       ctrl.Log.WithName("rw-reconciler"),
		DSClient:  rwDSClient,
		ATCounter: rwDSClient,
	}
	if err := rwReconciler.SetupWithManager(mgr); err != nil {
		setupLog.Error(err, "unable to create RemediationWorkflow reconciler")
		os.Exit(1)
	}
	setupLog.Info("Registered RemediationWorkflow finalizer reconciler")

	// ADR-059: CRD-based action type lifecycle. Reuses the same DS client
	// adapter since it connects to the same Data Storage service.
	atHandler := authwebhook.NewActionTypeHandler(rwDSClient, auditStore, mgr.GetClient())
	webhookServer.Register("/validate-actiontype", &webhook.Admission{Handler: atHandler})
	setupLog.Info("Registered ActionType webhook handler with DS client and audit store")

	// Issue #548: Startup reconciler syncs existing CRDs with DataStorage on boot.
	// Issue #1246: Graceful degradation — individual RW failures don't crash the pod.
	startupReconciler := &authwebhook.StartupReconciler{
		K8sClient:     mgr.GetClient(),
		DSWorkflow:    rwDSClient,
		DSActionType:  rwDSClient,
		Logger:        ctrl.Log.WithName("startup-reconciler"),
		Timeout:       cfg.DataStorage.Timeout,
		EventRecorder: mgr.GetEventRecorderFor("authwebhook-startup"),
		AuditStore:    auditStore,
	}
	if err := mgr.Add(startupReconciler); err != nil {
		setupLog.Error(err, "unable to add startup reconciler")
		os.Exit(1)
	}
	setupLog.Info("Registered startup reconciler (Issue #548: PVC-wipe resilience)")
}

// configureAuthWebhookTLSAndHotReload applies the OCP TLS security profile
// from config (Issue #748), starts the CA file watcher for client-side TLS
// hot-reload (Issue #756), and starts the log-level hot-reload watcher
// (Issue #876). Exits the process if the CA watcher fails to start,
// matching main()'s original fail-fast behavior. Returns a cleanup function
// that stops any watchers that were successfully started; callers should
// defer the returned function.
func configureAuthWebhookTLSAndHotReload(ctx context.Context, cfg *awconfig.Config, configPath string, atomicLevel zap2.AtomicLevel) func() {
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
	// ADR-030: YAML-based configuration via -config flag
	var configPath string
	flag.StringVar(&configPath, "config", awconfig.DefaultConfigPath, "Path to YAML configuration file (ADR-030)")
	flag.Parse()

	// Issue #876: Bootstrap logger at INFO for config loading
	atomicLevel := internalconfig.DefaultLoggingConfig().NewAtomicLevel()
	ctrl.SetLogger(zap.New(zap.Level(atomicLevel)))

	cfg := loadAuthWebhookConfig(configPath, atomicLevel)
	mgr := buildAuthWebhookManager(cfg)

	// Graceful shutdown: Flush audit store before exit
	ctx := ctrl.SetupSignalHandler()
	auditStore := wireAuthWebhookAuditStore(ctx, cfg)

	registerAuthWebhookHandlers(mgr, cfg, auditStore)

	cleanupHotReload := configureAuthWebhookTLSAndHotReload(ctx, cfg, configPath, atomicLevel)
	defer cleanupHotReload()

	setupLog.Info("Starting webhook server", "port", cfg.Webhook.Port)
	if err := mgr.Start(ctx); err != nil {
		setupLog.Error(err, "problem running manager")
		os.Exit(1)
	}
}
