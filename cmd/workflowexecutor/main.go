// Package main provides the entry point for the workflowexecutor controller.
package main

import (
	"flag"
	"os"

	// Standard Kubernetes imports
	"k8s.io/apimachinery/pkg/runtime"
	utilruntime "k8s.io/apimachinery/pkg/util/runtime"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/healthz"
	"sigs.k8s.io/controller-runtime/pkg/log/zap"
	metricsserver "sigs.k8s.io/controller-runtime/pkg/metrics/server"

	// workflowexecutor imports
	// TODO: Uncomment and customize after creating packages
	// workflowv1alpha1 "github.com/jordigilh/kubernaut/api/workflowexecution/v1alpha1"
	// "github.com/jordigilh/kubernaut/pkg/workflowexecution/config"
	// "github.com/jordigilh/kubernaut/pkg/workflowexecution/controllers"
)

var (
	scheme   = runtime.NewScheme()
	setupLog = ctrl.Log.WithName("setup")
)

func init() {
	utilruntime.Must(clientgoscheme.AddToScheme(scheme))

	// TODO: Uncomment after creating CRD API package
	// utilruntime.Must(workflowv1alpha1.AddToScheme(scheme))
}

func main() {
	var configFile string
	var metricsAddr string
	var enableLeaderElection bool
	var probeAddr string

	flag.StringVar(&configFile, "config", "/etc/workflowexecutor/config.yaml", "Path to configuration file")
	flag.StringVar(&metricsAddr, "metrics-bind-address", ":8080", "The address the metric endpoint binds to.")
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

	// Load configuration
	// TODO: Uncomment after creating config package
	// cfg, err := config.LoadConfig(configFile)
	// if err != nil {
	// 	setupLog.Error(err, "unable to load configuration")
	// 	os.Exit(1)
	// }

	// Override with environment variables
	// TODO: Uncomment after creating config package
	// if err := cfg.LoadFromEnv(); err != nil {
	// 	setupLog.Error(err, "unable to load environment overrides")
	// 	os.Exit(1)
	// }

	// Validate configuration
	// TODO: Uncomment after creating config package
	// if err := cfg.Validate(); err != nil {
	// 	setupLog.Error(err, "invalid configuration")
	// 	os.Exit(1)
	// }

	setupLog.Info("starting workflowexecutor controller",
		"config", configFile,
		"metrics-address", metricsAddr,
		"probe-address", probeAddr,
		"leader-election", enableLeaderElection,
	)

	// Setup controller manager
	mgr, err := ctrl.NewManager(ctrl.GetConfigOrDie(), ctrl.Options{
		Scheme: scheme,
		Metrics: metricsserver.Options{
			BindAddress: metricsAddr,
		},
		HealthProbeBindAddress: probeAddr,
		LeaderElection:         enableLeaderElection,
		LeaderElectionID:       "workflowexecutor.kubernaut.io",
		// TODO: Add namespace configuration
		// LeaderElectionNamespace: cfg.Namespace,
	})
	if err != nil {
		setupLog.Error(err, "unable to start manager")
		os.Exit(1)
	}

	// Setup reconciler
	// TODO: Uncomment after creating controllers package
	// if err = (&controllers.WorkflowExecutionReconciler{
	// 	Client: mgr.GetClient(),
	// 	Scheme: mgr.GetScheme(),
	// 	Config: cfg,
	// }).SetupWithManager(mgr); err != nil {
	// 	setupLog.Error(err, "unable to create controller", "controller", "WorkflowExecution")
	// 	os.Exit(1)
	// }

	// Setup health checks
	if err := mgr.AddHealthzCheck("healthz", healthz.Ping); err != nil {
		setupLog.Error(err, "unable to set up health check")
		os.Exit(1)
	}
	if err := mgr.AddReadyzCheck("readyz", healthz.Ping); err != nil {
		setupLog.Error(err, "unable to set up ready check")
		os.Exit(1)
	}

	setupLog.Info("starting manager")
	if err := mgr.Start(ctrl.SetupSignalHandler()); err != nil {
		setupLog.Error(err, "problem running manager")
		os.Exit(1)
	}
}

