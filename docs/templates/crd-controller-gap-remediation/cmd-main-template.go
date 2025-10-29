// Generic main.go template for CRD controllers
//
// CUSTOMIZATION INSTRUCTIONS:
// 1. Replace {{CONTROLLER_NAME}} with your controller name (e.g., remediationprocessor)
// 2. Replace {{PACKAGE_PATH}} with full package path
// 3. Replace {{CRD_GROUP}}/{{CRD_VERSION}}/{{CRD_KIND}} with your CRD details
// 4. Update import paths for your controller
// 5. Uncomment and customize the TODO sections after creating packages
//
// Example:
//
//	Controller Name: remediationprocessor
//	Package: github.com/jordigilh/kubernaut/pkg/remediationprocessor
//	CRD: remediation.kubernaut.io/v1alpha1/RemediationProcessing
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
	// {{CONTROLLER_NAME}} imports
	// TODO: Uncomment and customize after creating packages
	// {{CONTROLLER_NAME}}v1alpha1 "{{PACKAGE_PATH}}/api/{{CONTROLLER_NAME}}/v1alpha1"
	// "{{PACKAGE_PATH}}/pkg/{{CONTROLLER_NAME}}/config"
	// "{{PACKAGE_PATH}}/pkg/{{CONTROLLER_NAME}}/controllers"
)

var (
	scheme   = runtime.NewScheme()
	setupLog = ctrl.Log.WithName("setup")
)

func init() {
	utilruntime.Must(clientgoscheme.AddToScheme(scheme))

	// TODO: Uncomment after creating CRD API package
	// utilruntime.Must({{CONTROLLER_NAME}}v1alpha1.AddToScheme(scheme))
}

func main() {
	var configFile string
	var metricsAddr string
	var enableLeaderElection bool
	var probeAddr string

	flag.StringVar(&configFile, "config", "/etc/{{CONTROLLER_NAME}}/config.yaml", "Path to configuration file")
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

	setupLog.Info("starting {{CONTROLLER_NAME}} controller",
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
		LeaderElectionID:       "{{CONTROLLER_NAME}}.kubernaut.io",
		// TODO: Add namespace configuration
		// LeaderElectionNamespace: cfg.Namespace,
	})
	if err != nil {
		setupLog.Error(err, "unable to start manager")
		os.Exit(1)
	}

	// Setup reconciler
	// TODO: Uncomment after creating controllers package
	// if err = (&controllers.{{CRD_KIND}}Reconciler{
	// 	Client: mgr.GetClient(),
	// 	Scheme: mgr.GetScheme(),
	// 	Config: cfg,
	// }).SetupWithManager(mgr); err != nil {
	// 	setupLog.Error(err, "unable to create controller", "controller", "{{CRD_KIND}}")
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
