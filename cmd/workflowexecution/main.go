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
	"context"
	"flag"
	"fmt"
	"os"
	"time"

	tektonv1 "github.com/tektoncd/pipeline/pkg/apis/pipeline/v1"
	"k8s.io/apimachinery/pkg/runtime"
	utilruntime "k8s.io/apimachinery/pkg/util/runtime"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/healthz"
	"sigs.k8s.io/controller-runtime/pkg/log/zap"
	metricsserver "sigs.k8s.io/controller-runtime/pkg/metrics/server"

	workflowexecutionv1alpha1 "github.com/jordigilh/kubernaut/api/workflowexecution/v1alpha1"
	"github.com/jordigilh/kubernaut/internal/controller/workflowexecution"
	//+kubebuilder:scaffold:imports
)

var (
	scheme   = runtime.NewScheme()
	setupLog = ctrl.Log.WithName("setup")
)

func init() {
	utilruntime.Must(clientgoscheme.AddToScheme(scheme))
	utilruntime.Must(workflowexecutionv1alpha1.AddToScheme(scheme))
	utilruntime.Must(tektonv1.AddToScheme(scheme))
	//+kubebuilder:scaffold:scheme
}

func main() {
	var metricsAddr string
	var probeAddr string
	var enableLeaderElection bool
	var executionNamespace string
	var cooldownPeriodMinutes int

	flag.StringVar(&metricsAddr, "metrics-bind-address", ":9090", "The address the metric endpoint binds to.")
	flag.StringVar(&probeAddr, "health-probe-bind-address", ":8081", "The address the probe endpoint binds to.")
	flag.BoolVar(&enableLeaderElection, "leader-elect", false,
		"Enable leader election for controller manager. "+
			"Enabling this will ensure there is only one active controller manager.")
	flag.StringVar(&executionNamespace, "execution-namespace", "kubernaut-workflows",
		"The namespace where PipelineRuns are created (DD-WE-002)")
	flag.IntVar(&cooldownPeriodMinutes, "cooldown-period", 5,
		"Cooldown period in minutes between workflow executions on same target (DD-WE-001)")
	var serviceAccountName string
	flag.StringVar(&serviceAccountName, "service-account", "kubernaut-workflow-runner",
		"ServiceAccount name for PipelineRuns")

	opts := zap.Options{
		Development: true,
	}
	opts.BindFlags(flag.CommandLine)
	flag.Parse()

	ctrl.SetLogger(zap.New(zap.UseFlagOptions(&opts)))

	// ========================================
	// ADR-030: Validate Tekton CRDs are available
	// The controller MUST crash if Tekton is not installed
	// ========================================
	setupLog.Info("Validating Tekton Pipelines availability (ADR-030)")
	if err := checkTektonAvailable(); err != nil {
		setupLog.Error(err, "Required dependency check failed: Tekton Pipelines not available")
		setupLog.Info("Tekton Pipelines CRDs must be installed before starting this controller")
		os.Exit(1)
	}
	setupLog.Info("Tekton Pipelines CRDs verified")

	mgr, err := ctrl.NewManager(ctrl.GetConfigOrDie(), ctrl.Options{
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

	// Log configuration
	setupLog.Info("WorkflowExecution controller configuration",
		"executionNamespace", executionNamespace,
		"cooldownPeriodMinutes", cooldownPeriodMinutes,
		"serviceAccountName", serviceAccountName,
		"metricsAddr", metricsAddr,
		"probeAddr", probeAddr,
	)

	// Setup WorkflowExecution controller
	if err = (&workflowexecution.WorkflowExecutionReconciler{
		Client:             mgr.GetClient(),
		Scheme:             mgr.GetScheme(),
		Recorder:           mgr.GetEventRecorderFor("workflowexecution-controller"),
		ExecutionNamespace: executionNamespace,
		CooldownPeriod:     time.Duration(cooldownPeriodMinutes) * time.Minute,
		ServiceAccountName: serviceAccountName,
	}).SetupWithManager(mgr); err != nil {
		setupLog.Error(err, "unable to create controller", "controller", "WorkflowExecution")
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
	if err := mgr.Start(ctrl.SetupSignalHandler()); err != nil {
		setupLog.Error(err, "problem running manager")
		os.Exit(1)
	}
}

// ========================================
// ADR-030: Tekton CRD Availability Check
// Controller MUST crash if Tekton is not installed
// ========================================

// checkTektonAvailable verifies that Tekton Pipeline CRDs are installed
// Returns error if Tekton CRDs are not available
func checkTektonAvailable() error {
	config := ctrl.GetConfigOrDie()
	k8sClient, err := client.New(config, client.Options{Scheme: scheme})
	if err != nil {
		return fmt.Errorf("failed to create client for Tekton check: %w", err)
	}

	// Try to list PipelineRuns - this verifies the CRD exists
	var prList tektonv1.PipelineRunList
	if err := k8sClient.List(context.Background(), &prList, client.Limit(1)); err != nil {
		return fmt.Errorf("Tekton PipelineRun CRD not available: %w", err)
	}

	return nil
}
