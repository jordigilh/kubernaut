// Remediation Orchestrator Controller - Entry Point
//
// This is the main entry point for the Remediation Orchestrator CRD controller.
// It coordinates the entire remediation lifecycle by creating and monitoring
// child CRDs (SignalProcessing, AIAnalysis, WorkflowExecution, NotificationRequest).
//
// Business Requirements:
// - BR-ORCH-001: Approval notification creation
// - BR-ORCH-025: Workflow data pass-through
// - BR-ORCH-026: Approval orchestration
// - BR-ORCH-027, BR-ORCH-028: Timeout management
// - BR-ORCH-029-031: Notification handling
// - BR-ORCH-032-034: Resource lock deduplication handling
//
// Port Allocation (DD-TEST-001):
// - E2E NodePort: 30083 (host: 8083)
// - Metrics NodePort: 30183 (host: 9183)
package main

import (
	"flag"
	"os"

	"go.uber.org/zap/zapcore"
	"k8s.io/apimachinery/pkg/runtime"
	utilruntime "k8s.io/apimachinery/pkg/util/runtime"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/healthz"
	"sigs.k8s.io/controller-runtime/pkg/log/zap"
	metricsserver "sigs.k8s.io/controller-runtime/pkg/metrics/server"

	remediationv1 "github.com/jordigilh/kubernaut/api/remediation/v1alpha1"
	signalprocessingv1 "github.com/jordigilh/kubernaut/api/signalprocessing/v1alpha1"
	aianalysisv1 "github.com/jordigilh/kubernaut/api/aianalysis/v1alpha1"
	workflowexecutionv1 "github.com/jordigilh/kubernaut/api/workflowexecution/v1alpha1"
	notificationv1 "github.com/jordigilh/kubernaut/api/notification/v1alpha1"
	"github.com/jordigilh/kubernaut/pkg/remediationorchestrator"
	"github.com/jordigilh/kubernaut/pkg/remediationorchestrator/controller"
)

var (
	scheme   = runtime.NewScheme()
	setupLog = ctrl.Log.WithName("setup")
)

func init() {
	utilruntime.Must(clientgoscheme.AddToScheme(scheme))
	utilruntime.Must(remediationv1.AddToScheme(scheme))
	utilruntime.Must(signalprocessingv1.AddToScheme(scheme))
	utilruntime.Must(aianalysisv1.AddToScheme(scheme))
	utilruntime.Must(workflowexecutionv1.AddToScheme(scheme))
	utilruntime.Must(notificationv1.AddToScheme(scheme))
}

func main() {
	var (
		metricsAddr          string
		enableLeaderElection bool
		probeAddr            string
		maxConcurrent        int
	)

	flag.StringVar(&metricsAddr, "metrics-bind-address", ":9183", "The address the metric endpoint binds to.")
	flag.StringVar(&probeAddr, "health-probe-bind-address", ":8081", "The address the probe endpoint binds to.")
	flag.BoolVar(&enableLeaderElection, "leader-elect", false,
		"Enable leader election for controller manager. "+
			"Enabling this will ensure there is only one active controller manager.")
	flag.IntVar(&maxConcurrent, "max-concurrent-reconciles", 10, "Maximum concurrent reconciliations.")

	opts := zap.Options{
		Development: os.Getenv("ENVIRONMENT") != "production",
		TimeEncoder: zapcore.ISO8601TimeEncoder,
	}
	opts.BindFlags(flag.CommandLine)
	flag.Parse()

	ctrl.SetLogger(zap.New(zap.UseFlagOptions(&opts)))

	setupLog.Info("Starting Remediation Orchestrator Controller",
		"metricsAddr", metricsAddr,
		"probeAddr", probeAddr,
		"leaderElection", enableLeaderElection,
		"maxConcurrentReconciles", maxConcurrent)

	mgr, err := ctrl.NewManager(ctrl.GetConfigOrDie(), ctrl.Options{
		Scheme: scheme,
		Metrics: metricsserver.Options{
			BindAddress: metricsAddr,
		},
		HealthProbeBindAddress: probeAddr,
		LeaderElection:         enableLeaderElection,
		LeaderElectionID:       "remediationorchestrator.kubernaut.ai",
	})
	if err != nil {
		setupLog.Error(err, "unable to create manager")
		os.Exit(1)
	}

	// Configure orchestrator
	config := remediationorchestrator.DefaultConfig()
	config.MaxConcurrentReconciles = maxConcurrent

	// Create and register the reconciler
	reconciler := controller.NewReconciler(
		mgr.GetClient(),
		mgr.GetScheme(),
		config,
	)

	if err = reconciler.SetupWithManager(mgr); err != nil {
		setupLog.Error(err, "unable to create controller", "controller", "RemediationOrchestrator")
		os.Exit(1)
	}

	// Add health checks
	if err := mgr.AddHealthzCheck("healthz", healthz.Ping); err != nil {
		setupLog.Error(err, "unable to set up health check")
		os.Exit(1)
	}
	if err := mgr.AddReadyzCheck("readyz", healthz.Ping); err != nil {
		setupLog.Error(err, "unable to set up ready check")
		os.Exit(1)
	}

	setupLog.Info("Starting manager")
	if err := mgr.Start(ctrl.SetupSignalHandler()); err != nil {
		setupLog.Error(err, "problem running manager")
		os.Exit(1)
	}
}

