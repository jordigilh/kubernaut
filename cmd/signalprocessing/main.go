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

// ========================================
// SIGNAL PROCESSING CONTROLLER (DD-006)
// ========================================
//
// 📋 Design Decisions:
//   - DD-006: Controller Scaffolding Strategy
//   - DD-005: Observability Standards (Metrics & Logging)
//   - DD-CRD-001: API Group Domain (.kubernaut.ai)
//   - DD-014: Binary Version Logging
//
// 🎯 Business Requirements:
//   - BR-SP-001: K8s Context Enrichment
//   - BR-SP-070: Priority Assignment (Rego)
//   - BR-SP-090: Categorization Audit Trail
//
// ========================================
package main

import (
	"flag"
	"net/http"
	"os"
	"time"

	// Standard Kubernetes imports
	"k8s.io/apimachinery/pkg/runtime"
	utilruntime "k8s.io/apimachinery/pkg/util/runtime"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/healthz"
	"sigs.k8s.io/controller-runtime/pkg/log/zap"
	metricsserver "sigs.k8s.io/controller-runtime/pkg/metrics/server"

	// SignalProcessing imports
	signalprocessingv1alpha1 "github.com/jordigilh/kubernaut/api/signalprocessing/v1alpha1"
	"github.com/jordigilh/kubernaut/internal/controller/signalprocessing"
	sharedaudit "github.com/jordigilh/kubernaut/pkg/audit"
	"github.com/jordigilh/kubernaut/pkg/signalprocessing/audit"
	"github.com/jordigilh/kubernaut/pkg/signalprocessing/config"
)

var (
	scheme   = runtime.NewScheme()
	setupLog = ctrl.Log.WithName("setup")

	// Version information (DD-014: Binary Version Logging)
	// Set via -ldflags at build time
	version   = "dev"
	gitCommit = "unknown"
	buildDate = "unknown"
)

func init() {
	utilruntime.Must(clientgoscheme.AddToScheme(scheme))
	utilruntime.Must(signalprocessingv1alpha1.AddToScheme(scheme))
}

func main() {
	var configFile string

	// Controller config from DefaultControllerConfig() - NOT from flags for safety
	ctrlCfg := config.DefaultControllerConfig()

	flag.StringVar(&configFile, "config", "/etc/signalprocessing/config.yaml", "Path to configuration file")

	opts := zap.Options{
		Development: true,
	}
	opts.BindFlags(flag.CommandLine)
	flag.Parse()

	ctrl.SetLogger(zap.New(zap.UseFlagOptions(&opts)))

	// DD-014: Log version information on startup
	setupLog.Info("starting signalprocessing controller",
		"version", version,
		"gitCommit", gitCommit,
		"buildDate", buildDate,
	)

	// Load configuration
	cfg, err := config.LoadFromFile(configFile)
	if err != nil {
		// In development/testing, use defaults if config file not found
		setupLog.Info("config file not found, using defaults", "path", configFile, "error", err.Error())
		cfg = &config.Config{} // Will fail validation - that's intentional
	}

	// Validate configuration (skip in development if empty)
	if cfg.Enrichment.Timeout > 0 {
		if err := cfg.Validate(); err != nil {
			setupLog.Error(err, "invalid configuration")
			os.Exit(1)
		}
	}

	setupLog.Info("configuration loaded",
		"config", configFile,
		"metrics-address", ctrlCfg.MetricsAddr,
		"probe-address", ctrlCfg.HealthProbeAddr,
		"leader-election", ctrlCfg.LeaderElection,
	)

	// Setup controller manager
	mgr, err := ctrl.NewManager(ctrl.GetConfigOrDie(), ctrl.Options{
		Scheme: scheme,
		Metrics: metricsserver.Options{
			BindAddress: ctrlCfg.MetricsAddr,
		},
		HealthProbeBindAddress: ctrlCfg.HealthProbeAddr,
		LeaderElection:         ctrlCfg.LeaderElection,
		LeaderElectionID:       ctrlCfg.LeaderElectionID,
	})
	if err != nil {
		setupLog.Error(err, "unable to start manager")
		os.Exit(1)
	}

	// ========================================
	// AUDIT CLIENT SETUP (MANDATORY per ADR-032)
	// ========================================
	// ADR-032: Audit is MANDATORY - controller will crash if not configured
	// ADR-038: Fire-and-forget pattern via BufferedStore
	dataStorageURL := os.Getenv("DATA_STORAGE_URL")
	if dataStorageURL == "" {
		dataStorageURL = "http://datastorage-service:8080" // Default for in-cluster
	}
	setupLog.Info("configuring audit client", "dataStorageURL", dataStorageURL)

	httpClient := &http.Client{Timeout: 5 * time.Second}
	dsClient := sharedaudit.NewHTTPDataStorageClient(dataStorageURL, httpClient)

	auditStore, err := sharedaudit.NewBufferedStore(
		dsClient,
		sharedaudit.DefaultConfig(),
		"signalprocessing",
		ctrl.Log.WithName("audit"),
	)
	if err != nil {
		setupLog.Error(err, "FATAL: failed to create audit store - audit is MANDATORY per ADR-032")
		os.Exit(1)
	}

	// Create service-specific audit client (BR-SP-090)
	auditClient := audit.NewAuditClient(auditStore, ctrl.Log.WithName("audit"))
	setupLog.Info("audit client configured successfully")

	// Setup reconciler with MANDATORY audit client
	if err = (&signalprocessing.SignalProcessingReconciler{
		Client:      mgr.GetClient(),
		Scheme:      mgr.GetScheme(),
		AuditClient: auditClient, // MANDATORY per ADR-032
	}).SetupWithManager(mgr); err != nil {
		setupLog.Error(err, "unable to create controller", "controller", "SignalProcessing")
		os.Exit(1)
	}

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

