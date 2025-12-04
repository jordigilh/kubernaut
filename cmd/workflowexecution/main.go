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

	"github.com/go-logr/zapr"
	tektonv1 "github.com/tektoncd/pipeline/pkg/apis/pipeline/v1"
	zaplog "go.uber.org/zap"
	"k8s.io/apimachinery/pkg/api/meta"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	utilruntime "k8s.io/apimachinery/pkg/util/runtime"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/healthz"
	"sigs.k8s.io/controller-runtime/pkg/log/zap"
	metricsserver "sigs.k8s.io/controller-runtime/pkg/metrics/server"

	workflowexecutionv1alpha1 "github.com/jordigilh/kubernaut/api/workflowexecution/v1alpha1"
	"github.com/jordigilh/kubernaut/internal/controller/workflowexecution"
	"github.com/jordigilh/kubernaut/pkg/audit"
	//+kubebuilder:scaffold:imports
)

// ============================================================================
// WorkflowExecution Controller Entry Point
// Version: 1.0 - Tekton Delegation Architecture (ADR-044)
// ============================================================================
//
// Architecture Overview:
// - Creates Tekton PipelineRun from OCI bundle references
// - Watches PipelineRun status and updates WorkflowExecution status
// - Implements resource locking to prevent parallel workflows (DD-WE-001)
// - Writes audit trail for execution lifecycle
//
// Key Design Decisions:
// - ADR-044: Tekton handles step orchestration
// - ADR-030: Crash if critical dependencies missing (Tekton CRDs)
// - DD-WE-002: All PipelineRuns run in dedicated kubernaut-workflows namespace
// - DD-WE-003: Deterministic PipelineRun naming for race condition prevention
// ============================================================================

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
	var enableLeaderElection bool
	var probeAddr string
	var executionNamespace string
	var serviceAccountName string
	var cooldownPeriod time.Duration

	flag.StringVar(&metricsAddr, "metrics-bind-address", ":9090", "The address the metric endpoint binds to.")
	flag.StringVar(&probeAddr, "health-probe-bind-address", ":8081", "The address the probe endpoint binds to.")
	flag.BoolVar(&enableLeaderElection, "leader-elect", false,
		"Enable leader election for controller manager. "+
			"Enabling this will ensure there is only one active controller manager.")
	flag.StringVar(&executionNamespace, "execution-namespace", "kubernaut-workflows",
		"The namespace where Tekton PipelineRuns are created (DD-WE-002).")
	flag.StringVar(&serviceAccountName, "service-account-name", workflowexecution.DefaultServiceAccountName,
		"The ServiceAccount used for PipelineRun execution.")
	flag.DurationVar(&cooldownPeriod, "cooldown-period", workflowexecution.DefaultCooldownPeriod,
		"Duration to prevent redundant sequential executions on the same target (DD-WE-001).")

	opts := zap.Options{
		Development: true,
	}
	opts.BindFlags(flag.CommandLine)
	flag.Parse()

	ctrl.SetLogger(zap.New(zap.UseFlagOptions(&opts)))

	// ========================================
	// DD-014: Binary Version Logging
	// ========================================
	setupLog.Info("Starting WorkflowExecution Controller",
		"version", "1.0.0",
		"executionNamespace", executionNamespace,
		"serviceAccountName", serviceAccountName,
		"cooldownPeriod", cooldownPeriod.String(),
	)

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

	// ========================================
	// ADR-030: Crash if Tekton CRDs not available
	// Critical dependency check at startup
	// ========================================
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	if err := checkTektonAvailable(ctx, mgr.GetRESTMapper()); err != nil {
		setupLog.Error(err, "CRITICAL: Tekton Pipelines CRDs not found. "+
			"WorkflowExecution Controller requires Tekton Pipelines to be installed. "+
			"Please install Tekton Pipelines before starting this controller.",
			"tektonInstallUrl", "https://tekton.dev/docs/pipelines/install/",
		)
		os.Exit(1)
	}
	setupLog.Info("Tekton Pipelines CRDs verified successfully")

	// ========================================
	// Initialize Audit Store (ADR-034)
	// ========================================
	dataStorageURL := os.Getenv("DATA_STORAGE_URL")
	if dataStorageURL == "" {
		dataStorageURL = "http://datastorage-service.kubernaut.svc.cluster.local:8080"
		setupLog.Info("DATA_STORAGE_URL not set, using default", "url", dataStorageURL)
	}

	httpClient := &http.Client{
		Timeout: 5 * time.Second,
	}
	dataStorageClient := audit.NewHTTPDataStorageClient(dataStorageURL, httpClient)

	auditConfig := audit.Config{
		BufferSize:    10000,
		BatchSize:     100,
		FlushInterval: 5 * time.Second,
		MaxRetries:    3,
	}

	zapLogger, err := zaplog.NewProduction()
	if err != nil {
		setupLog.Error(err, "Failed to create zap logger for audit store")
		os.Exit(1)
	}
	auditLogger := zapr.NewLogger(zapLogger.Named("audit"))

	auditStore, err := audit.NewBufferedStore(dataStorageClient, auditConfig, "workflowexecution-controller", auditLogger)
	if err != nil {
		setupLog.Error(err, "Failed to create audit store")
		os.Exit(1)
	}
	setupLog.Info("Audit store initialized",
		"bufferSize", auditConfig.BufferSize,
		"batchSize", auditConfig.BatchSize,
	)

	// ========================================
	// Initialize Metrics (DD-005)
	// ========================================
	workflowexecution.InitMetrics()
	setupLog.Info("WorkflowExecution metrics initialized")

	// ========================================
	// Setup Controller
	// ========================================
	reconciler := &workflowexecution.WorkflowExecutionReconciler{
		Client:             mgr.GetClient(),
		Scheme:             mgr.GetScheme(),
		Recorder:           mgr.GetEventRecorderFor("workflowexecution-controller"),
		ExecutionNamespace: executionNamespace,
		ServiceAccountName: serviceAccountName,
		CooldownPeriod:     cooldownPeriod,
		AuditStore:         auditStore,
	}

	if err = reconciler.SetupWithManager(mgr); err != nil {
		setupLog.Error(err, "unable to create controller", "controller", "WorkflowExecution")
		os.Exit(1)
	}
	//+kubebuilder:scaffold:builder

	// ========================================
	// Health and Ready Checks
	// ========================================
	if err := mgr.AddHealthzCheck("healthz", healthz.Ping); err != nil {
		setupLog.Error(err, "unable to set up health check")
		os.Exit(1)
	}
	if err := mgr.AddReadyzCheck("readyz", healthz.Ping); err != nil {
		setupLog.Error(err, "unable to set up ready check")
		os.Exit(1)
	}

	// ========================================
	// Start Manager
	// ========================================
	setupLog.Info("starting manager")

	sigCtx := ctrl.SetupSignalHandler()

	if err := mgr.Start(sigCtx); err != nil {
		setupLog.Error(err, "problem running manager")
		os.Exit(1)
	}

	// ========================================
	// Graceful Shutdown (DD-007)
	// ========================================
	setupLog.Info("Shutting down WorkflowExecution controller, flushing remaining audit events")
	if err := auditStore.Close(); err != nil {
		setupLog.Error(err, "Failed to close audit store gracefully")
		os.Exit(1)
	}
	setupLog.Info("Audit store closed successfully, all events flushed")
}

// checkTektonAvailable verifies that Tekton Pipeline CRDs are registered
// in the cluster's API server. This is a fail-fast check per ADR-030.
func checkTektonAvailable(ctx context.Context, mapper meta.RESTMapper) error {
	// Check for PipelineRun CRD
	pipelineRunGVK := schema.GroupVersionKind{
		Group:   "tekton.dev",
		Version: "v1",
		Kind:    "PipelineRun",
	}

	_, err := mapper.RESTMapping(pipelineRunGVK.GroupKind(), pipelineRunGVK.Version)
	if err != nil {
		return fmt.Errorf("tekton.dev/v1 PipelineRun CRD not found: %w", err)
	}

	// Check for Pipeline CRD (used for bundle resolution)
	pipelineGVK := schema.GroupVersionKind{
		Group:   "tekton.dev",
		Version: "v1",
		Kind:    "Pipeline",
	}

	_, err = mapper.RESTMapping(pipelineGVK.GroupKind(), pipelineGVK.Version)
	if err != nil {
		return fmt.Errorf("tekton.dev/v1 Pipeline CRD not found: %w", err)
	}

	return nil
}

