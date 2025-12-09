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
	"net/http"
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
	"github.com/jordigilh/kubernaut/pkg/audit"
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

	// ========================================
	// DD-WE-004: Exponential Backoff Configuration (BR-WE-012)
	// ========================================
	var baseCooldownSeconds int
	var maxCooldownMinutes int
	var maxBackoffExponent int
	var maxConsecutiveFailures int
	flag.IntVar(&baseCooldownSeconds, "base-cooldown-seconds", 60,
		"Base cooldown in seconds for exponential backoff (DD-WE-004)")
	flag.IntVar(&maxCooldownMinutes, "max-cooldown-minutes", 10,
		"Maximum cooldown in minutes (caps exponential backoff, DD-WE-004)")
	flag.IntVar(&maxBackoffExponent, "max-backoff-exponent", 4,
		"Maximum exponent for backoff calculation (2^n multiplier, DD-WE-004)")
	flag.IntVar(&maxConsecutiveFailures, "max-consecutive-failures", 5,
		"Max consecutive pre-execution failures before ExhaustedRetries (DD-WE-004)")

	// ========================================
	// DD-AUDIT-003 P0 MUST: Audit Store Configuration
	// WorkflowExecution (Remediation Execution Controller) MUST generate audit traces
	// ========================================
	var dataStorageURL string
	flag.StringVar(&dataStorageURL, "datastorage-url", "http://datastorage-service:8080",
		"Data Storage Service URL for audit events (DD-AUDIT-003, DD-AUDIT-002)")

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
		"dataStorageURL", dataStorageURL,
		// DD-WE-004: Exponential Backoff Configuration
		"baseCooldownSeconds", baseCooldownSeconds,
		"maxCooldownMinutes", maxCooldownMinutes,
		"maxBackoffExponent", maxBackoffExponent,
		"maxConsecutiveFailures", maxConsecutiveFailures,
	)

	// ========================================
	// DD-AUDIT-003 P0 MUST: Initialize AuditStore
	// Per DD-AUDIT-002: Use pkg/audit/ shared library
	// Per ADR-038: Async buffered audit ingestion
	// ========================================
	setupLog.Info("Initializing audit store (DD-AUDIT-003, DD-AUDIT-002)",
		"dataStorageURL", dataStorageURL,
	)

	// Create HTTP client for Data Storage Service
	httpClient := &http.Client{
		Timeout: 10 * time.Second,
	}
	dsClient := audit.NewHTTPDataStorageClient(dataStorageURL, httpClient)

	// Create buffered audit store using shared library (DD-AUDIT-002)
	// Use recommended config for workflowexecution service
	auditConfig := audit.RecommendedConfig("workflowexecution")
	auditStore, err := audit.NewBufferedStore(
		dsClient,
		auditConfig,
		"workflowexecution",
		ctrl.Log.WithName("audit"),
	)
	if err != nil {
		// Per DD-AUDIT-002: Log error but don't crash - graceful degradation
		// Audit store initialization failure should NOT prevent controller from starting
		// The controller will operate without audit if Data Storage is unavailable
		setupLog.Error(err, "Failed to initialize audit store - controller will operate without audit (graceful degradation)")
		auditStore = nil
	} else {
		setupLog.Info("Audit store initialized successfully",
			"buffer_size", auditConfig.BufferSize,
			"batch_size", auditConfig.BatchSize,
			"flush_interval", auditConfig.FlushInterval,
		)
	}

	// Setup WorkflowExecution controller
	if err = (&workflowexecution.WorkflowExecutionReconciler{
		Client:             mgr.GetClient(),
		Scheme:             mgr.GetScheme(),
		Recorder:           mgr.GetEventRecorderFor("workflowexecution-controller"),
		ExecutionNamespace: executionNamespace,
		CooldownPeriod:     time.Duration(cooldownPeriodMinutes) * time.Minute,
		ServiceAccountName: serviceAccountName,
		AuditStore:         auditStore, // DD-AUDIT-003: Audit store for BR-WE-005
		// DD-WE-004: Exponential Backoff Configuration (BR-WE-012)
		BaseCooldownPeriod:     time.Duration(baseCooldownSeconds) * time.Second,
		MaxCooldownPeriod:      time.Duration(maxCooldownMinutes) * time.Minute,
		MaxBackoffExponent:     maxBackoffExponent,
		MaxConsecutiveFailures: maxConsecutiveFailures,
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

	// ========================================
	// DD-AUDIT-002: Graceful Shutdown - Flush Audit Events
	// Per DD-007: Kubernetes-aware graceful shutdown
	// ========================================
	if auditStore != nil {
		setupLog.Info("Flushing audit events on shutdown (DD-AUDIT-002)")
		if err := auditStore.Close(); err != nil {
			setupLog.Error(err, "Failed to close audit store")
		} else {
			setupLog.Info("Audit store closed successfully")
		}
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
