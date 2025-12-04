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

// Package main is the entry point for the WorkflowExecution controller
// Ports: Health=8081, Metrics=9090 (per DD-TEST-001)
package main

import (
	"flag"
	"fmt"
	"os"

	workflowexecutionv1 "github.com/jordigilh/kubernaut/api/workflowexecution/v1alpha1"
	"github.com/jordigilh/kubernaut/internal/controller/workflowexecution"
	tektonv1 "github.com/tektoncd/pipeline/pkg/apis/pipeline/v1"

	"k8s.io/apimachinery/pkg/runtime"
	utilruntime "k8s.io/apimachinery/pkg/util/runtime"
	"k8s.io/client-go/discovery"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	"k8s.io/client-go/rest"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/healthz"
	"sigs.k8s.io/controller-runtime/pkg/log/zap"
	metricsserver "sigs.k8s.io/controller-runtime/pkg/metrics/server"
)

var (
	scheme   = runtime.NewScheme()
	setupLog = ctrl.Log.WithName("setup")
)

func init() {
	utilruntime.Must(clientgoscheme.AddToScheme(scheme))
	utilruntime.Must(workflowexecutionv1.AddToScheme(scheme))
	utilruntime.Must(tektonv1.AddToScheme(scheme))
}

func main() {
	var metricsAddr string
	var probeAddr string
	var enableLeaderElection bool
	var executionNamespace string

	// Port allocation per DD-TEST-001
	flag.StringVar(&metricsAddr, "metrics-bind-address", ":9090", "The address the metric endpoint binds to.")
	flag.StringVar(&probeAddr, "health-probe-bind-address", ":8081", "The address the probe endpoint binds to.")
	flag.BoolVar(&enableLeaderElection, "leader-elect", false,
		"Enable leader election for controller manager. Enabling this will ensure there is only one active controller manager.")
	flag.StringVar(&executionNamespace, "execution-namespace", "kubernaut-workflows",
		"The namespace where PipelineRuns will be created (DD-WE-002)")

	opts := zap.Options{
		Development: true,
	}
	opts.BindFlags(flag.CommandLine)
	flag.Parse()

	ctrl.SetLogger(zap.New(zap.UseFlagOptions(&opts)))

	// Check if Tekton CRDs are available (ADR-030: crash-if-missing)
	setupLog.Info("checking Tekton availability")
	restConfig := ctrl.GetConfigOrDie()
	if err := checkTektonAvailable(restConfig); err != nil {
		setupLog.Error(err, "Tekton CRDs not available - controller requires Tekton Pipelines to be installed")
		os.Exit(1)
	}
	setupLog.Info("Tekton CRDs available")

	mgr, err := ctrl.NewManager(restConfig, ctrl.Options{
		Scheme: scheme,
		Metrics: metricsserver.Options{
			BindAddress: metricsAddr,
		},
		HealthProbeBindAddress: probeAddr,
		LeaderElection:         enableLeaderElection,
		LeaderElectionID:       "workflowexecution.kubernaut.ai",
	})
	if err != nil {
		setupLog.Error(err, "unable to start manager")
		os.Exit(1)
	}

	// Create metrics
	metrics := workflowexecution.NewMetrics()
	if err := metrics.Register(); err != nil {
		setupLog.Error(err, "unable to register metrics")
		os.Exit(1)
	}

	// Create reconciler
	reconciler := workflowexecution.NewWorkflowExecutionReconciler(
		mgr.GetClient(),
		mgr.GetScheme(),
		executionNamespace,
	)

	if err = reconciler.SetupWithManager(mgr); err != nil {
		setupLog.Error(err, "unable to create controller", "controller", "WorkflowExecution")
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

	setupLog.Info("starting manager",
		"metricsAddr", metricsAddr,
		"probeAddr", probeAddr,
		"executionNamespace", executionNamespace,
	)
	if err := mgr.Start(ctrl.SetupSignalHandler()); err != nil {
		setupLog.Error(err, "problem running manager")
		os.Exit(1)
	}
}

// checkTektonAvailable verifies that Tekton CRDs are installed
// Per ADR-030: Controller should crash at startup if required dependencies are missing
func checkTektonAvailable(cfg *rest.Config) error {
	// Use discovery client to check if Tekton CRDs exist
	discoveryClient, err := discovery.NewDiscoveryClientForConfig(cfg)
	if err != nil {
		return err
	}

	// Check for PipelineRun resource
	_, resources, err := discoveryClient.ServerGroupsAndResources()
	if err != nil {
		return err
	}

	for _, resourceList := range resources {
		for _, resource := range resourceList.APIResources {
			if resource.Kind == "PipelineRun" {
				return nil // Tekton is available
			}
		}
	}

	return fmt.Errorf("Tekton PipelineRun CRD not found - please install Tekton Pipelines")
}

